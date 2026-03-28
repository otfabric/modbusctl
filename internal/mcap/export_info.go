package mcap

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/otfabric/modbusctl/internal/types"
)

// ExportInfo reads the header and records from an MCAP file and writes a summary to the provided writer.
func ExportInfo(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open mcap file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header, err := ReadHeader(f)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	_, _ = fmt.Fprintf(w, "Header Information:\n")
	_, _ = fmt.Fprintf(w, "  IP:        %s\n", header.IP)
	_, _ = fmt.Fprintf(w, "  Port:      %d\n", header.Port)
	_, _ = fmt.Fprintf(w, "  Unit:      %d\n", header.Unit)
	_, _ = fmt.Fprintf(w, "  Function:  %d\n", header.Function)
	_, _ = fmt.Fprintf(w, "  StartTime: %s\n", time.Unix(0, header.StartTime).Format(time.RFC3339Nano))
	_, _ = fmt.Fprintf(w, "\n")

	iterDetails := make(map[uint32]*types.IterationDetail)
	var durations []time.Duration

	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read record: %w", err)
		}

		detail, ok := iterDetails[rec.Iteration]
		if !ok {
			detail = &types.IterationDetail{
				FirstRequestTS: rec.RequestTimestamp,
				LastResponseTS: rec.ResponseTimestamp,
				MinAddr:        rec.StartAddress,
				MaxAddr:        rec.StartAddress + rec.RegisterCount - 1,
			}
			iterDetails[rec.Iteration] = detail
		}
		detail.BlockCount++
		detail.TotalRegisters += int(rec.RegisterCount)
		if rec.RequestTimestamp < detail.FirstRequestTS {
			detail.FirstRequestTS = rec.RequestTimestamp
		}
		if rec.ResponseTimestamp > detail.LastResponseTS {
			detail.LastResponseTS = rec.ResponseTimestamp
		}
		if rec.StartAddress < detail.MinAddr {
			detail.MinAddr = rec.StartAddress
		}
		if end := rec.StartAddress + rec.RegisterCount - 1; end > detail.MaxAddr {
			detail.MaxAddr = end
		}
	}

	_, _ = fmt.Fprintf(w, "Record Summary:\n")
	_, _ = fmt.Fprintf(w, "  Iterations: %d\n", len(iterDetails))

	var minDur, maxDur time.Duration
	var totalDur time.Duration
	first := true

	var sortedIters []uint32
	for iter := range iterDetails {
		sortedIters = append(sortedIters, iter)
	}
	sort.Slice(sortedIters, func(i, j int) bool {
		return sortedIters[i] < sortedIters[j]
	})

	for _, iter := range sortedIters {
		d := iterDetails[iter]
		duration := time.Duration(d.LastResponseTS - d.FirstRequestTS)
		durations = append(durations, duration)
		if first {
			minDur = duration
			maxDur = duration
			totalDur = duration
			first = false
		} else {
			if duration < minDur {
				minDur = duration
			}
			if duration > maxDur {
				maxDur = duration
			}
			totalDur += duration
		}

		_, _ = fmt.Fprintf(w, "    Iteration %d:\n", iter)
		_, _ = fmt.Fprintf(w, "      Blocks: %d\n", d.BlockCount)
		_, _ = fmt.Fprintf(w, "      Total Registers: %d\n", d.TotalRegisters)
		_, _ = fmt.Fprintf(w, "      Time: %s → %s (duration: %dms)\n",
			time.Unix(0, d.FirstRequestTS).Format(time.RFC3339Nano),
			time.Unix(0, d.LastResponseTS).Format(time.RFC3339Nano),
			duration.Milliseconds())
		_, _ = fmt.Fprintf(w, "      Address Range: %d → %d\n", d.MinAddr, d.MaxAddr)
	}

	if len(durations) > 0 {
		avgDur := totalDur / time.Duration(len(durations))
		_, _ = fmt.Fprintf(w, "  Iteration Durations:\n")
		_, _ = fmt.Fprintf(w, "    Min: %v\n", minDur)
		_, _ = fmt.Fprintf(w, "    Avg: %v\n", avgDur)
		_, _ = fmt.Fprintf(w, "    Max: %v\n", maxDur)
	} else {
		_, _ = fmt.Fprintf(w, "  No records found.\n")
	}

	return nil
}
