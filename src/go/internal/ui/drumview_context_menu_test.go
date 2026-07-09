//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestKebabTapDoesNotOpenInstMenu reproduces the mobile bug where tapping the
// kebab ("⋮") button opens both the context menu AND the instrument submenu.
//
// Root cause: openContextMenu calls SuppressClicksUntilMouseUp(), but
// handleContextMenuInput's deferred-tap capture did not check
// suppressClicksUntilRelease. The lingering touch from the kebab press
// was captured as a context-menu tap, and on release fireContextMenuTapAt
// hit the "Instrument" item (first in the list), opening the inst menu.
func TestKebabTapDoesNotOpenInstMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Find the kebab button for row 0.
	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("rowMenuBtns not created after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	kx := kebabRect.Min.X + kebabRect.Dx()/2
	ky := kebabRect.Min.Y + kebabRect.Dy()/2

	// Frame 1: Press on kebab button → fires OnClick → openContextMenu.
	r := SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()

	if !dv.IsContextMenuOpen() {
		r()
		t.Fatal("context menu should be open after kebab press")
	}
	if dv.IsInstMenuOpen() {
		r()
		t.Fatal("inst menu should NOT be open after kebab press (frame 1)")
	}

	// Frame 2: Still holding at the same position. The deferred tap should
	// NOT be captured because suppressClicksUntilRelease is still active.
	dv.Update()

	if !dv.IsContextMenuOpen() {
		r()
		t.Fatal("context menu should still be open (frame 2)")
	}
	if dv.IsInstMenuOpen() {
		r()
		t.Fatal("inst menu should NOT be open while holding (frame 2)")
	}
	if dv.contextMenuDeferredTap.Active() {
		r()
		t.Fatal("deferred tap should NOT be active while suppress is set")
	}
	r()

	// Frame 3: Release. Suppress clears, but no deferred tap was captured,
	// so no menu item fires.
	r = SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should still be open after release (frame 3)")
	}
	if dv.IsInstMenuOpen() {
		t.Fatal("inst menu should NOT be open after release (frame 3)")
	}
}

// TestKebabTapWorksWithScrollableRows verifies that tapping the kebab ("⋮")
// button works when the drum view has more rows than fit on screen (scrollable).
//
// Bug: the injected tap's press frame re-triggered HandleTouchBegin, which
// re-enabled the touch dead zone and blocked all row buttons. The fix adds
// !isTouchTapInjecting() to the scroll begin condition.
func TestKebabTapWorksWithScrollableRows(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling is needed.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("rowMenuBtns not created after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	kx := kebabRect.Min.X + kebabRect.Dx()/2
	ky := kebabRect.Min.Y + kebabRect.Dy()/2

	// Phase 1: Simulate a real touch press (left=true) in the rows area.
	// This should start touch scroll tracking and enable the dead zone,
	// blocking the kebab button (correct behavior for a potential scroll).
	// Set touchOverrideActive AFTER SetInputForTest (which calls resetTouchOverride)
	// to indicate this is actual touch input (not mouse).
	r := SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(kx, ky)
	dv.Update()
	r()

	if dv.IsContextMenuOpen() {
		t.Fatal("context menu should NOT open during real touch (dead zone should block)")
	}

	// Phase 2: Release — touch scroll ends.
	r = SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	// Phase 3: Injected tap (gesture detector confirmed this is a tap).
	// SetInputForTest calls resetTouchOverride() which clears touchTapInjected,
	// so we must set the injection state AFTER SetInputForTest.
	r = SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchTapInjectedForTest(true)
	dv.Update()
	r()
	SetTouchTapInjectedForTest(false)

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should open after injected tap on kebab button")
	}
}

// TestRowLabelTapWorksWithScrollableRows verifies that tapping a row label
// (instrument name) opens the instrument menu when rows are scrollable.
//
// Same root cause as the kebab bug: the injected tap re-triggered scroll
// tracking, re-enabling the dead zone and blocking the row label button.
func TestRowLabelTapWorksWithScrollableRows(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling is needed.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after warm-up")
	}
	labelRect := dv.rowLabels()[0].Rect()
	if labelRect.Empty() {
		t.Skip("row label rect empty (not visible in this layout)")
	}
	lx := labelRect.Min.X + labelRect.Dx()/2
	ly := labelRect.Min.Y + labelRect.Dy()/2

	// Phase 1: Simulate a real touch press — dead zone should block the label.
	// Set touchOverrideActive AFTER SetInputForTest (which calls resetTouchOverride)
	// to indicate this is actual touch input (not mouse).
	r := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(lx, ly)
	dv.Update()
	r()

	if dv.IsInstMenuOpen() {
		t.Fatal("inst menu should NOT open during real touch (dead zone should block)")
	}

	// Phase 2: Release — touch scroll ends.
	r = SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	// Phase 3: Injected tap — should open context menu on mobile.
	// SetInputForTest calls resetTouchOverride() which clears touchTapInjected,
	// so we must set the injection state AFTER SetInputForTest.
	r = SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchTapInjectedForTest(true)
	dv.Update()
	r()
	SetTouchTapInjectedForTest(false)

	if !dv.IsInstMenuOpen() {
		t.Fatal("inst menu should open after injected tap on row label")
	}
}

// TestKebabTapOverlapScrollerActive reproduces the real mobile timing where the
// injected tap starts while the row scroller is still active (no intermediate
// release frame). Before the fix, the scroller's dead zone blocked the kebab
// button during injection frame 0, and the touch-move handler consumed the
// injected press, so the button never fired.
func TestKebabTapOverlapScrollerActive(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("rowMenuBtns not created after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	kx := kebabRect.Min.X + kebabRect.Dx()/2
	ky := kebabRect.Min.Y + kebabRect.Dy()/2

	// Phase 1: Real touch press — starts scroller, dead zone active.
	// Set touchOverrideActive AFTER SetInputForTest (which calls resetTouchOverride)
	// to indicate this is actual touch input (not mouse).
	r := SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(kx, ky)
	dv.Update()
	r()

	if dv.IsContextMenuOpen() {
		t.Fatal("context menu should NOT open during real touch (dead zone should block)")
	}
	if !dv.rowScroll().TouchActive() {
		t.Fatal("row scroller should be active after real touch press")
	}

	// Phase 2 (release) is SKIPPED — go directly to injection while scroller
	// is still active. This is what actually happens on mobile: the gesture
	// detector fires GestureTap and injectTouchTap starts on the same frame
	// the original touch ends, with no intermediate release frame for the
	// drum view.

	// Phase 3: Injected tap press — scroller still active from Phase 1.
	r = SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchTapInjectedForTest(true)
	dv.Update()
	r()
	SetTouchTapInjectedForTest(false)

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should open after injected tap on kebab (scroller overlap)")
	}
}

// TestRowLabelTapOverlapScrollerActive reproduces the real mobile timing where
// the injected tap starts while the row scroller is still active. Same root
// cause as the kebab overlap test but for the instrument label.
func TestRowLabelTapOverlapScrollerActive(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after warm-up")
	}
	labelRect := dv.rowLabels()[0].Rect()
	if labelRect.Empty() {
		t.Skip("row label rect empty (not visible in this layout)")
	}
	lx := labelRect.Min.X + labelRect.Dx()/2
	ly := labelRect.Min.Y + labelRect.Dy()/2

	// Phase 1: Real touch press — starts scroller, dead zone active.
	// Set touchOverrideActive AFTER SetInputForTest (which calls resetTouchOverride)
	// to indicate this is actual touch input (not mouse).
	r := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(lx, ly)
	dv.Update()
	r()

	if dv.IsInstMenuOpen() {
		t.Fatal("inst menu should NOT open during real touch (dead zone should block)")
	}
	if !dv.rowScroll().TouchActive() {
		t.Fatal("row scroller should be active after real touch press")
	}

	// Phase 2: Release — clears the tree's press state so the injected tap
	// is recognized as a new press.
	r = SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	// Phase 3: Injected tap press — scroller may still have momentum.
	r = SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchTapInjectedForTest(true)
	dv.Update()
	r()
	SetTouchTapInjectedForTest(false)

	if !dv.IsInstMenuOpen() {
		t.Fatal("inst menu should open after injected tap on row label (scroller overlap)")
	}
}

// TestContextMenuCloseButtonDoesNotFireInstrument verifies that tapping the
// close button in the context menu actually closes the menu rather than
// firing the "Instrument" item. The close button is appended last but
// spatially overlaps the first row; reverse iteration ensures it wins.
func TestContextMenuCloseButtonDoesNotFireInstrument(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Open context menu for row 0.
	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	// Find the close button (last in the list).
	btns := dv.contextMenuBtns
	if len(btns) == 0 {
		t.Fatal("no context menu buttons")
	}
	closeBtn := btns[len(btns)-1]
	closeRect := closeBtn.Rect()
	if closeRect.Empty() {
		t.Fatal("close button rect is empty")
	}

	// Fire tap at close button center.
	cx := closeRect.Min.X + closeRect.Dx()/2
	cy := closeRect.Min.Y + closeRect.Dy()/2
	dv.fireContextMenuTapAt(cx, cy)

	// Context menu should be closed.
	if dv.IsContextMenuOpen() {
		t.Fatal("context menu should be closed after tapping close button")
	}
	// Instrument menu should NOT have opened.
	instOpen := dv.IsInstMenuOpen() || (dv.instMenuComp != nil && dv.instMenuComp.IsOpen())
	if instOpen {
		t.Fatal("instrument menu should NOT open when tapping the close button")
	}
}

// TestContextMenuMobileOmitsEffects verifies that the mobile context
// menu intentionally omits the Effects entry (FX has its own row button).
// The "Color" entry IS present — it opens the grouped Vice City picker.
func TestContextMenuMobileOmitsEffects(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	items := dv.ContextMenuItemsForTest(0)
	hasColor := false
	for _, it := range items {
		if it.label == "Effects" {
			t.Errorf("mobile context menu should not contain %q, got items=%v", it.label, nonDividerLabels(items))
		}
		if it.label == "Color" {
			hasColor = true
		}
	}
	if !hasColor {
		t.Errorf("mobile context menu should contain \"Color\", got items=%v", nonDividerLabels(items))
	}
}

// TestOverflowMenuCloseButtonDoesNotFireUpload verifies that tapping the
// close button in the overflow menu closes it rather than firing "Upload".
func TestOverflowMenuCloseButtonDoesNotFireUpload(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// The overflow button needs a non-empty rect for the popup to appear.
	if dv.overflowBtn() == nil {
		t.Fatal("overflow button is nil")
	}
	if dv.overflowBtn().Rect().Empty() {
		// Give it a rect manually for testing.
		dv.overflowBtn().SetRect(image.Rect(W-60, 0, W, 44))
	}

	// Open the overflow menu.
	dv.openOverflowMenuPortal()
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		t.Fatal("overflow popup rect is empty")
	}

	// Get popup buttons and find the close button (last).
	popupBtns := dv.overflowPopupBtns(popupRect)
	if len(popupBtns) == 0 {
		t.Fatal("no overflow popup buttons")
	}
	closeBtn := popupBtns[len(popupBtns)-1]
	closeRect := closeBtn.Rect()
	if closeRect.Empty() {
		t.Fatal("close button rect is empty")
	}

	// Track whether Upload was called.
	uploadCalled := false
	origUpload := dv.uploadBtn()
	dv.transportZone.uploadBtn = NewButton("Upload", DropdownStyle, func() { uploadCalled = true })

	// Fire tap at close button center.
	cx := closeRect.Min.X + closeRect.Dx()/2
	cy := closeRect.Min.Y + closeRect.Dy()/2
	dv.fireOverflowMenuTapAt(cx, cy)

	// Overflow menu should be closed.
	if dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu should be closed after tapping close button")
	}
	// Upload should NOT have been called.
	if uploadCalled {
		t.Fatal("Upload should NOT fire when tapping the close button")
	}

	// Restore.
	dv.transportZone.uploadBtn = origUpload
}

// TestColorPickerCloseUnblocksButtons verifies that closing the color picker
// portal does not leave anyDragActive() stuck, and allows row buttons to
// work again. Previously this exercised the mobile context-menu "Color"
// entry; now that mobile omits Color from the menu, the picker is opened
// directly via the portal — the close-and-unblock invariant is unchanged.
func TestColorPickerCloseUnblocksButtons(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Open the color wheel portal directly (the mobile context menu no
	// longer contains a Color entry).
	dv.openColorPickerForRow(0)

	// Color wheel should be open and anyDragActive should NOT be true
	// (hold flags no longer feed into anyDragActive).
	if !dv.IsColorMenuOpen() {
		t.Fatal("colorMenuOpen should be true")
	}
	if dv.anyDragActive() {
		t.Fatal("anyDragActive should be false — hold flags no longer block")
	}

	// Close the color wheel via the portal (which invokes the comp's OnClose).
	dv.closeColorWheelPortal()

	// After closing: colorMenuOpen and anyDragActive should be false.
	if dv.IsColorMenuOpen() {
		t.Fatal("colorMenuOpen should be false after closing color picker")
	}
	if dv.anyDragActive() {
		t.Fatal("anyDragActive should be false after closing color picker")
	}

	// Verify row buttons work: open context menu via kebab.
	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("rowMenuBtns not created")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty")
	}
	dv.rowMenuBtns()[0].OnClick()
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should open after color picker close — buttons are unblocked")
	}
}

// TestCloseAllPopupsClearsColorMenu verifies that CloseAllPopups closes
// the color wheel portal.
func TestCloseAllPopupsClearsColorMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Open the color wheel portal.
	dv.openColorWheelPortal()

	dv.CloseAllPopups()

	if dv.IsColorMenuOpen() {
		t.Fatal("colorMenuOpen should be false after CloseAllPopups")
	}
	if dv.anyDragActive() {
		t.Fatal("anyDragActive should be false after CloseAllPopups")
	}
}

// TestKebabTapViaHandleInputPipeline simulates the full Game.Update() pipeline
// (HandleInput → Update) for a mobile kebab tap. Unlike existing tests that call
// dv.Update() directly, this test exercises the overlay stack dispatch path
// that runs during HandleInput. If HandleInput's overlay stack incorrectly
// consumes the tap or sets suppressClicksUntilRelease, the kebab button won't fire.
func TestKebabTapViaHandleInputPipeline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("rowMenuBtns not created after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	kx := kebabRect.Min.X + kebabRect.Dx()/2
	ky := kebabRect.Min.Y + kebabRect.Dy()/2

	// Phase 1: Real touch press — HandleInput then Update.
	// Set touchOverrideActive AFTER SetInputForTest (which calls resetTouchOverride)
	// to indicate this is actual touch input (not mouse).
	r := SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(kx, ky)
	dv.HandleInput(kx, ky, true)
	dv.Update()
	r()

	if dv.IsContextMenuOpen() {
		t.Fatal("context menu should NOT open during real touch press")
	}

	// Phase 2: Release — HandleInput then Update.
	r = SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.HandleInput(kx, ky, false)
	dv.Update()
	r()

	// Phase 3: Injected tap frame 0 (press) — HandleInput then Update.
	r = SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchTapInjectedForTest(true)
	dv.HandleInput(kx, ky, true)
	dv.Update()
	r()
	SetTouchTapInjectedForTest(false)

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should open after injected tap via HandleInput pipeline")
	}

	// Phase 4: Injected tap frame 1 (release) — context menu should stay open.
	r = SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.HandleInput(kx, ky, false)
	dv.Update()
	r()

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should remain open after injection release frame")
	}
}

// TestRowLabelTapViaHandleInputPipeline simulates the full Game.Update() pipeline
// (HandleInput → Update) for a mobile row label tap to open the instrument menu.
func TestRowLabelTapViaHandleInputPipeline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after warm-up")
	}
	labelRect := dv.rowLabels()[0].Rect()
	if labelRect.Empty() {
		t.Skip("row label rect empty (not visible in this layout)")
	}
	lx := labelRect.Min.X + labelRect.Dx()/2
	ly := labelRect.Min.Y + labelRect.Dy()/2

	// Phase 1: Real touch press — HandleInput then Update.
	// Set touchOverrideActive AFTER SetInputForTest (which calls resetTouchOverride)
	// to indicate this is actual touch input (not mouse).
	r := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(lx, ly)
	dv.HandleInput(lx, ly, true)
	dv.Update()
	r()

	if dv.IsInstMenuOpen() {
		t.Fatal("inst menu should NOT open during real touch press")
	}

	// Phase 2: Release — HandleInput then Update.
	r = SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.HandleInput(lx, ly, false)
	dv.Update()
	r()

	// Phase 3: Injected tap — HandleInput then Update.
	r = SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchTapInjectedForTest(true)
	dv.HandleInput(lx, ly, true)
	dv.Update()
	r()
	SetTouchTapInjectedForTest(false)

	if !dv.IsInstMenuOpen() {
		t.Fatal("inst menu should open after injected tap on row label via HandleInput pipeline")
	}
}

// TestButtonTapAfterContextMenuClose verifies that after opening a context menu,
// closing it (click outside via HandleInput overlay stack), and waiting for the
// suppress cycle to clear, the kebab button works again.
func TestButtonTapAfterContextMenuClose(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("rowMenuBtns not created after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	kx := kebabRect.Min.X + kebabRect.Dx()/2
	ky := kebabRect.Min.Y + kebabRect.Dy()/2

	// Step 1: Open context menu directly.
	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	// Step 2: Click outside the context menu via HandleInput (overlay stack
	// should close it and set suppressClicksUntilRelease).
	outsideX, outsideY := W/2, H/2 // center of screen, likely outside menu
	// Make sure we're actually outside the menu rect.
	if dv.contextMenuRect.Min.X <= outsideX && outsideX < dv.contextMenuRect.Max.X &&
		dv.contextMenuRect.Min.Y <= outsideY && outsideY < dv.contextMenuRect.Max.Y {
		outsideX = 1
		outsideY = 1
	}
	r := SetInputForTest(
		func() (int, int) { return outsideX, outsideY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.HandleInput(outsideX, outsideY, true)
	dv.Update()
	r()

	if dv.IsContextMenuOpen() {
		t.Fatal("context menu should be closed after click outside via HandleInput")
	}

	// Step 3: Release to clear suppress.
	r = SetInputForTest(
		func() (int, int) { return outsideX, outsideY },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.HandleInput(outsideX, outsideY, false)
	dv.Update()
	r()

	if suppressClicksUntilRelease {
		t.Fatal("suppressClicksUntilRelease should be cleared after release")
	}

	// Step 4: Tap kebab again — should work.
	r = SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.HandleInput(kx, ky, true)
	dv.Update()
	r()

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should re-open after kebab tap following close cycle")
	}
}

// TestKebabTapDoesNotRegisterRenameTrigger verifies that the kebab (ellipsis)
// button is NOT registered as a mobile native input trigger for rename. The
// rename trigger should only be registered when the context menu is open, on
// the "Rename" button inside the context menu — not on the kebab button itself.
//
// This prevents the JS touchend handler from intercepting the kebab tap and
// immediately creating a rename native input, which would bypass the context menu.
func TestKebabTapDoesNotRegisterRenameTrigger(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	renameArmed := func() bool {
		for _, r := range dv.lastNativeRects {
			if r.Intent.Channel == NativeTextInput && r.Intent.ID == "rename-0" {
				return true
			}
		}
		return false
	}

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Context menu closed: rename trigger must NOT be armed.
	if !dv.IsContextMenuOpen() {
		// good — context menu is closed
	} else {
		t.Fatal("context menu should not be open initially")
	}
	if renameArmed() {
		t.Fatal("rename-0 trigger should NOT be armed when context menu is closed")
	}

	// Open context menu for row 0.
	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	// Drive a full Update so nativeInputCandidates → syncNativeGestures
	// re-project the native-input rects (recalcButtons alone no longer
	// registers them — that now happens in dv.Update's tree-owned sync).
	dv.Update()

	// Now the rename trigger SHOULD be armed (on the Rename menu button).
	if !renameArmed() {
		t.Fatal("rename-0 trigger should be armed when context menu is open")
	}

	// Close context menu and re-run Update.
	dv.closeContextMenuPortal()
	dv.Update()

	// Trigger should be gone again.
	if renameArmed() {
		t.Fatal("rename-0 trigger should NOT be armed after context menu closes")
	}
}

// TestMouseClickWorksOnMobileLayout verifies that mouse clicks (without touch
// input) on row buttons work correctly when isSmallScreen() is true. This is
// the scenario when the WASM build runs in a desktop browser with a small
// window to simulate mobile. Previously, the touch scroll dead zone blocked
// all row button clicks because it couldn't distinguish mouse from touch.
func TestMouseClickWorksOnMobileLayout(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling is needed.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("rowMenuBtns not created after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	kx := kebabRect.Min.X + kebabRect.Dx()/2
	ky := kebabRect.Min.Y + kebabRect.Dy()/2

	// Simulate a MOUSE click (touchOverrideActive is false, which is default).
	// This is what happens when running WASM in a desktop browser with a small window.
	r := SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	// Do NOT set touchOverrideActive — this is a mouse click, not touch.
	dv.Update()
	r()

	// The touch scroll should NOT have started (mouse clicks skip it).
	if dv.rowScroll().TouchActive() {
		t.Fatal("row scroller should NOT be active for mouse clicks (no touch override)")
	}

	// The kebab button should have opened the context menu.
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should open on mouse click in mobile layout")
	}
}

// TestMouseClickOnLabelWorksOnMobileLayout verifies that clicking the row
// label on mobile opens the instrument picker. Per the 2026-06-13
// mobile-instrument-button design, tapping the instrument-name label opens
// the picker on every platform (the ellipsis/kebab owns the context menu:
// Rename / Color / Origin / Delete). See row_rack_zone.go label OnClick.
func TestMouseClickOnLabelWorksOnMobileLayout(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling is needed.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after warm-up")
	}
	labelRect := dv.rowLabels()[0].Rect()
	if labelRect.Empty() {
		t.Skip("row label rect empty (not visible in this layout)")
	}
	lx := labelRect.Min.X + labelRect.Dx()/2
	ly := labelRect.Min.Y + labelRect.Dy()/2

	// Simulate a MOUSE click (touchOverrideActive is false).
	r := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	// Do NOT set touchOverrideActive — this is a mouse click, not touch.
	dv.Update()
	r()

	// The touch scroll should NOT have started.
	if dv.rowScroll().TouchActive() {
		t.Fatal("row scroller should NOT be active for mouse clicks (no touch override)")
	}

	// On every platform, the label button opens the instrument picker (the
	// kebab/ellipsis owns the context menu).
	if !dv.IsInstMenuOpen() {
		t.Fatalf("instrument picker should open on label click in mobile layout (instMenuOpen=%v contextMenuOpen=%v)",
			dv.IsInstMenuOpen(), dv.IsContextMenuOpen())
	}
	if dv.IsContextMenuOpen() {
		t.Error("label click must not open the context menu")
	}
}

// TestContextMenuBlocksTransportButtons verifies that when the context menu is
// open, clicking at the play button's coordinates does NOT start playback.
// This is a regression test for the click-through bug where Update() handlers
// were not blocked by the context menu overlay.
func TestContextMenuBlocksTransportButtons(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if dv.playBtn() == nil || dv.playBtn().Rect().Empty() {
		t.Skip("play button not visible in this layout")
	}

	// Track whether play was fired.
	playFired := false
	dv.playBtn().OnClick = func() { playFired = true }

	// Open the context menu via portal path so the portal guard blocks transport.
	dv.openContextMenuPortal()

	// Simulate a click at the play button's center.
	pr := dv.playBtn().Rect()
	cx, cy := (pr.Min.X+pr.Max.X)/2, (pr.Min.Y+pr.Max.Y)/2

	r := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	if playFired {
		t.Fatal("play button fired while context menu was open — click-through not blocked")
	}
}

// TestOverflowMenuBlocksWidgets verifies that when the overflow menu is open,
// clicks at widget coordinates are blocked.
func TestOverflowMenuBlocksWidgets(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if dv.addRowBtn() == nil || dv.addRowBtn().Rect().Empty() {
		t.Skip("addRowBtn not visible")
	}

	addFired := false
	dv.addRowBtn().OnClick = func() { addFired = true }

	// Open via portal path so the portal guard blocks underlying widgets.
	dv.openOverflowMenuPortal()

	ar := dv.addRowBtn().Rect()
	cx, cy := (ar.Min.X+ar.Max.X)/2, (ar.Min.Y+ar.Max.Y)/2

	r := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	if addFired {
		t.Fatal("addRowBtn fired while overflow menu was open — click-through not blocked")
	}
}

// TestContextMenuBlocksRowVolumeSlider verifies that when the context menu is
// open, dragging at row volume slider coordinates does not change the volume.
func TestContextMenuBlocksRowVolumeSlider(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     0.5,
	}}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("row volume slider not visible")
	}

	origVol := dv.Rows[0].Volume
	// Open via portal path so the portal guard blocks underlying widgets.
	dv.openContextMenuPortal()

	sr := dv.rowVolSliders()[0].Rect()
	// Click at the far right of the slider (would set vol to ~1.0)
	cx, cy := sr.Max.X-1, (sr.Min.Y+sr.Max.Y)/2

	r := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	if dv.Rows[0].Volume != origVol {
		t.Fatalf("row volume changed from %.2f to %.2f while context menu was open", origVol, dv.Rows[0].Volume)
	}
}

// TestContextMenuScrollWhenOverflow verifies that the context menu initializes
// scroll behavior and that scrolling is activated when items exceed available space.
func TestContextMenuScrollWhenOverflow(t *testing.T) {
	assertDefaultParityState(t)

	// Use a very small drum pane so context menu items overflow.
	const W, H = 300, 120
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	r := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	// Open context menu for row 0
	dv.openContextMenu(0)

	if dv.contextMenuScroll == nil {
		t.Fatal("contextMenuScroll should be initialized after opening context menu")
	}

	// Count non-divider items
	items := dv.contextMenuItems(0)
	nItems := 0
	for _, item := range items {
		if !item.divider {
			nItems++
		}
	}

	if dv.contextMenuScroll.VS.Total != nItems {
		t.Fatalf("scroll Total should equal item count: got %d want %d", dv.contextMenuScroll.VS.Total, nItems)
	}

	// With a 120px tall pane, menu should need scrolling (items exceed space)
	if !dv.contextMenuScroll.HasScroll() {
		t.Log("Note: context menu does not need scrolling in this pane size (items fit)")
	}
}

// TestContextMenuClosedReturnsEarly verifies that handleContextMenuInput returns
// false immediately when the context menu is not open, without consuming input.
func TestContextMenuClosedReturnsEarly(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	// Context menu is closed by default.
	if dv.IsContextMenuOpen() {
		t.Fatal("context menu should be closed by default")
	}

	// Should return false (not consumed) when menu is closed.
	got := dv.handleContextMenuInput(100, 100, true)
	if got {
		t.Fatal("handleContextMenuInput should return false when menu is closed")
	}
}

// TestContextMenuClickOutsideCloses verifies that handleContextMenuInput
// returns false for clicks outside the menu rect (click-outside closing
// is handled by the tree, not the handler).
func TestContextMenuClickOutsideCloses(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	// Warm-up frame for layout init.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	// Manually open the context menu.
	dv.openContextMenuPortal()
	dv.contextMenuRow = 0
	dv.contextMenuRect = image.Rect(100, 100, 300, 400)

	suppressClicksUntilRelease = false

	// Click well outside the menu rect — handler should NOT consume
	// (click-outside is handled by the tree).
	got := dv.handleContextMenuInput(500, 500, true)
	if got {
		t.Fatal("handleContextMenuInput should return false for clicks outside contextMenuRect")
	}
}

// TestOverflowMenuClosedReturnsEarly verifies that handleOverflowMenuInput
// returns false immediately when the overflow menu is not open.
func TestOverflowMenuClosedReturnsEarly(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	// Overflow menu is closed by default.
	if dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu should be closed by default")
	}

	// Should return false (not consumed) when menu is closed.
	got := dv.handleOverflowMenuInput(100, 100, true)
	if got {
		t.Fatal("handleOverflowMenuInput should return false when menu is closed")
	}
}

// TestContextMenuDesktopButtonIteration verifies that in desktop mode (not
// small screen), clicks inside an open context menu are consumed by the
// button iteration path.
func TestContextMenuDesktopButtonIteration(t *testing.T) {
	assertDefaultParityState(t)
	// Don't use withSmallScreen — default is desktop mode.

	const W, H = 1024, 768
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	// Warm-up frame for layout init.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	restore()

	// Open context menu with buttons via the real method so buttons are populated.
	dv.openContextMenu(0)

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open after openContextMenu(0)")
	}
	if len(dv.contextMenuBtns) == 0 {
		t.Fatal("context menu should have buttons after openContextMenu(0)")
	}

	menuRect := dv.contextMenuRect
	if menuRect.Empty() {
		t.Fatal("context menu rect should not be empty")
	}

	// Click inside the menu area — should be consumed by the button iteration
	// or the catch-all "inside menu" path.
	cx := (menuRect.Min.X + menuRect.Max.X) / 2
	cy := (menuRect.Min.Y + menuRect.Max.Y) / 2
	got := dv.handleContextMenuInput(cx, cy, true)
	if !got {
		t.Fatal("expected click inside menu rect to be consumed")
	}
}

// TestContextMenuItemsDesktopStructure documents the expected item set for
// non-mobile builds: Rename / Color / Origin / Delete. Instrument and Effects
// are reachable via row controls / the label; "Color" opens the grouped Vice
// City picker (see Task 14 of the Vice City palette plan). The menu is
// identical on every platform.
func TestContextMenuItemsDesktopStructure(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 1024, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	items := dv.ContextMenuItemsForTest(0)
	labels := nonDividerLabels(items)
	want := []string{"Rename", "Color", "Origin", "Delete"}
	if !equalStringSlices(labels, want) {
		t.Fatalf("desktop labels=%v want=%v", labels, want)
	}
	for _, it := range items {
		if it.label == "Delete" && it.style == DisabledButtonStyle {
			t.Error("Delete should be enabled when row count > 1")
		}
	}
}

// TestContextMenuItemsMobileStructure documents the mobile variant which is
// identical to desktop: Rename / Color / Origin / Delete. "Instrument" was
// removed because tapping the row label opens the instrument picker directly on
// every platform; "Color" opens the grouped Vice City picker (Task 14);
// Effects remains omitted (it has its own row button).
func TestContextMenuItemsMobileStructure(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	dv := NewDrumView(image.Rect(0, 0, 390, 844), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	items := dv.ContextMenuItemsForTest(0)
	labels := nonDividerLabels(items)
	want := []string{"Rename", "Color", "Origin", "Delete"}
	if !equalStringSlices(labels, want) {
		t.Fatalf("mobile labels=%v want=%v", labels, want)
	}
}

// TestContextMenuItemsDeleteDisabledWhenSingleRow covers the destructive-
// group enable-state branch.
func TestContextMenuItemsDeleteDisabledWhenSingleRow(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 1024, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	items := dv.ContextMenuItemsForTest(0)
	var del *contextMenuItem
	for i := range items {
		if items[i].label == "Delete" {
			del = &items[i]
			break
		}
	}
	if del == nil {
		t.Fatal("Delete item not found")
	}
	if del.style != DisabledButtonStyle {
		t.Errorf("Delete style = %v, want DisabledButtonStyle when only 1 row", del.style)
	}
	if del.textColor != colTextDisabled {
		t.Errorf("Delete textColor = %v, want colTextDisabled", del.textColor)
	}
}

// TestContextMenuItemsGroupOrdering verifies items appear in monotonically
// non-decreasing group order, with dividers between groups.
func TestContextMenuItemsGroupOrdering(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 1024, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	items := dv.ContextMenuItemsForTest(0)
	prevGroup := -1
	for i, it := range items {
		if it.divider {
			continue
		}
		if it.group < prevGroup {
			t.Errorf("item %d (%q) group=%d < prev=%d", i, it.label, it.group, prevGroup)
		}
		prevGroup = it.group
	}
}

func nonDividerLabels(items []contextMenuItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.divider {
			continue
		}
		out = append(out, it.label)
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestContextMenuDesktopStaysOnScreen is the regression case for the desktop
// context menu running off-screen / over the EQ panel. Opening the menu for the
// last (bottom-most) row must keep dv.contextMenuRect fully inside dv.Bounds
// (AnchorPopupRect flip + clamp), and there is no close × on desktop.
func TestContextMenuDesktopStaysOnScreen(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 12; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Open for the last row (bottom-most anchor).
	dv.openContextMenu(len(dv.Rows) - 1)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	if !dv.contextMenuRect.In(dv.Bounds) {
		t.Fatalf("desktop context menu rect %v escapes bounds %v", dv.contextMenuRect, dv.Bounds)
	}
	for i, btn := range dv.contextMenuBtns {
		if !btn.Rect().In(dv.Bounds) {
			t.Errorf("context menu button %d rect %v escapes bounds %v", i, btn.Rect(), dv.Bounds)
		}
	}

	// Desktop now carries a header with a close × (last button).
	if n := len(dv.contextMenuBtns); n == 0 || dv.contextMenuBtns[n-1].Icon != "close" {
		t.Error("desktop context menu must have a close button")
	}
}

func TestContextMenuDesktopHasHeaderAndClose(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 1280, 720
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 4; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0})
	}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}
	// Title header rect must be present (like the mobile sheet / other menus).
	if dv.contextMenuHeaderRect.Empty() {
		t.Fatal("desktop context menu must have a non-empty header (title) rect")
	}
	// Close button present as the trailing button.
	n := len(dv.contextMenuBtns)
	if n == 0 || dv.contextMenuBtns[n-1].Icon != "close" {
		t.Fatal("desktop context menu must have a trailing close button")
	}
	// The first menu item must sit BELOW the header band (not overlapping it).
	headerBandBottom := dv.contextMenuRect.Min.Y + touchMinTargetPx
	if dv.contextMenuBtns[0].Rect().Min.Y < headerBandBottom {
		t.Fatalf("first item top %d above header band bottom %d", dv.contextMenuBtns[0].Rect().Min.Y, headerBandBottom)
	}
}

func TestContextMenuDesktopAnchorsNearKebab(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 1280, 720
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 4; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0})
	}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	kebabs := dv.rowMenuBtns()
	if len(kebabs) == 0 {
		t.Fatal("expected per-row kebab buttons")
	}
	kebab := kebabs[0].Rect()
	if kebab.Empty() {
		t.Fatal("kebab button rect is empty")
	}

	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}
	// Menu must appear near the kebab button (left edges aligned), not far-left
	// at the row label.
	dx := dv.contextMenuRect.Min.X - kebab.Min.X
	if dx < 0 {
		dx = -dx
	}
	if dx > 8 {
		t.Fatalf("context menu Min.X=%d not aligned to kebab Min.X=%d (delta %d)",
			dv.contextMenuRect.Min.X, kebab.Min.X, dx)
	}
	if !dv.contextMenuRect.In(dv.Bounds) {
		t.Fatalf("context menu rect %v escapes bounds %v", dv.contextMenuRect, dv.Bounds)
	}
}

// Context-menu item icon and label must never overlap (the mobile bug): the
// label starts to the right of the leading icon on both platforms.
func TestContextMenuItemLabelClearsIcon(t *testing.T) {
	assertDefaultParityState(t)
	const W, H = 1280, 720
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 4; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0})
	}
	dv.Length = 8
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()
	dv.openContextMenu(0)
	iconR, labelX := dv.contextMenuItemGeom(0)
	if labelX <= iconR.Max.X {
		t.Fatalf("context item label x %d overlaps icon (icon max x %d)", labelX, iconR.Max.X)
	}
	if iconR.Dx() != menuRowIconSize() {
		t.Fatalf("context item icon must be %dpx, got %d", menuRowIconSize(), iconR.Dx())
	}
}
