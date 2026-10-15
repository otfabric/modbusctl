package capfile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/otfabric/go-modbus-capture/capture"
)

// ErrUnsupportedFunction is returned for a request outside the read
// functions of the Modbus specification that modbusctl records and serves.
var ErrUnsupportedFunction = errors.New("unsupported function")

// IsSpecRead reports whether req is a read request of the Modbus
// specification that a capture may hold: a function of
// [capture.IsReadFunction] (FC01–04, FC07, FC08, FC11, FC12, FC17, FC20,
// FC24, FC43), FC08 with a data-returning sub-function only (see
// [IsDataDiagnostic]) and FC43 with MEI type 14 (Read Device Identification)
// only. These are the rules capture.CheckPDUs applies to every sample read or
// written. Vendor and unknown function codes are not supported.
func IsSpecRead(req []byte) bool {
	if len(req) == 0 || !capture.IsReadFunction(req[0]) {
		return false
	}
	switch req[0] {
	case 0x08:
		return len(req) >= 3 && req[1] == 0 && IsDataDiagnostic(req[2])
	case 0x2B:
		return len(req) >= 2 && req[1] == 0x0E
	}
	return true
}

// IsDataDiagnostic reports whether the FC08 sub-function 0x00sub returns
// data: Return Query Data (0x00), Return Diagnostic Register (0x02) and the
// counters 0x0B–0x12. The others change device state.
func IsDataDiagnostic(sub uint8) bool {
	return sub == 0x00 || sub == 0x02 || sub >= 0x0B && sub <= 0x12
}

// unsupportedSample reports whether err is go-modbus-capture rejecting a
// sample for its function code: capture.CheckPDUs, which every reader applies,
// reports a write, a function outside the read functions of the specification,
// a state-changing FC08 sub-function or an FC43 MEI type other than 14 as
// capture.ErrCorrupt with these messages (go-modbus-capture v0.0.2). Load
// reports them as ErrUnsupportedFunction, the clearer error for a capture that
// is well formed but holds what modbusctl does not serve.
func unsupportedSample(err error) bool {
	if !errors.Is(err, capture.ErrCorrupt) {
		return false
	}
	msg := err.Error()
	for _, m := range []string{
		"is a write; captures hold reads only",
		"is not a read function of the Modbus specification",
		"FC08 request without a data-returning sub-function",
		"FC43 request with an MEI type other than 14",
	} {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// checkFunctions rejects a capture holding a sample outside [IsSpecRead]. The
// readers of go-modbus-capture v0.0.2 already refuse such samples (see
// unsupportedSample); this keeps Load's guarantee independent of that.
func checkFunctions(c *capture.Capture) error {
	for i := range c.Samples {
		if s := &c.Samples[i]; !IsSpecRead(s.Request) {
			return fmt.Errorf("%w: sample %d (stream %d) is function code %d (request % X), which is not a Modbus read function modbusctl supports",
				ErrUnsupportedFunction, i, s.Stream, s.Function(), s.Request)
		}
	}
	return nil
}
