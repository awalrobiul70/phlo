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
	// MariaDB 11 and 12 on Windows default to innodb_flush_method=unbuffered,
	// innodb_log_file_buffering=OFF, and innodb_flush_log_at_trx_commit=1.
	// Each commit then waits for a 512-byte disk write. Buffering the redo log
	// and flushing it about once a second is the local-use tradeoff: a power
	// loss can drop about one second of commits.
	fmt.Fprintf(&b, "innodb_flush_log_at_trx_commit=2\n")
	fmt.Fprintf(&b, "innodb_log_file_buffering=ON\n")
	fmt.Fprintf(&b, "innodb_flush_neighbors=0\n")
	if runtime.GOOS == "windows" {
		fmt.Fprintf(&b, "innodb_flush_method=normal\n")
	}
	fmt.Fprintf(&b, "skip-log-bin\n")
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
