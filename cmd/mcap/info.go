package mcap

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var infoCfg config.InfoConfig

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Extract header and record info from MCAP file",
	Example: `
  # Get information about an MCAP file and print to stdout
  modbusctl mcap info --input data.mcap

  # Get information about an MCAP file and save to output file
  modbusctl mcap info --input data.mcap --output info.txt

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=data.mcap MODBUSCTL_OUTPUT=info.txt modbusctl mcap info
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckInfoConfig(infoCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		var out *os.File
		if infoCfg.OutputFile != "" {
			var err error
			out, err = os.Create(infoCfg.OutputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Failed to create output file: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = out.Close() }()
		} else {
			out = os.Stdout
		}

		if err := format.ExportInfo(out, infoCfg.InputFile); err != nil {
			fmt.Fprintf(os.Stderr, "❌ ExportInfo failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	McapCmd.AddCommand(infoCmd)
	infoCfg = config.InfoConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.LoadFromEnv(&infoCfg)
	config.RegisterFlags(infoCmd, &infoCfg)
	if infoCfg.InputFile == "" {
		if err := infoCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
}
