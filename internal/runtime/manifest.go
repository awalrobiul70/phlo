package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"phlo/internal/errs"
)

var (
	shaRe         = regexp.MustCompile(`^[0-9a-f]{64}$`)
	versionRe     = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	patchRe       = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	toolVersionRe = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){1,3}$`)
)

// Artifact is one downloadable file.
type Artifact struct {
	Version string   `json:"version"`
	Patch   string   `json:"patch,omitempty"`
	OS      string   `json:"os,omitempty"`
	Arch    string   `json:"arch,omitempty"`
	URL     string   `json:"url"`
	SHA256  string   `json:"sha256"`
	Format  string   `json:"format"`
	Bin     string   `json:"bin"`
	Skip    []string `json:"skip,omitempty"`
	// PHP and TS select the PHP build a phpExtensions entry is for, such as 8.3 non-thread-safe.
	PHP string `json:"php,omitempty"`
	TS  bool   `json:"ts,omitempty"`
}

// Manifest is config/manifest.json.
type Manifest struct {
	PHP        []Artifact            `json:"php"`
	Nginx      []Artifact            `json:"nginx"`
	MariaDB    []Artifact            `json:"mariadb"`
	Redis      []Artifact            `json:"redis"`
	Memcached  []Artifact            `json:"memcached"`
	Mailpit    []Artifact            `json:"mailpit"`
	Cronical   []Artifact            `json:"cronical,omitempty"`
	HeidiSQL   []Artifact            `json:"heidisql,omitempty"`
	SQLyog     []Artifact            `json:"sqlyog,omitempty"`
	Ngrok      []Artifact            `json:"ngrok,omitempty"`
	PHPMyAdmin *Artifact             `json:"phpmyadmin"`
	Composer   *Artifact             `json:"composer"`
	Tools      map[string][]Artifact `json:"tools,omitempty"`
	// PHPExtensions maps an extension name such as mongodb to one build per PHP version.
	PHPExtensions map[string][]Artifact `json:"phpExtensions,omitempty"`
}

func Load(path string) (Manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	seen := map[string]struct{}{}
	for _, art := range m.PHP {
		if err := art.validate("php", true); err != nil {
			return err
		}
		key := art.Version + "/" + art.OS + "/" + art.Arch
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate php artifact %s", key)
		}
		seen[key] = struct{}{}
	}
	for _, art := range m.Nginx {
		if err := art.validate("nginx", false); err != nil {
			return err
		}
	}
	for _, art := range m.MariaDB {
		if err := art.validate("mariadb", false); err != nil {
			return err
		}
	}
	for _, art := range m.Redis {
		if err := art.validate("redis", false); err != nil {
			return err
		}
	}
	for _, art := range m.Memcached {
		if err := art.validate("memcached", false); err != nil {
			return err
		}
	}
	for _, art := range m.Mailpit {
		if err := art.validate("mailpit", false); err != nil {
			return err
		}
	}
	for _, group := range []struct {
		kind  string
		items []Artifact
	}{
		{"cronical", m.Cronical},
		{"heidisql", m.HeidiSQL},
		{"sqlyog", m.SQLyog},
		{"ngrok", m.Ngrok},
	} {
		for _, art := range group.items {
			if err := art.validate(group.kind, false); err != nil {
				return err
			}
		}
	}
	if m.PHPMyAdmin != nil {
		if err := m.PHPMyAdmin.validate("phpmyadmin", false); err != nil {
			return err
		}
	}
	if m.Composer != nil {
		if err := m.Composer.validate("composer", false); err != nil {
			return err
		}
	}
	for kind, items := range m.Tools {
		if _, ok := LookupToolSpec(kind); !ok {
			return fmt.Errorf("unknown tool %s in manifest", kind)
		}
		seenTool := map[string]struct{}{}
		for _, art := range items {
			if err := art.validate(kind, false); err != nil {
				return err
			}
			if !toolVersionRe.MatchString(art.Version) {
				return fmt.Errorf("%s version %s must look like 24.21.0", kind, art.Version)
			}
			key := art.Version + "/" + art.OS + "/" + art.Arch
			if _, ok := seenTool[key]; ok {
				return fmt.Errorf("duplicate %s artifact %s", kind, key)
			}
			seenTool[key] = struct{}{}
		}
	}
	return m.validatePHPExtensions()
}

func (a Artifact) validate(kind string, php bool) error {
	if a.Version == "" || a.URL == "" || a.Bin == "" || a.Format == "" || a.SHA256 == "" {
		return fmt.Errorf("%s artifact is incomplete", kind)
	}
	if php && !versionRe.MatchString(a.Version) {
		return fmt.Errorf("php version %s must look like 8.3", a.Version)
	}
	if a.Patch != "" && (!patchRe.MatchString(a.Patch) || !strings.HasPrefix(a.Patch, a.Version+".")) {
		return fmt.Errorf("%s %s patch does not match the version", kind, a.Version)
	}
	if !shaRe.MatchString(strings.ToLower(a.SHA256)) {
		return fmt.Errorf("%s %s checksum must be 64 hex characters", kind, a.Version)
	}
	if !strings.HasPrefix(a.URL, "https://") {
		return fmt.Errorf("%s %s url must be https", kind, a.Version)
	}
	switch a.Format {
	case "zip", "tar.gz", "file":
	default:
		return fmt.Errorf("%s %s format must be zip, tar.gz, or file", kind, a.Version)
	}
	if err := validBin(a.Bin); err != nil {
		return fmt.Errorf("%s bin %s", kind, err)
	}
	for _, pattern := range a.Skip {
		if pattern == "" || strings.ContainsAny(pattern, `/\`) {
			return fmt.Errorf("%s %s skip entries must be a file or folder name", kind, a.Version)
		}
	}
	if kind != "composer" && kind != "phpmyadmin" && (a.OS == "" || a.Arch == "") {
		return fmt.Errorf("%s %s needs os and arch", kind, a.Version)
	}
	return nil
}

func (m Manifest) LookupPHP(version, goos, goarch string) (Artifact, error) {
	for _, art := range m.PHP {
		if art.Version == version && art.OS == goos && art.Arch == goarch {
			return art, nil
		}
	}
	return Artifact{}, errs.New(fmt.Sprintf("PHP %s is not in the Phlo manifest for %s/%s", version, goos, goarch))
}

func validBin(bin string) error {
	slash := filepathToSlash(bin)
	if bin == "" || bin == "." || strings.Contains(slash, "..") || strings.HasPrefix(slash, "/") || strings.Contains(slash, ":") {
		return fmt.Errorf("must be a relative file path")
	}
	return nil
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

func (m Manifest) LookupMariaDB(goos, goarch string) (Artifact, error) {
	for _, art := range m.MariaDB {
		if art.OS == goos && art.Arch == goarch {
			return art, nil
		}
	}
	return Artifact{}, errs.New(fmt.Sprintf("MariaDB is not in the Phlo manifest for %s/%s", goos, goarch))
}

func (m Manifest) LookupRedis(goos, goarch string) (Artifact, error) {
	return lookupOS(m.Redis, "Redis", goos, goarch)
}

func (m Manifest) LookupMemcached(goos, goarch string) (Artifact, error) {
	return lookupOS(m.Memcached, "Memcached", goos, goarch)
}

func (m Manifest) LookupMailpit(goos, goarch string) (Artifact, error) {
	return lookupOS(m.Mailpit, "Mailpit", goos, goarch)
}

func (m Manifest) LookupCronical(goos, goarch string) (Artifact, error) {
	return lookupOS(m.Cronical, "Cronical", goos, goarch)
}

func (m Manifest) LookupHeidiSQL(goos, goarch string) (Artifact, error) {
	return lookupOS(m.HeidiSQL, "HeidiSQL", goos, goarch)
}

func (m Manifest) LookupSQLyog(goos, goarch string) (Artifact, error) {
	return lookupOS(m.SQLyog, "SQLyog", goos, goarch)
}

func (m Manifest) LookupNgrok(goos, goarch string) (Artifact, error) {
	return lookupOS(m.Ngrok, "ngrok", goos, goarch)
}

func lookupOS(items []Artifact, label, goos, goarch string) (Artifact, error) {
	for _, art := range items {
		if art.OS == goos && art.Arch == goarch {
			return art, nil
		}
	}
	return Artifact{}, errs.New(fmt.Sprintf("%s is not in the Phlo manifest for %s/%s", label, goos, goarch))
}

// LookupTool finds a tools entry for this platform. An empty version is the first entry listed.
func (m Manifest) LookupTool(kind, version, goos, goarch string) (Artifact, error) {
	for _, art := range m.Tools[kind] {
		if art.OS == goos && art.Arch == goarch && (version == "" || art.Version == version) {
			return art, nil
		}
	}
	name := ToolLabel(kind)
	if version != "" {
		name += " " + version
	}
	return Artifact{}, errs.New(fmt.Sprintf("%s is not in the Phlo manifest for %s/%s", name, goos, goarch))
}

func (m Manifest) LookupNginx(goos, goarch string) (Artifact, error) {
	for _, art := range m.Nginx {
		if art.OS == goos && art.Arch == goarch {
			return art, nil
		}
	}
	return Artifact{}, errs.New(fmt.Sprintf("Nginx is not in the Phlo manifest for %s/%s", goos, goarch))
}
