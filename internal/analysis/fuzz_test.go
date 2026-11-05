// SPDX-License-Identifier: MIT

package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// FuzzDeviceProfileDecode feeds arbitrary device profiles (JSON files from
// elsewhere) to capture decode over a capture holding both ends of the address
// space. It must not panic, and what it prints carries no control character.
func FuzzDeviceProfileDecode(f *testing.F) {
	capture := capfiletest.WriteRecords(f, filepath.Join(f.TempDir(), "c.mbcap"),
		types.CaptureHeader{IP: "10.0.0.1", Port: 502, Unit: 1, Function: 3},
		types.CaptureRecord{Iteration: 1, StartAddress: 65533, RegisterCount: 3, Data: []byte{1, 2, 3, 4, 5, 6}},
		types.CaptureRecord{Iteration: 1, StartAddress: 0, RegisterCount: 2, Data: []byte{0x41, 0xAC, 0, 0}})
	for _, s := range []string{
		`{"protocolData":{"registers":[{"controlledPropertyId":"t","valueScaleFactor":1,"start":0,"size":2,"format":">f"}]}}`,
		`{"protocolData":{"registers":[{"controlledPropertyId":"w","valueScaleFactor":1,"start":65535,"size":65535,"format":">I"}]}}`,
		`{"protocolData":{"registers":[{"controlledPropertyId":"x\u001b[2J\n","start":65533,"size":1,"format":">H"}]}}`,
		`{"protocolData":{"registers":[{"controlledPropertyId":"y","start":65534,"size":2,"format":"??\u0007"}]}}`,
		`{}`, `[]`, `{"protocolData":{"registers":null}}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, profile []byte) {
		path := filepath.Join(t.TempDir(), "profile.json")
		if err := os.WriteFile(path, profile, 0o600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := ExportDeviceProfileDecode(&out, capture, path, config.CaptureSelection{}); err != nil {
			return
		}
		if strings.ContainsFunc(out.String(), func(r rune) bool { return r != '\n' && (r < 0x20 || r == 0x7f) }) {
			t.Fatalf("control character in %q", out.String())
		}
	})
}
