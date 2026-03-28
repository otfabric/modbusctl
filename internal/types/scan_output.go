package types

import (
	"fmt"
	"strings"
)

// ScanSummaryResult is the final stdout payload after a scan (progress goes to stderr).
type ScanSummaryResult struct {
	Target string `json:"target"`
	// Summary counts Modbus read requests (not devices/units); Requested/Succeeded/Failed are per-request.
	Summary             *ResultSummary `json:"summary,omitempty"`
	Algo                string         `json:"algo"`
	TotalRequests       int            `json:"total_requests"`
	SuccessCount        int            `json:"success_count"`
	FailCount           int            `json:"fail_count"`
	ExceptionCount      int            `json:"exception_count"`
	TimeoutCount        int            `json:"timeout_count"`
	TransportErrorCount int            `json:"transport_error_count"`
	BlocksCaptured      int            `json:"blocks_captured"`
	RegistersCaptured   int            `json:"registers_captured"`
	AvgResponseMs       int64          `json:"avg_response_ms,omitempty"`
	Duration            string         `json:"duration"`
	McapOutputPath      string         `json:"mcap_output_path"`
}

// MarshalTextOutput prints the historical scan summary block.
func (r *ScanSummaryResult) MarshalTextOutput() (string, error) {
	if r == nil {
		return "", nil
	}
	var b strings.Builder
	if sum := r.Summary; sum != nil {
		_, _ = fmt.Fprintf(&b, "Scan requests: %d  succeeded: %d  failed: %d\n", sum.Requested, sum.Succeeded, sum.Failed)
	}
	_, _ = fmt.Fprintf(&b, "Algo: %s\n", r.Algo)
	_, _ = fmt.Fprintf(&b, "Requests: %d\n", r.TotalRequests)
	_, _ = fmt.Fprintf(&b, "Success: %d\n", r.SuccessCount)
	_, _ = fmt.Fprintf(&b, "Failed: %d\n", r.FailCount)
	if r.ExceptionCount > 0 || r.TimeoutCount > 0 || r.TransportErrorCount > 0 {
		_, _ = fmt.Fprintf(&b, "  Exceptions: %d  Timeouts: %d  Transport errors: %d\n", r.ExceptionCount, r.TimeoutCount, r.TransportErrorCount)
	}
	_, _ = fmt.Fprintf(&b, "Blocks captured: %d\n", r.BlocksCaptured)
	_, _ = fmt.Fprintf(&b, "Registers captured: %d\n", r.RegistersCaptured)
	if r.SuccessCount > 0 {
		_, _ = fmt.Fprintf(&b, "Avg response time: %d ms\n", r.AvgResponseMs)
	}
	_, _ = fmt.Fprintf(&b, "Duration: %s\n", r.Duration)
	_, _ = fmt.Fprintf(&b, "Output: %s\n", r.McapOutputPath)
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ScanSummaryResult) TableHeaders() []string {
	return []string{"metric", "value"}
}

// TableRows implements format.TableMarshaler.
func (r *ScanSummaryResult) TableRows() [][]string {
	if r == nil {
		return nil
	}
	return [][]string{
		{"target", r.Target},
		{"algo", r.Algo},
		{"total_requests", fmt.Sprintf("%d", r.TotalRequests)},
		{"success_count", fmt.Sprintf("%d", r.SuccessCount)},
		{"fail_count", fmt.Sprintf("%d", r.FailCount)},
		{"exception_count", fmt.Sprintf("%d", r.ExceptionCount)},
		{"timeout_count", fmt.Sprintf("%d", r.TimeoutCount)},
		{"transport_error_count", fmt.Sprintf("%d", r.TransportErrorCount)},
		{"blocks_captured", fmt.Sprintf("%d", r.BlocksCaptured)},
		{"registers_captured", fmt.Sprintf("%d", r.RegistersCaptured)},
		{"avg_response_ms", fmt.Sprintf("%d", r.AvgResponseMs)},
		{"duration", r.Duration},
		{"mcap_output_path", r.McapOutputPath},
	}
}
