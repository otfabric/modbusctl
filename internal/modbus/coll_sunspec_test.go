package modbus

import (
	"context"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/require"
)

// collSunSpecRegs is a SunSpec block: marker, model 1 (length 4), model 103 (length 2), end model.
func collSunSpecRegs() []uint16 {
	return []uint16{
		0x5375, 0x6E53, // "SunS"
		1, 4, 11, 12, 13, 14, // model 1
		103, 2, 21, 22, // model 103
		0xFFFF, 0, // end model
	}
}

// collSunSpecModels is the header chain of collSunSpecRegs placed at base.
func collSunSpecModels(base uint16) []types.SunSpecModelHeader {
	return []types.SunSpecModelHeader{
		{ID: 1, Length: 4, StartAddress: base + 2, EndAddress: base + 7, NextAddress: base + 8},
		{ID: 103, Length: 2, StartAddress: base + 8, EndAddress: base + 11, NextAddress: base + 12},
		{ID: 0xFFFF, Length: 0, StartAddress: base + 12, EndAddress: base + 13, NextAddress: base + 14, IsEndModel: true},
	}
}

func collSunSpecBase(url string) config.SunSpecBaseConfig {
	return config.SunSpecBaseConfig{URL: url, Unit: 1, Timeout: 2000, Regtype: "holding"}
}

func TestCollSunSpecDetect(t *testing.T) {
	t.Run("default bases stop at the first match", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(40000, collSunSpecRegs()...)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		base.Unit = 4
		base.Verbose = true

		out, err := CollectSunSpecDetect(context.Background(), config.SunSpecDetectConfig{SunSpecBaseConfig: base})
		require.NoError(t, err)
		require.Equal(t, base.URL, out.Target)
		require.Equal(t, uint8(4), out.UnitID)
		require.Equal(t, "holding", out.Regtype)
		require.True(t, out.Verbose)
		require.True(t, out.Detected)
		require.Equal(t, uint16(40000), out.BaseAddress)

		// Default order is 0, 40000, ...: base 0 answers with an exception, 40000 matches.
		require.Len(t, out.Attempts, 2)
		require.Equal(t, 1, out.Attempts[0].Index)
		require.Equal(t, uint16(0), out.Attempts[0].BaseAddress)
		require.False(t, out.Attempts[0].Matched)
		require.NotEmpty(t, out.Attempts[0].Result)
		require.NotEqual(t, "matched SunS", out.Attempts[0].Result)
		require.NotEqual(t, "no marker", out.Attempts[0].Result, "a failed read must report its error")
		require.Equal(t, types.SunSpecProbeAttempt{Index: 2, BaseAddress: 40000, Matched: true, Result: "matched SunS"}, out.Attempts[1])
		require.Equal(t, 2, dev.RequestCount(0x03))
		for _, r := range dev.Requests() {
			require.Equal(t, uint8(4), r.Unit)
		}
	})

	t.Run("custom bases and input registers", func(t *testing.T) {
		dev := mbtest.NewDevice().
			SetInput(50000, collSunSpecRegs()...).
			SetInput(100, 0x1234, 0x5678).
			SetHolding(100, 0x5375, 0x6E53)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		base.Regtype = "input"

		out, err := CollectSunSpecDetect(context.Background(), config.SunSpecDetectConfig{SunSpecBaseConfig: base, Bases: "100, 50000, 40000"})
		require.NoError(t, err)
		require.True(t, out.Detected)
		require.Equal(t, "input", out.Regtype)
		require.Equal(t, uint16(50000), out.BaseAddress)
		require.Equal(t, []types.SunSpecProbeAttempt{
			{Index: 1, BaseAddress: 100, Matched: false, Result: "no marker"},
			{Index: 2, BaseAddress: 50000, Matched: true, Result: "matched SunS"},
		}, out.Attempts)
		require.Zero(t, dev.RequestCount(0x03), "regtype input must use FC04 only")
		require.Equal(t, 2, dev.RequestCount(0x04))
	})

	t.Run("not detected", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1, 2).SetHolding(40000, 3, 4)
		out, err := CollectSunSpecDetect(context.Background(), config.SunSpecDetectConfig{
			SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t)), Bases: "0,40000",
		})
		require.NoError(t, err)
		require.False(t, out.Detected)
		require.Zero(t, out.BaseAddress)
		require.Len(t, out.Attempts, 2)
		for _, a := range out.Attempts {
			require.False(t, a.Matched)
			require.Equal(t, "no marker", a.Result)
		}
	})

	t.Run("every probe fails", func(t *testing.T) {
		dev := mbtest.NewDevice()
		out, err := CollectSunSpecDetect(context.Background(), config.SunSpecDetectConfig{
			SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t)),
		})
		require.Nil(t, out)
		e := collTypedErr(t, err, errs.KindProtocol, errs.CodeSunSpecDetectFailed)
		require.Error(t, e.Cause)
	})

	t.Run("invalid bases", func(t *testing.T) {
		out, err := CollectSunSpecDetect(context.Background(), config.SunSpecDetectConfig{
			SunSpecBaseConfig: collSunSpecBase("tcp://" + collClosedAddr(t)), Bases: "40000,x",
		})
		require.Nil(t, out)
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	})
}

// collSunSpecSetupErrors runs the target and connection failures shared by every SunSpec collector.
func collSunSpecSetupErrors(t *testing.T, call func(base config.SunSpecBaseConfig) (any, error)) {
	t.Helper()
	t.Run("no target", func(t *testing.T) {
		_, err := call(config.SunSpecBaseConfig{Unit: 1})
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidURL)
	})
	t.Run("unknown scheme", func(t *testing.T) {
		_, err := call(collSunSpecBase("bogus://127.0.0.1:1"))
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	})
	t.Run("closed port", func(t *testing.T) {
		_, err := call(collSunSpecBase("tcp://" + collClosedAddr(t)))
		require.ErrorIs(t, err, ErrTCPConnection)
	})
}

func TestCollSunSpec_setupErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("detect", func(t *testing.T) {
		collSunSpecSetupErrors(t, func(b config.SunSpecBaseConfig) (any, error) {
			out, err := CollectSunSpecDetect(ctx, config.SunSpecDetectConfig{SunSpecBaseConfig: b})
			require.Nil(t, out)
			return out, err
		})
	})
	t.Run("models", func(t *testing.T) {
		collSunSpecSetupErrors(t, func(b config.SunSpecBaseConfig) (any, error) {
			out, err := CollectSunSpecModels(ctx, config.SunSpecModelsConfig{SunSpecBaseConfig: b})
			require.Nil(t, out)
			return out, err
		})
	})
	t.Run("map", func(t *testing.T) {
		collSunSpecSetupErrors(t, func(b config.SunSpecBaseConfig) (any, error) {
			out, err := CollectSunSpecMap(ctx, config.SunSpecMapConfig{SunSpecBaseConfig: b})
			require.Nil(t, out)
			return out, err
		})
	})
	t.Run("probe", func(t *testing.T) {
		collSunSpecSetupErrors(t, func(b config.SunSpecBaseConfig) (any, error) {
			out, err := CollectSunSpecProbe(ctx, config.SunSpecProbeConfig{SunSpecBaseConfig: b})
			require.Nil(t, out)
			return out, err
		})
	})
}

func TestCollSunSpecModels(t *testing.T) {
	ctx := context.Background()

	t.Run("detects the base", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(40000, collSunSpecRegs()...)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		out, err := CollectSunSpecModels(ctx, config.SunSpecModelsConfig{SunSpecBaseConfig: base})
		require.NoError(t, err)
		require.Equal(t, base.URL, out.Target)
		require.False(t, out.NotDetected)
		require.Equal(t, uint16(40000), out.Base)
		require.Equal(t, collSunSpecModels(40000), out.Models)
	})

	t.Run("known base skips detection", func(t *testing.T) {
		dev := mbtest.NewDevice().SetInput(1000, collSunSpecRegs()...)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		base.Regtype = "input"
		out, err := CollectSunSpecModels(ctx, config.SunSpecModelsConfig{SunSpecBaseConfig: base, Base: 1000})
		require.NoError(t, err)
		require.Equal(t, uint16(1000), out.Base)
		require.Equal(t, collSunSpecModels(1000), out.Models)
		require.Equal(t, 3, dev.RequestCount(0), "only the three model headers are read")
	})

	t.Run("max models", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(40000, collSunSpecRegs()...)
		out, err := CollectSunSpecModels(ctx, config.SunSpecModelsConfig{
			SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t)), MaxModels: 1,
		})
		require.NoError(t, err)
		require.Equal(t, collSunSpecModels(40000)[:1], out.Models)
	})

	t.Run("address span exceeded", func(t *testing.T) {
		for _, known := range []uint16{0, 40000} {
			dev := mbtest.NewDevice().SetHolding(40000, collSunSpecRegs()...)
			out, err := CollectSunSpecModels(ctx, config.SunSpecModelsConfig{
				SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t)), Base: known, MaxAddressSpan: 4,
			})
			require.Nil(t, out)
			collRequireErr(t, err, errs.KindProtocol, errs.CodeSunSpecModelHeadersFailed)
		}
	})

	t.Run("not detected", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1, 2)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		out, err := CollectSunSpecModels(ctx, config.SunSpecModelsConfig{SunSpecBaseConfig: base})
		require.NoError(t, err)
		require.Equal(t, &types.SunSpecModelsOutput{Target: base.URL, NotDetected: true}, out)
	})

	t.Run("detection fails", func(t *testing.T) {
		dev := mbtest.NewDevice()
		out, err := CollectSunSpecModels(ctx, config.SunSpecModelsConfig{SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t))})
		require.Nil(t, out)
		collRequireErr(t, err, errs.KindProtocol, errs.CodeSunSpecDetectFailed)
	})

	t.Run("marker without model chain", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(40000, 0x5375, 0x6E53)
		out, err := CollectSunSpecModels(ctx, config.SunSpecModelsConfig{SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t))})
		require.Nil(t, out)
		collRequireErr(t, err, errs.KindProtocol, errs.CodeSunSpecModelHeadersFailed)
	})
}

func TestCollSunSpecMap(t *testing.T) {
	ctx := context.Background()

	t.Run("detects the base", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(40000, collSunSpecRegs()...)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		out, err := CollectSunSpecMap(ctx, config.SunSpecMapConfig{SunSpecBaseConfig: base, ShowHeaderRegs: true, Compact: true})
		require.NoError(t, err)
		require.Equal(t, &types.SunSpecMapOutput{
			Target:         base.URL,
			Base:           40000,
			MarkerRegs:     "40000-40001",
			Models:         collSunSpecModels(40000),
			ShowHeaderRegs: true,
			Compact:        true,
		}, out)
	})

	t.Run("known base skips detection", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(200, collSunSpecRegs()...)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		out, err := CollectSunSpecMap(ctx, config.SunSpecMapConfig{SunSpecBaseConfig: base, Base: 200, ShowNext: true})
		require.NoError(t, err)
		require.Equal(t, &types.SunSpecMapOutput{
			Target:     base.URL,
			Base:       200,
			MarkerRegs: "200-201",
			Models:     collSunSpecModels(200),
			ShowNext:   true,
		}, out)
		require.Equal(t, 3, dev.RequestCount(0), "only the three model headers are read")
	})

	t.Run("known base without model chain", func(t *testing.T) {
		dev := mbtest.NewDevice()
		out, err := CollectSunSpecMap(ctx, config.SunSpecMapConfig{SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t)), Base: 200})
		require.Nil(t, out)
		collRequireErr(t, err, errs.KindProtocol, errs.CodeSunSpecModelHeadersFailed)
	})

	t.Run("not detected", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1, 2)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		out, err := CollectSunSpecMap(ctx, config.SunSpecMapConfig{SunSpecBaseConfig: base})
		require.NoError(t, err)
		require.Equal(t, &types.SunSpecMapOutput{Target: base.URL, NotDetected: true}, out)
	})

	t.Run("detection fails", func(t *testing.T) {
		dev := mbtest.NewDevice()
		out, err := CollectSunSpecMap(ctx, config.SunSpecMapConfig{SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t))})
		require.Nil(t, out)
		collRequireErr(t, err, errs.KindProtocol, errs.CodeSunSpecDetectFailed)
	})

	t.Run("marker without model chain", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(40000, 0x5375, 0x6E53)
		out, err := CollectSunSpecMap(ctx, config.SunSpecMapConfig{SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t))})
		require.Nil(t, out)
		collRequireErr(t, err, errs.KindProtocol, errs.CodeSunSpecModelHeadersFailed)
	})
}

func TestCollSunSpecProbe(t *testing.T) {
	ctx := context.Background()

	t.Run("sunspec device", func(t *testing.T) {
		dev := collIdentDevice().SetHolding(0, 7).SetInput(0, 7).SetHolding(40000, collSunSpecRegs()...)
		base := collSunSpecBase("tcp://" + dev.ServeTCP(t))
		base.Unit = 3
		out, err := CollectSunSpecProbe(ctx, config.SunSpecProbeConfig{SunSpecBaseConfig: base})
		require.NoError(t, err)
		require.Equal(t, base.URL, out.Target)
		require.Equal(t, uint8(3), out.UnitID)
		require.Equal(t, types.SunSpecProbeModbus{FC03: true, FC04: true, FC43: true}, out.Modbus)
		require.Equal(t, types.SunSpecProbeSummary{Detected: true, BaseAddress: 40000, ModelsFound: 3, EndModel: true}, out.SunSpecDetail)
		require.False(t, out.HasPartialFailure())
	})

	t.Run("chain without end model", func(t *testing.T) {
		regs := collSunSpecRegs()
		dev := mbtest.NewDevice().SetHolding(40000, regs[:len(regs)-2]...)
		out, err := CollectSunSpecProbe(ctx, config.SunSpecProbeConfig{SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t))})
		require.NoError(t, err)
		require.True(t, out.SunSpecDetail.Detected)
		require.Equal(t, uint16(40000), out.SunSpecDetail.BaseAddress)
		require.NotNil(t, out.SunSpecDetail.ModelHeadersError)
		require.NotEmpty(t, out.SunSpecDetail.ModelHeadersError.Message)
		require.Zero(t, out.SunSpecDetail.ModelsFound)
		require.False(t, out.SunSpecDetail.EndModel)
		require.Nil(t, out.SunSpecDetail.DetectionError)
		require.True(t, out.HasPartialFailure())
	})

	t.Run("plain modbus device", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1, 2)
		out, err := CollectSunSpecProbe(ctx, config.SunSpecProbeConfig{SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t))})
		require.NoError(t, err)
		require.True(t, out.Modbus.FC03)
		require.Nil(t, out.Modbus.FC03Error)
		require.Nil(t, out.Modbus.FC04Error)
		require.Nil(t, out.Modbus.FC43Error)
		require.Equal(t, types.SunSpecProbeSummary{}, out.SunSpecDetail)
		require.False(t, out.HasPartialFailure())
	})

	t.Run("connection lost", func(t *testing.T) {
		dev := mbtest.NewDevice()
		dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Close: true} }
		out, err := CollectSunSpecProbe(ctx, config.SunSpecProbeConfig{SunSpecBaseConfig: collSunSpecBase("tcp://" + dev.ServeTCP(t))})
		require.NoError(t, err)
		require.False(t, out.Modbus.FC03)
		require.NotNil(t, out.Modbus.FC03Error)
		require.NotNil(t, out.Modbus.FC04Error)
		require.NotNil(t, out.Modbus.FC43Error)
		require.NotEmpty(t, out.Modbus.FC03Error.Message)
		require.NotNil(t, out.SunSpecDetail.DetectionError)
		require.False(t, out.SunSpecDetail.Detected)
		require.True(t, out.HasPartialFailure())
	})
}

func TestCollSunSpecOperationContext(t *testing.T) {
	tests := []struct {
		name      string
		timeoutMS uint16
		want      time.Duration
	}{
		{name: "default timeout doubles to 20s", timeoutMS: 0, want: 20 * time.Second},
		{name: "small timeouts use the 5s floor", timeoutMS: 100, want: 10 * time.Second},
		{name: "scales with the timeout", timeoutMS: 30000, want: time.Minute},
		{name: "capped at two minutes", timeoutMS: 65535, want: 2 * time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now()
			ctx, cancel := sunspecOperationContext(context.Background(), &config.SunSpecBaseConfig{Timeout: tc.timeoutMS})
			defer cancel()
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.WithinDuration(t, before.Add(tc.want), deadline, time.Second)
		})
	}

	t.Run("parent cancellation propagates", func(t *testing.T) {
		parent, stop := context.WithCancel(context.Background())
		ctx, cancel := sunspecOperationContext(parent, &config.SunSpecBaseConfig{})
		defer cancel()
		stop()
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})
}

func TestCollSunSpecHelpers(t *testing.T) {
	require.Equal(t, time.Duration(10*time.Second), sunspecClientDialTimeout(&config.SunSpecBaseConfig{}))
	require.Equal(t, 250*time.Millisecond, sunspecClientDialTimeout(&config.SunSpecBaseConfig{Timeout: 250}))

	opts := sunSpecOptionsFromBase(&config.SunSpecBaseConfig{Unit: 9, Regtype: "input"}, nil)
	require.Equal(t, uint8(9), opts.UnitID)
	require.Equal(t, regTypeFromString("input"), opts.RegType)
	require.NotEqual(t, regTypeFromString("input"), regTypeFromString("holding"))
	require.Equal(t, regTypeFromString("holding"), regTypeFromString(""), "holding is the default register type")
	require.Equal(t, 256, opts.MaxModels)
	require.Equal(t, []uint16{0, 40000, 50000, 1, 39999, 40001, 49999, 50001}, opts.BaseAddresses)

	opts = sunSpecOptionsFromBase(&config.SunSpecBaseConfig{}, []uint16{7})
	require.Equal(t, []uint16{7}, opts.BaseAddresses)

	require.Empty(t, sunSpecModelHeaders(nil))
	require.NotNil(t, sunSpecModelHeaders(nil), "JSON output needs [] rather than null")
}
