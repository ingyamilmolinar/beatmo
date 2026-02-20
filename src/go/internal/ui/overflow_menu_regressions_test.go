//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newDrumViewForOverflowTest creates a DrumView inside the given bounds with a
// single row and runs one warm-up Update so buttons are laid out.
func newDrumViewForOverflowTest(t *testing.T, bounds image.Rectangle) *DrumView {
	t.Helper()
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8
	// Set the transport widget rect to span the full width so that button
	// layout has enough room. Without a widget board, the fallback uses
	// labelW+controlsW which is too narrow for mobile button sizing.
	dv.widgetRects[WidgetTransport] = image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Min.Y+dv.headerH)
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

// TestOverflowFilePickerRectsMatchUploadImportButtons verifies that
// registerFilePickerRects() registers the "upload" and "import" native
// file-picker rects at the actual Upload and Import button positions in the
// overflow popup. Previously the function used hardcoded indices 0 and 1,
// which were correct when the overflow menu had only 3 items
// (Upload/Import/Export). After adding Track/Len+/Len-/EQ before them,
// Upload moved to index 4 and Import to index 5 — but registerFilePickerRects
// still used index 0 and 1, pointing to the wrong buttons.
//
// Expected: upload rect Y == popupRect.Min.Y + 4*rowH (Upload is item 4)
// Actual before fix: upload rect Y == popupRect.Min.Y + 0*rowH (Track position)
func TestOverflowFilePickerRectsMatchUploadImportButtons(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	// Use a portrait screen with drum pane covering the bottom half.
	// The exact bounds don't matter — we only care about relative positions
	// within the popup.
	const W, H = 390, 300
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	// Open the overflow menu so overflowPopupRect() returns a valid rect.
	dv.SetOverflowMenuOpen(true)

	// Capture file picker rects registered by registerFilePickerRects().
	var captured []capturedFilePickerRect
	testCapturedFilePickerRects = &captured
	t.Cleanup(func() { testCapturedFilePickerRects = nil })

	dv.registerFilePickerRects()

	if len(captured) < 2 {
		t.Fatalf("expected ≥2 file picker rects (upload + import), got %d", len(captured))
	}

	// Find upload and import by ID.
	idxByID := map[string]int{}
	for i, c := range captured {
		idxByID[c.ID] = i
	}
	uploadIdx, hasUpload := idxByID["upload"]
	importIdx, hasImport := idxByID["import"]
	if !hasUpload {
		t.Fatal("no file picker rect registered with id='upload'")
	}
	if !hasImport {
		t.Fatal("no file picker rect registered with id='import'")
	}

	uploadRect := captured[uploadIdx]
	importRect := captured[importIdx]

	// Both rects must be non-empty.
	if uploadRect.W <= 0 || uploadRect.H <= 0 {
		t.Fatalf("upload rect has non-positive size: %+v", uploadRect)
	}
	if importRect.W <= 0 || importRect.H <= 0 {
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

	// The expected Y for each rect is popup.Min.Y + itemIndex * rowH
	// (with optional inset from buttonPad — we check the relative order and
	// that the Y is inside the correct item band, not the exact inset value).
	expectedUploadBandY := popupRect.Min.Y + uploadItemIdx*rowH
	expectedImportBandY := popupRect.Min.Y + importItemIdx*rowH

	// upload rect must be inside the Upload item band
	if uploadRect.Y < expectedUploadBandY || uploadRect.Y >= expectedUploadBandY+rowH {
		t.Errorf("upload file picker rect Y=%d is not in Upload item band [%d, %d) (item index %d)\n"+
			"  This means registerFilePickerRects uses wrong item index (likely hardcoded 0 instead of %d)",
			uploadRect.Y, expectedUploadBandY, expectedUploadBandY+rowH, uploadItemIdx, uploadItemIdx)
	}

	// import rect must be inside the Import item band
	if importRect.Y < expectedImportBandY || importRect.Y >= expectedImportBandY+rowH {
		t.Errorf("import file picker rect Y=%d is not in Import item band [%d, %d) (item index %d)\n"+
			"  This means registerFilePickerRects uses wrong item index (likely hardcoded 1 instead of %d)",
			importRect.Y, expectedImportBandY, expectedImportBandY+rowH, importItemIdx, importItemIdx)
	}
}

// TestOverflowPopupRectAlwaysWithinDrumBounds verifies that overflowPopupRect()
// returns a rectangle whose Min.Y is ≥ dv.Bounds.Min.Y even when the popup
// would not fit below the overflow button and flips above it.
//
// In the adaptive mobile portrait layout, the drum view can start near the
// bottom of the screen (e.g. Bounds.Min.Y ≈ 600 on an 800px screen). When the
// overflow button is near the top of the drum view (Y ≈ 610) and the popup
// height is ≈308px, the flip calculation sets y = anchor.Min.Y - h - 2 ≈ 300,
// which is well above dv.Bounds.Min.Y. The popup then renders in the grid pane
// and clicks on it are dispatched to the grid editor instead of the drum view,
// creating unwanted nodes.
//
// Expected: popupRect.Min.Y >= dv.Bounds.Min.Y
// Actual before fix: popupRect.Min.Y ≈ 300 < dv.Bounds.Min.Y ≈ 600
func TestOverflowPopupRectAlwaysWithinDrumBounds(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	// Simulate the drum view occupying only the bottom portion of a portrait
	// screen — the adaptive split formula puts it near the bottom when there
	// are few rows. Use a small height so the popup cannot fit below the
	// overflow button, forcing the flip.
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

// TestOverflowPopupClickDoesNotCreateGridNode verifies that tapping a menu item
// in the overflow popup does NOT create a node in the grid, even when the drum
// view occupies only the bottom portion of the screen and the popup would
// otherwise appear in the grid area.
//
// This is a consequence of Bug 2: if the popup rect escapes dv.Bounds, the
// InputDispatcher skips the drum view handler (because InputBounds() returns
// dv.Bounds) and handleEditor() processes the click as a grid tap.
func TestOverflowPopupClickDoesNotCreateGridNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait layout where drum pane is small — adaptive split gives drum
	// view a small height slice near the bottom.
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

// TestOverflowMenuStaysOpenAfterButtonTap reproduces the overflow-menu flicker
// bug on mobile.
//
// Root cause: the overflow button's OnClick (in drumview_ctor.go) opens the
// menu during drum.Update(), which runs AFTER the InputDispatcher each frame.
// On the next frame, handleOverflowMenuInput() runs with left=true at the
// button position. If the touch position falls outside the popup rect AND the
// raw button rect, the "click outside to close" path fires. Without the
// suppressClicksUntilRelease guard, the menu closes immediately — producing a
// 1-frame flicker.
//
// The fix: (1) call SuppressClicksUntilMouseUp() when opening the overflow
// menu, and (2) honor suppressClicksUntilRelease in handleOverflowMenuInput's
// "click outside to close" path.
func TestOverflowMenuStaysOpenAfterButtonTap(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 600
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	// Locate the overflow button.
	btnRect := dv.overflowBtn.Rect()
	if btnRect.Empty() {
		t.Skip("overflow button rect empty — layout may differ")
	}
	bx := btnRect.Min.X + btnRect.Dx()/2
	by := btnRect.Min.Y + btnRect.Dy()/2

	// ── Frame 0: tap the overflow button (opens menu via OnClick) ──
	resetFrame0 := SetInputForTest(
		func() (int, int) { return bx, by },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	// Save suppression state before reset clears it — in the real app the
	// touch hasn't been released so the flag stays true between frames.
	savedSuppress := suppressClicksUntilRelease
	resetFrame0()
	suppressClicksUntilRelease = savedSuppress
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	if !dv.overflowMenuOpen {
		t.Fatal("expected overflow menu to open after tapping button")
	}

	// ── Frame 1: HandleInput with left=true at a position outside the popup ──
	// Use a point that's inside the drum bounds but outside both the popup
	// rect and the button rect — this is where flicker would happen.
	popupRect := dv.overflowPopupRect()
	outsideX := popupRect.Max.X + 30
	outsideY := popupRect.Max.Y + 30
	if outsideX >= W {
		outsideX = popupRect.Min.X - 30
	}
	if outsideY >= H {
		outsideY = popupRect.Min.Y - 30
	}

	// Sanity: the point must be outside both popup and button rect.
	pt := image.Pt(outsideX, outsideY)
	if pt.In(popupRect) || pt.In(btnRect) {
		t.Fatalf("test point (%d,%d) must be outside popup %v and button %v",
			outsideX, outsideY, popupRect, btnRect)
	}

	dv.HandleInput(outsideX, outsideY, true)

	if !dv.overflowMenuOpen {
		t.Errorf(
			"overflow menu was closed by HandleInput(left=true) while suppressClicksUntilRelease — flicker bug\n"+
				"  tap pos:    (%d,%d)\n"+
				"  popup rect: %v\n"+
				"  button rect: %v\n"+
				"  Fix: guard 'click outside to close' with !suppressClicksUntilRelease",
			outsideX, outsideY, popupRect, btnRect,
		)
	}
}

// TestOverflowMenuClosesOnOutsideTapAfterSuppressClears verifies that the
// overflow menu DOES close when the user taps outside AFTER the suppression
// window ends (i.e., once the touch that opened the menu has been released).
// This ensures the fix doesn't break the normal "tap outside to close" flow.
func TestOverflowMenuClosesOnOutsideTapAfterSuppressClears(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	// OverflowMenuOverlay.Close() calls SuppressClicksUntilMouseUp() to
	// prevent click-through after closing; clean up the global.
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	const W, H = 390, 600
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	// Locate the overflow button.
	btnRect := dv.overflowBtn.Rect()
	if btnRect.Empty() {
		t.Skip("overflow button rect empty — layout may differ")
	}
	bx := btnRect.Min.X + btnRect.Dx()/2
	by := btnRect.Min.Y + btnRect.Dy()/2

	// Open the menu via Update().
	resetOpen := SetInputForTest(
		func() (int, int) { return bx, by },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetOpen()

	if !dv.overflowMenuOpen {
		t.Fatal("overflow menu must be open before testing close")
	}

	// Simulate touch release — clears suppressClicksUntilRelease.
	resetRelease := SetInputForTest(
		func() (int, int) { return bx, by },
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

	dv.HandleInput(outsideX, outsideY, true)

	if dv.overflowMenuOpen {
		t.Errorf("overflow menu should close when tapping outside after suppression cleared (tap pos: %d,%d, popup rect: %v)",
			outsideX, outsideY, popupRect)
	}
}

// TestOverflowBtnTapDoesNotTriggerAdjacentButton verifies that tapping the
// overflow button on mobile opens the overflow menu WITHOUT also activating
// the adjacent viewSwitchBtn. The root cause was that Button.Handle() expands
// hit areas to touchMinTargetPx (44px) for touch targets. With 8 buttons in a
// narrow transport column (~18px each), expanded areas overlap. The button loop
// checks left-to-right: viewSwitchBtn is checked before overflowBtn, so its
// expanded area steals the tap.
func TestOverflowBtnTapDoesNotTriggerAdjacentButton(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	bounds := image.Rect(0, 0, W, H)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8
	// Use the actual widget board transport column width (not full screen width)
	// so buttons are realistically narrow, reproducing the overlap.
	transportRect := dv.widgetRects[WidgetTransport]
	if transportRect.Empty() {
		transportRect = image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+dv.widgets.ColWidth(0), bounds.Min.Y+dv.headerH)
		dv.widgetRects[WidgetTransport] = transportRect
	}
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

	btnRect := dv.overflowBtn.Rect()
	if btnRect.Empty() {
		t.Skip("overflow button rect empty — layout may differ")
	}

	// Remember viewSwitchBtn state before tap.
	viewModeBefore := dv.currentViewMode

	// Tap at the center of the overflow button.
	bx := btnRect.Min.X + btnRect.Dx()/2
	by := btnRect.Min.Y + btnRect.Dy()/2

	// Frame 0: press
	reset = SetInputForTest(
		func() (int, int) { return bx, by },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	savedSuppress := suppressClicksUntilRelease
	reset()
	suppressClicksUntilRelease = savedSuppress
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	// Frame 1: release
	reset2 := SetInputForTest(
		func() (int, int) { return bx, by },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	reset2()

	if !dv.overflowMenuOpen {
		t.Error("expected overflow menu to open after tapping overflow button")
	}
	if dv.currentViewMode != viewModeBefore {
		t.Errorf("viewSwitchBtn was triggered (viewMode changed %d → %d) when tapping overflow button — hit area overlap bug",
			viewModeBefore, dv.currentViewMode)
	}
}

// TestBPMBoxFitsThreeDigits verifies that the BPM text box on mobile is wide
// enough to display a 3-digit BPM value like "120".
func TestBPMBoxFitsThreeDigits(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := newDrumViewForOverflowTest(t, image.Rect(0, 0, W, H))

	bpmW := dv.bpmBox.Rect.Dx()
	needed := TextWidth("120")

	if bpmW < needed {
		t.Errorf("BPM box width %d is too narrow for 3-digit text (TextWidth(\"120\")=%d)", bpmW, needed)
	}
}

// TestMobileTransportColumnAtLeastAsWideAsTimeline verifies that on mobile,
// the transport column gets at least as much width as the timeline column.
func TestMobileTransportColumnAtLeastAsWideAsTimeline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	bounds := image.Rect(0, 0, W, H)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
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
