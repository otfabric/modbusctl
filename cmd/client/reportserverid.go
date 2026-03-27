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

var reportServerIdCfg config.ReportServerIdConfig

var reportServerIdCmd = &cobra.Command{
	Use:   "reportserverid",
	Short: "Send Modbus FC17 Report Server ID request to the device",
	Example: `
  # Query server ID from unit 1 (default)
  modbusctl client reportserverid --ip 192.168.1.10

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client reportserverid --url tcp://192.168.1.10:502

  # Query a specific unit
  modbusctl client reportserverid --ip 192.168.1.10 --unit 5

  # Query a range of units
  modbusctl client reportserverid --ip 192.168.1.10 --unit 1-10

  # Query all units in parallel
  modbusctl client reportserverid --ip 192.168.1.10 --unit all --parallel 10
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckReportServerIdConfig(reportServerIdCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		outFmt, err := format.Parse(reportServerIdCfg.OutputFormat)
		if err != nil {
			return err
		}
		result, err := modbus.CollectReportServerID(reportServerIdCfg)
		if err != nil {
			return err
		}
		if err := format.Write(cmd.OutOrStdout(), outFmt, result); err != nil {
			return err
		}
		return reportServerIDExitError(result)
	},
}

func reportServerIDExitError(result *types.ReportServerIDResult) error {
	if result == nil || len(result.Units) != 1 {
		return nil
	}
	if result.Units[0].Error != "" {
		return fmt.Errorf("%s", result.Units[0].Error)
	}
	return nil
}

func init() {
	ClientCmd.AddCommand(reportServerIdCmd)
	reportServerIdCfg = config.ReportServerIdConfig{
		OutputFormat: string(format.FormatText),
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
	if err := format.RegisterStdoutFormatFlagCompletion(reportServerIdCmd); err != nil {
		panic(err)
	}
}
