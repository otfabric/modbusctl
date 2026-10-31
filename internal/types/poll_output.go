// SPDX-License-Identifier: MIT

package types

import (
	"fmt"
	"strings"
)

// PollSample is one iteration of `client poll`: either the values read or the error that
// prevented the read. With --format json every sample is one JSON object on its own line.
type PollSample struct {
	Poll         uint32 `json:"poll"`
	Timestamp    string `json:"timestamp"`
	UnitID       uint8  `json:"unit_id"`
	Function     uint8  `json:"function"`
	StartAddress uint16 `json:"start_address"`
	// Count is the number of coils or registers read.
	Count uint16 `json:"count"`
	// AddressStep is the address distance between consecutive Values (the codec register
	// width, or 1 without a codec).
	AddressStep uint16     `json:"address_step"`
	Codec       string     `json:"codec,omitempty"`
	RTTMs       float64    `json:"rtt_ms"`
	RawDataHex  string     `json:"raw_data_hex,omitempty"`
	Values      []any      `json:"values"`
	Error       *ErrorInfo `json:"error,omitempty"`
}

func (s *PollSample) step() int {
	if s.AddressStep == 0 {
		return 1
	}
	return int(s.AddressStep)
}

// valueCount is the number of values a successful sample carries.
func (s *PollSample) valueCount() int {
	return int(s.Count) / s.step()
}

func formatPollValue(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "1"
		}
		return "0"
	}
	return FormatDecodedValue(v)
}

// MarshalTextOutput renders the sample as a single line:
//
//	2026-10-09T12:00:00.123Z  #1  100=1234 101=5678
func (s *PollSample) MarshalTextOutput() (string, error) {
	if s == nil {
		return "", nil
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s  #%d ", s.Timestamp, s.Poll)
	if s.Error != nil {
		_, _ = fmt.Fprintf(&b, " error: %s", s.Error.Message)
		return b.String(), nil
	}
	for i, v := range s.Values {
		_, _ = fmt.Fprintf(&b, " %d=%s", int(s.StartAddress)+i*s.step(), formatPollValue(v))
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler: one column per polled value, named by address.
func (s *PollSample) TableHeaders() []string {
	h := []string{"poll", "timestamp", "rtt_ms"}
	if s == nil {
		return h
	}
	for i := 0; i < s.valueCount(); i++ {
		h = append(h, fmt.Sprintf("%d", int(s.StartAddress)+i*s.step()))
	}
	return append(h, "error")
}

// TableRows implements format.TableMarshaler.
func (s *PollSample) TableRows() [][]string {
	if s == nil {
		return nil
	}
	row := []string{fmt.Sprintf("%d", s.Poll), s.Timestamp, fmt.Sprintf("%.1f", s.RTTMs)}
	for i := 0; i < s.valueCount(); i++ {
		cell := ""
		if s.Error == nil && i < len(s.Values) {
			cell = formatPollValue(s.Values[i])
		}
		row = append(row, cell)
	}
	errCell := ""
	if s.Error != nil {
		errCell = s.Error.Message
	}
	return [][]string{append(row, errCell)}
}

// PollSummary is the final tally of a `client poll` run (reported on stderr).
type PollSummary struct {
	Target    string `json:"target"`
	Polls     uint32 `json:"polls"`
	Succeeded uint32 `json:"succeeded"`
	Failed    uint32 `json:"failed"`
	Duration  string `json:"duration"`
	// Interrupted is true when the run was stopped by a signal instead of reaching --polls.
	Interrupted bool `json:"interrupted"`
}

// String renders the summary as a single line.
func (p *PollSummary) String() string {
	if p == nil {
		return ""
	}
	s := fmt.Sprintf("%d poll(s) of %s in %s: %d succeeded, %d failed", p.Polls, p.Target, p.Duration, p.Succeeded, p.Failed)
	if p.Interrupted {
		s += " (interrupted)"
	}
	return s
}
