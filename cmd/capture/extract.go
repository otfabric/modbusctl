// SPDX-License-Identifier: MIT

package capture

import (
	"github.com/otfabric/modbusctl/internal/analysis"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract address blocks from capture file to JSON",
	Example: `
  # Extract address blocks from a capture file and output to stdout
  modbusctl capture extract --input input.mbcap --output register-ranges.json

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=input.mbcap MODBUSCTL_OUTPUT=register-ranges.json modbusctl capture extract
`,
}

func init() {
	cfg := config.ExtractConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.DeferLoadFromEnv(&cfg)
	config.RegisterFlags(extractCmd, &cfg)
	cli.MustMarkFlagRequired(extractCmd, "input")
	CaptureCmd.AddCommand(extractCmd)

	extractCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckExtractConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := analysis.ExportAddressBlocks(w, cfg.InputFile, cfg.CaptureSelection); err != nil {
			return capfile.WrapError(err)
		}
		return nil
	}
}
