package capture

import (
	"github.com/spf13/cobra"
)

// CaptureCmd groups the commands that analyse the register values of a capture
// file. Inspecting, verifying, converting, splitting, filtering, trimming and
// repairing capture files is mbcapctl's job (github.com/otfabric/go-modbus-capture).
var CaptureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Analyse the register values of capture files (MBCAP); use mbcapctl to inspect, verify or convert them",
}
