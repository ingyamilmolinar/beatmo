//go:build test

package ui

import (
	"image"
	"testing"
)

// Placed right to left, the tabs once marched over the channel pill at narrow widths.
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
