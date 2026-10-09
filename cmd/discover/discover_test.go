package discover

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
)

// servePort serves a device on 127.0.0.1 and returns its TCP port.
func servePort(t *testing.T, d *mbtest.Device) string {
	t.Helper()
	_, port, err := net.SplitHostPort(d.ServeTCP(t))
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestDiscover_formats(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(0, 1)
	port := servePort(t, d)
	portNum, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}

	r := run(t, "discover", "--subnets", "127.0.0.1/32", "--port", port)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "Modbus device found at 127.0.0.1:"+port)

	// The same host listed twice is probed and reported once.
	r = run(t, "discover", "--subnets", "127.0.0.1/32,127.0.0.1/32", "--port", port, "--parallel", "4", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var out types.DiscoverOutput
	decodeJSON(t, r.stdout, &out)
	if int(out.Port) != portNum || len(out.Subnets) != 2 || len(out.Devices) != 1 {
		t.Fatalf("unexpected discover output: %+v", out)
	}
	if dev := out.Devices[0]; dev.IP != "127.0.0.1" || int(dev.Port) != portNum || dev.Mac != "" {
		t.Fatalf("unexpected device: %+v", dev)
	}
	if n := d.RequestCount(0x03); n != 2 {
		t.Fatalf("expected one probe per run, device saw %d FC3 requests", n)
	}

	r = run(t, "discover", "--subnets", "127.0.0.1/32", "--port", port, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "IP", "Port", "MAC", "127.0.0.1", port)
}

func TestDiscover_exceptionCountsAsDevice(t *testing.T) {
	// A device that answers the probe with a Modbus exception is still a Modbus device.
	d := mbtest.NewDevice()
	port := servePort(t, d)
	r := run(t, "--debug", "discover", "--subnets", "127.0.0.1/32", "--port", port, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var out types.DiscoverOutput
	decodeJSON(t, r.stdout, &out)
	if len(out.Devices) != 1 {
		t.Fatalf("expected the device to be found, got %+v", out.Devices)
	}
}

func TestDiscover_outputFile(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(0, 1)
	port := servePort(t, d)
	file := filepath.Join(t.TempDir(), "devices.json")
	r := run(t, "discover", "--subnets", "127.0.0.1/32", "--port", port, "--output", file)
	wantExit(t, r, errs.ExitOK)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var devices []types.DiscoverJson
	decodeJSON(t, string(data), &devices)
	if len(devices) != 1 || devices[0].IP != "127.0.0.1" {
		t.Fatalf("unexpected devices file: %s", data)
	}

	// An output file that cannot be written is a fatal output error.
	r = run(t, "discover", "--subnets", "127.0.0.1/32", "--port", port, "--output", filepath.Join(t.TempDir(), "missing-dir", "devices.json"))
	wantKind(t, r, errs.KindOutput)
}

func TestDiscover_nothingFound(t *testing.T) {
	port := strings.TrimPrefix(refusedURL(t), "tcp://127.0.0.1:")
	r := run(t, "--debug", "discover", "--subnets", "127.0.0.1/32", "--port", port)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "No Modbus devices found.")
	wantContains(t, r.stderr, "DEBUG [discover] 127.0.0.1")

	r = run(t, "discover", "--subnets", "127.0.0.1/32", "--port", port, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var out types.DiscoverOutput
	decodeJSON(t, r.stdout, &out)
	if out.Devices == nil || len(out.Devices) != 0 {
		t.Fatalf("expected an empty device list, got %+v", out.Devices)
	}
}

func TestDiscover_resolveMAC(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(0, 1)
	port := servePort(t, d)
	// The interface does not exist, so no ARP request is sent and the MAC stays empty.
	r := run(t, "discover", "--subnets", "127.0.0.1/32", "--port", port, "--resolve-mac", "--interface", "mbtest-none0", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var out types.DiscoverOutput
	decodeJSON(t, r.stdout, &out)
	if out.Interface != "mbtest-none0" || len(out.Devices) != 1 || out.Devices[0].Mac != "" {
		t.Fatalf("unexpected discover output: %+v", out)
	}
	if os.Geteuid() != 0 {
		wantContains(t, r.stderr, "elevated privileges")
	}
}

func TestDiscover_cancelled(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(0, 1)
	port := servePort(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := runCtx(t, ctx, "discover", "--subnets", "127.0.0.1/32", "--port", port)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "No Modbus devices found.")
	if n := len(d.Requests()); n != 0 {
		t.Fatalf("a cancelled discovery must not probe, device saw %d requests", n)
	}
}

func TestDiscover_failures(t *testing.T) {
	// --subnets is a required flag: cobra rejects the command before RunE.
	r := run(t, "discover")
	if r.err == nil || !strings.Contains(r.err.Error(), "subnets") {
		t.Fatalf("expected a required-flag error, got %v", r.err)
	}

	wantKind(t, run(t, "discover", "--subnets", "not-a-cidr"), errs.KindInvalidInput)
	wantKind(t, run(t, "discover", "--subnets", "127.0.0.1/32", "--port", "0"), errs.KindInvalidInput)
	wantKind(t, run(t, "discover", "--subnets", "127.0.0.1/32", "--parallel", "0"), errs.KindInvalidInput)
	wantKind(t, run(t, "discover", "--subnets", "127.0.0.1/32", "--parallel", "65"), errs.KindInvalidInput)
	wantKind(t, run(t, "discover", "--subnets", "127.0.0.1/32", "--resolve-mac"), errs.KindInvalidInput)
	// A /15 is beyond the host cap without --force-large-scan.
	wantKind(t, run(t, "discover", "--subnets", "10.0.0.0/15"), errs.KindInvalidInput)
	wantKind(t, run(t, "discover", "--subnets", "127.0.0.1/32", "--format", "xml"), errs.KindInvalidInput)

	wantKind(t, runTo(t, context.Background(), failWriter{}, "discover", "--subnets", "127.0.0.1/32", "--port",
		strings.TrimPrefix(refusedURL(t), "tcp://127.0.0.1:")), errs.KindOutput)
}
