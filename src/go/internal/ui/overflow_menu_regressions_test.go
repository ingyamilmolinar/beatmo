//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newDrumViewForOverflowTest(t *testing.T, bounds image.Rectangle) *DrumView {
	t.Helper()
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8
	// The full-width transport rect gives layout room (the labelW+controlsW fallback is too narrow on mobile), and
	// headerH follows the active bar height rather than the constructor's default.
	dv.headerH = ActiveTopBarSpec().Height
	dv.widgetRects[WidgetTransport] = image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Min.Y+dv.headerH)
	// Below the bar, or the timeline-scrub band lands inside it.
	dv.widgetRects[WidgetTimeline] = image.Rect(bounds.Min.X, bounds.Min.Y+dv.headerH, bounds.Max.X, bounds.Min.Y+dv.headerH+timelineHeight)
	W, H := bounds.Dx(), bounds.Dy()
	reset := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	reset()
	return dv
}

// The picker rects must follow the Upload and Import buttons wherever they sit in the menu, never fixed indices.
func TestOverflowFilePickerRectsMatchUploadImportButtons(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	// Only positions relative to the popup matter.
	const W, H = 390, 300
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	// Open, so overflowPopupRect is valid and filePickerCandidates is unblocked.
	dv.SetOverflowMenuOpen(true)
	dv.overflowPage = 0

	// Returns the rects before gating, with no JS registry side effect.
	captured := dv.filePickerCandidates()

	if len(captured) < 2 {
		t.Fatalf("expected ≥2 file picker rects (upload + import), got %d", len(captured))
	}

	// Find upload and import by ID.
	idxByID := map[string]int{}
	for i, c := range captured {
		idxByID[c.Intent.ID] = i
	}
	uploadIdx, hasUpload := idxByID["upload"]
	importIdx, hasImport := idxByID["import"]
	if !hasUpload {
		t.Fatal("no file picker rect registered with id='upload'")
	}
	if !hasImport {
		t.Fatal("no file picker rect registered with id='import'")
	}

	uploadRect := captured[uploadIdx].Rect
	importRect := captured[importIdx].Rect

	// Both rects must be non-empty.
	if uploadRect.Dx() <= 0 || uploadRect.Dy() <= 0 {
		t.Fatalf("upload rect has non-positive size: %+v", uploadRect)
	}
	if importRect.Dx() <= 0 || importRect.Dy() <= 0 {
		t.Fatalf("import rect has non-positive size: %+v", importRect)
	}

	// Find the popup rect and item list to compute expected positions.
	popupRect := dv.overflowPopupRect()
	items := dv.overflowItems()
	rowH := touchMinTargetPx

	uploadItemIdx := -1
	importItemIdx := -1
	for i, item := range items {
		switch item.label {
		case "Upload":
			uploadItemIdx = i
		case "Import":
			importItemIdx = i
		}
	}
	if uploadItemIdx < 0 {
		t.Fatal("overflowItems() does not contain 'Upload'")
	}
	if importItemIdx < 0 {
		t.Fatal("overflowItems() does not contain 'Import'")
	}

	// Checked against each item's band, not the exact inset.
	expectedUploadBandY := popupRect.Min.Y + uploadItemIdx*rowH
	expectedImportBandY := popupRect.Min.Y + importItemIdx*rowH

	// upload rect must be inside the Upload item band
	if uploadRect.Min.Y < expectedUploadBandY || uploadRect.Min.Y >= expectedUploadBandY+rowH {
		t.Errorf("upload file picker rect Y=%d is not in Upload item band [%d, %d) (item index %d)\n"+
			"  This means filePickerCandidates uses wrong item index (likely hardcoded 0 instead of %d)",
			uploadRect.Min.Y, expectedUploadBandY, expectedUploadBandY+rowH, uploadItemIdx, uploadItemIdx)
	}

	// import rect must be inside the Import item band
	if importRect.Min.Y < expectedImportBandY || importRect.Min.Y >= expectedImportBandY+rowH {
		t.Errorf("import file picker rect Y=%d is not in Import item band [%d, %d) (item index %d)\n"+
			"  This means filePickerCandidates uses wrong item index (likely hardcoded 1 instead of %d)",
			importRect.Min.Y, expectedImportBandY, expectedImportBandY+rowH, importItemIdx, importItemIdx)
	}
}

// A popup that flips above its button must stay inside the drum pane: escaping into the grid pane routes its
// clicks to the grid editor, which creates nodes.
func TestOverflowPopupRectAlwaysWithinDrumBounds(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	// Little room below the button, forcing the flip.
	const drumTop = 600
	const screenH = 800
	const W = 400
	drumBounds := image.Rect(0, drumTop, W, screenH)

	dv := newDrumViewForOverflowTest(t, drumBounds)
	dv.SetOverflowMenuOpen(true)

	popupRect := dv.overflowPopupRect()

	if popupRect.Empty() {
		t.Fatal("overflowPopupRect() returned empty rect")
	}

	// The popup must be fully within the drum pane (vertically).
	if popupRect.Min.Y < dv.Bounds.Min.Y {
		t.Errorf("overflowPopupRect().Min.Y=%d < dv.Bounds.Min.Y=%d: popup extends above drum pane into grid area",
			popupRect.Min.Y, dv.Bounds.Min.Y)
	}
	if popupRect.Max.Y > dv.Bounds.Max.Y {
		t.Errorf("overflowPopupRect().Max.Y=%d > dv.Bounds.Max.Y=%d: popup extends below drum pane",
			popupRect.Max.Y, dv.Bounds.Max.Y)
	}
}

// A popup rect outside dv.Bounds makes the dispatcher skip the drum view, and the click becomes a grid tap.
func TestOverflowPopupClickDoesNotCreateGridNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	// Ensure the drum pane is positioned in the bottom portion of the screen.
	if g.drum.Bounds.Empty() {
		t.Fatal("drum bounds empty after layout")
	}
	if g.drum.Bounds.Min.Y < 200 {
		t.Skipf("drum pane not in bottom portion (Bounds.Min.Y=%d); layout may have changed", g.drum.Bounds.Min.Y)
	}

	// Count existing nodes.
	nodesBefore := len(g.graph.Nodes)

	// Open the overflow menu.
	g.drum.SetOverflowMenuOpen(true)

	// Get the popup rect and click at a menu item position.
	popupRect := g.drum.overflowPopupRect()
	if popupRect.Empty() {
		t.Fatal("overflowPopupRect() is empty")
	}

	// Click at the center of the popup (should be consumed by drum view overlay).
	clickX := popupRect.Min.X + popupRect.Dx()/2
	clickY := popupRect.Min.Y + popupRect.Dy()/2

	reset := SetInputForTest(
		func() (int, int) { return clickX, clickY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 800 },
	)
	g.Update()
	reset()

	// Release click.
	reset2 := SetInputForTest(
		func() (int, int) { return clickX, clickY },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 800 },
	)
	g.Update()
	reset2()

	nodesAfter := len(g.graph.Nodes)
	if nodesAfter > nodesBefore {
		t.Errorf("clicking overflow popup created %d new grid node(s); popup rect %v is outside drum bounds %v",
			nodesAfter-nodesBefore, popupRect, g.drum.Bounds)
	}
}

// The menu opens during drum.Update, after the dispatcher; on the next frame the still-held press outside it
// reads as a click outside unless the open suppresses clicks until release.
func TestOverflowMenuStaysOpenAfterButtonTap(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 600
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	// The suppression flag is the state the real open path leaves behind for a not-yet-released touch.
	dv.openOverflowMenuPortal()
	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	if !dv.IsOverflowMenuOpen() {
		t.Fatal("expected overflow menu to open")
	}

	// Inside the drum bounds, outside the popup: where the flicker would happen.
	popupRect := dv.overflowPopupRect()
	outsideX := popupRect.Max.X + 30
	outsideY := popupRect.Max.Y + 30
	if outsideX >= W {
		outsideX = popupRect.Min.X - 30
	}
	if outsideY >= H {
		outsideY = popupRect.Min.Y - 30
	}

	// Sanity: the point must be outside the popup.
	if image.Pt(outsideX, outsideY).In(popupRect) {
		t.Fatalf("test point (%d,%d) must be outside popup %v", outsideX, outsideY, popupRect)
	}

	dv.HandleInput(outsideX, outsideY, true)

	if !dv.IsOverflowMenuOpen() {
		t.Errorf(
			"overflow menu was closed by HandleInput(left=true) while suppressClicksUntilRelease — flicker bug\n"+
				"  tap pos:    (%d,%d)\n"+
				"  popup rect: %v\n"+
				"  Fix: guard 'click outside to close' with !suppressClicksUntilRelease",
			outsideX, outsideY, popupRect,
		)
	}
}

// Once the opening touch is released, a tap outside closes normally.
func TestOverflowMenuClosesOnOutsideTapAfterSuppressClears(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	// Close suppresses clicks until mouse-up; reset the global.
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	const W, H = 390, 600
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	dv.openOverflowMenuPortal()

	if !dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu must be open before testing close")
	}

	// Simulate touch release — clears suppressClicksUntilRelease.
	resetRelease := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false }, // left released
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetRelease()

	// Now tap outside the popup — should close.
	popupRect := dv.overflowPopupRect()
	outsideX := popupRect.Max.X + 30
	outsideY := popupRect.Max.Y + 30
	if outsideX >= W {
		outsideX = popupRect.Min.X - 30
	}
	if outsideY >= H {
		outsideY = popupRect.Min.Y - 30
	}

	// Click-outside closing is handled by the tree. Go through Update.
	resetOutside := SetInputForTest(
		func() (int, int) { return outsideX, outsideY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetOutside()

	if dv.IsOverflowMenuOpen() {
		t.Errorf("overflow menu should close when tapping outside after suppression cleared (tap pos: %d,%d, popup rect: %v)",
			outsideX, outsideY, popupRect)
	}
}

func TestBPMBoxFitsThreeDigits(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	bpmW := dv.bpmBox().Rect.Dx()
	needed := TextWidth("120")

	if bpmW < needed {
		t.Errorf("BPM box width %d is too narrow for 3-digit text (TextWidth(\"120\")=%d)", bpmW, needed)
	}
}

func TestMobileTransportColumnAtLeastAsWideAsTimeline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	bounds := image.Rect(0, 0, W, H)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	transportW := dv.widgets.ColWidth(0)
	timelineW := dv.widgets.ColWidth(1)

	if transportW < timelineW {
		t.Errorf("mobile transport column (%dpx) should be >= timeline column (%dpx)", transportW, timelineW)
	}
}
