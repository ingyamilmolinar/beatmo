package ui

import (
	"image/color"
	"time"
)

// sampler_playhead.go — the Sampler tab's preview playhead lines.
//
// A playhead is a bright vertical line that sweeps the kept (trim) region of
// the waveform while the sound plays, tracking the audio progression. Each
// individual trigger gets its OWN line with a monotonic start time, so:
//
//   - a single preview advances smoothly (no jitter — the position is a clean
//     function of one start timestamp, never a min() across re-firing clocks);
//   - when a node re-fires before its previous sound has finished (the next
//     beat's subdivision is shorter than the sound's duration), the in-flight
//     line is NOT reset — a new line spawns in a different colour, so every
//     overlapping voice is visible through to completion.
//
// Region/duration are read live from current state every frame (resilient to
// trim/pitch/sample edits); only the start time + colour are stored per line.

// samplerMaxPlayheads caps how many concurrent lines are kept, so a fast loop
// can't grow the slice without bound. The oldest are dropped first.
const samplerMaxPlayheads = 6

// samplerNowNanos is the playhead clock (wall-clock unix-nanos). Swappable in
// tests for deterministic positions.
var samplerNowNanos = func() int64 { return time.Now().UnixNano() }

// SwapSamplerNowFnForTest installs a deterministic clock for the playhead and
// returns a restore closure.
func SwapSamplerNowFnForTest(fn func() int64) func() {
	prev := samplerNowNanos
	if fn != nil {
		samplerNowNanos = fn
	}
	return func() { samplerNowNanos = prev }
}

// samplerPlayheadColor returns the line colour for a given spawn index, cycling
// the DESIGN.md instrument fallback palette so adjacent/overlapping lines are
// visually distinct. Falls back to the primary text colour if the palette is
// somehow empty (keeps the line visible rather than transparent).
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

// samplerPlayheadLine is one in-flight playhead: a single trigger's start time
// and its palette colour index. The trim region, duration, and direction are
// read live from samplerState, never snapshotted here.
type samplerPlayheadLine struct {
	startNanos int64
	colorIdx   int
}

// samplerPlayheadView is the per-frame draw input for one line: its [0,1]
// x-fraction over the waveform and its colour index.
type samplerPlayheadView struct {
	frac     float64
	colorIdx int
}

// samplerPlayheadTracker owns the active playhead lines. observe() spawns a new
// line for every trigger it hasn't seen yet (never touching existing lines) and
// prunes finished ones; active() projects the survivors to draw fractions.
type samplerPlayheadTracker struct {
	lines     []samplerPlayheadLine
	lastSeen  map[string]int64 // instrument id -> latest trigger nanos already spawned
	nextColor int
}

// reset clears all in-flight lines (e.g. when the buffer is unloaded). lastSeen
// is preserved so a stale trigger doesn't respawn on reload.
func (t *samplerPlayheadTracker) reset() { t.lines = t.lines[:0] }

// observe spawns a line for every id whose latest trigger is newer than the one
// already spawned and still within its playback window, then prunes finished
// lines and caps the count. triggers maps instrument id -> latest trigger time
// in unix-nanos (0 means "never triggered").
func (t *samplerPlayheadTracker) observe(nowNanos int64, triggers map[string]int64, durSec float64) {
	if t.lastSeen == nil {
		t.lastSeen = map[string]int64{}
	}
	durNanos := int64(durSec * float64(time.Second))
	for id, tn := range triggers {
		if tn <= 0 || tn <= t.lastSeen[id] {
			continue // never triggered, or already spawned for this trigger
		}
		t.lastSeen[id] = tn
		// Only spawn for a trigger that is still playing — guards against a
		// phantom line when the panel opens long after a trigger.
		if elapsed := nowNanos - tn; elapsed < 0 || (durNanos > 0 && elapsed > durNanos) {
			continue
		}
		t.lines = append(t.lines, samplerPlayheadLine{startNanos: tn, colorIdx: t.nextColor})
		t.nextColor++
	}
	t.prune(nowNanos, durNanos)
}

// prune drops lines that have finished playing (or have a future/invalid start)
// and caps the survivors to the most recent samplerMaxPlayheads.
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

// active projects every in-flight line to its current draw fraction. Lines that
// have run past the (live) duration are skipped — prune() removes them on the
// next observe(). lo/hi are the ordered trim fractions. The sweep is always
// forward (left→right); reversing the sound flips the buffer, not the highlight
// direction.
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
