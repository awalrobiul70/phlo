// Package version is the Phlo release embedded in this build.
package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	phloruntime "phlo/internal/runtime"
)

// Current is this build. 1.0.0 is the first public release.
const Current = "1.0.0"

// File is the release note shipped with the repository.
type File struct {
	Version   string   `json:"version"`
	Title     string   `json:"title"`
	Notes     []string `json:"notes"`
	UpdateURL string   `json:"updateUrl"`
}

// Remote is the JSON at updateUrl once a newer release is published.
// Download is an https zip of program files only.
type Remote struct {
	Version  string `json:"version"`
	URL      string `json:"url"`
	Notes    string `json:"notes"`
	Download string `json:"download"`
}

// ReadFile reads config/release.json. A missing file is an empty release.
func ReadFile(path string) (File, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{Version: Current, Title: "First release"}, nil
		}
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(body, &file); err != nil {
		return File{}, fmt.Errorf("read %s: %w", path, err)
	}
	if strings.TrimSpace(file.Version) == "" {
		file.Version = Current
	}
	return file, nil
}

// Newer reports whether latest sorts after running.
func Newer(running, latest string) bool {
	return phloruntime.CompareVersions(running, strings.TrimSpace(latest)) < 0
}

// Fetch reads a release JSON document. Only https is accepted, plus http on
// 127.0.0.1 so a local check can be tested.
func Fetch(ctx context.Context, rawURL string) (Remote, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !allowedReleaseURL(parsed) {
		return Remote{}, errors.New("the release address must be an https URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Remote{}, err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return Remote{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Remote{}, fmt.Errorf("release check returned %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Remote{}, err
	}
	var remote Remote
	if err := json.Unmarshal(body, &remote); err != nil {
		return Remote{}, err
	}
	if strings.TrimSpace(remote.Version) == "" {
		return Remote{}, errors.New("the release file has no version")
	}
	return remote, nil
}

func allowedReleaseURL(u *url.URL) bool {
	if u == nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	host := u.Hostname()
	return u.Scheme == "http" && (host == "127.0.0.1" || host == "localhost")
}
