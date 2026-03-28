package mcap

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

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

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"ip", "port", "unit", "function", "iteration", "request_timestamp", "response_timestamp", "start_address", "register_count", "data_hex"}); err != nil {
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
		row := []string{
			header.IP,
			strconv.FormatUint(uint64(header.Port), 10),
			strconv.FormatUint(uint64(header.Unit), 10),
			strconv.FormatUint(uint64(header.Function), 10),
			strconv.FormatUint(uint64(rec.Iteration), 10),
			time.Unix(0, rec.RequestTimestamp).Format(time.RFC3339Nano),
			time.Unix(0, rec.ResponseTimestamp).Format(time.RFC3339Nano),
			strconv.FormatUint(uint64(rec.StartAddress), 10),
			strconv.FormatUint(uint64(rec.RegisterCount), 10),
			fmt.Sprintf("%x", rec.Data),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("csv writer: %w", err)
	}

	return nil
}
