package modbus

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
)

// Modbus write function codes accepted by `client write`.
const (
	fcWriteSingleCoil        = 5
	fcWriteSingleRegister    = 6
	fcWriteMultipleCoils     = 15
	fcWriteMultipleRegisters = 16
)

// Protocol limits for a single write request.
const (
	MaxWriteCoils     = 1968
	MaxWriteRegisters = 123
)

// WritePlan is the wire-level form of a `client write`: the resolved function code and the
// coils or registers to send. Exactly one of Coils and Registers is set.
type WritePlan struct {
	Function  uint8
	Coils     []bool
	Registers []uint16
}

// Count returns the number of coils or registers the plan writes.
func (p *WritePlan) Count() int {
	return len(p.Coils) + len(p.Registers)
}

// PlanWrite turns the user-facing write configuration into a WritePlan: it parses the values
// (through the codec when one is set), resolves --function 0 to FC6 or FC16, and enforces
// the per-function quantity limits and the address range. It performs no I/O.
func PlanWrite(cfg config.WriteConfig) (*WritePlan, error) {
	values := make([]string, 0, len(cfg.Values))
	for _, v := range cfg.Values {
		if t := strings.TrimSpace(v); t != "" {
			values = append(values, t)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one value is required (use --values)")
	}

	plan := &WritePlan{Function: cfg.Function}
	codecID := strings.TrimSpace(cfg.Codec)

	switch cfg.Function {
	case fcWriteSingleCoil, fcWriteMultipleCoils:
		if codecID != "" {
			return nil, fmt.Errorf("--codec encodes registers and cannot be used with coil function code %d", cfg.Function)
		}
		for _, v := range values {
			c, err := parseCoilValue(v)
			if err != nil {
				return nil, err
			}
			plan.Coils = append(plan.Coils, c)
		}
	case 0, fcWriteSingleRegister, fcWriteMultipleRegisters:
		regs, err := registersFromValues(values, codecID)
		if err != nil {
			return nil, err
		}
		plan.Registers = regs
		if plan.Function == 0 {
			plan.Function = fcWriteMultipleRegisters
			if len(regs) == 1 {
				plan.Function = fcWriteSingleRegister
			}
		}
	default:
		return nil, fmt.Errorf("unsupported function code for write: %d — supported: 5 (Write Single Coil), 6 (Write Single Register), 15 (Write Multiple Coils), 16 (Write Multiple Registers)", cfg.Function)
	}

	n := plan.Count()
	switch plan.Function {
	case fcWriteSingleCoil:
		if n != 1 {
			return nil, fmt.Errorf("function code 5 writes exactly one coil, got %d values (use --function 15 for several)", n)
		}
	case fcWriteSingleRegister:
		if n != 1 {
			return nil, fmt.Errorf("function code 6 writes exactly one register, got %d (use --function 16 for several)", n)
		}
	case fcWriteMultipleCoils:
		if n > MaxWriteCoils {
			return nil, fmt.Errorf("function code 15 writes at most %d coils per request, got %d", MaxWriteCoils, n)
		}
	case fcWriteMultipleRegisters:
		if n > MaxWriteRegisters {
			return nil, fmt.Errorf("function code 16 writes at most %d registers per request, got %d", MaxWriteRegisters, n)
		}
	}
	if int(cfg.StartAddress)+n > 65536 {
		return nil, fmt.Errorf("start address %d with %d value(s) exceeds the maximum address 65535", cfg.StartAddress, n)
	}
	return plan, nil
}

// registersFromValues converts values to registers: one register per value without a codec,
// or the codec's encoding of each value otherwise.
func registersFromValues(values []string, codecID string) ([]uint16, error) {
	if codecID == "" {
		regs := make([]uint16, 0, len(values))
		for _, v := range values {
			r, err := parseRegisterValue(v)
			if err != nil {
				return nil, err
			}
			regs = append(regs, r)
		}
		return regs, nil
	}

	rc, ok, err := codec.RuntimeCodecByID(codecID)
	if err != nil {
		return nil, fmt.Errorf("codec %q could not be constructed: %v", codecID, err)
	}
	if !ok {
		return nil, fmt.Errorf("unknown codec %q (run 'modbusctl client codecs' to list available codecs)", codecID)
	}
	var regs []uint16
	for _, v := range values {
		typed, err := parseCodecValue(rc.ValueKind(), v)
		if err != nil {
			return nil, fmt.Errorf("value %q is not valid for codec %q: %v", v, codecID, err)
		}
		enc, err := codec.EncodeRegistersAny(typed, rc)
		if err != nil {
			return nil, fmt.Errorf("value %q could not be encoded with codec %q: %v", v, codecID, err)
		}
		regs = append(regs, enc...)
	}
	return regs, nil
}

// parseCoilValue accepts 1/0, true/false and on/off (case-insensitive).
func parseCoilValue(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "on":
		return true, nil
	case "0", "false", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid coil value %q: use 1/0, true/false or on/off", s)
	}
}

// parseRegisterValue accepts an unsigned value 0-65535 (decimal, or 0x/0o/0b prefixed) or a
// signed value -32768..-1, which is written as its two's complement.
func parseRegisterValue(s string) (uint16, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 0, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid register value %q: use an integer such as 1234, 0x04D2 or -5 (use --codec for floats and other types)", s)
	}
	if n < math.MinInt16 || n > math.MaxUint16 {
		return 0, fmt.Errorf("register value %s is out of range (-32768 to 65535)", s)
	}
	return uint16(n), nil
}

// parseCodecValue converts a command-line value to the Go type a codec of the given value
// kind encodes.
func parseCodecValue(kind codec.CodecValueKind, s string) (any, error) {
	switch kind {
	case codec.CodecValueUint16:
		n, err := strconv.ParseUint(s, 0, 16)
		return uint16(n), err
	case codec.CodecValueInt16:
		n, err := strconv.ParseInt(s, 0, 16)
		return int16(n), err
	case codec.CodecValueUint32:
		n, err := strconv.ParseUint(s, 0, 32)
		return uint32(n), err
	case codec.CodecValueInt32:
		n, err := strconv.ParseInt(s, 0, 32)
		return int32(n), err
	case codec.CodecValueUint48, codec.CodecValueUint64:
		return strconv.ParseUint(s, 0, 64)
	case codec.CodecValueInt48, codec.CodecValueInt64:
		return strconv.ParseInt(s, 0, 64)
	case codec.CodecValueFloat32:
		f, err := strconv.ParseFloat(s, 32)
		return float32(f), err
	case codec.CodecValueFloat64:
		return strconv.ParseFloat(s, 64)
	case codec.CodecValueString:
		return s, nil
	case codec.CodecValueByteSlice, codec.CodecValueUint8Slice:
		clean := strings.NewReplacer(" ", "", ":", "", "-", "").Replace(strings.TrimPrefix(s, "0x"))
		b, err := hex.DecodeString(clean)
		if err != nil {
			return nil, fmt.Errorf("expected hex bytes such as 0A1B2C: %v", err)
		}
		return b, nil
	case codec.CodecValueIP:
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("expected an IP address such as 192.168.1.10")
		}
		return ip, nil
	case codec.CodecValueHardwareAddr:
		return net.ParseMAC(s)
	case codec.CodecValueTime:
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("expected an RFC 3339 time such as 2026-01-31T12:00:00Z")
		}
		return t, nil
	default:
		return nil, fmt.Errorf("values of kind %s cannot be written", kind)
	}
}

// CollectWrite performs the write described by cfg and returns what was sent.
func CollectWrite(ctx context.Context, cfg config.WriteConfig) (*types.WriteResult, error) {
	plan, err := PlanWrite(cfg)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}

	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	link, err := openDeviceLink(modbusURL, cfg.Timeout, cfg.Debug, cfg.SerialConfig, cfg.TLSConfig)
	if err != nil {
		return nil, err
	}
	defer link.Close()

	// A write is sent once: a retry after a lost response could apply it twice.
	switch plan.Function {
	case fcWriteSingleCoil:
		err = link.client.WriteCoil(ctx, cfg.Unit, cfg.StartAddress, plan.Coils[0])
	case fcWriteMultipleCoils:
		err = link.client.WriteCoils(ctx, cfg.Unit, cfg.StartAddress, plan.Coils)
	case fcWriteSingleRegister:
		err = link.client.WriteRegister(ctx, cfg.Unit, cfg.StartAddress, plan.Registers[0])
	default:
		err = link.client.WriteRegisters(ctx, cfg.Unit, cfg.StartAddress, plan.Registers)
	}
	if err != nil {
		return nil, err
	}

	res := &types.WriteResult{
		Target:       modbusURL,
		UnitID:       cfg.Unit,
		Function:     plan.Function,
		StartAddress: cfg.StartAddress,
		Count:        plan.Count(),
		Codec:        strings.TrimSpace(cfg.Codec),
		Coils:        plan.Coils,
		Registers:    plan.Registers,
	}
	if len(plan.Registers) > 0 {
		raw := make([]byte, 0, 2*len(plan.Registers))
		for _, r := range plan.Registers {
			raw = append(raw, byte(r>>8), byte(r))
		}
		res.RawDataHex = fmt.Sprintf("% X", raw)
	}
	return res, nil
}
