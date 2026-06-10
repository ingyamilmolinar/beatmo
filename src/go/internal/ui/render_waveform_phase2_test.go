//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestWavePhase2_MSAxisTicksInBottomStrip — Wave tab should expose its
// time-base the way Chain does. Drawing tick marks at 0/25/50/75/100% of
// the waveform width in a dedicated bottom strip (~10px tall) makes the
// signal correlate to time at a glance. Test asserts ≥4 colTextSecondary
// tick rects in the bottom ~10px band of the wave subject when the wave
// is alive (snapshotWithSine).
func TestWavePhase2_MSAxisTicksInBottomStrip(t *testing.T) {
	g := driveScene(t, "crop_eq_tab_wave")
	rect, rects := drawWithSnapshot(t, g, SubjectEQTabWave, snapshotWithSine())

	// Bottom strip = bottom 12 px of the wave subject.
	bottomStrip := image.Rect(rect.Min.X, rect.Max.Y-12, rect.Max.X, rect.Max.Y)
	got := rectsWithColorInside(rects, bottomStrip, colTextSecondary)
	if got < 4 {
		t.Fatalf("expected ≥4 colTextSecondary tick rects in bottom strip %v (Wave ms axis), got %d", bottomStrip, got)
	}
}

// TestWavePhase2_ClipFlashOnSaturation — when the waveform exceeds ±1.0
// at any column, the renderer should paint that column in meterClip
// instead of colWaveTrace. The visual gives instant DAW-style headroom
// warning. Test feeds a wave that clips on alternating samples.
func TestWavePhase2_ClipFlashOnSaturation(t *testing.T) {
	g := driveScene(t, "crop_eq_tab_wave")

	// Clipping wave: half the samples at +1.6 (clip), half at -0.6 (clean).
	const n = 512
	wave := make([]float64, n)
	for i := range wave {
		if i%4 == 0 {
			wave[i] = 1.6 // clip!
		} else {
			wave[i] = 0.4 * math.Sin(2*math.Pi*float64(i)/32.0)
		}
	}
	snap := audio.AnalyzerSnapshot{Waveform: wave, Peak: 1.6, RMS: 0.7}

	rect, rects := drawWithSnapshot(t, g, SubjectEQTabWave, snap)

	// Expect at least one column painted in meterClip.
	gotClip := rectsWithColorInside(rects, rect, meterClip)
	if gotClip == 0 {
		t.Fatalf("expected meterClip rects in wave subject %v when signal clips, got 0", rect)
	}
}

// TestWavePhase2_NoClipFlashWhenClean — inverse contract: a clean
// wave (snapshotWithSine peaks at 0.6) must NOT paint any meterClip
// columns. Otherwise the warning is meaningless (always lit).
func TestWavePhase2_NoClipFlashWhenClean(t *testing.T) {
	g := driveScene(t, "crop_eq_tab_wave")
	rect, rects := drawWithSnapshot(t, g, SubjectEQTabWave, snapshotWithSine())
	if got := rectsWithColorInside(rects, rect, meterClip); got != 0 {
		t.Fatalf("expected 0 meterClip rects on clean wave, got %d in %v", got, rect)
	}
}

// TestWavePhase2_AxisTicksScaleWithWindowMs — pure unit test of the
// helper that decides label positions/text. Five fractions [0, .25,
// .5, .75, 1] with formatted ms labels.
func TestWavePhase2_AxisTicksScaleWithWindowMs(t *testing.T) {
	got := waveAxisLabels(20.0)
	if len(got) != 5 {
		t.Fatalf("waveAxisLabels(20ms): want 5 labels, got %d (%v)", len(got), got)
	}
	wantFrac := []float64{0, 0.25, 0.5, 0.75, 1}
	wantText := []string{"0.00ms", "5.0ms", "10ms", "15ms", "20ms"}
	for i, lbl := range got {
		if math.Abs(lbl.Frac-wantFrac[i]) > 1e-9 {
			t.Errorf("label %d: frac=%v want %v", i, lbl.Frac, wantFrac[i])
		}
		if lbl.Text != wantText[i] {
			t.Errorf("label %d: text=%q want %q", i, lbl.Text, wantText[i])
		}
	}
}
