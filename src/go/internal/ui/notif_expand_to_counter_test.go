//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestNotifAreaHugsBeatCounterEnd pins the request: the notification area must
// begin a small gap (SpaceSM) after the VISIBLE beat/timer text ends — not
// after a generous stable upper-bound slot. The counter reserved a fixed
// worst-case slot ("Beat 888 · 88:88" desktop, "888 · 8:88" mobile) and the
// notif started after THAT, so with the short live readout ("Beat 1 · 0:00")
// a large dead gap sat between the counter text and the notif, making the
// notif look far smaller than the band allowed. The notif's left edge should
// instead track the rendered readout width so it reclaims that dead space.
func TestNotifAreaHugsBeatCounterEnd(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mobile bool
		w, h   int
	}{
		{"desktop", false, 1280, 720},
		{"mobile", true, 390, 844},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertDefaultParityState(t)
			restore := SetForceSmallScreen(t, tc.mobile)
			defer restore()

			graph := model.NewGraph(testLogger)
			dv := NewDrumView(image.Rect(0, 0, tc.w, tc.h), graph, testLogger)
			dv.recalcButtons()

			if dv.beatCounterRect.Empty() {
				t.Fatal("beatCounterRect is empty")
			}
			if dv.notifRect.Empty() {
				t.Fatal("notifRect is empty — notification area collapsed")
			}

			readout := dv.timelineInfo(0)
			pillW := TextWidth(readout) + 2*beatCounterPillPadX
			pillRight := dv.beatCounterRect.Min.X + pillW
			gap := dv.notifRect.Min.X - pillRight
			t.Logf("%s readout=%q pillW=%d pillRight=%d notif.Min.X=%d gap=%d (SpaceSM=%d) counterSlotDx=%d",
				tc.name, readout, pillW, pillRight, dv.notifRect.Min.X, gap, SpaceSM, dv.beatCounterRect.Dx())

			if gap < 0 {
				t.Fatalf("%s: notif (minX=%d) overlaps the beat/timer text (right edge=%d)",
					tc.name, dv.notifRect.Min.X, pillRight)
			}
			if gap > SpaceSM+1 {
				t.Fatalf("%s: notif starts %dpx past where the beat/timer text ends; it should hug it "+
					"(~SpaceSM=%d). The dead space between the counter text and the notif was not reclaimed — "+
					"the notif is smaller than it should be.", tc.name, gap, SpaceSM)
			}
		})
	}
}
