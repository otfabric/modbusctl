package capfile

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/go-modbus-capture/jsonl"
	"github.com/otfabric/go-modbus-capture/mbcap"
	"github.com/otfabric/go-modbus-capture/mbcsv"
)

// ErrLegacyFormat is returned for a modbusctl 2.x capture file (.mcap), which
// modbusctl 3 does not read.
var ErrLegacyFormat = errors.New("this is a modbusctl 2.x capture file (.mcap), which modbusctl 3 does not read; record the device again")

// Encoding is a capture file encoding.
type Encoding string

// Encodings.
const (
	Binary Encoding = "mbcap"
	JSONL  Encoding = "jsonl"
	CSV    Encoding = "csv"
)

// OpenReader detects the encoding of r and returns a capture reader for it.
func OpenReader(r io.Reader) (capture.Reader, Encoding, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	head, _ := br.Peek(len(mbcap.Magic))
	switch {
	case string(head) == mbcap.Magic:
		cr, err := mbcap.NewReader(br)
		return cr, Binary, err
	case bytes.HasPrefix(head, []byte("MCAP")):
		return nil, "", ErrLegacyFormat
	}
	head, _ = br.Peek(64)
	head = bytes.TrimLeft(head, " \t\r\n")
	switch {
	case bytes.HasPrefix(head, []byte("{")):
		cr, err := jsonl.NewReader(br)
		return cr, JSONL, err
	case bytes.HasPrefix(head, []byte("t,")) || bytes.Contains(head, []byte(",t,")):
		cr, err := mbcsv.NewReader(br)
		return cr, CSV, err
	}
	return nil, "", fmt.Errorf("%w: not an MBCAP capture (.mbcap, .mbcap.jsonl or .mbcap.csv)", capture.ErrCorrupt)
}

// Loaded is a capture read into memory.
type Loaded struct {
	*capture.Capture
	Encoding Encoding
	// Truncated is true when the file ends early (for example a recording
	// that was interrupted): the capture holds what was written up to there
	// and its end is unknown.
	Truncated bool
}

// Load reads a whole capture file. A truncated file is not an error: Loaded
// says so. A capture holding a sample outside the supported read functions
// (see [IsSpecRead]) is rejected with [ErrUnsupportedFunction].
func Load(path string) (*Loaded, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r, enc, err := OpenReader(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c, err := capture.ReadAll(r)
	l := &Loaded{Capture: c, Encoding: enc}
	if errors.Is(err, capture.ErrTruncated) {
		l.Truncated = true
		err = nil
	}
	if err == nil && c != nil {
		err = checkFunctions(c)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

// End returns the capture end, or the last sample time when the end is
// unknown.
func (l *Loaded) End() capture.Ticks {
	if l.Capture.End.Known {
		return l.Capture.End.At
	}
	var last capture.Ticks
	for _, s := range l.Samples {
		last = max(last, s.At)
	}
	return last
}

// Target returns the target with id, if there is one.
func (l *Loaded) Target(id uint32) (capture.Target, bool) {
	for _, t := range l.Targets {
		if t.ID == id {
			return t, true
		}
	}
	return capture.Target{}, false
}

// TargetOf maps stream ids to their target ids.
func (l *Loaded) TargetOf() map[uint32]uint32 {
	m := make(map[uint32]uint32, len(l.Streams))
	for _, s := range l.Streams {
		m[s.ID] = s.Target
	}
	return m
}

// ListTargets describes the targets, for error messages.
func (l *Loaded) ListTargets() string {
	parts := make([]string, 0, len(l.Targets))
	for _, t := range l.Targets {
		label := ""
		if t.Label != "" {
			label = fmt.Sprintf(" %q", t.Label)
		}
		parts = append(parts, fmt.Sprintf("%d%s (unit %d)", t.ID, label, t.Unit))
	}
	return strings.Join(parts, ", ")
}
