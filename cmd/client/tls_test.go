// SPDX-License-Identifier: MIT

package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/spf13/cobra"
)

// newTLSDevice serves the device of newDevice over tcp+tls and returns it, its URL and
// the PKI whose files the client flags take.
func newTLSDevice(t *testing.T) (*mbtest.Device, string, *mbtest.PKI) {
	t.Helper()
	d, _ := newDevice(t)
	pki := mbtest.NewPKI(t)
	return d, "tcp+tls://" + d.ServeTLS(t, pki), pki
}

func tlsFlags(p *mbtest.PKI) []string {
	return []string{"--tls-cert", p.ClientCertFile, "--tls-key", p.ClientKeyFile, "--tls-ca", p.CAFile}
}

// Every client command that takes --url takes the TLS flags too.
func TestTLS_flagsOnEveryDialingCommand(t *testing.T) {
	n := 0
	walkCommands(ClientCmd, func(c *cobra.Command) {
		if c.Flags().Lookup("url") == nil {
			return
		}
		n++
		for _, f := range []string{"tls-cert", "tls-key", "tls-ca"} {
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
	wantContains(t, r.stdout, "tcp+tls://", "--tls-cert", "MODBUSCTL_TLS_CERT", "--tls-key", "MODBUSCTL_TLS_KEY", "--tls-ca", "MODBUSCTL_TLS_CA")
}

func TestTLS_readAndRecord(t *testing.T) {
	d, url, pki := newTLSDevice(t)
	tmp := t.TempDir()

	out := filepath.Join(tmp, "read.mbcap")
	r := run(t, append([]string{"client", "read", "--url", url, "--start", "100", "--count", "3", "--output", out, "--format", "json"}, tlsFlags(pki)...)...)
	wantExit(t, r, errs.ExitOK)
	var res types.ReadResult
	decodeJSON(t, r.stdout, &res)
	if res.Target != url || res.RawDataHex != "00 01 00 02 00 03" {
		t.Fatalf("unexpected read result: %+v", res)
	}
	wantFile(t, out)

	blocks := filepath.Join(tmp, "blocks.json")
	if err := os.WriteFile(blocks, []byte(`[{"start_address":100,"register_count":3}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	before := d.RequestCount(0x03)
	out = filepath.Join(tmp, "rec.mbcap")
	r = run(t, append([]string{"client", "record", "--url", url, "--blocks-file", blocks, "--interval", "10", "--duration", "25", "--output", out, "--format", "json"}, tlsFlags(pki)...)...)
	wantExit(t, r, errs.ExitOK)
	var rec types.RecordSummaryResult
	decodeJSON(t, r.stdout, &rec)
	if rec.Iterations < 1 || d.RequestCount(0x03)-before != rec.BlocksRecorded {
		t.Fatalf("unexpected record result: %+v", rec)
	}
	wantFile(t, out)

	// Certificate and key in one file.
	r = run(t, "client", "scan", "--url", url, "--start", "100", "--end", "104", "--output", filepath.Join(tmp, "scan.mbcap"),
		"--tls-cert", pki.ClientPEMFile, "--tls-ca", pki.CAFile, "--format", "json")
	wantExit(t, r, errs.ExitOK)
}

func TestTLS_failures(t *testing.T) {
	d, url, pki := newTLSDevice(t)
	other := mbtest.NewPKI(t)
	out := filepath.Join(t.TempDir(), "x.mbcap")
	read := func(extra ...string) cmdResult {
		return run(t, append([]string{"client", "read", "--url", url, "--start", "100", "--output", out}, extra...)...)
	}

	// No certificate, or files that cannot be used: invalid input, nothing sent.
	r := read("--tls-ca", pki.CAFile)
	wantKind(t, r, errs.KindInvalidInput)
	wantExit(t, r, errs.ExitInvalidInput)
	wantContains(t, r.err.Error(), "--tls-cert")
	wantKind(t, read("--tls-cert", pki.ClientCertFile, "--tls-ca", pki.CAFile), errs.KindInvalidInput)
	wantKind(t, read("--tls-cert", pki.ClientCertFile, "--tls-key", other.ClientKeyFile, "--tls-ca", pki.CAFile), errs.KindInvalidInput)
	wantKind(t, read("--tls-cert", pki.ClientPEMFile, "--tls-ca", filepath.Join(t.TempDir(), "none.pem")), errs.KindInvalidInput)
	if n := len(d.Requests()); n != 0 {
		t.Fatalf("validation failures must not reach the device, saw %d requests", n)
	}

	// A server the CA does not vouch for: a transport error, and no capture.
	r = read("--tls-cert", pki.ClientPEMFile, "--tls-ca", other.CAFile)
	wantKind(t, r, errs.KindTransport)
	wantExit(t, r, errs.ExitTransport)
	wantContains(t, r.err.Error(), "certificate")
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("a refused TLS connection left %s behind: %v", out, err)
	}
	// A client certificate the server does not trust. With TLS 1.3 the server reports it
	// after the client finished its handshake, so the first request fails.
	r = read("--tls-cert", other.ClientPEMFile, "--tls-ca", pki.CAFile)
	wantKind(t, r, errs.KindTransport)
	wantContains(t, r.err.Error(), "certificate")
	if n := len(d.Requests()); n != 0 {
		t.Fatalf("untrusted connections must not reach the device, saw %d requests", n)
	}
}
