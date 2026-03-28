package modbus

import (
	"context"
	"errors"
	"net"
	"syscall"

	gm "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
)

// classifyUnwrappedCollect maps a raw collector error to kind+code.
// Callers must return early when [errs.As] already matched a typed *errs.Error.
func classifyUnwrappedCollect(err error) (kind errs.Kind, code string, ok bool) {
	if err == nil {
		var k errs.Kind
		return k, "", false
	}
	if errors.Is(err, ErrTCPConnection) {
		return errs.KindTransport, errs.CodeTransportConnectFailed, true
	}
	if errors.Is(err, ErrModbusClientSetup) {
		return errs.KindProtocol, errs.CodeModbusClientSetupFailed, true
	}
	if errors.Is(err, ErrFC43NotSupported) {
		return errs.KindProtocol, errs.CodeDeviceIdentificationUnsupported, true
	}
	if errors.Is(err, gm.ErrRequestTimedOut) || errors.Is(err, context.DeadlineExceeded) {
		return errs.KindTimeout, errs.CodeRequestTimeout, true
	}
	if errors.Is(err, context.Canceled) {
		return errs.KindInternal, errs.CodeContextCanceled, true
	}
	var ex *gm.ExceptionError
	if errors.As(err, &ex) {
		return errs.KindModbus, errs.CodeModbusException, true
	}
	var ne *net.OpError
	if errors.As(err, &ne) {
		if ne.Timeout() {
			return errs.KindTimeout, errs.CodeTransportTimeout, true
		}
		return errs.KindTransport, errs.CodeTransportConnectFailed, true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.EPIPE, syscall.ENETUNREACH, syscall.EHOSTUNREACH:
			return errs.KindTransport, errs.CodeTransportConnectFailed, true
		case syscall.ETIMEDOUT:
			return errs.KindTimeout, errs.CodeTransportTimeout, true
		default:
			var zero errs.Kind
			return zero, "", false
		}
	}
	var k errs.Kind
	return k, "", false
}

// WrapCollectError maps known Modbus collector failures to *errs.Error for root rendering.
// Per-unit failures remain embedded in DTOs; this is for top-level collect/connection errors.
func WrapCollectError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.As(err); ok {
		return err
	}
	if kind, code, ok := classifyUnwrappedCollect(err); ok {
		return errs.New(kind, code, err.Error(), err)
	}
	return errs.Internal(err)
}

// EmbeddedErrorInfo maps an error to structured *types.ErrorInfo for per-unit / row JSON (partial results).
func EmbeddedErrorInfo(err error) *types.ErrorInfo {
	if err == nil {
		return nil
	}
	if e, ok := errs.As(err); ok {
		msg := e.Message
		if msg == "" && e.Cause != nil {
			msg = e.Cause.Error()
		}
		if msg == "" {
			msg = err.Error()
		}
		return &types.ErrorInfo{Kind: string(e.Kind), Code: e.Code, Message: msg}
	}
	if kind, code, ok := classifyUnwrappedCollect(err); ok {
		return &types.ErrorInfo{Kind: string(kind), Code: code, Message: err.Error()}
	}
	return &types.ErrorInfo{
		Kind:    string(errs.KindInternal),
		Code:    errs.CodeInternalUnexpected,
		Message: err.Error(),
	}
}
