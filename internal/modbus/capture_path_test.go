package modbus

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoCaptureMcapPath_joinsDir(t *testing.T) {
	t.Parallel()
	name := AutoCaptureMcapPath("captures", "read")
	if !strings.HasPrefix(name, "captures") || !strings.Contains(name, "modbusctl_read_") {
		t.Fatalf("unexpected path: %s", name)
	}
	if filepath.Base(name) == name {
		t.Fatalf("expected subdir: %s", name)
	}
}

func TestAutoCaptureMcapPath_emptyDirUsesDot(t *testing.T) {
	t.Parallel()
	name := AutoCaptureMcapPath("", "scan")
	base := filepath.Base(name)
	if !strings.HasPrefix(base, "modbusctl_scan_") {
		t.Fatalf("unexpected base: %s", base)
	}
}
