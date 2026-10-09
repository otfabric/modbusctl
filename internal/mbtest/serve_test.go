package mbtest

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	modbus "github.com/otfabric/go-modbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCRC16(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint16
	}{
		{"empty", nil, 0xFFFF},
		{"check string", []byte("123456789"), 0x4B37},
		{"read holding 40108", []byte{0x11, 0x03, 0x00, 0x6B, 0x00, 0x03}, 0x8776},
		{"read one register", []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01}, 0x0A84},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CRC16(tt.data))
		})
	}
}

func TestLRC(t *testing.T) {
	assert.Equal(t, byte(0x7E), LRC([]byte{0x11, 0x03, 0x00, 0x6B, 0x00, 0x03}))
	assert.Equal(t, byte(0x00), LRC(nil))
	assert.Equal(t, byte(0x01), LRC([]byte{0xFF}))
	assert.Equal(t, byte(0x00), LRC([]byte{0x80, 0x80}))
}

func TestASCIIFrame(t *testing.T) {
	assert.Equal(t, ":1103006B00037E\r\n", string(ASCIIFrame(0x11, []byte{0x03, 0x00, 0x6B, 0x00, 0x03})))
	assert.Equal(t, ":0A830271\r\n", string(ASCIIFrame(0x0A, []byte{0x83, 0x02})))
}

func TestParseASCIIFrame(t *testing.T) {
	unit, pdu, err := parseASCIIFrame(":1103006B00037E\r\n")
	require.NoError(t, err)
	assert.Equal(t, uint8(0x11), unit)
	assert.Equal(t, []byte{0x03, 0x00, 0x6B, 0x00, 0x03}, pdu)

	// Lower case hex and leading garbage before the last start delimiter are accepted.
	unit, pdu, err = parseASCIIFrame("junk:00:1103006b00037e\r\n")
	require.NoError(t, err)
	assert.Equal(t, uint8(0x11), unit)
	assert.Equal(t, []byte{0x03, 0x00, 0x6B, 0x00, 0x03}, pdu)

	for name, line := range map[string]string{
		"no start delimiter": "1103006B00037E\r\n",
		"not hex":            ":11ZZ\r\n",
		"odd length":         ":110\r\n",
		"too short":          ":11EF\r\n",
		"bad lrc":            ":1103006B00037F\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := parseASCIIFrame(line)
			assert.Error(t, err)
		})
	}
}

func TestMBAPFrame(t *testing.T) {
	assert.Equal(t,
		[]byte{0xAB, 0xCD, 0x00, 0x00, 0x00, 0x03, 0x07, 0x83, 0x02},
		mbapFrame([]byte{0xAB, 0xCD}, 7, []byte{0x83, 0x02}))
}

func TestRTURequestLength(t *testing.T) {
	head := func(fc, byteCount byte) []byte { return []byte{1, fc, 0, 0, 0, 0, byteCount} }
	assert.Equal(t, 8, rtuRequestLength(head(0x03, 0)))
	assert.Equal(t, 8, rtuRequestLength(head(0x06, 9)))
	assert.Equal(t, 8, rtuRequestLength(head(0x08, 0)))
	assert.Equal(t, 13, rtuRequestLength(head(0x10, 4)))
	assert.Equal(t, 10, rtuRequestLength(head(0x0F, 1)))
	assert.Equal(t, 4, rtuRequestLength(head(0x11, 0)))
	assert.Equal(t, 7, rtuRequestLength(head(0x2B, 0)))
}

// pipe runs serve on one end of an in-memory connection and returns the other end.
func pipe(t *testing.T, serve func(io.ReadWriter)) (net.Conn, <-chan struct{}) {
	t.Helper()
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = server.Close() }()
		serve(server)
	}()
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
	return client, done
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("serve loop did not return")
	}
}

func readN(t *testing.T, r io.Reader, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	require.NoError(t, err)
	return buf
}

func rtuFrame(unit uint8, pdu ...byte) []byte {
	out := append([]byte{unit}, pdu...)
	return binary.LittleEndian.AppendUint16(out, CRC16(out))
}

func TestServeMBAP_raw(t *testing.T) {
	d := testDevice()
	drop := false
	d.OnRequest = func(req Request) Action {
		switch {
		case req.PDU[0] == 0x63:
			return Action{Close: true}
		case drop:
			drop = false
			return Action{Drop: true}
		}
		return Action{}
	}
	conn, done := pipe(t, d.serveMBAP)

	_, err := conn.Write([]byte{0x00, 0x2A, 0, 0, 0, 6, 0x05, 0x03, 0x00, 0x64, 0x00, 0x02})
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x2A, 0, 0, 0, 7, 0x05, 0x03, 0x04, 0x12, 0x34, 0xAB, 0xCD}, readN(t, conn, 13))

	// A dropped request gets no answer; the next one is answered with its own transaction ID.
	drop = true
	_, err = conn.Write([]byte{0x00, 0x2B, 0, 0, 0, 6, 0x05, 0x03, 0x00, 0x64, 0x00, 0x01})
	require.NoError(t, err)
	_, err = conn.Write([]byte{0x00, 0x2C, 0, 0, 0, 6, 0x05, 0x03, 0x00, 0x65, 0x00, 0x01})
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x2C, 0, 0, 0, 5, 0x05, 0x03, 0x02, 0xAB, 0xCD}, readN(t, conn, 11))

	// Close ends the serve loop without an answer.
	_, err = conn.Write([]byte{0x00, 0x2D, 0, 0, 0, 2, 0x05, 0x63})
	require.NoError(t, err)
	waitDone(t, done)
	_, err = conn.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
	assert.Equal(t, 4, d.RequestCount(0))
}

func TestServeMBAP_invalidLengthEndsSession(t *testing.T) {
	for name, header := range map[string][]byte{
		"length too small": {0, 1, 0, 0, 0, 1, 1},
		"length too large": {0, 1, 0, 0, 0, 255, 1},
	} {
		t.Run(name, func(t *testing.T) {
			d := testDevice()
			conn, done := pipe(t, d.serveMBAP)
			_, err := conn.Write(header)
			require.NoError(t, err)
			waitDone(t, done)
			assert.Zero(t, d.RequestCount(0))
		})
	}
}

func TestServeMBAP_truncatedInputEndsSession(t *testing.T) {
	d := testDevice()
	conn, done := pipe(t, d.serveMBAP)
	_, err := conn.Write([]byte{0, 1, 0, 0, 0, 6, 1, 0x03, 0x00})
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	waitDone(t, done)
	assert.Zero(t, d.RequestCount(0))
}

func TestServeRTU_raw(t *testing.T) {
	d := testDevice()
	d.ServerID = &ServerID{ID: []byte{0x09}, Running: true}
	d.Identification = map[uint8]string{0: "V"}
	d.OnRequest = func(req Request) Action {
		switch req.PDU[0] {
		case 0x63:
			return Action{Close: true}
		case 0x04:
			return Action{Drop: true}
		}
		return Action{}
	}
	conn, done := pipe(t, d.serveRTU)
	send := func(frame []byte) {
		t.Helper()
		_, err := conn.Write(frame)
		require.NoError(t, err)
	}

	send(rtuFrame(0x11, 0x03, 0x00, 0x64, 0x00, 0x01))
	assert.Equal(t, rtuFrame(0x11, 0x03, 0x02, 0x12, 0x34), readN(t, conn, 7))

	// The 4-byte FC17 request and the 7-byte FC43 request are framed by function code.
	send(rtuFrame(0x11, 0x11))
	assert.Equal(t, rtuFrame(0x11, 0x11, 0x02, 0x09, 0xFF), readN(t, conn, 7))
	send(rtuFrame(0x11, 0x2B, 0x0E, 0x01, 0x00))
	assert.Equal(t, rtuFrame(0x11, 0x2B, 0x0E, 0x01, 0x81, 0x00, 0x00, 0x01, 0x00, 0x01, 'V'), readN(t, conn, 13))

	// Variable-length write request.
	send(rtuFrame(0x11, 0x10, 0x00, 0x64, 0x00, 0x02, 0x04, 0x00, 0x01, 0x00, 0x02))
	assert.Equal(t, rtuFrame(0x11, 0x10, 0x00, 0x64, 0x00, 0x02), readN(t, conn, 8))
	assert.Equal(t, []uint16{1, 2}, d.Holding(100, 2))

	// A frame with a bad CRC and a dropped request are both left unanswered.
	bad := rtuFrame(0x11, 0x06, 0x00, 0x64, 0x00, 0x63)
	bad[len(bad)-1] ^= 0xFF
	send(bad)
	send(rtuFrame(0x11, 0x04, 0x00, 0xC8, 0x00, 0x01))
	send(rtuFrame(0x11, 0x03, 0x00, 0x66, 0x00, 0x01))
	assert.Equal(t, rtuFrame(0x11, 0x03, 0x02, 0x00, 0x01), readN(t, conn, 7))
	assert.Equal(t, []uint16{1}, d.Holding(100, 1))
	assert.Zero(t, d.RequestCount(0x06))

	send(rtuFrame(0x11, 0x63, 0x00, 0x00, 0x00, 0x00))
	waitDone(t, done)
	_, err := conn.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
}

func TestServeRTU_truncatedInputEndsSession(t *testing.T) {
	full := rtuFrame(0x01, 0x10, 0x00, 0x64, 0x00, 0x02, 0x04, 0x00, 0x01, 0x00, 0x02)
	for _, n := range []int{2, 5, 9} {
		d := testDevice()
		conn, done := pipe(t, d.serveRTU)
		_, err := conn.Write(full[:n])
		require.NoError(t, err)
		require.NoError(t, conn.Close())
		waitDone(t, done)
		assert.Zero(t, d.RequestCount(0), "prefix of %d bytes", n)
	}
}

func TestServeRTU_oversizedFrameEndsSession(t *testing.T) {
	d := testDevice()
	conn, done := pipe(t, d.serveRTU)
	_, err := conn.Write([]byte{0x01, 0x10, 0x00, 0x64, 0x00, 0x7B, 0xFF})
	require.NoError(t, err)
	waitDone(t, done)
	assert.Zero(t, d.RequestCount(0))
}

func TestServeASCII_raw(t *testing.T) {
	d := testDevice()
	d.OnRequest = func(req Request) Action {
		switch req.PDU[0] {
		case 0x63:
			return Action{Close: true}
		case 0x04:
			return Action{Drop: true}
		}
		return Action{}
	}
	conn, done := pipe(t, d.serveASCII)
	send := func(frame []byte) {
		t.Helper()
		_, err := conn.Write(frame)
		require.NoError(t, err)
	}

	send(ASCIIFrame(0x11, []byte{0x03, 0x00, 0x64, 0x00, 0x01}))
	want := ASCIIFrame(0x11, []byte{0x03, 0x02, 0x12, 0x34})
	assert.Equal(t, ":1103021234A4\r\n", string(want))
	assert.Equal(t, want, readN(t, conn, len(want)))

	// Malformed and dropped frames are skipped; the session carries on.
	send([]byte(":1103006B00037F\r\n"))
	send([]byte("garbage\r\n"))
	send(ASCIIFrame(0x11, []byte{0x04, 0x00, 0xC8, 0x00, 0x01}))
	send(ASCIIFrame(0x12, []byte{0x03, 0x00, 0x65, 0x00, 0x01}))
	want = ASCIIFrame(0x12, []byte{0x03, 0x02, 0xAB, 0xCD})
	assert.Equal(t, want, readN(t, conn, len(want)))

	send(ASCIIFrame(0x11, []byte{0x63}))
	waitDone(t, done)
	_, err := conn.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
	assert.Equal(t, 4, d.RequestCount(0))
}

// failingWriter reads from a fixed request and fails every write.
type failingWriter struct{ io.Reader }

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestServe_writeErrorEndsSession(t *testing.T) {
	read := []byte{0x03, 0x00, 0x64, 0x00, 0x01}
	// Each input holds two requests: the loop must stop after failing to answer the first.
	inputs := map[string]struct {
		serve func(*Device) func(io.ReadWriter)
		frame []byte
	}{
		"mbap":  {func(d *Device) func(io.ReadWriter) { return d.serveMBAP }, mbapFrame([]byte{0, 1}, 1, read)},
		"rtu":   {func(d *Device) func(io.ReadWriter) { return d.serveRTU }, rtuFrame(1, read...)},
		"ascii": {func(d *Device) func(io.ReadWriter) { return d.serveASCII }, ASCIIFrame(1, read)},
	}
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			d := testDevice()
			in.serve(d)(failingWriter{bytes.NewReader(append(append([]byte(nil), in.frame...), in.frame...))})
			assert.Equal(t, 1, d.RequestCount(0))
		})
	}
}

func openClient(t *testing.T, conf modbus.Config) *modbus.Client {
	t.Helper()
	if conf.Timeout == 0 {
		conf.Timeout = 2 * time.Second
	}
	c, err := modbus.New(conf)
	require.NoError(t, err)
	require.NoError(t, c.Open())
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Every transport must carry each function code the device implements, end to end.
func TestServe_endToEnd(t *testing.T) {
	transports := []struct {
		name string
		conf func(t *testing.T, d *Device) modbus.Config
	}{
		{"tcp", func(t *testing.T, d *Device) modbus.Config { return modbus.Config{URL: "tcp://" + d.ServeTCP(t)} }},
		{"udp", func(t *testing.T, d *Device) modbus.Config { return modbus.Config{URL: "udp://" + d.ServeUDP(t)} }},
		{"rtuovertcp", func(t *testing.T, d *Device) modbus.Config {
			return modbus.Config{URL: "rtuovertcp://" + d.ServeRTUOverTCP(t)}
		}},
		{"asciiovertcp", func(t *testing.T, d *Device) modbus.Config {
			return modbus.Config{URL: "asciiovertcp://" + d.ServeASCIIOverTCP(t)}
		}},
		{"rtu serial", func(t *testing.T, d *Device) modbus.Config {
			return modbus.Config{URL: "rtu://" + d.ServeRTUSerial(t), Speed: 115200, Parity: modbus.ParityEven}
		}},
		{"ascii serial", func(t *testing.T, d *Device) modbus.Config {
			return modbus.Config{URL: "ascii://" + d.ServeASCIISerial(t), Speed: 115200, DataBits: 8}
		}},
	}
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			d := testDevice()
			d.Units = []uint8{5}
			d.ServerID = &ServerID{ID: []byte{0x42}, Running: true, Additional: []byte("mbtest")}
			d.Identification = map[uint8]string{0x00: "ACME", 0x01: "X1", 0x02: "1.0", 0x04: "Widget"}
			c := openClient(t, tr.conf(t, d))
			ctx := context.Background()
			const unit = 5

			coils, err := c.ReadCoils(ctx, unit, 0, 9)
			require.NoError(t, err)
			assert.Equal(t, []bool{true, false, true, true, false, false, false, false, true}, coils)

			inputs, err := c.ReadDiscreteInputs(ctx, unit, 10, 3)
			require.NoError(t, err)
			assert.Equal(t, []bool{false, true, true}, inputs)

			holding, err := c.ReadRegisters(ctx, unit, 100, 3, modbus.HoldingRegister)
			require.NoError(t, err)
			assert.Equal(t, []uint16{0x1234, 0xABCD, 0x0001}, holding)

			inreg, err := c.ReadRegisters(ctx, unit, 200, 2, modbus.InputRegister)
			require.NoError(t, err)
			assert.Equal(t, []uint16{0x00FF, 0xFF00}, inreg)

			require.NoError(t, c.WriteCoil(ctx, unit, 1, true))
			require.NoError(t, c.WriteCoils(ctx, unit, 4, []bool{true, true, false, true, false}))
			assert.Equal(t, []bool{true, true, true, true, true, true, false, true, false}, d.Coils(0, 9))

			require.NoError(t, c.WriteRegister(ctx, unit, 100, 0xBEEF))
			require.NoError(t, c.WriteRegisters(ctx, unit, 101, []uint16{7, 8}))
			assert.Equal(t, []uint16{0xBEEF, 7, 8}, d.Holding(100, 3))

			echo, err := c.DiagnosticLoopback(ctx, unit, 0xA537)
			require.NoError(t, err)
			assert.Equal(t, uint16(0xA537), echo)

			sid, err := c.ReportServerID(ctx, unit)
			require.NoError(t, err)
			assert.Equal(t, append([]byte{0x42, 0xFF}, "mbtest"...), sid.Data)

			di, err := c.ReadDeviceIdentification(ctx, unit, modbus.DeviceIDRegular, 0)
			require.NoError(t, err)
			assert.Equal(t, uint8(0x82), di.ConformityLevel)
			got := map[uint8]string{}
			for _, o := range di.Objects {
				got[uint8(o.ID)] = o.Value
			}
			assert.Equal(t, d.Identification, got)

			// Exceptions travel back as typed errors.
			_, err = c.ReadRegisters(ctx, unit, 5000, 1, modbus.HoldingRegister)
			assert.ErrorIs(t, err, modbus.ErrIllegalDataAddress)
			_, err = c.ReadRegisters(ctx, 9, 100, 1, modbus.HoldingRegister)
			assert.ErrorIs(t, err, modbus.ErrGWTargetFailedToRespond)

			assert.Equal(t, 2, d.RequestCount(0x10)+d.RequestCount(0x06))
			assert.Equal(t, 4, d.RequestCount(0x03)+d.RequestCount(0x04))
			reqs := d.Requests()
			for _, r := range reqs[:len(reqs)-1] {
				assert.Equal(t, uint8(unit), r.Unit)
			}
			assert.Equal(t, uint8(9), reqs[len(reqs)-1].Unit)
		})
	}
}

func TestServe_endToEnd_dropTimesOutAndRecovers(t *testing.T) {
	serve := map[string]func(t *testing.T, d *Device) string{
		"tcp":          func(t *testing.T, d *Device) string { return "tcp://" + d.ServeTCP(t) },
		"udp":          func(t *testing.T, d *Device) string { return "udp://" + d.ServeUDP(t) },
		"rtuovertcp":   func(t *testing.T, d *Device) string { return "rtuovertcp://" + d.ServeRTUOverTCP(t) },
		"asciiovertcp": func(t *testing.T, d *Device) string { return "asciiovertcp://" + d.ServeASCIIOverTCP(t) },
	}
	for name, url := range serve {
		t.Run(name, func(t *testing.T) {
			d := testDevice()
			d.OnRequest = func(req Request) Action {
				return Action{Drop: req.PDU[0] == 0x04}
			}
			u := url(t, d)
			ctx := context.Background()

			c := openClient(t, modbus.Config{URL: u, Timeout: 100 * time.Millisecond})
			_, err := c.ReadRegisters(ctx, 1, 200, 1, modbus.InputRegister)
			require.ErrorIs(t, err, modbus.ErrRequestTimedOut)
			assert.Equal(t, 1, d.RequestCount(0x04))

			// The device keeps serving new connections after an unanswered request.
			c2 := openClient(t, modbus.Config{URL: u})
			v, err := c2.ReadRegisters(ctx, 1, 100, 1, modbus.HoldingRegister)
			require.NoError(t, err)
			assert.Equal(t, []uint16{0x1234}, v)
		})
	}
}

func TestServe_endToEnd_closeDropsConnection(t *testing.T) {
	d := testDevice()
	d.OnRequest = func(req Request) Action {
		return Action{Close: req.PDU[0] == 0x04}
	}
	addr := d.ServeTCP(t)
	c := openClient(t, modbus.Config{URL: "tcp://" + addr})
	ctx := context.Background()

	_, err := c.ReadRegisters(ctx, 1, 200, 1, modbus.InputRegister)
	require.Error(t, err)
	assert.NotErrorIs(t, err, modbus.ErrRequestTimedOut)

	c2 := openClient(t, modbus.Config{URL: "tcp://" + addr})
	v, err := c2.ReadRegisters(ctx, 1, 100, 1, modbus.HoldingRegister)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0x1234}, v)
}

func TestServeUDP_ignoresShortDatagrams(t *testing.T) {
	d := testDevice()
	conn, err := net.Dial("udp", d.ServeUDP(t))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	_, err = conn.Write([]byte{0, 1, 0, 0, 0, 1, 1})
	require.NoError(t, err)
	_, err = conn.Write([]byte{0x12, 0x34, 0, 0, 0, 6, 0x01, 0x03, 0x00, 0x64, 0x00, 0x01})
	require.NoError(t, err)

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x12, 0x34, 0, 0, 0, 5, 0x01, 0x03, 0x02, 0x12, 0x34}, buf[:n])
	assert.Equal(t, 1, d.RequestCount(0))
}
