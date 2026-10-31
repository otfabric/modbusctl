// SPDX-License-Identifier: MIT

package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testHeader = types.CaptureHeader{IP: "10.0.0.7", Port: 1502, Unit: 9, Function: 4, StartTime: 1_700_000_000_123_456_789}

func writeCapture(t *testing.T, records ...types.CaptureRecord) string {
	t.Helper()
	return capfiletest.WriteRecords(t, capfiletest.Path(t, "capture.mbcap"), testHeader, records...)
}

func writeBytes(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, content, 0o600))
	return path
}

// twoTargets is a capture with FC03 samples of target 1 and FC04 samples of target 2.
func twoTargets(t *testing.T) string {
	c := &capture.Capture{
		Header:  capture.Header{TickNS: 1000, WallStart: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Producer: "test"},
		Targets: []capture.Target{{ID: 1, Label: "A", Unit: 1}, {ID: 2, Label: "B", Unit: 2}},
		Streams: []capture.Stream{{ID: 1, Target: 1, Role: "poll"}, {ID: 2, Target: 2, Role: "poll"}},
		Samples: []capture.Sample{
			{Stream: 1, At: 0, Request: []byte{0x03, 0, 10, 0, 1}, Response: []byte{0x03, 2, 0, 1}},
			{Stream: 2, At: 5, Request: []byte{0x04, 0, 20, 0, 1}, Response: []byte{0x04, 2, 0, 2}},
			{Stream: 1, At: 1_000_000, Request: []byte{0x03, 0, 10, 0, 1}, Response: []byte{0x03, 2, 0, 3}},
		},
		End: capture.End{Known: true, At: 2_000_000},
	}
	return capfiletest.Write(t, capfiletest.Path(t, "two.mbcap"), c)
}

func TestLoadRecords_selection(t *testing.T) {
	path := twoTargets(t)
	_, _, err := LoadRecords(path, config.CaptureSelection{})
	require.ErrorContains(t, err, "--target 1 --function 3 (2 samples); --target 2 --function 4 (1 samples)")

	recs, h, err := LoadRecords(path, config.CaptureSelection{Target: 1})
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, uint8(3), h.Function)
	assert.Equal(t, uint8(1), h.Unit)
	assert.Equal(t, []uint32{0, 1}, []uint32{recs[0].Iteration, recs[1].Iteration}, "the n-th poll of a block is iteration n")
	assert.Equal(t, []byte{0, 3}, recs[1].Data)

	recs, _, err = LoadRecords(path, config.CaptureSelection{Function: 4})
	require.NoError(t, err)
	require.Len(t, recs, 1)

	_, _, err = LoadRecords(path, config.CaptureSelection{Target: 2, Function: 3})
	require.ErrorContains(t, err, "no FC01-04 samples for target 2 and function 3")
}

func TestLoadRecords_legacyAndBadFiles(t *testing.T) {
	_, _, err := LoadRecords(writeBytes(t, "old.mbcap", []byte("MCAP\x01\x00\x02{}")), config.CaptureSelection{})
	require.ErrorIs(t, err, capfile.ErrLegacyFormat)
	_, _, err = LoadRecords(writeBytes(t, "x.txt", []byte("hello")), config.CaptureSelection{})
	require.ErrorIs(t, err, capture.ErrCorrupt)
	_, _, err = LoadRecords(filepath.Join(t.TempDir(), "missing.mbcap"), config.CaptureSelection{})
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestExportAddressBlocks_exact(t *testing.T) {
	path := writeCapture(t,
		types.CaptureRecord{Iteration: 0, StartAddress: 300, RegisterCount: 2, Data: []byte{0, 0, 0, 0}},
		types.CaptureRecord{Iteration: 0, StartAddress: 100, RegisterCount: 1, Data: []byte{0, 0}},
		types.CaptureRecord{Iteration: 1, StartAddress: 300, RegisterCount: 2, Data: []byte{0, 0, 0, 0}},
		types.CaptureRecord{Iteration: 1, StartAddress: 100, RegisterCount: 1, Data: []byte{0, 0}},
	)
	var buf bytes.Buffer
	require.NoError(t, ExportAddressBlocks(&buf, path, config.CaptureSelection{}))
	assert.Equal(t,
		"[\n  {\n    \"start_address\": 100,\n    \"register_count\": 1\n  },\n  {\n    \"start_address\": 300,\n    \"register_count\": 2\n  }\n]\n",
		buf.String())

	buf.Reset()
	require.NoError(t, ExportAddressBlocks(&buf, writeCapture(t), config.CaptureSelection{}))
	assert.Equal(t, "null\n", buf.String())
}

func TestExportDeviceProfileDecode(t *testing.T) {
	capture := writeCapture(t,
		// Registers 100-103, then 110; register 101 is overwritten by a later poll.
		types.CaptureRecord{Iteration: 0, StartAddress: 100, RegisterCount: 4, Data: []byte{0xFF, 0xFE, 0x00, 0x00, 0x3F, 0xC0, 0x00, 0x00}},
		types.CaptureRecord{Iteration: 0, StartAddress: 110, RegisterCount: 1, Data: []byte{0x12, 0x34}},
		types.CaptureRecord{Iteration: 1, StartAddress: 101, RegisterCount: 1, Data: []byte{0x01, 0xF4}},
		// The data of one register: only register 120 is known.
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
	require.NoError(t, ExportDeviceProfileDecode(&buf, capture, profile, config.CaptureSelection{}))
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

func TestExportDeviceProfileDecode_errors(t *testing.T) {
	capture := writeCapture(t)
	var buf bytes.Buffer

	err := ExportDeviceProfileDecode(&buf, capture, filepath.Join(t.TempDir(), "missing.json"), config.CaptureSelection{})
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.ErrorContains(t, err, "failed to open device profile")

	err = ExportDeviceProfileDecode(&buf, capture, writeBytes(t, "bad.json", []byte("{not json")), config.CaptureSelection{})
	assert.ErrorContains(t, err, "failed to decode device profile")
	assert.Empty(t, buf.String())

	// An empty profile over an empty capture still prints the heading.
	require.NoError(t, ExportDeviceProfileDecode(&buf, capture, writeBytes(t, "empty.json", []byte(`{}`)), config.CaptureSelection{}))
	assert.Equal(t, "Decoded Values:\n", buf.String())

	// Coils cannot be decoded as registers.
	coils := capfiletest.WriteRecords(t, capfiletest.Path(t, "coils.mbcap"), types.CaptureHeader{Unit: 1, Function: 1},
		types.CaptureRecord{StartAddress: 0, RegisterCount: 8, Data: []byte{0xFF}})
	err = ExportDeviceProfileDecode(&buf, coils, writeBytes(t, "e.json", []byte(`{}`)), config.CaptureSelection{})
	assert.True(t, strings.Contains(err.Error(), "needs register samples"), err.Error())
}
