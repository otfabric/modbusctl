// SPDX-License-Identifier: MIT

package types

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TerminalSafe returns s for a terminal or a table cell: control characters
// (C0 including tab and newline, DEL, C1), Unicode bidirectional controls and
// bytes that are not UTF-8 are written as Go escapes (\x1b, \n, \u202e), so a
// device string cannot move the cursor, recolour or rewrite the screen, or
// fake further lines or columns. Other text, emoji included, is unchanged.
// Use it for strings a device, a capture or another file supplied; JSON output
// keeps the exact value (encoding/json escapes control characters itself).
func TerminalSafe(s string) string {
	return sanitize(s, false)
}

// TerminalSafeText is [TerminalSafe] for multi-line text: newlines and tabs
// are kept.
func TerminalSafeText(s string) string {
	return sanitize(s, true)
}

func sanitize(s string, text bool) string {
	clean := true
	for i, r := range s {
		if unsafeRune(r, text) || (r == utf8.RuneError && !validAt(s, i)) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n <= 1:
			_, _ = fmt.Fprintf(&b, `\x%02x`, s[i])
		case unsafeRune(r, text):
			b.WriteString(escapeRune(r))
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

// validAt reports that the U+FFFD at s[i:] is the encoded character itself,
// not a decoding error.
func validAt(s string, i int) bool {
	_, n := utf8.DecodeRuneInString(s[i:])
	return n == 3
}

func unsafeRune(r rune, text bool) bool {
	switch {
	case text && (r == '\n' || r == '\t'):
		return false
	case r < 0x20, r == 0x7F, r >= 0x80 && r <= 0x9F:
		return true
	case r == 0x061C, r == 0x200E, r == 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true // bidirectional controls
	}
	return false
}

func escapeRune(r rune) string {
	switch r {
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	}
	if r < 0x100 {
		return fmt.Sprintf(`\x%02x`, r)
	}
	return fmt.Sprintf(`\u%04x`, r)
}
