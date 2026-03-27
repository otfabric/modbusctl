// Package cli holds small Cobra helpers shared by commands (e.g. generic enum flag completion).
package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// RegisterEnumFlagCompletion registers shell completion for a flag whose value must be one of values.
// Matches case-insensitively on the prefix of toComplete.
func RegisterEnumFlagCompletion(cmd *cobra.Command, flagName string, values []string) error {
	return cmd.RegisterFlagCompletionFunc(flagName, func(
		_ *cobra.Command,
		_ []string,
		toComplete string,
	) ([]string, cobra.ShellCompDirective) {
		prefix := strings.ToLower(strings.TrimSpace(toComplete))
		out := make([]string, 0, len(values))
		for _, v := range values {
			if prefix == "" || strings.HasPrefix(strings.ToLower(v), prefix) {
				out = append(out, v)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	})
}

// RegisterEnumFlagCompletionWithDescriptions registers completion where each candidate is "value\tdescription"
// (Cobra shows the description in the shell). Keys of descByValue are the legal flag values; values are short help.
// Output order is sorted by value for stable completions.
func RegisterEnumFlagCompletionWithDescriptions(cmd *cobra.Command, flagName string, descByValue map[string]string) error {
	return cmd.RegisterFlagCompletionFunc(flagName, func(
		_ *cobra.Command,
		_ []string,
		toComplete string,
	) ([]string, cobra.ShellCompDirective) {
		prefix := strings.ToLower(strings.TrimSpace(toComplete))
		keys := make([]string, 0, len(descByValue))
		for k := range descByValue {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []string
		for _, v := range keys {
			if prefix != "" && !strings.HasPrefix(strings.ToLower(v), prefix) {
				continue
			}
			if d := descByValue[v]; d != "" {
				out = append(out, v+"\t"+d)
			} else {
				out = append(out, v)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	})
}
