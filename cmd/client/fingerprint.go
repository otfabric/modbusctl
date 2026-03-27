package client

import (
	"fmt"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var fingerprintCfg config.FingerprintConfig

var fingerprintCmd = &cobra.Command{
	Use:   "fingerprint",
	Short: "Probe supported read functions (FC08/FC43/FC03/FC04/FC01/FC02/FC11/FC18/FC20) per unit via HasUnitReadFunction",
	Example: `
  # Fingerprint unit 1 (default)
  modbusctl client fingerprint --ip 192.168.1.10

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client fingerprint --url tcp://192.168.1.10:502

  # Fingerprint with delay between probes
  modbusctl client fingerprint --ip 192.168.1.10 --unit 1 --interval 100

  # Fingerprint a range or all units
  modbusctl client fingerprint --ip 192.168.1.10 --unit 1-10
  modbusctl client fingerprint --ip 192.168.1.10 --unit all
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckFingerprintConfig(fingerprintCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		outFmt, err := format.Parse(fingerprintCfg.OutputFormat)
		if err != nil {
			return err
		}
		result, err := modbus.CollectFingerprint(fingerprintCfg)
		if err != nil {
			return err
		}
		return format.Write(cmd.OutOrStdout(), outFmt, result)
	},
}

func init() {
	ClientCmd.AddCommand(fingerprintCmd)
	fingerprintCfg = config.FingerprintConfig{
		OutputFormat: string(format.FormatText),
		IP:           "",
		Port:         502,
		UnitID:       "1",
		Timeout:      2000,
		Interval:     0,
	}
	config.LoadFromEnv(&fingerprintCfg)
	config.RegisterFlags(fingerprintCmd, &fingerprintCfg)
	if err := format.RegisterStdoutFormatFlagCompletion(fingerprintCmd); err != nil {
		panic(err)
	}
}
