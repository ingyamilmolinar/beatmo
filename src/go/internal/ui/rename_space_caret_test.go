//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRenameSpaceInsertedAtBeginning reproduces the reported bug: open the
// instrument rename field, move the caret to the START (ArrowLeft), press
// Space — the space MUST be inserted at the beginning of the name, not swallowed
// (Space is also the transport play/pause shortcut, so the focused rename box
// must own it). Drives the real integrated frame: handleGlobalShortcuts (the
// keyboard-ownership gate that must yield Space to the box) + the rename portal's
// PollKeyboard update (which runs TextInput.Update to insert the char).
func TestRenameSpaceInsertedAtBeginning(t *testing.T) {
	g := newTestGameForUndo(t)
	rc := g.drum.renameComp
	if rc == nil {
		t.Skip("no rename component")
	}
	rc.SetProps(RenameProps{InitialText: "kick", MaxLen: 32, AnchorRect: image.Rect(0, 0, 120, 24)})
	rc.Open()
	g.drum.openRenamePortal()
	if !rc.IsOpen() {
		t.Skip("rename component did not open (no anchor in this environment)")
	}
	tb := rc.TextBox()
	if tb == nil {
		t.Fatal("rename text box nil")
	}
	if tb.Value() != "kick" {
		t.Fatalf("setup value=%q want %q", tb.Value(), "kick")
	}

	// Move the caret to the very beginning with ArrowLeft (4 discrete presses,
	// releasing between each so keyRepeat treats them as separate presses — as a
	// real user tapping the key does).
	pressLeft := func(down bool) {
		restore := SetInputForTest(
			func() (int, int) { return -1, -1 },
			func(ebiten.MouseButton) bool { return false },
			func(k ebiten.Key) bool { return down && k == ebiten.KeyLeft },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 800, 600 },
		)
		rc.PollKeyboard()
		restore()
	}
	for i := 0; i < 4; i++ {
		pressLeft(true)
		pressLeft(false) // release resets keyRepeat state
	}
	if tb.CursorForTest() != 0 {
		t.Fatalf("after ArrowLeft x4: cursor=%d want 0 (arrow nav to start broken)", tb.CursorForTest())
	}

	// Press Space. isKeyJustPressed(Space) is true (as on real hardware) so this
	// also verifies the transport shortcut does NOT steal it. inputChars carries
	// the ' ' rune the widget inserts.
	restoreChars := SetInputForTest(
		func() (int, int) { return -1, -1 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return []rune(" ") },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	restoreKeys := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySpace: true})
	// Frame order: the global-shortcut gate runs, then the portal update inserts.
	g.handleGlobalShortcuts()
	rc.PollKeyboard()
	restoreKeys()
	restoreChars()

	if got := tb.Value(); got != " kick" {
		t.Fatalf("Space at beginning: got %q want %q", got, " kick")
	}
}
