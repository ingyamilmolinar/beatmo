//go:build test

package ui

import (
	"strings"
	"testing"
)

// A tap on the counter resolves to its own catch-all, never to a neighbour's touch-expanded handler (the
// notification area or track chip); neighbours may appear lower in the list but never win.
func TestBeatCounterTapDoesNotTriggerNotification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mobile bool
		w, h   int
	}{
		{"mobile", true, 390, 844},
		{"desktop", false, 1280, 720},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertDefaultParityState(t)
			restore := SetForceSmallScreen(t, tc.mobile)
			defer restore()

			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(tc.w, tc.h)
			g.drum.recalcButtons()
			g.Update()
			g.Update()

			bc := g.drum.beatCounterRect
			if bc.Empty() {
				t.Fatal("beatCounterRect is empty — cannot probe the counter")
			}
			cx := bc.Min.X + bc.Dx()/2
			cy := bc.Min.Y + bc.Dy()/2

			hits := g.drum.tree.HitIndexRef().At(cx, cy)
			if len(hits) == 0 {
				t.Fatalf("no hit areas at beat-counter centre (%d,%d); expected the counter's own catch-all", cx, cy)
			}
			for i, h := range hits {
				t.Logf("  [%d] tag=%q z=%d rect=%v", i, h.Tag, h.ZIndex, h.Rect)
			}
			top := hits[0]
			for _, bad := range []string{"timeline-track"} {
				if top.Tag == bad {
					t.Fatalf("tap on beat counter at (%d,%d) resolves to %q (z=%d rect=%v) — a band-mate's "+
						"touch-expanded hit area leaked over the counter. The beat counter needs its own "+
						"catch-all so neighbour expansions can't steal its taps.",
						cx, cy, top.Tag, top.ZIndex, top.Rect)
				}
			}
		})
	}
}

func TestBeatCounterTapOpensNavigation(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.drum.recalcButtons()
	g.Update()
	g.Update()

	bc := g.drum.beatCounterRect
	if bc.Empty() {
		t.Fatal("beatCounterRect is empty — cannot probe the counter")
	}

	adapter := &beatCounterHitAdapter{zone: g.drum.timelineZone}
	adapter.OnPress(bc.Min.X+bc.Dx()/2, bc.Min.Y+bc.Dy()/2)

	if !g.drum.IsNavMenuOpen() {
		t.Fatal("beat-counter tap must open the navigation menu via OnOpenNavigation")
	}
}

// Both beat and time: "N · M:SS".
func TestMobileBeatCounterShowsTime(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	g.drum.recalcButtons()
	g.Update()

	readout := g.drum.timelineInfo(0)
	t.Logf("mobile beat-counter readout=%q", readout)
	if !strings.Contains(readout, ":") {
		t.Fatalf("mobile beat-counter readout %q has no time component — the time counter was dropped on mobile; "+
			"expected a compact \"N · M:SS\" form", readout)
	}
}
