// SPDX-License-Identifier: MIT

package config

import (
	"strings"

	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/spf13/cobra"
)

// Fixed-choice flag values (canonical lists). Completion helpers call [cli.RegisterEnumFlagCompletion] with these.

var scanAlgorithmValues = []string{"safe", "smart", "deep", "stepped", "linear", "boundary", "sunspec"}

// ScanAlgorithms returns valid --algo values for the scan command.
func ScanAlgorithms() []string {
	return scanAlgorithmValues
}

var functionCodeValues = []string{"1", "2", "3", "4"}

// FunctionCodes returns valid Modbus read function codes for --function (1=coils, 2=discrete, 3=holding, 4=input).
func FunctionCodes() []string {
	return functionCodeValues
}

var writeFunctionCodeValues = []string{"5", "6", "15", "16"}

// WriteFunctionCodes returns valid Modbus write function codes for `client write --function`
// (5=single coil, 6=single register, 15=multiple coils, 16=multiple registers).
func WriteFunctionCodes() []string {
	return writeFunctionCodeValues
}

var parityValues = []string{"none", "even", "odd"}

// Parities returns the canonical --parity values.
func Parities() []string {
	return parityValues
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

func RegisterFunctionCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "function", FunctionCodes())
}

// RegisterWriteFunctionCompletion registers shell completion for the --function flag of `client write`.
func RegisterWriteFunctionCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "function", WriteFunctionCodes())
}

// RegisterParityCompletion registers shell completion for the --parity flag when the command has one.
func RegisterParityCompletion(cmd *cobra.Command) {
	if cmd.Flags().Lookup("parity") == nil {
		return
	}
	_ = cli.RegisterEnumFlagCompletion(cmd, "parity", Parities())
}

// CodecIDs returns all registered codec IDs (sorted) from the go-modbus codec registry.
func CodecIDs() []string {
	descs := codec.AvailableCodecDescriptors()
	ids := make([]string, len(descs))
	for i, d := range descs {
		ids[i] = d.ID
	}
	return ids
}

// RegisterCodecCompletion registers shell completion for the --codec flag (decode command).
func RegisterCodecCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "codec", CodecIDs())
}

// RegisterDiagnosticSubFunctionCompletion registers shell completion for the --sub-function flag (diagnostic command).
func RegisterDiagnosticSubFunctionCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "sub-function", DiagnosticSubFunctions())
}

// RegisterRegtypeCompletion registers shell completion for the --regtype flag (sunspec commands).
func RegisterRegtypeCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "regtype", SunspecRegtypes())
}
