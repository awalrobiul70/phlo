package version

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxUpdateZip   = 100 << 20
	maxUpdateFiles = 40
	maxUpdateBytes = 200 << 20
)

// programFiles are the only paths an update may replace.
var programFiles = map[string]struct{}{
	"bin/phlo.exe":         {},
	"bin/phlo-panel.exe":   {},
	"bin/phlo":             {},
	"config/manifest.json": {},
	"config/release.json":  {},
	"README.md":            {},
	"Phlo.cmd":             {},
	"Phlo.vbs":             {},
	"Phlo.sh":              {},
}

// ProgramDownload returns download when it is an https URL, or http on loopback.
func ProgramDownload(raw string) string {
	return acceptedURL(raw)
}

func acceptedURL(raw string) string {
	parsed, err := parseReleaseURL(raw)
	if err != nil {
		return ""
	}
	return parsed
}

func parseReleaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !allowedReleaseURL(parsed) {
		return "", errors.New("the release address must be an https URL")
	}
	return parsed.String(), nil
}

// Install downloads a program zip and copies only Phlo's program files into root.
// Sites, databases, runtimes, logs, and config/phlo.json are left as they are.
func Install(ctx context.Context, root, rawURL string) error {
	downloadURL, err := parseReleaseURL(rawURL)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "var", "update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	zipPath := filepath.Join(dir, "phlo.zip")
	if err := downloadFile(ctx, downloadURL, zipPath); err != nil {
		return err
	}
	files, err := programZip(zipPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("the update has no Phlo program files")
	}
	for _, file := range files {
		if err := placeFile(root, file.rel, file.body); err != nil {
			return err
		}
	}
	return nil
}

type stagedFile struct {
	rel  string
	body []byte
}

func programZip(zipPath string) ([]stagedFile, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	if len(reader.File) > maxUpdateFiles {
		return nil, errors.New("the update archive has too many files")
	}
	var total int64
	var out []stagedFile
	seen := map[string]struct{}{}
	for _, file := range reader.File {
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("the update archive contains a symlink")
		}
		name, err := cleanZipName(file.Name)
		if err != nil {
			return nil, err
		}
		if file.FileInfo().IsDir() || name == "" {
			continue
		}
		rel, ok := matchProgram(name)
		if !ok {
			continue
		}
		if _, dup := seen[rel]; dup {
			continue
		}
		if file.UncompressedSize64 > maxUpdateBytes {
			return nil, errors.New("the update archive is too large")
		}
		src, err := file.Open()
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(src, maxUpdateBytes-total+1))
		src.Close()
		if readErr != nil {
			return nil, readErr
		}
		total += int64(len(body))
		if total > maxUpdateBytes {
			return nil, errors.New("the update archive is too large")
		}
		seen[rel] = struct{}{}
		out = append(out, stagedFile{rel: rel, body: body})
	}
	return out, nil
}

func cleanZipName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.Contains(name, "\x00") || strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("illegal archive path %q", name)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("illegal archive path %q", name)
	}
	return cleaned, nil
}

func matchProgram(name string) (string, bool) {
	if _, ok := programFiles[name]; ok {
		return name, true
	}
	if i := strings.IndexByte(name, '/'); i > 0 {
		rest := name[i+1:]
		if _, ok := programFiles[rest]; ok {
			return rest, true
		}
	}
	return "", false
}

func placeFile(root, rel string, body []byte) error {
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if strings.HasPrefix(rel, "bin/") {
		mode = 0o755
	}
	if err := writeNew(dest, body, mode); err == nil {
		return nil
	}
	old := dest + ".old"
	_ = os.Remove(old)
	if err := os.Rename(dest, old); err != nil {
		return fmt.Errorf("replace %s: %w", rel, err)
	}
	if err := writeNew(dest, body, mode); err != nil {
		_ = os.Rename(old, dest)
		return err
	}
	return nil
}

func writeNew(dest string, body []byte, mode os.FileMode) error {
	tmp := dest + ".new"
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func downloadFile(ctx context.Context, rawURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("update download returned %s", res.Status)
	}
	if res.Request != nil && res.Request.URL != nil && !allowedReleaseURL(res.Request.URL) {
		return errors.New("the release address must be an https URL")
	}
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := io.Copy(file, io.LimitReader(res.Body, maxUpdateZip+1))
	if err != nil {
		return err
	}
	if n > maxUpdateZip {
		return errors.New("the update download is too large")
	}
	return nil
}
