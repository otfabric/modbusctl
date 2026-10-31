// SPDX-License-Identifier: MIT

// Package types defines read command output. RawDataHex is the canonical post-read byte
// sequence written to the capture file. Typed decoding of these bytes is handled by `client decode`.
package types

import (
	"fmt"
	"strings"
)

// ReadResult is the structured outcome of a single client read (plus the capture file it writes).
type ReadResult struct {
	Target            string `json:"target"`
	UnitID            uint8  `json:"unit_id"`
	Function          uint8  `json:"function"`
	StartAddress      uint16 `json:"start_address"`
	RegisterCount     uint16 `json:"register_count"`
	RawByteCount      int    `json:"raw_byte_count"`
	RawDataHex        string `json:"raw_data_hex"`
	CaptureOutputPath string `json:"capture_output_path"`
}

// MarshalTextOutput matches historical read stdout (single coherent view of ReadResult fields).
func (r *ReadResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Reading %d registers starting from address %d using function code %d\n",
		r.RegisterCount, r.StartAddress, r.Function)
	_, _ = fmt.Fprintf(&b, "Read %d bytes of data: %s\n", r.RawByteCount, r.RawDataHex)
	_, _ = fmt.Fprintf(&b, "✅ Output written to %s\n", r.CaptureOutputPath)
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ReadResult) TableHeaders() []string {
	return []string{"field", "value"}
}

// TableRows implements format.TableMarshaler.
func (r *ReadResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	rows := [][]string{
		{"target", r.Target},
		{"unit_id", fmt.Sprintf("%d", r.UnitID)},
		{"function", fmt.Sprintf("%d", r.Function)},
		{"start_address", fmt.Sprintf("%d", r.StartAddress)},
		{"register_count", fmt.Sprintf("%d", r.RegisterCount)},
		{"raw_byte_count", fmt.Sprintf("%d", r.RawByteCount)},
		{"raw_data_hex", r.RawDataHex},
		{"capture_output_path", r.CaptureOutputPath},
	}
	return rows
}
