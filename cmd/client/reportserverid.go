package client

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var reportServerIdCfg config.ReportServerIdConfig

var reportServerIdCmd = &cobra.Command{
	Use:   "reportserverid",
	Short: "Send Modbus FC17 Report Server ID request to the device",
	Example: `
  # Query server ID from unit 1 (default)
  modbusctl client reportserverid --ip 192.168.1.10

  # Query a specific unit
  modbusctl client reportserverid --ip 192.168.1.10 --unit 5

  # Query a range of units
  modbusctl client reportserverid --ip 192.168.1.10 --unit 1-10

  # Query all units in parallel
  modbusctl client reportserverid --ip 192.168.1.10 --unit all --parallel 10
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckReportServerIdConfig(reportServerIdCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.RunReportServerId(reportServerIdCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	ClientCmd.AddCommand(reportServerIdCmd)
	reportServerIdCfg = config.ReportServerIdConfig{
		UnitClientConfig: config.UnitClientConfig{
			IP:       "",
			Port:     502,
			UnitID:   "1",
			Timeout:  2000,
			Parallel: 10,
		},
	}
	config.LoadFromEnv(&reportServerIdCfg)
	config.RegisterFlags(reportServerIdCmd, &reportServerIdCfg)
	if reportServerIdCfg.IP == "" {
		if err := reportServerIdCmd.MarkFlagRequired("ip"); err != nil {
			panic(err)
		}
	}
}
