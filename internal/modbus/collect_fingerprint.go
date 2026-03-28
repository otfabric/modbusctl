package modbus

import (
	"context"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
)

// CollectFingerprint probes supported read FCs per unit and returns structured results.
func CollectFingerprint(ctx context.Context, cfg config.FingerprintConfig) (*types.FingerprintResult, error) {
	units, err := parseUnitIDs(cfg.UnitID)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, err.Error(), err)
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	res := &types.FingerprintResult{Target: modbusURL}

	conf := buildDeviceClientConfig(modbusURL, cfg.Timeout, cfg.Debug)
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, ClientConfigInvalid(err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return nil, ClientSetupError(err)
	}
	if err := mc.Open(); err != nil {
		return nil, TCPConnectionError(err)
	}
	defer func() { _ = mc.Close() }()

	interval := time.Duration(cfg.Interval) * time.Millisecond
	for _, unit := range units {
		ur := types.FingerprintUnitResult{UnitID: unit}
		for _, fc := range readFCsForFingerprint {
			reqCtx, cancel := context.WithTimeout(ctx, clientRequestTimeout(cfg.Timeout))
			ok, ferr := mc.SupportsFunction(reqCtx, unit, fc)
			cancel()
			if ferr != nil {
				ur.Error = EmbeddedErrorInfo(ferr)
				ur.ProbeInterrupted = true
				break
			}
			if ok {
				ur.SupportedReads = append(ur.SupportedReads, fc.String())
			}
			if interval > 0 {
				if err := sleepContext(ctx, interval); err != nil {
					return nil, ctx.Err()
				}
			}
		}
		res.Units = append(res.Units, ur)
	}
	types.FillFingerprintSummary(res, len(units))
	return res, nil
}
