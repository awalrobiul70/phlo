//go:build !windows

package terminal

import (
	"os/exec"

	"phlo/internal/errs"
)

func openFile(file string) (*exec.Cmd, error) {
	bin := look("xdg-open")
	if bin == "" {
		return nil, errs.New("no program is set to open log files")
	}
	return exec.Command(bin, file), nil
}
