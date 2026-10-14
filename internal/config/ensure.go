package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Ensure copies committed examples into the local files the first time Phlo
// starts. An existing file is left as it is. Site files under home/ are not touched.
func Ensure(root string) error {
	configDir := filepath.Join(root, "config")
	if err := copyIfMissing(filepath.Join(configDir, "phlo.json.example"), filepath.Join(configDir, "phlo.json")); err != nil {
		return err
	}
	custom := filepath.Join(configDir, "custom")
	entries, err := os.ReadDir(custom)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".example") {
			continue
		}
		dest := strings.TrimSuffix(name, ".example")
		// cronical.dat is written later with this PC's PHP path. A yml example
		// can hold a token, so it stays a sample and is not copied.
		if name == "cronical.dat.example" || strings.HasSuffix(dest, ".yml") || strings.HasSuffix(dest, ".yaml") {
			continue
		}
		if err := copyIfMissing(filepath.Join(custom, name), filepath.Join(custom, dest)); err != nil {
			return err
		}
	}
	return nil
}

func copyIfMissing(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	body, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0o644)
}
