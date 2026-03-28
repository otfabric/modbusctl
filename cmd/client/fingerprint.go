package client

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

var fingerprintCmd = &cobra.Command{
	Use:   "fingerprint",
	Short: "Probe supported read functions (FC08/FC43/FC03/FC04/FC01/FC02/FC11/FC18/FC20) per unit via SupportsFunction",
	Example: `
  # Fingerprint unit 1 (default)
  modbusctl client fingerprint --ip 192.168.1.10

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client fingerprint --url tcp://192.168.1.10:502

  # Fingerprint with delay between probes
  modbusctl client fingerprint --ip 192.168.1.10 --unit 1 --interval 100

  # Fingerprint a range or all units
  modbusctl client fingerprint --ip 192.168.1.10 --unit 1-10
  modbusctl client fingerprint --ip 192.168.1.10 --unit all
`,
}

func init() {
	cfg := config.FingerprintConfig{
		OutputFormat: string(format.FormatText),
		IP:           "",
		Port:         502,
		UnitID:       "1",
		Timeout:      0,
		Interval:     0,
	}
	runner.WireClientCommand(ClientCmd, fingerprintCmd, &cfg)

	fingerprintCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error { return validate.CheckFingerprintConfig(cfg) },
			func(ctx context.Context) (any, error) {
				r, e := modbus.CollectFingerprint(ctx, cfg)
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			},
			runner.WithSuccessExit(types.SuccessExitForPayload))
	}
}
