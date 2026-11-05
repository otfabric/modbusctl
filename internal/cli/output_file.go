// SPDX-License-Identifier: MIT

package cli

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/fsutil"
)

// OpenStdoutOrFile returns os.Stdout when path is empty/whitespace; otherwise creates path for
// write with [fsutil.CreateOutput] (owner-only, never through a symbolic link). An output that
// names one of inputs is refused with [fsutil.ErrSameFile] before anything is truncated.
// cleanup must be called when non-nil (no-op for stdout).
func OpenStdoutOrFile(path string, inputs ...string) (w io.Writer, cleanup func(), err error) {
	if strings.TrimSpace(path) == "" {
		return os.Stdout, func() {}, nil
	}
	if fsutil.SameFile(path, inputs...) {
		return nil, nil, fsutil.ErrSameFile
	}
	f, err := fsutil.CreateOutput(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

// OutputOpenError maps an [OpenStdoutOrFile] failure to the command's fatal error: an output
// naming an input is an invalid flag combination (exit 3), any other failure an output error
// (exit 10).
func OutputOpenError(err error) error {
	if errors.Is(err, fsutil.ErrSameFile) {
		return errs.InvalidInput(errs.CodeInvalidFlagCombination, err.Error(), err)
	}
	return errs.Output(errs.CodeOutputFileCreateFailed, err)
}
