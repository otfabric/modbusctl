package server

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var staticCfg config.StaticServerConfig

var staticCmd = &cobra.Command{
	Use:   "static",
	Short: "Host a static Modbus TCP server with fixed register values",
	Example: `
  # Host a static Modbus TCP server with fixed register values from an MCAP file
  modbusctl server static --port 502 --unit 1 --input mydata.mcap

  # Host a static server with a specific port and unit ID
  modbusctl server static --port 1502 --unit 2 --input mydata.mcap

  # Use environment variables instead of CLI arguments
  MODBUSCTL_PORT=502 MODBUSCTL_UNIT=1 MODBUSCTL_INPUT=mydata.mcap modbusctl server static
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckStaticServerConfig(staticCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.LoadMCAPAndServeStatic(staticCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to start static server: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	ServerCmd.AddCommand(staticCmd)
	staticCfg = config.StaticServerConfig{

		Port:      502,
		Unit:      1,
		InputFile: "",
	}
	config.LoadFromEnv(&staticCfg)
	config.RegisterFlags(staticCmd, &staticCfg)
	config.RegisterFunctionCompletion(staticCmd)
	if staticCfg.InputFile == "" {
		if err := staticCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
}
