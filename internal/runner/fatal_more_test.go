// SPDX-License-Identifier: MIT

package runner

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestErrorInfoFromErr(t *testing.T) {
	cause := errors.New("dial tcp: refused")
	tests := []struct {
		name string
		err  error
		want types.ErrorInfo
	}{
		{"typed with message", errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "cannot connect", cause),
			types.ErrorInfo{Kind: "transport", Code: errs.CodeTransportConnectFailed, Message: "cannot connect"}},
		{"typed without message falls back to the cause", errs.New(errs.KindTimeout, errs.CodeRequestTimeout, "", cause),
			types.ErrorInfo{Kind: "timeout", Code: errs.CodeRequestTimeout, Message: "dial tcp: refused"}},
		{"typed without message or cause falls back to the kind", errs.New(errs.KindProtocol, "some_code", "", nil),
			types.ErrorInfo{Kind: "protocol", Code: "some_code", Message: "protocol"}},
		{"typed error wrapped by a plain one keeps kind and code", fmt.Errorf("while polling: %w", errs.InvalidInput(errs.CodeInvalidInput, "bad count", nil)),
			types.ErrorInfo{Kind: "invalid_input", Code: errs.CodeInvalidInput, Message: "bad count"}},
		{"untyped", cause,
			types.ErrorInfo{Kind: "internal", Code: errs.CodeInternalUnexpected, Message: "dial tcp: refused"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, errorInfoFromErr(tt.err))
		})
	}
}

func TestRenderFatal_exact(t *testing.T) {
	root := errors.New("connection refused")
	mid := fmt.Errorf("dial 10.0.0.1:502: %w", root)
	err := errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "cannot reach device", mid)
	jsonInv := &Invocation{}
	jsonInv.SetOutputFormat(format.FormatJSON)
	tableInv := &Invocation{}
	tableInv.SetOutputFormat(format.FormatTable)

	const envelope = "{\n  \"error\": {\n    \"kind\": \"transport\",\n    \"code\": \"transport_connect_failed\",\n    \"message\": \"cannot reach device\"\n  }\n}\n"
	const chain = "cause: dial 10.0.0.1:502: connection refused\ncause: connection refused\n"

	tests := []struct {
		name       string
		inv        *Invocation
		debug      bool
		wantStdout string
		wantStderr string
	}{
		{"no invocation", nil, false, "", "error: cannot reach device\n"},
		{"format not attached yet", &Invocation{OutputFormat: format.FormatJSON}, false, "", "error: cannot reach device\n"},
		{"table renders as text", tableInv, false, "", "error: cannot reach device\n"},
		{"text with debug", tableInv, true, "", "error: cannot reach device\n" + chain},
		{"json", jsonInv, false, envelope, ""},
		{"json with debug", jsonInv, true, envelope, chain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := RenderFatal(tt.inv, err, &stdout, &stderr, tt.debug)
			assert.Equal(t, errs.ExitTransport, code)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func TestRenderFatal_exitCodes(t *testing.T) {
	for kind, want := range map[errs.Kind]int{
		errs.KindUsage:        errs.ExitUsage,
		errs.KindInvalidInput: errs.ExitInvalidInput,
		errs.KindTransport:    errs.ExitTransport,
		errs.KindTimeout:      errs.ExitTimeout,
		errs.KindProtocol:     errs.ExitProtocol,
		errs.KindModbus:       errs.ExitProtocol,
		errs.KindOutput:       errs.ExitInternal,
		errs.KindInternal:     errs.ExitInternal,
		errs.KindInterrupted:  errs.ExitInterrupted,
	} {
		var stderr bytes.Buffer
		got := RenderFatal(nil, fmt.Errorf("wrapped: %w", errs.New(kind, "c", "m", nil)), &bytes.Buffer{}, &stderr, false)
		assert.Equal(t, want, got, string(kind))
		assert.Equal(t, "error: m\n", stderr.String())
	}
	var stderr bytes.Buffer
	assert.Equal(t, errs.ExitInternal, RenderFatal(nil, errors.New("plain"), &bytes.Buffer{}, &stderr, true))
	assert.Equal(t, "error: plain\n", stderr.String())
}

func TestRenderFatal_jsonStdoutWriteFails_exact(t *testing.T) {
	inv := &Invocation{}
	inv.SetOutputFormat(format.FormatJSON)
	var stderr bytes.Buffer
	code := RenderFatal(inv, errs.New(errs.KindTimeout, errs.CodeRequestTimeout, "timed out", errors.New("cause")), errWriter{}, &stderr, true)
	assert.Equal(t, errs.ExitInternal, code, "a fatal that cannot be rendered is an internal error")
	assert.Regexp(t, `^error: write stdout: .+\nerror: failed to render JSON fatal: .+\n$`, stderr.String())
	assert.NotContains(t, stderr.String(), "cause:", "the cause chain is not printed when the envelope could not be written")
}
