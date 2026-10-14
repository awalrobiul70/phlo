package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	versionRe     = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)
	toolVersionRe = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){1,3}$`)
	toolKindRe    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	labelRe       = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

// Config is config/phlo.json.
type Config struct {
	DefaultPHP string `json:"defaultPhp"`
	TLD        string `json:"tld"`
	HTTPPort   int    `json:"httpPort"`
	SSLPort    int    `json:"sslPort,omitempty"`
	// HostIP is a second address Nginx serves projects on and the address the hosts block uses. Empty means 127.0.0.1.
	HostIP           string            `json:"hostIp,omitempty"`
	DashboardPort    int               `json:"dashboardPort"`
	MySQLPort        int               `json:"mysqlPort"`
	RedisPort        int               `json:"redisPort,omitempty"`
	MemcachedPort    int               `json:"memcachedPort,omitempty"`
	MailpitSMTPPort  int               `json:"mailpitSmtpPort,omitempty"`
	MailpitUIPort    int               `json:"mailpitUiPort,omitempty"`
	MongoDBPort      int               `json:"mongodbPort,omitempty"`
	PostgresPort     int               `json:"postgresPort,omitempty"`
	Nginx            string            `json:"nginx,omitempty"`
	MariaDB          string            `json:"mariadb,omitempty"`
	Redis            string            `json:"redis,omitempty"`
	Memcached        string            `json:"memcached,omitempty"`
	Mailpit          string            `json:"mailpit,omitempty"`
	SSLEnabled       *bool             `json:"sslEnabled,omitempty"`
	MariaDBEnabled   *bool             `json:"mariadbEnabled,omitempty"`
	RedisEnabled     *bool             `json:"redisEnabled,omitempty"`
	MemcachedEnabled *bool             `json:"memcachedEnabled,omitempty"`
	MailpitEnabled   *bool             `json:"mailpitEnabled,omitempty"`
	MongoDBEnabled   *bool             `json:"mongodbEnabled,omitempty"`
	PostgresEnabled  *bool             `json:"postgresEnabled,omitempty"`
	CronEnabled      *bool             `json:"cronEnabled,omitempty"`
	Tools            map[string]string `json:"tools,omitempty"`
	ToolsOff         []string          `json:"toolsOff,omitempty"`
	// HomeDir moves the projects folder, and the three data folders move one database each. Empty means
	// {root}/home, {root}/data, {root}/data-postgres, and {root}/data-mongodb. A relative path is inside the Phlo root.
	HomeDir         string `json:"homeDir,omitempty"`
	MariaDBDataDir  string `json:"mariadbDataDir,omitempty"`
	PostgresDataDir string `json:"postgresDataDir,omitempty"`
	MongoDBDataDir  string `json:"mongodbDataDir,omitempty"`
	// DatabaseDir is the older shared parent of all three data folders. Load splits it into the fields above.
	DatabaseDir string `json:"databaseDir,omitempty"`
}

func Load(path string) (Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	if cfg.MySQLPort == 0 {
		cfg.MySQLPort = 3306
	}
	cfg.splitDatabaseDir()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// splitDatabaseDir turns databaseDir into a data folder for each database that has none of its own.
func (c *Config) splitDatabaseDir() {
	base := strings.TrimSpace(c.DatabaseDir)
	c.DatabaseDir = ""
	if base == "" {
		return
	}
	for _, item := range []struct {
		dir  *string
		name string
	}{{&c.MariaDBDataDir, "data"}, {&c.PostgresDataDir, "data-postgres"}, {&c.MongoDBDataDir, "data-mongodb"}} {
		if strings.TrimSpace(*item.dir) == "" {
			*item.dir = path.Join(filepath.ToSlash(base), item.name)
		}
	}
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func (c Config) Validate() error {
	if !versionRe.MatchString(c.DefaultPHP) {
		return fmt.Errorf("defaultPhp must look like 8.3")
	}
	if c.Nginx != "" && !versionRe.MatchString(c.Nginx) {
		return fmt.Errorf("nginx must look like 1.30.5")
	}
	if c.MariaDB != "" && !versionRe.MatchString(c.MariaDB) {
		return fmt.Errorf("mariadb must look like 11.4.13")
	}
	if c.Redis != "" && !versionRe.MatchString(c.Redis) {
		return fmt.Errorf("redis must look like 8.10.2")
	}
	if c.Memcached != "" && !versionRe.MatchString(c.Memcached) {
		return fmt.Errorf("memcached must look like 1.6.8")
	}
	if c.Mailpit != "" && !versionRe.MatchString(c.Mailpit) {
		return fmt.Errorf("mailpit must look like 1.31.4")
	}
	for kind, version := range c.Tools {
		if !toolKindRe.MatchString(kind) {
			return fmt.Errorf("tools key %q must be a lowercase name", kind)
		}
		if version != "" && !toolVersionRe.MatchString(version) {
			return fmt.Errorf("tools %s must look like 24.21.0", kind)
		}
	}
	for _, kind := range c.ToolsOff {
		if !toolKindRe.MatchString(kind) {
			return fmt.Errorf("toolsOff entry %q must be a lowercase name", kind)
		}
	}
	if !labelRe.MatchString(c.TLD) {
		return fmt.Errorf("tld must be a single hostname label")
	}
	if c.HostIP != "" {
		ip := net.ParseIP(c.HostIP)
		if ip == nil || ip.To4() == nil || ip.String() != c.HostIP || ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
			return fmt.Errorf("hostIp must be an IPv4 address of this PC, such as 192.168.1.100")
		}
	}
	for name, value := range map[string]string{
		"homeDir":         c.HomeDir,
		"mariadbDataDir":  c.MariaDBDataDir,
		"postgresDataDir": c.PostgresDataDir,
		"mongodbDataDir":  c.MongoDBDataDir,
		"databaseDir":     c.DatabaseDir,
	} {
		if strings.ContainsFunc(value, unicode.IsControl) {
			return fmt.Errorf("%s must be a folder path on one line", name)
		}
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return fmt.Errorf("httpPort must be between 1 and 65535")
	}
	if c.DashboardPort < 1 || c.DashboardPort > 65535 {
		return fmt.Errorf("dashboardPort must be between 1 and 65535")
	}
	if c.HTTPPort == c.DashboardPort {
		return fmt.Errorf("httpPort and dashboardPort must be different")
	}
	ports := map[string]int{
		"httpPort":        c.HTTPPort,
		"sslPort":         c.SSLListenPort(),
		"dashboardPort":   c.DashboardPort,
		"mysqlPort":       c.MySQLListenPort(),
		"redisPort":       c.RedisListenPort(),
		"memcachedPort":   c.MemcachedListenPort(),
		"mailpitSmtpPort": c.MailpitSMTPListenPort(),
		"mailpitUiPort":   c.MailpitUIListenPort(),
		"mongodbPort":     c.MongoDBListenPort(),
		"postgresPort":    c.PostgresListenPort(),
	}
	seen := map[int]string{}
	for name, port := range ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("%s must be between 1 and 65535", name)
		}
		if other, ok := seen[port]; ok {
			return fmt.Errorf("%s and %s must use different ports", name, other)
		}
		seen[port] = name
	}
	return nil
}

// SSLListenPort is sslPort, or 443 when the file leaves it empty.
func (c Config) SSLListenPort() int {
	if c.SSLPort == 0 {
		return 443
	}
	return c.SSLPort
}

// HostIPAddress is hostIp, or 127.0.0.1 when the file leaves it empty.
func (c Config) HostIPAddress() string {
	if c.HostIP == "" {
		return "127.0.0.1"
	}
	return c.HostIP
}

// SSLOn is true only when sslEnabled is explicitly true.
func (c Config) SSLOn() bool { return c.SSLEnabled != nil && *c.SSLEnabled }

// MySQLListenPort is mysqlPort, or 3306 when the file leaves it empty.
func (c Config) MySQLListenPort() int {
	if c.MySQLPort == 0 {
		return 3306
	}
	return c.MySQLPort
}

// RedisListenPort is redisPort, or 6379 when the file leaves it empty.
func (c Config) RedisListenPort() int {
	if c.RedisPort == 0 {
		return 6379
	}
	return c.RedisPort
}

// MemcachedListenPort is memcachedPort, or 11211 when the file leaves it empty.
func (c Config) MemcachedListenPort() int {
	if c.MemcachedPort == 0 {
		return 11211
	}
	return c.MemcachedPort
}

// MailpitSMTPListenPort is the SMTP capture port, or 1025 when the file leaves it empty.
func (c Config) MailpitSMTPListenPort() int {
	if c.MailpitSMTPPort == 0 {
		return 1025
	}
	return c.MailpitSMTPPort
}

// MailpitUIListenPort is the mailbox UI port, or 8025 when the file leaves it empty.
func (c Config) MailpitUIListenPort() int {
	if c.MailpitUIPort == 0 {
		return 8025
	}
	return c.MailpitUIPort
}

// MongoDBListenPort is mongodbPort, or 27017 when the file leaves it empty.
func (c Config) MongoDBListenPort() int {
	if c.MongoDBPort == 0 {
		return 27017
	}
	return c.MongoDBPort
}

// PostgresListenPort is postgresPort, or 5432 when the file leaves it empty.
func (c Config) PostgresListenPort() int {
	if c.PostgresPort == 0 {
		return 5432
	}
	return c.PostgresPort
}

// MongoDBOn is true only when mongodbEnabled is explicitly true.
func (c Config) MongoDBOn() bool { return c.MongoDBEnabled != nil && *c.MongoDBEnabled }

// PostgresOn is true only when postgresEnabled is explicitly true.
func (c Config) PostgresOn() bool { return c.PostgresEnabled != nil && *c.PostgresEnabled }

// CronOn is true only when Settings turns Cronical on. A missing value stays off.
func (c Config) CronOn() bool { return c.CronEnabled != nil && *c.CronEnabled }

// Tool is the selected version for a tools kind, or empty for the default build.
func (c Config) Tool(kind string) string { return c.Tools[kind] }

// ToolOnPath is true unless kind is listed in toolsOff.
func (c Config) ToolOnPath(kind string) bool {
	for _, off := range c.ToolsOff {
		if off == kind {
			return false
		}
	}
	return true
}

// WithTool returns a copy with kind set to version. The map is copied so a failed save leaves the old config intact.
func (c Config) WithTool(kind, version string) Config {
	tools := make(map[string]string, len(c.Tools)+1)
	for key, value := range c.Tools {
		tools[key] = value
	}
	if version == "" {
		delete(tools, kind)
	} else {
		tools[kind] = version
	}
	if len(tools) == 0 {
		tools = nil
	}
	c.Tools = tools
	return c
}

// WithToolOnPath returns a copy with kind added to or removed from toolsOff.
func (c Config) WithToolOnPath(kind string, on bool) Config {
	off := make([]string, 0, len(c.ToolsOff)+1)
	for _, item := range c.ToolsOff {
		if item != kind {
			off = append(off, item)
		}
	}
	if !on {
		off = append(off, kind)
	}
	if len(off) == 0 {
		off = nil
	}
	c.ToolsOff = off
	return c
}

// MariaDBOn is true unless mariadbEnabled is explicitly false.
func (c Config) MariaDBOn() bool { return c.MariaDBEnabled == nil || *c.MariaDBEnabled }

// RedisOn is true unless redisEnabled is explicitly false.
func (c Config) RedisOn() bool { return c.RedisEnabled == nil || *c.RedisEnabled }

// MemcachedOn is true unless memcachedEnabled is explicitly false.
func (c Config) MemcachedOn() bool { return c.MemcachedEnabled == nil || *c.MemcachedEnabled }

// MailpitOn is true unless mailpitEnabled is explicitly false.
func (c Config) MailpitOn() bool { return c.MailpitEnabled == nil || *c.MailpitEnabled }

// BoolPtr returns a pointer to v for JSON enabled fields.
func BoolPtr(v bool) *bool { return &v }
