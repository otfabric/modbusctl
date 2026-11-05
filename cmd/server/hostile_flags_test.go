// SPDX-License-Identifier: MIT

package server

import (
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
)

// TestReplay_nonFiniteSpeedIsRefused: "NaN" and "Inf" parse as floats and passed
// the positive check (NaN <= 0 is false), starting a replay whose time was
// undefined. They exit 3 now, as does a speed above a year per second.
func TestReplay_nonFiniteSpeedIsRefused(t *testing.T) {
	in, _ := recordHolding(t)
	port := freePort(t)
	for _, speed := range []string{"NaN", "Inf", "+Inf", "infinity", "1e300", "31536001"} {
		r := run(t, "server", "replay", "--input", in, "--port", port, "--speed", speed)
		wantExit(t, r, errs.ExitInvalidInput)
		wantContains(t, r.err.Error(), "invalid --speed")
	}
}
