// SPDX-License-Identifier: MIT

package capfile

import (
	"math"
	"strconv"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
)

// TestTargetFor_serialFieldsThatDoNotFit: the advisory serial fields of a capture
// target are recorded as unknown (0) rather than truncated.
func TestTargetFor_serialFieldsThatDoNotFit(t *testing.T) {
	baud := uint(19200)
	if strconv.IntSize == 64 {
		big := uint64(math.MaxUint32) + 19201 // would truncate to 19200
		baud = uint(big)
	}
	got := TargetFor(1, "rtu:///dev/ttyUSB0", 1, config.SerialConfig{Baud: baud, DataBits: 256 + 8, StopBits: 257}, "")
	if strconv.IntSize == 64 && got.Serial.Baud != 0 {
		t.Errorf("baud %d, want 0", got.Serial.Baud)
	}
	if got.Serial.DataBits != 0 || got.Serial.StopBits != 0 {
		t.Errorf("data bits %d, stop bits %d, want 0", got.Serial.DataBits, got.Serial.StopBits)
	}
	got = TargetFor(1, "rtu:///dev/ttyUSB0", 1, config.SerialConfig{Baud: 9600, DataBits: 7, StopBits: 2}, "")
	if got.Serial.Baud != 9600 || got.Serial.DataBits != 7 || got.Serial.StopBits != 2 {
		t.Errorf("serial %+v", got.Serial)
	}
}
