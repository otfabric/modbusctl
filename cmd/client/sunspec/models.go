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

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Enumerate SunSpec model headers",
	Long:  "Lists the SunSpec model chain (ID and length per model). Use --base to skip detection when the base address is already known.",
	Example: `
  modbusctl client sunspec models --url tcp://192.168.1.10:502 --unit 1
  modbusctl client sunspec models --ip 192.168.1.10 --unit 1 --base 40000 --max-models 64
  modbusctl client sunspec models --url tcp://192.168.1.10:502 --unit 1 --format json
`,
}

func init() {
	cfg := config.SunSpecModelsConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:         502,
			Unit:         1,
			Regtype:      "holding",
			OutputFormat: string(format.FormatText),
		},
	}
	runner.RegisterClientCommandCfg(modelsCmd, &cfg, config.RegisterRegtypeCompletion)

	modelsCmd.RunE = func(cmd *cobra.Command, args []string) error {
		mergeSunspecOutputFormat(&cfg.SunSpecBaseConfig)
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckSunSpecModelsConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, e := modbus.CollectSunSpecModels(ctx, cfg)
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			})
	}
}
