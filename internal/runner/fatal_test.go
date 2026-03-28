package runner

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
)

func TestFatalExitCode_typed(t *testing.T) {
	if got := fatalExitCode(errs.InvalidInput(errs.CodeInvalidInput, "bad", nil)); got != errs.ExitInvalidInput {
		t.Fatalf("got %d want %d", got, errs.ExitInvalidInput)
	}
	if got := fatalExitCode(errs.Output(errs.CodeJSONEncodeFailed, errors.New("x"))); got != errs.ExitInternal {
		t.Fatalf("KindOutput got %d want %d", got, errs.ExitInternal)
	}
}

func TestFatalExitCode_unknown(t *testing.T) {
	if got := fatalExitCode(errors.New("plain")); got != errs.ExitInternal {
		t.Fatalf("got %d want ExitInternal", got)
	}
}

func TestRenderFatal_textStderrAndCode(t *testing.T) {
	inv := &Invocation{}
	var stdout, stderr bytes.Buffer
	code := RenderFatal(inv, errs.InvalidInput(errs.CodeInvalidInput, "bad input", nil), &stdout, &stderr, false)
	if code != errs.ExitInvalidInput {
		t.Fatalf("exit code = %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout: %q", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("error:")) {
		t.Fatalf("stderr: %q", stderr.String())
	}
}

func TestRenderFatal_jsonStdout(t *testing.T) {
	inv := &Invocation{}
	inv.SetOutputFormat(format.FormatJSON)
	var stdout, stderr bytes.Buffer
	code := RenderFatal(inv, errs.InvalidInput(errs.CodeInvalidInput, "x", nil), &stdout, &stderr, false)
	if code != errs.ExitInvalidInput {
		t.Fatalf("exit code = %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr: %q", stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"error"`)) {
		t.Fatalf("stdout: %q", stdout.String())
	}
}

func TestRenderFatal_debugWritesCauseChain(t *testing.T) {
	inv := &Invocation{}
	inv.SetOutputFormat(format.FormatText)
	inner := errors.New("root cause")
	wrapped := fmt.Errorf("middle: %w", inner)
	err := errs.InvalidInput(errs.CodeInvalidInput, "top", wrapped)
	var stdout, stderr bytes.Buffer
	_ = RenderFatal(inv, err, &stdout, &stderr, true)
	se := stderr.String()
	if !strings.Contains(se, "cause:") || !strings.Contains(se, "root cause") {
		t.Fatalf("stderr: %q", se)
	}
}

func TestRenderFatal_jsonStdoutWriteFails(t *testing.T) {
	inv := &Invocation{}
	inv.SetOutputFormat(format.FormatJSON)
	err := errs.InvalidInput(errs.CodeInvalidInput, "x", nil)
	var stderr bytes.Buffer
	code := RenderFatal(inv, err, errWriter{}, &stderr, false)
	if code != errs.ExitInternal {
		t.Fatalf("code %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("write stdout")) {
		t.Fatalf("stderr: %q", stderr.String())
	}
}

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write denied")
}
