// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var writeCmd = &cobra.Command{
	Use:   "write",
	Short: "Write coils or holding registers on a Modbus device",
	Long: `Write coils (FC5, FC15) or holding registers (FC6, FC16).

With --function omitted, one register value is written with FC6 and several with FC16.
Coils need an explicit --function 5 (one coil) or --function 15 (several).

Register values are integers: 0-65535, 0x-prefixed hex, or -32768..-1 (two's complement).
With --codec each value is encoded into as many registers as the codec needs, so floats,
32/64-bit integers, strings, IP addresses and timestamps can be written directly. Run
"modbusctl client codecs" to list the codec IDs.

The command refuses to run without --yes, and has sent nothing then. All values are
validated before anything is sent, and the request is sent exactly once:
a failed write is never retried, because a retry after a lost response could apply it twice.

Writing changes the state of the device. Only write to equipment you are authorized to
operate, and make sure you know what the target coil or register controls.`,
	Example: `
  # Write 1234 to holding register 100 (FC6)
  modbusctl client write --ip 192.168.1.10 --start 100 --values 1234 --yes

  # Write three holding registers starting at 100 (FC16)
  modbusctl client write --ip 192.168.1.10 --start 100 --values 10,0x00FF,-1 --yes

  # Switch coil 8 on (FC5)
  modbusctl client write --ip 192.168.1.10 --function 5 --start 8 --values on --yes

  # Write four coils starting at 8 (FC15)
  modbusctl client write --ip 192.168.1.10 --function 15 --start 8 --values 1,0,1,1 --yes

  # Write a big-endian float32 setpoint (two registers, FC16)
  modbusctl client write --ip 192.168.1.10 --start 200 --codec float32/layout:4321 --values 21.5 --yes

  # Write over Modbus RTU on a serial line
  modbusctl client write --url rtu:///dev/ttyUSB0 --baud 9600 --parity even --unit 3 --start 100 --values 1234 --yes
`,
}

func init() {
	cfg := config.WriteConfig{
		DeviceConfig: config.DeviceConfig{
			Port: 502,
			Unit: 1,
		},
		OutputFormat: string(format.FormatText),
	}
	runner.WireClientCommand(ClientCmd, writeCmd, &cfg, config.RegisterWriteFunctionCompletion, config.RegisterCodecCompletion)

	writeCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error {
				if err := validate.CheckConfirmed(cfg.Yes, "client write"); err != nil {
					return err
				}
				// No default target address for a write: it must be given explicitly.
				if !cmd.Flags().Changed("start") && os.Getenv("MODBUSCTL_ADDRESS") == "" {
					return errors.New("the address to write is required (use --start)")
				}
				return validate.CheckWriteConfig(cfg)
			},
			func(ctx context.Context) (any, error) {
				r, err := modbus.CollectWrite(ctx, cfg)
				if err != nil {
					return nil, modbus.WrapCollectError(err)
				}
				return r, nil
			})
	}
}
