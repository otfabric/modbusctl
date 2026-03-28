package mcap

import (
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	mcapfile "github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var decodeCmd = &cobra.Command{
	Use:   "decode",
	Short: "Decode MCAP file to CSV or JSON based on a device profile",
	Example: `
  # Decode MCAP file using a device profile and output to CSV
  modbusctl mcap decode --input data.mcap --profile device_profile.json --output output.csv

  # Decode MCAP file using a device profile and output to JSON
  modbusctl mcap decode --input data.mcap --profile device_profile.json --output output.json

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=data.mcap MODBUSCTL_PROFILE=device_profile.json MODBUSCTL_OUTPUT=output.csv modbusctl mcap decode
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

		if err := mcapfile.ExportDeviceProfileDecode(w, cfg.InputFile, cfg.DeviceProfile); err != nil {
			return errs.Output(errs.CodeMCAPLoadFailed, err)
		}
		return nil
	}
}
