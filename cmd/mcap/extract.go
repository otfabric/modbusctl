package mcap

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var extractCfg config.ExtractConfig

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract address blocks from MCAP file to JSON",
	Example: `
  # Extract address blocks from an MCAP file and output to stdout
  modbusctl mcap extract --input input.mcap --output register-ranges.json

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=input.mcap MODBUSCTL_OUTPUT=register-ranges.json modbusctl mcap extract
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckExtractConfig(extractCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		var out *os.File
		if extractCfg.OutputFile != "" {
			var err error
			out, err = os.Create(extractCfg.OutputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Failed to create output file: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = out.Close() }()
		} else {
			out = os.Stdout
		}

		if err := format.ExportAddressBlocks(out, extractCfg.InputFile); err != nil {
			fmt.Fprintf(os.Stderr, "❌ ExportAddressBlocks failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	McapCmd.AddCommand(extractCmd)
	extractCfg = config.ExtractConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.LoadFromEnv(&extractCfg)
	config.RegisterFlags(extractCmd, &extractCfg)
	if extractCfg.InputFile == "" {
		if err := extractCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
}
