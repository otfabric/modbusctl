package sunspec

import (
	"context"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var probeCmd = &cobra.Command{
	Use:   "probe",
	Short: "Combined fingerprint and SunSpec detection summary",
	Long:  "One-shot summary: supported Modbus read functions (FC03, FC04, FC43) and SunSpec detection result (base, model count, end model). Complements fingerprint and identify.",
	Example: `
  modbusctl client sunspec probe --url tcp://192.168.1.10:502 --unit 1
  modbusctl client sunspec probe --ip 192.168.1.10 --unit 1 --format json
`,
}

func init() {
	cfg := config.SunSpecProbeConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			Port:         502,
			Unit:         1,
			Regtype:      "holding",
			OutputFormat: string(format.FormatText),
		},
	}
	runner.RegisterClientCommandCfg(probeCmd, &cfg, config.RegisterRegtypeCompletion)

	probeCmd.RunE = func(cmd *cobra.Command, args []string) error {
		mergeSunspecOutputFormat(&cfg.SunSpecBaseConfig)
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckSunSpecProbeConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, e := modbus.CollectSunSpecProbe(ctx, cfg)
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			},
			runner.WithSuccessExit(types.SuccessExitForPayload))
	}
}
