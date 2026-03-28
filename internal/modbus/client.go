package modbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/types"
)

const MaxBlockSize = 125

// Modbus standard valid slave/unit IDs when using "all".
const modbusUnitIDMin, modbusUnitIDMax = 1, 255

// defaultDialTimeout is used when building client config (TCP dial; TLS would use a longer value).
const defaultDialTimeout = 5 * time.Second

// Discovery uses a short Modbus op + dial budget so subnet sweeps do not wait 10s per host.
const discoveryModbusTimeout = 800 * time.Millisecond
const discoveryDialTimeout = 500 * time.Millisecond

var (
	ErrTCPConnection    = errors.New("TCP connection error")
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

// clientRequestTimeout maps CLI milliseconds to Modbus per-request timeout (0 = 10s).
func clientRequestTimeout(ms uint16) time.Duration {
	if ms == 0 {
		return 10 * time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

// dialTimeoutForRequest caps TCP dial time by the Modbus request timeout and by defaultDialTimeout.
func dialTimeoutForRequest(request time.Duration) time.Duration {
	if request <= 0 {
		return defaultDialTimeout
	}
	d := request
	if d > defaultDialTimeout {
		d = defaultDialTimeout
	}
	const minDial = 200 * time.Millisecond
	if d < minDial {
		d = minDial
	}
	return d
}

func buildDeviceClientConfig(modbusURL string, timeoutMS uint16, debug bool) modbus.Config {
	req := clientRequestTimeout(timeoutMS)
	c := buildClientConfig(modbusURL, req, debug)
	c.DialTimeout = dialTimeoutForRequest(req)
	return c
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

func connectDevice(modbusURL string, timeoutMS uint16, debug bool) (*modbus.Client, func(), error) {
	return validateAndConnect(buildDeviceClientConfig(modbusURL, timeoutMS, debug))
}

func discoveryClientConfig(modbusURL string, debug bool) modbus.Config {
	c := buildClientConfig(modbusURL, discoveryModbusTimeout, debug)
	c.DialTimeout = discoveryDialTimeout
	return c
}

// connectDiscovery opens one Modbus TCP client (dial + Open) with short timeouts for subnet discovery.
func connectDiscovery(modbusURL string, debug bool) (*modbus.Client, func(), error) {
	return validateAndConnect(discoveryClientConfig(modbusURL, debug))
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
	if errors.Is(err, modbus.ErrRequestTimedOut) || errors.Is(err, context.DeadlineExceeded) {
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
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ENETRESET) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.EPIPE, syscall.ECONNRESET, syscall.ENETRESET:
			return true
		default:
			// Other errno: fall through to string heuristics below.
		}
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil {
		if shouldReconnect(opErr.Err) {
			return true
		}
	}
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "EOF") || strings.Contains(msg, "connection reset")
}

func performRead(ctx context.Context, client *modbus.Client, unitID uint8, fc uint8, start, count uint16) ([]byte, error) {
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
		return nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("unsupported function code: %d", fc), nil)
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

func readRegisters(ctx context.Context, clientPtr **modbus.Client, fc uint8, start, count uint16, retries uint8, modbusURL string, unit uint8, cleanup *func(), delay uint16, debug bool, requestTimeoutMS uint16, stderr io.Writer) (data []byte, requestTS int64, responseTS int64, err error) {
	if stderr == nil {
		stderr = io.Discard
	}
	for attempt := 1; attempt <= int(retries); attempt++ {
		if delay > 0 {
			if debug {
				_, _ = fmt.Fprintf(stderr, "⏳ Waiting %d ms before retrying...\n", delay)
			}
			if err := sleepContext(ctx, time.Duration(delay)*time.Millisecond); err != nil {
				return nil, 0, 0, err
			}
		}
		requestTS = time.Now().UnixNano()
		data, err = performRead(ctx, *clientPtr, unit, fc, start, count)
		responseTS = time.Now().UnixNano()

		if err == nil {
			return data, requestTS, responseTS, nil
		}

		if shouldRetry(err) {
			if debug {
				_, _ = fmt.Fprintf(stderr, "🔁 Retrying due to Modbus read exception on address %d with count %d (attempt %d): %v\n", start, count, attempt, err)
			}
			if delay == 0 {
				if err := sleepContext(ctx, 20*time.Millisecond); err != nil {
					return nil, 0, 0, err
				}
			}
			continue
		}

		if shouldReconnect(err) {
			if debug {
				_, _ = fmt.Fprintf(stderr, "🔁 Reconnecting due to connection error (attempt %d)...\n", attempt)
			}
			if cleanup != nil {
				(*cleanup)()
			}
			var newClient *modbus.Client
			var newCleanup func()
			newClient, newCleanup, err = connectDevice(modbusURL, requestTimeoutMS, debug)
			if err != nil {
				return nil, 0, 0, TCPConnectionError(err)
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
func executeReadTask(ctx context.Context, clientPtr **modbus.Client, cfg config.ScanConfig, task ScanTask, cleanup *func(), modbusURL string, stderr io.Writer) ScanResult {
	data, reqTS, resTS, err := readRegisters(ctx, clientPtr, cfg.Function, task.Start, task.Count, 1, modbusURL, cfg.Unit, cleanup, cfg.Delay, cfg.Debug, cfg.Timeout, stderr)
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

// CollectRead performs the Modbus read, writes MCAP, and returns the stdout payload (no formatted stdout write).
func CollectRead(ctx context.Context, cfg config.ReadConfig, progress io.Writer) (*types.ReadResult, error) {
	if progress == nil {
		progress = io.Discard
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	client, cleanup, err := connectDevice(modbusURL, cfg.Timeout, cfg.Debug)
	if err != nil {
		return nil, TCPConnectionError(err)
	}
	defer cleanup()

	mcapPath := readCaptureMcapPath(cfg)
	f, err := os.Create(mcapPath)
	if err != nil {
		return nil, errs.Output(errs.CodeOutputFileCreateFailed, err)
	}
	defer func() { _ = f.Close() }()

	rawData, requestTimestamp, responseTimestamp, err := readRegisters(ctx, &client, cfg.Function, cfg.StartAddress, cfg.RegisterCount, 1, modbusURL, cfg.Unit, &cleanup, 0, cfg.Debug, cfg.Timeout, progress)
	if err != nil {
		return nil, err
	}

	byteCount := len(rawData)
	preSwapHex := ""
	if cfg.SwapBytes {
		preSwapHex = fmt.Sprintf("% X", rawData)
		if len(rawData)%2 != 0 {
			_, _ = fmt.Fprintf(progress, "⚠️ ByteSwap requested but data length (%d) is not even; last byte will be left as-is\n", len(rawData))
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

	if err := writeReadCaptureToMcap(f, cfg, modbusURL, rawData, requestTimestamp, responseTimestamp); err != nil {
		return nil, errs.Output(errs.CodeMcapWriteFailed, err)
	}

	return newReadResult(cfg, modbusURL, mcapPath, preSwapHex, finalHex, asciiDecoded, byteCount), nil
}

func ScanAndWriteMCAP(ctx context.Context, cfg config.ScanConfig, progress io.Writer) (*types.ScanSummaryResult, error) {
	if progress == nil {
		progress = io.Discard
	}
	// Progress classification (written to progress, typically stderr): always-on = scan banner, worst-case
	// hint, per-success block summary, and inter-request delay (silent when delay 0). Debug-only = executor
	// next-task/result lines gated on cfg.Debug; strategy debug uses scanSettings.DebugWriter (same stream when debug).
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	client, cleanup, err := connectDevice(modbusURL, cfg.Timeout, cfg.Debug)
	if err != nil {
		return nil, TCPConnectionError(err)
	}
	defer cleanup()

	out := cfg.OutputFile
	if out == "" || strings.HasSuffix(out, "/") {
		dir := out
		if dir == "" {
			dir = "./"
		}
		out = AutoCaptureMcapPath(dir, "scan")
	}

	f, err := os.Create(out)
	if err != nil {
		return nil, errs.Output(errs.CodeOutputFileCreateFailed, err)
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
	if err := mcap.WriteHeader(f, header); err != nil {
		return nil, errs.Output(errs.CodeMcapWriteFailed, err)
	}

	ss := scanSettings{ScanConfig: cfg}
	if cfg.Debug {
		ss.DebugWriter = progress
	}
	strategy, err := newScanStrategy(ss)
	if err != nil {
		return nil, err
	}
	strategy.Init(ss)

	algo := string(config.ScanAlgorithmForExecution(&cfg))
	if algo == "sunspec" {
		_, _ = fmt.Fprintf(progress, "SunSpec discovery with function code %d (algo: sunspec)\n", cfg.Function)
	} else {
		_, _ = fmt.Fprintf(progress, "Scanning registers from %d to %d with function code %d (algo: %s)\n", cfg.StartAddress, cfg.EndAddress, cfg.Function, algo)
	}
	printScanWorstCaseHint(cfg, algo, progress)

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
			_, _ = fmt.Fprintf(progress, "DEBUG [exec] next task: start=%d count=%d end=%d\n", task.Start, task.Count, end)
		}
		result := executeReadTask(ctx, &client, cfg, task, &cleanup, modbusURL, progress)
		// Milestone B: retry once on timeout/transport if configured
		retryAppliedDelay := false
		if !result.Success && cfg.RetryOnTimeoutTransport > 0 &&
			(result.OutcomeType == ScanOutcomeTimeout || result.OutcomeType == ScanOutcomeTransport) {
			stats.TotalRequests++
			if cfg.Delay > 0 {
				if err := sleepContext(ctx, time.Duration(cfg.Delay)*time.Millisecond); err != nil {
					return nil, err
				}
				retryAppliedDelay = true
			}
			result = executeReadTask(ctx, &client, cfg, task, &cleanup, modbusURL, progress)
		}
		if cfg.Debug {
			outcome := "success"
			if !result.Success {
				outcome = string(result.OutcomeType)
				if result.OutcomeType == ScanOutcomeException && result.ExceptionCode != 0 {
					outcome = fmt.Sprintf("%s code=0x%02x", result.OutcomeType, result.ExceptionCode)
				}
			}
			_, _ = fmt.Fprintf(progress, "DEBUG [exec] result: %s (start=%d count=%d)\n", outcome, result.Start, result.Count)
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
			if err := mcap.AppendRecord(f, rec); err != nil {
				return nil, errs.Output(errs.CodeMcapWriteFailed, err)
			}
			_, _ = fmt.Fprintf(progress, "Block: Start: %d, End: %d, Count: %d\n", rec.StartAddress, rec.StartAddress+rec.RegisterCount-1, rec.RegisterCount)
		} else {
			stats.FailCount++
			switch result.OutcomeType {
			case ScanOutcomeSuccess:
				// Unreachable when !result.Success; kept for exhaustiveness.
			case ScanOutcomeException:
				stats.ExceptionCount++
			case ScanOutcomeTimeout:
				stats.TimeoutCount++
			case ScanOutcomeTransport:
				stats.TransportErrorCount++
			case ScanOutcomeProtocol, ScanOutcomeUnknown:
				// No separate counter; failure already reflected in FailCount.
			}
		}

		if !retryAppliedDelay {
			if err := sleepContext(ctx, time.Duration(cfg.Delay)*time.Millisecond); err != nil {
				return nil, err
			}
		}
		iteration++
	}

	stats.TotalDurationNanos = time.Since(startTime).Nanoseconds()

	durStr := time.Duration(stats.TotalDurationNanos).Round(time.Millisecond).String()
	summary := &types.ScanSummaryResult{
		Target:              modbusURL,
		Summary:             types.NewResultSummary(stats.TotalRequests, stats.SuccessCount, stats.FailCount),
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

func RecordAndWriteMCAP(ctx context.Context, cfg config.RecordConfig, progress io.Writer) (*types.RecordSummaryResult, error) {
	if progress == nil {
		progress = io.Discard
	}
	// Progress classification: always-on = iteration start, per-block success/failure, read-retry chatter
	// when cfg.Debug inside readRegisters; interval sleep has no line (delay 0 skips sleep).
	file, err := os.Open(cfg.BlocksFile)
	if err != nil {
		return nil, errs.Output(errs.CodeInputFileOpenFailed, err)
	}
	defer func() { _ = file.Close() }()

	var blocks []types.AddressBlock
	if err := json.NewDecoder(file).Decode(&blocks); err != nil {
		return nil, errs.Output(errs.CodeInputDecodeFailed, err)
	}

	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	client, cleanup, err := connectDevice(modbusURL, cfg.Timeout, cfg.Debug)
	if err != nil {
		return nil, TCPConnectionError(err)
	}
	defer cleanup()

	out := cfg.OutputFile
	if out == "" || strings.HasSuffix(out, "/") {
		dir := out
		if dir == "" {
			dir = "./"
		}
		out = AutoCaptureMcapPath(dir, "record")
	}

	f, err := os.Create(out)
	if err != nil {
		return nil, errs.Output(errs.CodeOutputFileCreateFailed, err)
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
	if err := mcap.WriteHeader(f, header); err != nil {
		return nil, errs.Output(errs.CodeMcapWriteFailed, err)
	}

	startTime := time.Now()
	var i uint32 = 0
	var blockCount int
	for {
		elapsed := time.Since(startTime)
		if elapsed >= time.Duration(cfg.Duration)*time.Millisecond {
			break
		}
		_, _ = fmt.Fprintf(progress, "📟 Recording %d started...\n", i)
		for _, b := range blocks {
			data, requestTimestamp, responseTimestamp, err := readRegisters(ctx, &client, cfg.Function, b.StartAddress, b.RegisterCount, 5, modbusURL, cfg.Unit, &cleanup, 0, cfg.Debug, cfg.Timeout, progress)
			if err != nil {
				_, _ = fmt.Fprintf(progress, "⚠️ Failed to read block (start: %d, count: %d): %v\n", b.StartAddress, b.RegisterCount, err)
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
			if err := mcap.AppendRecord(f, rec); err != nil {
				return nil, errs.Output(errs.CodeMcapWriteFailed, err)
			}
			blockCount++
			_, _ = fmt.Fprintf(progress, "✓ Recorded block: Start %d, Count %d\n", b.StartAddress, b.RegisterCount)
		}
		i++
		if cfg.Interval > 0 {
			if err := sleepContext(ctx, time.Duration(cfg.Interval)*time.Millisecond); err != nil {
				return nil, err
			}
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
		return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, "unit ID cannot be empty", nil)
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
			return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, "unit list has an empty segment (remove extra commas)", nil)
		}
		if strings.Contains(part, "-") {
			// Range: N-M
			rangeParts := strings.SplitN(part, "-", 2)
			if len(rangeParts) != 2 {
				return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, fmt.Sprintf("invalid unit range %q", part), nil)
			}
			loStr := strings.TrimSpace(rangeParts[0])
			hiStr := strings.TrimSpace(rangeParts[1])
			if loStr == "" || hiStr == "" {
				return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, fmt.Sprintf("invalid unit range %q (empty bound)", part), nil)
			}
			lo, err := strconv.ParseUint(loStr, 10, 8)
			if err != nil {
				return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, fmt.Sprintf("invalid unit range %q: %v", part, err), err)
			}
			hi, err := strconv.ParseUint(hiStr, 10, 8)
			if err != nil {
				return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, fmt.Sprintf("invalid unit range %q: %v", part, err), err)
			}
			if lo < 1 || hi > 255 {
				return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, fmt.Sprintf("unit IDs must be 1-255 in range %q", part), nil)
			}
			if lo > hi {
				return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, fmt.Sprintf("invalid unit range %q: start > end", part), nil)
			}
			for i := lo; i <= hi; i++ {
				seen[uint8(i)] = struct{}{}
			}
		} else {
			// Single number
			n, err := strconv.ParseUint(part, 10, 8)
			if err != nil {
				return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, fmt.Sprintf("invalid unit ID %q: %v", part, err), err)
			}
			if n < 1 || n > 255 {
				return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, fmt.Sprintf("unit ID must be 1-255, got %d", n), nil)
			}
			seen[uint8(n)] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil, errs.InvalidInput(errs.CodeInvalidUnitSelector, `unit ID must be 1-255, "all", a range (e.g. 1-10), or a list (e.g. 1,5,25)`, nil)
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
