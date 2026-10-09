package types_test

import (
	"testing"

	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestSunSpecDetectOutput(t *testing.T) {
	attempts := []types.SunSpecProbeAttempt{
		{Index: 1, BaseAddress: 0, Matched: false, Result: "no marker"},
		{Index: 2, BaseAddress: 40000, Matched: true, Result: "SunS"},
	}
	t.Run("detected", func(t *testing.T) {
		o := &types.SunSpecDetectOutput{UnitID: 1, Detected: true, BaseAddress: 40000, Attempts: attempts}
		assert.Equal(t, "UNIT  DETECTED  BASE   REGTYPE\n1     yes       40000  holding\n", text(t, o))
		assert.Equal(t, [][]string{{"1", "yes", "40000", "holding"}}, o.TableRows())
	})
	t.Run("not detected on input registers", func(t *testing.T) {
		o := &types.SunSpecDetectOutput{UnitID: 247, Regtype: "input"}
		assert.Equal(t, "UNIT  DETECTED  BASE   REGTYPE\n247   no        0      input\n", text(t, o))
		assert.Equal(t, [][]string{{"247", "no", "0", "input"}}, o.TableRows())
	})
	t.Run("verbose lists attempts", func(t *testing.T) {
		o := &types.SunSpecDetectOutput{UnitID: 1, Regtype: "holding", Verbose: true, Detected: true, BaseAddress: 40000, Attempts: attempts}
		assert.Equal(t,
			"UNIT  DETECTED  BASE   REGTYPE\n"+
				"1     yes       40000  holding\n"+
				"\nATTEMPT   ADDRESS  RESULT\n"+
				"1        0        no marker\n"+
				"2        40000    SunS\n",
			text(t, o))
	})
	t.Run("verbose without attempts", func(t *testing.T) {
		o := &types.SunSpecDetectOutput{UnitID: 1, Verbose: true}
		assert.Equal(t, "UNIT  DETECTED  BASE   REGTYPE\n1     no        0      holding\n", text(t, o))
	})
}

func sunspecModels() []types.SunSpecModelHeader {
	return []types.SunSpecModelHeader{
		{ID: 1, Length: 66, StartAddress: 40002, EndAddress: 40069, NextAddress: 40070},
		{ID: 103, Length: 50, StartAddress: 40070, EndAddress: 40121, NextAddress: 40122},
		{ID: 65535, Length: 0, StartAddress: 40122, EndAddress: 40123, NextAddress: 40124, IsEndModel: true},
	}
}

func TestSunSpecModelsOutput(t *testing.T) {
	t.Run("not detected", func(t *testing.T) {
		o := &types.SunSpecModelsOutput{NotDetected: true, Models: sunspecModels()}
		assert.Equal(t, "SunSpec not detected.\n", text(t, o))
		assert.Nil(t, o.TableRows())
	})
	t.Run("models", func(t *testing.T) {
		o := &types.SunSpecModelsOutput{Base: 40000, Models: sunspecModels()}
		assert.Equal(t,
			"BASE: 40000\n\n"+
				"START   END     MODEL ID  LENGTH  END MODEL\n"+
				"40002   40069   1         66      no\n"+
				"40070   40121   103       50      no\n"+
				"40122   40123   65535     0       yes\n",
			text(t, o))
		assert.Equal(t, [][]string{
			{"40002", "40069", "1", "66", "no"},
			{"40070", "40121", "103", "50", "no"},
			{"40122", "40123", "65535", "0", "yes"},
		}, o.TableRows())
	})
	t.Run("no models", func(t *testing.T) {
		o := &types.SunSpecModelsOutput{Base: 0}
		assert.Equal(t, "BASE: 0\n\nSTART   END     MODEL ID  LENGTH  END MODEL\n", text(t, o))
		assert.Nil(t, o.TableRows())
	})
}

func TestSunSpecMapOutput(t *testing.T) {
	const head = "SunSpec map detected\n  Base marker : 40000\n  Marker regs : 40000-40001\n\nMODEL MAP\n"
	tests := []struct {
		name                     string
		hdr, next, compact       bool
		wantModel1, wantModel103 string
	}{
		{name: "default", wantModel1: "  40002-40069   model 1\n", wantModel103: "  40070-40121   model 103\n"},
		{name: "compact", compact: true, wantModel1: "  40002-40069 m1\n", wantModel103: "  40070-40121 m103\n"},
		{name: "next", next: true, wantModel1: "  40002-40069   model 1 (next 40070)\n", wantModel103: "  40070-40121   model 103 (next 40122)\n"},
		{name: "next overrides compact", next: true, compact: true, wantModel1: "  40002-40069   model 1 (next 40070)\n", wantModel103: "  40070-40121   model 103 (next 40122)\n"},
		{name: "header regs", hdr: true, wantModel1: "  40002-40069   model 1  hdr 40002-40003\n", wantModel103: "  40070-40121   model 103  hdr 40070-40071\n"},
		{name: "header regs and next", hdr: true, next: true, wantModel1: "  40002-40069   model 1  hdr 40002-40003 (next 40070)\n", wantModel103: "  40070-40121   model 103  hdr 40070-40071 (next 40122)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &types.SunSpecMapOutput{
				Base: 40000, MarkerRegs: "40000-40001", Models: sunspecModels(),
				ShowHeaderRegs: tt.hdr, ShowNext: tt.next, Compact: tt.compact,
			}
			assert.Equal(t, head+tt.wantModel1+tt.wantModel103+"  40122-40123   end\n", text(t, o))
			assert.Equal(t, [][]string{
				{"40002", "40069", "1", "no"},
				{"40070", "40121", "103", "no"},
				{"40122", "40123", "65535", "yes"},
			}, o.TableRows())
		})
	}
	t.Run("not detected", func(t *testing.T) {
		o := &types.SunSpecMapOutput{NotDetected: true, Models: sunspecModels()}
		assert.Equal(t, "SunSpec not detected.\n", text(t, o))
		assert.Nil(t, o.TableRows())
	})
}

func TestSunSpecProbeOutput(t *testing.T) {
	t.Run("detected", func(t *testing.T) {
		o := &types.SunSpecProbeOutput{
			Target: "tcp://h:502", UnitID: 3,
			Modbus:        types.SunSpecProbeModbus{FC03: true, FC43: true},
			SunSpecDetail: types.SunSpecProbeSummary{Detected: true, BaseAddress: 40000, ModelsFound: 4, EndModel: true},
		}
		assert.Equal(t,
			"TARGET\n"+
				"  URL    : tcp://h:502\n"+
				"  UNIT   : 3\n"+
				"\nMODBUS\n"+
				"  FC03   : supported\n"+
				"  FC04   : not supported\n"+
				"  FC43   : supported\n"+
				"\nSUNSPEC\n"+
				"  detected     : yes\n"+
				"  base address : 40000\n"+
				"  models found : 4\n"+
				"  end model    : true\n",
			text(t, o))
		assert.Equal(t, [][]string{
			{"target", "url", "tcp://h:502"},
			{"target", "unit_id", "3"},
			{"modbus", "fc03", "true"},
			{"modbus", "fc04", "false"},
			{"modbus", "fc43", "true"},
			{"sunspec", "detected", "true"},
			{"sunspec", "base_address", "40000"},
			{"sunspec", "models_found", "4"},
			{"sunspec", "end_model", "true"},
		}, o.TableRows())
		assert.False(t, o.HasPartialFailure())
	})
	t.Run("every error", func(t *testing.T) {
		o := &types.SunSpecProbeOutput{
			Target: "tcp://h:502", UnitID: 1,
			Modbus: types.SunSpecProbeModbus{
				FC03: true, FC03Error: errInfo("e03"), FC04Error: errInfo("e04"), FC43Error: errInfo("e43"),
			},
			SunSpecDetail: types.SunSpecProbeSummary{DetectionError: errInfo("edet"), ModelHeadersError: errInfo("ehdr")},
		}
		assert.Equal(t,
			"TARGET\n"+
				"  URL    : tcp://h:502\n"+
				"  UNIT   : 1\n"+
				"\nMODBUS\n"+
				"  FC03   : error: e03\n"+
				"  FC04   : error: e04\n"+
				"  FC43   : error: e43\n"+
				"\nSUNSPEC\n"+
				"  detection error : edet\n"+
				"  model headers error : ehdr\n"+
				"  detected     : no\n",
			text(t, o))
		assert.Equal(t, [][]string{
			{"target", "url", "tcp://h:502"},
			{"target", "unit_id", "1"},
			{"modbus", "fc03", "true"},
			{"modbus", "fc04", "false"},
			{"modbus", "fc43", "false"},
			{"modbus", "fc03_error", "e03"},
			{"modbus", "fc04_error", "e04"},
			{"modbus", "fc43_error", "e43"},
			{"sunspec", "detection_error", "edet"},
			{"sunspec", "model_headers_error", "ehdr"},
			{"sunspec", "detected", "false"},
			{"sunspec", "base_address", "0"},
			{"sunspec", "models_found", "0"},
			{"sunspec", "end_model", "false"},
		}, o.TableRows())
		assert.True(t, o.HasPartialFailure())
	})
}
