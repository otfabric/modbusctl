package client

import (
	"context"
	"fmt"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var pollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Continuously read coils or registers at an interval and print every sample",
	Long: `Read the same block of coils, discrete inputs or registers every --interval
milliseconds and print one sample per poll. Nothing is written to disk; use
"modbusctl client record" to capture to MCAP instead.

Polling runs until --polls samples were taken, or until interrupted (Ctrl-C) when --polls
is 0. With --codec, registers are decoded into typed values exactly as "client decode" does.

Each sample goes to stdout as it is read: one line with --format text, one JSON object per
line with --format json, or one table row with --format table. A summary is printed to
stderr at the end.

A poll that fails (timeout, Modbus exception, lost connection) is reported as a sample with
an error and polling continues, reconnecting when needed. Exit code 7 (partial success)
means at least one poll failed — see docs/exitcodes.md.`,
	Example: `
  # Poll 4 holding registers every second until Ctrl-C
  modbusctl client poll --ip 192.168.1.10 --start 100 --count 4

  # Poll a float32 input register pair every 500 ms, 20 times
  modbusctl client poll --ip 192.168.1.10 --function 4 --start 0 --codec float32/layout:4321 --interval 500 --polls 20

  # Poll 8 coils and emit JSON Lines for a script
  modbusctl client poll --ip 192.168.1.10 --function 1 --start 0 --count 8 --format json

  # Poll a serial RTU device
  modbusctl client poll --url rtu:///dev/ttyUSB0 --baud 9600 --unit 3 --start 0 --count 2
`,
}

func init() {
	cfg := config.PollConfig{
		DeviceConfig: config.DeviceConfig{
			Port: 502,
			Unit: 1,
		},
		Function:     3,
		Interval:     1000,
		OutputFormat: string(format.FormatText),
	}
	runner.WireClientCommand(ClientCmd, pollCmd, &cfg, config.RegisterFunctionCompletion, config.RegisterCodecCompletion)

	pollCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientStream(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckPollConfig(resolvePollCount(cmd, cfg)) },
			func(ctx context.Context, out *format.StreamWriter) (int, error) {
				emit := func(s *types.PollSample) error {
					if err := out.Write(s); err != nil {
						return errs.Output(errs.CodeOutputFileWriteFailed, err)
					}
					return nil
				}
				summary, err := modbus.Poll(ctx, resolvePollCount(cmd, cfg), cmd.ErrOrStderr(), emit)
				if err != nil {
					return 0, modbus.WrapCollectError(err)
				}
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), summary)
				if summary.Failed > 0 {
					return errs.ExitPartial, nil
				}
				return errs.ExitOK, nil
			})
	}
}

// resolvePollCount applies the --count default, which depends on --codec: one coil/register
// without a codec, one codec value (count 0) with one.
func resolvePollCount(cmd *cobra.Command, cfg config.PollConfig) config.PollConfig {
	if cfg.RegisterCount == 0 && cfg.Codec == "" && !cmd.Flags().Changed("count") {
		cfg.RegisterCount = 1
	}
	return cfg
}
