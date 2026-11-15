// SPDX-License-Identifier: MIT

package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
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
	w, finish, err := OpenStdoutOrFile("")
	if err != nil || w != os.Stdout {
		t.Fatalf("stdout: err=%v w=%T", err, w)
	}
	if got := finish(errSentinel); got != errSentinel {
		t.Fatalf("stdout: finish returned %v, want the command's error", got)
	}
	if got := finish(nil); got != nil {
		t.Fatalf("stdout: finish returned %v", got)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	w2, finish2, err := OpenStdoutOrFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w2.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := finish2(nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "hi" {
		t.Fatalf("read: %v %q", err, b)
	}

	w3, _, err := OpenStdoutOrFile("   ")
	if err != nil || w3 != os.Stdout {
		t.Fatalf("whitespace path: %v %T", err, w3)
	}
}

var errSentinel = errors.New("the command's own error")

// failingFile is an output file whose writes fail after limit bytes, and whose Close fails
// when closeErr is set.
type failingFile struct {
	limit    int
	written  bytes.Buffer
	closeErr error
	closed   int
}

func (f *failingFile) Write(p []byte) (int, error) {
	if f.written.Len()+len(p) > f.limit {
		n := f.limit - f.written.Len()
		f.written.Write(p[:n])
		return n, syscall.ENOSPC
	}
	return f.written.Write(p)
}

func (f *failingFile) Close() error {
	f.closed++
	return f.closeErr
}

// A write, flush or close of the output that fails is the command's result: an output error,
// exit 10, with code output_file_write_failed.
func TestBufferedOutput_errors(t *testing.T) {
	t.Parallel()
	big := bytes.Repeat([]byte("x"), 10_000) // more than the buffer holds
	tests := []struct {
		name     string
		limit    int
		closeErr error
		write    []byte
		cmdErr   error
		want     error // the cause, or the command's error when wantCode is empty
		wantCode string
	}{
		{name: "everything written", limit: 1 << 20, write: big},
		{name: "a write fails before the buffer is full", limit: 3, write: []byte("hello"), want: syscall.ENOSPC, wantCode: errs.CodeOutputFileWriteFailed},
		{name: "a write fails while the buffer is flushed", limit: 5000, write: big, want: syscall.ENOSPC, wantCode: errs.CodeOutputFileWriteFailed},
		{name: "a write that fails wins over the command's error", limit: 0, write: []byte("x"), cmdErr: errSentinel, want: syscall.ENOSPC, wantCode: errs.CodeOutputFileWriteFailed},
		{name: "the close fails", limit: 1 << 20, write: []byte("x"), closeErr: syscall.EIO, want: syscall.EIO, wantCode: errs.CodeOutputFileWriteFailed},
		{name: "the command's error wins over the close", limit: 1 << 20, closeErr: syscall.EIO, cmdErr: errSentinel, want: errSentinel},
		{name: "the command's error is kept", limit: 1 << 20, cmdErr: errSentinel, want: errSentinel},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &failingFile{limit: tc.limit, closeErr: tc.closeErr}
			w, finish := bufferedOutput(f)
			// The exports ignore the results of their writes; finish does not.
			_, _ = w.Write(tc.write)
			err := finish(tc.cmdErr)
			if f.closed != 1 {
				t.Fatalf("file closed %d times, want once", f.closed)
			}
			if tc.want == nil {
				if err != nil {
					t.Fatalf("finish: %v", err)
				}
				if !bytes.Equal(f.written.Bytes(), tc.write) {
					t.Fatalf("wrote %d bytes, want %d", f.written.Len(), len(tc.write))
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("finish returned %v, want %v", err, tc.want)
			}
			if tc.wantCode == "" {
				if err != tc.want {
					t.Fatalf("finish returned %v, want the command's error unchanged", err)
				}
				return
			}
			e, ok := errs.As(err)
			if !ok || e.Code != tc.wantCode || errs.ExitCodeForKind(e.Kind) != errs.ExitInternal {
				t.Fatalf("finish returned %#v, want an output error %s (exit 10)", err, tc.wantCode)
			}
		})
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

func TestOpenStdoutOrFile_createError(t *testing.T) {
	w, finish, err := OpenStdoutOrFile(filepath.Join(t.TempDir(), "missing-dir", "out.txt"))
	if err == nil {
		t.Fatal("expected an error for a path in a missing directory")
	}
	if w != nil || finish != nil {
		t.Fatalf("expected no writer and no finish on error, got %v", w)
	}
}
