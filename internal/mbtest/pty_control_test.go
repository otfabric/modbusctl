//go:build darwin || linux

// SPDX-License-Identifier: MIT

package mbtest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestControl(t *testing.T) {
	if err := control(nil, func(int) error { return nil }); err == nil {
		t.Error("a nil file has no descriptor")
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "f"))
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("from f")
	if err := control(f, func(fd int) error {
		if fd < 0 {
			t.Errorf("fd %d", fd)
		}
		return want
	}); !errors.Is(err, want) {
		t.Errorf("err = %v, want f's error", err)
	}
	_ = f.Close()
	if err := control(f, func(int) error { return nil }); err == nil {
		t.Error("a closed file has no descriptor")
	}
	if _, err := ptySlavePath(f); err == nil {
		t.Error("a closed file is no pseudo-terminal master")
	}
}
