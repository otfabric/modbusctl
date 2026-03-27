package modbus

import (
	"context"
	"fmt"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus/sunspec"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// readerAdapter adapts *modbus.Client to sunspec.Reader (sunspec expects ReadRawBytes; Client has ReadRegisterBytes).
type readerAdapter struct{ client *modbus.Client }

func (r *readerAdapter) ReadRawBytes(ctx context.Context, unitID uint8, addr uint16, byteCount uint16, regType sunspec.RegType) ([]byte, error) {
	return r.client.ReadRegisterBytes(ctx, unitID, addr, byteCount, regType)
}

func regTypeFromString(s string) modbus.RegType {
	switch s {
	case "input":
		return modbus.InputRegister
	default:
		return modbus.HoldingRegister
	}
}

func sunSpecOptionsFromBase(cfg *config.SunSpecBaseConfig, bases []uint16) *sunspec.Options {
	opts := &sunspec.Options{
		UnitID:    cfg.Unit,
		RegType:   regTypeFromString(cfg.Regtype),
		MaxModels: 256,
	}
	if len(bases) > 0 {
		opts.BaseAddresses = bases
	} else {
		opts.BaseAddresses = sunspec.DefaultBaseAddresses
	}
	return opts
}

func sunSpecModelHeaders(ms []sunspec.ModelHeader) []types.SunSpecModelHeader {
	out := make([]types.SunSpecModelHeader, 0, len(ms))
	for _, m := range ms {
		out = append(out, types.SunSpecModelHeader{
			ID:           m.ID,
			Length:       m.Length,
			StartAddress: m.StartAddress,
			EndAddress:   m.EndAddress,
			NextAddress:  m.NextAddress,
			IsEndModel:   m.IsEndModel,
		})
	}
	return out
}

// CollectSunSpecDetect runs SunSpec marker detection and returns structured output.
func CollectSunSpecDetect(cfg config.SunSpecDetectConfig) (*types.SunSpecDetectOutput, error) {
	modbusURL := config.SunSpecModbusURL(&cfg.SunSpecBaseConfig)
	if modbusURL == "" {
		return nil, fmt.Errorf("either --url or --ip must be set")
	}

	var bases []uint16
	if cfg.Bases != "" {
		var err error
		bases, err = config.ParseSunSpecBases(cfg.Bases)
		if err != nil {
			return nil, err
		}
	}

	opts := sunSpecOptionsFromBase(&cfg.SunSpecBaseConfig, bases)

	conf := buildClientConfig(modbusURL, 10*time.Second, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = mc.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := sunspec.Detect(ctx, &readerAdapter{mc}, opts)
	if err != nil {
		return nil, fmt.Errorf("detect failed: %w", err)
	}

	out := &types.SunSpecDetectOutput{
		Target:      modbusURL,
		UnitID:      res.UnitID,
		Regtype:     cfg.Regtype,
		Verbose:     cfg.Verbose,
		Detected:    res.Detected,
		BaseAddress: res.BaseAddress,
	}
	for i, a := range res.Attempts {
		result := "matched SunS"
		if !a.Matched {
			if a.ErrorString != "" {
				result = a.ErrorString
			} else if len(a.Registers) != 2 {
				result = "no marker"
			} else {
				result = "no marker"
			}
		}
		out.Attempts = append(out.Attempts, types.SunSpecProbeAttempt{
			Index:       i + 1,
			BaseAddress: a.BaseAddress,
			Matched:     a.Matched,
			Result:      result,
		})
	}
	return out, nil
}

// CollectSunSpecModels enumerates model headers and returns structured output.
func CollectSunSpecModels(cfg config.SunSpecModelsConfig) (*types.SunSpecModelsOutput, error) {
	modbusURL := config.SunSpecModbusURL(&cfg.SunSpecBaseConfig)
	if modbusURL == "" {
		return nil, fmt.Errorf("either --url or --ip must be set")
	}

	opts := sunSpecOptionsFromBase(&cfg.SunSpecBaseConfig, nil)
	opts.MaxModels = cfg.MaxModels
	if opts.MaxModels <= 0 {
		opts.MaxModels = 256
	}
	opts.MaxAddressSpan = cfg.MaxAddressSpan

	conf := buildClientConfig(modbusURL, 10*time.Second, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = mc.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var baseAddr uint16
	var models []sunspec.ModelHeader

	if cfg.Base != 0 {
		baseAddr = cfg.Base
		var readErr error
		models, readErr = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, baseAddr)
		if readErr != nil {
			return nil, fmt.Errorf("read model headers: %w", readErr)
		}
	} else {
		det, derr := sunspec.Detect(ctx, &readerAdapter{mc}, opts)
		if derr != nil {
			return nil, fmt.Errorf("detect: %w", derr)
		}
		if !det.Detected {
			return &types.SunSpecModelsOutput{Target: modbusURL, NotDetected: true}, nil
		}
		baseAddr = det.BaseAddress
		var readErr error
		models, readErr = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, baseAddr)
		if readErr != nil {
			return nil, fmt.Errorf("read model headers: %w", readErr)
		}
	}

	return &types.SunSpecModelsOutput{
		Target: modbusURL,
		Base:   baseAddr,
		Models: sunSpecModelHeaders(models),
	}, nil
}

// CollectSunSpecMap builds the SunSpec address map view.
func CollectSunSpecMap(cfg config.SunSpecMapConfig) (*types.SunSpecMapOutput, error) {
	modbusURL := config.SunSpecModbusURL(&cfg.SunSpecBaseConfig)
	if modbusURL == "" {
		return nil, fmt.Errorf("either --url or --ip must be set")
	}

	opts := sunSpecOptionsFromBase(&cfg.SunSpecBaseConfig, nil)

	conf := buildClientConfig(modbusURL, 10*time.Second, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = mc.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var baseAddr uint16
	var models []sunspec.ModelHeader

	if cfg.Base != 0 {
		baseAddr = cfg.Base
		var readErr error
		models, readErr = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, baseAddr)
		if readErr != nil {
			return nil, fmt.Errorf("read model headers: %w", readErr)
		}
	} else {
		det, derr := sunspec.Detect(ctx, &readerAdapter{mc}, opts)
		if derr != nil {
			return nil, fmt.Errorf("detect: %w", derr)
		}
		if !det.Detected {
			return &types.SunSpecMapOutput{Target: modbusURL, NotDetected: true}, nil
		}
		baseAddr = det.BaseAddress
		var readErr error
		models, readErr = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, baseAddr)
		if readErr != nil {
			return nil, fmt.Errorf("read model headers: %w", readErr)
		}
	}

	return &types.SunSpecMapOutput{
		Target:         modbusURL,
		Base:           baseAddr,
		MarkerRegs:     fmt.Sprintf("%d-%d", baseAddr, baseAddr+1),
		Models:         sunSpecModelHeaders(models),
		ShowHeaderRegs: cfg.ShowHeaderRegs,
		ShowNext:       cfg.ShowNext,
		Compact:        cfg.Compact,
	}, nil
}

// CollectSunSpecProbe runs Modbus FC probes and SunSpec summary.
func CollectSunSpecProbe(cfg config.SunSpecProbeConfig) (*types.SunSpecProbeOutput, error) {
	modbusURL := config.SunSpecModbusURL(&cfg.SunSpecBaseConfig)
	if modbusURL == "" {
		return nil, fmt.Errorf("either --url or --ip must be set")
	}

	conf := buildClientConfig(modbusURL, 10*time.Second, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = mc.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	out := &types.SunSpecProbeOutput{
		Target: modbusURL,
		UnitID: cfg.Unit,
	}

	for _, pair := range []struct {
		fc modbus.FunctionCode
	}{
		{modbus.FCReadHoldingRegisters},
		{modbus.FCReadInputRegisters},
		{modbus.FCEncapsulatedInterface},
	} {
		ok, _ := mc.SupportsFunction(ctx, cfg.Unit, pair.fc)
		switch pair.fc {
		case modbus.FCReadHoldingRegisters:
			out.Modbus.FC03 = ok
		case modbus.FCReadInputRegisters:
			out.Modbus.FC04 = ok
		case modbus.FCEncapsulatedInterface:
			out.Modbus.FC43 = ok
		}
	}

	opts := sunSpecOptionsFromBase(&cfg.SunSpecBaseConfig, nil)
	det, err := sunspec.Detect(ctx, &readerAdapter{mc}, opts)
	if err != nil {
		det = &sunspec.DetectionResult{UnitID: cfg.Unit}
	}

	modelsFound := 0
	endModel := false
	if det != nil && det.Detected {
		out.SunSpecDetail.Detected = true
		out.SunSpecDetail.BaseAddress = det.BaseAddress
		models, rerr := sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, det.BaseAddress)
		if rerr == nil {
			modelsFound = len(models)
			for _, m := range models {
				if m.IsEndModel {
					endModel = true
					break
				}
			}
		}
	}
	out.SunSpecDetail.ModelsFound = modelsFound
	out.SunSpecDetail.EndModel = endModel

	return out, nil
}
