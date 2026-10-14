package mariadb

import (
	"fmt"
	"strings"
)

// protected are the my.ini options Phlo writes itself, with where to change them instead.
var protected = map[string]string{
	"bind-address":    "Phlo keeps MariaDB on 127.0.0.1",
	"port":            "change the port in Settings, MariaDB",
	"datadir":         "change the data folder in Settings, MariaDB",
	"basedir":         "Phlo sets it to the selected MariaDB version",
	"pid-file":        "Phlo uses that file to stop MariaDB",
	"log-error":       "Phlo reads that log to report errors",
	"socket":          "Phlo connects over TCP",
	"skip-networking": "Phlo and PHP connect over TCP",
}

// serverGroups are the my.ini sections mysqld reads.
var serverGroups = []string{"mysqld", "server", "mariadb", "mariadbd", "client-server", "galera"}

// CheckCustom refuses custom my.ini lines that would change what Phlo manages. The file starts in [mysqld].
func CheckCustom(text string) error {
	group := "mysqld"
	for n, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || line[0] == '#' || line[0] == ';':
			continue
		case line[0] == '!':
			return fmt.Errorf("line %d: %s is not supported here. Put the settings in this file", n+1, strings.Fields(line)[0])
		case line[0] == '[':
			end := strings.IndexByte(line, ']')
			if end < 0 {
				return fmt.Errorf("line %d: a section needs a closing ], such as [mysqld]", n+1)
			}
			group = strings.ToLower(strings.TrimSpace(line[1:end]))
			continue
		}
		if !serverGroup(group) {
			continue
		}
		key, _, _ := strings.Cut(line, "=")
		key = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "_", "-")
		key = strings.TrimPrefix(key, "loose-")
		if why, ok := protected[key]; ok {
			return fmt.Errorf("line %d: Phlo sets %s itself (%s)", n+1, key, why)
		}
	}
	return nil
}

func serverGroup(group string) bool {
	for _, name := range serverGroups {
		if group == name || strings.HasPrefix(group, name+"-") {
			return true
		}
	}
	return false
}
