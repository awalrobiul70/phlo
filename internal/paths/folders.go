package paths

import (
	"path/filepath"
	"runtime"
	"strings"
)

// Folders are the folder values stored in config/phlo.json. Empty means the default folder.
type Folders struct {
	Home, MariaDB, Postgres, MongoDB string
}

// WithFolders applies the stored folder values.
func (l Layout) WithFolders(f Folders) Layout {
	l.HomeDir = l.FromConfig(f.Home)
	l.MariaDBDataDir = l.FromConfig(f.MariaDB)
	l.PostgresDataDir = l.FromConfig(f.Postgres)
	l.MongoDBDataDir = l.FromConfig(f.MongoDB)
	return l
}

// FromConfig turns a stored folder value into an absolute path, or "" for the default.
// A relative value, including one that starts with a slash on Windows, is inside the Phlo root.
func (l Layout) FromConfig(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	path := filepath.FromSlash(value)
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.Root, path)
	}
	return filepath.Clean(path)
}

// ToConfig is the value to store for an absolute folder: "" when it is the default,
// a slash path relative to the root when it is inside the root, and the absolute path otherwise.
func (l Layout) ToConfig(path, defaultPath string) string {
	if Same(path, defaultPath) {
		return ""
	}
	if Within(path, l.Root) {
		if rel, err := filepath.Rel(l.Root, path); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.Clean(path)
}

// DefaultHome is {root}/home.
func (l Layout) DefaultHome() string { return filepath.Join(l.Root, "home") }

func orDefault(path, defaultPath string) string {
	if path != "" {
		return path
	}
	return defaultPath
}

// Same reports whether a and b name the same folder. Windows paths ignore case.
func Same(a, b string) bool {
	return comparable(a) == comparable(b)
}

// Within reports whether path is parent or a folder inside parent.
func Within(path, parent string) bool {
	p, q := comparable(path), comparable(parent)
	if p == q {
		return true
	}
	if !strings.HasSuffix(q, string(filepath.Separator)) {
		q += string(filepath.Separator)
	}
	return strings.HasPrefix(p, q)
}

// Overlap reports whether one folder is the other or sits inside it.
func Overlap(a, b string) bool {
	return Within(a, b) || Within(b, a)
}

func comparable(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
