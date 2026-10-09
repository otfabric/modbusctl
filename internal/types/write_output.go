package types

import (
	"fmt"
	"strings"
)

// WriteResult is the structured outcome of a `client write`.
// Exactly one of Coils and Registers is set, depending on the function code.
type WriteResult struct {
	Target       string   `json:"target"`
	UnitID       uint8    `json:"unit_id"`
	Function     uint8    `json:"function"`
	StartAddress uint16   `json:"start_address"`
	Count        int      `json:"count"`
	Codec        string   `json:"codec,omitempty"`
	Coils        []bool   `json:"coils,omitempty"`
	Registers    []uint16 `json:"registers,omitempty"`
	RawDataHex   string   `json:"raw_data_hex,omitempty"`
}

func (r *WriteResult) kind() string {
	if r.Function == 5 || r.Function == 15 {
		return "coil"
	}
	return "register"
}

// writtenValues returns one display string per written coil or register.
func (r *WriteResult) writtenValues() []string {
	out := make([]string, 0, r.Count)
	for _, c := range r.Coils {
		if c {
			out = append(out, "1")
		} else {
			out = append(out, "0")
		}
	}
	for _, v := range r.Registers {
		out = append(out, fmt.Sprintf("%d (0x%04X)", v, v))
	}
	return out
}

// MarshalTextOutput renders a human-readable write confirmation.
func (r *WriteResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Writing %d %s(s) starting at address %d using function code %d\n",
		r.Count, r.kind(), r.StartAddress, r.Function)
	if r.Codec != "" {
		_, _ = fmt.Fprintf(&b, "Codec: %s\n", r.Codec)
	}
	for i, v := range r.writtenValues() {
		_, _ = fmt.Fprintf(&b, "  [%d] %s\n", int(r.StartAddress)+i, v)
	}
	_, _ = fmt.Fprintf(&b, "✅ Wrote %d %s(s) to unit %d\n", r.Count, r.kind(), r.UnitID)
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *WriteResult) TableHeaders() []string {
	return []string{"address", "type", "value"}
}

// TableRows implements format.TableMarshaler.
func (r *WriteResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	vals := r.writtenValues()
	rows := make([][]string, 0, len(vals))
	for i, v := range vals {
		rows = append(rows, []string{fmt.Sprintf("%d", int(r.StartAddress)+i), r.kind(), v})
	}
	return rows
}
