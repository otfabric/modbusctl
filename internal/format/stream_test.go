package format

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// record is a configurable text/table/JSON value.
type record struct {
	Text    string     `json:"text"`
	TextErr error      `json:"-"`
	Headers []string   `json:"-"`
	Rows    [][]string `json:"-"`
}

func (r record) MarshalTextOutput() (string, error) { return r.Text, r.TextErr }
func (r record) TableHeaders() []string             { return r.Headers }
func (r record) TableRows() [][]string              { return r.Rows }

// errWriter fails every write.
type errWriter struct{}

var errSink = errors.New("sink closed")

func (errWriter) Write([]byte) (int, error) { return 0, errSink }

func TestStreamWriter_text(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, FormatText)
	require.NoError(t, sw.Write(record{Text: "first"}))
	require.NoError(t, sw.Write(record{Text: "second\n"}))
	require.NoError(t, sw.Write(record{Text: ""}))
	require.NoError(t, sw.Write(record{Text: "two\nlines\n\n"}))
	assert.Equal(t, "first\nsecond\n\ntwo\nlines\n\n", buf.String())
}

func TestStreamWriter_textErrors(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, FormatText)

	assert.EqualError(t, sw.Write(struct{}{}), "text output is not supported for this result type")
	boom := errors.New("boom")
	assert.ErrorIs(t, sw.Write(record{Text: "ignored", TextErr: boom}), boom)
	assert.Empty(t, buf.String())

	assert.ErrorIs(t, NewStreamWriter(errWriter{}, FormatText).Write(record{Text: "x"}), errSink)
}

func TestStreamWriter_jsonLines(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, FormatJSON)
	require.NoError(t, sw.Write(record{Text: "a"}))
	require.NoError(t, sw.Write(map[string]int{"b": 2, "a": 1}))
	require.NoError(t, sw.Write(&types.PollSummary{Target: "t", Polls: 2, Succeeded: 1, Failed: 1, Duration: "1s"}))
	require.NoError(t, sw.Write(nil))
	assert.Equal(t,
		`{"text":"a"}`+"\n"+
			`{"a":1,"b":2}`+"\n"+
			`{"target":"t","polls":2,"succeeded":1,"failed":1,"duration":"1s","interrupted":false}`+"\n"+
			"null\n",
		buf.String())
}

func TestStreamWriter_jsonErrors(t *testing.T) {
	var buf bytes.Buffer
	err := NewStreamWriter(&buf, FormatJSON).Write(make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
	assert.Empty(t, buf.String())

	assert.ErrorIs(t, NewStreamWriter(errWriter{}, FormatJSON).Write(record{}), errSink)
}

func TestStreamWriter_table(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, FormatTable)
	headers := []string{"poll", "timestamp", "v", "error"}

	// The first record fixes the widths: the minimum (8), or the widest header or cell.
	require.NoError(t, sw.Write(record{Headers: headers, Rows: [][]string{{"1", "2026-10-09T12:00:00Z", "7", ""}}}))
	assert.Equal(t,
		"poll      timestamp             v         error\n"+
			"-         -                     -         -\n"+
			"1         2026-10-09T12:00:00Z  7\n",
		buf.String())

	// Later records get no header; wider cells are written in full and push the rest right.
	buf.Reset()
	require.NoError(t, sw.Write(record{Headers: headers, Rows: [][]string{
		{"2", "2026-10-09T12:00:01Z", "123456789012", "x"},
		{"3", "t", "", "request timed out"},
	}}))
	assert.Equal(t,
		"2         2026-10-09T12:00:01Z  123456789012  x\n"+
			"3         t                               request timed out\n",
		buf.String())

	// A record without rows writes nothing once the header is out.
	buf.Reset()
	require.NoError(t, sw.Write(record{Headers: headers}))
	assert.Empty(t, buf.String())
}

func TestStreamWriter_tableHeaderOnlyFirstRecord(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, FormatTable)
	require.NoError(t, sw.Write(record{Headers: []string{"a", "a-long-header"}}))
	require.NoError(t, sw.Write(record{Headers: []string{"a", "a-long-header"}, Rows: [][]string{{"1", "2"}}}))
	assert.Equal(t,
		"a         a-long-header\n"+
			"-         -\n"+
			"1         2\n",
		buf.String())
}

func TestStreamWriter_tablePollSamples(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, FormatTable)
	ok := &types.PollSample{
		Poll: 1, Timestamp: "2026-10-09T12:00:00.123Z", StartAddress: 100, Count: 2, RTTMs: 4.26,
		Values: []any{uint16(1234), uint16(5678)},
	}
	failed := &types.PollSample{
		Poll: 2, Timestamp: "2026-10-09T12:00:01.123Z", StartAddress: 100, Count: 2, RTTMs: 500,
		Error: &types.ErrorInfo{Message: "request timed out"},
	}
	require.NoError(t, sw.Write(ok))
	require.NoError(t, sw.Write(failed))
	assert.Equal(t,
		"poll      timestamp                 rtt_ms    100       101       error\n"+
			"-         -                         -         -         -         -\n"+
			"1         2026-10-09T12:00:00.123Z  4.3       1234      5678\n"+
			"2         2026-10-09T12:00:01.123Z  500.0                         request timed out\n",
		buf.String())
}

func TestStreamWriter_tableErrors(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, FormatTable)

	assert.EqualError(t, sw.Write(struct{}{}), "table output is not supported for this command")
	assert.EqualError(t, sw.Write(record{}), "table has no headers")
	assert.EqualError(t,
		sw.Write(record{Headers: []string{"a", "b"}, Rows: [][]string{{"1", "2"}, {"only"}}}),
		"table row has 1 columns, want 2")
	assert.Empty(t, buf.String(), "a rejected record must not be written partially")

	// A rejected first record does not count as the header having been written.
	require.NoError(t, sw.Write(record{Headers: []string{"a", "b"}, Rows: [][]string{{"1", "2"}}}))
	assert.Equal(t, "a         b\n-         -\n1         2\n", buf.String())

	assert.ErrorIs(t,
		NewStreamWriter(errWriter{}, FormatTable).Write(record{Headers: []string{"a"}, Rows: [][]string{{"1"}}}),
		errSink)
}

func TestStreamWriter_unsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	assert.EqualError(t, NewStreamWriter(&buf, OutputFormat("yaml")).Write(record{}), `unsupported format "yaml"`)
	assert.EqualError(t, NewStreamWriter(&buf, "").Write(record{}), `unsupported format ""`)
	assert.Empty(t, buf.String())
}

func TestWrite_errors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		w       io.Writer
		f       OutputFormat
		v       any
		wantErr string
	}{
		{"text marshal error", &bytes.Buffer{}, FormatText, record{TextErr: boom}, "boom"},
		{"text writer error", errWriter{}, FormatText, record{Text: "x"}, "sink closed"},
		{"table unsupported", &bytes.Buffer{}, FormatTable, struct{}{}, "table output is not supported for this command"},
		{"table no headers", &bytes.Buffer{}, FormatTable, record{}, "table has no headers"},
		{"table ragged row", &bytes.Buffer{}, FormatTable, record{Headers: []string{"a"}, Rows: [][]string{{"1", "2"}}}, "table row has 2 columns, want 1"},
		{"table writer error", errWriter{}, FormatTable, record{Headers: []string{"a"}}, "sink closed"},
		{"json encode error", &bytes.Buffer{}, FormatJSON, func() {}, "json: unsupported type: func()"},
		{"json writer error", errWriter{}, FormatJSON, record{}, "sink closed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, Write(tt.w, tt.f, tt.v), tt.wantErr)
			if buf, ok := tt.w.(*bytes.Buffer); ok {
				assert.Empty(t, buf.String())
			}
		})
	}
}

func TestWrite_exactOutput(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Write(&buf, FormatTable, record{
		Headers: []string{"unit_id", "value"},
		Rows:    [][]string{{"1", "a"}, {"247", "longer value"}},
	}))
	// No line carries trailing padding, the rule included.
	assert.Equal(t,
		"unit_id  value\n"+
			"-        -\n"+
			"1        a\n"+
			"247      longer value\n",
		buf.String())

	buf.Reset()
	require.NoError(t, Write(&buf, FormatJSON, record{Text: "x"}))
	assert.Equal(t, "{\n  \"text\": \"x\"\n}\n", buf.String())

	buf.Reset()
	require.NoError(t, Write(&buf, FormatText, record{Text: "a\n\n"}))
	assert.Equal(t, "a\n\n", buf.String())
}
