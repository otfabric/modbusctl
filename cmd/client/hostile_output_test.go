// SPDX-License-Identifier: MIT

package client

import (
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
)

// hostileText is what a hostile device puts in its strings: a screen clear, a
// window-title change, a carriage return that overwrites the line, a fake extra
// line, a tab that breaks table columns, a bidi override and a C1 CSI.
const hostileText = "Acme\x1b[2J\x1b]0;owned\x07\rFAKE\nObject 9: forged\tcol\u202eevil\u009b31m"

// requireTerminalSafe fails when out carries a control character other than a
// newline (and a tab in text output), or a bidi override.
func requireTerminalSafe(t *testing.T, what, out string, tabs bool) {
	t.Helper()
	for i, r := range out {
		ok := r == '\n' || (tabs && r == '\t') || (r >= 0x20 && r != 0x7f && (r < 0x80 || r > 0x9f))
		if !ok || (r >= 0x202a && r <= 0x202e) {
			t.Fatalf("%s: control character %U at byte %d reaches the terminal:\n%q", what, r, i, out)
		}
	}
}

// TestHostileDevice_identifyStringsAreEscaped: FC43 objects are device strings.
// Text and table output escape their control characters; JSON keeps the value.
func TestHostileDevice_identifyStringsAreEscaped(t *testing.T) {
	d, _ := newDevice(t)
	d.Identification = map[uint8]string{0: hostileText, 1: "P-1000", 2: "1.2.3"}
	d.Units = []uint8{1}
	url := "tcp://" + d.ServeTCP(t)

	r := run(t, "client", "identify", "--url", url)
	wantExit(t, r, errs.ExitOK)
	requireTerminalSafe(t, "identify text", r.stdout, true)
	wantContains(t, r.stdout, `Object 0: Acme\x1b[2J\x1b]0;owned\x07\rFAKE\nObject 9: forged\tcol\u202eevil\x9b31m [VendorName]`)
	if strings.Count(r.stdout, "\n - Object ") != 3 {
		t.Fatalf("the device string forged an output line:\n%s", r.stdout)
	}

	r = run(t, "client", "identify", "--url", url, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	requireTerminalSafe(t, "identify table", r.stdout, false)

	r = run(t, "client", "identify", "--url", url, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.IdentifyResult
	decodeJSON(t, r.stdout, &res)
	if got := res.Units[0].Objects[0].Value; got != hostileText {
		t.Fatalf("JSON value %q, want the device string unchanged %q", got, hostileText)
	}
}

// TestHostileDevice_decodedStringsAreEscaped: an ASCII codec turns register bytes
// into a string; text and table output escape its control characters.
func TestHostileDevice_decodedStringsAreEscaped(t *testing.T) {
	// "\x1b[2J" + "\r\nX" in four registers.
	d := mbtest.NewDevice().SetHolding(0, 0x1b5b, 0x324a, 0x0d0a, 0x5807)
	url := "tcp://" + d.ServeTCP(t)
	for _, f := range []string{"text", "table"} {
		r := run(t, "client", "decode", "--url", url, "--function", "3", "--start", "0", "--codec", "ascii_fixed/registers:4", "--format", f)
		wantExit(t, r, errs.ExitOK)
		requireTerminalSafe(t, "decode "+f, r.stdout, f == "text")
		wantContains(t, r.stdout, `\x1b[2J\r\nX\x07`)
	}
	r := run(t, "client", "poll", "--url", url, "--function", "3", "--start", "0", "--codec", "ascii_fixed/registers:4", "--polls", "1", "--interval", "1")
	wantExit(t, r, errs.ExitOK)
	requireTerminalSafe(t, "poll text", r.stdout, true)
}
