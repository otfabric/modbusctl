package modbus

import (
	"errors"
	"fmt"

	"github.com/otfabric/modbusctl/internal/errs"
)

// TCPConnectionError wraps cause as a transport connect/open failure.
// errors.Is(err, ErrTCPConnection) is true for the returned error.
func TCPConnectionError(cause error) error {
	if cause == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrTCPConnection, cause)
}

// ErrModbusClientSetup marks failures from modbus.New / client construction (not FC43 device support).
var ErrModbusClientSetup = errors.New("modbus client setup failed")

// ClientSetupError wraps modbus.New (or equivalent) failures. Use for any command; it is not FC43-specific.
// errors.Is(err, ErrModbusClientSetup) is true for the returned error.
func ClientSetupError(cause error) error {
	if cause == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrModbusClientSetup, cause)
}

// ClientConfigInvalid wraps go-modbus ValidateConfig failures as invalid input.
func ClientConfigInvalid(cause error) error {
	if cause == nil {
		return nil
	}
	return errs.InvalidInput(errs.CodeInvalidInput, "invalid modbus client configuration: "+cause.Error(), cause)
}
