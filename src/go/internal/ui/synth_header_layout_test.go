//go:build test

package ui

import (
	"image"
	"testing"
)

// synthHeaderForTest lays out the synth header in isolation and returns the
// resulting layout. headerH is generous so button height is governed by the
// density token, not adaptive shrink.
func synthHeaderForTest(t *testing.T, contentR image.Rectangle, inst, recipe string, mobile bool) synthHeaderLayout {
	t.Helper()
	dv := &DrumView{}
	dv.layoutSynthHeader(contentR, 64, inst, recipe, mobile)
	return dv.instEditorHeader
}

// TestSynthHeaderCaptionFitsRect — the mobile caption-overflow bug: the
// "inst — recipe" caption must be truncated to its captionRect so it never
// runs under the Save/Save As/Reset buttons.
func TestSynthHeaderCaptionFitsRect(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	h := synthHeaderForTest(t, image.Rect(0, 0, 390, 200), "kick-1", "drum-kick-punchy-extra-long", true)
	if h.captionRect.Dx() <= 0 {
		t.Fatal("captionRect has no width")
	}
	got := synthHeaderCaptionText(h)
	if TextWidth(got) > h.captionRect.Dx() {
		t.Errorf("caption %q width %d exceeds captionRect width %d (would overflow buttons)",
			got, TextWidth(got), h.captionRect.Dx())
	}
}

// TestSynthHeaderButtonHeightFromDensity — header button height must come
// from DensityValues().SynthHeaderButtonH, not a hardcoded 28/36.
func TestSynthHeaderButtonHeightFromDensity(t *testing.T) {
	measure := func(d Density) int {
		restore := SetDensityForTest(d)
		defer restore()
		h := synthHeaderForTest(t, image.Rect(0, 0, 1000, 200), "kick-1", "drum-kick", false)
		return h.saveRect.Dy()
	}
	for _, d := range []Density{DensityCompact, DensityComfortable, DensitySpacious} {
		restore := SetDensityForTest(d)
		want := Profile().DensityValues().SynthHeaderButtonH
		restore()
		if got := measure(d); got != want {
			t.Errorf("density=%v: save button height %d, want SynthHeaderButtonH %d", d, got, want)
		}
	}
}

// TestSynthHeaderButtonsTouchMinMobile — at Spacious density (mobile) every
// laid-out header button must meet the 44-px touch-min. The old hardcoded
// 36-px mobile height violated this.
func TestSynthHeaderButtonsTouchMinMobile(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	// Wide panel so no button collapses into the overflow chevron.
	h := synthHeaderForTest(t, image.Rect(0, 0, 900, 200), "kick-1", "drum-kick", true)
	minTarget := Profile().MinTarget
	for name, r := range map[string]image.Rectangle{"save": h.saveRect, "saveAs": h.saveAsRect, "reset": h.resetRect} {
		if r.Empty() {
			t.Errorf("%s button empty on a wide mobile header (should fit)", name)
			continue
		}
		if r.Dy() < minTarget {
			t.Errorf("%s button height %d < touch-min %d", name, r.Dy(), minTarget)
		}
	}
}

// TestSynthHeaderButtonsFitLabels — header buttons must be wide enough for
// their labels (label-derived width, not fixed 64/72/84).
func TestSynthHeaderButtonsFitLabels(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	h := synthHeaderForTest(t, image.Rect(0, 0, 1000, 200), "kick-1", "drum-kick", false)
	for _, tc := range []struct {
		name  string
		r     image.Rectangle
		label string
	}{
		{"save", h.saveRect, "Save"},
		{"saveAs", h.saveAsRect, "Save As"},
		{"reset", h.resetRect, "Reset"},
	} {
		if tc.r.Empty() {
			t.Errorf("%s button empty on a wide desktop header", tc.name)
			continue
		}
		if tc.r.Dx() < TextWidth(tc.label) {
			t.Errorf("%s button width %d < label %q width %d (would truncate)",
				tc.name, tc.r.Dx(), tc.label, TextWidth(tc.label))
		}
	}
}

// TestSynthHeaderThumbFromDensity — the waveform thumbnail width comes from
// DensityValues().SynthHeaderThumbW (clamped to a third of the header).
func TestSynthHeaderThumbFromDensity(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	want := Profile().DensityValues().SynthHeaderThumbW
	h := synthHeaderForTest(t, image.Rect(0, 0, 1200, 200), "kick-1", "drum-kick", false)
	if got := h.waveformRect.Dx(); got != want {
		t.Errorf("thumbnail width %d, want SynthHeaderThumbW %d", got, want)
	}
}

// TestSynthHeaderWaveformScratchReused — the float32→float64 conversion for
// the header mini-waveform must reuse a cached scratch (no per-Draw alloc).
func TestSynthHeaderWaveformScratchReused(t *testing.T) {
	sample := make([]float32, 1024)
	a := synthHeaderWaveFloat64(sample)
	capA := cap(a)
	b := synthHeaderWaveFloat64(sample)
	if cap(b) != capA {
		t.Errorf("synthHeaderWaveFloat64 reallocated: cap %d → %d", capA, cap(b))
	}
	if len(b) != len(sample) {
		t.Errorf("len=%d, want %d", len(b), len(sample))
	}
}

// Phase 8B unification removed the collapsed-section hint string: every
// standardized section is now either populated or carries an enable
// pill (the section IS used — the pill toggles it), and empty sections are
// pruned by synthSectionOrderForSchema. The hint discipline is replaced by
// TestSynthTab_NoCollapsedHintStringInSource (synth_modular_toggle_test.go) and
// the per-recipe unified-order tests.
