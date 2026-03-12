package mcap

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var decodeCfg config.DeviceProfileDecodeConfig

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
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckDeviceProfileDecodeConfig(decodeCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		var out *os.File
		if decodeCfg.OutputFile != "" {
			var err error
			out, err = os.Create(decodeCfg.OutputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Failed to create output file: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = out.Close() }()
		} else {
			out = os.Stdout
		}

		if err := format.ExportDeviceProfileDecode(out, decodeCfg.InputFile, decodeCfg.DeviceProfile); err != nil {
			fmt.Fprintf(os.Stderr, "❌ ExportDeviceProfileDecode failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	McapCmd.AddCommand(decodeCmd)
	decodeCfg = config.DeviceProfileDecodeConfig{
		InputFile:     "",
		DeviceProfile: "",
		OutputFile:    "",
	}
	config.LoadFromEnv(&decodeCfg)
	config.RegisterFlags(decodeCmd, &decodeCfg)
	if decodeCfg.InputFile == "" {
		if err := decodeCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
	if decodeCfg.DeviceProfile == "" {
		if err := decodeCmd.MarkFlagRequired("profile"); err != nil {
			panic(err)
		}
	}
}
