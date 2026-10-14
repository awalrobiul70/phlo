package supervisor

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"phlo/internal/config"
	"phlo/internal/cronical"
	"phlo/internal/errs"
	"phlo/internal/laravel"
	"phlo/internal/mariadb"
	"phlo/internal/mongodb"
	"phlo/internal/nginx"
	"phlo/internal/ngrok"
	"phlo/internal/paths"
	"phlo/internal/php"
	"phlo/internal/phpmyadmin"
	"phlo/internal/postgres"
	"phlo/internal/proc"
	phloruntime "phlo/internal/runtime"
	"phlo/internal/services"
	"phlo/internal/site"
	"phlo/internal/ssl"
)

type child struct {
	cmd *exec.Cmd
	log *os.File
}

type phpJob struct {
	Version string
	Exe     string
	Mode    string
	Port    int
	Dir     string
}

// Supervisor starts the bundled Nginx and PHP pools.
type Supervisor struct {
	mu        sync.Mutex
	Layout    paths.Layout
	Config    *config.Config
	Manifest  *phloruntime.Manifest
	procs     map[string]*child
	shareHost string
	shareURL  string
}

func New(layout paths.Layout, cfg *config.Config, manifest *phloruntime.Manifest) *Supervisor {
	return &Supervisor{
		Layout:   layout,
		Config:   cfg,
		Manifest: manifest,
		procs:    map[string]*child{},
	}
}

func (s *Supervisor) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nginxAlive()
}

func (s *Supervisor) DatabaseRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mariaAlive()
}

func (s *Supervisor) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.startLocked(); err != nil {
		s.stopLocked()
		return err
	}
	return nil
}

func (s *Supervisor) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	return nil
}

func (s *Supervisor) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.nginxAlive() {
		return nil
	}
	sites, err := s.scan()
	if err != nil {
		return err
	}
	if err := site.Duplicate(sites); err != nil {
		return err
	}
	jobs, err := s.phpJobs(sites)
	if err != nil {
		return err
	}
	nginxExe, err := s.nginxBinary()
	if err != nil {
		return err
	}
	if err := s.render(sites, jobs); err != nil {
		return err
	}
	for _, job := range jobs {
		s.stopPHP(job.Version)
		if err := s.launchPHP(job); err != nil {
			return err
		}
	}
	s.stopUnneeded(jobs)
	return s.signalNginx(nginxExe, "reload")
}

func (s *Supervisor) startLocked() error {
	if s.nginxAlive() {
		return errs.New("services are already running")
	}
	s.clearStalePID(s.Layout.NginxPID())
	sites, err := s.scan()
	if err != nil {
		return err
	}
	if err := site.Duplicate(sites); err != nil {
		return err
	}
	jobs, err := s.phpJobs(sites)
	if err != nil {
		return err
	}
	nginxExe, err := s.nginxBinary()
	if err != nil {
		return err
	}
	if err := s.httpPortsFree(); err != nil {
		return err
	}
	for _, job := range jobs {
		if err := freePort(job.Port); err != nil {
			return err
		}
	}
	dbExe, dbErr := s.mariaBinary()
	if dbErr == nil && s.Config.MariaDBOn() {
		if err := freePort(s.Config.MySQLListenPort()); err != nil {
			return fmt.Errorf("MariaDB: %w", err)
		}
	}
	if err := s.reserveAddonPorts(); err != nil {
		return err
	}
	if err := s.render(sites, jobs); err != nil {
		return err
	}
	for _, job := range jobs {
		if err := s.launchPHP(job); err != nil {
			return err
		}
	}
	if err := s.launchNginx(nginxExe); err != nil {
		return err
	}
	if dbErr == nil && s.Config.MariaDBOn() {
		if err := s.launchMariaDB(dbExe); err != nil {
			return err
		}
	}
	if err := s.startAddons(); err != nil {
		return err
	}
	return s.startCron()
}

func (s *Supervisor) scan() ([]site.Site, error) {
	return site.Scan(s.Layout.Home(), site.Options{
		TLD:        s.Config.TLD,
		DefaultPHP: s.Config.DefaultPHP,
	})
}

func (s *Supervisor) phpJobs(sites []site.Site) ([]phpJob, error) {
	versions := map[string]struct{}{}
	for _, item := range sites {
		if item.PublicHTML != nil {
			versions[item.PublicHTML.PHP] = struct{}{}
		}
		for _, sub := range item.Subdomains {
			versions[sub.PHP] = struct{}{}
		}
	}
	if phpmyadmin.Installed(s.Layout.PHPMyAdminDir()) {
		versions[s.Config.DefaultPHP] = struct{}{}
	}
	list := make([]string, 0, len(versions))
	for version := range versions {
		list = append(list, version)
	}
	sort.Strings(list)
	jobs := make([]phpJob, 0, len(list))
	for _, version := range list {
		exe, err := phloruntime.ResolvePHP(*s.Manifest, s.Layout, version, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return nil, err
		}
		port, err := php.Port(version)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, phpJob{
			Version: version,
			Exe:     exe,
			Mode:    php.Mode(exe),
			Port:    port,
			Dir:     filepath.Dir(exe),
		})
	}
	return jobs, nil
}

func (s *Supervisor) render(sites []site.Site, jobs []phpJob) error {
	temp := filepath.Join(s.Layout.NginxWork(), "temp")
	for _, sub := range []string{"", "client_body", "proxy", "fastcgi", "uwsgi", "scgi"} {
		if err := os.MkdirAll(filepath.Join(temp, sub), 0o755); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.Layout.Log("nginx.log")), 0o755); err != nil {
		return err
	}
	blocks, err := blocksFor(sites)
	if err != nil {
		return err
	}
	for i := range blocks {
		logPath, err := s.Layout.SiteLog(blocks[i].ServerName)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		file.Close()
		blocks[i].ErrorLog = logPath
	}
	if phpmyadmin.Installed(s.Layout.PHPMyAdminDir()) {
		block, err := s.phpMyAdminBlock()
		if err != nil {
			return err
		}
		blocks = append(blocks, block)
	}
	input := nginx.Input{
		Listen:    fmt.Sprintf("127.0.0.1:%d", s.Config.HTTPPort),
		PID:       s.Layout.NginxPID(),
		ErrorLog:  s.Layout.Log("nginx.error.log"),
		AccessLog: s.Layout.Log("nginx.access.log"),
		TempDir:   temp,
		Blocks:    blocks,
	}
	var hostIPs []string
	if host := s.Config.HostIP; host != "" {
		hostIPs = []string{host}
		input.HostListen = net.JoinHostPort(host, strconv.Itoa(s.Config.HTTPPort))
	}
	if s.Config.SSLOn() {
		cert, key, err := ssl.Ensure(s.Layout.SSLDir(), sslHosts(sites, s.Config.TLD), hostIPs)
		if err != nil {
			return err
		}
		input.SSLListen = fmt.Sprintf("127.0.0.1:%d", s.Config.SSLListenPort())
		input.SSLCert = cert
		input.SSLKey = key
		if input.HostListen != "" {
			input.HostSSLListen = net.JoinHostPort(s.Config.HostIP, strconv.Itoa(s.Config.SSLListenPort()))
		}
	}
	conf, err := nginx.Generate(input)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Layout.NginxWork(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.Layout.NginxConf(), []byte(conf), 0o644); err != nil {
		return err
	}
	for _, job := range jobs {
		if err := os.MkdirAll(s.Layout.PHPWork(job.Version), 0o755); err != nil {
			return err
		}
		ini, err := PHPINI(s.Layout, job.Version, job.Dir)
		if err != nil {
			return err
		}
		if err := os.WriteFile(s.Layout.PHPIni(job.Version), []byte(ini), 0o644); err != nil {
			return err
		}
		if job.Mode == "fpm" {
			body, err := php.FPMConfig(
				fmt.Sprintf("127.0.0.1:%d", job.Port),
				s.Layout.PHPPID(job.Version),
				s.Layout.Log("php-"+job.Version+".error.log"),
			)
			if err != nil {
				return err
			}
			if err := os.WriteFile(s.Layout.PHPFPM(job.Version), []byte(body), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Supervisor) writePHPMyAdminConfig() error {
	if !phpmyadmin.Installed(s.Layout.PHPMyAdminDir()) {
		return nil
	}
	secret, err := phpmyadmin.Secret(s.Layout.PHPMyAdminSecret())
	if err != nil {
		return err
	}
	return phpmyadmin.WriteConfig(s.Layout.PHPMyAdminDir(), secret, s.Layout.PHPMyAdminTemp(), s.Config.MySQLListenPort())
}

func (s *Supervisor) phpMyAdminBlock() (nginx.Block, error) {
	if err := s.writePHPMyAdminConfig(); err != nil {
		return nginx.Block{}, err
	}
	port, err := php.Port(s.Config.DefaultPHP)
	if err != nil {
		return nginx.Block{}, err
	}
	return nginx.Block{
		ServerName: site.PHPMyAdminHost(s.Config.TLD),
		Root:       s.Layout.PHPMyAdminDir(),
		FastCGI:    fmt.Sprintf("127.0.0.1:%d", port),
		Private:    true,
	}, nil
}

func blocksFor(sites []site.Site) ([]nginx.Block, error) {
	var blocks []nginx.Block
	add := func(root site.Root) error {
		port, err := php.Port(root.PHP)
		if err != nil {
			return err
		}
		blocks = append(blocks, nginx.Block{
			ServerName: root.Host,
			Root:       laravel.WebRoot(root.DocumentRoot),
			FastCGI:    fmt.Sprintf("127.0.0.1:%d", port),
		})
		return nil
	}
	for _, item := range sites {
		if item.PublicHTML != nil {
			if err := add(*item.PublicHTML); err != nil {
				return nil, err
			}
		}
		for _, sub := range item.Subdomains {
			if err := add(sub); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].ServerName < blocks[j].ServerName })
	return blocks, nil
}

func (s *Supervisor) launchPHP(job phpJob) error {
	s.clearStalePID(s.Layout.PHPPID(job.Version))
	if err := freePort(job.Port); err != nil {
		return fmt.Errorf("PHP %s: %w", job.Version, err)
	}
	ini := s.Layout.PHPIni(job.Version)
	var args []string
	if job.Mode == "fpm" {
		args = php.FPMArgs(s.Layout.PHPFPM(job.Version), ini)
	} else {
		args = php.CGIArgs(fmt.Sprintf("127.0.0.1:%d", job.Port), ini)
	}
	logPath := s.Layout.Log("php-" + job.Version + ".log")
	ch, err := startLogged(job.Exe, args, job.Dir, []string{"PHP_FCGI_MAX_REQUESTS=0"}, logPath)
	if err != nil {
		return err
	}
	s.procs["php:"+job.Version] = ch
	if err := waitAlive(ch.cmd.Process.Pid, logPath); err != nil {
		return fmt.Errorf("PHP %s: %w", job.Version, err)
	}
	return writePID(s.Layout.PHPPID(job.Version), ch.cmd.Process.Pid)
}

func (s *Supervisor) launchMariaDB(exe string) error {
	basedir := mariaBase(exe)
	datadir := s.Layout.DataDir()
	conf := s.Layout.MariaDBConf()
	pidFile := s.Layout.MariaDBPID()
	logPath := s.Layout.Log("mariadb.log")
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(datadir, 0o755); err != nil {
		return err
	}
	body, err := s.mariaINI(exe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(conf, []byte(body), 0o644); err != nil {
		return err
	}
	if mariadb.NeedsInit(datadir) {
		installer, err := phloruntime.ResolveInstallDB(exe)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, installer, "--datadir="+datadir)
		cmd.Dir = basedir
		proc.Configure(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("initialize MariaDB data directory: %w %s", err, strings.TrimSpace(string(out)))
		}
	}
	s.clearStalePID(pidFile)
	ch, err := startLogged(exe, mariadb.ServerArgs(conf), basedir, nil, logPath)
	if err != nil {
		return err
	}
	s.procs["mariadb"] = ch
	if err := waitAliveFor(ch.cmd.Process.Pid, logPath, 3*time.Second); err != nil {
		return s.customHint("mariadb", fmt.Errorf("MariaDB: %w", err))
	}
	return writePID(pidFile, ch.cmd.Process.Pid)
}

func (s *Supervisor) stopMariaDB() {
	port := s.mariaFilePort()
	if port == 0 {
		port = s.Config.MySQLListenPort()
	}
	if exe, err := s.mariaBinary(); err == nil {
		if admin := phloruntime.ResolveMySQLAdmin(exe); admin != "" && s.mariaAlive() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			args := append(mariadb.ClientArgs(port), "shutdown")
			cmd := exec.CommandContext(ctx, admin, args...)
			proc.Configure(cmd)
			_ = cmd.Run()
			time.Sleep(300 * time.Millisecond)
		}
	}
	pid := readPID(s.Layout.MariaDBPID())
	if ch := s.procs["mariadb"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		pid = ch.cmd.Process.Pid
	}
	s.killPID(pid)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && proc.Alive(pid) {
		time.Sleep(50 * time.Millisecond)
	}
	if ch := s.procs["mariadb"]; ch != nil {
		closeChild(ch)
		delete(s.procs, "mariadb")
	}
	_ = os.Remove(s.Layout.MariaDBPID())
}

func (s *Supervisor) mariaAlive() bool {
	if proc.Alive(readPID(s.Layout.MariaDBPID())) {
		return true
	}
	if ch := s.procs["mariadb"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		return proc.Alive(ch.cmd.Process.Pid)
	}
	return false
}

func (s *Supervisor) launchNginx(exe string) error {
	logPath := s.Layout.Log("nginx.log")
	ch, err := startLogged(exe, []string{"-p", s.prefix(), "-c", s.Layout.NginxConf()}, filepath.Dir(exe), nil, logPath)
	if err != nil {
		return err
	}
	s.procs["nginx"] = ch
	if err := waitAlive(ch.cmd.Process.Pid, logPath); err != nil {
		return fmt.Errorf("nginx: %w", err)
	}
	return nil
}

func (s *Supervisor) signalNginx(exe, signal string) error {
	cmd := exec.Command(exe, "-p", s.prefix(), "-c", s.Layout.NginxConf(), "-s", signal)
	proc.Configure(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx %s: %w %s", signal, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Supervisor) prefix() string {
	prefix := s.Layout.NginxWork()
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return prefix
}

func (s *Supervisor) SwitchNginx() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	exe, err := s.nginxBinary()
	if err != nil {
		return err
	}
	if !s.nginxAlive() {
		return nil
	}
	old := ""
	if ch := s.procs["nginx"]; ch != nil && ch.cmd != nil {
		old = ch.cmd.Path
		if samePath(old, exe) {
			return nil
		}
	}
	s.stopNginxLocked()
	if err := s.httpPortsFree(); err != nil {
		return err
	}
	if err := s.launchNginx(exe); err != nil {
		if old != "" {
			_ = s.launchNginx(old)
		}
		return err
	}
	return nil
}

// RestartHTTP re-renders Nginx and restarts it so listen and SSL changes take effect.
func (s *Supervisor) RestartHTTP() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.nginxAlive() {
		return nil
	}
	sites, err := s.scan()
	if err != nil {
		return err
	}
	if err := site.Duplicate(sites); err != nil {
		return err
	}
	jobs, err := s.phpJobs(sites)
	if err != nil {
		return err
	}
	nginxExe, err := s.nginxBinary()
	if err != nil {
		return err
	}
	s.stopNginxLocked()
	if err := s.httpPortsFree(); err != nil {
		return err
	}
	if err := s.render(sites, jobs); err != nil {
		return err
	}
	return s.launchNginx(nginxExe)
}

func sslHosts(sites []site.Site, tld string) []string {
	hosts := []string{"*." + tld, "phpmyadmin." + tld}
	for _, item := range sites {
		if item.EffectiveHost != "" {
			hosts = append(hosts, item.EffectiveHost)
		}
		for _, sub := range item.Subdomains {
			if sub.Host != "" {
				hosts = append(hosts, sub.Host)
			}
		}
	}
	return hosts
}

func (s *Supervisor) SwitchMariaDB() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Config.MariaDBOn() {
		s.stopMariaDB()
		return nil
	}
	exe, err := s.mariaBinary()
	if err != nil {
		return err
	}
	if !s.nginxAlive() && !s.mariaAlive() {
		return nil
	}
	old := ""
	if ch := s.procs["mariadb"]; ch != nil && ch.cmd != nil {
		old = ch.cmd.Path
		if keepRunningMariaDB(samePath(old, exe), s.mariaAlive(), s.mariaFilePort(), s.Config.MySQLListenPort()) {
			return s.writePHPMyAdminConfig()
		}
	}
	s.stopMariaDB()
	if err := freePort(s.Config.MySQLListenPort()); err != nil {
		return fmt.Errorf("MariaDB: %w", err)
	}
	if err := s.launchMariaDB(exe); err != nil {
		if old != "" {
			_ = s.launchMariaDB(old)
		}
		return err
	}
	return s.writePHPMyAdminConfig()
}

// keepRunningMariaDB reports that the process already matches the selected
// binary and the port in my.ini. A different port must restart the server;
// the port lives in my.ini, not on the process command line.
func keepRunningMariaDB(sameExe, alive bool, filePort, wantPort int) bool {
	if !sameExe || !alive {
		return false
	}
	return filePort == 0 || filePort == wantPort
}

func (s *Supervisor) mariaFilePort() int {
	body, err := os.ReadFile(s.Layout.MariaDBConf())
	if err != nil {
		return 0
	}
	return mariadb.ListenPort(string(body))
}

// StartMariaDB launches MariaDB when the stack is running and the service is enabled.
func (s *Supervisor) StartMariaDB() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Config.MariaDBOn() || !s.nginxAlive() || s.mariaAlive() {
		return nil
	}
	exe, err := s.mariaBinary()
	if err != nil {
		return nil
	}
	if err := freePort(s.Config.MySQLListenPort()); err != nil {
		return fmt.Errorf("MariaDB: %w", err)
	}
	return s.launchMariaDB(exe)
}

// SwitchAddon restarts one optional service when its selected binary changes.
func (s *Supervisor) SwitchAddon(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var item addon
	found := false
	for _, candidate := range s.addons() {
		if candidate.name == name {
			item = candidate
			found = true
			break
		}
	}
	if !found {
		return errs.New("unknown service")
	}
	if !s.addonEnabled(name) {
		s.stopAddonItem(item)
		return nil
	}
	exe, err := item.exe()
	if err != nil {
		return err
	}
	if !s.nginxAlive() && !s.addonAlive(item) {
		return nil
	}
	old := ""
	if ch := s.procs[name]; ch != nil && ch.cmd != nil {
		old = ch.cmd.Path
		if samePath(old, exe) && s.addonAlive(item) && !s.addonPortChanged(item) {
			return nil
		}
	}
	s.stopAddonItem(item)
	if err := freePort(item.port); err != nil {
		if old != "" {
			_ = s.launchAddonExe(item, old)
		}
		return fmt.Errorf("%s: %w", label(name), err)
	}
	if name == "mailpit" {
		if err := freePort(s.Config.MailpitUIListenPort()); err != nil {
			if old != "" {
				_ = s.launchAddonExe(item, old)
			}
			return fmt.Errorf("Mailpit: %w", err)
		}
	}
	if err := s.launchAddonExe(item, exe); err != nil {
		if old != "" {
			_ = s.launchAddonExe(item, old)
		}
		return err
	}
	return nil
}

type addon struct {
	name string
	port int
	pid  string
	log  string
	exe  func() (string, error)
	args func(exe string) ([]string, error)
	// stop asks the service to shut down cleanly before Phlo kills the recorded pid.
	stop func()
}

func (s *Supervisor) addons() []addon {
	return []addon{
		{
			name: "redis",
			port: s.Config.RedisListenPort(),
			pid:  s.Layout.RedisPID(),
			log:  "redis.log",
			exe: func() (string, error) {
				return phloruntime.ResolveRedis(*s.Manifest, s.Layout, s.Config.Redis, runtime.GOOS, runtime.GOARCH)
			},
			args: func(string) ([]string, error) {
				return services.RedisArgs(s.Config.RedisListenPort()), nil
			},
		},
		{
			name: "memcached",
			port: s.Config.MemcachedListenPort(),
			pid:  s.Layout.MemcachedPID(),
			log:  "memcached.log",
			exe: func() (string, error) {
				return phloruntime.ResolveMemcached(*s.Manifest, s.Layout, s.Config.Memcached, runtime.GOOS, runtime.GOARCH)
			},
			args: func(string) ([]string, error) {
				return services.MemcachedArgs(s.Config.MemcachedListenPort()), nil
			},
		},
		{
			name: "mailpit",
			port: s.Config.MailpitSMTPListenPort(),
			pid:  s.Layout.MailpitPID(),
			log:  "mailpit.log",
			exe: func() (string, error) {
				return phloruntime.ResolveMailpit(*s.Manifest, s.Layout, s.Config.Mailpit, runtime.GOOS, runtime.GOARCH)
			},
			args: func(string) ([]string, error) {
				if err := os.MkdirAll(s.Layout.MailpitWork(), 0o755); err != nil {
					return nil, err
				}
				return services.MailpitArgs(s.Config.MailpitSMTPListenPort(), s.Config.MailpitUIListenPort(), s.Layout.MailpitDB()), nil
			},
		},
		{
			name: "mongodb",
			port: s.Config.MongoDBListenPort(),
			pid:  s.Layout.MongoDBPID(),
			log:  "mongodb.log",
			exe: func() (string, error) {
				return s.toolExe("mongodb")
			},
			args: func(string) ([]string, error) {
				if err := os.MkdirAll(s.Layout.MongoDBData(), 0o755); err != nil {
					return nil, err
				}
				return s.mongoArgs()
			},
			stop: func() {
				shell, err := s.toolExe("mongosh")
				if err != nil {
					return
				}
				runQuiet(shell, mongodb.ShutdownArgs(s.Config.MongoDBListenPort()), 10*time.Second)
			},
		},
		{
			name: "postgres",
			port: s.Config.PostgresListenPort(),
			pid:  s.Layout.PostgresPID(),
			log:  "postgres.log",
			exe: func() (string, error) {
				return s.toolExe("postgres")
			},
			args: func(exe string) ([]string, error) {
				data := s.Layout.PostgresData()
				if postgres.NeedsInit(data) {
					if err := initPostgres(exe, data); err != nil {
						return nil, err
					}
				} else if err := checkPostgresMajor(exe, data); err != nil {
					return nil, err
				}
				return s.postgresArgs(data)
			},
			stop: func() {
				exe, err := s.toolExe("postgres")
				if err != nil {
					return
				}
				if ctl := phloruntime.FindSibling(exe, "pg_ctl.exe", "pg_ctl"); ctl != "" {
					runQuiet(ctl, postgres.StopArgs(s.Layout.PostgresData()), 25*time.Second)
				}
			},
		},
	}
}

func (s *Supervisor) toolExe(kind string) (string, error) {
	return phloruntime.ResolveTool(*s.Manifest, s.Layout, kind, s.Config.Tool(kind), runtime.GOOS, runtime.GOARCH)
}

// addonPortChanged reports whether the running child was started with another port.
func (s *Supervisor) addonPortChanged(item addon) bool {
	ch := s.procs[item.name]
	if ch == nil || ch.cmd == nil {
		return false
	}
	ports := []int{item.port}
	if item.name == "mailpit" {
		ports = append(ports, s.Config.MailpitUIListenPort())
	}
	for _, port := range ports {
		want := strconv.Itoa(port)
		found := false
		for _, arg := range ch.cmd.Args {
			if arg == want || strings.HasSuffix(arg, ":"+want) {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

func initPostgres(exe, data string) error {
	initdb := phloruntime.FindSibling(exe, "initdb.exe", "initdb")
	if initdb == "" {
		return errs.New("PostgreSQL: initdb is not installed next to " + exe)
	}
	if err := os.MkdirAll(filepath.Dir(data), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, initdb, postgres.InitArgs(data)...)
	cmd.Dir = filepath.Dir(initdb)
	proc.Configure(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("PostgreSQL: initialize %s: %w %s", data, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// checkPostgresMajor refuses to start a server on a data folder made by another major version.
// An unreadable version is left to PostgreSQL, which logs its own error.
func checkPostgresMajor(exe, data string) error {
	dataMajor := postgres.DataMajor(data)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--version")
	cmd.Dir = filepath.Dir(exe)
	proc.Configure(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	serverMajor := postgres.ServerMajor(string(out))
	if dataMajor == "" || serverMajor == "" || dataMajor == serverMajor {
		return nil
	}
	return errs.New(postgres.MajorMismatch(data, dataMajor, serverMajor).Error())
}

func runQuiet(exe string, args []string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = filepath.Dir(exe)
	proc.Configure(cmd)
	_ = cmd.Run()
}

func (s *Supervisor) reserveAddonPorts() error {
	for _, item := range s.addons() {
		if !s.addonEnabled(item.name) {
			continue
		}
		if _, err := item.exe(); err != nil {
			continue
		}
		if err := freePort(item.port); err != nil {
			return fmt.Errorf("%s: %w", label(item.name), err)
		}
		if item.name == "mailpit" {
			if err := freePort(s.Config.MailpitUIListenPort()); err != nil {
				return fmt.Errorf("Mailpit: %w", err)
			}
		}
	}
	return nil
}

func (s *Supervisor) startAddons() error {
	for _, item := range s.addons() {
		if !s.addonEnabled(item.name) {
			continue
		}
		if err := s.launchAddon(item); err != nil {
			return err
		}
	}
	return nil
}

// StartAddon launches one optional service when the web stack is already running.
func (s *Supervisor) StartAddon(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.nginxAlive() || !s.addonEnabled(name) {
		return nil
	}
	for _, item := range s.addons() {
		if item.name != name {
			continue
		}
		if s.addonAlive(item) {
			return nil
		}
		if err := freePort(item.port); err != nil {
			return fmt.Errorf("%s: %w", label(item.name), err)
		}
		if item.name == "mailpit" {
			if err := freePort(s.Config.MailpitUIListenPort()); err != nil {
				return fmt.Errorf("Mailpit: %w", err)
			}
		}
		return s.launchAddon(item)
	}
	return errs.New("unknown service")
}

func (s *Supervisor) launchAddon(item addon) error {
	if !s.addonEnabled(item.name) {
		return nil
	}
	exe, err := item.exe()
	if err != nil {
		return nil
	}
	return s.launchAddonExe(item, exe)
}

// StopAddon stops one optional service or MariaDB by name.
func (s *Supervisor) StopAddon(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "mariadb" {
		s.stopMariaDB()
		return nil
	}
	for _, item := range s.addons() {
		if item.name == name {
			s.stopAddonItem(item)
			return nil
		}
	}
	return errs.New("unknown service")
}

func (s *Supervisor) addonEnabled(name string) bool {
	switch name {
	case "redis":
		return s.Config.RedisOn()
	case "memcached":
		return s.Config.MemcachedOn()
	case "mailpit":
		return s.Config.MailpitOn()
	case "mongodb":
		return s.Config.MongoDBOn()
	case "postgres":
		return s.Config.PostgresOn()
	default:
		return true
	}
}

func (s *Supervisor) launchAddonExe(item addon, exe string) error {
	args, err := item.args(exe)
	if err != nil {
		return err
	}
	logPath := s.Layout.Log(item.log)
	ch, err := startLogged(exe, args, filepath.Dir(exe), nil, logPath)
	if err != nil {
		return fmt.Errorf("%s: %w", label(item.name), err)
	}
	s.procs[item.name] = ch
	pid := ch.cmd.Process.Pid
	if err := waitAliveFor(pid, logPath, 2*time.Second); err != nil {
		s.killPID(pid)
		closeChild(ch)
		delete(s.procs, item.name)
		return s.customHint(item.name, fmt.Errorf("%s: %w", label(item.name), err))
	}
	return writePID(item.pid, pid)
}

func (s *Supervisor) stopAddonItem(item addon) {
	pid := readPID(item.pid)
	if ch := s.procs[item.name]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		pid = ch.cmd.Process.Pid
	}
	if item.stop != nil && proc.Alive(pid) {
		item.stop()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) && proc.Alive(pid) {
			time.Sleep(100 * time.Millisecond)
		}
	}
	s.killPID(pid)
	if ch := s.procs[item.name]; ch != nil {
		closeChild(ch)
		delete(s.procs, item.name)
	}
	_ = os.Remove(item.pid)
}

func (s *Supervisor) AddonRunning(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.addons() {
		if item.name == name {
			return s.addonAlive(item)
		}
	}
	return false
}

func (s *Supervisor) addonAlive(item addon) bool {
	if ch := s.procs[item.name]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil && proc.Alive(ch.cmd.Process.Pid) {
		return true
	}
	pid := readPID(item.pid)
	return pid > 0 && proc.Alive(pid)
}

func label(name string) string {
	switch name {
	case "redis":
		return "Redis"
	case "memcached":
		return "Memcached"
	case "mailpit":
		return "Mailpit"
	case "mongodb":
		return "MongoDB"
	case "postgres":
		return "PostgreSQL"
	case "mariadb":
		return "MariaDB"
	default:
		return name
	}
}

func (s *Supervisor) nginxBinary() (string, error) {
	return phloruntime.ResolveNginx(*s.Manifest, s.Layout, s.Config.Nginx, runtime.GOOS, runtime.GOARCH)
}

func (s *Supervisor) mariaBinary() (string, error) {
	return phloruntime.ResolveMariaDB(*s.Manifest, s.Layout, s.Config.MariaDB, runtime.GOOS, runtime.GOARCH)
}

func (s *Supervisor) stopLocked() {
	s.stopShareLocked()
	s.stopCronLocked()
	items := s.addons()
	for i := len(items) - 1; i >= 0; i-- {
		s.stopAddonItem(items[i])
	}
	s.stopMariaDB()
	s.stopNginxLocked()
	matches, _ := filepath.Glob(filepath.Join(s.Layout.Run(), "php-*.pid"))
	for _, match := range matches {
		s.killPID(readPID(match))
		_ = os.Remove(match)
	}
	for key, ch := range s.procs {
		if ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
			_ = proc.Stop(ch.cmd.Process.Pid)
		}
		closeChild(ch)
		delete(s.procs, key)
	}
}

func (s *Supervisor) stopNginxLocked() {
	exe := ""
	if ch := s.procs["nginx"]; ch != nil && ch.cmd != nil {
		exe = ch.cmd.Path
	}
	if exe == "" {
		if found, err := s.nginxBinary(); err == nil {
			exe = found
		}
	}
	master := readPID(s.Layout.NginxPID())
	if ch := s.procs["nginx"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		master = ch.cmd.Process.Pid
	}
	if exe != "" {
		_ = s.signalNginx(exe, "quit")
		waitExit(master, 3*time.Second)
	}
	if ch := s.procs["nginx"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		_ = proc.Stop(ch.cmd.Process.Pid)
	}
	s.killPID(readPID(s.Layout.NginxPID()))
	_ = os.Remove(s.Layout.NginxPID())
	if ch := s.procs["nginx"]; ch != nil {
		closeChild(ch)
		delete(s.procs, "nginx")
	}
	// A worker that is still closing connections can hold the ports for a moment after the master is gone.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.httpPortsFree() != nil {
		time.Sleep(100 * time.Millisecond)
	}
}

func waitExit(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for pid > 0 && proc.Alive(pid) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Supervisor) stopPHP(version string) {
	pid := readPID(s.Layout.PHPPID(version))
	if ch := s.procs["php:"+version]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		pid = ch.cmd.Process.Pid
	}
	s.killPID(pid)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && proc.Alive(pid) {
		time.Sleep(50 * time.Millisecond)
	}
	if ch := s.procs["php:"+version]; ch != nil {
		closeChild(ch)
		delete(s.procs, "php:"+version)
	}
	_ = os.Remove(s.Layout.PHPPID(version))
}

func (s *Supervisor) stopUnneeded(jobs []phpJob) {
	needed := map[string]bool{}
	for _, job := range jobs {
		needed[job.Version] = true
	}
	matches, _ := filepath.Glob(filepath.Join(s.Layout.Run(), "php-*.pid"))
	for _, match := range matches {
		version := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(match), ".pid"), "php-")
		if !needed[version] {
			s.stopPHP(version)
		}
	}
}

func (s *Supervisor) nginxAlive() bool {
	if proc.Alive(readPID(s.Layout.NginxPID())) {
		return true
	}
	if ch := s.procs["nginx"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		return proc.Alive(ch.cmd.Process.Pid)
	}
	return false
}

func (s *Supervisor) clearStalePID(path string) {
	pid := readPID(path)
	if pid > 0 && !proc.Alive(pid) {
		_ = os.Remove(path)
	}
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func (s *Supervisor) killPID(pid int) {
	if proc.Alive(pid) {
		_ = proc.Stop(pid)
	}
}

func startLogged(exe string, args []string, dir string, extra []string, logPath string) (*child, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extra...)
	cmd.Stdout = file
	cmd.Stderr = file
	proc.Configure(cmd)
	if err := cmd.Start(); err != nil {
		file.Close()
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return &child{cmd: cmd, log: file}, nil
}

func closeChild(ch *child) {
	if ch != nil && ch.log != nil {
		_ = ch.log.Close()
	}
}

func waitAlive(pid int, logPath string) error {
	return waitAliveFor(pid, logPath, 400*time.Millisecond)
}

func waitAliveFor(pid int, logPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if !proc.Alive(pid) {
			return fmt.Errorf("process %d exited\n%s", pid, tail(logPath))
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func tail(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(body) > 2000 {
		body = body[len(body)-2000:]
	}
	return strings.TrimSpace(string(body))
}

func writePID(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

func readPID(path string) int {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(body)))
	return pid
}

func (s *Supervisor) startCron() error {
	if runtime.GOOS != "windows" || !s.Config.CronOn() {
		return nil
	}
	exe, err := phloruntime.ResolveCronical(*s.Manifest, s.Layout, "", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil
	}
	if s.cronAlive() {
		return nil
	}
	s.clearStalePID(s.Layout.CronicalPID())
	logDir := s.Layout.CronicalLogDir()
	if err := cronical.WriteRuntimeConfig(filepath.Dir(exe), logDir); err != nil {
		return err
	}
	phpExe := ""
	if cli, cliErr := phloruntime.ResolveCLI(*s.Manifest, s.Layout, s.Config.DefaultPHP); cliErr == nil {
		phpExe = cli
	}
	job := s.Layout.CronicalDat()
	if err := cronical.Seed(job, phpExe); err != nil {
		return err
	}
	var extra []string
	if phpExe != "" {
		extra = append(extra, "PATH="+filepath.Dir(phpExe)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	logPath := s.Layout.Log("cronical.log")
	ch, err := startLogged(exe, cronical.Command(job), filepath.Dir(exe), extra, logPath)
	if err != nil {
		return err
	}
	s.procs["cronical"] = ch
	if err := writePID(s.Layout.CronicalPID(), ch.cmd.Process.Pid); err != nil {
		s.stopCronLocked()
		return err
	}
	if err := waitAlive(ch.cmd.Process.Pid, logPath); err != nil {
		s.stopCronLocked()
		return fmt.Errorf("Cronical: %w", err)
	}
	return nil
}

// StartCron starts Cronical when Settings has it on and the binary is installed.
func (s *Supervisor) StartCron() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCron()
}

// StopCron stops the Cronical process Phlo started.
func (s *Supervisor) StopCron() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCronLocked()
	return nil
}

// PHPPool is one PHP version whose process is still alive.
type PHPPool struct {
	Version string
	Port    int
}

// PHPPools lists the PHP versions Phlo is running, with the port Nginx uses.
func (s *Supervisor) PHPPools() []PHPPool {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := map[string]struct{}{}
	for key := range s.procs {
		version, ok := strings.CutPrefix(key, "php:")
		if ok && s.phpAlive(version) {
			versions[version] = struct{}{}
		}
	}
	matches, _ := filepath.Glob(filepath.Join(s.Layout.Run(), "php-*.pid"))
	for _, match := range matches {
		version := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(match), ".pid"), "php-")
		if version != "" && proc.Alive(readPID(match)) {
			versions[version] = struct{}{}
		}
	}
	list := make([]string, 0, len(versions))
	for version := range versions {
		list = append(list, version)
	}
	sort.Strings(list)
	pools := make([]PHPPool, 0, len(list))
	for _, version := range list {
		port, err := php.Port(version)
		if err != nil {
			continue
		}
		pools = append(pools, PHPPool{Version: version, Port: port})
	}
	return pools
}

func (s *Supervisor) phpAlive(version string) bool {
	if proc.Alive(readPID(s.Layout.PHPPID(version))) {
		return true
	}
	if ch := s.procs["php:"+version]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		return proc.Alive(ch.cmd.Process.Pid)
	}
	return false
}

// CronRunning reports the Cronical process Phlo started.
func (s *Supervisor) CronRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cronAlive()
}

func (s *Supervisor) cronAlive() bool {
	if proc.Alive(readPID(s.Layout.CronicalPID())) {
		return true
	}
	if ch := s.procs["cronical"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		return proc.Alive(ch.cmd.Process.Pid)
	}
	return false
}

func (s *Supervisor) stopCronLocked() {
	pid := readPID(s.Layout.CronicalPID())
	if ch := s.procs["cronical"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		pid = ch.cmd.Process.Pid
	}
	s.killPID(pid)
	if ch := s.procs["cronical"]; ch != nil {
		closeChild(ch)
		delete(s.procs, "cronical")
	}
	_ = os.Remove(s.Layout.CronicalPID())
}

// Share starts one ngrok tunnel for host. Nginx must already be running.
func (s *Supervisor) Share(host string, httpPort int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.nginxAlive() {
		return "", errs.New("Start Phlo before sharing a project. Nginx is not running.")
	}
	if !ngrok.TokenSet(s.Layout.NgrokConfig()) {
		return "", errs.New("Add an ngrok authtoken in Settings. Get one at " + ngrok.Dashboard)
	}
	s.stopShareLocked()
	deadline := time.Now().Add(2 * time.Second)
	var portErr error
	for {
		portErr = freePort(ngrok.InspectPort)
		if portErr == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if portErr != nil {
		return "", errs.New("127.0.0.1:4049 is already in use. The other program was left running.")
	}
	exe, err := phloruntime.ResolveNgrok(*s.Manifest, s.Layout, "", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", errs.New("ngrok is not installed. Download it under Runtimes.")
	}
	logPath := s.Layout.Log("ngrok.log")
	ch, err := startLogged(exe, ngrok.Command(s.Layout.NgrokConfig(), host, httpPort), filepath.Dir(exe), nil, logPath)
	if err != nil {
		return "", err
	}
	s.procs["ngrok"] = ch
	if err := writePID(s.Layout.NgrokPID(), ch.cmd.Process.Pid); err != nil {
		s.stopShareLocked()
		return "", err
	}
	if err := waitAlive(ch.cmd.Process.Pid, logPath); err != nil {
		s.stopShareLocked()
		return "", errs.New("ngrok stopped. " + ngrok.RedactLog(tail(logPath)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	url, err := ngrok.WaitPublicURL(ctx)
	if err != nil || url == "" {
		detail := ngrok.RedactLog(tail(logPath))
		s.stopShareLocked()
		if detail == "" {
			return "", errs.New("ngrok did not publish a URL.")
		}
		return "", errs.New("ngrok did not publish a URL. " + detail)
	}
	s.shareHost = host
	s.shareURL = url
	return url, nil
}

// StopShare stops the ngrok process Phlo started.
func (s *Supervisor) StopShare() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopShareLocked()
	return nil
}

// ShareInfo is the project host and public URL of the tunnel Phlo started.
func (s *Supervisor) ShareInfo() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.shareAlive() {
		return "", ""
	}
	return s.shareHost, s.shareURL
}

func (s *Supervisor) shareAlive() bool {
	if proc.Alive(readPID(s.Layout.NgrokPID())) {
		return true
	}
	if ch := s.procs["ngrok"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		return proc.Alive(ch.cmd.Process.Pid)
	}
	return false
}

func (s *Supervisor) stopShareLocked() {
	pid := readPID(s.Layout.NgrokPID())
	if ch := s.procs["ngrok"]; ch != nil && ch.cmd != nil && ch.cmd.Process != nil {
		pid = ch.cmd.Process.Pid
	}
	s.killPID(pid)
	if ch := s.procs["ngrok"]; ch != nil {
		closeChild(ch)
		delete(s.procs, "ngrok")
	}
	_ = os.Remove(s.Layout.NgrokPID())
	s.shareHost = ""
	s.shareURL = ""
}

func freePort(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return errs.New(fmt.Sprintf("port %d is not available: %v", port, err))
	}
	return ln.Close()
}
