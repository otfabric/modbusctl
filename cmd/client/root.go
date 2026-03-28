package client

import "github.com/spf13/cobra"

var ClientCmd = &cobra.Command{
	Use:   "client",
	Short: "Commands for Modbus TCP client operations",
}
