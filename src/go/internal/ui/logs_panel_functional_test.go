//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestGridLogsButtonPresentBothPlatforms asserts the logs button exists on both
// platforms, sits left of the folder button (same size, disjoint), and carries
// the logs icon.
func TestGridLogsButtonPresentBothPlatforms(t *testing.T) {
	check := func(t *testing.T, g *Game) {
		if g.gridLogsBtn == nil {
			t.Fatal("gridLogsBtn is nil")
		}
		if g.gridLogsBtn.Icon != string(IconLogs) {
			t.Fatalf("logs icon=%q want %q", g.gridLogsBtn.Icon, IconLogs)
		}
		file := g.gridFileButtonRect()
		logs := g.gridLogsButtonRect()
		if file.Empty() || logs.Empty() {
			t.Fatalf("rects empty file=%v logs=%v", file, logs)
		}
		if logs.Overlaps(file) {
			t.Fatalf("logs %v overlaps file %v", logs, file)
		}
		if logs.Max.X > file.Min.X {
			t.Fatalf("logs button must sit LEFT of the folder button (logs=%v file=%v)", logs, file)
		}
		if logs.Dx() != file.Dx() || logs.Dy() != file.Dy() {
			t.Fatalf("logs/file size mismatch logs=%v file=%v", logs, file)
		}
	}
	t.Run("desktop", func(t *testing.T) { check(t, newGameWithStart(t)) })
	t.Run("mobile", func(t *testing.T) { check(t, newMobileGame(t)) })
}

// TestIconLogsDispatch verifies the logs glyph is registered in the icon
// renderer (drawIconByID returns true).
func TestIconLogsDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	col := color.NRGBA{255, 255, 255, 255}
	if !drawIconByID(dst, IconLogs, r, col) {
		t.Fatal("drawIconByID returned false for IconLogs; switch case or IconID missing")
	}
}

// TestLogsIconTogglesPanel drives the toggle open/close through the portal.
func TestLogsIconTogglesPanel(t *testing.T) {
	g := newGameWithStart(t)
	if g.drum.IsLogsPanelOpen() {
		t.Fatal("panel should start closed")
	}
	g.toggleLogsPanel()
	if !g.drum.IsLogsPanelOpen() {
		t.Fatal("panel should open on first toggle")
	}
	g.toggleLogsPanel()
	if g.drum.IsLogsPanelOpen() {
		t.Fatal("panel should close on second toggle")
	}
}

// TestLogsPanelAnchorsUnderLogsButton asserts the logs panel drops from the
// grid-pane logs button and paints into the grid pane, not centered in the
// bottom drum pane (the dv.Bounds-clamp bug).
func TestLogsPanelAnchorsUnderLogsButton(t *testing.T) {
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	g.toggleLogsPanel()
	advanceFrames(g, 2)

	logsBtn := g.gridLogsButtonRect()
	if logsBtn.Empty() {
		t.Fatal("logs button rect empty after layout")
	}
	r := g.drum.ensureLogsComp().panelRect()

	if r.Min.Y < logsBtn.Min.Y {
		t.Fatalf("logs panel %v should be at/below the logs button %v", r, logsBtn)
	}
	// Panel STARTS in the grid pane (above the split line), proving it is no
	// longer clamped into the bottom drum pane.
	if r.Min.Y >= g.drum.Bounds.Min.Y {
		t.Fatalf("logs panel %v is pinned into the bottom drum pane (top %d); expected it to start in the grid pane",
			r, g.drum.Bounds.Min.Y)
	}
}
