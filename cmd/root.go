// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/otfabric/modbusctl/cmd/capture"
	"github.com/otfabric/modbusctl/cmd/client"
	"github.com/otfabric/modbusctl/cmd/discover"
	"github.com/otfabric/modbusctl/cmd/server"
	"github.com/otfabric/modbusctl/internal/buildinfo"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/runner"

	"github.com/spf13/cobra"
)

// SetBuildMeta is called from main before Execute; values come from ldflags -X main.*.
func SetBuildMeta(version, tag, commit, buildDate string) {
	buildinfo.Set(version, tag, commit, buildDate)
	if v := strings.TrimSpace(version); v != "" {
		capfile.Producer = "modbusctl " + v
	}
}

var rootCmd = &cobra.Command{
	Use:   "modbusctl",
	Short: "Modbusctl CLI application",
}

func init() {
	rootCmd.PersistentFlags().Bool("debug", false, "Print debug information")
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.AddCommand(client.ClientCmd)
	rootCmd.AddCommand(discover.DiscoverCmd)
	rootCmd.AddCommand(capture.CaptureCmd)
	rootCmd.AddCommand(server.ServerCmd)
	rootCmd.AddCommand(versionCmd)
	refuseUnknownSubcommands(rootCmd)
	registerFlagCompletions(rootCmd)
}

// refuseUnknownSubcommands makes the unknown subcommand of a command group a usage error.
// Cobra treats a group as not runnable, and answers any arguments with its help and exit 0.
func refuseUnknownSubcommands(c *cobra.Command) {
	if c.HasParent() && c.HasSubCommands() && !c.Runnable() {
		c.Args = cobra.NoArgs
		c.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	}
	for _, sub := range c.Commands() {
		refuseUnknownSubcommands(sub)
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of modbusctl",
	Long: "Print the version, tag, commit, build date, Go version and platform of modbusctl on one line, " +
		"or as one JSON object with --json or MODBUSCTL_FORMAT=json.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		info := buildinfo.Get("modbusctl")
		asJSON, _ := cmd.Flags().GetBool("json")
		if !asJSON {
			if f, err := format.Parse(config.Getenv("MODBUSCTL_FORMAT")); err == nil && f == format.FormatJSON {
				asJSON = true
			}
		}
		line := info.String()
		if asJSON {
			line = info.JSON()
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), line)
		return err
	},
}

func init() {
	versionCmd.Flags().Bool("json", false, "Print the build information as one JSON object")
}

var typeRunErrors sync.Once

// typeCommandErrors makes every error a command returns while running typed, which leaves
// only the errors cobra raises itself untyped: see [classifyFatal].
func typeCommandErrors(c *cobra.Command) {
	if run := c.RunE; run != nil {
		c.RunE = func(cmd *cobra.Command, args []string) error {
			err := run(cmd, args)
			if err == nil {
				return nil
			}
			if _, ok := errs.As(err); ok {
				return err
			}
			return errs.New(errs.KindInternal, errs.CodeInternalUnexpected, err.Error(), err)
		}
	}
	for _, sub := range c.Commands() {
		typeCommandErrors(sub)
	}
}

// classifyFatal settles the kind of a fatal error for the exit code. An interruption is
// interrupted whatever operation it stopped, and an error that is still untyped comes from
// cobra, which rejected the invocation before a command ran.
func classifyFatal(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return errs.New(errs.KindInterrupted, errs.CodeContextCanceled, "interrupted", err)
	}
	if _, ok := errs.As(err); ok {
		return err
	}
	return errs.New(errs.KindUsage, errs.CodeInvalidUsage, err.Error(), err)
}

// Execute runs the root command and returns the process exit code (main should call os.Exit).
func Execute() int {
	return run(os.Args[1:])
}

// run is Execute with its arguments: what the tests drive.
func run(args []string) int {
	if err := config.EnvError(); err != nil {
		fatal := errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("invalid environment: %v", err), err)
		return runner.RenderFatal(requestedInvocation(args), fatal, os.Stdout, os.Stderr, false)
	}
	inv := &runner.Invocation{}
	ctx := runner.WithInvocation(context.Background(), inv)
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	rootCmd.SetContext(ctx)
	rootCmd.SetArgs(args)
	typeRunErrors.Do(func() { typeCommandErrors(rootCmd) })

	err := classifyFatal(rootCmd.ExecuteContext(ctx))
	debug, _ := rootCmd.PersistentFlags().GetBool("debug")

	if err != nil {
		if !inv.HasOutputFormat {
			// Cobra or the command refused the invocation before the format was attached.
			inv = requestedInvocation(args)
		}
		return runner.RenderFatal(inv, err, os.Stdout, os.Stderr, debug)
	}

	if inv.SuccessExitSet {
		return inv.SuccessExitCode
	}
	return 0
}

// requestedInvocation says in which format to report an error from before a command attached
// its format (an unknown flag, a missing argument, a malformed environment variable): as
// JSON when the invocation asks for JSON, with --format or the environment, so that a script
// that reads JSON from stdout gets its error object in every case.
func requestedInvocation(args []string) *runner.Invocation {
	inv := &runner.Invocation{}
	asked := config.Getenv("MODBUSCTL_FORMAT")
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if v, ok := strings.CutPrefix(arg, "--format="); ok {
			asked = v
		} else if arg == "--format" && i+1 < len(args) {
			asked = args[i+1]
		}
	}
	if f, err := format.Parse(asked); err == nil && f == format.FormatJSON {
		inv.SetOutputFormat(f)
	}
	return inv
}
