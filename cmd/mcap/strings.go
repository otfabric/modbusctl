package mcap

import (
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var stringsCmd = &cobra.Command{
	Use:   "strings",
	Short: "Extract ASCII strings from MCAP file",
	Example: `
  # Extract strings from an MCAP file and print to stdout
  modbusctl mcap strings --input file.mcap

  # Extract strings and write output to a file
  modbusctl mcap strings --input file.mcap --output strings.txt

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=file.mcap MODBUSCTL_OUTPUT=strings.txt modbusctl mcap strings
`,
}

func init() {
	cfg := config.StringsConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(stringsCmd, &cfg)
	cli.MustMarkFlagRequired(stringsCmd, "input")
	McapCmd.AddCommand(stringsCmd)

	stringsCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckStringsConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := format.ExportStrings(w, cfg); err != nil {
			return errs.Output(errs.CodeMCAPLoadFailed, err)
		}
		return nil
	}
}
