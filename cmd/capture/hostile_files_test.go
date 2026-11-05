// SPDX-License-Identifier: MIT

package capture

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/fsutil"
	"github.com/otfabric/modbusctl/internal/types"
)

// TestOutputNamingTheInput_isRefusedBeforeTruncating: --output naming the capture
// (or the profile) truncated it before it was read, losing the recording.
func TestOutputNamingTheInput_isRefusedBeforeTruncating(t *testing.T) {
	in := recordCapture(t)
	before := readFile(t, in)
	profile := filepath.Join(t.TempDir(), "profile.json")
	writeFile(t, profile, `{"protocolData":{"registers":[{"controlledPropertyId":"f","valueScaleFactor":1,"start":104,"size":1,"format":">H"}]}}`)
	profileBefore := readFile(t, profile)
	// Another spelling of the input path names the same file.
	alias := filepath.Join(filepath.Dir(in), ".", filepath.Base(in))
	for _, args := range [][]string{
		{"capture", "extract", "--input", in, "--output", in},
		{"capture", "strings", "--input", in, "--output", alias},
		{"capture", "frequencies", "--input", in, "--output", in},
		{"capture", "decode", "--input", in, "--profile", profile, "--output", in},
		{"capture", "decode", "--input", in, "--profile", profile, "--output", profile},
	} {
		r := run(t, args...)
		wantExit(t, r, errs.ExitInvalidInput)
		if e, _ := errs.As(r.err); e.Code != errs.CodeInvalidFlagCombination {
			t.Fatalf("%v: code %q", args, e.Code)
		}
	}
	if readFile(t, in) != before || readFile(t, profile) != profileBefore {
		t.Fatal("an input was truncated")
	}
}

// TestOutputs_ownerOnlyAndNeverThroughSymlinks: outputs were created 0644 and written
// through a symbolic link planted at the output path.
func TestOutputs_ownerOnlyAndNeverThroughSymlinks(t *testing.T) {
	in := recordCapture(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "blocks.json")
	wantExit(t, run(t, "capture", "extract", "--input", in, "--output", out), errs.ExitOK)
	if st, err := os.Stat(out); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != fsutil.OutputMode) {
		t.Fatalf("output mode: %v %v", st.Mode(), err)
	}

	victim := filepath.Join(dir, "victim")
	writeFile(t, victim, "keep")
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	r := run(t, "capture", "extract", "--input", in, "--output", link)
	wantExit(t, r, errs.ExitInternal)
	if readFile(t, victim) != "keep" {
		t.Fatal("written through the symbolic link")
	}
}

// TestDecodeProfile_hostileProfiles: a profile is a file from elsewhere.
func TestDecodeProfile_hostileProfiles(t *testing.T) {
	dir := t.TempDir()
	// A capture holding registers 65534, 65535 and 0.
	in := capfiletest.WriteRecords(t, filepath.Join(dir, "top.mbcap"), types.CaptureHeader{IP: "10.0.0.1", Port: 502, Unit: 1, Function: 3},
		types.CaptureRecord{Iteration: 1, StartAddress: 65534, RegisterCount: 2, Data: []byte{0x12, 0x34, 0x56, 0x78}},
		types.CaptureRecord{Iteration: 1, StartAddress: 0, RegisterCount: 1, Data: []byte{0xAA, 0xBB}})
	out := filepath.Join(dir, "out.txt")

	// A register running past 65535 is refused by the profile check (exit 3);
	// analysis.ExportDeviceProfileDecode guards against it as well.
	profile := filepath.Join(dir, "wrap.json")
	writeFile(t, profile, `{"protocolData":{"registers":[
		{"controlledPropertyId":"top","valueScaleFactor":1,"start":65535,"size":2,"format":">I"}]}}`)
	wantExit(t, run(t, "capture", "decode", "--input", in, "--profile", profile, "--output", out), errs.ExitInvalidInput)

	// Names go to a terminal: control characters are escaped.
	profile = filepath.Join(dir, "names.json")
	writeFile(t, profile, `{"protocolData":{"registers":[{"controlledPropertyId":"x\u001b[2J","valueScaleFactor":1,"start":65534,"size":1,"format":">H"}]}}`)
	wantExit(t, run(t, "capture", "decode", "--input", in, "--profile", profile, "--output", out), errs.ExitOK)
	if got := readFile(t, out); strings.ContainsRune(got, 0x1b) || !strings.Contains(got, `x\x1b[2J = 4660.000000`) {
		t.Fatalf("output %q", got)
	}

	// A profile larger than any real one is refused without reading it whole.
	big := filepath.Join(dir, "big.json")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(fsutil.MaxJSONInput + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	wantExit(t, run(t, "capture", "decode", "--input", in, "--profile", big, "--output", out), errs.ExitInvalidInput)
}
