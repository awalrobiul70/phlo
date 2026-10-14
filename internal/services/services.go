package services

import (
	"fmt"
	"strconv"
)

// RedisArgs starts redis-server on loopback without snapshot files.
// The save value is two quote characters because the Windows build is an MSYS
// program and rejects a Windows path in a config file.
func RedisArgs(port int) []string {
	return []string{
		"--bind", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--protected-mode", "yes",
		"--daemonize", "no",
		"--save", `""`,
		"--appendonly", "no",
		"--maxmemory", "64mb",
		"--maxmemory-policy", "allkeys-lru",
		"--maxclients", "512",
	}
}

// MemcachedArgs listens on loopback and disables UDP.
func MemcachedArgs(port int) []string {
	return []string{"-l", "127.0.0.1", "-p", strconv.Itoa(port), "-U", "0"}
}

// MailpitArgs captures SMTP on loopback and serves the mailbox UI on loopback.
func MailpitArgs(smtpPort, uiPort int, database string) []string {
	return []string{
		"--smtp", fmt.Sprintf("127.0.0.1:%d", smtpPort),
		"--listen", fmt.Sprintf("127.0.0.1:%d", uiPort),
		"--database", database,
		"--disable-version-check",
	}
}
