package modbus

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
)

// CollectDiagnostics sends FC08 and returns a structured result.
func CollectDiagnostics(ctx context.Context, cfg config.DiagnosticConfig) (*types.DiagnosticResult, error) {
	subFuncCode, err := config.ParseDiagnosticSubFunction(cfg.SubFunction)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)

	mc, cleanup, err := connectDevice(modbusURL, cfg.Timeout, cfg.Debug)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var data []byte
	if cfg.Data != "" {
		clean := strings.ReplaceAll(strings.TrimSpace(cfg.Data), " ", "")
		data, err = hex.DecodeString(clean)
		if err != nil {
			return nil, errs.InvalidInput(errs.CodeInvalidInput, "invalid hex data: "+err.Error(), err)
		}
	} else {
		data = []byte{0x00, 0x00}
	}

	sf := modbus.DiagnosticSubFunction(subFuncCode)
	reqCtx, cancel := context.WithTimeout(ctx, clientRequestTimeout(cfg.Timeout))
	defer cancel()

	resp, err := mc.Diagnostics(reqCtx, cfg.UnitID, sf, data)
	if err != nil {
		return nil, errs.New(errs.KindModbus, errs.CodeDiagnosticsFailed, "Modbus diagnostics (FC08) failed", err)
	}

	sfName := cfg.SubFunction
	if sfName == "" {
		sfName = "returnquerydata"
	}

	return &types.DiagnosticResult{
		Target:         modbusURL,
		UnitID:         cfg.UnitID,
		SubFunction:    sfName,
		SubFunctionHex: uint16(resp.SubFunction),
		DataHex:        fmt.Sprintf("% X", resp.Data),
	}, nil
}
