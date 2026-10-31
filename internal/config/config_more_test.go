// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModbusURLSchemes(t *testing.T) {
	want := []string{"tcp", "tcp+tls", "udp", "rtuovertcp", "rtuoverudp", "asciiovertcp", "rtu", "ascii"}
	got := ModbusURLSchemes()
	assert.Equal(t, want, got)

	// The result is a copy: changing it must not affect later calls or validation.
	got[0] = "bogus"
	assert.Equal(t, want, ModbusURLSchemes())
	assert.NoError(t, ValidateModbusURL("tcp://10.0.0.1:502"))
}

func TestModbusURL_more(t *testing.T) {
	assert.Equal(t, "rtu:///dev/ttyUSB0", ModbusURL("  rtu:///dev/ttyUSB0 ", "10.0.0.1", 502))
	assert.Equal(t, "", ModbusURL(" ", "  ", 502))
	assert.Equal(t, "tcp://10.0.0.1:502", ModbusURL("", " 10.0.0.1 ", 0))
	assert.Equal(t, "tcp://10.0.0.1:1502", ModbusURL("", "10.0.0.1", 1502))
	assert.Equal(t, "tcp://[fe80::1]:502", ModbusURL("", "fe80::1", 0))

	assert.Equal(t, "tcp://10.0.0.2:503", SunSpecModbusURL(&SunSpecBaseConfig{IP: "10.0.0.2", Port: 503}))
	assert.Equal(t, "udp://h:1", SunSpecModbusURL(&SunSpecBaseConfig{URL: "udp://h:1", IP: "10.0.0.2"}))
}

func TestSerialURLs(t *testing.T) {
	tests := []struct {
		url      string
		serial   bool
		device   string
		endpoint string
		port     uint16
	}{
		{"rtu:///dev/ttyUSB0", true, "/dev/ttyUSB0", "/dev/ttyUSB0", 0},
		{"  ascii://COM3 ", true, "COM3", "COM3", 0},
		{"rtu://", true, "", "", 0},
		{"tcp://10.0.0.1:1502", false, "", "10.0.0.1", 1502},
		{"udp://[::1]", false, "", "::1", 502},
		{"rtuovertcp://gw.local:4001", false, "", "gw.local", 4001},
		{"asciiovertcp://gw.local:4002", false, "", "gw.local", 4002},
		{"rtuoverudp://10.0.0.9:4003", false, "", "10.0.0.9", 4003},
		{"tcp+tls://secure.local:802", false, "", "secure.local", 802},
		{"RTU:///dev/ttyUSB0", false, "", "", 0},
		{"/dev/ttyUSB0", false, "", "", 0},
		{"http://10.0.0.1:80", false, "", "", 0},
		{"", false, "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.serial, IsSerialURL(tt.url))
			assert.Equal(t, tt.device, SerialDevice(tt.url))
			endpoint, port := CaptureEndpoint(tt.url)
			assert.Equal(t, tt.endpoint, endpoint)
			assert.Equal(t, tt.port, port)
		})
	}
}

func TestParseModbusURLHostPort_more(t *testing.T) {
	tests := []struct {
		url  string
		host string
		port uint16
	}{
		{"tcp://10.0.0.1:65535", "10.0.0.1", 65535},
		{"tcp://10.0.0.1:65536", "", 0},
		{"tcp://:502", "", 0},
		{"tcp://", "", 0},
		{"   ", "", 0},
		{"tcp://bad host:502", "", 0},
		{"rtu:///dev/ttyUSB0", "", 0},
		{"rtu://COM3", "", 0},
		{"ftp://10.0.0.1:21", "", 0},
	}
	for _, tt := range tests {
		host, port := ParseModbusURLHostPort(tt.url)
		assert.Equal(t, tt.host, host, tt.url)
		assert.Equal(t, tt.port, port, tt.url)
	}
}

func TestValidateModbusURL(t *testing.T) {
	const schemes = "tcp, tcp+tls, udp, rtuovertcp, rtuoverudp, asciiovertcp, rtu, ascii"
	tests := []struct {
		url  string
		want string
	}{
		{"tcp://10.0.0.1:502", ""},
		{"tcp://10.0.0.1", ""},
		{" tcp+tls://h:802 ", ""},
		{"udp://[::1]:502", ""},
		{"rtuovertcp://h:1", ""},
		{"rtuoverudp://h:65535", ""},
		{"asciiovertcp://h:1", ""},
		{"rtu:///dev/ttyUSB0", ""},
		{"ascii://COM3", ""},
		{"", `invalid Modbus URL "": use one of ` + schemes + ` followed by ://`},
		{"10.0.0.1:502", `invalid Modbus URL "10.0.0.1:502": use one of ` + schemes + ` followed by ://`},
		{"modbus://10.0.0.1:502", `invalid Modbus URL "modbus://10.0.0.1:502": use one of ` + schemes + ` followed by ://`},
		{"rtu://", `invalid Modbus URL "rtu://": rtu:// needs a serial device (e.g. rtu:///dev/ttyUSB0 or rtu://COM3)`},
		{"ascii://", `invalid Modbus URL "ascii://": ascii:// needs a serial device (e.g. ascii:///dev/ttyUSB0 or ascii://COM3)`},
		{"udp://", `invalid Modbus URL "udp://": udp:// needs a host and port (e.g. udp://192.168.1.10:502)`},
		{"tcp://:502", `invalid Modbus URL "tcp://:502": tcp:// needs a host and port (e.g. tcp://192.168.1.10:502)`},
		{"tcp://h:0", `invalid Modbus URL "tcp://h:0": port must be between 1 and 65535`},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := ValidateModbusURL(tt.url)
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestValidateModbusAddress_more(t *testing.T) {
	assert.NoError(t, ValidateModbusAddress("tcp://h:502", ""))
	assert.NoError(t, ValidateModbusAddress("rtu://COM1", " "))
	assert.NoError(t, ValidateModbusAddress("", "10.0.0.1"))
	assert.NoError(t, ValidateModbusAddress("", "not-validated-here"))
	assert.EqualError(t, ValidateModbusAddress("tcp://h:502", "10.0.0.1"),
		"endpoint conflict: use either --url (host:port in the URL) or --ip with --port, not both")
	assert.EqualError(t, ValidateModbusAddress(" ", ""),
		"modbus endpoint required: set --url (e.g. tcp://192.168.1.10:502 or rtu:///dev/ttyUSB0) or --ip with --port")
	assert.ErrorContains(t, ValidateModbusAddress("bogus", ""), `invalid Modbus URL "bogus"`)
}

func TestNormalizeParity(t *testing.T) {
	for in, want := range map[string]string{
		"": "none", "  ": "none", "n": "none", "N": "none", "none": "none", "NONE": "none",
		"e": "even", "E": "even", " Even ": "even",
		"o": "odd", "O": "odd", "ODD": "odd",
	} {
		got, err := NormalizeParity(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, "%q", in)
	}
	for _, in := range []string{"mark", "space", "0", "ev", "nope"} {
		got, err := NormalizeParity(in)
		assert.Equal(t, "", got)
		assert.EqualError(t, err, `invalid parity "`+in+`": use none, even, or odd`)
	}
}

func TestValidateSerialConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  SerialConfig
		want string
	}{
		{"zero values select defaults", SerialConfig{}, ""},
		{"8E1", SerialConfig{Baud: 19200, DataBits: 8, Parity: "even", StopBits: 1}, ""},
		{"7O2", SerialConfig{Baud: 300, DataBits: 7, Parity: "O", StopBits: 2}, ""},
		{"5 data bits", SerialConfig{DataBits: 5}, ""},
		{"6 data bits", SerialConfig{DataBits: 6}, ""},
		{"any baud rate", SerialConfig{Baud: 1}, ""},
		{"bad parity", SerialConfig{Parity: "mark"}, `invalid parity "mark": use none, even, or odd`},
		{"4 data bits", SerialConfig{DataBits: 4}, "invalid data bits 4: use 5, 6, 7 or 8 (0 = default)"},
		{"9 data bits", SerialConfig{DataBits: 9}, "invalid data bits 9: use 5, 6, 7 or 8 (0 = default)"},
		{"3 stop bits", SerialConfig{StopBits: 3}, "invalid stop bits 3: use 1 or 2 (0 = default)"},
		{"parity is reported first", SerialConfig{Parity: "x", DataBits: 1, StopBits: 9}, `invalid parity "x": use none, even, or odd`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSerialConfig(tt.cfg)
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestParseSunSpecBases_more(t *testing.T) {
	got, err := ParseSunSpecBases(" 40000, 0 ,,40000,65535 ")
	require.NoError(t, err)
	assert.Equal(t, []uint16{40000, 0, 65535}, got)

	got, err = ParseSunSpecBases("")
	require.NoError(t, err)
	assert.Nil(t, got)

	for _, in := range []string{"65536", "-1", "0x10", "1;2"} {
		_, err = ParseSunSpecBases("0," + in)
		assert.ErrorContains(t, err, `invalid base address "`+in+`"`)
	}
}

func TestParseDiagnosticSubFunction_all(t *testing.T) {
	want := map[string]uint16{
		"returnquerydata": 0x00, "restartcommunications": 0x01, "returndiagnosticregister": 0x02,
		"changeasciiinputdelimiter": 0x03, "forcelistenonlymode": 0x04, "clearcountersanddiagnosticreg": 0x0A,
		"returnbusmessagecount": 0x0B, "returnbuscommunicationerrorcount": 0x0C, "returnbusexceptionerrorcount": 0x0D,
		"returnservermessagecount": 0x0E, "returnservernoresponsecount": 0x0F, "returnservernakcount": 0x10,
		"returnserverbusycount": 0x11, "returnbuscharacteroverruncount": 0x12, "clearoverruncounterandflag": 0x14,
	}
	names := DiagnosticSubFunctions()
	require.Len(t, names, len(want))
	for _, name := range names {
		code, err := ParseDiagnosticSubFunction(name)
		require.NoError(t, err)
		assert.Equal(t, want[name], code, name)
	}
	code, err := ParseDiagnosticSubFunction("  ReturnBusMessageCount ")
	require.NoError(t, err)
	assert.Equal(t, uint16(0x0B), code)
	code, err = ParseDiagnosticSubFunction(" ")
	require.NoError(t, err)
	assert.Equal(t, uint16(0), code)

	_, err = ParseDiagnosticSubFunction("nope")
	assert.ErrorContains(t, err, `unknown diagnostic sub-function "nope" (allowed: [returnquerydata `)

	// The list is a copy.
	names[0] = "changed"
	assert.Equal(t, "returnquerydata", DiagnosticSubFunctions()[0])
}

func TestScanAlgorithmForExecution_more(t *testing.T) {
	assert.Equal(t, ScanAlgoSafe, ScanAlgorithmForExecution(nil))
	assert.Equal(t, ScanAlgoSafe, ScanAlgorithmForExecution(&ScanConfig{}))
	assert.Equal(t, ScanAlgoSafe, ScanAlgorithmForExecution(&ScanConfig{Algo: "  "}))
	assert.Equal(t, ScanAlgoBoundary, ScanAlgorithmForExecution(&ScanConfig{Algo: " Boundary "}))
	assert.Equal(t, ScanAlgoSunspec, ScanAlgorithmForExecution(&ScanConfig{Algo: "linear", NormalizedAlgo: ScanAlgoSunspec}))
}

func TestEnumValueLists(t *testing.T) {
	assert.Equal(t, []string{"5", "6", "15", "16"}, WriteFunctionCodes())
	assert.Equal(t, []string{"none", "even", "odd"}, Parities())
	assert.Equal(t, []string{"1", "2", "3", "4"}, FunctionCodes())
	assert.Equal(t, []string{"holding", "input"}, SunspecRegtypes())
	assert.Equal(t, []string{"safe", "smart", "deep", "stepped", "linear", "boundary", "sunspec"}, ScanAlgorithms())

	// Every canonical parity must normalize to itself.
	for _, p := range Parities() {
		got, err := NormalizeParity(p)
		require.NoError(t, err)
		assert.Equal(t, p, got)
	}
	for _, a := range ScanAlgorithms() {
		assert.True(t, ValidScanAlgo(a), a)
	}
}

func complete(t *testing.T, cmd *cobra.Command, flag, prefix string) []string {
	t.Helper()
	fn, ok := cmd.GetFlagCompletionFunc(flag)
	require.True(t, ok, "no completion registered for --%s", flag)
	out, directive := fn(cmd, nil, prefix)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	return out
}

func TestRegisterWriteFunctionCompletion(t *testing.T) {
	cmd := &cobra.Command{Use: "write"}
	cmd.Flags().Uint8("function", 0, "")
	RegisterWriteFunctionCompletion(cmd)
	assert.Equal(t, []string{"5", "6", "15", "16"}, complete(t, cmd, "function", ""))
	assert.Equal(t, []string{"15", "16"}, complete(t, cmd, "function", "1"))
	assert.Empty(t, complete(t, cmd, "function", "3"))
}

func TestRegisterParityCompletion(t *testing.T) {
	cmd := &cobra.Command{Use: "read"}
	cmd.Flags().String("parity", "", "")
	RegisterParityCompletion(cmd)
	assert.Equal(t, []string{"none", "even", "odd"}, complete(t, cmd, "parity", ""))
	assert.Equal(t, []string{"even"}, complete(t, cmd, "parity", "E"))
	assert.Empty(t, complete(t, cmd, "parity", "x"))

	// A command without --parity is left alone.
	bare := &cobra.Command{Use: "convert"}
	assert.NotPanics(t, func() { RegisterParityCompletion(bare) })
	_, ok := bare.GetFlagCompletionFunc("parity")
	assert.False(t, ok)
}

func TestRegisterCodecCompletion(t *testing.T) {
	ids := CodecIDs()
	require.NotEmpty(t, ids)
	assert.IsIncreasing(t, ids)
	assert.Contains(t, ids, "ip_addr")

	cmd := &cobra.Command{Use: "decode"}
	cmd.Flags().String("codec", "", "")
	RegisterCodecCompletion(cmd)
	assert.Equal(t, ids, complete(t, cmd, "codec", ""))
	got := complete(t, cmd, "codec", "IP_")
	assert.Contains(t, got, "ip_addr")
	for _, id := range got {
		assert.Regexp(t, "^ip_", id)
	}
}

func TestRegisterOtherCompletions(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("regtype", "", "")
	RegisterRegtypeCompletion(cmd)
	assert.Equal(t, []string{"input"}, complete(t, cmd, "regtype", "in"))
}
