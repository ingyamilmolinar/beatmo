//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Through LogsComponent.Update → searchBox.Update, the real path.
func TestLogsSearchCaretInsertMidString(t *testing.T) {
	assertDefaultParityState(t)
	prevSuppress := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prevSuppress })

	g := newGameWithStart(t)
	g.toggleLogsPanel()
	if !g.drum.IsLogsPanelOpen() {
		t.Fatal("logs panel should be open")
	}
	// Draw once to create + lay out the search box.
	g.Draw(ebiten.NewImage(640, 480))
	c := g.drum.logsComp
	if c == nil || c.searchBox == nil {
		t.Fatal("logs search box not created after Draw")
	}
	c.searchBox.FocusForTest(true)

	drive := func(pressed func(ebiten.Key) bool, chars string) {
		restore := SetInputForTest(
			func() (int, int) { return -1, -1 },
			func(ebiten.MouseButton) bool { return false },
			pressed,
			func() []rune { return []rune(chars) },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 640, 480 },
		)
		c.Update()
		restore()
	}
	noKey := func(ebiten.Key) bool { return false }
	leftKey := func(k ebiten.Key) bool { return k == ebiten.KeyLeft }

	// Type "helo".
	drive(noKey, "helo")
	if got := c.searchBox.Value(); got != "helo" {
		t.Fatalf("after typing: got %q want %q", got, "helo")
	}
	// ArrowLeft: caret 4 -> 3 (between 'l' and 'o').
	drive(leftKey, "")
	if cur := c.searchBox.CursorForTest(); cur != 3 {
		t.Fatalf("after ArrowLeft: cursor=%d want 3", cur)
	}
	// Insert 'l' at the caret -> "hello".
	drive(noKey, "l")
	if got := c.searchBox.Value(); got != "hello" {
		t.Fatalf("after mid-insert: got %q want %q", got, "hello")
	}
	if c.nameQuery != "hello" {
		t.Fatalf("nameQuery=%q want %q", c.nameQuery, "hello")
	}

	// Home jumps to start; type 'X' inserts there.
	drive(func(k ebiten.Key) bool { return k == ebiten.KeyHome }, "")
	if cur := c.searchBox.CursorForTest(); cur != 0 {
		t.Fatalf("after Home: cursor=%d want 0", cur)
	}
	drive(noKey, "X")
	if got := c.searchBox.Value(); got != "Xhello" {
		t.Fatalf("after Home+insert: got %q want %q", got, "Xhello")
	}
	// Delete (forward) at start removes 'X'.
	drive(func(k ebiten.Key) bool { return k == ebiten.KeyHome }, "")
	drive(func(k ebiten.Key) bool { return k == ebiten.KeyDelete }, "")
	if got := c.searchBox.Value(); got != "hello" {
		t.Fatalf("after Delete: got %q want %q", got, "hello")
	}
	// End jumps to the end.
	drive(func(k ebiten.Key) bool { return k == ebiten.KeyEnd }, "")
	if cur := c.searchBox.CursorForTest(); cur != 5 {
		t.Fatalf("after End: cursor=%d want 5", cur)
	}
}

// The caret owns ArrowLeft while the panel is open; the grid must not pan.
func TestArrowKeysYieldToLogsSearch(t *testing.T) {
	g := newGameWithStart(t)
	g.toggleLogsPanel()
	if !g.drum.IsLogsPanelOpen() {
		t.Fatal("logs panel should be open")
	}
	if !g.drum.KeyboardClaimed() {
		t.Fatal("logs panel open: KeyboardClaimed() must be true")
	}
	g.cam.OffsetX = 0
	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()
	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowLeft must NOT pan the grid while the logs search is open (OffsetX=%v)", g.cam.OffsetX)
	}
}
