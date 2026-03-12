package sunspec

import "github.com/spf13/cobra"

// SunspecCmd is the parent command for SunSpec discovery (detect, models, map, probe).
var SunspecCmd = &cobra.Command{
	Use:   "sunspec",
	Short: "SunSpec marker detection and model header discovery (transport-level, no semantic decoding)",
}

func init() {
	SunspecCmd.AddCommand(detectCmd)
	SunspecCmd.AddCommand(modelsCmd)
	SunspecCmd.AddCommand(mapCmd)
	SunspecCmd.AddCommand(probeCmd)
}
