package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StreamWriter renders a sequence of records of the same shape, one at a time, for commands
// that produce output continuously:
//
//   - text: one [TextMarshaler] rendering per record;
//   - json: one compact JSON object per line (JSON Lines);
//   - table: a header before the first record, then one line per row. Column widths are
//     fixed by the first record; wider cells are not truncated.
type StreamWriter struct {
	w      io.Writer
	f      OutputFormat
	widths []int
}

// NewStreamWriter returns a StreamWriter that renders to w in format f.
func NewStreamWriter(w io.Writer, f OutputFormat) *StreamWriter {
	return &StreamWriter{w: w, f: f}
}

// minStreamColumnWidth leaves room for typical values under a short header such as an address.
const minStreamColumnWidth = 8

// Write renders one record.
func (s *StreamWriter) Write(v any) error {
	switch s.f {
	case FormatJSON:
		line, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = s.w.Write(append(line, '\n'))
		return err
	case FormatText:
		tm, ok := v.(TextMarshaler)
		if !ok {
			return fmt.Errorf("text output is not supported for this result type")
		}
		text, err := tm.MarshalTextOutput()
		if err != nil {
			return err
		}
		return writeTextWithTrailingNewline(s.w, text)
	case FormatTable:
		tm, ok := v.(TableMarshaler)
		if !ok {
			return fmt.Errorf("table output is not supported for this command")
		}
		return s.writeTable(tm)
	default:
		return fmt.Errorf("unsupported format %q", s.f)
	}
}

func (s *StreamWriter) writeTable(tm TableMarshaler) error {
	headers := tm.TableHeaders()
	rows := tm.TableRows()
	if len(headers) == 0 {
		return fmt.Errorf("table has no headers")
	}
	for _, row := range rows {
		if len(row) != len(headers) {
			return fmt.Errorf("table row has %d columns, want %d", len(row), len(headers))
		}
	}

	var b strings.Builder
	if s.widths == nil {
		s.widths = make([]int, len(headers))
		for i, h := range headers {
			s.widths[i] = max(len(h), minStreamColumnWidth)
			for _, row := range rows {
				s.widths[i] = max(s.widths[i], len(row[i]))
			}
		}
		s.writeRow(&b, headers)
		rule := make([]string, len(headers))
		for i := range rule {
			rule[i] = "-"
		}
		s.writeRow(&b, rule)
	}
	for _, row := range rows {
		s.writeRow(&b, row)
	}
	_, err := io.WriteString(s.w, b.String())
	return err
}

// writeRow pads every cell to its column width, separated by two spaces, and trims the
// trailing padding.
func (s *StreamWriter) writeRow(b *strings.Builder, cells []string) {
	var line strings.Builder
	for i, c := range cells {
		width := minStreamColumnWidth
		if i < len(s.widths) {
			width = s.widths[i]
		}
		_, _ = fmt.Fprintf(&line, "%-*s  ", width, c)
	}
	b.WriteString(strings.TrimRight(line.String(), " "))
	b.WriteString("\n")
}
