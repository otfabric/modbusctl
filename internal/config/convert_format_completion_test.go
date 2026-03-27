package config

import "testing"

func TestConvertFormatDescriptionsMatchConvertFormats(t *testing.T) {
	seen := make(map[string]struct{})
	for _, v := range ConvertFormats() {
		seen[v] = struct{}{}
		if _, ok := ConvertFormatDescriptions[v]; !ok {
			t.Errorf("ConvertFormats() contains %q missing from ConvertFormatDescriptions", v)
		}
	}
	for k := range ConvertFormatDescriptions {
		if _, ok := seen[k]; !ok {
			t.Errorf("ConvertFormatDescriptions contains extra key %q not in ConvertFormats()", k)
		}
	}
}
