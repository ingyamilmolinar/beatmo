package ui

import (
	"image/color"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Each trigger gets its own line from one monotonic start time, so a preview never jitters and a re-fire
// before the sound ends spawns a new line instead of resetting the old. Region and duration are read live.

// Caps the lines so a fast loop cannot grow the slice without bound; the oldest go first.
const samplerMaxPlayheads = 6

// A var so tests get deterministic positions.
var samplerNowNanos = func() int64 { return time.Now().UnixNano() }

func SwapSamplerNowFnForTest(fn func() int64) func() {
	prev := samplerNowNanos
	if fn != nil {
		samplerNowNanos = fn
	}
	return func() { samplerNowNanos = prev }
}

// Cycles the instrument palette so overlapping lines stay distinct; an empty palette falls back to text
// color rather than going transparent.
func samplerPlayheadColor(idx int) color.Color {
	n := len(customPalette)
	if n == 0 {
		return TokenTextPrimary()
	}
	if idx < 0 {
		idx = -idx
	}
	return customPalette[idx%n]
}

// Trim, duration and direction are read live from samplerState, never snapshotted.
type samplerPlayheadLine struct {
	startNanos int64
	colorIdx   int
}

type samplerPlayheadView struct {
	frac     float64
	colorIdx int
}

// observe never touches an existing line; it only spawns new ones and prunes finished ones.
type samplerPlayheadTracker struct {
	lines     []samplerPlayheadLine
	lastSeen  map[instrument.ID]int64 // instrument id -> latest trigger nanos already spawned
	nextColor int
}

// lastSeen is kept so a stale trigger does not respawn on reload.
func (t *samplerPlayheadTracker) reset() { t.lines = t.lines[:0] }

// triggers maps id → latest trigger in unix-nanos, 0 = never.
func (t *samplerPlayheadTracker) observe(nowNanos int64, triggers map[instrument.ID]int64, durSec float64) {
	if t.lastSeen == nil {
		t.lastSeen = map[instrument.ID]int64{}
	}
	durNanos := int64(durSec * float64(time.Second))
	for id, tn := range triggers {
		if tn <= 0 || tn <= t.lastSeen[id] {
			continue // never triggered, or already spawned for this trigger
		}
		t.lastSeen[id] = tn
		// Only a trigger still playing: opening the panel long after one must not spawn a phantom line.
		if elapsed := nowNanos - tn; elapsed < 0 || (durNanos > 0 && elapsed > durNanos) {
			continue
		}
		t.lines = append(t.lines, samplerPlayheadLine{startNanos: tn, colorIdx: t.nextColor})
		t.nextColor++
	}
	t.prune(nowNanos, durNanos)
}

func (t *samplerPlayheadTracker) prune(nowNanos, durNanos int64) {
	kept := t.lines[:0]
	for _, ln := range t.lines {
		elapsed := nowNanos - ln.startNanos
		if elapsed < 0 || durNanos <= 0 || elapsed > durNanos {
			continue
		}
		kept = append(kept, ln)
	}
	t.lines = kept
	if n := len(t.lines); n > samplerMaxPlayheads {
		t.lines = append([]samplerPlayheadLine(nil), t.lines[n-samplerMaxPlayheads:]...)
	}
}

// Always forward: reversing flips the buffer, not the sweep.
func (t *samplerPlayheadTracker) active(nowNanos int64, durSec, lo, hi float64) []samplerPlayheadView {
	if len(t.lines) == 0 {
		return nil
	}
	out := make([]samplerPlayheadView, 0, len(t.lines))
	for _, ln := range t.lines {
		elapsed := float64(nowNanos-ln.startNanos) / float64(time.Second)
		frac, vis := samplerPlayheadFrac(elapsed, durSec, lo, hi)
		if vis {
			out = append(out, samplerPlayheadView{frac: frac, colorIdx: ln.colorIdx})
		}
	}
	return out
}
