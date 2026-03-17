package modbus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus/sunspec"
	"github.com/otfabric/modbusctl/internal/config"
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

// SunSpecDetect runs DetectSunSpec and prints the result (table or JSON).
func SunSpecDetect(cfg config.SunSpecDetectConfig) error {
	modbusURL := config.SunSpecModbusURL(&cfg.SunSpecBaseConfig)
	if modbusURL == "" {
		return fmt.Errorf("either --url or --ip must be set")
	}

	var bases []uint16
	if cfg.Bases != "" {
		var err error
		bases, err = config.ParseSunSpecBases(cfg.Bases)
		if err != nil {
			return err
		}
	}

	opts := sunSpecOptionsFromBase(&cfg.SunSpecBaseConfig, bases)

	conf := buildClientConfig(modbusURL, 10*time.Second, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = mc.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := sunspec.Detect(ctx, &readerAdapter{mc}, opts)
	if err != nil {
		return fmt.Errorf("detect failed: %w", err)
	}

	if cfg.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}

	// Human-readable table
	retypeStr := "holding"
	if cfg.Regtype == "input" {
		retypeStr = "input"
	}
	detected := "no"
	if res.Detected {
		detected = "yes"
	}
	fmt.Printf("UNIT  DETECTED  BASE   REGTYPE\n")
	fmt.Printf("%-5d %-9s %-6d %s\n", res.UnitID, detected, res.BaseAddress, retypeStr)

	if cfg.Verbose && len(res.Attempts) > 0 {
		fmt.Println()
		fmt.Printf("ATTEMPT   ADDRESS  RESULT\n")
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
			fmt.Printf("%-8d %-8d %s\n", i+1, a.BaseAddress, result)
		}
	}
	return nil
}

// SunSpecModels enumerates SunSpec model headers and prints them (table or JSON).
func SunSpecModels(cfg config.SunSpecModelsConfig) error {
	modbusURL := config.SunSpecModbusURL(&cfg.SunSpecBaseConfig)
	if modbusURL == "" {
		return fmt.Errorf("either --url or --ip must be set")
	}

	opts := sunSpecOptionsFromBase(&cfg.SunSpecBaseConfig, nil)
	opts.MaxModels = cfg.MaxModels
	if opts.MaxModels <= 0 {
		opts.MaxModels = 256
	}
	opts.MaxAddressSpan = cfg.MaxAddressSpan

	conf := buildClientConfig(modbusURL, 10*time.Second, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = mc.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var baseAddr uint16
	var models []sunspec.ModelHeader

	if cfg.Base != 0 {
		baseAddr = cfg.Base
		models, err = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, baseAddr)
		if err != nil {
			return fmt.Errorf("read model headers: %w", err)
		}
	} else {
		det, err := sunspec.Detect(ctx, &readerAdapter{mc}, opts)
		if err != nil {
			return fmt.Errorf("detect: %w", err)
		}
		if !det.Detected {
			if cfg.JSON {
				return json.NewEncoder(os.Stdout).Encode(struct {
					Base   uint16                `json:"base"`
					Models []sunspec.ModelHeader `json:"models"`
				}{Models: nil})
			}
			fmt.Println("SunSpec not detected.")
			return nil
		}
		baseAddr = det.BaseAddress
		models, err = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, baseAddr)
		if err != nil {
			return fmt.Errorf("read model headers: %w", err)
		}
	}

	if cfg.JSON {
		out := struct {
			Base   uint16                `json:"base"`
			Models []sunspec.ModelHeader `json:"models"`
		}{Base: baseAddr, Models: models}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("BASE: %d\n\n", baseAddr)
	fmt.Printf("START   END     MODEL ID  LENGTH  END MODEL\n")
	for _, m := range models {
		endYes := "no"
		if m.IsEndModel {
			endYes = "yes"
		}
		fmt.Printf("%-7d %-7d %-9d %-7d %s\n", m.StartAddress, m.EndAddress, m.ID, m.Length, endYes)
	}
	return nil
}

// SunSpecMap prints the SunSpec address map (human-friendly layout view).
func SunSpecMap(cfg config.SunSpecMapConfig) error {
	modbusURL := config.SunSpecModbusURL(&cfg.SunSpecBaseConfig)
	if modbusURL == "" {
		return fmt.Errorf("either --url or --ip must be set")
	}

	opts := sunSpecOptionsFromBase(&cfg.SunSpecBaseConfig, nil)

	conf := buildClientConfig(modbusURL, 10*time.Second, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = mc.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var baseAddr uint16
	var models []sunspec.ModelHeader

	if cfg.Base != 0 {
		baseAddr = cfg.Base
		models, err = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, baseAddr)
		if err != nil {
			return fmt.Errorf("read model headers: %w", err)
		}
	} else {
		det, err := sunspec.Detect(ctx, &readerAdapter{mc}, opts)
		if err != nil {
			return fmt.Errorf("detect: %w", err)
		}
		if !det.Detected {
			fmt.Println("SunSpec not detected.")
			return nil
		}
		baseAddr = det.BaseAddress
		models, err = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, baseAddr)
		if err != nil {
			return fmt.Errorf("read model headers: %w", err)
		}
	}

	if cfg.JSON {
		out := struct {
			Base       uint16                `json:"base"`
			MarkerRegs string                `json:"marker_regs"`
			Models     []sunspec.ModelHeader `json:"models"`
		}{
			Base:       baseAddr,
			MarkerRegs: fmt.Sprintf("%d-%d", baseAddr, baseAddr+1),
			Models:     models,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Println("SunSpec map detected")
	fmt.Printf("  Base marker : %d\n", baseAddr)
	fmt.Printf("  Marker regs : %d-%d\n", baseAddr, baseAddr+1)
	fmt.Println()
	fmt.Println("MODEL MAP")
	for _, m := range models {
		if m.IsEndModel {
			fmt.Printf("  %d-%d   end\n", m.StartAddress, m.EndAddress)
			continue
		}
		switch {
		case cfg.ShowHeaderRegs && cfg.ShowNext:
			fmt.Printf("  %d-%d   model %d  hdr %d-%d (next %d)\n", m.StartAddress, m.EndAddress, m.ID, m.StartAddress, m.StartAddress+1, m.NextAddress)
		case cfg.ShowHeaderRegs:
			fmt.Printf("  %d-%d   model %d  hdr %d-%d\n", m.StartAddress, m.EndAddress, m.ID, m.StartAddress, m.StartAddress+1)
		case cfg.ShowNext:
			fmt.Printf("  %d-%d   model %d (next %d)\n", m.StartAddress, m.EndAddress, m.ID, m.NextAddress)
		default:
			fmt.Printf("  %d-%d   model %d\n", m.StartAddress, m.EndAddress, m.ID)
		}
	}
	return nil
}

// SunSpecProbe runs a combined fingerprint (supported FCs) and SunSpec detection summary.
func SunSpecProbe(cfg config.SunSpecProbeConfig) error {
	modbusURL := config.SunSpecModbusURL(&cfg.SunSpecBaseConfig)
	if modbusURL == "" {
		return fmt.Errorf("either --url or --ip must be set")
	}

	conf := buildClientConfig(modbusURL, 10*time.Second, false)
	if err := modbus.ValidateConfig(conf); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	mc, err := modbus.New(conf)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = mc.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Modbus FC probe (FC03, FC04, FC43)
	type fcStatus struct {
		label string
		ok    bool
	}
	var fcResults []fcStatus
	for _, pair := range []struct {
		label string
		fc    modbus.FunctionCode
	}{
		{"FC03", modbus.FCReadHoldingRegisters},
		{"FC04", modbus.FCReadInputRegisters},
		{"FC43", modbus.FCEncapsulatedInterface},
	} {
		ok, _ := mc.SupportsFunction(ctx, cfg.Unit, pair.fc)
		fcResults = append(fcResults, fcStatus{label: pair.label, ok: ok})
	}

	// SunSpec detection
	opts := sunSpecOptionsFromBase(&cfg.SunSpecBaseConfig, nil)
	det, err := sunspec.Detect(ctx, &readerAdapter{mc}, opts)
	if err != nil {
		det = &sunspec.DetectionResult{UnitID: cfg.Unit}
	}

	var models []sunspec.ModelHeader
	modelsFound := 0
	endModel := false
	if det != nil && det.Detected {
		models, err = sunspec.ReadModelHeaders(ctx, &readerAdapter{mc}, opts, det.BaseAddress)
		if err == nil {
			modelsFound = len(models)
			for _, m := range models {
				if m.IsEndModel {
					endModel = true
					break
				}
			}
		}
	}

	if cfg.JSON {
		sunspecDetected := det != nil && det.Detected
		baseAddr := uint16(0)
		if sunspecDetected {
			baseAddr = det.BaseAddress
		}
		modbusMap := make(map[string]bool)
		for _, r := range fcResults {
			modbusMap[r.label] = r.ok
		}
		out := map[string]interface{}{
			"target": map[string]interface{}{
				"url":  modbusURL,
				"unit": cfg.Unit,
			},
			"modbus": modbusMap,
			"sunspec": map[string]interface{}{
				"detected":     sunspecDetected,
				"base_address": baseAddr,
				"models_found": modelsFound,
				"end_model":    endModel,
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Println("TARGET")
	fmt.Printf("  URL    : %s\n", modbusURL)
	fmt.Printf("  UNIT   : %d\n", cfg.Unit)
	fmt.Println()
	fmt.Println("MODBUS")
	for _, r := range fcResults {
		supported := "supported"
		if !r.ok {
			supported = "not supported"
		}
		fmt.Printf("  %-6s : %s\n", r.label, supported)
	}
	fmt.Println()
	fmt.Println("SUNSPEC")
	detectedStr := "no"
	if det != nil && det.Detected {
		detectedStr = "yes"
	}
	fmt.Printf("  detected     : %s\n", detectedStr)
	if det != nil && det.Detected {
		fmt.Printf("  base address : %d\n", det.BaseAddress)
		fmt.Printf("  models found : %d\n", modelsFound)
		fmt.Printf("  end model    : %v\n", endModel)
	}
	_ = models
	return nil
}
