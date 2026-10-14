package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Layout is the portable Phlo directory tree.
// HomeDir and the three data folders are absolute overrides; empty means {root}/home,
// {root}/data, {root}/data-postgres, and {root}/data-mongodb.
type Layout struct {
	Root            string
	HomeDir         string
	MariaDBDataDir  string
	PostgresDataDir string
	MongoDBDataDir  string
}

func (l Layout) ConfigFile() string { return filepath.Join(l.Root, "config", "phlo.json") }

// Custom files hold the settings the Runtimes editors save. Phlo adds them after its own on every start.
func (l Layout) Custom(name string) string { return filepath.Join(l.Root, "config", "custom", name) }
func (l Layout) CustomPHPIni(version string) string {
	return l.Custom("php-" + version + ".ini")
}
func (l Layout) CronicalDat() string    { return l.Custom("cronical.dat") }
func (l Layout) CronicalLogDir() string { return filepath.Join(l.Var(), "log", "cronical") }
func (l Layout) CustomMariaDB() string  { return l.Custom("mariadb.ini") }
func (l Layout) CustomPostgres() string { return l.Custom("postgresql.conf") }
func (l Layout) CustomMongoDB() string  { return l.Custom("mongod.conf") }
func (l Layout) ManifestFile() string {
	return filepath.Join(l.Root, "config", "manifest.json")
}
func (l Layout) Home() string {
	if l.HomeDir != "" {
		return l.HomeDir
	}
	return l.DefaultHome()
}
func (l Layout) Account(name string) string {
	return filepath.Join(l.Home(), name)
}
func (l Layout) PublicHTML(account string) string {
	return filepath.Join(l.Account(account), "public_html")
}
func (l Layout) SubdomainDir(account string) string {
	return filepath.Join(l.Account(account), "subdomain")
}
func (l Layout) Subdomain(account, name string) string {
	return filepath.Join(l.SubdomainDir(account), name)
}
func (l Layout) SiteFile(account string) string {
	return filepath.Join(l.Account(account), "phlo.site.json")
}
func (l Layout) Runtime() string { return filepath.Join(l.Root, "runtime") }
func (l Layout) PHPDir(version, goos, goarch string) string {
	return filepath.Join(l.Runtime(), "php", version, goos+"-"+goarch)
}
func (l Layout) NginxDir(goos, goarch string) string {
	return filepath.Join(l.Runtime(), "nginx", goos+"-"+goarch)
}
func (l Layout) PHPMyAdminDir() string { return filepath.Join(l.Runtime(), "phpmyadmin") }
func (l Layout) PHPMyAdminTemp() string {
	return filepath.Join(l.Var(), "phpmyadmin", "tmp")
}
func (l Layout) PHPMyAdminSecret() string {
	return filepath.Join(l.Var(), "phpmyadmin", "blowfish")
}
func (l Layout) ComposerFile() string {
	return filepath.Join(l.Runtime(), "composer", "composer.phar")
}
func (l Layout) DataDir() string { return orDefault(l.MariaDBDataDir, filepath.Join(l.Root, "data")) }
func (l Layout) MariaDBDir(goos, goarch string) string {
	return filepath.Join(l.Runtime(), "mariadb", goos+"-"+goarch)
}
func (l Layout) MariaDBConf() string { return filepath.Join(l.Var(), "mariadb", "my.ini") }
func (l Layout) Var() string         { return filepath.Join(l.Root, "var") }
func (l Layout) NginxWork() string   { return filepath.Join(l.Var(), "nginx") }
func (l Layout) NginxConf() string   { return filepath.Join(l.NginxWork(), "nginx.conf") }
func (l Layout) NginxPID() string    { return filepath.Join(l.NginxWork(), "nginx.pid") }
func (l Layout) SSLDir() string      { return filepath.Join(l.Var(), "ssl") }
func (l Layout) PHPWork(version string) string {
	return filepath.Join(l.Var(), "php", version)
}
func (l Layout) PHPIni(version string) string {
	return filepath.Join(l.PHPWork(version), "php.ini")
}
func (l Layout) PHPFPM(version string) string {
	return filepath.Join(l.PHPWork(version), "php-fpm.conf")
}
func (l Layout) Log(name string) string { return filepath.Join(l.Var(), "log", name) }

// SiteLog is one project's Nginx and PHP log, var/log/sites/{host}.log.
func (l Layout) SiteLog(host string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.Contains(host, "..") || !siteLogName.MatchString(host) {
		return "", fmt.Errorf("invalid project log name")
	}
	return filepath.Join(l.Var(), "log", "sites", host+".log"), nil
}

var siteLogName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,250}[a-z0-9])?$`)

func (l Layout) Run() string     { return filepath.Join(l.Var(), "run") }
func (l Layout) PhloPID() string { return filepath.Join(l.Run(), "phlo.pid") }
func (l Layout) PHPPID(version string) string {
	return filepath.Join(l.Run(), "php-"+version+".pid")
}
func (l Layout) MariaDBPID() string { return filepath.Join(l.Run(), "mariadb.pid") }
func (l Layout) ToolDir(name, goos, goarch string) string {
	return filepath.Join(l.Runtime(), name, goos+"-"+goarch)
}

// VersionedToolDir is runtime/{name}/{version} for Nginx, MariaDB, and addons.
func (l Layout) VersionedToolDir(name, version string) string {
	return filepath.Join(l.Runtime(), name, version)
}

// PHPExtDir holds one extension build for a PHP flavor key such as 8.3-nts.
func (l Layout) PHPExtDir(name, key string) string {
	return filepath.Join(l.Runtime(), "phpext", name, key)
}
func (l Layout) RedisPID() string     { return filepath.Join(l.Run(), "redis.pid") }
func (l Layout) MemcachedPID() string { return filepath.Join(l.Run(), "memcached.pid") }
func (l Layout) MailpitWork() string  { return filepath.Join(l.Var(), "mailpit") }
func (l Layout) MailpitDB() string    { return filepath.Join(l.MailpitWork(), "mailpit.db") }
func (l Layout) MailpitPID() string   { return filepath.Join(l.Run(), "mailpit.pid") }
func (l Layout) CronicalPID() string  { return filepath.Join(l.Run(), "cronical.pid") }
func (l Layout) NgrokDir() string     { return filepath.Join(l.Var(), "ngrok") }
func (l Layout) NgrokConfig() string  { return filepath.Join(l.NgrokDir(), "ngrok.yml") }
func (l Layout) NgrokPID() string     { return filepath.Join(l.Run(), "ngrok.pid") }
func (l Layout) HostsState() string   { return filepath.Join(l.Var(), "hosts") }

// MongoDBData and PostgresData default to folders next to the MariaDB data folder so MariaDB's folder is untouched.
func (l Layout) MongoDBData() string {
	return orDefault(l.MongoDBDataDir, filepath.Join(l.Root, "data-mongodb"))
}
func (l Layout) PostgresData() string {
	return orDefault(l.PostgresDataDir, filepath.Join(l.Root, "data-postgres"))
}
func (l Layout) MongoDBPID() string  { return filepath.Join(l.Run(), "mongodb.pid") }
func (l Layout) PostgresPID() string { return filepath.Join(l.Run(), "postgres.pid") }
func (l Layout) Cache(name string) string {
	return filepath.Join(l.Var(), "cache", name)
}
func (l Layout) NPMPrefix() string    { return filepath.Join(l.Var(), "npm-global") }
func (l Layout) DockerConfig() string { return filepath.Join(l.Var(), "docker") }
func (l Layout) Shims() string        { return filepath.Join(l.Var(), "shims") }

// NonASCIIPart returns the first folder name in path with a character outside ASCII, or "".
func NonASCIIPart(path string) string {
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		for _, r := range part {
			if r > unicode.MaxASCII {
				return part
			}
		}
	}
	return ""
}

// FindRoot walks parents until config/phlo.json exists.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		info, statErr := os.Stat(filepath.Join(dir, "config", "phlo.json"))
		if statErr == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("config/phlo.json not found from %s", start)
		}
		dir = parent
	}
}

// Resolve finds the project root from a flag, PHLO_ROOT, the working directory, or the executable path.
func Resolve(flag, env, cwd, exe string) (string, error) {
	if flag != "" {
		return filepath.Abs(flag)
	}
	if env != "" {
		return filepath.Abs(env)
	}
	if cwd != "" {
		if root, err := FindRoot(cwd); err == nil {
			return root, nil
		}
	}
	if exe != "" {
		if root, err := FindRoot(filepath.Dir(exe)); err == nil {
			return root, nil
		}
	}
	return "", fmt.Errorf("cannot find Phlo root (config/phlo.json); pass --root")
}
