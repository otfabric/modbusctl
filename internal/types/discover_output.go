package types

import (
	"fmt"
	"strings"
)

// DiscoverOutput is the structured result for subnet discovery (stdout via [format.Write]).
type DiscoverOutput struct {
	Port      uint16         `json:"port"`
	Interface string         `json:"interface,omitempty"`
	Subnets   []string       `json:"subnets,omitempty"`
	Devices   []DiscoverJson `json:"devices"`
}

// MarshalTextOutput matches historical discover text lines on stdout.
func (o *DiscoverOutput) MarshalTextOutput() (string, error) {
	if len(o.Devices) == 0 {
		return "No Modbus devices found.", nil
	}
	var b strings.Builder
	for _, d := range o.Devices {
		port := d.Port
		if d.Mac != "" {
			_, _ = fmt.Fprintf(&b, "✅ Modbus device found at %s:%d (MAC: %s)\n", d.IP, port, d.Mac)
		} else {
			_, _ = fmt.Fprintf(&b, "✅ Modbus device found at %s:%d\n", d.IP, port)
		}
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// TableHeaders implements format.TableMarshaler.
func (o *DiscoverOutput) TableHeaders() []string {
	return []string{"IP", "Port", "MAC", "Interface"}
}

// TableRows implements format.TableMarshaler.
func (o *DiscoverOutput) TableRows() [][]string {
	rows := make([][]string, len(o.Devices))
	for i, d := range o.Devices {
		rows[i] = []string{
			d.IP,
			fmt.Sprintf("%d", d.Port),
			d.Mac,
			d.Interface,
		}
	}
	return rows
}
