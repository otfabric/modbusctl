// SPDX-License-Identifier: MIT

package sunspec

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
)

// sunSpecDevice serves a SunSpec map at 40000: common model 1 (4 registers), model 103
// (2 registers) and the end model. Only unit 1 answers.
func sunSpecDevice(t *testing.T) (*mbtest.Device, string) {
	t.Helper()
	d := mbtest.NewDevice().
		SetHolding(40000, 0x5375, 0x6E53, 1, 4, 11, 12, 13, 14, 103, 2, 21, 22, 0xFFFF, 0).
		SetInput(0, 1, 2)
	d.Identification = map[uint8]string{0: "Acme", 1: "PV-1", 2: "1.0"}
	d.Units = []uint8{1}
	return d, "tcp://" + d.ServeTCP(t)
}

// plainDevice serves holding registers at every default base, none holding a SunSpec marker.
func plainDevice(t *testing.T) string {
	t.Helper()
	d := mbtest.NewDevice().SetInput(0, 1, 2)
	for _, base := range []uint16{0, 39999, 49999} {
		d.SetHolding(base, 1, 2, 3, 4)
	}
	return "tcp://" + d.ServeTCP(t)
}

var wantModels = []types.SunSpecModelHeader{
	{ID: 1, Length: 4, StartAddress: 40002, EndAddress: 40007, NextAddress: 40008},
	{ID: 103, Length: 2, StartAddress: 40008, EndAddress: 40011, NextAddress: 40012},
	{ID: 0xFFFF, Length: 0, StartAddress: 40012, EndAddress: 40013, NextAddress: 40014, IsEndModel: true},
}

func TestDetect(t *testing.T) {
	_, url := sunSpecDevice(t)

	r := run(t, "sunspec", "detect", "--url", url)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "DETECTED", "yes", "40000", "holding")
	if strings.Contains(r.stdout, "ATTEMPT") {
		t.Fatalf("attempts are only listed with --verbose:\n%s", r.stdout)
	}

	r = run(t, "sunspec", "detect", "--url", url, "--verbose", "--bases", "0,40000")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "ATTEMPT", "matched SunS", "Illegal Data Address")

	r = run(t, "sunspec", "detect", "--url", url, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var det types.SunSpecDetectOutput
	decodeJSON(t, r.stdout, &det)
	if det.Target != url || det.UnitID != 1 || det.Regtype != "holding" || !det.Detected || det.BaseAddress != 40000 {
		t.Fatalf("unexpected detect output: %+v", det)
	}
	if len(det.Attempts) != 2 || det.Attempts[0].Matched || det.Attempts[0].BaseAddress != 0 ||
		!det.Attempts[1].Matched || det.Attempts[1].BaseAddress != 40000 || det.Attempts[1].Result != "matched SunS" {
		t.Fatalf("unexpected attempts: %+v", det.Attempts)
	}

	r = run(t, "sunspec", "detect", "--url", url, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "base_address", "yes", "40000")
}

func TestDetect_notSunSpec(t *testing.T) {
	url := plainDevice(t)
	r := run(t, "sunspec", "detect", "--url", url, "--bases", "0,39999,49999", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var det types.SunSpecDetectOutput
	decodeJSON(t, r.stdout, &det)
	if det.Detected || len(det.Attempts) != 3 {
		t.Fatalf("unexpected detect output: %+v", det)
	}
	for _, a := range det.Attempts {
		if a.Matched || a.Result != "no marker" {
			t.Fatalf("unexpected attempt: %+v", a)
		}
	}

	r = run(t, "sunspec", "detect", "--url", url, "--bases", "0,39999,49999", "--verbose")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "no", "no marker")
}

func TestModels(t *testing.T) {
	_, url := sunSpecDevice(t)

	r := run(t, "sunspec", "models", "--url", url)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "BASE: 40000", "MODEL ID", "103", "65535")

	for _, args := range [][]string{
		{"sunspec", "models", "--url", url, "--format", "json"},
		{"sunspec", "models", "--url", url, "--base", "40000", "--format", "json"},
		// The hidden, deprecated parent flag still selects JSON.
		{"sunspec", "--json", "models", "--url", url},
	} {
		r = run(t, args...)
		wantExit(t, r, errs.ExitOK)
		var out types.SunSpecModelsOutput
		decodeJSON(t, r.stdout, &out)
		if out.Target != url || out.Base != 40000 || out.NotDetected || !reflect.DeepEqual(out.Models, wantModels) {
			t.Fatalf("%v: unexpected models output: %+v", args, out)
		}
	}

	r = run(t, "sunspec", "models", "--url", url, "--max-models", "1", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var limited types.SunSpecModelsOutput
	decodeJSON(t, r.stdout, &limited)
	if len(limited.Models) != 1 || limited.Models[0].ID != 1 {
		t.Fatalf("--max-models 1 returned %+v", limited.Models)
	}

	r = run(t, "sunspec", "models", "--url", url, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "model_id", "end_model", "40008", "103")
}

func TestMap(t *testing.T) {
	_, url := sunSpecDevice(t)

	r := run(t, "sunspec", "map", "--url", url)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "SunSpec map detected", "Base marker : 40000", "Marker regs : 40000-40001", "40002-40007   model 1", "40008-40011   model 103", "40012-40013   end")

	r = run(t, "sunspec", "map", "--url", url, "--show-header-regs", "--show-next")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "hdr 40002-40003", "(next 40008)")

	r = run(t, "sunspec", "map", "--url", url, "--compact")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "40002-40007 m1", "40008-40011 m103")

	r = run(t, "sunspec", "map", "--url", url, "--base", "40000", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var out types.SunSpecMapOutput
	decodeJSON(t, r.stdout, &out)
	if out.Target != url || out.Base != 40000 || out.MarkerRegs != "40000-40001" || !reflect.DeepEqual(out.Models, wantModels) {
		t.Fatalf("unexpected map output: %+v", out)
	}

	r = run(t, "sunspec", "map", "--url", url, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "model_id", "40012", "yes")
}

func TestModelsAndMap_notSunSpec(t *testing.T) {
	url := plainDevice(t)
	for _, sub := range []string{"models", "map"} {
		for _, format := range []string{"text", "json", "table"} {
			r := run(t, "sunspec", sub, "--url", url, "--format", format)
			if r.err != nil {
				// Default bases the device does not map answer with an exception: a protocol failure.
				wantKind(t, r, errs.KindProtocol)
				continue
			}
			if format == "json" && !strings.Contains(r.stdout, `"not_detected": true`) {
				t.Fatalf("sunspec %s on a non-SunSpec device:\n%s", sub, r.stdout)
			}
		}
	}
}

func TestProbe(t *testing.T) {
	_, url := sunSpecDevice(t)

	r := run(t, "sunspec", "probe", "--url", url)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "URL    : "+url, "FC03   : supported", "FC04   : supported", "FC43   : supported", "base address : 40000", "models found : 3", "end model    : true")

	r = run(t, "sunspec", "probe", "--url", url, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var out types.SunSpecProbeOutput
	decodeJSON(t, r.stdout, &out)
	if out.Target != url || out.UnitID != 1 || !out.Modbus.FC03 || !out.Modbus.FC04 || !out.Modbus.FC43 {
		t.Fatalf("unexpected probe output: %+v", out)
	}
	if s := out.SunSpecDetail; !s.Detected || s.BaseAddress != 40000 || s.ModelsFound != 3 || !s.EndModel || s.DetectionError != nil {
		t.Fatalf("unexpected sunspec summary: %+v", s)
	}

	r = run(t, "sunspec", "probe", "--url", url, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "section", "models_found", "base_address")
}

func TestProbe_partial(t *testing.T) {
	_, url := sunSpecDevice(t)
	// Unit 2 does not answer: the probe result is written with an embedded detection error.
	for _, format := range []string{"text", "json", "table"} {
		r := run(t, "sunspec", "probe", "--url", url, "--unit", "2", "--format", format)
		if r.err != nil {
			t.Fatalf("--format %s: an embedded failure must not be fatal: %v", format, r.err)
		}
		wantExit(t, r, errs.ExitPartial)
		wantContains(t, r.stdout, "Gateway Target Failed To Respond")
		if format == "json" {
			var out types.SunSpecProbeOutput
			decodeJSON(t, r.stdout, &out)
			if out.UnitID != 2 || out.SunSpecDetail.Detected || out.SunSpecDetail.DetectionError == nil {
				t.Fatalf("unexpected probe output: %+v", out)
			}
		}
	}
}

func TestFailures(t *testing.T) {
	_, url := sunSpecDevice(t)
	for _, sub := range []string{"detect", "models", "map", "probe"} {
		wantKind(t, run(t, "sunspec", sub), errs.KindInvalidInput)
		wantKind(t, run(t, "sunspec", sub, "--url", url, "--regtype", "coil"), errs.KindInvalidInput)
		wantKind(t, run(t, "sunspec", sub, "--url", url, "--format", "xml"), errs.KindInvalidInput)
		wantKind(t, run(t, "sunspec", sub, "--url", url, "--ip", "127.0.0.1"), errs.KindInvalidInput)
		wantKind(t, run(t, "sunspec", sub, "--url", url, "--timeout", "20"), errs.KindInvalidInput)
		r := run(t, "sunspec", sub, "--url", refusedURL(t))
		wantKind(t, r, errs.KindTransport)
		wantExit(t, r, errs.ExitTransport)
	}
	wantKind(t, run(t, "sunspec", "detect", "--url", url, "--bases", "abc"), errs.KindInvalidInput)

	// A unit that does not answer makes detection itself fail.
	for _, sub := range []string{"detect", "models", "map"} {
		r := run(t, "sunspec", sub, "--url", url, "--unit", "2")
		wantKind(t, r, errs.KindProtocol)
		wantExit(t, r, errs.ExitProtocol)
	}
}

func TestInputRegisters(t *testing.T) {
	d := mbtest.NewDevice().SetInput(40000, 0x5375, 0x6E53, 1, 2, 7, 8, 0xFFFF, 0)
	url := "tcp://" + d.ServeTCP(t)
	r := run(t, "--debug", "sunspec", "models", "--url", url, "--regtype", "input", "--base", "40000", "--format", "json", "--timeout", "500")
	wantExit(t, r, errs.ExitOK)
	var out types.SunSpecModelsOutput
	decodeJSON(t, r.stdout, &out)
	if len(out.Models) != 2 || out.Models[0].ID != 1 || !out.Models[1].IsEndModel {
		t.Fatalf("unexpected models over input registers: %+v", out.Models)
	}
	if d.RequestCount(0x04) == 0 || d.RequestCount(0x03) != 0 {
		t.Fatalf("expected only FC4 requests, got FC3=%d FC4=%d", d.RequestCount(0x03), d.RequestCount(0x04))
	}
}

func TestOutputWriteFailure(t *testing.T) {
	_, url := sunSpecDevice(t)
	for _, sub := range []string{"detect", "models", "map", "probe"} {
		r := runTo(t, context.Background(), failWriter{}, "sunspec", sub, "--url", url)
		wantKind(t, r, errs.KindOutput)
	}
}
