package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"phlo/internal/api"
	"phlo/internal/cronical"
	"phlo/internal/errs"
	"phlo/internal/mariadb"
	"phlo/internal/mongodb"
	"phlo/internal/php"
	"phlo/internal/postgres"
	"phlo/internal/proc"
	phloruntime "phlo/internal/runtime"
	"phlo/internal/supervisor"
)

const maxConfigFile = 64 << 10

var phpLineRe = regexp.MustCompile(` on line ([0-9]+)`)

// configKinds are the settings files the Runtimes editors open, keyed by the API kind.
var configKinds = map[string]struct {
	label   string
	file    string
	comment string
	hint    string
	example string
}{
	"php": {
		label: "PHP", file: "php.ini", comment: ";",
		hint: "Your lines go after the php.ini Phlo writes, so they win. Phlo checks them with the PHP CLI before it saves, " +
			"then restarts PHP. They apply to every site on this PHP version.",
		example: "memory_limit = 1G\nupload_max_filesize = 256M\npost_max_size = 256M\nmax_execution_time = 300",
	},
	"mariadb": {
		label: "MariaDB", file: "my.ini", comment: "#",
		hint: "Your lines go into [mysqld] after Phlo's settings, so they win. Phlo sets bind-address, port, datadir, basedir, pid-file, " +
			"and log-error itself. Saving restarts MariaDB if it is running, and puts the previous file back if MariaDB does not start. " +
			"They apply to every MariaDB version.",
		example: "max_connections = 300\ninnodb_buffer_pool_size = 512M\nmax_allowed_packet = 256M",
	},
	"postgres": {
		label: "PostgreSQL", file: "postgresql.conf", comment: "#",
		hint: "Write name = value lines, as in postgresql.conf. Phlo passes them to PostgreSQL as -c options, so they win over " +
			"postgresql.conf in the data folder. Phlo sets listen_addresses, port, data_directory, and unix_socket_directories itself. " +
			"Saving restarts PostgreSQL if it is running, and puts the previous file back if it does not start. They apply to every PostgreSQL version.",
		example: "shared_buffers = 256MB\nwork_mem = '16MB'\nlog_min_duration_statement = 500",
	},
	"mongodb": {
		label: "MongoDB", file: "mongod.conf", comment: "#",
		hint: "YAML indented with spaces, as in mongod.conf. Phlo starts mongod with --config set to this file. Phlo sets net.bindIp, " +
			"net.port, and storage.dbPath itself. Saving restarts MongoDB if it is running, and puts the previous file back if it does not start. " +
			"They apply to every MongoDB version.",
		example: "operationProfiling:\n  mode: slowOp\n  slowOpThresholdMs: 200",
	},
	"cronical": {
		label: "Cronical", file: "cronical.dat", comment: "#",
		hint: "Jobs for Cronical. Phlo runs Cronical.exe --console with this file. It does not install a Windows service. " +
			"php on PATH is the Phlo default PHP. Saving restarts Cronical when it is running.",
		example: "# @hourly php -r \"echo 'hourly';\"",
	},
}

// ConfigFile returns a custom settings file and the settings Phlo writes before it.
func (a *App) ConfigFile(kind, version string) (api.ConfigFile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	file, version, err := a.customFile(kind, version)
	if err != nil {
		return api.ConfigFile{}, err
	}
	custom, err := supervisor.ReadCustom(file)
	if err != nil {
		return api.ConfigFile{}, err
	}
	generated, err := a.generatedConfig(kind, version)
	if err != nil {
		return api.ConfigFile{}, err
	}
	info := configKinds[kind]
	title := info.label + " " + info.file
	if kind == "php" {
		title = "PHP " + version + " php.ini"
	}
	return api.ConfigFile{
		Kind:      kind,
		Version:   version,
		Title:     title,
		File:      file,
		Hint:      info.hint,
		Example:   info.example,
		Custom:    custom,
		Generated: generated,
	}, nil
}

// SaveConfigFile checks and writes a custom settings file, then restarts what uses it.
// When the service does not start, the previous file is put back and started again.
func (a *App) SaveConfigFile(kind, version, content string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	file, version, err := a.customFile(kind, version)
	if err != nil {
		return "", err
	}
	if len(content) > maxConfigFile {
		return "", errs.New("The settings are larger than 64 KB.")
	}
	if strings.ContainsRune(content, 0) || !utf8.ValidString(content) {
		return "", errs.New("The settings must be plain UTF-8 text.")
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		content = ""
	} else {
		content = strings.TrimRight(content, "\n") + "\n"
	}
	if err := a.checkCustom(kind, version, file, content); err != nil {
		return "", err
	}
	old, existed, err := readExisting(file)
	if err != nil {
		return "", err
	}
	if err := writeCustom(file, content); err != nil {
		return "", err
	}
	label := configKinds[kind].label
	if kind == "php" {
		label = "PHP " + version
	}
	running := a.customRunning(kind)
	if err := a.applyCustom(kind, running); err != nil {
		if restoreErr := restoreCustom(file, old, existed); restoreErr != nil {
			return "", fmt.Errorf("%s did not start with these settings, and the previous file could not be put back: %v", label, restoreErr)
		}
		problem := startupProblem(err)
		if again := a.applyCustom(kind, running); again != nil {
			return "", errs.New(fmt.Sprintf("%s did not start with these settings. Phlo put the previous file back, but %s did not start with it either.\n%s\n%s",
				label, label, problem, startupProblem(again)))
		}
		return "", errs.New(fmt.Sprintf("%s did not start with these settings, so Phlo put the previous file back and started it again.\n%s", label, problem))
	}
	switch {
	case running:
		return "Saved. " + label + " restarted with these settings.", nil
	case kind == "php":
		return "Saved. " + label + " uses these settings when Phlo starts.", nil
	default:
		return "Saved. " + label + " uses these settings the next time it starts.", nil
	}
}

func (a *App) customFile(kind, version string) (string, string, error) {
	switch kind {
	case "php":
		if php.Accept(version) != nil {
			return "", "", errs.New("php version must look like 8.3")
		}
		if _, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
			return "", "", errs.New("PHP " + version + " is not installed. Download it in Runtimes first.")
		}
		return a.Layout.CustomPHPIni(version), version, nil
	case "mariadb":
		return a.Layout.CustomMariaDB(), "", nil
	case "postgres":
		return a.Layout.CustomPostgres(), "", nil
	case "mongodb":
		return a.Layout.CustomMongoDB(), "", nil
	case "cronical":
		if runtime.GOOS != "windows" {
			return "", "", errs.New("Cronical runs on Windows. On Linux, use the system crontab.")
		}
		phpExe := ""
		if cli, cliErr := phloruntime.ResolveCLI(a.Manifest, a.Layout, a.Config.DefaultPHP); cliErr == nil {
			phpExe = cli
		}
		if err := cronical.Seed(a.Layout.CronicalDat(), phpExe); err != nil {
			return "", "", err
		}
		return a.Layout.CronicalDat(), "", nil
	}
	return "", "", errs.New("unknown settings file " + kind + ". Use php, mariadb, postgres, mongodb, or cronical.")
}

func (a *App) generatedConfig(kind, version string) (string, error) {
	switch kind {
	case "php":
		return a.Supervisor.PHPConfig(version)
	case "mariadb":
		body, err := a.Supervisor.MariaDBConfig()
		if err != nil {
			return "# MariaDB is not installed. Download it in Runtimes; Phlo writes my.ini when MariaDB starts.\n", nil
		}
		return body, nil
	case "postgres":
		return "# Phlo starts postgres with these options. Everything else comes from postgresql.conf in the data folder.\n" +
			argLines(postgres.ServerArgs(a.Layout.PostgresData(), a.Config.PostgresListenPort(), nil)), nil
	case "cronical":
		return "# Phlo starts Cronical.exe --console -c this file.\n# It does not install a Windows service.\n", nil
	default:
		return "# Phlo starts mongod with these options. They win over the same keys in mongod.conf.\n" +
			argLines(mongodb.ServerArgs(a.Layout.MongoDBData(), a.Config.MongoDBListenPort(), "")), nil
	}
}

// argLines prints command-line options one per line, each flag with its value.
func argLines(args []string) string {
	var b strings.Builder
	for i := 0; i < len(args); i++ {
		b.WriteString(args[i])
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			b.WriteString(" " + args[i+1])
			i++
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (a *App) checkCustom(kind, version, file, content string) error {
	var err error
	switch kind {
	case "php":
		return a.checkPHPSettings(version, file, content)
	case "mariadb":
		err = mariadb.CheckCustom(content)
	case "postgres":
		_, err = postgres.CustomArgs(content)
	case "mongodb":
		err = mongodb.CheckCustom(content)
	case "cronical":
		return nil
	}
	if err != nil {
		return errs.New(err.Error())
	}
	return nil
}

// checkPHPSettings runs the PHP CLI with Phlo's php.ini and with the custom lines added,
// and refuses the lines when PHP reports a problem the generated file alone does not have.
func (a *App) checkPHPSettings(version, file, content string) error {
	if err := php.CheckCustom(content); err != nil {
		return errs.New(err.Error())
	}
	if content == "" {
		return nil
	}
	cli, err := phloruntime.ResolveCLI(a.Manifest, a.Layout, version)
	if err != nil {
		return nil
	}
	generated, err := a.Supervisor.PHPConfig(version)
	if err != nil {
		return err
	}
	work := a.Layout.PHPWork(version)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	basePath := filepath.Join(work, "check-base.ini")
	fullPath := filepath.Join(work, "check.ini")
	defer os.Remove(basePath)
	defer os.Remove(fullPath)
	prefix := generated + supervisor.CustomHeader(";", file)
	if err := os.WriteFile(basePath, []byte(generated), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(fullPath, []byte(prefix+content), 0o644); err != nil {
		return err
	}
	before := phpStartupLines(cli, basePath)
	offset := strings.Count(prefix, "\n")
	var problems []string
	for _, line := range phpStartupLines(cli, fullPath) {
		if slices.Contains(before, line) || slices.Contains(problems, line) {
			continue
		}
		problems = append(problems, line)
	}
	if len(problems) == 0 {
		return nil
	}
	for i, line := range problems {
		line = strings.ReplaceAll(line, "{ini}", "your settings")
		problems[i] = phpLineRe.ReplaceAllStringFunc(line, func(match string) string {
			n, _ := strconv.Atoi(phpLineRe.FindStringSubmatch(match)[1])
			if n > offset {
				n -= offset
			}
			return " on line " + strconv.Itoa(n)
		})
	}
	if len(problems) > 8 {
		problems = problems[:8]
	}
	return errs.New("PHP " + version + " did not accept these settings, so Phlo did not save them:\n" + strings.Join(problems, "\n"))
}

// phpStartupLines are the warnings PHP prints while it reads ini, with the ini path replaced by {ini}.
func phpStartupLines(cli, ini string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, "-c", ini, "-r", "")
	cmd.Dir = filepath.Dir(cli)
	proc.Configure(cmd)
	out, _ := cmd.CombinedOutput()
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, path := range []string{ini, filepath.ToSlash(ini)} {
			line = strings.ReplaceAll(line, path, "{ini}")
		}
		lines = append(lines, line)
	}
	return lines
}

// customRunning reports whether what reads the custom file is running now.
func (a *App) customRunning(kind string) bool {
	if kind == "php" {
		return a.Supervisor.Running()
	}
	if kind == "cronical" {
		return a.Supervisor.CronRunning()
	}
	return a.databaseRunning(kind)
}

// applyCustom rewrites the PHP CLI ini files and, when running is true, restarts what reads the custom file.
func (a *App) applyCustom(kind string, running bool) error {
	if kind == "php" {
		if err := a.refreshPHPINIs(); err != nil {
			return err
		}
		if !running {
			return nil
		}
		return a.Supervisor.Reload()
	}
	if kind == "cronical" {
		if !running {
			return nil
		}
		if err := a.Supervisor.StopCron(); err != nil {
			return err
		}
		return a.Supervisor.StartCron()
	}
	if !running {
		return nil
	}
	if err := a.Supervisor.StopAddon(kind); err != nil {
		return err
	}
	return a.startDatabase(kind)
}

// startupProblem keeps the log lines that explain why a service stopped.
func startupProblem(err error) string {
	lines := strings.Split(strings.ReplaceAll(err.Error(), "\r\n", "\n"), "\n")
	var picked []string
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, word := range []string{"error", "fatal", "unknown", "invalid", "unrecognized", `"s":"f"`, `"s":"e"`} {
			if strings.Contains(lower, word) {
				picked = append(picked, strings.TrimSpace(line))
				break
			}
		}
	}
	if len(picked) == 0 {
		picked = lines
	}
	if len(picked) > 6 {
		picked = picked[len(picked)-6:]
	}
	text := strings.Join(picked, "\n")
	if len(text) > 1500 {
		text = text[len(text)-1500:]
	}
	return text
}

func readExisting(file string) ([]byte, bool, error) {
	body, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return body, err == nil, err
}

func writeCustom(file, content string) error {
	if content == "" {
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(content), 0o644)
}

func restoreCustom(file string, old []byte, existed bool) error {
	if !existed {
		return writeCustom(file, "")
	}
	return os.WriteFile(file, old, 0o644)
}

var builtInCache sync.Map

// builtIn reports whether the PHP CLI has every module in names compiled in, without any ini.
func builtIn(cli string, names ...string) bool {
	info, err := os.Stat(cli)
	if err != nil || info.IsDir() {
		return false
	}
	key := cli + "|" + info.ModTime().String()
	modules, ok := builtInCache.Load(key)
	if !ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, cli, "-n", "-m")
		cmd.Dir = filepath.Dir(cli)
		proc.Configure(cmd)
		out, _ := cmd.Output()
		var list []string
		for _, line := range strings.Split(string(out), "\n") {
			list = append(list, strings.ToLower(strings.TrimSpace(line)))
		}
		modules, _ = builtInCache.LoadOrStore(key, list)
	}
	for _, name := range names {
		if !slices.Contains(modules.([]string), name) {
			return false
		}
	}
	return true
}

// pgsqlView reports pgsql and pdo_pgsql for the PostgreSQL panel. They come with the PHP build.
func pgsqlView(version, build, dir string, bundled []string, goos string) api.PHPExt {
	view := api.PHPExt{Name: "pgsql", Label: "pgsql and pdo_pgsql", PHP: version, Build: build, Bundled: true}
	files := slices.Contains(bundled, "pgsql") && slices.Contains(bundled, "pdo_pgsql")
	switch {
	case files && goos == "windows" && !exists(filepath.Join(dir, "libpq.dll")):
		view.Missing = "libpq.dll is missing from the PHP " + version + " folder, so pgsql cannot load. Delete that folder and download PHP " + version + " again in Runtimes."
	case files || builtIn(php.CLIExe(dir, goos), "pgsql", "pdo_pgsql"):
		view.Installed = true
	default:
		view.Missing = "This PHP " + version + " build has no pgsql or pdo_pgsql. Delete its folder and download PHP " + version + " again in Runtimes, or use a build that includes them."
	}
	return view
}
