package types

import "github.com/otfabric/modbusctl/internal/errs"

// PartialResult marks aggregate client payloads that may include embedded per-target failures.
// Implementations include multi-unit results, [ScanSummaryResult] (per-request counts), and [SunSpecProbeOutput] (probe/SunSpec errors).
// Commands use this with [SuccessExitForPayload] instead of a growing type switch.
type PartialResult interface {
	HasPartialFailure() bool
}

func summaryHasFailure(s *ResultSummary) bool {
	return s != nil && s.Failed > 0
}

// HasPartialFailure implements [PartialResult].
func (r *IdentifyResult) HasPartialFailure() bool {
	return r != nil && summaryHasFailure(r.Summary)
}

// HasPartialFailure implements [PartialResult].
func (r *FingerprintResult) HasPartialFailure() bool {
	if r == nil {
		return false
	}
	for _, u := range r.Units {
		if FingerprintUnitPartial(u) {
			return true
		}
	}
	return summaryHasFailure(r.Summary)
}

// HasPartialFailure implements [PartialResult].
func (r *ReportServerIDResult) HasPartialFailure() bool {
	return r != nil && summaryHasFailure(r.Summary)
}

// HasPartialFailure implements [PartialResult].
func (r *ScanSummaryResult) HasPartialFailure() bool {
	return r != nil && summaryHasFailure(r.Summary)
}

// SuccessExitForPayload returns ExitPartial when the payload reports any failed targets
// after a successful write; otherwise ExitOK.
func SuccessExitForPayload(v any) int {
	if p, ok := v.(PartialResult); ok && p.HasPartialFailure() {
		return errs.ExitPartial
	}
	return errs.ExitOK
}
