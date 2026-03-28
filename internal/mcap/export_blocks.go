package mcap

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/otfabric/modbusctl/internal/types"
)

// ExportAddressBlocks extracts a unique set of address blocks from an MCAP file and writes them as JSON to the provided writer.
func ExportAddressBlocks(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open mcap file: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, err = ReadHeader(f)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	blockSet := make(map[string]types.AddressBlock)
	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read record: %w", err)
		}
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
