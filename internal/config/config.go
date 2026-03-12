package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// RegisterFlags registers flags based on struct tags
func RegisterFlags(cmd *cobra.Command, cfg interface{}) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		flagName := field.Tag.Get("flag")
		desc := field.Tag.Get("desc")
		envVar := field.Tag.Get("env")

		if field.Anonymous {
			RegisterFlags(cmd, v.Field(i).Addr().Interface())
			continue
		}

		if flagName == "" {
			continue
		}

		if envVar != "" {
			desc = fmt.Sprintf("%s [env: %s]", desc, envVar)
		}

		switch field.Type.Kind() {
		case reflect.String:
			cmd.Flags().StringVar(v.Field(i).Addr().Interface().(*string), flagName, v.Field(i).String(), desc)
		case reflect.Uint8:
			val := uint8(v.Field(i).Uint())
			cmd.Flags().Uint8Var((*uint8)(v.Field(i).Addr().UnsafePointer()), flagName, val, desc)
		case reflect.Uint16:
			val := uint16(v.Field(i).Uint())
			cmd.Flags().Uint16Var((*uint16)(v.Field(i).Addr().UnsafePointer()), flagName, val, desc)
		case reflect.Uint32:
			val := uint32(v.Field(i).Uint())
			cmd.Flags().Uint32Var((*uint32)(v.Field(i).Addr().UnsafePointer()), flagName, val, desc)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val := int(v.Field(i).Int())
			cmd.Flags().IntVar((*int)(v.Field(i).Addr().UnsafePointer()), flagName, val, desc)
		case reflect.Bool:
			val := v.Field(i).Bool()
			cmd.Flags().BoolVar((*bool)(v.Field(i).Addr().UnsafePointer()), flagName, val, desc)
		case reflect.Slice:
			if field.Type.Elem().Kind() == reflect.String {
				cmd.Flags().StringSliceVar(v.Field(i).Addr().Interface().(*[]string), flagName, nil, desc)
			}
		}
	}
}

// LoadFromEnv populates the config struct with values from environment variables
func LoadFromEnv(cfg interface{}) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		if fieldType.Anonymous && field.Kind() == reflect.Struct {
			LoadFromEnv(field.Addr().Interface())
			continue
		}

		tag := fieldType.Tag.Get("env")
		if tag == "" {
			continue
		}

		envValue := os.Getenv(tag)
		if envValue == "" {
			continue
		}

		switch field.Kind() {
		case reflect.String:
			field.SetString(envValue)
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if val, err := strconv.ParseUint(envValue, 10, 64); err == nil {
				field.SetUint(val)
			}
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if val, err := strconv.ParseInt(envValue, 10, 64); err == nil {
				field.SetInt(val)
			}
		case reflect.Bool:
			if val, err := strconv.ParseBool(envValue); err == nil {
				field.SetBool(val)
			}
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.String {
				parts := strings.Split(envValue, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				field.Set(reflect.ValueOf(parts))
			}
		}
	}
}

type DeviceConfig struct {
	IP   string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP address" flag:"ip"`
	Port uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port" flag:"port"`
	Unit uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID of the Modbus device" flag:"unit"`
}

// UnitClientConfig is used by commands that support a single unit ID or "all" (1–255).
type UnitClientConfig struct {
	IP       string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP address" flag:"ip"`
	Port     uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port" flag:"port"`
	UnitID   string `env:"MODBUSCTL_UNIT" desc:"Unit ID: single (1), range (1-10), list (1,5,25), mixed (1-10,255), or 'all' (1-255)" flag:"unit"`
	Timeout  uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds for the request" flag:"timeout"`
	Parallel uint16 `env:"MODBUSCTL_PARALLEL" desc:"When --unit all, number of concurrent probes (1-64, ignored otherwise)" flag:"parallel"`
}

type IdentifyConfig struct {
	UnitClientConfig
	Basic    bool `env:"MODBUSCTL_IDENTIFY_BASIC" desc:"Request Basic category only (VendorName, ProductCode, MajorMinorRevision)" flag:"basic"`
	Regular  bool `env:"MODBUSCTL_IDENTIFY_REGULAR" desc:"Request Regular category only (Basic + VendorUrl, ProductName, ModelName, UserApplicationName)" flag:"regular"`
	Extended bool `env:"MODBUSCTL_IDENTIFY_EXTENDED" desc:"Request Extended category only (Regular + vendor-specific objects)" flag:"extended"`
	ServerID bool `env:"MODBUSCTL_IDENTIFY_SERVERID" desc:"Also query FC17 Report Server ID for additional device information" flag:"server-id"`
}

type ReadConfig struct {
	DeviceConfig
	Function      uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	StartAddress  uint16 `env:"MODBUSCTL_ADDRESS" desc:"Starting register address" flag:"start"`
	RegisterCount uint16 `env:"MODBUSCTL_COUNT" desc:"Number of registers to read" flag:"count"`
	Ascii         bool   `env:"MODBUSCTL_ASCII" desc:"Attempt ASCII decoding for output" flag:"ascii"`
	SwapBytes     bool   `env:"MODBUSCTL_BYTESWAP" desc:"Enable byte swapping for registers" flag:"byteswap"`
	OutputFile    string `env:"MODBUSCTL_OUTPUT" desc:"Output MCAP file or directory" flag:"output"`
	Debug         bool   // set from global --debug (root persistent flag)
}

type RecordConfig struct {
	DeviceConfig
	Function   uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	Interval   uint32 `env:"MODBUSCTL_INTERVAL" desc:"Interval in milliseconds between reads" flag:"interval"`
	Duration   uint32 `env:"MODBUSCTL_DURATION" desc:"Total duration to record in milliseconds" flag:"duration"`
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file to append to" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output MCAP file or directory" flag:"output"`
	Debug      bool   // set from global --debug (root persistent flag)
}

// ScanAlgorithm selects the scan strategy: safe (conservative linear), smart (interval splitting), or deep (smart + boundary refinement).
type ScanAlgorithm string

const (
	ScanAlgoSafe  ScanAlgorithm = "safe"
	ScanAlgoSmart ScanAlgorithm = "smart"
	ScanAlgoDeep  ScanAlgorithm = "deep"
)

type ScanConfig struct {
	DeviceConfig
	Delay                   uint16 `env:"MODBUSCTL_DELAY" desc:"Delay in milliseconds between requests" flag:"delay"`
	Function                uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	StartAddress            uint16 `env:"MODBUSCTL_START" desc:"Start register address" flag:"start"`
	EndAddress              uint16 `env:"MODBUSCTL_END" desc:"End register address" flag:"end"`
	OutputFile              string `env:"MODBUSCTL_OUTPUT" desc:"Output file" flag:"output"`
	Algo                    string `env:"MODBUSCTL_ALGO" desc:"Scan algorithm: safe, smart, deep, stepped, linear, or boundary" flag:"algo"`
	Step                    uint16 `env:"MODBUSCTL_STEP" desc:"Stepped algo: step size (e.g. 100, 1000, 2000)" flag:"step"`
	StepHalfOffset          bool   `env:"MODBUSCTL_STEP_HALF_OFFSET" desc:"Stepped algo: also probe at step/2 offset" flag:"step-half-offset"`
	SeedStart               uint16 `env:"MODBUSCTL_SEED_START" desc:"Boundary algo: seed start address (known good read)" flag:"seed-start"`
	SeedCount               uint16 `env:"MODBUSCTL_SEED_COUNT" desc:"Boundary algo: seed register count (1-125)" flag:"seed-count"`
	RetryOnTimeoutTransport uint8  `env:"MODBUSCTL_RETRY_TIMEOUT" desc:"Retry once on timeout or transport error (0=no, 1=yes)" flag:"retry-timeout"`
	Debug                   bool   // set from global --debug (root persistent flag)
}

type ConvertConfig struct {
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file" flag:"input"`
	FormatType string `env:"MODBUSCTL_FORMAT" desc:"Output format type (e.g., CSV, JSON)" flag:"format"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type ExtractConfig struct {
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type StringsConfig struct {
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type InfoConfig struct {
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type StaticServerConfig struct {
	Port      uint16 `env:"MODBUSCTL_PORT" desc:"Port for the static server" flag:"port"`
	Unit      uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID for the Modbus device" flag:"unit"`
	Function  uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	InputFile string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file to serve" flag:"input"`
}

type ReplayServerConfig struct {
	Port      uint16 `env:"MODBUSCTL_PORT" desc:"Port for the replay server" flag:"port"`
	Unit      uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID for the Modbus device" flag:"unit"`
	Function  uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	Loops     uint16 `env:"MODBUSCTL_LOOPS" desc:"Number of times to loop the replay" flag:"loops"`
	Interval  uint32 `env:"MODBUSCTL_INTERVAL" desc:"Interval in milliseconds between iterations (0 for original timing)" flag:"interval"`
	InputFile string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file to replay" flag:"input"`
}

type DiscoverConfig struct {
	Subnets          []string `env:"MODBUSCTL_SUBNETS" desc:"List of subnets to scan for Modbus devices (e.g., 192.168.1.0/24,192.168.2.0/24)" flag:"subnets"`
	Port             uint16   `env:"MODBUSCTL_PORT" desc:"Port to use for Modbus TCP discovery" flag:"port"`
	Parallel         uint8    `env:"MODBUSCTL_PARALLEL" desc:"Number of parallel discovery threads" flag:"parallel"`
	ResolveMAC       bool     `env:"MODBUSCTL_RESOLVE_MAC" desc:"Resolve MAC addresses of discovered devices" flag:"resolve-mac"`
	NetworkInterface string   `env:"MODBUSCTL_INTERFACE" desc:"Network interface to use for discovery" flag:"interface"`
	OutputFile       string   `env:"MODBUSCTL_OUTPUT" desc:"Output file for discovered devices" flag:"output"`
}

// FingerprintConfig is used by the fingerprint command (probe supported read FCs per unit via HasUnitReadFunction).
type FingerprintConfig struct {
	IP       string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP address" flag:"ip"`
	Port     uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port" flag:"port"`
	UnitID   string `env:"MODBUSCTL_UNIT" desc:"Unit ID: single, range (1-10), list (1,5,25), mixed (1-10,255), or 'all'" flag:"unit"`
	Timeout  uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds per probe" flag:"timeout"`
	Interval uint32 `env:"MODBUSCTL_INTERVAL" desc:"Interval in milliseconds between probes" flag:"interval"`
}

// DiagnosticConfig is used by the diagnostic command (FC08 Diagnostics).
type DiagnosticConfig struct {
	IP          string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP address" flag:"ip"`
	Port        uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port" flag:"port"`
	UnitID      uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID of the Modbus device" flag:"unit"`
	Timeout     uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds for the request" flag:"timeout"`
	SubFunction string `env:"MODBUSCTL_SUB_FUNCTION" desc:"FC08 sub-function name (e.g. returnquerydata, clearcountersanddiagnosticreg)" flag:"sub-function"`
	Data        string `env:"MODBUSCTL_DATA" desc:"Hex-encoded request data (e.g. 'A537'); defaults to 0000" flag:"data"`
}

// Diagnostic sub-functions (match github.com/boeboe/modbus DiagnosticSubFunction). Single source of truth for names and codes.
var diagnosticSubFunctions = []struct {
	name string
	code uint16
}{
	{"returnquerydata", 0x0000},
	{"restartcommunications", 0x0001},
	{"returndiagnosticregister", 0x0002},
	{"changeasciiinputdelimiter", 0x0003},
	{"forcelistenonlymode", 0x0004},
	{"clearcountersanddiagnosticreg", 0x000A},
	{"returnbusmessagecount", 0x000B},
	{"returnbuscommunicationerrorcount", 0x000C},
	{"returnbusexceptionerrorcount", 0x000D},
	{"returnservermessagecount", 0x000E},
	{"returnservernoresponsecount", 0x000F},
	{"returnservernakcount", 0x0010},
	{"returnserverbusycount", 0x0011},
	{"returnbuscharacteroverruncount", 0x0012},
	{"clearoverruncounterandflag", 0x0014},
}

// DiagnosticSubFunctionNames is the list of valid --sub-function values (for completion and validation).
var DiagnosticSubFunctionNames []string

// diagnosticSubFunctionMap maps lowercase name -> code for ParseDiagnosticSubFunction.
var diagnosticSubFunctionMap map[string]uint16

func init() {
	DiagnosticSubFunctionNames = make([]string, len(diagnosticSubFunctions))
	diagnosticSubFunctionMap = make(map[string]uint16, len(diagnosticSubFunctions))
	for i, sf := range diagnosticSubFunctions {
		DiagnosticSubFunctionNames[i] = sf.name
		diagnosticSubFunctionMap[sf.name] = sf.code
	}
}

// ParseDiagnosticSubFunction returns the uint16 value for the given sub-function name (case-insensitive).
func ParseDiagnosticSubFunction(name string) (uint16, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return 0x0000, nil // default ReturnQueryData
	}
	if v, ok := diagnosticSubFunctionMap[key]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unknown diagnostic sub-function %q (allowed: %v)", name, DiagnosticSubFunctionNames)
}

// ReportServerIdConfig is used by the reportserverid command (FC17 Report Server ID).
type ReportServerIdConfig struct {
	UnitClientConfig
}

type DeviceProfileDecodeConfig struct {
	InputFile     string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file with device profile" flag:"input"`
	DeviceProfile string `env:"MODBUSCTL_PROFILE" desc:"Device profile to decode" flag:"profile"`
	OutputFile    string `env:"MODBUSCTL_OUTPUT" desc:"Output file for decoded profile" flag:"output"`
}

// Enum values for shell completion (single source of truth; validation should use these too).

// ScanAlgoValues is the list of valid --algo values for the scan command.
var ScanAlgoValues = []string{"safe", "smart", "deep", "stepped", "linear", "boundary"}

// ConvertFormatValues is the list of valid --format values for mcap convert.
var ConvertFormatValues = []string{"csv", "json"}

// FunctionCodeValues is the list of valid Modbus read function codes for --function (1=coils, 2=discrete, 3=holding, 4=input).
var FunctionCodeValues = []string{"1", "2", "3", "4"}

// ValidScanAlgo returns true if algo (after trim and lower) is in ScanAlgoValues. Use for validation so allowed values stay in sync with completion.
func ValidScanAlgo(algo string) bool {
	algo = strings.ToLower(strings.TrimSpace(algo))
	if algo == "" {
		return true
	}
	for _, v := range ScanAlgoValues {
		if algo == v {
			return true
		}
	}
	return false
}

// RegisterScanAlgoCompletion registers shell completion for the --algo flag (scan command).
func RegisterScanAlgoCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("algo", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return ScanAlgoValues, cobra.ShellCompDirectiveNoFileComp
	})
}

// RegisterConvertFormatCompletion registers shell completion for the --format flag (mcap convert).
func RegisterConvertFormatCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return ConvertFormatValues, cobra.ShellCompDirectiveNoFileComp
	})
}

// RegisterFunctionCompletion registers shell completion for the --function flag (read/record/scan/static/replay).
func RegisterFunctionCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("function", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return FunctionCodeValues, cobra.ShellCompDirectiveNoFileComp
	})
}

// RegisterDiagnosticSubFunctionCompletion registers shell completion for the --sub-function flag (diagnostic command).
func RegisterDiagnosticSubFunctionCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("sub-function", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return DiagnosticSubFunctionNames, cobra.ShellCompDirectiveNoFileComp
	})
}
