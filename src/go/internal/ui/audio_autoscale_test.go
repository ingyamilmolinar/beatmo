//go:build test

package ui

import (
	"math"
	"testing"
)

// The dB tabs (Spectrum, EQ backdrop) map a dB value to a bar height. With a
// FIXED -80..0 window a normal mix never reaches the top of the plot, so the
// data band is a fraction of the viewport it was given. These pin the adaptive
// window that replaces it: the ceiling rides the signal's smoothed peak, the
// floor sits a fixed span below, and both snap to a grid so the axis labels
// step instead of jittering every frame.

func TestAudioDBWindow_SilencePinsToTheFixedReference(t *testing.T) {
	// Below the silence threshold there is nothing to frame, and zooming into
	// the noise floor would render silence as a full-height spectrum.
	floor, ceil := audioDBWindow(-95)
	if floor != spectrumMinDB || ceil != spectrumMaxDB {
		t.Errorf("silence window = (%.1f, %.1f), want the fixed reference (%.1f, %.1f)",
			floor, ceil, spectrumMinDB, spectrumMaxDB)
	}
}

func TestAudioDBWindow_CeilingTracksThePeak(t *testing.T) {
	// A -28 dB peak must not leave the top of the plot permanently empty.
	floor, ceil := audioDBWindow(-28)
	if ceil >= spectrumMaxDB {
		t.Errorf("ceiling %.1f did not descend toward the -28 dB peak", ceil)
	}
	if ceil < -28 {
		t.Errorf("ceiling %.1f clips the peak it is framing", ceil)
	}
	if got := ceil - floor; math.Abs(got-audioDBWindowSpan) > 0.001 {
		t.Errorf("window span = %.1f, want %.1f", got, audioDBWindowSpan)
	}
	// The peak should land near the top of the plot — that is the whole point.
	if n := audioDBNorm(-28, floor, ceil); n < 0.85 {
		t.Errorf("peak normalises to %.2f of plot height, want >= 0.85", n)
	}
}

func TestAudioDBWindow_NeverRisesAboveFullScale(t *testing.T) {
	// A clipping signal must not push the ceiling above 0 dBFS — the axis
	// would then claim headroom that cannot exist.
	if _, ceil := audioDBWindow(+4); ceil != spectrumMaxDB {
		t.Errorf("ceiling = %.1f for a +4 dB peak, want %.1f", ceil, spectrumMaxDB)
	}
}

func TestAudioDBWindow_SnapsToAGridSoLabelsDoNotJitter(t *testing.T) {
	// The window must be a STEP function of the peak, not a continuous one:
	// a continuous ceiling renumbers the axis labels on every analyser
	// refresh (~30 Hz), which reads as flicker rather than as scale. Any
	// snapping has boundaries, so the property is not "small changes never
	// move it" — it is "a whole sweep produces only a handful of windows".
	seen := map[spectrumDBWindow]bool{}
	const from, to = -36.0, -12.0
	for db := from; db <= to; db += 0.1 {
		floor, ceil := audioDBWindow(db)
		seen[spectrumDBWindow{floor, ceil}] = true
	}
	// A 24 dB sweep on a 6 dB grid can cross at most 5 boundaries.
	if want := int((to-from)/audioDBWindowGrid) + 1; len(seen) > want {
		t.Errorf("a %.0f dB sweep produced %d distinct windows, want at most %d", to-from, len(seen), want)
	}
	if len(seen) < 2 {
		t.Errorf("the window never moved across a %.0f dB sweep — it is not tracking", to-from)
	}
}

// TestAudioDBWindow_StaysInsideTheReferenceRange — BOTH edges are bounded,
// and both bounds come from the data rather than from taste.
//
// Ceiling: nothing exists above full scale.
//
// Floor: every bin reaching a renderer has already been clamped at
// analyzerDBFloor (-80) by LinearToDBSafe, and spectrumBandDB clamps the
// band levels there again. A window below -80 therefore spends plot height
// on a region no bin can ever occupy and maps every quiet band onto one
// constant row of pixels.
//
// The floor clamp was deleted on 2026-09-18 to chase a browser-only empty
// Spectrum plot, on the theory that WASM bins run ~25 dB below desktop's.
// They do not — both platforms divide the FFT magnitude by fftSize — and
// the real defect was on the FREQUENCY axis (analyzer.SpectrumFFTSize,
// guarded by spectrum_bin_resolution_test.go). Under this clamp a -55 dB
// peak still fills 60%% of the plot, so it could never have produced the
// empty plot it was blamed for.
func TestAudioDBWindow_StaysInsideTheReferenceRange(t *testing.T) {
	for _, peak := range []float64{-40, -55, -69, -20, 0} {
		floor, ceil := audioDBWindow(peak)
		if floor < spectrumMinDB {
			t.Errorf("peak %.0f: floor %.1f sinks below the reference %.1f — below every value LinearToDBSafe can produce",
				peak, floor, spectrumMinDB)
		}
		if ceil > spectrumMaxDB {
			t.Errorf("peak %.0f: ceiling %.1f rises above the reference %.1f", peak, ceil, spectrumMaxDB)
		}
		if ceil <= floor {
			t.Errorf("peak %.0f: degenerate window %.1f..%.1f", peak, floor, ceil)
		}
	}
}

func TestAudioDBNorm_ClampsToThePlot(t *testing.T) {
	floor, ceil := -60.0, -12.0
	for _, tc := range []struct{ db, want float64 }{
		{-12, 1.0},
		{-60, 0.0},
		{-36, 0.5},
		{+6, 1.0},  // above the window
		{-90, 0.0}, // below the window
	} {
		if got := audioDBNorm(tc.db, floor, ceil); math.Abs(got-tc.want) > 0.001 {
			t.Errorf("audioDBNorm(%.0f) = %.3f, want %.3f", tc.db, got, tc.want)
		}
	}
}

func TestRaiseDBMax_OnlyEverRises(t *testing.T) {
	// Instant attack: a louder peak must be representable immediately.
	if got := raiseDBMax(-40, -10); got != -10 {
		t.Errorf("attack: got %.2f, want -10", got)
	}
	// No release: this is the whole difference from the amplitude family.
	// A decaying anchor slid the axis down between transients, which read as
	// the bars jumping.
	held := -10.0
	for i := 0; i < 500; i++ {
		held = raiseDBMax(held, -40)
	}
	if held != -10 {
		t.Errorf("anchor fell to %.2f over 500 quiet frames, want it pinned at -10", held)
	}
}

// TestAudioDBWindow_LeavesHeadroomAboveThePeak — a bar drawn flush against
// the top edge of the plot reads as clipping, not as "loudest". The same
// reasoning already governs the 0 dB reference line, which is suppressed when
// it would land flush under the sticky bar. The window must therefore always
// sit strictly above the peak it frames.
func TestAudioDBWindow_LeavesHeadroomAboveThePeak(t *testing.T) {
	// Walk across a grid boundary: the worst case is a peak sitting exactly
	// ON a grid multiple, where a naive ceil() returns the peak itself.
	for _, peak := range []float64{-24.0, -23.9, -18.0, -12.0, -30.0, -26.3} {
		floor, ceil := audioDBWindow(peak)
		if ceil <= peak {
			t.Errorf("peak %.1f: ceiling %.1f leaves no headroom", peak, ceil)
		}
		if n := audioDBNorm(peak, floor, ceil); n >= 1.0 {
			t.Errorf("peak %.1f normalises to %.3f — the bar touches the ceiling", peak, n)
		} else if n < 0.8 {
			t.Errorf("peak %.1f normalises to %.3f — too much dead space above it", peak, n)
		}
	}
}
