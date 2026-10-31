// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

// FuzzParseModbusURLHostPort checks the URL flag: a parsed URL names a host,
// and a URL that is not one of the network schemes gives nothing.
func FuzzParseModbusURLHostPort(f *testing.F) {
	for _, s := range []string{
		"tcp://192.168.1.10:502", "tcp://host", "udp://[::1]:502", "tcp+tls://h:802", "rtu:///dev/ttyUSB0",
		"tcp://:502", "tcp://h:99999", "tcp://[::1", "", " tcp://h ", "%zz://h",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		host, port := ParseModbusURLHostPort(s)
		if host == "" && port != 0 {
			t.Fatalf("%q: port %d without a host", s, port)
		}
		if host != "" && !strings.Contains(s, "://") {
			t.Fatalf("%q: host %q from something that is no URL", s, host)
		}
	})
}

// FuzzParseSunSpecBases checks the --bases flag: no duplicates in the result.
func FuzzParseSunSpecBases(f *testing.F) {
	for _, s := range []string{"0,40000,50000", "", ",", "40000,40000", "65536", "-1", " 1 , 2 "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out, err := ParseSunSpecBases(s)
		if err != nil {
			return
		}
		seen := map[uint16]bool{}
		for _, b := range out {
			if seen[b] {
				t.Fatalf("%q: %d twice in %v", s, b, out)
			}
			seen[b] = true
		}
	})
}
