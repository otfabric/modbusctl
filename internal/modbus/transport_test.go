// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/require"
)

// transportCase serves a device over one transport and returns the URL to reach it.
type transportCase struct {
	name   string
	serve  func(t *testing.T, d *mbtest.Device) string
	serial config.SerialConfig
}

func transportCases() []transportCase {
	return []transportCase{
		{name: "tcp", serve: func(t *testing.T, d *mbtest.Device) string { return "tcp://" + d.ServeTCP(t) }},
		{name: "udp", serve: func(t *testing.T, d *mbtest.Device) string { return "udp://" + d.ServeUDP(t) }},
		{name: "rtuovertcp", serve: func(t *testing.T, d *mbtest.Device) string { return "rtuovertcp://" + d.ServeRTUOverTCP(t) }},
		{name: "asciiovertcp", serve: func(t *testing.T, d *mbtest.Device) string { return "asciiovertcp://" + d.ServeASCIIOverTCP(t) }},
		{
			name:   "rtu serial",
			serve:  func(t *testing.T, d *mbtest.Device) string { return "rtu://" + d.ServeRTUSerial(t) },
			serial: config.SerialConfig{Baud: 115200, Parity: "even"},
		},
		{
			name:   "ascii serial",
			serve:  func(t *testing.T, d *mbtest.Device) string { return "ascii://" + d.ServeASCIISerial(t) },
			serial: config.SerialConfig{Baud: 115200, DataBits: 8},
		},
	}
}

// Every transport must carry reads and writes of both coils and registers.
func TestTransports_writeThenReadBack(t *testing.T) {
	for _, tc := range transportCases() {
		t.Run(tc.name, func(t *testing.T) {
			dev := mbtest.NewDevice().
				SetHolding(100, 0, 0, 0).
				SetCoils(8, false, false, false, false)
			dc := config.DeviceConfig{SerialConfig: tc.serial, URL: tc.serve(t, dev), Unit: 3}
			ctx := context.Background()

			w, err := CollectWrite(ctx, config.WriteConfig{DeviceConfig: dc, Timeout: 2000, StartAddress: 100, Values: []string{"1234", "0xABCD", "-1"}})
			require.NoError(t, err)
			require.Equal(t, uint8(16), w.Function)
			require.Equal(t, []uint16{1234, 0xABCD, 0xFFFF}, dev.Holding(100, 3))

			_, err = CollectWrite(ctx, config.WriteConfig{DeviceConfig: dc, Timeout: 2000, Function: 15, StartAddress: 8, Values: []string{"1", "0", "on", "true"}})
			require.NoError(t, err)
			require.Equal(t, []bool{true, false, true, true}, dev.Coils(8, 4))

			var samples []*types.PollSample
			sum, err := Poll(ctx, config.PollConfig{DeviceConfig: dc, Timeout: 2000, Function: 3, StartAddress: 100, RegisterCount: 3, Interval: 1, Polls: 2},
				nil, func(s *types.PollSample) error { samples = append(samples, s); return nil })
			require.NoError(t, err)
			require.Equal(t, uint32(2), sum.Succeeded)
			require.Len(t, samples, 2)
			require.Nil(t, samples[1].Error)
			require.Equal(t, []any{uint16(1234), uint16(0xABCD), uint16(0xFFFF)}, samples[1].Values)

			samples = nil
			_, err = Poll(ctx, config.PollConfig{DeviceConfig: dc, Timeout: 2000, Function: 1, StartAddress: 8, RegisterCount: 4, Interval: 1, Polls: 1},
				nil, func(s *types.PollSample) error { samples = append(samples, s); return nil })
			require.NoError(t, err)
			require.Equal(t, []any{true, false, true, true}, samples[0].Values)
		})
	}
}
