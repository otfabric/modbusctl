// SPDX-License-Identifier: MIT

package runner

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ctxKey struct{}

// streamCmd is a subcommand of a root with --debug, wired to an Invocation and a stdout buffer.
func streamCmd(t *testing.T, debug bool) (*cobra.Command, *Invocation, *bytes.Buffer) {
	t.Helper()
	root := &cobra.Command{Use: "modbusctl"}
	root.PersistentFlags().Bool("debug", false, "")
	if debug {
		require.NoError(t, root.PersistentFlags().Set("debug", "true"))
	}
	cmd := &cobra.Command{Use: "poll"}
	root.AddCommand(cmd)
	inv := &Invocation{}
	cmd.SetContext(context.WithValue(WithInvocation(context.Background(), inv), ctxKey{}, "marker"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	return cmd, inv, &out
}

func sample(poll uint32) *types.PollSample {
	return &types.PollSample{Poll: poll, Timestamp: "T", StartAddress: 100, Count: 1, RTTMs: 1, Values: []any{uint16(40 + poll)}}
}

func TestRunClientStream_success(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		wantFmt  format.OutputFormat
		wantOut  string
		exitCode int
	}{
		{"text", "text", format.FormatText, "T  #1  100=41\nT  #2  100=42\n", errs.ExitOK},
		{"default is text", "", format.FormatText, "T  #1  100=41\nT  #2  100=42\n", errs.ExitOK},
		{"json lines", " JSON ", format.FormatJSON,
			`{"poll":1,"timestamp":"T","unit_id":0,"function":0,"start_address":100,"count":1,"address_step":0,"rtt_ms":1,"values":[41]}` + "\n" +
				`{"poll":2,"timestamp":"T","unit_id":0,"function":0,"start_address":100,"count":1,"address_step":0,"rtt_ms":1,"values":[42]}` + "\n",
			errs.ExitPartial},
		{"table", "table", format.FormatTable,
			"poll      timestamp  rtt_ms    100       error\n" +
				"-         -          -         -         -\n" +
				"1         T          1.0       41\n" +
				"2         T          1.0       42\n",
			errs.ExitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, inv, out := streamCmd(t, true)
			var debug, validated bool
			err := RunClientStream(cmd, func(b bool) { debug = b }, tt.format,
				func() error { validated = true; return nil },
				func(ctx context.Context, w *format.StreamWriter) (int, error) {
					assert.True(t, validated, "validate must run before run")
					assert.Equal(t, "marker", ctx.Value(ctxKey{}), "run must receive the command context")
					require.NoError(t, w.Write(sample(1)))
					require.NoError(t, w.Write(sample(2)))
					return tt.exitCode, nil
				})
			require.NoError(t, err)
			assert.True(t, debug)
			assert.Equal(t, tt.wantOut, out.String())
			assert.Equal(t, Invocation{OutputFormat: tt.wantFmt, HasOutputFormat: true, SuccessExitCode: tt.exitCode, SuccessExitSet: true}, *inv)
		})
	}
}

func TestRunClientStream_formatError(t *testing.T) {
	cmd, inv, out := streamCmd(t, false)
	debug := true
	err := RunClientStream(cmd, func(b bool) { debug = b }, "yaml",
		func() error { t.Fatal("validate must not run"); return nil },
		func(context.Context, *format.StreamWriter) (int, error) { t.Fatal("run must not run"); return 0, nil })

	e, ok := errs.As(err)
	require.True(t, ok, "want a typed error, got %v", err)
	assert.Equal(t, errs.KindInvalidInput, e.Kind)
	assert.Equal(t, errs.CodeInvalidFormat, e.Code)
	assert.Equal(t, `invalid output format: invalid format "yaml" (expected: text, json, table)`, e.Message)
	assert.False(t, debug, "debug is propagated even when the format is rejected")
	assert.Equal(t, Invocation{}, *inv)
	assert.Empty(t, out.String())
}

func TestRunClientStream_validationError(t *testing.T) {
	t.Run("plain error is wrapped as invalid input", func(t *testing.T) {
		cmd, inv, out := streamCmd(t, false)
		cause := errors.New("interval must be at least 1 millisecond")
		err := RunClientStream(cmd, func(bool) {}, "json",
			func() error { return cause },
			func(context.Context, *format.StreamWriter) (int, error) { t.Fatal("run must not run"); return 0, nil })

		e, ok := errs.As(err)
		require.True(t, ok)
		assert.Equal(t, errs.KindInvalidInput, e.Kind)
		assert.Equal(t, errs.CodeInvalidInput, e.Code)
		assert.Equal(t, "invalid input: interval must be at least 1 millisecond", e.Message)
		assert.ErrorIs(t, err, cause)
		// The format is known by then, so a fatal can be rendered as JSON; no success exit is recorded.
		assert.Equal(t, Invocation{OutputFormat: format.FormatJSON, HasOutputFormat: true}, *inv)
		assert.Empty(t, out.String())
	})
	t.Run("typed error is passed through", func(t *testing.T) {
		cmd, _, _ := streamCmd(t, false)
		typed := errs.InvalidInput(errs.CodeInvalidUnitSelector, "bad unit", nil)
		err := RunClientStream(cmd, func(bool) {}, "text",
			func() error { return typed },
			func(context.Context, *format.StreamWriter) (int, error) { return 0, nil })
		assert.Same(t, typed, err)
	})
}

func TestRunClientStream_runError(t *testing.T) {
	cmd, inv, out := streamCmd(t, false)
	boom := errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "connection refused", nil)
	err := RunClientStream(cmd, func(bool) {}, "text",
		func() error { return nil },
		func(_ context.Context, w *format.StreamWriter) (int, error) {
			require.NoError(t, w.Write(sample(1)))
			return errs.ExitPartial, boom
		})
	assert.Same(t, boom, err, "the run error is returned unchanged")
	assert.Equal(t, "T  #1  100=41\n", out.String(), "records written before the failure stay on stdout")
	assert.False(t, inv.SuccessExitSet, "a failed run must not record a success exit code")
	assert.Zero(t, inv.SuccessExitCode)
}

func TestRunClientStream_withoutInvocation(t *testing.T) {
	cmd := &cobra.Command{Use: "poll"}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	debug := true
	err := RunClientStream(cmd, func(b bool) { debug = b }, "text",
		func() error { return nil },
		func(_ context.Context, w *format.StreamWriter) (int, error) {
			return errs.ExitPartial, w.Write(sample(3))
		})
	require.NoError(t, err)
	assert.False(t, debug, "no --debug flag on the root means debug is off")
	assert.Equal(t, "T  #3  100=43\n", out.String())
}

func TestRunFormatted(t *testing.T) {
	cmd, inv, out := streamCmd(t, false)
	rr, err := RunFormatted(cmd, "table", func(ctx context.Context) (any, error) {
		assert.Equal(t, "marker", ctx.Value(ctxKey{}))
		return &types.WriteResult{Function: 6, StartAddress: 7, Count: 1, Registers: []uint16{1}}, nil
	}, WithSuccessExit(func(v any) int {
		assert.IsType(t, &types.WriteResult{}, v)
		return errs.ExitPartial
	}))
	require.NoError(t, err)
	assert.Equal(t, RunResult{ExitCode: errs.ExitPartial}, rr)
	assert.Equal(t, "address  type      value\n-        -         -\n7        register  1 (0x0001)\n", out.String())
	assert.Equal(t, Invocation{OutputFormat: format.FormatTable, HasOutputFormat: true, SuccessExitCode: errs.ExitPartial, SuccessExitSet: true}, *inv)

	// A bad format is rejected before collect runs.
	cmd, inv, out = streamCmd(t, false)
	rr, err = RunFormatted(cmd, "xml", func(context.Context) (any, error) { t.Fatal("collect must not run"); return nil, nil })
	e, ok := errs.As(err)
	require.True(t, ok)
	assert.Equal(t, errs.CodeInvalidFormat, e.Code)
	assert.Equal(t, RunResult{}, rr)
	assert.Equal(t, Invocation{}, *inv)
	assert.Empty(t, out.String())
}

func TestRunClientFormatted_paths(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd, inv, out := streamCmd(t, false)
		order := ""
		err := RunClientFormatted(cmd, "text",
			func() error { order += "v"; return nil },
			func(context.Context) (any, error) {
				order += "c"
				return &types.PollSummary{Target: "t", Polls: 1, Succeeded: 1, Duration: "1s"}, nil
			})
		// PollSummary has no text rendering, so the write fails as an output error.
		e, ok := errs.As(err)
		require.True(t, ok)
		assert.Equal(t, errs.KindOutput, e.Kind)
		assert.Equal(t, "text output is not supported for this result type", e.Message)
		assert.Equal(t, "vc", order)
		assert.False(t, inv.SuccessExitSet)
		assert.Empty(t, out.String())

		cmd, inv, out = streamCmd(t, false)
		require.NoError(t, RunClientFormatted(cmd, "text", func() error { return nil },
			func(context.Context) (any, error) { return sample(1), nil }))
		assert.Equal(t, "T  #1  100=41\n", out.String())
		assert.Equal(t, Invocation{OutputFormat: format.FormatText, HasOutputFormat: true, SuccessExitSet: true}, *inv)
	})
	t.Run("format error", func(t *testing.T) {
		cmd, _, _ := streamCmd(t, false)
		err := RunClientFormatted(cmd, "nope",
			func() error { t.Fatal("validate must not run"); return nil },
			func(context.Context) (any, error) { t.Fatal("collect must not run"); return nil, nil })
		e, ok := errs.As(err)
		require.True(t, ok)
		assert.Equal(t, errs.CodeInvalidFormat, e.Code)
	})
	t.Run("collect error", func(t *testing.T) {
		cmd, inv, out := streamCmd(t, false)
		boom := errors.New("device unreachable")
		err := RunClientFormatted(cmd, "json", func() error { return nil },
			func(context.Context) (any, error) { return sample(1), boom })
		assert.Same(t, boom, err)
		assert.Empty(t, out.String())
		assert.False(t, inv.SuccessExitSet)
	})
}

func TestOutputWriteErr(t *testing.T) {
	cause := errors.New("disk full")
	for f, code := range map[format.OutputFormat]string{
		format.FormatJSON:  errs.CodeJSONEncodeFailed,
		format.FormatTable: errs.CodeTableRenderFailed,
	} {
		e, ok := errs.As(outputWriteErr(f, cause))
		require.True(t, ok)
		assert.Equal(t, errs.KindOutput, e.Kind)
		assert.Equal(t, code, e.Code, string(f))
		assert.Equal(t, "disk full", e.Message)
		assert.ErrorIs(t, e, cause)
	}
	e, ok := errs.As(outputWriteErr(format.FormatText, cause))
	require.True(t, ok)
	assert.Equal(t, errs.KindOutput, e.Kind)
	assert.ErrorIs(t, e, cause)
}

func TestRegisterClientCommandCfg(t *testing.T) {
	t.Setenv("MODBUSCTL_PARITY", "odd")
	t.Setenv("MODBUSCTL_TIMEOUT", "750")
	var cfg config.PollConfig
	cmd := &cobra.Command{Use: "poll"}
	var hooks []string
	RegisterClientCommandCfg(cmd, &cfg,
		func(c *cobra.Command) { hooks = append(hooks, "first:"+c.Name()) },
		nil,
		func(c *cobra.Command) { hooks = append(hooks, "second:"+c.Name()) },
	)

	assert.Equal(t, []string{"first:poll", "second:poll"}, hooks)
	assert.Equal(t, "odd", cfg.Parity)
	assert.Equal(t, uint16(750), cfg.Timeout)
	assert.Equal(t, "750", cmd.Flags().Lookup("timeout").DefValue)

	// Flags override the environment and write straight into the config.
	require.NoError(t, cmd.Flags().Parse([]string{"--url", "rtu://COM3", "--parity", "even", "--interval", "250"}))
	assert.Equal(t, "rtu://COM3", cfg.URL)
	assert.Equal(t, "even", cfg.Parity)
	assert.Equal(t, uint32(250), cfg.Interval)

	fn, ok := cmd.GetFlagCompletionFunc("format")
	require.True(t, ok)
	got, _ := fn(cmd, nil, "t")
	assert.Equal(t, []string{"table\thuman-readable tabular output", "text\thuman-readable default"}, got)

	fn, ok = cmd.GetFlagCompletionFunc("parity")
	require.True(t, ok)
	got, _ = fn(cmd, nil, "")
	assert.Equal(t, []string{"none", "even", "odd"}, got)
}

func TestRegisterClientCommandCfg_invalidEnvironmentIsDeferred(t *testing.T) {
	t.Setenv("MODBUSCTL_TIMEOUT", "soon")
	var cfg config.PollConfig
	assert.NotPanics(t, func() { RegisterClientCommandCfg(&cobra.Command{Use: "poll"}, &cfg) })
	assert.EqualError(t, config.EnvError(), `MODBUSCTL_TIMEOUT="soon": strconv.ParseUint: parsing "soon": invalid syntax`)
}
