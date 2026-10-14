package server

import (
	"context"
	"errors"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/validate"
	"github.com/spf13/cobra"
)

var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay a capture as a Modbus TCP device whose values follow the recording",
	Example: `
  # Replay a capture at its original speed, looping forever, every target on its recorded unit id
  modbusctl server replay --input=recording.mbcap

  # Ten times faster, three times, starting 5 minutes in
  modbusctl server replay --input=recording.mbcap --speed=10 --loops=3 --from=5m

  # One target as unit 1 on port 1502; addresses are absent until first sampled
  modbusctl server replay --input=site.mbcap --target=2 --unit=1 --port=1502 --before-first=absent

  # Use environment variables instead of CLI arguments
  MODBUSCTL_PORT=1502 MODBUSCTL_INPUT=recording.mbcap MODBUSCTL_SPEED=60 modbusctl server replay
`,
}

func init() {
	cfg := config.ReplayServerConfig{
		Port:      502,
		Loops:     0,
		InputFile: "",
	}
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(replayCmd, &cfg)
	cli.MustMarkFlagRequired(replayCmd, "input")
	ServerCmd.AddCommand(replayCmd)

	replayCmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg.Debug = cli.Debug(cmd)
		if err := validate.CheckReplayServerConfig(cfg); err != nil {
			return errs.WrapValidation(err)
		}
		if err := modbus.ServeReplay(cmd.Context(), cfg, cmd.OutOrStdout()); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return modbus.WrapCollectError(err)
		}
		return nil
	}
}
