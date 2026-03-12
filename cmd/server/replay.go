package server

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var replayCfg config.ReplayServerConfig

var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay a Modbus TCP register recording as a dynamic Modbus server",
	Example: `
  # Replay a recording from a file
  modbusctl server replay --input=recording.mcap

  # Replay a recording with a specific port and unit ID
  modbusctl server replay --input=recording.mcap --port=1502 --unit=1

  # Replay a recording with infinite loops
  modbusctl server replay --input=recording.mcap --loops=0

  # Replay a recording with a specific number of loops
  modbusctl server replay --input=recording.mcap --loops=5

	# Replay a recording with infinite loops and an interval between iterations
  modbusctl server replay --input=recording.mcap --loops=0 --interval=100
  # Use environment variables instead of CLI arguments
  MODBUSCTL_PORT=1502 MODBUSCTL_INPUT=recording.mcap modbusctl server replay
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckReplayServerConfig(replayCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.LoadMCAPAndServeReplay(replayCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to start replay server: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	ServerCmd.AddCommand(replayCmd)

	replayCfg = config.ReplayServerConfig{
		Port:      502,
		Unit:      1,
		Loops:     0,
		Interval:  0, // Default to 0 (original timing)
		InputFile: "",
	}
	config.LoadFromEnv(&replayCfg)
	config.RegisterFlags(replayCmd, &replayCfg)
	config.RegisterFunctionCompletion(replayCmd)
	if replayCfg.InputFile == "" {
		if err := replayCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
}
