//go:build test

package ui

import "testing"

// Setup run before the first Layout anchors popups against zero rects (menus clipped or offscreen), so RunScene on
// a game not yet laid out defers it into the update loop, as the browser's runScene export does.
func TestRunSceneDefersSetupUntilLaidOut(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// No Layout yet — mirrors the -scene flag firing before RunGame.
	if err := RunScene(g, "context_menu_open"); err != nil {
		t.Fatalf("RunScene: %v", err)
	}
	if g.drum.IsContextMenuOpen() {
		t.Fatal("scene Setup ran before the game was laid out")
	}

	g.Layout(1280, 720)
	advanceFrames(g, 3)

	if !g.drum.IsContextMenuOpen() {
		t.Fatal("deferred scene Setup never ran after layout")
	}
	// Fully on-screen, not clamped at the viewport's bottom-left.
	r := g.drum.contextMenuRect
	if r.Empty() {
		t.Fatal("context menu rect empty after deferred setup")
	}
	if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		t.Fatalf("context menu rect %v not fully on-screen", r)
	}
}

func TestRunSceneImmediateWhenLaidOut(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 1)

	if err := RunScene(g, "context_menu_open"); err != nil {
		t.Fatalf("RunScene: %v", err)
	}
	if !g.drum.IsContextMenuOpen() {
		t.Fatal("scene Setup did not run synchronously on a laid-out game")
	}
}
