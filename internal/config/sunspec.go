package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSunSpecBases parses a comma-separated list of base addresses (e.g. "0,40000,50000") into []uint16.
// Returns an error if any token is not a valid uint16.
func ParseSunSpecBases(bases string) ([]uint16, error) {
	var out []uint16
	seen := make(map[uint16]struct{})
	for _, s := range strings.Split(bases, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		v, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid base address %q: %w", s, err)
		}
		u := uint16(v)
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out, nil
}
