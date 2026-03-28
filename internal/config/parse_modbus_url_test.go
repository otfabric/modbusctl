package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseModbusURLHostPort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw      string
		wantHost string
		wantPort uint16
	}{
		{"tcp://192.168.1.10:502", "192.168.1.10", 502},
		{"tcp://10.0.0.1:1502", "10.0.0.1", 1502},
		{"tcp+tls://gateway.example:443", "gateway.example", 443},
		{"tcp://[::1]:502", "::1", 502},
		{"tcp://[2001:db8::1]:1502", "2001:db8::1", 1502},
		{"rtuovertcp://192.168.0.5", "192.168.0.5", 502},
		{"tcp://host", "host", 502},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			host, port := ParseModbusURLHostPort(tt.raw)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
		})
	}
}

func TestParseModbusURLHostPort_invalid(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"",
		"http://192.168.1.1:502",
		"tcp://:badport",
		"not-a-url",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			host, port := ParseModbusURLHostPort(raw)
			assert.Empty(t, host)
			assert.Zero(t, port)
		})
	}
}

func TestModbusURL_IPv6(t *testing.T) {
	t.Parallel()
	u := ModbusURL("", "::1", 502)
	require.Equal(t, "tcp://[::1]:502", u)
	host, port := ParseModbusURLHostPort(u)
	require.Equal(t, "::1", host)
	require.Equal(t, uint16(502), port)
}

func TestValidateModbusAddress_URLParsed(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateModbusAddress("tcp://192.168.1.10:502", ""))
	require.Error(t, ValidateModbusAddress("http://192.168.1.10:502", ""))
	require.Error(t, ValidateModbusAddress("tcp://192.168.1.10:0", ""))
	require.Error(t, ValidateModbusAddress("tcp:///no-host:502", ""))
}
