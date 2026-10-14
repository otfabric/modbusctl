package mcap

import (
	"fmt"
	"io"
	"os"

	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/go-modbus-capture/jsonl"
	"github.com/otfabric/go-modbus-capture/mbcap"
	"github.com/otfabric/go-modbus-capture/mbcsv"
	"github.com/otfabric/modbusctl/internal/capfile"
)

// Convert writes the capture at path to w in format: mbcap, jsonl or csv
// (lossless), or wide-csv (a one-way view of raw values per address).
func Convert(w io.Writer, path, format string) error {
	if format == "wide-csv" {
		l, err := capfile.Load(path)
		if err != nil {
			return err
		}
		_, err = mbcsv.WriteWide(w, l.Capture)
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	r, _, err := capfile.OpenReader(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var cw capture.Writer
	switch format {
	case "mbcap":
		cw, err = mbcap.NewWriter(w, r.Header(), nil)
	case "jsonl":
		cw, err = jsonl.NewWriter(w, r.Header())
	case "csv":
		cw, err = mbcsv.NewWriter(w, r.Header())
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
	if err != nil {
		return err
	}
	if err := capture.Copy(cw, r); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Verify reads the whole capture at path, checking its structure and every
// rule of the format, and writes one line about it.
func Verify(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	r, enc, err := capfile.OpenReader(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	c, err := capture.ReadAll(r)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	end := "end not recorded"
	if c.End.Known {
		end = "end at " + c.Header.Duration(c.End.At).String()
	}
	_, _ = fmt.Fprintf(w, "%s: ok (%s, %d targets, %d streams, %d samples, %s)\n", path, enc, len(c.Targets), len(c.Streams), len(c.Samples), end)
	return nil
}
