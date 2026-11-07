//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// A real press at the button's rect goes HitIndex → adapter → OnClick (accumulating bpmDelta) → the next frame's
// SetBPM, a deferred chain direct OnClick tests never exercise; on desktop and mobile.
func TestBPMIncrement_RealClickDispatch(t *testing.T) {
	cases := []struct {
		name   string
		mobile bool
		w, h   int
	}{
		{"desktop", false, 1280, 720},
		{"mobile", true, 390, 720},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mobile {
				setupMobileTest(t, true)
			} else {
				assertDefaultParityState(t)
			}

			logger := game_log.New(testLogOutput(), game_log.LevelError)
			g := New(logger)
			t.Cleanup(g.CloseForTest)
			g.Layout(tc.w, tc.h)
			advanceFrames(g, 2)
			dv := g.drum

			incRect := dv.bpmIncBtn().Rect()
			if incRect.Empty() {
				t.Fatalf("bpmIncBtn rect is empty: %v", incRect)
			}

			before := dv.BPM()
			cx := (incRect.Min.X + incRect.Max.X) / 2
			cy := (incRect.Min.Y + incRect.Max.Y) / 2
			// Held across frames, as a browser click is.
			holdTap := makeHoldTap(g, tc.w, tc.h)
			for i := 0; i < 3; i++ {
				holdTap(cx, cy, 4)
				advanceFrames(g, 2)
			}
			after := dv.BPM()
			if after <= before {
				t.Fatalf("BPM did not increase via real click at inc rect %v center (%d,%d): before=%d after=%d", incRect, cx, cy, before, after)
			}
		})
	}
}

// As a 50 ms browser click and hold.
func makeHoldTap(g *Game, w, h int) func(mx, my, holdFrames int) {
	return func(mx, my, holdFrames int) {
		var px, py int
		var pressed bool
		restore := SetInputForTest(
			func() (int, int) { return px, py },
			func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return w, h },
		)
		px, py = mx, my
		pressed = true
		for i := 0; i < holdFrames; i++ {
			g.Update()
		}
		pressed = false
		g.Update()
		restore()
	}
}
