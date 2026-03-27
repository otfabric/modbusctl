package format

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestParse_emptyDefaultsToText(t *testing.T) {
	f, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if f != FormatText {
		t.Fatalf("got %q want text", f)
	}
	f, err = Parse("   ")
	if err != nil {
		t.Fatal(err)
	}
	if f != FormatText {
		t.Fatalf("got %q want text", f)
	}
}

func TestParse(t *testing.T) {
	for _, s := range []string{"text", "TEXT", " json ", "table"} {
		f, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if !f.Valid() {
			t.Fatalf("Parse(%q) not valid", s)
		}
	}
	if _, err := Parse("yaml"); err == nil {
		t.Fatal("expected error for invalid format")
	}
}

type textStub struct{}

func (textStub) MarshalTextOutput() (string, error) { return "hello", nil }

func TestWrite_textTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, textStub{}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "hello\n" {
		t.Fatalf("got %q want hello + newline", got)
	}
}

type tableStub struct{}

func (tableStub) TableHeaders() []string { return []string{"a", "b"} }
func (tableStub) TableRows() [][]string {
	return [][]string{{"1", "2"}}
}

func TestWrite_tableTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatTable, tableStub{}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.HasSuffix(s, "\n") || strings.Count(s, "\n") < 2 {
		t.Fatalf("unexpected table output: %q", s)
	}
}

type noText struct{}

func TestWrite_textUnsupported(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, noText{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestWrite_jsonFieldNames(t *testing.T) {
	type row struct {
		Target string `json:"target"`
		UnitID uint8  `json:"unit_id"`
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatJSON, row{Target: "tcp://x:502", UnitID: 1}); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["target"]; !ok {
		t.Fatal("missing target")
	}
	if _, ok := m["unit_id"]; !ok {
		t.Fatal("missing unit_id")
	}
}
