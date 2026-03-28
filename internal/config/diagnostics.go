package config

import (
	"fmt"
	"slices"
	"strings"
)

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

// diagnosticSubFunctionNames is populated in init (valid --sub-function values).
var diagnosticSubFunctionNames []string

// diagnosticSubFunctionMap maps lowercase name -> code for ParseDiagnosticSubFunction.
var diagnosticSubFunctionMap map[string]uint16

func init() {
	diagnosticSubFunctionNames = make([]string, len(diagnosticSubFunctions))
	diagnosticSubFunctionMap = make(map[string]uint16, len(diagnosticSubFunctions))
	for i, sf := range diagnosticSubFunctions {
		diagnosticSubFunctionNames[i] = sf.name
		diagnosticSubFunctionMap[sf.name] = sf.code
	}
}

// DiagnosticSubFunctions returns valid FC08 --sub-function names (completion and validation).
func DiagnosticSubFunctions() []string {
	return slices.Clone(diagnosticSubFunctionNames)
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
	return 0, fmt.Errorf("unknown diagnostic sub-function %q (allowed: %v)", name, DiagnosticSubFunctions())
}
