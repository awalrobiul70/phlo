package app

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"phlo/internal/dbgui"
	"phlo/internal/errs"
	"phlo/internal/ngrok"
	"phlo/internal/proc"
	phloruntime "phlo/internal/runtime"
	"phlo/internal/site"
)

// OpenHeidiSQL opens the portable HeidiSQL against MariaDB on 127.0.0.1.
func (a *App) OpenHeidiSQL() (string, error) {
	if runtime.GOOS != "windows" {
		return "", errs.New("HeidiSQL is a Windows program.")
	}
	a.mu.Lock()
	port := a.Config.MySQLListenPort()
	exe, err := phloruntime.ResolveHeidiSQL(a.Manifest, a.Layout, "", runtime.GOOS, runtime.GOARCH)
	a.mu.Unlock()
	if err != nil {
		return "", errs.New("HeidiSQL is not installed. Download it under Runtimes.")
	}
	if err := dbgui.EnsurePortable(exe); err != nil {
		return "", err
	}
	if err := proc.StartVisible(exe, dbgui.HeidiArgs(port), filepath.Dir(exe)); err != nil {
		return "", err
	}
	return "", nil
}

// OpenSQLyog opens the installed SQLyog Community against MariaDB.
func (a *App) OpenSQLyog() (string, error) {
	if runtime.GOOS != "windows" {
		return "", errs.New("SQLyog Community is a Windows program.")
	}
	a.mu.Lock()
	port := a.Config.MySQLListenPort()
	a.mu.Unlock()
	exe := dbgui.FindSQLyog()
	if exe == "" {
		return "", errs.New("SQLyog Community is not installed. Download the installer and accept the license. It connects to 127.0.0.1:" + strconv.Itoa(port) + " as root with an empty password.")
	}
	if err := proc.StartVisible(exe, dbgui.SQLyogArgs(port), filepath.Dir(exe)); err != nil {
		return "", err
	}
	return "", nil
}

// OpenSQLyogInstaller opens the downloaded installer. It does not pass a silent-install switch.
func (a *App) OpenSQLyogInstaller() (string, error) {
	if runtime.GOOS != "windows" {
		return "", errs.New("SQLyog Community is a Windows program.")
	}
	a.mu.Lock()
	exe, err := phloruntime.ResolveSQLyog(a.Manifest, a.Layout, "", runtime.GOOS, runtime.GOARCH)
	a.mu.Unlock()
	if err != nil {
		return "", errs.New("Download the SQLyog installer first.")
	}
	if err := proc.StartVisible(exe, nil, filepath.Dir(exe)); err != nil {
		return "", err
	}
	return "The SQLyog installer is open. Accept the license to install it. Phlo does not install it for you.", nil
}

// SetNgrokToken writes the authtoken under var/ngrok. It is not stored in config/phlo.json.
func (a *App) SetNgrokToken(token string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ngrok.WriteToken(a.Layout.NgrokConfig(), token); err != nil {
		return "", errs.New(err.Error())
	}
	return "Authtoken saved.", nil
}

// Share opens one ngrok tunnel for a project host. Nginx has to be running.
func (a *App) Share(host string) (string, error) {
	a.mu.Lock()
	if err := a.knownProjectHost(host); err != nil {
		a.mu.Unlock()
		return "", err
	}
	port := a.Config.HTTPPort
	host = strings.ToLower(strings.TrimSpace(host))
	a.mu.Unlock()
	url, err := a.Supervisor.Share(host, port)
	if err != nil {
		return "", err
	}
	return "Sharing " + host + " at " + url, nil
}

// StopShare stops the ngrok process Phlo started.
func (a *App) StopShare() (string, error) {
	if err := a.Supervisor.StopShare(); err != nil {
		return "", err
	}
	return "Share stopped.", nil
}

func (a *App) knownProjectHost(host string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || strings.ContainsAny(host, " \t\r\n") || strings.HasPrefix(host, "-") {
		return errs.New("Choose a project to share.")
	}
	sites, err := a.listSites()
	if err != nil {
		return err
	}
	for _, name := range site.Hostnames(sites) {
		if strings.EqualFold(name, host) {
			return nil
		}
	}
	return errs.New(host + " is not a project in Phlo.")
}
