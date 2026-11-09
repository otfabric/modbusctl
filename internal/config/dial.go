// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"strings"

	"github.com/otfabric/modbusctl/internal/netdial"
)

// DialConfig selects how the network connection to the device is opened. Every command
// that connects to a device embeds it. Zero values keep go-modbus's own dialer.
type DialConfig struct {
	Proxy      string `env:"MODBUSCTL_PROXY" desc:"Connect through a SOCKS5 proxy: socks5://host:port (socks5h:// resolves names at the proxy); tcp://, tcp+tls://, rtuovertcp:// and asciiovertcp:// only" flag:"proxy"`
	SourceAddr string `env:"MODBUSCTL_SOURCE_ADDR" desc:"Local IP address to connect from (multi-homed hosts); network URLs only" flag:"source-addr"`
}

// proxySchemes are the network schemes that run over TCP, which a SOCKS5 CONNECT carries.
var proxySchemes = []string{"tcp", "tcp+tls", "rtuovertcp", "asciiovertcp"}

// IsSet reports whether --proxy or --source-addr is set.
func (d DialConfig) IsSet() bool {
	return strings.TrimSpace(d.Proxy) != "" || strings.TrimSpace(d.SourceAddr) != ""
}

// Dialer returns the dialer of the flags for modbus.Config.Dialer, or nil when neither is
// set. An error means a flag is invalid.
func (d DialConfig) Dialer() (netdial.Dialer, error) {
	return netdial.New(strings.TrimSpace(d.SourceAddr), strings.TrimSpace(d.Proxy))
}

// ValidateDialConfig checks --proxy and --source-addr against modbusURL (the URL of --url,
// or "" for --ip/--port, which is tcp://). A serial URL takes neither; a UDP URL takes no
// proxy, since SOCKS5 UDP ASSOCIATE is not implemented. Unset flags always pass.
func ValidateDialConfig(modbusURL string, d DialConfig) error {
	if !d.IsSet() {
		return nil
	}
	scheme := "tcp"
	if strings.TrimSpace(modbusURL) != "" {
		scheme, _, _ = splitModbusURL(modbusURL)
	}
	if hasScheme(serialSchemes, scheme) {
		return fmt.Errorf("--proxy and --source-addr do not apply to %s:// (a serial line)", scheme)
	}
	if strings.TrimSpace(d.Proxy) != "" && !hasScheme(proxySchemes, scheme) {
		return fmt.Errorf("--proxy does not apply to %s://: SOCKS5 carries TCP only (use %s)", scheme, strings.Join(proxySchemes, ", "))
	}
	_, err := d.Dialer()
	return err
}
