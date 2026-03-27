package format

import (
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/spf13/cobra"
)

// Descriptions for [Values]; keys must match [Values] exactly (see [stdout_format_completion_test]).
var stdoutFormatDescriptions = map[string]string{
	string(FormatText):  "human-readable default",
	string(FormatJSON):  "machine-readable structured output",
	string(FormatTable): "human-readable tabular output",
}

// RegisterStdoutFormatFlagCompletion registers --format completion for client stdout reporting.
// Thin wrapper over [cli.RegisterEnumFlagCompletionWithDescriptions]; legal values are [Values].
func RegisterStdoutFormatFlagCompletion(cmd *cobra.Command) error {
	return cli.RegisterEnumFlagCompletionWithDescriptions(cmd, "format", stdoutFormatDescriptions)
}
