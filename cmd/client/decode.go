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

var decodeCmd = &cobra.Command{
	Use:   "decode",
	Short: "Read registers and decode them into typed values using a codec",
	Long: `Read holding (FC3) or input (FC4) registers and decode them into typed values
using a go-modbus codec. Run "modbusctl client codecs" to list available codec IDs.

The codec determines how many registers each value spans. With --count omitted a
single value is decoded; when --count is a multiple of the codec register width the
block is decoded as an array of values.`,
	Example: `
  # Decode two registers at 40001 as a big-endian float32
  modbusctl client decode --ip 192.168.1.10 --start 40001 --codec float32/layout:1234

  # Decode an 8-register ASCII string from input registers (function 4)
  modbusctl client decode --ip 192.168.1.10 --function 4 --start 0 --codec ascii/registers:8

  # Decode an array of four uint32 values (2 registers each) as JSON
  modbusctl client decode --ip 192.168.1.10 --start 100 --codec uint32/layout:4321 --count 8 --format json

  # Decode an IPv4 address stored in two registers
  modbusctl client decode --ip 192.168.1.10 --start 200 --codec ip_addr
`,
}

func init() {
	cfg := config.DecodeConfig{
		DeviceConfig: config.DeviceConfig{
			IP:   "",
			Port: 502,
			Unit: 1,
		},
		Timeout:       0,
		Function:      3,
		StartAddress:  1,
		Codec:         "",
		RegisterCount: 0,
		OutputFormat:  string(format.FormatText),
		Debug:         false,
	}
	runner.WireClientCommand(ClientCmd, decodeCmd, &cfg, config.RegisterFunctionCompletion, config.RegisterCodecCompletion)

	decodeCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckDecodeConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, err := modbus.CollectDecode(ctx, cfg, cmd.ErrOrStderr())
				if err != nil {
					return nil, modbus.WrapCollectError(err)
				}
				return r, nil
			})
	}
}
