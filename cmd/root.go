package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/otfabric/modbusctl/cmd/client"
	"github.com/otfabric/modbusctl/cmd/discover"
	"github.com/otfabric/modbusctl/cmd/mcap"
	"github.com/otfabric/modbusctl/cmd/server"
	"github.com/otfabric/modbusctl/internal/runner"

	"github.com/spf13/cobra"
)

// buildMeta holds link-time metadata set from main via SetBuildMeta.
var buildMeta struct {
	version, tag, commit, buildDate string
}

// SetBuildMeta is called from main before Execute; values come from ldflags -X main.*.
func SetBuildMeta(version, tag, commit, buildDate string) {
	buildMeta.version = version
	buildMeta.tag = tag
	buildMeta.commit = commit
	buildMeta.buildDate = buildDate
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
	rootCmd.AddCommand(mcap.McapCmd)
	rootCmd.AddCommand(server.ServerCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of modbusctl",
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		ver := strings.TrimSpace(buildMeta.version)
		if ver == "" {
			ver = "(unknown)"
		}
		_, _ = fmt.Fprintf(out, "modbusctl version: %s\n", ver)
		if s := strings.TrimSpace(buildMeta.tag); s != "" {
			_, _ = fmt.Fprintf(out, "tag:         %s\n", s)
		}
		if s := strings.TrimSpace(buildMeta.commit); s != "" {
			_, _ = fmt.Fprintf(out, "commit:      %s\n", s)
		}
		if s := strings.TrimSpace(buildMeta.buildDate); s != "" {
			_, _ = fmt.Fprintf(out, "build date:  %s\n", s)
		}
	},
}

// Execute runs the root command and returns the process exit code (main should call os.Exit).
func Execute() int {
	inv := &runner.Invocation{}
	ctx := runner.WithInvocation(context.Background(), inv)
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	rootCmd.SetContext(ctx)

	err := rootCmd.ExecuteContext(ctx)
	debug, _ := rootCmd.PersistentFlags().GetBool("debug")

	if err != nil {
		return runner.RenderFatal(inv, err, os.Stdout, os.Stderr, debug)
	}

	if inv.SuccessExitSet {
		return inv.SuccessExitCode
	}
	return 0
}
