// SPDX-License-Identifier: MIT

// Package types decode output. DecodeResult is the structured outcome of a `client decode`:
// raw register bytes plus one or more values decoded with a single codec. CodecList backs
// the `client codecs` discovery command.
package types

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// DecodeResult is the structured outcome of a single codec-decoded read.
type DecodeResult struct {
	Target        string `json:"target"`
	UnitID        uint8  `json:"unit_id"`
	Function      uint8  `json:"function"`
	StartAddress  uint16 `json:"start_address"`
	RegisterCount uint16 `json:"register_count"`
	Codec         string `json:"codec"`
	Family        string `json:"family"`
	ValueKind     string `json:"value_kind"`
	RawDataHex    string `json:"raw_data_hex"`
	Values        []any  `json:"values"`
}

// MarshalTextOutput renders a human-readable decode summary.
func (r *DecodeResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Decoding %d register(s) from address %d using function code %d\n",
		r.RegisterCount, r.StartAddress, r.Function)
	_, _ = fmt.Fprintf(&b, "Codec: %s [family=%s valueKind=%s]\n", r.Codec, r.Family, r.ValueKind)
	_, _ = fmt.Fprintf(&b, "Raw data: %s\n", r.RawDataHex)
	switch len(r.Values) {
	case 0:
		_, _ = fmt.Fprintf(&b, "Decoded value: (none)\n")
	case 1:
		_, _ = fmt.Fprintf(&b, "Decoded value: %s\n", FormatDecodedValue(r.Values[0]))
	default:
		_, _ = fmt.Fprintf(&b, "Decoded %d values:\n", len(r.Values))
		for i, v := range r.Values {
			_, _ = fmt.Fprintf(&b, "  [%d] %s\n", i, FormatDecodedValue(v))
		}
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *DecodeResult) TableHeaders() []string {
	return []string{"field", "value"}
}

// TableRows implements format.TableMarshaler.
func (r *DecodeResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	rows := [][]string{
		{"target", r.Target},
		{"unit_id", fmt.Sprintf("%d", r.UnitID)},
		{"function", fmt.Sprintf("%d", r.Function)},
		{"start_address", fmt.Sprintf("%d", r.StartAddress)},
		{"register_count", fmt.Sprintf("%d", r.RegisterCount)},
		{"codec", r.Codec},
		{"family", r.Family},
		{"value_kind", r.ValueKind},
		{"raw_data_hex", r.RawDataHex},
	}
	if len(r.Values) == 1 {
		rows = append(rows, []string{"value", FormatDecodedValue(r.Values[0])})
	} else {
		for i, v := range r.Values {
			rows = append(rows, []string{fmt.Sprintf("value[%d]", i), FormatDecodedValue(v)})
		}
	}
	return rows
}

// FormatDecodedValue renders a decoded value for text/table output. Byte slices and
// hardware addresses are already normalized to strings by the decode engine; this
// handles the remaining native types (time, IP, numbers, strings).
func FormatDecodedValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case time.Time:
		return t.Format(time.RFC3339)
	case net.IP:
		return t.String()
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

// CodecInfo is a single codec entry for the `client codecs` listing.
type CodecInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Family    string `json:"family"`
	ValueKind string `json:"value_kind"`
	Registers uint16 `json:"registers"`
	Bytes     uint16 `json:"bytes"`
}

// CodecList is the structured outcome of `client codecs`.
type CodecList struct {
	Codecs []CodecInfo `json:"codecs"`
}

// MarshalTextOutput renders codecs as tab-separated rows (id, name, registers, bytes, family, valueKind).
func (l *CodecList) MarshalTextOutput() (string, error) {
	if l == nil || len(l.Codecs) == 0 {
		return "No codecs match the given filters.", nil
	}
	var b strings.Builder
	for _, c := range l.Codecs {
		_, _ = fmt.Fprintf(&b, "%s\t%s\tregisters=%d bytes=%d family=%s valueKind=%s\n",
			c.ID, c.Name, c.Registers, c.Bytes, c.Family, c.ValueKind)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (l *CodecList) TableHeaders() []string {
	return []string{"id", "name", "family", "value_kind", "registers", "bytes"}
}

// TableRows implements format.TableMarshaler.
func (l *CodecList) TableRows() [][]string {
	if l == nil {
		return nil
	}
	rows := make([][]string, 0, len(l.Codecs))
	for _, c := range l.Codecs {
		rows = append(rows, []string{
			c.ID,
			c.Name,
			c.Family,
			c.ValueKind,
			fmt.Sprintf("%d", c.Registers),
			fmt.Sprintf("%d", c.Bytes),
		})
	}
	return rows
}
