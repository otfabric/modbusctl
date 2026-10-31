// SPDX-License-Identifier: MIT

package modbus

// End-to-end tests of the static and replay servers over Modbus/TCP Security:
// a capture recorded over tcp+tls is served over tcp+tls and read back with
// the modbusctl TLS client; clients without a certificate the CA signed are
// rejected, and bad server TLS files fail before the server binds.

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/stretchr/testify/require"
)

// tlsProgress is a progress writer safe to read while the server writes it.
type tlsProgress struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (p *tlsProgress) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.Write(b)
}

func (p *tlsProgress) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.String()
}

// e2eRecordTLSDevice records holding registers 100-103 and FC17 of an
// e2eTLSDevice served by go-modbus over tcp+tls, and returns the capture and
// the register 100 values recorded (one per iteration, counting up).
func e2eRecordTLSDevice(t *testing.T, pki *mbtest.PKI) (string, []uint16) {
	t.Helper()
	url := e2eTLSServe(t, pki, &e2eTLSDevice{})
	dir := t.TempDir()
	bf := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(bf, []byte(`[{"function":3,"start_address":100,"register_count":4},{"function":17}]`), 0o600))
	rec := filepath.Join(dir, "tls.mbcap")
	sum, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: config.DeviceConfig{URL: url, Unit: 1, TLSConfig: tlsClient(pki)},
		Timeout:      coreTimeoutMS, Interval: 20, Duration: 300, BlocksFile: bf, OutputFile: rec,
	}, nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sum.Iterations, uint32(2))
	l := capfiletest.Load(t, rec)
	require.Equal(t, capture.TransportTLS, l.Targets[0].Transport)
	counter := e2eCounterValues(t, l)
	require.Len(t, counter, int(sum.Iterations))
	return rec, counter
}

// tlsRead reads holding registers 100-103 from url with the modbusctl client.
func tlsRead(t *testing.T, url string, tlsCfg config.TLSConfig, out string) (string, error) {
	t.Helper()
	res, err := CollectRead(context.Background(), config.ReadConfig{
		DeviceConfig: config.DeviceConfig{URL: url, Unit: 1, TLSConfig: tlsCfg},
		Timeout:      coreTimeoutMS, Function: 3, StartAddress: 100, RegisterCount: 4, OutputFile: out,
	}, nil)
	if err != nil {
		return "", err
	}
	return res.RawDataHex, nil
}

func counterHex(v uint16) string {
	return fmt.Sprintf("%02X %02X BE EF 12 34 00 42", byte(v>>8), byte(v))
}

// Record over tcp+tls, replay over tcp+tls with `server replay`, and read the
// replay back with the TLS client: the served values walk through the
// recorded ones and end on the last.
func TestE2ETLS_serverReplay(t *testing.T) {
	pki := mbtest.NewPKI(t)
	rec, counter := e2eRecordTLSDevice(t, pki)
	recorded := map[string]bool{}
	for _, v := range counter {
		recorded[counterHex(v)] = true
	}
	last := counterHex(counter[len(counter)-1])

	progress := &tlsProgress{}
	rs := collStartServer(t, func(ctx context.Context, a collAddr) error {
		return ServeReplay(ctx, config.ReplayServerConfig{
			ServerTLSConfig: tlsServer(pki), Port: a.Port, Listen: a.Listen, InputFile: rec, Speed: "4", Loops: 1,
		}, a.Progress(progress))
	})
	require.Contains(t, progress.String(), fmt.Sprintf("🚀 Serving Modbus TCP Security (tcp+tls, client certificates required) on 127.0.0.1 port %d", rs.port))

	var served []string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		got, err := tlsRead(t, rs.tlsURL(), tlsClient(pki), "")
		require.NoError(t, err)
		require.True(t, recorded[got], "replay served %s, which was never recorded", got)
		if len(served) == 0 || served[len(served)-1] != got {
			served = append(served, got)
		}
		if got == last {
			break
		}
	}
	require.Equal(t, last, served[len(served)-1], "replay never reached the last value: %v", served)

	// A read of the replay recorded with the TLS client says it was over TLS.
	out := filepath.Join(t.TempDir(), "read.mbcap")
	got, err := tlsRead(t, rs.tlsURL(), tlsClient(pki), out)
	require.NoError(t, err)
	require.Equal(t, last, got)
	rl := capfiletest.Load(t, out)
	require.Equal(t, capture.TransportTLS, rl.Targets[0].Transport)
	require.Equal(t, rs.tlsURL(), rl.Targets[0].Tags["url"])

	require.ErrorIs(t, rs.stop(t), context.Canceled)
	require.Contains(t, progress.String(), "📊 Served ")
}

// Record over tcp+tls and serve the last values over tcp+tls with `server
// static`; FC17 answers the recorded response.
func TestE2ETLS_serverStatic(t *testing.T) {
	pki := mbtest.NewPKI(t)
	rec, counter := e2eRecordTLSDevice(t, pki)
	progress := &tlsProgress{}
	ss := collStartServer(t, func(ctx context.Context, a collAddr) error {
		return ServeStatic(ctx, config.StaticServerConfig{ServerTLSConfig: tlsServer(pki), Port: a.Port, Listen: a.Listen, InputFile: rec}, a.Progress(progress))
	})
	got, err := tlsRead(t, ss.tlsURL(), tlsClient(pki), "")
	require.NoError(t, err)
	require.Equal(t, counterHex(counter[len(counter)-1]), got)

	// Certificate and key in one file, on the client side.
	got, err = tlsRead(t, ss.tlsURL(), config.TLSConfig{TLSCert: pki.ClientPEMFile, TLSCA: pki.CAFile}, "")
	require.NoError(t, err)
	require.Equal(t, counterHex(counter[len(counter)-1]), got)

	id, err := CollectReportServerID(context.Background(), config.ReportServerIDConfig{UnitClientConfig: config.UnitClientConfig{
		URL: ss.tlsURL(), UnitID: "1", Timeout: coreTimeoutMS, TLSConfig: tlsClient(pki),
	}})
	require.NoError(t, err)
	require.Nil(t, id.Units[0].Error)
	require.ErrorIs(t, ss.stop(t), context.Canceled)
	require.Contains(t, progress.String(), "Serving Modbus TCP Security (tcp+tls, client certificates required) on 127.0.0.1 port")
}

// The server key may sit in the certificate file.
func TestE2ETLS_serverKeyInCertFile(t *testing.T) {
	pki := mbtest.NewPKI(t)
	rec, counter := e2eRecordTLSDevice(t, pki)
	cert, err := os.ReadFile(pki.ServerCertFile)
	require.NoError(t, err)
	key, err := os.ReadFile(pki.ServerKeyFile)
	require.NoError(t, err)
	bundle := filepath.Join(t.TempDir(), "server-bundle.pem")
	require.NoError(t, os.WriteFile(bundle, append(cert, key...), 0o600))
	ss := collStartServer(t, func(ctx context.Context, a collAddr) error {
		return ServeStatic(ctx, config.StaticServerConfig{
			ServerTLSConfig: config.ServerTLSConfig{TLSCert: bundle, TLSCA: pki.CAFile}, Port: a.Port, Listen: a.Listen, InputFile: rec,
		}, a.Progress(nil))
	})
	got, err := tlsRead(t, ss.tlsURL(), tlsClient(pki), "")
	require.NoError(t, err)
	require.Equal(t, counterHex(counter[len(counter)-1]), got)
}

// A tcp+tls server answers only clients with a certificate its --tls-ca
// signed: a client certificate of another CA, a plain Modbus TCP client and a
// client that does not trust the server get nothing.
func TestE2ETLS_serverRejectsUntrustedClients(t *testing.T) {
	pki, other := mbtest.NewPKI(t), mbtest.NewPKI(t)
	rec, _ := e2eRecordTLSDevice(t, pki)
	for _, tc := range []struct {
		name  string
		serve func(ctx context.Context, a collAddr, progress *tlsProgress) error
	}{
		{"static", func(ctx context.Context, a collAddr, progress *tlsProgress) error {
			return ServeStatic(ctx, config.StaticServerConfig{ServerTLSConfig: tlsServer(pki), Port: a.Port, Listen: a.Listen, InputFile: rec}, a.Progress(progress))
		}},
		{"replay", func(ctx context.Context, a collAddr, progress *tlsProgress) error {
			return ServeReplay(ctx, config.ReplayServerConfig{ServerTLSConfig: tlsServer(pki), Port: a.Port, Listen: a.Listen, InputFile: rec}, a.Progress(progress))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			progress := &tlsProgress{}
			s := collStartServer(t, func(ctx context.Context, a collAddr) error { return tc.serve(ctx, a, progress) })

			t.Run("client certificate of another CA", func(t *testing.T) {
				_, err := tlsRead(t, s.tlsURL(), config.TLSConfig{TLSCert: other.ClientCertFile, TLSKey: other.ClientKeyFile, TLSCA: pki.CAFile}, "")
				require.Error(t, err)
				require.NotErrorIs(t, err, gomodbus.ErrRequestTimedOut, "rejected, not timed out")
			})
			t.Run("server certificate as a client certificate", func(t *testing.T) {
				// Signed by the CA, but not for client authentication.
				_, err := tlsRead(t, s.tlsURL(), config.TLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ServerKeyFile, TLSCA: pki.CAFile}, "")
				require.Error(t, err)
				require.NotErrorIs(t, err, gomodbus.ErrRequestTimedOut, "rejected, not timed out")
			})
			t.Run("client that does not trust the server", func(t *testing.T) {
				_, err := tlsRead(t, s.tlsURL(), config.TLSConfig{TLSCert: pki.ClientCertFile, TLSKey: pki.ClientKeyFile, TLSCA: other.CAFile}, "")
				require.ErrorIs(t, err, ErrTCPConnection)
				var unknown x509.UnknownAuthorityError
				require.ErrorAs(t, err, &unknown)
			})
			t.Run("plain Modbus TCP client", func(t *testing.T) {
				_, err := tlsRead(t, fmt.Sprintf("tcp://127.0.0.1:%d", s.port), config.TLSConfig{}, "")
				require.Error(t, err)
			})

			require.ErrorIs(t, s.stop(t), context.Canceled)
			require.Contains(t, progress.String(), "📊 Served 0 requests", "no untrusted client reached the handler")
		})
	}
}

// Bad server TLS settings fail as invalid input before the server binds.
func TestE2ETLS_serverBadFiles(t *testing.T) {
	pki, other := mbtest.NewPKI(t), mbtest.NewPKI(t)
	rec, _ := e2eRecordTLSDevice(t, pki)
	empty := filepath.Join(t.TempDir(), "empty.pem")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	missing := filepath.Join(t.TempDir(), "nope.pem")

	for _, tc := range []struct {
		name string
		tls  config.ServerTLSConfig
		want string
	}{
		{"no client CA", config.ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ServerKeyFile}, "requires client certificates: set --tls-ca"},
		{"CA only", config.ServerTLSConfig{TLSCA: pki.CAFile}, "needs a server certificate: set --tls-cert"},
		{"key only", config.ServerTLSConfig{TLSKey: pki.ServerKeyFile, TLSCA: pki.CAFile}, "set --tls-cert"},
		{"certificate without key", config.ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSCA: pki.CAFile}, "load TLS server certificate"},
		{"missing certificate file", config.ServerTLSConfig{TLSCert: missing, TLSCA: pki.CAFile}, "no such file"},
		{"key of another certificate", config.ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: other.ServerKeyFile, TLSCA: pki.CAFile}, "private key does not match"},
		{"missing CA file", config.ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ServerKeyFile, TLSCA: missing}, "load TLS client CA certificates"},
		{"empty CA file", config.ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ServerKeyFile, TLSCA: empty}, "empty file"},
		{"CA file without certificates", config.ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ServerKeyFile, TLSCA: pki.ServerKeyFile}, "no certificate found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var progress bytes.Buffer
			err := ServeStatic(context.Background(), config.StaticServerConfig{ServerTLSConfig: tc.tls, Port: 1, InputFile: rec}, &progress)
			coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
			require.ErrorContains(t, err, tc.want)
			err = ServeReplay(context.Background(), config.ReplayServerConfig{ServerTLSConfig: tc.tls, Port: 1, InputFile: rec}, &progress)
			coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
			require.ErrorContains(t, err, tc.want)
			require.Empty(t, progress.String(), "nothing printed before the TLS files are loaded")
		})
	}
}
