package mcap

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var convertCfg config.ConvertConfig

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert MCAP file to CSV or JSON",
	Example: `
  # Convert MCAP file to CSV and output to stdout
  modbusctl mcap convert --input file.mcap --format csv

  # Convert MCAP file to JSON and save to output file
  modbusctl mcap convert --input file.mcap --format json --output output.json

  # Use environment variables instead of CLI arguments
  MODBUSCTL_INPUT=file.mcap MODBUSCTL_FORMAT=csv modbusctl mcap convert
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckConvertConfig(convertCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		var out *os.File
		if convertCfg.OutputFile != "" {
			var err error
			out, err = os.Create(convertCfg.OutputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Failed to create output file: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = out.Close() }()
		} else {
			out = os.Stdout
		}

		switch convertCfg.FormatType {
		case "csv":
			if err := format.ExportCSV(out, convertCfg.InputFile); err != nil {
				fmt.Fprintf(os.Stderr, "❌ ExportCSV failed: %v\n", err)
				os.Exit(1)
			}
		case "json":
			if err := format.ExportJSON(out, convertCfg.InputFile); err != nil {
				fmt.Fprintf(os.Stderr, "❌ ExportJSON failed: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "❌ Unsupported format: %s\n", convertCfg.FormatType)
			os.Exit(1)
		}
	},
}

func init() {
	McapCmd.AddCommand(convertCmd)
	convertCfg = config.ConvertConfig{
		InputFile:  "",
		FormatType: "",
		OutputFile: "",
	}
	config.LoadFromEnv(&convertCfg)
	config.RegisterFlags(convertCmd, &convertCfg)
	config.RegisterConvertFormatCompletion(convertCmd)
	if convertCfg.InputFile == "" {
		if err := convertCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
	if convertCfg.FormatType == "" {
		if err := convertCmd.MarkFlagRequired("format"); err != nil {
			panic(err)
		}
	}
}
