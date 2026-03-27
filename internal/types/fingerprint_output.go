package types

import (
	"fmt"
	"strings"
)

// FingerprintResult lists supported read function codes per unit.
type FingerprintResult struct {
	Target string                  `json:"target"`
	Units  []FingerprintUnitResult `json:"units"`
}

// FingerprintUnitResult is one unit’s fingerprint outcome.
type FingerprintUnitResult struct {
	UnitID           uint8    `json:"unit_id"`
	Error            string   `json:"error,omitempty"`
	SupportedReads   []string `json:"supported_read_functions,omitempty"`
	ProbeInterrupted bool     `json:"probe_interrupted,omitempty"`
}

// MarshalTextOutput matches historical fingerprint layout.
func (r *FingerprintResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "🔍 Fingerprinting device at %s (supported read functions per unit)...\n", r.Target)
	for _, u := range r.Units {
		if len(r.Units) > 1 {
			_, _ = fmt.Fprintf(&b, "\n--- Unit ID %d ---\n", u.UnitID)
		}
		if u.Error != "" {
			_, _ = fmt.Fprintf(&b, "⚠️ Unit %d: %s\n", u.UnitID, u.Error)
			continue
		}
		if len(u.SupportedReads) > 0 {
			_, _ = fmt.Fprintf(&b, "✅ Unit %d: supported read functions:\n", u.UnitID)
			for _, s := range u.SupportedReads {
				_, _ = fmt.Fprintf(&b, "  %s\n", s)
			}
		} else if len(r.Units) == 1 {
			_, _ = fmt.Fprintf(&b, "— No supported read functions detected for unit %d\n", u.UnitID)
		}
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *FingerprintResult) TableHeaders() []string {
	return []string{"unit_id", "function", "error"}
}

// TableRows implements format.TableMarshaler (one row per supported function; errors as single row).
func (r *FingerprintResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	var rows [][]string
	for _, u := range r.Units {
		if u.Error != "" {
			rows = append(rows, []string{fmt.Sprintf("%d", u.UnitID), "", u.Error})
			continue
		}
		if len(u.SupportedReads) == 0 {
			rows = append(rows, []string{fmt.Sprintf("%d", u.UnitID), "(none)", ""})
			continue
		}
		for _, fn := range u.SupportedReads {
			rows = append(rows, []string{fmt.Sprintf("%d", u.UnitID), fn, ""})
		}
	}
	return rows
}
