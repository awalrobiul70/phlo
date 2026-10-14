package terminal

import (
	"os"

	"phlo/internal/errs"
)

// OpenFolder opens dir in the system file manager.
func OpenFolder(dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return errs.New("folder does not exist")
	}
	cmd, err := openFolder(dir)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
