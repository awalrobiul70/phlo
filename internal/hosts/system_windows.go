//go:build windows

package hosts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func SystemPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}

// elevateCopy copies src over dst from an elevated PowerShell.
// The script starts with a UTF-8 BOM because Windows PowerShell 5.1 reads a BOM-less file in the
// ANSI code page, and the script path is quoted because Start-Process joins -ArgumentList with spaces.
func elevateCopy(src, dst string) error {
	script := filepath.Join(os.TempDir(), "phlo-copy-hosts.ps1")
	body := "\uFEFF$ErrorActionPreference = 'Stop'\r\nCopy-Item -LiteralPath " + psLiteral(src) + " -Destination " + psLiteral(dst) + " -Force\r\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		"Start-Process -FilePath powershell.exe -Verb RunAs -Wait -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File',"+psLiteral(`"`+script+`"`))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run Phlo as Administrator to edit the hosts file: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func psLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
