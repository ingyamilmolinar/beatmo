//go:build test

package ui

import (
	"testing"
)

// assertNotifBand checks the unified band ordering: track chip → counter →
// notification area, all inside the timeline widget's X range.
func assertNotifBand(t *testing.T, dv *DrumView, label string) {
	t.Helper()
	track := dv.trackBtn().Rect()
	counter := dv.beatCounterRect
	notif := dv.notifRect

	if track.Empty() {
		t.Fatalf("%s: track chip rect empty (should be the left anchor of the band)", label)
	}
	if counter.Empty() {
		t.Fatalf("%s: beat counter rect empty", label)
	}
	if notif.Empty() {
		t.Fatalf("%s: notifRect empty (dedicated notification area missing)", label)
	}
	// Ordering left→right: track chip, then counter, then notif.
	if track.Max.X > counter.Min.X {
		t.Errorf("%s: track chip (maxX=%d) not left of counter (minX=%d)", label, track.Max.X, counter.Min.X)
	}
	if counter.Max.X > notif.Min.X {
		t.Errorf("%s: counter (maxX=%d) not left of notif area (minX=%d)", label, counter.Max.X, notif.Min.X)
	}
	// Notif area stays within the band's right limit (timeline width).
	if notif.Max.X > dv.timelineRect.Max.X+2 {
		t.Errorf("%s: notif area (maxX=%d) overflows timeline right edge (%d)", label, notif.Max.X, dv.timelineRect.Max.X)
	}
	// Notif area shares the band's vertical extent.
	if notif.Min.Y != counter.Min.Y {
		t.Errorf("%s: notif top (%d) should align with counter top (%d)", label, notif.Min.Y, counter.Min.Y)
	}
}

func TestNotifBandLayout_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	for i := 0; i < 4; i++ {
		_ = g.Update()
	}
	if Profile().IsMobile() {
		t.Fatalf("1280x720 should be desktop-class, got mobile")
	}
	assertNotifBand(t, g.drum, "desktop")
}

func TestNotifBandLayout_Mobile(t *testing.T) {
	setupMobileTest(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	if !Profile().IsMobile() {
		t.Fatalf("360x700 with forceSmallScreen should be mobile-class, got desktop")
	}
	assertNotifBand(t, g.drum, "mobile")
}
