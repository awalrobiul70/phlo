//go:build windows

package terminal

import "os/exec"

func openFile(path string) (*exec.Cmd, error) {
	return exec.Command("notepad.exe", path), nil
}
