package terminal

import (
	"os"

	"phlo/internal/errs"
)

// OpenFile opens path with the program the operating system uses for that file.
func OpenFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return errs.New("log file does not exist")
	}
	cmd, err := openFile(path)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
