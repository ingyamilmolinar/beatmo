//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestGridFileButtonResetsWhenMenuClosesExternally reproduces the reported bug:
// tapping the folder button opens the File menu, but the release edge is
// swallowed by the menu/scrim that just opened, so the button stays latched
// "pressed" (cap down) with held==1. When the menu later closes for an EXTERNAL
// reason — a failed or cancelled mobile file picker, a click-outside, Esc — the
// button must not stay stuck: its toggle resets and it becomes clickable again.
// reconcileGridButtonToggles (run every Update) is the guarantee.
func TestGridFileButtonResetsWhenMenuClosesExternally(t *testing.T) {
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	fb := g.gridFileBtn
	if fb == nil {
		t.Fatal("grid file button missing")
	}

	// Open the File menu (what the folder button's OnClick does).
	g.toggleFileMenu()
	advanceFrames(g, 1)
	if !g.drum.portal().Has(overflowMenuID) {
		t.Fatal("File menu should be open after toggleFileMenu")
	}
	if !fb.Toggled() {
		t.Error("folder button should read toggled/active while its menu is open")
	}

	// Simulate the swallowed release: the button is left stuck pressed with
	// held latched — exactly the state a menu/scrim leaves it in when it eats
	// the release edge.
	fb.pressed = true
	fb.held = 1

	// The menu closes for an external reason (e.g. the file picker failed or was
	// cancelled). portal.Close stands in for any close path that is NOT a button
	// release routed back to the folder button.
	g.drum.portal().Close(overflowMenuID)
	advanceFrames(g, 1) // one Update → reconcileGridButtonToggles heals (pointer up)

	if g.drum.portal().Has(overflowMenuID) {
		t.Fatal("File menu should be closed")
	}
	if fb.Toggled() {
		t.Error("folder button toggle must reset once its menu is gone")
	}
	if fb.pressed || fb.held != 0 {
		t.Errorf("folder button must not stay stuck-pressed after the menu closed (pressed=%v held=%d)", fb.pressed, fb.held)
	}

	// It must be clickable again: with held reset to 0 a fresh press is a real
	// press edge that fires OnClick (had held stayed at 1, the edge check
	// held==1 would fail on the increment to 2 and swallow the tap).
	clicks := 0
	fb.OnClick = func() { clicks++ }
	r := g.gridFileButtonRect()
	fb.SetRect(r)
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	fb.HandleInputResult(cx, cy, true) // fresh press edge
	if clicks != 1 {
		t.Errorf("folder button must be clickable after the heal; OnClick fired %d times, want 1", clicks)
	}
}

// TestGridButtonsTogglesTrackMenuOpenState pins that every top-right grid button
// (settings gear, folder, logs) reports toggled == "its menu is open", driven by
// the portal (the single source of truth), for both the open and closed states.
func TestGridButtonsTogglesTrackMenuOpenState(t *testing.T) {
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	cases := []struct {
		name   string
		btn    *Button
		toggle func()
		menuID string
	}{
		{"settings", g.gridHelpBtn, g.toggleSettingsOverlay, settingsOverlayID},
		{"file", g.gridFileBtn, g.toggleFileMenu, overflowMenuID},
		{"logs", g.gridLogsBtn, g.toggleLogsPanel, logsPanelID},
	}
	for _, c := range cases {
		if c.btn == nil {
			t.Fatalf("%s: grid button missing", c.name)
		}
		if c.btn.Toggled() {
			t.Errorf("%s: button should not be toggled before its menu opens", c.name)
		}
		c.toggle() // open
		advanceFrames(g, 1)
		if !g.drum.portal().Has(c.menuID) {
			t.Fatalf("%s: menu should be open", c.name)
		}
		if !c.btn.Toggled() {
			t.Errorf("%s: button should be toggled while its menu is open", c.name)
		}
		c.toggle() // close
		advanceFrames(g, 1)
		if c.btn.Toggled() {
			t.Errorf("%s: button toggle must clear when its menu closes", c.name)
		}
	}
}
