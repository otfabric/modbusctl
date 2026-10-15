package capfile

import (
	"errors"

	"github.com/otfabric/modbusctl/internal/errs"
)

// WrapError maps an error from loading or selecting capture data to a typed
// modbusctl error with a stable code:
//
//   - a modbusctl 2.x capture file: invalid input, capture_legacy_format (exit 3);
//   - an ambiguous or empty --target/--function selection: invalid input,
//     capture_selection_ambiguous or capture_selection_empty (exit 3);
//   - a sample of a function modbusctl does not support (vendor or unknown
//     function codes): invalid input, capture_unsupported_function (exit 3);
//   - anything else (a missing, unreadable, corrupt or truncated file): output,
//     capture_load_failed (exit 10).
//
// An error that is already typed is returned as it is.
func WrapError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.As(err); ok {
		return err
	}
	switch {
	case errors.Is(err, ErrLegacyFormat):
		return errs.InvalidInput(errs.CodeCaptureLegacyFormat, err.Error(), err)
	case errors.Is(err, ErrAmbiguousSelection):
		return errs.InvalidInput(errs.CodeCaptureSelectionAmbiguous, err.Error(), err)
	case errors.Is(err, ErrEmptySelection):
		return errs.InvalidInput(errs.CodeCaptureSelectionEmpty, err.Error(), err)
	case errors.Is(err, ErrUnsupportedFunction):
		return errs.InvalidInput(errs.CodeCaptureUnsupportedFunction, err.Error(), err)
	}
	return errs.Output(errs.CodeCaptureLoadFailed, err)
}
