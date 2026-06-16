//go:build test

package ui

import (
	"strings"
	"testing"
)

// TestBeatCounterTapDoesNotTriggerNotification pins the user-reported bug: a
// tap on the beat-counter pill opened the notification history popup. Root
// cause — the beat counter is a visible opaque surface that registered NO
// hit area, so its band-mates' touch-expanded hit areas (the notification
// area on its right and the track chip on its left, both Touch:true with
// ClipRect == the whole timeline zone) grew 44 px inward over the pill. A tap
// on the counter resolved to the notif (or track) handler instead of doing
// nothing.
//
// Contract (opaque-to-z): a tap whose exact point lands on the beat counter
// must resolve to the beat counter's own catch-all, NOT a neighbour's
// touch-expanded handler. We assert the WINNING hit (hits[0], the first the
// dispatcher tries) at the centre of the counter pill is not a neighbour's
// interactive handler — the neighbour areas may still appear lower in the
// list via touch expansion, but they must never win.
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
			for _, bad := range []string{"timeline-notif", "timeline-track"} {
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

// TestMobileBeatCounterShowsTime pins the second request: the mobile beat
// counter must show the elapsed time, not just the beat position. Desktop
// already renders "Beat N · M:SS"; mobile previously dropped the time entirely
// ("Beat N"). The compact mobile form keeps both ("N · M:SS").
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
