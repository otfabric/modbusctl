package modbus

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
)

// CollectReportServerID runs FC17 for configured units and returns structured results.
func CollectReportServerID(ctx context.Context, cfg config.ReportServerIDConfig) (*types.ReportServerIDResult, error) {
	units, err := parseUnitIDs(cfg.UnitID)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, err.Error(), err)
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	res := &types.ReportServerIDResult{Target: modbusURL}

	conf := buildDeviceClientConfig(modbusURL, cfg.Timeout, cfg.Debug)
	reqBudget := clientRequestTimeout(cfg.Timeout)

	useParallel := len(units) > 1 && cfg.Parallel > 1
	if !useParallel {
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

		for _, unit := range units {
			reqCtx, cancel := context.WithTimeout(ctx, reqBudget)
			rs, rerr := mc.ReportServerID(reqCtx, unit)
			cancel()
			ur := types.ReportServerIDUnitResult{UnitID: unit}
			if rerr != nil {
				ur.Error = EmbeddedErrorInfo(rerr)
			} else {
				fillReportServerUnit(&ur, rs)
			}
			res.Units = append(res.Units, ur)
		}
		sortReportServerUnits(res.Units)
		types.FillReportServerIDSummary(res, len(units))
		return res, nil
	}

	n := int(cfg.Parallel)
	if n > len(units) {
		n = len(units)
	}
	clients, cleanup, err := openModbusClientPool(n, conf)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	type job struct {
		unit uint8
		ur   types.ReportServerIDUnitResult
	}
	unitsCh := make(chan uint8, len(units))
	for _, u := range units {
		unitsCh <- u
	}
	close(unitsCh)
	outCh := make(chan job, len(units))

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		mc := clients[i]
		go func() {
			defer wg.Done()
			for unit := range unitsCh {
				reqCtx, cancel := context.WithTimeout(ctx, reqBudget)
				rs, rerr := mc.ReportServerID(reqCtx, unit)
				cancel()
				ur := types.ReportServerIDUnitResult{UnitID: unit}
				if rerr != nil {
					ur.Error = EmbeddedErrorInfo(rerr)
				} else {
					fillReportServerUnit(&ur, rs)
				}
				outCh <- job{unit: unit, ur: ur}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(outCh)
	}()

	for j := range outCh {
		res.Units = append(res.Units, j.ur)
	}
	sortReportServerUnits(res.Units)
	types.FillReportServerIDSummary(res, len(units))
	return res, nil
}

func fillReportServerUnit(ur *types.ReportServerIDUnitResult, rs *modbus.ReportServerIDResponse) {
	ur.DataHex = fmt.Sprintf("% X", rs.Data)
	if rs.RunIndicatorStatus != nil {
		v := *rs.RunIndicatorStatus
		ur.RunIndicatorOn = &v
	}
}

func sortReportServerUnits(units []types.ReportServerIDUnitResult) {
	sort.Slice(units, func(i, j int) bool { return units[i].UnitID < units[j].UnitID })
}
