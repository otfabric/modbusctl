package types

import (
	"fmt"
	"strings"
)

// RecordSummaryResult is the final stdout payload after recording (per-iteration progress on stderr).
type RecordSummaryResult struct {
	Target         string `json:"target"`
	BlocksRecorded int    `json:"blocks_recorded"`
	Iterations     uint32 `json:"iterations"`
	McapOutputPath string `json:"mcap_output_path"`
}

// MarshalTextOutput matches historical record summary lines.
func (r *RecordSummaryResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "📦 Total recorded blocks: %d\n", r.BlocksRecorded)
	_, _ = fmt.Fprintf(&b, "🔄 Total iterations: %d\n", r.Iterations)
	_, _ = fmt.Fprintf(&b, "✅ Output written to %s\n", r.McapOutputPath)
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *RecordSummaryResult) TableHeaders() []string {
	return []string{"field", "value"}
}

// TableRows implements format.TableMarshaler.
func (r *RecordSummaryResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	return [][]string{
		{"target", r.Target},
		{"blocks_recorded", fmt.Sprintf("%d", r.BlocksRecorded)},
		{"iterations", fmt.Sprintf("%d", r.Iterations)},
		{"mcap_output_path", r.McapOutputPath},
	}
}
