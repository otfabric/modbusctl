package modbus

// Record -> file -> replay round trips for every read function of the Modbus
// specification modbusctl supports: FC01-04, FC07, FC08 (sub-functions that
// return data), FC11, FC12, FC17, FC20, FC24 and FC43/14. A device is recorded
// with `client record` and a plan holding every function; the file is checked
// with go-modbus-capture's mbcapctl verify; then the static and replay servers
// must answer each request with the recorded response PDU, byte for byte.

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/plan"
	"github.com/stretchr/testify/require"
)

// rtPlan reads every supported function once. Its address blocks do not
// overlap, so each address is recorded by one request only.
const rtPlan = `[
	{"function": 1, "start_address": 0, "register_count": 13},
	{"function": 2, "start_address": 8, "register_count": 4},
	{"function": 3, "start_address": 100, "register_count": 4},
	{"function": 4, "start_address": 0, "register_count": 2},
	{"function": 7},
	{"function": 8, "sub_function": 0, "data": "A537"},
	{"function": 8, "sub_function": 2},
	{"function": 8, "sub_function": 11},
	{"function": 8, "sub_function": 18},
	{"function": 11},
	{"function": 12},
	{"function": 17},
	{"function": 20, "file_records": [{"file": 1, "record": 2, "length": 3}, {"file": 4, "record": 0, "length": 1}]},
	{"function": 24, "fifo_address": 1234},
	{"function": 43},
	{"function": 43, "device_id_code": 4, "object_id": 2}
]`

// rtFunctions are the function codes of rtPlan.
var rtFunctions = []uint8{1, 2, 3, 4, 7, 8, 11, 12, 17, 20, 24, 43}

func rtRequests(t testing.TB) []plan.Request {
	t.Helper()
	reqs, err := plan.Parse([]byte(rtPlan), 3)
	require.NoError(t, err)
	return reqs
}

// rtObjects are the identification objects of the round-trip devices. They do
// not change: FC43 is served per object, so a value that changed between a
// stream read and an individual read would not come back as either response.
var rtObjects = map[uint8]string{0: "OTFabric", 1: "MBCTL-RT", 2: "3.1.0"}

// rtConformity is the FC43 conformity level: basic, stream and individual access.
const rtConformity = 0x81

// rtRespond returns a valid response PDU to a supported read request, with
// values derived from n, so that successive answers to the same request differ.
func rtRespond(req []byte, n uint16) []byte {
	u16 := func(v uint16) []byte { return []byte{byte(v >> 8), byte(v)} }
	switch req[0] {
	case 1, 2:
		start, qty := binary.BigEndian.Uint16(req[1:]), binary.BigEndian.Uint16(req[3:])
		data := make([]byte, (qty+7)/8)
		for i := range qty {
			if (uint16(start)+i+n)%3 != 0 {
				data[i/8] |= 1 << (i % 8)
			}
		}
		return append([]byte{req[0], byte(len(data))}, data...)
	case 3, 4:
		start, qty := binary.BigEndian.Uint16(req[1:]), binary.BigEndian.Uint16(req[3:])
		resp := []byte{req[0], byte(2 * qty)}
		for i := range qty {
			resp = append(resp, u16(start+i+1000*n)...)
		}
		return resp
	case 7:
		return []byte{7, byte(n)}
	case 8:
		if req[1] == 0 && req[2] == 0 {
			return append([]byte(nil), req...) // Return Query Data echoes
		}
		return append([]byte{8, req[1], req[2]}, u16(n*7+uint16(req[2]))...)
	case 11:
		return append([]byte{11, 0x00, 0x00}, u16(n)...)
	case 12:
		events := []byte{0x20, byte(n), 0x00}
		resp := append([]byte{12, byte(6 + len(events)), 0x00, 0x00}, u16(n)...)
		return append(append(resp, u16(n*2)...), events...)
	case 17:
		data := []byte{0x2A, 'r', 't', byte(n), 0xFF}
		return append([]byte{17, byte(len(data))}, data...)
	case 20:
		var body []byte
		for p := 2; p+7 <= len(req); p += 7 {
			length := binary.BigEndian.Uint16(req[p+5:])
			body = append(body, byte(1+2*length), 6)
			for k := range length {
				body = append(body, u16(binary.BigEndian.Uint16(req[p+1:])<<8+k+n)...)
			}
		}
		return append([]byte{20, byte(len(body))}, body...)
	case 24:
		count := 1 + n%3
		resp := append(append([]byte{24}, u16(2+2*count)...), u16(count)...)
		for i := range count {
			resp = append(resp, u16(binary.BigEndian.Uint16(req[1:])+i*n)...)
		}
		return resp
	case 43:
		code, first := req[2], req[3]
		resp := []byte{43, 0x0E, code, rtConformity, 0x00, 0x00}
		var ids []uint8
		if code == 4 {
			ids = []uint8{first}
		} else {
			for id := first; id <= 2; id++ {
				ids = append(ids, id)
			}
		}
		resp = append(resp, byte(len(ids)))
		for _, id := range ids {
			resp = append(append(resp, id, byte(len(rtObjects[id]))), rtObjects[id]...)
		}
		return resp
	}
	return []byte{req[0] | 0x80, mbtest.ExIllegalFunction}
}

// rtDevice is an mbtest device answering every supported read with rtRespond.
// Each answer to the same request gets the next n.
func rtDevice() *mbtest.Device {
	d := mbtest.NewDevice()
	var mu sync.Mutex
	counts := map[string]uint16{}
	d.OnRequest = func(r mbtest.Request) mbtest.Action {
		mu.Lock()
		defer mu.Unlock()
		counts[string(r.PDU)]++
		return mbtest.Action{Reply: rtRespond(r.PDU, counts[string(r.PDU)])}
	}
	return d
}

// rtGoModbusDevice is the go-modbus server counterpart of rtDevice: the same
// answers through the typed handlers of a real go-modbus server.
type rtGoModbusDevice struct {
	mu     sync.Mutex
	counts map[string]uint16
}

func (d *rtGoModbusDevice) respond(req []byte) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.counts == nil {
		d.counts = map[string]uint16{}
	}
	d.counts[string(req)]++
	return rtRespond(req, d.counts[string(req)])
}

func rtReadReq(fc uint8, addr, qty uint16) []byte {
	return []byte{fc, byte(addr >> 8), byte(addr), byte(qty >> 8), byte(qty)}
}

func rtBits(resp []byte, qty uint16) []bool {
	out := make([]bool, qty)
	for i := range out {
		out[i] = resp[2+i/8]&(1<<(i%8)) != 0
	}
	return out
}

func rtWords(resp []byte, from, count int) []uint16 {
	out := make([]uint16, count)
	for i := range out {
		out[i] = binary.BigEndian.Uint16(resp[from+2*i:])
	}
	return out
}

func (d *rtGoModbusDevice) HandleCoils(_ context.Context, r *gomodbus.CoilsRequest) ([]bool, error) {
	if r.IsWrite {
		return nil, gomodbus.ErrIllegalFunction
	}
	return rtBits(d.respond(rtReadReq(1, r.Addr, r.Quantity)), r.Quantity), nil
}

func (d *rtGoModbusDevice) HandleDiscreteInputs(_ context.Context, r *gomodbus.DiscreteInputsRequest) ([]bool, error) {
	return rtBits(d.respond(rtReadReq(2, r.Addr, r.Quantity)), r.Quantity), nil
}

func (d *rtGoModbusDevice) HandleHoldingRegisters(_ context.Context, r *gomodbus.HoldingRegistersRequest) ([]uint16, error) {
	if r.IsWrite {
		return nil, gomodbus.ErrIllegalFunction
	}
	return rtWords(d.respond(rtReadReq(3, r.Addr, r.Quantity)), 2, int(r.Quantity)), nil
}

func (d *rtGoModbusDevice) HandleInputRegisters(_ context.Context, r *gomodbus.InputRegistersRequest) ([]uint16, error) {
	return rtWords(d.respond(rtReadReq(4, r.Addr, r.Quantity)), 2, int(r.Quantity)), nil
}

func (d *rtGoModbusDevice) HandleExceptionStatus(context.Context, *gomodbus.ExceptionStatusRequest) (uint8, error) {
	return d.respond([]byte{7})[1], nil
}

func (d *rtGoModbusDevice) HandleDiagnostics(_ context.Context, r *gomodbus.DiagnosticsRequest) ([]byte, error) {
	sub := uint16(r.SubFunction)
	return d.respond(append([]byte{8, byte(sub >> 8), byte(sub)}, r.Data...))[3:], nil
}

func (d *rtGoModbusDevice) HandleCommEventCounter(context.Context, *gomodbus.CommEventCounterRequest) (*gomodbus.CommEventCounterResponse, error) {
	p := d.respond([]byte{11})
	return &gomodbus.CommEventCounterResponse{Status: binary.BigEndian.Uint16(p[1:]), EventCount: binary.BigEndian.Uint16(p[3:])}, nil
}

func (d *rtGoModbusDevice) HandleCommEventLog(context.Context, *gomodbus.CommEventLogRequest) (*gomodbus.CommEventLogResponse, error) {
	p := d.respond([]byte{12})
	return &gomodbus.CommEventLogResponse{Status: binary.BigEndian.Uint16(p[2:]), EventCount: binary.BigEndian.Uint16(p[4:]),
		MessageCount: binary.BigEndian.Uint16(p[6:]), Events: p[8:]}, nil
}

func (d *rtGoModbusDevice) HandleReportServerID(context.Context, *gomodbus.ReportServerIDRequest) ([]byte, error) {
	return d.respond([]byte{17})[2:], nil
}

func (d *rtGoModbusDevice) HandleReadFileRecords(_ context.Context, r *gomodbus.ReadFileRecordsRequest) ([][]uint16, error) {
	req := []byte{20, byte(7 * len(r.Requests))}
	for _, s := range r.Requests {
		req = append(req, 6, byte(s.FileNumber>>8), byte(s.FileNumber), byte(s.RecordNumber>>8), byte(s.RecordNumber),
			byte(s.RecordLength>>8), byte(s.RecordLength))
	}
	p := d.respond(req)
	out, at := make([][]uint16, len(r.Requests)), 2
	for i, s := range r.Requests {
		out[i] = rtWords(p, at+2, int(s.RecordLength))
		at += 2 + 2*int(s.RecordLength)
	}
	return out, nil
}

func (d *rtGoModbusDevice) HandleReadFIFOQueue(_ context.Context, r *gomodbus.FIFOQueueRequest) ([]uint16, error) {
	p := d.respond([]byte{24, byte(r.Addr >> 8), byte(r.Addr)})
	return rtWords(p, 5, int(binary.BigEndian.Uint16(p[3:]))), nil
}

func (d *rtGoModbusDevice) HandleDeviceIdentification(_ context.Context, r *gomodbus.DeviceIdentificationRequest) (*gomodbus.DeviceIdentificationResponse, error) {
	res := &gomodbus.DeviceIdentificationResponse{ConformityLevel: rtConformity}
	for id := uint8(0); id <= 2; id++ {
		res.Objects = append(res.Objects, gomodbus.DeviceIdentificationObject{ID: gomodbus.DeviceIDObjectID(id), Value: rtObjects[id]})
	}
	return res, nil
}

// rtServeGoModbus serves a rtGoModbusDevice with a go-modbus server on a free
// loopback port and returns its URL.
func rtServeGoModbus(t *testing.T) string {
	t.Helper()
	return e2eFunctionsServe(t, &rtGoModbusDevice{})
}

// rtRecord records reqs from url (unit 1) with `client record` for duration
// ms, every interval ms, and returns the capture path.
func rtRecord(t *testing.T, url, planJSON string, interval, duration uint32, timeoutMS uint16) string {
	t.Helper()
	dir := t.TempDir()
	pf := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(pf, []byte(planJSON), 0o600))
	out := filepath.Join(dir, "roundtrip.mbcap")
	_, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: config.DeviceConfig{URL: url, Unit: 1}, Timeout: timeoutMS, Function: 3,
		Interval: interval, Duration: duration, BlocksFile: pf, OutputFile: out,
	}, nil)
	require.NoError(t, err)
	return out
}

// rtLoad loads a capture and returns its responses per request PDU, in time
// order. Every sample must be a supported read.
func rtLoad(t *testing.T, path string) map[string][][]byte {
	t.Helper()
	l, err := capfile.Load(path)
	require.NoError(t, err)
	require.False(t, l.Truncated)
	out := map[string][][]byte{}
	for _, s := range l.Samples {
		require.Truef(t, capfile.IsSpecRead(s.Request), "sample % X", s.Request)
		require.Falsef(t, s.IsException(), "exception sample % X", s.Response)
		require.NoError(t, capture.CheckPDUs(s.Request, s.Response))
		out[string(s.Request)] = append(out[string(s.Request)], s.Response)
	}
	return out
}

// mbcapctl is go-modbus-capture's command-line tool, built once per test
// binary by mbcapctlPath and removed by TestMain.
var mbcapctl struct {
	once sync.Once
	dir  string
	path string
	err  error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if mbcapctl.dir != "" {
		_ = os.RemoveAll(mbcapctl.dir)
	}
	os.Exit(code)
}

// mbcapctlPath returns the path of a built mbcapctl, skipping the test when
// the go tool is not available to build it.
func mbcapctlPath(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH: cannot build mbcapctl")
	}
	mbcapctl.once.Do(func() {
		if mbcapctl.dir, mbcapctl.err = os.MkdirTemp("", "mbcapctl"); mbcapctl.err != nil {
			return
		}
		mbcapctl.path = filepath.Join(mbcapctl.dir, "mbcapctl")
		out, err := exec.Command(goBin, "build", "-o", mbcapctl.path, "github.com/otfabric/go-modbus-capture/cmd/mbcapctl").CombinedOutput()
		if err != nil {
			mbcapctl.err = fmt.Errorf("building mbcapctl: %w: %s", err, out)
		}
	})
	require.NoError(t, mbcapctl.err)
	return mbcapctl.path
}

// mbcapctlVerify runs go-modbus-capture's `mbcapctl verify` on paths.
func mbcapctlVerify(t *testing.T, paths ...string) {
	t.Helper()
	out, err := exec.Command(mbcapctlPath(t), append([]string{"verify"}, paths...)...).CombinedOutput()
	require.NoErrorf(t, err, "mbcapctl verify: %s", out)
	for _, p := range paths {
		require.Containsf(t, string(out), p+": ok", "mbcapctl verify: %s", out)
	}
}

// rtAsk sends every request of reqs to c (tapped by ex) and returns the
// exchanges of this round, several for an FC43 read that pages.
func rtAsk(t *testing.T, c *gomodbus.Client, ex *e2eExchanges, reqs []plan.Request) []gomodbus.PDUExchange {
	t.Helper()
	from := ex.len()
	for i := range reqs {
		require.NoErrorf(t, callPlanRequest(context.Background(), c, 1, &reqs[i]), "%s", reqs[i].String())
	}
	got := ex.since(from)
	require.GreaterOrEqual(t, len(got), len(reqs))
	return got
}

// rtRequireAll checks that the capture holds samples of every function of
// rtPlan, at least min of each request.
func rtRequireAll(t *testing.T, recorded map[string][][]byte, reqs []plan.Request, min int) {
	t.Helper()
	for i := range reqs {
		require.GreaterOrEqualf(t, len(recorded[string(reqs[i].PDU())]), min, "samples of %s", reqs[i].String())
	}
	fcs := map[uint8]bool{}
	for k := range recorded {
		fcs[k[0]] = true
	}
	for _, fc := range rtFunctions {
		require.Truef(t, fcs[fc], "FC%d recorded", fc)
	}
}

func TestRoundTrip_everyFunction(t *testing.T) {
	reqs := rtRequests(t)
	// duration (ms) leaves room for at least two rounds of the plan: RTU framing
	// over TCP is slower than the others.
	sources := []struct {
		name     string
		url      func(t *testing.T) string
		duration uint32
	}{
		{"go-modbus server", rtServeGoModbus, 160},
		{"mbtest tcp", func(t *testing.T) string { return "tcp://" + rtDevice().ServeTCP(t) }, 160},
		{"mbtest rtuovertcp", func(t *testing.T) string { return "rtuovertcp://" + rtDevice().ServeRTUOverTCP(t) }, 1500},
		{"mbtest udp", func(t *testing.T) string { return "udp://" + rtDevice().ServeUDP(t) }, 160},
	}
	for _, src := range sources {
		t.Run(src.name, func(t *testing.T) {
			path := rtRecord(t, src.url(t), rtPlan, 40, src.duration, coreTimeoutMS)
			mbcapctlVerify(t, path)
			recorded := rtLoad(t, path)
			rtRequireAll(t, recorded, reqs, 2)

			t.Run("static", func(t *testing.T) {
				srv, progress := staticServer(t, config.StaticServerConfig{InputFile: path})
				var ex e2eExchanges
				c := e2eFunctionsClient(t, fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), &ex)
				for range 2 {
					requireRecordedLast(t, recorded, rtAsk(t, c, &ex, reqs))
				}
				require.ErrorIs(t, srv.stop(t), context.Canceled)
				require.Contains(t, progress.String(), "0 with an exception, 0 left unanswered; 0 asked for data the capture does not hold")
			})

			t.Run("replay", func(t *testing.T) {
				l, err := capfile.Load(path)
				require.NoError(t, err)
				span := l.Header.Duration(l.End())
				srv := collStartServer(t, func(ctx context.Context, a collAddr) error {
					return ServeReplay(ctx, config.ReplayServerConfig{Port: a.Port, Listen: a.Listen, InputFile: path, Speed: "1", Loops: 1}, a.Progress(nil))
				})
				started := time.Now()
				var ex e2eExchanges
				c := e2eFunctionsClient(t, fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), &ex)
				changed := map[string]map[string]bool{}
				for time.Since(started) < span {
					for _, e := range rtAsk(t, c, &ex, reqs) {
						found := false
						for _, r := range recorded[string(e.Request)] {
							found = found || bytes.Equal(r, e.Response)
						}
						require.Truef(t, found, "request % X answered % X, never recorded for it", e.Request, e.Response)
						if changed[string(e.Request)] == nil {
							changed[string(e.Request)] = map[string]bool{}
						}
						changed[string(e.Request)][string(e.Response)] = true
					}
				}
				// Once the single loop is over, the last values stay.
				time.Sleep(span - time.Since(started) + 100*time.Millisecond)
				requireRecordedLast(t, recorded, rtAsk(t, c, &ex, reqs))
				require.ErrorIs(t, srv.stop(t), context.Canceled)
			})
		})
	}
}

// The recorded file round-trips through every MBCAP encoding: the servers
// load .mbcap.jsonl and .mbcap.csv conversions of it and answer the same.
func TestRoundTrip_textEncodings(t *testing.T) {
	reqs := rtRequests(t)
	path := rtRecord(t, rtServeGoModbus(t), rtPlan, 30, 90, coreTimeoutMS)
	recorded := rtLoad(t, path)
	for _, ext := range []string{".mbcap.jsonl", ".mbcap.csv"} {
		conv := strings.TrimSuffix(path, ".mbcap") + ext
		rtConvert(t, path, conv)
		mbcapctlVerify(t, conv)
		require.Equal(t, recorded, rtLoad(t, conv), ext)
		srv, _ := staticServer(t, config.StaticServerConfig{InputFile: conv})
		var ex e2eExchanges
		c := e2eFunctionsClient(t, fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), &ex)
		requireRecordedLast(t, recorded, rtAsk(t, c, &ex, reqs))
	}
}

// rtConvert converts the capture at in to out (encoding from the extension)
// with mbcapctl convert.
func rtConvert(t *testing.T, in, out string) {
	t.Helper()
	msg, err := exec.Command(mbcapctlPath(t), "convert", in, out).CombinedOutput()
	require.NoErrorf(t, err, "mbcapctl convert: %s", msg)
}

// Chaos: a device that answers every function busy, not at all, after closing
// the connection, malformed, late or normally, at random. The capture keeps
// exactly the valid answers the device sent, passes mbcapctl verify, and the
// servers answer only with recorded responses.
func TestChaosRoundTrip_everyFunction(t *testing.T) {
	reqs := rtRequests(t)
	for _, tr := range []struct {
		name   string
		stream bool
		serve  func(d *mbtest.Device, t testing.TB) string
	}{
		{"tcp", true, func(d *mbtest.Device, t testing.TB) string { return "tcp://" + d.ServeTCP(t) }},
		{"rtuovertcp", true, func(d *mbtest.Device, t testing.TB) string { return "rtuovertcp://" + d.ServeRTUOverTCP(t) }},
	} {
		t.Run(tr.name, func(t *testing.T) {
			d := mbtest.NewDevice()
			var mu sync.Mutex
			rng := rand.New(rand.NewSource(42))
			counts := map[string]uint16{}
			sent := map[string]bool{} // request + valid response the device sent
			d.OnRequest = func(r mbtest.Request) mbtest.Action {
				mu.Lock()
				defer mu.Unlock()
				switch x := rng.Intn(100); {
				case x < 12:
					return mbtest.Action{Exception: mbtest.ExServerDeviceBusy}
				case x < 18:
					return mbtest.Action{Drop: true}
				case x < 22 && tr.stream:
					return mbtest.Action{Close: true}
				case x < 26:
					// Truncated: the client rejects it, so it must not be recorded.
					counts[string(r.PDU)]++
					good := rtRespond(r.PDU, counts[string(r.PDU)])
					return mbtest.Action{Reply: good[:len(good)-1]}
				case x < 30:
					counts[string(r.PDU)]++
					resp := rtRespond(r.PDU, counts[string(r.PDU)])
					sent[string(r.PDU)+"|"+string(resp)] = true
					return mbtest.Action{Reply: resp, Delay: 15 * time.Millisecond}
				}
				counts[string(r.PDU)]++
				resp := rtRespond(r.PDU, counts[string(r.PDU)])
				sent[string(r.PDU)+"|"+string(resp)] = true
				return mbtest.Action{Reply: resp}
			}
			path := rtRecord(t, tr.serve(d, t), rtPlan, 0, 1500, 150)
			mbcapctlVerify(t, path)
			recorded := rtLoad(t, path)
			rtRequireAll(t, recorded, reqs, 1)
			mu.Lock()
			for req, resps := range recorded {
				for _, resp := range resps {
					require.Truef(t, sent[req+"|"+string(resp)], "recorded % X -> % X, which the device never sent as a valid answer", []byte(req), resp)
				}
			}
			mu.Unlock()

			srv, _ := staticServer(t, config.StaticServerConfig{InputFile: path})
			var ex e2eExchanges
			c := e2eFunctionsClient(t, fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), &ex)
			requireRecordedLast(t, recorded, rtAsk(t, c, &ex, reqs))
		})
	}
}

// Parallel: several devices are recorded at once, then many clients ask every
// function of every recording from static and replay servers at once.
func TestParallelRoundTrip_everyFunction(t *testing.T) {
	reqs := rtRequests(t)
	const devices = 4
	paths := make([]string, devices)
	var wg sync.WaitGroup
	urls := make([]string, devices)
	for i := range urls {
		if i%2 == 0 {
			urls[i] = rtServeGoModbus(t)
		} else {
			urls[i] = "tcp://" + rtDevice().ServeTCP(t)
		}
	}
	for i := range devices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			paths[i] = rtRecord(t, urls[i], rtPlan, 20, 120, coreTimeoutMS)
		}()
	}
	wg.Wait()
	mbcapctlVerify(t, paths...)

	for i, path := range paths {
		recorded := rtLoad(t, path)
		rtRequireAll(t, recorded, reqs, 2)
		static, _ := staticServer(t, config.StaticServerConfig{InputFile: path, MaxClients: 20})
		replaySrv := collStartServer(t, func(ctx context.Context, a collAddr) error {
			return ServeReplay(ctx, config.ReplayServerConfig{Port: a.Port, Listen: a.Listen, InputFile: path, Speed: "1", MaxClients: 20}, a.Progress(nil))
		})
		for w := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				port, isStatic := static.port, w%2 == 0
				if !isStatic {
					port = replaySrv.port
				}
				var ex e2eExchanges
				c, err := gomodbus.New(gomodbus.Config{URL: fmt.Sprintf("tcp://127.0.0.1:%d", port), Timeout: 2 * time.Second,
					Logger: gomodbus.NopLogger(), PDUTap: &ex})
				if err != nil || c.Open() != nil {
					t.Errorf("device %d worker %d: client: %v", i, w, err)
					return
				}
				defer func() { _ = c.Close() }()
				for round := range 10 {
					from := ex.len()
					for k := range reqs {
						r := &reqs[(k+round+w)%len(reqs)]
						if err := callPlanRequest(context.Background(), c, 1, r); err != nil {
							t.Errorf("device %d worker %d %s: %v", i, w, r.String(), err)
							return
						}
					}
					for _, e := range ex.since(from) {
						resps := recorded[string(e.Request)]
						if isStatic && (len(resps) == 0 || !bytes.Equal(resps[len(resps)-1], e.Response)) {
							t.Errorf("static device %d: % X answered % X, not the last recorded response", i, e.Request, e.Response)
						}
						found := false
						for _, r := range resps {
							found = found || bytes.Equal(r, e.Response)
						}
						if !found {
							t.Errorf("device %d: % X answered % X, never recorded for it", i, e.Request, e.Response)
						}
					}
				}
			}()
		}
	}
	wg.Wait()
}
