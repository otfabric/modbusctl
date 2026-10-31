// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coreTimeoutError is a net.Error reporting a timeout.
type coreTimeoutError struct{}

func (coreTimeoutError) Error() string   { return "i/o timeout" }
func (coreTimeoutError) Timeout() bool   { return true }
func (coreTimeoutError) Temporary() bool { return true }

func TestCoreClassifyUnwrappedCollect(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantOK   bool
		wantKind errs.Kind
		wantCode string
	}{
		{name: "nil", err: nil},
		{name: "connection", err: TCPConnectionError(errors.New("refused")), wantOK: true, wantKind: errs.KindTransport, wantCode: errs.CodeTransportConnectFailed},
		{name: "client setup", err: ClientSetupError(errors.New("bad")), wantOK: true, wantKind: errs.KindProtocol, wantCode: errs.CodeModbusClientSetupFailed},
		{name: "fc43 unsupported", err: fmt.Errorf("unit 1: %w", ErrFC43NotSupported), wantOK: true, wantKind: errs.KindProtocol, wantCode: errs.CodeDeviceIdentificationUnsupported},
		{name: "request timeout", err: fmt.Errorf("read: %w", gomodbus.ErrRequestTimedOut), wantOK: true, wantKind: errs.KindTimeout, wantCode: errs.CodeRequestTimeout},
		{name: "deadline", err: context.DeadlineExceeded, wantOK: true, wantKind: errs.KindTimeout, wantCode: errs.CodeRequestTimeout},
		{name: "canceled", err: fmt.Errorf("scan: %w", context.Canceled), wantOK: true, wantKind: errs.KindInterrupted, wantCode: errs.CodeContextCanceled},
		{name: "modbus exception", err: fmt.Errorf("read: %w", &gomodbus.ExceptionError{ExceptionCode: 0x02}), wantOK: true, wantKind: errs.KindModbus, wantCode: errs.CodeModbusException},
		{name: "network timeout", err: &net.OpError{Op: "read", Err: coreTimeoutError{}}, wantOK: true, wantKind: errs.KindTimeout, wantCode: errs.CodeTransportTimeout},
		{name: "network error", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, wantOK: true, wantKind: errs.KindTransport, wantCode: errs.CodeTransportConnectFailed},
		{name: "errno refused", err: fmt.Errorf("dial: %w", syscall.ECONNREFUSED), wantOK: true, wantKind: errs.KindTransport, wantCode: errs.CodeTransportConnectFailed},
		{name: "errno reset", err: syscall.ECONNRESET, wantOK: true, wantKind: errs.KindTransport, wantCode: errs.CodeTransportConnectFailed},
		{name: "errno broken pipe", err: syscall.EPIPE, wantOK: true, wantKind: errs.KindTransport, wantCode: errs.CodeTransportConnectFailed},
		{name: "errno network unreachable", err: syscall.ENETUNREACH, wantOK: true, wantKind: errs.KindTransport, wantCode: errs.CodeTransportConnectFailed},
		{name: "errno host unreachable", err: syscall.EHOSTUNREACH, wantOK: true, wantKind: errs.KindTransport, wantCode: errs.CodeTransportConnectFailed},
		{name: "errno timed out", err: syscall.ETIMEDOUT, wantOK: true, wantKind: errs.KindTimeout, wantCode: errs.CodeTransportTimeout},
		{name: "other errno", err: syscall.EINVAL},
		{name: "anything else", err: errors.New("boom")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, code, ok := classifyUnwrappedCollect(tc.err)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantKind, kind)
			assert.Equal(t, tc.wantCode, code)

			if tc.err == nil {
				assert.NoError(t, WrapCollectError(nil))
				assert.Nil(t, EmbeddedErrorInfo(nil))
				return
			}
			wantKind, wantCode := tc.wantKind, tc.wantCode
			if !tc.wantOK {
				wantKind, wantCode = errs.KindInternal, errs.CodeInternalUnexpected
			}
			wrapped, isErrs := errs.As(WrapCollectError(tc.err))
			require.True(t, isErrs)
			assert.Equal(t, wantKind, wrapped.Kind)
			assert.Equal(t, wantCode, wrapped.Code)
			assert.ErrorIs(t, wrapped, tc.err, "the cause stays reachable")

			info := EmbeddedErrorInfo(tc.err)
			assert.Equal(t, &types.ErrorInfo{Kind: string(wantKind), Code: wantCode, Message: tc.err.Error()}, info)
		})
	}
}

func TestCoreEmbeddedErrorInfo_typedErrors(t *testing.T) {
	t.Run("message wins", func(t *testing.T) {
		err := errs.InvalidInput(errs.CodeInvalidInput, "bad value", errors.New("cause"))
		assert.Equal(t, &types.ErrorInfo{Kind: "invalid_input", Code: errs.CodeInvalidInput, Message: "bad value"}, EmbeddedErrorInfo(err))
	})

	t.Run("cause text when there is no message", func(t *testing.T) {
		err := errs.New(errs.KindOutput, errs.CodeCaptureWriteFailed, "", os.ErrPermission)
		assert.Equal(t, &types.ErrorInfo{Kind: "output", Code: errs.CodeCaptureWriteFailed, Message: os.ErrPermission.Error()}, EmbeddedErrorInfo(err))
	})

	t.Run("neither message nor cause", func(t *testing.T) {
		err := errs.New(errs.KindProtocol, errs.CodeDiagnosticsFailed, "", nil)
		info := EmbeddedErrorInfo(err)
		require.NotNil(t, info)
		assert.Equal(t, "protocol", info.Kind)
		assert.Equal(t, errs.CodeDiagnosticsFailed, info.Code)
		assert.Equal(t, err.Error(), info.Message)
	})

	t.Run("typed error inside a wrapper", func(t *testing.T) {
		inner := errs.Output(errs.CodeOutputFileCreateFailed, os.ErrNotExist)
		err := fmt.Errorf("unit 3: %w", inner)
		assert.Same(t, err, WrapCollectError(err), "an error that already carries a kind is passed through")
		info := EmbeddedErrorInfo(err)
		assert.Equal(t, "output", info.Kind)
		assert.Equal(t, errs.CodeOutputFileCreateFailed, info.Code)
	})
}

func TestCoreErrorConstructors(t *testing.T) {
	cause := errors.New("dial tcp: refused")

	conn := TCPConnectionError(cause)
	assert.ErrorIs(t, conn, ErrTCPConnection)
	assert.ErrorIs(t, conn, cause)
	assert.Equal(t, "connection error: dial tcp: refused", conn.Error())

	setup := ClientSetupError(cause)
	assert.ErrorIs(t, setup, ErrModbusClientSetup)
	assert.ErrorIs(t, setup, cause)
	assert.NotErrorIs(t, setup, ErrTCPConnection)

	invalid := ClientConfigInvalid(cause)
	e, ok := errs.As(invalid)
	require.True(t, ok)
	assert.Equal(t, errs.KindInvalidInput, e.Kind)
	assert.Equal(t, errs.CodeInvalidInput, e.Code)
	assert.Equal(t, "invalid modbus client configuration: dial tcp: refused", e.Message)
	assert.ErrorIs(t, invalid, cause)
	assert.NoError(t, ClientConfigInvalid(nil))
}
