package sunspec

import (
	"fmt"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
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
  modbusctl client sunspec probe --ip 192.168.1.10 --unit 1 --format json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckSunSpecProbeConfig(probeCfg); err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		mergeSunspecOutputFormat(&probeCfg.SunSpecBaseConfig)
		outFmt, err := format.Parse(probeCfg.OutputFormat)
		if err != nil {
			return err
		}
		result, err := modbus.CollectSunSpecProbe(probeCfg)
		if err != nil {
			return err
		}
		return format.Write(cmd.OutOrStdout(), outFmt, result)
	},
}

func init() {
	probeCfg = config.SunSpecProbeConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:         502,
			Unit:         1,
			Regtype:      "holding",
			OutputFormat: string(format.FormatText),
		},
	}
	config.LoadFromEnv(&probeCfg)
	config.RegisterFlags(probeCmd, &probeCfg)
	if err := format.RegisterStdoutFormatFlagCompletion(probeCmd); err != nil {
		panic(err)
	}
	config.RegisterRegtypeCompletion(probeCmd)
}
