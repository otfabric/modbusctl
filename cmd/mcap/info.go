package mcap

import (
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	mcapfile "github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Extract header and record info from MCAP file",
	Example: `
  # Get information about an MCAP file and print to stdout
  modbusctl mcap info --input data.mcap

  # Get information about an MCAP file and save to output file
  modbusctl mcap info --input data.mcap --output info.txt

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=data.mcap MODBUSCTL_OUTPUT=info.txt modbusctl mcap info
`,
}

func init() {
	cfg := config.InfoConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(infoCmd, &cfg)
	cli.MustMarkFlagRequired(infoCmd, "input")
	McapCmd.AddCommand(infoCmd)

	infoCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckInfoConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := mcapfile.ExportInfo(w, cfg.InputFile); err != nil {
			return errs.Output(errs.CodeMCAPLoadFailed, err)
		}
		return nil
	}
}
