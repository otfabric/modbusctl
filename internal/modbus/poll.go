// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/types"
)

// pollTimestampLayout is RFC 3339 in UTC with millisecond precision.
const pollTimestampLayout = "2006-01-02T15:04:05.000Z07:00"

// pollReadAttempts lets a poll survive one dropped connection: the failed attempt re-dials
// and the second one reads.
const pollReadAttempts = 2

// ResolvePollCodec returns the codec selected by cfg (nil when none) and the number of
// coils/registers each poll reads: cfg.RegisterCount, or one codec value when that is 0.
func ResolvePollCodec(cfg config.PollConfig) (rc codec.RuntimeCodec, count uint16, err error) {
	id := strings.TrimSpace(cfg.Codec)
	if id == "" {
		return nil, cfg.RegisterCount, nil
	}
	rc, ok, err := codec.RuntimeCodecByID(id)
	if err != nil {
		return nil, 0, fmt.Errorf("codec %q could not be constructed: %v", id, err)
	}
	if !ok {
		return nil, 0, fmt.Errorf("unknown codec %q (run 'modbusctl client codecs' to list available codecs)", id)
	}
	per := rc.RegisterSpec().Count
	if per == 0 {
		return nil, 0, fmt.Errorf("codec %q has zero register width", id)
	}
	count = cfg.RegisterCount
	if count == 0 {
		count = per
	} else if count%per != 0 {
		return nil, 0, fmt.Errorf("register count %d must be a multiple of the codec register width %d for %q", count, per, id)
	}
	return rc, count, nil
}

// Poll reads the configured block every cfg.Interval milliseconds and passes each sample to
// emit, until cfg.Polls samples were taken, ctx is cancelled, or emit fails.
//
// A failed read (timeout, exception, lost connection) does not stop the run: it is emitted
// as a sample carrying the error and counted in the summary. Cancelling ctx is the normal
// way to end an unbounded run and is not an error; the summary then has Interrupted set.
// Only a failure to connect in the first place, or an emit error, is returned.
func Poll(ctx context.Context, cfg config.PollConfig, progress io.Writer, emit func(*types.PollSample) error) (*types.PollSummary, error) {
	if progress == nil {
		progress = io.Discard
	}
	rc, count, err := ResolvePollCodec(cfg)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	step := uint16(1)
	if rc != nil {
		step = rc.RegisterSpec().Count
	}

	modbusURL := config.ModbusURL(cfg.URL, cfg.IP, cfg.Port)
	link, err := openDeviceLink(modbusURL, cfg.Timeout, cfg.Debug, cfg.SerialConfig, cfg.TLSConfig)
	if err != nil {
		return nil, err
	}
	defer link.Close()

	summary := &types.PollSummary{Target: modbusURL}
	interval := time.Duration(cfg.Interval) * time.Millisecond
	started := time.Now()
	finish := func(interrupted bool) (*types.PollSummary, error) {
		summary.Interrupted = interrupted
		summary.Duration = time.Since(started).Round(time.Millisecond).String()
		return summary, nil
	}

	for n := uint32(1); cfg.Polls == 0 || n <= cfg.Polls; n++ {
		if ctx.Err() != nil {
			return finish(true)
		}
		pollStart := time.Now()
		sample := &types.PollSample{
			Poll:         n,
			Timestamp:    pollStart.UTC().Format(pollTimestampLayout),
			UnitID:       cfg.Unit,
			Function:     cfg.Function,
			StartAddress: cfg.StartAddress,
			Count:        count,
			AddressStep:  step,
			Codec:        strings.TrimSpace(cfg.Codec),
			Values:       []any{},
		}

		raw, reqTS, resTS, rerr := readRegisters(ctx, link, cfg.Function, cfg.StartAddress, count, pollReadAttempts, cfg.Unit, 0, progress)
		if rerr == nil {
			sample.RawDataHex = fmt.Sprintf("% X", raw)
			sample.Values, rerr = pollValues(raw, cfg.Function, count, rc)
		}
		if resTS > reqTS {
			sample.RTTMs = float64(resTS-reqTS) / float64(time.Millisecond)
		}
		if rerr != nil {
			// A read cut short by cancellation is the end of the run, not a failed poll.
			if ctx.Err() != nil {
				return finish(true)
			}
			sample.Values = []any{}
			sample.Error = EmbeddedErrorInfo(rerr)
			summary.Failed++
		} else {
			summary.Succeeded++
		}
		summary.Polls++

		if err := emit(sample); err != nil {
			return nil, err
		}
		if cfg.Polls != 0 && n == cfg.Polls {
			break
		}
		// Polls start on a fixed cadence; a slow poll is followed by the next one at once.
		if err := sleepContext(ctx, interval-time.Since(pollStart)); err != nil {
			return finish(true)
		}
	}
	return finish(false)
}

// pollValues converts raw read bytes into sample values: booleans for coils and discrete
// inputs, register values for holding/input registers, or codec-decoded values.
func pollValues(raw []byte, fc uint8, count uint16, rc codec.RuntimeCodec) ([]any, error) {
	if fc == 1 || fc == 2 {
		values := make([]any, 0, count)
		for i := 0; i < int(count) && i/8 < len(raw); i++ {
			values = append(values, raw[i/8]&(1<<(i%8)) != 0)
		}
		return values, nil
	}

	regs := bytesToRegisters(raw)
	if rc == nil {
		values := make([]any, 0, len(regs))
		for _, r := range regs {
			values = append(values, r)
		}
		return values, nil
	}
	per := int(rc.RegisterSpec().Count)
	values := make([]any, 0, len(regs)/per)
	for i := 0; i+per <= len(regs); i += per {
		v, err := codec.DecodeRegistersAny(regs[i:i+per], rc)
		if err != nil {
			return nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("decode with codec %q failed: %v", rc.ID(), err), err)
		}
		values = append(values, normalizeDecoded(v))
	}
	return values, nil
}
