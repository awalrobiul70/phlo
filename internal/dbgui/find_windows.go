//go:build windows

package dbgui

import (
	"os"
	"path/filepath"
)

// FindSQLyog returns SQLyogCommunity.exe when the user has installed Community.
func FindSQLyog() string {
	rel := []string{
		filepath.Join("SQLyog Community", "SQLyogCommunity.exe"),
		filepath.Join("SQLyog", "SQLyogCommunity.exe"),
	}
	seen := map[string]struct{}{}
	for _, root := range []string{os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if root == "" {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		for _, name := range rel {
			path := filepath.Join(root, name)
			info, err := os.Stat(path)
			if err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}
