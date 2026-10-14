//go:build windows

package vcruntime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"phlo/internal/errs"
	"phlo/internal/proc"
)

const maxInstallerBytes = 200 << 20

// Install downloads Microsoft's redistributable into cacheDir, checks that Microsoft signed it,
// and runs it elevated. Phlo does not ship the DLLs because Microsoft only allows the official
// installer to be passed on. The warning is set when Windows wants a restart.
func Install(ctx context.Context, cacheDir string) (string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(cacheDir, "vc_redist.x64.exe")
	if err := download(ctx, path); err != nil {
		return "", fmt.Errorf("download the Visual C++ runtime installer: %w", err)
	}
	if err := verifySignature(ctx, path); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	code, err := runElevated(path)
	if err != nil {
		return "", err
	}
	switch code {
	case 0, 1638:
		return "", nil
	case 3010:
		return "Restart Windows to finish installing the Visual C++ runtime.", nil
	case 1223, 1602:
		return "", errs.New("The Visual C++ runtime install was canceled.")
	default:
		return "", fmt.Errorf("the Visual C++ installer exited with code %d; its log is dd_vcredist_*.log in %%TEMP%%", code)
	}
}

func download(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, InstallerURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", InstallerURL, resp.Status)
	}
	if resp.Request.URL.Scheme != "https" {
		return fmt.Errorf("%s redirected to %s", InstallerURL, resp.Request.URL)
	}
	partial := path + ".partial"
	file, err := os.Create(partial)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(file, io.LimitReader(resp.Body, maxInstallerBytes+1))
	closeErr := file.Close()
	if copyErr == nil && n > maxInstallerBytes {
		copyErr = fmt.Errorf("installer is larger than %d MB", maxInstallerBytes>>20)
	}
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(partial)
		return copyErr
	}
	return os.Rename(partial, path)
}

// verifySignature requires a valid Authenticode signature from Microsoft Corporation.
func verifySignature(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	script := "$s = Get-AuthenticodeSignature -LiteralPath " + psLiteral(path) + "; " +
		"Write-Output $s.Status; Write-Output $s.SignerCertificate.Subject"
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	proc.Configure(cmd)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("check the Visual C++ installer signature: %w", err)
	}
	status, subject, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if strings.TrimSpace(status) != "Valid" || !strings.Contains(subject, "O=Microsoft Corporation") {
		return fmt.Errorf("the downloaded Visual C++ installer is not signed by Microsoft (%s %s)",
			strings.TrimSpace(status), strings.TrimSpace(subject))
	}
	return nil
}

// runElevated asks for administrator approval and waits for the installer.
// It is not tied to the request context so closing the dashboard does not leave a half-run install.
func runElevated(path string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	script := "try { $p = Start-Process -FilePath " + psLiteral(path) +
		" -ArgumentList '/install','/passive','/norestart' -Verb RunAs -Wait -PassThru -ErrorAction Stop } catch { exit 1223 }; exit $p.ExitCode"
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	proc.Configure(cmd)
	err := cmd.Run()
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("run the Visual C++ installer: %w", err)
	}
	return 0, nil
}

func psLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
