package capfile

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/types"
)

// Errors of [Records], to be tested with errors.Is.
var (
	// ErrAmbiguousSelection: the capture holds FC01–04 samples of several targets or
	// functions, and the selection does not pick one.
	ErrAmbiguousSelection = errors.New("the capture holds samples of several targets or functions")
	// ErrEmptySelection: no FC01–04 sample matches the selection.
	ErrEmptySelection = errors.New("the capture holds no FC01-04 samples")
)

// Selection picks the samples the register-block commands (decode, strings,
// frequencies, extract) work on: FC01–04 samples of one target and one
// function. Zero values mean "the only one in the capture".
type Selection struct {
	Target   uint32
	Function uint8
}

// Records returns the FC01–04 samples of the selected target and function as
// register-block records: one per sample, with the response data bytes
// (packed bits for FC01/02, big-endian words for FC03/04). Iteration is the
// number of earlier samples of the same request, so the n-th poll of every
// block shares iteration n. Exception responses and malformed responses are
// left out.
func Records(l *Loaded, sel Selection) ([]types.CaptureRecord, types.CaptureHeader, error) {
	targetOf := l.TargetOf()
	type key struct {
		target uint32
		fc     uint8
	}
	found := map[key]int{}
	for i := range l.Samples {
		s := &l.Samples[i]
		if fc := s.Function(); fc >= 1 && fc <= 4 && !s.IsException() {
			found[key{targetOf[s.Stream], fc}]++
		}
	}
	var keys []key
	for k := range found {
		if (sel.Target == 0 || k.target == sel.Target) && (sel.Function == 0 || k.fc == sel.Function) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].target != keys[j].target {
			return keys[i].target < keys[j].target
		}
		return keys[i].fc < keys[j].fc
	})
	switch len(keys) {
	case 0:
		if len(found) == 0 && sel == (Selection{}) {
			// An empty capture: no records, and the header of its only target, if any.
			h := types.CaptureHeader{StartTime: l.Header.WallStart.UnixNano()}
			if len(l.Targets) == 1 {
				h = header(l, l.Targets[0], 0)
			}
			return nil, h, nil
		}
		return nil, types.CaptureHeader{}, fmt.Errorf("%w%s", ErrEmptySelection, selText(sel))
	case 1:
	default:
		var opts []string
		for _, k := range keys {
			opts = append(opts, fmt.Sprintf("--target %d --function %d (%d samples)", k.target, k.fc, found[k]))
		}
		return nil, types.CaptureHeader{}, fmt.Errorf("%w; choose one: %s", ErrAmbiguousSelection, strings.Join(opts, "; "))
	}
	k := keys[0]
	t, _ := l.Target(k.target)
	h := header(l, t, k.fc)

	seen := map[string]uint32{}
	var recs []types.CaptureRecord
	for i := range l.Samples {
		s := &l.Samples[i]
		if targetOf[s.Stream] != k.target || s.Function() != k.fc {
			continue
		}
		av, ok := capture.SplitAddresses(s)
		if !ok {
			continue
		}
		reqKey := fmt.Sprintf("%d:%x", s.Stream, s.Request)
		iter := seen[reqKey]
		seen[reqKey] = iter + 1
		wall := l.Header.Time(s.At).UnixNano()
		recs = append(recs, types.CaptureRecord{
			Iteration:         iter,
			RequestTimestamp:  wall,
			ResponseTimestamp: wall,
			StartAddress:      av.Start,
			RegisterCount:     uint16(len(av.Values)),
			Data:              append([]byte(nil), s.Response[2:]...),
		})
	}
	return recs, h, nil
}

// header describes target t and function fc in the old capture header form.
func header(l *Loaded, t capture.Target, fc uint8) types.CaptureHeader {
	h := types.CaptureHeader{Unit: t.Unit, Function: fc, StartTime: l.Header.WallStart.UnixNano()}
	if !t.Serial.IsZero() {
		h.IP = t.Serial.Device
	} else {
		h.IP, h.Port = t.Host, t.Port
	}
	return h
}

func selText(sel Selection) string {
	var parts []string
	if sel.Target != 0 {
		parts = append(parts, fmt.Sprintf("target %d", sel.Target))
	}
	if sel.Function != 0 {
		parts = append(parts, fmt.Sprintf("function %d", sel.Function))
	}
	if len(parts) == 0 {
		return ""
	}
	return " for " + strings.Join(parts, " and ")
}
