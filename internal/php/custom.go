package php

import (
	"fmt"
	"strings"
)

// CheckCustom refuses PHP lines that would move a project's error log.
// Phlo sends each project's PHP errors to var/log/sites/{host}.log.
func CheckCustom(text string) error {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
			continue
		}
		key := trimmed
		if n := strings.IndexAny(trimmed, "= \t"); n >= 0 {
			key = trimmed[:n]
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "error_log", "log_errors":
			return fmt.Errorf("line %d: Phlo writes each project's PHP errors to var/log/sites. Open that file with Log on the project", i+1)
		}
	}
	return nil
}
