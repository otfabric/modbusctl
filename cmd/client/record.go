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

var recordCfg config.RecordConfig

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Continuously read and record registers from a Modbus TCP device",
	Example: `
  # Record holding registers from a Modbus device at 192.168.1.100 every 5 seconds for 1 minute
  modbusctl client record --ip 192.168.1.100 --input registers.yaml --output data.mcap --interval 5 --duration 60

  # Record input registers with function code 4, polling every 10 seconds for 2 minutes
  modbusctl client record --ip 192.168.1.100 --function 4 --input register-ranges.json --output data.mcap --interval 10 --duration 120

  # Use environment variables instead of CLI arguments
  MODBUSCTL_IP=192.168.1.100 MODBUSCTL_INPUT=registers.yaml MODBUSCTL_OUTPUT=data.mcap MODBUSCTL_INTERVAL=5 MODBUSCTL_DURATION=60 modbusctl client record
`,
	Run: func(cmd *cobra.Command, args []string) {
		recordCfg.Debug = cli.Debug(cmd)
		if err := validate.CheckRecordConfig(recordCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("⚙️ Running record mode")
		if err := modbus.RecordAndWriteMCAP(recordCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	ClientCmd.AddCommand(recordCmd)
	recordCfg = config.RecordConfig{
		DeviceConfig: config.DeviceConfig{
			IP:   "",
			Port: 502,
			Unit: 1,
		},
		Function:   3,
		InputFile:  "",
		Interval:   5000,  // Default to 5 seconds
		Duration:   60000, // Default to 1 minute
		OutputFile: "",
		Debug:      false,
	}
	config.LoadFromEnv(&recordCfg)
	config.RegisterFlags(recordCmd, &recordCfg)
	config.RegisterFunctionCompletion(recordCmd)
	if recordCfg.IP == "" {
		if err := recordCmd.MarkFlagRequired("ip"); err != nil {
			panic(err)
		}
	}
	if recordCfg.InputFile == "" {
		if err := recordCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
}
