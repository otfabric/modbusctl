package sunspec

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var probeCfg config.SunSpecProbeConfig

var probeCmd = &cobra.Command{
	Use:   "probe",
	Short: "Combined fingerprint and SunSpec detection summary",
	Long:  "One-shot summary: supported Modbus read functions (FC03, FC04, FC43) and SunSpec detection result (base, model count, end model). Complements fingerprint and identify.",
	Example: `
  modbusctl client sunspec probe --url tcp://192.168.1.10:502 --unit 1
  modbusctl client sunspec probe --ip 192.168.1.10 --unit 1 --json
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckSunSpecProbeConfig(probeCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}
		if err := modbus.SunSpecProbe(probeCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	probeCfg = config.SunSpecProbeConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:    502,
			Unit:    1,
			Regtype: "holding",
		},
	}
	config.LoadFromEnv(&probeCfg)
	config.RegisterFlags(probeCmd, &probeCfg)
	config.RegisterRegtypeCompletion(probeCmd)
}
