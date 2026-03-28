package types

import (
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
)

func TestNewResultSummary(t *testing.T) {
	t.Parallel()
	if NewResultSummary(0, 0, 0) != nil {
		t.Fatal("all zero -> nil")
	}
	s := NewResultSummary(5, 4, 1)
	if s.Requested != 5 || s.Succeeded != 4 || s.Failed != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestFillIdentifySummary(t *testing.T) {
	t.Parallel()
	FillIdentifySummary(nil, 3)
	res := &IdentifyResult{
		Units: []IdentifyUnitResult{
			{UnitID: 1},
			{UnitID: 2, Error: &ErrorInfo{Message: "timeout"}},
		},
	}
	FillIdentifySummary(res, 2)
	if res.Summary == nil || res.Summary.Requested != 2 || res.Summary.Failed != 1 || res.Summary.Succeeded != 1 {
		t.Fatalf("%+v", res.Summary)
	}
	FillIdentifySummary(res, 0)
}

func TestFillFingerprintSummary(t *testing.T) {
	t.Parallel()
	res := &FingerprintResult{
		Units: []FingerprintUnitResult{
			{UnitID: 1, SupportedReads: []string{"3"}},
			{UnitID: 2, Error: &ErrorInfo{Message: "x"}},
		},
	}
	FillFingerprintSummary(res, 2)
	if res.Summary == nil || res.Summary.Failed != 1 {
		t.Fatalf("%+v", res.Summary)
	}
}

func TestFillReportServerIDSummary(t *testing.T) {
	t.Parallel()
	res := &ReportServerIDResult{
		Units: []ReportServerIDUnitResult{
			{UnitID: 1, DataHex: "ab"},
			{UnitID: 2, Error: &ErrorInfo{Message: "nope"}},
		},
	}
	FillReportServerIDSummary(res, 2)
	if res.Summary == nil || res.Summary.Failed != 1 {
		t.Fatalf("%+v", res.Summary)
	}
}

func TestHasPartialFailure(t *testing.T) {
	t.Parallel()
	if (*IdentifyResult)(nil).HasPartialFailure() {
		t.Fatal("nil identify")
	}
	id := &IdentifyResult{Summary: &ResultSummary{Failed: 1}}
	if !id.HasPartialFailure() {
		t.Fatal("identify summary failed")
	}
	fp := &FingerprintResult{
		Units: []FingerprintUnitResult{
			{ProbeInterrupted: true, SupportedReads: []string{"3"}},
		},
	}
	if !fp.HasPartialFailure() {
		t.Fatal("fingerprint partial unit")
	}
	fp2 := &FingerprintResult{Summary: &ResultSummary{Failed: 1}}
	if !fp2.HasPartialFailure() {
		t.Fatal("fingerprint summary")
	}
	rs := &ReportServerIDResult{Summary: &ResultSummary{Failed: 1}}
	if !rs.HasPartialFailure() {
		t.Fatal("report server id")
	}
	scan := &ScanSummaryResult{Summary: &ResultSummary{Failed: 2}}
	if !scan.HasPartialFailure() {
		t.Fatal("scan")
	}
}

func TestSuccessExitForPayload(t *testing.T) {
	t.Parallel()
	if c := SuccessExitForPayload("not partial"); c != errs.ExitOK {
		t.Fatalf("plain: %d", c)
	}
	ok := &IdentifyResult{Summary: &ResultSummary{Failed: 0}}
	if c := SuccessExitForPayload(ok); c != errs.ExitOK {
		t.Fatalf("no failures: %d", c)
	}
	bad := &IdentifyResult{Summary: &ResultSummary{Failed: 1}}
	if c := SuccessExitForPayload(bad); c != errs.ExitPartial {
		t.Fatalf("partial: %d", c)
	}
}
