package mcap

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/otfabric/modbusctl/internal/types"
)

// ExportDeviceProfileDecode reads the records from an MCAP file and writes a summary to the provided writer.
func ExportDeviceProfileDecode(w io.Writer, path string, deviceProfilePath string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open mcap file: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, err = ReadHeader(f)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	profileFile, err := os.Open(deviceProfilePath)
	if err != nil {
		return fmt.Errorf("failed to open device profile: %w", err)
	}
	defer func() { _ = profileFile.Close() }()

	var profile types.DeviceProfile
	if err := json.NewDecoder(profileFile).Decode(&profile); err != nil {
		return fmt.Errorf("failed to decode device profile: %w", err)
	}

	mem := make(map[uint16][]byte)
	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read record: %w", err)
		}
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
		var raw []byte
		found := true
		for i := uint16(0); i < reg.Size; i++ {
			part, ok := mem[reg.Start+i]
			if !ok {
				found = false
				break
			}
			raw = append(raw, part...)
		}
		if !found {
			_, _ = fmt.Fprintf(w, "⚠️  Missing data for %s at %d (size %d)\n", reg.ControlledPropertyId, reg.Start, reg.Size)
			continue
		}
		decoder, ok := types.RegisterDecoders[reg.Format]
		if !ok {
			_, _ = fmt.Fprintf(w, "❌ Unsupported format for %s: %s\n", reg.ControlledPropertyId, reg.Format)
			continue
		}
		if len(raw) < decoder.Size {
			_, _ = fmt.Fprintf(w, "❌ Not enough bytes for %s: have %d, need %d\n", reg.ControlledPropertyId, len(raw), decoder.Size)
			continue
		}
		value, err := decoder.Decode(raw)
		if err != nil {
			_, _ = fmt.Fprintf(w, "❌ Failed decoding %s: %v\n", reg.ControlledPropertyId, err)
			continue
		}
		scaled := value * reg.ValueScaleFactor
		_, _ = fmt.Fprintf(w, "%s = %f\n", reg.ControlledPropertyId, scaled)
	}

	return nil
}
