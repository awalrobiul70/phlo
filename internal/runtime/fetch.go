package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxDownload = 2 << 30

// Fetch downloads art into destDir and checks the checksum.
func Fetch(ctx context.Context, art Artifact, destDir string) (err error) {
	if !allowed(art.URL) {
		return fmt.Errorf("refusing to download %s", art.URL)
	}
	staging := destDir + ".staging"
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(staging)
		}
	}()
	partial := filepath.Join(staging, "download.part")
	if err := download(ctx, art.URL, partial, art.SHA256); err != nil {
		return err
	}
	extractDir := filepath.Join(staging, "root")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	switch art.Format {
	case "file":
		target := filepath.Join(extractDir, art.Bin)
		if err := os.Rename(partial, target); err != nil {
			return err
		}
		if err := os.Chmod(target, 0o755); err != nil {
			return err
		}
	case "zip", "tar.gz":
		if err := extract(art.Format, partial, extractDir, art.Skip); err != nil {
			return err
		}
		_ = os.Remove(partial)
		bin := filepath.Join(extractDir, filepath.FromSlash(art.Bin))
		info, statErr := os.Stat(bin)
		if statErr != nil || info.IsDir() {
			return fmt.Errorf("archive does not contain %s", art.Bin)
		}
		_ = os.Chmod(bin, 0o755)
	default:
		return fmt.Errorf("unknown format %s", art.Format)
	}
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("could not replace %s: %w", destDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return err
	}
	if err := os.Rename(extractDir, destDir); err != nil {
		return err
	}
	_ = os.RemoveAll(staging)
	return nil
}

func download(ctx context.Context, rawURL, dest, checksum string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Phlo/0.1")
	client := &http.Client{Timeout: 90 * time.Minute, CheckRedirect: checkRedirect}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned %d", rawURL, resp.StatusCode)
	}
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return err
	}
	if n > maxDownload {
		return fmt.Errorf("download is larger than %d bytes", maxDownload)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, checksum) {
		return fmt.Errorf("checksum mismatch: got %s", got)
	}
	return nil
}

func allowed(rawURL string) bool {
	if strings.HasPrefix(rawURL, "https://") {
		return true
	}
	return strings.HasPrefix(rawURL, "http://127.0.0.1") || strings.HasPrefix(rawURL, "http://localhost")
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if !allowed(req.URL.String()) {
		return fmt.Errorf("redirect left a trusted url")
	}
	return nil
}
