package modbus

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
)

// TestCollectRead_rejectedWhenContextPreCanceled documents that TCP connect/open does not use ctx;
// cancellation is still honored once the Modbus read path runs (integration with a real server would be needed
// to assert ctx.Err() dominates). Here we only ensure the call completes without panic.
func TestCollectRead_rejectedWhenContextPreCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tmp, err := os.CreateTemp("", "read_cancel_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmp.Name()) }()

	cfg := config.ReadConfig{
		DeviceConfig: config.DeviceConfig{
			IP:   "127.0.0.1",
			Port: 1,
			Unit: 1,
		},
		Timeout:       200,
		Function:      3,
		StartAddress:  1,
		RegisterCount: 1,
		OutputFile:    tmp.Name(),
	}
	_, err = CollectRead(ctx, cfg, io.Discard)
	if err == nil {
		t.Fatal("expected error (connection refused or similar)")
	}
}
