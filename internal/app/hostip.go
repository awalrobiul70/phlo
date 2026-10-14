package app

import (
	"fmt"
	"net"
	"strings"

	"phlo/internal/config"
	"phlo/internal/errs"
	"phlo/internal/supervisor"
)

// hostIPSetting is the hostIp to store for value. Empty and 127.0.0.1 both mean the default.
func hostIPSetting(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "127.0.0.1" {
		return "", nil
	}
	ip := net.ParseIP(value).To4()
	if ip == nil {
		return "", errs.New(fmt.Sprintf("%q is not an IPv4 address. Enter one like 192.168.1.100, or 127.0.0.1 to keep the sites on this PC only.", value))
	}
	value = ip.String()
	if !supervisor.OnThisPC(value) {
		return "", errs.New(fmt.Sprintf("%s is not an address of this PC. This PC has %s. Use one of those, or 127.0.0.1 to keep the sites on this PC only.",
			value, strings.Join(supervisor.LocalIPv4s(), ", ")))
	}
	return value, nil
}

// checkNewHostIP makes sure Nginx can listen on a changed host IP before Phlo saves it,
// so a busy port does not leave the web server stopped.
func checkNewHostIP(prev, next config.Config) error {
	if next.HostIP == "" || next.HostIP == prev.HostIP {
		return nil
	}
	ports := []int{next.HTTPPort}
	if next.SSLOn() {
		ports = append(ports, next.SSLListenPort())
	}
	return supervisor.HostIPFree(next.HostIP, ports)
}
