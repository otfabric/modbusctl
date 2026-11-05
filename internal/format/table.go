// SPDX-License-Identifier: MIT

package format

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/otfabric/modbusctl/internal/types"
)

// safeRows returns rows with every cell made safe for a terminal ([types.TerminalSafe]):
// a cell holding a tab or a newline from a device would break the columns, and escape
// sequences would reach the terminal.
func safeRows(rows [][]string) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		out[i] = make([]string, len(row))
		for j, c := range row {
			out[i][j] = types.TerminalSafe(c)
		}
	}
	return out
}

// RenderTable renders headers and rows as column-aligned text using tabwriter.
func RenderTable(m TableMarshaler) (string, error) {
	headers := m.TableHeaders()
	rows := safeRows(m.TableRows())
	if len(headers) == 0 {
		return "", fmt.Errorf("table has no headers")
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	rule := make([]string, len(headers))
	for i := range rule {
		rule[i] = "-"
	}
	_, _ = fmt.Fprintln(tw, strings.Join(rule, "\t"))
	for _, row := range rows {
		if len(row) != len(headers) {
			return "", fmt.Errorf("table row has %d columns, want %d", len(row), len(headers))
		}
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return "", err
	}
	return b.String(), nil
}
