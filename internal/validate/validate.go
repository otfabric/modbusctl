package validate

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/types"
)

const (
	maxModbusRegister  = 65535
	maxModbusBlockSize = 125
	maxModbusUnitID    = 255
)

func CheckIdentifyConfig(cfg config.IdentifyConfig) error {
	if err := validateUnitClientConfig(cfg.UnitClientConfig); err != nil {
		return err
	}
	if cfg.Timeout == 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	return nil
}

// validateUnitClientConfig validates IP, Port, and UnitID ("all" or 1-255).
func validateUnitClientConfig(cfg config.UnitClientConfig) error {
	if !isValidIPv4(cfg.IP) {
		return fmt.Errorf("invalid IP address: %s", cfg.IP)
	}
	if cfg.Port == 0 {
		return fmt.Errorf("invalid port: %d", cfg.Port)
	}
	if cfg.UnitID == "" {
		return fmt.Errorf("unit ID must be 1-255 or \"all\"")
	}
	u := strings.TrimSpace(strings.ToLower(cfg.UnitID))
	if u == "all" {
		if cfg.Parallel < 1 || cfg.Parallel > 64 {
			return fmt.Errorf("parallel must be between 1 and 64 when using --unit all, got %d", cfg.Parallel)
		}
		return nil
	}
	if _, err := modbus.ParseUnitIDs(cfg.UnitID); err != nil {
		return err
	}
	return nil
}

func CheckReadConfig(cfg config.ReadConfig) error {
	if err := validateDeviceConfig(cfg.DeviceConfig); err != nil {
		return err
	}
	if err := validateFunctionCode(cfg.Function); err != nil {
		return err
	}
	if cfg.RegisterCount == 0 || cfg.RegisterCount > maxModbusBlockSize {
		return fmt.Errorf("register count must be between 1 and %d", maxModbusBlockSize)
	}
	if err := validateAddressRange(cfg.StartAddress, cfg.StartAddress+cfg.RegisterCount); err != nil {
		return err
	}
	return validateFile(cfg.OutputFile, false)
}

func CheckScanConfig(cfg config.ScanConfig) error {
	if err := validateDeviceConfig(cfg.DeviceConfig); err != nil {
		return err
	}
	if err := validateFunctionCode(cfg.Function); err != nil {
		return err
	}
	if err := validateAddressRange(cfg.StartAddress, cfg.EndAddress); err != nil {
		return err
	}
	if cfg.Delay > 60000 {
		return fmt.Errorf("delay must be between 0 and 60000 milliseconds (1 minute)")
	}
	if !config.ValidScanAlgo(cfg.Algo) {
		return fmt.Errorf("algo must be one of %v, got %q", config.ScanAlgoValues, cfg.Algo)
	}
	algo := strings.ToLower(strings.TrimSpace(cfg.Algo))
	if algo == "stepped" && cfg.Step < 1 {
		return fmt.Errorf("step must be between 1 and 65535 when using algo stepped, got %d", cfg.Step)
	}
	if algo == "boundary" {
		if cfg.SeedCount < 1 || cfg.SeedCount > 125 {
			return fmt.Errorf("boundary algo requires --seed-count between 1 and 125, got %d", cfg.SeedCount)
		}
		if uint32(cfg.SeedStart)+uint32(cfg.SeedCount)-1 > 65535 {
			return fmt.Errorf("boundary seed range [%d, %d] exceeds max address 65535", cfg.SeedStart, cfg.SeedStart+cfg.SeedCount-1)
		}
		// Full seed must lie inside configured range [StartAddress, EndAddress]
		if cfg.SeedStart < cfg.StartAddress {
			return fmt.Errorf("boundary seed start %d must be >= start address %d", cfg.SeedStart, cfg.StartAddress)
		}
		if uint32(cfg.SeedStart)+uint32(cfg.SeedCount)-1 > uint32(cfg.EndAddress) {
			return fmt.Errorf("boundary seed end %d exceeds end address %d", cfg.SeedStart+cfg.SeedCount-1, cfg.EndAddress)
		}
	}
	return validateFile(cfg.OutputFile, false)
}

func CheckRecordConfig(cfg config.RecordConfig) error {
	if err := validateDeviceConfig(cfg.DeviceConfig); err != nil {
		return err
	}
	if err := validateFunctionCode(cfg.Function); err != nil {
		return err
	}
	if cfg.Duration < cfg.Interval {
		return fmt.Errorf("duration (%d) must be greater than or equal to interval (%d)", cfg.Duration, cfg.Interval)
	}
	if err := validateAddressBlockFile(cfg.InputFile); err != nil {
		return err
	}
	return validateFile(cfg.OutputFile, false)
}

func CheckConvertConfig(cfg config.ConvertConfig) error {
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if err := validateFormatType(cfg.FormatType); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OutputFile) != "" {
		return validateFile(cfg.OutputFile, false)
	}
	return nil
}

func CheckDeviceProfileDecodeConfig(cfg config.DeviceProfileDecodeConfig) error {
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if err := validateFile(cfg.OutputFile, false); err != nil {
		return err
	}
	if err := validateDeviceProfileFile(cfg.DeviceProfile); err != nil {
		return err
	}
	return nil
}

func CheckExtractConfig(cfg config.ExtractConfig) error {
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OutputFile) != "" {
		return validateFile(cfg.OutputFile, false)
	}
	return nil
}

func CheckStringsConfig(cfg config.StringsConfig) error {
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OutputFile) != "" {
		return validateFile(cfg.OutputFile, false)
	}
	return nil
}

func CheckInfoConfig(cfg config.InfoConfig) error {
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OutputFile) != "" {
		return validateFile(cfg.OutputFile, false)
	}
	return nil
}

func CheckFingerprintConfig(cfg config.FingerprintConfig) error {
	if !isValidIPv4(cfg.IP) {
		return fmt.Errorf("invalid IP address: %s", cfg.IP)
	}
	if cfg.Port == 0 {
		return fmt.Errorf("invalid port: %d", cfg.Port)
	}
	if cfg.Timeout == 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	if _, err := modbus.ParseUnitIDs(cfg.UnitID); err != nil {
		return err
	}
	return nil
}

func CheckDiagnosticConfig(cfg config.DiagnosticConfig) error {
	if !isValidIPv4(cfg.IP) {
		return fmt.Errorf("invalid IP address: %s", cfg.IP)
	}
	if cfg.Port == 0 {
		return fmt.Errorf("invalid port: %d", cfg.Port)
	}
	if cfg.Timeout == 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	if _, err := config.ParseDiagnosticSubFunction(cfg.SubFunction); err != nil {
		return err
	}
	if cfg.Data != "" {
		if _, err := hex.DecodeString(cfg.Data); err != nil {
			return fmt.Errorf("invalid hex data %q: %w", cfg.Data, err)
		}
	}
	return nil
}

func CheckReportServerIdConfig(cfg config.ReportServerIdConfig) error {
	if err := validateUnitClientConfig(cfg.UnitClientConfig); err != nil {
		return err
	}
	if cfg.Timeout == 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	return nil
}

func CheckDiscoverConfig(cfg config.DiscoverConfig) error {
	if len(cfg.Subnets) == 0 {
		return fmt.Errorf("at least one subnet must be provided")
	}
	for _, subnet := range cfg.Subnets {
		if _, _, err := net.ParseCIDR(subnet); err != nil {
			return fmt.Errorf("invalid subnet '%s': %v", subnet, err)
		}
	}

	if cfg.Port == 0 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	if cfg.ResolveMAC && strings.TrimSpace(cfg.NetworkInterface) == "" {
		return fmt.Errorf("network interface must be provided if ResolveMAC is enabled")
	}

	if cfg.ResolveMAC {
		currentUser, err := user.Current()
		if err != nil {
			return fmt.Errorf("could not determine current user: %w", err)
		}
		if currentUser.Uid != "0" {
			fmt.Printf("⚠️  Warning: Resolving MAC addresses typically requires elevated privileges (e.g., sudo).\n")
		}
	}

	return nil
}

func CheckStaticServerConfig(cfg config.StaticServerConfig) error {
	if cfg.Port == 0 {
		return fmt.Errorf("port must be greater than 0")
	}

	if cfg.Unit > maxModbusUnitID {
		return fmt.Errorf("invalid Modbus unit ID: %d (must be between 0 and %d)", cfg.Unit, maxModbusUnitID)
	}

	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}

	return nil
}
func CheckReplayServerConfig(cfg config.ReplayServerConfig) error {
	if cfg.Port == 0 {
		return fmt.Errorf("port must be greater than 0")
	}

	if cfg.Unit > maxModbusUnitID {
		return fmt.Errorf("invalid Modbus unit ID: %d (must be between 0 and %d)", cfg.Unit, maxModbusUnitID)
	}

	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}

	return nil
}

func isValidIPv4(ip string) bool {
	return net.ParseIP(ip) != nil && strings.Count(ip, ":") < 2
}

func validateDeviceConfig(cfg config.DeviceConfig) error {
	if !isValidIPv4(cfg.IP) {
		return fmt.Errorf("invalid IP address: %s", cfg.IP)
	}
	if cfg.Port == 0 {
		return fmt.Errorf("invalid port: %d", cfg.Port)
	}
	if cfg.Unit > maxModbusUnitID {
		return fmt.Errorf("invalid Modbus unit ID: %d (must be between 0 and %d)", cfg.Unit, maxModbusUnitID)
	}
	return nil
}

func validateFunctionCode(fc uint8) error {
	switch fc {
	case 1, 2, 3, 4:
		return nil
	default:
		return fmt.Errorf("unsupported function code: %d — supported: 1 (Read Coils), 2 (Read Discrete Inputs), 3 (Read Holding Registers), 4 (Read Input Registers)", fc)
	}
}

func validateAddressRange(start, end uint16) error {
	if start > maxModbusRegister {
		return fmt.Errorf("start address %d is out of range", start)
	}
	if end > maxModbusRegister {
		return fmt.Errorf("end address %d is out of range", end)
	}
	if start >= end {
		return fmt.Errorf("start address (%d) must be less than end address (%d)", start, end)
	}
	return nil
}

func validateFormatType(format string) error {
	switch format {
	case "csv", "json":
		return nil
	default:
		return fmt.Errorf("unsupported format: %s (expected csv or json)", format)
	}
}

func validateAddressBlockFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("input file path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("input file not accessible: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("input file cannot be a directory")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}
	var blocks []types.AddressBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return fmt.Errorf("input file does not contain valid address blocks: %w", err)
	}
	if len(blocks) == 0 {
		return fmt.Errorf("input file contains no address blocks")
	}
	for _, b := range blocks {
		if b.RegisterCount == 0 ||
			b.StartAddress > maxModbusRegister ||
			uint32(b.StartAddress)+uint32(b.RegisterCount) > maxModbusRegister+1 {
			return fmt.Errorf("invalid address block: start=%d, count=%d", b.StartAddress, b.RegisterCount)
		}
	}
	return nil
}

func validateDeviceProfileFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("device profile file path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("device profile file not accessible: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("device profile file cannot be a directory")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read device profile file: %w", err)
	}
	var profile types.DeviceProfile
	if err := json.Unmarshal(content, &profile); err != nil {
		return fmt.Errorf("invalid device profile JSON: %w", err)
	}
	if len(profile.ProtocolData.Registers) == 0 {
		return fmt.Errorf("device profile contains no register definitions")
	}
	for _, r := range profile.ProtocolData.Registers {
		if r.ControlledPropertyId == "not.used" {
			continue
		}
		if r.ControlledPropertyId == "" {
			return fmt.Errorf("register has empty controlledPropertyId")
		}
		if r.Size == 0 {
			return fmt.Errorf("register %s has invalid size: must be > 0", r.ControlledPropertyId)
		}
		if r.Start > maxModbusRegister || uint32(r.Start)+uint32(r.Size) > maxModbusRegister+1 {
			return fmt.Errorf("register %s has invalid address range: start=%d, size=%d", r.ControlledPropertyId, r.Start, r.Size)
		}
		if _, ok := types.RegisterDecoders[r.Format]; !ok {
			return fmt.Errorf("register %s uses unsupported format: %s", r.ControlledPropertyId, r.Format)
		}
	}
	return nil
}

func validateFile(path string, mustExist bool) error {
	path = strings.TrimSpace(path)
	if mustExist {
		if path == "" {
			return fmt.Errorf("file path is required")
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("file not accessible: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("path cannot be a directory: %s", path)
		}
		return nil
	}
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	dirInfo, err := os.Stat(dir)
	if err != nil || !dirInfo.IsDir() {
		return fmt.Errorf("output directory not accessible: %w", err)
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return fmt.Errorf("path cannot be a directory: %s", path)
	}
	return nil
}

