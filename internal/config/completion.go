package config

import (
	"strings"

	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/spf13/cobra"
)

// Fixed-choice flag values (canonical lists). Completion helpers call [cli.RegisterEnumFlagCompletion] with these.

var scanAlgorithmValues = []string{"safe", "smart", "deep", "stepped", "linear", "boundary", "sunspec"}

// ScanAlgorithms returns valid --algo values for the scan command.
func ScanAlgorithms() []string {
	return scanAlgorithmValues
}

var convertFormatValues = []string{"csv", "json"}

// ConvertFormats returns valid mcap convert --format values (distinct from client stdout [format.Values]).
func ConvertFormats() []string {
	return convertFormatValues
}

// ConvertFormatDescriptions maps mcap convert --format values to short shell-completion descriptions (keys must match [ConvertFormats]).
var ConvertFormatDescriptions = map[string]string{
	"csv":  "comma-separated values",
	"json": "JSON export",
}

var functionCodeValues = []string{"1", "2", "3", "4"}

// FunctionCodes returns valid Modbus read function codes for --function (1=coils, 2=discrete, 3=holding, 4=input).
func FunctionCodes() []string {
	return functionCodeValues
}

var sunspecRegtypeValues = []string{"holding", "input"}

// SunspecRegtypes returns valid sunspec --regtype values.
func SunspecRegtypes() []string {
	return sunspecRegtypeValues
}

// ValidScanAlgo returns true if algo (after trim and lower) is allowed for --algo.
func ValidScanAlgo(algo string) bool {
	algo = strings.ToLower(strings.TrimSpace(algo))
	if algo == "" {
		return true
	}
	for _, v := range scanAlgorithmValues {
		if algo == v {
			return true
		}
	}
	return false
}

// RegisterScanAlgoCompletion registers shell completion for the --algo flag (scan command).
func RegisterScanAlgoCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "algo", ScanAlgorithms())
}

// RegisterConvertFormatCompletion registers shell completion for the --format flag (mcap convert).
func RegisterConvertFormatCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletionWithDescriptions(cmd, "format", ConvertFormatDescriptions)
}

// RegisterFunctionCompletion registers shell completion for the --function flag (read/record/scan/static/replay).
func RegisterFunctionCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "function", FunctionCodes())
}

// RegisterDiagnosticSubFunctionCompletion registers shell completion for the --sub-function flag (diagnostic command).
func RegisterDiagnosticSubFunctionCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "sub-function", DiagnosticSubFunctions())
}

// RegisterRegtypeCompletion registers shell completion for the --regtype flag (sunspec commands).
func RegisterRegtypeCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "regtype", SunspecRegtypes())
}
