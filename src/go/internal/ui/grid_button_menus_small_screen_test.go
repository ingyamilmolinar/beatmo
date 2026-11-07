//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The menus are portal overlays whose bounds are the drum pane, so on a small screen (a 833 px window, under
// 900) they must still be placed in the GRID pane, never centred or bottom-sheeted in the drum view.
func TestGridButtonMenusStayInGridPaneOnSmallScreen(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(833, 577)
	advanceFrames(g, 2)
	if !Profile().UseBottomSheet {
		t.Fatal("setup: expected small-screen profile (UseBottomSheet) at 833px")
	}
	gridH := g.split.GridH(g.winH)

	// Settings
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay not open")
	}
	if ov.rect.Max.Y > gridH {
		t.Errorf("Settings panel %v spills into the drum pane (gridH=%d) on small screen", ov.rect, gridH)
	}
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)

	// Logs
	g.toggleLogsPanel()
	advanceFrames(g, 2)
	if lr := g.drum.ensureLogsComp().panelRect(); lr.Max.Y > gridH {
		t.Errorf("Logs panel %v spills into the drum pane (gridH=%d) on small screen", lr, gridH)
	}
	g.toggleLogsPanel()
	advanceFrames(g, 2)

	// File
	g.drum.OpenFileMenu()
	advanceFrames(g, 2)
	if fr := g.drum.overflowPopupRect(); fr.Max.Y > gridH {
		t.Errorf("File menu %v spills into the drum pane (gridH=%d) on small screen", fr, gridH)
	}
}
