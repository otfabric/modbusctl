// Package types defines read command output. RawDataHex is always the canonical post-read byte
// sequence written to MCAP (after optional byte swap). AsciiDecoded is a human view of those
// same bytes when --ascii is set, not an alternate acquisition path.
package types

import (
	"fmt"
	"strings"
)

// ReadResult is the structured outcome of a single client read (plus MCAP side effect).
type ReadResult struct {
	Target         string `json:"target"`
	UnitID         uint8  `json:"unit_id"`
	Function       uint8  `json:"function"`
	StartAddress   uint16 `json:"start_address"`
	RegisterCount  uint16 `json:"register_count"`
	RawByteCount   int    `json:"raw_byte_count"`
	PreSwapHex     string `json:"pre_swap_hex,omitempty"`
	RawDataHex     string `json:"raw_data_hex"`
	BytesSwapped   bool   `json:"bytes_swapped"`
	AsciiDecoded   string `json:"ascii_decoded,omitempty"`
	McapOutputPath string `json:"mcap_output_path"`
}

// MarshalTextOutput matches historical read stdout (single coherent view of ReadResult fields).
func (r *ReadResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Reading %d registers starting from address %d using function code %d\n",
		r.RegisterCount, r.StartAddress, r.Function)
	if r.BytesSwapped && r.PreSwapHex != "" {
		_, _ = fmt.Fprintf(&b, "Read %d bytes of data: %s\n", r.RawByteCount, r.PreSwapHex)
		_, _ = fmt.Fprintf(&b, "🔁 Byte-swapped data: %s\n", r.RawDataHex)
	} else {
		_, _ = fmt.Fprintf(&b, "Read %d bytes of data: %s\n", r.RawByteCount, r.RawDataHex)
	}
	if r.AsciiDecoded != "" {
		_, _ = fmt.Fprintf(&b, "Decoding data as ASCII:\n")
		_, _ = fmt.Fprintf(&b, "ASCII: %s\n", r.AsciiDecoded)
	}
	_, _ = fmt.Fprintf(&b, "✅ Output written to %s\n", r.McapOutputPath)
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
		{"pre_swap_hex", r.PreSwapHex},
		{"raw_data_hex", r.RawDataHex},
		{"bytes_swapped", fmt.Sprintf("%v", r.BytesSwapped)},
	}
	if r.AsciiDecoded != "" {
		rows = append(rows, []string{"ascii_decoded", r.AsciiDecoded})
	}
	rows = append(rows, []string{"mcap_output_path", r.McapOutputPath})
	return rows
}
