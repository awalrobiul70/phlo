package mariadb

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// NeedsInit reports whether datadir has no MariaDB system schema yet.
func NeedsInit(datadir string) bool {
	info, err := os.Stat(filepath.Join(datadir, "mysql"))
	return err != nil || !info.IsDir()
}

// Config is a my.ini that listens on loopback and stores data in datadir.
func Config(basedir, datadir, pidFile, errorLog string, port int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[mysqld]\n")
	fmt.Fprintf(&b, "basedir=%s\n", iniPath(basedir))
	fmt.Fprintf(&b, "datadir=%s\n", iniPath(datadir))
	fmt.Fprintf(&b, "bind-address=127.0.0.1\n")
	fmt.Fprintf(&b, "port=%d\n", port)
	fmt.Fprintf(&b, "pid-file=%s\n", iniPath(pidFile))
	fmt.Fprintf(&b, "log-error=%s\n", iniPath(errorLog))
	fmt.Fprintf(&b, "character-set-server=utf8mb4\n")
	fmt.Fprintf(&b, "collation-server=utf8mb4_unicode_ci\n")
	fmt.Fprintf(&b, "skip-name-resolve\n")
	// Local use. A crash can lose about one second of commits, which is the
	// trade for not flushing every commit and not writing a binary log.
	fmt.Fprintf(&b, "innodb_flush_log_at_trx_commit=2\n")
	fmt.Fprintf(&b, "skip-log-bin\n")
	fmt.Fprintf(&b, "performance_schema=OFF\n")
	fmt.Fprintf(&b, "innodb_buffer_pool_size=256M\n")
	fmt.Fprintf(&b, "max_allowed_packet=64M\n")
	return b.String()
}

// ListenPort reads port= from the [mysqld] section. A port in another
// section, such as [client], is ignored. Zero means the file has no port.
func ListenPort(body string) int {
	section := ""
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line)
			continue
		}
		if section != "[mysqld]" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "port") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// ClientArgs connects to the loopback server as root with an empty password.
// --skip-ssl stops the client from warning that certificate checks are off
// for a passwordless login. The server is only on 127.0.0.1.
func ClientArgs(port int) []string {
	return []string{
		"--protocol=TCP",
		"-h", "127.0.0.1",
		"-P", strconv.Itoa(port),
		"-u", "root",
		"--skip-ssl",
	}
}

// ServerArgs starts mysqld in the foreground with the generated ini.
func ServerArgs(defaultsFile string) []string {
	args := []string{"--defaults-file=" + defaultsFile}
	if runtime.GOOS == "windows" {
		args = append(args, "--console")
	} else {
		args = append(args, "--skip-daemonize")
	}
	return args
}

func iniPath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	if strings.ContainsAny(path, " \t#\"") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}
