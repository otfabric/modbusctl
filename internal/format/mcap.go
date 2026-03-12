package format

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

const (
	mcapMagic   = "MCAP"
	mcapVersion = 1
)

// WriteHeader writes the MCAP file header to the given file.
// The header includes magic bytes, format version, the length of the header JSON,
// and the encoded CaptureHeader metadata.
func WriteHeader(f *os.File, header types.CaptureHeader) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to start: %w", err)
	}
	if _, err := f.Write([]byte(mcapMagic)); err != nil {
		return fmt.Errorf("failed to write magic: %w", err)
	}
	if _, err := f.Write([]byte{mcapVersion}); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}
	if len(headerJSON) > 65535 {
		return errors.New("header too large")
	}

	if err := binary.Write(f, binary.BigEndian, uint16(len(headerJSON))); err != nil {
		return fmt.Errorf("failed to write header length: %w", err)
	}
	if _, err := f.Write(headerJSON); err != nil {
		return fmt.Errorf("failed to write header JSON: %w", err)
	}

	return nil
}

// AppendRecord appends a CaptureRecord to the end of the MCAP file.
// Each record includes a timestamp, start address, register count, and raw data payload.
func AppendRecord(f *os.File, rec types.CaptureRecord) error {
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek to end: %w", err)
	}
	if err := binary.Write(f, binary.BigEndian, rec.Iteration); err != nil {
		return fmt.Errorf("failed to write iteration: %w", err)
	}
	if err := binary.Write(f, binary.BigEndian, rec.RequestTimestamp); err != nil {
		return fmt.Errorf("failed to write request_timestamp: %w", err)
	}
	if err := binary.Write(f, binary.BigEndian, rec.ResponseTimestamp); err != nil {
		return fmt.Errorf("failed to write response_timestamp: %w", err)
	}
	if err := binary.Write(f, binary.BigEndian, rec.StartAddress); err != nil {
		return fmt.Errorf("failed to write start address: %w", err)
	}
	if err := binary.Write(f, binary.BigEndian, rec.RegisterCount); err != nil {
		return fmt.Errorf("failed to write register count: %w", err)
	}
	dataLen := uint16(len(rec.Data))
	if err := binary.Write(f, binary.BigEndian, dataLen); err != nil {
		return fmt.Errorf("failed to write data length: %w", err)
	}
	if _, err := f.Write(rec.Data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	return nil
}

// ReadHeader reads and parses the MCAP file header from the given reader.
// It validates the magic sequence and version, and decodes the JSON header metadata.
func ReadHeader(r io.Reader) (types.CaptureHeader, error) {
	var header types.CaptureHeader

	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil {
		return header, fmt.Errorf("failed to read magic: %w", err)
	}
	if string(magic) != mcapMagic {
		return header, errors.New("invalid magic header")
	}

	var version byte
	if err := binary.Read(r, binary.BigEndian, &version); err != nil {
		return header, fmt.Errorf("failed to read version: %w", err)
	}
	if version != mcapVersion {
		return header, fmt.Errorf("unsupported version: %d", version)
	}

	var length uint16
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return header, fmt.Errorf("failed to read header length: %w", err)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return header, fmt.Errorf("failed to read header JSON: %w", err)
	}
	if err := json.Unmarshal(buf, &header); err != nil {
		return header, fmt.Errorf("failed to unmarshal header JSON: %w", err)
	}

	return header, nil
}

// ReadRecord reads a single CaptureRecord from the given reader.
// It returns the iteration, timestamp, register address range, and raw data block.
func ReadRecord(r io.Reader) (*types.CaptureRecord, error) {
	var rec types.CaptureRecord

	if err := binary.Read(r, binary.BigEndian, &rec.Iteration); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &rec.RequestTimestamp); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &rec.ResponseTimestamp); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &rec.StartAddress); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &rec.RegisterCount); err != nil {
		return nil, err
	}
	var dataLen uint16
	if err := binary.Read(r, binary.BigEndian, &dataLen); err != nil {
		return nil, err
	}
	rec.Data = make([]byte, dataLen)
	if _, err := io.ReadFull(r, rec.Data); err != nil {
		return nil, err
	}

	return &rec, nil
}

// CountRecords opens the given MCAP file and counts the number of valid CaptureRecord entries.
// It skips and counts records following the initial file header.
func CountRecords(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	_, err = ReadHeader(f)
	if err != nil {
		return 0, err
	}

	count := 0
	for {
		_, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// LoadRecordsFromMCAP reads the header and all records from a .mcap file
func LoadRecordsFromMCAP(path string) ([]types.CaptureRecord, types.CaptureHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, types.CaptureHeader{}, fmt.Errorf("failed to open mcap file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header, err := ReadHeader(f)
	if err != nil {
		return nil, types.CaptureHeader{}, fmt.Errorf("failed to read header: %w", err)
	}

	var records []types.CaptureRecord
	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, types.CaptureHeader{}, fmt.Errorf("failed to read record: %w", err)
		}
		records = append(records, *rec)
	}

	return records, header, nil
}

// ExportCSV reads an MCAP file and writes all records as CSV to the provided writer.
func ExportCSV(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open mcap file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header, err := ReadHeader(f)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	if _, err := fmt.Fprintf(w, "ip,port,unit,function,iteration,request_timestamp,response_timestamp,start_address,register_count,data_hex\n"); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read record: %w", err)
		}
		_, _ = fmt.Fprintf(w, "%s,%d,%d,%d,%d,%s,%s,%d,%d,%x\n", header.IP, header.Port, header.Unit, header.Function, rec.Iteration, time.Unix(0, rec.RequestTimestamp).Format(time.RFC3339Nano), time.Unix(0, rec.ResponseTimestamp).Format(time.RFC3339Nano), rec.StartAddress, rec.RegisterCount, rec.Data)
	}

	return nil
}

// ExportJSON reads an MCAP file and writes all records and header as JSON to the provided writer.
func ExportJSON(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open mcap file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header, err := ReadHeader(f)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	var records []types.CaptureRecordJson
	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read record: %w", err)
		}
		records = append(records, types.CaptureRecordJson{
			Iteration:         rec.Iteration,
			RequestTimestamp:  time.Unix(0, rec.RequestTimestamp).Format(time.RFC3339Nano),
			ResponseTimestamp: time.Unix(0, rec.ResponseTimestamp).Format(time.RFC3339Nano),
			StartAddress:      rec.StartAddress,
			RegisterCount:     rec.RegisterCount,
			Data:              fmt.Sprintf("%x", rec.Data),
		})
	}

	output := struct {
		Header  types.CaptureHeaderJson   `json:"header"`
		Records []types.CaptureRecordJson `json:"records"`
	}{
		Header: types.CaptureHeaderJson{
			IP:        header.IP,
			Port:      header.Port,
			Unit:      header.Unit,
			Function:  header.Function,
			StartTime: time.Unix(0, header.StartTime).Format(time.RFC3339Nano),
		},
		Records: records,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// ExportAddressBlocks extracts a unique set of address blocks from an MCAP file and writes them as JSON to the provided writer.
func ExportAddressBlocks(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open mcap file: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, err = ReadHeader(f)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	blockSet := make(map[string]types.AddressBlock)
	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read record: %w", err)
		}
		key := fmt.Sprintf("%d:%d", rec.StartAddress, rec.RegisterCount)
		blockSet[key] = types.AddressBlock{
			StartAddress:  rec.StartAddress,
			RegisterCount: rec.RegisterCount,
		}
	}

	var blocks []types.AddressBlock
	for _, b := range blockSet {
		blocks = append(blocks, b)
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].StartAddress < blocks[j].StartAddress
	})

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(blocks); err != nil {
		return fmt.Errorf("failed to write blocks as JSON: %w", err)
	}

	return nil
}

// ExportStrings scans each record's data for embedded ASCII substrings and writes them with their address range.
func ExportStrings(w io.Writer, cfg config.StringsConfig) error {
	records, _, err := LoadRecordsFromMCAP(cfg.InputFile)
	if err != nil {
		return fmt.Errorf("failed to load records: %w", err)
	}

	for _, rec := range stitchAdjacentRecords(records) {
		processASCII(w, &rec)
	}
	return nil
}

// ExportHeuristicFrequency scans the records for potential frequency values and writes them to the provided writer.
func ExportHeuristicFrequency(w io.Writer, path string) error {
	records, _, err := LoadRecordsFromMCAP(path)
	if err != nil {
		return fmt.Errorf("failed to load records: %w", err)
	}

	matches := processFrequency(stitchAdjacentRecords(records))
	for _, m := range matches[:int(math.Min(20, float64(len(matches))))] {
		_, _ = fmt.Fprintf(w, "[%d] %d regs (%s) = %.4f → candidate frequency (confidence: %.2f)\n",
			m.Addr, m.Regs, m.Format, m.Value, m.Confidence)
	}
	return nil
}

// ExportInfo reads the header and records from an MCAP file and writes a summary to the provided writer.
func ExportInfo(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open mcap file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header, err := ReadHeader(f)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	_, _ = fmt.Fprintf(w, "Header Information:\n")
	_, _ = fmt.Fprintf(w, "  IP:        %s\n", header.IP)
	_, _ = fmt.Fprintf(w, "  Port:      %d\n", header.Port)
	_, _ = fmt.Fprintf(w, "  Unit:      %d\n", header.Unit)
	_, _ = fmt.Fprintf(w, "  Function:  %d\n", header.Function)
	_, _ = fmt.Fprintf(w, "  StartTime: %s\n", time.Unix(0, header.StartTime).Format(time.RFC3339Nano))
	_, _ = fmt.Fprintf(w, "\n")

	iterDetails := make(map[uint32]*types.IterationDetail)
	var durations []time.Duration

	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read record: %w", err)
		}

		detail, ok := iterDetails[rec.Iteration]
		if !ok {
			detail = &types.IterationDetail{
				FirstRequestTS: rec.RequestTimestamp,
				LastResponseTS: rec.ResponseTimestamp,
				MinAddr:        rec.StartAddress,
				MaxAddr:        rec.StartAddress + rec.RegisterCount - 1,
			}
			iterDetails[rec.Iteration] = detail
		}
		detail.BlockCount++
		detail.TotalRegisters += int(rec.RegisterCount)
		if rec.RequestTimestamp < detail.FirstRequestTS {
			detail.FirstRequestTS = rec.RequestTimestamp
		}
		if rec.ResponseTimestamp > detail.LastResponseTS {
			detail.LastResponseTS = rec.ResponseTimestamp
		}
		if rec.StartAddress < detail.MinAddr {
			detail.MinAddr = rec.StartAddress
		}
		if end := rec.StartAddress + rec.RegisterCount - 1; end > detail.MaxAddr {
			detail.MaxAddr = end
		}
	}

	_, _ = fmt.Fprintf(w, "Record Summary:\n")
	_, _ = fmt.Fprintf(w, "  Iterations: %d\n", len(iterDetails))

	var minDur, maxDur time.Duration
	var totalDur time.Duration
	first := true

	var sortedIters []uint32
	for iter := range iterDetails {
		sortedIters = append(sortedIters, iter)
	}
	sort.Slice(sortedIters, func(i, j int) bool {
		return sortedIters[i] < sortedIters[j]
	})

	for _, iter := range sortedIters {
		d := iterDetails[iter]
		duration := time.Duration(d.LastResponseTS - d.FirstRequestTS)
		durations = append(durations, duration)
		if first {
			minDur = duration
			maxDur = duration
			totalDur = duration
			first = false
		} else {
			if duration < minDur {
				minDur = duration
			}
			if duration > maxDur {
				maxDur = duration
			}
			totalDur += duration
		}

		_, _ = fmt.Fprintf(w, "    Iteration %d:\n", iter)
		_, _ = fmt.Fprintf(w, "      Blocks: %d\n", d.BlockCount)
		_, _ = fmt.Fprintf(w, "      Total Registers: %d\n", d.TotalRegisters)
		_, _ = fmt.Fprintf(w, "      Time: %s → %s (duration: %dms)\n",
			time.Unix(0, d.FirstRequestTS).Format(time.RFC3339Nano),
			time.Unix(0, d.LastResponseTS).Format(time.RFC3339Nano),
			duration.Milliseconds())
		_, _ = fmt.Fprintf(w, "      Address Range: %d → %d\n", d.MinAddr, d.MaxAddr)
	}

	if len(durations) > 0 {
		avgDur := totalDur / time.Duration(len(durations))
		_, _ = fmt.Fprintf(w, "  Iteration Durations:\n")
		_, _ = fmt.Fprintf(w, "    Min: %v\n", minDur)
		_, _ = fmt.Fprintf(w, "    Avg: %v\n", avgDur)
		_, _ = fmt.Fprintf(w, "    Max: %v\n", maxDur)
	} else {
		_, _ = fmt.Fprintf(w, "  No records found.\n")
	}

	return nil
}

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
