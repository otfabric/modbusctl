package main_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLI_fatalText_emptyStdout_noCobraErrorPrefix(t *testing.T) {
	bin := buildModbusctl(t)
	cmd := exec.Command(bin, "client", "identify", "--format", "not-a-format")
	cmd.Env = modbusctlEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout not empty: %q", stdout.String())
	}
	se := stderr.String()
	if !strings.Contains(se, "error:") {
		t.Fatalf("stderr: %q", se)
	}
	if strings.Contains(se, "\nError:") || strings.HasPrefix(strings.TrimSpace(se), "Error:") {
		t.Fatalf("unexpected Cobra-style Error: line in stderr: %q", se)
	}
}

func TestCLI_mcap_convert_fatal_missingMCAP(t *testing.T) {
	bin := buildModbusctl(t)
	badPath := filepath.Join(t.TempDir(), "does-not-exist.mcap")
	cmd := exec.Command(bin, "mcap", "convert", "--input", badPath, "--format", "csv")
	cmd.Env = modbusctlEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("want ExitError, got %T %v", err, err)
	}
	if code := ee.ExitCode(); code != 3 {
		t.Fatalf("exit code %d want 3 (invalid input)", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout not empty: %q", stdout.String())
	}
	se := stderr.String()
	if !strings.Contains(se, "error:") {
		t.Fatalf("stderr: %q", se)
	}
}

func TestCLI_fatalJSON_stdoutEnvelope_emptyStderr(t *testing.T) {
	bin := buildModbusctl(t)
	cmd := exec.Command(bin, "client", "read", "--format", "json", "--start", "40001")
	cmd.Env = modbusctlEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr without --debug, got %q", stderr.String())
	}
	var env struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if len(env.Error) == 0 {
		t.Fatalf("missing error object: %s", stdout.String())
	}
}

func modbusctlEnv() []string {
	var out []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "MODBUSCTL_") {
			continue
		}
		out = append(out, e)
	}
	return out
}

func buildModbusctl(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	bin := filepath.Join(t.TempDir(), "modbusctl")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}
