package mbtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDevice() *Device {
	return NewDevice().
		SetCoils(0, true, false, true, true, false, false, false, false, true).
		SetDiscreteInputs(10, false, true, true).
		SetHolding(100, 0x1234, 0xABCD, 0x0001).
		SetInput(200, 0x00FF, 0xFF00)
}

func TestDevice_handle_functionCodes(t *testing.T) {
	tests := []struct {
		name string
		pdu  []byte
		want []byte
	}{
		{"fc1 read 9 coils", []byte{0x01, 0x00, 0x00, 0x00, 0x09}, []byte{0x01, 0x02, 0x0D, 0x01}},
		{"fc1 undefined address", []byte{0x01, 0x00, 0x05, 0x00, 0x08}, []byte{0x81, ExIllegalDataAddress}},
		{"fc1 zero quantity", []byte{0x01, 0x00, 0x00, 0x00, 0x00}, []byte{0x81, ExIllegalDataValue}},
		{"fc1 quantity too large", []byte{0x01, 0x00, 0x00, 0x07, 0xD1}, []byte{0x81, ExIllegalDataValue}},
		{"fc1 short pdu", []byte{0x01, 0x00, 0x00}, []byte{0x81, ExIllegalDataValue}},
		{"fc2 read discrete inputs", []byte{0x02, 0x00, 0x0A, 0x00, 0x03}, []byte{0x02, 0x01, 0x06}},
		{"fc2 undefined address", []byte{0x02, 0x00, 0x00, 0x00, 0x01}, []byte{0x82, ExIllegalDataAddress}},
		{"fc3 read holding", []byte{0x03, 0x00, 0x64, 0x00, 0x03}, []byte{0x03, 0x06, 0x12, 0x34, 0xAB, 0xCD, 0x00, 0x01}},
		{"fc3 undefined address", []byte{0x03, 0x00, 0x64, 0x00, 0x04}, []byte{0x83, ExIllegalDataAddress}},
		{"fc3 zero quantity", []byte{0x03, 0x00, 0x64, 0x00, 0x00}, []byte{0x83, ExIllegalDataValue}},
		{"fc3 quantity too large", []byte{0x03, 0x00, 0x64, 0x00, 0x7E}, []byte{0x83, ExIllegalDataValue}},
		{"fc3 short pdu", []byte{0x03}, []byte{0x83, ExIllegalDataValue}},
		{"fc4 read input", []byte{0x04, 0x00, 0xC8, 0x00, 0x02}, []byte{0x04, 0x04, 0x00, 0xFF, 0xFF, 0x00}},
		{"fc4 undefined address", []byte{0x04, 0x00, 0x64, 0x00, 0x01}, []byte{0x84, ExIllegalDataAddress}},
		{"fc5 write coil on", []byte{0x05, 0x00, 0x01, 0xFF, 0x00}, []byte{0x05, 0x00, 0x01, 0xFF, 0x00}},
		{"fc5 bad value", []byte{0x05, 0x00, 0x01, 0x12, 0x34}, []byte{0x85, ExIllegalDataValue}},
		{"fc5 short pdu", []byte{0x05, 0x00}, []byte{0x85, ExIllegalDataValue}},
		{"fc5 undefined address", []byte{0x05, 0x00, 0x63, 0x00, 0x00}, []byte{0x85, ExIllegalDataAddress}},
		{"fc6 write register", []byte{0x06, 0x00, 0x65, 0xBE, 0xEF}, []byte{0x06, 0x00, 0x65, 0xBE, 0xEF}},
		{"fc6 short pdu", []byte{0x06, 0x00, 0x65}, []byte{0x86, ExIllegalDataValue}},
		{"fc6 undefined address", []byte{0x06, 0x00, 0x00, 0x00, 0x01}, []byte{0x86, ExIllegalDataAddress}},
		{"fc15 write coils", []byte{0x0F, 0x00, 0x00, 0x00, 0x09, 0x02, 0xF2, 0x00}, []byte{0x0F, 0x00, 0x00, 0x00, 0x09}},
		{"fc15 byte count mismatch", []byte{0x0F, 0x00, 0x00, 0x00, 0x09, 0x01, 0xF2}, []byte{0x8F, ExIllegalDataValue}},
		{"fc15 truncated data", []byte{0x0F, 0x00, 0x00, 0x00, 0x09, 0x02, 0xF2}, []byte{0x8F, ExIllegalDataValue}},
		{"fc15 zero quantity", []byte{0x0F, 0x00, 0x00, 0x00, 0x00, 0x00}, []byte{0x8F, ExIllegalDataValue}},
		{"fc15 no byte count", []byte{0x0F, 0x00, 0x00, 0x00, 0x01}, []byte{0x8F, ExIllegalDataValue}},
		{"fc15 undefined address", []byte{0x0F, 0x00, 0x08, 0x00, 0x02, 0x01, 0x03}, []byte{0x8F, ExIllegalDataAddress}},
		{"fc16 write registers", []byte{0x10, 0x00, 0x64, 0x00, 0x02, 0x04, 0x00, 0x0A, 0x01, 0x02}, []byte{0x10, 0x00, 0x64, 0x00, 0x02}},
		{"fc16 byte count mismatch", []byte{0x10, 0x00, 0x64, 0x00, 0x02, 0x02, 0x00, 0x0A}, []byte{0x90, ExIllegalDataValue}},
		{"fc16 truncated data", []byte{0x10, 0x00, 0x64, 0x00, 0x02, 0x04, 0x00, 0x0A}, []byte{0x90, ExIllegalDataValue}},
		{"fc16 quantity too large", []byte{0x10, 0x00, 0x64, 0x00, 0x7C, 0xF8}, []byte{0x90, ExIllegalDataValue}},
		{"fc16 undefined address", []byte{0x10, 0x00, 0x66, 0x00, 0x02, 0x04, 0, 1, 0, 2}, []byte{0x90, ExIllegalDataAddress}},
		{"fc8 echo", []byte{0x08, 0x00, 0x00, 0xA5, 0x37}, []byte{0x08, 0x00, 0x00, 0xA5, 0x37}},
		{"fc8 short pdu", []byte{0x08, 0x00, 0x00}, []byte{0x88, ExIllegalDataValue}},
		{"fc17 without server id", []byte{0x11}, []byte{0x91, ExIllegalFunction}},
		{"fc43 without identification", []byte{0x2B, 0x0E, 0x01, 0x00}, []byte{0xAB, ExIllegalFunction}},
		{"unknown function", []byte{0x07}, []byte{0x87, ExIllegalFunction}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testDevice().handle(1, tt.pdu)
			assert.Equal(t, tt.want, r.pdu)
			assert.False(t, r.close)
		})
	}
}

func TestDevice_handle_emptyPDU(t *testing.T) {
	d := testDevice()
	r := d.handle(1, nil)
	assert.Nil(t, r.pdu)
	assert.False(t, r.close)
	assert.Empty(t, d.Requests())
}

func TestDevice_writesChangeState(t *testing.T) {
	d := testDevice()

	d.handle(1, []byte{0x05, 0x00, 0x01, 0xFF, 0x00})
	d.handle(1, []byte{0x05, 0x00, 0x00, 0x00, 0x00})
	assert.Equal(t, []bool{false, true, true}, d.Coils(0, 3))

	d.handle(1, []byte{0x0F, 0x00, 0x00, 0x00, 0x09, 0x02, 0xF2, 0x00})
	assert.Equal(t, []bool{false, true, false, false, true, true, true, true, false}, d.Coils(0, 9))

	d.handle(1, []byte{0x06, 0x00, 0x65, 0xBE, 0xEF})
	assert.Equal(t, []uint16{0x1234, 0xBEEF, 0x0001}, d.Holding(100, 3))

	d.handle(1, []byte{0x10, 0x00, 0x64, 0x00, 0x02, 0x04, 0x00, 0x0A, 0x01, 0x02})
	assert.Equal(t, []uint16{0x000A, 0x0102, 0x0001}, d.Holding(100, 3))

	// A rejected multi-write must not be applied partially.
	d.handle(1, []byte{0x10, 0x00, 0x66, 0x00, 0x02, 0x04, 0x11, 0x11, 0x22, 0x22})
	assert.Equal(t, []uint16{0x0001, 0x0000}, d.Holding(102, 2))
	d.handle(1, []byte{0x0F, 0x00, 0x08, 0x00, 0x02, 0x01, 0x03})
	assert.Equal(t, []bool{false, false}, d.Coils(8, 2))

	// Undefined values read as zero through the accessors.
	assert.Equal(t, []uint16{0, 0}, d.Holding(5000, 2))
	assert.Equal(t, []bool{false}, d.Coils(5000, 1))
}

func TestDevice_addressWrapIsRejected(t *testing.T) {
	d := NewDevice().
		SetHolding(0xFFFF, 1).SetHolding(0, 2).
		SetCoils(0xFFFF, true).SetCoils(0, true)

	assert.Equal(t, []byte{0x83, ExIllegalDataAddress}, d.handle(1, []byte{0x03, 0xFF, 0xFF, 0x00, 0x02}).pdu)
	assert.Equal(t, []byte{0x81, ExIllegalDataAddress}, d.handle(1, []byte{0x01, 0xFF, 0xFF, 0x00, 0x02}).pdu)
	assert.Equal(t, []byte{0x8F, ExIllegalDataAddress}, d.handle(1, []byte{0x0F, 0xFF, 0xFF, 0x00, 0x02, 0x01, 0x00}).pdu)
	assert.Equal(t, []byte{0x90, ExIllegalDataAddress}, d.handle(1, []byte{0x10, 0xFF, 0xFF, 0x00, 0x02, 0x04, 0, 9, 0, 9}).pdu)
	assert.Equal(t, []uint16{1}, d.Holding(0xFFFF, 1))
	assert.Equal(t, []uint16{2}, d.Holding(0, 1))
}

func TestDevice_reportServerID(t *testing.T) {
	d := NewDevice()
	d.ServerID = &ServerID{ID: []byte{0x42, 0x43}, Running: true, Additional: []byte("ab")}
	assert.Equal(t, []byte{0x11, 0x05, 0x42, 0x43, 0xFF, 'a', 'b'}, d.handle(1, []byte{0x11}).pdu)

	d.ServerID = &ServerID{ID: []byte{0x01}}
	assert.Equal(t, []byte{0x11, 0x02, 0x01, 0x00}, d.handle(1, []byte{0x11}).pdu)
}

func TestDevice_deviceIdentification(t *testing.T) {
	basic := map[uint8]string{0x00: "ACME", 0x01: "X1", 0x02: "1.0"}
	regular := map[uint8]string{0x00: "ACME", 0x01: "X1", 0x02: "1.0", 0x04: "Widget"}
	extended := map[uint8]string{0x00: "ACME", 0x01: "X1", 0x02: "1.0", 0x04: "Widget", 0x80: "priv"}

	basicObjects := []byte{0x00, 4, 'A', 'C', 'M', 'E', 0x01, 2, 'X', '1', 0x02, 3, '1', '.', '0'}
	widget := []byte{0x04, 6, 'W', 'i', 'd', 'g', 'e', 't'}
	private := []byte{0x80, 4, 'p', 'r', 'i', 'v'}
	join := func(header []byte, parts ...[]byte) []byte {
		out := append([]byte(nil), header...)
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	tests := []struct {
		name string
		ids  map[uint8]string
		pdu  []byte
		want []byte
	}{
		{"basic stream", basic, []byte{0x2B, 0x0E, 0x01, 0x00}, join([]byte{0x2B, 0x0E, 0x01, 0x81, 0, 0, 3}, basicObjects)},
		{"basic stream on regular device", regular, []byte{0x2B, 0x0E, 0x01, 0x00}, join([]byte{0x2B, 0x0E, 0x01, 0x82, 0, 0, 3}, basicObjects)},
		{"regular stream", regular, []byte{0x2B, 0x0E, 0x02, 0x00}, join([]byte{0x2B, 0x0E, 0x02, 0x82, 0, 0, 4}, basicObjects, widget)},
		{"regular stream on extended device", extended, []byte{0x2B, 0x0E, 0x02, 0x00}, join([]byte{0x2B, 0x0E, 0x02, 0x83, 0, 0, 4}, basicObjects, widget)},
		{"extended stream", extended, []byte{0x2B, 0x0E, 0x03, 0x00}, join([]byte{0x2B, 0x0E, 0x03, 0x83, 0, 0, 5}, basicObjects, widget, private)},
		{"individual object", extended, []byte{0x2B, 0x0E, 0x04, 0x04}, join([]byte{0x2B, 0x0E, 0x04, 0x83, 0, 0, 1}, widget)},
		{"individual object undefined", basic, []byte{0x2B, 0x0E, 0x04, 0x07}, []byte{0xAB, ExIllegalDataAddress}},
		{"unknown category", basic, []byte{0x2B, 0x0E, 0x05, 0x00}, []byte{0xAB, ExIllegalDataValue}},
		{"wrong mei type", basic, []byte{0x2B, 0x0D, 0x01, 0x00}, []byte{0xAB, ExIllegalFunction}},
		{"short pdu", basic, []byte{0x2B, 0x0E, 0x01}, []byte{0xAB, ExIllegalFunction}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDevice()
			d.Identification = tt.ids
			assert.Equal(t, tt.want, d.handle(1, tt.pdu).pdu)
		})
	}
}

func TestDevice_unitsFilter(t *testing.T) {
	d := testDevice()
	d.Units = []uint8{3, 7}
	read := []byte{0x03, 0x00, 0x64, 0x00, 0x01}

	assert.Equal(t, []byte{0x03, 0x02, 0x12, 0x34}, d.handle(3, read).pdu)
	assert.Equal(t, []byte{0x03, 0x02, 0x12, 0x34}, d.handle(7, read).pdu)
	assert.Equal(t, []byte{0x83, ExGatewayTargetFailed}, d.handle(1, read).pdu)

	// A write to a filtered unit must not be applied.
	assert.Equal(t, []byte{0x86, ExGatewayTargetFailed}, d.handle(4, []byte{0x06, 0x00, 0x64, 0x00, 0x00}).pdu)
	assert.Equal(t, []uint16{0x1234}, d.Holding(100, 1))
}

func TestDevice_onRequestActions(t *testing.T) {
	read := []byte{0x03, 0x00, 0x64, 0x00, 0x01}
	tests := []struct {
		name   string
		action Action
		want   reply
	}{
		{"no override", Action{}, reply{pdu: []byte{0x03, 0x02, 0x12, 0x34}}},
		{"exception", Action{Exception: ExServerDeviceBusy}, reply{pdu: []byte{0x83, ExServerDeviceBusy}}},
		{"drop", Action{Drop: true}, reply{}},
		{"close", Action{Close: true}, reply{close: true}},
		{"close wins over drop and exception", Action{Close: true, Drop: true, Exception: ExServerDeviceFailure}, reply{close: true}},
		{"drop wins over exception", Action{Drop: true, Exception: ExServerDeviceFailure}, reply{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDevice()
			var seen []Request
			d.OnRequest = func(req Request) Action {
				seen = append(seen, req)
				return tt.action
			}
			assert.Equal(t, tt.want, d.handle(9, read))
			assert.Equal(t, []Request{{Unit: 9, PDU: read}}, seen)
		})
	}
}

func TestDevice_onRequestDelay(t *testing.T) {
	d := testDevice()
	d.OnRequest = func(Request) Action { return Action{Delay: 30 * time.Millisecond} }
	start := time.Now()
	r := d.handle(1, []byte{0x03, 0x00, 0x64, 0x00, 0x01})
	assert.GreaterOrEqual(t, time.Since(start), 30*time.Millisecond)
	assert.Equal(t, []byte{0x03, 0x02, 0x12, 0x34}, r.pdu)
}

func TestDevice_onRequestExceptionSkipsWrite(t *testing.T) {
	d := testDevice()
	d.OnRequest = func(req Request) Action {
		if req.PDU[0] == 0x06 {
			return Action{Exception: ExServerDeviceFailure}
		}
		return Action{}
	}
	assert.Equal(t, []byte{0x86, ExServerDeviceFailure}, d.handle(1, []byte{0x06, 0x00, 0x64, 0x00, 0x00}).pdu)
	assert.Equal(t, []uint16{0x1234}, d.Holding(100, 1))
}

func TestDevice_requestsLog(t *testing.T) {
	d := testDevice()
	read := []byte{0x03, 0x00, 0x64, 0x00, 0x01}
	d.handle(1, read)
	d.handle(2, []byte{0x06, 0x00, 0x64, 0x00, 0x09})
	d.handle(1, read)
	d.handle(1, []byte{0x63})

	require.Equal(t, 4, d.RequestCount(0))
	assert.Equal(t, 2, d.RequestCount(0x03))
	assert.Equal(t, 1, d.RequestCount(0x06))
	assert.Equal(t, 1, d.RequestCount(0x63))
	assert.Equal(t, 0, d.RequestCount(0x10))

	got := d.Requests()
	require.Len(t, got, 4)
	assert.Equal(t, Request{Unit: 2, PDU: []byte{0x06, 0x00, 0x64, 0x00, 0x09}}, got[1])

	// The log holds copies: neither the caller's buffer nor the returned slice aliases it.
	read[0] = 0xEE
	got[1] = Request{}
	assert.Equal(t, uint8(0x03), d.Requests()[0].PDU[0])
	assert.Equal(t, uint8(2), d.Requests()[1].Unit)
}

func TestDevice_replyOverride(t *testing.T) {
	d := testDevice()
	d.OnRequest = func(Request) Action { return Action{Reply: []byte{0x03, 0x02, 0xBE}} }
	r := d.handle(1, []byte{0x03, 0x00, 0x64, 0x00, 0x01})
	assert.Equal(t, []byte{0x03, 0x02, 0xBE}, r.pdu, "the override is sent as it is, malformed or not")
}
