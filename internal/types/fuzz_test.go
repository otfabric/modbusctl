// SPDX-License-Identifier: MIT

package types

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzTerminalSafe: whatever a device sends, the escaped text holds no control
// character (but newlines and tabs in text mode), no bidi control and only UTF-8,
// and text that needs no escaping comes back unchanged.
func FuzzTerminalSafe(f *testing.F) {
	for _, s := range []string{"", "ACME", "\x1b[2J", "a\nb\tc\r", "\u202e\u2066", "\xff\xfe", "Größe ⚠️", "\ufffd", "\u009b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, text := range []bool{false, true} {
			out := sanitize(s, text)
			if !utf8.ValidString(out) {
				t.Fatalf("%q: invalid UTF-8 in %q", s, out)
			}
			for _, r := range out {
				if unsafeRune(r, text) {
					t.Fatalf("%q (text %v): %U left in %q", s, text, r, out)
				}
			}
			if !strings.ContainsFunc(s, func(r rune) bool { return unsafeRune(r, text) }) && utf8.ValidString(s) && out != s {
				t.Fatalf("%q (text %v) changed to %q", s, text, out)
			}
		}
	})
}
