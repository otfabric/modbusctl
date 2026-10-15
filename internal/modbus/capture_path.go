package modbus

import (
	"fmt"
	"path/filepath"
	"time"
)

// AutoCapturePath returns filepath.Join(dir, "modbusctl_<kind>_<UTC_timestamp>.mbcap") with a filesystem-safe timestamp (no colons).
// kind is typically "read", "scan", or "record". dir may be ".", "./captures", or "captures/" — Join normalizes separators.
func AutoCapturePath(dir, kind string) string {
	if kind == "" {
		kind = "capture"
	}
	if dir == "" {
		dir = "."
	}
	ts := time.Now().UTC().Format("20060102_150405")
	name := fmt.Sprintf("modbusctl_%s_%s.mbcap", kind, ts)
	return filepath.Join(dir, name)
}
