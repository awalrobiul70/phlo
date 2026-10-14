package mongodb

import (
	"fmt"
	"regexp"
	"strings"
)

var yamlKeyRe = regexp.MustCompile(`^\s*(?:-\s+)?["']?([A-Za-z_.]+)["']?\s*:`)

// protected are the mongod.conf keys Phlo passes on the command line, or that would stop Phlo from managing mongod.
var protected = map[string]string{
	"bindip":    "Phlo keeps MongoDB on 127.0.0.1",
	"bindipall": "Phlo keeps MongoDB on 127.0.0.1",
	"port":      "change the port in Settings, MongoDB",
	"dbpath":    "change the data folder in Settings, MongoDB",
	"fork":      "Phlo runs mongod in the foreground",
}

// CheckCustom refuses mongod.conf lines Phlo cannot honor. mongod parses the YAML itself.
func CheckCustom(text string) error {
	for n, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}
		if indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]; strings.Contains(indent, "\t") {
			return fmt.Errorf("line %d: indent with spaces. YAML does not allow tabs", n+1)
		}
		match := yamlKeyRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		parts := strings.Split(strings.ToLower(match[1]), ".")
		key := parts[len(parts)-1]
		if why, ok := protected[key]; ok {
			return fmt.Errorf("line %d: Phlo sets %s itself (%s)", n+1, match[1], why)
		}
	}
	return nil
}
