package mbtest

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
)

// ServeTCP serves the device as Modbus TCP (MBAP) on a loopback port until the test ends.
// It returns the host:port to connect to.
func (d *Device) ServeTCP(t testing.TB) string {
	t.Helper()
	return d.serveStream(t, d.serveMBAP)
}

// ServeRTUOverTCP serves the device with RTU framing over TCP on a loopback port.
func (d *Device) ServeRTUOverTCP(t testing.TB) string {
	t.Helper()
	return d.serveStream(t, d.serveRTU)
}

// ServeASCIIOverTCP serves the device with ASCII framing over TCP on a loopback port.
func (d *Device) ServeASCIIOverTCP(t testing.TB) string {
	t.Helper()
	return d.serveStream(t, d.serveASCII)
}

func (d *Device) serveStream(t testing.TB, serve func(io.ReadWriter)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mbtest: listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				serve(conn)
			}()
		}
	}()
	return ln.Addr().String()
}

// ServeUDP serves the device as Modbus TCP framing (MBAP) over UDP on a loopback port.
func (d *Device) ServeUDP(t testing.TB) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mbtest: listen udp: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			n, peer, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 8 {
				continue
			}
			r := d.handle(buf[6], buf[7:n])
			if r.pdu == nil {
				continue
			}
			_, _ = pc.WriteTo(mbapFrame(buf[0:2], buf[6], r.pdu), peer)
		}
	}()
	return pc.LocalAddr().String()
}

func mbapFrame(txn []byte, unit uint8, pdu []byte) []byte {
	out := make([]byte, 7, 7+len(pdu))
	copy(out, txn)
	binary.BigEndian.PutUint16(out[4:], uint16(len(pdu)+1))
	out[6] = unit
	return append(out, pdu...)
}

// serveMBAP answers MBAP-framed requests on rw until it fails or the device closes it.
func (d *Device) serveMBAP(rw io.ReadWriter) {
	header := make([]byte, 7)
	for {
		if _, err := io.ReadFull(rw, header); err != nil {
			return
		}
		length := int(binary.BigEndian.Uint16(header[4:]))
		if length < 2 || length > 254 {
			return
		}
		pdu := make([]byte, length-1)
		if _, err := io.ReadFull(rw, pdu); err != nil {
			return
		}
		r := d.handle(header[6], pdu)
		if r.close {
			return
		}
		if r.pdu == nil {
			continue
		}
		if _, err := rw.Write(mbapFrame(header[0:2], header[6], r.pdu)); err != nil {
			return
		}
	}
}

// CRC16 returns the Modbus RTU CRC of data.
func CRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// rtuRequestLength returns the total length of an RTU request frame given its first seven
// bytes, for the function codes the device implements.
func rtuRequestLength(head []byte) int {
	switch head[1] {
	case 0x0F, 0x10:
		return 7 + int(head[6]) + 2
	case 0x11:
		return 4
	case 0x2B:
		return 7
	default:
		return 8
	}
}

// serveRTU answers RTU-framed requests on rw until it fails or the device closes it.
func (d *Device) serveRTU(rw io.ReadWriter) {
	for {
		frame := make([]byte, 256)
		// Every supported request is at least 4 bytes; read those, then what the
		// function code calls for.
		if _, err := io.ReadFull(rw, frame[:4]); err != nil {
			return
		}
		n := 4
		if frame[1] != 0x11 {
			if _, err := io.ReadFull(rw, frame[4:7]); err != nil {
				return
			}
			n = 7
		}
		total := rtuRequestLength(frame[:7])
		if total > len(frame) {
			return
		}
		if total > n {
			if _, err := io.ReadFull(rw, frame[n:total]); err != nil {
				return
			}
		}
		frame = frame[:total]
		if CRC16(frame[:total-2]) != binary.LittleEndian.Uint16(frame[total-2:]) {
			continue
		}
		r := d.handle(frame[0], frame[1:total-2])
		if r.close {
			return
		}
		if r.pdu == nil {
			continue
		}
		out := append([]byte{frame[0]}, r.pdu...)
		out = binary.LittleEndian.AppendUint16(out, CRC16(out))
		if _, err := rw.Write(out); err != nil {
			return
		}
	}
}

// LRC returns the Modbus ASCII longitudinal redundancy check of data.
func LRC(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return -sum
}

// ASCIIFrame encodes unit and pdu as a Modbus ASCII frame.
func ASCIIFrame(unit uint8, pdu []byte) []byte {
	raw := append([]byte{unit}, pdu...)
	raw = append(raw, LRC(raw))
	return []byte(":" + strings.ToUpper(hex.EncodeToString(raw)) + "\r\n")
}

func parseASCIIFrame(line string) (unit uint8, pdu []byte, err error) {
	start := strings.LastIndexByte(line, ':')
	if start < 0 {
		return 0, nil, errors.New("mbtest: no start delimiter")
	}
	raw, err := hex.DecodeString(strings.TrimRight(line[start+1:], "\r\n"))
	if err != nil {
		return 0, nil, err
	}
	if len(raw) < 3 || LRC(raw[:len(raw)-1]) != raw[len(raw)-1] {
		return 0, nil, errors.New("mbtest: bad ascii frame")
	}
	return raw[0], raw[1 : len(raw)-1], nil
}

// serveASCII answers ASCII-framed requests on rw until it fails or the device closes it.
func (d *Device) serveASCII(rw io.ReadWriter) {
	br := bufio.NewReader(rw)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		unit, pdu, err := parseASCIIFrame(line)
		if err != nil {
			continue
		}
		r := d.handle(unit, pdu)
		if r.close {
			return
		}
		if r.pdu == nil {
			continue
		}
		if _, err := rw.Write(ASCIIFrame(unit, r.pdu)); err != nil {
			return
		}
	}
}
