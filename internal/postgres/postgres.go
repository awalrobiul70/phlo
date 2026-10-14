package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var serverVersionRe = regexp.MustCompile(`\(PostgreSQL\)\s+([0-9]+)`)

// NeedsInit reports whether datadir has no cluster yet.
func NeedsInit(datadir string) bool {
	info, err := os.Stat(filepath.Join(datadir, "PG_VERSION"))
	return err != nil || info.IsDir()
}

// DataMajor is the major version written in datadir/PG_VERSION, such as "18", or empty.
func DataMajor(datadir string) string {
	body, err := os.ReadFile(filepath.Join(datadir, "PG_VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

// ServerMajor reads the major version from `postgres --version`, such as "postgres (PostgreSQL) 18.6".
func ServerMajor(versionOutput string) string {
	match := serverVersionRe.FindStringSubmatch(versionOutput)
	if match == nil {
		return ""
	}
	return match[1]
}

// MajorMismatch explains what to do when the data folder belongs to another major version.
func MajorMismatch(folder, dataMajor, serverMajor string) error {
	name := filepath.Base(folder)
	return fmt.Errorf("PostgreSQL %s cannot open %s because PostgreSQL %s created it. Your databases are still in that folder. "+
		"To keep using them, choose PostgreSQL %s again in Settings. To start fresh on %s, stop Phlo, rename %s to %s-%s, and start Phlo again; it creates a new empty %s",
		serverMajor, name, dataMajor, dataMajor, serverMajor, name, name, dataMajor, name)
}

// InitArgs creates a cluster owned by postgres with trust auth.
// Trust is only acceptable because the server binds 127.0.0.1.
func InitArgs(datadir string) []string {
	return []string{"-D", datadir, "-U", "postgres", "--auth=trust", "-E", "UTF8", "--locale=C"}
}

// ServerArgs runs postgres in the foreground on loopback TCP without a Unix socket.
// custom holds the -c settings from CustomArgs. Phlo's own options come after them.
func ServerArgs(datadir string, port int, custom []string) []string {
	args := append([]string{"-D", datadir}, custom...)
	return append(args,
		"-h", "127.0.0.1",
		"-p", strconv.Itoa(port),
		"-c", "unix_socket_directories=",
	)
}

// StopArgs are the pg_ctl arguments for a fast shutdown.
func StopArgs(datadir string) []string {
	return []string{"stop", "-D", datadir, "-m", "fast", "-w", "-t", "20"}
}

// ClientArgs connects psql to the loopback server as postgres.
func ClientArgs(port int) []string {
	return []string{"-h", "127.0.0.1", "-p", strconv.Itoa(port), "-U", "postgres"}
}
