package mcap

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/otfabric/modbusctl/internal/types"
)

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
