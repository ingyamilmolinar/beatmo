//go:build test

package ui

import (
	"image"
	"testing"
)

// Never overlapping at any desktop width: center-expanding to a minimum size once pushed play and record into
// Stop until it was a sliver.
func TestDesktopTransportNoOverlap(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	widths := []int{900, 1000, 1024, 1152, 1280, 1366, 1440, 1680, 1920}
	for _, w := range widths {
		g := New(testLogger)
		g.Layout(w, 720)
		advanceFrames(g, 3)
		if Profile().IsMobile() {
			g.CloseForTest()
			t.Fatalf("w=%d should be desktop-class", w)
		}
		named := []struct {
			name string
			r    image.Rectangle
		}{
			{"play", g.drum.transportZone.playBtn.Rect()},
			{"stop", g.drum.transportZone.stopBtn.Rect()},
			{"record", g.drum.transportZone.recordBtn.Rect()},
		}
		for i := 0; i < len(named); i++ {
			for j := i + 1; j < len(named); j++ {
				a, b := named[i].r, named[j].r
				if a.Empty() || b.Empty() {
					continue
				}
				if ov := a.Intersect(b); !ov.Empty() {
					t.Errorf("w=%d: %s %v overlaps %s %v (overlap=%v)",
						w, named[i].name, a, named[j].name, b, ov)
				}
			}
		}
		// Comparable to play.
		stopW := g.drum.stopBtn().Rect().Dx()
		playW := g.drum.playBtn().Rect().Dx()
		if stopW > 0 && playW > 0 && stopW < playW/2 {
			t.Errorf("w=%d: stop button is a sliver: stopW=%d playW=%d", w, stopW, playW)
		}
		g.CloseForTest()
	}
}
