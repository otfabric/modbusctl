package types

import "github.com/otfabric/modbusctl/internal/errs"

// ErrorInfo is the structured error object for JSON output and embedded per-unit rows.
type ErrorInfo struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorEnvelope is the top-level JSON shape for fatal errors (--format json).
type ErrorEnvelope struct {
	Error ErrorInfo `json:"error"`
}

// ErrorMessage returns the display/API message for an embedded error pointer.
func ErrorMessage(e *ErrorInfo) string {
	if e == nil {
		return ""
	}
	return e.Message
}

// EmbeddedModbusError forces kind/code to a generic Modbus exception bucket (tests and legacy call sites).
// For real errors from collectors, use modbus.EmbeddedErrorInfo so JSON carries accurate kind/code.
func EmbeddedModbusError(message string) *ErrorInfo {
	return &ErrorInfo{
		Kind:    string(errs.KindModbus),
		Code:    errs.CodeModbusException,
		Message: message,
	}
}
