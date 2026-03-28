package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ModbusURL returns the Modbus URL from either URL or IP:Port. If url is non-empty it is trimmed and returned; otherwise tcp://ip:port is built (port defaults to 502 if 0). Returns empty string when both url and ip are empty.
func ModbusURL(url, ip string, port uint16) string {
	if strings.TrimSpace(url) != "" {
		return strings.TrimSpace(url)
	}
	if strings.TrimSpace(ip) == "" {
		return ""
	}
	if port == 0 {
		port = 502
	}
	ip = strings.TrimSpace(ip)
	return "tcp://" + net.JoinHostPort(ip, strconv.FormatUint(uint64(port), 10))
}

// ParseModbusURLHostPort parses a Modbus URL (e.g. tcp://192.168.1.10:502, tcp://[::1]:502) and returns host and port.
// Host is suitable for dial (IPv6 without brackets). Returns empty host and 0 port if parsing fails.
func ParseModbusURLHostPort(modbusURL string) (host string, port uint16) {
	s := strings.TrimSpace(modbusURL)
	if s == "" {
		return "", 0
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", 0
	}
	switch u.Scheme {
	case "tcp", "tcp+tls", "rtuovertcp":
	default:
		return "", 0
	}
	host = u.Hostname()
	if host == "" {
		return "", 0
	}
	ps := u.Port()
	if ps == "" {
		return host, 502
	}
	p, err := strconv.ParseUint(ps, 10, 16)
	if err != nil {
		return "", 0
	}
	return host, uint16(p)
}

// ValidateModbusAddress ensures exactly one of url or ip is set (mutually exclusive).
// Use --url tcp://HOST:PORT for a single connection string, or --ip with --port for a literal IP (not a hostname).
func ValidateModbusAddress(url, ip string) error {
	urlSet := strings.TrimSpace(url) != ""
	ipSet := strings.TrimSpace(ip) != ""
	if urlSet && ipSet {
		return fmt.Errorf("endpoint conflict: use either --url (host:port in the URL) or --ip with --port, not both")
	}
	if !urlSet && !ipSet {
		return fmt.Errorf("modbus endpoint required: set --url (e.g. tcp://192.168.1.10:502) or --ip with --port")
	}
	if urlSet {
		u := strings.TrimSpace(url)
		host, port := ParseModbusURLHostPort(u)
		if host == "" {
			return fmt.Errorf("invalid Modbus URL %q: use tcp://, tcp+tls://, or rtuovertcp:// with a non-empty host", u)
		}
		if port == 0 {
			return fmt.Errorf("invalid Modbus URL %q: port must be between 1 and 65535", u)
		}
	}
	return nil
}

// SunSpecModbusURL returns the Modbus URL for the given base config (uses ModbusURL).
func SunSpecModbusURL(cfg *SunSpecBaseConfig) string {
	return ModbusURL(cfg.URL, cfg.IP, cfg.Port)
}
