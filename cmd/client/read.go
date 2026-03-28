package client

import (
	"context"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read registers from a Modbus TCP device",
	Example: `
  # Read a single holding register at address 40001
  modbusctl client read --ip 192.168.1.10 --start 40001

  # Read 10 holding registers from a specific Modbus unit
  modbusctl client read --ip 192.168.1.10 --unit 2 --start 30001 --count 10

  # Read 10 input registers with function code 4
  modbusctl client read --ip 192.168.1.10 --function 4 --start 40001 --count 10

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client read --url tcp://192.168.1.10:502 --start 40001

  # Use environment variables instead of CLI arguments
  MODBUSCTL_IP=192.168.1.10 MODBUSCTL_START=40001 MODBUSCTL_COUNT=5 modbusctl client read
`,
}

func init() {
	cfg := config.ReadConfig{
		DeviceConfig: config.DeviceConfig{
			IP:   "",
			Port: 502,
			Unit: 1,
		},
		Timeout:       0,
		Function:      3,
		StartAddress:  1,
		RegisterCount: 1,
		Ascii:         false,
		SwapBytes:     false,
		OutputFile:    "",
		OutputFormat:  string(format.FormatText),
		Debug:         false,
	}
	runner.WireClientCommand(ClientCmd, readCmd, &cfg, config.RegisterFunctionCompletion)

	readCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckReadConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, err := modbus.CollectRead(ctx, cfg, cmd.ErrOrStderr())
				if err != nil {
					return nil, modbus.WrapCollectError(err)
				}
				return r, nil
			})
	}
}
