package modbus

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/types"
)

const MaxBlockSize = 125

// Modbus standard valid slave/unit IDs when using "all".
const modbusUnitIDMin, modbusUnitIDMax = 1, 255

var (
	ErrTCPConnection    = errors.New("TCP connection error")
	ErrFC43Timeout      = errors.New("FC43 timeout")
	ErrFC43NotSupported = errors.New("FC43 not supported or invalid response")
)

func connect(modbusURL string) (*modbus.ModbusClient, func(), error) {
	conf := &modbus.ClientConfiguration{
		URL:     modbusURL,
		Timeout: 10 * time.Second,
	}
	client, err := modbus.NewClient(conf)
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
	if errors.Is(err, modbus.ErrProtocolError) || errors.Is(err, modbus.ErrBadUnitId) ||
		errors.Is(err, modbus.ErrBadTransactionId) || errors.Is(err, modbus.ErrUnknownProtocolId) {
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

func performRead(client *modbus.ModbusClient, unitID uint8, fc uint8, start, count uint16) ([]byte, error) {
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
		return client.ReadRawBytes(ctx, unitID, start, count*2, modbus.HoldingRegister)
	case 4:
		return client.ReadRawBytes(ctx, unitID, start, count*2, modbus.InputRegister)
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

func readRegisters(clientPtr **modbus.ModbusClient, fc uint8, start, count uint16, retries uint8, modbusURL string, unit uint8, cleanup *func(), delay uint16, debug bool) (data []byte, requestTS int64, responseTS int64, err error) {
	for attempt := 1; attempt <= int(retries); attempt++ {
		if delay > 0 {
			if debug {
				fmt.Printf("⏳ Waiting %d ms before retrying...\n", delay)
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
			var newClient *modbus.ModbusClient
			var newCleanup func()
			newClient, newCleanup, err = connect(modbusURL)
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
func executeReadTask(clientPtr **modbus.ModbusClient, cfg config.ScanConfig, task ScanTask, cleanup *func(), modbusURL string) ScanResult {
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
	client, cleanup, err := connect(modbusURL)
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

	fmt.Printf("Reading %d registers starting from address %d using function code %d\n", cfg.RegisterCount, cfg.StartAddress, cfg.Function)
	rawData, requestTimestamp, responseTimestamp, err := readRegisters(&client, cfg.Function, cfg.StartAddress, cfg.RegisterCount, 1, modbusURL, cfg.Unit, &cleanup, 0, cfg.Debug)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}
	fmt.Printf("Read %d bytes of data: % X\n", len(rawData), rawData)

	if cfg.SwapBytes {
		if len(rawData)%2 != 0 {
			fmt.Fprintf(os.Stderr, "⚠️ ByteSwap requested but data length (%d) is not even; last byte will be left as-is\n", len(rawData))
		}
		for i := 0; i+1 < len(rawData); i += 2 {
			rawData[i], rawData[i+1] = rawData[i+1], rawData[i]
		}
		fmt.Printf("🔁 Byte-swapped data: % X\n", rawData)
	}

	if cfg.Ascii {
		fmt.Println("Decoding data as ASCII:")
		var builder strings.Builder
		for _, b := range rawData {
			if strconv.IsPrint(rune(b)) {
				builder.WriteByte(b)
			} else {
				builder.WriteByte('.')
			}
		}
		fmt.Printf("ASCII: %s\n", builder.String())
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

	fmt.Printf("✅ Output written to %s\n", out)
	return nil
}

func ScanAndWriteMCAP(cfg config.ScanConfig) error {
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	client, cleanup, err := connect(modbusURL)
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
		out = fmt.Sprintf("%smodbusctl_scan_%s.mcap", dir, ts[:10]+"_"+ts[11:19])
	}

	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
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
		return fmt.Errorf("failed to write header: %w", err)
	}

	strategy, err := newScanStrategy(cfg)
	if err != nil {
		return err
	}
	strategy.Init(cfg)

	algo := strings.ToLower(strings.TrimSpace(cfg.Algo))
	if algo == "" {
		algo = "safe"
	}
	if algo == "sunspec" {
		fmt.Printf("SunSpec discovery with function code %d (algo: sunspec)\n", cfg.Function)
	} else {
		fmt.Printf("Scanning registers from %d to %d with function code %d (algo: %s)\n", cfg.StartAddress, cfg.EndAddress, cfg.Function, algo)
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
			fmt.Printf("DEBUG [exec] next task: start=%d count=%d end=%d\n", task.Start, task.Count, end)
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
			fmt.Printf("DEBUG [exec] result: %s (start=%d count=%d)\n", outcome, result.Start, result.Count)
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
				return fmt.Errorf("failed to write record: %w", err)
			}
			fmt.Printf("Block: Start: %d, End: %d, Count: %d\n", rec.StartAddress, rec.StartAddress+rec.RegisterCount-1, rec.RegisterCount)
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

	// Summary
	fmt.Println()
	fmt.Printf("Algo: %s\n", algo)
	fmt.Printf("Requests: %d\n", stats.TotalRequests)
	fmt.Printf("Success: %d\n", stats.SuccessCount)
	fmt.Printf("Failed: %d\n", stats.FailCount)
	if stats.ExceptionCount > 0 || stats.TimeoutCount > 0 || stats.TransportErrorCount > 0 {
		fmt.Printf("  Exceptions: %d  Timeouts: %d  Transport errors: %d\n", stats.ExceptionCount, stats.TimeoutCount, stats.TransportErrorCount)
	}
	fmt.Printf("Blocks captured: %d\n", stats.BlocksCaptured)
	fmt.Printf("Registers captured: %d\n", stats.RegistersCaptured)
	if stats.SuccessCount > 0 {
		avgMs := (stats.ResponseTimeNanos / int64(stats.SuccessCount)) / 1e6
		fmt.Printf("Avg response time: %d ms\n", avgMs)
	}
	fmt.Printf("Duration: %s\n", time.Duration(stats.TotalDurationNanos).Round(time.Millisecond))
	fmt.Printf("Output: %s\n", out)
	return nil
}

func RecordAndWriteMCAP(cfg config.RecordConfig) error {
	file, err := os.Open(cfg.InputFile)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var blocks []types.AddressBlock
	if err := json.NewDecoder(file).Decode(&blocks); err != nil {
		return fmt.Errorf("failed to decode input blocks: %w", err)
	}

	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	client, cleanup, err := connect(modbusURL)
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
		out = fmt.Sprintf("%smodbusctl_record_%s.mcap", dir, ts[:10]+"_"+ts[11:19])
	}

	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
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
		return fmt.Errorf("failed to write header: %w", err)
	}

	startTime := time.Now()
	var i uint32 = 0
	var blockCount int
	for {
		elapsed := time.Since(startTime)
		if elapsed >= time.Duration(cfg.Duration)*time.Millisecond {
			break
		}
		fmt.Printf("📟 Recording %d started...\n", i)
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
				return fmt.Errorf("failed to write record: %w", err)
			}
			blockCount++
			fmt.Printf("✓ Recorded block: Start %d, Count %d\n", b.StartAddress, b.RegisterCount)
		}
		i++
		if cfg.Interval > 0 {
			time.Sleep(time.Duration(cfg.Interval) * time.Millisecond)
		}
	}

	fmt.Printf("📦 Total recorded blocks: %d\n", blockCount)
	fmt.Printf("🔄 Total iterations: %d\n", i)
	fmt.Printf("✅ Output written to %s\n", out)
	return nil
}

// objectDescription returns a human-readable name for a device identification
// object ID when the library does not provide one (e.g. extended objects).
func objectDescription(id byte) string {
	switch {
	case id == 0x00:
		return "VendorName"
	case id == 0x01:
		return "ProductCode"
	case id == 0x02:
		return "MajorMinorRevision"
	case id == 0x03:
		return "VendorUrl"
	case id == 0x04:
		return "ProductName"
	case id == 0x05:
		return "ModelName"
	case id == 0x06:
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

// DeviceIdentification reads device identification for one or all units (--unit 1-255 or --unit all).
// If no category flag is set it uses ReadAllDeviceIdentification; otherwise requests selected categories.
// When --unit all and --parallel > 1, probes units concurrently with a pool of connections.
func DeviceIdentification(cfg config.IdentifyConfig) error {
	units, err := parseUnitIDs(cfg.UnitID)
	if err != nil {
		return err
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	fmt.Printf("🔍 Connecting to %s...\n", modbusURL)

	conf := &modbus.ClientConfiguration{
		URL:     modbusURL,
		Timeout: time.Duration(cfg.Timeout) * time.Millisecond,
	}

	useParallel := len(units) > 1 && cfg.Parallel > 1
	if !useParallel {
		mc, err := modbus.NewClient(conf)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrFC43NotSupported, err)
		}
		if err := mc.Open(); err != nil {
			return fmt.Errorf("%w: %v", ErrTCPConnection, err)
		}
		defer func() { _ = mc.Close() }()

		for _, unit := range units {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
			if len(units) > 1 {
				fmt.Printf("\n--- Unit ID %d ---\n", unit)
			}
			err := deviceIdentificationForUnit(ctx, mc, cfg, unit, os.Stdout)
			cancel()
			if err != nil {
				if len(units) > 1 {
					fmt.Printf("⚠️ Unit %d: %v\n", unit, err)
					continue
				}
				return err
			}
		}
		return nil
	}

	// Parallel path: pool of clients, workers send (unit, output, err), collect and print in unit order.
	n := int(cfg.Parallel)
	if n > len(units) {
		n = len(units)
	}
	clients := make([]*modbus.ModbusClient, 0, n)
	for i := 0; i < n; i++ {
		mc, err := modbus.NewClient(conf)
		if err != nil {
			for _, c := range clients {
				_ = c.Close()
			}
			return fmt.Errorf("%w: %v", ErrFC43NotSupported, err)
		}
		if err := mc.Open(); err != nil {
			for _, c := range clients {
				_ = c.Close()
			}
			return fmt.Errorf("%w: %v", ErrTCPConnection, err)
		}
		clients = append(clients, mc)
	}
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()

	type identifyResult struct {
		unit   uint8
		output string
		err    error
	}
	unitsCh := make(chan uint8, len(units))
	for _, u := range units {
		unitsCh <- u
	}
	close(unitsCh)
	resultsCh := make(chan identifyResult, len(units))

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		mc := clients[i]
		go func() {
			defer wg.Done()
			for unit := range unitsCh {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
				buf := &bytes.Buffer{}
				err := deviceIdentificationForUnit(ctx, mc, cfg, unit, buf)
				cancel()
				resultsCh <- identifyResult{unit: unit, output: buf.String(), err: err}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var results []identifyResult
	for r := range resultsCh {
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].unit < results[j].unit })
	for _, r := range results {
		fmt.Printf("\n--- Unit ID %d ---\n", r.unit)
		if r.err != nil {
			fmt.Printf("⚠️ Unit %d: %v\n", r.unit, r.err)
		} else {
			fmt.Print(r.output)
		}
	}
	return nil
}

func deviceIdentificationForUnit(ctx context.Context, mc *modbus.ModbusClient, cfg config.IdentifyConfig, unit uint8, w io.Writer) error {
	useCategories := cfg.Basic || cfg.Regular || cfg.Extended
	if !useCategories {
		di, err := mc.ReadAllDeviceIdentification(ctx, unit)
		if err != nil {
			return err
		}
		if di == nil {
			return ErrFC43NotSupported
		}
		printDeviceIdentification(w, di)
		if cfg.ServerID {
			printReportServerId(ctx, mc, unit, w)
		}
		return nil
	}

	objectsByID := make(map[uint8]modbus.DeviceIdentificationObject)
	var header *modbus.DeviceIdentification
	for _, category := range []struct {
		flag bool
		code uint8
	}{
		{cfg.Basic, modbus.ReadDeviceIdBasic},
		{cfg.Regular, modbus.ReadDeviceIdRegular},
		{cfg.Extended, modbus.ReadDeviceIdExtended},
	} {
		if !category.flag {
			continue
		}
		di, err := mc.ReadDeviceIdentification(ctx, unit, category.code, 0)
		if err != nil {
			return err
		}
		if di == nil {
			continue
		}
		if header == nil {
			header = di
		}
		for _, obj := range di.Objects {
			if _, seen := objectsByID[obj.Id]; !seen {
				objectsByID[obj.Id] = obj
			}
		}
	}
	if header == nil {
		return ErrFC43NotSupported
	}

	ids := make([]uint8, 0, len(objectsByID))
	for id := range objectsByID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	_, _ = fmt.Fprintf(w, "✅ Device Identification (DevID Code: %d, Conformity Level: 0x%02X, More Follows: 0x%02X, Next Object ID: %d, Object Count: %d)\n",
		header.ReadDeviceIdCode, header.ConformityLevel, header.MoreFollows, header.NextObjectId, len(ids))
	for _, id := range ids {
		obj := objectsByID[id]
		desc := obj.Name
		if desc == "" {
			desc = objectDescription(obj.Id)
		}
		if desc != "" {
			_, _ = fmt.Fprintf(w, " - Object %d: %s [%s]\n", obj.Id, obj.Value, desc)
		} else {
			_, _ = fmt.Fprintf(w, " - Object %d: %s\n", obj.Id, obj.Value)
		}
	}
	if cfg.ServerID {
		printReportServerId(ctx, mc, unit, w)
	}
	return nil
}

func printReportServerId(ctx context.Context, mc *modbus.ModbusClient, unit uint8, w io.Writer) {
	rs, err := mc.ReportServerId(ctx, unit)
	if err != nil {
		_, _ = fmt.Fprintf(w, "  FC17 Report Server ID: ⚠️ %v\n", err)
		return
	}
	_, _ = fmt.Fprintf(w, "  FC17 Report Server ID (byte count: %d): % X\n", rs.ByteCount, rs.Data)
}

func printDeviceIdentification(w io.Writer, di *modbus.DeviceIdentification) {
	_, _ = fmt.Fprintf(w, "✅ Device Identification (DevID Code: %d, Conformity Level: 0x%02X, More Follows: 0x%02X, Next Object ID: %d, Object Count: %d)\n",
		di.ReadDeviceIdCode, di.ConformityLevel, di.MoreFollows, di.NextObjectId, len(di.Objects))
	for _, obj := range di.Objects {
		desc := obj.Name
		if desc == "" {
			desc = objectDescription(obj.Id)
		}
		if desc != "" {
			_, _ = fmt.Fprintf(w, " - Object %d: %s [%s]\n", obj.Id, obj.Value, desc)
		} else {
			_, _ = fmt.Fprintf(w, " - Object %d: %s\n", obj.Id, obj.Value)
		}
	}
}

// readFCsForFingerprint are the read-style function codes probed by HasUnitReadFunction (FC08, FC43, FC03, FC04, FC01, FC02, FC11, FC18, FC20).
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

// FingerprintDeviceProbe probes each requested unit with HasUnitReadFunction for supported read FCs and prints results.
// Uses --interval (ms) between probes; no parallel.
func FingerprintDeviceProbe(cfg config.FingerprintConfig) error {
	units, err := parseUnitIDs(cfg.UnitID)
	if err != nil {
		return err
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	fmt.Printf("🔍 Fingerprinting device at %s (supported read functions per unit)...\n", modbusURL)

	conf := &modbus.ClientConfiguration{
		URL:     modbusURL,
		Timeout: time.Duration(cfg.Timeout) * time.Millisecond,
	}
	mc, err := modbus.NewClient(conf)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return fmt.Errorf("%w: %v", ErrTCPConnection, err)
	}
	defer func() { _ = mc.Close() }()

	interval := time.Duration(cfg.Interval) * time.Millisecond
	for i, unit := range units {
		if i > 0 && interval > 0 {
			time.Sleep(interval)
		}
		if len(units) > 1 {
			fmt.Printf("\n--- Unit ID %d ---\n", unit)
		}
		var supported []string
		for _, fc := range readFCsForFingerprint {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
			ok, err := mc.HasUnitReadFunction(ctx, unit, fc)
			cancel()
			if err != nil {
				if len(units) > 1 {
					fmt.Printf("⚠️ Unit %d: %v\n", unit, err)
					break
				}
				return err
			}
			if ok {
				supported = append(supported, fc.String())
			}
			if interval > 0 {
				time.Sleep(interval)
			}
		}
		if len(supported) > 0 {
			fmt.Printf("✅ Unit %d: supported read functions:\n", unit)
			for _, s := range supported {
				fmt.Printf("  %s\n", s)
			}
		} else if len(units) == 1 {
			fmt.Printf("— No supported read functions detected for unit %d\n", unit)
		}
	}
	return nil
}

// RunDiagnostics sends FC08 Diagnostics and prints the response.
func RunDiagnostics(cfg config.DiagnosticConfig) error {
	subFuncCode, err := config.ParseDiagnosticSubFunction(cfg.SubFunction)
	if err != nil {
		return err
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	fmt.Printf("🔍 Sending FC08 Diagnostics to %s (unit %d, sub-function %s / 0x%04X)...\n", modbusURL, cfg.UnitID, cfg.SubFunction, subFuncCode)

	conf := &modbus.ClientConfiguration{
		URL:     modbusURL,
		Timeout: time.Duration(cfg.Timeout) * time.Millisecond,
	}

	mc, err := modbus.NewClient(conf)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	if err := mc.Open(); err != nil {
		return fmt.Errorf("%w: %v", ErrTCPConnection, err)
	}
	defer func() { _ = mc.Close() }()

	var data []byte
	if cfg.Data != "" {
		data, err = hex.DecodeString(cfg.Data)
		if err != nil {
			return fmt.Errorf("invalid hex data: %w", err)
		}
	} else {
		data = []byte{0x00, 0x00}
	}

	sf := modbus.DiagnosticSubFunction(subFuncCode)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
	defer cancel()

	resp, err := mc.Diagnostics(ctx, cfg.UnitID, sf, data)
	if err != nil {
		return fmt.Errorf("FC08 Diagnostics failed: %w", err)
	}

	fmt.Printf("✅ Diagnostics response:\n")
	fmt.Printf("  Sub-function: 0x%04X (%s)\n", uint16(resp.SubFunction), resp.SubFunction)
	fmt.Printf("  Data:         % X\n", resp.Data)
	return nil
}

// RunReportServerId sends FC17 Report Server ID for one or more unit IDs.
func RunReportServerId(cfg config.ReportServerIdConfig) error {
	units, err := parseUnitIDs(cfg.UnitID)
	if err != nil {
		return err
	}
	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	fmt.Printf("🔍 Sending FC17 Report Server ID to %s...\n", modbusURL)

	conf := &modbus.ClientConfiguration{
		URL:     modbusURL,
		Timeout: time.Duration(cfg.Timeout) * time.Millisecond,
	}

	useParallel := len(units) > 1 && cfg.Parallel > 1
	if !useParallel {
		mc, err := modbus.NewClient(conf)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		if err := mc.Open(); err != nil {
			return fmt.Errorf("%w: %v", ErrTCPConnection, err)
		}
		defer func() { _ = mc.Close() }()

		for _, unit := range units {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
			if len(units) > 1 {
				fmt.Printf("\n--- Unit ID %d ---\n", unit)
			}
			rs, err := mc.ReportServerId(ctx, unit)
			cancel()
			if err != nil {
				if len(units) > 1 {
					fmt.Printf("⚠️ Unit %d: %v\n", unit, err)
					continue
				}
				return fmt.Errorf("FC17 Report Server ID failed: %w", err)
			}
			printReportServerIdResult(os.Stdout, unit, rs)
		}
		return nil
	}

	// Parallel path
	n := int(cfg.Parallel)
	if n > len(units) {
		n = len(units)
	}
	clients := make([]*modbus.ModbusClient, 0, n)
	for i := 0; i < n; i++ {
		mc, err := modbus.NewClient(conf)
		if err != nil {
			for _, c := range clients {
				_ = c.Close()
			}
			return fmt.Errorf("failed to create client: %w", err)
		}
		if err := mc.Open(); err != nil {
			for _, c := range clients {
				_ = c.Close()
			}
			return fmt.Errorf("%w: %v", ErrTCPConnection, err)
		}
		clients = append(clients, mc)
	}
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()

	type rsResult struct {
		unit   uint8
		output string
		err    error
	}
	unitsCh := make(chan uint8, len(units))
	for _, u := range units {
		unitsCh <- u
	}
	close(unitsCh)
	resultsCh := make(chan rsResult, len(units))

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		mc := clients[i]
		go func() {
			defer wg.Done()
			for unit := range unitsCh {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
				rs, err := mc.ReportServerId(ctx, unit)
				cancel()
				buf := &bytes.Buffer{}
				if err == nil {
					printReportServerIdResult(buf, unit, rs)
				}
				resultsCh <- rsResult{unit: unit, output: buf.String(), err: err}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var results []rsResult
	for r := range resultsCh {
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].unit < results[j].unit })
	for _, r := range results {
		fmt.Printf("\n--- Unit ID %d ---\n", r.unit)
		if r.err != nil {
			fmt.Printf("⚠️ Unit %d: %v\n", r.unit, r.err)
		} else {
			fmt.Print(r.output)
		}
	}
	return nil
}

func printReportServerIdResult(w io.Writer, unit uint8, rs *modbus.ReportServerIdResponse) {
	_, _ = fmt.Fprintf(w, "✅ Report Server ID (unit %d, byte count: %d):\n", unit, rs.ByteCount)
	_, _ = fmt.Fprintf(w, "  Data: % X\n", rs.Data)
	if len(rs.Data) > 0 {
		// First byte is typically the server ID
		_, _ = fmt.Fprintf(w, "  Server ID: 0x%02X (%d)\n", rs.Data[0], rs.Data[0])
	}
	if len(rs.Data) > 1 {
		// Second byte is the run indicator (0x00=OFF, 0xFF=ON)
		status := "OFF"
		if rs.Data[1] == 0xFF {
			status = "ON"
		}
		_, _ = fmt.Fprintf(w, "  Run Indicator: 0x%02X (%s)\n", rs.Data[1], status)
	}
	if len(rs.Data) > 2 {
		_, _ = fmt.Fprintf(w, "  Additional Data: % X\n", rs.Data[2:])
	}
}
