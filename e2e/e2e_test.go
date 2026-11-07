// SPDX-License-Identifier: MIT

//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// run executes the binary named by MODBUSCTL_E2E_BIN (set by make e2e).
func run(t *testing.T, args ...string) (output string, exitCode int) {
	t.Helper()
	bin := os.Getenv("MODBUSCTL_E2E_BIN")
	if bin == "" {
		t.Fatal("MODBUSCTL_E2E_BIN is not set; run make e2e")
	}
	var out bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0
	case errors.As(err, &exit):
		return out.String(), exit.ExitCode()
	default:
		t.Fatalf("run %s: %v", bin, err)
		return "", -1
	}
}

func TestVersionPrintsTheBuiltVersion(t *testing.T) {
	output, code := run(t, "version")
	if code != 0 {
		t.Fatalf("exit code %d, want 0\n%s", code, output)
	}
	want := os.Getenv("MODBUSCTL_E2E_VERSION")
	if want == "" {
		t.Fatal("MODBUSCTL_E2E_VERSION is not set; run make e2e")
	}
	want = strings.TrimPrefix(want, "v")
	if !strings.HasPrefix(output, "modbusctl "+want+" (") || strings.Count(output, "\n") != 1 {
		t.Fatalf("output is not one line naming modbusctl %s:\n%s", want, output)
	}

	output, code = run(t, "version", "--json")
	if code != 0 {
		t.Fatalf("version --json: exit code %d, want 0\n%s", code, output)
	}
	var info map[string]string
	if err := json.Unmarshal([]byte(output), &info); err != nil || info["name"] != "modbusctl" || info["version"] != want {
		t.Fatalf("version --json is not the build information of modbusctl %s (%v):\n%s", want, err, output)
	}
}

// An unknown command is a usage error: exit code 2 in docs/exitcodes.md.
func TestUnknownCommandIsAUsageError(t *testing.T) {
	output, code := run(t, "nosuchcommand")
	if code != 2 {
		t.Fatalf("exit code %d, want 2\n%s", code, output)
	}
	if !strings.Contains(output, `unknown command "nosuchcommand"`) {
		t.Fatalf("output does not name the unknown command:\n%s", output)
	}
}
