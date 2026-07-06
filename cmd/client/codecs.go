package client

import (
	"context"
	"sort"
	"strings"

	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/spf13/cobra"
)

var codecsCmd = &cobra.Command{
	Use:   "codecs",
	Short: "List available register codecs for 'client decode'",
	Example: `
  # List all available codecs
  modbusctl client codecs

  # List only float codecs
  modbusctl client codecs --family float

  # List codecs that span two registers as a table
  modbusctl client codecs --registers 2 --format table
`,
}

func init() {
	var (
		family       string
		valueKind    string
		registers    uint16
		outputFormat string
	)
	codecsCmd.Flags().StringVar(&family, "family", "", "Filter by codec family (e.g. integer, float, text, bcd, time, network, bytes, hardware_address, decimal_limb)")
	codecsCmd.Flags().StringVar(&valueKind, "value-kind", "", "Filter by value kind (e.g. uint32, int32, float32, string, time, ip, hardware_addr, byte_slice)")
	codecsCmd.Flags().Uint16Var(&registers, "registers", 0, "Filter by register width (0 = no filter)")
	codecsCmd.Flags().StringVar(&outputFormat, "format", string(format.FormatText), "Output format: text, json, or table")
	runner.RegisterStdoutFormatCompletion(codecsCmd)
	ClientCmd.AddCommand(codecsCmd)

	codecsCmd.RunE = func(cmd *cobra.Command, args []string) error {
		list := buildCodecList(family, valueKind, registers)
		_, err := runner.RunFormatted(cmd, outputFormat, func(context.Context) (any, error) {
			return list, nil
		})
		return err
	}
}

// buildCodecList returns registered codecs filtered by family, value kind, and register width.
func buildCodecList(family, valueKind string, registers uint16) *types.CodecList {
	family = strings.ToLower(strings.TrimSpace(family))
	valueKind = strings.ToLower(strings.TrimSpace(valueKind))

	descs := codec.AvailableCodecDescriptors()
	out := make([]types.CodecInfo, 0, len(descs))
	for _, d := range descs {
		if family != "" && d.Family.String() != family {
			continue
		}
		if valueKind != "" && d.ValueKind.String() != valueKind {
			continue
		}
		if registers != 0 && d.RegisterSpec.Count != registers {
			continue
		}
		out = append(out, types.CodecInfo{
			ID:        d.ID,
			Name:      d.Name,
			Family:    d.Family.String(),
			ValueKind: d.ValueKind.String(),
			Registers: d.RegisterSpec.Count,
			Bytes:     d.ByteSpec.Count,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return &types.CodecList{Codecs: out}
}
