package runner

import (
	"context"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/spf13/cobra"
)

// RunClientFormattedWithDebug sets debug via setDebug(cli.Debug(cmd)), then runs [RunClientFormatted].
// Use from command RunE closures to avoid repeating the debug line across client subcommands.
func RunClientFormattedWithDebug(cmd *cobra.Command, setDebug func(bool), outputFormat string, validate func() error, collect func(context.Context) (any, error), opts ...FormatRunOption) error {
	setDebug(cli.Debug(cmd))
	return RunClientFormatted(cmd, outputFormat, validate, collect, opts...)
}
