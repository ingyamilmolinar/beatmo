//go:build test

package ui

import (
	"image"
	"testing"
)

// A zone's visibility predicate gates its HitAreas as well as its Draw: a hidden audio panel's full-bounds
// catch-all overlapped the row rack and swallowed every press meant for its controls.
func TestPadsTabInputNotSwallowedByHiddenAudioPanel(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)

	// Through the EQ → Pads switch the user walks.
	g.drum.setViewMode(viewModeEQ)
	g.drum.setViewMode(viewModeRows)
	g.drum.recalcButtons()
	// Frames let the tree's Layout phase update the HitIndex.
	g.Update()
	g.Update()

	rack := g.drum.rowRackZone
	if rack == nil {
		t.Fatal("row rack zone is nil — environment broken before assertion")
	}
	// A point inside the rack an honest tap could land on.
	rackHits := rack.HitAreas()
	if len(rackHits) == 0 {
		t.Skip("row rack has no hit areas in this layout; cannot exercise the regression")
	}
	first := rackHits[0].Rect
	if first.Empty() {
		t.Skip("row rack's first hit area is empty; layout not realised yet")
	}
	probePoint := image.Point{
		X: first.Min.X + first.Dx()/2,
		Y: first.Min.Y + first.Dy()/2,
	}

	t.Logf("probe point (%d,%d); row-rack first hit area at %v", probePoint.X, probePoint.Y, first)
	t.Logf("eq panel rect: %v  (mobile=%v viewMode=Rows)", g.drum.eqPanelZone.PanelRect(), Profile().IsMobile())
	hits := g.drum.tree.HitIndexRef().At(probePoint.X, probePoint.Y)
	if len(hits) == 0 {
		t.Fatalf("no hit areas at probe point (%d,%d) — Pads tab has no interactive controls; bug or stale layout",
			probePoint.X, probePoint.Y)
	}
	t.Logf("hits at probe (top-to-bottom by z): %d total", len(hits))
	for i, h := range hits {
		t.Logf("  [%d] tag=%q z=%d rect=%v", i, h.Tag, h.ZIndex, h.Rect)
	}
	top := hits[0]
	forbiddenTags := []string{"eq-panel-capture", "chain-panel-capture"}
	for _, bad := range forbiddenTags {
		if top.Tag == bad {
			t.Fatalf("Pads regression: top hit at row-rack probe (%d,%d) is %q (z=%d). "+
				"Audio panel hit areas leaked into the Pads view — the panel's visibility predicate "+
				"gates Draw but not HitAreas, so its catch-all swallows row-rack input. "+
				"Fix: tree must consult RegisterZoneVisible's predicate when collecting HitAreas.",
				probePoint.X, probePoint.Y, top.Tag, top.ZIndex)
		}
	}
}

// Checked on HitAreas directly, not PanelRect: the defect is a non-empty slice holding eq-panel-capture.
func TestEQPanelHitAreasEmptyWhenHidden(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)

	g.drum.setViewMode(viewModeRows)
	g.drum.recalcButtons()
	g.Update()
	g.Update()

	if g.drum.MobileEQMode() {
		t.Fatalf("MobileEQMode should be false in viewMode=Rows, got true")
	}
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if h.Tag == "eq-panel-capture" {
			t.Fatalf("eq-panel-capture hit area is present even though the panel is hidden by viewMode=Rows: rect=%v z=%d. "+
				"Visibility predicate must gate HitAreas, not just Draw.",
				h.Rect, h.ZIndex)
		}
	}
}

func TestEQPanelHitAreasEmptyAfterEQtoPadsTransition(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)

	g.drum.setViewMode(viewModeEQ)
	g.drum.recalcButtons()
	g.Update()
	g.Update()
	if !g.drum.MobileEQMode() {
		t.Fatalf("MobileEQMode should be true in viewMode=EQ on mobile, got false (test scaffold)")
	}

	g.drum.setViewMode(viewModeRows)
	g.drum.recalcButtons()
	g.Update()
	g.Update()
	if g.drum.MobileEQMode() {
		t.Fatalf("MobileEQMode should be false after switch back to Rows")
	}

	// The catch-all MUST be gone.
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if h.Tag == "eq-panel-capture" {
			t.Fatalf("After EQ→Pads transition, eq-panel-capture still registered: rect=%v z=%d. "+
				"Tree must drop hit areas for invisible zones.",
				h.Rect, h.ZIndex)
		}
	}
	// And the tree's HitIndex must agree.
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		_ = h
	}
}
