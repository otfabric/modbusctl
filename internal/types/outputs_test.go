package types_test

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderer is the text/table surface every output DTO exposes.
type renderer interface {
	MarshalTextOutput() (string, error)
	TableHeaders() []string
	TableRows() [][]string
}

func text(t *testing.T, r renderer) string {
	t.Helper()
	s, err := r.MarshalTextOutput()
	require.NoError(t, err)
	return s
}

func ptr[T any](v T) *T { return &v }

func errInfo(msg string) *types.ErrorInfo { return types.EmbeddedModbusError(msg) }

// Nil receivers render as nothing but still describe their columns.
func TestOutputs_nilReceivers(t *testing.T) {
	tests := []struct {
		name    string
		r       renderer
		headers []string
		text    string
	}{
		{"decode", (*types.DecodeResult)(nil), []string{"field", "value"}, ""},
		{"codecs", (*types.CodecList)(nil), []string{"id", "name", "family", "value_kind", "registers", "bytes"}, "No codecs match the given filters."},
		{"diagnostic", (*types.DiagnosticResult)(nil), []string{"field", "value"}, ""},
		{"fingerprint", (*types.FingerprintResult)(nil), []string{"unit_id", "function", "error"}, ""},
		{"identify", (*types.IdentifyResult)(nil), []string{"unit_id", "object_id", "description", "value"}, ""},
		{"poll sample", (*types.PollSample)(nil), []string{"poll", "timestamp", "rtt_ms"}, ""},
		{"read", (*types.ReadResult)(nil), []string{"field", "value"}, ""},
		{"record", (*types.RecordSummaryResult)(nil), []string{"field", "value"}, ""},
		{"report server id", (*types.ReportServerIDResult)(nil), []string{"unit_id", "data_hex", "run_indicator", "error"}, ""},
		{"scan", (*types.ScanSummaryResult)(nil), []string{"metric", "value"}, ""},
		{"sunspec detect", (*types.SunSpecDetectOutput)(nil), []string{"unit_id", "detected", "base_address", "regtype"}, ""},
		{"sunspec models", (*types.SunSpecModelsOutput)(nil), []string{"start", "end", "model_id", "length", "end_model"}, ""},
		{"sunspec map", (*types.SunSpecMapOutput)(nil), []string{"start", "end", "model_id", "end"}, ""},
		{"sunspec probe", (*types.SunSpecProbeOutput)(nil), []string{"section", "key", "value"}, ""},
		{"write", (*types.WriteResult)(nil), []string{"address", "type", "value"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.text, text(t, tt.r))
			assert.Equal(t, tt.headers, tt.r.TableHeaders())
			assert.Nil(t, tt.r.TableRows())
		})
	}
}

func TestDecodeResult(t *testing.T) {
	base := types.DecodeResult{
		Target: "tcp://10.0.0.1:502", UnitID: 7, Function: 3, StartAddress: 100, RegisterCount: 2,
		Codec: "uint32/be", Family: "integer", ValueKind: "uint32", RawDataHex: "00010002",
	}
	fixed := [][]string{
		{"target", "tcp://10.0.0.1:502"},
		{"unit_id", "7"},
		{"function", "3"},
		{"start_address", "100"},
		{"register_count", "2"},
		{"codec", "uint32/be"},
		{"family", "integer"},
		{"value_kind", "uint32"},
		{"raw_data_hex", "00010002"},
	}
	header := "Decoding 2 register(s) from address 100 using function code 3\n" +
		"Codec: uint32/be [family=integer valueKind=uint32]\n" +
		"Raw data: 00010002\n"

	t.Run("no values", func(t *testing.T) {
		r := base
		assert.Equal(t, header+"Decoded value: (none)\n", text(t, &r))
		assert.Equal(t, fixed, r.TableRows())
	})
	t.Run("one value", func(t *testing.T) {
		r := base
		r.Values = []any{uint32(65538)}
		assert.Equal(t, header+"Decoded value: 65538\n", text(t, &r))
		assert.Equal(t, append(append([][]string(nil), fixed...), []string{"value", "65538"}), r.TableRows())
	})
	t.Run("several values", func(t *testing.T) {
		r := base
		r.Values = []any{int16(-1), "abc", nil}
		assert.Equal(t, header+"Decoded 3 values:\n  [0] -1\n  [1] abc\n  [2] \n", text(t, &r))
		assert.Equal(t,
			append(append([][]string(nil), fixed...), []string{"value[0]", "-1"}, []string{"value[1]", "abc"}, []string{"value[2]", ""}),
			r.TableRows())
	})
}

func TestFormatDecodedValue(t *testing.T) {
	ts := time.Date(2026, 10, 9, 12, 30, 45, 0, time.FixedZone("x", 2*3600))
	tests := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{ts, "2026-10-09T12:30:45+02:00"},
		{net.IPv4(192, 168, 1, 20), "192.168.1.20"},
		{net.ParseIP("fe80::1"), "fe80::1"},
		{"plain", "plain"},
		{uint16(65535), "65535"},
		{int32(-5), "-5"},
		{float64(1.5), "1.5"},
		{float32(0.25), "0.25"},
		{true, "true"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, types.FormatDecodedValue(tt.in), "%#v", tt.in)
	}
}

func TestCodecList(t *testing.T) {
	empty := &types.CodecList{}
	assert.Equal(t, "No codecs match the given filters.", text(t, empty))
	assert.Empty(t, empty.TableRows())
	assert.NotNil(t, empty.TableRows())

	l := &types.CodecList{Codecs: []types.CodecInfo{
		{ID: "uint16/be", Name: "Unsigned 16", Family: "integer", ValueKind: "uint16", Registers: 1, Bytes: 2},
		{ID: "float32/le", Name: "Float 32", Family: "float", ValueKind: "float32", Registers: 2, Bytes: 4},
	}}
	assert.Equal(t,
		"uint16/be\tUnsigned 16\tregisters=1 bytes=2 family=integer valueKind=uint16\n"+
			"float32/le\tFloat 32\tregisters=2 bytes=4 family=float valueKind=float32\n",
		text(t, l))
	assert.Equal(t, [][]string{
		{"uint16/be", "Unsigned 16", "integer", "uint16", "1", "2"},
		{"float32/le", "Float 32", "float", "float32", "2", "4"},
	}, l.TableRows())
}

func TestPollSample(t *testing.T) {
	t.Run("registers without codec", func(t *testing.T) {
		s := &types.PollSample{
			Poll: 1, Timestamp: "2026-10-09T12:00:00.123Z", UnitID: 1, Function: 3,
			StartAddress: 100, Count: 2, RTTMs: 4.26, Values: []any{uint16(1234), uint16(5678)},
		}
		assert.Equal(t, "2026-10-09T12:00:00.123Z  #1  100=1234 101=5678", text(t, s))
		assert.Equal(t, []string{"poll", "timestamp", "rtt_ms", "100", "101", "error"}, s.TableHeaders())
		assert.Equal(t, [][]string{{"1", "2026-10-09T12:00:00.123Z", "4.3", "1234", "5678", ""}}, s.TableRows())
	})
	t.Run("coils render as 0 and 1", func(t *testing.T) {
		s := &types.PollSample{
			Poll: 12, Timestamp: "T", Function: 1, StartAddress: 8, Count: 3, AddressStep: 1,
			Values: []any{true, false, true},
		}
		assert.Equal(t, "T  #12  8=1 9=0 10=1", text(t, s))
		assert.Equal(t, []string{"poll", "timestamp", "rtt_ms", "8", "9", "10", "error"}, s.TableHeaders())
		assert.Equal(t, [][]string{{"12", "T", "0.0", "1", "0", "1", ""}}, s.TableRows())
	})
	t.Run("codec spanning two registers steps addresses by two", func(t *testing.T) {
		s := &types.PollSample{
			Poll: 3, Timestamp: "T", Function: 4, StartAddress: 65530, Count: 6, AddressStep: 2,
			Codec: "float32/be", RTTMs: 10, Values: []any{float32(1.5), float32(-2), "x"},
		}
		assert.Equal(t, "T  #3  65530=1.5 65532=-2 65534=x", text(t, s))
		assert.Equal(t, []string{"poll", "timestamp", "rtt_ms", "65530", "65532", "65534", "error"}, s.TableHeaders())
		assert.Equal(t, [][]string{{"3", "T", "10.0", "1.5", "-2", "x", ""}}, s.TableRows())
	})
	t.Run("error keeps the value columns but leaves them empty", func(t *testing.T) {
		s := &types.PollSample{
			Poll: 2, Timestamp: "T", Function: 3, StartAddress: 100, Count: 2, RTTMs: 250.04,
			Values: []any{uint16(1), uint16(2)}, Error: errInfo("request timed out"),
		}
		assert.Equal(t, "T  #2  error: request timed out", text(t, s))
		assert.Equal(t, []string{"poll", "timestamp", "rtt_ms", "100", "101", "error"}, s.TableHeaders())
		assert.Equal(t, [][]string{{"2", "T", "250.0", "", "", "request timed out"}}, s.TableRows())
	})
	t.Run("fewer values than columns pads with empty cells", func(t *testing.T) {
		s := &types.PollSample{Poll: 1, Timestamp: "T", StartAddress: 0, Count: 3, Values: []any{uint16(9)}}
		assert.Equal(t, [][]string{{"1", "T", "0.0", "9", "", "", ""}}, s.TableRows())
	})
	t.Run("json omits empty optional fields", func(t *testing.T) {
		s := &types.PollSample{Poll: 1, Timestamp: "T", UnitID: 2, Function: 3, StartAddress: 4, Count: 1, AddressStep: 1, RTTMs: 1.5, Values: []any{uint16(7)}}
		b, err := json.Marshal(s)
		require.NoError(t, err)
		assert.JSONEq(t, `{"poll":1,"timestamp":"T","unit_id":2,"function":3,"start_address":4,"count":1,"address_step":1,"rtt_ms":1.5,"values":[7]}`, string(b))
	})
}

func TestPollSummary_String(t *testing.T) {
	assert.Equal(t, "", (*types.PollSummary)(nil).String())
	p := &types.PollSummary{Target: "tcp://h:502", Polls: 10, Succeeded: 8, Failed: 2, Duration: "9.5s"}
	assert.Equal(t, "10 poll(s) of tcp://h:502 in 9.5s: 8 succeeded, 2 failed", p.String())
	p.Interrupted = true
	assert.Equal(t, "10 poll(s) of tcp://h:502 in 9.5s: 8 succeeded, 2 failed (interrupted)", p.String())
}

func TestWriteResult(t *testing.T) {
	t.Run("single register", func(t *testing.T) {
		r := &types.WriteResult{Target: "tcp://h:502", UnitID: 3, Function: 6, StartAddress: 100, Count: 1, Registers: []uint16{4660}}
		assert.Equal(t,
			"Writing 1 register(s) starting at address 100 using function code 6\n"+
				"  [100] 4660 (0x1234)\n"+
				"✅ Wrote 1 register(s) to unit 3\n",
			text(t, r))
		assert.Equal(t, [][]string{{"100", "register", "4660 (0x1234)"}}, r.TableRows())
	})
	t.Run("multiple registers with codec at the top of the address space", func(t *testing.T) {
		r := &types.WriteResult{UnitID: 1, Function: 16, StartAddress: 65534, Count: 2, Codec: "float32/be", Registers: []uint16{0x3FC0, 0}, RawDataHex: "3fc00000"}
		assert.Equal(t,
			"Writing 2 register(s) starting at address 65534 using function code 16\n"+
				"Codec: float32/be\n"+
				"  [65534] 16320 (0x3FC0)\n"+
				"  [65535] 0 (0x0000)\n"+
				"✅ Wrote 2 register(s) to unit 1\n",
			text(t, r))
		assert.Equal(t, [][]string{
			{"65534", "register", "16320 (0x3FC0)"},
			{"65535", "register", "0 (0x0000)"},
		}, r.TableRows())
	})
	t.Run("single coil", func(t *testing.T) {
		r := &types.WriteResult{UnitID: 9, Function: 5, StartAddress: 0, Count: 1, Coils: []bool{true}}
		assert.Equal(t,
			"Writing 1 coil(s) starting at address 0 using function code 5\n"+
				"  [0] 1\n"+
				"✅ Wrote 1 coil(s) to unit 9\n",
			text(t, r))
		assert.Equal(t, [][]string{{"0", "coil", "1"}}, r.TableRows())
	})
	t.Run("multiple coils", func(t *testing.T) {
		r := &types.WriteResult{UnitID: 9, Function: 15, StartAddress: 8, Count: 3, Coils: []bool{true, false, true}}
		assert.Equal(t,
			"Writing 3 coil(s) starting at address 8 using function code 15\n"+
				"  [8] 1\n  [9] 0\n  [10] 1\n"+
				"✅ Wrote 3 coil(s) to unit 9\n",
			text(t, r))
		assert.Equal(t, [][]string{{"8", "coil", "1"}, {"9", "coil", "0"}, {"10", "coil", "1"}}, r.TableRows())
	})
	t.Run("nothing written", func(t *testing.T) {
		r := &types.WriteResult{Function: 16}
		assert.Empty(t, r.TableRows())
	})
	t.Run("json has exactly one of coils and registers", func(t *testing.T) {
		b, err := json.Marshal(&types.WriteResult{Target: "t", UnitID: 1, Function: 5, StartAddress: 2, Count: 1, Coils: []bool{false}})
		require.NoError(t, err)
		assert.JSONEq(t, `{"target":"t","unit_id":1,"function":5,"start_address":2,"count":1,"coils":[false]}`, string(b))
	})
}

func TestDiagnosticResult_table(t *testing.T) {
	r := &types.DiagnosticResult{Target: "tcp://h:502", UnitID: 4, SubFunction: "return-query-data", SubFunctionHex: 0, DataHex: "a537"}
	assert.Equal(t,
		"🔍 Sending FC08 Diagnostics to tcp://h:502 (unit 4, sub-function return-query-data / 0x0000)...\n"+
			"✅ Diagnostics response:\n"+
			"  Sub-function: 0x0000 (return-query-data)\n"+
			"  Data:         a537\n",
		text(t, r))
	assert.Equal(t, [][]string{
		{"target", "tcp://h:502"},
		{"unit_id", "4"},
		{"sub_function", "return-query-data"},
		{"sub_function_hex", "0x0000"},
		{"data_hex", "a537"},
	}, r.TableRows())
}

func TestRecordSummaryResult(t *testing.T) {
	r := &types.RecordSummaryResult{Target: "tcp://h:502", BlocksRecorded: 12, Iterations: 4, CaptureOutputPath: "out.mbcap"}
	assert.Equal(t, "📦 Total recorded blocks: 12\n🔄 Total iterations: 4\n✅ Output written to out.mbcap\n", text(t, r))
	assert.Equal(t, [][]string{
		{"target", "tcp://h:502"},
		{"blocks_recorded", "12"},
		{"iterations", "4"},
		{"capture_output_path", "out.mbcap"},
	}, r.TableRows())
}

func TestFingerprintResult(t *testing.T) {
	t.Run("single unit with reads", func(t *testing.T) {
		r := &types.FingerprintResult{Target: "tcp://h:502", Units: []types.FingerprintUnitResult{
			{UnitID: 1, SupportedReads: []string{"FC03 Read Holding Registers", "FC04 Read Input Registers"}},
		}}
		assert.Equal(t,
			"🔍 Fingerprinting device at tcp://h:502 (supported read functions per unit)...\n"+
				"✅ Unit 1: supported read functions:\n"+
				"  FC03 Read Holding Registers\n"+
				"  FC04 Read Input Registers\n",
			text(t, r))
		assert.Equal(t, [][]string{
			{"1", "FC03 Read Holding Registers", ""},
			{"1", "FC04 Read Input Registers", ""},
		}, r.TableRows())
		assert.False(t, r.HasPartialFailure())
	})
	t.Run("single unit without reads", func(t *testing.T) {
		r := &types.FingerprintResult{Target: "t", Units: []types.FingerprintUnitResult{{UnitID: 5}}}
		assert.Equal(t,
			"🔍 Fingerprinting device at t (supported read functions per unit)...\n"+
				"— No supported read functions detected for unit 5\n",
			text(t, r))
		assert.Equal(t, [][]string{{"5", "(none)", ""}}, r.TableRows())
	})
	t.Run("every unit state with summary", func(t *testing.T) {
		r := &types.FingerprintResult{Target: "t", Units: []types.FingerprintUnitResult{
			{UnitID: 1, SupportedReads: []string{"FC03"}},
			{UnitID: 2, Error: errInfo("timeout")},
			{UnitID: 3, ProbeInterrupted: true},
			{UnitID: 4, ProbeInterrupted: true, SupportedReads: []string{"FC01", "FC02"}},
			{UnitID: 5, ProbeInterrupted: true, SupportedReads: []string{"FC04"}, Error: errInfo("connection reset")},
			{UnitID: 6},
		}}
		types.FillFingerprintSummary(r, 6)
		require.Equal(t, &types.ResultSummary{Requested: 6, Succeeded: 4, Failed: 2}, r.Summary)
		assert.Equal(t,
			"🔍 Fingerprinting device at t (supported read functions per unit)...\n"+
				"Units requested: 6  succeeded: 4  failed: 2\n\n"+
				"\n--- Unit ID 1 ---\n"+
				"✅ Unit 1: supported read functions:\n  FC03\n"+
				"\n--- Unit ID 2 ---\n"+
				"⚠️ Unit 2: timeout\n"+
				"\n--- Unit ID 3 ---\n"+
				"⚠️ Unit 3: fingerprint probe interrupted before any supported function was confirmed\n"+
				"\n--- Unit ID 4 ---\n"+
				"⚠️ Unit 4: probe interrupted after partial results\n"+
				"✅ Unit 4: supported read functions (partial):\n  FC01\n  FC02\n"+
				"\n--- Unit ID 5 ---\n"+
				"⚠️ Unit 5: connection reset\n"+
				"✅ Unit 5: supported read functions (partial):\n  FC04\n"+
				"\n--- Unit ID 6 ---\n",
			text(t, r))
		assert.Equal(t, [][]string{
			{"1", "FC03", ""},
			{"2", "", "timeout"},
			{"3", "", "interrupted before any supported function"},
			{"4", "", "partial (probe interrupted)"},
			{"4", "FC01", ""},
			{"4", "FC02", ""},
			{"5", "", "connection reset"},
			{"5", "FC04", ""},
			{"6", "(none)", ""},
		}, r.TableRows())
		assert.True(t, r.HasPartialFailure())
		assert.Equal(t, errs.ExitPartial, types.SuccessExitForPayload(r))
	})
	t.Run("no units", func(t *testing.T) {
		r := &types.FingerprintResult{Target: "t"}
		assert.Equal(t, "🔍 Fingerprinting device at t (supported read functions per unit)...\n", text(t, r))
		assert.Nil(t, r.TableRows())
	})
}

func TestFingerprintUnitPredicates(t *testing.T) {
	tests := []struct {
		name             string
		u                types.FingerprintUnitResult
		partial, failing bool
	}{
		{"clean", types.FingerprintUnitResult{SupportedReads: []string{"FC03"}}, false, false},
		{"empty", types.FingerprintUnitResult{}, false, false},
		{"error", types.FingerprintUnitResult{Error: errInfo("x")}, false, true},
		{"error with empty message", types.FingerprintUnitResult{Error: &types.ErrorInfo{}}, false, false},
		{"interrupted early", types.FingerprintUnitResult{ProbeInterrupted: true}, false, true},
		{"interrupted late", types.FingerprintUnitResult{ProbeInterrupted: true, SupportedReads: []string{"FC03"}}, true, false},
		{"reads with error but not interrupted", types.FingerprintUnitResult{Error: errInfo("x"), SupportedReads: []string{"FC03"}}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.partial, types.FingerprintUnitPartial(tt.u))
			assert.Equal(t, tt.failing, types.FingerprintUnitUnusable(tt.u))
			assert.Equal(t, tt.failing, types.FingerprintUnitFailed(tt.u))
		})
	}
}

func TestIdentifyResult_multiUnit(t *testing.T) {
	r := &types.IdentifyResult{Target: "tcp://h:502", Units: []types.IdentifyUnitResult{
		{
			UnitID: 1, Category: ptr(uint8(2)), ConformityLevel: ptr(uint8(0x82)), MoreFollows: ptr(true), NextObjectID: ptr(uint8(5)),
			Objects: []types.IdentifyObjectRow{
				{ID: 0, Value: "Acme", Description: "VendorName"},
				{ID: 4, Value: "raw"},
			},
			ReportServerID: &types.IdentifyReportServerOutput{DataHex: "0102", RunIndicatorOn: ptr(false)},
		},
		{UnitID: 2, Error: errInfo("illegal function")},
		{UnitID: 3, ReportServerID: &types.IdentifyReportServerOutput{Error: errInfo("fc17 unsupported"), RunIndicatorOn: ptr(true)}},
		{UnitID: 4, Category: ptr(uint8(1)), Objects: []types.IdentifyObjectRow{{ID: 1, Value: "only"}}, ReportServerID: &types.IdentifyReportServerOutput{}},
	}}
	types.FillIdentifySummary(r, 4)
	require.Equal(t, &types.ResultSummary{Requested: 4, Succeeded: 3, Failed: 1}, r.Summary)

	assert.Equal(t,
		"🔍 Connecting to tcp://h:502...\n"+
			"Units requested: 4  succeeded: 3  failed: 1\n\n"+
			"\n--- Unit ID 1 ---\n"+
			"✅ Device Identification (Category: 2, Conformity Level: 0x82, More Follows: true, Next Object ID: 5, Object Count: 2)\n"+
			" - Object 0: Acme [VendorName]\n"+
			" - Object 4: raw\n"+
			"  FC17 Report Server ID: 0102\n"+
			"  Run Indicator: OFF\n"+
			"\n--- Unit ID 2 ---\n"+
			"⚠️ Unit 2: illegal function\n"+
			"\n--- Unit ID 3 ---\n"+
			"  FC17 Report Server ID: ⚠️ fc17 unsupported\n"+
			"\n--- Unit ID 4 ---\n"+
			" - Object 1: only\n",
		text(t, r))
	assert.Equal(t, [][]string{
		{"1", "0", "VendorName", "Acme"},
		{"1", "4", "", "raw"},
		{"2", "-", "error", "illegal function"},
		{"3", "-", "(no objects)", ""},
		{"4", "1", "", "only"},
	}, r.TableRows())
	assert.True(t, r.HasPartialFailure())
}

func TestReportServerIDResult_multiUnit(t *testing.T) {
	r := &types.ReportServerIDResult{Target: "tcp://h:502", Units: []types.ReportServerIDUnitResult{
		{UnitID: 1, DataHex: "42ff", RunIndicatorOn: ptr(true)},
		{UnitID: 2, Error: errInfo("timeout"), DataHex: "ignored"},
		{UnitID: 3, RunIndicatorOn: ptr(false)},
		{UnitID: 4},
	}}
	types.FillReportServerIDSummary(r, 4)
	require.Equal(t, &types.ResultSummary{Requested: 4, Succeeded: 3, Failed: 1}, r.Summary)

	assert.Equal(t,
		"🔍 Sending FC17 Report Server ID to tcp://h:502...\n"+
			"Units requested: 4  succeeded: 3  failed: 1\n\n"+
			"\n--- Unit ID 1 ---\n"+
			"✅ Report Server ID (unit 1):\n  Data: 42ff\n  Run Indicator: ON\n"+
			"\n--- Unit ID 2 ---\n"+
			"⚠️ Unit 2: timeout\n"+
			"\n--- Unit ID 3 ---\n"+
			"✅ Report Server ID (unit 3):\n  Run Indicator: OFF\n"+
			"\n--- Unit ID 4 ---\n"+
			"✅ Report Server ID (unit 4):\n",
		text(t, r))
	assert.Equal(t, [][]string{
		{"1", "42ff", "ON", ""},
		{"2", "ignored", "", "timeout"},
		{"3", "", "OFF", ""},
		{"4", "", "", ""},
	}, r.TableRows())
	assert.True(t, r.HasPartialFailure())
}

func TestFillSummary_noOps(t *testing.T) {
	assert.Nil(t, types.NewResultSummary(0, 0, 0))
	assert.Equal(t, &types.ResultSummary{Failed: 1}, types.NewResultSummary(0, 0, 1))

	assert.NotPanics(t, func() {
		types.FillIdentifySummary(nil, 3)
		types.FillFingerprintSummary(nil, 3)
		types.FillReportServerIDSummary(nil, 3)
	})

	id := &types.IdentifyResult{Units: []types.IdentifyUnitResult{{Error: errInfo("x")}}}
	types.FillIdentifySummary(id, 0)
	assert.Nil(t, id.Summary)
	fp := &types.FingerprintResult{Units: []types.FingerprintUnitResult{{Error: errInfo("x")}}}
	types.FillFingerprintSummary(fp, 0)
	assert.Nil(t, fp.Summary)
	rs := &types.ReportServerIDResult{Units: []types.ReportServerIDUnitResult{{Error: errInfo("x")}}}
	types.FillReportServerIDSummary(rs, 0)
	assert.Nil(t, rs.Summary)
}

func TestSuccessExitForPayload(t *testing.T) {
	failed := &types.ResultSummary{Requested: 2, Succeeded: 1, Failed: 1}
	clean := &types.ResultSummary{Requested: 2, Succeeded: 2}
	tests := []struct {
		name string
		v    any
		want int
	}{
		{"nil", nil, errs.ExitOK},
		{"not a partial result", &types.ReadResult{}, errs.ExitOK},
		{"identify nil pointer", (*types.IdentifyResult)(nil), errs.ExitOK},
		{"identify clean", &types.IdentifyResult{Summary: clean}, errs.ExitOK},
		{"identify failed", &types.IdentifyResult{Summary: failed}, errs.ExitPartial},
		{"fingerprint nil pointer", (*types.FingerprintResult)(nil), errs.ExitOK},
		{"fingerprint no summary", &types.FingerprintResult{}, errs.ExitOK},
		{"fingerprint failed", &types.FingerprintResult{Summary: failed}, errs.ExitPartial},
		{"fingerprint partial unit without summary", &types.FingerprintResult{Units: []types.FingerprintUnitResult{{ProbeInterrupted: true, SupportedReads: []string{"FC03"}}}}, errs.ExitPartial},
		{"report server id nil pointer", (*types.ReportServerIDResult)(nil), errs.ExitOK},
		{"report server id failed", &types.ReportServerIDResult{Summary: failed}, errs.ExitPartial},
		{"scan nil pointer", (*types.ScanSummaryResult)(nil), errs.ExitOK},
		{"scan clean", &types.ScanSummaryResult{Summary: clean}, errs.ExitOK},
		{"scan failed", &types.ScanSummaryResult{Summary: failed}, errs.ExitPartial},
		{"sunspec probe nil pointer", (*types.SunSpecProbeOutput)(nil), errs.ExitOK},
		{"sunspec probe clean", &types.SunSpecProbeOutput{}, errs.ExitOK},
		{"sunspec probe fc03 error", &types.SunSpecProbeOutput{Modbus: types.SunSpecProbeModbus{FC03Error: errInfo("x")}}, errs.ExitPartial},
		{"sunspec probe fc04 error", &types.SunSpecProbeOutput{Modbus: types.SunSpecProbeModbus{FC04Error: errInfo("x")}}, errs.ExitPartial},
		{"sunspec probe fc43 error", &types.SunSpecProbeOutput{Modbus: types.SunSpecProbeModbus{FC43Error: errInfo("x")}}, errs.ExitPartial},
		{"sunspec probe detection error", &types.SunSpecProbeOutput{SunSpecDetail: types.SunSpecProbeSummary{DetectionError: errInfo("x")}}, errs.ExitPartial},
		{"sunspec probe model headers error", &types.SunSpecProbeOutput{SunSpecDetail: types.SunSpecProbeSummary{ModelHeadersError: errInfo("x")}}, errs.ExitPartial},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, types.SuccessExitForPayload(tt.v))
		})
	}
}

func TestErrorInfoHelpers(t *testing.T) {
	assert.Equal(t, "", types.ErrorMessage(nil))
	e := types.EmbeddedModbusError("boom")
	assert.Equal(t, &types.ErrorInfo{Kind: string(errs.KindModbus), Code: errs.CodeModbusException, Message: "boom"}, e)
	assert.Equal(t, "boom", types.ErrorMessage(e))
}
