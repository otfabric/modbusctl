package runner

import (
	"context"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/spf13/cobra"
)

// RunClientStream is the streaming counterpart of [RunClientFormattedWithDebug] for commands
// that write records continuously instead of one final result. It sets debug, attaches the
// output format, runs validate, then calls run with a [format.StreamWriter] on stdout.
// run returns the process exit code for a completed stream (ExitOK or ExitPartial).
func RunClientStream(cmd *cobra.Command, setDebug func(bool), outputFormat string, validate func() error, run func(ctx context.Context, out *format.StreamWriter) (int, error)) error {
	setDebug(cli.Debug(cmd))
	outFmt, err := AttachOutputFormat(cmd, outputFormat)
	if err != nil {
		return err
	}
	if err := validate(); err != nil {
		return errs.WrapValidation(err)
	}
	code, err := run(cmd.Context(), format.NewStreamWriter(cmd.OutOrStdout(), outFmt))
	if err != nil {
		return err
	}
	if inv := InvocationFrom(cmd.Context()); inv != nil {
		inv.SetSuccessExit(code)
	}
	return nil
}
