//go:build test

package ui

import (
	"testing"
)

func laidOutDesktopGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	for i := 0; i < 4; i++ {
		_ = g.Update()
	}
	return g
}

func TestNotifHistory_OpensWithRectWithinBounds(t *testing.T) {
	g := laidOutDesktopGame(t)
	dv := g.drum
	dv.notifyInfo("first")
	dv.notifyError("second boom")

	dv.openNotifHistoryPortal()
	if !dv.IsNotifHistoryOpen() {
		t.Fatal("history popup did not open")
	}
	r := dv.notifHistoryRect
	if r.Empty() {
		t.Fatal("notifHistoryRect empty after open")
	}
	if !r.In(dv.Bounds) {
		t.Fatalf("notifHistoryRect %v not within drum bounds %v", r, dv.Bounds)
	}
}

func TestNotifHistory_ClickTogglesClosed(t *testing.T) {
	g := laidOutDesktopGame(t)
	dv := g.drum
	dv.notifyInfo("hi")

	dv.openNotifHistoryPortal()
	if !dv.IsNotifHistoryOpen() {
		t.Fatal("expected open after first call")
	}
	// A second activation (e.g. clicking the area again) closes it.
	dv.openNotifHistoryPortal()
	if dv.IsNotifHistoryOpen() {
		t.Fatal("expected closed after second activation")
	}
}

func TestNotifHistory_EmptyShowsPlaceholderRect(t *testing.T) {
	g := laidOutDesktopGame(t)
	dv := g.drum
	// No notifications pushed this session and none seeded.
	dv.openNotifHistoryPortal()
	if !dv.IsNotifHistoryOpen() {
		t.Fatal("popup should still open with empty history")
	}
	if dv.notifHistoryRect.Empty() {
		t.Fatal("empty-history popup should still reserve a placeholder rect")
	}
}
