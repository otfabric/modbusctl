package validate

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/plan"
	"github.com/otfabric/modbusctl/internal/types"
)

const (
	maxModbusRegister  = 65535
	maxModbusBlockSize = 125
	maxModbusUnitID    = 255
)

// validateOptionalTimeoutMilliseconds is the shared client timeout rule for Modbus client commands:
// 0 selects the default per-request budget (10000 ms) and matching dial cap; otherwise the value must be ≥ 200 ms.
func validateOptionalTimeoutMilliseconds(ms uint16) error {
	if ms != 0 && ms < 200 {
		return fmt.Errorf("timeout must be 0 for default (10000 ms) or at least 200 ms")
	}
	return nil
}

// validateModbusEndpoint applies layered checks: [config.ValidateModbusAddress] (exactly one of URL or IP),
// the serial-line settings, the TLS files of a tcp+tls:// URL, then when --ip is set literal IP
// syntax and a non-zero port.
func validateModbusEndpoint(url, ip string, port uint16, serial config.SerialConfig, tlsCfg config.TLSConfig) error {
	if err := config.ValidateModbusAddress(url, ip); err != nil {
		return err
	}
	if err := config.ValidateSerialConfig(serial); err != nil {
		return err
	}
	if err := config.ValidateTLSConfig(url, tlsCfg); err != nil {
		return err
	}
	if strings.TrimSpace(ip) == "" {
		return nil
	}
	if !isValidHostIP(ip) {
		return fmt.Errorf("invalid IP address: %s", ip)
	}
	if port == 0 {
		return fmt.Errorf("invalid port: %d", port)
	}
	return nil
}

func CheckIdentifyConfig(cfg config.IdentifyConfig) error {
	if err := validateUnitClientConfig(cfg.UnitClientConfig); err != nil {
		return err
	}
	return validateOptionalTimeoutMilliseconds(cfg.Timeout)
}

// validateUnitClientConfig validates URL-or-IP (mutually exclusive), and when IP is set also Port and UnitID ("all" or 1-255).
func validateUnitClientConfig(cfg config.UnitClientConfig) error {
	if err := validateModbusEndpoint(cfg.URL, cfg.IP, cfg.Port, cfg.SerialConfig, cfg.TLSConfig); err != nil {
		return err
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
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	if err := validateFunctionCode(cfg.Function); err != nil {
		return err
	}
	if cfg.RegisterCount == 0 || cfg.RegisterCount > maxModbusBlockSize {
		return fmt.Errorf("register count must be between 1 and %d", maxModbusBlockSize)
	}
	if err := validateExclusiveRegisterWindow(cfg.StartAddress, cfg.RegisterCount); err != nil {
		return err
	}
	return validateFile(cfg.OutputFile, false)
}

func CheckDecodeConfig(cfg config.DecodeConfig) error {
	if err := validateDeviceConfig(cfg.DeviceConfig); err != nil {
		return err
	}
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	if err := validateDecodeFunctionCode(cfg.Function); err != nil {
		return err
	}
	id := strings.TrimSpace(cfg.Codec)
	if id == "" {
		return fmt.Errorf("codec is required (use --codec; run 'modbusctl client codecs' to list available codecs)")
	}
	rc, ok, err := codec.RuntimeCodecByID(id)
	if err != nil {
		return fmt.Errorf("codec %q could not be constructed: %v", id, err)
	}
	if !ok {
		return fmt.Errorf("unknown codec %q (run 'modbusctl client codecs' to list available codecs)", id)
	}
	per := rc.RegisterSpec().Count
	if per == 0 {
		return fmt.Errorf("codec %q has zero register width", id)
	}
	total := cfg.RegisterCount
	if total == 0 {
		total = per
	} else if total%per != 0 {
		return fmt.Errorf("register count %d must be a multiple of the codec register width %d for %q", total, per, id)
	}
	if total > maxModbusBlockSize {
		return fmt.Errorf("register count must be between 1 and %d (codec %q has register width %d)", maxModbusBlockSize, id, per)
	}
	return validateExclusiveRegisterWindow(cfg.StartAddress, total)
}

// validateDecodeFunctionCode restricts decode to register reads (holding/input); codecs operate on registers.
func validateDecodeFunctionCode(fc uint8) error {
	switch fc {
	case 3, 4:
		return nil
	default:
		return fmt.Errorf("unsupported function code for decode: %d — supported: 3 (Read Holding Registers), 4 (Read Input Registers)", fc)
	}
}

func CheckScanConfig(cfg *config.ScanConfig) error {
	if err := validateDeviceConfig(cfg.DeviceConfig); err != nil {
		return err
	}
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	if err := validateFunctionCode(cfg.Function); err != nil {
		return err
	}
	if cfg.Delay > 60000 {
		return fmt.Errorf("delay must be between 0 and 60000 milliseconds (1 minute)")
	}
	if !config.ValidScanAlgo(cfg.Algo) {
		return fmt.Errorf("algo must be one of %v, got %q", config.ScanAlgorithms(), cfg.Algo)
	}
	algo := strings.ToLower(strings.TrimSpace(cfg.Algo))
	if algo == "" {
		algo = "safe"
	}
	cfg.NormalizedAlgo = config.ScanAlgorithm(algo)
	if algo == "sunspec" {
		// SunSpec algo does not use start/end range — skip address validation.
		if cfg.SunSpecBase > 0 && uint32(cfg.SunSpecBase)+2 > 65535 {
			return fmt.Errorf("sunspec base %d + 2 would overflow", cfg.SunSpecBase)
		}
		if strings.TrimSpace(cfg.SunSpecBases) != "" {
			if _, err := config.ParseSunSpecBases(cfg.SunSpecBases); err != nil {
				return fmt.Errorf("sunspec bases: %w", err)
			}
		}
		return validateFile(cfg.OutputFile, false)
	}
	if err := validateInclusiveScanRange(cfg.StartAddress, cfg.EndAddress); err != nil {
		return err
	}
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

// CheckWriteConfig validates the endpoint and the write itself (values, codec, function code,
// quantity limits and address range) without touching the device.
func CheckWriteConfig(cfg config.WriteConfig) error {
	if err := validateDeviceConfig(cfg.DeviceConfig); err != nil {
		return err
	}
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	_, err := modbus.PlanWrite(cfg)
	return err
}

// maxPollBits is the Modbus limit for one FC1/FC2 read.
const maxPollBits = 2000

func CheckPollConfig(cfg config.PollConfig) error {
	if err := validateDeviceConfig(cfg.DeviceConfig); err != nil {
		return err
	}
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	if err := validateFunctionCode(cfg.Function); err != nil {
		return err
	}
	isBits := cfg.Function == 1 || cfg.Function == 2
	if isBits && strings.TrimSpace(cfg.Codec) != "" {
		return fmt.Errorf("--codec decodes registers and cannot be used with function code %d (use 3 or 4)", cfg.Function)
	}
	_, count, err := modbus.ResolvePollCodec(cfg)
	if err != nil {
		return err
	}
	limit := uint16(maxModbusBlockSize)
	if isBits {
		limit = maxPollBits
	}
	if count == 0 || count > limit {
		return fmt.Errorf("count must be between 1 and %d for function code %d", limit, cfg.Function)
	}
	if cfg.Interval == 0 {
		return fmt.Errorf("interval must be at least 1 millisecond")
	}
	return validateExclusiveRegisterWindow(cfg.StartAddress, count)
}

func CheckRecordConfig(cfg config.RecordConfig) error {
	if err := validateDeviceConfig(cfg.DeviceConfig); err != nil {
		return err
	}
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	if err := validateFunctionCode(cfg.Function); err != nil {
		return err
	}
	if cfg.Duration < cfg.Interval {
		return fmt.Errorf("duration (%d) must be greater than or equal to interval (%d)", cfg.Duration, cfg.Interval)
	}
	if err := validateAddressBlockFile(cfg.BlocksFile, cfg.Function); err != nil {
		return err
	}
	return validateFile(cfg.OutputFile, false)
}

func CheckDeviceProfileDecodeConfig(cfg config.DeviceProfileDecodeConfig) error {
	if err := checkSelection(cfg.CaptureSelection); err != nil {
		return err
	}
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
	if err := checkSelection(cfg.CaptureSelection); err != nil {
		return err
	}
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OutputFile) != "" {
		return validateFile(cfg.OutputFile, false)
	}
	return nil
}

func CheckStringsConfig(cfg config.StringsConfig) error {
	if err := checkSelection(cfg.CaptureSelection); err != nil {
		return err
	}
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OutputFile) != "" {
		return validateFile(cfg.OutputFile, false)
	}
	return nil
}

func checkSelection(sel config.CaptureSelection) error {
	if sel.Function > 4 {
		return fmt.Errorf("invalid --function %d: use 1-4 (0 = the only one in the capture)", sel.Function)
	}
	return nil
}

func CheckFingerprintConfig(cfg config.FingerprintConfig) error {
	if err := validateModbusEndpoint(cfg.URL, cfg.IP, cfg.Port, cfg.SerialConfig, cfg.TLSConfig); err != nil {
		return err
	}
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	if _, err := modbus.ParseUnitIDs(cfg.UnitID); err != nil {
		return err
	}
	return nil
}

// CheckConfirmed refuses what changes a device without --yes. what names it in the message.
func CheckConfirmed(yes bool, what string) error {
	if yes {
		return nil
	}
	return errs.New(errs.KindUsage, errs.CodeConfirmationRequired,
		what+" changes the state of the device: confirm with --yes (nothing was sent)", nil)
}

// CheckDiagnosticConfirmed is [CheckConfirmed] for the FC08 sub-functions that change the
// state of the device. An unknown sub-function is left to [CheckDiagnosticConfig].
func CheckDiagnosticConfirmed(cfg config.DiagnosticConfig) error {
	if code, err := config.ParseDiagnosticSubFunction(cfg.SubFunction); err == nil && config.DiagnosticChangesState(code) {
		return CheckConfirmed(cfg.Yes, "client diagnostic --sub-function "+cfg.SubFunction)
	}
	return nil
}

func CheckDiagnosticConfig(cfg config.DiagnosticConfig) error {
	if err := validateModbusEndpoint(cfg.URL, cfg.IP, cfg.Port, cfg.SerialConfig, cfg.TLSConfig); err != nil {
		return err
	}
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	subCode, err := config.ParseDiagnosticSubFunction(cfg.SubFunction)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Data) != "" {
		clean := strings.ReplaceAll(strings.TrimSpace(cfg.Data), " ", "")
		if len(clean)%2 != 0 {
			return fmt.Errorf("diagnostic --data must be an even number of hex digits")
		}
		raw, derr := hex.DecodeString(clean)
		if derr != nil {
			return fmt.Errorf("invalid hex data: %w", derr)
		}
		if err := validateDiagnosticFC08Payload(subCode, raw); err != nil {
			return err
		}
	}
	return nil
}

// validateDiagnosticFC08Payload applies minimal size rules after hex decode.
// Return Query Data (0) may carry longer echo payloads; other sub-functions use at most 2 data bytes on the wire.
func validateDiagnosticFC08Payload(subCode uint16, data []byte) error {
	const maxFC08Data = 250
	if len(data) > maxFC08Data {
		return fmt.Errorf("diagnostic data exceeds %d bytes", maxFC08Data)
	}
	if subCode != 0 && len(data) > 2 {
		return fmt.Errorf("for this diagnostic sub-function, data must be at most 2 bytes, got %d", len(data))
	}
	return nil
}

func CheckReportServerIDConfig(cfg config.ReportServerIDConfig) error {
	if err := validateUnitClientConfig(cfg.UnitClientConfig); err != nil {
		return err
	}
	return validateOptionalTimeoutMilliseconds(cfg.Timeout)
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

	if cfg.Parallel < 1 || cfg.Parallel > 64 {
		return fmt.Errorf("parallel must be between 1 and 64, got %d", cfg.Parallel)
	}

	if cfg.ResolveMAC && strings.TrimSpace(cfg.NetworkInterface) == "" {
		return fmt.Errorf("network interface must be provided if ResolveMAC is enabled")
	}

	if !cfg.ForceLargeScan {
		n, err := modbus.EstimateDiscoverHostCount(cfg)
		if err != nil {
			return fmt.Errorf("estimate discovery scope: %w", err)
		}
		const maxDiscoverHosts = 65536
		if n > maxDiscoverHosts {
			return fmt.Errorf("discovery would probe %d unique hosts (exceeds cap %d); use --force-large-scan to override", n, maxDiscoverHosts)
		}
	}

	return nil
}

// checkListen accepts an empty listen address (every interface), an IP
// address or a host name, without a port.
func checkListen(host string) error {
	if host == "" || net.ParseIP(host) != nil {
		return nil
	}
	for _, r := range host {
		hostChar := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.'
		if !hostChar {
			return fmt.Errorf("invalid --listen %q: use an IP address or host name without a port, e.g. 127.0.0.1", host)
		}
	}
	return nil
}

func CheckStaticServerConfig(cfg config.StaticServerConfig) error {
	if cfg.Port == 0 {
		return fmt.Errorf("port must be greater than 0")
	}
	if err := checkListen(cfg.Listen); err != nil {
		return err
	}
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if err := checkMissing(cfg.Missing); err != nil {
		return err
	}
	if err := checkOffset("at", cfg.At); err != nil {
		return err
	}
	_, _, err := cfg.Load()
	return err
}

func CheckReplayServerConfig(cfg config.ReplayServerConfig) error {
	if cfg.Port == 0 {
		return fmt.Errorf("port must be greater than 0")
	}
	if err := checkListen(cfg.Listen); err != nil {
		return err
	}
	if err := validateFile(cfg.InputFile, true); err != nil {
		return err
	}
	if err := checkMissing(cfg.Missing); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(cfg.BeforeFirst)) {
	case "", "hold", "absent":
	default:
		return fmt.Errorf("invalid --before-first %q: use hold or absent", cfg.BeforeFirst)
	}
	if s := strings.TrimSpace(cfg.Speed); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err != nil || v <= 0 {
			return fmt.Errorf("invalid --speed %q: use a positive factor such as 1, 0.1 or 60", cfg.Speed)
		}
	}
	if err := checkOffset("from", cfg.From); err != nil {
		return err
	}
	_, _, err := cfg.Load()
	return err
}

func checkMissing(s string) error {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "exception", "zero", "silence":
		return nil
	}
	return fmt.Errorf("invalid --missing %q: use exception, zero or silence", s)
}

func checkOffset(flag, s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if d, err := time.ParseDuration(strings.TrimSpace(s)); err != nil || d < 0 {
		return fmt.Errorf("invalid --%s %q: use a duration since the capture start, such as 90s or 1m30s", flag, s)
	}
	return nil
}

// isValidHostIP reports whether s is a literal IPv4 or IPv6 address (not a hostname).
func isValidHostIP(s string) bool {
	return net.ParseIP(strings.TrimSpace(s)) != nil
}

func validateDeviceConfig(cfg config.DeviceConfig) error {
	if err := validateModbusEndpoint(cfg.URL, cfg.IP, cfg.Port, cfg.SerialConfig, cfg.TLSConfig); err != nil {
		return err
	}
	// uint8 cannot exceed 255; reject 0 (broadcast / invalid for directed client requests).
	if cfg.Unit < 1 || cfg.Unit > maxModbusUnitID {
		return fmt.Errorf("invalid Modbus unit ID: %d (must be between 1 and %d)", cfg.Unit, maxModbusUnitID)
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

// validateExclusiveRegisterWindow ensures [start, start+count) lies in valid Modbus address space using uint32 math (avoids uint16 wrap on start+count).
func validateExclusiveRegisterWindow(start, count uint16) error {
	if start > maxModbusRegister {
		return fmt.Errorf("start address %d is out of range", start)
	}
	sum := uint32(start) + uint32(count)
	if sum > uint32(maxModbusRegister)+1 {
		return fmt.Errorf("start address %d with count %d exceeds maximum register space", start, count)
	}
	return nil
}

// validateInclusiveScanRange requires start ≤ end ≤ maxModbusRegister and start < end (scan end is inclusive).
func validateInclusiveScanRange(start, end uint16) error {
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

// CheckSunSpecDetectConfig validates SunSpec detect config (address, unit, regtype, bases).
func CheckSunSpecDetectConfig(cfg config.SunSpecDetectConfig) error {
	if err := checkSunSpecBaseConfig(&cfg.SunSpecBaseConfig); err != nil {
		return err
	}
	if cfg.Bases != "" {
		parsed, err := config.ParseSunSpecBases(cfg.Bases)
		if err != nil {
			return err
		}
		if len(parsed) == 0 {
			return fmt.Errorf("at least one base address required when --bases is set")
		}
	}
	return nil
}

// CheckSunSpecModelsConfig validates SunSpec models config.
func CheckSunSpecModelsConfig(cfg config.SunSpecModelsConfig) error {
	if err := checkSunSpecBaseConfig(&cfg.SunSpecBaseConfig); err != nil {
		return err
	}
	if cfg.MaxModels < 0 {
		return fmt.Errorf("max-models must be >= 0")
	}
	return nil
}

// CheckSunSpecMapConfig validates SunSpec map config.
func CheckSunSpecMapConfig(cfg config.SunSpecMapConfig) error {
	return checkSunSpecBaseConfig(&cfg.SunSpecBaseConfig)
}

// CheckSunSpecProbeConfig validates SunSpec probe config.
func CheckSunSpecProbeConfig(cfg config.SunSpecProbeConfig) error {
	return checkSunSpecBaseConfig(&cfg.SunSpecBaseConfig)
}

func checkSunSpecBaseConfig(cfg *config.SunSpecBaseConfig) error {
	if err := validateModbusEndpoint(cfg.URL, cfg.IP, cfg.Port, cfg.SerialConfig, cfg.TLSConfig); err != nil {
		return err
	}
	if err := validateOptionalTimeoutMilliseconds(cfg.Timeout); err != nil {
		return err
	}
	// Unit ID: allow full Modbus range 0-255 (e.g. SunSpec over gateway)
	rt := strings.ToLower(strings.TrimSpace(cfg.Regtype))
	if rt != "" && rt != "holding" && rt != "input" {
		return fmt.Errorf("regtype must be holding or input, got %q", cfg.Regtype)
	}
	return nil
}

// validateAddressBlockFile checks the recording plan at path: address blocks
// read with defaultFC, or requests of any supported read function (see
// package plan).
func validateAddressBlockFile(path string, defaultFC uint8) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("blocks file path is required (--blocks-file / MODBUSCTL_BLOCKS_FILE)")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("blocks file not accessible: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("blocks file cannot be a directory")
	}
	_, err = plan.Read(path, defaultFC)
	return err
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
	// Directory-style output: auto-named capture file under this directory (trailing / or \).
	trimTrail := strings.TrimRight(path, `/\`)
	if trimTrail != path {
		dir := filepath.Clean(trimTrail)
		if dir == "" || dir == "." {
			dir = "."
		}
		return validateOutputDir(dir)
	}
	dir := filepath.Dir(path)
	if err := validateOutputDir(dir); err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return fmt.Errorf("path cannot be a directory: %s", path)
	}
	return nil
}

// validateOutputDir requires dir to exist and to be a directory.
func validateOutputDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("output directory not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output directory is not a directory: %s", dir)
	}
	return nil
}
