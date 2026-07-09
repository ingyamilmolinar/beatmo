//go:build test

package ui

import "testing"

// The native -scene path used to run Setup synchronously BEFORE Ebiten's
// first Layout/Update, so popup-opening setups anchored against zero rects:
// the context menu rendered clipped in the bottom-left corner, the
// instrument menu landed fully offscreen, and the volume/long-press popups
// captured nothing (A10 in the 2026-07-04 critique — the browser pass was
// always correct because its runScene export defers via QueueAction).
// RunScene on a not-yet-laid-out game must defer Setup into the update
// loop and only then apply it against real geometry.
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
	// The menu must be anchored fully on-screen — the pre-fix failure mode
	// was a rect clamped/clipped at the viewport's bottom-left.
	r := g.drum.contextMenuRect
	if r.Empty() {
		t.Fatal("context menu rect empty after deferred setup")
	}
	if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		t.Fatalf("context menu rect %v not fully on-screen", r)
	}
}

// A scene applied to an already-laid-out game (tests, JS export after boot)
// keeps the synchronous behavior.
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
