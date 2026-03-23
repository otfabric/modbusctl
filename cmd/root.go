package cmd

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/cmd/client"
	"github.com/otfabric/modbusctl/cmd/discover"
	"github.com/otfabric/modbusctl/cmd/mcap"
	"github.com/otfabric/modbusctl/cmd/server"

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
		fmt.Printf("modbusctl version: %s\n", buildMeta.version)
		fmt.Printf("tag:         %s\n", buildMeta.tag)
		fmt.Printf("commit:      %s\n", buildMeta.commit)
		fmt.Printf("build date:  %s\n", buildMeta.buildDate)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
