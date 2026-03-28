package types

import (
	"fmt"
	"strings"
)

// SunSpecDetectOutput is the result of SunSpec marker detection.
type SunSpecDetectOutput struct {
	Target      string                `json:"target"`
	UnitID      uint8                 `json:"unit_id"`
	Regtype     string                `json:"regtype"`
	Verbose     bool                  `json:"-"`
	Detected    bool                  `json:"detected"`
	BaseAddress uint16                `json:"base_address"`
	Attempts    []SunSpecProbeAttempt `json:"attempts,omitempty"`
}

// SunSpecProbeAttempt is one base-address probe during detection.
type SunSpecProbeAttempt struct {
	Index       int    `json:"index"`
	BaseAddress uint16 `json:"base_address"`
	Matched     bool   `json:"matched"`
	Result      string `json:"result"`
}

// MarshalTextOutput matches the historical detect table layout.
func (o *SunSpecDetectOutput) MarshalTextOutput() (string, error) {
	if o == nil {
		return "", nil
	}
	var b strings.Builder
	retypeStr := "holding"
	if o.Regtype == "input" {
		retypeStr = "input"
	}
	detected := "no"
	if o.Detected {
		detected = "yes"
	}
	_, _ = fmt.Fprintf(&b, "UNIT  DETECTED  BASE   REGTYPE\n")
	_, _ = fmt.Fprintf(&b, "%-5d %-9s %-6d %s\n", o.UnitID, detected, o.BaseAddress, retypeStr)
	if o.Verbose && len(o.Attempts) > 0 {
		_, _ = fmt.Fprintf(&b, "\nATTEMPT   ADDRESS  RESULT\n")
		for _, a := range o.Attempts {
			_, _ = fmt.Fprintf(&b, "%-8d %-8d %s\n", a.Index, a.BaseAddress, a.Result)
		}
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (o *SunSpecDetectOutput) TableHeaders() []string {
	return []string{"unit_id", "detected", "base_address", "regtype"}
}

// TableRows implements format.TableMarshaler.
func (o *SunSpecDetectOutput) TableRows() [][]string {
	if o == nil {
		return nil
	}
	d := "no"
	if o.Detected {
		d = "yes"
	}
	rt := o.Regtype
	if rt == "" {
		rt = "holding"
	}
	return [][]string{{
		fmt.Sprintf("%d", o.UnitID),
		d,
		fmt.Sprintf("%d", o.BaseAddress),
		rt,
	}}
}

// SunSpecModelHeader is a stable JSON view of a SunSpec model header.
type SunSpecModelHeader struct {
	ID           uint16 `json:"id"`
	Length       uint16 `json:"length"`
	StartAddress uint16 `json:"start_address"`
	EndAddress   uint16 `json:"end_address"`
	NextAddress  uint16 `json:"next_address"`
	IsEndModel   bool   `json:"is_end_model"`
}

// SunSpecModelsOutput lists model headers at a base address.
type SunSpecModelsOutput struct {
	Target      string               `json:"target"`
	Base        uint16               `json:"base"`
	Models      []SunSpecModelHeader `json:"models"`
	NotDetected bool                 `json:"not_detected,omitempty"`
}

// MarshalTextOutput matches historical models output.
func (o *SunSpecModelsOutput) MarshalTextOutput() (string, error) {
	if o == nil {
		return "", nil
	}
	var b strings.Builder
	if o.NotDetected {
		_, _ = fmt.Fprintf(&b, "SunSpec not detected.\n")
		return b.String(), nil
	}
	_, _ = fmt.Fprintf(&b, "BASE: %d\n\n", o.Base)
	_, _ = fmt.Fprintf(&b, "START   END     MODEL ID  LENGTH  END MODEL\n")
	for _, m := range o.Models {
		endYes := "no"
		if m.IsEndModel {
			endYes = "yes"
		}
		_, _ = fmt.Fprintf(&b, "%-7d %-7d %-9d %-7d %s\n", m.StartAddress, m.EndAddress, m.ID, m.Length, endYes)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (o *SunSpecModelsOutput) TableHeaders() []string {
	return []string{"start", "end", "model_id", "length", "end_model"}
}

// TableRows implements format.TableMarshaler.
func (o *SunSpecModelsOutput) TableRows() [][]string {
	if o == nil || o.NotDetected {
		return nil
	}
	var rows [][]string
	for _, m := range o.Models {
		em := "no"
		if m.IsEndModel {
			em = "yes"
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", m.StartAddress),
			fmt.Sprintf("%d", m.EndAddress),
			fmt.Sprintf("%d", m.ID),
			fmt.Sprintf("%d", m.Length),
			em,
		})
	}
	return rows
}

// SunSpecMapOutput is the human / JSON map view of models.
type SunSpecMapOutput struct {
	Target         string               `json:"target"`
	Base           uint16               `json:"base"`
	MarkerRegs     string               `json:"marker_regs"`
	Models         []SunSpecModelHeader `json:"models"`
	NotDetected    bool                 `json:"not_detected,omitempty"`
	ShowHeaderRegs bool                 `json:"-"`
	ShowNext       bool                 `json:"-"`
	Compact        bool                 `json:"-"`
}

// MarshalTextOutput matches historical map layout.
func (o *SunSpecMapOutput) MarshalTextOutput() (string, error) {
	if o == nil {
		return "", nil
	}
	var b strings.Builder
	if o.NotDetected {
		_, _ = fmt.Fprintf(&b, "SunSpec not detected.\n")
		return b.String(), nil
	}
	_, _ = fmt.Fprintf(&b, "SunSpec map detected\n")
	_, _ = fmt.Fprintf(&b, "  Base marker : %d\n", o.Base)
	_, _ = fmt.Fprintf(&b, "  Marker regs : %s\n", o.MarkerRegs)
	_, _ = fmt.Fprintf(&b, "\nMODEL MAP\n")
	for _, m := range o.Models {
		if m.IsEndModel {
			_, _ = fmt.Fprintf(&b, "  %d-%d   end\n", m.StartAddress, m.EndAddress)
			continue
		}
		switch {
		case o.ShowHeaderRegs && o.ShowNext:
			_, _ = fmt.Fprintf(&b, "  %d-%d   model %d  hdr %d-%d (next %d)\n", m.StartAddress, m.EndAddress, m.ID, m.StartAddress, m.StartAddress+1, m.NextAddress)
		case o.ShowHeaderRegs:
			_, _ = fmt.Fprintf(&b, "  %d-%d   model %d  hdr %d-%d\n", m.StartAddress, m.EndAddress, m.ID, m.StartAddress, m.StartAddress+1)
		case o.ShowNext:
			_, _ = fmt.Fprintf(&b, "  %d-%d   model %d (next %d)\n", m.StartAddress, m.EndAddress, m.ID, m.NextAddress)
		default:
			if o.Compact {
				_, _ = fmt.Fprintf(&b, "  %d-%d m%d\n", m.StartAddress, m.EndAddress, m.ID)
			} else {
				_, _ = fmt.Fprintf(&b, "  %d-%d   model %d\n", m.StartAddress, m.EndAddress, m.ID)
			}
		}
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (o *SunSpecMapOutput) TableHeaders() []string {
	return []string{"start", "end", "model_id", "end"}
}

// TableRows implements format.TableMarshaler.
func (o *SunSpecMapOutput) TableRows() [][]string {
	if o == nil || o.NotDetected {
		return nil
	}
	var rows [][]string
	for _, m := range o.Models {
		em := "no"
		if m.IsEndModel {
			em = "yes"
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", m.StartAddress),
			fmt.Sprintf("%d", m.EndAddress),
			fmt.Sprintf("%d", m.ID),
			em,
		})
	}
	return rows
}

// SunSpecProbeOutput combines Modbus FC support bits with SunSpec summary.
type SunSpecProbeOutput struct {
	Target        string              `json:"target"`
	UnitID        uint8               `json:"unit_id"`
	Modbus        SunSpecProbeModbus  `json:"modbus"`
	SunSpecDetail SunSpecProbeSummary `json:"sunspec"`
}

// SunSpecProbeModbus lists support for key function codes.
type SunSpecProbeModbus struct {
	FC03 bool `json:"fc03"`
	FC04 bool `json:"fc04"`
	FC43 bool `json:"fc43"`
	// FC*Error is set when SupportsFunction failed (transport/protocol); FC* may be false.
	FC03Error *ErrorInfo `json:"fc03_error,omitempty"`
	FC04Error *ErrorInfo `json:"fc04_error,omitempty"`
	FC43Error *ErrorInfo `json:"fc43_error,omitempty"`
}

// HasPartialFailure implements [PartialResult] for exit code 7 when any probe or SunSpec step errored.
func (o *SunSpecProbeOutput) HasPartialFailure() bool {
	if o == nil {
		return false
	}
	if o.Modbus.FC03Error != nil || o.Modbus.FC04Error != nil || o.Modbus.FC43Error != nil {
		return true
	}
	if o.SunSpecDetail.DetectionError != nil || o.SunSpecDetail.ModelHeadersError != nil {
		return true
	}
	return false
}

// SunSpecProbeSummary is SunSpec detection summary from probe.
type SunSpecProbeSummary struct {
	Detected       bool       `json:"detected"`
	BaseAddress    uint16     `json:"base_address"`
	ModelsFound    int        `json:"models_found"`
	EndModel       bool       `json:"end_model"`
	DetectionError *ErrorInfo `json:"detection_error,omitempty"` // set when sunspec.Detect fails (transport/protocol), not when the device is simply not SunSpec
	// ModelHeadersError is set when the marker was found but ReadModelHeaders failed (transport/protocol).
	ModelHeadersError *ErrorInfo `json:"model_headers_error,omitempty"`
}

// MarshalTextOutput matches historical probe layout.
func (o *SunSpecProbeOutput) MarshalTextOutput() (string, error) {
	if o == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "TARGET\n")
	_, _ = fmt.Fprintf(&b, "  URL    : %s\n", o.Target)
	_, _ = fmt.Fprintf(&b, "  UNIT   : %d\n", o.UnitID)
	_, _ = fmt.Fprintf(&b, "\nMODBUS\n")
	for _, line := range []struct {
		label string
		ok    bool
		err   *ErrorInfo
	}{
		{"FC03", o.Modbus.FC03, o.Modbus.FC03Error},
		{"FC04", o.Modbus.FC04, o.Modbus.FC04Error},
		{"FC43", o.Modbus.FC43, o.Modbus.FC43Error},
	} {
		if msg := ErrorMessage(line.err); msg != "" {
			_, _ = fmt.Fprintf(&b, "  %-6s : error: %s\n", line.label, msg)
			continue
		}
		supported := "supported"
		if !line.ok {
			supported = "not supported"
		}
		_, _ = fmt.Fprintf(&b, "  %-6s : %s\n", line.label, supported)
	}
	_, _ = fmt.Fprintf(&b, "\nSUNSPEC\n")
	if msg := ErrorMessage(o.SunSpecDetail.DetectionError); msg != "" {
		_, _ = fmt.Fprintf(&b, "  detection error : %s\n", msg)
	}
	if msg := ErrorMessage(o.SunSpecDetail.ModelHeadersError); msg != "" {
		_, _ = fmt.Fprintf(&b, "  model headers error : %s\n", msg)
	}
	detectedStr := "no"
	if o.SunSpecDetail.Detected {
		detectedStr = "yes"
	}
	_, _ = fmt.Fprintf(&b, "  detected     : %s\n", detectedStr)
	if o.SunSpecDetail.Detected {
		_, _ = fmt.Fprintf(&b, "  base address : %d\n", o.SunSpecDetail.BaseAddress)
		_, _ = fmt.Fprintf(&b, "  models found : %d\n", o.SunSpecDetail.ModelsFound)
		_, _ = fmt.Fprintf(&b, "  end model    : %v\n", o.SunSpecDetail.EndModel)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (o *SunSpecProbeOutput) TableHeaders() []string {
	return []string{"section", "key", "value"}
}

// TableRows implements format.TableMarshaler.
func (o *SunSpecProbeOutput) TableRows() [][]string {
	if o == nil {
		return nil
	}
	rows := [][]string{
		{"target", "url", o.Target},
		{"target", "unit_id", fmt.Sprintf("%d", o.UnitID)},
		{"modbus", "fc03", fmt.Sprintf("%v", o.Modbus.FC03)},
		{"modbus", "fc04", fmt.Sprintf("%v", o.Modbus.FC04)},
		{"modbus", "fc43", fmt.Sprintf("%v", o.Modbus.FC43)},
	}
	if msg := ErrorMessage(o.Modbus.FC03Error); msg != "" {
		rows = append(rows, []string{"modbus", "fc03_error", msg})
	}
	if msg := ErrorMessage(o.Modbus.FC04Error); msg != "" {
		rows = append(rows, []string{"modbus", "fc04_error", msg})
	}
	if msg := ErrorMessage(o.Modbus.FC43Error); msg != "" {
		rows = append(rows, []string{"modbus", "fc43_error", msg})
	}
	if msg := ErrorMessage(o.SunSpecDetail.DetectionError); msg != "" {
		rows = append(rows, []string{"sunspec", "detection_error", msg})
	}
	if msg := ErrorMessage(o.SunSpecDetail.ModelHeadersError); msg != "" {
		rows = append(rows, []string{"sunspec", "model_headers_error", msg})
	}
	rows = append(rows,
		[]string{"sunspec", "detected", fmt.Sprintf("%v", o.SunSpecDetail.Detected)},
		[]string{"sunspec", "base_address", fmt.Sprintf("%d", o.SunSpecDetail.BaseAddress)},
		[]string{"sunspec", "models_found", fmt.Sprintf("%d", o.SunSpecDetail.ModelsFound)},
		[]string{"sunspec", "end_model", fmt.Sprintf("%v", o.SunSpecDetail.EndModel)},
	)
	return rows
}
