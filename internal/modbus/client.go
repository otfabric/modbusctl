package modbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/types"
)

const MaxBlockSize = 125

// Modbus standard valid slave/unit IDs when using "all".
const modbusUnitIDMin, modbusUnitIDMax = 1, 255

// defaultDialTimeout is used when building client config (TCP dial; TLS would use a longer value).
const defaultDialTimeout = 5 * time.Second

var (
	ErrTCPConnection    = errors.New("TCP connection error")
	ErrFC43Timeout      = errors.New("FC43 timeout")
	ErrFC43NotSupported = errors.New("FC43 not supported or invalid response")
)

// buildClientConfig returns a modbus.Config with URL, Timeout, DialTimeout, and Logger set.
// When debug is true, Logger is modbus.NewStdLogger(nil); otherwise modbus.NopLogger().
func buildClientConfig(modbusURL string, timeout time.Duration, debug bool) modbus.Config {
	logger := modbus.NopLogger()
	if debug {
		logger = modbus.NewStdLogger(nil)
	}
	return modbus.Config{
		URL:         modbusURL,
		Timeout:     timeout,
		DialTimeout: defaultDialTimeout,
		Logger:      logger,
	}
}

// validateAndConnect validates conf, creates a client, and opens the connection.
func validateAndConnect(conf modbus.Config) (*modbus.Client, func(), error) {
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, nil, err
	}
	client, err := modbus.New(conf)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Open(); err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = client.Close()
	}
	return client, cleanup, nil
}

func connect(modbusURL string, debug bool) (*modbus.Client, func(), error) {
	conf := buildClientConfig(modbusURL, 10*time.Second, debug)
	return validateAndConnect(conf)
}

// classifyOutcome returns outcome type and Modbus exception code (0 when not an exception).
// reqTS and resTS are passed through from callers for ScanResult; they are not used for classification.
func classifyOutcome(err error, _, _ int64) (ScanOutcomeType, uint8) {
	if err == nil {
		return ScanOutcomeSuccess, 0
	}
	var excErr *modbus.ExceptionError
	if errors.As(err, &excErr) {
		return ScanOutcomeException, uint8(excErr.ExceptionCode)
	}
	if errors.Is(err, modbus.ErrRequestTimedOut) {
		return ScanOutcomeTimeout, 0
	}
	if errors.Is(err, modbus.ErrProtocolError) || errors.Is(err, modbus.ErrBadUnitID) ||
		errors.Is(err, modbus.ErrBadTransactionID) || errors.Is(err, modbus.ErrUnknownProtocolID) {
		return ScanOutcomeProtocol, 0
	}
	if shouldReconnect(err) {
		return ScanOutcomeTransport, 0
	}
	return ScanOutcomeUnknown, 0
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, modbus.ErrServerDeviceBusy) || errors.Is(err, modbus.ErrGWTargetFailedToRespond)
}

func shouldReconnect(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "EOF") || strings.Contains(msg, "connection reset")
}

func performRead(client *modbus.Client, unitID uint8, fc uint8, start, count uint16) ([]byte, error) {
	ctx := context.Background()
	switch fc {
	case 1:
		bools, err := client.ReadCoils(ctx, unitID, start, count)
		if err != nil {
			return nil, err
		}
		return packCoilsToBytes(bools), nil
	case 2:
		bools, err := client.ReadDiscreteInputs(ctx, unitID, start, count)
		if err != nil {
			return nil, err
		}
		return packCoilsToBytes(bools), nil
	case 3:
		return client.ReadRegisterBytes(ctx, unitID, start, count*2, modbus.HoldingRegister)
	case 4:
		return client.ReadRegisterBytes(ctx, unitID, start, count*2, modbus.InputRegister)
	default:
		return nil, fmt.Errorf("unsupported function code: %d", fc)
	}
}

// packCoilsToBytes packs []bool (1 bit per coil, LSB first) into bytes.
func packCoilsToBytes(bools []bool) []byte {
	n := (len(bools) + 7) / 8
	out := make([]byte, n)
	for i, b := range bools {
		if b {
			out[i/8] |= 1 << (i % 8)
		}
	}
	return out
}

func readRegisters(clientPtr **modbus.Client, fc uint8, start, count uint16, retries uint8, modbusURL string, unit uint8, cleanup *func(), delay uint16, debug bool) (data []byte, requestTS int64, responseTS int64, err error) {
	for attempt := 1; attempt <= int(retries); attempt++ {
		if delay > 0 {
			if debug {
				fmt.Fprintf(os.Stderr, "⏳ Waiting %d ms before retrying...\n", delay)
			}
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		requestTS = time.Now().UnixNano()
		data, err = performRead(*clientPtr, unit, fc, start, count)
		responseTS = time.Now().UnixNano()

		if err == nil {
			return data, requestTS, responseTS, nil
		}

		if shouldRetry(err) {
			fmt.Fprintf(os.Stderr, "🔁 Retrying due to Modbus read exception on address %d with count %d (attempt %d): %v\n", start, count, attempt, err)
			if delay == 0 {
				time.Sleep(20 * time.Millisecond)
			}
			continue
		}

		if shouldReconnect(err) {
			fmt.Fprintf(os.Stderr, "🔁 Reconnecting due to connection error (attempt %d)...\n", attempt)
			if cleanup != nil {
				(*cleanup)()
			}
			var newClient *modbus.Client
			var newCleanup func()
			newClient, newCleanup, err = connect(modbusURL, debug)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("reconnect failed: %w", err)
			}
			*clientPtr = newClient
			*cleanup = newCleanup
			continue
		}

		return nil, requestTS, responseTS, err
	}
	return nil, requestTS, responseTS, err
}

// executeReadTask runs a single read and returns a ScanResult.
func executeReadTask(clientPtr **modbus.Client, cfg config.ScanConfig, task ScanTask, cleanup *func(), modbusURL string) ScanResult {
	data, reqTS, resTS, err := readRegisters(clientPtr, cfg.Function, task.Start, task.Count, 1, modbusURL, cfg.Unit, cleanup, cfg.Delay, cfg.Debug)
	outcome, excCode := classifyOutcome(err, reqTS, resTS)
	rtt := int64(0)
	if resTS > reqTS {
		rtt = resTS - reqTS
	}
	return ScanResult{
		Success:           err == nil,
		Start:             task.Start,
		Count:             task.Count,
		Data:              data,
		RequestTimestamp:  reqTS,
		ResponseTimestamp: resTS,
		Err:               err,
		OutcomeType:       outcome,
		ExceptionCode:     excCode,
		RTTNanos:          rtt,
	}
}

func ReadAndWriteMCAP(cfg config.ReadConfig) error {
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	client, cleanup, err := connect(modbusURL, cfg.Debug)
	if err != nil {
		return fmt.Errorf("connection error: %w", err)
	}
	defer cleanup()

	out := cfg.OutputFile
	if out == "" || strings.HasSuffix(out, "/") {
		dir := out
		if dir == "" {
			dir = "./"
		}
		ts := time.Now().Format(time.RFC3339)
		out = fmt.Sprintf("%smodbusctl_read_%s.mcap", dir, ts[:10]+"_"+ts[11:19])
	}

	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	rawData, requestTimestamp, responseTimestamp, err := readRegisters(&client, cfg.Function, cfg.StartAddress, cfg.RegisterCount, 1, modbusURL, cfg.Unit, &cleanup, 0, cfg.Debug)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	byteCount := len(rawData)
	preSwapHex := ""
	if cfg.SwapBytes {
		preSwapHex = fmt.Sprintf("% X", rawData)
		if len(rawData)%2 != 0 {
			fmt.Fprintf(os.Stderr, "⚠️ ByteSwap requested but data length (%d) is not even; last byte will be left as-is\n", len(rawData))
		}
		for i := 0; i+1 < len(rawData); i += 2 {
			rawData[i], rawData[i+1] = rawData[i+1], rawData[i]
		}
	}

	finalHex := fmt.Sprintf("% X", rawData)
	asciiDecoded := ""
	if cfg.Ascii {
		var builder strings.Builder
		for _, b := range rawData {
			if strconv.IsPrint(rune(b)) {
				builder.WriteByte(b)
			} else {
				builder.WriteByte('.')
			}
		}
		asciiDecoded = builder.String()
	}

	headerIP, headerPort := cfg.IP, cfg.Port
	if strings.TrimSpace(cfg.URL) != "" {
		headerIP, headerPort = config.ParseModbusURLHostPort(modbusURL)
	}
	header := types.CaptureHeader{
		IP:        headerIP,
		Port:      headerPort,
		Unit:      cfg.Unit,
		Function:  cfg.Function,
		StartTime: time.Now().UnixNano(),
	}
	record := types.CaptureRecord{
		Iteration:         0,
		RequestTimestamp:  requestTimestamp,
		ResponseTimestamp: responseTimestamp,
		StartAddress:      cfg.StartAddress,
		RegisterCount:     cfg.RegisterCount,
		Data:              rawData,
	}

	if err := format.WriteHeader(f, header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if err := format.AppendRecord(f, record); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	readRes := &types.ReadResult{
		Target:         modbusURL,
		UnitID:         cfg.Unit,
		Function:       cfg.Function,
		StartAddress:   cfg.StartAddress,
		RegisterCount:  cfg.RegisterCount,
		RawByteCount:   byteCount,
		PreSwapHex:     preSwapHex,
		RawDataHex:     finalHex,
		BytesSwapped:   cfg.SwapBytes,
		AsciiDecoded:   asciiDecoded,
		McapOutputPath: out,
	}
	outFmt, err := format.Parse(cfg.OutputFormat)
	if err != nil {
		return err
	}
	if err := format.Write(os.Stdout, outFmt, readRes); err != nil {
		return err
	}
	return nil
}

func ScanAndWriteMCAP(cfg config.ScanConfig) (*types.ScanSummaryResult, error) {
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	client, cleanup, err := connect(modbusURL, cfg.Debug)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}
	defer cleanup()

	out := cfg.OutputFile
	if out == "" || strings.HasSuffix(out, "/") {
		dir := out
		if dir == "" {
			dir = "./"
		}
		ts := time.Now().Format(time.RFC3339)
		out = fmt.Sprintf("%smodbusctl_scan_%s.mcap", dir, ts[:10]+"_"+ts[11:19])
	}

	f, err := os.Create(out)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	headerIP, headerPort := cfg.IP, cfg.Port
	if strings.TrimSpace(cfg.URL) != "" {
		headerIP, headerPort = config.ParseModbusURLHostPort(modbusURL)
	}
	header := types.CaptureHeader{
		IP:        headerIP,
		Port:      headerPort,
		Unit:      cfg.Unit,
		Function:  cfg.Function,
		StartTime: time.Now().UnixNano(),
	}
	if err := format.WriteHeader(f, header); err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	strategy, err := newScanStrategy(cfg)
	if err != nil {
		return nil, err
	}
	strategy.Init(cfg)

	algo := strings.ToLower(strings.TrimSpace(cfg.Algo))
	if algo == "" {
		algo = "safe"
	}
	if algo == "sunspec" {
		fmt.Fprintf(os.Stderr, "SunSpec discovery with function code %d (algo: sunspec)\n", cfg.Function)
	} else {
		fmt.Fprintf(os.Stderr, "Scanning registers from %d to %d with function code %d (algo: %s)\n", cfg.StartAddress, cfg.EndAddress, cfg.Function, algo)
	}
	printScanWorstCaseHint(cfg, algo)

	var stats ScanStats
	var iteration uint32
	startTime := time.Now()

	for !strategy.Done() {
		task, ok := strategy.Next()
		if !ok {
			break
		}
		if cfg.Debug && task.Count > 0 {
			end := task.Start + task.Count - 1
			fmt.Fprintf(os.Stderr, "DEBUG [exec] next task: start=%d count=%d end=%d\n", task.Start, task.Count, end)
		}
		result := executeReadTask(&client, cfg, task, &cleanup, modbusURL)
		// Milestone B: retry once on timeout/transport if configured
		if !result.Success && cfg.RetryOnTimeoutTransport > 0 &&
			(result.OutcomeType == ScanOutcomeTimeout || result.OutcomeType == ScanOutcomeTransport) {
			stats.TotalRequests++
			time.Sleep(time.Duration(cfg.Delay) * time.Millisecond)
			result = executeReadTask(&client, cfg, task, &cleanup, modbusURL)
		}
		if cfg.Debug {
			outcome := "success"
			if !result.Success {
				outcome = string(result.OutcomeType)
				if result.OutcomeType == ScanOutcomeException && result.ExceptionCode != 0 {
					outcome = fmt.Sprintf("%s code=0x%02x", result.OutcomeType, result.ExceptionCode)
				}
			}
			fmt.Fprintf(os.Stderr, "DEBUG [exec] result: %s (start=%d count=%d)\n", outcome, result.Start, result.Count)
		}
		strategy.OnResult(task, result)

		stats.TotalRequests++
		if result.Success {
			stats.SuccessCount++
			stats.BlocksCaptured++
			stats.RegistersCaptured += int(result.Count)
			stats.ResponseTimeNanos += result.ResponseTimestamp - result.RequestTimestamp

			rec := types.CaptureRecord{
				Iteration:         iteration,
				RequestTimestamp:  result.RequestTimestamp,
				ResponseTimestamp: result.ResponseTimestamp,
				StartAddress:      result.Start,
				RegisterCount:     result.Count,
				Data:              result.Data,
			}
			if err := format.AppendRecord(f, rec); err != nil {
				return nil, fmt.Errorf("failed to write record: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Block: Start: %d, End: %d, Count: %d\n", rec.StartAddress, rec.StartAddress+rec.RegisterCount-1, rec.RegisterCount)
		} else {
			stats.FailCount++
			switch result.OutcomeType {
			case ScanOutcomeException:
				stats.ExceptionCount++
			case ScanOutcomeTimeout:
				stats.TimeoutCount++
			case ScanOutcomeTransport:
				stats.TransportErrorCount++
			}
		}

		time.Sleep(time.Duration(cfg.Delay) * time.Millisecond)
		iteration++
	}

	stats.TotalDurationNanos = time.Since(startTime).Nanoseconds()

	durStr := time.Duration(stats.TotalDurationNanos).Round(time.Millisecond).String()
	summary := &types.ScanSummaryResult{
		Target:              modbusURL,
		Algo:                algo,
		TotalRequests:       stats.TotalRequests,
		SuccessCount:        stats.SuccessCount,
		FailCount:           stats.FailCount,
		ExceptionCount:      stats.ExceptionCount,
		TimeoutCount:        stats.TimeoutCount,
		TransportErrorCount: stats.TransportErrorCount,
		BlocksCaptured:      stats.BlocksCaptured,
		RegistersCaptured:   stats.RegistersCaptured,
		Duration:            durStr,
		McapOutputPath:      out,
	}
	if stats.SuccessCount > 0 {
		summary.AvgResponseMs = (stats.ResponseTimeNanos / int64(stats.SuccessCount)) / 1e6
	}
	return summary, nil
}

func RecordAndWriteMCAP(cfg config.RecordConfig) (*types.RecordSummaryResult, error) {
	file, err := os.Open(cfg.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var blocks []types.AddressBlock
	if err := json.NewDecoder(file).Decode(&blocks); err != nil {
		return nil, fmt.Errorf("failed to decode input blocks: %w", err)
	}

	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	client, cleanup, err := connect(modbusURL, cfg.Debug)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}
	defer cleanup()

	out := cfg.OutputFile
	if out == "" || strings.HasSuffix(out, "/") {
		dir := out
		if dir == "" {
			dir = "./"
		}
		ts := time.Now().Format(time.RFC3339)
		out = fmt.Sprintf("%smodbusctl_record_%s.mcap", dir, ts[:10]+"_"+ts[11:19])
	}

	f, err := os.Create(out)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	headerIP, headerPort := cfg.IP, cfg.Port
	if strings.TrimSpace(cfg.URL) != "" {
		headerIP, headerPort = config.ParseModbusURLHostPort(modbusURL)
	}
	header := types.CaptureHeader{
		IP:        headerIP,
		Port:      headerPort,
		Unit:      cfg.Unit,
		Function:  cfg.Function,
		StartTime: time.Now().UnixNano(),
	}
	if err := format.WriteHeader(f, header); err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	startTime := time.Now()
	var i uint32 = 0
	var blockCount int
	for {
		elapsed := time.Since(startTime)
		if elapsed >= time.Duration(cfg.Duration)*time.Millisecond {
			break
		}
		fmt.Fprintf(os.Stderr, "📟 Recording %d started...\n", i)
		for _, b := range blocks {
			data, requestTimestamp, responseTimestamp, err := readRegisters(&client, cfg.Function, b.StartAddress, b.RegisterCount, 5, modbusURL, cfg.Unit, &cleanup, 0, cfg.Debug)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ Failed to read block (start: %d, count: %d): %v\n", b.StartAddress, b.RegisterCount, err)
				continue
			}
			rec := types.CaptureRecord{
				Iteration:         i,
				RequestTimestamp:  requestTimestamp,
				ResponseTimestamp: responseTimestamp,
				StartAddress:      b.StartAddress,
				RegisterCount:     b.RegisterCount,
				Data:              data,
			}
			if err := format.AppendRecord(f, rec); err != nil {
				return nil, fmt.Errorf("failed to write record: %w", err)
			}
			blockCount++
			fmt.Fprintf(os.Stderr, "✓ Recorded block: Start %d, Count %d\n", b.StartAddress, b.RegisterCount)
		}
		i++
		if cfg.Interval > 0 {
			time.Sleep(time.Duration(cfg.Interval) * time.Millisecond)
		}
	}

	return &types.RecordSummaryResult{
		Target:         modbusURL,
		BlocksRecorded: blockCount,
		Iterations:     i,
		McapOutputPath: out,
	}, nil
}

// ObjectDescription returns a human-readable name for a device identification
// object ID when the library does not provide one (e.g. extended objects).
func ObjectDescription(id modbus.DeviceIDObjectID) string {
	switch {
	case id == modbus.DeviceIDObjectID(0x00):
		return "VendorName"
	case id == modbus.DeviceIDObjectID(0x01):
		return "ProductCode"
	case id == modbus.DeviceIDObjectID(0x02):
		return "MajorMinorRevision"
	case id == modbus.DeviceIDObjectID(0x03):
		return "VendorUrl"
	case id == modbus.DeviceIDObjectID(0x04):
		return "ProductName"
	case id == modbus.DeviceIDObjectID(0x05):
		return "ModelName"
	case id == modbus.DeviceIDObjectID(0x06):
		return "UserApplicationName"
	case id >= 0x07 && id <= 0x7F:
		return "Reserved"
	case id >= 0x80:
		return "Extended"
	default:
		return ""
	}
}

// ParseUnitIDs parses --unit value and returns the list of unit IDs (1-255).
// Accepts: "all" (1-255), single "N", range "N-M", list "N,M,P", or mixed "N-M,P,Q-R".
// Used by both the modbus package and validate.
func ParseUnitIDs(unitID string) ([]uint8, error) {
	s := strings.TrimSpace(strings.ToLower(unitID))
	if s == "" {
		return nil, fmt.Errorf("unit ID cannot be empty")
	}
	if s == "all" {
		ids := make([]uint8, 0, modbusUnitIDMax-modbusUnitIDMin+1)
		for i := modbusUnitIDMin; i <= modbusUnitIDMax; i++ {
			ids = append(ids, uint8(i))
		}
		return ids, nil
	}

	seen := make(map[uint8]struct{})
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			// Range: N-M
			rangeParts := strings.SplitN(part, "-", 2)
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid unit range %q", part)
			}
			lo, err := strconv.ParseUint(strings.TrimSpace(rangeParts[0]), 10, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid unit range %q: %w", part, err)
			}
			hi, err := strconv.ParseUint(strings.TrimSpace(rangeParts[1]), 10, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid unit range %q: %w", part, err)
			}
			if lo < 1 || hi > 255 {
				return nil, fmt.Errorf("unit IDs must be 1-255 in range %q", part)
			}
			if lo > hi {
				return nil, fmt.Errorf("invalid unit range %q: start > end", part)
			}
			for i := lo; i <= hi; i++ {
				seen[uint8(i)] = struct{}{}
			}
		} else {
			// Single number
			n, err := strconv.ParseUint(part, 10, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid unit ID %q: %w", part, err)
			}
			if n < 1 || n > 255 {
				return nil, fmt.Errorf("unit ID must be 1-255, got %d", n)
			}
			seen[uint8(n)] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("unit ID must be 1-255, \"all\", a range (e.g. 1-10), or a list (e.g. 1,5,25)")
	}
	ids := make([]uint8, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func parseUnitIDs(unitID string) ([]uint8, error) {
	return ParseUnitIDs(unitID)
}

// readFCsForFingerprint are the read-style function codes probed by SupportsFunction (FC08, FC43, FC03, FC04, FC01, FC02, FC11, FC18, FC20).
var readFCsForFingerprint = []modbus.FunctionCode{
	modbus.FCDiagnostics,           // 0x08
	modbus.FCEncapsulatedInterface, // 0x2B (FC43)
	modbus.FCReadHoldingRegisters,  // 0x03
	modbus.FCReadInputRegisters,    // 0x04
	modbus.FCReadCoils,             // 0x01
	modbus.FCReadDiscreteInputs,    // 0x02
	modbus.FCReportServerID,        // 0x11
	modbus.FCReadFIFOQueue,         // 0x18
	modbus.FCReadFileRecord,        // 0x14 (FC20)
}
