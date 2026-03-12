package client

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
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

  # Fingerprint with delay between probes
  modbusctl client fingerprint --ip 192.168.1.10 --unit 1 --interval 100

  # Fingerprint a range or all units
  modbusctl client fingerprint --ip 192.168.1.10 --unit 1-10
  modbusctl client fingerprint --ip 192.168.1.10 --unit all
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckFingerprintConfig(fingerprintCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.FingerprintDeviceProbe(fingerprintCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	ClientCmd.AddCommand(fingerprintCmd)
	fingerprintCfg = config.FingerprintConfig{
		IP:       "",
		Port:     502,
		UnitID:   "1",
		Timeout:  2000,
		Interval: 0,
	}
	config.LoadFromEnv(&fingerprintCfg)
	config.RegisterFlags(fingerprintCmd, &fingerprintCfg)
	if fingerprintCfg.IP == "" {
		if err := fingerprintCmd.MarkFlagRequired("ip"); err != nil {
			panic(err)
		}
	}
}
