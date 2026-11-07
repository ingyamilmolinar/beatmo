//go:build test

package ui

import (
	"image"
	"testing"
)

// Kept for the clip override and the segmented control's hit area.
func TestTransportZone_SetBarRectStoresField(t *testing.T) {
	z, _ := newTestTransportZone()
	bar := image.Rect(0, 600, 360, 644)
	z.SetBarRect(bar)
	if got := z.BarRect(); got != bar {
		t.Fatalf("BarRect()=%v, want %v", got, bar)
	}
}

// Only stores geometry: vol/view/overflow clip to the toolbar, and clipping them to the bar would cull their taps.
func TestTransportZone_SetBarRectDoesNotMutateHitAreas(t *testing.T) {
	z, _ := newTestTransportZone()
	bar := image.Rect(0, 600, 360, 644)
	z.hitAreas = append(z.hitAreas,
		HitArea{Tag: "transport-vol-icon"},
		HitArea{Tag: "transport-view-switch"},
		HitArea{Tag: "transport-subdiv"},
		HitArea{Tag: "transport-other"},
	)
	z.SetBarRect(bar)
	for _, a := range z.hitAreas {
		if a.ClipRect != (image.Rectangle{}) {
			t.Errorf("tag=%q ClipRect=%v should remain zero — Theme 4 keeps vol/overflow on the top toolbar", a.Tag, a.ClipRect)
		}
	}
}
