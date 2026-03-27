package client

import (
	"fmt"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var identifyCfg config.IdentifyConfig

var identifyCmd = &cobra.Command{
	Use:   "identify",
	Short: "Send Modbus FC43/14 request to identify the Modbus TCP device",
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
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckIdentifyConfig(identifyCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}

		outFmt, err := format.Parse(identifyCfg.OutputFormat)
		if err != nil {
			return err
		}

		result, err := modbus.CollectDeviceIdentification(identifyCfg)
		if err != nil {
			return err
		}

		if err := format.Write(cmd.OutOrStdout(), outFmt, result); err != nil {
			return err
		}

		return identifyExitError(result)
	},
}

// identifyExitError preserves legacy exit status: exactly one requested unit with a Modbus-level failure exits non-zero.
func identifyExitError(result *types.IdentifyResult) error {
	if result == nil || len(result.Units) != 1 {
		return nil
	}
	if result.Units[0].Error != "" {
		return fmt.Errorf("%s", result.Units[0].Error)
	}
	return nil
}

func init() {
	ClientCmd.AddCommand(identifyCmd)
	identifyCfg = config.IdentifyConfig{
		OutputFormat: string(format.FormatText),
		UnitClientConfig: config.UnitClientConfig{
			IP:       "",
			Port:     502,
			UnitID:   "1",
			Timeout:  2000,
			Parallel: 10,
		},
	}
	config.LoadFromEnv(&identifyCfg)
	config.RegisterFlags(identifyCmd, &identifyCfg)
	if err := format.RegisterStdoutFormatFlagCompletion(identifyCmd); err != nil {
		panic(err)
	}
}
