// Package cronical builds the job file and the .NET config for Cronical.
// Phlo runs Cronical.exe --console. It does not install a Windows service.
package cronical

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Command is Cronical.exe --console -c jobFile. It never installs a service.
func Command(jobFile string) []string {
	return []string{"--console", "-c", jobFile}
}

// Seed writes an example job file when path is missing. An existing file is left as it is.
// phpExe is the Phlo default PHP, so the example uses that binary.
func Seed(path, phpExe string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(Example(phpExe)), 0o644)
}

// Example is the job file created the first time. Every job line is commented out.
func Example(phpExe string) string {
	phpExe = strings.TrimSpace(phpExe)
	note := "# php is the Phlo default PHP on PATH."
	cmd := `php -r "echo 'hourly';"`
	if phpExe != "" {
		note = "# php is " + phpExe
		cmd = quoteExe(phpExe) + ` -r "echo 'hourly';"`
	}
	lines := []string{
		"# Phlo Cronical jobs. This file is kept when Phlo starts.",
		note,
		"#",
		"# Minute  Hour  Day  Month  Weekday  Command",
		"# 0-59    0-23  1-31 1-12   0-6      program and arguments",
		"# * means every value. Weekday 0 is Sunday and 1 is Monday.",
		"#",
		"# Every minute. Uncomment to try a job. It runs within one minute:",
		"# * * * * * " + cmd,
		"#",
		"# Every hour, at minute 0:",
		"# 0 * * * * " + cmd,
		"#",
		"# Every day at 09:15:",
		"# 15 9 * * * " + cmd,
		"#",
		"# Mondays at 09:00:",
		"# 0 9 * * 1 " + cmd,
		"#",
		"# Every 5 minutes:",
		"# */5 * * * * " + cmd,
		"#",
		"# When Cronical starts:",
		"# @reboot " + cmd,
		"#",
		"# Same as minute 0 of every hour:",
		"# @hourly " + cmd,
		"",
	}
	return strings.Join(lines, "\n")
}

func quoteExe(exe string) string {
	if strings.ContainsAny(exe, " \t") {
		return `"` + exe + `"`
	}
	return exe
}

// AppendRun adds one instant test to this month's job log.
func AppendRun(logDir string, when time.Time, command, output string, exitCode int) error {
	path, err := EnsureLog(logDir, when)
	if err != nil {
		return err
	}
	output = strings.TrimRight(output, "\r\n")
	if output == "" {
		output = "(no output)"
	}
	block := fmt.Sprintf("---- %s test ----\n%s\n%s\nexit %d\n\n", when.Format("2006-01-02 15:04:05"), command, output, exitCode)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(block)
	return err
}

// LogFile is Cronical's monthly job log, cronical-YYYY-MM.log inside logDir.
func LogFile(logDir string, when time.Time) string {
	return filepath.Join(logDir, when.Format("cronical-2006-01.log"))
}

// EnsureLog creates this month's job log when it is missing and returns its path.
func EnsureLog(logDir string, when time.Time) (string, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", err
	}
	path := LogFile(logDir, when)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return path, nil
}

// WriteRuntimeConfig writes Cronical.exe.config next to the binary.
// Job logs go to logDir as cronical-YYYY-MM.log.
func WriteRuntimeConfig(exeDir, logDir string) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	body := `<?xml version="1.0" standalone="yes"?>
<configuration>
  <appSettings>
    <add key="LogDebug" value="false" />
    <add key="LogPath" value="` + xmlText(logDir) + `" />
    <add key="LogRetention" value="3" />
  </appSettings>
  <startup>
    <supportedRuntime version="v4.0" sku=".NETFramework,Version=v4.7.2" />
  </startup>
</configuration>
`
	return os.WriteFile(filepath.Join(exeDir, "Cronical.exe.config"), []byte(body), 0o644)
}

func xmlText(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	return strings.ReplaceAll(value, ">", "&gt;")
}
