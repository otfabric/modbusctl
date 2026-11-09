// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/socks5test"
	"github.com/spf13/cobra"
)

// unroutable is a TEST-NET-1 address (RFC 5737): a URL naming it reaches the device only
// through a proxy that forwards it there.
const unroutable = "192.0.2.10:502"

// forwardingProxy is a test proxy that sends every CONNECT to the device at addr.
func forwardingProxy(t *testing.T, addr string) *socks5test.Server {
	t.Helper()
	px := socks5test.Start(t)
	px.Forward = func(string) string { return addr }
	return px
}

// dialingCommands are the client commands that connect to a device, each with the
// arguments of a successful run against newDevice; url is the --url value.
func dialingCommands(t *testing.T, url string) map[string][]string {
	t.Helper()
	tmp := t.TempDir()
	blocks := filepath.Join(tmp, "blocks.json")
	if err := os.WriteFile(blocks, []byte(`[{"start_address":100,"register_count":3}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	return map[string][]string{
		"read":           {"client", "read", "--url", url, "--start", "100", "--count", "3", "--output", filepath.Join(tmp, "read.mbcap")},
		"decode":         {"client", "decode", "--url", url, "--start", "103", "--codec", "float32/layout:4321"},
		"write":          {"client", "write", "--yes", "--url", url, "--start", "200", "--values", "7"},
		"poll":           {"client", "poll", "--url", url, "--start", "100", "--count", "3", "--polls", "2", "--interval", "1"},
		"record":         {"client", "record", "--url", url, "--blocks-file", blocks, "--interval", "10", "--duration", "25", "--output", filepath.Join(tmp, "rec.mbcap")},
		"scan":           {"client", "scan", "--url", url, "--start", "100", "--end", "104", "--output", filepath.Join(tmp, "scan.mbcap")},
		"identify":       {"client", "identify", "--url", url},
		"reportserverid": {"client", "reportserverid", "--url", url},
		"fingerprint":    {"client", "fingerprint", "--url", url},
		"diagnostic":     {"client", "diagnostic", "--url", url},
		"sunspec":        {"client", "sunspec", "probe", "--url", url},
	}
}

// Every client command that takes --url takes --proxy and --source-addr too.
func TestDial_flagsOnEveryDialingCommand(t *testing.T) {
	n := 0
	walkCommands(ClientCmd, func(c *cobra.Command) {
		if c.Flags().Lookup("url") == nil {
			return
		}
		n++
		for _, f := range []string{"proxy", "source-addr"} {
			if c.Flags().Lookup(f) == nil {
				t.Errorf("%s has --url but no --%s", c.CommandPath(), f)
			}
		}
	})
	if n < 10 {
		t.Fatalf("only %d commands take --url", n)
	}
	r := run(t, "client", "read", "--help")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "--proxy", "MODBUSCTL_PROXY", "--source-addr", "MODBUSCTL_SOURCE_ADDR")
}

// Each command reaches the device only through the proxy: the URL names an address the
// proxy alone forwards to the device.
func TestDial_proxyEveryCommand(t *testing.T) {
	d, url := newDevice(t)
	px := forwardingProxy(t, strings.TrimPrefix(url, "tcp://"))
	for name, args := range dialingCommands(t, "tcp://"+unroutable) {
		t.Run(name, func(t *testing.T) {
			before := len(px.Requests())
			r := run(t, append(args, "--proxy", px.URL())...)
			if r.exit() != errs.ExitOK && r.exit() != errs.ExitPartial {
				t.Fatalf("exit %d: %v\n%s", r.exit(), r.err, r.stderr)
			}
			reqs := px.Requests()
			if len(reqs) == before {
				t.Fatal("the command did not connect through the proxy")
			}
			if got := reqs[len(reqs)-1]; got.AddrType != 1 || got.Host != "192.0.2.10" || got.Port != 502 {
				t.Fatalf("proxy saw %+v, want 192.0.2.10:502", got)
			}
		})
	}
	if len(d.Requests()) == 0 {
		t.Fatal("the device saw no request")
	}
}

// The proxy carries every TCP transport; with socks5h the host name goes to the proxy.
func TestDial_proxyTransports(t *testing.T) {
	d, _ := newDevice(t)
	for scheme, addr := range map[string]string{
		"tcp":          d.ServeTCP(t),
		"rtuovertcp":   d.ServeRTUOverTCP(t),
		"asciiovertcp": d.ServeASCIIOverTCP(t),
	} {
		t.Run(scheme, func(t *testing.T) {
			px := forwardingProxy(t, addr)
			r := run(t, "client", "read", "--url", scheme+"://plc.invalid:502", "--start", "100", "--count", "3",
				"--output", filepath.Join(t.TempDir(), "x.mbcap"), "--proxy", "socks5h://"+px.Addr)
			wantExit(t, r, errs.ExitOK)
			if reqs := px.Requests(); len(reqs) != 1 || reqs[0].AddrType != 3 || reqs[0].Host != "plc.invalid" {
				t.Fatalf("proxy saw %+v, want the name plc.invalid", reqs)
			}
		})
	}
}

// tcp+tls through the proxy: the handshake runs end to end and checks the certificate
// against the URL host, not the proxy.
func TestDial_proxyTLS(t *testing.T) {
	d, _ := newDevice(t)
	pki := mbtest.NewPKI(t)
	_, port, _ := net.SplitHostPort(d.ServeTLS(t, pki))
	px := forwardingProxy(t, net.JoinHostPort("127.0.0.1", port))
	read := func(host string) cmdResult {
		return run(t, append([]string{"client", "read", "--url", "tcp+tls://" + host + ":" + port, "--start", "100",
			"--output", filepath.Join(t.TempDir(), "x.mbcap"), "--proxy", "socks5h://" + px.Addr}, tlsFlags(pki)...)...)
	}
	wantExit(t, read("localhost"), errs.ExitOK)
	// The certificate names localhost and 127.0.0.1, not this host: refused, although the
	// proxy delivered the connection to the same device.
	r := read("plc.invalid")
	wantKind(t, r, errs.KindTransport)
	wantContains(t, r.err.Error(), "certificate")
}

func TestDial_refusingProxy(t *testing.T) {
	d, url := newDevice(t)
	px := socks5test.Start(t)
	px.Reply = 2
	out := filepath.Join(t.TempDir(), "x.mbcap")
	r := run(t, "client", "read", "--url", url, "--start", "100", "--output", out, "--proxy", px.URL())
	wantKind(t, r, errs.KindTransport)
	wantExit(t, r, errs.ExitTransport)
	wantContains(t, r.err.Error(), "socks5 proxy "+px.Addr, "socks5: connection not allowed by ruleset")
	if len(d.Requests()) != 0 {
		t.Fatal("a refused connection reached the device")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("a refused connection left %s behind: %v", out, err)
	}
}

func TestDial_sourceAddr(t *testing.T) {
	d, url := newDevice(t)
	udp := "udp://" + d.ServeUDP(t)
	for _, u := range []string{url, udp, "rtuovertcp://" + d.ServeRTUOverTCP(t)} {
		r := run(t, "client", "read", "--url", u, "--start", "100", "--output", filepath.Join(t.TempDir(), "x.mbcap"), "--source-addr", "127.0.0.1")
		wantExit(t, r, errs.ExitOK)
	}
	// With the proxy, the connection to the proxy is made from the source address.
	px := forwardingProxy(t, strings.TrimPrefix(url, "tcp://"))
	r := run(t, "client", "read", "--url", "tcp://"+unroutable, "--start", "100", "--output", filepath.Join(t.TempDir(), "x.mbcap"),
		"--source-addr", "127.0.0.1", "--proxy", px.URL())
	wantExit(t, r, errs.ExitOK)
	// A source address this host does not have: the bind fails, a transport error.
	for _, u := range []string{url, udp} {
		r = run(t, "client", "read", "--url", u, "--start", "100", "--output", filepath.Join(t.TempDir(), "x.mbcap"), "--source-addr", "192.0.2.77")
		wantKind(t, r, errs.KindTransport)
	}
}

// Invalid values and transports the flags do not apply to are refused before anything is
// sent, on every command.
func TestDial_invalid(t *testing.T) {
	d, url := newDevice(t)
	udp := "udp://" + d.ServeUDP(t)
	cases := []struct {
		url, flag, value, want string
	}{
		{url, "--proxy", "http://127.0.0.1:8080", "the scheme must be socks5 or socks5h"},
		{url, "--proxy", "socks5://user:pw@127.0.0.1:1080", "proxy credentials are not supported"},
		{url, "--proxy", "socks5://127.0.0.1", "want socks5://host:port"},
		{url, "--source-addr", "eth0", "invalid --source-addr"},
		{udp, "--proxy", "socks5://127.0.0.1:1080", "--proxy does not apply to udp://"},
		{"rtuoverudp://" + d.ServeUDP(t), "--proxy", "socks5://127.0.0.1:1080", "--proxy does not apply to rtuoverudp://"},
		{"rtu:///dev/null", "--proxy", "socks5://127.0.0.1:1080", "do not apply to rtu://"},
		{"rtu:///dev/null", "--source-addr", "127.0.0.1", "do not apply to rtu://"},
		{"ascii:///dev/null", "--source-addr", "127.0.0.1", "do not apply to ascii://"},
	}
	for _, tc := range cases {
		for name, args := range dialingCommands(t, tc.url) {
			r := run(t, append(args, tc.flag, tc.value)...)
			e, ok := errs.As(r.err)
			if !ok || e.Kind != errs.KindInvalidInput || !strings.Contains(r.err.Error(), tc.want) {
				t.Errorf("%s %s %s %s: exit %d, %v; want invalid input mentioning %q", name, tc.url, tc.flag, tc.value, r.exit(), r.err, tc.want)
			}
		}
	}
	if n := len(d.Requests()); n != 0 {
		t.Fatalf("validation failures must not reach the device, saw %d requests", n)
	}
}
