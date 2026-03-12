package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	completionBash       = "bash"
	completionZsh        = "zsh"
	completionFish       = "fish"
	completionPowerShell = "powershell"
)

var completionShells = []string{completionBash, completionZsh, completionFish, completionPowerShell}

var completionCmd = &cobra.Command{
	Use:       "completion [bash|zsh|fish|powershell]",
	Short:     "Generate shell completion script",
	Long:      "Output a completion script for the given shell. Install with: Bash: source <(modbusctl completion bash); Zsh: source <(modbusctl completion zsh); Fish: modbusctl completion fish | source.",
	ValidArgs: completionShells,
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := args[0]
		root := cmd.Root()
		switch shell {
		case completionBash:
			return root.GenBashCompletionV2(os.Stdout, true)
		case completionZsh:
			return root.GenZshCompletion(os.Stdout)
		case completionFish:
			return root.GenFishCompletion(os.Stdout, true)
		case completionPowerShell:
			return root.GenPowerShellCompletion(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell %q (use one of: %v)", shell, completionShells)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
