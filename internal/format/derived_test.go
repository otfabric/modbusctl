package format

import (
	"bytes"
	"encoding/binary"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCapture(t *testing.T, records ...types.CaptureRecord) string {
	t.Helper()
	return capfiletest.WriteRecords(t, capfiletest.Path(t, "capture.mbcap"),
		types.CaptureHeader{IP: "127.0.0.1", Port: 502, Unit: 1, Function: 3}, records...)
}

func TestExportDerived_loadErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.mbcap")
	var buf bytes.Buffer

	err := ExportStrings(&buf, config.StringsConfig{InputFile: missing})
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to load records: open "), err.Error())

	err = ExportHeuristicFrequency(&buf, missing, config.CaptureSelection{})
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to load records: open "), err.Error())
	assert.Empty(t, buf.String())
}

func TestExportStrings_stitchesAndDedupes(t *testing.T) {
	// "ABCDEFGH" split over two adjacent blocks of one iteration; the same block again in
	// the next iteration must be reported separately (no stitching across iterations).
	path := writeCapture(t,
		types.CaptureRecord{Iteration: 0, StartAddress: 102, RegisterCount: 2, Data: []byte("EFGH")},
		types.CaptureRecord{Iteration: 0, StartAddress: 100, RegisterCount: 2, Data: []byte("ABCD")},
		types.CaptureRecord{Iteration: 1, StartAddress: 100, RegisterCount: 2, Data: []byte("ABCD")},
	)
	var buf bytes.Buffer
	require.NoError(t, ExportStrings(&buf, config.StringsConfig{InputFile: path}))
	assert.Equal(t,
		"[ABCD] [100-103] 4 regs: ABCDEFGH\n"+
			"[BADC] [100-103] 4 regs: BADCFEHG\n"+
			"[CDAB] [100-103] 4 regs: CDABGHEF\n"+
			"[DCBA] [100-103] 4 regs: DCBAHGFE\n"+
			"[ABCD] [100-101] 2 regs: ABCD\n"+
			"[BADC] [100-101] 2 regs: BADC\n"+
			"[CDAB] [100-101] 2 regs: CDAB\n"+
			"[DCBA] [100-101] 2 regs: DCBA\n",
		buf.String())
}

func TestExportHeuristicFrequency_exactLineAndLimit(t *testing.T) {
	f32 := make([]byte, 4)
	binary.BigEndian.PutUint32(f32, math.Float32bits(49.95))

	var buf bytes.Buffer
	require.NoError(t, ExportHeuristicFrequency(&buf, writeCapture(t,
		types.CaptureRecord{StartAddress: 40070, RegisterCount: 2, Data: f32}), config.CaptureSelection{}))
	assert.Equal(t, "[40070] 2 regs (float32 BE) = 49.9500 → candidate frequency (confidence: 0.75)\n", buf.String())

	// 30 registers of 5000 (centi-Hz) give 30 uint16 BE and 30 int16 BE candidates; only 20 are reported.
	many := bytes.Repeat([]byte{0x13, 0x88}, 30)
	buf.Reset()
	require.NoError(t, ExportHeuristicFrequency(&buf, writeCapture(t,
		types.CaptureRecord{StartAddress: 0, RegisterCount: 30, Data: many}), config.CaptureSelection{}))
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	require.Len(t, lines, 20)
	for _, l := range lines {
		assert.Regexp(t, `^\[\d+\] 1 regs \(u?int16 BE\) = 5000\.0000 → candidate frequency \(confidence: 0\.95\)$`, l)
	}

	buf.Reset()
	require.NoError(t, ExportHeuristicFrequency(&buf, writeCapture(t), config.CaptureSelection{}))
	assert.Empty(t, buf.String())
}

func TestReorderASCIIBytes(t *testing.T) {
	src := []byte("ABCDEFGHxyz")
	tests := []struct {
		order ASCIIOrder
		want  string
	}{
		{ASCIIOrderABCD, "ABCDEFGHxyz"},
		{ASCIIOrderBADC, "BADCFEHGxyz"},
		{ASCIIOrderCDAB, "CDABGHEFxyz"},
		{ASCIIOrderDCBA, "DCBAHGFExyz"},
		{ASCIIOrder("????"), "ABCDEFGHxyz"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, string(reorderASCIIBytes(src, tt.order)), string(tt.order))
	}
	assert.Equal(t, "ABCDEFGHxyz", string(src), "the source must not be modified")
	assert.Nil(t, reorderASCIIBytes(nil, ASCIIOrderDCBA))
	assert.Equal(t, "ab", string(reorderASCIIBytes([]byte("ab"), ASCIIOrderDCBA)))
}

func TestProcessASCII(t *testing.T) {
	t.Run("palindromic layouts are reported once", func(t *testing.T) {
		var buf bytes.Buffer
		processASCII(&buf, &types.CaptureRecord{StartAddress: 10, RegisterCount: 2, Data: []byte("AAAA")})
		assert.Equal(t, "[ABCD] [10-11] 2 regs: AAAA\n", buf.String())
	})
	t.Run("runs shorter than four characters and non-printable bytes are skipped", func(t *testing.T) {
		var buf bytes.Buffer
		data := []byte{0x00, 0x00, 'a', 'b', 'c', 0x00, 'T', 'E', 'S', 'T', '!', 0x7F}
		processASCIIForOrder(&buf, &types.CaptureRecord{StartAddress: 200}, data, ASCIIOrderABCD, 4, map[string]struct{}{})
		assert.Equal(t, "[ABCD] [203-205] 3 regs: TEST!\n", buf.String())
	})
	t.Run("run reaching the end of the data", func(t *testing.T) {
		var buf bytes.Buffer
		data := []byte{0x01, 'l', 'a', 's', 't'}
		processASCIIForOrder(&buf, &types.CaptureRecord{StartAddress: 0}, data, ASCIIOrderBADC, 4, map[string]struct{}{})
		assert.Equal(t, "[BADC] [0-2] 3 regs: last\n", buf.String())
	})
	t.Run("no data", func(t *testing.T) {
		var buf bytes.Buffer
		processASCII(&buf, &types.CaptureRecord{})
		assert.Empty(t, buf.String())
	})
}

func TestIsPrintableASCII(t *testing.T) {
	assert.False(t, isPrintableASCII(31))
	assert.True(t, isPrintableASCII(' '))
	assert.True(t, isPrintableASCII('~'))
	assert.False(t, isPrintableASCII(127))
}

func TestStitch_edgeCases(t *testing.T) {
	assert.Nil(t, stitchAdjacentRecords(nil))
	assert.Nil(t, stitchContiguousSameIteration(nil))

	got := stitchAdjacentRecords([]types.CaptureRecord{
		{Iteration: 2, StartAddress: 10, RegisterCount: 1, Data: []byte{9, 9}},
		{Iteration: 1, StartAddress: 5, RegisterCount: 1, Data: []byte{3, 3}},
		{Iteration: 1, StartAddress: 0, RegisterCount: 2, Data: []byte{1, 1, 2, 2}},
		{Iteration: 1, StartAddress: 2, RegisterCount: 1, Data: []byte{0xA, 0xB}},
	})
	assert.Equal(t, []types.CaptureRecord{
		{Iteration: 1, StartAddress: 0, RegisterCount: 3, Data: []byte{1, 1, 2, 2, 0xA, 0xB}},
		{Iteration: 1, StartAddress: 5, RegisterCount: 1, Data: []byte{3, 3}},
		{Iteration: 2, StartAddress: 10, RegisterCount: 1, Data: []byte{9, 9}},
	}, got)
}

func TestFrequencyConfidence_edges(t *testing.T) {
	assert.Zero(t, frequencyConfidence(47.5), "the lower bound is 2.5 Hz away and scores nothing")
	assert.InDelta(t, 0.95, frequencyConfidence(50), 1e-9)
	assert.InDelta(t, 0.95, frequencyConfidence(500), 1e-9)
	assert.InDelta(t, 0.95, frequencyConfidence(5000), 1e-9)
	assert.InDelta(t, 0.5, frequencyConfidence(50.1), 1e-9)
	assert.InDelta(t, 0.4, frequencyConfidence(494), 1e-9)
	assert.InDelta(t, 0.2, frequencyConfidence(5175), 1e-9)
	assert.Zero(t, frequencyConfidence(52.01))
	assert.Zero(t, frequencyConfidence(474))
	assert.Zero(t, frequencyConfidence(-50))
}

func TestProcessFrequency_skipsNaNAndInf(t *testing.T) {
	nan := make([]byte, 8)
	binary.BigEndian.PutUint64(nan, math.Float64bits(math.NaN()))
	inf := make([]byte, 4)
	binary.BigEndian.PutUint32(inf, math.Float32bits(float32(math.Inf(1))))
	assert.Empty(t, processFrequency([]types.CaptureRecord{
		{StartAddress: 0, RegisterCount: 4, Data: nan},
		{StartAddress: 10, RegisterCount: 2, Data: inf},
		{StartAddress: 20, RegisterCount: 1, Data: []byte{0x01}},
	}))
}
