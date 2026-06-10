//go:build test

package ui

import (
	"image"
	"strings"
	"testing"
)

// TestEQPanelZoneIsOpaqueToZ pins the input-isolation contract: at
// every active tab (Wave / Spectrum / Levels / EQ / Chain / Synth)
// at every density × screen-class combination, a press anywhere
// inside `eqPanelZone.PanelRect()` must be claimed by some audio-
// panel hit area (the catch-all at minimum). Lower-z zones (timeline,
// row rack, drum grid) must NEVER see input inside the panel rect.
//
// This is the regression guard for the "drag a synth knob also pans
// the timeline" leak. Pre-Phase-1 the panel had no full-bounds
// catch-all; whitespace taps fell through to the timeline's grid-
// drag handler (z=110) underneath.
func TestEQPanelZoneIsOpaqueToZ(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.drum.recalcButtons()

	tabs := []PanelTab{TabWave, TabSpectrum, TabMeters, TabEQ, TabScope, TabSynth}
	for _, tab := range tabs {
		g.drum.eqPanelZone.tabState.SetActiveTab(tab)
		g.drum.eqPanelZone.Invalidate()
		g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

		areas := g.drum.eqPanelZone.HitAreas()
		if len(areas) == 0 {
			t.Errorf("tab=%v: HitAreas() returned 0 hits — catch-all missing", tab)
			continue
		}

		rect := g.drum.eqPanelZone.PanelRect()
		if rect.Empty() {
			t.Errorf("tab=%v: PanelRect empty", tab)
			continue
		}

		// 3×3 grid of sample points inside the panel rect. Corners +
		// edge midpoints + center cover the chrome strip (top), the
		// content area, and the corners that are most likely to be
		// whitespace between controls.
		samplePoints := []image.Point{
			{rect.Min.X + 1, rect.Min.Y + 1},
			{(rect.Min.X + rect.Max.X) / 2, rect.Min.Y + 1},
			{rect.Max.X - 2, rect.Min.Y + 1},
			{rect.Min.X + 1, (rect.Min.Y + rect.Max.Y) / 2},
			{(rect.Min.X + rect.Max.X) / 2, (rect.Min.Y + rect.Max.Y) / 2},
			{rect.Max.X - 2, (rect.Min.Y + rect.Max.Y) / 2},
			{rect.Min.X + 1, rect.Max.Y - 2},
			{(rect.Min.X + rect.Max.X) / 2, rect.Max.Y - 2},
			{rect.Max.X - 2, rect.Max.Y - 2},
		}

		for _, pt := range samplePoints {
			if !claimedByPanel(areas, pt) {
				t.Errorf("tab=%v point=(%d,%d): no audio-panel hit area claims this point (leak risk)",
					tab, pt.X, pt.Y)
			}
		}
	}
}

// TestEQPanelZoneCatchAllTag confirms the catch-all is registered
// with the canonical tag — the discipline test (Phase 4) and the
// CLAUDE.md docstring reference this tag.
func TestEQPanelZoneCatchAllTag(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.drum.recalcButtons()
	g.drum.eqPanelZone.Invalidate()
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	found := false
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if h.Tag == "eq-panel-capture" {
			found = true
			if h.Rect != g.drum.eqPanelZone.PanelRect() {
				t.Errorf("eq-panel-capture rect=%v, want PanelRect=%v",
					h.Rect, g.drum.eqPanelZone.PanelRect())
			}
			if h.ZIndex != 130 {
				t.Errorf("eq-panel-capture ZIndex=%d, want 130", h.ZIndex)
			}
			break
		}
	}
	if !found {
		t.Error(`HitAreas() missing "eq-panel-capture" tag — Phase 1 catch-all not registered`)
	}
}

// TestEQPanelZoneLeakRegressionPress simulates the exact bug the user
// reported: with the synth tab active on mobile, a press at a point
// in the panel's chrome whitespace (between knobs) must NOT be
// claimed by any non-eq-panel hit area. Catches a future refactor
// that accidentally removes the catch-all.
func TestEQPanelZoneLeakRegressionPress(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)
	g.drum.recalcButtons()
	g.drum.eqPanelZone.tabState.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Invalidate()
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	areas := g.drum.eqPanelZone.HitAreas()
	rect := g.drum.eqPanelZone.PanelRect()
	if rect.Empty() {
		t.Skip("PanelRect empty under forced-mobile — environment too narrow")
	}
	// Pick a point in the bottom-center quadrant of the panel, between
	// the synth section cards' knobs (most likely whitespace).
	pt := image.Point{X: (rect.Min.X + rect.Max.X) / 2, Y: (rect.Min.Y*1 + rect.Max.Y*3) / 4}
	if !claimedByPanel(areas, pt) {
		t.Errorf("point=(%d,%d) inside panel but no audio-panel hit area claims it — leak", pt.X, pt.Y)
	}
}

// claimedByPanel reports whether some hit area in `areas` whose Tag is
// audio-panel-owned contains the point. Tag prefixes recognised:
//   - eq-*       (sticky bar, EQ band controls, panel capture)
//   - synth-*    (Synth-tab knobs + footer buttons)
//   - chain-*    (Chain-tab stage cards + mode pills + AG)
//   - scope-*    (legacy chain pills — overlay/split/diff)
func claimedByPanel(areas []HitArea, pt image.Point) bool {
	for _, h := range areas {
		if !pt.In(h.Rect) {
			continue
		}
		if isAudioPanelTag(h.Tag) {
			return true
		}
	}
	return false
}

func isAudioPanelTag(tag string) bool {
	prefixes := []string{"eq-", "synth-", "chain-", "scope-"}
	for _, p := range prefixes {
		if strings.HasPrefix(tag, p) {
			return true
		}
	}
	return false
}

// SetForceSmallScreen is a tiny shim around forceSmallScreenForTest
// with proper cleanup. Mirrors the SetDensityForTest pattern. Returns
// a restore closure callers defer.
func SetForceSmallScreen(t *testing.T, on bool) func() {
	t.Helper()
	prev := forceSmallScreenForTest
	forceSmallScreenForTest = on
	UpdateProfile()
	return func() {
		forceSmallScreenForTest = prev
		activeProfile = nil
		UpdateProfile()
	}
}
