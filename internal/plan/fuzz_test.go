// SPDX-License-Identifier: MIT

package plan

import "testing"

// FuzzParse feeds arbitrary blocks files. A plan that parses must have only
// requests that pass Check: the rest of the program relies on that.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		`[]`, `{}`, `null`, `[{}]`,
		`[{"start_address":0,"register_count":10}]`,
		`[{"function":1,"start_address":65535,"register_count":1}]`,
		`[{"function":3,"start_address":65530,"register_count":125}]`,
		`[{"function":8,"sub_function":0,"data":"ff00"}]`,
		`[{"function":8,"sub_function":11}]`,
		`[{"function":20,"file_records":[{"file":1,"record":0,"length":2}]}]`,
		`[{"function":20,"file_records":[{"file":0,"record":10000,"length":0}]}]`,
		`[{"function":43}]`,
	} {
		f.Add([]byte(s), uint8(3))
	}
	f.Fuzz(func(t *testing.T, data []byte, defaultFC uint8) {
		reqs, err := Parse(data, defaultFC)
		if err != nil {
			return
		}
		if len(reqs) == 0 {
			t.Fatal("a plan without requests parsed")
		}
		for i := range reqs {
			if err := reqs[i].Check(); err != nil {
				t.Fatalf("request %d passed Parse and fails Check: %v", i, err)
			}
		}
	})
}
