package mcap

import (
	"fmt"
	"io"
	"os"

	"github.com/otfabric/modbusctl/internal/types"
)

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

// LoadRecordsFromMCAP reads the header and all records from a .mcap file.
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
