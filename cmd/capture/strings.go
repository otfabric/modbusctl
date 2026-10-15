package capture

import (
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var stringsCmd = &cobra.Command{
	Use:   "strings",
	Short: "Extract ASCII strings from capture file",
	Example: `
  # Extract strings from a capture file and print to stdout
  modbusctl capture strings --input file.mbcap

  # Extract strings and write output to a file
  modbusctl capture strings --input file.mbcap --output strings.txt

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=file.mbcap MODBUSCTL_OUTPUT=strings.txt modbusctl capture strings
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
	CaptureCmd.AddCommand(stringsCmd)

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
			return capfile.WrapError(err)
		}
		return nil
	}
}
