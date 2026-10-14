//go:build !windows

package proc

import "os/exec"

// StartVisible starts a program without waiting. HeidiSQL and SQLyog are Windows programs.
func StartVisible(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
