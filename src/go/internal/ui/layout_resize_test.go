package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func setTestInput(t *testing.T, pos func() (int, int), mouse func(ebiten.MouseButton) bool, key func(ebiten.Key) bool, runes func() []rune, wheel func() (float64, float64), size func() (int, int)) {
	t.Helper()
	restore := SetInputForTest(pos, mouse, key, runes, wheel, size)
	t.Cleanup(restore)
}

// Verify the wave/timeline divider is hoverable and draggable.
func TestWaveDividerHoverAndResize(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), nil, logger)

	// Position cursor on the boundary between timeline row (index 1) and wave row (index 2).
	rowPos := dv.widgets.rowPos
	if len(rowPos) < 3 {
		t.Fatalf("expected at least 3 row boundaries, got %d", len(rowPos))
	}
	yBoundary := rowPos[2] // start of wave row
	restore := SetInputForTest(
		func() (int, int) { return dv.Bounds.Max.X / 2, yBoundary },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()

	dv.handleLayoutResize()
	if dv.layoutHoverAxis != "row" || dv.layoutHoverIdx != 1 {
		t.Fatalf("hover should detect wave divider: axis=%s idx=%d", dv.layoutHoverAxis, dv.layoutHoverIdx)
	}

	// Drag the divider downward to shrink the wave widget.
	initialWave := dv.widgetRects[WidgetWave].Dy()
	leftDown := true
	setTestInput(t,
		func() (int, int) { return dv.Bounds.Max.X / 2, yBoundary },
		func(btn ebiten.MouseButton) bool { return leftDown },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize() // start drag
	// Move cursor down
	setTestInput(t,
		func() (int, int) { return dv.Bounds.Max.X / 2, yBoundary + 40 },
		func(btn ebiten.MouseButton) bool { return leftDown },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize() // apply delta
	leftDown = false
	dv.handleLayoutResize() // release

	dv.refreshWidgetLayout()
	newWave := dv.widgetRects[WidgetWave].Dy()
	if !(newWave < initialWave) {
		t.Fatalf("wave height should shrink after dragging divider: before=%d after=%d", initialWave, newWave)
	}
}

func TestWaveResizeDoesNotShrinkTopRow(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1200, 700), nil, logger)
	topBefore := dv.widgets.RowHeight(0)
	// Drag boundary between row1 and row2 upward.
	dv.widgets.ResizeAxis("row", 1, -80)
	dv.refreshWidgetLayout()
	topAfter := dv.widgets.RowHeight(0)
	if topAfter < topBefore-1 {
		t.Fatalf("top row shrank after wave resize: before=%d after=%d", topBefore, topAfter)
	}
}

// Verify column divider hover and resize uses the same grey highlight.
func TestColumnDividerHover(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), nil, logger)
	x := dv.widgets.colPos[1] // between rack and timeline
	restore := SetInputForTest(
		func() (int, int) { return x, dv.Bounds.Min.Y + 10 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	dv.handleLayoutResize()
	if dv.layoutHoverAxis != "col" || dv.layoutHoverIdx != 0 {
		t.Fatalf("column hover not detected: axis=%s idx=%d", dv.layoutHoverAxis, dv.layoutHoverIdx)
	}
	// Drag to expand rack.
	left := true
	setTestInput(t,
		func() (int, int) { return x, dv.Bounds.Min.Y + 10 },
		func(ebiten.MouseButton) bool { return left },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	before := dv.widgetRects[WidgetRack].Dx()
	dv.handleLayoutResize()
	setTestInput(t,
		func() (int, int) { return x + 40, dv.Bounds.Min.Y + 10 },
		func(ebiten.MouseButton) bool { return left },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize()
	left = false
	dv.handleLayoutResize()
	if !(dv.widgetRects[WidgetRack].Dx() > before) {
		t.Fatalf("rack width did not grow after column drag: before=%d after=%d", before, dv.widgetRects[WidgetRack].Dx())
	}
}

// Drag the boundary below the transport to ensure it is draggable and visible.
func TestTransportBottomDividerDraggable(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), nil, logger)
	rowPos := dv.widgets.rowPos
	if len(rowPos) < 2 {
		t.Fatalf("not enough rows for transport divider")
	}
	y := rowPos[1] // boundary under transport
	beforeTop := rowPos[1] - rowPos[0]
	restore := SetInputForTest(
		func() (int, int) { return dv.Bounds.Max.X / 2, y },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	dv.handleLayoutResize() // hover
	if dv.layoutHoverAxis != "row" || dv.layoutHoverIdx != 0 {
		t.Fatalf("expected hover on transport divider, got axis=%s idx=%d", dv.layoutHoverAxis, dv.layoutHoverIdx)
	}
	left := true
	setTestInput(t,
		func() (int, int) { return dv.Bounds.Max.X / 2, y },
		func(ebiten.MouseButton) bool { return left },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize() // start drag
	setTestInput(t,
		func() (int, int) { return dv.Bounds.Max.X / 2, y + 40 },
		func(ebiten.MouseButton) bool { return left },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize() // apply delta
	left = false
	dv.handleLayoutResize() // release
	dv.refreshWidgetLayout()
	afterTop := dv.widgets.rowPos[1] - dv.widgets.rowPos[0]
	if !(afterTop > beforeTop) {
		t.Fatalf("transport height did not grow after drag: before=%d after=%d", beforeTop, afterTop)
	}
}

func TestWidgetBoundariesVisibleAfterResize(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1200, 700), nil, logger)
	dv.refreshWidgetLayout()
	colGap := dv.widgets.colPos[1] - dv.widgets.colPos[0]
	if colGap <= 0 {
		t.Fatalf("initial column divider collapsed")
	}
	rowGap1 := dv.widgets.rowPos[1] - dv.widgets.rowPos[0]
	rowGap2 := dv.widgets.rowPos[2] - dv.widgets.rowPos[1]
	if rowGap1 <= 0 || rowGap2 <= 0 {
		t.Fatalf("initial row divider collapsed")
	}

	// Resize widgets and redraw
	dv.widgets.ResizeAxis("row", 1, 60)
	dv.widgets.ResizeAxis("col", 0, 40)
	dv.refreshWidgetLayout()
	colGap = dv.widgets.colPos[1] - dv.widgets.colPos[0]
	if colGap <= 0 {
		t.Fatalf("column divider collapsed after resize")
	}
	rowGap1 = dv.widgets.rowPos[1] - dv.widgets.rowPos[0]
	rowGap2 = dv.widgets.rowPos[2] - dv.widgets.rowPos[1]
	if rowGap1 <= 0 || rowGap2 <= 0 {
		t.Fatalf("row divider collapsed after resize")
	}
}

// Reproduce scenario: drag wave divider, resize overall panel, drag other dividers; wave line stays visible.
func TestWaveDividerStaysVisibleAfterMixedResizes(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), nil, logger)
	// 1) Drag wave divider
	yWave := dv.widgets.rowPos[2]
	setTestInput(t,
		func() (int, int) { return dv.Bounds.Max.X / 2, yWave },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize()
	setTestInput(t,
		func() (int, int) { return dv.Bounds.Max.X / 2, yWave - 30 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize()
	setTestInput(t,
		func() (int, int) { return dv.Bounds.Max.X / 2, yWave - 30 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize()

	// 2) Shrink overall bottom panel (simulate splitter) by reducing Bounds height.
	dv.SetBounds(image.Rect(0, 300, 1280, 900))

	// 3) Drag column divider
	x := dv.widgets.colPos[1]
	setTestInput(t,
		func() (int, int) { return x, dv.Bounds.Min.Y + 20 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize()
	setTestInput(t,
		func() (int, int) { return x + 30, dv.Bounds.Min.Y + 20 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize()
	setTestInput(t,
		func() (int, int) { return x + 30, dv.Bounds.Min.Y + 20 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.handleLayoutResize()

	// After all interactions the wave divider should still have thickness and positive height.
	if len(dv.widgets.rowPos) < 3 {
		t.Fatalf("missing wave row after resizes")
	}
	waveHeight := dv.widgets.rowPos[2] - dv.widgets.rowPos[1]
	if waveHeight <= 0 {
		t.Fatalf("wave height collapsed after mixed resizes: %d", waveHeight)
	}
	// Draw guides and confirm divider line passes through the expected sample point
	// without requiring GPU readback (real Ebiten forbids At before a game loop).
	xSample := dv.Bounds.Min.X + dv.Bounds.Dx()/2
	ySample := dv.widgets.rowPos[2]
	hit := false
	orig := drawRect
	t.Cleanup(func() { drawRect = orig })
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if xSample >= r.Min.X && xSample < r.Max.X && ySample >= r.Min.Y && ySample < r.Max.Y {
			hit = true
		}
		orig(dst, r, c, filled)
	}
	img := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	dv.drawLayoutGuides(img)
	drawRect = orig
	if !hit {
		t.Fatalf("wave divider line not visible after mixed resizes")
	}
}
