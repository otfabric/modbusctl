// SPDX-License-Identifier: MIT

package discover

import (
	"net"
	"sort"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/socks5test"
	"github.com/otfabric/modbusctl/internal/types"
)

// Discovery through the proxy: every host of a TEST-NET range is probed through it, and
// the proxy forwards each CONNECT to the one fake device, which so answers for all.
func TestDiscover_proxy(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(0, 1)
	addr := d.ServeTCP(t)
	px := socks5test.Start(t)
	px.Forward = func(string) string { return addr }

	r := run(t, "discover", "--subnets", "192.0.2.8/30", "--port", "502", "--proxy", px.URL(), "--source-addr", "127.0.0.1", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var out types.DiscoverOutput
	decodeJSON(t, r.stdout, &out)
	var hosts []string
	for _, dev := range out.Devices {
		hosts = append(hosts, dev.IP)
	}
	if len(hosts) != 2 || hosts[0] != "192.0.2.9" || hosts[1] != "192.0.2.10" {
		t.Fatalf("devices %v, want 192.0.2.9 and 192.0.2.10", hosts)
	}
	var seen []string
	for _, req := range px.Requests() {
		if req.Port != 502 {
			t.Errorf("proxy saw port %d, want 502", req.Port)
		}
		seen = append(seen, req.Host)
	}
	sort.Strings(seen)
	if len(seen) != 2 || seen[0] != "192.0.2.10" || seen[1] != "192.0.2.9" {
		t.Fatalf("proxy saw %v", seen)
	}
}

func TestDiscover_sourceAddr(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(0, 1)
	port := servePort(t, d)
	r := run(t, "discover", "--subnets", "127.0.0.1/32", "--port", port, "--source-addr", "127.0.0.1")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "Modbus device found at "+net.JoinHostPort("127.0.0.1", port))
}

func TestDiscover_dialInvalid(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(0, 1)
	port := servePort(t, d)
	for _, args := range [][]string{
		{"--proxy", "http://127.0.0.1:1"},
		{"--proxy", "socks5://127.0.0.1"},
		{"--source-addr", "eth0"},
		{"--proxy", "socks5://127.0.0.1:1080", "--resolve-mac", "--interface", "lo0"},
	} {
		r := run(t, append([]string{"discover", "--subnets", "127.0.0.1/32", "--port", port}, args...)...)
		wantKind(t, r, errs.KindInvalidInput)
	}
	if n := len(d.Requests()); n != 0 {
		t.Fatalf("validation failures must not probe, device saw %d requests", n)
	}
	r := run(t, "discover", "--subnets", "127.0.0.1/32", "--port", port, "--proxy", "socks5://127.0.0.1:1080", "--resolve-mac", "--interface", "lo0")
	wantContains(t, r.err.Error(), "--resolve-mac", "--proxy")
}
