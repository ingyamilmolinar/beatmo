//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// makeDrumViewWithManyRows creates a DrumView with enough rows to require scrolling.
func makeDrumViewWithManyRows(t *testing.T, nRows int) *DrumView {
	t.Helper()
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 300), nil, logger)
	for i := 1; i < nRows; i++ {
		dv.AddRow()
	}
	// Ensure layout is computed.
	dv.Update()
	if dv.visibleRows() >= len(dv.Rows) {
		t.Fatalf("test requires more rows than visible: rows=%d vis=%d", len(dv.Rows), dv.visibleRows())
	}
	return dv
}

// stepsCenter returns the center of the timeline stepsRect.
func stepsCenter(dv *DrumView) (int, int) {
	r := dv.timelineZone.StepsRect()
	return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
}

// TestTimelineGridWheelScrollsRows verifies that wheel events over the
// timeline grid's stepsRect scroll rows up/down via the row rack zone.
func TestTimelineGridWheelScrollsRows(t *testing.T) {
	assertDefaultParityState(t)
	dv := makeDrumViewWithManyRows(t, 20)
	sx, sy := stepsCenter(dv)

	// Position cursor over steps area.
	restore := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, -1 }, // wheel down
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)

	before := dv.rowOffset
	dv.Update()
	restore()

	if dv.rowOffset <= before {
		t.Fatalf("wheel down over grid should scroll rows down: before=%d after=%d", before, dv.rowOffset)
	}

	// Row scroll over the cell grid reuses the SAME behavior as scrolling over
	// the instrument labels: one row per notch, throttled by a step cooldown.
	// Let the cooldown elapse before reversing direction (a real user wheeling
	// up after wheeling down clears it the same way).
	idle := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	for i := 0; i < controlGridScrollCooldownFrames+1; i++ {
		dv.Update()
	}
	idle()

	// Now wheel up to scroll back.
	restore = SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 1 }, // wheel up
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)

	afterDown := dv.rowOffset
	dv.Update()
	restore()

	if dv.rowOffset >= afterDown {
		t.Fatalf("wheel up over grid should scroll rows up: before=%d after=%d", afterDown, dv.rowOffset)
	}
}

// TestTimelineGridWheelDoesNotChangeLength verifies that wheel events over
// the grid don't change the beat length (only row scroll).
func TestTimelineGridWheelDoesNotChangeLength(t *testing.T) {
	assertDefaultParityState(t)
	dv := makeDrumViewWithManyRows(t, 20)
	sx, sy := stepsCenter(dv)
	origLen := dv.Length

	restore := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, -3 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)

	for i := 0; i < 5; i++ {
		dv.Update()
	}
	restore()

	if dv.Length != origLen {
		t.Fatalf("wheel over grid must not change length: before=%d after=%d", origLen, dv.Length)
	}
}

// TestTimelineGridVerticalDragScrollsRows verifies that a vertical drag in
// the stepsRect scrolls rows (not the horizontal offset).
func TestTimelineGridVerticalDragScrollsRows(t *testing.T) {
	assertDefaultParityState(t)
	dv := makeDrumViewWithManyRows(t, 20)
	sx, sy := stepsCenter(dv)
	rh := dv.rowHeight()

	origOffset := dv.Offset
	origRowOffset := dv.rowOffset

	// Press at center of steps area.
	curX, curY := sx, sy
	restore := SetInputForTest(
		func() (int, int) { return curX, curY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	dv.Update()

	// Drag UPWARD (decreasing Y) by more than 2 row heights.
	// Direct manipulation: drag up = scroll down = see more rows below.
	curY = sy - rh*3
	for i := 0; i < 3; i++ {
		dv.Update()
	}
	restore()

	// Release.
	restore = SetInputForTest(
		func() (int, int) { return curX, curY },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	dv.Update()
	restore()

	// Vertical drag should scroll rows.
	if dv.rowOffset == origRowOffset {
		t.Fatalf("vertical drag over grid should scroll rows: rowOffset unchanged at %d", dv.rowOffset)
	}
	// Horizontal offset should NOT change.
	if dv.Offset != origOffset {
		t.Fatalf("vertical drag should not change horizontal offset: before=%d after=%d", origOffset, dv.Offset)
	}
}

// TestTimelineGridHorizontalDragScrollsOffset verifies that a horizontal drag
// in the stepsRect scrolls the timeline offset (not the row scroll).
func TestTimelineGridHorizontalDragScrollsOffset(t *testing.T) {
	assertDefaultParityState(t)
	dv := makeDrumViewWithManyRows(t, 20)
	sx, sy := stepsCenter(dv)

	origRowOffset := dv.rowOffset

	// Set a non-zero offset so we can drag left to increase it.
	dv.Offset = 0

	// Press at center.
	restore := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	dv.Update()
	restore()

	// Drag left (negative X) by several cells — this should increase Offset.
	cell := dv.cell
	if cell < 1 {
		cell = 10 // fallback
	}
	dragX := sx - cell*5
	restore = SetInputForTest(
		func() (int, int) { return dragX, sy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	for i := 0; i < 3; i++ {
		dv.Update()
	}
	restore()

	// Release.
	restore = SetInputForTest(
		func() (int, int) { return dragX, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	dv.Update()
	restore()

	// Horizontal drag should change offset.
	if dv.Offset == 0 {
		t.Fatalf("horizontal drag over grid should change offset: still at 0")
	}
	// Row offset should NOT change.
	if dv.rowOffset != origRowOffset {
		t.Fatalf("horizontal drag should not change row offset: before=%d after=%d", origRowOffset, dv.rowOffset)
	}
}
