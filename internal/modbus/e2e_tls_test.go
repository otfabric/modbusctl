package modbus

// End-to-end tests of Modbus/TCP Security (tcp+tls://): the client presents the
// certificate of --tls-cert/--tls-key and checks the server against --tls-ca.
// A go-modbus tcp+tls server is recorded and the recording replayed, every
// client command runs over TLS, and the client refuses a server it does not
// trust and fails cleanly without a certificate or with one the server does
// not trust.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
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
	"github.com/otfabric/modbusctl/internal/replay"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tlsClient returns the client TLS settings for a PKI: its client key pair and CA.
func tlsClient(p *mbtest.PKI) config.TLSConfig {
	return config.TLSConfig{TLSCert: p.ClientCertFile, TLSKey: p.ClientKeyFile, TLSCA: p.CAFile}
}

// tlsDevice serves d over tcp+tls with the PKI and returns the device config to reach it.
func tlsDevice(t *testing.T, p *mbtest.PKI, d *mbtest.Device) config.DeviceConfig {
	t.Helper()
	return config.DeviceConfig{URL: "tcp+tls://" + d.ServeTLS(t, p), Unit: 1, TLSConfig: tlsClient(p)}
}

// e2eTLSServe serves h with a go-modbus tcp+tls server on a free port: the
// server presents the PKI's server certificate and accepts clients whose
// certificate the PKI signed. It returns the server URL.
func e2eTLSServe(t *testing.T, p *mbtest.PKI, h gomodbus.RequestHandler) string {
	t.Helper()
	var url string
	collListen(t, func(port uint16) error {
		url = fmt.Sprintf("tcp+tls://127.0.0.1:%d", port)
		srv, err := gomodbus.NewServer(&gomodbus.ServerConfig{
			URL: url, TLSServerCert: &p.ServerCert, TLSClientCAs: p.Pool, Logger: gomodbus.NopLogger(),
		}, h)
		require.NoError(t, err)
		if err := srv.Start(); err != nil {
			return err
		}
		t.Cleanup(func() { _ = srv.Stop() })
		return nil
	})
	return url
}

// e2eTLSDevice answers FC08, FC17, FC20 and FC24 like e2eFunctionsDevice, and
// holding registers 100-103: register 100 counts the reads of it, the others
// are constant.
type e2eTLSDevice struct {
	e2eFunctionsDevice
	mu    sync.Mutex
	reads uint16
}

func (d *e2eTLSDevice) HandleHoldingRegisters(_ context.Context, req *gomodbus.HoldingRegistersRequest) ([]uint16, error) {
	if req.IsWrite || req.Addr < 100 || int(req.Addr)+int(req.Quantity) > 104 {
		return nil, gomodbus.ErrIllegalDataAddress
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	regs := []uint16{0, 0xBEEF, 0x1234, 0x0042}
	if req.Addr == 100 {
		d.reads++
		regs[0] = d.reads
	}
	return regs[req.Addr-100 : req.Addr-100+req.Quantity], nil
}

// Record a go-modbus tcp+tls server, then replay the recording: over plain
// Modbus TCP with `server replay`, and over tcp+tls from a go-modbus server
// that answers from the capture, read back with the TLS client.
func TestE2ETLS_recordAndReplay(t *testing.T) {
	pki := mbtest.NewPKI(t)
	dev := &e2eTLSDevice{}
	url := e2eTLSServe(t, pki, dev)

	dir := t.TempDir()
	plan := []map[string]any{
		{"function": 3, "start_address": 100, "register_count": 4},
		{"function": 8, "sub_function": 0, "data": "A537"},
		{"function": 17},
		{"function": 20, "file_records": []map[string]int{{"file": 1, "record": 2, "length": 3}}},
		{"function": 24, "fifo_address": 1234},
	}
	data, err := json.Marshal(plan)
	require.NoError(t, err)
	bf := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(bf, data, 0o600))
	rec := filepath.Join(dir, "tls.mbcap")
	sum, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: config.DeviceConfig{URL: url, Unit: 1, TLSConfig: tlsClient(pki)},
		Timeout:      coreTimeoutMS, Interval: 20, Duration: 300, BlocksFile: bf, OutputFile: rec,
	}, nil)
	require.NoError(t, err)
	require.Positive(t, sum.Iterations)

	// The capture holds every request of every iteration, from a TLS target.
	l := capfiletest.Load(t, rec)
	require.Len(t, l.Targets, 1)
	tgt := l.Targets[0]
	require.Equal(t, capture.TransportTLS, tgt.Transport)
	require.Equal(t, url, tgt.Tags["url"])
	require.Equal(t, "127.0.0.1", tgt.Host)
	require.Len(t, l.Samples, len(plan)*int(sum.Iterations))
	counter := e2eCounterValues(t, l)
	require.Len(t, counter, int(sum.Iterations))
	requireStrictlyIncreasing(t, counter, "recorded")
	last := counter[len(counter)-1]
	require.Equal(t, dev.reads, last, "the last read was recorded")
	var lastFC17 []byte
	for i := range l.Samples {
		if l.Samples[i].Function() == 0x11 {
			lastFC17 = l.Samples[i].Response
		}
	}
	require.NotEmpty(t, lastFC17)

	// Replay over plain Modbus TCP: register 100 walks through the recorded
	// values and stays on the last one.
	rs := collStartServer(t, func(ctx context.Context, a collAddr) error {
		return ServeReplay(ctx, config.ReplayServerConfig{Port: a.Port, Listen: a.Listen, InputFile: rec, Speed: "4", Loops: 1}, a.Progress(nil))
	})
	rc := rs.client(t)
	recorded := map[uint16]bool{}
	for _, v := range counter {
		recorded[v] = true
	}
	var served []uint16
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		got, err := rc.ReadHoldingRegisters(context.Background(), 1, 100, 4)
		require.NoError(t, err)
		require.True(t, recorded[got[0]], "replay served %d, which was never recorded", got[0])
		require.Equal(t, []uint16{0xBEEF, 0x1234, 0x0042}, got[1:])
		if len(served) == 0 || served[len(served)-1] != got[0] {
			served = append(served, got[0])
		}
		if got[0] == last {
			break
		}
	}
	require.Equal(t, last, served[len(served)-1], "replay never reached the last value: %v", served)
	requireStrictlyIncreasing(t, served, "replayed")
	id, err := rc.ReportServerID(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, lastFC17[2:], id.Data, "FC17 answers the recorded response")
	require.ErrorIs(t, rs.stop(t), context.Canceled)

	// The capture served over tcp+tls and read with the TLS client: the last
	// recorded values, and a capture of the read that says it was over TLS.
	loaded, devices, err := serveTargets(rec, 0, 0, replay.Options{})
	require.NoError(t, err)
	end := loaded.End()
	replayURL := e2eTLSServe(t, pki, &captureHandler{devices: devices, now: func() capture.Ticks { return end }})
	readOut := filepath.Join(dir, "read.mbcap")
	res, err := CollectRead(context.Background(), config.ReadConfig{
		DeviceConfig: config.DeviceConfig{URL: replayURL, Unit: 1, TLSConfig: tlsClient(pki)},
		Timeout:      coreTimeoutMS, Function: 3, StartAddress: 100, RegisterCount: 4, OutputFile: readOut,
	}, nil)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%02X %02X BE EF 12 34 00 42", byte(last>>8), byte(last)), res.RawDataHex)
	rl := capfiletest.Load(t, readOut)
	require.Equal(t, capture.TransportTLS, rl.Targets[0].Transport)
	require.Len(t, rl.Samples, 1)
}

// Every client command that dials reaches a device over tcp+tls.
func TestE2ETLS_everyClientCommand(t *testing.T) {
	pki := mbtest.NewPKI(t)
	dev := collIdentDevice().
		SetHolding(0, 1, 2, 3, 4).
		SetHolding(200, 0x41AC, 0x0000).
		SetHolding(40000, collSunSpecRegs()...)
	d := tlsDevice(t, pki, dev)
	ctx := context.Background()
	unit := config.UnitClientConfig{URL: d.URL, UnitID: "1", Timeout: coreTimeoutMS, TLSConfig: d.TLSConfig}
	sunspec := collSunSpecBase(d.URL)
	sunspec.TLSConfig = d.TLSConfig

	t.Run("read", func(t *testing.T) {
		res, err := CollectRead(ctx, config.ReadConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, RegisterCount: 4,
			OutputFile: filepath.Join(t.TempDir(), "r.mbcap")}, nil)
		require.NoError(t, err)
		require.Equal(t, "00 01 00 02 00 03 00 04", res.RawDataHex)
	})
	t.Run("scan", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "s.mbcap")
		res, err := ScanAndRecord(ctx, config.ScanConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 10,
			Algo: "smart", OutputFile: out}, nil)
		require.NoError(t, err)
		require.Positive(t, res.SuccessCount)
		require.Equal(t, capture.TransportTLS, capfiletest.Load(t, out).Targets[0].Transport)
	})
	t.Run("poll", func(t *testing.T) {
		n := 0
		sum, err := Poll(ctx, config.PollConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, RegisterCount: 2, Interval: 5, Polls: 2},
			nil, func(*types.PollSample) error { n++; return nil })
		require.NoError(t, err)
		require.Equal(t, uint32(2), sum.Succeeded)
		require.Equal(t, 2, n)
	})
	t.Run("decode", func(t *testing.T) {
		res, err := CollectDecode(ctx, config.DecodeConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, StartAddress: 200,
			Codec: "float32/layout:4321"}, nil)
		require.NoError(t, err)
		require.Equal(t, uint16(2), res.RegisterCount)
	})
	t.Run("write", func(t *testing.T) {
		_, err := CollectWrite(ctx, config.WriteConfig{DeviceConfig: d, Timeout: coreTimeoutMS, StartAddress: 2, Values: []string{"77"}})
		require.NoError(t, err)
		require.Equal(t, []uint16{77}, dev.Holding(2, 1))
	})
	t.Run("identify", func(t *testing.T) {
		res, err := CollectDeviceIdentification(ctx, config.IdentifyConfig{UnitClientConfig: unit})
		require.NoError(t, err)
		require.Nil(t, res.Units[0].Error)
	})
	t.Run("identify, units in parallel", func(t *testing.T) {
		u := unit
		u.UnitID, u.Parallel = "1-3", 3
		res, err := CollectDeviceIdentification(ctx, config.IdentifyConfig{UnitClientConfig: u})
		require.NoError(t, err)
		require.Len(t, res.Units, 3)
	})
	t.Run("report server id", func(t *testing.T) {
		res, err := CollectReportServerID(ctx, config.ReportServerIDConfig{UnitClientConfig: unit})
		require.NoError(t, err)
		require.Nil(t, res.Units[0].Error)
	})
	t.Run("fingerprint", func(t *testing.T) {
		res, err := CollectFingerprint(ctx, config.FingerprintConfig{URL: d.URL, UnitID: "1", Timeout: coreTimeoutMS, TLSConfig: d.TLSConfig})
		require.NoError(t, err)
		require.Nil(t, res.Units[0].Error)
	})
	t.Run("diagnostic", func(t *testing.T) {
		res, err := CollectDiagnostics(ctx, config.DiagnosticConfig{URL: d.URL, UnitID: 1, Timeout: coreTimeoutMS, Data: "BEEF", TLSConfig: d.TLSConfig})
		require.NoError(t, err)
		require.Equal(t, "BE EF", res.DataHex)
	})
	t.Run("sunspec detect", func(t *testing.T) {
		out, err := CollectSunSpecDetect(ctx, config.SunSpecDetectConfig{SunSpecBaseConfig: sunspec})
		require.NoError(t, err)
		require.True(t, out.Detected)
	})
	t.Run("record", func(t *testing.T) {
		rec, sum, err := e2eRecordTLS(t, d, 50)
		require.NoError(t, err)
		require.Positive(t, sum.BlocksRecorded)
		require.Equal(t, capture.TransportTLS, capfiletest.Load(t, rec).Targets[0].Transport)
	})
}

// e2eRecordTLS records holding registers 0-3 of d for duration ms.
func e2eRecordTLS(t *testing.T, d config.DeviceConfig, duration uint32) (string, *types.RecordSummaryResult, error) {
	t.Helper()
	dir := t.TempDir()
	bf := filepath.Join(dir, "blocks.json")
	require.NoError(t, os.WriteFile(bf, []byte(`[{"start_address":0,"register_count":4}]`), 0o600))
	out := filepath.Join(dir, "record.mbcap")
	sum, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, Interval: 10, Duration: duration, BlocksFile: bf, OutputFile: out,
	}, nil)
	return out, sum, err
}

// The client refuses a server whose certificate --tls-ca does not vouch for,
// and leaves no capture behind.
func TestE2ETLS_untrustedServer(t *testing.T) {
	pki, other := mbtest.NewPKI(t), mbtest.NewPKI(t)
	d := tlsDevice(t, pki, mbtest.NewDevice().SetHolding(0, 1, 2, 3, 4))

	t.Run("CA of another PKI", func(t *testing.T) {
		d := d
		d.TLSCA = other.CAFile
		out := filepath.Join(t.TempDir(), "r.mbcap")
		_, err := CollectRead(context.Background(), config.ReadConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1, OutputFile: out}, nil)
		require.ErrorIs(t, err, ErrTCPConnection)
		var unknown x509.UnknownAuthorityError
		require.ErrorAs(t, err, &unknown)
		require.NoFileExists(t, out)

		rec, _, err := e2eRecordTLS(t, d, 50)
		require.ErrorIs(t, err, ErrTCPConnection)
		require.NoFileExists(t, rec)
	})
	t.Run("system roots", func(t *testing.T) {
		d := d
		d.TLSCA = ""
		_, err := CollectRead(context.Background(), config.ReadConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1,
			OutputFile: filepath.Join(t.TempDir(), "r.mbcap")}, nil)
		require.ErrorIs(t, err, ErrTCPConnection)
	})
	t.Run("server certificate for another host", func(t *testing.T) {
		// A certificate the CA signed, but for a client: no IP SAN for 127.0.0.1.
		cert, key := pki.Issue("not a server", false)
		conf := pki.ServerConfig(pki.Pool)
		pair, err := tls.LoadX509KeyPair(cert, key)
		require.NoError(t, err)
		conf.Certificates[0] = pair
		d := d
		d.URL = "tcp+tls://" + mbtest.NewDevice().ServeTLSConfig(t, conf)
		_, err = CollectRead(context.Background(), config.ReadConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1,
			OutputFile: filepath.Join(t.TempDir(), "r.mbcap")}, nil)
		require.ErrorIs(t, err, ErrTCPConnection)
	})
}

// A server that does not trust the client certificate never answers it.
func TestE2ETLS_clientCertNotTrusted(t *testing.T) {
	pki, other := mbtest.NewPKI(t), mbtest.NewPKI(t)
	dev := &e2eTLSDevice{}
	url := e2eTLSServe(t, pki, dev)
	cfg := config.TLSConfig{TLSCert: other.ClientCertFile, TLSKey: other.ClientKeyFile, TLSCA: pki.CAFile}
	_, err := CollectRead(context.Background(), config.ReadConfig{
		DeviceConfig: config.DeviceConfig{URL: url, Unit: 1, TLSConfig: cfg}, Timeout: coreTimeoutMS, Function: 3, StartAddress: 100, RegisterCount: 1,
		OutputFile: filepath.Join(t.TempDir(), "r.mbcap"),
	}, nil)
	require.Error(t, err)
	require.NotErrorIs(t, err, gomodbus.ErrRequestTimedOut, "rejected, not timed out")
	dev.mu.Lock()
	defer dev.mu.Unlock()
	require.Zero(t, dev.reads, "the device never served the untrusted client")
}

// Without usable TLS files the command fails as invalid input before dialing.
func TestE2ETLS_missingOrBadFiles(t *testing.T) {
	pki := mbtest.NewPKI(t)
	d := tlsDevice(t, pki, mbtest.NewDevice().SetHolding(0, 1))
	empty := filepath.Join(t.TempDir(), "empty.pem")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	other := mbtest.NewPKI(t)

	for _, tc := range []struct {
		name string
		tls  config.TLSConfig
		want string
	}{
		{"no certificate", config.TLSConfig{TLSCA: pki.CAFile}, "needs a client certificate: set --tls-cert"},
		{"key only", config.TLSConfig{TLSKey: pki.ClientKeyFile, TLSCA: pki.CAFile}, "set --tls-cert"},
		{"certificate without key", config.TLSConfig{TLSCert: pki.ClientCertFile, TLSCA: pki.CAFile}, "load TLS client certificate"},
		{"missing certificate file", config.TLSConfig{TLSCert: filepath.Join(t.TempDir(), "nope.pem"), TLSCA: pki.CAFile}, "no such file"},
		{"key of another certificate", config.TLSConfig{TLSCert: pki.ClientCertFile, TLSKey: other.ClientKeyFile, TLSCA: pki.CAFile}, "private key does not match"},
		{"missing CA file", config.TLSConfig{TLSCert: pki.ClientPEMFile, TLSCA: filepath.Join(t.TempDir(), "nope.pem")}, "load TLS CA certificates"},
		{"empty CA file", config.TLSConfig{TLSCert: pki.ClientPEMFile, TLSCA: empty}, "empty file"},
		{"CA file without certificates", config.TLSConfig{TLSCert: pki.ClientPEMFile, TLSCA: pki.ClientKeyFile}, "no certificate found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, config.ValidateTLSConfig(d.URL, tc.tls), tc.want)
			d := d
			d.TLSConfig = tc.tls
			out := filepath.Join(t.TempDir(), "r.mbcap")
			_, err := CollectRead(context.Background(), config.ReadConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1, OutputFile: out}, nil)
			coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
			require.ErrorContains(t, err, tc.want)
			require.NoFileExists(t, out)
		})
	}

	t.Run("certificate and key in one file", func(t *testing.T) {
		d := d
		d.TLSConfig = config.TLSConfig{TLSCert: pki.ClientPEMFile, TLSCA: pki.CAFile}
		res, err := CollectRead(context.Background(), config.ReadConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1,
			OutputFile: filepath.Join(t.TempDir(), "r.mbcap")}, nil)
		require.NoError(t, err)
		require.Equal(t, "00 01", res.RawDataHex)
	})
}

// The TLS settings apply to tcp+tls:// only: other URLs ignore them, even
// when they name files that do not exist.
func TestE2ETLS_ignoredForOtherURLs(t *testing.T) {
	bogus := config.TLSConfig{TLSCert: "/no/such/cert.pem", TLSCA: "/no/such/ca.pem"}
	dev := mbtest.NewDevice().SetHolding(0, 9)
	d := coreTCP(t, dev)
	require.NoError(t, config.ValidateTLSConfig(d.URL, bogus))
	d.TLSConfig = bogus
	res, err := CollectRead(context.Background(), config.ReadConfig{DeviceConfig: d, Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1,
		OutputFile: filepath.Join(t.TempDir(), "r.mbcap")}, nil)
	require.NoError(t, err)
	assert.Equal(t, "00 09", res.RawDataHex)
	conf, err := buildDeviceClientConfig(d.URL, coreTimeoutMS, false, config.SerialConfig{}, bogus)
	require.NoError(t, err)
	assert.Nil(t, conf.TLSClientCert)
	assert.Nil(t, conf.TLSRootCAs)
}
