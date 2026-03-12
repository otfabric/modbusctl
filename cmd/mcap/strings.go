package mcap

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var stringsCfg config.StringsConfig

var stringsCmd = &cobra.Command{
	Use:   "strings",
	Short: "Extract ASCII strings from MCAP file",
	Example: `
  # Extract strings from an MCAP file and print to stdout
  modbusctl mcap strings --input file.mcap

  # Extract strings and write output to a file
  modbusctl mcap strings --input file.mcap --output strings.txt

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=file.mcap MODBUSCTL_OUTPUT=strings.txt modbusctl mcap strings
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckStringsConfig(stringsCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		var out *os.File
		if stringsCfg.OutputFile != "" {
			var err error
			out, err = os.Create(stringsCfg.OutputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Failed to create output file: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = out.Close() }()
		} else {
			out = os.Stdout
		}

		if err := format.ExportStrings(out, stringsCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ ExportStrings failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	McapCmd.AddCommand(stringsCmd)
	stringsCfg = config.StringsConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.LoadFromEnv(&stringsCfg)
	config.RegisterFlags(stringsCmd, &stringsCfg)
	if stringsCfg.InputFile == "" {
		if err := stringsCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
}
