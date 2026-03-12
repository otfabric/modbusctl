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

var scanCfg config.ScanConfig

var scanClientCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a range of Modbus TCP registers and output raw values in real time and to file",
	Long:  `Scan algorithms: safe = conservative linear probing with descending block sizes; smart = adaptive interval splitting; deep = smart + local boundary refinement.`,
	Example: `
  # Scan holding registers 0-10 (default algo: safe)
  modbusctl client scan --ip 192.168.1.100 --start 0 --end 10 --output scan.mcap

  # Scan with smart algorithm (interval splitting)
  modbusctl client scan --ip 192.168.1.100 --function 3 --algo smart --start 0 --end 1000 --output scan.mcap

  # Scan input registers with delay between requests
  modbusctl client scan --ip 192.168.1.100 --function 4 --start 100 --end 110 --delay 100 --output input_scan.mcap

  # Use environment variables
  MODBUSCTL_IP=192.168.1.100 MODBUSCTL_START=0 MODBUSCTL_END=10 MODBUSCTL_OUTPUT=scan.mcap modbusctl client scan
`,
	Run: func(cmd *cobra.Command, args []string) {
		scanCfg.Debug = cli.Debug(cmd)
		if err := validate.CheckScanConfig(scanCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.ScanAndWriteMCAP(scanCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
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
		Function:     3,
		Delay:        0,
		StartAddress: 1,
		EndAddress:   65535,
		OutputFile:   "",
		Algo:            "safe",
		Step:            1000,
		StepHalfOffset:  false,
		SeedStart:       0,
		SeedCount:               0,
		RetryOnTimeoutTransport: 0,
		Debug:                   false,
	}
	config.LoadFromEnv(&scanCfg)
	config.RegisterFlags(scanClientCmd, &scanCfg)
	config.RegisterScanAlgoCompletion(scanClientCmd)
	config.RegisterFunctionCompletion(scanClientCmd)
	if scanCfg.IP == "" {
		if err := scanClientCmd.MarkFlagRequired("ip"); err != nil {
			panic(err)
		}
	}
}
