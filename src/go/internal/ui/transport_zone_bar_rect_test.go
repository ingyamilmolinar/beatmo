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

// TestTransportZone_SetBarRectClipsHostedHitAreas verifies the rename
// preserved the prior behavior: vol/view/overflow hit areas have ClipRect
// set to the bar.
func TestTransportZone_SetBarRectClipsHostedHitAreas(t *testing.T) {
	z, _ := newTestTransportZone()
	bar := image.Rect(0, 600, 360, 644)
	// Seed hit areas with the tags that SetBarRect should clip.
	z.hitAreas = append(z.hitAreas,
		HitArea{Tag: "transport-vol-icon"},
		HitArea{Tag: "transport-view-switch"},
		HitArea{Tag: "transport-overflow"},
		HitArea{Tag: "transport-other"},
	)
	z.SetBarRect(bar)
	wantClip := bar
	for i, a := range z.hitAreas {
		if a.Tag == "transport-other" {
			if a.ClipRect != (image.Rectangle{}) {
				t.Errorf("non-bar tag clip mutated: %v", a)
			}
			continue
		}
		if z.hitAreas[i].ClipRect != wantClip {
			t.Errorf("tag=%q ClipRect=%v want %v", a.Tag, z.hitAreas[i].ClipRect, wantClip)
		}
	}
}
