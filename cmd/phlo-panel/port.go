package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"phlo/internal/config"
)

type launchConfig struct {
	Port   int
	TLD    string
	HostIP string
}

func readLaunchConfig(root string) launchConfig {
	cfg := launchConfig{Port: 7700, TLD: "test"}
	body, err := os.ReadFile(filepath.Join(root, "config", "phlo.json"))
	if err != nil {
		return cfg
	}
	var file struct {
		DashboardPort int    `json:"dashboardPort"`
		TLD           string `json:"tld"`
		HostIP        string `json:"hostIp"`
	}
	if json.Unmarshal(body, &file) != nil {
		return cfg
	}
	if file.DashboardPort >= 1 && file.DashboardPort <= 65535 {
		cfg.Port = file.DashboardPort
	}
	if file.TLD != "" {
		cfg.TLD = file.TLD
	}
	cfg.HostIP = file.HostIP
	return cfg
}

func localURL(cfg launchConfig, raw string) (string, error) {
	if strings.ContainsAny(raw, "\r\n") {
		return "", fmt.Errorf("local links only")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || strings.Contains(parsed.Host, `\`) {
		return "", fmt.Errorf("local links only")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("local links only")
	}
	if !allowedHost(cfg, parsed.Hostname()) {
		return "", fmt.Errorf("local links only")
	}
	return parsed.String(), nil
}

func dashboardBindFailed(detail string) bool {
	text := strings.ToLower(detail)
	return strings.Contains(text, "only one usage") || strings.Contains(text, "already in use") || strings.Contains(text, "bind:")
}

func portTaken(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func dashboardPortAllowed(root string, port int) error {
	file := filepath.Join(root, "config", "phlo.json")
	cfg, err := config.Load(file)
	if err != nil {
		return err
	}
	cfg.DashboardPort = port
	return cfg.Validate()
}

func saveDashboardPort(root string, port int) error {
	file := filepath.Join(root, "config", "phlo.json")
	cfg, err := config.Load(file)
	if err != nil {
		return err
	}
	cfg.DashboardPort = port
	return config.Save(file, cfg)
}

func suggestDashboardPort(root string, current int) int {
	cfg, err := config.Load(filepath.Join(root, "config", "phlo.json"))
	if err != nil {
		return 0
	}
	used := map[int]bool{
		cfg.HTTPPort: true, cfg.SSLListenPort(): true, cfg.DashboardPort: true,
		cfg.MySQLListenPort(): true, cfg.RedisListenPort(): true, cfg.MemcachedListenPort(): true,
		cfg.MailpitSMTPListenPort(): true, cfg.MailpitUIListenPort(): true,
		cfg.MongoDBListenPort(): true, cfg.PostgresListenPort(): true,
	}
	candidates := []int{current + 10}
	for offset := 1; offset <= 30; offset++ {
		candidates = append(candidates, current+offset)
	}
	for _, candidate := range candidates {
		if candidate < 1 || candidate > 65535 || used[candidate] || portTaken(candidate) {
			continue
		}
		return candidate
	}
	return 0
}

func allowedHost(cfg launchConfig, host string) bool {
	host = strings.ToLower(host)
	if host == "127.0.0.1" || host == "localhost" {
		return true
	}
	if cfg.HostIP != "" && host == strings.ToLower(cfg.HostIP) {
		return true
	}
	tld := strings.ToLower(cfg.TLD)
	return tld != "" && strings.HasSuffix(host, "."+tld)
}
