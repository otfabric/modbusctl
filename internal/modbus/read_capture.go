package modbus

import (
	"strings"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// readCapturePath returns the capture (.mbcap) file path for a single client read capture.
func readCapturePath(cfg config.ReadConfig) string {
	capPath := cfg.OutputFile
	if capPath == "" || strings.HasSuffix(capPath, "/") {
		dir := capPath
		if dir == "" {
			dir = "./"
		}
		capPath = AutoCapturePath(dir, "read")
	}
	return capPath
}

// newReadResult builds the stdout DTO from raw capture bytes and paths.
func newReadResult(cfg config.ReadConfig, modbusURL, capPath, finalHex string, byteCount int) *types.ReadResult {
	return &types.ReadResult{
		Target:            modbusURL,
		UnitID:            cfg.Unit,
		Function:          cfg.Function,
		StartAddress:      cfg.StartAddress,
		RegisterCount:     cfg.RegisterCount,
		RawByteCount:      byteCount,
		RawDataHex:        finalHex,
		CaptureOutputPath: capPath,
	}
}
