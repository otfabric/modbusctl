package server

import (
	"context"
	"errors"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay a Modbus TCP register recording as a dynamic Modbus server",
	Example: `
  # Replay a recording from a file (FC from MCAP header; --interval 0 uses captured timing between iterations)
  modbusctl server replay --input=recording.mcap

  # Optional: confirm FC matches the file (e.g. FC4 input registers)
  modbusctl server replay --input=input_regs.mcap --function 4 --port=1502 --unit=1

  # Replay a recording with infinite loops
  modbusctl server replay --input=recording.mcap --loops=0

  # Replay a recording with a specific number of loops
  modbusctl server replay --input=recording.mcap --loops=5

  # Infinite loops with a fixed wall-clock pause between iterations (milliseconds)
  modbusctl server replay --input=recording.mcap --loops=0 --interval=100

  # Use environment variables instead of CLI arguments
  MODBUSCTL_PORT=1502 MODBUSCTL_INPUT=recording.mcap modbusctl server replay
`,
}

func init() {
	cfg := config.ReplayServerConfig{
		Port:      502,
		Unit:      1,
		Loops:     0,
		Interval:  0, // Default to 0 (original timing)
		InputFile: "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(replayCmd, &cfg)
	config.RegisterFunctionCompletion(replayCmd)
	cli.MustMarkFlagRequired(replayCmd, "input")
	ServerCmd.AddCommand(replayCmd)

	replayCmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg.Debug = cli.Debug(cmd)
		if err := validate.CheckReplayServerConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		if err := modbus.LoadMCAPAndServeReplay(cmd.Context(), cfg, cmd.OutOrStdout()); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return modbus.WrapCollectError(err)
		}
		return nil
	}
}
