package types

// ResultSummary counts aggregate outcomes for multi-target client commands (Phase 8 / B2).
type ResultSummary struct {
	Requested int `json:"requested"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// NewResultSummary builds a summary.
func NewResultSummary(requested, succeeded, failed int) *ResultSummary {
	if requested == 0 && succeeded == 0 && failed == 0 {
		return nil
	}
	return &ResultSummary{Requested: requested, Succeeded: succeeded, Failed: failed}
}

// FillIdentifySummary sets res.Summary from unit rows and the requested unit count.
func FillIdentifySummary(res *IdentifyResult, requested int) {
	if res == nil || requested == 0 {
		return
	}
	failed := countIdentifyUnitFailures(res.Units)
	res.Summary = NewResultSummary(requested, requested-failed, failed)
}

// FillFingerprintSummary sets res.Summary from fingerprint unit rows.
func FillFingerprintSummary(res *FingerprintResult, requested int) {
	if res == nil || requested == 0 {
		return
	}
	failed := countFingerprintUnitFailures(res.Units)
	res.Summary = NewResultSummary(requested, requested-failed, failed)
}

// FillReportServerIDSummary sets res.Summary from FC17 unit rows.
func FillReportServerIDSummary(res *ReportServerIDResult, requested int) {
	if res == nil || requested == 0 {
		return
	}
	failed := countReportServerIDFailures(res.Units)
	res.Summary = NewResultSummary(requested, requested-failed, failed)
}

func countIdentifyUnitFailures(units []IdentifyUnitResult) int {
	n := 0
	for i := range units {
		if ErrorMessage(units[i].Error) != "" {
			n++
		}
	}
	return n
}

func countFingerprintUnitFailures(units []FingerprintUnitResult) int {
	n := 0
	for i := range units {
		if FingerprintUnitUnusable(units[i]) {
			n++
		}
	}
	return n
}

func countReportServerIDFailures(units []ReportServerIDUnitResult) int {
	n := 0
	for i := range units {
		if ErrorMessage(units[i].Error) != "" {
			n++
		}
	}
	return n
}
