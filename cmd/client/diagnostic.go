// SPDX-License-Identifier: MIT

package client

import (
	"context"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var diagnosticCmd = &cobra.Command{
	Use:   "diagnostic",
	Short: "Send Modbus FC08 Diagnostics request to the device",
	Long: `Send an FC08 Diagnostics request.

The sub-functions that change the state of the device need --yes, and nothing is sent
without it: restartcommunications, changeasciiinputdelimiter, forcelistenonlymode,
clearcountersanddiagnosticreg and clearoverruncounterandflag. The others only read.`,
	Example: `
  # Loopback test (returnquerydata, default)
  modbusctl client diagnostic --ip 192.168.1.10

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client diagnostic --url tcp://192.168.1.10:502

  # Loopback test with custom data
  modbusctl client diagnostic --ip 192.168.1.10 --data A537

  # Restart Communications
  modbusctl client diagnostic --ip 192.168.1.10 --sub-function restartcommunications --yes

  # Return Bus Message Count
  modbusctl client diagnostic --ip 192.168.1.10 --sub-function returnbusmessagecount

  # Clear Counters and Diagnostic Register
  modbusctl client diagnostic --ip 192.168.1.10 --sub-function clearcountersanddiagnosticreg --yes

  # Specific unit ID
  modbusctl client diagnostic --ip 192.168.1.10 --unit 5 --sub-function returnquerydata
`,
}

func init() {
	cfg := config.DiagnosticConfig{
		OutputFormat: string(format.FormatText),
		IP:           "",
		Port:         502,
		UnitID:       1,
		Timeout:      0,
		SubFunction:  "returnquerydata",
		Data:         "",
	}
	runner.WireClientCommand(ClientCmd, diagnosticCmd, &cfg, config.RegisterDiagnosticSubFunctionCompletion)

	diagnosticCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runner.RunClientFormattedWithDebug(cmd, func(d bool) { cfg.Debug = d }, cfg.OutputFormat,
			func() error {
				if err := validate.CheckDiagnosticConfirmed(cfg); err != nil {
					return err
				}
				return validate.CheckDiagnosticConfig(cfg)
			},
			func(ctx context.Context) (any, error) {
				r, e := modbus.CollectDiagnostics(ctx, cfg)
				if e != nil {
					return nil, modbus.WrapCollectError(e)
				}
				return r, nil
			})
	}
}
