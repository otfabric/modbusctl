package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/mbtest"
)

// mainArgsEnv carries the modbusctl command line to the child process of [runMain].
const mainArgsEnv = "MAIN_TEST_ARGS"

// argSep separates the arguments in [mainArgsEnv].
const argSep = "\x1f"

// runMain runs the real main function with args in a child process (this test binary,
// re-executed) and returns its exit code, stdout and stderr.
func runMain(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainProcess$")
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "MODBUSCTL_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, mainArgsEnv+"="+strings.Join(args, argSep))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run child process: %v", err)
		}
		code = ee.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

// TestMainProcess is the child side of [runMain]: it hands control to main, which exits.
func TestMainProcess(t *testing.T) {
	args, ok := os.LookupEnv(mainArgsEnv)
	if !ok {
		t.Skip("helper for runMain; runs only in the child process")
	}
	os.Args = []string{"modbusctl"}
	if args != "" {
		os.Args = append(os.Args, strings.Split(args, argSep)...)
	}
	main()
	t.Fatal("main returned instead of exiting")
}

func TestMain_version(t *testing.T) {
	code, stdout, stderr := runMain(t, "version")
	if code != 0 || stderr != "" {
		t.Fatalf("version: exit %d, stderr %q", code, stderr)
	}
	if want := "modbusctl version: " + version + "\n"; stdout != want {
		t.Fatalf("version output %q, want %q", stdout, want)
	}
}

func TestMain_exitCodes(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(100, 1, 2)
	d.Identification = map[uint8]string{0: "Acme", 1: "P1", 2: "1.0"}
	d.Units = []uint8{1}
	url := "tcp://" + d.ServeTCP(t)
	out := filepath.Join(t.TempDir(), "read.mcap")

	// 0: success.
	code, stdout, stderr := runMain(t, "client", "read", "--url", url, "--start", "100", "--count", "2", "--output", out, "--format", "json")
	if code != 0 {
		t.Fatalf("read: exit %d\n%s\n%s", code, stdout, stderr)
	}
	var res struct {
		RawDataHex string `json:"raw_data_hex"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.RawDataHex != "00 01 00 02" {
		t.Fatalf("read output (%v):\n%s", err, stdout)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("capture file: %v", err)
	}

	// 3: invalid input, rendered once on stderr.
	code, stdout, stderr = runMain(t, "client", "write", "--url", url, "--values", "1")
	if code != 3 || stdout != "" || !strings.HasPrefix(stderr, "error: ") {
		t.Fatalf("write without --start: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// 6: Modbus exception.
	code, _, stderr = runMain(t, "client", "write", "--url", url, "--start", "9000", "--values", "1")
	if code != 6 || !strings.Contains(stderr, "Illegal Data Address") {
		t.Fatalf("write to an unmapped register: exit %d, stderr %q", code, stderr)
	}

	// 7: partial success, with the result on stdout.
	code, stdout, _ = runMain(t, "client", "identify", "--url", url, "--unit", "1-2")
	if code != 7 || !strings.Contains(stdout, "succeeded: 1  failed: 1") {
		t.Fatalf("identify with a failing unit: exit %d\n%s", code, stdout)
	}

	// 10: errors cobra raises itself.
	code, _, stderr = runMain(t, "no-such-command")
	if code != 10 || !strings.Contains(stderr, "error:") {
		t.Fatalf("unknown command: exit %d, stderr %q", code, stderr)
	}
}
