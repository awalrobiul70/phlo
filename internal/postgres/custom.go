package postgres

import (
	"fmt"
	"regexp"
	"strings"
)

var settingRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*`)

// protected are the settings Phlo passes on the command line or keeps in the data folder.
var protected = map[string]string{
	"listen_addresses":        "Phlo keeps PostgreSQL on 127.0.0.1",
	"port":                    "change the port in Settings, PostgreSQL",
	"data_directory":          "change the data folder in Settings, PostgreSQL",
	"config_file":             "Phlo uses postgresql.conf in the data folder",
	"hba_file":                "Phlo uses pg_hba.conf in the data folder",
	"ident_file":              "Phlo uses pg_ident.conf in the data folder",
	"unix_socket_directories": "Phlo connects over TCP",
	"external_pid_file":       "Phlo tracks the server process itself",
}

// CustomArgs reads postgresql.conf lines (name = value, # comments, 'quoted values')
// into -c name=value arguments for postgres.
func CustomArgs(text string) ([]string, error) {
	var args []string
	for n, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		name := settingRe.FindString(line)
		if name == "" {
			return nil, fmt.Errorf("line %d: expected name = value, such as work_mem = '16MB'", n+1)
		}
		key := strings.ToLower(name)
		if strings.HasPrefix(key, "include") {
			return nil, fmt.Errorf("line %d: %s is not supported here. Put the settings in this file", n+1, name)
		}
		if why, ok := protected[key]; ok {
			return nil, fmt.Errorf("line %d: Phlo sets %s itself (%s)", n+1, key, why)
		}
		rest := strings.TrimSpace(line[len(name):])
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
		value, tail, err := settingValue(rest)
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", n+1, err)
		}
		if tail = strings.TrimSpace(tail); tail != "" && tail[0] != '#' {
			return nil, fmt.Errorf("line %d: unexpected %q after the value. Quote a value with spaces, such as '64MB'", n+1, tail)
		}
		if value == "" && !strings.HasPrefix(rest, "'") {
			return nil, fmt.Errorf("line %d: %s needs a value", n+1, name)
		}
		args = append(args, "-c", key+"="+value)
	}
	return args, nil
}

func settingValue(text string) (value, tail string, err error) {
	if !strings.HasPrefix(text, "'") {
		end := strings.IndexAny(text, " \t#")
		if end < 0 {
			return text, "", nil
		}
		return text[:end], text[end:], nil
	}
	var b strings.Builder
	for i := 1; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\'' && i+1 < len(text) && text[i+1] == '\'':
			b.WriteByte('\'')
			i++
		case c == '\'':
			return b.String(), text[i+1:], nil
		case c == '\\' && i+1 < len(text):
			b.WriteByte(text[i+1])
			i++
		default:
			b.WriteByte(c)
		}
	}
	return "", "", fmt.Errorf("the quoted value has no closing '")
}
