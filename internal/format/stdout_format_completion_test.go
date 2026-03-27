package format

import "testing"

func TestStdoutFormatDescriptionsMatchValues(t *testing.T) {
	legal := make(map[string]struct{}, len(stdoutFormatDescriptions))
	for k := range stdoutFormatDescriptions {
		legal[k] = struct{}{}
	}
	for _, v := range Values() {
		if _, ok := legal[v]; !ok {
			t.Errorf("Values() contains %q missing from stdoutFormatDescriptions", v)
		}
		delete(legal, v)
	}
	for k := range legal {
		t.Errorf("stdoutFormatDescriptions contains extra key %q not in Values()", k)
	}
}
