package config

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type EmbeddedKinds struct {
	Inner string `env:"MBT_INNER" flag:"inner" desc:"inner value"`
}

type allKinds struct {
	EmbeddedKinds
	S     string   `env:"MBT_S" flag:"s" desc:"a string"`
	B     bool     `env:"MBT_B" flag:"b" desc:"a bool"`
	U     uint     `env:"MBT_U" flag:"u" desc:"a uint"`
	U8    uint8    `env:"MBT_U8" flag:"u8" desc:"a uint8"`
	U16   uint16   `env:"MBT_U16" flag:"u16" desc:"a uint16"`
	U32   uint32   `env:"MBT_U32" flag:"u32" desc:"a uint32"`
	U64   uint64   `env:"MBT_U64" flag:"u64" desc:"a uint64"`
	I     int      `env:"MBT_I" flag:"i" desc:"an int"`
	I8    int8     `env:"MBT_I8" flag:"i8" desc:"an int8"`
	I16   int16    `env:"MBT_I16" flag:"i16" desc:"an int16"`
	I32   int32    `env:"MBT_I32" flag:"i32" desc:"an int32"`
	I64   int64    `env:"MBT_I64" flag:"i64" desc:"an int64"`
	L     []string `env:"MBT_L" flag:"l" desc:"a list"`
	NoEnv string   `flag:"no-env" desc:"flag only"`
	Plain string
	Ints  []int   `env:"MBT_INTS"`
	F     float64 `env:"MBT_F"`
	// Unexported on purpose: the loader must skip fields it cannot set.
	hidden string `env:"MBT_HIDDEN"`
}

func TestLoadFromEnv_everyKind(t *testing.T) {
	for k, v := range map[string]string{
		"MBT_INNER": "in", "MBT_S": "str", "MBT_B": "true",
		"MBT_U": "1", "MBT_U8": "255", "MBT_U16": "65535", "MBT_U32": "4294967295", "MBT_U64": "18446744073709551615",
		"MBT_I": "-1", "MBT_I8": "-128", "MBT_I16": "32767", "MBT_I32": "-2147483648", "MBT_I64": "9223372036854775807",
		"MBT_L": " a, b ,c", "MBT_INTS": "1,2", "MBT_F": "1.5", "MBT_HIDDEN": "x",
	} {
		t.Setenv(k, v)
	}
	c := allKinds{Plain: "kept", NoEnv: "kept"}
	require.NoError(t, LoadFromEnv(&c))
	assert.Equal(t, allKinds{
		EmbeddedKinds: EmbeddedKinds{Inner: "in"},
		S:             "str", B: true,
		U: 1, U8: 255, U16: 65535, U32: 4294967295, U64: 18446744073709551615,
		I: -1, I8: -128, I16: 32767, I32: -2147483648, I64: 9223372036854775807,
		L:     []string{"a", "b", "c"},
		NoEnv: "kept", Plain: "kept",
	}, c)
	assert.Empty(t, c.hidden, "unexported fields are skipped")
}

func TestLoadFromEnv_unsetAndEmptyKeepDefaults(t *testing.T) {
	t.Setenv("MBT_S", "")
	t.Setenv("MBT_U16", "")
	c := allKinds{S: "default", U16: 502, L: []string{"x"}}
	require.NoError(t, LoadFromEnv(&c))
	assert.Equal(t, allKinds{S: "default", U16: 502, L: []string{"x"}}, c)
}

func TestLoadFromEnv_errorsAreJoined(t *testing.T) {
	for k, v := range map[string]string{
		"MBT_U8": "256", "MBT_U16": "-1", "MBT_I8": "128", "MBT_I": "1.5", "MBT_B": "maybe",
		"MBT_INNER": "ok", "MBT_S": "still loaded",
	} {
		t.Setenv(k, v)
	}
	c := allKinds{U8: 7, I8: 7, B: true}
	err := LoadFromEnv(&c)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, `MBT_U8="256": value overflows uint8`)
	assert.Contains(t, msg, `MBT_U16="-1": strconv.ParseUint`)
	assert.Contains(t, msg, `MBT_I8="128": value overflows int8`)
	assert.Contains(t, msg, `MBT_I="1.5": strconv.ParseInt`)
	assert.Contains(t, msg, `MBT_B="maybe": strconv.ParseBool`)

	// Valid variables are still applied; invalid ones leave the field untouched.
	assert.Equal(t, "still loaded", c.S)
	assert.Equal(t, "ok", c.Inner)
	assert.Equal(t, uint8(7), c.U8)
	assert.Equal(t, int8(7), c.I8)
	assert.True(t, c.B)
}

func TestLoadFromEnv_embeddedError(t *testing.T) {
	type inner struct {
		N uint8 `env:"MBT_EMB_N"`
	}
	type outer struct {
		inner
		M uint8 `env:"MBT_EMB_M"`
	}
	t.Setenv("MBT_EMB_N", "x")
	t.Setenv("MBT_EMB_M", "y")
	err := LoadFromEnv(&outer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `MBT_EMB_N="x"`)
	assert.Contains(t, err.Error(), `MBT_EMB_M="y"`)
}

func TestLoadFromEnv_ignoresNonStructPointers(t *testing.T) {
	t.Setenv("MBT_S", "str")
	assert.NoError(t, LoadFromEnv(nil))
	assert.NoError(t, LoadFromEnv(allKinds{}))
	n := 3
	assert.NoError(t, LoadFromEnv(&n))
	var np *allKinds
	assert.NoError(t, LoadFromEnv(&np))
}

func TestMustLoadFromEnv(t *testing.T) {
	t.Setenv("MBT_U16", "1502")
	c := allKinds{}
	assert.NotPanics(t, func() { MustLoadFromEnv(&c) })
	assert.Equal(t, uint16(1502), c.U16)

	t.Setenv("MBT_U16", "70000")
	assert.PanicsWithValue(t, `modbusctl: invalid environment: MBT_U16="70000": value overflows uint16`,
		func() { MustLoadFromEnv(&allKinds{}) })
}

func TestLoadFromEnv_realConfigs(t *testing.T) {
	for k, v := range map[string]string{
		"MODBUSCTL_URL": "rtu:///dev/ttyUSB0", "MODBUSCTL_UNIT": "17", "MODBUSCTL_BAUD": "9600",
		"MODBUSCTL_DATA_BITS": "8", "MODBUSCTL_PARITY": "even", "MODBUSCTL_STOP_BITS": "1",
		"MODBUSCTL_TIMEOUT": "750", "MODBUSCTL_FUNCTION": "16", "MODBUSCTL_ADDRESS": "100",
		"MODBUSCTL_VALUES": "1, 0x10 ,-5", "MODBUSCTL_CODEC": "ip_addr", "MODBUSCTL_OUTPUT_FORMAT": "json",
		"MODBUSCTL_COUNT": "4", "MODBUSCTL_INTERVAL": "250", "MODBUSCTL_POLLS": "9",
		"MODBUSCTL_TLS_CERT": "client.pem", "MODBUSCTL_TLS_KEY": "client-key.pem", "MODBUSCTL_TLS_CA": "ca.pem",
	} {
		t.Setenv(k, v)
	}
	serial := SerialConfig{Baud: 9600, DataBits: 8, Parity: "even", StopBits: 1}
	tlsCfg := TLSConfig{TLSCert: "client.pem", TLSKey: "client-key.pem", TLSCA: "ca.pem"}
	device := DeviceConfig{SerialConfig: serial, TLSConfig: tlsCfg, URL: "rtu:///dev/ttyUSB0", Unit: 17}

	var w WriteConfig
	require.NoError(t, LoadFromEnv(&w))
	assert.Equal(t, WriteConfig{
		DeviceConfig: device, Timeout: 750, Function: 16, StartAddress: 100,
		Values: []string{"1", "0x10", "-5"}, Codec: "ip_addr", OutputFormat: "json",
	}, w)

	var p PollConfig
	require.NoError(t, LoadFromEnv(&p))
	assert.Equal(t, PollConfig{
		DeviceConfig: device, Timeout: 750, Function: 16, StartAddress: 100, RegisterCount: 4,
		Codec: "ip_addr", Interval: 250, Polls: 9, OutputFormat: "json",
	}, p)

	// The same variable feeds a string unit list on multi-unit commands.
	var id IdentifyConfig
	require.NoError(t, LoadFromEnv(&id))
	assert.Equal(t, "17", id.UnitID)
	assert.Equal(t, serial, id.SerialConfig)
	assert.Equal(t, tlsCfg, id.TLSConfig)

	// Every command that dials takes the TLS settings.
	var fp FingerprintConfig
	var diag DiagnosticConfig
	var ss SunSpecDetectConfig
	require.NoError(t, LoadFromEnv(&fp))
	require.NoError(t, LoadFromEnv(&diag))
	require.NoError(t, LoadFromEnv(&ss))
	assert.Equal(t, tlsCfg, fp.TLSConfig)
	assert.Equal(t, tlsCfg, diag.TLSConfig)
	assert.Equal(t, tlsCfg, ss.TLSConfig)

	// The servers read the same variables as their certificate, key and client CA bundle.
	serverTLS := ServerTLSConfig{TLSCert: "client.pem", TLSKey: "client-key.pem", TLSCA: "ca.pem"}
	var static StaticServerConfig
	var rp ReplayServerConfig
	require.NoError(t, LoadFromEnv(&static))
	require.NoError(t, LoadFromEnv(&rp))
	assert.Equal(t, serverTLS, static.ServerTLSConfig)
	assert.Equal(t, serverTLS, rp.ServerTLSConfig)
}

func TestIsTLSURL(t *testing.T) {
	assert.True(t, IsTLSURL("tcp+tls://10.0.0.1:802"))
	assert.True(t, IsTLSURL("  tcp+tls://h:802 "))
	for _, u := range []string{"", "tcp://h:502", "tcp+tls", "rtu:///dev/ttyUSB0", "tls://h:802"} {
		assert.False(t, IsTLSURL(u), u)
	}
}

func TestRegisterFlags_everyKind(t *testing.T) {
	c := allKinds{
		EmbeddedKinds: EmbeddedKinds{Inner: "d-inner"},
		S:             "d", B: true, U: 1, U8: 2, U16: 3, U32: 4, U64: 5,
		I: -1, I8: -2, I16: -3, I32: -4, I64: -5, NoEnv: "d-noenv",
	}
	cmd := &cobra.Command{Use: "x"}
	RegisterFlags(cmd, &c)

	// Defaults come from the struct; usage carries the env var when there is one.
	wantDefaults := map[string]string{
		"inner": "d-inner", "s": "d", "b": "true", "u": "1", "u8": "2", "u16": "3", "u32": "4", "u64": "5",
		"i": "-1", "i8": "-2", "i16": "-3", "i32": "-4", "i64": "-5", "l": "[]", "no-env": "d-noenv",
	}
	registered := 0
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		registered++
		want, ok := wantDefaults[f.Name]
		assert.True(t, ok, "unexpected flag --%s", f.Name)
		assert.Equal(t, want, f.DefValue, f.Name)
	})
	assert.Equal(t, len(wantDefaults), registered)
	assert.Equal(t, "a uint16 [env: MBT_U16]", cmd.Flags().Lookup("u16").Usage)
	assert.Equal(t, "inner value [env: MBT_INNER]", cmd.Flags().Lookup("inner").Usage)
	assert.Equal(t, "flag only", cmd.Flags().Lookup("no-env").Usage)

	require.NoError(t, cmd.Flags().Parse([]string{
		"--inner=in", "--s=str", "--b=false", "--u=10", "--u8=255", "--u16=65535", "--u32=4294967295",
		"--u64=18446744073709551615", "--i=-10", "--i8=-128", "--i16=32767", "--i32=-2147483648",
		"--i64=9223372036854775807", "--l=a,b", "--l=c", "--no-env=n",
	}))
	assert.Equal(t, allKinds{
		EmbeddedKinds: EmbeddedKinds{Inner: "in"},
		S:             "str", B: false, U: 10, U8: 255, U16: 65535, U32: 4294967295, U64: 18446744073709551615,
		I: -10, I8: -128, I16: 32767, I32: -2147483648, I64: 9223372036854775807,
		L: []string{"a", "b", "c"}, NoEnv: "n",
	}, c)

	// Out-of-range values are rejected by the typed flag.
	assert.Error(t, cmd.Flags().Parse([]string{"--u8=256"}))
	assert.Error(t, cmd.Flags().Parse([]string{"--i8=128"}))
}

func TestRegisterFlags_envThenFlagPrecedence(t *testing.T) {
	t.Setenv("MODBUSCTL_BAUD", "9600")
	t.Setenv("MODBUSCTL_PARITY", "odd")
	var cfg ReadConfig
	MustLoadFromEnv(&cfg)
	cmd := &cobra.Command{Use: "read"}
	RegisterFlags(cmd, &cfg)

	// The embedded serial settings are exposed as flags, defaulting to the environment.
	for _, name := range []string{"baud", "data-bits", "parity", "stop-bits", "url", "ip", "port", "unit", "timeout", "function", "start", "count", "output", "format"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), name)
	}
	assert.Nil(t, cmd.Flags().Lookup("debug"))
	assert.Equal(t, "9600", cmd.Flags().Lookup("baud").DefValue)

	require.NoError(t, cmd.Flags().Parse([]string{"--parity", "even", "--url", "rtu://COM3"}))
	assert.Equal(t, uint(9600), cfg.Baud)
	assert.Equal(t, "even", cfg.Parity)
	assert.Equal(t, "rtu://COM3", cfg.URL)
}

func TestRegisterFlags_panicsOnUnsupportedKinds(t *testing.T) {
	type intSlice struct {
		N []int `flag:"n"`
	}
	assert.PanicsWithValue(t, `config.RegisterFlags: field N: flag "n" only supports []string slices`,
		func() { RegisterFlags(&cobra.Command{}, &intSlice{}) })

	type float struct {
		F float64 `flag:"f"`
	}
	assert.PanicsWithValue(t,
		`config.RegisterFlags: field F (float64) has flag "f" but unsupported reflect.Kind float64 — add a case or remove the flag tag`,
		func() { RegisterFlags(&cobra.Command{}, &float{}) })

	type nested struct {
		M map[string]string `flag:"m"`
	}
	assert.Panics(t, func() { RegisterFlags(&cobra.Command{}, &nested{}) })
}

func TestLoadFromEnv_formatSpellings(t *testing.T) {
	for _, c := range []struct{ canonical, legacy, want string }{
		{"", "", ""},
		{"json", "", "json"},
		{"", "json", "json"},
		{"table", "json", "table"},
	} {
		t.Setenv("MODBUSCTL_FORMAT", c.canonical)
		t.Setenv("MODBUSCTL_OUTPUT_FORMAT", c.legacy)
		var cfg ReadConfig
		if err := LoadFromEnv(&cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.OutputFormat != c.want {
			t.Errorf("canonical %q, legacy %q: got %q, want %q", c.canonical, c.legacy, cfg.OutputFormat, c.want)
		}
	}
}
