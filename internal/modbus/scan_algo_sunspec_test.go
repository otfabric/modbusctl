package modbus

import (
	"encoding/binary"
	"testing"

	"github.com/otfabric/go-modbus/sunspec"
	"github.com/otfabric/modbusctl/internal/config"
)

// makeSunSpecData encodes two uint16 values into a 4-byte big-endian slice.
func makeSunSpecData(r0, r1 uint16) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:2], r0)
	binary.BigEndian.PutUint16(b[2:4], r1)
	return b
}

// makeSunSpecBodyData creates a byte slice of n registers (2*n bytes) with sequential values.
func makeSunSpecBodyData(n uint16) []byte {
	b := make([]byte, int(n)*2)
	for i := uint16(0); i < n; i++ {
		binary.BigEndian.PutUint16(b[i*2:i*2+2], i+1)
	}
	return b
}

// drainBody drives body-reading tasks to completion and returns the number of body reads.
func drainBody(t *testing.T, s *sunspecStrategy, length uint16) {
	t.Helper()
	remaining := length
	for remaining > 0 {
		task, ok := s.Next()
		if !ok {
			t.Fatalf("expected body read task, remaining=%d", remaining)
		}
		expected := remaining
		if expected > 125 {
			expected = 125
		}
		if task.Count != expected {
			t.Errorf("body task count = %d, want %d", task.Count, expected)
		}
		s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecBodyData(task.Count)})
		remaining -= task.Count
	}
}

func sunSpecMarkerData() []byte {
	return makeSunSpecData(sunspec.MarkerReg0, sunspec.MarkerReg1)
}

func sunSpecEndModelData() []byte {
	return makeSunSpecData(sunspec.EndModelID, sunspec.EndModelLength)
}

func TestSunSpec_NonSunSpecDevice(t *testing.T) {
	cfg := config.ScanConfig{}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Should probe all default bases, all fail.
	count := 0
	for !s.Done() {
		task, ok := s.Next()
		if !ok {
			break
		}
		count++
		s.OnResult(task, ScanResult{Success: false})
	}
	if count != len(sunspec.DefaultBaseAddresses) {
		t.Errorf("expected %d base probes, got %d", len(sunspec.DefaultBaseAddresses), count)
	}
	if !s.Done() {
		t.Error("expected Done() after all bases failed")
	}
}

func TestSunSpec_DetectAt40000(t *testing.T) {
	cfg := config.ScanConfig{}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Default bases: [0, 40000, 50000, 1, 39999, 40001, 49999, 50001]
	// First probe (base=0) fails, second (base=40000) succeeds with SunS marker.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected first probe task")
	}
	if task.Start != sunspec.DefaultBaseAddresses[0] {
		t.Errorf("first probe start = %d, want %d", task.Start, sunspec.DefaultBaseAddresses[0])
	}
	s.OnResult(task, ScanResult{Success: false})

	task, ok = s.Next()
	if !ok {
		t.Fatal("expected second probe task")
	}
	if task.Start != 40000 {
		t.Errorf("second probe start = %d, want 40000", task.Start)
	}
	s.OnResult(task, ScanResult{Success: true, Data: sunSpecMarkerData()})

	// Now in walk phase. Walk 2 model headers (with body) + end model.
	models := []struct {
		id     uint16
		length uint16
	}{
		{1, 66},   // Common model
		{201, 50}, // AC meter
	}

	expectedAddr := uint16(40002)
	for i, m := range models {
		// Header read.
		task, ok = s.Next()
		if !ok {
			t.Fatalf("expected header task %d", i)
		}
		if task.Start != expectedAddr {
			t.Errorf("header task %d: start = %d, want %d", i, task.Start, expectedAddr)
		}
		if task.Count != 2 {
			t.Errorf("header task %d: count = %d, want 2", i, task.Count)
		}
		s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecData(m.id, m.length)})

		// Body reads.
		drainBody(t, s, m.length)

		expectedAddr += 2 + m.length
	}

	// End model header.
	task, ok = s.Next()
	if !ok {
		t.Fatal("expected end model header task")
	}
	if task.Start != expectedAddr {
		t.Errorf("end model task: start = %d, want %d", task.Start, expectedAddr)
	}
	s.OnResult(task, ScanResult{Success: true, Data: sunSpecEndModelData()})

	if !s.Done() {
		t.Error("expected Done() after end model")
	}
}

func TestSunSpec_KnownBase(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBase: 40000}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Should skip detection and start walking immediately at 40002.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected walk task immediately")
	}
	if task.Start != 40002 {
		t.Errorf("first walk task start = %d, want 40002", task.Start)
	}

	// End model immediately.
	s.OnResult(task, ScanResult{Success: true, Data: sunSpecEndModelData()})
	if !s.Done() {
		t.Error("expected Done() after end model")
	}
}

func TestSunSpec_MaxModelsLimit(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBase: 40000, SunSpecMaxModels: 2}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Walk 2 models (header + body each), then should stop even though chain continues.
	addr := uint16(40002)
	for i := 0; i < 2; i++ {
		task, ok := s.Next()
		if !ok {
			t.Fatalf("expected header task %d", i)
		}
		if task.Start != addr {
			t.Errorf("header task %d: start = %d, want %d", i, task.Start, addr)
		}
		length := uint16(10)
		s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecData(uint16(100+i), length)})
		drainBody(t, s, length)
		addr += 2 + length
	}

	// Third call should stop due to maxModels.
	_, ok := s.Next()
	if ok {
		t.Error("expected no more tasks after maxModels reached")
	}
	if !s.Done() {
		t.Error("expected Done() after maxModels")
	}
}

func TestSunSpec_MaxSpanLimit(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBase: 40000, SunSpecMaxSpan: 10}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Walk one model with length 10 → next addr = 40002+2+10 = 40014.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected first walk task")
	}
	s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecData(100, 10)})
	drainBody(t, s, 10)

	// currentAddr = 40014, span = 40014 - 40000 = 14 > 10 → should stop.
	_, ok = s.Next()
	if ok {
		t.Error("expected no more tasks after maxSpan exceeded")
	}
	if !s.Done() {
		t.Error("expected Done() after maxSpan exceeded")
	}
}

func TestSunSpec_MalformedChain(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBase: 40000}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// First model header: length=0 but ID is not end model → malformed.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected first walk task")
	}
	s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecData(100, 0)})

	if !s.Done() {
		t.Error("expected Done() after malformed chain (length=0 non-end)")
	}
}

func TestSunSpec_AddressOverflow(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBase: 65530}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Walk from 65532 with a model of length 10 → 65532 + 2 + 10 = 65544 > 65535.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected first walk task")
	}
	if task.Start != 65532 {
		t.Errorf("first walk task start = %d, want 65532", task.Start)
	}
	s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecData(100, 10)})

	if !s.Done() {
		t.Error("expected Done() after address overflow")
	}
}

func TestSunSpec_ReadFailureDuringWalk(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBase: 40000}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// First model header succeeds.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected first walk task")
	}
	s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecData(1, 66)})

	// Drain body for model 1.
	drainBody(t, s, 66)

	// Second header read fails.
	task, ok = s.Next()
	if !ok {
		t.Fatal("expected second walk task")
	}
	s.OnResult(task, ScanResult{Success: false})

	if !s.Done() {
		t.Error("expected Done() after read failure during walk")
	}
}

func TestSunSpec_EmptyBases(t *testing.T) {
	// Custom empty bases + no known base → immediately done.
	cfg := config.ScanConfig{SunSpecBases: ""}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Empty string falls back to default bases, so it should NOT be immediately done.
	if s.Done() {
		t.Error("empty string bases should fall back to defaults, not be done immediately")
	}
}

func TestSunSpec_CustomBases(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBases: "100,200"}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Should probe exactly 100 and 200.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected first probe")
	}
	if task.Start != 100 {
		t.Errorf("first probe start = %d, want 100", task.Start)
	}
	s.OnResult(task, ScanResult{Success: false})

	task, ok = s.Next()
	if !ok {
		t.Fatal("expected second probe")
	}
	if task.Start != 200 {
		t.Errorf("second probe start = %d, want 200", task.Start)
	}
	s.OnResult(task, ScanResult{Success: false})

	if !s.Done() {
		t.Error("expected Done() after all custom bases failed")
	}
}

func TestNewScanStrategy_Sunspec(t *testing.T) {
	cfg := config.ScanConfig{Algo: "sunspec"}
	st, err := newScanStrategy(ss(cfg))
	if err != nil {
		t.Fatalf("newScanStrategy(sunspec): %v", err)
	}
	if st.Name() != "sunspec" {
		t.Errorf("strategy Name = %q, want sunspec", st.Name())
	}
}

func TestSunSpec_BodyReadFailure(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBase: 40000}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Header read succeeds.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected header task")
	}
	s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecData(1, 66)})

	// First body chunk fails.
	task, ok = s.Next()
	if !ok {
		t.Fatal("expected body task")
	}
	if task.Count != 66 {
		t.Errorf("body task count = %d, want 66", task.Count)
	}
	s.OnResult(task, ScanResult{Success: false})

	if !s.Done() {
		t.Error("expected Done() after body read failure")
	}
}

func TestSunSpec_LargeModelBodyChunking(t *testing.T) {
	cfg := config.ScanConfig{SunSpecBase: 40000}
	s := newSunSpecStrategy(ss(cfg))
	s.Init(ss(cfg))

	// Header with length 200 → should be read in chunks of 125 + 75.
	task, ok := s.Next()
	if !ok {
		t.Fatal("expected header task")
	}
	s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecData(1, 200)})

	// First body chunk: 125 starting at 40004.
	task, ok = s.Next()
	if !ok {
		t.Fatal("expected first body chunk")
	}
	if task.Start != 40004 || task.Count != 125 {
		t.Errorf("first body chunk = (%d, %d), want (40004, 125)", task.Start, task.Count)
	}
	s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecBodyData(125)})

	// Second body chunk: 75 starting at 40129.
	task, ok = s.Next()
	if !ok {
		t.Fatal("expected second body chunk")
	}
	if task.Start != 40129 || task.Count != 75 {
		t.Errorf("second body chunk = (%d, %d), want (40129, 75)", task.Start, task.Count)
	}
	s.OnResult(task, ScanResult{Success: true, Data: makeSunSpecBodyData(75)})

	// Next should be the next header read at 40002+2+200 = 40204.
	task, ok = s.Next()
	if !ok {
		t.Fatal("expected next header task")
	}
	if task.Start != 40204 || task.Count != 2 {
		t.Errorf("next header = (%d, %d), want (40204, 2)", task.Start, task.Count)
	}
	// End model.
	s.OnResult(task, ScanResult{Success: true, Data: sunSpecEndModelData()})
	if !s.Done() {
		t.Error("expected Done() after end model")
	}
}
