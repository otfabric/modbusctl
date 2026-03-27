package format

import (
	"fmt"
	"strings"
)

// OutputFormat selects how CLI results are rendered to stdout.
type OutputFormat string

const (
	FormatText  OutputFormat = "text"
	FormatJSON  OutputFormat = "json"
	FormatTable OutputFormat = "table"
)

// Values returns all legal client stdout format strings. Single source for [Parse], [OutputFormat.Valid], tests, and completion.
func Values() []string {
	return []string{string(FormatText), string(FormatJSON), string(FormatTable)}
}

// Valid reports whether f is a supported output format.
func (f OutputFormat) Valid() bool {
	if f == "" {
		return false
	}
	for _, v := range Values() {
		if f == OutputFormat(v) {
			return true
		}
	}
	return false
}

// Parse normalizes and validates a user-provided format string.
// Empty or whitespace-only strings default to [FormatText] (CLI default / unset env).
func Parse(s string) (OutputFormat, error) {
	f := OutputFormat(strings.ToLower(strings.TrimSpace(s)))
	if f == "" {
		return FormatText, nil
	}
	if !f.Valid() {
		return "", fmt.Errorf("invalid format %q (expected: %s)", s, strings.Join(Values(), ", "))
	}
	return f, nil
}
