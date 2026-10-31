// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"net"
	"testing"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoreCollectDecode_singleValue(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(40, 0x3FC0, 0x0000, 0x9999)
	dc := coreTCP(t, dev)
	dc.Unit = 4

	res, err := CollectDecode(context.Background(), config.DecodeConfig{
		DeviceConfig: dc, Timeout: coreTimeoutMS, Function: 3, StartAddress: 40, Codec: "float32/layout:4321",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, dc.URL, res.Target)
	assert.Equal(t, uint8(4), res.UnitID)
	assert.Equal(t, uint8(3), res.Function)
	assert.Equal(t, uint16(40), res.StartAddress)
	assert.Equal(t, uint16(2), res.RegisterCount, "count 0 reads exactly one codec value")
	assert.Equal(t, "float32/layout:4321", res.Codec)
	assert.Equal(t, "float", res.Family)
	assert.Equal(t, "float32", res.ValueKind)
	assert.Equal(t, "3F C0 00 00", res.RawDataHex)
	assert.Equal(t, []any{float32(1.5)}, res.Values)

	reqs := dev.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, uint8(4), reqs[0].Unit)
	assert.Equal(t, []byte{0x03, 0x00, 0x28, 0x00, 0x02}, reqs[0].PDU)
}

func TestCoreCollectDecode_arrayOfValues(t *testing.T) {
	dev := mbtest.NewDevice().SetInput(0, 0x0000, 0x0001, 0xFFFF, 0xFFFF, 0x0001, 0x0000)
	res, err := CollectDecode(context.Background(), config.DecodeConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 4, RegisterCount: 6, Codec: "int32/layout:4321",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, uint16(6), res.RegisterCount)
	assert.Equal(t, []any{int32(1), int32(-1), int32(65536)}, res.Values)
	assert.Equal(t, "integer", res.Family)
	assert.Equal(t, 1, dev.RequestCount(4))
}

// Registers left over after the last whole value are read but not decoded.
func TestCoreCollectDecode_trailingRegistersIgnored(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 0, 7, 0xDEAD)
	res, err := CollectDecode(context.Background(), config.DecodeConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, RegisterCount: 3, Codec: "uint32/layout:4321",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, []any{uint32(7)}, res.Values)
	assert.Equal(t, "00 00 00 07 DE AD", res.RawDataHex)
}

func TestCoreCollectDecode_failures(t *testing.T) {
	t.Run("unknown codec", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		_, err := CollectDecode(context.Background(), config.DecodeConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Codec: "no-such-codec",
		}, nil)
		e, ok := errs.As(err)
		require.True(t, ok)
		assert.Equal(t, errs.KindInvalidInput, e.Kind)
		assert.Contains(t, e.Message, `unknown codec "no-such-codec"`)
		assert.Zero(t, dev.RequestCount(0), "the codec is resolved before connecting")
	})

	t.Run("connect refused", func(t *testing.T) {
		_, err := CollectDecode(context.Background(), config.DecodeConfig{
			DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t), Unit: 1}, Timeout: coreShortTimeoutMS, Function: 3, Codec: "uint16/layout:12",
		}, nil)
		require.ErrorIs(t, err, ErrTCPConnection)
	})

	t.Run("modbus exception", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		_, err := CollectDecode(context.Background(), config.DecodeConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Codec: "uint32/layout:4321",
		}, nil)
		require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress)
		coreRequireKind(t, err, errs.KindModbus, errs.CodeModbusException)
	})

	t.Run("unsupported function code", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		_, err := CollectDecode(context.Background(), config.DecodeConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 9, Codec: "uint16/layout:12",
		}, nil)
		coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	})

	// Month 13 is not a date: the registers are read but cannot be decoded.
	t.Run("registers the codec rejects", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF)
		_, err := CollectDecode(context.Background(), config.DecodeConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Codec: "datetime_iec870",
		}, nil)
		e, ok := errs.As(err)
		require.True(t, ok, "error: %v", err)
		assert.Equal(t, errs.KindInvalidInput, e.Kind)
		assert.Contains(t, e.Message, `decode with codec "datetime_iec870" failed`)
		assert.Equal(t, 1, dev.RequestCount(3))
	})
}

func TestCoreNormalizeDecoded(t *testing.T) {
	mac := net.HardwareAddr{0x00, 0x1A, 0x2B, 0x3C, 0x4D, 0x5E}
	assert.Equal(t, "00:1a:2b:3c:4d:5e", normalizeDecoded(mac))
	assert.Equal(t, "0aff", normalizeDecoded([]byte{0x0A, 0xFF}))
	assert.Equal(t, uint16(3), normalizeDecoded(uint16(3)))
	ip := net.ParseIP("10.0.0.1")
	assert.Equal(t, ip, normalizeDecoded(ip), "an IP is left as is")
}

func TestCoreBytesToRegisters(t *testing.T) {
	assert.Empty(t, bytesToRegisters(nil))
	assert.Equal(t, []uint16{0x0102, 0x0304}, bytesToRegisters([]byte{1, 2, 3, 4}))
	assert.Equal(t, []uint16{0x0102}, bytesToRegisters([]byte{1, 2, 3}), "an odd trailing byte is dropped")
}
