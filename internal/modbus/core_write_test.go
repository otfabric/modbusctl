// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coreRepeat returns n copies of v.
func coreRepeat(v string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestCorePlanWrite_valid(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.WriteConfig
		wantFC    uint8
		wantCoils []bool
		wantRegs  []uint16
	}{
		{
			name:   "auto function, one register",
			cfg:    config.WriteConfig{Values: []string{" 0x04D2 "}},
			wantFC: 6, wantRegs: []uint16{1234},
		},
		{
			name:   "auto function, several registers",
			cfg:    config.WriteConfig{Values: []string{"1", "0b11", "0o17", "-32768", "65535"}},
			wantFC: 16, wantRegs: []uint16{1, 3, 15, 0x8000, 0xFFFF},
		},
		{
			name:   "blank values are ignored",
			cfg:    config.WriteConfig{Values: []string{"", "  ", "9"}},
			wantFC: 6, wantRegs: []uint16{9},
		},
		{
			name:   "fc6",
			cfg:    config.WriteConfig{Function: 6, Values: []string{"-1"}},
			wantFC: 6, wantRegs: []uint16{0xFFFF},
		},
		{
			name:   "fc16 with a single register stays fc16",
			cfg:    config.WriteConfig{Function: 16, Values: []string{"7"}},
			wantFC: 16, wantRegs: []uint16{7},
		},
		{
			name:   "fc16 at the protocol limit",
			cfg:    config.WriteConfig{Function: 16, Values: coreRepeat("1", MaxWriteRegisters)},
			wantFC: 16, wantRegs: bytesToRegisters([]byte(strings.Repeat("\x00\x01", MaxWriteRegisters))),
		},
		{
			name:   "fc5",
			cfg:    config.WriteConfig{Function: 5, Values: []string{"ON"}},
			wantFC: 5, wantCoils: []bool{true},
		},
		{
			name:   "fc15 with every spelling",
			cfg:    config.WriteConfig{Function: 15, Values: []string{"1", "0", "true", "false", "on", "off", "TRUE", " Off "}},
			wantFC: 15, wantCoils: []bool{true, false, true, false, true, false, true, false},
		},
		{
			name:   "last addressable register",
			cfg:    config.WriteConfig{StartAddress: 65535, Values: []string{"1"}},
			wantFC: 6, wantRegs: []uint16{1},
		},
		{
			name:   "codec, auto function picks fc16 for a two-register value",
			cfg:    config.WriteConfig{Codec: " uint32/layout:4321 ", Values: []string{"0x01020304"}},
			wantFC: 16, wantRegs: []uint16{0x0102, 0x0304},
		},
		{
			name:   "codec, auto function picks fc6 for a one-register value",
			cfg:    config.WriteConfig{Codec: "int16/layout:21", Values: []string{"-2"}},
			wantFC: 6, wantRegs: []uint16{0xFFFE},
		},
		{
			name:   "codec, several values are concatenated",
			cfg:    config.WriteConfig{Function: 16, Codec: "uint32/layout:4321", Values: []string{"1", "2"}},
			wantFC: 16, wantRegs: []uint16{0x0000, 0x0001, 0x0000, 0x0002},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanWrite(tc.cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFC, plan.Function)
			assert.Equal(t, tc.wantCoils, plan.Coils)
			assert.Equal(t, tc.wantRegs, plan.Registers)
			assert.Equal(t, len(tc.wantCoils)+len(tc.wantRegs), plan.Count())
		})
	}
}

func TestCorePlanWrite_invalid(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.WriteConfig
		wantErr string
	}{
		{name: "no values", cfg: config.WriteConfig{}, wantErr: "at least one value is required"},
		{name: "only blank values", cfg: config.WriteConfig{Values: []string{" ", ""}}, wantErr: "at least one value is required"},
		{name: "read function code", cfg: config.WriteConfig{Function: 3, Values: []string{"1"}}, wantErr: "unsupported function code for write: 3"},
		{name: "codec with fc5", cfg: config.WriteConfig{Function: 5, Codec: "uint16/layout:12", Values: []string{"1"}}, wantErr: "cannot be used with coil function code 5"},
		{name: "codec with fc15", cfg: config.WriteConfig{Function: 15, Codec: "uint16/layout:12", Values: []string{"1"}}, wantErr: "cannot be used with coil function code 15"},
		{name: "bad coil value", cfg: config.WriteConfig{Function: 15, Values: []string{"1", "yes"}}, wantErr: `invalid coil value "yes"`},
		{name: "fc5 with two values", cfg: config.WriteConfig{Function: 5, Values: []string{"1", "0"}}, wantErr: "function code 5 writes exactly one coil, got 2"},
		{name: "fc15 above the limit", cfg: config.WriteConfig{Function: 15, Values: coreRepeat("1", MaxWriteCoils+1)}, wantErr: "at most 1968 coils per request, got 1969"},
		{name: "register is not a number", cfg: config.WriteConfig{Values: []string{"1.5"}}, wantErr: `invalid register value "1.5"`},
		{name: "register too large", cfg: config.WriteConfig{Values: []string{"65536"}}, wantErr: "register value 65536 is out of range"},
		{name: "register too small", cfg: config.WriteConfig{Values: []string{"-32769"}}, wantErr: "register value -32769 is out of range"},
		{name: "fc6 with two values", cfg: config.WriteConfig{Function: 6, Values: []string{"1", "2"}}, wantErr: "function code 6 writes exactly one register, got 2"},
		{name: "fc6 with a two-register codec", cfg: config.WriteConfig{Function: 6, Codec: "float32/layout:4321", Values: []string{"1"}}, wantErr: "function code 6 writes exactly one register, got 2"},
		{name: "fc16 above the limit", cfg: config.WriteConfig{Function: 16, Values: coreRepeat("1", MaxWriteRegisters+1)}, wantErr: "at most 123 registers per request, got 124"},
		{name: "auto function above the limit", cfg: config.WriteConfig{Values: coreRepeat("1", MaxWriteRegisters+1)}, wantErr: "at most 123 registers per request, got 124"},
		{name: "registers past the last address", cfg: config.WriteConfig{StartAddress: 65535, Values: []string{"1", "2"}}, wantErr: "start address 65535 with 2 value(s) exceeds the maximum address 65535"},
		{name: "coils past the last address", cfg: config.WriteConfig{Function: 15, StartAddress: 65530, Values: coreRepeat("0", 7)}, wantErr: "exceeds the maximum address 65535"},
		{name: "unknown codec", cfg: config.WriteConfig{Codec: "nope", Values: []string{"1"}}, wantErr: `unknown codec "nope"`},
		{name: "value not valid for codec", cfg: config.WriteConfig{Codec: "uint16/layout:12", Values: []string{"-1"}}, wantErr: `value "-1" is not valid for codec "uint16/layout:12"`},
		{name: "value out of codec range", cfg: config.WriteConfig{Codec: "int16/layout:12", Values: []string{"40000"}}, wantErr: `value "40000" is not valid for codec "int16/layout:12"`},
		{name: "value the codec cannot encode", cfg: config.WriteConfig{Codec: "ip_addr", Values: []string{"2001:db8::1"}}, wantErr: `value "2001:db8::1" could not be encoded with codec "ip_addr"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanWrite(tc.cfg)
			require.Error(t, err)
			assert.Nil(t, plan)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestCoreParseCodecValue(t *testing.T) {
	mac, err := net.ParseMAC("00:1a:2b:3c:4d:5e")
	require.NoError(t, err)
	tests := []struct {
		kind codec.CodecValueKind
		in   string
		want any
	}{
		{kind: codec.CodecValueUint16, in: "0xFFFF", want: uint16(65535)},
		{kind: codec.CodecValueInt16, in: "-32768", want: int16(-32768)},
		{kind: codec.CodecValueUint32, in: "4294967295", want: uint32(4294967295)},
		{kind: codec.CodecValueInt32, in: "-2147483648", want: int32(-2147483648)},
		{kind: codec.CodecValueUint48, in: "281474976710655", want: uint64(281474976710655)},
		{kind: codec.CodecValueUint64, in: "18446744073709551615", want: uint64(18446744073709551615)},
		{kind: codec.CodecValueInt48, in: "-140737488355328", want: int64(-140737488355328)},
		{kind: codec.CodecValueInt64, in: "-9223372036854775808", want: int64(-9223372036854775808)},
		{kind: codec.CodecValueFloat32, in: "1.5", want: float32(1.5)},
		{kind: codec.CodecValueFloat64, in: "-2.25e3", want: float64(-2250)},
		{kind: codec.CodecValueString, in: "hello world", want: "hello world"},
		{kind: codec.CodecValueByteSlice, in: "0x0A1B2C", want: []byte{0x0A, 0x1B, 0x2C}},
		{kind: codec.CodecValueByteSlice, in: "0a:1b-2c 3d", want: []byte{0x0A, 0x1B, 0x2C, 0x3D}},
		{kind: codec.CodecValueUint8Slice, in: "FF00", want: []byte{0xFF, 0x00}},
		{kind: codec.CodecValueIP, in: "192.168.1.10", want: net.ParseIP("192.168.1.10")},
		{kind: codec.CodecValueIP, in: "2001:db8::1", want: net.ParseIP("2001:db8::1")},
		{kind: codec.CodecValueHardwareAddr, in: "00:1a:2b:3c:4d:5e", want: mac},
		{kind: codec.CodecValueTime, in: "2026-01-31T12:00:00Z", want: time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.kind.String()+" "+tc.in, func(t *testing.T) {
			got, err := parseCodecValue(tc.kind, tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	invalid := []struct {
		kind    codec.CodecValueKind
		in      string
		wantErr string
	}{
		{kind: codec.CodecValueUint16, in: "65536", wantErr: "out of range"},
		{kind: codec.CodecValueInt16, in: "x", wantErr: "invalid syntax"},
		{kind: codec.CodecValueUint32, in: "-1", wantErr: "invalid syntax"},
		{kind: codec.CodecValueInt32, in: "2147483648", wantErr: "out of range"},
		{kind: codec.CodecValueUint64, in: "1.0", wantErr: "invalid syntax"},
		{kind: codec.CodecValueInt64, in: "", wantErr: "invalid syntax"},
		{kind: codec.CodecValueFloat32, in: "1e99", wantErr: "out of range"},
		{kind: codec.CodecValueFloat64, in: "abc", wantErr: "invalid syntax"},
		{kind: codec.CodecValueByteSlice, in: "0xZZ", wantErr: "expected hex bytes such as 0A1B2C"},
		{kind: codec.CodecValueUint8Slice, in: "ABC", wantErr: "expected hex bytes such as 0A1B2C"},
		{kind: codec.CodecValueIP, in: "300.1.1.1", wantErr: "expected an IP address"},
		{kind: codec.CodecValueHardwareAddr, in: "00:1a", wantErr: "invalid MAC address"},
		{kind: codec.CodecValueTime, in: "2026-01-31", wantErr: "expected an RFC 3339 time"},
		{kind: codec.CodecValueUnknown, in: "1", wantErr: "values of kind unknown cannot be written"},
	}
	for _, tc := range invalid {
		t.Run("invalid "+tc.kind.String()+" "+tc.in, func(t *testing.T) {
			_, err := parseCodecValue(tc.kind, tc.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// Every value kind must survive a write through its codec followed by a decode of the same
// registers.
func TestCoreWriteDecode_roundTrip(t *testing.T) {
	tests := []struct {
		codec    string
		in       string
		wantRegs []uint16
		want     any
	}{
		{codec: "uint16/layout:21", in: "513", wantRegs: []uint16{0x0201}, want: uint16(513)},
		{codec: "uint16/layout:12", in: "513", wantRegs: []uint16{0x0102}, want: uint16(513)},
		{codec: "int16/layout:21", in: "-5", wantRegs: []uint16{0xFFFB}, want: int16(-5)},
		{codec: "uint32/layout:4321", in: "0xDEADBEEF", wantRegs: []uint16{0xDEAD, 0xBEEF}, want: uint32(0xDEADBEEF)},
		{codec: "uint32/layout:2143", in: "0xDEADBEEF", wantRegs: []uint16{0xBEEF, 0xDEAD}, want: uint32(0xDEADBEEF)},
		{codec: "uint32/layout:1234", in: "0xDEADBEEF", wantRegs: []uint16{0xEFBE, 0xADDE}, want: uint32(0xDEADBEEF)},
		{codec: "int32/layout:4321", in: "-2", wantRegs: []uint16{0xFFFF, 0xFFFE}, want: int32(-2)},
		{codec: "uint48/layout:654321", in: "0x010203040506", wantRegs: []uint16{0x0102, 0x0304, 0x0506}, want: uint64(0x010203040506)},
		{codec: "int48/layout:654321", in: "-2", wantRegs: []uint16{0xFFFF, 0xFFFF, 0xFFFE}, want: int64(-2)},
		{codec: "uint64/layout:87654321", in: "0x0102030405060708", wantRegs: []uint16{0x0102, 0x0304, 0x0506, 0x0708}, want: uint64(0x0102030405060708)},
		{codec: "int64/layout:12345678", in: "-2", wantRegs: []uint16{0xFEFF, 0xFFFF, 0xFFFF, 0xFFFF}, want: int64(-2)},
		{codec: "float32/layout:4321", in: "1.5", wantRegs: []uint16{0x3FC0, 0x0000}, want: float32(1.5)},
		{codec: "float32/layout:2143", in: "1.5", wantRegs: []uint16{0x0000, 0x3FC0}, want: float32(1.5)},
		{codec: "float64/layout:87654321", in: "-2.5", wantRegs: []uint16{0xC004, 0x0000, 0x0000, 0x0000}, want: float64(-2.5)},
		{codec: "ascii/registers:2", in: "ABCD", wantRegs: []uint16{0x4142, 0x4344}, want: "ABCD"},
		{codec: "bytes/bytes:10", in: "00:01:02:03:04:05:06:07:08:09", wantRegs: []uint16{0x0001, 0x0203, 0x0405, 0x0607, 0x0809}, want: "00010203040506070809"},
		{codec: "uint8_slice/bytes:10", in: "0xFFEEDDCCBBAA99887766", wantRegs: []uint16{0xFFEE, 0xDDCC, 0xBBAA, 0x9988, 0x7766}, want: "ffeeddccbbaa99887766"},
		{codec: "ip_addr", in: "192.168.1.10", wantRegs: []uint16{0xC0A8, 0x010A}, want: net.ParseIP("192.168.1.10").To4()},
		{codec: "eui48", in: "00:1a:2b:3c:4d:5e", wantRegs: []uint16{0x001A, 0x2B3C, 0x4D5E}, want: "00:1a:2b:3c:4d:5e"},
	}
	for _, tc := range tests {
		t.Run(tc.codec, func(t *testing.T) {
			n := uint16(len(tc.wantRegs))
			dev := mbtest.NewDevice().SetHolding(1000, make([]uint16, n)...)
			dc := coreTCP(t, dev)

			w, err := CollectWrite(context.Background(), config.WriteConfig{
				DeviceConfig: dc, Timeout: coreTimeoutMS, StartAddress: 1000, Codec: tc.codec, Values: []string{tc.in},
			})
			require.NoError(t, err)
			assert.Equal(t, tc.wantRegs, w.Registers)
			assert.Equal(t, tc.wantRegs, dev.Holding(1000, n))
			assert.Equal(t, tc.codec, w.Codec)
			assert.Equal(t, int(n), w.Count)

			d, err := CollectDecode(context.Background(), config.DecodeConfig{
				DeviceConfig: dc, Timeout: coreTimeoutMS, Function: 3, StartAddress: 1000, Codec: tc.codec,
			}, nil)
			require.NoError(t, err)
			require.Len(t, d.Values, 1)
			if ip, isIP := tc.want.(net.IP); isIP {
				got, ok := d.Values[0].(net.IP)
				require.True(t, ok, "decoded %T", d.Values[0])
				assert.True(t, ip.Equal(got), "got %v", got)
			} else {
				assert.Equal(t, tc.want, d.Values[0])
			}
			assert.Equal(t, w.RawDataHex, d.RawDataHex)
			assert.Equal(t, n, d.RegisterCount)
		})
	}
}

// A time written through a codec reads back as the same instant.
func TestCoreWriteDecode_roundTripTime(t *testing.T) {
	want := time.Date(2026, 1, 31, 12, 30, 45, 0, time.UTC)
	for _, id := range []string{"datetime2_s2000", "datetime_iec870"} {
		t.Run(id, func(t *testing.T) {
			rc, ok, err := codec.RuntimeCodecByID(id)
			require.NoError(t, err)
			require.True(t, ok)
			n := rc.RegisterSpec().Count
			dev := mbtest.NewDevice().SetHolding(0, make([]uint16, n)...)
			dc := coreTCP(t, dev)

			_, err = CollectWrite(context.Background(), config.WriteConfig{
				DeviceConfig: dc, Timeout: coreTimeoutMS, Codec: id, Values: []string{want.Format(time.RFC3339)},
			})
			require.NoError(t, err)

			d, err := CollectDecode(context.Background(), config.DecodeConfig{
				DeviceConfig: dc, Timeout: coreTimeoutMS, Function: 3, Codec: id,
			}, nil)
			require.NoError(t, err)
			require.Len(t, d.Values, 1)
			got, isTime := d.Values[0].(time.Time)
			require.True(t, isTime, "decoded %T", d.Values[0])
			assert.True(t, want.Equal(got), "got %v", got)
			assert.Equal(t, "time", d.ValueKind)
		})
	}
}

func TestCoreCollectWrite_requestsAndResult(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.WriteConfig
		wantFC    uint8
		wantPDU   []byte
		wantHex   string
		wantRegs  []uint16
		wantCoils []bool
	}{
		{
			name:   "single register",
			cfg:    config.WriteConfig{StartAddress: 2, Values: []string{"0x1234"}},
			wantFC: 6, wantPDU: []byte{0x06, 0x00, 0x02, 0x12, 0x34}, wantHex: "12 34", wantRegs: []uint16{0x1234},
		},
		{
			name:   "multiple registers",
			cfg:    config.WriteConfig{StartAddress: 1, Values: []string{"1", "2"}},
			wantFC: 16, wantPDU: []byte{0x10, 0x00, 0x01, 0x00, 0x02, 0x04, 0x00, 0x01, 0x00, 0x02}, wantHex: "00 01 00 02", wantRegs: []uint16{1, 2},
		},
		{
			name:   "single coil",
			cfg:    config.WriteConfig{Function: 5, StartAddress: 3, Values: []string{"on"}},
			wantFC: 5, wantPDU: []byte{0x05, 0x00, 0x03, 0xFF, 0x00}, wantCoils: []bool{true},
		},
		{
			name:   "multiple coils",
			cfg:    config.WriteConfig{Function: 15, StartAddress: 0, Values: []string{"1", "0", "1"}},
			wantFC: 15, wantPDU: []byte{0x0F, 0x00, 0x00, 0x00, 0x03, 0x01, 0x05}, wantCoils: []bool{true, false, true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dev := mbtest.NewDevice().SetHolding(0, 0, 0, 0, 0).SetCoils(0, false, false, false, false)
			cfg := tc.cfg
			cfg.DeviceConfig = coreTCP(t, dev)
			cfg.Unit = 17
			cfg.Timeout = coreTimeoutMS

			res, err := CollectWrite(context.Background(), cfg)
			require.NoError(t, err)
			assert.Equal(t, cfg.URL, res.Target)
			assert.Equal(t, uint8(17), res.UnitID)
			assert.Equal(t, tc.wantFC, res.Function)
			assert.Equal(t, cfg.StartAddress, res.StartAddress)
			assert.Equal(t, len(tc.wantRegs)+len(tc.wantCoils), res.Count)
			assert.Equal(t, tc.wantRegs, res.Registers)
			assert.Equal(t, tc.wantCoils, res.Coils)
			assert.Equal(t, tc.wantHex, res.RawDataHex)
			assert.Empty(t, res.Codec)

			reqs := dev.Requests()
			require.Len(t, reqs, 1)
			assert.Equal(t, uint8(17), reqs[0].Unit)
			assert.Equal(t, tc.wantPDU, reqs[0].PDU)
			if len(tc.wantRegs) > 0 {
				assert.Equal(t, tc.wantRegs, dev.Holding(cfg.StartAddress, uint16(len(tc.wantRegs))))
			} else {
				assert.Equal(t, tc.wantCoils, dev.Coils(cfg.StartAddress, uint16(len(tc.wantCoils))))
			}
		})
	}
}

func TestCoreCollectWrite_failures(t *testing.T) {
	t.Run("invalid plan sends nothing", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 0)
		_, err := CollectWrite(context.Background(), config.WriteConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Values: []string{"70000"},
		})
		e, ok := errs.As(err)
		require.True(t, ok)
		assert.Equal(t, errs.KindInvalidInput, e.Kind)
		assert.Equal(t, errs.CodeInvalidInput, e.Code)
		assert.Contains(t, e.Message, "out of range")
		assert.Zero(t, dev.RequestCount(0))
	})

	t.Run("connect refused", func(t *testing.T) {
		_, err := CollectWrite(context.Background(), config.WriteConfig{
			DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t), Unit: 1}, Timeout: coreShortTimeoutMS, Values: []string{"1"},
		})
		require.ErrorIs(t, err, ErrTCPConnection)
		coreRequireKind(t, err, errs.KindTransport, errs.CodeTransportConnectFailed)
	})

	for _, fc := range []uint8{5, 6, 15, 16} {
		t.Run("illegal address leaves the device untouched, fc"+string(rune('0'+fc/10))+string(rune('0'+fc%10)), func(t *testing.T) {
			dev := mbtest.NewDevice().SetHolding(0, 0xAAAA).SetCoils(0, true)
			_, err := CollectWrite(context.Background(), config.WriteConfig{
				DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: fc, StartAddress: 900, Values: []string{"0"},
			})
			require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress)
			coreRequireKind(t, err, errs.KindModbus, errs.CodeModbusException)
			assert.Equal(t, 1, dev.RequestCount(fc))
			assert.Equal(t, []uint16{0xAAAA}, dev.Holding(0, 1))
			assert.Equal(t, []bool{true}, dev.Coils(0, 1))
		})
	}

	// A write must never be repeated: a lost response does not mean a lost request.
	t.Run("timeout is not retried", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 0)
		dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Drop: true} }
		_, err := CollectWrite(context.Background(), config.WriteConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreShortTimeoutMS, Values: []string{"1"},
		})
		coreRequireKind(t, err, errs.KindTimeout, errs.CodeRequestTimeout)
		assert.Equal(t, 1, dev.RequestCount(0))
	})

	t.Run("busy exception is not retried", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 0)
		dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Exception: mbtest.ExServerDeviceBusy} }
		_, err := CollectWrite(context.Background(), config.WriteConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Values: []string{"1"},
		})
		require.ErrorIs(t, err, gomodbus.ErrServerDeviceBusy)
		assert.Equal(t, 1, dev.RequestCount(0))
		assert.Equal(t, []uint16{0}, dev.Holding(0, 1))
	})

	t.Run("closed connection is not retried", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 0)
		dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Close: true} }
		_, err := CollectWrite(context.Background(), config.WriteConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Values: []string{"1"},
		})
		require.Error(t, err)
		assert.Equal(t, 1, dev.RequestCount(0))
	})
}
