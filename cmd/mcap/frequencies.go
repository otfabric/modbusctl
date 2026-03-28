package mcap

import (
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var freqCmd = &cobra.Command{
	Use:   "frequencies",
	Short: "Heuristically detect frequency values in MCAP file",
	Example: `
  # Scan for potential frequency values in an MCAP file and print to stdout
  modbusctl mcap frequencies --input file.mcap

  # Write detected frequencies to a file
  modbusctl mcap frequencies --input file.mcap --output freq.txt

  # Use environment variables
  MODBUSCTL_INPUT=file.mcap MODBUSCTL_OUTPUT=freq.txt modbusctl mcap frequencies
`,
}

func init() {
	cfg := config.StringsConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(freqCmd, &cfg)
	cli.MustMarkFlagRequired(freqCmd, "input")
	McapCmd.AddCommand(freqCmd)

	freqCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckStringsConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := format.ExportHeuristicFrequency(w, cfg.InputFile); err != nil {
			return errs.Output(errs.CodeMCAPLoadFailed, err)
		}
		return nil
	}
}
