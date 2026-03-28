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

var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Print SunSpec address map summary",
	Long:  "Shows the SunSpec register layout in a human-friendly way (address ranges per model). Use --base to skip detection when the base address is already known.",
	Example: `
  modbusctl client sunspec map --url tcp://192.168.1.10:502 --unit 1
  modbusctl client sunspec map --ip 192.168.1.10 --unit 1 --show-header-regs --compact
  modbusctl client sunspec map --url tcp://192.168.1.10:502 --unit 1 --format json
`,
}

func init() {
	cfg := config.SunSpecMapConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:         502,
			Unit:         1,
			Regtype:      "holding",
			OutputFormat: string(format.FormatText),
		},
	}
	runner.RegisterClientCommandCfg(mapCmd, &cfg, config.RegisterRegtypeCompletion)

	mapCmd.RunE = func(cmd *cobra.Command, args []string) error {
		mergeSunspecOutputFormat(&cfg.SunSpecBaseConfig)
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckSunSpecMapConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, e := modbus.CollectSunSpecMap(ctx, cfg)
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			})
	}
}
