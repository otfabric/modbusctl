// Package plan reads recording plans: the requests `client record` sends
// every interval. A plan is a JSON array; an entry with only start_address and
// register_count is an address block read with the default function (the
// --function flag), as written by `client scan` and `capture extract`.
// Entries with a function may use any read function of the Modbus
// specification that a capture can hold; nothing else is accepted.
package plan

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/otfabric/modbusctl/internal/capfile"
)

// FileRecord is one sub-request of an FC20 Read File Record request.
type FileRecord struct {
	File   uint16 `json:"file"`
	Record uint16 `json:"record"`
	Length uint16 `json:"length"`
}

// Request is one entry of a recording plan. Which fields apply depends on
// Function.
type Request struct {
	// Function is the function code; 0 is the default function of the plan.
	Function uint8 `json:"function,omitempty"`
	// StartAddress and RegisterCount: FC01–04.
	StartAddress  uint16 `json:"start_address,omitempty"`
	RegisterCount uint16 `json:"register_count,omitempty"`
	// SubFunction and Data (hex): FC08. Data defaults to 0000.
	SubFunction uint16 `json:"sub_function,omitempty"`
	Data        string `json:"data,omitempty"`
	// FileRecords: FC20.
	FileRecords []FileRecord `json:"file_records,omitempty"`
	// FIFOAddress: FC24.
	FIFOAddress uint16 `json:"fifo_address,omitempty"`
	// DeviceIDCode (1 basic, the default; 2 regular; 3 extended; 4 one
	// object) and ObjectID (the object to start from, or to read with 4):
	// FC43 Read Device Identification.
	DeviceIDCode uint8 `json:"device_id_code,omitempty"`
	ObjectID     uint8 `json:"object_id,omitempty"`
}

// Supported describes the functions a plan may use, for error messages.
const Supported = "1, 2, 3, 4 (coils, discrete inputs, holding and input registers), 7 (Read Exception Status), " +
	"8 (Diagnostics, sub-functions that return data: 0x00, 0x02, 0x0B-0x12), 11 (Get Comm Event Counter), " +
	"12 (Get Comm Event Log), 17 (Report Server ID), 20 (Read File Record), 24 (Read FIFO Queue), " +
	"43 (Read Device Identification)"

// Limits of the Modbus specification.
const (
	maxBits          = 2000
	maxRegisters     = 125
	maxFileRequests  = 35  // 7 bytes each in a 253-byte PDU
	maxFileResponse  = 251 // sub-response bytes after FC and length
	maxFileRecordNum = 0x270F
	maxDiagEcho      = 250
)

// Read reads the plan at path. Entries without a function get defaultFC.
// Every entry is checked; the first invalid one is reported with its index.
func Read(path string, defaultFC uint8) ([]Request, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("blocks file path is required (--blocks-file / MODBUSCTL_BLOCKS_FILE)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read blocks file: %w", err)
	}
	return Parse(data, defaultFC)
}

// Parse parses and checks a plan; see [Read].
func Parse(data []byte, defaultFC uint8) ([]Request, error) {
	var reqs []Request
	if err := json.Unmarshal(data, &reqs); err != nil {
		return nil, fmt.Errorf("blocks file does not contain valid plan JSON (an array of address blocks or requests): %w", err)
	}
	if len(reqs) == 0 {
		return nil, errors.New("blocks file contains no address blocks or requests")
	}
	for i := range reqs {
		if reqs[i].Function == 0 {
			reqs[i].Function = defaultFC
		}
		if err := reqs[i].Check(); err != nil {
			return nil, fmt.Errorf("blocks file entry %d: %w", i, err)
		}
	}
	return reqs, nil
}

// Check reports whether r is a supported read request with valid parameters.
func (r *Request) Check() error {
	switch r.Function {
	case 1, 2, 3, 4:
		limit := uint32(maxRegisters)
		if r.Function <= 2 {
			limit = maxBits
		}
		if r.RegisterCount == 0 || uint32(r.RegisterCount) > limit || uint32(r.StartAddress)+uint32(r.RegisterCount) > 1<<16 {
			return fmt.Errorf("invalid address block: start=%d, count=%d (function %d reads 1-%d within 0-65535)",
				r.StartAddress, r.RegisterCount, r.Function, limit)
		}
	case 8:
		if r.SubFunction > 0xFF || !capfile.IsDataDiagnostic(uint8(r.SubFunction)) {
			return fmt.Errorf("diagnostics sub-function 0x%04X is not recorded: only sub-functions that return data are (0x00, 0x02, 0x0B-0x12)", r.SubFunction)
		}
		data, err := r.DiagnosticData()
		if err != nil {
			return err
		}
		if r.SubFunction == 0 && len(data) > maxDiagEcho {
			return fmt.Errorf("diagnostics data is %d bytes, at most %d fit", len(data), maxDiagEcho)
		}
		if r.SubFunction != 0 && len(data) != 2 {
			return fmt.Errorf("diagnostics sub-function 0x%04X takes 2 data bytes, got %d", r.SubFunction, len(data))
		}
	case 20:
		if len(r.FileRecords) == 0 || len(r.FileRecords) > maxFileRequests {
			return fmt.Errorf("read file record needs 1-%d file_records, got %d", maxFileRequests, len(r.FileRecords))
		}
		size := 0
		for _, f := range r.FileRecords {
			if f.File == 0 || f.Record > maxFileRecordNum || f.Length == 0 {
				return fmt.Errorf("invalid file record: file=%d, record=%d, length=%d (file 1-65535, record 0-%d, length at least 1)",
					f.File, f.Record, f.Length, maxFileRecordNum)
			}
			size += 2 + 2*int(f.Length)
		}
		if size > maxFileResponse {
			return fmt.Errorf("file records ask for %d response bytes, at most %d fit", size, maxFileResponse)
		}
	case 43:
		if r.DeviceIDCode > 4 {
			return fmt.Errorf("device_id_code %d: use 1 (basic), 2 (regular), 3 (extended) or 4 (one object)", r.DeviceIDCode)
		}
	case 7, 11, 12, 17, 24:
	default:
		return fmt.Errorf("function code %d is not a Modbus read function modbusctl records; supported: %s", r.Function, Supported)
	}
	if !capfile.IsSpecRead(r.PDU()) {
		return fmt.Errorf("function code %d: request % X is not a supported read", r.Function, r.PDU())
	}
	return nil
}

// DiagnosticData returns the FC08 request data: Data decoded, 0000 when empty.
func (r *Request) DiagnosticData() ([]byte, error) {
	if strings.TrimSpace(r.Data) == "" {
		return []byte{0, 0}, nil
	}
	b, err := hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(r.Data), " ", ""))
	if err != nil {
		return nil, fmt.Errorf("diagnostics data %q is not hex: %w", r.Data, err)
	}
	return b, nil
}

// IDCode returns the FC43 read device ID code, 1 (basic) by default.
func (r *Request) IDCode() uint8 {
	if r.DeviceIDCode == 0 {
		return 1
	}
	return r.DeviceIDCode
}

// PDU returns the request PDU r sends (for FC43, its first request).
func (r *Request) PDU() []byte {
	be := func(v uint16) []byte { return []byte{byte(v >> 8), byte(v)} }
	switch r.Function {
	case 1, 2, 3, 4:
		return append(append([]byte{r.Function}, be(r.StartAddress)...), be(r.RegisterCount)...)
	case 8:
		data, _ := r.DiagnosticData()
		return append(append([]byte{8}, be(r.SubFunction)...), data...)
	case 20:
		pdu := []byte{20, byte(7 * len(r.FileRecords))}
		for _, f := range r.FileRecords {
			pdu = append(append(append(append(pdu, 6), be(f.File)...), be(f.Record)...), be(f.Length)...)
		}
		return pdu
	case 24:
		return append([]byte{24}, be(r.FIFOAddress)...)
	case 43:
		return []byte{43, 0x0E, r.IDCode(), r.ObjectID}
	}
	return []byte{r.Function}
}

// String describes r for progress lines.
func (r *Request) String() string {
	switch r.Function {
	case 1, 2, 3, 4:
		return fmt.Sprintf("FC%02d %d x%d", r.Function, r.StartAddress, r.RegisterCount)
	case 8:
		return fmt.Sprintf("FC08 sub-function 0x%04X", r.SubFunction)
	case 20:
		return fmt.Sprintf("FC20 %d file record(s)", len(r.FileRecords))
	case 24:
		return fmt.Sprintf("FC24 FIFO %d", r.FIFOAddress)
	case 43:
		return fmt.Sprintf("FC43 device identification code %d from object %d", r.IDCode(), r.ObjectID)
	}
	return fmt.Sprintf("FC%02d", r.Function)
}
