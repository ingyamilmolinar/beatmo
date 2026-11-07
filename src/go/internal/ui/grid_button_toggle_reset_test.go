//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The menu that opens swallows the button's release, leaving it latched; when the menu closes for an
// external reason the button must unlatch and take taps again.
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

	// The state a menu that eats the release edge leaves behind.
	fb.pressed = true
	fb.held = 1

	// Any close that is not a release routed back to the button.
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

	// With held back at 0 a fresh press is a real edge; stuck at 1, the increment to 2 swallows the tap.
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

// Every button's toggled state is "its menu is open", read from the portal.
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
