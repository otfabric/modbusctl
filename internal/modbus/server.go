package modbus

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/types"
)

type RegisterStore interface {
	get(fc uint8, start uint16, count uint16) ([]byte, error)
}

type MemoryStore struct {
	mu        sync.RWMutex
	registers map[uint16][]byte
	coils     map[uint16][]byte // FC01
	discrete  map[uint16][]byte // FC02 (packed bits, same layout as coils)
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
	Debug    bool
}

type StaticHandler struct {
	Store RegisterStore
	Cfg   ServerConfig
}

func writeRegistersToStore(store *MemoryStore, rec types.CaptureRecord, fc uint8) error {
	if fc == 1 || fc == 2 {
		// FC01/FC02 response data is packed: one bit per point, LSB-first within each byte (same as [packCoilsToBytes]).
		packLen := (int(rec.RegisterCount) + 7) / 8
		if packLen == 0 || len(rec.Data) < packLen {
			return fmt.Errorf("record at address %d: FC%d data length %d incompatible with point count %d", rec.StartAddress, fc, len(rec.Data), rec.RegisterCount)
		}
		pack := make([]byte, packLen)
		copy(pack, rec.Data[:packLen])
		dest := store.coils
		if fc == 2 {
			dest = store.discrete
		}
		for i := uint16(0); i < rec.RegisterCount; i++ {
			byteIdx := i / 8
			bitIdx := i % 8
			bit := (pack[byteIdx] >> bitIdx) & 1
			dest[rec.StartAddress+i] = []byte{bit}
		}
		return nil
	}
	if len(rec.Data) != int(rec.RegisterCount)*2 {
		return fmt.Errorf("record %d has mismatched data size", rec.StartAddress)
	}
	for i := uint16(0); i < rec.RegisterCount; i++ {
		offset := i * 2
		chunk := make([]byte, 2)
		copy(chunk, rec.Data[offset:offset+2])
		store.registers[rec.StartAddress+i] = chunk
	}
	return nil
}

func newMemoryStoreLastValues(records []types.CaptureRecord, fc uint8) (*MemoryStore, error) {
	s := &MemoryStore{
		registers: make(map[uint16][]byte),
		coils:     make(map[uint16][]byte),
		discrete:  make(map[uint16][]byte),
	}
	for _, rec := range records {
		if err := writeRegistersToStore(s, rec, fc); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func newMemoryStoreFirstValues(records []types.CaptureRecord, fc uint8, progress io.Writer) *MemoryStore {
	if progress == nil {
		progress = io.Discard
	}
	store := &MemoryStore{
		registers: make(map[uint16][]byte),
		coils:     make(map[uint16][]byte),
		discrete:  make(map[uint16][]byte),
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
			_, _ = fmt.Fprintf(progress, "⚠️ Failed to write record at address %d: %v\n", rec.StartAddress, err)
		}
	}
	return store
}

// upsertRecord merges one capture record into the store (used by replay updates; takes write lock).
func (s *MemoryStore) upsertRecord(rec types.CaptureRecord, fc uint8) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeRegistersToStore(s, rec, fc)
}

func (s *MemoryStore) get(fc uint8, start uint16, count uint16) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	capacity := int(count)
	if fc == 3 || fc == 4 {
		capacity = int(count) * 2
	}
	out := make([]byte, 0, capacity)
	for i := uint16(0); i < count; i++ {
		var chunk []byte
		var ok bool
		switch fc {
		case 1:
			chunk, ok = s.coils[start+i]
		case 2:
			chunk, ok = s.discrete[start+i]
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

func newModbusServer(ctx context.Context, store RegisterStore, cfg ServerConfig, progress io.Writer) error {
	if progress == nil {
		progress = io.Discard
	}
	handler := &StaticHandler{
		Store: store,
		Cfg:   cfg,
	}

	logger := modbus.NopLogger()
	if cfg.Debug {
		logger = modbus.NewStdLogger(nil)
	}
	server, err := modbus.NewServer(&modbus.ServerConfig{
		URL:     fmt.Sprintf("tcp://0.0.0.0:%d", cfg.Port),
		Timeout: 0,
		Logger:  logger,
	}, handler)
	if err != nil {
		return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "failed to start modbus server: "+err.Error(), err)
	}

	_, _ = fmt.Fprintf(progress, "🚀 Serving Modbus TCP on port %d for unit %d\n", cfg.Port, cfg.Unit)

	err = server.Start()
	if err != nil {
		return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "modbus server failed: "+err.Error(), err)
	}

	// Block until context canceled (e.g. SIGINT/SIGTERM via root Execute).
	<-ctx.Done()
	// Close listener and client sockets (go-modbus); ignore error—return cancel reason for CLI exit handling.
	_ = server.Stop()
	return ctx.Err()
}

func (h *StaticHandler) HandleCoils(ctx context.Context, req *modbus.CoilsRequest) ([]bool, error) {
	if h.Cfg.Function != 1 {
		return nil, modbus.ErrIllegalFunction
	}
	if req.UnitID != h.Cfg.Unit {
		return nil, modbus.ErrBadUnitID
	}
	data, err := h.Store.get(1, req.Addr, req.Quantity)
	if err != nil {
		if errors.Is(err, modbus.ErrIllegalFunction) {
			return nil, err
		}
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
	if h.Cfg.Function != 2 {
		return nil, modbus.ErrIllegalFunction
	}
	if req.UnitID != h.Cfg.Unit {
		return nil, modbus.ErrBadUnitID
	}
	data, err := h.Store.get(2, req.Addr, req.Quantity)
	if err != nil {
		if errors.Is(err, modbus.ErrIllegalFunction) {
			return nil, err
		}
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

func (h *StaticHandler) HandleHoldingRegisters(ctx context.Context, req *modbus.HoldingRegistersRequest) ([]uint16, error) {
	if h.Cfg.Function != 3 {
		return nil, modbus.ErrIllegalFunction
	}
	if req.UnitID != h.Cfg.Unit {
		return nil, modbus.ErrBadUnitID
	}
	data, err := h.Store.get(3, req.Addr, req.Quantity)
	if err != nil {
		if errors.Is(err, modbus.ErrIllegalFunction) {
			return nil, err
		}
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
	if h.Cfg.Function != 4 {
		return nil, modbus.ErrIllegalFunction
	}
	if req.UnitID != h.Cfg.Unit {
		return nil, modbus.ErrBadUnitID
	}
	data, err := h.Store.get(4, req.Addr, req.Quantity)
	if err != nil {
		if errors.Is(err, modbus.ErrIllegalFunction) {
			return nil, err
		}
		return nil, modbus.ErrIllegalDataAddress
	}
	var out []uint16
	for i := 0; i+1 < len(data); i += 2 {
		val := binary.BigEndian.Uint16(data[i : i+2])
		out = append(out, val)
	}
	return out, nil
}

func LoadMCAPAndServeStatic(ctx context.Context, cfg config.StaticServerConfig, progress io.Writer) error {
	if progress == nil {
		progress = io.Discard
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	records, header, err := mcap.LoadRecordsFromMCAP(cfg.InputFile)
	if err != nil {
		return errs.Output(errs.CodeMCAPLoadFailed, err)
	}

	if cfg.Function != 0 && cfg.Function != header.Function {
		return errs.InvalidInput(errs.CodeInvalidFlagCombination,
			fmt.Sprintf("function code mismatch: MCAP header FC=%d, --function=%d (omit --function to use the file's FC)", header.Function, cfg.Function), nil)
	}

	store, err := newMemoryStoreLastValues(records, header.Function)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, "invalid MCAP data for static server: "+err.Error(), err)
	}

	_, _ = fmt.Fprintf(progress, "✅ Static server ready on port %d for unit %d and function %d\n", cfg.Port, cfg.Unit, header.Function)
	_, _ = fmt.Fprintf(progress, "🧱 Loaded %d address entries from MCAP (per-point coil/discrete or per-register map)\n",
		len(store.registers)+len(store.coils)+len(store.discrete))
	return newModbusServer(ctx, store, ServerConfig{
		Port:     cfg.Port,
		Unit:     cfg.Unit,
		Function: header.Function,
		Debug:    cfg.Debug,
	}, progress)
}

func LoadMCAPAndServeReplay(ctx context.Context, cfg config.ReplayServerConfig, progress io.Writer) error {
	if progress == nil {
		progress = io.Discard
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	records, header, err := mcap.LoadRecordsFromMCAP(cfg.InputFile)
	if err != nil {
		return errs.Output(errs.CodeMCAPLoadFailed, err)
	}
	replayFC := header.Function
	if cfg.Function != 0 {
		if cfg.Function != header.Function {
			return errs.InvalidInput(errs.CodeInvalidFlagCombination,
				fmt.Sprintf("function code mismatch: MCAP header FC=%d, --function=%d (omit --function to use the file's FC)", header.Function, cfg.Function), nil)
		}
		replayFC = cfg.Function
	}

	store := newMemoryStoreFirstValues(records, replayFC, progress)
	controller := &ReplayController{
		store:   store,
		records: records,
		start:   time.Unix(0, header.StartTime),
		loop:    int(cfg.Loops),
	}

	go func() {
		w := progress
		if w == nil {
			w = io.Discard
		}
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
		for loop := 0; ctx.Err() == nil && (controller.loop == 0 || loop < controller.loop); loop++ {
			_, _ = fmt.Fprintf(w, "🔄 Starting replay loop %d/%d\n", loop+1, controller.loop)
			for _, iter := range iterations {
				if ctx.Err() != nil {
					return
				}
				_, _ = fmt.Fprintf(w, "  Processing iteration %d with %d records\n", iter, len(grouped[iter]))
				records := grouped[iter]
				startTs := records[0].RequestTimestamp
				var delay time.Duration
				if lastIterationStart > 0 {
					// Interval 0: preserve relative spacing from captured request timestamps (first iteration uses absolute MCAP times).
					// Interval > 0: fixed wall-clock pause between iterations (ignores original capture timing).
					if cfg.Interval == 0 {
						delay = time.Duration(startTs - lastIterationStart)
					} else {
						delay = time.Duration(cfg.Interval) * time.Millisecond
					}
					if delay > 0 {
						if err := sleepContext(ctx, delay); err != nil {
							return
						}
					}
				}
				lastIterationStart = startTs
				for _, rec := range records {
					if err := store.upsertRecord(rec, replayFC); err != nil {
						_, _ = fmt.Fprintf(w, "⚠️ Failed to write record at address %d: %v\n", rec.StartAddress, err)
					}
				}
			}
		}
	}()

	_, _ = fmt.Fprintf(progress, "✅ Replay server ready on port %d for unit %d and function %d\n", cfg.Port, cfg.Unit, replayFC)
	_, _ = fmt.Fprintf(progress, "🧱 Loaded %d address entries from MCAP (per-point coil/discrete or per-register map)\n",
		len(store.registers)+len(store.coils)+len(store.discrete))
	return newModbusServer(ctx, store, ServerConfig{
		Port:     cfg.Port,
		Unit:     cfg.Unit,
		Function: replayFC,
		Debug:    cfg.Debug,
	}, progress)
}
