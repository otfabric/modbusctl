package mcap

import (
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	mcapfile "github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert MCAP file to CSV or JSON",
	Example: `
  # Convert MCAP file to CSV and output to stdout
  modbusctl mcap convert --input file.mcap --format csv

  # Convert MCAP file to JSON and save to output file
  modbusctl mcap convert --input file.mcap --format json --output output.json

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=file.mcap MODBUSCTL_FORMAT=csv modbusctl mcap convert
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

		switch cfg.FormatType {
		case "csv":
			if err := mcapfile.ExportCSV(w, cfg.InputFile); err != nil {
				return errs.Output(errs.CodeMCAPLoadFailed, err)
			}
		case "json":
			if err := mcapfile.ExportJSON(w, cfg.InputFile); err != nil {
				return errs.Output(errs.CodeMCAPLoadFailed, err)
			}
		default:
			return errs.InvalidInput(errs.CodeInvalidInput, "unsupported format: "+cfg.FormatType, nil)
		}
		return nil
	}
}
