//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestBPMIncrement_RealClickDispatch guards the full real-click dispatch path
// for the BPM+ button — a real press/release at the button's reported rect
// flows through the tree's HitIndex → repeatButtonHitAdapter → Button.OnClick
// (which accumulates z.bpmDelta) → TransportZone.Update() (which applies
// SetBPM on the next frame) → DrumView.BPM(). Existing button tests call
// OnClick() directly and so never exercise this dispatch + deferred-apply
// chain; this asserts a tapped BPM+ button actually raises the BPM on both
// desktop and mobile layouts.
//
// Context: the e2e_transport.browser "Test 3" flaked under 4-job parallel CI
// load. That was a separate timing issue — the JS scenario read getBPM()
// synchronously before the deferred-apply frame ran — fixed by polling in
// src/js/scenarios/e2e_transport.js. This Go test confirms the underlying
// dispatch is correct.
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
			// Faithful to the browser: each click holds the press across several
			// frames (50ms ≈ multiple frames) before releasing.
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

// makeHoldTap presses at a point, holds for holdFrames frames, then releases —
// mirroring the browser's 50ms click-and-hold.
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
