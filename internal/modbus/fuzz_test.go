// SPDX-License-Identifier: MIT

package modbus

import (
	"testing"
)

// FuzzParseUnitIDs checks the --unit selector: the IDs of a valid selector
// are 1-255, sorted and unique.
func FuzzParseUnitIDs(f *testing.F) {
	for _, s := range []string{"all", "ALL", "1", "255", "1-10", "1,5,25", "1-3,2-4,9", "", ",", "0", "256", "5-1", "1-", "-1", "1--2"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		ids, err := ParseUnitIDs(s)
		if err != nil {
			return
		}
		if len(ids) == 0 {
			t.Fatalf("%q: no IDs and no error", s)
		}
		for i, id := range ids {
			if id == 0 {
				t.Fatalf("%q: unit 0", s)
			}
			if i > 0 && ids[i-1] >= id {
				t.Fatalf("%q: not sorted and unique: %v", s, ids)
			}
		}
	})
}

// FuzzParseOffset checks the --from and --to flags: never a negative offset.
func FuzzParseOffset(f *testing.F) {
	for _, s := range []string{"", " ", "90s", "1m30s", "-1s", "1", "1h1h", "9999999999h"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := ParseOffset("from", s)
		if err == nil && d < 0 {
			t.Fatalf("%q: negative offset %v", s, d)
		}
	})
}
