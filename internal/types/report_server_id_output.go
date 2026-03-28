package types

import (
	"fmt"
	"strings"
)

// ReportServerIDResult is FC17 aggregate output for one or more units.
type ReportServerIDResult struct {
	Target  string                     `json:"target"`
	Summary *ResultSummary             `json:"summary,omitempty"`
	Units   []ReportServerIDUnitResult `json:"units"`
}

// ReportServerIDUnitResult is FC17 outcome for one unit.
type ReportServerIDUnitResult struct {
	UnitID         uint8      `json:"unit_id"`
	Error          *ErrorInfo `json:"error,omitempty"`
	DataHex        string     `json:"data_hex,omitempty"`
	RunIndicatorOn *bool      `json:"run_indicator_on,omitempty"`
}

// MarshalTextOutput matches historical reportserverid text layout.
func (r *ReportServerIDResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "🔍 Sending FC17 Report Server ID to %s...\n", r.Target)
	if sum := r.Summary; sum != nil {
		_, _ = fmt.Fprintf(&b, "Units requested: %d  succeeded: %d  failed: %d\n\n", sum.Requested, sum.Succeeded, sum.Failed)
	}
	for _, u := range r.Units {
		if len(r.Units) > 1 {
			_, _ = fmt.Fprintf(&b, "\n--- Unit ID %d ---\n", u.UnitID)
		}
		if msg := ErrorMessage(u.Error); msg != "" {
			_, _ = fmt.Fprintf(&b, "⚠️ Unit %d: %s\n", u.UnitID, msg)
			continue
		}
		_, _ = fmt.Fprintf(&b, "✅ Report Server ID (unit %d):\n", u.UnitID)
		if u.DataHex != "" {
			_, _ = fmt.Fprintf(&b, "  Data: %s\n", u.DataHex)
		}
		if u.RunIndicatorOn != nil {
			status := "OFF"
			if *u.RunIndicatorOn {
				status = "ON"
			}
			_, _ = fmt.Fprintf(&b, "  Run Indicator: %s\n", status)
		}
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ReportServerIDResult) TableHeaders() []string {
	return []string{"unit_id", "data_hex", "run_indicator", "error"}
}

// TableRows implements format.TableMarshaler.
func (r *ReportServerIDResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	var rows [][]string
	for _, u := range r.Units {
		ri := ""
		if u.RunIndicatorOn != nil {
			if *u.RunIndicatorOn {
				ri = "ON"
			} else {
				ri = "OFF"
			}
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", u.UnitID),
			u.DataHex,
			ri,
			ErrorMessage(u.Error),
		})
	}
	return rows
}
