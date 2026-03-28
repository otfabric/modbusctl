package mcap

import (
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	mcapfile "github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract address blocks from MCAP file to JSON",
	Example: `
  # Extract address blocks from an MCAP file and output to stdout
  modbusctl mcap extract --input input.mcap --output register-ranges.json

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=input.mcap MODBUSCTL_OUTPUT=register-ranges.json modbusctl mcap extract
`,
}

func init() {
	cfg := config.ExtractConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(extractCmd, &cfg)
	cli.MustMarkFlagRequired(extractCmd, "input")
	McapCmd.AddCommand(extractCmd)

	extractCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckExtractConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := mcapfile.ExportAddressBlocks(w, cfg.InputFile); err != nil {
			return errs.Output(errs.CodeMCAPLoadFailed, err)
		}
		return nil
	}
}
