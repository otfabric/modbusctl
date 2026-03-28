package config

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestScanAlgorithms_FunctionCodes_SunspecRegtypes(t *testing.T) {
	t.Parallel()
	if len(ScanAlgorithms()) == 0 {
		t.Fatal("scan algos")
	}
	if !slices.Contains(ScanAlgorithms(), "safe") {
		t.Fatal("missing safe")
	}
	if len(FunctionCodes()) != 4 {
		t.Fatalf("function codes: %v", FunctionCodes())
	}
	if len(SunspecRegtypes()) != 2 {
		t.Fatalf("regtypes: %v", SunspecRegtypes())
	}
}

func TestValidScanAlgo(t *testing.T) {
	t.Parallel()
	if !ValidScanAlgo("") || !ValidScanAlgo("  ") {
		t.Fatal("empty allowed")
	}
	if !ValidScanAlgo("SAFE") || !ValidScanAlgo(" boundary ") {
		t.Fatal("normalize")
	}
	if ValidScanAlgo("nope") {
		t.Fatal("reject unknown")
	}
}

func TestRegisterScanAlgoCompletion_wiresFlag(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "scan"}
	cmd.Flags().String("algo", "", "")
	RegisterScanAlgoCompletion(cmd)
	fn, ok := cmd.GetFlagCompletionFunc("algo")
	if !ok || fn == nil {
		t.Fatal("completion")
	}
	out, _ := fn(cmd, nil, "saf")
	if len(out) != 1 || out[0] != "safe" {
		t.Fatalf("got %v", out)
	}
}

func TestRegisterFunctionCompletion(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("function", "", "")
	RegisterFunctionCompletion(cmd)
	fn, ok := cmd.GetFlagCompletionFunc("function")
	if !ok {
		t.Fatal("missing")
	}
	out, _ := fn(cmd, nil, "3")
	if len(out) != 1 || out[0] != "3" {
		t.Fatalf("got %v", out)
	}
}

func TestRegisterDiagnosticSubFunctionCompletion(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("sub-function", "", "")
	RegisterDiagnosticSubFunctionCompletion(cmd)
	fn, ok := cmd.GetFlagCompletionFunc("sub-function")
	if !ok {
		t.Fatal("missing")
	}
	out, _ := fn(cmd, nil, "return")
	if len(out) < 1 || !strings.HasPrefix(out[0], "return") {
		t.Fatalf("got %v", out)
	}
}

func TestRegisterRegtypeCompletion(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("regtype", "", "")
	RegisterRegtypeCompletion(cmd)
	fn, ok := cmd.GetFlagCompletionFunc("regtype")
	if !ok {
		t.Fatal("missing")
	}
	out, _ := fn(cmd, nil, "ho")
	if len(out) != 1 || out[0] != "holding" {
		t.Fatalf("got %v", out)
	}
}

func TestRegisterConvertFormatCompletion(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "", "")
	RegisterConvertFormatCompletion(cmd)
	fn, ok := cmd.GetFlagCompletionFunc("format")
	if !ok {
		t.Fatal("missing")
	}
	out, _ := fn(cmd, nil, "c")
	if len(out) != 1 || !strings.HasPrefix(out[0], "csv") {
		t.Fatalf("got %v", out)
	}
}

func TestDiagnosticSubFunctions_ParseDiagnosticSubFunction(t *testing.T) {
	t.Parallel()
	names := DiagnosticSubFunctions()
	if len(names) != len(diagnosticSubFunctions) {
		t.Fatalf("len %d vs %d", len(names), len(diagnosticSubFunctions))
	}
	code, err := ParseDiagnosticSubFunction("")
	if err != nil || code != 0 {
		t.Fatalf("empty: %v %d", err, code)
	}
	code, err = ParseDiagnosticSubFunction("  ReturnQueryData  ")
	if err != nil || code != 0 {
		t.Fatalf("default name: %v %d", err, code)
	}
	code, err = ParseDiagnosticSubFunction("returnbusmessagecount")
	if err != nil || code != 0x000B {
		t.Fatalf("known: %v %x", err, code)
	}
	_, err = ParseDiagnosticSubFunction("not-a-real-subfunction")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMustLoadFromEnv_panicsOnBadUint(t *testing.T) {
	type cfg struct {
		Port uint16 `env:"TEST_MODBUSCTL_PORT"`
	}
	t.Setenv("TEST_MODBUSCTL_PORT", "not-a-number")
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustLoadFromEnv(&cfg{})
}
