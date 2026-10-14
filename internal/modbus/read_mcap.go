package modbus

import (
	"strings"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// readCaptureMcapPath returns the capture (.mbcap) file path for a single client read capture.
func readCaptureMcapPath(cfg config.ReadConfig) string {
	mcapPath := cfg.OutputFile
	if mcapPath == "" || strings.HasSuffix(mcapPath, "/") {
		dir := mcapPath
		if dir == "" {
			dir = "./"
		}
		mcapPath = AutoCaptureMcapPath(dir, "read")
	}
	return mcapPath
}

// newReadResult builds the stdout DTO from raw capture bytes and paths.
func newReadResult(cfg config.ReadConfig, modbusURL, mcapPath, finalHex string, byteCount int) *types.ReadResult {
	return &types.ReadResult{
		Target:         modbusURL,
		UnitID:         cfg.Unit,
		Function:       cfg.Function,
		StartAddress:   cfg.StartAddress,
		RegisterCount:  cfg.RegisterCount,
		RawByteCount:   byteCount,
		RawDataHex:     finalHex,
		McapOutputPath: mcapPath,
	}
}
