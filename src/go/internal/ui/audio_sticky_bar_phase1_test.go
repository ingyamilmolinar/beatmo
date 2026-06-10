//go:build test

package ui

import (
	"image"
	"testing"
)

// TestStickyBar_SpectrumPillsOnlyOnSpectrum — the slope / Pre / Reset
// Hold pills (Phase 1 audio-panel redesign) are spectrum-only. On any
// other tab their rects must be empty so they cannot be drawn or hit-
// tested. On TabSpectrum each pill must claim a non-empty rect.
func TestStickyBar_SpectrumPillsOnlyOnSpectrum(t *testing.T) {
	tabs := []PanelTab{TabWave, TabMeters, TabEQ, TabScope, TabSynth}
	for _, tab := range tabs {
		bar := NewAudioStickyBar(0, func() {}, func() {}, func() {}, func(PanelTab) {})
		bar.SetActiveTab(tab)
		bar.Layout(image.Rect(0, 0, 800, stickyBarH))
		if slp := bar.SlopeBtn(); slp != nil && !slp.Rect().Empty() {
			t.Errorf("tab=%v: slope pill rect=%v want empty", tab, slp.Rect())
		}
		if pre := bar.PreBtn(); pre != nil && !pre.Rect().Empty() {
			t.Errorf("tab=%v: Pre pill rect=%v want empty", tab, pre.Rect())
		}
		if rst := bar.ResetHoldBtn(); rst != nil && !rst.Rect().Empty() {
			t.Errorf("tab=%v: reset-hold pill rect=%v want empty", tab, rst.Rect())
		}
	}

	bar := NewAudioStickyBar(0, func() {}, func() {}, func() {}, func(PanelTab) {})
	bar.SetActiveTab(TabSpectrum)
	bar.Layout(image.Rect(0, 0, 800, stickyBarH))
	if slp := bar.SlopeBtn(); slp == nil || slp.Rect().Empty() {
		t.Errorf("TabSpectrum: slope pill must have a non-empty rect")
	}
	if pre := bar.PreBtn(); pre == nil || pre.Rect().Empty() {
		t.Errorf("TabSpectrum: Pre pill must have a non-empty rect")
	}
	if rst := bar.ResetHoldBtn(); rst == nil || rst.Rect().Empty() {
		t.Errorf("TabSpectrum: reset-hold pill must have a non-empty rect")
	}
}

// TestStickyBar_SlopePillCyclesValues — each click of the slope pill
// advances through 0 → 3 → 4.5 → 0 and updates the global spectrum
// slope tilt used by the renderer.
func TestStickyBar_SlopePillCyclesValues(t *testing.T) {
	bar := NewAudioStickyBar(0, func() {}, func() {}, func() {}, func(PanelTab) {})
	bar.SetActiveTab(TabSpectrum)
	bar.Layout(image.Rect(0, 0, 800, stickyBarH))

	want := []float64{3, 4.5, 0, 3}
	for i, w := range want {
		bar.SlopeBtn().OnClick()
		if got := bar.SlopeDBPerOct(); got != w {
			t.Errorf("click[%d]: SlopeDBPerOct=%g want %g", i, got, w)
		}
		if got := SpectrumSlope(); got != w {
			t.Errorf("click[%d]: SpectrumSlope (global)=%g want %g", i, got, w)
		}
	}
	SetSpectrumSlope(0)
}

// TestStickyBar_PreOverlayToggle pins the Pre|Post pill semantics.
func TestStickyBar_PreOverlayToggle(t *testing.T) {
	bar := NewAudioStickyBar(0, func() {}, func() {}, func() {}, func(PanelTab) {})
	bar.SetActiveTab(TabSpectrum)
	bar.Layout(image.Rect(0, 0, 800, stickyBarH))
	if bar.PreOverlay() {
		t.Fatalf("PreOverlay default should be false")
	}
	bar.PreBtn().OnClick()
	if !bar.PreOverlay() {
		t.Errorf("PreOverlay after click: false want true")
	}
	bar.PreBtn().OnClick()
	if bar.PreOverlay() {
		t.Errorf("PreOverlay after second click: true want false")
	}
}

// TestHzToNote covers well-known landmarks used by the cursor chip.
func TestHzToNote(t *testing.T) {
	cases := []struct {
		hz   float64
		want string
	}{
		{440.0, "A4"},
		{261.63, "C4"},
		{1046.50, "C6"},
		{2093.0, "C7"},
		{15.0, ""}, // sub-audible → empty
	}
	for _, c := range cases {
		got := hzToNote(c.hz)
		if got != c.want {
			t.Errorf("hzToNote(%g) = %q want %q", c.hz, got, c.want)
		}
	}
}

// TestFormatSlopeLabel pins the chip-label format used by the slope
// pill so screenshot baselines stay stable.
func TestFormatSlopeLabel(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{0, "0dB/o"},
		{3, "3dB/o"},
		{4.5, "4.5dB/o"},
	}
	for _, c := range cases {
		if got := formatSlopeLabel(c.v); got != c.want {
			t.Errorf("formatSlopeLabel(%g) = %q want %q", c.v, got, c.want)
		}
	}
}
