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

var reportServerIdCmd = &cobra.Command{
	Use:   "reportserverid",
	Short: "Send Modbus FC17 Report Server ID request to the device",
	Long:  `Connection pooling matches identify: one client for a single unit or --parallel 1; up to --parallel clients when multiple units are selected. --parallel is only range-checked when --unit all.`,
	Example: `
  # Query server ID from unit 1 (default)
  modbusctl client reportserverid --ip 192.168.1.10

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client reportserverid --url tcp://192.168.1.10:502

  # Query a specific unit
  modbusctl client reportserverid --ip 192.168.1.10 --unit 5

  # Query a range of units
  modbusctl client reportserverid --ip 192.168.1.10 --unit 1-10

  # Query all units in parallel
  modbusctl client reportserverid --ip 192.168.1.10 --unit all --parallel 10
`,
}

func init() {
	cfg := config.ReportServerIDConfig{
		OutputFormat: string(format.FormatText),
		UnitClientConfig: config.UnitClientConfig{
			IP:       "",
			Port:     502,
			UnitID:   "1",
			Timeout:  0,
			Parallel: 10,
		},
	}
	runner.WireClientCommand(ClientCmd, reportServerIdCmd, &cfg)

	reportServerIdCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckReportServerIDConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, e := modbus.CollectReportServerID(ctx, cfg)
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			},
			runner.WithSuccessExit(types.SuccessExitForPayload))
	}
}
