//go:build !windows

package terminal

import (
	"os/exec"

	"phlo/internal/errs"
)

func spawn(title, dir string, command []string) (*exec.Cmd, error) {
	if path := look("gnome-terminal"); path != "" {
		args := []string{"--working-directory", dir, "--title", title}
		if len(command) > 0 {
			args = append(args, "--")
			args = append(args, command...)
		}
		return exec.Command(path, args...), nil
	}
	if path := look("konsole"); path != "" {
		args := []string{"--workdir", dir, "--title", title}
		if len(command) > 0 {
			args = append(args, "-e")
			args = append(args, command...)
		}
		return exec.Command(path, args...), nil
	}
	if path := look("xfce4-terminal"); path != "" {
		args := []string{"--working-directory=" + dir, "--title=" + title}
		if len(command) > 0 {
			args = append(args, "-x")
			args = append(args, command...)
		}
		return exec.Command(path, args...), nil
	}
	if path := look("x-terminal-emulator", "xterm"); path != "" {
		args := []string{"-T", title}
		if len(command) > 0 {
			args = append(args, "-e")
			args = append(args, command...)
		}
		cmd := exec.Command(path, args...)
		cmd.Dir = dir
		return cmd, nil
	}
	return nil, errs.New("no terminal program found")
}
