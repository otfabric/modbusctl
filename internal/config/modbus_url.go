package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// URL schemes accepted by --url, grouped by how the address part is interpreted.
var (
	// networkSchemes take host:port.
	networkSchemes = []string{"tcp", "tcp+tls", "udp", "rtuovertcp", "rtuoverudp", "asciiovertcp"}
	// serialSchemes take a serial device (e.g. /dev/ttyUSB0 or COM3).
	serialSchemes = []string{"rtu", "ascii"}
)

// ModbusURLSchemes returns every URL scheme accepted by --url.
func ModbusURLSchemes() []string {
	return append(append([]string{}, networkSchemes...), serialSchemes...)
}

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

// splitModbusURL splits "<scheme>://<address>" the same way go-modbus does.
func splitModbusURL(modbusURL string) (scheme, address string, ok bool) {
	scheme, address, ok = strings.Cut(strings.TrimSpace(modbusURL), "://")
	return scheme, address, ok
}

func hasScheme(schemes []string, scheme string) bool {
	for _, s := range schemes {
		if s == scheme {
			return true
		}
	}
	return false
}

// IsSerialURL reports whether modbusURL addresses a serial line (rtu:// or ascii://).
// A serial line carries one request at a time, so it cannot be shared by parallel clients.
func IsSerialURL(modbusURL string) bool {
	scheme, _, ok := splitModbusURL(modbusURL)
	return ok && hasScheme(serialSchemes, scheme)
}

// SerialDevice returns the device of a serial Modbus URL (rtu:///dev/ttyUSB0 → /dev/ttyUSB0,
// rtu://COM3 → COM3), or "" when modbusURL is not a serial URL.
func SerialDevice(modbusURL string) string {
	scheme, address, ok := splitModbusURL(modbusURL)
	if !ok || !hasScheme(serialSchemes, scheme) {
		return ""
	}
	return address
}

// ParseModbusURLHostPort parses a network Modbus URL (e.g. tcp://192.168.1.10:502, udp://[::1]:502) and returns host and port.
// Host is suitable for dial (IPv6 without brackets). Returns empty host and 0 port if parsing fails or the URL is a serial one.
func ParseModbusURLHostPort(modbusURL string) (host string, port uint16) {
	s := strings.TrimSpace(modbusURL)
	if s == "" {
		return "", 0
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", 0
	}
	if !hasScheme(networkSchemes, u.Scheme) {
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

// CaptureEndpoint returns the endpoint recorded in capture headers for modbusURL:
// host and port for network URLs, or the serial device and port 0 for serial URLs.
func CaptureEndpoint(modbusURL string) (endpoint string, port uint16) {
	if dev := SerialDevice(modbusURL); dev != "" {
		return dev, 0
	}
	return ParseModbusURLHostPort(modbusURL)
}

// ValidateModbusURL checks that modbusURL has a supported scheme and a usable address:
// host and port (1-65535) for network schemes, a device for serial schemes.
func ValidateModbusURL(modbusURL string) error {
	u := strings.TrimSpace(modbusURL)
	scheme, address, ok := splitModbusURL(u)
	if !ok || (!hasScheme(networkSchemes, scheme) && !hasScheme(serialSchemes, scheme)) {
		return fmt.Errorf("invalid Modbus URL %q: use one of %s followed by ://", u, strings.Join(ModbusURLSchemes(), ", "))
	}
	if hasScheme(serialSchemes, scheme) {
		if strings.TrimSpace(address) == "" {
			return fmt.Errorf("invalid Modbus URL %q: %s:// needs a serial device (e.g. %s:///dev/ttyUSB0 or %s://COM3)", u, scheme, scheme, scheme)
		}
		return nil
	}
	host, port := ParseModbusURLHostPort(u)
	if host == "" {
		return fmt.Errorf("invalid Modbus URL %q: %s:// needs a host and port (e.g. %s://192.168.1.10:502)", u, scheme, scheme)
	}
	if port == 0 {
		return fmt.Errorf("invalid Modbus URL %q: port must be between 1 and 65535", u)
	}
	return nil
}

// ValidateModbusAddress ensures exactly one of url or ip is set (mutually exclusive).
// Use --url for a single connection string (any supported scheme), or --ip with --port for a literal IP (not a hostname) over Modbus TCP.
func ValidateModbusAddress(url, ip string) error {
	urlSet := strings.TrimSpace(url) != ""
	ipSet := strings.TrimSpace(ip) != ""
	if urlSet && ipSet {
		return fmt.Errorf("endpoint conflict: use either --url (host:port in the URL) or --ip with --port, not both")
	}
	if !urlSet && !ipSet {
		return fmt.Errorf("modbus endpoint required: set --url (e.g. tcp://192.168.1.10:502 or rtu:///dev/ttyUSB0) or --ip with --port")
	}
	if urlSet {
		return ValidateModbusURL(url)
	}
	return nil
}

// SunSpecModbusURL returns the Modbus URL for the given base config (uses ModbusURL).
func SunSpecModbusURL(cfg *SunSpecBaseConfig) string {
	return ModbusURL(cfg.URL, cfg.IP, cfg.Port)
}
