//go:build windows

package folders

import (
	"fmt"
	"os/exec"
	"strings"

	"phlo/internal/proc"
)

// junction makes a directory junction, which Windows allows without administrator rights.
func junction(link, target string) error {
	cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target)
	proc.Configure(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %s: %w %s", link, err, strings.TrimSpace(string(out)))
	}
	return nil
}
