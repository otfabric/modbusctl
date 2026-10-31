// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collServerIDCfg(url, unit string) config.ReportServerIDConfig {
	return config.ReportServerIDConfig{UnitClientConfig: config.UnitClientConfig{URL: url, UnitID: unit, Timeout: 2000}}
}

func TestCollReportServerID_singleUnit(t *testing.T) {
	tests := []struct {
		name    string
		id      *mbtest.ServerID
		wantHex string
		wantRun *bool
	}{
		{name: "running", id: &mbtest.ServerID{ID: []byte{0x12, 0x34}, Running: true}, wantHex: "12 34 FF", wantRun: collPtr(true)},
		{name: "stopped", id: &mbtest.ServerID{ID: []byte{0x12}, Running: false}, wantHex: "12 00", wantRun: collPtr(false)},
		{
			name:    "additional data follows the run indicator",
			id:      &mbtest.ServerID{ID: []byte{0x01}, Running: true, Additional: []byte{0xFF}},
			wantHex: "01 FF FF", wantRun: collPtr(true),
		},
		{name: "single byte has no run indicator", id: &mbtest.ServerID{}, wantHex: "00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dev := mbtest.NewDevice()
			dev.ServerID = tc.id
			cfg := collServerIDCfg("tcp://"+dev.ServeTCP(t), "9")

			res, err := CollectReportServerID(context.Background(), cfg)
			require.NoError(t, err)
			require.Equal(t, cfg.URL, res.Target)
			require.Len(t, res.Units, 1)
			u := res.Units[0]
			require.Equal(t, uint8(9), u.UnitID)
			require.Nil(t, u.Error)
			require.Equal(t, tc.wantHex, u.DataHex)
			require.Equal(t, tc.wantRun, u.RunIndicatorOn)
			require.Equal(t, 1, res.Summary.Requested)
			require.Equal(t, 1, res.Summary.Succeeded)
			require.Equal(t, 0, res.Summary.Failed)
		})
	}
}

func collPtr[T any](v T) *T { return &v }

func TestCollReportServerID_unsupported(t *testing.T) {
	dev := mbtest.NewDevice()
	res, err := CollectReportServerID(context.Background(), collServerIDCfg("tcp://"+dev.ServeTCP(t), "1"))
	require.NoError(t, err)
	u := res.Units[0]
	collRequireInfo(t, u.Error, errs.KindModbus, errs.CodeModbusException)
	require.Empty(t, u.DataHex)
	require.Nil(t, u.RunIndicatorOn)
	require.Equal(t, 1, res.Summary.Failed)
}

// Sequential and parallel sweeps must return the same per-unit rows, sorted by unit ID.
func TestCollReportServerID_multipleUnits(t *testing.T) {
	for _, parallel := range []uint16{0, 1, 3, 64} {
		t.Run(fmt.Sprintf("parallel %d", parallel), func(t *testing.T) {
			dev := mbtest.NewDevice()
			dev.ServerID = &mbtest.ServerID{ID: []byte{0xAB}, Running: true}
			dev.Units = []uint8{1, 2, 4, 5}
			cfg := collServerIDCfg("tcp://"+dev.ServeTCP(t), "5,1-4")
			cfg.Parallel = parallel

			res, err := CollectReportServerID(context.Background(), cfg)
			require.NoError(t, err)
			var got []uint8
			for _, u := range res.Units {
				got = append(got, u.UnitID)
				if u.UnitID == 3 {
					collRequireInfo(t, u.Error, errs.KindModbus, errs.CodeModbusException)
					assert.Empty(t, u.DataHex)
					continue
				}
				require.Nilf(t, u.Error, "unit %d", u.UnitID)
				assert.Equal(t, "AB FF", u.DataHex)
				require.NotNil(t, u.RunIndicatorOn)
				assert.True(t, *u.RunIndicatorOn)
			}
			require.Equal(t, []uint8{1, 2, 3, 4, 5}, got)
			require.Equal(t, 5, res.Summary.Requested)
			require.Equal(t, 4, res.Summary.Succeeded)
			require.Equal(t, 1, res.Summary.Failed)
			require.True(t, res.HasPartialFailure())
			require.Equal(t, 5, dev.RequestCount(0x11))
		})
	}
}

func TestCollReportServerID_timeoutIsPerUnit(t *testing.T) {
	for _, parallel := range []uint16{1, 2} {
		dev := mbtest.NewDevice()
		dev.ServerID = &mbtest.ServerID{ID: []byte{0x01}, Running: true}
		dev.OnRequest = func(req mbtest.Request) mbtest.Action {
			return mbtest.Action{Drop: req.Unit == 1}
		}
		cfg := collServerIDCfg("tcp://"+dev.ServeTCP(t), "1,2")
		cfg.Timeout = 200
		cfg.Parallel = parallel

		res, err := CollectReportServerID(context.Background(), cfg)
		require.NoError(t, err)
		require.Len(t, res.Units, 2)
		collRequireInfo(t, res.Units[0].Error, errs.KindTimeout, errs.CodeRequestTimeout)
		require.Nil(t, res.Units[1].Error)
		require.Equal(t, "01 FF", res.Units[1].DataHex)
		require.Equal(t, 1, res.Summary.Failed)
	}
}

// A serial line must be opened once even when --parallel asks for more.
func TestCollReportServerID_serialIgnoresParallel(t *testing.T) {
	dev := mbtest.NewDevice()
	dev.ServerID = &mbtest.ServerID{ID: []byte{0x07}, Running: true}
	cfg := collServerIDCfg("ascii://"+dev.ServeASCIISerial(t), "1-3")
	cfg.SerialConfig = config.SerialConfig{Baud: 115200, DataBits: 8}
	cfg.Parallel = 8

	res, err := CollectReportServerID(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, res.Units, 3)
	for i, u := range res.Units {
		require.Equal(t, uint8(i+1), u.UnitID)
		require.Nil(t, u.Error)
		require.Equal(t, "07 FF", u.DataHex)
	}
}

func TestCollReportServerID_setupErrors(t *testing.T) {
	closed := "tcp://" + collClosedAddr(t)
	tests := []struct {
		name     string
		url      string
		unit     string
		parallel uint16
		check    func(t *testing.T, err error)
	}{
		{name: "invalid unit selector", url: closed, unit: "0", check: func(t *testing.T, err error) {
			collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidUnitSelector)
		}},
		{name: "reversed range", url: closed, unit: "9-3", check: func(t *testing.T, err error) {
			collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidUnitSelector)
		}},
		{name: "no target", unit: "1", check: func(t *testing.T, err error) {
			collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
		}},
		{name: "unknown scheme, parallel", url: "bogus://127.0.0.1:1", unit: "1-4", parallel: 4, check: func(t *testing.T, err error) {
			collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
		}},
		{name: "closed port", url: closed, unit: "1", check: func(t *testing.T, err error) {
			require.ErrorIs(t, err, ErrTCPConnection)
		}},
		{name: "closed port, parallel", url: closed, unit: "1-4", parallel: 4, check: func(t *testing.T, err error) {
			require.ErrorIs(t, err, ErrTCPConnection)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := collServerIDCfg(tc.url, tc.unit)
			cfg.Parallel = tc.parallel
			res, err := CollectReportServerID(context.Background(), cfg)
			require.Nil(t, res)
			tc.check(t, err)
		})
	}
}

func TestCollDiagnostics(t *testing.T) {
	dev := mbtest.NewDevice()
	url := "tcp://" + dev.ServeTCP(t)
	tests := []struct {
		name        string
		subFunction string
		data        string
		wantName    string
		wantCode    uint16
		wantHex     string
		wantPDU     []byte
	}{
		{name: "defaults", wantName: "returnquerydata", wantCode: 0, wantHex: "00 00", wantPDU: []byte{0x08, 0, 0, 0, 0}},
		{name: "explicit data with spaces", data: " A5 37 ", wantName: "returnquerydata", wantCode: 0, wantHex: "A5 37", wantPDU: []byte{0x08, 0, 0, 0xA5, 0x37}},
		{
			name: "named sub-function", subFunction: "ReturnServerBusyCount",
			wantName: "ReturnServerBusyCount", wantCode: 0x0011, wantHex: "00 00", wantPDU: []byte{0x08, 0x00, 0x11, 0, 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := CollectDiagnostics(context.Background(), config.DiagnosticConfig{
				URL: url, UnitID: 5, Timeout: 2000, SubFunction: tc.subFunction, Data: tc.data,
			})
			require.NoError(t, err)
			require.Equal(t, url, res.Target)
			require.Equal(t, uint8(5), res.UnitID)
			require.Equal(t, tc.wantName, res.SubFunction)
			require.Equal(t, tc.wantCode, res.SubFunctionHex)
			require.Equal(t, tc.wantHex, res.DataHex)
			reqs := dev.Requests()
			last := reqs[len(reqs)-1]
			require.Equal(t, uint8(5), last.Unit)
			require.Equal(t, tc.wantPDU, last.PDU)
		})
	}
}

func TestCollDiagnostics_overSerial(t *testing.T) {
	dev := mbtest.NewDevice()
	res, err := CollectDiagnostics(context.Background(), config.DiagnosticConfig{
		SerialConfig: config.SerialConfig{Baud: 115200, Parity: "even"},
		URL:          "rtu://" + dev.ServeRTUSerial(t), UnitID: 2, Timeout: 2000, Data: "BEEF",
	})
	require.NoError(t, err)
	require.Equal(t, "BE EF", res.DataHex)
}

func TestCollDiagnostics_errors(t *testing.T) {
	dev := mbtest.NewDevice()
	dev.OnRequest = func(req mbtest.Request) mbtest.Action {
		switch req.Unit {
		case 2:
			return mbtest.Action{Exception: mbtest.ExIllegalFunction}
		case 3:
			return mbtest.Action{Drop: true}
		default:
			return mbtest.Action{}
		}
	}
	url := "tcp://" + dev.ServeTCP(t)

	t.Run("unknown sub-function", func(t *testing.T) {
		res, err := CollectDiagnostics(context.Background(), config.DiagnosticConfig{URL: url, UnitID: 1, SubFunction: "nope"})
		require.Nil(t, res)
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
		require.Zero(t, dev.RequestCount(0), "nothing may be sent for an invalid request")
	})
	t.Run("invalid hex data", func(t *testing.T) {
		res, err := CollectDiagnostics(context.Background(), config.DiagnosticConfig{URL: url, UnitID: 1, Data: "XYZ"})
		require.Nil(t, res)
		e := collTypedErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
		require.Contains(t, e.Message, "invalid hex data")
		require.Zero(t, dev.RequestCount(0), "nothing may be sent for an invalid request")
	})
	t.Run("no target", func(t *testing.T) {
		res, err := CollectDiagnostics(context.Background(), config.DiagnosticConfig{UnitID: 1})
		require.Nil(t, res)
		require.Error(t, err)
	})
	t.Run("closed port", func(t *testing.T) {
		res, err := CollectDiagnostics(context.Background(), config.DiagnosticConfig{URL: "tcp://" + collClosedAddr(t), UnitID: 1, Timeout: 500})
		require.Nil(t, res)
		require.ErrorIs(t, err, ErrTCPConnection)
	})
	t.Run("exception", func(t *testing.T) {
		res, err := CollectDiagnostics(context.Background(), config.DiagnosticConfig{URL: url, UnitID: 2, Timeout: 2000})
		require.Nil(t, res)
		e := collTypedErr(t, err, errs.KindModbus, errs.CodeDiagnosticsFailed)
		require.Error(t, e.Cause)
	})
	t.Run("timeout", func(t *testing.T) {
		start := time.Now()
		res, err := CollectDiagnostics(context.Background(), config.DiagnosticConfig{URL: url, UnitID: 3, Timeout: 200})
		require.Nil(t, res)
		require.Error(t, err)
		_, typed := errs.As(err)
		require.True(t, typed)
		require.Less(t, time.Since(start), 2*time.Second)
	})
}

func collFingerprintDevice() *mbtest.Device {
	return collIdentDevice().
		SetHolding(0, 1).
		SetInput(0, 1).
		SetCoils(0, true).
		SetDiscreteInputs(0, true)
}

// The functions the device really implements must be listed, in probe order.
func TestCollFingerprint_supportedReads(t *testing.T) {
	dev := collFingerprintDevice()
	url := "tcp://" + dev.ServeTCP(t)

	res, err := CollectFingerprint(context.Background(), config.FingerprintConfig{URL: url, UnitID: "1,2", Timeout: 2000, Interval: 1})
	require.NoError(t, err)
	require.Equal(t, url, res.Target)
	require.Len(t, res.Units, 2)
	for i, u := range res.Units {
		require.Equal(t, uint8(i+1), u.UnitID)
		require.Nil(t, u.Error)
		require.False(t, u.ProbeInterrupted)
		// Exactly the functions the device implements: Read FIFO Queue (0x18) and Read File
		// Record (0x14) are answered with Illegal Function and must not be listed.
		want := []string{"(0x08)", "(0x2B)", "(0x03)", "(0x04)", "(0x01)", "(0x02)", "(0x11)"}
		require.Len(t, u.SupportedReads, len(want), "%v", u.SupportedReads)
		for j, suffix := range want {
			require.Truef(t, strings.HasSuffix(u.SupportedReads[j], suffix), "function %d: %q, want suffix %s", j, u.SupportedReads[j], suffix)
		}
	}
	require.Equal(t, 2, res.Summary.Requested)
	require.Equal(t, 2, res.Summary.Succeeded)
	require.Equal(t, 0, res.Summary.Failed)
	require.Equal(t, 2*len(readFCsForFingerprint), dev.RequestCount(0), "one probe per function code and unit")
}

// A unit that never answers has no supported functions, but the sweep goes on.
func TestCollFingerprint_silentUnit(t *testing.T) {
	dev := collFingerprintDevice()
	dev.OnRequest = func(req mbtest.Request) mbtest.Action {
		return mbtest.Action{Drop: req.Unit == 1}
	}
	res, err := CollectFingerprint(context.Background(), config.FingerprintConfig{URL: "tcp://" + dev.ServeTCP(t), UnitID: "1,2", Timeout: 40})
	require.NoError(t, err)
	require.Len(t, res.Units, 2)
	require.Empty(t, res.Units[0].SupportedReads)
	require.NotEmpty(t, res.Units[1].SupportedReads)
	require.Nil(t, res.Units[1].Error)
}

// A broken connection stops the unit's probes and keeps what was learned so far.
func TestCollFingerprint_connectionLostKeepsPartialResult(t *testing.T) {
	dev := collFingerprintDevice()
	dev.OnRequest = func(req mbtest.Request) mbtest.Action {
		return mbtest.Action{Close: req.PDU[0] == 0x04}
	}
	res, err := CollectFingerprint(context.Background(), config.FingerprintConfig{URL: "tcp://" + dev.ServeTCP(t), UnitID: "1", Timeout: 1000})
	require.NoError(t, err)
	require.Len(t, res.Units, 1)
	u := res.Units[0]
	require.True(t, u.ProbeInterrupted)
	require.NotNil(t, u.Error)
	require.NotEmpty(t, u.Error.Message)
	require.Len(t, u.SupportedReads, 3, "FC08, FC43 and FC03 were confirmed before FC04 broke the link")
	require.Contains(t, u.SupportedReads[0], "0x08")
	require.Contains(t, u.SupportedReads[1], "0x2B")
	require.Contains(t, u.SupportedReads[2], "0x03")
	require.Zero(t, dev.RequestCount(0x01), "probing must stop at the first hard failure")
	require.Equal(t, 0, res.Summary.Failed, "a partial result is not a failed unit")
}

func TestCollFingerprint_contextCanceled(t *testing.T) {
	dev := collFingerprintDevice()
	url := "tcp://" + dev.ServeTCP(t)

	t.Run("before the first probe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res, err := CollectFingerprint(ctx, config.FingerprintConfig{URL: url, UnitID: "1-3", Timeout: 1000})
		require.NoError(t, err)
		require.Len(t, res.Units, 3)
		for _, u := range res.Units {
			require.True(t, u.ProbeInterrupted)
			require.Empty(t, u.SupportedReads)
			collRequireInfo(t, u.Error, errs.KindInterrupted, errs.CodeContextCanceled)
		}
		require.Equal(t, 3, res.Summary.Failed)
	})

	t.Run("while waiting between probes", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		res, err := CollectFingerprint(ctx, config.FingerprintConfig{URL: url, UnitID: "1", Timeout: 1000, Interval: 60_000})
		require.Nil(t, res)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(start), 2*time.Second)
	})
}

func TestCollFingerprint_setupErrors(t *testing.T) {
	closed := "tcp://" + collClosedAddr(t)
	t.Run("invalid unit selector", func(t *testing.T) {
		res, err := CollectFingerprint(context.Background(), config.FingerprintConfig{URL: closed, UnitID: "abc"})
		require.Nil(t, res)
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidUnitSelector)
	})
	t.Run("no target", func(t *testing.T) {
		res, err := CollectFingerprint(context.Background(), config.FingerprintConfig{UnitID: "1"})
		require.Nil(t, res)
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	})
	t.Run("closed port", func(t *testing.T) {
		res, err := CollectFingerprint(context.Background(), config.FingerprintConfig{URL: closed, UnitID: "1", Timeout: 500})
		require.Nil(t, res)
		require.ErrorIs(t, err, ErrTCPConnection)
	})
}

// A gateway that answers for an absent unit with "gateway target failed to respond" must not
// make that unit look like a device that supports everything.
func TestCollFingerprint_absentUnitBehindGateway(t *testing.T) {
	dev := collFingerprintDevice()
	dev.Units = []uint8{1}
	res, err := CollectFingerprint(context.Background(), config.FingerprintConfig{URL: "tcp://" + dev.ServeTCP(t), UnitID: "1,9", Timeout: 1000})
	require.NoError(t, err)
	require.Len(t, res.Units, 2)
	require.NotEmpty(t, res.Units[0].SupportedReads)
	require.Empty(t, res.Units[1].SupportedReads, "unit 9 does not exist")
}
