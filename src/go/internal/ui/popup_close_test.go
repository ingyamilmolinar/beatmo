//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestNodeMenuCloseButton verifies the close rect exists and clicking it closes the menu.
func TestNodeMenuCloseButton(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	r, ok := g.sidebar.rects["close"]
	if !ok || r.Empty() {
		t.Fatalf("close rect missing or empty")
	}

	btn := g.sidebar.btns["close"]
	if btn == nil {
		t.Fatalf("close button not wired")
	}
	if btn.Icon != "close" {
		t.Fatalf("expected icon='close', got %q", btn.Icon)
	}

	// Click the close button
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	g.handleEditor()
	g.Update()

	if g.sidebar.IsOpen() {
		t.Fatalf("node menu still open after clicking close button")
	}
}

// TestNodeMenuEscClose verifies ESC closes the node popup.
func TestNodeMenuEscClose(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	g.handleEditor()

	if g.sidebar.IsOpen() {
		t.Fatalf("node menu still open after ESC")
	}
}

// TestInstMenuCloseButton verifies the close button on the instrument menu component.
func TestInstMenuCloseButton(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	closeCalled := false
	comp.SetProps(InstrumentMenuProps{
		Categories:  []string{"Kicks"},
		Instruments: []InstrumentOption{{ID: "kick", Label: "Kick", Category: "Kicks"}},
		AnchorRect:  image.Rect(100, 50, 200, 70),
		VertBounds:  image.Rect(0, 0, 400, 600),
		RowHeight:   24,
		OnClose:     func() { closeCalled = true },
	})
	comp.Open()
	suppressClicksUntilRelease = false

	btn := comp.CloseBtn()
	if btn == nil {
		t.Fatalf("close button nil")
	}
	if btn.Icon != "close" {
		t.Fatalf("expected icon='close', got %q", btn.Icon)
	}

	// Simulate click on close button
	r := btn.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	btn.HandleInputResult(cx, cy, true)

	if comp.IsOpen() {
		t.Fatalf("instrument menu still open after clicking close button")
	}
	if !closeCalled {
		t.Fatalf("OnClose not called")
	}
}

// TestInstMenuEscClose verifies ESC closes the instrument menu.
func TestInstMenuEscClose(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	closeCalled := false
	comp.SetProps(InstrumentMenuProps{
		Categories:  []string{"Kicks"},
		Instruments: []InstrumentOption{{ID: "kick", Label: "Kick", Category: "Kicks"}},
		AnchorRect:  image.Rect(100, 50, 200, 70),
		VertBounds:  image.Rect(0, 0, 400, 600),
		RowHeight:   24,
		OnClose:     func() { closeCalled = true },
	})
	comp.Open()
	suppressClicksUntilRelease = false

	// Mock ESC key
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 600 },
	)
	defer restore()

	comp.HandleInput(0, 0, false)

	if comp.IsOpen() {
		t.Fatalf("instrument menu still open after ESC")
	}
	if !closeCalled {
		t.Fatalf("OnClose not called")
	}
}

// TestColorWheelCloseButton verifies the close button on the color wheel.
func TestColorWheelCloseButton(t *testing.T) {
	assertDefaultParityState(t)

	comp := NewColorWheelComponent()
	closeCalled := false
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 100, 130, 120),
		Bounds:     image.Rect(0, 0, 400, 400),
		RowHeight:  24,
		OnClose:    func() { closeCalled = true },
	})
	comp.Open()

	btn := comp.CloseBtn()
	if btn == nil {
		t.Fatalf("close button nil")
	}

	r := btn.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	btn.HandleInputResult(cx, cy, true)

	if comp.IsOpen() {
		t.Fatalf("color wheel still open after clicking close button")
	}
	if !closeCalled {
		t.Fatalf("OnClose not called")
	}
}

// TestContextMenuCloseButton verifies the close button appended to context menu.
func TestContextMenuCloseButton(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 800), nil, game_log.New(nil, game_log.LevelError))
	dv.calcLayout()

	dv.OpenContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatalf("context menu not open")
	}

	// Find the close button (last button in contextMenuBtns)
	btns := dv.ContextMenuBtns()
	if len(btns) == 0 {
		t.Fatalf("no context menu buttons")
	}
	closeBtn := btns[len(btns)-1]
	if closeBtn.Icon != "close" {
		t.Fatalf("last button is not close button, icon=%q", closeBtn.Icon)
	}

	r := closeBtn.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	// Clear suppress so click fires
	suppressClicksUntilRelease = false
	closeBtn.HandleInputResult(cx, cy, true)

	if dv.IsContextMenuOpen() {
		t.Fatalf("context menu still open after clicking close button")
	}
}

// TestContextMenuEscClose verifies ESC closes context menu.
func TestContextMenuEscClose(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 800), nil, game_log.New(nil, game_log.LevelError))
	dv.calcLayout()

	dv.OpenContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatalf("context menu not open")
	}

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 800 },
	)
	defer restore()

	dv.Update()

	if dv.IsContextMenuOpen() {
		t.Fatalf("context menu still open after ESC")
	}
}

// TestOverflowMenuCloseButton verifies the close button in the overflow popup.
func TestOverflowMenuCloseButton(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 800), nil, game_log.New(nil, game_log.LevelError))
	dv.calcLayout()

	dv.SetOverflowMenuOpen(true)
	if !dv.OverflowMenuOpen() {
		t.Fatalf("overflow menu not open")
	}

	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		t.Skipf("overflow popup rect empty (no overflow button)")
	}

	btns := dv.overflowPopupBtns(popupRect)
	if len(btns) == 0 {
		t.Fatalf("no overflow buttons")
	}
	closeBtn := btns[len(btns)-1]
	if closeBtn.Icon != "close" {
		t.Fatalf("last button is not close button, icon=%q", closeBtn.Icon)
	}

	r := closeBtn.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	suppressClicksUntilRelease = false
	closeBtn.HandleInputResult(cx, cy, true)

	if dv.OverflowMenuOpen() {
		t.Fatalf("overflow menu still open after clicking close button")
	}
}

// TestOverflowMenuEscClose verifies ESC closes the overflow menu.
func TestOverflowMenuEscClose(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 800), nil, game_log.New(nil, game_log.LevelError))
	dv.calcLayout()

	// Open via portal path so tree's ESC handler can close it.
	dv.openOverflowMenuPortal()

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 800 },
	)
	defer restore()

	dv.Update()

	if dv.OverflowMenuOpen() {
		t.Fatalf("overflow menu still open after ESC")
	}
}

// TestSubdivDropdownEscClose verifies ESC closes the subdiv dropdown.
func TestSubdivDropdownEscClose(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 640, 200), nil, game_log.New(nil, game_log.LevelError))
	dv.calcLayout()

	// Open via portal path so tree's ESC handler can close it.
	dv.subdivBtn().OnClick()

	if !dv.IsSubdivMenuOpen() {
		t.Fatalf("subdiv menu did not open")
	}

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 200 },
	)
	defer restore()

	dv.Update()

	if dv.IsSubdivMenuOpen() {
		t.Fatalf("subdiv menu still open after ESC")
	}
}

// TestEQChannelDropdownEscClose verifies ESC closes the EQ channel dropdown.
func TestEQChannelDropdownEscClose(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 640, 200), nil, game_log.New(nil, game_log.LevelError))
	dv.calcLayout()

	// Open via portal path so tree's ESC handler can close it.
	dv.eqPanelZone.stickyBar.ChannelBtn().OnClick()

	if !dv.tree.Portal().Has("eq-channel-dropdown") {
		t.Fatalf("eq-channel-dropdown portal not open")
	}

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 200 },
	)
	defer restore()

	dv.Update()

	if dv.tree.Portal().Has("eq-channel-dropdown") {
		t.Fatalf("EQ channel dropdown portal still open after ESC")
	}
}

// TestCloseAllPopups verifies that closeAllPopups closes everything.
func TestCloseAllPopups(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Open node menu
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)

	// Open drum view popups
	g.drum.openInstMenuPortal()
	g.drum.openColorWheelPortal()
	g.drum.openSubdivMenuPortal()
	g.drum.eqPanelZone.stickyBar.ChannelBtn().OnClick()
	g.drum.openOverflowMenuPortal()
	g.drum.openContextMenuPortal()

	g.closeAllPopups()

	if g.sidebar.IsOpen() {
		t.Fatalf("node menu still open")
	}
	if g.drum.IsInstMenuOpen() {
		t.Fatalf("inst menu still open")
	}
	if g.drum.IsColorMenuOpen() {
		t.Fatalf("color menu still open")
	}
	if g.drum.IsSubdivMenuOpen() {
		t.Fatalf("subdiv menu still open")
	}
	if g.drum.IsEQChannelOpen() {
		t.Fatalf("EQ channel still open")
	}
	if g.drum.IsOverflowMenuOpen() {
		t.Fatalf("overflow menu still open")
	}
	if g.drum.IsContextMenuOpen() {
		t.Fatalf("context menu still open")
	}
}
