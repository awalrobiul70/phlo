// Package ngrok writes the loopback agent config and the one-tunnel command.
// The authtoken stays in var/ngrok. It is not a command argument and not a status field.
package ngrok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	InspectPort = 4049
	InspectAddr = "127.0.0.1:4049"
	Dashboard   = "https://dashboard.ngrok.com"
)

var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_\-]{8,200}$`)

// Command is ngrok http 127.0.0.1:port --host-header=host --config=file.
func Command(configFile, host string, port int) []string {
	return []string{
		"http",
		fmt.Sprintf("127.0.0.1:%d", port),
		"--host-header=" + host,
		"--config=" + configFile,
	}
}

// WriteToken stores the authtoken in var/ngrok/ngrok.yml. The inspect API stays on 127.0.0.1:4049.
func WriteToken(path, token string) error {
	token = strings.TrimSpace(token)
	if !tokenRe.MatchString(token) {
		return errors.New("Paste the authtoken from " + Dashboard + ". It is one value, with no spaces.")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := "version: \"3\"\nagent:\n  authtoken: " + token + "\n  web_addr: " + InspectAddr + "\n"
	return os.WriteFile(path, []byte(body), 0o600)
}

// TokenSet reports whether a token is stored. It does not return the token.
func TokenSet(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "authtoken:") {
			continue
		}
		value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "authtoken:")), `"'`)
		return tokenRe.MatchString(value)
	}
	return false
}

// PublicURL picks the https tunnel from an ngrok inspect response.
func PublicURL(body []byte) string {
	var payload struct {
		Tunnels []struct {
			PublicURL string `json:"public_url"`
			Proto     string `json:"proto"`
		} `json:"tunnels"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	fallback := ""
	for _, tunnel := range payload.Tunnels {
		if tunnel.PublicURL == "" {
			continue
		}
		if tunnel.Proto == "https" || strings.HasPrefix(tunnel.PublicURL, "https://") {
			return tunnel.PublicURL
		}
		if fallback == "" {
			fallback = tunnel.PublicURL
		}
	}
	return fallback
}

// WaitPublicURL polls the inspect API until a public URL appears or ctx ends.
func WaitPublicURL(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		url, err := fetchPublicURL(ctx, client)
		if err == nil && url != "" {
			return url, nil
		}
		select {
		case <-ctx.Done():
			return "", errors.New("timed out waiting for the ngrok URL")
		case <-ticker.C:
		}
	}
}

func fetchPublicURL(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+InspectAddr+"/api/tunnels", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return PublicURL(body), nil
}

// RedactLog drops lines that might repeat an authtoken.
func RedactLog(text string) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(line), "authtoken") {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	if len(out) > 800 {
		out = out[len(out)-800:]
	}
	return out
}
