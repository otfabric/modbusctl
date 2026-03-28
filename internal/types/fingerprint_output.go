package types

import (
	"fmt"
	"strings"
)

// FingerprintResult lists supported read function codes per unit.
type FingerprintResult struct {
	Target  string                  `json:"target"`
	Summary *ResultSummary          `json:"summary,omitempty"`
	Units   []FingerprintUnitResult `json:"units"`
}

// FingerprintUnitResult is one unit’s fingerprint outcome.
type FingerprintUnitResult struct {
	UnitID           uint8      `json:"unit_id"`
	Error            *ErrorInfo `json:"error,omitempty"`
	SupportedReads   []string   `json:"supported_read_functions,omitempty"`
	ProbeInterrupted bool       `json:"probe_interrupted,omitempty"`
}

// FingerprintUnitPartial is true when at least one read FC was confirmed before a later probe error stopped the run.
func FingerprintUnitPartial(u FingerprintUnitResult) bool {
	return u.ProbeInterrupted && len(u.SupportedReads) > 0
}

// FingerprintUnitUnusable is true when there is no supported-function list to show (hard failure or interrupted before any success).
func FingerprintUnitUnusable(u FingerprintUnitResult) bool {
	if len(u.SupportedReads) > 0 {
		return false
	}
	return ErrorMessage(u.Error) != "" || u.ProbeInterrupted
}

// FingerprintUnitFailed is an alias for [FingerprintUnitUnusable] (summary and “no data” rows).
func FingerprintUnitFailed(u FingerprintUnitResult) bool {
	return FingerprintUnitUnusable(u)
}

// MarshalTextOutput matches historical fingerprint layout.
func (r *FingerprintResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "🔍 Fingerprinting device at %s (supported read functions per unit)...\n", r.Target)
	if sum := r.Summary; sum != nil {
		_, _ = fmt.Fprintf(&b, "Units requested: %d  succeeded: %d  failed: %d\n\n", sum.Requested, sum.Succeeded, sum.Failed)
	}
	for _, u := range r.Units {
		if len(r.Units) > 1 {
			_, _ = fmt.Fprintf(&b, "\n--- Unit ID %d ---\n", u.UnitID)
		}
		if FingerprintUnitPartial(u) {
			msg := ErrorMessage(u.Error)
			if msg == "" {
				msg = "probe interrupted after partial results"
			}
			_, _ = fmt.Fprintf(&b, "⚠️ Unit %d: %s\n", u.UnitID, msg)
			_, _ = fmt.Fprintf(&b, "✅ Unit %d: supported read functions (partial):\n", u.UnitID)
			for _, s := range u.SupportedReads {
				_, _ = fmt.Fprintf(&b, "  %s\n", s)
			}
			continue
		}
		if FingerprintUnitUnusable(u) {
			msg := ErrorMessage(u.Error)
			if msg == "" && u.ProbeInterrupted {
				msg = "fingerprint probe interrupted before any supported function was confirmed"
			}
			if msg != "" {
				_, _ = fmt.Fprintf(&b, "⚠️ Unit %d: %s\n", u.UnitID, msg)
				continue
			}
			_, _ = fmt.Fprintf(&b, "⚠️ Unit %d: probe failed\n", u.UnitID)
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
		if FingerprintUnitPartial(u) {
			msg := ErrorMessage(u.Error)
			if msg == "" {
				msg = "partial (probe interrupted)"
			}
			rows = append(rows, []string{fmt.Sprintf("%d", u.UnitID), "", msg})
			for _, fn := range u.SupportedReads {
				rows = append(rows, []string{fmt.Sprintf("%d", u.UnitID), fn, ""})
			}
			continue
		}
		if FingerprintUnitUnusable(u) {
			msg := ErrorMessage(u.Error)
			if msg == "" && u.ProbeInterrupted {
				msg = "interrupted before any supported function"
			}
			if msg == "" {
				msg = "probe failed"
			}
			rows = append(rows, []string{fmt.Sprintf("%d", u.UnitID), "", msg})
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
