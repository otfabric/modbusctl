// SPDX-License-Identifier: MIT

// Package fsutil opens the files an operator names on the command line:
// inputs read with a size limit, and outputs created owner-only, never
// through a symbolic link.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// OutputMode is the mode of the files modbusctl creates (captures, exports,
// discovery results): owner only, as captures can carry plant data.
const OutputMode = 0o600

// ErrOutputLink refuses an output path that is a symbolic link: writing
// through it would truncate whatever file the link points to.
var ErrOutputLink = errors.New("the output is a symbolic link; modbusctl does not write through links (name the target itself)")

// ErrSameFile refuses an output that names an input of the same command:
// creating the output would truncate the input before it is read.
var ErrSameFile = errors.New("the output names an input file of this command; choose another output path")

// ErrInputTooLarge is returned by [ReadInput] for a file above its limit.
var ErrInputTooLarge = errors.New("input file is too large")

// MaxJSONInput is the size limit of the JSON inputs (blocks files, device
// profiles): far above any real one, low enough not to exhaust memory.
const MaxJSONInput = 16 << 20

// CreateOutput creates or truncates the file an operator named, owner-only
// for a new file (an existing regular file keeps its mode). A symbolic link
// is refused ([ErrOutputLink]); one that appears between the check and the
// open fails the open (O_NOFOLLOW, where the platform has it).
func CreateOutput(path string) (*os.File, error) {
	path = filepath.Clean(path)
	if IsLink(path) {
		return nil, fmt.Errorf("%s: %w", path, ErrOutputLink)
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|oNoFollow, OutputMode)
}

// WriteOutput writes data to path as [CreateOutput] creates it.
func WriteOutput(path string, data []byte) error {
	f, err := CreateOutput(path)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Close())
}

// IsLink reports that path exists as a symbolic link.
func IsLink(path string) bool {
	st, err := os.Lstat(filepath.Clean(path))
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

// SameFile reports that output names one of the existing inputs (through a
// link or another spelling of the path included).
func SameFile(output string, inputs ...string) bool {
	if output == "" {
		return false
	}
	so, err := os.Stat(filepath.Clean(output))
	if err != nil {
		return false
	}
	for _, in := range inputs {
		if in == "" {
			continue
		}
		if si, err := os.Stat(filepath.Clean(in)); err == nil && os.SameFile(so, si) {
			return true
		}
	}
	return false
}

// OpenInput opens an input file an operator named.
func OpenInput(path string) (*os.File, error) {
	return os.Open(filepath.Clean(path))
}

// ReadInput reads the input file at path, refusing one above limit bytes
// ([ErrInputTooLarge]) without reading more than that.
func ReadInput(path string, limit int64) ([]byte, error) {
	f, err := OpenInput(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: %w (more than %d bytes)", path, ErrInputTooLarge, limit)
	}
	return data, nil
}
