// SPDX-License-Identifier: MIT

package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateOutput_newFileIsOwnerOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.json")
	if err := WriteOutput(p, []byte("x")); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != OutputMode {
		t.Errorf("mode %v, want %v", st.Mode().Perm(), os.FileMode(OutputMode))
	}
	if b, _ := os.ReadFile(p); string(b) != "x" {
		t.Errorf("content %q", b)
	}
}

func TestCreateOutput_refusesSymlink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := WriteOutput(link, []byte("overwritten")); !errors.Is(err, ErrOutputLink) {
		t.Fatalf("err = %v, want ErrOutputLink", err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("the link target was written: %q", b)
	}
	if !IsLink(link) || IsLink(victim) || IsLink(filepath.Join(dir, "none")) {
		t.Error("IsLink")
	}
}

func TestWriteOutput_errors(t *testing.T) {
	if err := WriteOutput(filepath.Join(t.TempDir(), "no", "such", "dir"), nil); err == nil {
		t.Error("missing directory accepted")
	}
}

func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !SameFile(a, b, filepath.Join(dir, ".", "a")) {
		t.Error("another spelling of the same path is the same file")
	}
	if SameFile(a, b, "") || SameFile("", a) || SameFile(filepath.Join(dir, "new"), a) || SameFile(a, filepath.Join(dir, "none")) {
		t.Error("different files")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(a, link); err == nil && !SameFile(link, a) {
		t.Error("a link to the input is the input")
	}
}

func TestReadInput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "in")
	if err := os.WriteFile(p, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := ReadInput(p, 10); err != nil || string(b) != "0123456789" {
		t.Fatalf("at the limit: %q %v", b, err)
	}
	if _, err := ReadInput(p, 9); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("above the limit: %v", err)
	}
	if _, err := ReadInput(filepath.Join(t.TempDir(), "none"), 10); err == nil {
		t.Fatal("missing file")
	}
	if _, err := ReadInput(t.TempDir(), 10); err == nil {
		t.Fatal("a directory is not an input")
	}
}
