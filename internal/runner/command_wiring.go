// SPDX-License-Identifier: MIT

package runner

import (
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/spf13/cobra"
)

// RegisterClientCommandCfg runs [config.DeferLoadFromEnv], [config.RegisterFlags],
// [RegisterStdoutFormatCompletion], [config.RegisterParityCompletion], then optional hooks (extra completions, required flags, etc.).
func RegisterClientCommandCfg(cmd *cobra.Command, cfg any, hooks ...func(*cobra.Command)) {
	config.DeferLoadFromEnv(cfg)
	config.RegisterFlags(cmd, cfg)
	RegisterStdoutFormatCompletion(cmd)
	config.RegisterParityCompletion(cmd)
	for _, h := range hooks {
		if h != nil {
			h(cmd)
		}
	}
}

// WireClientCommand calls [RegisterClientCommandCfg] and adds cmd to parent.
func WireClientCommand(parent, cmd *cobra.Command, cfg any, hooks ...func(*cobra.Command)) {
	RegisterClientCommandCfg(cmd, cfg, hooks...)
	parent.AddCommand(cmd)
}
