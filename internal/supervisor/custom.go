package supervisor

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"phlo/internal/errs"
	"phlo/internal/mariadb"
	"phlo/internal/mongodb"
	"phlo/internal/paths"
	"phlo/internal/postgres"
	phloruntime "phlo/internal/runtime"
)

// CustomHeader separates Phlo's settings from the user's custom file. comment is the format's comment marker.
func CustomHeader(comment, file string) string {
	return "\n" + comment + " Custom settings from " + filepath.ToSlash(file) + "\n"
}

// Combine puts custom after body so the user's lines win.
func Combine(body, comment, file, custom string) string {
	if strings.TrimSpace(custom) == "" {
		return body
	}
	return body + CustomHeader(comment, file) + strings.TrimRight(custom, "\r\n") + "\n"
}

// ReadCustom is the custom file's text, or empty when it does not exist.
func ReadCustom(file string) (string, error) {
	body, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(body), err
}

// PHPConfig is the php.ini Phlo writes for version, without the custom settings.
func (s *Supervisor) PHPConfig(version string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exe, err := phloruntime.ResolvePHP(*s.Manifest, s.Layout, version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	return phloruntime.PHPINI(s.Layout, version, filepath.Dir(exe), runtime.GOOS), nil
}

// PHPINI is the php.ini for the PHP build in dir: Phlo's settings, then config/custom/php-{version}.ini.
func PHPINI(layout paths.Layout, version, dir string) (string, error) {
	file := layout.CustomPHPIni(version)
	custom, err := ReadCustom(file)
	if err != nil {
		return "", err
	}
	return Combine(phloruntime.PHPINI(layout, version, dir, runtime.GOOS), ";", file, custom), nil
}

// MariaDBConfig is the my.ini Phlo writes for the selected MariaDB, without the custom settings.
func (s *Supervisor) MariaDBConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exe, err := s.mariaBinary()
	if err != nil {
		return "", err
	}
	return s.mariaConfig(exe), nil
}

func (s *Supervisor) mariaConfig(exe string) string {
	return mariadb.Config(mariaBase(exe), s.Layout.DataDir(), s.Layout.MariaDBPID(), s.Layout.Log("mariadb.log"), s.Config.MySQLListenPort())
}

func mariaBase(exe string) string {
	basedir := filepath.Dir(exe)
	if strings.EqualFold(filepath.Base(basedir), "bin") {
		basedir = filepath.Dir(basedir)
	}
	return basedir
}

func (s *Supervisor) mariaINI(exe string) (string, error) {
	file := s.Layout.CustomMariaDB()
	custom, err := ReadCustom(file)
	if err != nil {
		return "", err
	}
	if err := mariadb.CheckCustom(custom); err != nil {
		return "", fmt.Errorf("MariaDB: %s %w", customName(file), err)
	}
	return Combine(s.mariaConfig(exe), "#", file, custom), nil
}

func (s *Supervisor) postgresArgs(data string) ([]string, error) {
	file := s.Layout.CustomPostgres()
	custom, err := ReadCustom(file)
	if err != nil {
		return nil, err
	}
	extra, err := postgres.CustomArgs(custom)
	if err != nil {
		return nil, fmt.Errorf("PostgreSQL: %s %w", customName(file), err)
	}
	return postgres.ServerArgs(data, s.Config.PostgresListenPort(), extra), nil
}

func (s *Supervisor) mongoArgs() ([]string, error) {
	file := s.Layout.CustomMongoDB()
	custom, err := ReadCustom(file)
	if err != nil {
		return nil, err
	}
	if err := mongodb.CheckCustom(custom); err != nil {
		return nil, fmt.Errorf("MongoDB: %s %w", customName(file), err)
	}
	if strings.TrimSpace(custom) == "" {
		file = ""
	}
	return mongodb.ServerArgs(s.Layout.MongoDBData(), s.Config.MongoDBListenPort(), file), nil
}

func customName(file string) string {
	return "config/custom/" + filepath.Base(file)
}

// customHint names the custom settings file when a database that reads one does not start.
func (s *Supervisor) customHint(name string, err error) error {
	files := map[string][2]string{
		"mariadb":  {s.Layout.CustomMariaDB(), "my.ini"},
		"postgres": {s.Layout.CustomPostgres(), "postgresql.conf"},
		"mongodb":  {s.Layout.CustomMongoDB(), "mongod.conf"},
	}
	file, ok := files[name]
	if !ok {
		return err
	}
	if custom, _ := ReadCustom(file[0]); strings.TrimSpace(custom) == "" {
		return err
	}
	return fmt.Errorf("%w\nIf a line in %s caused this, fix it in Runtimes with Edit %s next to %s.", err, customName(file[0]), file[1], label(name))
}

// LocalIPv4s lists this PC's IPv4 addresses, 127.0.0.1 first.
func LocalIPv4s() []string {
	list := []string{"127.0.0.1"}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return list
	}
	for _, addr := range addrs {
		prefix, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := prefix.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		if value := ip.String(); !slices.Contains(list, value) {
			list = append(list, value)
		}
	}
	return list
}

// OnThisPC reports whether ip is in 127.0.0.0/8 or belongs to a network adapter of this PC.
func OnThisPC(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || slices.Contains(LocalIPv4s(), parsed.String())
}

// httpPortsFree checks that Nginx can listen on 127.0.0.1 and on the host IP.
func (s *Supervisor) httpPortsFree() error {
	ports := []int{s.Config.HTTPPort}
	if s.Config.SSLOn() {
		ports = append(ports, s.Config.SSLListenPort())
	}
	for _, port := range ports {
		if err := freePort(port); err != nil {
			return err
		}
	}
	if s.Config.HostIP == "" {
		return nil
	}
	return HostIPFree(s.Config.HostIP, ports)
}

// HostIPFree checks that Nginx can listen on ip at each port.
func HostIPFree(ip string, ports []int) error {
	if !OnThisPC(ip) {
		return errs.New(fmt.Sprintf("Host IP %s is not an address of this PC right now. Connect to that network, or change Host IP in Settings, Web server (127.0.0.1 always works).", ip))
	}
	for _, port := range ports {
		if err := freeHostPort(ip, port); err != nil {
			return err
		}
	}
	return nil
}

// freeHostPort dials a LAN address instead of listening on it, so phlo.exe does not
// trigger a Windows Firewall prompt. Nginx is the program that listens there.
func freeHostPort(ip string, port int) error {
	address := net.JoinHostPort(ip, strconv.Itoa(port))
	if net.ParseIP(ip).IsLoopback() {
		ln, err := net.Listen("tcp", address)
		if err != nil {
			return errs.New(fmt.Sprintf("port %d on %s is not available: %v", port, ip, err))
		}
		return ln.Close()
	}
	conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		return nil
	}
	_ = conn.Close()
	return errs.New(fmt.Sprintf("port %d on %s is already used by another program. Stop that program or change the port in Settings, Web server.", port, ip))
}

// Port80Taken reports that some other program answers on port 80 while Phlo's HTTP port is different.
// It dials 127.0.0.1 and does not stop that program.
func Port80Taken(httpPort int) bool {
	if httpPort == 80 {
		return false
	}
	return tcpAnswers("127.0.0.1", 80)
}

func tcpAnswers(host string, port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
