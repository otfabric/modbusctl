package mcap

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testHeader = types.CaptureHeader{IP: "10.0.0.7", Port: 1502, Unit: 9, Function: 4, StartTime: 1_700_000_000_123_456_789}

// captureBytes returns the encoded form of a capture with the given records.
func captureBytes(t *testing.T, records ...types.CaptureRecord) []byte {
	t.Helper()
	b, err := os.ReadFile(writeCapture(t, records...))
	require.NoError(t, err)
	return b
}

func writeCapture(t *testing.T, records ...types.CaptureRecord) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.mcap")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.NoError(t, WriteHeader(f, testHeader))
	for _, r := range records {
		require.NoError(t, AppendRecord(f, r))
	}
	return path
}

func writeBytes(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, content, 0o600))
	return path
}

func rfc(ns int64) string { return time.Unix(0, ns).Format(time.RFC3339Nano) }

func TestCodec_roundTrip(t *testing.T) {
	records := []types.CaptureRecord{
		{Iteration: 0, RequestTimestamp: 1, ResponseTimestamp: 2, StartAddress: 0, RegisterCount: 2, Data: []byte{0xDE, 0xAD, 0xBE, 0xEF}},
		{Iteration: 4294967295, RequestTimestamp: -5, ResponseTimestamp: 1 << 62, StartAddress: 65535, RegisterCount: 1, Data: []byte{}},
		{Iteration: 7, StartAddress: 10, RegisterCount: 125, Data: bytes.Repeat([]byte{0xA5}, 250)},
	}
	path := writeCapture(t, records...)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte{'M', 'C', 'A', 'P', 1}, raw[:5], "magic and version")

	got, header, err := LoadRecordsFromMCAP(path)
	require.NoError(t, err)
	assert.Equal(t, testHeader, header)
	assert.Equal(t, records, got)

	n, err := CountRecords(path)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestWriteHeader_rewritesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.mcap")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	require.NoError(t, WriteHeader(f, testHeader))
	rec := types.CaptureRecord{Iteration: 1, StartAddress: 5, RegisterCount: 1, Data: []byte{1, 2}}
	require.NoError(t, AppendRecord(f, rec))

	// A header of the same encoded size can be rewritten without disturbing the records.
	updated := testHeader
	updated.StartTime = 1_700_000_000_999_999_999
	require.NoError(t, WriteHeader(f, updated))
	require.NoError(t, AppendRecord(f, rec))

	got, header, err := LoadRecordsFromMCAP(path)
	require.NoError(t, err)
	assert.Equal(t, updated, header)
	assert.Equal(t, []types.CaptureRecord{rec, rec}, got)
}

func TestWriteHeaderAndAppendRecord_fileErrors(t *testing.T) {
	path := writeCapture(t)

	closed, err := os.Open(path)
	require.NoError(t, err)
	require.NoError(t, closed.Close())
	assert.ErrorContains(t, WriteHeader(closed, testHeader), "failed to seek to start")
	assert.ErrorContains(t, AppendRecord(closed, types.CaptureRecord{}), "failed to seek to end")

	readOnly, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = readOnly.Close() }()
	assert.ErrorContains(t, WriteHeader(readOnly, testHeader), "failed to write magic")
	assert.ErrorContains(t, AppendRecord(readOnly, types.CaptureRecord{}), "failed to write iteration")
}

func TestReadHeader_errors(t *testing.T) {
	valid := captureBytes(t)
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", nil, "failed to read magic: EOF"},
		{"short magic", []byte("MC"), "failed to read magic: unexpected EOF"},
		{"wrong magic", []byte("PCAP\x01\x00\x02{}"), "invalid magic header"},
		{"no version", []byte("MCAP"), "failed to read version: EOF"},
		{"future version", []byte("MCAP\x02\x00\x02{}"), "unsupported version: 2"},
		{"no length", []byte("MCAP\x01"), "failed to read header length: EOF"},
		{"half a length", []byte("MCAP\x01\x00"), "failed to read header length: unexpected EOF"},
		{"truncated json", valid[:len(valid)-1], "failed to read header JSON: unexpected EOF"},
		{"missing json", []byte("MCAP\x01\x00\x05"), "failed to read header JSON: EOF"},
		{"invalid json", []byte("MCAP\x01\x00\x03{{{"), "failed to unmarshal header JSON: invalid character"},
		{"json of the wrong shape", []byte("MCAP\x01\x00\x02[]"), "failed to unmarshal header JSON: json: cannot unmarshal array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadHeader(bytes.NewReader(tt.in))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}

	h, err := ReadHeader(bytes.NewReader([]byte("MCAP\x01\x00\x02{}")))
	require.NoError(t, err)
	assert.Equal(t, types.CaptureHeader{}, h)
}

// encodedRecord is one record with 4 data bytes: 26 bytes of fixed fields, then the data.
func encodedRecord(t *testing.T) []byte {
	t.Helper()
	full := captureBytes(t, types.CaptureRecord{Iteration: 3, RequestTimestamp: 10, ResponseTimestamp: 20, StartAddress: 100, RegisterCount: 2, Data: []byte{1, 2, 3, 4}})
	rec := full[len(captureBytes(t)):]
	require.Len(t, rec, 30)
	return rec
}

func TestReadRecord_truncated(t *testing.T) {
	rec := encodedRecord(t)

	got, err := ReadRecord(bytes.NewReader(rec))
	require.NoError(t, err)
	assert.Equal(t, &types.CaptureRecord{Iteration: 3, RequestTimestamp: 10, ResponseTimestamp: 20, StartAddress: 100, RegisterCount: 2, Data: []byte{1, 2, 3, 4}}, got)

	// A clean end of file before a record is io.EOF, the loop terminator.
	_, err = ReadRecord(bytes.NewReader(nil))
	assert.ErrorIs(t, err, io.EOF)

	// A record cut anywhere, in the middle of a field or exactly between two fields
	// (4, 12, 20, 22, 24, 26), must be reported as truncated.
	for n := 1; n < len(rec); n++ {
		got, err := ReadRecord(bytes.NewReader(rec[:n]))
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF, "prefix of %d bytes", n)
		assert.Nil(t, got, "prefix of %d bytes", n)
	}
}

// Every reader of a capture must fail on a record that is cut short mid-field.
func TestReaders_truncatedRecord(t *testing.T) {
	good := types.CaptureRecord{Iteration: 0, StartAddress: 1, RegisterCount: 1, Data: []byte{0, 1}}
	full := captureBytes(t, good, good)
	// Cut exactly at a field boundary: the whole data block of the last record is missing.
	path := writeBytes(t, "cut.mcap", full[:len(full)-2])
	profile := writeBytes(t, "p.json", []byte(`{"protocolData":{"registers":[]}}`))

	n, err := CountRecords(path)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Equal(t, 1, n, "the records before the damage are still counted")

	recs, _, err := LoadRecordsFromMCAP(path)
	assert.ErrorContains(t, err, "failed to read record: unexpected EOF")
	assert.Nil(t, recs)

	exports := map[string]func(io.Writer) error{
		"csv":     func(w io.Writer) error { return ExportCSV(w, path) },
		"json":    func(w io.Writer) error { return ExportJSON(w, path) },
		"blocks":  func(w io.Writer) error { return ExportAddressBlocks(w, path) },
		"info":    func(w io.Writer) error { return ExportInfo(w, path) },
		"profile": func(w io.Writer) error { return ExportDeviceProfileDecode(w, path, profile) },
	}
	for name, export := range exports {
		t.Run(name, func(t *testing.T) {
			err := export(io.Discard)
			require.Error(t, err)
			assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
			assert.Contains(t, err.Error(), "failed to read record")
		})
	}
}

func TestReaders_missingFileAndBadHeader(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.mcap")
	bad := writeBytes(t, "bad.mcap", []byte("not an mcap file"))
	profile := writeBytes(t, "p.json", []byte(`{}`))

	_, err := CountRecords(missing)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = CountRecords(bad)
	assert.EqualError(t, err, "invalid magic header")

	_, _, err = LoadRecordsFromMCAP(missing)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.ErrorContains(t, err, "failed to open mcap file")
	_, _, err = LoadRecordsFromMCAP(bad)
	assert.EqualError(t, err, "failed to read header: invalid magic header")

	exports := map[string]func(w io.Writer, path string) error{
		"csv":     ExportCSV,
		"json":    ExportJSON,
		"blocks":  ExportAddressBlocks,
		"info":    ExportInfo,
		"profile": func(w io.Writer, path string) error { return ExportDeviceProfileDecode(w, path, profile) },
	}
	for name, export := range exports {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			err := export(&buf, missing)
			assert.ErrorIs(t, err, os.ErrNotExist)
			assert.ErrorContains(t, err, "failed to open mcap file")

			err = export(&buf, bad)
			assert.EqualError(t, err, "failed to read header: invalid magic header")
			assert.Empty(t, buf.String())
		})
	}
}

// failWriter fails every write.
type failWriter struct{}

var errSink = errors.New("sink closed")

func (failWriter) Write([]byte) (int, error) { return 0, errSink }

func TestExports_writerErrors(t *testing.T) {
	path := writeCapture(t, types.CaptureRecord{StartAddress: 1, RegisterCount: 1, Data: []byte{0, 1}})

	err := ExportCSV(failWriter{}, path)
	assert.ErrorIs(t, err, errSink)
	assert.ErrorContains(t, err, "csv writer")

	assert.ErrorIs(t, ExportJSON(failWriter{}, path), errSink)

	err = ExportAddressBlocks(failWriter{}, path)
	assert.ErrorIs(t, err, errSink)
	assert.ErrorContains(t, err, "failed to write blocks as JSON")
}

func TestExportCSV_exact(t *testing.T) {
	path := writeCapture(t,
		types.CaptureRecord{Iteration: 0, RequestTimestamp: 1_700_000_000_000_000_000, ResponseTimestamp: 1_700_000_000_005_000_000, StartAddress: 100, RegisterCount: 2, Data: []byte{0xDE, 0xAD, 0xBE, 0xEF}},
		types.CaptureRecord{Iteration: 1, RequestTimestamp: 1_700_000_001_000_000_000, ResponseTimestamp: 1_700_000_001_000_000_001, StartAddress: 65535, RegisterCount: 1, Data: nil},
	)
	var buf bytes.Buffer
	require.NoError(t, ExportCSV(&buf, path))
	assert.Equal(t,
		"ip,port,unit,function,iteration,request_timestamp,response_timestamp,start_address,register_count,data_hex\n"+
			"10.0.0.7,1502,9,4,0,"+rfc(1_700_000_000_000_000_000)+","+rfc(1_700_000_000_005_000_000)+",100,2,deadbeef\n"+
			"10.0.0.7,1502,9,4,1,"+rfc(1_700_000_001_000_000_000)+","+rfc(1_700_000_001_000_000_001)+",65535,1,\n",
		buf.String())

	buf.Reset()
	require.NoError(t, ExportCSV(&buf, writeCapture(t)))
	assert.Equal(t, "ip,port,unit,function,iteration,request_timestamp,response_timestamp,start_address,register_count,data_hex\n", buf.String())
}

func TestExportJSON_exact(t *testing.T) {
	path := writeCapture(t, types.CaptureRecord{Iteration: 2, RequestTimestamp: 1_700_000_000_000_000_000, ResponseTimestamp: 1_700_000_000_005_000_000, StartAddress: 100, RegisterCount: 1, Data: []byte{0x0A, 0xFF}})
	var buf bytes.Buffer
	require.NoError(t, ExportJSON(&buf, path))
	assert.JSONEq(t, `{
		"header": {"ip":"10.0.0.7","port":1502,"unit":9,"function":4,"start_time":"`+rfc(testHeader.StartTime)+`"},
		"records": [{
			"iteration":2,
			"request_timestamp":"`+rfc(1_700_000_000_000_000_000)+`",
			"response_timestamp":"`+rfc(1_700_000_000_005_000_000)+`",
			"start_address":100,"register_count":1,"raw_data":"0aff"}]
	}`, buf.String())
}

func TestExportAddressBlocks_exact(t *testing.T) {
	path := writeCapture(t,
		types.CaptureRecord{Iteration: 0, StartAddress: 300, RegisterCount: 2, Data: []byte{0, 0, 0, 0}},
		types.CaptureRecord{Iteration: 0, StartAddress: 100, RegisterCount: 1, Data: []byte{0, 0}},
		types.CaptureRecord{Iteration: 1, StartAddress: 300, RegisterCount: 2, Data: []byte{0, 0, 0, 0}},
		types.CaptureRecord{Iteration: 1, StartAddress: 100, RegisterCount: 1, Data: []byte{0, 0}},
	)
	var buf bytes.Buffer
	require.NoError(t, ExportAddressBlocks(&buf, path))
	assert.Equal(t,
		"[\n  {\n    \"start_address\": 100,\n    \"register_count\": 1\n  },\n  {\n    \"start_address\": 300,\n    \"register_count\": 2\n  }\n]\n",
		buf.String())

	buf.Reset()
	require.NoError(t, ExportAddressBlocks(&buf, writeCapture(t)))
	assert.Equal(t, "null\n", buf.String())
}

func TestExportInfo_exact(t *testing.T) {
	const t0 = int64(1_700_000_000_000_000_000)
	ms := int64(time.Millisecond)
	head := "Header Information:\n" +
		"  IP:        10.0.0.7\n" +
		"  Port:      1502\n" +
		"  Unit:      9\n" +
		"  Function:  4\n" +
		"  StartTime: " + rfc(testHeader.StartTime) + "\n\n" +
		"Record Summary:\n"

	t.Run("no records", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, ExportInfo(&buf, writeCapture(t)))
		assert.Equal(t, head+"  Iterations: 0\n  No records found.\n", buf.String())
	})

	t.Run("iterations are sorted and timed from first request to last response", func(t *testing.T) {
		path := writeCapture(t,
			// Iteration 5 is written first and out of time order: 30 ms in total.
			types.CaptureRecord{Iteration: 5, RequestTimestamp: t0 + 110*ms, ResponseTimestamp: t0 + 120*ms, StartAddress: 200, RegisterCount: 10, Data: make([]byte, 20)},
			types.CaptureRecord{Iteration: 5, RequestTimestamp: t0 + 100*ms, ResponseTimestamp: t0 + 105*ms, StartAddress: 50, RegisterCount: 5, Data: make([]byte, 10)},
			types.CaptureRecord{Iteration: 5, RequestTimestamp: t0 + 125*ms, ResponseTimestamp: t0 + 130*ms, StartAddress: 100, RegisterCount: 1, Data: make([]byte, 2)},
			// Iteration 1: 10 ms.
			types.CaptureRecord{Iteration: 1, RequestTimestamp: t0, ResponseTimestamp: t0 + 10*ms, StartAddress: 0, RegisterCount: 125, Data: make([]byte, 250)},
			// Iteration 9: 80 ms.
			types.CaptureRecord{Iteration: 9, RequestTimestamp: t0 + 200*ms, ResponseTimestamp: t0 + 280*ms, StartAddress: 65535, RegisterCount: 1, Data: make([]byte, 2)},
		)
		var buf bytes.Buffer
		require.NoError(t, ExportInfo(&buf, path))
		assert.Equal(t, head+
			"  Iterations: 3\n"+
			"    Iteration 1:\n"+
			"      Blocks: 1\n"+
			"      Total Registers: 125\n"+
			"      Time: "+rfc(t0)+" → "+rfc(t0+10*ms)+" (duration: 10ms)\n"+
			"      Address Range: 0 → 124\n"+
			"    Iteration 5:\n"+
			"      Blocks: 3\n"+
			"      Total Registers: 16\n"+
			"      Time: "+rfc(t0+100*ms)+" → "+rfc(t0+130*ms)+" (duration: 30ms)\n"+
			"      Address Range: 50 → 209\n"+
			"    Iteration 9:\n"+
			"      Blocks: 1\n"+
			"      Total Registers: 1\n"+
			"      Time: "+rfc(t0+200*ms)+" → "+rfc(t0+280*ms)+" (duration: 80ms)\n"+
			"      Address Range: 65535 → 65535\n"+
			"  Iteration Durations:\n"+
			"    Min: 10ms\n"+
			"    Avg: 40ms\n"+
			"    Max: 80ms\n",
			buf.String())
	})
}

func TestExportDeviceProfileDecode(t *testing.T) {
	capture := writeCapture(t,
		// Registers 100-103, then 110; register 101 is overwritten by a later iteration.
		types.CaptureRecord{Iteration: 0, StartAddress: 100, RegisterCount: 4, Data: []byte{0xFF, 0xFE, 0x00, 0x00, 0x3F, 0xC0, 0x00, 0x00}},
		types.CaptureRecord{Iteration: 0, StartAddress: 110, RegisterCount: 1, Data: []byte{0x12, 0x34}},
		types.CaptureRecord{Iteration: 1, StartAddress: 101, RegisterCount: 1, Data: []byte{0x01, 0xF4}},
		// Declares two registers but carries the data of one: only register 120 is known.
		types.CaptureRecord{Iteration: 1, StartAddress: 120, RegisterCount: 2, Data: []byte{0x00, 0x07}},
	)
	profile := writeBytes(t, "profile.json", []byte(`{"protocolData":{"registers":[
		{"controlledPropertyId":"signed","valueScaleFactor":1,"start":100,"size":1,"format":">h"},
		{"controlledPropertyId":"scaled","valueScaleFactor":0.1,"start":101,"size":1,"format":">H"},
		{"controlledPropertyId":"float","valueScaleFactor":2,"start":102,"size":2,"format":">f"},
		{"controlledPropertyId":"little","valueScaleFactor":1,"start":110,"size":1,"format":"<H"},
		{"controlledPropertyId":"not.used","valueScaleFactor":1,"start":9999,"size":1,"format":">H"},
		{"controlledPropertyId":"absent","valueScaleFactor":1,"start":500,"size":2,"format":">i"},
		{"controlledPropertyId":"half","valueScaleFactor":1,"start":120,"size":2,"format":">i"},
		{"controlledPropertyId":"odd","valueScaleFactor":1,"start":100,"size":1,"format":"u16"},
		{"controlledPropertyId":"short","valueScaleFactor":1,"start":110,"size":1,"format":">i"},
		{"controlledPropertyId":"unscaled","start":110,"size":1,"format":">H"},
		{"controlledPropertyId":"wide","valueScaleFactor":1,"start":100,"size":4,"format":">h"}
	]}}`))

	var buf bytes.Buffer
	require.NoError(t, ExportDeviceProfileDecode(&buf, capture, profile))
	assert.Equal(t,
		"Decoded Values:\n"+
			"signed = -2.000000\n"+
			"scaled = 50.000000\n"+
			"float = 3.000000\n"+
			"little = 13330.000000\n"+
			"⚠️  Missing data for absent at 500 (size 2)\n"+
			"⚠️  Missing data for half at 120 (size 2)\n"+
			"❌ Unsupported format for odd: u16\n"+
			"❌ Not enough bytes for short: have 2, need 4\n"+
			"unscaled = 0.000000\n"+
			"wide = -2.000000\n",
		buf.String())
}

func TestExportDeviceProfileDecode_profileErrors(t *testing.T) {
	capture := writeCapture(t)
	var buf bytes.Buffer

	err := ExportDeviceProfileDecode(&buf, capture, filepath.Join(t.TempDir(), "missing.json"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.ErrorContains(t, err, "failed to open device profile")

	err = ExportDeviceProfileDecode(&buf, capture, writeBytes(t, "bad.json", []byte("{not json")))
	assert.ErrorContains(t, err, "failed to decode device profile")
	assert.Empty(t, buf.String())

	// An empty profile over an empty capture still prints the heading.
	require.NoError(t, ExportDeviceProfileDecode(&buf, capture, writeBytes(t, "empty.json", []byte(`{}`))))
	assert.Equal(t, "Decoded Values:\n", buf.String())
}

// A record whose data does not fit the 16-bit length field is rejected before anything is
// written, leaving the capture readable.
func TestAppendRecord_oversizedDataRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.mcap")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.NoError(t, WriteHeader(f, types.CaptureHeader{IP: "127.0.0.1", Port: 502, Unit: 1, Function: 3}))
	require.NoError(t, AppendRecord(f, types.CaptureRecord{StartAddress: 1, RegisterCount: 1, Data: []byte{0, 1}}))

	err = AppendRecord(f, types.CaptureRecord{Data: make([]byte, 65536)})
	require.ErrorContains(t, err, "record data too large")

	n, err := CountRecords(path)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
