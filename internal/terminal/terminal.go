package terminal

import (
	"os"
	"os/exec"

	"phlo/internal/errs"
)

// Open starts a visible terminal in dir. command is run inside it when set.
// extraEnv is appended to the process environment.
func Open(title, dir string, command []string, extraEnv []string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return errs.New("folder does not exist")
	}
	if title == "" {
		return errs.New("terminal needs a title")
	}
	cmd, err := spawn(title, dir, command)
	if err != nil {
		return err
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func look(names ...string) string {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
