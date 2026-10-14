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

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert a capture between mbcap, jsonl and csv, or to a wide-csv view",
	Example: `
  # Convert a capture to JSON Lines on stdout (lossless; converts back with --format mbcap)
  modbusctl mcap convert --input data.mbcap --format jsonl

  # Convert to long CSV for a spreadsheet (lossless)
  modbusctl mcap convert --input data.mbcap --format csv --output data.mbcap.csv

  # Chart raw register values per address over time (one-way view)
  modbusctl mcap convert --input data.mbcap --format wide-csv --output registers.wide.csv

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=data.mbcap MODBUSCTL_FORMAT=jsonl modbusctl mcap convert
`,
}

func init() {
	cfg := config.ConvertConfig{
		InputFile:  "",
		FormatType: "",
		OutputFile: "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(convertCmd, &cfg)
	config.RegisterConvertFormatCompletion(convertCmd)
	cli.MustMarkFlagRequired(convertCmd, "input")
	cli.MustMarkFlagRequired(convertCmd, "format")
	McapCmd.AddCommand(convertCmd)

	convertCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckConvertConfig(&cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := mcapfile.Convert(w, cfg.InputFile, cfg.FormatType); err != nil {
			return capfile.WrapError(err)
		}
		return nil
	}
}
