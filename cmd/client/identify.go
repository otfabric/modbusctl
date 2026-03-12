package client

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
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
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckIdentifyConfig(identifyCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.DeviceIdentification(identifyCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	ClientCmd.AddCommand(identifyCmd)
	identifyCfg = config.IdentifyConfig{
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
}
