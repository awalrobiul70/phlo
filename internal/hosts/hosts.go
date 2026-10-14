package hosts

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	Begin = "# BEGIN PHLO"
	End   = "# END PHLO"
)

// Entry is one line of the Phlo block.
type Entry struct {
	IP   string
	Name string
}

// Loopback points every name at 127.0.0.1.
func Loopback(names []string) []Entry {
	entries := make([]Entry, 0, len(names))
	for _, name := range names {
		entries = append(entries, Entry{IP: "127.0.0.1", Name: name})
	}
	return entries
}

// Merge replaces the Phlo block and leaves every other line in place.
func Merge(existing string, entries []Entry) (string, error) {
	clean, err := normalize(entries)
	if err != nil {
		return "", err
	}
	nl := "\n"
	if strings.Contains(existing, "\r\n") {
		nl = "\r\n"
	}
	text := strings.ReplaceAll(existing, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	var kept []string
	inBlock := false
	seen := false
	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case Begin:
			if inBlock || seen {
				return "", fmt.Errorf("hosts file has more than one %s block", Begin)
			}
			inBlock = true
			seen = true
			continue
		case End:
			if !inBlock {
				return "", fmt.Errorf("hosts file has %s without %s", End, Begin)
			}
			inBlock = false
			continue
		}
		if !inBlock {
			kept = append(kept, line)
		}
	}
	if inBlock {
		return "", fmt.Errorf("hosts file has %s without %s", Begin, End)
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) > 0 {
		kept = append(kept, "")
	}
	kept = append(kept, Begin)
	for _, entry := range clean {
		kept = append(kept, entry.IP+" "+entry.Name)
	}
	kept = append(kept, End)
	return strings.Join(kept, nl) + nl, nil
}

// normalize lowercases and sorts the names. The first entry for a name wins.
func normalize(entries []Entry) ([]Entry, error) {
	seen := map[string]struct{}{}
	var out []Entry
	for _, entry := range entries {
		name := strings.ToLower(strings.TrimSpace(entry.Name))
		if name == "" {
			continue
		}
		if strings.ContainsAny(name, " \t#\\/") || strings.Contains(name, "..") {
			return nil, fmt.Errorf("invalid hosts name %q", name)
		}
		ip := net.ParseIP(strings.TrimSpace(entry.IP))
		if ip == nil {
			return nil, fmt.Errorf("invalid hosts address %q for %s", entry.IP, name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, Entry{IP: ip.String(), Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ApplyTo merges entries into systemFile and writes a copy under stateDir.
func ApplyTo(systemFile, stateDir string, entries []Entry) error {
	current, err := os.ReadFile(systemFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	merged, err := Merge(string(current), entries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	block, err := Merge("", entries)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "hosts.block"), []byte(block), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "hosts.generated"), []byte(merged), 0o644); err != nil {
		return err
	}
	if string(current) == merged {
		return nil
	}
	if len(current) > 0 {
		if err := os.WriteFile(filepath.Join(stateDir, "hosts.backup"), current, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(systemFile, []byte(merged), 0o644)
}

// ApplySystem updates the operating system hosts file.
func ApplySystem(stateDir string, entries []Entry) error {
	system := SystemPath()
	err := ApplyTo(system, stateDir, entries)
	if err == nil || !isPermission(err) {
		return err
	}
	generated := filepath.Join(stateDir, "hosts.generated")
	if copyErr := elevateCopy(generated, system); copyErr != nil {
		return fmt.Errorf("could not update the hosts file (%s). A copy is in %s: %w", err, generated, copyErr)
	}
	got, readErr := os.ReadFile(system)
	want, _ := os.ReadFile(generated)
	if readErr != nil || string(got) != string(want) {
		return fmt.Errorf("hosts file was not updated; approve the administrator prompt and try again")
	}
	return nil
}

func isPermission(err error) bool {
	if err == nil {
		return false
	}
	if os.IsPermission(err) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "access is denied") || strings.Contains(msg, "permission denied")
}
