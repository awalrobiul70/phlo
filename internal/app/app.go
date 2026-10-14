package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"phlo/internal/api"
	"phlo/internal/config"
	"phlo/internal/cronical"
	"phlo/internal/dbgui"
	"phlo/internal/errs"
	"phlo/internal/hosts"
	"phlo/internal/laravel"
	"phlo/internal/mariadb"
	"phlo/internal/ngrok"
	"phlo/internal/paths"
	"phlo/internal/php"
	"phlo/internal/phpmyadmin"
	"phlo/internal/proc"
	phloruntime "phlo/internal/runtime"
	"phlo/internal/site"
	"phlo/internal/supervisor"
	"phlo/internal/terminal"
	"phlo/internal/vcruntime"
	"phlo/internal/version"
)

// App is the Phlo controller.
type App struct {
	Layout     paths.Layout
	Config     config.Config
	Manifest   phloruntime.Manifest
	Supervisor *supervisor.Supervisor
	ApplyHosts func(entries []hosts.Entry) error

	mu           sync.Mutex
	vcMu         sync.Mutex
	updateMu     sync.Mutex
	hostsWarning string
}

func Open(root string) (*App, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := config.Ensure(root); err != nil {
		return nil, err
	}
	cfg, err := config.Load(filepath.Join(root, "config", "phlo.json"))
	if err != nil {
		return nil, err
	}
	manifest, err := phloruntime.Load(filepath.Join(root, "config", "manifest.json"))
	if err != nil {
		return nil, err
	}
	application := &App{
		Layout:   paths.Layout{Root: root}.WithFolders(folderValues(cfg)),
		Config:   cfg,
		Manifest: manifest,
	}
	if err := phloruntime.MigrateLegacyDirs(manifest, application.Layout, runtime.GOOS, runtime.GOARCH); err != nil {
		return nil, err
	}
	application.Supervisor = supervisor.New(application.Layout, &application.Config, &application.Manifest)
	return application, nil
}

func (a *App) Status() (api.Status, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	shareHost, shareURL := a.Supervisor.ShareInfo()
	return api.Status{
		Running:           a.Supervisor.Running(),
		DefaultPHP:        a.Config.DefaultPHP,
		TLD:               a.Config.TLD,
		HTTPPort:          a.Config.HTTPPort,
		SSLPort:           a.Config.SSLListenPort(),
		SSLEnabled:        a.Config.SSLOn(),
		HostIP:            a.Config.HostIPAddress(),
		HostIPs:           supervisor.LocalIPv4s(),
		DashboardPort:     a.Config.DashboardPort,
		MySQLPort:         a.Config.MySQLListenPort(),
		MySQLRunning:      a.Supervisor.DatabaseRunning(),
		MariaDBEnabled:    a.Config.MariaDBOn(),
		DataDir:           a.Layout.DataDir(),
		Root:              a.Layout.Root,
		HomeDir:           a.Layout.Home(),
		PHPMyAdmin:        a.phpMyAdminHost(),
		RedisPort:         a.Config.RedisListenPort(),
		RedisRunning:      a.Supervisor.AddonRunning("redis"),
		RedisEnabled:      a.Config.RedisOn(),
		MemcachedPort:     a.Config.MemcachedListenPort(),
		MemcachedRunning:  a.Supervisor.AddonRunning("memcached"),
		MemcachedEnabled:  a.Config.MemcachedOn(),
		MailpitSMTPPort:   a.Config.MailpitSMTPListenPort(),
		MailpitUIPort:     a.Config.MailpitUIListenPort(),
		MailpitRunning:    a.Supervisor.AddonRunning("mailpit"),
		MailpitEnabled:    a.Config.MailpitOn(),
		MailpitURL:        fmt.Sprintf("http://127.0.0.1:%d", a.Config.MailpitUIListenPort()),
		Redis:             phloruntime.SelectedVersion(a.Config.Redis, phloruntime.ListRedis(a.Manifest, a.Layout, runtime.GOOS, runtime.GOARCH)),
		Memcached:         phloruntime.SelectedVersion(a.Config.Memcached, phloruntime.ListMemcached(a.Manifest, a.Layout, runtime.GOOS, runtime.GOARCH)),
		Mailpit:           phloruntime.SelectedVersion(a.Config.Mailpit, phloruntime.ListMailpit(a.Manifest, a.Layout, runtime.GOOS, runtime.GOARCH)),
		Nginx:             phloruntime.SelectedVersion(a.Config.Nginx, phloruntime.ListNginx(a.Manifest, a.Layout, runtime.GOOS, runtime.GOARCH)),
		MariaDB:           phloruntime.SelectedVersion(a.Config.MariaDB, phloruntime.ListMariaDB(a.Manifest, a.Layout, runtime.GOOS, runtime.GOARCH)),
		MongoDBPort:       a.Config.MongoDBListenPort(),
		MongoDBRunning:    a.Supervisor.AddonRunning("mongodb"),
		MongoDBEnabled:    a.Config.MongoDBOn(),
		MongoDB:           a.toolVersion("mongodb"),
		MongoDBData:       a.Layout.MongoDBData(),
		PostgresPort:      a.Config.PostgresListenPort(),
		PostgresRunning:   a.Supervisor.AddonRunning("postgres"),
		PostgresEnabled:   a.Config.PostgresOn(),
		Postgres:          a.toolVersion("postgres"),
		PostgresData:      a.Layout.PostgresData(),
		HostsWarning:      a.hostsWarning,
		VCRuntime:         a.vcRuntime(),
		RootProblem:       rootProblem(a.Layout.Root, runtime.GOOS),
		Port80Taken:       supervisor.Port80Taken(a.Config.HTTPPort),
		PHPPools:          phpPools(a.Supervisor.PHPPools()),
		CronEnabled:       a.Config.CronOn(),
		CronRunning:       a.Supervisor.CronRunning(),
		Cronical:          phloruntime.SelectedVersion("", phloruntime.ListCronical(a.Manifest, a.Layout, runtime.GOOS, runtime.GOARCH)),
		CronicalInstalled: cronicalInstalled(a),
		NgrokTokenSet:     ngrok.TokenSet(a.Layout.NgrokConfig()),
		SQLyogInstalled:   dbgui.FindSQLyog() != "",
		ShareHost:         shareHost,
		ShareURL:          shareURL,
		Version:           version.Current,
	}, nil
}

// Release compares this build with config/release.json and, when updateUrl is
// set, with the published JSON at that address.
func (a *App) Release(ctx context.Context) (api.Release, error) {
	file, err := version.ReadFile(filepath.Join(a.Layout.Root, "config", "release.json"))
	if err != nil {
		return api.Release{}, err
	}
	latest := file.Version
	page := strings.TrimSpace(file.UpdateURL)
	var download string
	if page != "" {
		remote, ferr := version.Fetch(ctx, page)
		if ferr != nil {
			upToDate := !version.Newer(version.Current, latest)
			message := "You are up to date."
			if !upToDate {
				message = "Could not check for an update. " + ferr.Error()
			}
			return api.Release{
				Version:  version.Current,
				Title:    file.Title,
				Notes:    file.Notes,
				Latest:   latest,
				UpToDate: upToDate,
				Message:  message,
			}, nil
		}
		if strings.TrimSpace(remote.Version) != "" {
			latest = remote.Version
		}
		if strings.TrimSpace(remote.URL) != "" {
			page = remote.URL
		}
		if remote.Notes != "" && version.Newer(version.Current, remote.Version) {
			file.Notes = []string{remote.Notes}
		}
		download = version.ProgramDownload(remote.Download)
	}
	upToDate := !version.Newer(version.Current, latest)
	message := "You are up to date."
	updateURL := ""
	canUpdate := false
	if !upToDate {
		message = "Phlo " + latest + " is available. You are running " + version.Current + "."
		if strings.HasPrefix(page, "https://") {
			updateURL = page
		}
		canUpdate = download != ""
		if !canUpdate && updateURL == "" {
			message = "Phlo " + latest + " is listed in config/release.json. You are running " + version.Current + "."
		}
	} else {
		download = ""
	}
	return api.Release{
		Version:     version.Current,
		Title:       file.Title,
		Notes:       file.Notes,
		Latest:      latest,
		UpToDate:    upToDate,
		UpdateURL:   updateURL,
		DownloadURL: download,
		CanUpdate:   canUpdate,
		Message:     message,
	}, nil
}

// Update downloads the published program files and installs them beside this
// Phlo folder. Sites, databases, runtimes, and config/phlo.json stay in place.
func (a *App) Update(ctx context.Context) (string, error) {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	info, err := a.Release(ctx)
	if err != nil {
		return "", err
	}
	if info.UpToDate || !info.CanUpdate {
		if info.UpToDate {
			return "", errors.New("You are up to date.")
		}
		return "", errors.New("This release has no program download yet.")
	}
	if err := version.Install(ctx, a.Layout.Root, info.DownloadURL); err != nil {
		return "", err
	}
	return "Phlo " + info.Latest + " is installed. Phlo is restarting. Your sites and databases stay where they are.", nil
}

func cronicalInstalled(a *App) bool {
	_, err := phloruntime.ResolveCronical(a.Manifest, a.Layout, "", runtime.GOOS, runtime.GOARCH)
	return err == nil
}

func phpPools(pools []supervisor.PHPPool) []api.PHPPool {
	if len(pools) == 0 {
		return nil
	}
	out := make([]api.PHPPool, len(pools))
	for i, pool := range pools {
		out[i] = api.PHPPool{Version: pool.Version, Port: pool.Port}
	}
	return out
}

func (a *App) toolVersion(kind string) string {
	return phloruntime.SelectedVersion(a.Config.Tool(kind), phloruntime.ListTool(a.Manifest, a.Layout, kind, runtime.GOOS, runtime.GOARCH))
}

func (a *App) toolExe(kind string) (string, string, error) {
	version := a.toolVersion(kind)
	exe, err := phloruntime.ResolveTool(a.Manifest, a.Layout, kind, version, runtime.GOOS, runtime.GOARCH)
	return version, exe, err
}

func (a *App) ListSites() ([]site.Site, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.listSites()
}

func (a *App) listSites() ([]site.Site, error) {
	sites, err := site.Scan(a.Layout.Home(), site.Options{TLD: a.Config.TLD, DefaultPHP: a.Config.DefaultPHP})
	if err != nil {
		return nil, err
	}
	a.attachPHPPaths(sites)
	return sites, nil
}

func (a *App) attachPHPPaths(sites []site.Site) {
	cache := map[string]string{}
	for i := range sites {
		if sites[i].PublicHTML != nil {
			sites[i].PublicHTML.PHPPath = a.phpCLI(cache, sites[i].PublicHTML.PHP)
		}
		for j := range sites[i].Subdomains {
			sites[i].Subdomains[j].PHPPath = a.phpCLI(cache, sites[i].Subdomains[j].PHP)
		}
	}
}

func (a *App) phpCLI(cache map[string]string, version string) string {
	if path, ok := cache[version]; ok {
		return path
	}
	path, err := phloruntime.ResolveCLI(a.Manifest, a.Layout, version)
	if err != nil {
		path = ""
	}
	cache[version] = path
	return path
}

func (a *App) CreateSite(account string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := site.ValidateAccount(account); err != nil {
		return "", err
	}
	sites, err := a.listSites()
	if err != nil {
		return "", err
	}
	if err := site.HostInUse(sites, site.DefaultHost(account, a.Config.TLD)); err != nil {
		return "", err
	}
	if err := site.Create(a.Layout.Home(), account); err != nil {
		return "", err
	}
	warn, err := a.syncHosts()
	if err != nil {
		return "", err
	}
	if a.Supervisor.Running() {
		if err := a.Supervisor.Reload(); err != nil {
			return warn, err
		}
	}
	return warn, nil
}

func (a *App) AddSubdomain(account, name string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sites, err := a.listSites()
	if err != nil {
		return "", err
	}
	current, ok := findSite(sites, account)
	if !ok {
		return "", errs.New("account not found")
	}
	if err := site.ValidateLabel(name); err != nil {
		return "", errs.New("subdomain name must use lowercase letters, numbers, and hyphens")
	}
	candidate := name + "." + current.EffectiveHost
	if err := site.HostInUse(sites, candidate); err != nil {
		return "", err
	}
	if err := site.AddSubdomain(a.Layout.Home(), account, name); err != nil {
		return "", err
	}
	warn, err := a.syncHosts()
	if err != nil {
		return "", err
	}
	if a.Supervisor.Running() {
		if err := a.Supervisor.Reload(); err != nil {
			return warn, err
		}
	}
	return warn, nil
}

func (a *App) SetHost(account, host string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sites, err := a.listSites()
	if err != nil {
		return "", err
	}
	index := -1
	for i := range sites {
		if sites[i].Account == account {
			index = i
			break
		}
	}
	if index < 0 {
		return "", errs.New("account not found")
	}
	updated, err := site.WithHost(sites[index], host, a.Config.TLD)
	if err != nil {
		return "", err
	}
	if site.ReservesPHPMyAdmin(updated, a.Config.TLD) {
		return "", errs.New(site.PHPMyAdminHost(a.Config.TLD) + " is reserved")
	}
	next := append([]site.Site(nil), sites...)
	next[index] = updated
	if err := site.Duplicate(next); err != nil {
		return "", err
	}
	if err := site.UpdateHost(a.Layout.SiteFile(account), updated.CustomHost); err != nil {
		return "", err
	}
	warn, err := a.syncHosts()
	if err != nil {
		return "", err
	}
	if a.Supervisor.Running() {
		if err := a.Supervisor.Reload(); err != nil {
			return warn, err
		}
	}
	return warn, nil
}

func (a *App) SetPHP(account, target, version string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if version != "" {
		if err := php.Accept(version); err != nil {
			return "", errs.New("php version must look like 8.3 or 7.4.9")
		}
	}
	sites, err := a.listSites()
	if err != nil {
		return "", err
	}
	current, ok := findSite(sites, account)
	if !ok {
		return "", errs.New("account not found")
	}
	if target == site.PublicRoot {
		if current.PublicHTML == nil {
			return "", errs.New("public_html does not exist")
		}
	} else if !hasSubdomain(current, target) {
		return "", errs.New("subdomain not found")
	}
	resolved := version
	if resolved == "" {
		resolved = a.Config.DefaultPHP
	}
	if _, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, resolved, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	if err := site.UpdatePHP(a.Layout.SiteFile(account), target, version); err != nil {
		return "", err
	}
	if a.Supervisor.Running() {
		if err := a.Supervisor.Reload(); err != nil {
			return "", err
		}
	}
	return "", nil
}

func (a *App) SetDefaultPHP(version string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := php.Accept(version); err != nil {
		return "", errs.New("php version must look like 8.3 or 7.4.9")
	}
	if _, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	a.Config.DefaultPHP = version
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		return "", err
	}
	if a.Supervisor.Running() {
		if err := a.Supervisor.Reload(); err != nil {
			return "", err
		}
	}
	return "", nil
}

func (a *App) SetNginx(version string) (string, error) {
	return a.useComponent(version, "Nginx", func(version string) error {
		if _, err := phloruntime.ResolveNginx(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
			return err
		}
		a.Config.Nginx = version
		if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
			return err
		}
		return a.Supervisor.SwitchNginx()
	})
}

func (a *App) SetMariaDB(version string) (string, error) {
	return a.useComponent(version, "MariaDB", func(version string) error {
		if _, err := phloruntime.ResolveMariaDB(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
			return err
		}
		a.Config.MariaDB = version
		if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
			return err
		}
		return a.Supervisor.SwitchMariaDB()
	})
}

func (a *App) SetRedis(version string) (string, error) {
	return a.useComponent(version, "Redis", func(version string) error {
		if _, err := phloruntime.ResolveRedis(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
			return err
		}
		a.Config.Redis = version
		if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
			return err
		}
		return a.Supervisor.SwitchAddon("redis")
	})
}

func (a *App) SetMemcached(version string) (string, error) {
	return a.useComponent(version, "Memcached", func(version string) error {
		if _, err := phloruntime.ResolveMemcached(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
			return err
		}
		a.Config.Memcached = version
		if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
			return err
		}
		return a.Supervisor.SwitchAddon("memcached")
	})
}

func (a *App) SetMailpit(version string) (string, error) {
	return a.useComponent(version, "Mailpit", func(version string) error {
		if _, err := phloruntime.ResolveMailpit(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
			return err
		}
		a.Config.Mailpit = version
		if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
			return err
		}
		return a.Supervisor.SwitchAddon("mailpit")
	})
}

// SettingsPatch is a partial update for ports and service enable flags.
type SettingsPatch struct {
	DefaultPHP       *string
	Nginx            *string
	MariaDB          *string
	Redis            *string
	Memcached        *string
	Mailpit          *string
	HTTPPort         *int
	SSLPort          *int
	HostIP           *string
	MySQLPort        *int
	RedisPort        *int
	MemcachedPort    *int
	MailpitSMTPPort  *int
	MailpitUIPort    *int
	MongoDBPort      *int
	PostgresPort     *int
	SSLEnabled       *bool
	MariaDBEnabled   *bool
	RedisEnabled     *bool
	MemcachedEnabled *bool
	MailpitEnabled   *bool
	MongoDBEnabled   *bool
	PostgresEnabled  *bool
	CronEnabled      *bool
	// Tools selects a version per tools kind. An empty version means the default build.
	Tools map[string]string
	// ToolPath turns a toolchain on or off for the project Terminal PATH.
	ToolPath map[string]bool
}

func (a *App) ApplySettings(patch SettingsPatch) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := a.Config
	next := a.Config
	if patch.HTTPPort != nil {
		next.HTTPPort = *patch.HTTPPort
	}
	if patch.SSLPort != nil {
		next.SSLPort = *patch.SSLPort
	}
	if patch.SSLEnabled != nil {
		next.SSLEnabled = config.BoolPtr(*patch.SSLEnabled)
	}
	if patch.HostIP != nil {
		ip, err := hostIPSetting(*patch.HostIP)
		if err != nil {
			return "", err
		}
		next.HostIP = ip
	}
	if patch.MySQLPort != nil {
		next.MySQLPort = *patch.MySQLPort
	}
	if patch.RedisPort != nil {
		next.RedisPort = *patch.RedisPort
	}
	if patch.MemcachedPort != nil {
		next.MemcachedPort = *patch.MemcachedPort
	}
	if patch.MailpitSMTPPort != nil {
		next.MailpitSMTPPort = *patch.MailpitSMTPPort
	}
	if patch.MailpitUIPort != nil {
		next.MailpitUIPort = *patch.MailpitUIPort
	}
	if patch.MariaDBEnabled != nil {
		next.MariaDBEnabled = config.BoolPtr(*patch.MariaDBEnabled)
	}
	if patch.RedisEnabled != nil {
		next.RedisEnabled = config.BoolPtr(*patch.RedisEnabled)
	}
	if patch.MemcachedEnabled != nil {
		next.MemcachedEnabled = config.BoolPtr(*patch.MemcachedEnabled)
	}
	if patch.MailpitEnabled != nil {
		next.MailpitEnabled = config.BoolPtr(*patch.MailpitEnabled)
	}
	if patch.MongoDBPort != nil {
		next.MongoDBPort = *patch.MongoDBPort
	}
	if patch.PostgresPort != nil {
		next.PostgresPort = *patch.PostgresPort
	}
	if patch.MongoDBEnabled != nil {
		next.MongoDBEnabled = config.BoolPtr(*patch.MongoDBEnabled)
	}
	if patch.PostgresEnabled != nil {
		next.PostgresEnabled = config.BoolPtr(*patch.PostgresEnabled)
	}
	if patch.CronEnabled != nil {
		next.CronEnabled = config.BoolPtr(*patch.CronEnabled)
	}
	for _, kind := range sortedKeys(patch.ToolPath) {
		spec, ok := phloruntime.LookupToolSpec(kind)
		if !ok || !spec.Toolchain {
			return "", errs.New("unknown toolchain " + kind)
		}
		next = next.WithToolOnPath(kind, patch.ToolPath[kind])
	}
	if err := next.Validate(); err != nil {
		return "", errs.New(err.Error())
	}
	if err := checkNewHostIP(prev, next); err != nil {
		return "", err
	}
	a.Config = next
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		a.Config = prev
		return "", err
	}
	if err := a.syncServiceState(prev); err != nil {
		return "", err
	}
	var warn, hostsWarn string
	var err error
	if prev.HostIPAddress() != a.Config.HostIPAddress() {
		if hostsWarn, err = a.syncHosts(); err != nil {
			return "", err
		}
	}
	if patch.DefaultPHP != nil {
		warn, err = a.setDefaultPHPLocked(*patch.DefaultPHP)
	}
	if err == nil && patch.Nginx != nil {
		warn, err = a.setNginxLocked(*patch.Nginx)
	}
	if err == nil && patch.MariaDB != nil {
		warn, err = a.setMariaDBLocked(*patch.MariaDB)
	}
	if err == nil && patch.Redis != nil {
		warn, err = a.setRedisLocked(*patch.Redis)
	}
	if err == nil && patch.Memcached != nil {
		warn, err = a.setMemcachedLocked(*patch.Memcached)
	}
	if err == nil && patch.Mailpit != nil {
		warn, err = a.setMailpitLocked(*patch.Mailpit)
	}
	for _, kind := range sortedKeys(patch.Tools) {
		if err != nil {
			break
		}
		warn, err = a.setToolLocked(kind, patch.Tools[kind])
	}
	if warn == "" {
		warn = hostsWarn
	}
	return warn, err
}

func (a *App) setToolLocked(kind, version string) (string, error) {
	spec, ok := phloruntime.LookupToolSpec(kind)
	if !ok {
		return "", errs.New("unknown tool " + kind)
	}
	next := a.Config.WithTool(kind, version)
	if err := next.Validate(); err != nil {
		return "", errs.New(spec.Label + " version must look like 24.21.0")
	}
	if _, err := phloruntime.ResolveTool(a.Manifest, a.Layout, kind, version, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	prev := a.Config
	a.Config = next
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		a.Config = prev
		return "", err
	}
	if spec.Service {
		return "", a.Supervisor.SwitchAddon(kind)
	}
	return "", nil
}

func sortedKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (a *App) syncServiceState(prev config.Config) error {
	if !a.Supervisor.Running() {
		return nil
	}
	if prev.HTTPPort != a.Config.HTTPPort || prev.SSLOn() != a.Config.SSLOn() || prev.SSLListenPort() != a.Config.SSLListenPort() || prev.HostIP != a.Config.HostIP {
		if err := a.Supervisor.RestartHTTP(); err != nil {
			return err
		}
	}
	if prev.MariaDBOn() != a.Config.MariaDBOn() || prev.MySQLListenPort() != a.Config.MySQLListenPort() {
		if a.Config.MariaDBOn() {
			if err := a.Supervisor.StartMariaDB(); err != nil {
				return err
			}
			if prev.MySQLListenPort() != a.Config.MySQLListenPort() {
				if err := a.Supervisor.SwitchMariaDB(); err != nil {
					return err
				}
			}
		} else if err := a.Supervisor.StopAddon("mariadb"); err != nil {
			return err
		}
	}
	for _, name := range []string{"redis", "memcached", "mailpit", "mongodb", "postgres"} {
		was := addonWasOn(prev, name)
		now := a.addonEnabled(name)
		portChanged := addonPort(prev, name) != addonPort(a.Config, name)
		if !now {
			if err := a.Supervisor.StopAddon(name); err != nil {
				return err
			}
			continue
		}
		if !was {
			if err := a.Supervisor.StartAddon(name); err != nil {
				return err
			}
			continue
		}
		if portChanged {
			if err := a.Supervisor.SwitchAddon(name); err != nil {
				return err
			}
		}
	}
	if prev.CronOn() != a.Config.CronOn() {
		if a.Config.CronOn() {
			return a.Supervisor.StartCron()
		}
		return a.Supervisor.StopCron()
	}
	return nil
}

func addonWasOn(cfg config.Config, name string) bool {
	switch name {
	case "redis":
		return cfg.RedisOn()
	case "memcached":
		return cfg.MemcachedOn()
	case "mailpit":
		return cfg.MailpitOn()
	case "mongodb":
		return cfg.MongoDBOn()
	case "postgres":
		return cfg.PostgresOn()
	default:
		return false
	}
}

func addonPort(cfg config.Config, name string) int {
	switch name {
	case "redis":
		return cfg.RedisListenPort()
	case "memcached":
		return cfg.MemcachedListenPort()
	case "mailpit":
		return cfg.MailpitSMTPListenPort()*100000 + cfg.MailpitUIListenPort()
	case "mongodb":
		return cfg.MongoDBListenPort()
	case "postgres":
		return cfg.PostgresListenPort()
	default:
		return 0
	}
}

func (a *App) addonEnabled(name string) bool {
	return addonWasOn(a.Config, name)
}

func (a *App) setDefaultPHPLocked(version string) (string, error) {
	if err := php.Accept(version); err != nil {
		return "", errs.New("PHP version must look like 8.3")
	}
	if _, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	a.Config.DefaultPHP = version
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		return "", err
	}
	if a.Supervisor.Running() {
		return "", a.Supervisor.Reload()
	}
	return "", nil
}

func (a *App) setNginxLocked(version string) (string, error) {
	if err := php.Accept(version); err != nil {
		return "", errs.New("Nginx version must look like 1.30.5")
	}
	if _, err := phloruntime.ResolveNginx(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	a.Config.Nginx = version
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		return "", err
	}
	return "", a.Supervisor.SwitchNginx()
}

func (a *App) setMariaDBLocked(version string) (string, error) {
	if err := php.Accept(version); err != nil {
		return "", errs.New("MariaDB version must look like 1.30.5")
	}
	if _, err := phloruntime.ResolveMariaDB(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	a.Config.MariaDB = version
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		return "", err
	}
	return "", a.Supervisor.SwitchMariaDB()
}

func (a *App) setRedisLocked(version string) (string, error) {
	if err := php.Accept(version); err != nil {
		return "", errs.New("Redis version must look like 1.30.5")
	}
	if _, err := phloruntime.ResolveRedis(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	a.Config.Redis = version
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		return "", err
	}
	return "", a.Supervisor.SwitchAddon("redis")
}

func (a *App) setMemcachedLocked(version string) (string, error) {
	if err := php.Accept(version); err != nil {
		return "", errs.New("Memcached version must look like 1.30.5")
	}
	if _, err := phloruntime.ResolveMemcached(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	a.Config.Memcached = version
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		return "", err
	}
	return "", a.Supervisor.SwitchAddon("memcached")
}

func (a *App) setMailpitLocked(version string) (string, error) {
	if err := php.Accept(version); err != nil {
		return "", errs.New("Mailpit version must look like 1.30.5")
	}
	if _, err := phloruntime.ResolveMailpit(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	a.Config.Mailpit = version
	if err := config.Save(a.Layout.ConfigFile(), a.Config); err != nil {
		return "", err
	}
	return "", a.Supervisor.SwitchAddon("mailpit")
}

func (a *App) useComponent(version, label string, apply func(string) error) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := php.Accept(version); err != nil {
		return "", errs.New(label + " version must look like 1.30.5")
	}
	if err := apply(version); err != nil {
		return "", err
	}
	return "", nil
}

func (a *App) OpenTerminal(kind, account, target string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch kind {
	case "mysql":
		server, err := phloruntime.ResolveMariaDB(a.Manifest, a.Layout, a.Config.MariaDB, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", err
		}
		client, err := phloruntime.ResolveMySQLClient(server)
		if err != nil {
			return "", err
		}
		args := append([]string{client}, mariadb.ClientArgs(a.Config.MySQLListenPort())...)
		return "", terminal.Open("Phlo MySQL", filepath.Dir(client), args, nil)
	case "psql", "mongosh":
		title, dir, args, err := a.clientTerminal(kind)
		if err != nil {
			return "", err
		}
		return "", terminal.Open(title, dir, args, nil)
	case "folder":
		dir, _, _, err := a.projectDir(account, target)
		if err != nil {
			return "", err
		}
		return "", terminal.OpenFolder(dir)
	case "log":
		return "", a.openProjectLog(account, target)
	case "cron-log":
		if runtime.GOOS != "windows" {
			return "", errs.New("Cronical runs on Windows. On Linux, use the system crontab.")
		}
		return "", a.openCronLog()
	case "site":
		if err := a.CheckRoot(); err != nil {
			return "", err
		}
		dir, title, version, err := a.projectDir(account, target)
		if err != nil {
			return "", err
		}
		command, err := a.projectShell(dir, version, account, target)
		if err != nil {
			return "", err
		}
		return "", terminal.Open(title, dir, command, nil)
	case "laravel":
		if err := a.CheckRoot(); err != nil {
			return "", err
		}
		dir, title, version, err := a.projectDir(account, target)
		if err != nil {
			return "", err
		}
		command, ini, err := a.laravelCommand(dir, version)
		if err != nil {
			return "", err
		}
		return "", terminal.Open("Laravel "+title, dir, command, laravel.Env(ini))
	default:
		return "", errs.New("unknown terminal")
	}
}

func (a *App) projectShell(dir, version, account, target string) ([]string, error) {
	cli, err := phloruntime.ResolveCLI(a.Manifest, a.Layout, version)
	if err != nil {
		return nil, err
	}
	ini, err := a.phpINI(version, cli)
	if err != nil {
		return nil, err
	}
	phpDir := filepath.Dir(cli)
	tools := a.toolchains(true)
	if runtime.GOOS != "windows" {
		return terminal.UnixProjectCommand(phpDir, ini, version, tools), nil
	}
	if err := os.MkdirAll(a.Layout.Run(), 0o755); err != nil {
		return nil, err
	}
	name := "shell-" + shellToken(account) + "-" + shellToken(target) + ".cmd"
	path := filepath.Join(a.Layout.Run(), name)
	body := terminal.WindowsProjectScript(dir, phpDir, ini, version, tools)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func shellToken(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "folder"
	}
	return b.String()
}

func (a *App) laravelCommand(dir, version string) ([]string, string, error) {
	constraint, err := laravel.Release(version)
	if err != nil {
		return nil, "", err
	}
	composer := a.Layout.ComposerFile()
	if _, err := os.Stat(composer); err != nil {
		return nil, "", errs.New("Composer is not installed")
	}
	cli, err := phloruntime.ResolveCLI(a.Manifest, a.Layout, version)
	if err != nil {
		return nil, "", err
	}
	if err := laravel.Prepare(dir); err != nil {
		return nil, "", err
	}
	ini, err := a.phpINI(version, cli)
	if err != nil {
		return nil, "", err
	}
	return laravel.Command(cli, ini, composer, constraint), ini, nil
}

func (a *App) phpINI(version, cli string) (string, error) {
	ini := a.Layout.PHPIni(version)
	if info, err := os.Stat(ini); err == nil && !info.IsDir() {
		return ini, nil
	}
	if err := os.MkdirAll(filepath.Dir(ini), 0o755); err != nil {
		return "", err
	}
	body, err := supervisor.PHPINI(a.Layout, version, filepath.Dir(cli))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(ini, []byte(body), 0o644); err != nil {
		return "", err
	}
	return ini, nil
}

func (a *App) projectDir(account, target string) (string, string, string, error) {
	if err := site.ValidateAccount(account); err != nil {
		return "", "", "", err
	}
	sites, err := a.listSites()
	if err != nil {
		return "", "", "", err
	}
	current, ok := findSite(sites, account)
	if !ok {
		return "", "", "", errs.New("account not found")
	}
	var dir, title, version string
	switch target {
	case site.PublicRoot:
		if current.PublicHTML == nil {
			return "", "", "", errs.New("public_html does not exist")
		}
		dir = current.PublicHTML.DocumentRoot
		title = account
		version = current.PublicHTML.PHP
	default:
		found := false
		for _, sub := range current.Subdomains {
			if sub.Name == target {
				dir = sub.DocumentRoot
				title = account + "/" + target
				version = sub.PHP
				found = true
				break
			}
		}
		if !found {
			return "", "", "", errs.New("folder not found")
		}
	}
	home := a.Layout.Home()
	rel, err := filepath.Rel(home, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", "", errs.New("folder is outside home")
	}
	return dir, title, version, nil
}

// openProjectLog opens var/log/sites/{host}.log in the default editor.
func (a *App) openProjectLog(account, target string) error {
	if _, _, _, err := a.projectDir(account, target); err != nil {
		return err
	}
	sites, err := a.listSites()
	if err != nil {
		return err
	}
	current, ok := findSite(sites, account)
	if !ok {
		return errs.New("account not found")
	}
	var host string
	switch target {
	case site.PublicRoot:
		if current.PublicHTML == nil {
			return errs.New("public_html does not exist")
		}
		host = current.PublicHTML.Host
	default:
		for _, sub := range current.Subdomains {
			if sub.Name == target {
				host = sub.Host
				break
			}
		}
	}
	if host == "" {
		return errs.New("folder not found")
	}
	path, err := a.Layout.SiteLog(host)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return terminal.OpenFile(path)
}

// openCronLog opens this month's Cronical job log. Cronical writes cronical-YYYY-MM.log there.
func (a *App) openCronLog() error {
	path, err := cronical.EnsureLog(a.Layout.CronicalLogDir(), time.Now())
	if err != nil {
		return err
	}
	return terminal.OpenFile(path)
}

// RunCronTest runs the example PHP command now and appends its output to the job log.
func (a *App) RunCronTest() (string, error) {
	if runtime.GOOS != "windows" {
		return "", errs.New("Cronical runs on Windows. On Linux, use the system crontab.")
	}
	a.mu.Lock()
	phpExe, err := phloruntime.ResolveCLI(a.Manifest, a.Layout, a.Config.DefaultPHP)
	logDir := a.Layout.CronicalLogDir()
	a.mu.Unlock()
	if err != nil {
		return "", errs.New("PHP is not installed. Download it in Runtimes first.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, phpExe, "-r", "echo 'hourly';")
	proc.Configure(cmd)
	out, runErr := cmd.CombinedOutput()
	code := 0
	if runErr != nil {
		code = 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			code = exitErr.ExitCode()
		}
	}
	shown := cronicalQuote(phpExe) + ` -r "echo 'hourly';"`
	if err := cronical.AppendRun(logDir, time.Now(), shown, string(out), code); err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		text = "(no output)"
	}
	if code != 0 {
		return "", errs.New("Test exited " + strconv.Itoa(code) + ". " + text)
	}
	return "Test wrote to the job log: " + text, nil
}

func cronicalQuote(exe string) string {
	if strings.ContainsAny(exe, " \t") {
		return `"` + exe + `"`
	}
	return exe
}

func (a *App) Start() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.CheckRoot(); err != nil {
		return "", err
	}
	if err := a.Supervisor.Start(); err != nil {
		return "", err
	}
	return a.syncHosts()
}

func (a *App) Stop() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.Supervisor.Stop(); err != nil {
		return "", err
	}
	return "", nil
}

func (a *App) Download(ctx context.Context, component, version string) (string, error) {
	a.mu.Lock()
	art, dest, err := a.downloadTarget(component, version)
	a.mu.Unlock()
	if err != nil {
		return "", err
	}
	if err := phloruntime.Fetch(ctx, art, dest); err != nil {
		return "", err
	}
	switch component {
	case "redis", "memcached", "mailpit", "mongodb", "postgres":
		a.mu.Lock()
		enabled := a.addonEnabled(component)
		a.mu.Unlock()
		if enabled {
			if err := a.Supervisor.StartAddon(component); err != nil {
				return "", err
			}
		}
		return "", nil
	case "heidisql":
		exe, resolveErr := phloruntime.ResolveHeidiSQL(a.Manifest, a.Layout, "", runtime.GOOS, runtime.GOARCH)
		if resolveErr == nil {
			if err := dbgui.EnsurePortable(exe); err != nil {
				return "", err
			}
		}
		return "", nil
	case "sqlyog":
		return a.OpenSQLyogInstaller()
	case "cronical":
		exe, resolveErr := phloruntime.ResolveCronical(a.Manifest, a.Layout, "", runtime.GOOS, runtime.GOARCH)
		if resolveErr == nil {
			if err := cronical.WriteRuntimeConfig(filepath.Dir(exe), a.Layout.CronicalLogDir()); err != nil {
				return "", err
			}
		}
		a.mu.Lock()
		start := a.Config.CronOn() && a.Supervisor.Running()
		a.mu.Unlock()
		if start {
			if err := a.Supervisor.StartCron(); err != nil {
				return "", err
			}
		}
		return "", nil
	case "phpmyadmin":
	default:
		if strings.HasPrefix(component, "phpext-") {
			a.mu.Lock()
			defer a.mu.Unlock()
			if err := a.refreshPHPINIs(); err != nil {
				return "", err
			}
			if a.Supervisor.Running() {
				return "", a.Supervisor.Reload()
			}
		}
		return "", nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	warn, err := a.syncHosts()
	if err != nil {
		return "", err
	}
	if a.Supervisor.Running() {
		if err := a.Supervisor.Reload(); err != nil {
			return warn, err
		}
	}
	return warn, nil
}

func (a *App) Runtime() (api.Runtime, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.runtimeView(), nil
}

func (a *App) MySQLExe() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	server, err := phloruntime.ResolveMariaDB(a.Manifest, a.Layout, a.Config.MariaDB, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	return phloruntime.ResolveMySQLClient(server)
}

func (a *App) PHPExe(version string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return phloruntime.ResolveCLI(a.Manifest, a.Layout, version)
}

func (a *App) syncHosts() (string, error) {
	sites, err := a.listSites()
	if err != nil {
		return "", err
	}
	apply := a.ApplyHosts
	if apply == nil {
		state := a.Layout.HostsState()
		apply = func(entries []hosts.Entry) error {
			return hosts.ApplySystem(state, entries)
		}
	}
	ip := a.Config.HostIPAddress()
	var entries []hosts.Entry
	for _, name := range site.Hostnames(sites) {
		entries = append(entries, hosts.Entry{IP: ip, Name: name})
	}
	if host := a.phpMyAdminHost(); host != "" {
		entries = append(entries, hosts.Entry{IP: "127.0.0.1", Name: host})
	}
	if err := apply(entries); err != nil {
		msg := fmt.Sprintf("Hosts file was not updated: %s. The generated file is in var/hosts/hosts.generated.", err)
		a.hostsWarning = msg
		return msg, nil
	}
	a.hostsWarning = ""
	return "", nil
}

func (a *App) downloadTarget(component, version string) (phloruntime.Artifact, string, error) {
	switch component {
	case "php":
		norm, err := php.Normalize(version)
		if err != nil {
			return phloruntime.Artifact{}, "", errs.New("php version must look like 8.3")
		}
		art, err := a.Manifest.LookupPHP(norm, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.PHPDir(norm, runtime.GOOS, runtime.GOARCH), nil
	case "nginx":
		art, err := a.Manifest.LookupNginx(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("nginx", art.Version), nil
	case "phpmyadmin":
		if a.Manifest.PHPMyAdmin == nil {
			return phloruntime.Artifact{}, "", errs.New("phpMyAdmin is not in the Phlo manifest")
		}
		return *a.Manifest.PHPMyAdmin, a.Layout.PHPMyAdminDir(), nil
	case "mariadb":
		art, err := a.Manifest.LookupMariaDB(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("mariadb", art.Version), nil
	case "redis":
		art, err := a.Manifest.LookupRedis(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("redis", art.Version), nil
	case "memcached":
		art, err := a.Manifest.LookupMemcached(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("memcached", art.Version), nil
	case "mailpit":
		art, err := a.Manifest.LookupMailpit(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("mailpit", art.Version), nil
	case "cronical":
		art, err := a.Manifest.LookupCronical(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("cronical", art.Version), nil
	case "heidisql":
		art, err := a.Manifest.LookupHeidiSQL(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("heidisql", art.Version), nil
	case "sqlyog":
		art, err := a.Manifest.LookupSQLyog(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("sqlyog", art.Version), nil
	case "ngrok":
		art, err := a.Manifest.LookupNgrok(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir("ngrok", art.Version), nil
	case "composer":
		if a.Manifest.Composer == nil {
			return phloruntime.Artifact{}, "", errs.New("Composer is not in the Phlo manifest")
		}
		return *a.Manifest.Composer, filepath.Dir(a.Layout.ComposerFile()), nil
	default:
		if name, ok := strings.CutPrefix(component, "phpext-"); ok {
			return a.phpExtTarget(name, version)
		}
		if _, ok := phloruntime.LookupToolSpec(component); !ok {
			return phloruntime.Artifact{}, "", errs.New("unknown component")
		}
		art, err := a.Manifest.LookupTool(component, version, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return phloruntime.Artifact{}, "", err
		}
		return art, a.Layout.VersionedToolDir(component, art.Version), nil
	}
}

// phpExtTarget picks the extension build that matches the installed PHP version and its thread-safety.
func (a *App) phpExtTarget(name, version string) (phloruntime.Artifact, string, error) {
	if _, ok := phloruntime.PHPExtensionLabels[name]; !ok {
		return phloruntime.Artifact{}, "", errs.New("unknown PHP extension")
	}
	if err := php.Accept(version); err != nil {
		return phloruntime.Artifact{}, "", errs.New(err.Error())
	}
	exe, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return phloruntime.Artifact{}, "", errs.New("install PHP " + version + " before adding an extension to it")
	}
	minor, _ := php.Normalize(version)
	ts := phloruntime.PHPThreadSafe(filepath.Dir(exe))
	art, err := a.Manifest.LookupPHPExtension(name, minor, ts, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return phloruntime.Artifact{}, "", err
	}
	return art, a.Layout.PHPExtDir(name, phloruntime.PHPExtKey(minor, ts)), nil
}

// refreshPHPINIs rewrites var/php/{version}/php.ini for every PHP build that already has one.
func (a *App) refreshPHPINIs() error {
	dirs, _ := filepath.Glob(filepath.Join(a.Layout.Var(), "php", "*"))
	for _, dir := range dirs {
		version := filepath.Base(dir)
		ini := a.Layout.PHPIni(version)
		if _, err := os.Stat(ini); err != nil {
			continue
		}
		exe, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, version, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			continue
		}
		body, err := supervisor.PHPINI(a.Layout, version, filepath.Dir(exe))
		if err != nil {
			return err
		}
		if err := os.WriteFile(ini, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// phpList is every manifest PHP version for this system plus dropped-in builds, sorted.
func (a *App) phpList() []api.PHP {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	byVersion := map[string]api.PHP{}
	for _, art := range a.Manifest.PHP {
		if art.OS != goos || art.Arch != goarch {
			continue
		}
		_, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, art.Version, goos, goarch)
		byVersion[art.Version] = api.PHP{
			Version:      art.Version,
			Patch:        art.Patch,
			Installed:    err == nil,
			Downloadable: true,
		}
	}
	base := filepath.Join(a.Layout.Runtime(), "php")
	entries, _ := os.ReadDir(base)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		if _, ok := byVersion[version]; ok {
			continue
		}
		if _, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, version, goos, goarch); err == nil {
			byVersion[version] = api.PHP{Version: version, Installed: true}
		}
	}
	versions := make([]string, 0, len(byVersion))
	for version := range byVersion {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	phpList := make([]api.PHP, 0, len(versions))
	for _, version := range versions {
		phpList = append(phpList, byVersion[version])
	}
	return phpList
}

// InstallVCRuntime runs Microsoft's Visual C++ installer, then reloads PHP when the stack is running.
func (a *App) InstallVCRuntime(ctx context.Context) (string, error) {
	if !a.vcMu.TryLock() {
		return "", errs.New("The Visual C++ runtime installer is already running.")
	}
	defer a.vcMu.Unlock()
	warn, err := vcruntime.Install(ctx, a.Layout.Cache("vcredist"))
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if check := a.vcRuntime(); check != nil && !check.OK && warn == "" {
		warn = "The installer finished, but PHP " + strings.Join(check.Blocked, ", ") + " still needs Visual C++ " + check.Need + ". Restart Windows and check again."
	}
	if a.Supervisor.Running() {
		if err := a.Supervisor.Reload(); err != nil {
			return warn, err
		}
	}
	return warn, nil
}

// vcRuntime checks the Visual C++ runtime against the installed Windows PHP builds.
func (a *App) vcRuntime() *api.VCRuntime {
	if runtime.GOOS != "windows" {
		return nil
	}
	var builds []vcruntime.Build
	for _, item := range a.phpList() {
		if !item.Installed {
			continue
		}
		if exe, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, item.Version, runtime.GOOS, runtime.GOARCH); err == nil {
			builds = append(builds, vcruntime.Build{Version: item.Version, Dir: filepath.Dir(exe)})
		}
	}
	if len(builds) == 0 {
		return nil
	}
	check := vcruntime.Check(vcruntime.SystemDir(), builds)
	return &check
}

func (a *App) runtimeView() api.Runtime {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	phpList := a.phpList()
	nginxList := phloruntime.ListNginx(a.Manifest, a.Layout, goos, goarch)
	nginxSelected := phloruntime.SelectedVersion(a.Config.Nginx, nginxList)
	mariaList := phloruntime.ListMariaDB(a.Manifest, a.Layout, goos, goarch)
	mariaSelected := phloruntime.SelectedVersion(a.Config.MariaDB, mariaList)
	redisList := phloruntime.ListRedis(a.Manifest, a.Layout, goos, goarch)
	redisSelected := phloruntime.SelectedVersion(a.Config.Redis, redisList)
	memcachedList := phloruntime.ListMemcached(a.Manifest, a.Layout, goos, goarch)
	memcachedSelected := phloruntime.SelectedVersion(a.Config.Memcached, memcachedList)
	mailpitList := phloruntime.ListMailpit(a.Manifest, a.Layout, goos, goarch)
	mailpitSelected := phloruntime.SelectedVersion(a.Config.Mailpit, mailpitList)
	cronicalList := phloruntime.ListCronical(a.Manifest, a.Layout, goos, goarch)
	heidisqlList := phloruntime.ListHeidiSQL(a.Manifest, a.Layout, goos, goarch)
	sqlyogList := phloruntime.ListSQLyog(a.Manifest, a.Layout, goos, goarch)
	ngrokList := phloruntime.ListNgrok(a.Manifest, a.Layout, goos, goarch)
	composerVersion := ""
	composerDownloadable := false
	if a.Manifest.Composer != nil {
		composerVersion = a.Manifest.Composer.Version
		composerDownloadable = true
	}
	_, composerErr := os.Stat(a.Layout.ComposerFile())
	return api.Runtime{
		DefaultPHP:      a.Config.DefaultPHP,
		PHP:             phpList,
		Nginx:           componentByVersion(nginxList, nginxSelected),
		NginxVersions:   toComponents(nginxList),
		MariaDB:         componentByVersion(mariaList, mariaSelected),
		MariaDBVersions: toComponents(mariaList),
		PHPMyAdmin: api.Component{
			Version:      phpMyAdminVersion(a.Manifest),
			Installed:    phpmyadmin.Installed(a.Layout.PHPMyAdminDir()),
			Downloadable: a.Manifest.PHPMyAdmin != nil,
		},
		Composer: api.Component{
			Version:      composerVersion,
			Installed:    composerErr == nil,
			Downloadable: composerDownloadable,
		},
		Redis:             componentByVersion(redisList, redisSelected),
		RedisVersions:     toComponents(redisList),
		Memcached:         componentByVersion(memcachedList, memcachedSelected),
		MemcachedVersions: toComponents(memcachedList),
		Mailpit:           componentByVersion(mailpitList, mailpitSelected),
		MailpitVersions:   toComponents(mailpitList),
		Cronical:          componentByVersion(cronicalList, phloruntime.SelectedVersion("", cronicalList)),
		CronicalVersions:  toComponents(cronicalList),
		HeidiSQL:          componentByVersion(heidisqlList, phloruntime.SelectedVersion("", heidisqlList)),
		HeidiSQLVersions:  toComponents(heidisqlList),
		SQLyog:            componentByVersion(sqlyogList, phloruntime.SelectedVersion("", sqlyogList)),
		SQLyogVersions:    toComponents(sqlyogList),
		Ngrok:             componentByVersion(ngrokList, phloruntime.SelectedVersion("", ngrokList)),
		NgrokVersions:     toComponents(ngrokList),
		Tools:             a.toolViews(),
		PHPExtensions:     a.phpExtViews(phpList),
	}
}

func (a *App) phpExtViews(phpList []api.PHP) []api.PHPExt {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	names := make([]string, 0, len(phloruntime.PHPExtensionLabels))
	for name := range phloruntime.PHPExtensionLabels {
		names = append(names, name)
	}
	sort.Strings(names)
	views := []api.PHPExt{}
	for _, item := range phpList {
		if !item.Installed {
			continue
		}
		exe, err := phloruntime.ResolvePHP(a.Manifest, a.Layout, item.Version, goos, goarch)
		if err != nil {
			continue
		}
		dir := filepath.Dir(exe)
		minor, err := php.Normalize(item.Version)
		if err != nil {
			continue
		}
		ts := phloruntime.PHPThreadSafe(dir)
		build := minor + " NTS"
		if ts {
			build = minor + " TS"
		}
		files := phloruntime.PHPExtensionFiles(a.Layout, item.Version, dir, goos)
		bundled := php.DiscoverExtensions(filepath.Join(dir, "ext"))
		for _, name := range names {
			view := api.PHPExt{
				Name:      name,
				Label:     phloruntime.PHPExtensionLabels[name],
				PHP:       item.Version,
				Build:     build,
				Installed: phloruntime.HasPHPExtension(files, name) || slices.Contains(bundled, name),
			}
			if art, err := a.Manifest.LookupPHPExtension(name, minor, ts, goos, goarch); err == nil {
				view.Version = art.Version
				view.Downloadable = true
			}
			views = append(views, view)
		}
		views = append(views, pgsqlView(item.Version, build, dir, bundled, goos))
	}
	return views
}

func (a *App) toolViews() []api.Tool {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	specs := phloruntime.ToolSpecs()
	views := make([]api.Tool, 0, len(specs))
	for _, spec := range specs {
		list := phloruntime.ListTool(a.Manifest, a.Layout, spec.Kind, goos, goarch)
		selected := phloruntime.SelectedVersion(a.Config.Tool(spec.Kind), list)
		view := api.Tool{
			Kind:      spec.Kind,
			Label:     spec.Label,
			Service:   spec.Service,
			Toolchain: spec.Toolchain,
			Selected:  componentByVersion(list, selected),
			Versions:  toComponents(list),
			OnPath:    spec.Toolchain && a.Config.ToolOnPath(spec.Kind),
		}
		if exe, err := phloruntime.ResolveTool(a.Manifest, a.Layout, spec.Kind, selected, goos, goarch); err == nil {
			view.Path = exe
		}
		if spec.Kind == "gcc" {
			system := ""
			if goos != "windows" && view.Path == "" {
				system, _ = exec.LookPath("g++")
			}
			view.Note = gccNote(view.Path != "", goos, system)
		}
		views = append(views, view)
	}
	return views
}

func toComponents(items []phloruntime.Install) []api.Component {
	list := make([]api.Component, 0, len(items))
	for _, item := range items {
		list = append(list, api.Component{
			Version:      item.Version,
			Installed:    item.Installed,
			Downloadable: item.Downloadable,
		})
	}
	return list
}

func componentByVersion(items []phloruntime.Install, version string) api.Component {
	for _, item := range items {
		if item.Version == version {
			return api.Component{Version: item.Version, Installed: item.Installed, Downloadable: item.Downloadable}
		}
	}
	if len(items) > 0 {
		item := items[0]
		return api.Component{Version: item.Version, Installed: item.Installed, Downloadable: item.Downloadable}
	}
	return api.Component{}
}

func (a *App) phpMyAdminHost() string {
	if !phpmyadmin.Installed(a.Layout.PHPMyAdminDir()) {
		return ""
	}
	return site.PHPMyAdminHost(a.Config.TLD)
}

func phpMyAdminVersion(manifest phloruntime.Manifest) string {
	if manifest.PHPMyAdmin == nil {
		return ""
	}
	return manifest.PHPMyAdmin.Version
}

func findSite(sites []site.Site, account string) (site.Site, bool) {
	for _, item := range sites {
		if item.Account == account {
			return item, true
		}
	}
	return site.Site{}, false
}

func hasSubdomain(item site.Site, name string) bool {
	for _, sub := range item.Subdomains {
		if sub.Name == name {
			return true
		}
	}
	return false
}
