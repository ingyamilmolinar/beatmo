//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The kebab press lingers as a touch; without the suppress check it lands on the menu's first item
// (Instrument) on release.
func TestKebabTapDoesNotOpenInstMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

	// Still held: suppressClicksUntilRelease must block the deferred tap.
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

	// Release clears suppress, but no tap was captured, so nothing fires.
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

// The injected tap's press must not restart touch-scroll tracking, whose dead zone blocks every row button.
func TestKebabTapWorksWithScrollableRows(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling is needed.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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

	// touchOverrideActive is set AFTER SetInputForTest, which resets it, so this reads as real touch.
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

	// The injection state is set AFTER SetInputForTest, which clears it.
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

// The injected tap must not re-enable the scroll dead zone over the label.
func TestRowLabelTapWorksWithScrollableRows(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling is needed.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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

	// touchOverrideActive is set AFTER SetInputForTest, which resets it.
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

	// The injection state is set AFTER SetInputForTest, which clears it.
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

// The real phone timing: the injected tap starts while the scroller is still active, with no release frame between.
func TestKebabTapOverlapScrollerActive(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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

	// touchOverrideActive is set AFTER SetInputForTest, which resets it.
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

	// No release frame: on a phone injectTouchTap starts on the frame the touch ends.

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

// The real phone timing, for the label.
func TestRowLabelTapOverlapScrollerActive(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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

	// touchOverrideActive is set AFTER SetInputForTest, which resets it.
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

	// Clears the tree's press state so the injected tap is a new press.
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

// The close × is appended last but overlaps the first row; reverse iteration must let it win.
func TestContextMenuCloseButtonDoesNotFireInstrument(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

// FX has its own row button.
func TestContextMenuMobileOmitsEffects(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

// It closes via the folder toggle, Esc or click-outside.
func TestOverflowMenuHasNoCloseButtonMobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

	dv.openOverflowMenuPortal()
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		t.Fatal("overflow popup rect is empty")
	}

	popupBtns := dv.overflowPopupBtns(popupRect)
	if len(popupBtns) == 0 {
		t.Fatal("no overflow popup buttons")
	}
	for _, b := range popupBtns {
		if b != nil && b.Icon == "close" {
			t.Fatalf("File menu must have no × button; found one with icon=%q", b.Icon)
		}
	}
}

func TestColorPickerCloseUnblocksButtons(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

	dv.openColorPickerForRow(0)

	// Hold flags no longer feed anyDragActive.
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

func TestCloseAllPopupsClearsColorMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

// Through HandleInput's overlay dispatch, which a direct dv.Update() skips.
func TestKebabTapViaHandleInputPipeline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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

	// touchOverrideActive is set AFTER SetInputForTest, which resets it.
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

func TestRowLabelTapViaHandleInputPipeline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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

	// touchOverrideActive is set AFTER SetInputForTest, which resets it.
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

// After an outside-click close and the suppress cycle, the kebab works again.
func TestButtonTapAfterContextMenuClose(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

	// The overlay stack closes it and sets suppressClicksUntilRelease.
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

// Only the menu's own Rename button is a rename trigger: on the kebab, the JS touchend would create a
// native input and bypass the menu.
func TestKebabTapDoesNotRegisterRenameTrigger(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

	// The native-input rects are re-projected only by dv.Update's sync.
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

// A small-window desktop browser: the touch dead zone must not block mouse clicks.
func TestMouseClickWorksOnMobileLayout(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling is needed.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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

	// touchOverrideActive stays false: a mouse.
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

// The label opens the picker on every platform; the kebab owns the context menu.
func TestMouseClickOnLabelWorksOnMobileLayout(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create 20 rows to guarantee vertical scrolling is needed.
	for i := 0; i < 20; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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

	if !dv.IsInstMenuOpen() {
		t.Fatalf("instrument picker should open on label click in mobile layout (instMenuOpen=%v contextMenuOpen=%v)",
			dv.IsInstMenuOpen(), dv.IsContextMenuOpen())
	}
	if dv.IsContextMenuOpen() {
		t.Error("label click must not open the context menu")
	}
}

// Update handlers must be blocked by the menu overlay, or the click starts playback.
func TestContextMenuBlocksTransportButtons(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

func TestOverflowMenuBlocksWidgets(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

func TestContextMenuBlocksRowVolumeSlider(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

func TestContextMenuScrollWhenOverflow(t *testing.T) {
	assertDefaultParityState(t)

	// Use a very small drum pane so context menu items overflow.
	const W, H = 300, 120
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

func TestContextMenuClosedReturnsEarly(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
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

// Click-outside closing belongs to the tree, not the handler.
func TestContextMenuClickOutsideCloses(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
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

	got := dv.handleContextMenuInput(500, 500, true)
	if got {
		t.Fatal("handleContextMenuInput should return false for clicks outside contextMenuRect")
	}
}

func TestOverflowMenuClosedReturnsEarly(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
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

func TestContextMenuDesktopButtonIteration(t *testing.T) {
	assertDefaultParityState(t)
	// Don't use withSmallScreen — default is desktop mode.

	const W, H = 1024, 768
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
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

	cx := (menuRect.Min.X + menuRect.Max.X) / 2
	cy := (menuRect.Min.Y + menuRect.Max.Y) / 2
	got := dv.handleContextMenuInput(cx, cy, true)
	if !got {
		t.Fatal("expected click inside menu rect to be consumed")
	}
}

// Instrument and Effects are reached through the label and row controls; the menu is the same on every platform.
func TestContextMenuItemsDesktopStructure(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 1024, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: roster.Snare, Steps: make([]bool, 8), Volume: 1.0},
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

// Identical to desktop: the label opens the picker, and FX has its own row button.
func TestContextMenuItemsMobileStructure(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	dv := NewDrumView(image.Rect(0, 0, 390, 844), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: roster.Snare, Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	items := dv.ContextMenuItemsForTest(0)
	labels := nonDividerLabels(items)
	want := []string{"Rename", "Color", "Origin", "Delete"}
	if !equalStringSlices(labels, want) {
		t.Fatalf("mobile labels=%v want=%v", labels, want)
	}
}

func TestContextMenuItemsDeleteDisabledWhenSingleRow(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 1024, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0},
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

func TestContextMenuItemsGroupOrdering(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 1024, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: roster.Snare, Steps: make([]bool, 8), Volume: 1.0},
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

// The bottom row's menu must stay inside dv.Bounds (flip + clamp), with no close × on desktop.
func TestContextMenuDesktopStaysOnScreen(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 12; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
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
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0})
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
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0})
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
	// Near the kebab, not far left at the row label.
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

func TestContextMenuItemLabelClearsIcon(t *testing.T) {
	assertDefaultParityState(t)
	const W, H = 1280, 720
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 4; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0})
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
