// SPDX-License-Identifier: MIT

package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_everySupportedFunction(t *testing.T) {
	reqs, err := Parse([]byte(`[
		{"start_address": 100, "register_count": 2},
		{"function": 1, "start_address": 0, "register_count": 2000},
		{"function": 2, "start_address": 8, "register_count": 4},
		{"function": 4, "start_address": 65535, "register_count": 1},
		{"function": 7},
		{"function": 8, "sub_function": 0, "data": "A5 37"},
		{"function": 8, "sub_function": 11},
		{"function": 11},
		{"function": 12},
		{"function": 17},
		{"function": 20, "file_records": [{"file": 1, "record": 2, "length": 3}, {"file": 4, "record": 9999, "length": 1}]},
		{"function": 24, "fifo_address": 1234},
		{"function": 43},
		{"function": 43, "device_id_code": 4, "object_id": 5}
	]`), 3)
	require.NoError(t, err)
	want := [][]byte{
		{3, 0, 100, 0, 2},
		{1, 0, 0, 0x07, 0xD0},
		{2, 0, 8, 0, 4},
		{4, 0xFF, 0xFF, 0, 1},
		{7},
		{8, 0, 0, 0xA5, 0x37},
		{8, 0, 0x0B, 0, 0},
		{11},
		{12},
		{17},
		{20, 14, 6, 0, 1, 0, 2, 0, 3, 6, 0, 4, 0x27, 0x0F, 0, 1},
		{24, 0x04, 0xD2},
		{43, 0x0E, 1, 0},
		{43, 0x0E, 4, 5},
	}
	require.Len(t, reqs, len(want))
	for i, r := range reqs {
		assert.Equalf(t, want[i], r.PDU(), "entry %d (%s)", i, r.String())
	}
}

func TestParse_refused(t *testing.T) {
	for _, tc := range []struct{ plan, want string }{
		{`[{"function": 65}]`, "function code 65 is not a Modbus read function modbusctl records"},
		{`[{"function": 100}]`, "function code 100 is not a Modbus read function"},
		{`[{"function": 9}]`, "function code 9 is not"},
		{`[{"function": 5, "start_address": 1, "register_count": 1}]`, "function code 5 is not"},
		{`[{"function": 16, "start_address": 1, "register_count": 1}]`, "function code 16 is not"},
		{`[{"function": 21}]`, "function code 21 is not"},
		{`[{"function": 23}]`, "function code 23 is not"},
		{`[{"function": 8, "sub_function": 1}]`, "diagnostics sub-function 0x0001 is not recorded"},
		{`[{"function": 8, "sub_function": 4}]`, "diagnostics sub-function 0x0004 is not recorded"},
		{`[{"function": 8, "sub_function": 10}]`, "diagnostics sub-function 0x000A is not recorded"},
		{`[{"function": 8, "sub_function": 20}]`, "diagnostics sub-function 0x0014 is not recorded"},
		{`[{"function": 8, "sub_function": 267}]`, "diagnostics sub-function 0x010B is not recorded"},
		{`[{"function": 8, "sub_function": 11, "data": "01"}]`, "takes 2 data bytes, got 1"},
		{`[{"function": 8, "data": "zz"}]`, "is not hex"},
		{`[{"function": 8, "data": "` + strings.Repeat("00", 251) + `"}]`, "at most 250 fit"},
		{`[{"function": 20}]`, "needs 1-35 file_records, got 0"},
		{`[{"function": 20, "file_records": [{"file": 0, "record": 0, "length": 1}]}]`, "invalid file record"},
		{`[{"function": 20, "file_records": [{"file": 1, "record": 10000, "length": 1}]}]`, "invalid file record"},
		{`[{"function": 20, "file_records": [{"file": 1, "record": 0, "length": 125}]}]`, "ask for 252 response bytes, at most 245 fit"},
		{`[{"function": 43, "device_id_code": 5}]`, "device_id_code 5"},
		{`[{"function": 3, "start_address": 1, "register_count": 126}]`, "invalid address block: start=1, count=126"},
		{`[{"function": 1, "start_address": 1, "register_count": 2001}]`, "invalid address block: start=1, count=2001"},
		{`[{"start_address": 65535, "register_count": 2}]`, "invalid address block: start=65535, count=2"},
		{`[{"start_address": 1, "register_count": 2}, {"function": 65}]`, "blocks file entry 1: function code 65"},
		{`[]`, "contains no address blocks or requests"},
		{`{}`, "does not contain valid plan JSON"},
	} {
		_, err := Parse([]byte(tc.plan), 3)
		require.Errorf(t, err, "%s", tc.plan)
		assert.Containsf(t, err.Error(), tc.want, "%s", tc.plan)
	}
	_, err := Parse([]byte(`[{"start_address": 1, "register_count": 1}]`), 0)
	require.ErrorContains(t, err, "function code 0 is not", "a block needs a default function")
}

// TestParse_fileRecordLimits checks the FC20 limits of the Modbus Application
// Protocol Specification V1.1b3 section 6.14: request Byte Count and response
// Resp. data Length 0x07 to 0xF5 (245). Every sub-response is 2 + 2*length
// bytes, so a response has an even length: 244 is the largest one accepted
// and 246 the smallest one refused.
func TestParse_fileRecordLimits(t *testing.T) {
	records := func(lengths ...int) string {
		parts := make([]string, len(lengths))
		for i, n := range lengths {
			parts[i] = fmt.Sprintf(`{"file": 1, "record": %d, "length": %d}`, i, n)
		}
		return `[{"function": 20, "file_records": [` + strings.Join(parts, ", ") + `]}]`
	}
	ones := func(n int) []int {
		l := make([]int, n)
		for i := range l {
			l[i] = 1
		}
		return l
	}
	for _, tc := range []struct {
		name, plan, want string // want "" means accepted
	}{
		{"one record of 121 registers, 244 bytes", records(121), ""},
		{"one record of 122 registers, 246 bytes", records(122), "blocks file entry 0: file records ask for 246 response bytes, at most 245 fit"},
		{"two records, 244 bytes", records(60, 60), ""},
		{"two records, 246 bytes", records(60, 61), "blocks file entry 0: file records ask for 246 response bytes, at most 245 fit"},
		{"35 records, request byte count 245", records(ones(35)...), ""},
		{"36 records, request byte count 252", records(ones(36)...), "blocks file entry 0: read file record needs 1-35 file_records, got 36"},
		{"35 records, 244 bytes", records(append(ones(34), 53)...), ""},
		{"35 records, 246 bytes", records(append(ones(34), 54)...), "blocks file entry 0: file records ask for 246 response bytes, at most 245 fit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reqs, err := Parse([]byte(tc.plan), 3)
			if tc.want == "" {
				require.NoError(t, err)
				require.Len(t, reqs, 1)
				return
			}
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestRead(t *testing.T) {
	_, err := Read(" ", 3)
	require.ErrorContains(t, err, "blocks file path is required")
	_, err = Read(filepath.Join(t.TempDir(), "missing.json"), 3)
	require.ErrorContains(t, err, "failed to read blocks file")
	path := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(path, []byte(`[{"function": 17}]`), 0o600))
	reqs, err := Read(path, 3)
	require.NoError(t, err)
	require.Equal(t, []byte{17}, reqs[0].PDU())
}
