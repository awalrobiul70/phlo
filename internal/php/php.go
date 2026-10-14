package php

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var versionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)

var wantedExtensions = []string{
	"curl", "mbstring", "openssl", "fileinfo", "gd", "intl",
	"pdo_mysql", "mysqli", "pdo_pgsql", "pgsql", "pdo_sqlite", "sqlite3", "sodium",
	"zip", "exif", "gettext", "bz2", "xsl", "soap", "ftp", "sockets", "mongodb",
}

// Accept reports whether version is major.minor or major.minor.patch.
func Accept(version string) error {
	if !versionRe.MatchString(version) {
		return fmt.Errorf("php version must look like 8.3")
	}
	return nil
}

// Normalize returns major.minor.
func Normalize(version string) (string, error) {
	major, minor, err := parts(version)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d", major, minor), nil
}

// Port maps 8.3 to 9083 and 8.2 to 9082.
func Port(version string) (int, error) {
	major, minor, err := parts(version)
	if err != nil {
		return 0, err
	}
	if major > 9 || minor > 9 {
		return 0, fmt.Errorf("php version %s is outside the supported port map", version)
	}
	return 9000 + major*10 + minor, nil
}

func parts(version string) (int, int, error) {
	if !versionRe.MatchString(version) {
		return 0, 0, fmt.Errorf("php version must look like 8.3")
	}
	bits := strings.Split(version, ".")
	major, _ := strconv.Atoi(bits[0])
	minor, _ := strconv.Atoi(bits[1])
	return major, minor, nil
}

// Mode is "fpm" or "cgi" based on the executable name.
func Mode(exe string) string {
	if strings.Contains(strings.ToLower(filepath.Base(exe)), "php-fpm") {
		return "fpm"
	}
	return "cgi"
}

func CGIArgs(bind, ini string) []string {
	return []string{"-b", bind, "-c", ini}
}

func FPMArgs(conf, ini string) []string {
	return []string{"-F", "-y", conf, "-c", ini}
}

// INI is a generated development php.ini. Phlo rewrites this file on start.
// An extension given as an absolute path is loaded from that file.
func INI(extensionDir string, extensions []string) string {
	var b strings.Builder
	b.WriteString("date.timezone=UTC\n")
	b.WriteString("display_errors=On\n")
	b.WriteString("error_reporting=E_ALL\n")
	b.WriteString("memory_limit=512M\n")
	b.WriteString("upload_max_filesize=64M\n")
	b.WriteString("post_max_size=64M\n")
	b.WriteString("max_execution_time=120\n")
	b.WriteString("expose_php=On\n")
	if extensionDir != "" {
		b.WriteString("extension_dir=" + strconv.Quote(filepath.ToSlash(extensionDir)) + "\n")
	}
	for _, name := range extensions {
		if filepath.IsAbs(name) {
			b.WriteString("extension=" + strconv.Quote(filepath.ToSlash(name)) + "\n")
			continue
		}
		b.WriteString("extension=" + name + "\n")
	}
	return b.String()
}

// DiscoverExtensions returns bundled extensions that exist in extDir.
func DiscoverExtensions(extDir string) []string {
	var found []string
	for _, name := range wantedExtensions {
		candidates := []string{
			filepath.Join(extDir, "php_"+name+".dll"),
			filepath.Join(extDir, name+".dll"),
			filepath.Join(extDir, name+".so"),
		}
		for _, candidate := range candidates {
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				found = append(found, name)
				break
			}
		}
	}
	return found
}

// FPMConfig is a foreground pool bound to listen.
func FPMConfig(listen, pidPath, errorLog string) (string, error) {
	for _, value := range []string{listen, pidPath, errorLog} {
		if strings.ContainsAny(value, "\r\n") {
			return "", fmt.Errorf("invalid php-fpm path")
		}
	}
	return fmt.Sprintf(`[global]
daemonize = no
pid = %s
error_log = %s

[phlo]
listen = %s
listen.allowed_clients = 127.0.0.1
pm = dynamic
pm.max_children = 5
pm.start_servers = 2
pm.min_spare_servers = 1
pm.max_spare_servers = 3
clear_env = no
`, pidPath, errorLog, listen), nil
}

// CLIExe is php.exe or php beside the FastCGI binary.
func CLIExe(dir, goos string) string {
	name := "php"
	if goos == "windows" {
		name = "php.exe"
	}
	return filepath.Join(dir, name)
}
