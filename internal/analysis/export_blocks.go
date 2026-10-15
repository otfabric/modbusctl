package analysis

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// ExportAddressBlocks extracts the distinct address blocks the selected samples of a capture read,
// as JSON: the blocks file format of client record.
func ExportAddressBlocks(w io.Writer, path string, sel config.CaptureSelection) error {
	records, _, err := LoadRecords(path, sel)
	if err != nil {
		return err
	}
	blockSet := make(map[string]types.AddressBlock)
	for _, rec := range records {
		key := fmt.Sprintf("%d:%d", rec.StartAddress, rec.RegisterCount)
		blockSet[key] = types.AddressBlock{
			StartAddress:  rec.StartAddress,
			RegisterCount: rec.RegisterCount,
		}
	}

	var blocks []types.AddressBlock
	for _, b := range blockSet {
		blocks = append(blocks, b)
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].StartAddress < blocks[j].StartAddress
	})

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(blocks); err != nil {
		return fmt.Errorf("failed to write blocks as JSON: %w", err)
	}

	return nil
}
