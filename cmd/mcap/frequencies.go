package mcap

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var freqCfg config.StringsConfig

var freqCmd = &cobra.Command{
	Use:   "frequencies",
	Short: "Heuristically detect frequency values in MCAP file",
	Example: `
  # Scan for potential frequency values in an MCAP file and print to stdout
  modbusctl mcap frequencies --input file.mcap

  # Write detected frequencies to a file
  modbusctl mcap frequencies --input file.mcap --output freq.txt

  # Use environment variables
  MODBUSCTL_INPUT=file.mcap MODBUSCTL_OUTPUT=freq.txt modbusctl mcap frequencies
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckStringsConfig(freqCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		var out *os.File
		if freqCfg.OutputFile != "" {
			var err error
			out, err = os.Create(freqCfg.OutputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Failed to create output file: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = out.Close() }()
		} else {
			out = os.Stdout
		}

		if err := format.ExportHeuristicFrequency(out, freqCfg.InputFile); err != nil {
			fmt.Fprintf(os.Stderr, "❌ ExportHeuristicFrequency failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	McapCmd.AddCommand(freqCmd)
	freqCfg = config.StringsConfig{
		InputFile:  "",
		OutputFile: "",
	}
	config.LoadFromEnv(&freqCfg)
	config.RegisterFlags(freqCmd, &freqCfg)
	if freqCfg.InputFile == "" {
		if err := freqCmd.MarkFlagRequired("input"); err != nil {
			panic(err)
		}
	}
}
