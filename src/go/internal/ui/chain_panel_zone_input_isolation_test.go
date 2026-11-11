//go:build test

package ui

import (
	"image"
	"testing"
)

// TestChainPanelZoneCatchAllRegistered confirms the Phase 2 catch-all
// is wired at the chain zone's nominal z. Mirrors the EQ panel test
// shape so both zones obey the same contract.
func TestChainPanelZoneCatchAllRegistered(t *testing.T) {
	cb := ChainCallbacks{}
	z := NewChainPanelZone(cb)
	z.Layout(image.Rect(0, 0, 400, 200))

	var (
		found       bool
		matchedRect image.Rectangle
		zIndex      int
	)
	for _, h := range z.HitAreas() {
		if h.Tag == "chain-panel-capture" {
			found = true
			matchedRect = h.Rect
			zIndex = h.ZIndex
			break
		}
	}
	if !found {
		t.Fatal(`HitAreas() missing "chain-panel-capture" tag — Phase 2 catch-all not registered`)
	}
	if matchedRect != z.Rect() {
		t.Errorf("chain-panel-capture rect=%v, want zone.Rect()=%v", matchedRect, z.Rect())
	}
	if zIndex != 140 {
		t.Errorf("chain-panel-capture ZIndex=%d, want 140", zIndex)
	}
}

// TestChainPanelZoneIsOpaqueToZ — every point inside the chain zone's
// rect is claimed by some chain-owned hit area (catch-all or per-
// control). Pre-Phase-2 a tap in trace-area whitespace fell through
// to lower-z zones. Same contract as the EQ isolation test.
func TestChainPanelZoneIsOpaqueToZ(t *testing.T) {
	cb := ChainCallbacks{}
	z := NewChainPanelZone(cb)
	z.Layout(image.Rect(0, 0, 400, 200))

	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("HitAreas() returned 0 hits")
	}
	rect := z.Rect()
	samples := []image.Point{
		{rect.Min.X + 1, rect.Min.Y + 1},
		{rect.Max.X - 2, rect.Min.Y + 1},
		{rect.Min.X + 1, rect.Max.Y - 2},
		{rect.Max.X - 2, rect.Max.Y - 2},
		{(rect.Min.X + rect.Max.X) / 2, (rect.Min.Y + rect.Max.Y) / 2},
	}
	for _, pt := range samples {
		if !chainClaimedByPanel(areas, pt) {
			t.Errorf("point=(%d,%d) inside chain panel but no chain hit area claims it", pt.X, pt.Y)
		}
	}
}

func chainClaimedByPanel(areas []HitArea, pt image.Point) bool {
	for _, h := range areas {
		if !pt.In(h.Rect) {
			continue
		}
		if h.Tag == "chain-panel-capture" || len(h.Tag) >= 6 && (h.Tag[:6] == "scope-" || h.Tag[:6] == "chain-") {
			return true
		}
	}
	return false
}
