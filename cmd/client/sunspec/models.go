package sunspec

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var modelsCfg config.SunSpecModelsConfig

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Enumerate SunSpec model headers",
	Long:  "Lists the SunSpec model chain (ID and length per model). Use --base to skip detection when the base address is already known.",
	Example: `
  modbusctl client sunspec models --url tcp://192.168.1.10:502 --unit 1
  modbusctl client sunspec models --ip 192.168.1.10 --unit 1 --base 40000 --max-models 64
  modbusctl client sunspec models --url tcp://192.168.1.10:502 --unit 1 --json
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckSunSpecModelsConfig(modelsCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}
		if err := modbus.SunSpecModels(modelsCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	modelsCfg = config.SunSpecModelsConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:    502,
			Unit:    1,
			Regtype: "holding",
		},
	}
	config.LoadFromEnv(&modelsCfg)
	config.RegisterFlags(modelsCmd, &modelsCfg)
	config.RegisterRegtypeCompletion(modelsCmd)
}
