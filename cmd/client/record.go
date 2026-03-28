package client

import (
	"context"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Continuously read and record registers from a Modbus TCP device",
	Example: `
  # Record holding registers from a Modbus device at 192.168.1.100 every 5 seconds for 1 minute
  modbusctl client record --ip 192.168.1.100 --blocks-file blocks.json --output data.mcap --interval 5 --duration 60

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client record --url tcp://192.168.1.100:502 --blocks-file blocks.json --output data.mcap --interval 5 --duration 60

  # Record input registers with function code 4, polling every 10 seconds for 2 minutes
  modbusctl client record --ip 192.168.1.100 --function 4 --blocks-file register-ranges.json --output data.mcap --interval 10 --duration 120

  # Use environment variables instead of CLI arguments
  MODBUSCTL_IP=192.168.1.100 MODBUSCTL_BLOCKS_FILE=blocks.json MODBUSCTL_OUTPUT=data.mcap MODBUSCTL_INTERVAL=5 MODBUSCTL_DURATION=60 modbusctl client record
`,
}

func init() {
	cfg := config.RecordConfig{
		DeviceConfig: config.DeviceConfig{
			IP:   "",
			Port: 502,
			Unit: 1,
		},
		Timeout:      0,
		Function:     3,
		BlocksFile:   "",
		Interval:     5000,
		Duration:     60000,
		OutputFile:   "",
		OutputFormat: string(format.FormatText),
		Debug:        false,
	}
	runner.WireClientCommand(ClientCmd, recordCmd, &cfg, config.RegisterFunctionCompletion, func(c *cobra.Command) {
		cli.MustMarkFlagRequired(c, "blocks-file")
	})

	recordCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckRecordConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, e := modbus.RecordAndWriteMCAP(ctx, cfg, cmd.ErrOrStderr())
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			})
	}
}
