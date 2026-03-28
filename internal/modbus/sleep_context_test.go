package modbus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSleepContext_zeroDuration(t *testing.T) {
	if err := sleepContext(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := sleepContext(context.Background(), -1); err != nil {
		t.Fatal(err)
	}
}

func TestSleepContext_cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sleepContext(ctx, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want %v", err, context.Canceled)
	}
}

func TestSleepContext_completes(t *testing.T) {
	start := time.Now()
	if err := sleepContext(context.Background(), 25*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 15*time.Millisecond {
		t.Fatalf("slept too little: %v", d)
	}
}
