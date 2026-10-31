// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/modbus"
)

func serverTLSFlags(p *mbtest.PKI) []string {
	return []string{"--tls-cert", p.ServerCertFile, "--tls-key", p.ServerKeyFile, "--tls-ca", p.CAFile}
}

func clientTLS(p *mbtest.PKI) config.TLSConfig {
	return config.TLSConfig{TLSCert: p.ClientCertFile, TLSKey: p.ClientKeyFile, TLSCA: p.CAFile}
}

// recordTLS records holding registers 100-102 of a device served over tcp+tls whose
// register 100 counts the iterations, and returns the capture and the last count.
func recordTLS(t *testing.T, p *mbtest.PKI) (string, uint16) {
	t.Helper()
	d := mbtest.NewDevice().SetHolding(100, 0, 0xBEEF, 0x1234)
	var n uint16
	d.OnRequest = func(mbtest.Request) mbtest.Action {
		n++
		d.SetHolding(100, n)
		return mbtest.Action{}
	}
	tmp := t.TempDir()
	blocks := filepath.Join(tmp, "blocks.json")
	if err := os.WriteFile(blocks, []byte(`[{"start_address":100,"register_count":3}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "capture.mbcap")
	res, err := modbus.RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: config.DeviceConfig{URL: "tcp+tls://" + d.ServeTLS(t, p), Unit: 1, TLSConfig: clientTLS(p)},
		Function:     3, BlocksFile: blocks, Interval: 10, Duration: 60, OutputFile: out,
	}, io.Discard)
	if err != nil {
		t.Fatalf("record capture over tcp+tls: %v", err)
	}
	if res.Iterations < 2 {
		t.Fatalf("expected at least two iterations, got %+v", res)
	}
	return out, uint16(res.Iterations)
}

// readTLS reads holding registers 100-102 from the server with the modbusctl TLS client.
func readTLS(s *server, tlsCfg config.TLSConfig) (string, error) {
	res, err := modbus.CollectRead(context.Background(), config.ReadConfig{
		DeviceConfig: config.DeviceConfig{URL: "tcp+tls://127.0.0.1:" + s.port, Unit: 1, TLSConfig: tlsCfg},
		Timeout:      2000, Function: 3, StartAddress: 100, RegisterCount: 3,
	}, nil)
	if err != nil {
		return "", err
	}
	return res.RawDataHex, nil
}

// hexWords formats registers like a read result's raw data.
func hexWords(words ...uint16) string {
	var b strings.Builder
	for i, w := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%02X %02X", byte(w>>8), byte(w))
	}
	return b.String()
}

func TestTLS_help(t *testing.T) {
	for _, sub := range []string{"static", "replay"} {
		r := run(t, "server", sub, "--help")
		wantExit(t, r, errs.ExitOK)
		wantContains(t, r.stdout, "--tls-cert", "MODBUSCTL_TLS_CERT", "--tls-key", "MODBUSCTL_TLS_KEY", "--tls-ca", "MODBUSCTL_TLS_CA", "tcp+tls")
	}
}

// Record over tcp+tls, serve the capture over tcp+tls with `server static` and `server
// replay`, and read it back with the modbusctl TLS client.
func TestTLS_staticAndReplay(t *testing.T) {
	pki := mbtest.NewPKI(t)
	in, iterations := recordTLS(t, pki)
	last := hexWords(iterations, 0xBEEF, 0x1234)

	s := startServer(t, append([]string{"static", "--input", in}, serverTLSFlags(pki)...)...)
	got, err := readTLS(s, clientTLS(pki))
	if err != nil || got != last {
		t.Fatalf("static over tcp+tls: %q, %v; want %q", got, err, last)
	}
	if r := s.stop(t); r.err != nil {
		t.Fatalf("an interrupted server must exit cleanly, got %v", r.err)
	}
	wantContains(t, s.stderr.String(), "Serving Modbus TCP Security (tcp+tls, client certificates required) on 127.0.0.1 port "+s.port)

	s = startServer(t, append([]string{"replay", "--input", in, "--loops", "1", "--speed", "4"}, serverTLSFlags(pki)...)...)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := readTLS(s, clientTLS(pki))
		if err != nil {
			t.Fatalf("replay over tcp+tls: %v", err)
		}
		if got == last {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replay over tcp+tls never reached %q, last served %q", last, got)
		}
		time.Sleep(time.Millisecond)
	}
	if r := s.stop(t); r.err != nil {
		t.Fatalf("an interrupted server must exit cleanly, got %v", r.err)
	}
	wantContains(t, s.stderr.String(), "Serving Modbus TCP Security (tcp+tls, client certificates required) on 127.0.0.1 port "+s.port)
}

// A client whose certificate another CA signed is rejected; so is a plain Modbus TCP client.
func TestTLS_untrustedClientRejected(t *testing.T) {
	pki, other := mbtest.NewPKI(t), mbtest.NewPKI(t)
	in, _ := recordTLS(t, pki)
	for _, sub := range []string{"static", "replay"} {
		s := startServer(t, append([]string{sub, "--input", in}, serverTLSFlags(pki)...)...)
		if _, err := readTLS(s, config.TLSConfig{TLSCert: other.ClientCertFile, TLSKey: other.ClientKeyFile, TLSCA: pki.CAFile}); err == nil {
			t.Fatalf("server %s answered a client certificate of another CA", sub)
		}
		if _, err := readHolding(t, s.dial(t), 1, 100, 1); err == nil {
			t.Fatalf("server %s answered a plain Modbus TCP client", sub)
		}
		if r := s.stop(t); r.err != nil {
			t.Fatalf("an interrupted server must exit cleanly, got %v", r.err)
		}
		wantContains(t, s.stderr.String(), "📊 Served 0 requests")
	}
}

// Incomplete or unreadable server TLS files fail validation before the server binds.
func TestTLS_invalidServerFlags(t *testing.T) {
	pki := mbtest.NewPKI(t)
	in, _ := recordTLS(t, pki)
	port := freePort(t)
	for _, sub := range []string{"static", "replay"} {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"--tls-cert", pki.ServerCertFile, "--tls-key", pki.ServerKeyFile}, "set --tls-ca"},
			{[]string{"--tls-ca", pki.CAFile}, "set --tls-cert"},
			{[]string{"--tls-cert", pki.ServerCertFile, "--tls-ca", pki.CAFile}, "load TLS server certificate"},
			{[]string{"--tls-cert", pki.ServerCertFile, "--tls-key", pki.ServerKeyFile, "--tls-ca", filepath.Join(t.TempDir(), "nope.pem")}, "load TLS client CA certificates"},
		} {
			r := run(t, append([]string{"server", sub, "--input", in, "--port", port}, tc.args...)...)
			wantExit(t, r, errs.ExitInvalidInput)
			wantContains(t, r.err.Error(), tc.want)
			if r.stdout != "" {
				t.Fatalf("server %s printed before failing validation:\n%s", sub, r.stdout)
			}
		}
	}
}
