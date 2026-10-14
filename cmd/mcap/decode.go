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

var decodeCmd = &cobra.Command{
	Use:   "decode",
	Short: "Decode the registers of a capture with a device profile",
	Example: `
  # Decode the registers of a capture with a device profile, to a file
  modbusctl mcap decode --input data.mbcap --profile device_profile.json --output output.csv

  # Decode one target and function of a capture holding several
  modbusctl mcap decode --input data.mbcap --profile device_profile.json --output output.json

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=data.mbcap MODBUSCTL_PROFILE=device_profile.json MODBUSCTL_OUTPUT=output.csv modbusctl mcap decode
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
	McapCmd.AddCommand(decodeCmd)

	decodeCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validate.CheckDeviceProfileDecodeConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		w, cleanup, err := cli.OpenStdoutOrFile(cfg.OutputFile)
		if err != nil {
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		defer cleanup()

		if err := mcapfile.ExportDeviceProfileDecode(w, cfg.InputFile, cfg.DeviceProfile, cfg.CaptureSelection); err != nil {
			return capfile.WrapError(err)
		}
		return nil
	}
}
