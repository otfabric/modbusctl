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
	Short: "Serve the values of a capture as a static Modbus TCP (or tcp+tls) device",
	Example: `
  # Serve the last recorded values of every target in a capture, each on its recorded unit id
  modbusctl server static --port 502 --input site.mbcap

  # Serve one target as unit 1, with the values it had 10 minutes into the capture
  modbusctl server static --port 1502 --input site.mbcap --target 2 --unit 1 --at 10m

  # Answer unrecorded registers with zeros instead of an exception
  modbusctl server static --input site.mbcap --missing zero

  # Serve over Modbus/TCP Security: clients need a certificate signed by plant-ca.pem
  modbusctl server static --input site.mbcap --port 802 --tls-cert server.pem --tls-key server-key.pem --tls-ca plant-ca.pem

  # Use environment variables instead of CLI arguments
  MODBUSCTL_PORT=502 MODBUSCTL_INPUT=site.mbcap modbusctl server static
`,
}

func init() {
	cfg := config.StaticServerConfig{
		Port:      502,
		InputFile: "",
	}
	config.DeferLoadFromEnv(&cfg)
	config.RegisterFlags(staticCmd, &cfg)
	cli.MustMarkFlagRequired(staticCmd, "input")
	ServerCmd.AddCommand(staticCmd)

	staticCmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg.Debug = cli.Debug(cmd)
		if err := validate.CheckStaticServerConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		if err := modbus.ServeStatic(cmd.Context(), cfg, cmd.ErrOrStderr()); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return modbus.WrapCollectError(err)
		}
		return nil
	}
}
