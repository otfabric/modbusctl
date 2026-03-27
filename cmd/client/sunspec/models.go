package sunspec

import (
	"fmt"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
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
  modbusctl client sunspec models --url tcp://192.168.1.10:502 --unit 1 --format json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckSunSpecModelsConfig(modelsCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		mergeSunspecOutputFormat(&modelsCfg.SunSpecBaseConfig)
		outFmt, err := format.Parse(modelsCfg.OutputFormat)
		if err != nil {
			return err
		}
		result, err := modbus.CollectSunSpecModels(modelsCfg)
		if err != nil {
			return err
		}
		return format.Write(cmd.OutOrStdout(), outFmt, result)
	},
}

func init() {
	modelsCfg = config.SunSpecModelsConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:         502,
			Unit:         1,
			Regtype:      "holding",
			OutputFormat: string(format.FormatText),
		},
	}
	config.LoadFromEnv(&modelsCfg)
	config.RegisterFlags(modelsCmd, &modelsCfg)
	if err := format.RegisterStdoutFormatFlagCompletion(modelsCmd); err != nil {
		panic(err)
	}
	config.RegisterRegtypeCompletion(modelsCmd)
}
