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

// CollectDeviceIdentification performs FC43 device identification (and optional FC17) for the
// configured units and returns a structured result. Connection and client setup failures return a
// non-nil error; per-unit Modbus failures are encoded in IdentifyUnitResult.Error.
func CollectDeviceIdentification(ctx context.Context, cfg config.IdentifyConfig) (*types.IdentifyResult, error) {
	units, err := parseUnitIDs(cfg.UnitID)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, err.Error(), err)
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	res := &types.IdentifyResult{Target: modbusURL}

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
			u := collectDeviceIdentificationForUnit(reqCtx, mc, cfg, unit)
			cancel()
			res.Units = append(res.Units, *u)
		}
		sortIdentifyUnitsByID(res.Units)
		types.FillIdentifySummary(res, len(units))
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

	unitsCh := make(chan uint8, len(units))
	for _, u := range units {
		unitsCh <- u
	}
	close(unitsCh)
	resultsCh := make(chan *types.IdentifyUnitResult, len(units))

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		mc := clients[i]
		go func() {
			defer wg.Done()
			for unit := range unitsCh {
				reqCtx, cancel := context.WithTimeout(ctx, reqBudget)
				u := collectDeviceIdentificationForUnit(reqCtx, mc, cfg, unit)
				cancel()
				resultsCh <- u
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for u := range resultsCh {
		res.Units = append(res.Units, *u)
	}
	sortIdentifyUnitsByID(res.Units)
	types.FillIdentifySummary(res, len(units))
	return res, nil
}

func sortIdentifyUnitsByID(units []types.IdentifyUnitResult) {
	sort.Slice(units, func(i, j int) bool { return units[i].UnitID < units[j].UnitID })
}

func collectDeviceIdentificationForUnit(ctx context.Context, mc *modbus.Client, cfg config.IdentifyConfig, unit uint8) *types.IdentifyUnitResult {
	out := &types.IdentifyUnitResult{UnitID: unit}
	useCategories := cfg.Basic || cfg.Regular || cfg.Extended

	if !useCategories {
		di, err := mc.ReadAllDeviceIdentification(ctx, unit)
		if err != nil {
			out.Error = EmbeddedErrorInfo(err)
			return out
		}
		if di == nil {
			out.Error = EmbeddedErrorInfo(ErrFC43NotSupported)
			return out
		}
		fillIdentifyUnitFromDI(out, di)
		if cfg.ServerID {
			appendReportServerID(out, ctx, mc, unit)
		}
		return out
	}

	objectsByID := make(map[modbus.DeviceIDObjectID]modbus.DeviceIdentificationObject)
	var header *modbus.DeviceIdentification
	for _, category := range []struct {
		flag bool
		cat  modbus.DeviceIDCategory
	}{
		{cfg.Basic, modbus.DeviceIDBasic},
		{cfg.Regular, modbus.DeviceIDRegular},
		{cfg.Extended, modbus.DeviceIDExtended},
	} {
		if !category.flag {
			continue
		}
		di, err := mc.ReadDeviceIdentification(ctx, unit, category.cat, 0)
		if err != nil {
			out.Error = EmbeddedErrorInfo(err)
			return out
		}
		if di == nil {
			continue
		}
		if header == nil {
			header = di
		}
		for _, obj := range di.Objects {
			if _, seen := objectsByID[obj.ID]; !seen {
				objectsByID[obj.ID] = obj
			}
		}
	}
	if header == nil {
		out.Error = EmbeddedErrorInfo(ErrFC43NotSupported)
		return out
	}

	ids := make([]modbus.DeviceIDObjectID, 0, len(objectsByID))
	for id := range objectsByID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	cat := uint8(header.Category)
	cl := header.ConformityLevel
	mf := header.MoreFollows
	noid := uint8(header.NextObjectID)
	out.Category = &cat
	out.ConformityLevel = &cl
	out.MoreFollows = &mf
	out.NextObjectID = &noid

	for _, id := range ids {
		obj := objectsByID[id]
		desc := obj.Name
		if desc == "" {
			desc = ObjectDescription(obj.ID)
		}
		out.Objects = append(out.Objects, types.IdentifyObjectRow{
			ID:          uint8(obj.ID),
			Value:       obj.Value,
			Description: desc,
		})
	}

	if cfg.ServerID {
		appendReportServerID(out, ctx, mc, unit)
	}
	return out
}

func fillIdentifyUnitFromDI(out *types.IdentifyUnitResult, di *modbus.DeviceIdentification) {
	cat := uint8(di.Category)
	cl := di.ConformityLevel
	mf := di.MoreFollows
	noid := uint8(di.NextObjectID)
	out.Category = &cat
	out.ConformityLevel = &cl
	out.MoreFollows = &mf
	out.NextObjectID = &noid
	for _, obj := range di.Objects {
		desc := obj.Name
		if desc == "" {
			desc = ObjectDescription(obj.ID)
		}
		out.Objects = append(out.Objects, types.IdentifyObjectRow{
			ID:          uint8(obj.ID),
			Value:       obj.Value,
			Description: desc,
		})
	}
}

func appendReportServerID(out *types.IdentifyUnitResult, ctx context.Context, mc *modbus.Client, unit uint8) {
	rs, err := mc.ReportServerID(ctx, unit)
	if err != nil {
		out.ReportServerID = &types.IdentifyReportServerOutput{Error: EmbeddedErrorInfo(err)}
		return
	}
	payload := &types.IdentifyReportServerOutput{
		DataHex: fmt.Sprintf("% X", rs.Data),
	}
	if rs.RunIndicatorStatus != nil {
		v := *rs.RunIndicatorStatus
		payload.RunIndicatorOn = &v
	}
	out.ReportServerID = payload
}
