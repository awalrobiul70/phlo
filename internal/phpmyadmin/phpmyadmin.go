package phpmyadmin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Installed reports whether index.php is in dir.
func Installed(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.php"))
	return err == nil && !info.IsDir()
}

// Secret reads a saved blowfish secret or creates one.
func Secret(path string) (string, error) {
	if body, err := os.ReadFile(path); err == nil {
		text := strings.TrimSpace(string(body))
		if len(text) >= 32 && !strings.ContainsAny(text, "'\\\n\r") {
			return text, nil
		}
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	text := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(text+"\n"), 0o600); err != nil {
		return "", err
	}
	return text, nil
}

// Config is a config.inc.php that signs in as root with an empty password.
func Config(secret, temp string, port int) string {
	temp = filepath.ToSlash(temp)
	var b strings.Builder
	b.WriteString("<?php\n")
	fmt.Fprintf(&b, "$cfg['blowfish_secret'] = '%s';\n", secret)
	b.WriteString("$i = 1;\n")
	b.WriteString("$cfg['Servers'][$i]['auth_type'] = 'config';\n")
	b.WriteString("$cfg['Servers'][$i]['host'] = '127.0.0.1';\n")
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['port'] = '%d';\n", port)
	b.WriteString("$cfg['Servers'][$i]['user'] = 'root';\n")
	b.WriteString("$cfg['Servers'][$i]['password'] = '';\n")
	b.WriteString("$cfg['Servers'][$i]['AllowNoPassword'] = true;\n")
	fmt.Fprintf(&b, "$cfg['TempDir'] = '%s';\n", temp)
	return b.String()
}

// WriteConfig stores config.inc.php beside the phpMyAdmin files.
func WriteConfig(dir, secret, temp string, port int) error {
	if err := os.MkdirAll(temp, 0o755); err != nil {
		return err
	}
	body := Config(secret, temp, port)
	return os.WriteFile(filepath.Join(dir, "config.inc.php"), []byte(body), 0o644)
}
