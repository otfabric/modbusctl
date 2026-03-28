package mcap

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

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
