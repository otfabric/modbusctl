package client

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var diagnosticCfg config.DiagnosticConfig

var diagnosticCmd = &cobra.Command{
	Use:   "diagnostic",
	Short: "Send Modbus FC08 Diagnostics request to the device",
	Example: `
  # Loopback test (returnquerydata, default)
  modbusctl client diagnostic --ip 192.168.1.10

  # Connect via URL (mutually exclusive with --ip/--port)
  modbusctl client diagnostic --url tcp://192.168.1.10:502

  # Loopback test with custom data
  modbusctl client diagnostic --ip 192.168.1.10 --data A537

  # Restart Communications
  modbusctl client diagnostic --ip 192.168.1.10 --sub-function restartcommunications

  # Return Bus Message Count
  modbusctl client diagnostic --ip 192.168.1.10 --sub-function returnbusmessagecount

  # Clear Counters and Diagnostic Register
  modbusctl client diagnostic --ip 192.168.1.10 --sub-function clearcountersanddiagnosticreg

  # Specific unit ID
  modbusctl client diagnostic --ip 192.168.1.10 --unit 5 --sub-function returnquerydata
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckDiagnosticConfig(diagnosticCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.RunDiagnostics(diagnosticCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	ClientCmd.AddCommand(diagnosticCmd)
	diagnosticCfg = config.DiagnosticConfig{
		IP:          "",
		Port:        502,
		UnitID:      1,
		Timeout:     2000,
		SubFunction: "returnquerydata",
		Data:        "",
	}
	config.LoadFromEnv(&diagnosticCfg)
	config.RegisterFlags(diagnosticCmd, &diagnosticCfg)
	config.RegisterDiagnosticSubFunctionCompletion(diagnosticCmd)
}
