package sunspec

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var detectCfg config.SunSpecDetectConfig

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect SunSpec marker and base address",
	Long:  "Probes candidate base addresses for the SunSpec \"SunS\" marker and reports whether the unit is SunSpec and at which base address.",
	Example: `
  modbusctl client sunspec detect --url tcp://192.168.1.10:502 --unit 1
  modbusctl client sunspec detect --ip 192.168.1.10 --unit 1 --regtype holding --bases 0,40000,50000
  modbusctl client sunspec detect --url tcp://192.168.1.10:502 --unit 1 --verbose --json
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckSunSpecDetectConfig(detectCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}
		if err := modbus.SunSpecDetect(detectCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	detectCfg = config.SunSpecDetectConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:    502,
			Unit:    1,
			Regtype: "holding",
		},
	}
	config.LoadFromEnv(&detectCfg)
	config.RegisterFlags(detectCmd, &detectCfg)
	config.RegisterRegtypeCompletion(detectCmd)
}
