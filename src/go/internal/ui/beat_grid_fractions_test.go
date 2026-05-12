package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestBeatGridFractions_SilentRow asserts the helper returns no fractions
// when the requested row has no beatInfos behind it. The helper must not
// panic and must not emit out-of-range fractions for a silent row.
func TestBeatGridFractions_SilentRow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows = []*DrumRow{{Instrument: "snare"}}
	g.drum.SetLength(8)
	g.beatInfosByRow = [][]model.BeatInfo{nil}
	g.isLoopByRow = []bool{false}

	fracs := g.beatGridFractions(0, 2200)
	// Silent row: the helper still emits subdivision ticks (a missing/mute
	// cell still occupies the time slot) — but they must be in range. The
	// row's emptiness must not cause a panic.
	for _, f := range fracs {
		if f < 0 || f >= 1 {
			t.Fatalf("frac out of [0,1): %v (slice=%v)", f, fracs)
		}
	}
}

// TestBeatGridFractions_DenseRow asserts the small-window case: at the
// default test sample rate (44.1 kHz), BPM=120, subdiv=8, one tick spans
// ~62.5 ms (~2756 samples). A 2200-sample window therefore catches at
// most one tick.
func TestBeatGridFractions_DenseRow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows = []*DrumRow{{Instrument: "snare"}}
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

// TestBeatGridFractions_LongerWindow asserts a wider window catches many
// subdivision ticks. At 22050 samples (~500 ms @ 44.1 kHz) we expect at
// least 6 ticks (BPM=120 / subdiv=8 → 1 tick ≈ 62.5 ms → 22050/2756 ≈ 8).
func TestBeatGridFractions_LongerWindow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows = []*DrumRow{{Instrument: "snare"}}
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

// TestBeatGridFractions_OutOfBounds asserts negative or beyond-range row
// indices return nil. Defensive contract: the caller may resolve from a
// channel ID that doesn't map to a current row.
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

// TestBeatGridFractions_FractionsInRange asserts every returned fraction
// is strictly inside [0, 1). The renderer relies on this invariant to
// place vertical ticks within the wave content rect.
func TestBeatGridFractions_FractionsInRange(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows = []*DrumRow{{Instrument: "snare"}}
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

// TestBeatGridFractions_WiredThroughEQCallbacks asserts the drumview ctor
// wires the BeatGridFrac callback into EQPanelZone. Without the wiring
// the wave-panel beat-grid overlay never renders.
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
