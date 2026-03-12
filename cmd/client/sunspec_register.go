package client

import (
	"github.com/otfabric/modbusctl/cmd/client/sunspec"
)

func init() {
	ClientCmd.AddCommand(sunspec.SunspecCmd)
}
