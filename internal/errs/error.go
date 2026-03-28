package errs

import (
	"errors"
	"fmt"
)

// Error is a typed CLI/modbus failure with stable Kind and Code.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// New constructs a typed error with a user-facing message.
func New(kind Kind, code, message string, cause error) *Error {
	return &Error{Kind: kind, Code: code, Message: message, Cause: cause}
}

// InvalidInput wraps validation or bad user input.
func InvalidInput(code, message string, cause error) *Error {
	return New(KindInvalidInput, code, message, cause)
}

// Output wraps encode/render/write failures.
func Output(code string, cause error) *Error {
	msg := "output error"
	if cause != nil {
		msg = cause.Error()
	}
	return New(KindOutput, code, msg, cause)
}

// Internal wraps unexpected failures.
func Internal(cause error) *Error {
	return New(KindInternal, CodeInternalUnexpected, "internal error", cause)
}

// As returns (*Error, true) if err unwraps to *Error.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// FormatInvalid wraps format.Parse failures.
func FormatInvalid(cause error) *Error {
	return InvalidInput(CodeInvalidFormat, fmt.Sprintf("invalid output format: %v", cause), cause)
}

// WrapValidation wraps validate.* / config check failures as invalid input with a stable message prefix.
// If cause is already a typed *Error (e.g. invalid_unit_selector from unit-ID parsing), it is returned as-is.
func WrapValidation(cause error) *Error {
	if cause == nil {
		return nil
	}
	if e, ok := As(cause); ok {
		return e
	}
	return InvalidInput(CodeInvalidInput, fmt.Sprintf("invalid input: %v", cause), cause)
}
