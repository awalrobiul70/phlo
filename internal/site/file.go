package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type file struct {
	Host string  `json:"host,omitempty"`
	PHP  phpFile `json:"php,omitempty"`
}

type phpFile struct {
	PublicHTML string            `json:"publicHtml,omitempty"`
	Subdomains map[string]string `json:"subdomains,omitempty"`
}

func load(path string) (file, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || len(bytes.TrimSpace(body)) == 0 {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return file{}, err
		}
		return file{}, nil
	}
	if err != nil {
		return file{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var meta file
	if err := dec.Decode(&meta); err != nil {
		return file{}, err
	}
	return meta, nil
}

func store(path string, meta file) error {
	if meta.Host == "" && meta.PHP.PublicHTML == "" && len(meta.PHP.Subdomains) == 0 {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return writeJSON(path, meta)
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

// UpdateHost sets or clears the custom host.
func UpdateHost(path, host string) error {
	meta, err := load(path)
	if err != nil {
		return err
	}
	meta.Host = host
	return store(path, meta)
}

// UpdatePHP sets the PHP version for public_html or a subdomain. An empty version clears the override.
func UpdatePHP(path, target, version string) error {
	meta, err := load(path)
	if err != nil {
		return err
	}
	if target == PublicRoot {
		meta.PHP.PublicHTML = version
	} else {
		if meta.PHP.Subdomains == nil {
			meta.PHP.Subdomains = map[string]string{}
		}
		if version == "" {
			delete(meta.PHP.Subdomains, target)
		} else {
			meta.PHP.Subdomains[target] = version
		}
	}
	return store(path, meta)
}
