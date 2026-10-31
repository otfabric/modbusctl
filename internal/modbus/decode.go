// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"

	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
)

// CollectDecode reads holding/input registers (FC3/FC4) and decodes them into one
// or more typed values using the codec identified by cfg.Codec. When cfg.RegisterCount
// is 0 a single codec value is read; otherwise it must be a multiple of the codec's
// register width and the block is decoded as an array of values.
func CollectDecode(ctx context.Context, cfg config.DecodeConfig, progress io.Writer) (*types.DecodeResult, error) {
	if progress == nil {
		progress = io.Discard
	}

	rc, ok, err := codec.RuntimeCodecByID(cfg.Codec)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("codec %q could not be constructed: %v", cfg.Codec, err), err)
	}
	if !ok {
		return nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("unknown codec %q (run 'modbusctl client codecs' to list available codecs)", cfg.Codec), nil)
	}
	per := int(rc.RegisterSpec().Count)
	if per == 0 {
		return nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("codec %q has zero register width", cfg.Codec), nil)
	}
	total := int(cfg.RegisterCount)
	if total == 0 {
		total = per
	}

	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	link, err := openDeviceLink(modbusURL, cfg.Timeout, cfg.Debug, cfg.SerialConfig, cfg.TLSConfig)
	if err != nil {
		return nil, err
	}
	defer link.Close()

	rawData, _, _, err := readRegisters(ctx, link, cfg.Function, cfg.StartAddress, uint16(total), 1, cfg.Unit, 0, progress)
	if err != nil {
		return nil, err
	}

	regs := bytesToRegisters(rawData)
	values := make([]any, 0, len(regs)/per)
	for i := 0; i+per <= len(regs); i += per {
		v, derr := codec.DecodeRegistersAny(regs[i:i+per], rc)
		if derr != nil {
			return nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("decode with codec %q failed: %v", cfg.Codec, derr), derr)
		}
		values = append(values, normalizeDecoded(v))
	}

	family := ""
	if desc, ok := codec.CodecDescriptorByID(cfg.Codec); ok {
		family = desc.Family.String()
	}

	return &types.DecodeResult{
		Target:        modbusURL,
		UnitID:        cfg.Unit,
		Function:      cfg.Function,
		StartAddress:  cfg.StartAddress,
		RegisterCount: uint16(total),
		Codec:         cfg.Codec,
		Family:        family,
		ValueKind:     rc.ValueKind().String(),
		RawDataHex:    fmt.Sprintf("% X", rawData),
		Values:        values,
	}, nil
}

// bytesToRegisters converts wire-order register bytes (big-endian per register) into []uint16.
func bytesToRegisters(b []byte) []uint16 {
	regs := make([]uint16, len(b)/2)
	for i := range regs {
		regs[i] = binary.BigEndian.Uint16(b[i*2:])
	}
	return regs
}

// normalizeDecoded converts codec output into a stable form for JSON/text/table rendering.
// net.HardwareAddr and byte slices become strings; net.IP and time.Time are left as-is
// (they already render as strings in JSON and via types.FormatDecodedValue in text/table).
func normalizeDecoded(v any) any {
	switch t := v.(type) {
	case net.HardwareAddr:
		return t.String()
	case []byte:
		return hex.EncodeToString(t)
	default:
		return v
	}
}
