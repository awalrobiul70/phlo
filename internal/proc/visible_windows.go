//go:build windows

package proc

import "os/exec"

// StartVisible starts a GUI program and leaves its window on screen.
// It does not wait, and it does not hide the window.
func StartVisible(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
