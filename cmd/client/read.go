package client

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var readCfg config.ReadConfig

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

  # Use environment variables instead of CLI arguments
  MODBUSCTL_IP=192.168.1.10 MODBUSCTL_START=40001 MODBUSCTL_COUNT=5 modbusctl client read
`,
	Run: func(cmd *cobra.Command, args []string) {
		readCfg.Debug = cli.Debug(cmd)
		if err := validate.CheckReadConfig(readCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.ReadAndWriteMCAP(readCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	ClientCmd.AddCommand(readCmd)
	readCfg = config.ReadConfig{
		DeviceConfig: config.DeviceConfig{
			IP:   "",
			Port: 502,
			Unit: 1,
		},
		Function:      3,
		StartAddress:  1,
		RegisterCount: 1,
		Ascii:         false,
		SwapBytes:     false,
		OutputFile:    "",
		Debug:         false,
	}
	config.LoadFromEnv(&readCfg)
	config.RegisterFlags(readCmd, &readCfg)
	config.RegisterFunctionCompletion(readCmd)
	if readCfg.IP == "" {
		if err := readCmd.MarkFlagRequired("ip"); err != nil {
			panic(err)
		}
	}
}
