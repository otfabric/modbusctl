// SPDX-License-Identifier: MIT

package cli

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/fsutil"
)

// OpenStdoutOrFile returns os.Stdout when path is empty/whitespace; otherwise creates path for
// write with [fsutil.CreateOutput] (owner-only, never through a symbolic link) and writes to it
// through a buffer. An output that names one of inputs is refused with [fsutil.ErrSameFile]
// before anything is truncated. finish must be called when non-nil, with the command's error
// or nil, and its result returned: for a file it is as [bufferedOutput] says, for stdout the
// error unchanged.
func OpenStdoutOrFile(path string, inputs ...string) (w io.Writer, finish func(error) error, err error) {
	if strings.TrimSpace(path) == "" {
		return os.Stdout, func(err error) error { return err }, nil
	}
	if fsutil.SameFile(path, inputs...) {
		return nil, nil, fsutil.ErrSameFile
	}
	f, err := fsutil.CreateOutput(path)
	if err != nil {
		return nil, nil, err
	}
	w, finish = bufferedOutput(f)
	return w, finish, nil
}

// bufferedOutput writes to f through a buffer; finish flushes the buffer and closes f. A write
// or a flush that failed is returned as an output error (output_file_write_failed, exit 10),
// also in place of an error of the command: the capture commands write after they have read
// their input, so such an error comes from the write. A close that fails is returned the same
// way when the command had no error. Otherwise finish returns err.
func bufferedOutput(f io.WriteCloser) (io.Writer, func(error) error) {
	bw := bufio.NewWriter(f)
	return bw, func(err error) error {
		if ferr := bw.Flush(); ferr != nil {
			_ = f.Close()
			return errs.Output(errs.CodeOutputFileWriteFailed, ferr)
		}
		if cerr := f.Close(); cerr != nil && err == nil {
			return errs.Output(errs.CodeOutputFileWriteFailed, cerr)
		}
		return err
	}
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
