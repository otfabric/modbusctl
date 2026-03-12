package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/otfabric/modbus"
)

type RegisterStore interface {
	get(fc uint8, start uint16, count uint16) ([]byte, error)
}

type MemoryStore struct {
	mu        sync.RWMutex
	registers map[uint16][]byte
	coils     map[uint16][]byte
}

type ReplayController struct {
	store   *MemoryStore
	records []types.CaptureRecord
	start   time.Time
	loop    int
}

type ServerConfig struct {
	Port     uint16
	Unit     uint8
	Function uint8
}

type StaticHandler struct {
	Store RegisterStore
	Cfg   ServerConfig
}

func writeRegistersToStore(store *MemoryStore, rec types.CaptureRecord, fc uint8) error {
	if fc != 1 && len(rec.Data) != int(rec.RegisterCount)*2 {
		return fmt.Errorf("record %d has mismatched data size", rec.StartAddress)
	}
	for i := uint16(0); i < rec.RegisterCount; i++ {
		offset := i * 2
		if fc == 1 {
			store.coils[rec.StartAddress+i] = rec.Data[i : i+1]
		} else {
			store.registers[rec.StartAddress+i] = rec.Data[offset : offset+2]
		}
	}
	return nil
}

func newMemoryStoreLastValues(records []types.CaptureRecord, fc uint8) (*MemoryStore, error) {
	s := &MemoryStore{
		registers: make(map[uint16][]byte),
		coils:     make(map[uint16][]byte),
	}
	for _, rec := range records {
		if err := writeRegistersToStore(s, rec, fc); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func newMemoryStoreFirstValues(records []types.CaptureRecord, fc uint8) *MemoryStore {
	store := &MemoryStore{
		registers: make(map[uint16][]byte),
		coils:     make(map[uint16][]byte),
	}
	if len(records) == 0 {
		return store
	}
	firstIteration := records[0].Iteration
	for _, rec := range records {
		if rec.Iteration != firstIteration {
			continue
		}
		if err := writeRegistersToStore(store, rec, fc); err != nil {
			fmt.Printf("⚠️ Failed to write record at address %d: %v\n", rec.StartAddress, err)
		}
	}
	return store
}

func (s *MemoryStore) get(fc uint8, start uint16, count uint16) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []byte
	for i := range count {
		var chunk []byte
		var ok bool
		switch fc {
		case 1:
			chunk, ok = s.coils[start+i]
		case 3, 4:
			chunk, ok = s.registers[start+i]
		default:
			return nil, modbus.ErrIllegalFunction
		}
		if !ok {
			return nil, modbus.ErrIllegalDataAddress
		}
		out = append(out, chunk...)
	}
	return out, nil
}

func newModbusServer(store RegisterStore, cfg ServerConfig) error {
	handler := &StaticHandler{
		Store: store,
		Cfg:   cfg,
	}

	server, err := modbus.NewServer(&modbus.ServerConfiguration{
		URL:     fmt.Sprintf("tcp://0.0.0.0:%d", cfg.Port),
		Timeout: 0,
	}, handler)
	if err != nil {
		return fmt.Errorf("failed to start modbus server: %w", err)
	}

	fmt.Printf("🚀 Serving Modbus TCP on port %d for unit %d\n", cfg.Port, cfg.Unit)

	err = server.Start()
	if err != nil {
		return fmt.Errorf("modbus server failed: %w", err)
	}

	// 🔒 Prevent exit
	select {}
}

func (h *StaticHandler) HandleCoils(ctx context.Context, req *modbus.CoilsRequest) ([]bool, error) {
	if req.UnitId != h.Cfg.Unit {
		return nil, modbus.ErrBadUnitId
	}
	data, err := h.Store.get(1, req.Addr, req.Quantity)
	if err != nil {
		return nil, modbus.ErrIllegalDataAddress
	}
	out := make([]bool, 0, req.Quantity)
	for _, b := range data {
		for i := 0; i < 8 && len(out) < int(req.Quantity); i++ {
			out = append(out, (b&(1<<i)) != 0)
		}
	}
	return out, nil
}

func (h *StaticHandler) HandleDiscreteInputs(ctx context.Context, req *modbus.DiscreteInputsRequest) ([]bool, error) {
	return nil, modbus.ErrIllegalFunction
}

func (h *StaticHandler) HandleHoldingRegisters(ctx context.Context, req *modbus.HoldingRegistersRequest) ([]uint16, error) {
	if req.UnitId != h.Cfg.Unit {
		return nil, modbus.ErrBadUnitId
	}
	data, err := h.Store.get(3, req.Addr, req.Quantity)
	if err != nil {
		return nil, modbus.ErrIllegalDataAddress
	}
	var out []uint16
	for i := 0; i+1 < len(data); i += 2 {
		val := binary.BigEndian.Uint16(data[i : i+2])
		out = append(out, val)
	}
	return out, nil
}

func (h *StaticHandler) HandleInputRegisters(ctx context.Context, req *modbus.InputRegistersRequest) ([]uint16, error) {
	if req.UnitId != h.Cfg.Unit {
		return nil, modbus.ErrBadUnitId
	}
	data, err := h.Store.get(4, req.Addr, req.Quantity)
	if err != nil {
		return nil, modbus.ErrIllegalDataAddress
	}
	var out []uint16
	for i := 0; i+1 < len(data); i += 2 {
		val := binary.BigEndian.Uint16(data[i : i+2])
		out = append(out, val)
	}
	return out, nil
}

func LoadMCAPAndServeStatic(cfg config.StaticServerConfig) error {
	records, header, err := format.LoadRecordsFromMCAP(cfg.InputFile)
	if err != nil {
		return fmt.Errorf("failed to load MCAP file: %w", err)
	}

	store, err := newMemoryStoreLastValues(records, header.Function)
	if err != nil {
		return fmt.Errorf("failed to initialize static store: %w", err)
	}

	fmt.Printf("✅ Static server ready on port %d for unit %d and function %d\n", cfg.Port, cfg.Unit, header.Function)
	fmt.Printf("🧱 Loaded %d blocks from MCAP\n", len(store.registers)+len(store.coils))
	return newModbusServer(store, ServerConfig{
		Port:     cfg.Port,
		Unit:     cfg.Unit,
		Function: header.Function,
	})
}

func LoadMCAPAndServeReplay(cfg config.ReplayServerConfig) error {
	records, header, err := format.LoadRecordsFromMCAP(cfg.InputFile)
	if err != nil {
		return fmt.Errorf("failed to load MCAP file: %w", err)
	}
	if header.Function != cfg.Function {
		return fmt.Errorf("function code mismatch: header FC=%d, config FC=%d", header.Function, cfg.Function)
	}

	store := newMemoryStoreFirstValues(records, header.Function)
	controller := &ReplayController{
		store:   store,
		records: records,
		start:   time.Unix(0, header.StartTime),
		loop:    int(cfg.Loops),
	}

	go func() {
		grouped := make(map[uint32][]types.CaptureRecord)
		var iterations []uint32
		for _, rec := range controller.records {
			if _, ok := grouped[rec.Iteration]; !ok {
				iterations = append(iterations, rec.Iteration)
			}
			grouped[rec.Iteration] = append(grouped[rec.Iteration], rec)
		}
		slices.Sort(iterations)

		var lastIterationStart int64
		for loop := 0; controller.loop == 0 || loop < controller.loop; loop++ {
			fmt.Printf("🔄 Starting replay loop %d/%d\n", loop+1, controller.loop)
			for _, iter := range iterations {
				fmt.Printf("  Processing iteration %d with %d records\n", iter, len(grouped[iter]))
				records := grouped[iter]
				startTs := records[0].RequestTimestamp
				var delay time.Duration
				if lastIterationStart > 0 {
					if cfg.Interval == 0 {
						delay = time.Duration(startTs - lastIterationStart)
					} else {
						delay = time.Duration(cfg.Interval) * time.Millisecond
					}
					if delay > 0 {
						time.Sleep(delay)
					}
				}
				lastIterationStart = startTs
				for _, rec := range records {
					for i := uint16(0); i < rec.RegisterCount; i++ {
						offset := i * 2
						if cfg.Function == 1 {
							store.coils[rec.StartAddress+i] = rec.Data[i : i+1]
						} else {
							store.registers[rec.StartAddress+i] = rec.Data[offset : offset+2]
						}
					}
				}
			}
		}
	}()

	fmt.Printf("✅ Replay server ready on port %d for unit %d and function %d\n", cfg.Port, cfg.Unit, header.Function)
	fmt.Printf("🧱 Loaded %d blocks from MCAP\n", len(store.registers)+len(store.coils))
	return newModbusServer(store, ServerConfig{
		Port:     cfg.Port,
		Unit:     cfg.Unit,
		Function: cfg.Function,
	})
}
