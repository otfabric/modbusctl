package sunspec

import (
	"fmt"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
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
  modbusctl client sunspec map --url tcp://192.168.1.10:502 --unit 1 --format json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckSunSpecMapConfig(mapCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		mergeSunspecOutputFormat(&mapCfg.SunSpecBaseConfig)
		outFmt, err := format.Parse(mapCfg.OutputFormat)
		if err != nil {
			return err
		}
		result, err := modbus.CollectSunSpecMap(mapCfg)
		if err != nil {
			return err
		}
		return format.Write(cmd.OutOrStdout(), outFmt, result)
	},
}

func init() {
	mapCfg = config.SunSpecMapConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:         502,
			Unit:         1,
			Regtype:      "holding",
			OutputFormat: string(format.FormatText),
		},
	}
	config.LoadFromEnv(&mapCfg)
	config.RegisterFlags(mapCmd, &mapCfg)
	if err := format.RegisterStdoutFormatFlagCompletion(mapCmd); err != nil {
		panic(err)
	}
	config.RegisterRegtypeCompletion(mapCmd)
}
