//go:build windows

package terminal

import "os/exec"

func openFolder(dir string) (*exec.Cmd, error) {
	return exec.Command("explorer.exe", dir), nil
}
