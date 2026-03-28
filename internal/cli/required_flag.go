package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// MustMarkFlagRequired marks a flag required and panics if Cobra wiring is invalid (fail-fast at init).
func MustMarkFlagRequired(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(fmt.Sprintf("modbusctl: MarkFlagRequired(%q) on %q: %v", name, cmd.Name(), err))
	}
}
