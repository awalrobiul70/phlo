// Package dbgui launches HeidiSQL and SQLyog against the local MariaDB.
// Neither program starts with the stack, and neither is installed silently.
package dbgui

import (
	"fmt"
	"os"
	"path/filepath"
)

// HeidiArgs opens a temporary session on 127.0.0.1 as root. The password stays empty.
func HeidiArgs(port int) []string {
	return []string{
		"--host=127.0.0.1",
		"--user=root",
		fmt.Sprintf("--port=%d", port),
	}
}

// SQLyogArgs passes the MariaDB host, user, empty password, and port.
// The dashboard also shows that address, for a build that ignores these flags.
func SQLyogArgs(port int) []string {
	return []string{
		"-h127.0.0.1",
		"-uroot",
		"-p",
		fmt.Sprintf("-P%d", port),
	}
}

// EnsurePortable creates portable.lock beside heidisql.exe so settings stay in that folder.
func EnsurePortable(exe string) error {
	return os.WriteFile(filepath.Join(filepath.Dir(exe), "portable.lock"), nil, 0o644)
}
