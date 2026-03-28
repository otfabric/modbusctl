package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	gm "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/stretchr/testify/require"
)

func TestWrapCollectError_passthroughTyped(t *testing.T) {
	inner := errs.InvalidInput(errs.CodeInvalidInput, "x", nil)
	got := WrapCollectError(inner)
	require.Equal(t, inner, got)
}

func TestWrapCollectError_passthroughCollectorDomain(t *testing.T) {
	inner := errs.New(errs.KindProtocol, errs.CodeSunSpecDetectFailed, "SunSpec detect failed", errors.New("underlying"))
	got := WrapCollectError(inner)
	require.Equal(t, inner, got)
}

func TestWrapCollectError_requestTimeout(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", gm.ErrRequestTimedOut)
	got := WrapCollectError(wrapped)
	e, ok := errs.As(got)
	require.True(t, ok)
	require.Equal(t, errs.KindTimeout, e.Kind)
	require.Equal(t, errs.CodeRequestTimeout, e.Code)
}

func TestWrapCollectError_deadlineExceeded(t *testing.T) {
	got := WrapCollectError(context.DeadlineExceeded)
	e, ok := errs.As(got)
	require.True(t, ok)
	require.Equal(t, errs.KindTimeout, e.Kind)
}

func TestWrapCollectError_contextCanceled(t *testing.T) {
	got := WrapCollectError(context.Canceled)
	e, ok := errs.As(got)
	require.True(t, ok)
	require.Equal(t, errs.KindInternal, e.Kind)
	require.Equal(t, errs.CodeContextCanceled, e.Code)
}

func TestWrapCollectError_fc43Unsupported(t *testing.T) {
	got := WrapCollectError(ErrFC43NotSupported)
	e, ok := errs.As(got)
	require.True(t, ok)
	require.Equal(t, errs.KindProtocol, e.Kind)
	require.Equal(t, errs.CodeDeviceIdentificationUnsupported, e.Code)
}

func TestWrapCollectError_netOpError_refused(t *testing.T) {
	op := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	got := WrapCollectError(op)
	e, ok := errs.As(got)
	require.True(t, ok)
	require.Equal(t, errs.KindTransport, e.Kind)
	require.Equal(t, errs.CodeTransportConnectFailed, e.Code)
}

func TestWrapCollectError_errno_timedOut(t *testing.T) {
	got := WrapCollectError(syscall.ETIMEDOUT)
	e, ok := errs.As(got)
	require.True(t, ok)
	require.Equal(t, errs.KindTimeout, e.Kind)
	require.Equal(t, errs.CodeTransportTimeout, e.Code)
}

func TestWrapCollectError_unknownInternal(t *testing.T) {
	got := WrapCollectError(errors.New("weird"))
	e, ok := errs.As(got)
	require.True(t, ok)
	require.Equal(t, errs.KindInternal, e.Kind)
}

func TestEmbeddedErrorInfo_nil(t *testing.T) {
	require.Nil(t, EmbeddedErrorInfo(nil))
}

func TestEmbeddedErrorInfo_typedWithCauseMessage(t *testing.T) {
	inner := errors.New("cause line")
	e := errs.InvalidInput(errs.CodeInvalidInput, "", inner)
	info := EmbeddedErrorInfo(e)
	require.Equal(t, string(errs.KindInvalidInput), info.Kind)
	require.Equal(t, errs.CodeInvalidInput, info.Code)
	require.Equal(t, "cause line", info.Message)
}

func TestEmbeddedErrorInfo_classifyTCP(t *testing.T) {
	inner := errors.New("refused")
	info := EmbeddedErrorInfo(TCPConnectionError(inner))
	require.Equal(t, string(errs.KindTransport), info.Kind)
	require.Equal(t, errs.CodeTransportConnectFailed, info.Code)
}

func TestEmbeddedErrorInfo_classifyClientSetup(t *testing.T) {
	info := EmbeddedErrorInfo(ClientSetupError(errors.New("new failed")))
	require.Equal(t, string(errs.KindProtocol), info.Kind)
	require.Equal(t, errs.CodeModbusClientSetupFailed, info.Code)
}

func TestEmbeddedErrorInfo_plain(t *testing.T) {
	info := EmbeddedErrorInfo(errors.New("plain failure"))
	require.Equal(t, string(errs.KindInternal), info.Kind)
	require.Equal(t, errs.CodeInternalUnexpected, info.Code)
	require.Equal(t, "plain failure", info.Message)
}
