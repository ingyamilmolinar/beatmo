//go:build test

package ui

import (
	"image"
	"testing"
)

// TestTransportZone_SetBarRectStoresField verifies SetBarRect persists the
// rect for downstream consumers (hit-area clip override AND the new
// segmented-control hit area registered in Phase 3). Prior API
// (SetBottomBarHostedHitClip) only mutated existing hit-areas in place;
// nothing read back the bar geometry.
func TestTransportZone_SetBarRectStoresField(t *testing.T) {
	z, _ := newTestTransportZone()
	bar := image.Rect(0, 600, 360, 644)
	z.SetBarRect(bar)
	if got := z.BarRect(); got != bar {
		t.Fatalf("BarRect()=%v, want %v", got, bar)
	}
}

// TestTransportZone_SetBarRectDoesNotMutateHitAreas (Theme 4) verifies
// SetBarRect now only stores the bar geometry — vol/view/overflow live
// in the top toolbar with ClipRect = z.rect, NOT the bar. Mutating their
// clip to the bar (the prior behavior) would cull top-toolbar taps.
func TestTransportZone_SetBarRectDoesNotMutateHitAreas(t *testing.T) {
	z, _ := newTestTransportZone()
	bar := image.Rect(0, 600, 360, 644)
	z.hitAreas = append(z.hitAreas,
		HitArea{Tag: "transport-vol-icon"},
		HitArea{Tag: "transport-view-switch"},
		HitArea{Tag: "transport-overflow"},
		HitArea{Tag: "transport-other"},
	)
	z.SetBarRect(bar)
	for _, a := range z.hitAreas {
		if a.ClipRect != (image.Rectangle{}) {
			t.Errorf("tag=%q ClipRect=%v should remain zero — Theme 4 keeps vol/overflow on the top toolbar", a.Tag, a.ClipRect)
		}
	}
}
