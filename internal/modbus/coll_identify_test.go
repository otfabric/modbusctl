// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"fmt"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collIdentifyCfg(url, unit string) config.IdentifyConfig {
	return config.IdentifyConfig{UnitClientConfig: config.UnitClientConfig{URL: url, UnitID: unit, Timeout: 2000}}
}

func collObjectIDs(rows []types.IdentifyObjectRow) []uint8 {
	ids := make([]uint8, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestCollIdentify_allObjectsWithServerID(t *testing.T) {
	dev := collIdentDevice()
	cfg := collIdentifyCfg("tcp://"+dev.ServeTCP(t), "7")
	cfg.ServerID = true

	res, err := CollectDeviceIdentification(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, cfg.URL, res.Target)
	require.Len(t, res.Units, 1)
	u := res.Units[0]
	require.Equal(t, uint8(7), u.UnitID)
	require.Nil(t, u.Error)

	require.NotNil(t, u.Category)
	require.Equal(t, uint8(0x03), *u.Category, "ReadAll asks for the extended category")
	require.NotNil(t, u.ConformityLevel)
	require.Equal(t, uint8(0x83), *u.ConformityLevel)
	require.NotNil(t, u.MoreFollows)
	require.False(t, *u.MoreFollows)
	require.NotNil(t, u.NextObjectID)
	require.Equal(t, uint8(0), *u.NextObjectID)

	require.Equal(t, []uint8{0x00, 0x01, 0x02, 0x03, 0x04, 0x80}, collObjectIDs(u.Objects))
	require.Equal(t, "OTFabric", u.Objects[0].Value)
	require.Equal(t, "MB-1", u.Objects[1].Value)
	require.Equal(t, "1.2", u.Objects[2].Value)
	require.Equal(t, "private", u.Objects[5].Value)
	for _, o := range u.Objects {
		require.NotEmptyf(t, o.Description, "object 0x%02X has no description", o.ID)
	}

	require.NotNil(t, u.ReportServerID)
	require.Nil(t, u.ReportServerID.Error)
	require.Equal(t, "42 FF", u.ReportServerID.DataHex)
	require.NotNil(t, u.ReportServerID.RunIndicatorOn)
	require.True(t, *u.ReportServerID.RunIndicatorOn)

	require.NotNil(t, res.Summary)
	require.Equal(t, 1, res.Summary.Requested)
	require.Equal(t, 1, res.Summary.Succeeded)
	require.Equal(t, 0, res.Summary.Failed)
	require.False(t, res.HasPartialFailure())

	require.Equal(t, 1, dev.RequestCount(0x2B))
	require.Equal(t, 1, dev.RequestCount(0x11))
}

func TestCollIdentify_withoutServerIDFlagSkipsFC17(t *testing.T) {
	dev := collIdentDevice()
	res, err := CollectDeviceIdentification(context.Background(), collIdentifyCfg("tcp://"+dev.ServeTCP(t), "1"))
	require.NoError(t, err)
	require.Nil(t, res.Units[0].ReportServerID)
	require.Zero(t, dev.RequestCount(0x11))
}

func TestCollIdentify_categories(t *testing.T) {
	tests := []struct {
		name                     string
		basic, regular, extended bool
		wantIDs                  []uint8
		wantCategory             uint8
		wantRequests             int
	}{
		{name: "basic", basic: true, wantIDs: []uint8{0, 1, 2}, wantCategory: 1, wantRequests: 1},
		{name: "regular", regular: true, wantIDs: []uint8{0, 1, 2, 3, 4}, wantCategory: 2, wantRequests: 1},
		{name: "extended", extended: true, wantIDs: []uint8{0, 1, 2, 3, 4, 0x80}, wantCategory: 3, wantRequests: 1},
		{name: "basic+regular merges without duplicates", basic: true, regular: true, wantIDs: []uint8{0, 1, 2, 3, 4}, wantCategory: 1, wantRequests: 2},
		{name: "all three", basic: true, regular: true, extended: true, wantIDs: []uint8{0, 1, 2, 3, 4, 0x80}, wantCategory: 1, wantRequests: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dev := collIdentDevice()
			cfg := collIdentifyCfg("tcp://"+dev.ServeTCP(t), "1")
			cfg.Basic, cfg.Regular, cfg.Extended = tc.basic, tc.regular, tc.extended
			cfg.ServerID = true

			res, err := CollectDeviceIdentification(context.Background(), cfg)
			require.NoError(t, err)
			require.Len(t, res.Units, 1)
			u := res.Units[0]
			require.Nil(t, u.Error)
			require.Equal(t, tc.wantIDs, collObjectIDs(u.Objects))
			require.NotNil(t, u.Category)
			require.Equal(t, tc.wantCategory, *u.Category, "header comes from the first category answered")
			require.NotNil(t, u.ConformityLevel)
			require.Equal(t, uint8(0x83), *u.ConformityLevel)
			require.NotNil(t, u.MoreFollows)
			require.NotNil(t, u.NextObjectID)
			require.Equal(t, "OTFabric", u.Objects[0].Value)
			for _, o := range u.Objects {
				require.NotEmpty(t, o.Description)
			}
			require.NotNil(t, u.ReportServerID)
			require.Equal(t, "42 FF", u.ReportServerID.DataHex)
			require.Equal(t, tc.wantRequests, dev.RequestCount(0x2B))
		})
	}
}

func TestCollIdentify_categoryExceptionStopsUnit(t *testing.T) {
	dev := collIdentDevice()
	dev.OnRequest = func(req mbtest.Request) mbtest.Action {
		// Fail the regular category only (PDU: FC, MEI, category, object).
		if req.PDU[0] == 0x2B && req.PDU[2] == 0x02 {
			return mbtest.Action{Exception: mbtest.ExServerDeviceFailure}
		}
		return mbtest.Action{}
	}
	cfg := collIdentifyCfg("tcp://"+dev.ServeTCP(t), "1")
	cfg.Basic, cfg.Regular, cfg.Extended, cfg.ServerID = true, true, true, true

	res, err := CollectDeviceIdentification(context.Background(), cfg)
	require.NoError(t, err)
	u := res.Units[0]
	collRequireInfo(t, u.Error, errs.KindModbus, errs.CodeModbusException)
	require.Empty(t, u.Objects)
	require.Nil(t, u.ReportServerID)
	require.Equal(t, 2, dev.RequestCount(0x2B), "extended must not be requested after regular failed")
	require.Equal(t, 1, res.Summary.Failed)
	require.True(t, res.HasPartialFailure())
}

func TestCollIdentify_fc43Unsupported(t *testing.T) {
	for _, categories := range []bool{false, true} {
		dev := mbtest.NewDevice()
		cfg := collIdentifyCfg("tcp://"+dev.ServeTCP(t), "1")
		cfg.Basic = categories

		res, err := CollectDeviceIdentification(context.Background(), cfg)
		require.NoError(t, err)
		require.Len(t, res.Units, 1)
		collRequireInfo(t, res.Units[0].Error, errs.KindModbus, errs.CodeModbusException)
		require.Nil(t, res.Units[0].Category)
		require.Equal(t, 1, res.Summary.Requested)
		require.Equal(t, 0, res.Summary.Succeeded)
		require.Equal(t, 1, res.Summary.Failed)
	}
}

func TestCollIdentify_serverIDUnsupportedKeepsObjects(t *testing.T) {
	for _, categories := range []bool{false, true} {
		dev := collIdentDevice()
		dev.ServerID = nil
		cfg := collIdentifyCfg("tcp://"+dev.ServeTCP(t), "1")
		cfg.ServerID = true
		cfg.Basic = categories

		res, err := CollectDeviceIdentification(context.Background(), cfg)
		require.NoError(t, err)
		u := res.Units[0]
		require.Nil(t, u.Error)
		require.NotEmpty(t, u.Objects)
		require.NotNil(t, u.ReportServerID)
		collRequireInfo(t, u.ReportServerID.Error, errs.KindModbus, errs.CodeModbusException)
		require.Empty(t, u.ReportServerID.DataHex)
		require.Nil(t, u.ReportServerID.RunIndicatorOn)
		require.Equal(t, 0, res.Summary.Failed, "an FC17 failure does not fail the unit")
	}
}

func TestCollIdentify_serverIDSingleByteHasNoRunIndicator(t *testing.T) {
	dev := collIdentDevice()
	dev.OnRequest = nil
	dev.ServerID = &mbtest.ServerID{Running: false}
	cfg := collIdentifyCfg("tcp://"+dev.ServeTCP(t), "1")
	cfg.ServerID = true

	res, err := CollectDeviceIdentification(context.Background(), cfg)
	require.NoError(t, err)
	rs := res.Units[0].ReportServerID
	require.NotNil(t, rs)
	require.Nil(t, rs.Error)
	require.Equal(t, "00", rs.DataHex)
	require.Nil(t, rs.RunIndicatorOn, "one data byte cannot carry both an ID and a run indicator")
}

// Sequential and parallel sweeps must return the same per-unit rows, sorted by unit ID.
func TestCollIdentify_multipleUnits(t *testing.T) {
	for _, parallel := range []uint16{0, 1, 2, 4, 64} {
		t.Run(fmt.Sprintf("parallel %d", parallel), func(t *testing.T) {
			dev := collIdentDevice()
			dev.Units = []uint8{1, 3, 4, 6}
			cfg := collIdentifyCfg("tcp://"+dev.ServeTCP(t), "6,1-4")
			cfg.Parallel = parallel
			cfg.ServerID = true

			res, err := CollectDeviceIdentification(context.Background(), cfg)
			require.NoError(t, err)
			require.Len(t, res.Units, 5)
			var got []uint8
			for _, u := range res.Units {
				got = append(got, u.UnitID)
				if u.UnitID == 2 {
					collRequireInfo(t, u.Error, errs.KindModbus, errs.CodeModbusException)
					assert.Empty(t, u.Objects)
					assert.Nil(t, u.ReportServerID)
					continue
				}
				require.Nilf(t, u.Error, "unit %d", u.UnitID)
				assert.Equal(t, []uint8{0, 1, 2, 3, 4, 0x80}, collObjectIDs(u.Objects))
				require.NotNil(t, u.ReportServerID)
				assert.Equal(t, "42 FF", u.ReportServerID.DataHex)
			}
			require.Equal(t, []uint8{1, 2, 3, 4, 6}, got)
			require.Equal(t, 5, res.Summary.Requested)
			require.Equal(t, 4, res.Summary.Succeeded)
			require.Equal(t, 1, res.Summary.Failed)
			require.True(t, res.HasPartialFailure())
			require.Equal(t, 5, dev.RequestCount(0x2B))
		})
	}
}

// A serial line must be opened once even when --parallel asks for more.
func TestCollIdentify_serialIgnoresParallel(t *testing.T) {
	dev := collIdentDevice()
	cfg := collIdentifyCfg("rtu://"+dev.ServeRTUSerial(t), "1-3")
	cfg.SerialConfig = config.SerialConfig{Baud: 115200, Parity: "even"}
	cfg.Parallel = 8

	res, err := CollectDeviceIdentification(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, res.Units, 3)
	for i, u := range res.Units {
		require.Equal(t, uint8(i+1), u.UnitID)
		require.Nil(t, u.Error)
		require.Equal(t, "OTFabric", u.Objects[0].Value)
	}
	require.Equal(t, 3, res.Summary.Succeeded)
}

func TestCollIdentify_timeoutIsPerUnit(t *testing.T) {
	for _, parallel := range []uint16{1, 2} {
		dev := collIdentDevice()
		dev.OnRequest = func(req mbtest.Request) mbtest.Action {
			return mbtest.Action{Drop: req.Unit == 2}
		}
		cfg := collIdentifyCfg("tcp://"+dev.ServeTCP(t), "1-3")
		cfg.Timeout = 200
		cfg.Parallel = parallel

		res, err := CollectDeviceIdentification(context.Background(), cfg)
		require.NoError(t, err)
		require.Len(t, res.Units, 3)
		require.Nil(t, res.Units[0].Error)
		collRequireInfo(t, res.Units[1].Error, errs.KindTimeout, errs.CodeRequestTimeout)
		require.Nil(t, res.Units[2].Error, "a timeout on unit 2 must not poison unit 3")
		require.Equal(t, 1, res.Summary.Failed)
	}
}

func TestCollIdentify_setupErrors(t *testing.T) {
	closed := "tcp://" + collClosedAddr(t)
	tests := []struct {
		name     string
		cfg      config.IdentifyConfig
		parallel uint16
		check    func(t *testing.T, err error)
	}{
		{
			name: "invalid unit selector",
			cfg:  collIdentifyCfg(closed, "1,,2"),
			check: func(t *testing.T, err error) {
				collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidUnitSelector)
			},
		},
		{
			name: "empty unit selector",
			cfg:  collIdentifyCfg(closed, ""),
			check: func(t *testing.T, err error) {
				collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidUnitSelector)
			},
		},
		{
			name: "no target",
			cfg:  collIdentifyCfg("", "1"),
			check: func(t *testing.T, err error) {
				collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
			},
		},
		{
			name:     "unknown scheme, parallel",
			cfg:      collIdentifyCfg("bogus://127.0.0.1:1", "1-4"),
			parallel: 4,
			check: func(t *testing.T, err error) {
				collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
			},
		},
		{
			name: "closed port",
			cfg:  collIdentifyCfg(closed, "1"),
			check: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrTCPConnection)
				collRequireErr(t, WrapCollectError(err), errs.KindTransport, errs.CodeTransportConnectFailed)
			},
		},
		{
			name:     "closed port, parallel",
			cfg:      collIdentifyCfg(closed, "1-4"),
			parallel: 4,
			check: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrTCPConnection)
				collRequireErr(t, WrapCollectError(err), errs.KindTransport, errs.CodeTransportConnectFailed)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.Parallel = tc.parallel
			res, err := CollectDeviceIdentification(context.Background(), tc.cfg)
			require.Nil(t, res)
			tc.check(t, err)
		})
	}
}

func TestCollIdentify_ipPortTarget(t *testing.T) {
	dev := collIdentDevice()
	addr := dev.ServeTCP(t)
	cfg := config.IdentifyConfig{UnitClientConfig: config.UnitClientConfig{IP: "127.0.0.1", Port: collPort(t, addr), UnitID: "1", Timeout: 2000}}
	res, err := CollectDeviceIdentification(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, "tcp://"+addr, res.Target)
	require.Nil(t, res.Units[0].Error)
}

func TestCollClientPool(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 0xBEEF)
	conf, err := buildDeviceClientConfig("tcp://"+dev.ServeTCP(t), 1000, false, config.SerialConfig{}, config.TLSConfig{})
	require.NoError(t, err)

	t.Run("opens n independent clients", func(t *testing.T) {
		clients, cleanup, err := openModbusClientPool(3, conf)
		require.NoError(t, err)
		require.Len(t, clients, 3)
		for _, c := range clients {
			v, rerr := c.ReadHoldingRegister(context.Background(), 1, 0)
			require.NoError(t, rerr)
			require.Equal(t, uint16(0xBEEF), v)
		}
		cleanup()
		_, rerr := clients[0].ReadHoldingRegister(context.Background(), 1, 0)
		require.Error(t, rerr, "cleanup must close the clients")
	})

	t.Run("n below one opens a single client", func(t *testing.T) {
		clients, cleanup, err := openModbusClientPool(0, conf)
		require.NoError(t, err)
		defer cleanup()
		require.Len(t, clients, 1)
	})

	t.Run("invalid config", func(t *testing.T) {
		bad := conf
		bad.URL = "nope"
		clients, cleanup, err := openModbusClientPool(2, bad)
		require.Nil(t, clients)
		require.Nil(t, cleanup)
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	})

	t.Run("connect failure", func(t *testing.T) {
		bad := conf
		bad.URL = "tcp://" + collClosedAddr(t)
		clients, cleanup, err := openModbusClientPool(2, bad)
		require.Nil(t, clients)
		require.Nil(t, cleanup)
		require.ErrorIs(t, err, ErrTCPConnection)
	})

	t.Run("closeModbusClients tolerates nil entries", func(t *testing.T) {
		clients, _, err := openModbusClientPool(1, conf)
		require.NoError(t, err)
		require.NotPanics(t, func() { closeModbusClients(append(clients, nil)) })
	})
}
