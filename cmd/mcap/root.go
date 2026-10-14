package mcap

import (
	"github.com/spf13/cobra"
)

var McapCmd = &cobra.Command{
	Use:   "mcap",
	Short: "Inspect, verify, convert and analyse capture files (MBCAP)",
}
