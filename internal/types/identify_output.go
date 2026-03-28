package types

import (
	"fmt"
	"strings"
)

// IdentifyResult is the aggregate FC43 (and optional FC17) identification output for one connection target.
type IdentifyResult struct {
	Target  string               `json:"target"`
	Summary *ResultSummary       `json:"summary,omitempty"`
	Units   []IdentifyUnitResult `json:"units"`
}

// IdentifyUnitResult is one unit’s identification outcome (success payload or per-unit error).
type IdentifyUnitResult struct {
	UnitID uint8      `json:"unit_id"`
	Error  *ErrorInfo `json:"error,omitempty"`

	Category        *uint8 `json:"category,omitempty"`
	ConformityLevel *uint8 `json:"conformity_level,omitempty"`
	MoreFollows     *bool  `json:"more_follows,omitempty"`
	NextObjectID    *uint8 `json:"next_object_id,omitempty"`

	Objects []IdentifyObjectRow `json:"objects,omitempty"`

	ReportServerID *IdentifyReportServerOutput `json:"report_server_id,omitempty"`
}

// IdentifyObjectRow is one device identification object (FC43).
type IdentifyObjectRow struct {
	ID          uint8  `json:"id"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// IdentifyReportServerOutput is FC17 Report Server ID payload when requested.
type IdentifyReportServerOutput struct {
	Error          *ErrorInfo `json:"error,omitempty"`
	DataHex        string     `json:"data_hex,omitempty"`
	RunIndicatorOn *bool      `json:"run_indicator_on,omitempty"`
}

// MarshalTextOutput preserves the historical human-readable identify layout.
func (r *IdentifyResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "🔍 Connecting to %s...\n", r.Target)
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
		if u.Category != nil && u.ConformityLevel != nil && u.MoreFollows != nil && u.NextObjectID != nil {
			moreStr := "false"
			if *u.MoreFollows {
				moreStr = "true"
			}
			_, _ = fmt.Fprintf(&b, "✅ Device Identification (Category: %d, Conformity Level: 0x%02X, More Follows: %s, Next Object ID: %d, Object Count: %d)\n",
				*u.Category, *u.ConformityLevel, moreStr, *u.NextObjectID, len(u.Objects))
		}
		for _, obj := range u.Objects {
			if obj.Description != "" {
				_, _ = fmt.Fprintf(&b, " - Object %d: %s [%s]\n", obj.ID, obj.Value, obj.Description)
			} else {
				_, _ = fmt.Fprintf(&b, " - Object %d: %s\n", obj.ID, obj.Value)
			}
		}
		if rs := u.ReportServerID; rs != nil {
			if msg := ErrorMessage(rs.Error); msg != "" {
				_, _ = fmt.Fprintf(&b, "  FC17 Report Server ID: ⚠️ %s\n", msg)
			} else if rs.DataHex != "" {
				_, _ = fmt.Fprintf(&b, "  FC17 Report Server ID: %s\n", rs.DataHex)
			}
			if rs.RunIndicatorOn != nil && ErrorMessage(rs.Error) == "" {
				status := "OFF"
				if *rs.RunIndicatorOn {
					status = "ON"
				}
				_, _ = fmt.Fprintf(&b, "  Run Indicator: %s\n", status)
			}
		}
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler (object grid; per-unit errors as rows).
func (r *IdentifyResult) TableHeaders() []string {
	return []string{"unit_id", "object_id", "description", "value"}
}

// TableRows flattens objects; failed units appear as a single row with the error in value.
func (r *IdentifyResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	var rows [][]string
	for _, u := range r.Units {
		if msg := ErrorMessage(u.Error); msg != "" {
			rows = append(rows, []string{
				fmt.Sprintf("%d", u.UnitID),
				"-",
				"error",
				msg,
			})
			continue
		}
		if len(u.Objects) == 0 {
			rows = append(rows, []string{
				fmt.Sprintf("%d", u.UnitID),
				"-",
				"(no objects)",
				"",
			})
			continue
		}
		for _, obj := range u.Objects {
			rows = append(rows, []string{
				fmt.Sprintf("%d", u.UnitID),
				fmt.Sprintf("%d", obj.ID),
				obj.Description,
				obj.Value,
			})
		}
	}
	return rows
}
