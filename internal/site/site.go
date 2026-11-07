package site

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"phlo/internal/errs"
)

const PublicRoot = "public_html"

var labelRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

var reserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true,
	"lpt1": true, "lpt2": true, "lpt3": true,
}

// Options are the global naming defaults.
type Options struct {
	TLD        string
	DefaultPHP string
}

// Root is public_html or one subdomain.
type Root struct {
	Name         string `json:"name"`
	DocumentRoot string `json:"documentRoot"`
	Host         string `json:"host"`
	PHP          string `json:"php"`
	PHPSource    string `json:"phpSource"`
	PHPPath      string `json:"phpPath,omitempty"`
	// Path is empty for this folder, or public_html / another subdomain name when shared.
	Path string `json:"path,omitempty"`
}

// Site is one home/{account} directory.
type Site struct {
	Account       string `json:"account"`
	DefaultHost   string `json:"defaultHost"`
	EffectiveHost string `json:"host"`
	CustomHost    string `json:"customHost"`
	PublicHTML    *Root  `json:"publicHtml"`
	Subdomains    []Root `json:"subdomains"`
}

func ValidateLabel(name string) error {
	if !labelRe.MatchString(name) {
		return errs.New("use lowercase letters, numbers, and hyphens")
	}
	return nil
}

func ValidateAccount(name string) error {
	if err := ValidateLabel(name); err != nil {
		return errs.New("account name must use lowercase letters, numbers, and hyphens")
	}
	if reserved[name] {
		return errs.New("account name is reserved on Windows")
	}
	if name == "phpmyadmin" {
		return errs.New("phpmyadmin is reserved")
	}
	return nil
}

// PHPMyAdminHost is the hostname Phlo keeps for phpMyAdmin.
func PHPMyAdminHost(tld string) string {
	return "phpmyadmin." + strings.ToLower(tld)
}

// ReservesPHPMyAdmin reports a site that would take the phpMyAdmin hostname.
func ReservesPHPMyAdmin(s Site, tld string) bool {
	host := PHPMyAdminHost(tld)
	if strings.EqualFold(s.EffectiveHost, host) {
		return true
	}
	for _, sub := range s.Subdomains {
		if strings.EqualFold(sub.Host, host) {
			return true
		}
	}
	return false
}

func ValidateHost(host, tld string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if len(host) > 253 || !strings.HasSuffix(host, "."+tld) {
		return errs.New("host must end with ." + tld)
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return errs.New("host must end with ." + tld)
	}
	for _, label := range labels {
		if err := ValidateLabel(label); err != nil {
			return errs.New("host must use lowercase letters, numbers, and hyphens")
		}
	}
	return nil
}

func DefaultHost(account, tld string) string {
	return strings.ToLower(account) + "." + tld
}

// Scan reads home/* accounts.
func Scan(home string, opt Options) ([]Site, error) {
	entries, err := os.ReadDir(home)
	if os.IsNotExist(err) {
		return []Site{}, nil
	}
	if err != nil {
		return nil, err
	}
	var sites []Site
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		account := entry.Name()
		if err := ValidateLabel(strings.ToLower(account)); err != nil {
			return nil, errs.New("account folder " + account + " is not a valid host label")
		}
		site, ok, err := readAccount(home, account, opt)
		if err != nil {
			return nil, err
		}
		if ok {
			sites = append(sites, site)
		}
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].Account < sites[j].Account })
	if sites == nil {
		sites = []Site{}
	}
	return sites, nil
}

func readAccount(home, account string, opt Options) (Site, bool, error) {
	dir := filepath.Join(home, account)
	meta, err := load(filepath.Join(dir, "phlo.site.json"))
	if err != nil {
		return Site{}, false, err
	}
	custom := strings.ToLower(strings.TrimSpace(meta.Host))
	if custom != "" {
		if err := ValidateHost(custom, opt.TLD); err != nil {
			return Site{}, false, fmt.Errorf("account %s: %w", account, err)
		}
	}
	eff := custom
	if eff == "" {
		eff = DefaultHost(account, opt.TLD)
	}
	site := Site{
		Account:       account,
		DefaultHost:   DefaultHost(account, opt.TLD),
		EffectiveHost: eff,
		CustomHost:    custom,
		Subdomains:    []Root{},
	}
	found := false
	pub := filepath.Join(dir, PublicRoot)
	if info, err := os.Stat(pub); err == nil && info.IsDir() {
		version, source := pick(meta.PHP.PublicHTML, opt.DefaultPHP)
		site.PublicHTML = &Root{
			Name:         PublicRoot,
			DocumentRoot: pub,
			Host:         eff,
			PHP:          version,
			PHPSource:    source,
		}
		found = true
	}
	subDir := filepath.Join(dir, "subdomain")
	subEntries, err := os.ReadDir(subDir)
	if err != nil && !os.IsNotExist(err) {
		return Site{}, false, err
	}
	for _, entry := range subEntries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		if err := ValidateLabel(strings.ToLower(name)); err != nil {
			return Site{}, false, errs.New("subdomain folder " + account + "/" + name + " is not a valid host label")
		}
		override := ""
		if meta.PHP.Subdomains != nil {
			override = meta.PHP.Subdomains[name]
		}
		version, source := pick(override, opt.DefaultPHP)
		doc, pathTarget, err := resolveDocumentRoot(dir, name, meta.Paths)
		if err != nil {
			return Site{}, false, fmt.Errorf("account %s subdomain %s: %w", account, name, err)
		}
		site.Subdomains = append(site.Subdomains, Root{
			Name:         name,
			DocumentRoot: doc,
			Host:         strings.ToLower(name) + "." + eff,
			PHP:          version,
			PHPSource:    source,
			Path:         pathTarget,
		})
		found = true
	}
	sort.Slice(site.Subdomains, func(i, j int) bool { return site.Subdomains[i].Name < site.Subdomains[j].Name })
	return site, found, nil
}

func pick(override, fallback string) (string, string) {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override), "override"
	}
	return fallback, "default"
}

// PhysicalRoot is the on-disk folder for public_html or a subdomain name.
func PhysicalRoot(accountDir, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == PublicRoot {
		return filepath.Join(accountDir, PublicRoot), nil
	}
	if err := ValidateLabel(key); err != nil {
		return "", errs.New("path must be public_html or a subdomain in this project")
	}
	return filepath.Join(accountDir, "subdomain", key), nil
}

// resolveDocumentRoot returns the folder Nginx should serve and the saved path target.
func resolveDocumentRoot(accountDir, subdomain string, paths map[string]string) (string, string, error) {
	target := strings.TrimSpace(paths[subdomain])
	if target == "" || target == subdomain {
		doc, err := PhysicalRoot(accountDir, subdomain)
		return doc, "", err
	}
	if target != PublicRoot {
		if err := ValidateLabel(target); err != nil {
			return "", "", errs.New("path must be public_html or a subdomain in this project")
		}
	}
	doc, err := PhysicalRoot(accountDir, target)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(doc)
	if err != nil || !info.IsDir() {
		return "", "", errs.New("path " + target + " does not exist")
	}
	return doc, target, nil
}

// ValidatePathTarget checks that a subdomain can use pathTarget as its document root.
func ValidatePathTarget(accountDir, subdomain, pathTarget string) error {
	pathTarget = strings.TrimSpace(pathTarget)
	if pathTarget == "" || pathTarget == subdomain {
		return nil
	}
	if pathTarget == PublicRoot {
		info, err := os.Stat(filepath.Join(accountDir, PublicRoot))
		if err != nil || !info.IsDir() {
			return errs.New("public_html does not exist")
		}
		return nil
	}
	if err := ValidateLabel(pathTarget); err != nil {
		return errs.New("path must be public_html or a subdomain in this project")
	}
	if pathTarget == subdomain {
		return nil
	}
	info, err := os.Stat(filepath.Join(accountDir, "subdomain", pathTarget))
	if err != nil || !info.IsDir() {
		return errs.New("subdomain " + pathTarget + " does not exist")
	}
	return nil
}

// WithHost returns a copy of the site using a custom host. An empty host restores the default.
func WithHost(s Site, host, tld string) (Site, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host != "" {
		if err := ValidateHost(host, tld); err != nil {
			return Site{}, err
		}
	}
	eff := host
	if eff == "" {
		eff = DefaultHost(s.Account, tld)
	}
	out := s
	out.CustomHost = host
	out.EffectiveHost = eff
	out.DefaultHost = DefaultHost(s.Account, tld)
	if s.PublicHTML != nil {
		cp := *s.PublicHTML
		cp.Host = eff
		out.PublicHTML = &cp
	}
	out.Subdomains = append([]Root(nil), s.Subdomains...)
	if out.Subdomains == nil {
		out.Subdomains = []Root{}
	}
	for i := range out.Subdomains {
		out.Subdomains[i].Host = strings.ToLower(out.Subdomains[i].Name) + "." + eff
	}
	return out, nil
}

// Duplicate reports a hostname used by two roots.
func Duplicate(sites []Site) error {
	seen := map[string]string{}
	for _, s := range sites {
		if err := claim(seen, s.EffectiveHost, s.Account); err != nil {
			return err
		}
		for _, sub := range s.Subdomains {
			if err := claim(seen, sub.Host, s.Account+"/"+sub.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// HostInUse reports when host is already claimed.
func HostInUse(sites []Site, host string) error {
	seen := map[string]string{}
	for _, s := range sites {
		_ = claim(seen, s.EffectiveHost, s.Account)
		for _, sub := range s.Subdomains {
			_ = claim(seen, sub.Host, s.Account+"/"+sub.Name)
		}
	}
	if owner, ok := seen[host]; ok {
		return errs.New("host " + host + " is already used by " + owner)
	}
	return nil
}

func claim(seen map[string]string, host, owner string) error {
	if host == "" {
		return nil
	}
	if prev, ok := seen[host]; ok {
		return errs.New("host " + host + " is used by both " + prev + " and " + owner)
	}
	seen[host] = owner
	return nil
}

// Hostnames are the names Phlo writes into the hosts file.
func Hostnames(sites []Site) []string {
	var names []string
	for _, s := range sites {
		if s.EffectiveHost != "" {
			names = append(names, s.EffectiveHost)
		}
		for _, sub := range s.Subdomains {
			names = append(names, sub.Host)
		}
	}
	return names
}
