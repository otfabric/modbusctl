package client

import (
	"context"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var identifyCmd = &cobra.Command{
	Use:   "identify",
	Short: "Send Modbus FC43/14 request to identify the Modbus TCP device",
	Long:  `Uses one TCP connection when probing a single unit or when --parallel is 1. With multiple unit IDs and --parallel > 1, opens that many clients and distributes units across them (same model as reportserverid). The --parallel flag is only validated when --unit all; for explicit unit lists it is ignored if set to 1.`,
	Example: `
  # Identify a device (all categories: basic + regular + extended)
  modbusctl client identify --ip 192.168.1.10

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client identify --url tcp://192.168.1.10:502

  # Also retrieve FC17 Report Server ID information
  modbusctl client identify --ip 192.168.1.10 --server-id

  # Request only specific category or combine --basic, --regular, --extended
  modbusctl client identify --ip 192.168.1.10 --basic
  modbusctl client identify --ip 192.168.1.10 --basic --regular

  # Unit: single (1), range (1-10), list (1,5,25), mixed (1-10,255), or all (1-255); use --parallel with multiple units
  modbusctl client identify --ip 192.168.1.10 --unit 1
  modbusctl client identify --ip 192.168.1.10 --unit 1-10
  modbusctl client identify --ip 192.168.1.10 --unit 1,5,25
  modbusctl client identify --ip 192.168.1.10 --unit all
  modbusctl client identify --ip 192.168.1.10 --unit all --parallel 10
  MODBUSCTL_IP=192.168.1.10 modbusctl client identify
`,
}

func init() {
	cfg := config.IdentifyConfig{
		OutputFormat: string(format.FormatText),
		UnitClientConfig: config.UnitClientConfig{
			IP:       "",
			Port:     502,
			UnitID:   "1",
			Timeout:  0,
			Parallel: 10,
		},
	}
	runner.WireClientCommand(ClientCmd, identifyCmd, &cfg)

	identifyCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckIdentifyConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, e := modbus.CollectDeviceIdentification(ctx, cfg)
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			},
			runner.WithSuccessExit(types.SuccessExitForPayload))
	}
}
