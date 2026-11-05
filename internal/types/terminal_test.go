// SPDX-License-Identifier: MIT

package types

import "testing"

func TestTerminalSafe(t *testing.T) {
	for _, tc := range []struct{ in, cell, text string }{
		{"ACME", "ACME", "ACME"},
		{"Größe ⚠️ 🧱 \ufffd", "Größe ⚠️ 🧱 \ufffd", "Größe ⚠️ 🧱 \ufffd"},
		{"\x1b[2J\x1b]0;pwned\x07", `\x1b[2J\x1b]0;pwned\x07`, `\x1b[2J\x1b]0;pwned\x07`},
		{"a\nb\tc\rd", `a\nb\tc\rd`, "a\nb\tc\\rd"},
		{"x\x7f\u009by", `x\x7f\x9by`, `x\x7f\x9by`},
		{"abc\u202edef\u2066", `abc\u202edef\u2066`, `abc\u202edef\u2066`},
		{"bad\xff\xfe", `bad\xff\xfe`, `bad\xff\xfe`},
		{"", "", ""},
	} {
		if got := TerminalSafe(tc.in); got != tc.cell {
			t.Errorf("TerminalSafe(%q) = %q, want %q", tc.in, got, tc.cell)
		}
		if got := TerminalSafeText(tc.in); got != tc.text {
			t.Errorf("TerminalSafeText(%q) = %q, want %q", tc.in, got, tc.text)
		}
	}
}
