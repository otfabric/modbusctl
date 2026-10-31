// SPDX-License-Identifier: MIT

// Package analysis implements the modbusctl capture commands: register-block
// analyses of MBCAP capture files (decode, extract, strings, frequencies).
// Inspecting, verifying and converting capture files is left to mbcapctl of
// github.com/otfabric/go-modbus-capture.
package analysis

import (
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// LoadRecords reads a capture file and returns the FC01-04 samples of the
// selected target and function as register-block records (see
// [capfile.Records]).
func LoadRecords(path string, sel config.CaptureSelection) ([]types.CaptureRecord, types.CaptureHeader, error) {
	l, err := capfile.Load(path)
	if err != nil {
		return nil, types.CaptureHeader{}, err
	}
	return capfile.Records(l, capfile.Selection{Target: sel.Target, Function: sel.Function})
}
