// SPDX-License-Identifier: MIT

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

var freqCmd = &cobra.Command{
	Use:   "frequencies",
	Short: "Heuristically detect frequency values in capture file",
	Example: `
  # Scan for potential frequency values in a capture file and print to stdout
  modbusctl capture frequencies --input file.mbcap

  # Write detected frequencies to a file
  modbusctl capture frequencies --input file.mbcap --output freq.txt

  # Use environment variables
  MODBUSCTL_INPUT=file.mbcap MODBUSCTL_OUTPUT=freq.txt modbusctl capture frequencies
`,
}

func init() {
	cfg := config.StringsConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.DeferLoadFromEnv(&cfg)
	config.RegisterFlags(freqCmd, &cfg)
	cli.MustMarkFlagRequired(freqCmd, "input")
	CaptureCmd.AddCommand(freqCmd)

	freqCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckStringsConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := format.ExportHeuristicFrequency(w, cfg.InputFile, cfg.CaptureSelection); err != nil {
			return capfile.WrapError(err)
		}
		return nil
	}
}
