package discover

import (
	"context"
	"fmt"
	"os/user"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

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
}

func init() {
	cfg := config.DiscoverConfig{
		Subnets:          []string{},
		Port:             502,
		Parallel:         1,
		ResolveMAC:       false,
		NetworkInterface: "",
		OutputFile:       "",
		OutputFormat:     string(format.FormatText),
		ForceLargeScan:   false,
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(DiscoverCmd, &cfg)
	cli.MustMarkFlagRequired(DiscoverCmd, "subnets")
	runner.RegisterStdoutFormatCompletion(DiscoverCmd)

	DiscoverCmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg.Debug = cli.Debug(cmd)
		if err := validate.CheckDiscoverConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		if cfg.ResolveMAC {
			if u, err := user.Current(); err == nil && u.Uid != "0" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: Resolving MAC addresses typically requires elevated privileges (e.g., sudo).\n")
			}
		}
		_, err := runner.RunFormatted(cmd, cfg.OutputFormat, func(ctx context.Context) (any, error) {
			return modbus.CollectDiscover(ctx, cfg, cmd.ErrOrStderr())
		})
		return err
	}
}
