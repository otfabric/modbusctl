//go:build !windows

// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
)

// A signal during a request that never gets its answer ends the command as interrupted.
func TestExecute_interrupted(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(100, 1)
	url := "tcp://" + d.ServeTCP(t)
	sent := make(chan struct{}, 1)
	d.OnRequest = func(mbtest.Request) mbtest.Action {
		select {
		case sent <- struct{}{}:
		default:
		}
		return mbtest.Action{Drop: true}
	}
	go func() {
		select {
		case <-sent:
		case <-time.After(10 * time.Second):
		}
		_ = interruptSelf()
	}()
	code, _, stderr := execute(t, "client", "read", "--url", url, "--start", "100", "--timeout", "30000",
		"--output", filepath.Join(t.TempDir(), "x.mbcap"))
	if code != errs.ExitInterrupted {
		t.Fatalf("exit %d, want %d\n%s", code, errs.ExitInterrupted, stderr)
	}
}

func interruptSelf() error {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return p.Signal(os.Interrupt)
}
