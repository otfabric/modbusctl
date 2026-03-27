package client

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
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

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client record --url tcp://192.168.1.100:502 --input registers.yaml --output data.mcap --interval 5 --duration 60

  # Record input registers with function code 4, polling every 10 seconds for 2 minutes
  modbusctl client record --ip 192.168.1.100 --function 4 --input register-ranges.json --output data.mcap --interval 10 --duration 120

  # Use environment variables instead of CLI arguments
  MODBUSCTL_IP=192.168.1.100 MODBUSCTL_INPUT=registers.yaml MODBUSCTL_OUTPUT=data.mcap MODBUSCTL_INTERVAL=5 MODBUSCTL_DURATION=60 modbusctl client record
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		recordCfg.Debug = cli.Debug(cmd)
		if err := validate.CheckRecordConfig(recordCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		fmt.Fprintf(os.Stderr, "⚙️ Running record mode\n")
		outFmt, err := format.Parse(recordCfg.OutputFormat)
		if err != nil {
			return err
		}
		result, err := modbus.RecordAndWriteMCAP(recordCfg)
		if err != nil {
			return err
		}
		return format.Write(cmd.OutOrStdout(), outFmt, result)
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
		Function:     3,
		InputFile:    "",
		Interval:     5000,  // Default to 5 seconds
		Duration:     60000, // Default to 1 minute
		OutputFile:   "",
		OutputFormat: string(format.FormatText),
		Debug:        false,
	}
	config.LoadFromEnv(&recordCfg)
	config.RegisterFlags(recordCmd, &recordCfg)
	if err := format.RegisterStdoutFormatFlagCompletion(recordCmd); err != nil {
		panic(err)
	}
	config.RegisterFunctionCompletion(recordCmd)
	if recordCfg.InputFile == "" {
		if err := recordCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
}
