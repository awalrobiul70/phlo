package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"phlo/internal/config"
	"phlo/internal/errs"
	"phlo/internal/folders"
	"phlo/internal/paths"
	phloruntime "phlo/internal/runtime"
	"phlo/internal/site"
)

// reservedTop are Phlo's own folders at the root. Projects and databases cannot live inside them.
var reservedTop = []string{"config", "runtime", "var", "bin", "cmd", "internal", "ui", "docs", ".ai", ".git", ".cursor"}

// SetHomeDir points Phlo at another projects folder. With copy, the current projects are copied into
// the new folder, which must be empty. The current folder is only read, never changed or removed.
func (a *App) SetHomeDir(value string, copyFiles bool) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	target, err := a.folderPath(value, a.Layout.DefaultHome())
	if err != nil {
		return "", err
	}
	current := a.Layout.Home()
	if paths.Same(target, current) {
		return "", nil
	}
	if err := a.checkPlacement(target, "projects folder"); err != nil {
		return "", err
	}
	if paths.Overlap(target, current) {
		return "", errs.New("The new projects folder cannot be inside " + current + " or contain it. Choose a folder next to it instead.")
	}
	for _, dir := range a.databaseDirs() {
		if paths.Overlap(target, dir) {
			return "", errs.New("The projects folder cannot share a folder with the database folder " + dir + ".")
		}
	}
	var copied folders.Copied
	if copyFiles {
		if err := requireEmpty(target, "projects"); err != nil {
			return "", err
		}
		if exists(current) {
			if copied, err = folders.Copy(current, target); err != nil {
				return "", err
			}
		}
	} else if err := a.checkProjects(target); err != nil {
		return "", err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		_ = copied.Undo()
		return "", err
	}
	prevConfig, prevLayout := a.Config, a.Layout
	revert := func() {
		a.Config, a.Layout = prevConfig, prevLayout
		a.Supervisor.SetLayout(a.Layout)
		_ = copied.Undo()
	}
	a.Config.HomeDir = a.Layout.ToConfig(target, a.Layout.DefaultHome())
	a.Layout = a.Layout.WithFolders(folderValues(a.Config))
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		revert()
		return "", err
	}
	a.Supervisor.SetLayout(a.Layout)
	if a.Supervisor.Running() {
		if err := a.Supervisor.Reload(); err != nil {
			revert()
			_ = config.Save(a.Layout.ConfigFile(), a.Config)
			_ = a.Supervisor.Reload()
			return "", errs.New(err.Error() + " Phlo kept using " + current + ".")
		}
	}
	hostsWarn, err := a.syncHosts()
	if err != nil {
		return "", err
	}
	return joinWarnings(skippedWarning(copied.Skipped), hostsWarn), nil
}

// dataFolder is a database server whose data folder Settings can move.
type dataFolder struct {
	label  string
	dir    func(paths.Layout) string
	stored func(*config.Config) *string
}

var dataFolders = map[string]dataFolder{
	"mariadb":  {"MariaDB", paths.Layout.DataDir, func(c *config.Config) *string { return &c.MariaDBDataDir }},
	"postgres": {"PostgreSQL", paths.Layout.PostgresData, func(c *config.Config) *string { return &c.PostgresDataDir }},
	"mongodb":  {"MongoDB", paths.Layout.MongoDBData, func(c *config.Config) *string { return &c.MongoDBDataDir }},
}

// SetDataDir moves the data folder of one database: mariadb, postgres, or mongodb.
// If that server is running it is stopped first and started again on the new folder; the others keep running.
// With copy, the current folder is copied into the new one, which must be empty. The old folder stays as it is.
func (a *App) SetDataDir(name, value string, copyFiles bool) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	db, ok := dataFolders[name]
	if !ok {
		return "", errs.New("Unknown database " + name + ". Use mariadb, postgres, or mongodb.")
	}
	defaultPath := db.dir(paths.Layout{Root: a.Layout.Root})
	target, err := a.folderPath(value, defaultPath)
	if err != nil {
		return "", err
	}
	current := db.dir(a.Layout)
	if paths.Same(target, current) {
		return "", nil
	}
	what := db.label + " data folder"
	if err := a.checkPlacement(target, what); err != nil {
		return "", err
	}
	if paths.Overlap(target, a.Layout.Home()) {
		return "", errs.New("The " + what + " cannot share a folder with the projects folder " + a.Layout.Home() + ".")
	}
	if paths.Overlap(target, current) {
		return "", errs.New("The new " + what + " cannot be inside " + current + " or contain it. Choose a folder next to it instead.")
	}
	for other, item := range dataFolders {
		if dir := item.dir(a.Layout); other != name && paths.Overlap(target, dir) {
			return "", errs.New("The " + what + " cannot share a folder with the " + item.label + " data folder " + dir + ".")
		}
	}
	if copyFiles {
		if err := requireEmpty(target, "databases"); err != nil {
			return "", err
		}
	}
	running := a.databaseRunning(name)
	restart := func() {
		if running {
			_ = a.startDatabase(name)
		}
	}
	if running {
		if err := a.Supervisor.StopAddon(name); err != nil {
			restart()
			return "", err
		}
	}
	var copied folders.Copied
	if copyFiles && exists(current) {
		if copied, err = folders.Copy(current, target); err != nil {
			restart()
			return "", err
		}
	}
	prevConfig, prevLayout := a.Config, a.Layout
	revert := func() {
		a.Config, a.Layout = prevConfig, prevLayout
		a.Supervisor.SetLayout(a.Layout)
		_ = copied.Undo()
	}
	*db.stored(&a.Config) = a.Layout.ToConfig(target, defaultPath)
	a.Layout = a.Layout.WithFolders(folderValues(a.Config))
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		revert()
		restart()
		return "", err
	}
	a.Supervisor.SetLayout(a.Layout)
	if running {
		if err := a.startDatabase(name); err != nil {
			_ = a.Supervisor.StopAddon(name)
			revert()
			_ = config.Save(a.Layout.ConfigFile(), a.Config)
			restart()
			return "", errs.New(err.Error() + " Phlo kept using " + current + ".")
		}
	}
	return skippedWarning(copied.Skipped), nil
}

func folderValues(c config.Config) paths.Folders {
	return paths.Folders{Home: c.HomeDir, MariaDB: c.MariaDBDataDir, Postgres: c.PostgresDataDir, MongoDB: c.MongoDBDataDir}
}

// folderPath resolves a folder typed in Settings. Empty means the default folder.
func (a *App) folderPath(value, defaultPath string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultPath, nil
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return "", errs.New("Type the folder path on one line.")
	}
	native := filepath.FromSlash(value)
	if filepath.VolumeName(native) != "" && !filepath.IsAbs(native) {
		return "", errs.New("Use a full path such as D:\\www, or a folder name such as www for a folder inside Phlo.")
	}
	return a.Layout.FromConfig(value), nil
}

// checkPlacement refuses folders that Nginx, PHP, or PostgreSQL cannot open, and Phlo's own folders.
func (a *App) checkPlacement(path, what string) error {
	root := a.Layout.Root
	if paths.Within(root, path) {
		return errs.New("The " + what + " cannot be the Phlo folder " + root + " or a folder that contains it.")
	}
	if paths.Within(path, root) {
		if rel, err := filepath.Rel(root, path); err == nil {
			top := strings.Split(filepath.ToSlash(rel), "/")[0]
			for _, name := range reservedTop {
				if strings.EqualFold(top, name) {
					return errs.New("The " + what + " cannot be inside Phlo's " + name + " folder.")
				}
			}
		}
	}
	if strings.ContainsAny(path, "$\"") {
		return errs.New("The " + what + " path cannot contain $ or \". Nginx and MariaDB cannot read it.")
	}
	if runtime.GOOS == "windows" {
		if part := paths.NonASCIIPart(path); part != "" {
			return errs.New(fmt.Sprintf("The %s cannot be %s because the folder name \"%s\" has non-English characters. "+
				"On Windows, Nginx, PHP, and PostgreSQL cannot open a path like that. Use only English letters, numbers, spaces, - or _.", what, path, part))
		}
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return errs.New(path + " is a file, not a folder.")
	}
	return nil
}

// checkProjects makes sure Phlo can serve the projects that are already in dir.
func (a *App) checkProjects(dir string) error {
	sites, err := site.Scan(dir, site.Options{TLD: a.Config.TLD, DefaultPHP: a.Config.DefaultPHP})
	if err != nil {
		return errs.New("Phlo cannot use " + dir + ": " + err.Error() + ". Each folder in it must be a project named with lowercase letters, numbers, and hyphens.")
	}
	if err := site.Duplicate(sites); err != nil {
		return errs.New("Phlo cannot use " + dir + ": " + err.Error())
	}
	for _, item := range sites {
		if site.ReservesPHPMyAdmin(item, a.Config.TLD) {
			return errs.New("Phlo cannot use " + dir + ": " + site.PHPMyAdminHost(a.Config.TLD) + " is reserved for phpMyAdmin.")
		}
		roots := append([]site.Root(nil), item.Subdomains...)
		if item.PublicHTML != nil {
			roots = append(roots, *item.PublicHTML)
		}
		for _, root := range roots {
			if _, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, root.PHP, runtime.GOOS, runtime.GOARCH); err != nil {
				return errs.New(fmt.Sprintf("%s in %s uses PHP %s, which is not installed. Download PHP %s first.", root.Host, dir, root.PHP, root.PHP))
			}
		}
	}
	return nil
}

func (a *App) databaseDirs() []string {
	return []string{a.Layout.DataDir(), a.Layout.PostgresData(), a.Layout.MongoDBData()}
}

func (a *App) databaseRunning(name string) bool {
	if name == "mariadb" {
		return a.Supervisor.DatabaseRunning()
	}
	return a.Supervisor.AddonRunning(name)
}

func (a *App) startDatabase(name string) error {
	if name == "mariadb" {
		return a.Supervisor.StartMariaDB()
	}
	return a.Supervisor.StartAddon(name)
}

func requireEmpty(dir, what string) error {
	empty, err := folders.Empty(dir)
	if err != nil {
		return errs.New(err.Error())
	}
	if !empty {
		return errs.New(dir + " already has files. Choose an empty folder, or turn off Copy to use the " + what + " that are already there.")
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

func skippedWarning(skipped []string) string {
	if len(skipped) == 0 {
		return ""
	}
	shown := skipped
	more := ""
	if len(shown) > 5 {
		more = fmt.Sprintf(" and %d more", len(shown)-5)
		shown = shown[:5]
	}
	return "These links were not copied: " + strings.Join(shown, ", ") + more +
		". Create them again in the new folder, for example with php artisan storage:link."
}

func joinWarnings(items ...string) string {
	var out []string
	for _, item := range items {
		if item != "" {
			out = append(out, item)
		}
	}
	return strings.Join(out, " ")
}
