package types

import (
	"fmt"
	"strings"
)

// DiagnosticResult is the FC08 diagnostics response payload.
type DiagnosticResult struct {
	Target         string `json:"target"`
	UnitID         uint8  `json:"unit_id"`
	SubFunction    string `json:"sub_function"`
	SubFunctionHex uint16 `json:"sub_function_hex"`
	DataHex        string `json:"data_hex"`
}

// MarshalTextOutput matches historical diagnostic layout.
func (r *DiagnosticResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "🔍 Sending FC08 Diagnostics to %s (unit %d, sub-function %s / 0x%04X)...\n",
		r.Target, r.UnitID, r.SubFunction, r.SubFunctionHex)
	_, _ = fmt.Fprintf(&b, "✅ Diagnostics response:\n")
	_, _ = fmt.Fprintf(&b, "  Sub-function: 0x%04X (%s)\n", r.SubFunctionHex, r.SubFunction)
	_, _ = fmt.Fprintf(&b, "  Data:         %s\n", r.DataHex)
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *DiagnosticResult) TableHeaders() []string {
	return []string{"field", "value"}
}

// TableRows implements format.TableMarshaler.
func (r *DiagnosticResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	return [][]string{
		{"target", r.Target},
		{"unit_id", fmt.Sprintf("%d", r.UnitID)},
		{"sub_function", r.SubFunction},
		{"sub_function_hex", fmt.Sprintf("0x%04X", r.SubFunctionHex)},
		{"data_hex", r.DataHex},
	}
}
