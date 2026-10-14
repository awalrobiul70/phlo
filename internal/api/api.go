// Package api holds the dashboard's JSON shapes.
package api

// Status is the running Phlo process.
type Status struct {
	Running           bool       `json:"running"`
	DefaultPHP        string     `json:"defaultPhp"`
	TLD               string     `json:"tld"`
	HTTPPort          int        `json:"httpPort"`
	SSLPort           int        `json:"sslPort"`
	SSLEnabled        bool       `json:"sslEnabled"`
	HostIP            string     `json:"hostIp"`
	HostIPs           []string   `json:"hostIps"`
	DashboardPort     int        `json:"dashboardPort"`
	MySQLPort         int        `json:"mysqlPort"`
	MySQLRunning      bool       `json:"mysqlRunning"`
	MariaDBEnabled    bool       `json:"mariadbEnabled"`
	DataDir           string     `json:"dataDir"`
	Root              string     `json:"root"`
	HomeDir           string     `json:"homeDir"`
	PHPMyAdmin        string     `json:"phpMyAdmin,omitempty"`
	RedisPort         int        `json:"redisPort"`
	RedisRunning      bool       `json:"redisRunning"`
	RedisEnabled      bool       `json:"redisEnabled"`
	MemcachedPort     int        `json:"memcachedPort"`
	MemcachedRunning  bool       `json:"memcachedRunning"`
	MemcachedEnabled  bool       `json:"memcachedEnabled"`
	MailpitSMTPPort   int        `json:"mailpitSmtpPort"`
	MailpitUIPort     int        `json:"mailpitUiPort"`
	MailpitRunning    bool       `json:"mailpitRunning"`
	MailpitEnabled    bool       `json:"mailpitEnabled"`
	MailpitURL        string     `json:"mailpitUrl"`
	Redis             string     `json:"redis"`
	Memcached         string     `json:"memcached"`
	Mailpit           string     `json:"mailpit"`
	Nginx             string     `json:"nginx"`
	MariaDB           string     `json:"mariadb"`
	MongoDBPort       int        `json:"mongodbPort"`
	MongoDBRunning    bool       `json:"mongodbRunning"`
	MongoDBEnabled    bool       `json:"mongodbEnabled"`
	MongoDB           string     `json:"mongodb"`
	MongoDBData       string     `json:"mongodbData"`
	PostgresPort      int        `json:"postgresPort"`
	PostgresRunning   bool       `json:"postgresRunning"`
	PostgresEnabled   bool       `json:"postgresEnabled"`
	Postgres          string     `json:"postgres"`
	PostgresData      string     `json:"postgresData"`
	HostsWarning      string     `json:"hostsWarning,omitempty"`
	VCRuntime         *VCRuntime `json:"vcRuntime,omitempty"`
	RootProblem       string     `json:"rootProblem,omitempty"`
	Port80Taken       bool       `json:"port80Taken"`
	PHPPools          []PHPPool  `json:"phpPools,omitempty"`
	CronEnabled       bool       `json:"cronEnabled"`
	CronRunning       bool       `json:"cronRunning"`
	Cronical          string     `json:"cronical,omitempty"`
	CronicalInstalled bool       `json:"cronicalInstalled"`
	NgrokTokenSet     bool       `json:"ngrokTokenSet"`
	SQLyogInstalled   bool       `json:"sqlyogInstalled"`
	ShareHost         string     `json:"shareHost,omitempty"`
	ShareURL          string     `json:"shareUrl,omitempty"`
	// Version is the Phlo release compiled into this build.
	Version string `json:"version"`
}

// Release is what About shows for this build and any newer published version.
type Release struct {
	Version     string   `json:"version"`
	Title       string   `json:"title"`
	Notes       []string `json:"notes"`
	Latest      string   `json:"latest"`
	UpToDate    bool     `json:"upToDate"`
	UpdateURL   string   `json:"updateUrl,omitempty"`
	DownloadURL string   `json:"downloadUrl,omitempty"`
	CanUpdate   bool     `json:"canUpdate"`
	Message     string   `json:"message"`
}

// PHPPool is one PHP version Phlo is running, and the loopback port Nginx sends it.
type PHPPool struct {
	Version string `json:"version"`
	Port    int    `json:"port"`
}

// VCRuntime compares the Microsoft Visual C++ runtime with what the Windows PHP builds were linked with.
type VCRuntime struct {
	OK      bool     `json:"ok"`
	Have    string   `json:"have,omitempty"`
	Need    string   `json:"need,omitempty"`
	Blocked []string `json:"blocked,omitempty"`
}

// Tool is one kind from the manifest tools map.
type Tool struct {
	Kind      string      `json:"kind"`
	Label     string      `json:"label"`
	Service   bool        `json:"service"`
	Toolchain bool        `json:"toolchain"`
	Selected  Component   `json:"selected"`
	Versions  []Component `json:"versions"`
	OnPath    bool        `json:"onPath"`
	Path      string      `json:"path,omitempty"`
	Note      string      `json:"note,omitempty"`
}

// Docker is the result of asking the portable CLI for the engine version.
type Docker struct {
	Installed bool   `json:"installed"`
	Reachable bool   `json:"reachable"`
	Client    string `json:"client,omitempty"`
	Server    string `json:"server,omitempty"`
	Error     string `json:"error,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

// Runtime lists PHP versions and bundled tools for this operating system.
type Runtime struct {
	DefaultPHP        string      `json:"defaultPhp"`
	PHP               []PHP       `json:"php"`
	Nginx             Component   `json:"nginx"`
	NginxVersions     []Component `json:"nginxVersions"`
	MariaDB           Component   `json:"mariadb"`
	MariaDBVersions   []Component `json:"mariadbVersions"`
	PHPMyAdmin        Component   `json:"phpmyadmin"`
	Composer          Component   `json:"composer"`
	Redis             Component   `json:"redis"`
	RedisVersions     []Component `json:"redisVersions"`
	Memcached         Component   `json:"memcached"`
	MemcachedVersions []Component `json:"memcachedVersions"`
	Mailpit           Component   `json:"mailpit"`
	MailpitVersions   []Component `json:"mailpitVersions"`
	Cronical          Component   `json:"cronical"`
	CronicalVersions  []Component `json:"cronicalVersions"`
	HeidiSQL          Component   `json:"heidisql"`
	HeidiSQLVersions  []Component `json:"heidisqlVersions"`
	SQLyog            Component   `json:"sqlyog"`
	SQLyogVersions    []Component `json:"sqlyogVersions"`
	Ngrok             Component   `json:"ngrok"`
	NgrokVersions     []Component `json:"ngrokVersions"`
	Tools             []Tool      `json:"tools"`
	PHPExtensions     []PHPExt    `json:"phpExtensions"`
}

// PHPExt is one extension for one installed PHP version: downloadable, such as mongodb,
// or bundled with the PHP build, such as pgsql.
type PHPExt struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	PHP          string `json:"php"`
	Build        string `json:"build"`
	Version      string `json:"version,omitempty"`
	Installed    bool   `json:"installed"`
	Downloadable bool   `json:"downloadable"`
	Bundled      bool   `json:"bundled,omitempty"`
	Missing      string `json:"missing,omitempty"`
}

// ConfigFile is one custom settings file the Runtimes editors open.
type ConfigFile struct {
	Kind      string `json:"kind"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title"`
	File      string `json:"file"`
	Hint      string `json:"hint"`
	Example   string `json:"example"`
	Custom    string `json:"custom"`
	Generated string `json:"generated"`
}

// PHP is one selectable PHP version.
type PHP struct {
	Version      string `json:"version"`
	Patch        string `json:"patch,omitempty"`
	Installed    bool   `json:"installed"`
	Downloadable bool   `json:"downloadable"`
}

// Component is Nginx or Composer.
type Component struct {
	Version      string `json:"version"`
	Installed    bool   `json:"installed"`
	Downloadable bool   `json:"downloadable"`
}
