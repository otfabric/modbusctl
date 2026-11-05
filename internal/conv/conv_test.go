// SPDX-License-Identifier: MIT

package conv

import (
	"math"
	"testing"
)

func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", name)
		}
	}()
	f()
}

func TestConversions(t *testing.T) {
	if Uint8(0) != 0 || Uint8(255) != 255 {
		t.Error("Uint8 in range")
	}
	if Uint16(0) != 0 || Uint16(math.MaxUint16) != math.MaxUint16 {
		t.Error("Uint16 in range")
	}
	if Addr16(0) != 0 || Addr16(math.MaxUint16) != math.MaxUint16 {
		t.Error("Addr16 in range")
	}
	mustPanic(t, "Uint8(-1)", func() { Uint8(-1) })
	mustPanic(t, "Uint8(256)", func() { Uint8(256) })
	mustPanic(t, "Uint16(-1)", func() { Uint16(-1) })
	mustPanic(t, "Uint16(65536)", func() { Uint16(65536) })
	mustPanic(t, "Addr16(65536)", func() { Addr16(65536) })
}
