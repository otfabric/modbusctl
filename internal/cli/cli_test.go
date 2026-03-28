package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRegisterEnumFlagCompletion_unknownFlag(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "x"}
	if err := RegisterEnumFlagCompletion(cmd, "missing", []string{"a"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterEnumFlagCompletion_behavior(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("algo", "", "")
	if err := RegisterEnumFlagCompletion(cmd, "algo", []string{"safe", "smart", "deep"}); err != nil {
		t.Fatal(err)
	}
	fn, ok := cmd.GetFlagCompletionFunc("algo")
	if !ok || fn == nil {
		t.Fatal("no completion func")
	}
	dir := cobra.ShellCompDirectiveNoFileComp

	out, d := fn(cmd, nil, "")
	if d != dir || len(out) != 3 {
		t.Fatalf("empty prefix: %v %v", out, d)
	}
	out, _ = fn(cmd, nil, "sm")
	if !slices.Equal(out, []string{"smart"}) {
		t.Fatalf("prefix sm: %v", out)
	}
	out, _ = fn(cmd, nil, "SM")
	if !slices.Equal(out, []string{"smart"}) {
		t.Fatalf("case: %v", out)
	}
	out, _ = fn(cmd, nil, "nomatch")
	if len(out) != 0 {
		t.Fatalf("nomatch: %v", out)
	}
}

func TestRegisterEnumFlagCompletionWithDescriptions(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("format", "", "")
	desc := map[string]string{"csv": "comma", "json": "js", "bare": ""}
	if err := RegisterEnumFlagCompletionWithDescriptions(cmd, "format", desc); err != nil {
		t.Fatal(err)
	}
	fn, ok := cmd.GetFlagCompletionFunc("format")
	if !ok {
		t.Fatal("missing fn")
	}
	out, _ := fn(cmd, nil, "")
	// Sorted by key: bare, csv, json
	var foundBare, foundCSV bool
	for _, s := range out {
		if s == "bare" {
			foundBare = true
		}
		if strings.HasPrefix(s, "csv\t") {
			foundCSV = true
		}
	}
	if !foundBare || !foundCSV {
		t.Fatalf("out=%q", out)
	}
	out, _ = fn(cmd, nil, "j")
	if len(out) != 1 || !strings.HasPrefix(out[0], "json") {
		t.Fatalf("json prefix: %v", out)
	}
}

func TestOpenStdoutOrFile(t *testing.T) {
	t.Parallel()
	w, cleanup, err := OpenStdoutOrFile("")
	if err != nil || w != os.Stdout {
		t.Fatalf("stdout: err=%v w=%T", err, w)
	}
	cleanup()

	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	w2, cleanup2, err := OpenStdoutOrFile(p)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup2()
	if _, err := w2.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	cleanup2()
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "hi" {
		t.Fatalf("read: %v %q", err, b)
	}

	w3, _, err := OpenStdoutOrFile("   ")
	if err != nil || w3 != os.Stdout {
		t.Fatalf("whitespace path: %v %T", err, w3)
	}
}

func TestDebug(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{}
	root.PersistentFlags().Bool("debug", false, "")
	sub := &cobra.Command{Use: "sub"}
	root.AddCommand(sub)
	if Debug(sub) {
		t.Fatal("default false")
	}
	_ = root.PersistentFlags().Set("debug", "true")
	if !Debug(sub) {
		t.Fatal("want true")
	}
	orphan := &cobra.Command{}
	if Debug(orphan) {
		t.Fatal("no root flags")
	}
}

func TestMustMarkFlagRequired_ok(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("need", "", "")
	MustMarkFlagRequired(cmd, "need")
}

func TestMustMarkFlagRequired_panics(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustMarkFlagRequired(cmd, "nope")
}
