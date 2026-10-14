//go:build !windows

package terminal

import (
	"os/exec"

	"phlo/internal/errs"
)

func openFolder(dir string) (*exec.Cmd, error) {
	if path := look("xdg-open"); path != "" {
		return exec.Command(path, dir), nil
	}
	return nil, errs.New("no file manager found")
}
