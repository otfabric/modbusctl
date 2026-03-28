package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/types"
)

// RenderFatal renders a fatal error to stdout/stderr and returns the process exit code (no os.Exit).
func RenderFatal(inv *Invocation, err error, stdout, stderr io.Writer, debug bool) int {
	code := fatalExitCode(err)
	if inv != nil && inv.HasOutputFormat && inv.OutputFormat == format.FormatJSON {
		if err := writeFatalJSON(stdout, stderr, err, debug); err != nil {
			_, _ = fmt.Fprintf(stderr, "error: failed to render JSON fatal: %v\n", err)
			return errs.ExitInternal
		}
		return code
	}
	writeFatalText(stderr, err, debug)
	return code
}

// RenderFatalAndExit renders a fatal error and terminates the process.
func RenderFatalAndExit(inv *Invocation, err error, stdout, stderr io.Writer, debug bool) {
	os.Exit(RenderFatal(inv, err, stdout, stderr, debug))
}

func fatalExitCode(err error) int {
	if e, ok := errs.As(err); ok {
		return errs.ExitCodeForKind(e.Kind)
	}
	return errs.ExitInternal
}

func writeFatalText(w io.Writer, err error, debug bool) {
	info := errorInfoFromErr(err)
	_, _ = fmt.Fprintf(w, "error: %s\n", info.Message)
	if debug {
		writeCauseChain(w, err)
	}
}

func writeFatalJSON(stdout, stderr io.Writer, err error, debug bool) error {
	env := types.ErrorEnvelope{Error: errorInfoFromErr(err)}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if jerr := enc.Encode(env); jerr != nil {
		_, _ = fmt.Fprintf(stderr, "error: failed to encode error envelope: %v\n", jerr)
		return jerr
	}
	if _, werr := stdout.Write(buf.Bytes()); werr != nil {
		_, _ = fmt.Fprintf(stderr, "error: write stdout: %v\n", werr)
		return werr
	}
	if debug {
		writeCauseChain(stderr, err)
	}
	return nil
}

func errorInfoFromErr(err error) types.ErrorInfo {
	if e, ok := errs.As(err); ok {
		msg := e.Message
		if msg == "" && e.Cause != nil {
			msg = e.Cause.Error()
		}
		if msg == "" {
			msg = err.Error()
		}
		return types.ErrorInfo{
			Kind:    string(e.Kind),
			Code:    e.Code,
			Message: msg,
		}
	}
	return types.ErrorInfo{
		Kind:    string(errs.KindInternal),
		Code:    errs.CodeInternalUnexpected,
		Message: err.Error(),
	}
}

func writeCauseChain(w io.Writer, err error) {
	for c := errors.Unwrap(err); c != nil; c = errors.Unwrap(c) {
		_, _ = fmt.Fprintf(w, "cause: %v\n", c)
	}
}
