package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFingerprintUnitFailed(t *testing.T) {
	t.Parallel()
	assert.False(t, FingerprintUnitFailed(FingerprintUnitResult{UnitID: 1}))
	assert.True(t, FingerprintUnitFailed(FingerprintUnitResult{
		UnitID: 1,
		Error:  &ErrorInfo{Message: "timeout"},
	}))
	assert.True(t, FingerprintUnitFailed(FingerprintUnitResult{
		UnitID:           1,
		ProbeInterrupted: true,
	}))
}

func TestFillFingerprintSummary_countsProbeInterrupted(t *testing.T) {
	t.Parallel()
	res := &FingerprintResult{
		Units: []FingerprintUnitResult{
			{UnitID: 1, SupportedReads: []string{"ReadHoldingRegisters"}},
			{UnitID: 2, ProbeInterrupted: true}, // defensive: no Error embedded
		},
	}
	FillFingerprintSummary(res, 2)
	if assert.NotNil(t, res.Summary) {
		assert.Equal(t, 2, res.Summary.Requested)
		assert.Equal(t, 1, res.Summary.Succeeded)
		assert.Equal(t, 1, res.Summary.Failed)
	}
}

func TestFillFingerprintSummary_partialNotCountedFailed(t *testing.T) {
	t.Parallel()
	res := &FingerprintResult{
		Units: []FingerprintUnitResult{
			{
				UnitID:           1,
				ProbeInterrupted: true,
				SupportedReads:   []string{"ReadHoldingRegisters"},
				Error:            &ErrorInfo{Message: "timeout"},
			},
		},
	}
	FillFingerprintSummary(res, 1)
	if assert.NotNil(t, res.Summary) {
		assert.Equal(t, 1, res.Summary.Succeeded)
		assert.Equal(t, 0, res.Summary.Failed)
	}
	assert.True(t, res.HasPartialFailure())
}
