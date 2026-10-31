// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"strings"
)

// NormalizeParity maps a --parity value to its canonical form ("none", "even" or "odd").
// Matching is case-insensitive; the single letters n, e and o are accepted, and an empty
// value means none.
func NormalizeParity(parity string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(parity)) {
	case "", "n", "none":
		return "none", nil
	case "e", "even":
		return "even", nil
	case "o", "odd":
		return "odd", nil
	default:
		return "", fmt.Errorf("invalid parity %q: use none, even, or odd", parity)
	}
}

// ValidateSerialConfig checks the serial-line settings. Zero values are valid and select defaults.
func ValidateSerialConfig(cfg SerialConfig) error {
	if _, err := NormalizeParity(cfg.Parity); err != nil {
		return err
	}
	if cfg.DataBits != 0 && (cfg.DataBits < 5 || cfg.DataBits > 8) {
		return fmt.Errorf("invalid data bits %d: use 5, 6, 7 or 8 (0 = default)", cfg.DataBits)
	}
	if cfg.StopBits > 2 {
		return fmt.Errorf("invalid stop bits %d: use 1 or 2 (0 = default)", cfg.StopBits)
	}
	return nil
}
