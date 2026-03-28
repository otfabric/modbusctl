package modbus

import (
	"errors"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/stretchr/testify/require"
)

func TestTCPConnectionError_is(t *testing.T) {
	inner := errors.New("dial refused")
	wrapped := TCPConnectionError(inner)
	require.True(t, errors.Is(wrapped, ErrTCPConnection))
	require.True(t, errors.Is(wrapped, inner))
}

func TestClientSetupError_is(t *testing.T) {
	inner := errors.New("new client failed")
	wrapped := ClientSetupError(inner)
	require.True(t, errors.Is(wrapped, ErrModbusClientSetup))
	require.True(t, errors.Is(wrapped, inner))
}

func TestTCPConnectionError_nil(t *testing.T) {
	require.Nil(t, TCPConnectionError(nil))
}

func TestClientSetupError_nil(t *testing.T) {
	require.Nil(t, ClientSetupError(nil))
}

func TestClientConfigInvalid(t *testing.T) {
	require.Nil(t, ClientConfigInvalid(nil))
	cause := errors.New("bad baud")
	wrapped := ClientConfigInvalid(cause)
	e, ok := errs.As(wrapped)
	require.True(t, ok)
	require.Equal(t, errs.KindInvalidInput, e.Kind)
	require.Contains(t, e.Message, "invalid modbus client configuration")
	require.Equal(t, cause, e.Unwrap())
}
