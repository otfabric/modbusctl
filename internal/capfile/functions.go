package capfile

import (
	"errors"
	"fmt"

	"github.com/otfabric/go-modbus-capture/capture"
)

// ErrUnsupportedFunction is returned for a request outside the read
// functions of the Modbus specification that modbusctl records and serves.
var ErrUnsupportedFunction = errors.New("unsupported function")

// IsSpecRead reports whether req is a read request of the Modbus
// specification that a capture may hold: FC01–04, FC07, FC08 with a
// data-returning sub-function (Return Query Data, Return Diagnostic Register,
// the counters 0x0B–0x12), FC11, FC12, FC17, FC20, FC24, and FC43 Read Device
// Identification (MEI type 0x0E). Vendor and unknown function codes are not.
func IsSpecRead(req []byte) bool {
	if len(req) == 0 {
		return false
	}
	switch req[0] {
	case 0x01, 0x02, 0x03, 0x04, 0x07, 0x0B, 0x0C, 0x11, 0x14, 0x18:
		return true
	case 0x08:
		return len(req) >= 3 && req[1] == 0 && IsDataDiagnostic(req[2])
	case 0x2B:
		return len(req) >= 2 && req[1] == 0x0E
	}
	return false
}

// IsDataDiagnostic reports whether the FC08 sub-function 0x00sub returns
// data: Return Query Data (0x00), Return Diagnostic Register (0x02) and the
// counters 0x0B–0x12. The others change device state.
func IsDataDiagnostic(sub uint8) bool {
	return sub == 0x00 || sub == 0x02 || sub >= 0x0B && sub <= 0x12
}

// checkFunctions rejects a capture holding a sample outside [IsSpecRead].
func checkFunctions(c *capture.Capture) error {
	for i := range c.Samples {
		if s := &c.Samples[i]; !IsSpecRead(s.Request) {
			return fmt.Errorf("%w: sample %d (stream %d) is function code %d (request % X), which is not a Modbus read function modbusctl supports",
				ErrUnsupportedFunction, i, s.Stream, s.Function(), s.Request)
		}
	}
	return nil
}
