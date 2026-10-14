//go:build !windows

package hosts

import (
	"fmt"
	"os/exec"
	"strings"
)

func SystemPath() string { return "/etc/hosts" }

func elevateCopy(src, dst string) error {
	cmd := exec.Command("sudo", "-n", "cp", "--", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not update /etc/hosts (%s): %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
