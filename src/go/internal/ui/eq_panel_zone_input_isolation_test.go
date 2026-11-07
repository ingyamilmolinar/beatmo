//go:build test

package ui

import (
	"image"
	"strings"
	"testing"
)

// At every tab, density and screen class a press inside the panel is claimed by a panel hit area, so lower
// zones (timeline, rack, grid) never see it; whitespace once fell through to the timeline's grid drag.
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

		// Corners, edge midpoints and the centre: chrome, content and likely whitespace.
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

// A press on the synth tab's whitespace between knobs is claimed by the panel alone.
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
	// Between the section cards' knobs, most likely whitespace.
	pt := image.Point{X: (rect.Min.X + rect.Max.X) / 2, Y: (rect.Min.Y*1 + rect.Max.Y*3) / 4}
	if !claimedByPanel(areas, pt) {
		t.Errorf("point=(%d,%d) inside panel but no audio-panel hit area claims it — leak", pt.X, pt.Y)
	}
}

// Panel-owned tags: eq-*, synth-*, chain-*, scope-*.
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
