package format

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const outputFormatFlag = "format"

// AddOutputFormatFlag registers --format with default "text".
// Prefer also wiring MODBUSCTL_OUTPUT_FORMAT via your command config struct tags
// (see IdentifyConfig) so LoadFromEnv applies before flags.
func AddOutputFormatFlag(cmd *cobra.Command, target *string) {
	if target != nil && *target == "" {
		*target = string(FormatText)
	}
	cmd.Flags().StringVar(target, outputFormatFlag, string(FormatText),
		fmt.Sprintf("Output format: %s [env: MODBUSCTL_OUTPUT_FORMAT]", strings.Join(Values(), ", ")))
}
