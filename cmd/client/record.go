// SPDX-License-Identifier: MIT

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
	Short: "Record the read requests of a plan from a Modbus device to a capture file",
	Example: `
  # Record the holding-register blocks of blocks.json every 5 seconds for 1 minute
  modbusctl client record --ip 192.168.1.100 --blocks-file blocks.json --output data.mbcap --interval 5000 --duration 60000

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client record --url tcp://192.168.1.100:502 --blocks-file blocks.json --output data.mbcap --interval 5000 --duration 60000

  # Modbus/TCP Security: TLS with a client certificate (see --tls-cert, --tls-key, --tls-ca)
  modbusctl client record --url tcp+tls://192.168.1.100:802 --tls-cert client.pem --tls-key client-key.pem --tls-ca ca.pem --blocks-file blocks.json --output data.mbcap --interval 5000 --duration 60000

  # Record input-register blocks (function code 4) every 10 seconds for 2 minutes
  modbusctl client record --ip 192.168.1.100 --function 4 --blocks-file register-ranges.json --output data.mbcap --interval 10000 --duration 120000

  # A plan with other read functions: diagnostics counters, server ID, FIFO queue, device identification
  #   [{"start_address": 100, "register_count": 10}, {"function": 8, "sub_function": 11},
  #    {"function": 17}, {"function": 24, "fifo_address": 1234}, {"function": 43}]
  modbusctl client record --ip 192.168.1.100 --blocks-file plan.json --output data.mbcap --interval 1000 --duration 60000

  # Use environment variables instead of CLI arguments
  MODBUSCTL_IP=192.168.1.100 MODBUSCTL_BLOCKS_FILE=blocks.json MODBUSCTL_OUTPUT=data.mbcap MODBUSCTL_INTERVAL=5000 MODBUSCTL_DURATION=60000 modbusctl client record
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
				r, e := modbus.RecordToCapture(ctx, cfg, cmd.ErrOrStderr())
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			})
	}
}
