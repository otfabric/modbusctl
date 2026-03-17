package modbus

import (
	"errors"
	"reflect"
	"testing"

	"github.com/otfabric/go-modbus"
)

func TestObjectDescription(t *testing.T) {
	tests := []struct {
		id   byte
		want string
	}{
		{0x00, "VendorName"},
		{0x01, "ProductCode"},
		{0x02, "MajorMinorRevision"},
		{0x03, "VendorUrl"},
		{0x04, "ProductName"},
		{0x05, "ModelName"},
		{0x06, "UserApplicationName"},
		{0x07, "Reserved"},
		{0x20, "Reserved"},
		{0x7F, "Reserved"},
		{0x80, "Extended"},
		{0xFF, "Extended"},
	}

	for _, tt := range tests {
		if got := objectDescription(modbus.DeviceIDObjectID(tt.id)); got != tt.want {
			t.Errorf("objectDescription(%#02x) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestParseUnitIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []uint8
		wantErr bool
	}{
		{"single", "1", []uint8{1}, false},
		{"single 255", "255", []uint8{255}, false},
		{"all", "all", nil, false}, // nil means we check length 255
		{"range", "1-5", []uint8{1, 2, 3, 4, 5}, false},
		{"range one", "10-10", []uint8{10}, false},
		{"list", "1,5,25", []uint8{1, 5, 25}, false},
		{"mixed", "1-3,10,20-22", []uint8{1, 2, 3, 10, 20, 21, 22}, false},
		{"mixed with space", " 1-2 , 5 ", []uint8{1, 2, 5}, false},
		{"empty", "", nil, true},
		{"invalid single", "999", nil, true},
		{"invalid range", "5-2", nil, true},
		{"invalid part", "1,abc", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUnitIDs(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUnitIDs(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.input == "all" {
				if len(got) != 255 {
					t.Errorf("ParseUnitIDs(%q) len = %d, want 255", tt.input, len(got))
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseUnitIDs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyOutcome(t *testing.T) {
	req, res := int64(1000), int64(2000)
	// Success
	ot, code := classifyOutcome(nil, req, res)
	if ot != ScanOutcomeSuccess || code != 0 {
		t.Errorf("classifyOutcome(nil) = %q, %d; want success, 0", ot, code)
	}
	// Unknown error
	ot, _ = classifyOutcome(errors.New("unknown"), req, res)
	if ot != ScanOutcomeUnknown {
		t.Errorf("classifyOutcome(unknown err) = %q, want unknown", ot)
	}
}
