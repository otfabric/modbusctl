package sunspec

import (
	"context"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect SunSpec marker and base address",
	Long:  "Probes candidate base addresses for the SunSpec \"SunS\" marker and reports whether the unit is SunSpec and at which base address.",
	Example: `
  modbusctl client sunspec detect --url tcp://192.168.1.10:502 --unit 1
  modbusctl client sunspec detect --ip 192.168.1.10 --unit 1 --regtype holding --bases 0,40000,50000
  modbusctl client sunspec detect --url tcp://192.168.1.10:502 --unit 1 --verbose --format json
`,
}

func init() {
	cfg := config.SunSpecDetectConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:         502,
			Unit:         1,
			Regtype:      "holding",
			OutputFormat: string(format.FormatText),
		},
	}
	runner.RegisterClientCommandCfg(detectCmd, &cfg, config.RegisterRegtypeCompletion)

	detectCmd.RunE = func(cmd *cobra.Command, args []string) error {
		mergeSunspecOutputFormat(&cfg.SunSpecBaseConfig)
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckSunSpecDetectConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, e := modbus.CollectSunSpecDetect(ctx, cfg)
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			})
	}
}
