package sunspec

import (
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/spf13/cobra"
)

// SunspecCmd is the parent command for SunSpec discovery (detect, models, map, probe).
var SunspecCmd = &cobra.Command{
	Use:   "sunspec",
	Short: "SunSpec marker detection and model header discovery (transport-level, no semantic decoding)",
}

// Deprecated: use --format json on each subcommand.
var sunspecLegacyJSON bool

// mergeSunspecOutputFormat applies hidden parent --json as --format json.
func mergeSunspecOutputFormat(cfg *config.SunSpecBaseConfig) {
	if sunspecLegacyJSON {
		cfg.OutputFormat = string(format.FormatJSON)
	}
}

func init() {
	SunspecCmd.PersistentFlags().BoolVar(&sunspecLegacyJSON, "json", false, "Deprecated: use --format json")
	_ = SunspecCmd.PersistentFlags().MarkHidden("json")
	SunspecCmd.AddCommand(detectCmd)
	SunspecCmd.AddCommand(modelsCmd)
	SunspecCmd.AddCommand(mapCmd)
	SunspecCmd.AddCommand(probeCmd)
}
