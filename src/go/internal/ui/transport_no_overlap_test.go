//go:build test

package ui

import (
	"image"
	"testing"
)

// TestDesktopTransportNoOverlap guards against the desktop transport "Stop
// button collapses to a sliver" bug: the weighted top-bar grid produces
// non-overlapping cells, but enforceMinSize used to center-expand play/stop/
// record past their cells into each other at narrow desktop widths (~900-1440
// px), leaving Stop a sliver wedged between Play and Record. Buttons must never
// overlap at any desktop width.
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
		// Stop must not collapse to a sliver: it should keep a usable width
		// comparable to play (within the same cell-derived sizing).
		stopW := g.drum.stopBtn().Rect().Dx()
		playW := g.drum.playBtn().Rect().Dx()
		if stopW > 0 && playW > 0 && stopW < playW/2 {
			t.Errorf("w=%d: stop button is a sliver: stopW=%d playW=%d", w, stopW, playW)
		}
		g.CloseForTest()
	}
}
