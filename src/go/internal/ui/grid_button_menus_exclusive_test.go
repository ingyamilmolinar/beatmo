//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTopRightMenusMutuallyExclusive: the three top-right grid-button menus
// (Settings, Logs, File) are mutually exclusive — opening one closes any other
// that is open, so two can never be visible at once.
func TestTopRightMenusMutuallyExclusive(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(testLogOutput(), game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	p := g.drum.portal()

	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	if !p.Has(settingsOverlayID) {
		t.Fatal("settings should be open")
	}

	// Open Logs → Settings must close.
	g.toggleLogsPanel()
	advanceFrames(g, 2)
	if p.Has(settingsOverlayID) {
		t.Error("opening Logs must close Settings (mutually exclusive)")
	}
	if !p.Has(logsPanelID) {
		t.Fatal("logs should be open")
	}

	// Open File → Logs must close.
	g.toggleFileMenu()
	advanceFrames(g, 2)
	if p.Has(logsPanelID) {
		t.Error("opening File must close Logs (mutually exclusive)")
	}
	if !g.drum.IsOverflowMenuOpen() {
		t.Fatal("file menu should be open")
	}

	// Open Settings → File must close.
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	if g.drum.IsOverflowMenuOpen() {
		t.Error("opening Settings must close File (mutually exclusive)")
	}
	if !p.Has(settingsOverlayID) {
		t.Fatal("settings should be open")
	}
}

// TestFileMenuTogglesClosed: tapping the folder button while the File menu is
// open closes it (toggle), mirroring Settings/Logs.
func TestFileMenuTogglesClosed(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(testLogOutput(), game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	g.toggleFileMenu()
	advanceFrames(g, 2)
	if !g.drum.IsOverflowMenuOpen() {
		t.Fatal("first folder tap should open the file menu")
	}
	g.toggleFileMenu()
	advanceFrames(g, 2)
	if g.drum.IsOverflowMenuOpen() {
		t.Error("second folder tap must close the file menu (toggle)")
	}
}
