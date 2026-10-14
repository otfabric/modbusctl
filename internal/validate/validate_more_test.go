package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// check asserts err against want: "" means no error, otherwise the error must contain want.
func check(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		assert.NoError(t, err)
		return
	}
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), want)
	}
}

func tcpDevice() config.DeviceConfig {
	return config.DeviceConfig{URL: "tcp://192.168.1.10:502", Unit: 1}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// Every client command validates its endpoint through validateModbusEndpoint.
func TestValidateModbusEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		ip     string
		port   uint16
		serial config.SerialConfig
		want   string
	}{
		{name: "tcp", url: "tcp://192.168.1.10:502"},
		{name: "tcp hostname without port", url: "tcp://plc.local"},
		{name: "tcp ipv6", url: "tcp://[::1]:1502"},
		{name: "tcp+tls", url: "tcp+tls://plc.local:802"},
		{name: "udp", url: "udp://10.0.0.1:502"},
		{name: "rtuovertcp", url: "rtuovertcp://10.0.0.1:4001"},
		{name: "rtuoverudp", url: "rtuoverudp://10.0.0.1:4001"},
		{name: "asciiovertcp", url: "asciiovertcp://10.0.0.1:4001"},
		{name: "rtu unix device", url: "rtu:///dev/ttyUSB0"},
		{name: "rtu windows device", url: "rtu://COM3"},
		{name: "ascii", url: "ascii:///dev/ttyS1"},
		{name: "url surrounded by spaces", url: "  tcp://10.0.0.1:502  "},
		{name: "ip and port", ip: "10.0.0.1", port: 502},
		{name: "ipv6 and port", ip: " fe80::1 ", port: 502},

		{name: "nothing set", want: "modbus endpoint required"},
		{name: "blank url and ip", url: "  ", ip: " ", want: "modbus endpoint required"},
		{name: "url and ip", url: "tcp://10.0.0.1:502", ip: "10.0.0.1", port: 502, want: "endpoint conflict"},
		{name: "unknown scheme", url: "http://10.0.0.1:502", want: `invalid Modbus URL "http://10.0.0.1:502": use one of tcp, tcp+tls, udp, rtuovertcp, rtuoverudp, asciiovertcp, rtu, ascii followed by ://`},
		{name: "scheme is case sensitive", url: "TCP://10.0.0.1:502", want: "use one of"},
		{name: "no scheme", url: "10.0.0.1:502", want: "use one of"},
		{name: "network url without host", url: "tcp://", want: "tcp:// needs a host and port (e.g. tcp://192.168.1.10:502)"},
		{name: "network url with only a port", url: "udp://:502", want: "udp:// needs a host and port"},
		{name: "port zero", url: "tcp://10.0.0.1:0", want: "port must be between 1 and 65535"},
		{name: "serial url without device", url: "rtu://", want: "rtu:// needs a serial device (e.g. rtu:///dev/ttyUSB0 or rtu://COM3)"},
		{name: "ascii url with blank device", url: "ascii://  ", want: "ascii:// needs a serial device"},
		{name: "hostname as ip", ip: "plc.local", port: 502, want: "invalid IP address: plc.local"},
		{name: "ip without port", ip: "10.0.0.1", want: "invalid port: 0"},

		{name: "serial settings", url: "rtu:///dev/ttyUSB0", serial: config.SerialConfig{Baud: 9600, DataBits: 8, Parity: "E", StopBits: 1}},
		{name: "data bits 5", url: "rtu://COM1", serial: config.SerialConfig{DataBits: 5}},
		{name: "stop bits 2 odd parity", url: "ascii://COM1", serial: config.SerialConfig{DataBits: 7, Parity: " Odd ", StopBits: 2}},
		{name: "bad parity", url: "rtu://COM1", serial: config.SerialConfig{Parity: "mark"}, want: `invalid parity "mark": use none, even, or odd`},
		{name: "data bits too low", url: "rtu://COM1", serial: config.SerialConfig{DataBits: 4}, want: "invalid data bits 4: use 5, 6, 7 or 8 (0 = default)"},
		{name: "data bits too high", url: "rtu://COM1", serial: config.SerialConfig{DataBits: 9}, want: "invalid data bits 9"},
		{name: "stop bits too high", url: "rtu://COM1", serial: config.SerialConfig{StopBits: 3}, want: "invalid stop bits 3: use 1 or 2 (0 = default)"},
		{name: "serial settings are checked for network urls too", url: "tcp://10.0.0.1:502", serial: config.SerialConfig{Parity: "x"}, want: `invalid parity "x"`},
		{name: "endpoint is checked before serial settings", serial: config.SerialConfig{Parity: "x"}, want: "modbus endpoint required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check(t, validateModbusEndpoint(tt.url, tt.ip, tt.port, tt.serial), tt.want)
		})
	}
}

func TestValidateDeviceConfig(t *testing.T) {
	check(t, validateDeviceConfig(tcpDevice()), "")
	check(t, validateDeviceConfig(config.DeviceConfig{URL: "rtu:///dev/ttyUSB0", Unit: 255, SerialConfig: config.SerialConfig{Baud: 115200, Parity: "even"}}), "")
	check(t, validateDeviceConfig(config.DeviceConfig{URL: "tcp://h:502", Unit: 0}), "invalid Modbus unit ID: 0 (must be between 1 and 255)")
	check(t, validateDeviceConfig(config.DeviceConfig{Unit: 1}), "modbus endpoint required")
	check(t, validateDeviceConfig(config.DeviceConfig{URL: "rtu://COM1", Unit: 1, SerialConfig: config.SerialConfig{StopBits: 9}}), "invalid stop bits 9")
}

func TestValidateOptionalTimeoutMilliseconds(t *testing.T) {
	assert.NoError(t, validateOptionalTimeoutMilliseconds(0))
	assert.NoError(t, validateOptionalTimeoutMilliseconds(200))
	assert.NoError(t, validateOptionalTimeoutMilliseconds(65535))
	assert.EqualError(t, validateOptionalTimeoutMilliseconds(199), "timeout must be 0 for default (10000 ms) or at least 200 ms")
	assert.Error(t, validateOptionalTimeoutMilliseconds(1))
}

func TestValidateUnitClientConfig(t *testing.T) {
	base := config.UnitClientConfig{URL: "tcp://10.0.0.1:502", UnitID: "1"}
	tests := []struct {
		name string
		mod  func(*config.UnitClientConfig)
		want string
	}{
		{"single unit ignores parallel", func(*config.UnitClientConfig) {}, ""},
		{"range and list", func(c *config.UnitClientConfig) { c.UnitID = "1-10,255" }, ""},
		{"all with parallel", func(c *config.UnitClientConfig) { c.UnitID = " ALL "; c.Parallel = 64 }, ""},
		{"all with parallel 1", func(c *config.UnitClientConfig) { c.UnitID = "all"; c.Parallel = 1 }, ""},
		{"all without parallel", func(c *config.UnitClientConfig) { c.UnitID = "all" }, "parallel must be between 1 and 64 when using --unit all, got 0"},
		{"all with too many workers", func(c *config.UnitClientConfig) { c.UnitID = "all"; c.Parallel = 65 }, "parallel must be between 1 and 64 when using --unit all, got 65"},
		{"empty unit", func(c *config.UnitClientConfig) { c.UnitID = "" }, `unit ID must be 1-255 or "all"`},
		{"unit zero", func(c *config.UnitClientConfig) { c.UnitID = "0" }, "0"},
		{"unit out of range", func(c *config.UnitClientConfig) { c.UnitID = "256" }, "256"},
		{"no endpoint", func(c *config.UnitClientConfig) { c.URL = "" }, "modbus endpoint required"},
		{"serial url", func(c *config.UnitClientConfig) { c.URL = "ascii://COM7"; c.Parity = "odd" }, ""},
		{"bad serial settings", func(c *config.UnitClientConfig) { c.URL = "ascii://COM7"; c.DataBits = 3 }, "invalid data bits 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, validateUnitClientConfig(cfg), tt.want)
			// Identify and Report Server ID share the rule and add the timeout check.
			check(t, CheckIdentifyConfig(config.IdentifyConfig{UnitClientConfig: cfg}), tt.want)
			check(t, CheckReportServerIDConfig(config.ReportServerIDConfig{UnitClientConfig: cfg}), tt.want)
		})
	}

	slow := base
	slow.Timeout = 150
	check(t, CheckReportServerIDConfig(config.ReportServerIDConfig{UnitClientConfig: slow}), "timeout must be 0")
	check(t, CheckIdentifyConfig(config.IdentifyConfig{UnitClientConfig: slow}), "timeout must be 0")
}

func TestCheckFingerprintConfig_more(t *testing.T) {
	check(t, CheckFingerprintConfig(config.FingerprintConfig{URL: "rtu:///dev/ttyUSB0", UnitID: "1,2", SerialConfig: config.SerialConfig{Parity: "n"}}), "")
	check(t, CheckFingerprintConfig(config.FingerprintConfig{URL: "tcp://h:502", UnitID: "all"}), "")
	check(t, CheckFingerprintConfig(config.FingerprintConfig{UnitID: "1"}), "modbus endpoint required")
	check(t, CheckFingerprintConfig(config.FingerprintConfig{URL: "rtu://", UnitID: "1"}), "needs a serial device")
}

func TestCheckReadConfig_more(t *testing.T) {
	base := config.ReadConfig{DeviceConfig: tcpDevice(), Function: 3, StartAddress: 100, RegisterCount: 10}
	tests := []struct {
		name string
		mod  func(*config.ReadConfig)
		want string
	}{
		{"valid", func(*config.ReadConfig) {}, ""},
		{"coils", func(c *config.ReadConfig) { c.Function = 1 }, ""},
		{"last register", func(c *config.ReadConfig) { c.StartAddress = 65535; c.RegisterCount = 1 }, ""},
		{"window ends exactly at the top", func(c *config.ReadConfig) { c.StartAddress = 65411; c.RegisterCount = 125 }, ""},
		{"window past the top", func(c *config.ReadConfig) { c.StartAddress = 65412; c.RegisterCount = 125 }, "start address 65412 with count 125 exceeds maximum register space"},
		{"unit zero", func(c *config.ReadConfig) { c.Unit = 0 }, "invalid Modbus unit ID: 0"},
		{"short timeout", func(c *config.ReadConfig) { c.Timeout = 10 }, "timeout must be 0"},
		{"write function", func(c *config.ReadConfig) { c.Function = 6 }, "unsupported function code: 6 — supported: 1 (Read Coils), 2 (Read Discrete Inputs), 3 (Read Holding Registers), 4 (Read Input Registers)"},
		{"zero count", func(c *config.ReadConfig) { c.RegisterCount = 0 }, "register count must be between 1 and 125"},
		{"count too large", func(c *config.ReadConfig) { c.RegisterCount = 126 }, "register count must be between 1 and 125"},
		{"output in missing directory", func(c *config.ReadConfig) { c.OutputFile = filepath.Join(t.TempDir(), "nope", "out.mcap") }, "output directory not accessible"},
		{"output in existing directory", func(c *config.ReadConfig) { c.OutputFile = filepath.Join(t.TempDir(), "out.mcap") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckReadConfig(cfg), tt.want)
		})
	}
}

func TestCheckDecodeConfig(t *testing.T) {
	base := config.DecodeConfig{DeviceConfig: tcpDevice(), Function: 3, StartAddress: 100, Codec: "float32/layout:4321"}
	tests := []struct {
		name string
		mod  func(*config.DecodeConfig)
		want string
	}{
		{"one value by default", func(*config.DecodeConfig) {}, ""},
		{"input registers", func(c *config.DecodeConfig) { c.Function = 4 }, ""},
		{"several values", func(c *config.DecodeConfig) { c.RegisterCount = 124 }, ""},
		{"codec id with spaces", func(c *config.DecodeConfig) { c.Codec = "  float32/layout:4321 " }, ""},
		{"serial endpoint", func(c *config.DecodeConfig) { c.URL = "rtu:///dev/ttyUSB0"; c.Baud = 9600 }, ""},
		{"no endpoint", func(c *config.DecodeConfig) { c.URL = "" }, "modbus endpoint required"},
		{"short timeout", func(c *config.DecodeConfig) { c.Timeout = 199 }, "timeout must be 0"},
		{"coil function", func(c *config.DecodeConfig) { c.Function = 1 }, "unsupported function code for decode: 1 — supported: 3 (Read Holding Registers), 4 (Read Input Registers)"},
		{"function zero", func(c *config.DecodeConfig) { c.Function = 0 }, "unsupported function code for decode: 0"},
		{"no codec", func(c *config.DecodeConfig) { c.Codec = "  " }, "codec is required (use --codec; run 'modbusctl client codecs' to list available codecs)"},
		{"unknown codec", func(c *config.DecodeConfig) { c.Codec = "nope" }, `unknown codec "nope" (run 'modbusctl client codecs' to list available codecs)`},
		{"count not a multiple of the width", func(c *config.DecodeConfig) { c.RegisterCount = 3 }, `register count 3 must be a multiple of the codec register width 2 for "float32/layout:4321"`},
		{"count too large", func(c *config.DecodeConfig) { c.RegisterCount = 126 }, `register count must be between 1 and 125 (codec "float32/layout:4321" has register width 2)`},
		{"default width at the last address", func(c *config.DecodeConfig) { c.StartAddress = 65535 }, "start address 65535 with count 2 exceeds maximum register space"},
		{"default width at the last fitting address", func(c *config.DecodeConfig) { c.StartAddress = 65534 }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckDecodeConfig(cfg), tt.want)
		})
	}
}

func TestCheckWriteConfig(t *testing.T) {
	base := config.WriteConfig{DeviceConfig: tcpDevice(), StartAddress: 100, Values: []string{"1234"}}
	many := func(n int, v string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = v
		}
		return out
	}
	tests := []struct {
		name string
		mod  func(*config.WriteConfig)
		want string
	}{
		{"one register, function resolved", func(*config.WriteConfig) {}, ""},
		{"several registers, function resolved", func(c *config.WriteConfig) { c.Values = []string{"1", "0xFFFF", "-32768"} }, ""},
		{"fc6", func(c *config.WriteConfig) { c.Function = 6 }, ""},
		{"fc16 with one value", func(c *config.WriteConfig) { c.Function = 16 }, ""},
		{"fc16 at the limit", func(c *config.WriteConfig) { c.Function = 16; c.Values = many(123, "1") }, ""},
		{"fc5", func(c *config.WriteConfig) { c.Function = 5; c.Values = []string{"ON"} }, ""},
		{"fc15", func(c *config.WriteConfig) {
			c.Function = 15
			c.Values = []string{"1", "0", "true", "false", "on", "off"}
		}, ""},
		{"fc15 at the limit", func(c *config.WriteConfig) { c.Function = 15; c.StartAddress = 0; c.Values = many(1968, "1") }, ""},
		{"codec", func(c *config.WriteConfig) { c.Codec = "float32/layout:4321"; c.Values = []string{"1.5", "-2"} }, ""},
		{"blank values are ignored", func(c *config.WriteConfig) { c.Values = []string{" ", "7", ""} }, ""},
		{"serial endpoint", func(c *config.WriteConfig) { c.URL = "ascii://COM3"; c.DataBits = 7; c.Parity = "even" }, ""},
		{"last address", func(c *config.WriteConfig) { c.StartAddress = 65535 }, ""},

		{"no endpoint", func(c *config.WriteConfig) { c.URL = "" }, "modbus endpoint required"},
		{"unit zero", func(c *config.WriteConfig) { c.Unit = 0 }, "invalid Modbus unit ID: 0"},
		{"bad parity", func(c *config.WriteConfig) { c.Parity = "space" }, `invalid parity "space"`},
		{"short timeout", func(c *config.WriteConfig) { c.Timeout = 50 }, "timeout must be 0"},
		{"no values", func(c *config.WriteConfig) { c.Values = nil }, "at least one value is required (use --values)"},
		{"only blank values", func(c *config.WriteConfig) { c.Values = []string{"", "  "} }, "at least one value is required"},
		{"read function", func(c *config.WriteConfig) { c.Function = 3 }, "unsupported function code for write: 3 — supported: 5 (Write Single Coil), 6 (Write Single Register), 15 (Write Multiple Coils), 16 (Write Multiple Registers)"},
		{"register value not a number", func(c *config.WriteConfig) { c.Values = []string{"1.5"} }, `invalid register value "1.5"`},
		{"register value too large", func(c *config.WriteConfig) { c.Values = []string{"65536"} }, "register value 65536 is out of range (-32768 to 65535)"},
		{"register value too small", func(c *config.WriteConfig) { c.Values = []string{"-32769"} }, "register value -32769 is out of range"},
		{"fc6 with two values", func(c *config.WriteConfig) { c.Function = 6; c.Values = []string{"1", "2"} }, "function code 6 writes exactly one register, got 2 (use --function 16 for several)"},
		{"fc6 with a two-register codec", func(c *config.WriteConfig) {
			c.Function = 6
			c.Codec = "float32/layout:4321"
			c.Values = []string{"1.5"}
		}, "function code 6 writes exactly one register, got 2"},
		{"fc16 over the limit", func(c *config.WriteConfig) { c.Function = 16; c.Values = many(124, "1") }, "function code 16 writes at most 123 registers per request, got 124"},
		{"resolved fc16 over the limit", func(c *config.WriteConfig) { c.Values = many(124, "1") }, "function code 16 writes at most 123 registers per request, got 124"},
		{"fc5 with two values", func(c *config.WriteConfig) { c.Function = 5; c.Values = []string{"1", "0"} }, "function code 5 writes exactly one coil, got 2 values (use --function 15 for several)"},
		{"fc5 with a register value", func(c *config.WriteConfig) { c.Function = 5; c.Values = []string{"2"} }, `invalid coil value "2": use 1/0, true/false or on/off`},
		{"fc15 over the limit", func(c *config.WriteConfig) { c.Function = 15; c.StartAddress = 0; c.Values = many(1969, "0") }, "function code 15 writes at most 1968 coils per request, got 1969"},
		{"fc15 with codec", func(c *config.WriteConfig) {
			c.Function = 15
			c.Codec = "float32/layout:4321"
			c.Values = []string{"1"}
		}, "--codec encodes registers and cannot be used with coil function code 15"},
		{"unknown codec", func(c *config.WriteConfig) { c.Codec = "nope" }, `unknown codec "nope"`},
		{"value invalid for codec", func(c *config.WriteConfig) { c.Codec = "float32/layout:4321"; c.Values = []string{"abc"} }, `value "abc" is not valid for codec "float32/layout:4321"`},
		{"past the last address", func(c *config.WriteConfig) { c.StartAddress = 65535; c.Values = []string{"1", "2"} }, "start address 65535 with 2 value(s) exceeds the maximum address 65535"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckWriteConfig(cfg), tt.want)
		})
	}
}

func TestCheckPollConfig(t *testing.T) {
	base := config.PollConfig{DeviceConfig: tcpDevice(), Function: 3, StartAddress: 100, RegisterCount: 10, Interval: 1000}
	tests := []struct {
		name string
		mod  func(*config.PollConfig)
		want string
	}{
		{"registers", func(*config.PollConfig) {}, ""},
		{"register limit", func(c *config.PollConfig) { c.Function = 4; c.RegisterCount = 125 }, ""},
		{"coil limit", func(c *config.PollConfig) { c.Function = 1; c.RegisterCount = 2000 }, ""},
		{"discrete inputs", func(c *config.PollConfig) { c.Function = 2; c.RegisterCount = 126 }, ""},
		{"codec with default count", func(c *config.PollConfig) { c.Codec = "float32/layout:4321"; c.RegisterCount = 0 }, ""},
		{"codec with several values", func(c *config.PollConfig) { c.Codec = "float32/layout:4321"; c.RegisterCount = 124 }, ""},
		{"fastest interval", func(c *config.PollConfig) { c.Interval = 1 }, ""},
		{"serial endpoint", func(c *config.PollConfig) { c.URL = "rtu://COM4"; c.StopBits = 1 }, ""},
		{"window ends at the top", func(c *config.PollConfig) { c.StartAddress = 65526 }, ""},

		{"no endpoint", func(c *config.PollConfig) { c.URL = "" }, "modbus endpoint required"},
		{"unit zero", func(c *config.PollConfig) { c.Unit = 0 }, "invalid Modbus unit ID: 0"},
		{"short timeout", func(c *config.PollConfig) { c.Timeout = 199 }, "timeout must be 0"},
		{"write function", func(c *config.PollConfig) { c.Function = 16 }, "unsupported function code: 16"},
		{"codec on coils", func(c *config.PollConfig) { c.Function = 1; c.Codec = "float32/layout:4321" }, "--codec decodes registers and cannot be used with function code 1 (use 3 or 4)"},
		{"codec on discrete inputs", func(c *config.PollConfig) { c.Function = 2; c.Codec = "x" }, "cannot be used with function code 2"},
		{"unknown codec", func(c *config.PollConfig) { c.Codec = "nope" }, `unknown codec "nope"`},
		{"count not a multiple of the codec width", func(c *config.PollConfig) { c.Codec = "float32/layout:4321"; c.RegisterCount = 5 }, `register count 5 must be a multiple of the codec register width 2 for "float32/layout:4321"`},
		{"codec count too large", func(c *config.PollConfig) { c.Codec = "float32/layout:4321"; c.RegisterCount = 126 }, "count must be between 1 and 125 for function code 3"},
		{"zero count", func(c *config.PollConfig) { c.RegisterCount = 0 }, "count must be between 1 and 125 for function code 3"},
		{"too many registers", func(c *config.PollConfig) { c.RegisterCount = 126 }, "count must be between 1 and 125 for function code 3"},
		{"too many coils", func(c *config.PollConfig) { c.Function = 1; c.RegisterCount = 2001 }, "count must be between 1 and 2000 for function code 1"},
		{"zero coils", func(c *config.PollConfig) { c.Function = 2; c.RegisterCount = 0 }, "count must be between 1 and 2000 for function code 2"},
		{"zero interval", func(c *config.PollConfig) { c.Interval = 0 }, "interval must be at least 1 millisecond"},
		{"window past the top", func(c *config.PollConfig) { c.StartAddress = 65527 }, "start address 65527 with count 10 exceeds maximum register space"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckPollConfig(cfg), tt.want)
		})
	}
}

func TestCheckScanConfig_more(t *testing.T) {
	base := config.ScanConfig{DeviceConfig: tcpDevice(), Function: 3, StartAddress: 0, EndAddress: 1000}
	tests := []struct {
		name     string
		mod      func(*config.ScanConfig)
		want     string
		wantAlgo config.ScanAlgorithm
	}{
		{name: "default algo is safe", mod: func(*config.ScanConfig) {}, wantAlgo: config.ScanAlgoSafe},
		{name: "algo is normalized", mod: func(c *config.ScanConfig) { c.Algo = "  LINEAR " }, wantAlgo: config.ScanAlgoLinear},
		{name: "stepped", mod: func(c *config.ScanConfig) { c.Algo = "stepped"; c.Step = 1 }, wantAlgo: config.ScanAlgoStepped},
		{name: "boundary", mod: func(c *config.ScanConfig) { c.Algo = "boundary"; c.SeedStart = 876; c.SeedCount = 125 }, wantAlgo: config.ScanAlgoBoundary},
		{name: "sunspec ignores the range", mod: func(c *config.ScanConfig) {
			c.Algo = "sunspec"
			c.StartAddress = 9
			c.EndAddress = 1
			c.SunSpecBases = "0, 40000"
		}, wantAlgo: config.ScanAlgoSunspec},
		{name: "sunspec base at the top", mod: func(c *config.ScanConfig) { c.Algo = "sunspec"; c.SunSpecBase = 65533 }, wantAlgo: config.ScanAlgoSunspec},
		{name: "max delay", mod: func(c *config.ScanConfig) { c.Delay = 60000 }, wantAlgo: config.ScanAlgoSafe},

		{name: "no endpoint", mod: func(c *config.ScanConfig) { c.URL = "" }, want: "modbus endpoint required"},
		{name: "short timeout", mod: func(c *config.ScanConfig) { c.Timeout = 1 }, want: "timeout must be 0"},
		{name: "bad function", mod: func(c *config.ScanConfig) { c.Function = 5 }, want: "unsupported function code: 5"},
		{name: "delay too long", mod: func(c *config.ScanConfig) { c.Delay = 60001 }, want: "delay must be between 0 and 60000 milliseconds (1 minute)"},
		{name: "unknown algo", mod: func(c *config.ScanConfig) { c.Algo = "fast" }, want: `got "fast"`},
		{name: "start equals end", mod: func(c *config.ScanConfig) { c.StartAddress = 1000 }, want: "start address (1000) must be less than end address (1000)", wantAlgo: config.ScanAlgoSafe},
		{name: "start after end", mod: func(c *config.ScanConfig) { c.StartAddress = 2000 }, want: "start address (2000) must be less than end address (1000)", wantAlgo: config.ScanAlgoSafe},
		{name: "stepped without step", mod: func(c *config.ScanConfig) { c.Algo = "stepped" }, want: "step must be between 1 and 65535 when using algo stepped, got 0", wantAlgo: config.ScanAlgoStepped},
		{name: "boundary without seed count", mod: func(c *config.ScanConfig) { c.Algo = "boundary" }, want: "boundary algo requires --seed-count between 1 and 125, got 0", wantAlgo: config.ScanAlgoBoundary},
		{name: "boundary seed count too large", mod: func(c *config.ScanConfig) { c.Algo = "boundary"; c.SeedCount = 126 }, want: "boundary algo requires --seed-count between 1 and 125, got 126", wantAlgo: config.ScanAlgoBoundary},
		{name: "boundary seed past the address space", mod: func(c *config.ScanConfig) {
			c.Algo = "boundary"
			c.EndAddress = 65535
			c.SeedStart = 65535
			c.SeedCount = 2
		}, want: "exceeds max address 65535", wantAlgo: config.ScanAlgoBoundary},
		{name: "boundary seed before start", mod: func(c *config.ScanConfig) {
			c.Algo = "boundary"
			c.StartAddress = 100
			c.SeedStart = 99
			c.SeedCount = 1
		}, want: "boundary seed start 99 must be >= start address 100", wantAlgo: config.ScanAlgoBoundary},
		{name: "boundary seed past end", mod: func(c *config.ScanConfig) { c.Algo = "boundary"; c.SeedStart = 877; c.SeedCount = 125 }, want: "boundary seed end 1001 exceeds end address 1000", wantAlgo: config.ScanAlgoBoundary},
		{name: "sunspec base overflow", mod: func(c *config.ScanConfig) { c.Algo = "sunspec"; c.SunSpecBase = 65534 }, want: "sunspec base 65534 + 2 would overflow", wantAlgo: config.ScanAlgoSunspec},
		{name: "sunspec bad bases", mod: func(c *config.ScanConfig) { c.Algo = "sunspec"; c.SunSpecBases = "0,x" }, want: `sunspec bases: invalid base address "x"`, wantAlgo: config.ScanAlgoSunspec},
		{name: "sunspec output in missing directory", mod: func(c *config.ScanConfig) {
			c.Algo = "sunspec"
			c.OutputFile = filepath.Join(t.TempDir(), "nope", "o.mcap")
		}, want: "output directory not accessible", wantAlgo: config.ScanAlgoSunspec},
		{name: "output in missing directory", mod: func(c *config.ScanConfig) { c.OutputFile = filepath.Join(t.TempDir(), "nope", "o.mcap") }, want: "output directory not accessible", wantAlgo: config.ScanAlgoSafe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckScanConfig(&cfg), tt.want)
			assert.Equal(t, tt.wantAlgo, cfg.NormalizedAlgo)
		})
	}
}

func TestCheckRecordConfig_more(t *testing.T) {
	blocks := writeFile(t, "blocks.json", `[{"start_address":0,"register_count":10},{"start_address":65535,"register_count":1}]`)
	base := config.RecordConfig{DeviceConfig: tcpDevice(), Function: 3, Interval: 1000, Duration: 1000, BlocksFile: blocks}
	tests := []struct {
		name string
		mod  func(*config.RecordConfig)
		want string
	}{
		{"valid", func(*config.RecordConfig) {}, ""},
		{"serial endpoint", func(c *config.RecordConfig) { c.URL = "rtu:///dev/ttyUSB0" }, ""},
		{"no endpoint", func(c *config.RecordConfig) { c.URL = "" }, "modbus endpoint required"},
		{"short timeout", func(c *config.RecordConfig) { c.Timeout = 199 }, "timeout must be 0"},
		{"bad function", func(c *config.RecordConfig) { c.Function = 0 }, "unsupported function code: 0"},
		{"duration shorter than interval", func(c *config.RecordConfig) { c.Duration = 999 }, "duration (999) must be greater than or equal to interval (1000)"},
		{"no blocks file", func(c *config.RecordConfig) { c.BlocksFile = " " }, "blocks file path is required (--blocks-file / MODBUSCTL_BLOCKS_FILE)"},
		{"output in missing directory", func(c *config.RecordConfig) { c.OutputFile = filepath.Join(t.TempDir(), "nope", "o.mcap") }, "output directory not accessible"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckRecordConfig(cfg), tt.want)
		})
	}
}

func TestValidateAddressBlockFile(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"valid", `[{"start_address":100,"register_count":125}]`, ""},
		{"block ending at the top", `[{"start_address":65535,"register_count":1}]`, ""},
		{"not json", `start,count`, "blocks file does not contain valid address-block JSON"},
		{"wrong shape", `{"start_address":1}`, "blocks file does not contain valid address-block JSON"},
		{"address out of range", `[{"start_address":65536,"register_count":1}]`, "blocks file does not contain valid address-block JSON"},
		{"empty list", `[]`, "blocks file contains no address blocks"},
		{"zero count", `[{"start_address":1,"register_count":0}]`, "invalid address block: start=1, count=0"},
		{"block past the top", `[{"start_address":1,"register_count":2},{"start_address":65535,"register_count":2}]`, "invalid address block: start=65535, count=2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check(t, validateAddressBlockFile(writeFile(t, "blocks.json", tt.content)), tt.want)
		})
	}
	check(t, validateAddressBlockFile(""), "blocks file path is required")
	check(t, validateAddressBlockFile(filepath.Join(dir, "missing.json")), "blocks file not accessible")
	check(t, validateAddressBlockFile(dir), "blocks file cannot be a directory")
}

func TestValidateDeviceProfileFile(t *testing.T) {
	profile := func(registers string) string { return `{"protocolData":{"registers":[` + registers + `]}}` }
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"valid", profile(`{"controlledPropertyId":"p","start":100,"size":2,"format":">f"}`), ""},
		{"register ending at the top", profile(`{"controlledPropertyId":"p","start":65535,"size":1,"format":">H"}`), ""},
		{"unused registers are not checked", profile(`{"controlledPropertyId":"not.used","start":65535,"size":0,"format":"?"},{"controlledPropertyId":"p","start":1,"size":1,"format":">h"}`), ""},
		{"not json", `registers:`, "invalid device profile JSON"},
		{"no registers", `{"protocolData":{}}`, "device profile contains no register definitions"},
		{"empty property id", profile(`{"start":1,"size":1,"format":">h"}`), "register has empty controlledPropertyId"},
		{"zero size", profile(`{"controlledPropertyId":"p","start":1,"size":0,"format":">h"}`), "register p has invalid size: must be > 0"},
		{"past the top", profile(`{"controlledPropertyId":"p","start":65535,"size":2,"format":">i"}`), "register p has invalid address range: start=65535, size=2"},
		{"unknown format", profile(`{"controlledPropertyId":"p","start":1,"size":1,"format":"u16"}`), "register p uses unsupported format: u16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check(t, validateDeviceProfileFile(writeFile(t, "profile.json", tt.content)), tt.want)
		})
	}
	dir := t.TempDir()
	check(t, validateDeviceProfileFile(" "), "device profile file path is required")
	check(t, validateDeviceProfileFile(filepath.Join(dir, "missing.json")), "device profile file not accessible")
	check(t, validateDeviceProfileFile(dir), "device profile file cannot be a directory")
}

func TestCheckDeviceProfileDecodeConfig_more(t *testing.T) {
	input := writeFile(t, "in.mcap", "x")
	profile := writeFile(t, "p.json", `{"protocolData":{"registers":[{"controlledPropertyId":"p","start":1,"size":1,"format":">h"}]}}`)
	check(t, CheckDeviceProfileDecodeConfig(config.DeviceProfileDecodeConfig{InputFile: input, DeviceProfile: profile}), "")
	check(t, CheckDeviceProfileDecodeConfig(config.DeviceProfileDecodeConfig{DeviceProfile: profile}), "file path is required")
	check(t, CheckDeviceProfileDecodeConfig(config.DeviceProfileDecodeConfig{InputFile: input, DeviceProfile: profile, OutputFile: filepath.Join(t.TempDir(), "nope", "o.txt")}), "output directory not accessible")
	check(t, CheckDeviceProfileDecodeConfig(config.DeviceProfileDecodeConfig{InputFile: input}), "device profile file path is required")
}

func TestValidateFile(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, "in.mcap", "x")

	t.Run("must exist", func(t *testing.T) {
		check(t, validateFile(file, true), "")
		check(t, validateFile("  "+file+" ", true), "")
		check(t, validateFile("", true), "file path is required")
		check(t, validateFile(filepath.Join(dir, "missing"), true), "file not accessible")
		check(t, validateFile(dir, true), "path cannot be a directory: "+dir)
	})
	t.Run("output", func(t *testing.T) {
		check(t, validateFile("", false), "")
		check(t, validateFile("   ", false), "")
		check(t, validateFile(filepath.Join(dir, "new.mcap"), false), "")
		check(t, validateFile(file, false), "")
		check(t, validateFile(dir, false), "path cannot be a directory: "+dir)
		check(t, validateFile(dir+"/", false), "")
		check(t, validateFile(dir+`\/`, false), "")
		check(t, validateFile(filepath.Join(dir, "missing")+"/", false), "output directory not accessible")
		check(t, validateFile(filepath.Join(dir, "missing", "out.mcap"), false), "output directory not accessible")
		check(t, validateFile("./", false), "")
	})
}

// The input-file commands share one rule: the input must exist, the output is optional.
func TestCheckInputFileConfigs(t *testing.T) {
	dir := t.TempDir()
	input := writeFile(t, "in.mcap", "x")
	badOut := filepath.Join(dir, "nope", "out")

	checks := map[string]func(in, out string) error{
		"extract": func(in, out string) error {
			return CheckExtractConfig(config.ExtractConfig{InputFile: in, OutputFile: out})
		},
		"strings": func(in, out string) error {
			return CheckStringsConfig(config.StringsConfig{InputFile: in, OutputFile: out})
		},
		"info": func(in, out string) error { return CheckInfoConfig(config.InfoConfig{InputFile: in, OutputFile: out}) },
		"convert": func(in, out string) error {
			return CheckConvertConfig(&config.ConvertConfig{InputFile: in, OutputFile: out, FormatType: "csv"})
		},
	}
	for name, fn := range checks {
		t.Run(name, func(t *testing.T) {
			check(t, fn(input, ""), "")
			check(t, fn(input, "  "), "")
			check(t, fn(input, filepath.Join(dir, "out")), "")
			check(t, fn(input, dir+"/"), "")
			check(t, fn("", ""), "file path is required")
			check(t, fn(filepath.Join(dir, "missing"), ""), "file not accessible")
			check(t, fn(dir, ""), "path cannot be a directory")
			check(t, fn(input, badOut), "output directory not accessible")
		})
	}
}

func TestCheckConvertConfig_format(t *testing.T) {
	input := writeFile(t, "in.mcap", "x")
	check(t, CheckConvertConfig(nil), "convert config is nil")

	for in, want := range map[string]string{"csv": "csv", " JSONL ": "jsonl", "Mbcap": "mbcap", "wide-csv": "wide-csv"} {
		cfg := &config.ConvertConfig{InputFile: input, FormatType: in}
		require.NoError(t, CheckConvertConfig(cfg))
		assert.Equal(t, want, cfg.FormatType)
	}
	for _, in := range []string{"", "xml", "csv,json"} {
		cfg := &config.ConvertConfig{InputFile: input, FormatType: in}
		assert.EqualError(t, CheckConvertConfig(cfg), `unsupported format: "`+in+`" (expected mbcap, jsonl, csv or wide-csv)`)
		assert.Equal(t, in, cfg.FormatType, "a rejected format must be left untouched")
	}
	// The v2 json export is gone; say what replaces it.
	assert.ErrorContains(t, CheckConvertConfig(&config.ConvertConfig{InputFile: input, FormatType: "json"}), "use jsonl")
}

func TestCheckDiagnosticConfig_more(t *testing.T) {
	base := config.DiagnosticConfig{URL: "tcp://10.0.0.1:502", UnitID: 1}
	tests := []struct {
		name string
		mod  func(*config.DiagnosticConfig)
		want string
	}{
		{"defaults", func(*config.DiagnosticConfig) {}, ""},
		{"serial endpoint", func(c *config.DiagnosticConfig) { c.URL = "rtu://COM1"; c.Parity = "e" }, ""},
		{"spaced hex data", func(c *config.DiagnosticConfig) { c.Data = " A5 37 " }, ""},
		{"long echo payload on return query data", func(c *config.DiagnosticConfig) { c.Data = strings.Repeat("AB", 250) }, ""},
		{"no endpoint", func(c *config.DiagnosticConfig) { c.URL = "" }, "modbus endpoint required"},
		{"bad serial settings", func(c *config.DiagnosticConfig) { c.URL = "rtu://COM1"; c.DataBits = 12 }, "invalid data bits 12"},
		{"short timeout", func(c *config.DiagnosticConfig) { c.Timeout = 199 }, "timeout must be 0"},
		{"unknown sub-function", func(c *config.DiagnosticConfig) { c.SubFunction = "selfdestruct" }, `unknown diagnostic sub-function "selfdestruct"`},
		{"odd number of hex digits", func(c *config.DiagnosticConfig) { c.Data = "A53" }, "diagnostic --data must be an even number of hex digits"},
		{"not hex", func(c *config.DiagnosticConfig) { c.Data = "ZZZZ" }, "invalid hex data"},
		{"payload too long", func(c *config.DiagnosticConfig) { c.Data = strings.Repeat("AB", 251) }, "diagnostic data exceeds 250 bytes"},
		{"more than two bytes on another sub-function", func(c *config.DiagnosticConfig) { c.SubFunction = "clearcountersanddiagnosticreg"; c.Data = "000000" }, "for this diagnostic sub-function, data must be at most 2 bytes, got 3"},
		{"two bytes on another sub-function", func(c *config.DiagnosticConfig) { c.SubFunction = "clearcountersanddiagnosticreg"; c.Data = "0000" }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckDiagnosticConfig(cfg), tt.want)
		})
	}
}

func TestCheckDiscoverConfig_more(t *testing.T) {
	base := config.DiscoverConfig{Subnets: []string{"192.168.1.0/30"}, Port: 502, Parallel: 4}
	tests := []struct {
		name string
		mod  func(*config.DiscoverConfig)
		want string
	}{
		{"valid", func(*config.DiscoverConfig) {}, ""},
		{"resolve mac with interface", func(c *config.DiscoverConfig) { c.ResolveMAC = true; c.NetworkInterface = "en0" }, ""},
		{"no subnets", func(c *config.DiscoverConfig) { c.Subnets = nil }, "at least one subnet must be provided"},
		{"bad subnet", func(c *config.DiscoverConfig) { c.Subnets = []string{"192.168.1.0/30", "10.0.0.1"} }, "invalid subnet '10.0.0.1'"},
		{"port zero", func(c *config.DiscoverConfig) { c.Port = 0 }, "port must be between 1 and 65535"},
		{"no workers", func(c *config.DiscoverConfig) { c.Parallel = 0 }, "parallel must be between 1 and 64, got 0"},
		{"resolve mac without interface", func(c *config.DiscoverConfig) { c.ResolveMAC = true; c.NetworkInterface = " " }, "network interface must be provided if ResolveMAC is enabled"},
		{"too many hosts", func(c *config.DiscoverConfig) { c.Subnets = []string{"10.0.0.0/15"} }, "use --force-large-scan to override"},
		{"too many hosts, forced", func(c *config.DiscoverConfig) { c.Subnets = []string{"10.0.0.0/15"}; c.ForceLargeScan = true }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckDiscoverConfig(cfg), tt.want)
		})
	}
}

func TestCheckServerConfigs(t *testing.T) {
	dir := t.TempDir()
	input := writeFile(t, "in.mbcap", "x")
	check(t, CheckStaticServerConfig(config.StaticServerConfig{Port: 1502, InputFile: input}), "")
	check(t, CheckStaticServerConfig(config.StaticServerConfig{Port: 1502, InputFile: input, At: "90s", Missing: "zero"}), "")
	check(t, CheckStaticServerConfig(config.StaticServerConfig{Port: 0, InputFile: input}), "port must be greater than 0")
	check(t, CheckStaticServerConfig(config.StaticServerConfig{Port: 1502}), "file path is required")
	check(t, CheckStaticServerConfig(config.StaticServerConfig{Port: 1502, InputFile: filepath.Join(dir, "missing.mbcap")}), "file not accessible")
	check(t, CheckStaticServerConfig(config.StaticServerConfig{Port: 1502, InputFile: dir}), "path cannot be a directory")
	check(t, CheckStaticServerConfig(config.StaticServerConfig{Port: 1502, InputFile: input, Missing: "panic"}), `invalid --missing "panic"`)
	check(t, CheckStaticServerConfig(config.StaticServerConfig{Port: 1502, InputFile: input, At: "soon"}), `invalid --at "soon"`)

	check(t, CheckReplayServerConfig(config.ReplayServerConfig{Port: 1502, InputFile: input}), "")
	check(t, CheckReplayServerConfig(config.ReplayServerConfig{Port: 1502, InputFile: input, Speed: "0.5", From: "1m", BeforeFirst: "absent", Missing: "silence"}), "")
	check(t, CheckReplayServerConfig(config.ReplayServerConfig{Port: 0, InputFile: input}), "port must be greater than 0")
	check(t, CheckReplayServerConfig(config.ReplayServerConfig{Port: 1502, InputFile: input, Speed: "0"}), `invalid --speed "0"`)
	check(t, CheckReplayServerConfig(config.ReplayServerConfig{Port: 1502, InputFile: input, Speed: "fast"}), `invalid --speed "fast"`)
	check(t, CheckReplayServerConfig(config.ReplayServerConfig{Port: 1502, InputFile: input, From: "-1s"}), `invalid --from "-1s"`)
	check(t, CheckReplayServerConfig(config.ReplayServerConfig{Port: 1502, InputFile: input, BeforeFirst: "later"}), `invalid --before-first "later"`)
}

func TestCheckSunSpecConfigs_more(t *testing.T) {
	base := config.SunSpecBaseConfig{URL: "tcp://10.0.0.1:502"}
	tests := []struct {
		name string
		mod  func(*config.SunSpecBaseConfig)
		want string
	}{
		{"unit zero is allowed", func(*config.SunSpecBaseConfig) {}, ""},
		{"regtype is normalized", func(c *config.SunSpecBaseConfig) { c.Regtype = " Input " }, ""},
		{"holding", func(c *config.SunSpecBaseConfig) { c.Regtype = "holding" }, ""},
		{"serial endpoint", func(c *config.SunSpecBaseConfig) { c.URL = "rtu:///dev/ttyUSB0"; c.Baud = 9600 }, ""},
		{"no endpoint", func(c *config.SunSpecBaseConfig) { c.URL = "" }, "modbus endpoint required"},
		{"bad serial settings", func(c *config.SunSpecBaseConfig) { c.StopBits = 3 }, "invalid stop bits 3"},
		{"short timeout", func(c *config.SunSpecBaseConfig) { c.Timeout = 199 }, "timeout must be 0"},
		{"bad regtype", func(c *config.SunSpecBaseConfig) { c.Regtype = "coil" }, `regtype must be holding or input, got "coil"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mod(&cfg)
			check(t, CheckSunSpecDetectConfig(config.SunSpecDetectConfig{SunSpecBaseConfig: cfg}), tt.want)
			check(t, CheckSunSpecModelsConfig(config.SunSpecModelsConfig{SunSpecBaseConfig: cfg}), tt.want)
			check(t, CheckSunSpecMapConfig(config.SunSpecMapConfig{SunSpecBaseConfig: cfg}), tt.want)
			check(t, CheckSunSpecProbeConfig(config.SunSpecProbeConfig{SunSpecBaseConfig: cfg}), tt.want)
		})
	}

	check(t, CheckSunSpecDetectConfig(config.SunSpecDetectConfig{SunSpecBaseConfig: base, Bases: "0,40000,40000"}), "")
	check(t, CheckSunSpecDetectConfig(config.SunSpecDetectConfig{SunSpecBaseConfig: base, Bases: "0,70000"}), `invalid base address "70000"`)
	check(t, CheckSunSpecDetectConfig(config.SunSpecDetectConfig{SunSpecBaseConfig: base, Bases: " , "}), "at least one base address required when --bases is set")
	check(t, CheckSunSpecModelsConfig(config.SunSpecModelsConfig{SunSpecBaseConfig: base, MaxModels: -1}), "max-models must be >= 0")
	check(t, CheckSunSpecModelsConfig(config.SunSpecModelsConfig{SunSpecBaseConfig: base, MaxModels: 10}), "")
}

func TestValidateRanges(t *testing.T) {
	assert.NoError(t, validateExclusiveRegisterWindow(0, 0))
	assert.NoError(t, validateExclusiveRegisterWindow(0, 65535))
	assert.NoError(t, validateExclusiveRegisterWindow(65535, 1))
	assert.EqualError(t, validateExclusiveRegisterWindow(65535, 2), "start address 65535 with count 2 exceeds maximum register space")
	assert.EqualError(t, validateExclusiveRegisterWindow(2, 65535), "start address 2 with count 65535 exceeds maximum register space")

	assert.NoError(t, validateInclusiveScanRange(0, 65535))
	assert.NoError(t, validateInclusiveScanRange(65534, 65535))
	assert.EqualError(t, validateInclusiveScanRange(0, 0), "start address (0) must be less than end address (0)")
	assert.EqualError(t, validateInclusiveScanRange(65535, 65535), "start address (65535) must be less than end address (65535)")

	for fc := uint8(0); fc < 20; fc++ {
		assert.Equal(t, fc >= 1 && fc <= 4, validateFunctionCode(fc) == nil, "read fc %d", fc)
		assert.Equal(t, fc == 3 || fc == 4, validateDecodeFunctionCode(fc) == nil, "decode fc %d", fc)
	}

	assert.True(t, isValidHostIP(" 10.0.0.1 "))
	assert.True(t, isValidHostIP("::1"))
	assert.False(t, isValidHostIP("localhost"))
	assert.False(t, isValidHostIP("10.0.0.256"))
	assert.False(t, isValidHostIP(""))
}

func TestValidateJSONFiles_unreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions are not enforced for root")
	}
	path := writeFile(t, "locked.json", `[]`)
	require.NoError(t, os.Chmod(path, 0o000))
	check(t, validateAddressBlockFile(path), "failed to read blocks file")
	check(t, validateDeviceProfileFile(path), "failed to read device profile file")
}

// An output path whose parent exists but is a regular file must be rejected with a readable message.
func TestValidateFile_parentIsRegularFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	for _, path := range []string{filepath.Join(file, "out.mcap"), file + "/"} {
		err := validateFile(path, false)
		require.Error(t, err, path)
		assert.Contains(t, err.Error(), "output directory is not a directory: "+file)
		assert.NotContains(t, err.Error(), "%!w")
	}
}
