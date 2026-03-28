package modbus

import (
	"os"
	"strings"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/types"
)

// readCaptureMcapPath returns the MCAP file path for a single client read capture.
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

// writeReadCaptureToMcap writes the capture header and one record for a read operation.
func writeReadCaptureToMcap(f *os.File, cfg config.ReadConfig, modbusURL string, raw []byte, requestTimestamp, responseTimestamp int64) error {
	headerIP, headerPort := cfg.IP, cfg.Port
	if strings.TrimSpace(cfg.URL) != "" {
		headerIP, headerPort = config.ParseModbusURLHostPort(modbusURL)
	}
	header := types.CaptureHeader{
		IP:        headerIP,
		Port:      headerPort,
		Unit:      cfg.Unit,
		Function:  cfg.Function,
		StartTime: time.Now().UnixNano(),
	}
	record := types.CaptureRecord{
		Iteration:         0,
		RequestTimestamp:  requestTimestamp,
		ResponseTimestamp: responseTimestamp,
		StartAddress:      cfg.StartAddress,
		RegisterCount:     cfg.RegisterCount,
		Data:              raw,
	}
	if err := mcap.WriteHeader(f, header); err != nil {
		return err
	}
	return mcap.AppendRecord(f, record)
}

// newReadResult builds the stdout DTO from raw capture bytes (after optional swap) and paths.
func newReadResult(cfg config.ReadConfig, modbusURL, mcapPath, preSwapHex, finalHex, asciiDecoded string, byteCount int) *types.ReadResult {
	return &types.ReadResult{
		Target:         modbusURL,
		UnitID:         cfg.Unit,
		Function:       cfg.Function,
		StartAddress:   cfg.StartAddress,
		RegisterCount:  cfg.RegisterCount,
		RawByteCount:   byteCount,
		PreSwapHex:     preSwapHex,
		RawDataHex:     finalHex,
		BytesSwapped:   cfg.SwapBytes,
		AsciiDecoded:   asciiDecoded,
		McapOutputPath: mcapPath,
	}
}
