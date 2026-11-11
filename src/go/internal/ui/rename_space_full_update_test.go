//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRenameSpaceViaFullGameUpdate drives the REAL Game.Update() frame pipeline
// (not manual handleGlobalShortcuts+PollKeyboard) to reproduce the desktop bug:
// rename open, caret at start, press Space -> " kick". Space is stubbed into
// inputChars AND isKeyJustPressed(Space) so the transport-gate path is exercised
// exactly as on native.
func TestRenameSpaceViaFullGameUpdate(t *testing.T) {
	g := newTestGameForUndo(t)
	rc := g.drum.renameComp
	if rc == nil {
		t.Skip("no rename component")
	}
	rc.SetProps(RenameProps{InitialText: "kick", MaxLen: 32, AnchorRect: image.Rect(0, 0, 120, 24)})
	rc.Open()
	g.drum.openRenamePortal()
	if !rc.IsOpen() {
		t.Skip("rename component did not open")
	}
	tb := rc.TextBox()
	if tb == nil {
		t.Fatal("rename text box nil")
	}
	// Caret to the very beginning.
	tb.cursor = 0

	// One real frame with Space held/just-pressed and ' ' in inputChars.
	restoreChars := SetInputForTest(
		func() (int, int) { return -1, -1 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return []rune(" ") },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	restoreKeys := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySpace: true})
	if err := g.Update(); err != nil {
		t.Fatalf("g.Update: %v", err)
	}
	restoreKeys()
	restoreChars()

	got := ""
	if tb2 := rc.TextBox(); tb2 != nil {
		got = tb2.Value()
	}
	if got != " kick" {
		t.Fatalf("Space at beginning via full Update: got %q want %q (rename open=%v)", got, " kick", rc.IsOpen())
	}
}
