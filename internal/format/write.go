package format

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// TextMarshaler produces human-oriented CLI text (default --format text).
type TextMarshaler interface {
	MarshalTextOutput() (string, error)
}

// TableMarshaler produces a rectangular grid for --format table.
type TableMarshaler interface {
	TableHeaders() []string
	TableRows() [][]string
}

// Write renders v to w according to f. JSON always uses encoding/json.
// Text requires TextMarshaler; table requires TableMarshaler.
// Text and table outputs get exactly one trailing newline after successful write.
// JSON is encoded fully into memory first, then written in one Write (no partial JSON on encode failure).
func Write(w io.Writer, f OutputFormat, v any) error {
	switch f {
	case FormatJSON:
		return writeJSON(w, v)
	case FormatText:
		tm, ok := v.(TextMarshaler)
		if !ok {
			return fmt.Errorf("text output is not supported for this result type")
		}
		s, err := tm.MarshalTextOutput()
		if err != nil {
			return err
		}
		return writeTextWithTrailingNewline(w, s)
	case FormatTable:
		tm, ok := v.(TableMarshaler)
		if !ok {
			return fmt.Errorf("table output is not supported for this command")
		}
		s, err := RenderTable(tm)
		if err != nil {
			return err
		}
		return writeTextWithTrailingNewline(w, s)
	default:
		return fmt.Errorf("unsupported format %q", f)
	}
}

func writeJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func writeTextWithTrailingNewline(w io.Writer, s string) error {
	s = strings.TrimSuffix(s, "\n")
	if _, err := io.WriteString(w, s+"\n"); err != nil {
		return err
	}
	return nil
}
