package replay

import (
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
)

// Clock maps time elapsed since a replay started to the replay time T. It
// holds no state and reads no clock: the caller passes the elapsed time.
type Clock struct {
	// TickNS is the capture's tick size.
	TickNS uint64
	// Speed is the replay speed: 1 original, 0.1 ten times slower, 60 a
	// minute per second. Zero or negative stops time at From (a static
	// device).
	Speed float64
	// From is the capture time the replay starts at; To the time it plays
	// until (usually the capture end).
	From, To capture.Ticks
	// Loops is the number of times the window [From, To) is played, after
	// which T stays at To. Zero loops forever. Values jump back at each seam.
	Loops int
}

// At returns T after elapsed wall-clock time.
func (c Clock) At(elapsed time.Duration) capture.Ticks {
	if c.Speed <= 0 || elapsed <= 0 || c.TickNS == 0 {
		return c.From
	}
	if c.To <= c.From {
		return c.To
	}
	span := float64(c.To - c.From)
	pos := float64(elapsed) * c.Speed / float64(c.TickNS) // ticks played
	loop := pos / span
	if c.Loops > 0 && loop >= float64(c.Loops) {
		return c.To
	}
	into := pos - float64(int64(loop))*span
	if into >= span {
		into = span - 1
	}
	return c.From + capture.Ticks(into)
}
