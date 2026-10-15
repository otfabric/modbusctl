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

var decodeCmd = &cobra.Command{
	Use:   "decode",
	Short: "Decode the registers of a capture with a device profile",
	Example: `
  # Decode the registers of a capture with a device profile, to a file
  modbusctl capture decode --input data.mbcap --profile device_profile.json --output output.csv

  # Decode one target and function of a capture holding several
  modbusctl capture decode --input data.mbcap --profile device_profile.json --output output.json

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=data.mbcap MODBUSCTL_PROFILE=device_profile.json MODBUSCTL_OUTPUT=output.csv modbusctl capture decode
`,
}

func init() {
	cfg := config.DeviceProfileDecodeConfig{
		InputFile:     "",
		DeviceProfile: "",
		OutputFile:    "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(decodeCmd, &cfg)
	CaptureCmd.AddCommand(decodeCmd)

	decodeCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckDeviceProfileDecodeConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := analysis.ExportDeviceProfileDecode(w, cfg.InputFile, cfg.DeviceProfile, cfg.CaptureSelection); err != nil {
			return capfile.WrapError(err)
		}
		return nil
	}
}
