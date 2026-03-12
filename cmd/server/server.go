package server

import "github.com/spf13/cobra"

var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Commands for Modbus TCP server operations",
}

func init() {
}
