package format

import (
	"fmt"
	"io"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/mcap"
)

// ExportStrings scans each record's data for embedded ASCII substrings and writes them with their address range.
func ExportStrings(w io.Writer, cfg config.StringsConfig) error {
	records, _, err := mcap.LoadRecordsFromMCAP(cfg.InputFile)
	if err != nil {
		return fmt.Errorf("failed to load records: %w", err)
	}

	for _, rec := range stitchAdjacentRecords(records) {
		processASCII(w, &rec)
	}
	return nil
}

// ExportHeuristicFrequency scans the records for potential frequency values and writes them to the provided writer.
func ExportHeuristicFrequency(w io.Writer, path string) error {
	records, _, err := mcap.LoadRecordsFromMCAP(path)
	if err != nil {
		return fmt.Errorf("failed to load records: %w", err)
	}

	matches := processFrequency(stitchAdjacentRecords(records))
	n := len(matches)
	if n > 20 {
		n = 20
	}
	for _, m := range matches[:n] {
		_, _ = fmt.Fprintf(w, "[%d] %d regs (%s) = %.4f → candidate frequency (confidence: %.2f)\n",
			m.Addr, m.Regs, m.Format, m.Value, m.Confidence)
	}
	return nil
}
