//go:build test

package ui

import (
	"testing"
)

// TestNotifArea_HitAreaOpensHistory — the timeline zone registers a
// "timeline-notif" hit area over the notification area, and pressing it opens
// the notif-history portal.
func TestNotifArea_HitAreaOpensHistory(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	for i := 0; i < 4; i++ {
		_ = g.Update()
	}
	dv := g.drum
	dv.notifyInfo("hello world")
	for i := 0; i < 2; i++ {
		_ = g.Update()
	}

	if dv.notifRect.Empty() {
		t.Fatal("precondition: notifRect should be non-empty on desktop")
	}
	var found *HitArea
	for i := range dv.timelineZone.hitAreas {
		if dv.timelineZone.hitAreas[i].Tag == "timeline-notif" {
			found = &dv.timelineZone.hitAreas[i]
			break
		}
	}
	if found == nil {
		t.Fatal("notification area did not register a 'timeline-notif' hit area")
	}
	if found.Handler == nil {
		t.Fatal("'timeline-notif' hit area has no handler")
	}
	found.Handler.OnPress(dv.notifRect.Min.X+1, dv.notifRect.Min.Y+1)
	if !dv.tree.Portal().Has("notif-history") {
		t.Fatal("pressing the notification area did not open the notif-history popup")
	}
}
