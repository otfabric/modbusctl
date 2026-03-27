package modbus

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// CollectDiagnostics sends FC08 and returns a structured result.
func CollectDiagnostics(cfg config.DiagnosticConfig) (*types.DiagnosticResult, error) {
	subFuncCode, err := config.ParseDiagnosticSubFunction(cfg.SubFunction)
	if err != nil {
		return nil, err
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)

	conf := buildClientConfig(modbusURL, time.Duration(cfg.Timeout)*time.Millisecond, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	mc, err := modbus.New(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTCPConnection, err)
	}
	defer func() { _ = mc.Close() }()

	var data []byte
	if cfg.Data != "" {
		data, err = hex.DecodeString(cfg.Data)
		if err != nil {
			return nil, fmt.Errorf("invalid hex data: %w", err)
		}
	} else {
		data = []byte{0x00, 0x00}
	}

	sf := modbus.DiagnosticSubFunction(subFuncCode)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
	defer cancel()

	resp, err := mc.Diagnostics(ctx, cfg.UnitID, sf, data)
	if err != nil {
		return nil, fmt.Errorf("FC08 Diagnostics failed: %w", err)
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
