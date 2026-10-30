package config

import (
	"fmt"
	"slices"
	"strings"
)

// Diagnostic sub-functions (match github.com/boeboe/modbus DiagnosticSubFunction). Single source of truth for names and codes.
var diagnosticSubFunctions = []struct {
	name    string
	code    uint16
	changes bool // changes the state of the device: needs --yes
}{
	{"returnquerydata", 0x0000, false},
	{"restartcommunications", 0x0001, true},
	{"returndiagnosticregister", 0x0002, false},
	{"changeasciiinputdelimiter", 0x0003, true},
	{"forcelistenonlymode", 0x0004, true},
	{"clearcountersanddiagnosticreg", 0x000A, true},
	{"returnbusmessagecount", 0x000B, false},
	{"returnbuscommunicationerrorcount", 0x000C, false},
	{"returnbusexceptionerrorcount", 0x000D, false},
	{"returnservermessagecount", 0x000E, false},
	{"returnservernoresponsecount", 0x000F, false},
	{"returnservernakcount", 0x0010, false},
	{"returnserverbusycount", 0x0011, false},
	{"returnbuscharacteroverruncount", 0x0012, false},
	{"clearoverruncounterandflag", 0x0014, true},
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

// DiagnosticChangesState reports whether the FC08 sub-function code changes the state of
// the device. A code outside the table counts as changing it.
func DiagnosticChangesState(code uint16) bool {
	for _, sf := range diagnosticSubFunctions {
		if sf.code == code {
			return sf.changes
		}
	}
	return true
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
