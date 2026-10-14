package mcap

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile"
)

// ExportInfo writes a summary of a capture file: header, targets, streams,
// sample counts, function mix and per-stream poll interval.
func ExportInfo(w io.Writer, path string) error {
	l, err := capfile.Load(path)
	if err != nil {
		return err
	}
	h := l.Header
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }

	p("Capture: %s (%s)\n", path, l.Encoding)
	if h.Producer != "" {
		p("  Producer:  %s\n", h.Producer)
	}
	p("  Start:     %s\n", h.WallStart.Format(time.RFC3339Nano))
	p("  Tick:      %v\n", time.Duration(h.TickNS))
	mode := "full capture"
	if h.ChangeOnly {
		mode = "change-only"
	}
	if h.Exceptions {
		mode += ", exceptions recorded"
	}
	p("  Mode:      %s\n", mode)
	switch {
	case l.Truncated:
		p("  End:       unknown: the file is truncated (last sample at %v)\n", h.Duration(l.End()))
	case !l.Capture.End.Known:
		p("  End:       not recorded (last sample at %v)\n", h.Duration(l.End()))
	default:
		p("  End:       %v\n", h.Duration(l.End()))
	}
	exc := 0
	fcs := map[uint8]int{}
	for i := range l.Samples {
		fcs[l.Samples[i].Function()]++
		if l.Samples[i].IsException() {
			exc++
		}
	}
	p("  Samples:   %d (%d exception responses)\n", len(l.Samples), exc)
	if len(fcs) > 0 {
		var parts []string
		for _, fc := range sortedKeys(fcs) {
			parts = append(parts, fmt.Sprintf("FC%02d x%d", fc, fcs[fc]))
		}
		p("  Functions: %s\n", strings.Join(parts, ", "))
	}

	p("\nTargets:\n")
	for _, t := range l.Targets {
		p("  %d  %s  %s  unit %d\n", t.ID, orDash(quote(t.Label)), endpoint(t), t.Unit)
	}
	p("\nStreams:\n")
	for _, s := range l.Streams {
		n, first, last, interval := streamStats(l.Samples, s.ID)
		p("  %d  target %d  %s  %s  %d samples", s.ID, s.Target, orDash(s.Role), orDash(quote(s.Label)), n)
		if n > 0 {
			p(", %v to %v", h.Duration(first), h.Duration(last))
		}
		if interval > 0 {
			p(", every %v", h.Duration(interval))
		}
		p("\n")
	}
	return nil
}

// streamStats returns the sample count, time span and median interval
// between two samples of the same request of stream.
func streamStats(samples []capture.Sample, stream uint32) (n int, first, last, interval capture.Ticks) {
	lastAt := map[string]capture.Ticks{}
	var gaps []capture.Ticks
	for i := range samples {
		s := &samples[i]
		if s.Stream != stream {
			continue
		}
		if n == 0 || s.At < first {
			first = s.At
		}
		last = max(last, s.At)
		n++
		if prev, ok := lastAt[string(s.Request)]; ok && s.At > prev {
			gaps = append(gaps, s.At-prev)
		}
		lastAt[string(s.Request)] = s.At
	}
	if len(gaps) > 0 {
		sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
		interval = gaps[len(gaps)/2]
	}
	return n, first, last, interval
}

func endpoint(t capture.Target) string {
	if !t.Serial.IsZero() {
		return fmt.Sprintf("%s %s %d baud", t.Transport, t.Serial.Device, t.Serial.Baud)
	}
	if t.Host != "" || t.Port != 0 {
		return fmt.Sprintf("%s %s:%d", t.Transport, t.Host, t.Port)
	}
	return t.Transport.String()
}

func sortedKeys(m map[uint8]int) []uint8 {
	keys := make([]uint8, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func quote(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("%q", s)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
