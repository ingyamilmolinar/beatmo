package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func setTestInput(t *testing.T, pos func() (int, int), mouse func(ebiten.MouseButton) bool, key func(ebiten.Key) bool, runes func() []rune, wheel func() (float64, float64), size func() (int, int)) {
	t.Helper()
	restore := SetInputForTest(pos, mouse, key, runes, wheel, size)
	t.Cleanup(restore)
}

// Verify the wave/EQ divider is hoverable and draggable via the legacy path.
// The row 1/2 boundary is detectable for EQ resizing even though no line is drawn.
func TestWaveDividerHoverAndResize(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), nil, logger)

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
		t.Fatalf("EQ divider should be hoverable: axis=%s idx=%d", dv.layoutHoverAxis, dv.layoutHoverIdx)
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
	if newWave >= initialWave {
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
	if dv.widgetRects[WidgetRack].Dx() <= before {
		t.Fatalf("rack width did not grow after column drag: before=%d after=%d", before, dv.widgetRects[WidgetRack].Dx())
	}
}

// Verify the boundary below the transport is NOT draggable — the Timeline
// widget spans rows 0-1, so the row 0 divider is fully suppressed.
func TestTransportBottomDividerSuppressed(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), nil, logger)
	rowPos := dv.widgets.rowPos
	if len(rowPos) < 2 {
		t.Fatalf("not enough rows for transport divider")
	}
	y := rowPos[1] // boundary under transport
	restore := SetInputForTest(
		func() (int, int) { return dv.Bounds.Max.X / 2, y },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	dv.handleLayoutResize()
	// Row 0 divider should NOT be detected (Timeline spans rows 0-1).
	if dv.layoutHoverAxis == "row" && dv.layoutHoverIdx == 0 {
		t.Fatalf("transport divider should NOT be hoverable: axis=%s idx=%d", dv.layoutHoverAxis, dv.layoutHoverIdx)
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

// Verify that after mixed resizes, the wave/EQ boundary still functions:
// no divider LINE is drawn (suppressed), but the EQ pill remains accessible
// for resizing via the LayoutResizeHandler.
func TestWaveDividerStaysVisibleAfterMixedResizes(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), nil, logger)

	// Shrink overall bottom panel (simulate splitter) by reducing Bounds height.
	dv.SetBounds(image.Rect(0, 300, 1280, 900))

	// After interactions, the wave row should still have positive height.
	if len(dv.widgets.rowPos) < 3 {
		t.Fatalf("missing wave row after resizes")
	}
	waveHeight := dv.widgets.rowPos[2] - dv.widgets.rowPos[1]
	if waveHeight <= 0 {
		t.Fatalf("wave height collapsed after mixed resizes: %d", waveHeight)
	}

	// The EQ pill should exist (non-empty handle rect).
	if dv.layoutHandler != nil {
		hr := dv.layoutHandler.rowHandleRect(1)
		if hr.Empty() {
			t.Fatal("EQ boundary pill should exist after resizes")
		}
	}

	// Draw guides and confirm the wave divider LINE is NOT drawn (suppressed
	// because WidgetWave is full-width below the row 1/2 boundary).
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
	if hit {
		t.Fatalf("wave divider line should NOT be drawn (full-width widget suppression)")
	}
}
