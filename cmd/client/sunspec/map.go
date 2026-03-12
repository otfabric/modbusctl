package sunspec

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var mapCfg config.SunSpecMapConfig

var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Print SunSpec address map summary",
	Long:  "Shows the SunSpec register layout in a human-friendly way (address ranges per model). Use --base to skip detection when the base address is already known.",
	Example: `
  modbusctl client sunspec map --url tcp://192.168.1.10:502 --unit 1
  modbusctl client sunspec map --ip 192.168.1.10 --unit 1 --show-header-regs --compact
  modbusctl client sunspec map --url tcp://192.168.1.10:502 --unit 1 --json
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckSunSpecMapConfig(mapCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}
		if err := modbus.SunSpecMap(mapCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	mapCfg = config.SunSpecMapConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:    502,
			Unit:    1,
			Regtype: "holding",
		},
	}
	config.LoadFromEnv(&mapCfg)
	config.RegisterFlags(mapCmd, &mapCfg)
	config.RegisterRegtypeCompletion(mapCmd)
}
