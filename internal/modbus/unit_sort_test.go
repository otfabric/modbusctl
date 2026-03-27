package modbus

import (
	"testing"

	"github.com/otfabric/modbusctl/internal/types"
)

func TestSortIdentifyUnitsByID(t *testing.T) {
	units := []types.IdentifyUnitResult{
		{UnitID: 200},
		{UnitID: 3},
		{UnitID: 99},
	}
	sortIdentifyUnitsByID(units)
	if units[0].UnitID != 3 || units[1].UnitID != 99 || units[2].UnitID != 200 {
		t.Fatalf("got order %+v", units)
	}
}

func TestSortReportServerUnits(t *testing.T) {
	units := []types.ReportServerIDUnitResult{
		{UnitID: 250},
		{UnitID: 1},
		{UnitID: 128},
	}
	sortReportServerUnits(units)
	if units[0].UnitID != 1 || units[1].UnitID != 128 || units[2].UnitID != 250 {
		t.Fatalf("got order %+v", units)
	}
}
