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

var staticCmd = &cobra.Command{
	Use:   "static",
	Short: "Host a static Modbus TCP server with fixed register values",
	Example: `
  # Serve last-known values from an MCAP file (function code comes from the MCAP header)
  modbusctl server static --port 502 --unit 1 --input mydata.mcap

  # Override is only for verification: --function must match the file's FC (here FC3 holding registers)
  modbusctl server static --port 502 --unit 1 --input holding.mcap --function 3

  # Use environment variables instead of CLI arguments
  MODBUSCTL_PORT=502 MODBUSCTL_UNIT=1 MODBUSCTL_INPUT=mydata.mcap modbusctl server static
`,
}

func init() {
	cfg := config.StaticServerConfig{
		Port:      502,
		Unit:      1,
		InputFile: "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(staticCmd, &cfg)
	config.RegisterFunctionCompletion(staticCmd)
	cli.MustMarkFlagRequired(staticCmd, "input")
	ServerCmd.AddCommand(staticCmd)

	staticCmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg.Debug = cli.Debug(cmd)
		if err := validate.CheckStaticServerConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		if err := modbus.LoadMCAPAndServeStatic(cmd.Context(), cfg, cmd.OutOrStdout()); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return modbus.WrapCollectError(err)
		}
		return nil
	}
}
