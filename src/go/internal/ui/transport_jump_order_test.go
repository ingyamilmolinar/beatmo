//go:build test

package ui

import "testing"

// TestJumpSelectorBeforeRecordDesktop pins the requested running order: the
// jump-size (×N) selector sits immediately BEFORE the Record button on desktop
// (RW | Play | Stop | FF | ×N | Record | …), not after it.
func TestJumpSelectorBeforeRecordDesktop(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 3)
	if Profile().IsMobile() {
		t.Fatalf("w=1280 should be desktop-class")
	}
	z := g.drum.transportZone

	jump := z.jumpBtn.Rect()
	record := z.recordBtn.Rect()
	ff := z.ffBtn.Rect()
	if jump.Empty() || record.Empty() || ff.Empty() {
		t.Fatalf("desktop: ff/jump/record must all be laid out (ff=%v jump=%v record=%v)", ff, jump, record)
	}
	// ×N selector strictly left of Record.
	if jump.Max.X > record.Min.X {
		t.Fatalf("jump selector must be BEFORE record: jump=%v record=%v", jump, record)
	}
	// And after FF (FF | ×N | Record).
	if ff.Max.X > jump.Min.X {
		t.Fatalf("jump selector must be after FF: ff=%v jump=%v", ff, jump)
	}
}

// TestProgressionButtonsHiddenOnMobile pins that on a mobile layout only the
// Record button remains in the transport — the RW/FF shuttle and the ×N jump
// selector are not shown (empty rects).
func TestProgressionButtonsHiddenOnMobile(t *testing.T) {
	setupMobileTest(t, true) // forces small-screen (mobile) profile, with cleanup

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844) // portrait phone
	advanceFrames(g, 3)
	if !Profile().IsMobile() {
		t.Fatalf("w=390 + small-screen should be mobile-class")
	}
	z := g.drum.transportZone

	if !z.rwBtn.Rect().Empty() {
		t.Fatalf("mobile: rewind button must be hidden, got %v", z.rwBtn.Rect())
	}
	if !z.ffBtn.Rect().Empty() {
		t.Fatalf("mobile: fast-forward button must be hidden, got %v", z.ffBtn.Rect())
	}
	if !z.jumpBtn.Rect().Empty() {
		t.Fatalf("mobile: jump-size (×N) selector must be hidden, got %v", z.jumpBtn.Rect())
	}
	if z.recordBtn.Rect().Empty() {
		t.Fatalf("mobile: record button must still be shown")
	}
}
