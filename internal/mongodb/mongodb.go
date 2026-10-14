package mongodb

import (
	"fmt"
	"strconv"
)

// ServerArgs runs mongod on loopback with its data in dbpath. A non-empty configFile adds the
// user's mongod.conf; the command-line options still win over the same keys in that file.
func ServerArgs(dbpath string, port int, configFile string) []string {
	var args []string
	if configFile != "" {
		args = append(args, "--config", configFile)
	}
	return append(args,
		"--bind_ip", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--dbpath", dbpath,
	)
}

// URI is the connection string for the loopback server.
func URI(port int) string {
	return fmt.Sprintf("mongodb://127.0.0.1:%d", port)
}

// ShutdownArgs asks mongosh to stop the server cleanly.
// The server drops the connection, so mongosh exits with an error even when it worked.
func ShutdownArgs(port int) []string {
	return []string{"--quiet", URI(port) + "/admin", "--eval", "db.shutdownServer()"}
}
