package discover

import (
	"fmt"
	"os"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var discoverCfg config.DiscoverConfig

var DiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover Modbus devices in one or more subnets",
	Example: `
  # Discover devices in the 192.168.1.0/24 subnet
  modbusctl discover --subnets 192.168.1.0/24

  # Discover devices in multiple subnets
  modbusctl discover --subnets 192.168.1.0/24,192.168.2.0/24

  # Discover devices on a custom port with parallel scans
  modbusctl discover --subnets 192.168.1.0/24 --port 1502 --parallel 5

  # Discover devices and resolve MAC addresses
  sudo modbusctl discover --subnets 192.168.1.0/24 --resolve-mac

  # Discover devices using a custom network interface and resolve MAC addresses
  sudo modbusctl discover --subnets 192.168.1.0/24 --resolve-mac --interface eth1

  # Use environment variables instead of CLI arguments
  MODBUSCTL_SUBNETS=192.168.1.0/24,192.168.2.0/24 MODBUSCTL_PORT=1502 modbusctl discover
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validate.CheckDiscoverConfig(discoverCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid input: %v\n", err)
			os.Exit(1)
		}

		if err := modbus.PerformDiscoveryScan(discoverCfg); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Discovery failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	discoverCfg = config.DiscoverConfig{
		Subnets:          []string{},
		Port:             502,
		Parallel:         1,
		ResolveMAC:       false,
		NetworkInterface: "eth0",
		OutputFile:       "",
	}
	config.LoadFromEnv(&discoverCfg)
	config.RegisterFlags(DiscoverCmd, &discoverCfg)
	if len(discoverCfg.Subnets) == 0 {
		if err := DiscoverCmd.MarkFlagRequired("subnets"); err != nil {
			panic(err)
		}
	}
}
