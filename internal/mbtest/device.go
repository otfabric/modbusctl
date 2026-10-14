// Package mbtest provides an in-memory Modbus slave for tests. A [Device] holds coils and
// registers and can be served over every transport modbusctl speaks: Modbus TCP, Modbus UDP,
// RTU or ASCII framing over TCP, and RTU or ASCII on a pseudo-terminal standing in for a
// serial port.
package mbtest

import (
	"encoding/binary"
	"sort"
	"sync"
	"time"
)

// Modbus exception codes.
const (
	ExIllegalFunction     = 0x01
	ExIllegalDataAddress  = 0x02
	ExIllegalDataValue    = 0x03
	ExServerDeviceFailure = 0x04
	ExServerDeviceBusy    = 0x06
	ExGatewayTargetFailed = 0x0B
)

// Action tells the device what to do with a request instead of answering it normally.
type Action struct {
	// Exception, when non-zero, is returned as a Modbus exception response.
	Exception uint8
	// Drop leaves the request unanswered (the client times out).
	Drop bool
	// Close closes the connection without answering (stream transports only).
	Close bool
	// Delay is waited before the request is processed.
	Delay time.Duration
	// Reply, when set, is sent as the response PDU instead of the normal answer,
	// for example a malformed one.
	Reply []byte
}

// Request is one request seen by the device.
type Request struct {
	Unit uint8
	PDU  []byte
}

// ServerID is the FC17 Report Server ID payload.
type ServerID struct {
	ID         []byte
	Running    bool
	Additional []byte
}

// Device is an in-memory Modbus slave. Configure its exported fields before serving it;
// use the methods to read or change data while it is being served.
type Device struct {
	// Units restricts the unit IDs the device answers; requests for other units get a
	// "gateway target failed to respond" exception. Empty means every unit.
	Units []uint8
	// Identification holds the FC43 device identification objects by object ID. When
	// empty, FC43 is answered with an illegal-function exception.
	Identification map[uint8]string
	// ServerID is the FC17 answer. When nil, FC17 is answered with an illegal-function exception.
	ServerID *ServerID
	// OnRequest, when set, is called for every request and can override the response.
	OnRequest func(req Request) Action

	mu       sync.Mutex
	coils    map[uint16]bool
	discrete map[uint16]bool
	holding  map[uint16]uint16
	input    map[uint16]uint16
	requests []Request
}

// NewDevice returns an empty device that answers every unit ID.
func NewDevice() *Device {
	return &Device{
		coils:    map[uint16]bool{},
		discrete: map[uint16]bool{},
		holding:  map[uint16]uint16{},
		input:    map[uint16]uint16{},
	}
}

// SetCoils defines consecutive coils starting at addr.
func (d *Device) SetCoils(addr uint16, values ...bool) *Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, v := range values {
		d.coils[addr+uint16(i)] = v
	}
	return d
}

// SetDiscreteInputs defines consecutive discrete inputs starting at addr.
func (d *Device) SetDiscreteInputs(addr uint16, values ...bool) *Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, v := range values {
		d.discrete[addr+uint16(i)] = v
	}
	return d
}

// SetHolding defines consecutive holding registers starting at addr.
func (d *Device) SetHolding(addr uint16, values ...uint16) *Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, v := range values {
		d.holding[addr+uint16(i)] = v
	}
	return d
}

// SetInput defines consecutive input registers starting at addr.
func (d *Device) SetInput(addr uint16, values ...uint16) *Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, v := range values {
		d.input[addr+uint16(i)] = v
	}
	return d
}

// Holding returns count holding registers starting at addr; undefined registers read as 0.
func (d *Device) Holding(addr, count uint16) []uint16 {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]uint16, count)
	for i := range out {
		out[i] = d.holding[addr+uint16(i)]
	}
	return out
}

// Coils returns count coils starting at addr; undefined coils read as false.
func (d *Device) Coils(addr, count uint16) []bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]bool, count)
	for i := range out {
		out[i] = d.coils[addr+uint16(i)]
	}
	return out
}

// Requests returns a copy of every request received so far.
func (d *Device) Requests() []Request {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Request(nil), d.requests...)
}

// RequestCount returns how many requests with function code fc were received (fc 0 = all).
func (d *Device) RequestCount(fc uint8) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, r := range d.requests {
		if fc == 0 || (len(r.PDU) > 0 && r.PDU[0] == fc) {
			n++
		}
	}
	return n
}

// reply is the outcome of handling one request.
type reply struct {
	pdu   []byte // nil: no response
	close bool
}

func exception(fc, code uint8) []byte {
	return []byte{fc | 0x80, code}
}

// handle processes one request PDU addressed to unit.
func (d *Device) handle(unit uint8, pdu []byte) reply {
	if len(pdu) == 0 {
		return reply{}
	}
	req := Request{Unit: unit, PDU: append([]byte(nil), pdu...)}
	d.mu.Lock()
	d.requests = append(d.requests, req)
	d.mu.Unlock()

	fc := pdu[0]
	if d.OnRequest != nil {
		act := d.OnRequest(req)
		if act.Delay > 0 {
			time.Sleep(act.Delay)
		}
		switch {
		case act.Close:
			return reply{close: true}
		case act.Drop:
			return reply{}
		case act.Exception != 0:
			return reply{pdu: exception(fc, act.Exception)}
		case act.Reply != nil:
			return reply{pdu: append([]byte(nil), act.Reply...)}
		}
	}
	if len(d.Units) > 0 {
		known := false
		for _, u := range d.Units {
			known = known || u == unit
		}
		if !known {
			return reply{pdu: exception(fc, ExGatewayTargetFailed)}
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	return reply{pdu: d.dispatch(pdu)}
}

func (d *Device) dispatch(pdu []byte) []byte {
	fc := pdu[0]
	switch fc {
	case 0x01:
		return d.readBits(pdu, d.coils)
	case 0x02:
		return d.readBits(pdu, d.discrete)
	case 0x03:
		return d.readRegisters(pdu, d.holding)
	case 0x04:
		return d.readRegisters(pdu, d.input)
	case 0x05:
		return d.writeSingleCoil(pdu)
	case 0x06:
		return d.writeSingleRegister(pdu)
	case 0x0F:
		return d.writeMultipleCoils(pdu)
	case 0x10:
		return d.writeMultipleRegisters(pdu)
	case 0x08:
		// Diagnostics: echo the request (correct for sub-function 0, harmless for the rest).
		if len(pdu) < 5 {
			return exception(fc, ExIllegalDataValue)
		}
		return append([]byte(nil), pdu...)
	case 0x11:
		return d.reportServerID()
	case 0x2B:
		return d.deviceIdentification(pdu)
	default:
		return exception(fc, ExIllegalFunction)
	}
}

func addrQty(pdu []byte) (addr, qty uint16, ok bool) {
	if len(pdu) < 5 {
		return 0, 0, false
	}
	return binary.BigEndian.Uint16(pdu[1:]), binary.BigEndian.Uint16(pdu[3:]), true
}

func (d *Device) readBits(pdu []byte, bits map[uint16]bool) []byte {
	addr, qty, ok := addrQty(pdu)
	if !ok || qty == 0 || qty > 2000 {
		return exception(pdu[0], ExIllegalDataValue)
	}
	out := make([]byte, 2+(int(qty)+7)/8)
	out[0], out[1] = pdu[0], byte(len(out)-2)
	for i := 0; i < int(qty); i++ {
		v, defined := bits[addr+uint16(i)]
		if !defined || int(addr)+i > 0xFFFF {
			return exception(pdu[0], ExIllegalDataAddress)
		}
		if v {
			out[2+i/8] |= 1 << (i % 8)
		}
	}
	return out
}

func (d *Device) readRegisters(pdu []byte, regs map[uint16]uint16) []byte {
	addr, qty, ok := addrQty(pdu)
	if !ok || qty == 0 || qty > 125 {
		return exception(pdu[0], ExIllegalDataValue)
	}
	out := make([]byte, 2, 2+2*int(qty))
	out[0], out[1] = pdu[0], byte(2*qty)
	for i := 0; i < int(qty); i++ {
		v, defined := regs[addr+uint16(i)]
		if !defined || int(addr)+i > 0xFFFF {
			return exception(pdu[0], ExIllegalDataAddress)
		}
		out = binary.BigEndian.AppendUint16(out, v)
	}
	return out
}

func (d *Device) writeSingleCoil(pdu []byte) []byte {
	addr, value, ok := addrQty(pdu)
	if !ok || (value != 0xFF00 && value != 0x0000) {
		return exception(pdu[0], ExIllegalDataValue)
	}
	if _, defined := d.coils[addr]; !defined {
		return exception(pdu[0], ExIllegalDataAddress)
	}
	d.coils[addr] = value == 0xFF00
	return append([]byte(nil), pdu[:5]...)
}

func (d *Device) writeSingleRegister(pdu []byte) []byte {
	addr, value, ok := addrQty(pdu)
	if !ok {
		return exception(pdu[0], ExIllegalDataValue)
	}
	if _, defined := d.holding[addr]; !defined {
		return exception(pdu[0], ExIllegalDataAddress)
	}
	d.holding[addr] = value
	return append([]byte(nil), pdu[:5]...)
}

func (d *Device) writeMultipleCoils(pdu []byte) []byte {
	addr, qty, ok := addrQty(pdu)
	if !ok || qty == 0 || qty > 1968 || len(pdu) < 6 || int(pdu[5]) != (int(qty)+7)/8 || len(pdu) < 6+int(pdu[5]) {
		return exception(pdu[0], ExIllegalDataValue)
	}
	for i := 0; i < int(qty); i++ {
		if _, defined := d.coils[addr+uint16(i)]; !defined || int(addr)+i > 0xFFFF {
			return exception(pdu[0], ExIllegalDataAddress)
		}
	}
	for i := 0; i < int(qty); i++ {
		d.coils[addr+uint16(i)] = pdu[6+i/8]&(1<<(i%8)) != 0
	}
	return append([]byte(nil), pdu[:5]...)
}

func (d *Device) writeMultipleRegisters(pdu []byte) []byte {
	addr, qty, ok := addrQty(pdu)
	if !ok || qty == 0 || qty > 123 || len(pdu) < 6 || int(pdu[5]) != 2*int(qty) || len(pdu) < 6+int(pdu[5]) {
		return exception(pdu[0], ExIllegalDataValue)
	}
	for i := 0; i < int(qty); i++ {
		if _, defined := d.holding[addr+uint16(i)]; !defined || int(addr)+i > 0xFFFF {
			return exception(pdu[0], ExIllegalDataAddress)
		}
	}
	for i := 0; i < int(qty); i++ {
		d.holding[addr+uint16(i)] = binary.BigEndian.Uint16(pdu[6+2*i:])
	}
	return append([]byte(nil), pdu[:5]...)
}

func (d *Device) reportServerID() []byte {
	if d.ServerID == nil {
		return exception(0x11, ExIllegalFunction)
	}
	run := byte(0x00)
	if d.ServerID.Running {
		run = 0xFF
	}
	body := append(append([]byte(nil), d.ServerID.ID...), run)
	body = append(body, d.ServerID.Additional...)
	return append([]byte{0x11, byte(len(body))}, body...)
}

// deviceIdentification answers FC43 / MEI 0x0E. Stream requests (categories 1-3) return
// every object of that category in one response; category 4 returns the requested object.
func (d *Device) deviceIdentification(pdu []byte) []byte {
	if len(d.Identification) == 0 || len(pdu) < 4 || pdu[1] != 0x0E {
		return exception(pdu[0], ExIllegalFunction)
	}
	category, objectID := pdu[2], pdu[3]

	var maxID uint8
	switch category {
	case 0x01:
		maxID = 0x02
	case 0x02:
		maxID = 0x7F
	case 0x03:
		maxID = 0xFF
	case 0x04:
		if _, defined := d.Identification[objectID]; !defined {
			return exception(pdu[0], ExIllegalDataAddress)
		}
	default:
		return exception(pdu[0], ExIllegalDataValue)
	}

	ids := make([]int, 0, len(d.Identification))
	conformity := uint8(0x81)
	for id := range d.Identification {
		switch {
		case id >= 0x80:
			conformity = 0x83
		case id >= 0x03 && conformity < 0x82:
			conformity = 0x82
		}
		if (category == 0x04 && id == objectID) || (category != 0x04 && id <= maxID) {
			ids = append(ids, int(id))
		}
	}
	sort.Ints(ids)

	out := []byte{pdu[0], 0x0E, category, conformity, 0x00, 0x00, byte(len(ids))}
	for _, id := range ids {
		v := d.Identification[uint8(id)]
		out = append(out, byte(id), byte(len(v)))
		out = append(out, v...)
	}
	return out
}
