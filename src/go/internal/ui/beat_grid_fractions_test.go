package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// No panic and nothing out of range for a row with no beatInfos.
func TestBeatGridFractions_SilentRow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows = []*DrumRow{{Instrument: roster.Snare}}
	g.drum.SetLength(8)
	g.beatInfosByRow = [][]model.BeatInfo{nil}
	g.isLoopByRow = []bool{false}

	fracs := g.beatGridFractions(0, 2200)
	// Ticks still occupy the time slots, but in range.
	for _, f := range fracs {
		if f < 0 || f >= 1 {
			t.Fatalf("frac out of [0,1): %v (slice=%v)", f, fracs)
		}
	}
}

// At 44.1 kHz, 120 BPM, subdiv 8 a tick is ~2756 samples, so a 2200-sample window holds at most one.
func TestBeatGridFractions_DenseRow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows = []*DrumRow{{Instrument: roster.Snare}}
	g.drum.SetLength(8)
	g.drum.SetBPM(120)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions(8): %v", err)
	}
	g.beatInfosByRow = [][]model.BeatInfo{make([]model.BeatInfo, 8)}
	g.isLoopByRow = []bool{false}

	fracs := g.beatGridFractions(0, 2200)
	if len(fracs) > 1 {
		t.Fatalf("dense row: expected <=1 fraction in 2200-sample window, got %d (%v)", len(fracs), fracs)
	}
}

// 22050 samples hold ~8 ticks; at least 6 expected.
func TestBeatGridFractions_LongerWindow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows = []*DrumRow{{Instrument: roster.Snare}}
	g.drum.SetLength(8)
	g.drum.SetBPM(120)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions(8): %v", err)
	}
	g.beatInfosByRow = [][]model.BeatInfo{make([]model.BeatInfo, 16)}
	g.isLoopByRow = []bool{false}

	fracs := g.beatGridFractions(0, 22050)
	if len(fracs) < 6 {
		t.Fatalf("longer window: expected >=6 fractions in 22050-sample window, got %d (%v)", len(fracs), fracs)
	}
}

// The caller may resolve a channel that maps to no current row.
func TestBeatGridFractions_OutOfBounds(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.beatInfosByRow = [][]model.BeatInfo{make([]model.BeatInfo, 4)}

	if fracs := g.beatGridFractions(-1, 2200); fracs != nil {
		t.Fatalf("rowIdx=-1: expected nil, got %v", fracs)
	}
	if fracs := g.beatGridFractions(99, 2200); fracs != nil {
		t.Fatalf("rowIdx beyond range: expected nil, got %v", fracs)
	}
	if fracs := g.beatGridFractions(0, 0); fracs != nil {
		t.Fatalf("windowSamples=0: expected nil, got %v", fracs)
	}
	if fracs := g.beatGridFractions(0, -100); fracs != nil {
		t.Fatalf("windowSamples=-100: expected nil, got %v", fracs)
	}
}

// Strictly in [0, 1): the renderer places ticks inside the content rect by it.
func TestBeatGridFractions_FractionsInRange(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows = []*DrumRow{{Instrument: roster.Snare}}
	g.drum.SetLength(8)
	g.drum.SetBPM(120)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions(8): %v", err)
	}
	g.beatInfosByRow = [][]model.BeatInfo{make([]model.BeatInfo, 16)}
	g.isLoopByRow = []bool{false}

	// Sweep across a few window sizes to broaden coverage.
	for _, w := range []int{1000, 2200, 5000, 22050, 60000} {
		fracs := g.beatGridFractions(0, w)
		for _, f := range fracs {
			if f < 0 || f >= 1 {
				t.Fatalf("window=%d frac=%v out of [0,1) (slice=%v)", w, f, fracs)
			}
		}
	}
}

// Without the wiring the beat-grid overlay never renders.
func TestBeatGridFractions_WiredThroughEQCallbacks(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	if g.drum == nil || g.drum.eqPanelZone == nil {
		t.Fatalf("drum/eqPanelZone nil after construction")
	}
	cb := g.drum.eqPanelZone.callbacks
	if cb.BeatGridFrac == nil {
		t.Fatalf("BeatGridFrac callback should be wired by drumview ctor")
	}
	// Invoke; verify it returns without panic and all fractions are in [0,1).
	fracs := cb.BeatGridFrac()
	for _, f := range fracs {
		if f < 0 || f >= 1 {
			t.Fatalf("wired callback returned out-of-range frac=%v (slice=%v)", f, fracs)
		}
	}
}
