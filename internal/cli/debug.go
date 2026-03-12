package cli

import "github.com/spf13/cobra"

// Debug returns the root --debug flag value. Use from any command's Run to pass
// debug into config (e.g. for scan, read, record). More per-command debug
// behavior can be added later.
func Debug(c *cobra.Command) bool {
	root := c.Root()
	if root == nil {
		return false
	}
	v, err := root.PersistentFlags().GetBool("debug")
	if err != nil {
		return false
	}
	return v
}
