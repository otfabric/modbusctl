// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func silentDevice(t *testing.T) (*mbtest.Device, string) {
	t.Helper()
	dev := mbtest.NewDevice()
	dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Drop: true} }
	return dev, "tcp://" + dev.ServeTCP(t)
}

// Every SunSpec read is bounded by --timeout. It used to inherit the budget of the whole
// operation (10 s or more), so a silent device blocked on the first base address.
func TestSunSpecDetect_readsHonourRequestTimeout(t *testing.T) {
	dev, url := silentDevice(t)

	start := time.Now()
	out, err := CollectSunSpecDetect(context.Background(), config.SunSpecDetectConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{URL: url, Unit: 1, Timeout: 200, Regtype: "holding"},
		Bases:             "0,40000",
	})
	elapsed := time.Since(start)

	require.Less(t, elapsed, 3*time.Second, "reads must time out after --timeout, not after the operation budget")
	if err == nil {
		assert.False(t, out.Detected)
	}
	assert.GreaterOrEqual(t, dev.RequestCount(0), 1)
}

// A cancelled context stops multi-unit collectors at the next unit instead of waiting one
// request timeout for every remaining unit.
func TestUnitCollectors_stopOnCancel(t *testing.T) {
	for _, parallel := range []uint16{1, 4} {
		dev, url := silentDevice(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		uc := config.UnitClientConfig{URL: url, UnitID: "1-20", Timeout: 300, Parallel: parallel}

		start := time.Now()
		_, err := CollectDeviceIdentification(ctx, config.IdentifyConfig{UnitClientConfig: uc})
		require.ErrorIs(t, err, context.Canceled, "identify, parallel=%d", parallel)

		_, err = CollectReportServerID(ctx, config.ReportServerIDConfig{UnitClientConfig: uc})
		require.ErrorIs(t, err, context.Canceled, "reportserverid, parallel=%d", parallel)

		assert.Less(t, time.Since(start), 2*time.Second)
		assert.Zero(t, dev.RequestCount(0), "no unit may be queried after cancellation")
	}
}

// A connection closed by the peer is a transport failure, not an internal error.
func TestWrapCollectError_peerClosedConnectionIsTransport(t *testing.T) {
	for _, cause := range []error{io.EOF, io.ErrUnexpectedEOF} {
		e, ok := errs.As(WrapCollectError(cause))
		require.True(t, ok)
		assert.Equal(t, errs.KindTransport, e.Kind)
		assert.Equal(t, errs.CodeTransportConnectFailed, e.Code)

		info := EmbeddedErrorInfo(cause)
		assert.Equal(t, string(errs.KindTransport), info.Kind)
	}
}
