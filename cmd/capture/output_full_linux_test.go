//go:build linux

// SPDX-License-Identifier: MIT

package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
)

// An --output that cannot be written ends every capture command with an output error, exit 10:
// /dev/full answers each write with "no space left on device".
func TestOutputWriteFailure(t *testing.T) {
	const full = "/dev/full"
	if _, err := os.Stat(full); err != nil {
		if os.Getenv("MODBUSCTL_REQUIRE_PREREQS") == "1" {
			t.Fatalf("MODBUSCTL_REQUIRE_PREREQS=1: %s: %v", full, err)
		}
		t.Skipf("%s: %v (MODBUSCTL_REQUIRE_PREREQS=1 makes this a failure)", full, err)
	}
	in := recordCapture(t)
	profile := filepath.Join(t.TempDir(), "profile.json")
	writeFile(t, profile, `{"protocolData":{"registers":[{"controlledPropertyId":"f","valueScaleFactor":1,"start":104,"size":1,"format":">H"}]}}`)
	for _, args := range [][]string{
		{"extract"},
		{"strings"},
		{"frequencies"},
		{"decode", "--profile", profile},
	} {
		t.Run(args[0], func(t *testing.T) {
			r := run(t, append(append([]string{"capture"}, args...), "--input", in, "--output", full)...)
			wantKind(t, r, errs.KindOutput)
			wantExit(t, r, errs.ExitInternal)
			if e, _ := errs.As(r.err); e.Code != errs.CodeOutputFileWriteFailed {
				t.Fatalf("code %s, want %s", e.Code, errs.CodeOutputFileWriteFailed)
			}
		})
	}
}
