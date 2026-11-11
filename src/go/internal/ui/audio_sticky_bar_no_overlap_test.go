//go:build test

package ui

import (
	"image"
	"testing"
)

// TestStickyBarTabsNeverOverlapChannel guards the desktop audio sticky-bar
// overlap: the tab pills are placed right-to-left and used to march straight
// over the left-anchored channel pill at narrow widths, producing garbled
// overlapping text ("WaMaSspectrum"). No visible tab pill may overlap the
// channel pill at any width.
func TestStickyBarTabsNeverOverlapChannel(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	if Profile().IsMobile() {
		t.Skip("desktop-only: mobile hides the tab pills")
	}

	for _, w := range []int{1280, 1024, 800, 640, 520, 440, 360} {
		b := NewAudioStickyBar(0, nil, nil)
		b.SetActiveTab(TabEQ)
		b.Layout(image.Rect(0, 0, w, 40))
		ch := b.channelBtn.Rect()
		for i := 0; i < len(b.tabBtns); i++ {
			r := b.tabBtns[i].Rect()
			if r.Empty() {
				continue
			}
			if ov := r.Intersect(ch); !ov.Empty() {
				t.Errorf("w=%d: tab %d %v overlaps channel pill %v (overlap=%v)", w, i, r, ch, ov)
			}
		}
	}
}
