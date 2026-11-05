// SPDX-License-Identifier: MIT

package analysis

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/fsutil"
	"github.com/otfabric/modbusctl/internal/types"
)

// ExportDeviceProfileDecode decodes the register samples of a capture with a device profile and
// writes the values. Each register holds its latest recorded value.
func ExportDeviceProfileDecode(w io.Writer, path string, deviceProfilePath string, sel config.CaptureSelection) error {
	records, header, err := LoadRecords(path, sel)
	if err != nil {
		return err
	}
	if len(records) > 0 && header.Function != 3 && header.Function != 4 {
		return fmt.Errorf("decode needs register samples (FC03 or FC04); the selection holds FC%02d", header.Function)
	}
	content, err := fsutil.ReadInput(deviceProfilePath, fsutil.MaxJSONInput)
	if err != nil {
		return fmt.Errorf("failed to open device profile: %w", err)
	}

	var profile types.DeviceProfile
	if err := json.Unmarshal(content, &profile); err != nil {
		return fmt.Errorf("failed to decode device profile: %w", err)
	}

	mem := make(map[uint16][]byte)
	for _, rec := range records {
		for i := uint16(0); i < rec.RegisterCount; i++ {
			addr := rec.StartAddress + i
			offset := i * 2
			if int(offset+2) <= len(rec.Data) {
				mem[addr] = rec.Data[offset : offset+2]
			}
		}
	}

	_, _ = fmt.Fprintf(w, "Decoded Values:\n")
	for _, reg := range profile.ProtocolData.Registers {
		if reg.ControlledPropertyId == "not.used" {
			continue
		}
		// The profile is a file from elsewhere: its names go to a terminal.
		name, regFormat := types.TerminalSafe(reg.ControlledPropertyId), types.TerminalSafe(reg.Format)
		var raw []byte
		found := uint32(reg.Start)+uint32(reg.Size) <= 1<<16 // a register past 65535 is never in the capture
		for i := uint16(0); found && i < reg.Size; i++ {
			part, ok := mem[reg.Start+i]
			if !ok {
				found = false
				break
			}
			raw = append(raw, part...)
		}
		if !found {
			_, _ = fmt.Fprintf(w, "⚠️  Missing data for %s at %d (size %d)\n", name, reg.Start, reg.Size)
			continue
		}
		decoder, ok := types.RegisterDecoders[reg.Format]
		if !ok {
			_, _ = fmt.Fprintf(w, "❌ Unsupported format for %s: %s\n", name, regFormat)
			continue
		}
		if len(raw) < decoder.Size {
			_, _ = fmt.Fprintf(w, "❌ Not enough bytes for %s: have %d, need %d\n", name, len(raw), decoder.Size)
			continue
		}
		value, err := decoder.Decode(raw)
		if err != nil {
			_, _ = fmt.Fprintf(w, "❌ Failed decoding %s: %v\n", name, err)
			continue
		}
		scaled := value * reg.ValueScaleFactor
		_, _ = fmt.Fprintf(w, "%s = %f\n", name, scaled)
	}

	return nil
}
