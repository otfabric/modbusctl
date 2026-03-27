package sunspec

import (
	"fmt"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
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
  modbusctl client sunspec detect --url tcp://192.168.1.10:502 --unit 1 --verbose --format json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckSunSpecDetectConfig(detectCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		mergeSunspecOutputFormat(&detectCfg.SunSpecBaseConfig)
		outFmt, err := format.Parse(detectCfg.OutputFormat)
		if err != nil {
			return err
		}
		result, err := modbus.CollectSunSpecDetect(detectCfg)
		if err != nil {
			return err
		}
		return format.Write(cmd.OutOrStdout(), outFmt, result)
	},
}

func init() {
	detectCfg = config.SunSpecDetectConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:         502,
			Unit:         1,
			Regtype:      "holding",
			OutputFormat: string(format.FormatText),
		},
	}
	config.LoadFromEnv(&detectCfg)
	config.RegisterFlags(detectCmd, &detectCfg)
	if err := format.RegisterStdoutFormatFlagCompletion(detectCmd); err != nil {
		panic(err)
	}
	config.RegisterRegtypeCompletion(detectCmd)
}
