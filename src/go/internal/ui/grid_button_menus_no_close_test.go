//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTopRightMenusHaveNoCloseButton: the three top-right grid-button menus
// (Settings, Logs, File) have NO "×" close button on any platform. They close by
// tapping the same grid button again (toggle), by Esc, or by clicking outside.
func TestTopRightMenusHaveNoCloseButton(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(testLogOutput(), game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	dst := ebiten.NewImage(1280, 720)

	// Settings — no "settings-close" hit area.
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay not open")
	}
	if _, ok := findHitAreaByTag(ov.HitAreas(), "settings-close"); ok {
		t.Error("Settings must have NO close (×) button")
	}
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)

	// Logs — no close (×) button; "Clear history" now occupies the header's
	// top-right (the slot the × used to hold).
	g.toggleLogsPanel()
	advanceFrames(g, 2)
	lc := g.drum.ensureLogsComp()
	lc.Draw(dst)
	pr := lc.panelRect()
	if lc.clearRect.Max.X < pr.Max.X-3*SpaceMD {
		t.Errorf("Logs: Clear history %v should hug the panel's top-right (no × button); panel %v", lc.clearRect, pr)
	}
	g.toggleLogsPanel()
	advanceFrames(g, 2)

	// File — no button in the overflow set carries the close icon.
	g.toggleFileMenu()
	advanceFrames(g, 2)
	g.drum.overflowPopupBtns(g.drum.overflowPopupRect()) // build the button set
	if len(g.drum.overflowBtns) == 0 {
		t.Fatal("file menu buttons not built")
	}
	for _, b := range g.drum.overflowBtns {
		if b != nil && b.Icon == "close" {
			t.Error("File menu must have NO close (×) button")
			break
		}
	}
}
