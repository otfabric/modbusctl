package client

import (
	"fmt"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"

	"github.com/spf13/cobra"
)

var scanCfg config.ScanConfig

var scanClientCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a range of Modbus TCP registers and output raw values in real time and to file",
	Long:  `Scan algorithms: safe = conservative linear probing with descending block sizes; smart = adaptive interval splitting; deep = smart + local boundary refinement.`,
	Example: `
  # Scan holding registers 0-10 (default algo: safe)
  modbusctl client scan --ip 192.168.1.100 --start 0 --end 10 --output scan.mcap

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client scan --url tcp://192.168.1.100:502 --start 0 --end 10 --output scan.mcap

  # Scan with smart algorithm (interval splitting)
  modbusctl client scan --ip 192.168.1.100 --function 3 --algo smart --start 0 --end 1000 --output scan.mcap

  # Scan input registers with delay between requests
  modbusctl client scan --ip 192.168.1.100 --function 4 --start 100 --end 110 --delay 100 --output input_scan.mcap

  # Use environment variables
  MODBUSCTL_IP=192.168.1.100 MODBUSCTL_START=0 MODBUSCTL_END=10 MODBUSCTL_OUTPUT=scan.mcap modbusctl client scan
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		scanCfg.Debug = cli.Debug(cmd)
		if err := validate.CheckScanConfig(scanCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		outFmt, err := format.Parse(scanCfg.OutputFormat)
		if err != nil {
			return err
		}
		result, err := modbus.ScanAndWriteMCAP(scanCfg)
		if err != nil {
			return err
		}
		return format.Write(cmd.OutOrStdout(), outFmt, result)
	},
}

func init() {
	ClientCmd.AddCommand(scanClientCmd)
	scanCfg = config.ScanConfig{
		DeviceConfig: config.DeviceConfig{
			IP:   "",
			Port: 502,
			Unit: 1,
		},
		Function:                3,
		Delay:                   0,
		StartAddress:            1,
		EndAddress:              65535,
		OutputFile:              "",
		OutputFormat:            string(format.FormatText),
		Algo:                    "safe",
		Step:                    1000,
		StepHalfOffset:          false,
		SeedStart:               0,
		SeedCount:               0,
		RetryOnTimeoutTransport: 0,
		SunSpecBase:             0,
		SunSpecBases:            "",
		SunSpecMaxModels:        0,
		SunSpecMaxSpan:          0,
		Debug:                   false,
	}
	config.LoadFromEnv(&scanCfg)
	config.RegisterFlags(scanClientCmd, &scanCfg)
	if err := format.RegisterStdoutFormatFlagCompletion(scanClientCmd); err != nil {
		panic(err)
	}
	config.RegisterScanAlgoCompletion(scanClientCmd)
	config.RegisterFunctionCompletion(scanClientCmd)
}
