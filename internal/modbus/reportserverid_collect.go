package modbus

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// CollectReportServerID runs FC17 for configured units and returns structured results.
func CollectReportServerID(cfg config.ReportServerIdConfig) (*types.ReportServerIDResult, error) {
	units, err := parseUnitIDs(cfg.UnitID)
	if err != nil {
		return nil, err
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	res := &types.ReportServerIDResult{Target: modbusURL}

	conf := buildClientConfig(modbusURL, time.Duration(cfg.Timeout)*time.Millisecond, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	useParallel := len(units) > 1 && cfg.Parallel > 1
	if !useParallel {
		mc, err := modbus.New(conf)
		if err != nil {
			return nil, fmt.Errorf("failed to create client: %w", err)
		}
		if err := mc.Open(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrTCPConnection, err)
		}
		defer func() { _ = mc.Close() }()

		for _, unit := range units {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
			rs, rerr := mc.ReportServerID(ctx, unit)
			cancel()
			ur := types.ReportServerIDUnitResult{UnitID: unit}
			if rerr != nil {
				ur.Error = rerr.Error()
			} else {
				fillReportServerUnit(&ur, rs)
			}
			res.Units = append(res.Units, ur)
		}
		sortReportServerUnits(res.Units)
		return res, nil
	}

	n := int(cfg.Parallel)
	if n > len(units) {
		n = len(units)
	}
	clients := make([]*modbus.Client, 0, n)
	for i := 0; i < n; i++ {
		mc, err := modbus.New(conf)
		if err != nil {
			for _, c := range clients {
				_ = c.Close()
			}
			return nil, fmt.Errorf("failed to create client: %w", err)
		}
		if err := mc.Open(); err != nil {
			for _, c := range clients {
				_ = c.Close()
			}
			return nil, fmt.Errorf("%w: %v", ErrTCPConnection, err)
		}
		clients = append(clients, mc)
	}
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()

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
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
				rs, rerr := mc.ReportServerID(ctx, unit)
				cancel()
				ur := types.ReportServerIDUnitResult{UnitID: unit}
				if rerr != nil {
					ur.Error = rerr.Error()
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
