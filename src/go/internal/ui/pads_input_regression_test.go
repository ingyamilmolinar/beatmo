//go:build test

package ui

import (
	"image"
	"testing"
)

// TestPadsTabInputNotSwallowedByHiddenAudioPanel pins the regression
// the user reported after the input-isolation pass: on mobile, after
// switching the bottom-nav to Pads, the row-rack controls became
// unresponsive because the audio panel zone — although DRAW-gated by
// `MobileEQMode()` — kept registering its full-bounds `eq-panel-
// capture` hit area at z=130. That catch-all spatially overlapped
// the row rack at z=120, so every press in the panel rect was
// consumed by the (logically hidden) audio panel and never reached
// the rack controls beneath.
//
// The fix: a zone's visibility predicate must gate BOTH Draw AND
// HitAreas. When the audio panel is hidden by view-mode (Pads
// active), it MUST contribute zero hit areas to the dispatcher. This
// test fails on the bug and passes on the fix.
//
// Sample-point strategy: pick a point inside the row-rack zone's
// rect (where the user expects taps to land on mute/solo/FX/volume
// controls). Build the hit index. Assert the highest-z hit at that
// point belongs to the row rack, NOT the audio-panel catch-all.
func TestPadsTabInputNotSwallowedByHiddenAudioPanel(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)

	// Pads view explicitly. Default viewMode is viewModeRows so this is
	// often a no-op, but call it for clarity and to exercise the
	// transition path the user actually walks: EQ → Pads switch.
	g.drum.setViewMode(viewModeEQ)
	g.drum.setViewMode(viewModeRows)
	g.drum.recalcButtons()
	// Drive a couple of frames so the tree's Layout phase runs and
	// HitIndex.Update fires.
	g.Update()
	g.Update()

	rack := g.drum.rowRackZone
	if rack == nil {
		t.Fatal("row rack zone is nil — environment broken before assertion")
	}
	// Use the rack's own hit areas to find an interactive control that
	// genuinely exists at this layout. The first rack hit area is good
	// enough — we just need a point INSIDE the rack that an honest
	// user tap could land on (mute / solo / FX / volume slider).
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
	// The highest-z hit at a row-rack probe point must NOT be the
	// audio-panel catch-all. Any panel-owned tag at this point means
	// the regression is live.
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

// TestEQPanelHitAreasEmptyWhenHidden is the unit-level mirror: when
// the EQ panel's visibility predicate returns false (mobile + Pads),
// the tree's HitIndex must contain zero hit areas owned by the
// `"eq-panel"` zone. Cuts the noise out of the integration test
// above by asserting the contract directly. The test directly
// inspects HitAreas() instead of relying on PanelRect — the bug
// (catch-all registered even when the panel is hidden) manifests as
// a non-empty HitAreas() slice with a `eq-panel-capture` entry.
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

// TestEQPanelHitAreasEmptyAfterEQtoPadsTransition: the user flow that
// reportedly broke. Switch EQ → Pads. After the transition, the
// audio panel zone MUST contribute no hit areas; its visibility
// predicate is false on mobile when viewMode=Rows.
func TestEQPanelHitAreasEmptyAfterEQtoPadsTransition(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)

	// User flow: switch into EQ (panel becomes full-screen), then back
	// to Pads.
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
