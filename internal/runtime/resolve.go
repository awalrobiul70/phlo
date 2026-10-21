package runtime

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"

	"phlo/internal/errs"
	"phlo/internal/paths"
	"phlo/internal/php"
)

var componentVersionRe = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){1,3}$`)

// Install is one Nginx or MariaDB build Phlo can select.
type Install struct {
	Version      string
	Installed    bool
	Downloadable bool
}

// ResolvePHP finds the FastCGI or php-fpm binary for a version.
func ResolvePHP(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	dir := layout.PHPDir(version, goos, goarch)
	if art, err := manifest.LookupPHP(version, goos, goarch); err == nil {
		exe := filepath.Join(dir, art.Bin)
		if info, statErr := os.Stat(exe); statErr == nil && !info.IsDir() {
			return exe, nil
		}
		return "", errs.New("PHP " + version + " is not installed. Expected " + exe)
	}
	if exe := findBinary(dir, "php-fpm", "php-fpm.exe", "php-cgi", "php-cgi.exe"); exe != "" {
		return exe, nil
	}
	flat := filepath.Join(layout.Runtime(), "php", version)
	names := []string{"php-cgi.exe", "php-fpm.exe", "php-cgi", "php-fpm"}
	if exe := findBinary(flat, names...); exe != "" {
		return exe, nil
	}
	if exe := findBinaryInChildren(flat, names...); exe != "" {
		return exe, nil
	}
	return "", errs.New("PHP " + version + " is not in the Phlo manifest for " + goos + "/" + goarch)
}

// ResolveNginx finds the selected Nginx binary. An empty version uses the manifest build.
func ResolveNginx(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	art, artErr := manifest.LookupNginx(goos, goarch)
	if version == "" && artErr == nil {
		version = art.Version
	}
	if version != "" {
		if exe := findVersionBinary(layout, "nginx", version, goos, goarch, "nginx.exe", "nginx"); exe != "" {
			return exe, nil
		}
	}
	if version == "" || (artErr == nil && version == art.Version) {
		dir := layout.NginxDir(goos, goarch)
		if artErr == nil {
			exe := filepath.Join(dir, art.Bin)
			if info, statErr := os.Stat(exe); statErr == nil && !info.IsDir() {
				return exe, nil
			}
		}
		if exe := findBinary(dir, "nginx.exe", "nginx"); exe != "" {
			return exe, nil
		}
	}
	if version == "" {
		return "", errs.New("Nginx is not in the Phlo manifest for " + goos + "/" + goarch)
	}
	return "", errs.New("Nginx " + version + " is not installed")
}

// ResolveMariaDB finds the selected mysqld or mariadbd. An empty version uses the manifest build.
func ResolveMariaDB(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	art, artErr := manifest.LookupMariaDB(goos, goarch)
	if version == "" && artErr == nil {
		version = art.Version
	}
	names := []string{"bin/mysqld.exe", "bin/mariadbd.exe", "bin/mysqld", "bin/mariadbd", "mysqld.exe", "mariadbd", "mysqld"}
	if version != "" {
		if exe := findVersionBinary(layout, "mariadb", version, goos, goarch, names...); exe != "" {
			return exe, nil
		}
	}
	if version == "" || (artErr == nil && version == art.Version) {
		dir := layout.MariaDBDir(goos, goarch)
		if artErr == nil {
			exe := filepath.Join(dir, filepath.FromSlash(art.Bin))
			if info, statErr := os.Stat(exe); statErr == nil && !info.IsDir() {
				return exe, nil
			}
		}
		if exe := findBinary(dir, names...); exe != "" {
			return exe, nil
		}
	}
	if version == "" {
		return "", errs.New("MariaDB is not in the Phlo manifest for " + goos + "/" + goarch)
	}
	return "", errs.New("MariaDB " + version + " is not installed")
}

// ResolveRedis finds the selected redis-server. An empty version uses the manifest build.
func ResolveRedis(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	return resolveVersioned(layout, "redis", version, goos, goarch, "Redis", func() (Artifact, error) {
		return manifest.LookupRedis(goos, goarch)
	}, "redis-server.exe", "redis-server")
}

// ResolveMemcached finds the selected memcached binary. An empty version uses the manifest build.
func ResolveMemcached(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	return resolveVersioned(layout, "memcached", version, goos, goarch, "Memcached", func() (Artifact, error) {
		return manifest.LookupMemcached(goos, goarch)
	}, "bin/memcached.exe", "memcached.exe", "memcached")
}

// ResolveMailpit finds the selected mailpit binary. An empty version uses the manifest build.
func ResolveMailpit(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	return resolveVersioned(layout, "mailpit", version, goos, goarch, "Mailpit", func() (Artifact, error) {
		return manifest.LookupMailpit(goos, goarch)
	}, "mailpit.exe", "mailpit")
}

// ResolveCronical finds Cronical.exe. An empty version uses the manifest build.
func ResolveCronical(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	return resolveVersioned(layout, "cronical", version, goos, goarch, "Cronical", func() (Artifact, error) {
		return manifest.LookupCronical(goos, goarch)
	}, "Cronical.exe")
}

// ResolveHeidiSQL finds the portable heidisql.exe. An empty version uses the manifest build.
func ResolveHeidiSQL(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	return resolveVersioned(layout, "heidisql", version, goos, goarch, "HeidiSQL", func() (Artifact, error) {
		return manifest.LookupHeidiSQL(goos, goarch)
	}, "heidisql.exe")
}

// ResolveSQLyog finds the downloaded Community installer. An empty version uses the manifest build.
func ResolveSQLyog(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	return resolveVersioned(layout, "sqlyog", version, goos, goarch, "SQLyog", func() (Artifact, error) {
		return manifest.LookupSQLyog(goos, goarch)
	}, "SQLyog-13.3.1-0.x64Community.exe")
}

// ResolveNgrok finds the ngrok CLI. An empty version uses the manifest build.
func ResolveNgrok(manifest Manifest, layout paths.Layout, version, goos, goarch string) (string, error) {
	return resolveVersioned(layout, "ngrok", version, goos, goarch, "ngrok", func() (Artifact, error) {
		return manifest.LookupNgrok(goos, goarch)
	}, "ngrok.exe", "ngrok")
}

func resolveVersioned(layout paths.Layout, kind, version, goos, goarch, label string, lookup func() (Artifact, error), names ...string) (string, error) {
	art, artErr := lookup()
	if version == "" && artErr == nil {
		version = art.Version
	}
	if version != "" {
		if exe := findVersionBinary(layout, kind, version, goos, goarch, names...); exe != "" {
			return exe, nil
		}
	}
	if version == "" || (artErr == nil && version == art.Version) {
		dir := layout.ToolDir(kind, goos, goarch)
		if artErr == nil {
			if exe := findBinary(dir, filepath.FromSlash(art.Bin)); exe != "" {
				return exe, nil
			}
		}
		if exe := findBinary(dir, names...); exe != "" {
			return exe, nil
		}
	}
	if version == "" {
		return "", errs.New(label + " is not in the Phlo manifest for " + goos + "/" + goarch)
	}
	return "", errs.New(label + " " + version + " is not installed")
}

// ListNginx returns the manifest build and any version folder under runtime/nginx.
func ListNginx(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "nginx", func() (Artifact, error) {
		return manifest.LookupNginx(goos, goarch)
	}, func(version string) error {
		_, err := ResolveNginx(manifest, layout, version, goos, goarch)
		return err
	})
}

// ListMariaDB returns the manifest build and any version folder under runtime/mariadb.
func ListMariaDB(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "mariadb", func() (Artifact, error) {
		return manifest.LookupMariaDB(goos, goarch)
	}, func(version string) error {
		_, err := ResolveMariaDB(manifest, layout, version, goos, goarch)
		return err
	})
}

// ListRedis returns the manifest build and any version folder under runtime/redis.
func ListRedis(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "redis", func() (Artifact, error) {
		return manifest.LookupRedis(goos, goarch)
	}, func(version string) error {
		_, err := ResolveRedis(manifest, layout, version, goos, goarch)
		return err
	})
}

// ListMemcached returns the manifest build and any version folder under runtime/memcached.
func ListMemcached(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "memcached", func() (Artifact, error) {
		return manifest.LookupMemcached(goos, goarch)
	}, func(version string) error {
		_, err := ResolveMemcached(manifest, layout, version, goos, goarch)
		return err
	})
}

// ListMailpit returns the manifest build and any version folder under runtime/mailpit.
func ListMailpit(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "mailpit", func() (Artifact, error) {
		return manifest.LookupMailpit(goos, goarch)
	}, func(version string) error {
		_, err := ResolveMailpit(manifest, layout, version, goos, goarch)
		return err
	})
}

// ListCronical returns the manifest build and any version folder under runtime/cronical.
func ListCronical(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "cronical", func() (Artifact, error) {
		return manifest.LookupCronical(goos, goarch)
	}, func(version string) error {
		_, err := ResolveCronical(manifest, layout, version, goos, goarch)
		return err
	})
}

// ListHeidiSQL returns the manifest build and any version folder under runtime/heidisql.
func ListHeidiSQL(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "heidisql", func() (Artifact, error) {
		return manifest.LookupHeidiSQL(goos, goarch)
	}, func(version string) error {
		_, err := ResolveHeidiSQL(manifest, layout, version, goos, goarch)
		return err
	})
}

// ListSQLyog returns the manifest installer and any version folder under runtime/sqlyog.
func ListSQLyog(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "sqlyog", func() (Artifact, error) {
		return manifest.LookupSQLyog(goos, goarch)
	}, func(version string) error {
		_, err := ResolveSQLyog(manifest, layout, version, goos, goarch)
		return err
	})
}

// ListNgrok returns the manifest build and any version folder under runtime/ngrok.
func ListNgrok(manifest Manifest, layout paths.Layout, goos, goarch string) []Install {
	return listInstalls(layout, "ngrok", func() (Artifact, error) {
		return manifest.LookupNgrok(goos, goarch)
	}, func(version string) error {
		_, err := ResolveNgrok(manifest, layout, version, goos, goarch)
		return err
	})
}

// SelectedVersion is the saved choice, or the installed manifest build, or the only dropped-in build.
func SelectedVersion(chosen string, items []Install) string {
	if chosen != "" {
		return chosen
	}
	for _, item := range items {
		if item.Installed && item.Downloadable {
			return item.Version
		}
	}
	for _, item := range items {
		if item.Installed {
			return item.Version
		}
	}
	if len(items) > 0 {
		return items[0].Version
	}
	return ""
}

func listInstalls(layout paths.Layout, kind string, lookup func() (Artifact, error), resolve func(string) error) []Install {
	byVersion := map[string]Install{}
	art, err := lookup()
	if err == nil {
		byVersion[art.Version] = Install{
			Version:      art.Version,
			Installed:    resolve(art.Version) == nil,
			Downloadable: true,
		}
	}
	entries, _ := os.ReadDir(filepath.Join(layout.Runtime(), kind))
	for _, entry := range entries {
		if !entry.IsDir() || !componentVersionRe.MatchString(entry.Name()) {
			continue
		}
		version := entry.Name()
		item, known := byVersion[version]
		if !known {
			// A pasted folder is a version only when its program is inside, the same way a PHP folder needs php-cgi.exe.
			if resolve(version) != nil {
				continue
			}
			byVersion[version] = Install{Version: version, Installed: true}
			continue
		}
		item.Version = version
		if resolve(version) == nil {
			item.Installed = true
		}
		byVersion[version] = item
	}
	versions := make([]string, 0, len(byVersion))
	for version := range byVersion {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	list := make([]Install, 0, len(versions))
	for _, version := range versions {
		list = append(list, byVersion[version])
	}
	return list
}

func findVersionBinary(layout paths.Layout, kind, version, goos, goarch string, names ...string) string {
	root := filepath.Join(layout.Runtime(), kind, version)
	if exe := findBinary(root, names...); exe != "" {
		return exe
	}
	if exe := findBinary(filepath.Join(root, goos+"-"+goarch), names...); exe != "" {
		return exe
	}
	// A zip often unpacks as runtime/{kind}/{version}/{ArchiveName}/program.exe.
	return findBinaryInChildren(root, names...)
}

// findBinaryInChildren looks one folder down. It does not walk further.
func findBinaryInChildren(dir string, names ...string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if exe := findBinary(filepath.Join(dir, entry.Name()), names...); exe != "" {
			return exe
		}
	}
	return ""
}

// ResolveMySQLClient finds mysql or mariadb next to the server binary.
func ResolveMySQLClient(server string) (string, error) {
	if exe := findBinary(filepath.Dir(server), "mysql.exe", "mariadb.exe", "mysql", "mariadb"); exe != "" {
		return exe, nil
	}
	return "", errs.New("MySQL client is not installed next to " + server)
}

// ResolveInstallDB finds the datadir initializer next to the server binary.
func ResolveInstallDB(server string) (string, error) {
	if exe := findBinary(filepath.Dir(server), "mysql_install_db.exe", "mariadb-install-db.exe", "mysql_install_db", "mariadb-install-db"); exe != "" {
		return exe, nil
	}
	return "", errs.New("mariadb-install-db is not installed next to " + server)
}

// ResolveMySQLAdmin finds mysqladmin next to the server binary.
func ResolveMySQLAdmin(server string) string {
	return findBinary(filepath.Dir(server), "mysqladmin.exe", "mariadb-admin.exe", "mysqladmin", "mariadb-admin")
}

// ResolveCLI finds php.exe or php for the default version.
func ResolveCLI(manifest Manifest, layout paths.Layout, version string) (string, error) {
	fastcgi, err := ResolvePHP(manifest, layout, version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	cli := php.CLIExe(filepath.Dir(fastcgi), runtime.GOOS)
	info, err := os.Stat(cli)
	if err != nil || info.IsDir() {
		return "", errs.New("PHP CLI is not installed. Expected " + cli)
	}
	return cli, nil
}

func findBinary(dir string, names ...string) string {
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}
