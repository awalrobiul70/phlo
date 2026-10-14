package proc

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Acquire records this process as the running Phlo instance.
func Acquire(path string) error {
	if body, err := os.ReadFile(path); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(body)))
		if Alive(pid) {
			return fmt.Errorf("phlo is already running (pid %d)", pid)
		}
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// Release removes the pid file when it still belongs to this process.
func Release(path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(body)))
	if pid != os.Getpid() {
		return nil
	}
	return os.Remove(path)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
