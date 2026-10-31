// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coreCollectSamples returns an emit callback appending to *out.
func coreCollectSamples(out *[]*types.PollSample) func(*types.PollSample) error {
	return func(s *types.PollSample) error {
		*out = append(*out, s)
		return nil
	}
}

func TestCoreResolvePollCodec(t *testing.T) {
	t.Run("no codec", func(t *testing.T) {
		rc, count, err := ResolvePollCodec(config.PollConfig{RegisterCount: 7, Codec: "  "})
		require.NoError(t, err)
		assert.Nil(t, rc)
		assert.Equal(t, uint16(7), count)
	})

	t.Run("count defaults to one codec value", func(t *testing.T) {
		rc, count, err := ResolvePollCodec(config.PollConfig{Codec: " float64/layout:87654321 "})
		require.NoError(t, err)
		require.NotNil(t, rc)
		assert.Equal(t, "float64/layout:87654321", rc.ID())
		assert.Equal(t, uint16(4), count)
	})

	t.Run("count is a multiple of the codec width", func(t *testing.T) {
		rc, count, err := ResolvePollCodec(config.PollConfig{Codec: "uint32/layout:4321", RegisterCount: 6})
		require.NoError(t, err)
		require.NotNil(t, rc)
		assert.Equal(t, uint16(6), count)
	})

	t.Run("count is not a multiple of the codec width", func(t *testing.T) {
		rc, count, err := ResolvePollCodec(config.PollConfig{Codec: "uint32/layout:4321", RegisterCount: 5})
		require.EqualError(t, err, `register count 5 must be a multiple of the codec register width 2 for "uint32/layout:4321"`)
		assert.Nil(t, rc)
		assert.Zero(t, count)
	})

	t.Run("unknown codec", func(t *testing.T) {
		rc, count, err := ResolvePollCodec(config.PollConfig{Codec: "nope"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown codec "nope"`)
		assert.Nil(t, rc)
		assert.Zero(t, count)
	})
}

func TestCorePoll_samplesAndSummary(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(10, 1, 2, 3)
	dc := coreTCP(t, dev)
	dc.Unit = 5

	var samples []*types.PollSample
	before := time.Now().UTC().Truncate(time.Millisecond)
	sum, err := Poll(context.Background(), config.PollConfig{
		DeviceConfig: dc, Timeout: coreTimeoutMS, Function: 3, StartAddress: 10, RegisterCount: 3, Interval: 20, Polls: 3,
	}, nil, coreCollectSamples(&samples))
	require.NoError(t, err)

	assert.Equal(t, dc.URL, sum.Target)
	assert.Equal(t, uint32(3), sum.Polls)
	assert.Equal(t, uint32(3), sum.Succeeded)
	assert.Zero(t, sum.Failed)
	assert.False(t, sum.Interrupted)
	dur, err := time.ParseDuration(sum.Duration)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, dur, 40*time.Millisecond, "three polls are two intervals apart")

	require.Len(t, samples, 3)
	var prev time.Time
	for i, s := range samples {
		assert.Equal(t, uint32(i+1), s.Poll)
		assert.Equal(t, uint8(5), s.UnitID)
		assert.Equal(t, uint8(3), s.Function)
		assert.Equal(t, uint16(10), s.StartAddress)
		assert.Equal(t, uint16(3), s.Count)
		assert.Equal(t, uint16(1), s.AddressStep)
		assert.Empty(t, s.Codec)
		assert.Equal(t, "00 01 00 02 00 03", s.RawDataHex)
		assert.Equal(t, []any{uint16(1), uint16(2), uint16(3)}, s.Values)
		assert.Nil(t, s.Error)
		assert.Positive(t, s.RTTMs)

		ts, err := time.Parse(pollTimestampLayout, s.Timestamp)
		require.NoError(t, err, "timestamp %q", s.Timestamp)
		assert.Equal(t, time.UTC, ts.Location())
		assert.False(t, ts.Before(before))
		if i > 0 {
			assert.GreaterOrEqual(t, ts.Sub(prev), 15*time.Millisecond, "polls start on the configured cadence")
		}
		prev = ts
	}
	assert.Equal(t, 3, dev.RequestCount(3))
}

func TestCorePoll_codecValues(t *testing.T) {
	dev := mbtest.NewDevice().SetInput(0, 0x3FC0, 0x0000, 0xC020, 0x0000)
	var samples []*types.PollSample
	sum, err := Poll(context.Background(), config.PollConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 4, RegisterCount: 4,
		Codec: " float32/layout:4321 ", Interval: 1, Polls: 1,
	}, nil, coreCollectSamples(&samples))
	require.NoError(t, err)
	assert.Equal(t, uint32(1), sum.Succeeded)
	require.Len(t, samples, 1)
	assert.Equal(t, []any{float32(1.5), float32(-2.5)}, samples[0].Values)
	assert.Equal(t, uint16(4), samples[0].Count)
	assert.Equal(t, uint16(2), samples[0].AddressStep)
	assert.Equal(t, "float32/layout:4321", samples[0].Codec)
}

// Without a count, a codec poll reads exactly one value; byte-like values are rendered as text.
func TestCorePoll_codecDefaultCount(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 0x001A, 0x2B3C, 0x4D5E)
	var samples []*types.PollSample
	_, err := Poll(context.Background(), config.PollConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Codec: "eui48", Interval: 1, Polls: 1,
	}, nil, coreCollectSamples(&samples))
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, uint16(3), samples[0].Count)
	assert.Equal(t, []any{"00:1a:2b:3c:4d:5e"}, samples[0].Values)
}

func TestCorePoll_discreteInputs(t *testing.T) {
	dev := mbtest.NewDevice().SetDiscreteInputs(0, true, true, false, false, false, false, false, false, false, true)
	var samples []*types.PollSample
	_, err := Poll(context.Background(), config.PollConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 2, RegisterCount: 10, Interval: 1, Polls: 1,
	}, nil, coreCollectSamples(&samples))
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, []any{true, true, false, false, false, false, false, false, false, true}, samples[0].Values)
	assert.Equal(t, "03 02", samples[0].RawDataHex)
}

func TestCorePollValues(t *testing.T) {
	t.Run("coils stop at the bytes received", func(t *testing.T) {
		values, err := pollValues([]byte{0x05}, 1, 12, nil)
		require.NoError(t, err)
		assert.Equal(t, []any{true, false, true, false, false, false, false, false}, values)
	})

	t.Run("registers", func(t *testing.T) {
		values, err := pollValues([]byte{0x12, 0x34, 0xFF, 0xFF}, 3, 2, nil)
		require.NoError(t, err)
		assert.Equal(t, []any{uint16(0x1234), uint16(0xFFFF)}, values)
	})

	t.Run("codec decode failure", func(t *testing.T) {
		rc, ok, err := codec.RuntimeCodecByID("datetime_iec870")
		require.NoError(t, err)
		require.True(t, ok)
		values, err := pollValues([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, 3, 4, rc)
		assert.Nil(t, values)
		e, isErrs := errs.As(err)
		require.True(t, isErrs, "error: %v", err)
		assert.Equal(t, errs.KindInvalidInput, e.Kind)
		assert.Contains(t, e.Message, `decode with codec "datetime_iec870" failed`)
	})
}

// A failed read is reported as a sample and the run goes on.
func TestCorePoll_failedReadsAreSamples(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 77)
	var n atomic.Int32
	dev.OnRequest = func(mbtest.Request) mbtest.Action {
		switch n.Add(1) {
		case 1:
			return mbtest.Action{Exception: mbtest.ExServerDeviceFailure}
		case 2:
			return mbtest.Action{Drop: true}
		default:
			return mbtest.Action{}
		}
	}

	var samples []*types.PollSample
	sum, err := Poll(context.Background(), config.PollConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreShortTimeoutMS, Function: 3, RegisterCount: 1, Interval: 1, Polls: 3,
	}, nil, coreCollectSamples(&samples))
	require.NoError(t, err)
	assert.Equal(t, uint32(3), sum.Polls)
	assert.Equal(t, uint32(1), sum.Succeeded)
	assert.Equal(t, uint32(2), sum.Failed)
	assert.False(t, sum.Interrupted)

	require.Len(t, samples, 3)
	require.NotNil(t, samples[0].Error)
	assert.Equal(t, string(errs.KindModbus), samples[0].Error.Kind)
	assert.Equal(t, errs.CodeModbusException, samples[0].Error.Code)
	assert.NotEmpty(t, samples[0].Error.Message)
	assert.Equal(t, []any{}, samples[0].Values, "a failed sample has an empty, non-nil value list")
	assert.Empty(t, samples[0].RawDataHex)

	require.NotNil(t, samples[1].Error)
	assert.Equal(t, string(errs.KindTimeout), samples[1].Error.Kind)
	assert.Equal(t, errs.CodeRequestTimeout, samples[1].Error.Code)
	assert.GreaterOrEqual(t, samples[1].RTTMs, float64(coreShortTimeoutMS))

	assert.Nil(t, samples[2].Error)
	assert.Equal(t, []any{uint16(77)}, samples[2].Values)
}

// A registers-that-do-not-decode poll is a failed sample, not the end of the run.
func TestCorePoll_decodeFailureIsASample(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF)
	var samples []*types.PollSample
	sum, err := Poll(context.Background(), config.PollConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Codec: "datetime_iec870", Interval: 1, Polls: 2,
	}, nil, coreCollectSamples(&samples))
	require.NoError(t, err)
	assert.Equal(t, uint32(2), sum.Failed)
	require.Len(t, samples, 2)
	require.NotNil(t, samples[0].Error)
	assert.Equal(t, string(errs.KindInvalidInput), samples[0].Error.Kind)
	assert.Equal(t, []any{}, samples[0].Values)
	assert.Equal(t, "FF FF FF FF FF FF FF FF", samples[0].RawDataHex)
}

// One dropped connection costs no sample: the poll re-dials and reads again.
func TestCorePoll_survivesDroppedConnection(t *testing.T) {
	dev := mbtest.NewDevice().SetCoils(0, true, false, true)
	var n atomic.Int32
	dev.OnRequest = func(mbtest.Request) mbtest.Action {
		return mbtest.Action{Close: n.Add(1) == 2}
	}

	var samples []*types.PollSample
	sum, err := Poll(context.Background(), config.PollConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 1, RegisterCount: 3, Interval: 1, Polls: 3,
	}, nil, coreCollectSamples(&samples))
	require.NoError(t, err)
	assert.Equal(t, uint32(3), sum.Succeeded)
	assert.Zero(t, sum.Failed)
	assert.Equal(t, 4, dev.RequestCount(1), "the closed request is repeated once")
	for _, s := range samples {
		assert.Equal(t, []any{true, false, true}, s.Values)
	}
}

func TestCorePoll_setupFailures(t *testing.T) {
	t.Run("invalid codec", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		called := false
		sum, err := Poll(context.Background(), config.PollConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Codec: "uint32/layout:4321", RegisterCount: 3, Polls: 1,
		}, nil, func(*types.PollSample) error { called = true; return nil })
		assert.Nil(t, sum)
		e, ok := errs.As(err)
		require.True(t, ok)
		assert.Equal(t, errs.KindInvalidInput, e.Kind)
		assert.Equal(t, errs.CodeInvalidInput, e.Code)
		assert.Contains(t, e.Message, "must be a multiple of the codec register width 2")
		assert.False(t, called)
		assert.Zero(t, dev.RequestCount(0))
	})

	t.Run("connect refused", func(t *testing.T) {
		sum, err := Poll(context.Background(), config.PollConfig{
			DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t), Unit: 1}, Timeout: coreShortTimeoutMS, Function: 3, RegisterCount: 1, Polls: 1,
		}, nil, func(*types.PollSample) error { return nil })
		assert.Nil(t, sum)
		require.ErrorIs(t, err, ErrTCPConnection)
	})
}

func TestCorePoll_emitErrorStopsTheRun(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 1)
	boom := errors.New("stdout closed")
	calls := 0
	sum, err := Poll(context.Background(), config.PollConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1, Interval: 1, Polls: 10,
	}, nil, func(*types.PollSample) error {
		calls++
		if calls == 2 {
			return boom
		}
		return nil
	})
	require.ErrorIs(t, err, boom)
	assert.Nil(t, sum)
	assert.Equal(t, 2, calls)
	assert.Equal(t, 2, dev.RequestCount(3))
}

func TestCorePoll_cancellation(t *testing.T) {
	t.Run("before the first poll", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var samples []*types.PollSample
		sum, err := Poll(ctx, config.PollConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1, Interval: 10,
		}, nil, coreCollectSamples(&samples))
		require.NoError(t, err)
		assert.True(t, sum.Interrupted)
		assert.Zero(t, sum.Polls)
		assert.Empty(t, samples)
		assert.Zero(t, dev.RequestCount(0))
	})

	// Polls 0 runs until interrupted; the interruption arrives during the interval sleep.
	t.Run("unbounded run stopped between polls", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var samples []*types.PollSample
		sum, err := Poll(ctx, config.PollConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1, Interval: 5,
		}, nil, func(s *types.PollSample) error {
			samples = append(samples, s)
			if len(samples) == 3 {
				cancel()
			}
			return nil
		})
		require.NoError(t, err)
		assert.True(t, sum.Interrupted)
		assert.Equal(t, uint32(3), sum.Polls)
		assert.Equal(t, uint32(3), sum.Succeeded)
		assert.Len(t, samples, 3)
		assert.NotEmpty(t, sum.Duration)
	})

	// A read that fails after cancellation ends the run without a failed sample. (The
	// in-flight request itself is not aborted: it still runs into its timeout.)
	t.Run("during a read", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dev.OnRequest = func(mbtest.Request) mbtest.Action {
			cancel()
			return mbtest.Action{Drop: true}
		}
		var samples []*types.PollSample
		sum, err := Poll(ctx, config.PollConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreShortTimeoutMS, Function: 3, RegisterCount: 1, Interval: 5,
		}, nil, coreCollectSamples(&samples))
		require.NoError(t, err)
		assert.True(t, sum.Interrupted)
		assert.Zero(t, sum.Polls)
		assert.Zero(t, sum.Failed)
		assert.Empty(t, samples)
		assert.Equal(t, 1, dev.RequestCount(0))
	})
}
