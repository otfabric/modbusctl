package mcap

import (
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	mcapfile "github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Check a capture file: block checksums, structure and the rules of the format",
	Example: `
  # Verify a capture
  modbusctl mcap verify --input data.mbcap
`,
}

func init() {
	cfg := config.VerifyConfig{}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(verifyCmd, &cfg)
	cli.MustMarkFlagRequired(verifyCmd, "input")
	McapCmd.AddCommand(verifyCmd)

	verifyCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckVerifyConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		if err := mcapfile.Verify(cmd.OutOrStdout(), cfg.InputFile); err != nil {
			return capfile.WrapError(err)
		}
		return nil
	}
}
