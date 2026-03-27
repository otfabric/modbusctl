package modbus

import (
	"context"
	"fmt"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// CollectFingerprint probes supported read FCs per unit and returns structured results.
func CollectFingerprint(cfg config.FingerprintConfig) (*types.FingerprintResult, error) {
	units, err := parseUnitIDs(cfg.UnitID)
	if err != nil {
		return nil, err
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	res := &types.FingerprintResult{Target: modbusURL}

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

	interval := time.Duration(cfg.Interval) * time.Millisecond
	for i, unit := range units {
		if i > 0 && interval > 0 {
			time.Sleep(interval)
		}
		ur := types.FingerprintUnitResult{UnitID: unit}
		for _, fc := range readFCsForFingerprint {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
			ok, ferr := mc.SupportsFunction(ctx, unit, fc)
			cancel()
			if ferr != nil {
				ur.Error = ferr.Error()
				ur.ProbeInterrupted = true
				break
			}
			if ok {
				ur.SupportedReads = append(ur.SupportedReads, fc.String())
			}
			if interval > 0 {
				time.Sleep(interval)
			}
		}
		res.Units = append(res.Units, ur)
	}
	return res, nil
}
