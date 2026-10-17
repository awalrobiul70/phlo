package proc

import (
	"os"
	"path/filepath"
	"strings"
)

// IsPhloService reports whether image is a service program Phlo started
// from this folder's runtime directory. A PHP file inside a site, and a
// service from another Phlo folder, are not included.
func IsPhloService(root, image string) bool {
	root = strings.TrimSpace(root)
	if root == "" || image == "" {
		return false
	}
	if !underDir(filepath.Join(root, "runtime"), image) {
		return false
	}
	switch strings.ToLower(filepath.Base(image)) {
	case "php-cgi.exe", "php-cgi", "php-fpm", "php-fpm.exe",
		"nginx.exe", "nginx",
		"mysqld.exe", "mysqld", "mariadbd.exe", "mariadbd",
		"mongod.exe", "mongod",
		"postgres.exe", "postgres",
		"redis-server.exe", "redis-server",
		"memcached.exe", "memcached",
		"mailpit.exe", "mailpit",
		"cronical.exe",
		"ngrok.exe", "ngrok":
		return true
	default:
		return false
	}
}

func underDir(dir, path string) bool {
	dir = filepath.Clean(dir)
	path = filepath.Clean(path)
	if len(path) <= len(dir) {
		return false
	}
	if !strings.EqualFold(path[:len(dir)], dir) {
		return false
	}
	sep := path[len(dir)]
	return sep == os.PathSeparator || sep == '/'
}
