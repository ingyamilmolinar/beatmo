//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestInstMenuBackHeldNoSpuriousDeferredTap reproduces the desktop back-button
// bug: after clicking Back (which switches mode and calls SuppressClicksUntilMouseUp),
// the held mouse position could land inside the new scroll view area, capturing a
// spurious deferred tap. On release, fireTapAt would hit a category button and flip
// the mode back to instruments.
func TestInstMenuBackHeldNoSpuriousDeferredTap(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	// Open menu via row label click.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open after clicking row label")
	}

	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instMenuComp should be open")
	}

	// Ensure we're in instruments mode (with Back button visible).
	if comp.Mode() == InstMenuModeCategories {
		ri := idleFrames(dv, 2, W, H)
		ri()
		catBtns := comp.CategoryBtns()
		if len(catBtns) == 0 {
			t.Fatal("no category buttons")
		}
		cr := catBtns[0].Rect()
		clickDrumViewAt(t, dv, cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2, W, H)
		ri = idleFrames(dv, 2, W, H)
		ri()
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %s", comp.Mode())
	}

	backBtn := comp.BackBtn()
	if backBtn == nil {
		t.Fatal("back button missing")
	}
	br := backBtn.Rect()
	bx := br.Min.X + br.Dx()/2
	by := br.Min.Y + br.Dy()/2

	// Press on Back button and hold for multiple frames WITHOUT calling
	// SetInputForTest between frames. SetInputForTest clears
	// suppressClicksUntilRelease, which prevents the suppress guard from
	// persisting across frames as it would in real runtime.
	r := SetInputForTest(
		func() (int, int) { return bx, by },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)

	// Frame 1: Press fires Back button → mode switches to categories.
	dv.Update()
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should stay open during back button press")
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories after back press, got %s", comp.Mode())
	}

	// Frames 2-4: Keep holding at same position. Suppress should block
	// the deferred tap capture at line 729.
	for i := 0; i < 3; i++ {
		dv.Update()
		if !dv.IsInstMenuOpen() {
			t.Fatalf("menu closed during held frame %d", i)
		}
		if comp.Mode() != InstMenuModeCategories {
			t.Fatalf("mode changed to %s on held frame %d", comp.Mode(), i)
		}
	}
	r() // restore before release

	// Frame 5: Release at the same position.
	r = releaseDrumView(dv, bx, by, W, H)
	dv.Update()
	r()

	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should stay open after release")
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories mode after held back+release, got %s", comp.Mode())
	}

	t.Log("back held no spurious deferred tap — PASS")
}

// TestInstMenuBackHeldMobileNoFlicker tests the same scenario on mobile layout
// where the bottom-sheet categories VS.View covers a larger area.
func TestInstMenuBackHeldMobileNoFlicker(t *testing.T) {
	assertDefaultParityState(t)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, _, _ := newCategoryDrumView(t, W, H)

	// Open menu programmatically.
	dv.openInstMenuForRow(0)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open")
	}

	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instMenuComp should be open")
	}

	// Navigate to instruments mode if in categories.
	if comp.Mode() == InstMenuModeCategories {
		ri := idleFrames(dv, 2, W, H)
		ri()
		catBtns := comp.CategoryBtns()
		if len(catBtns) == 0 {
			t.Fatal("no category buttons")
		}
		cr := catBtns[0].Rect()
		clickDrumViewAt(t, dv, cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2, W, H)
		ri = idleFrames(dv, 2, W, H)
		ri()
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %s", comp.Mode())
	}

	backBtn := comp.BackBtn()
	if backBtn == nil {
		t.Fatal("back button missing")
	}
	br := backBtn.Rect()
	bx := br.Min.X + br.Dx()/2
	by := br.Min.Y + br.Dy()/2

	// Press on Back button and hold for multiple frames using a single
	// SetInputForTest call to preserve suppressClicksUntilRelease.
	r := SetInputForTest(
		func() (int, int) { return bx, by },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)

	// Frame 1: Press fires Back → categories.
	dv.Update()

	// Frames 2-4: hold.
	for i := 0; i < 3; i++ {
		dv.Update()
	}
	r() // restore before release

	// Frame 5: Release.
	r = releaseDrumView(dv, bx, by, W, H)
	dv.Update()
	r()

	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should stay open after mobile back+hold+release")
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories on mobile after back+hold, got %s", comp.Mode())
	}

	t.Log("mobile back held no flicker — PASS")
}

// TestInstMenuCategoryHeldNoDeferredTapOnInstruments tests the reverse direction:
// pressing a category button switches to instruments mode, and holding the mouse
// at the same position should not capture a deferred tap that flips back.
func TestInstMenuCategoryHeldNoDeferredTapOnInstruments(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	// Open menu.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open")
	}

	comp := dv.instMenuComp
	if comp == nil {
		t.Fatal("instMenuComp is nil")
	}

	// Navigate to categories mode.
	if comp.Mode() == InstMenuModeInstruments {
		backBtn := comp.BackBtn()
		if backBtn == nil {
			t.Fatal("back button missing")
		}
		br := backBtn.Rect()
		clickDrumViewAt(t, dv, br.Min.X+br.Dx()/2, br.Min.Y+br.Dy()/2, W, H)
		ri := idleFrames(dv, 2, W, H)
		ri()
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories mode, got %s", comp.Mode())
	}

	// Let suppress clear.
	ri := idleFrames(dv, 3, W, H)
	ri()

	catBtns := comp.CategoryBtns()
	if len(catBtns) == 0 {
		t.Fatal("no category buttons")
	}
	cr := catBtns[0].Rect()
	catX := cr.Min.X + cr.Dx()/2
	catY := cr.Min.Y + cr.Dy()/2

	// Press on category and hold using a single SetInputForTest call
	// to preserve suppressClicksUntilRelease across frames.
	r := SetInputForTest(
		func() (int, int) { return catX, catY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)

	// Frame 1: Press fires category → instruments mode.
	dv.Update()

	// Frames 2-4: Hold.
	for i := 0; i < 3; i++ {
		dv.Update()
	}
	r() // restore before release

	// Release.
	r = releaseDrumView(dv, catX, catY, W, H)
	dv.Update()
	r()

	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should stay open after category+hold+release")
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode after category hold+release, got %s", comp.Mode())
	}

	t.Log("category held no deferred tap on instruments — PASS")
}

// TestInstMenuOpenClearsDeferredTapState verifies that Open() clears any
// stale deferredTapActive state as a defense-in-depth measure.
func TestInstMenuOpenClearsDeferredTapState(t *testing.T) {
	assertDefaultParityState(t)

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:    image.Rect(100, 100, 200, 124),
		VertBounds:    image.Rect(0, 0, 800, 600),
		RowHeight:     24,
		LabelWidth:    100,
		ControlsWidth: 100,
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick"},
			{ID: "snare", Label: "Snare"},
		},
	})

	// Simulate stale deferred tap state.
	comp.deferredTap.Begin(150, 112)

	comp.Open()

	if comp.deferredTap.Active() {
		t.Fatal("Open() should clear deferredTap")
	}
	if !comp.IsOpen() {
		t.Fatal("menu should be open after Open()")
	}
}

// TestEQChannelMenuSuppressBlocksDeferredTap verifies that DeferredTap.Begin()
// does not capture a tap while suppressClicksUntilRelease is active. This is
// the portal-era equivalent of the legacy handleEQChannelMenuInput guard.
func TestEQChannelMenuSuppressBlocksDeferredTap(t *testing.T) {
	assertDefaultParityState(t)

	// DeferredTap.Begin() checks suppressClicksUntilRelease directly.
	var dt DeferredTap

	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	if dt.Begin(100, 200) {
		t.Fatal("Begin should return false while suppress is active")
	}
	if dt.Active() {
		t.Fatal("deferred tap should NOT be active while suppress blocks it")
	}
}

// TestInstMenuCloseButtonViaDeferredTap reproduces the mobile close button bug:
// on mobile, the close button rect is inside m.scroll.VS.View, so a touch press
// captures it as a deferred tap. On release, fireTapAt must check the close
// button to properly close the menu.
func TestInstMenuCloseButtonViaDeferredTap(t *testing.T) {
	assertDefaultParityState(t)

	// The deferred-tap (fire-on-release) close path is a mobile affordance;
	// desktop closes on the press edge. Force mobile so this exercises the
	// deferred-tap path the test name describes.
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	// Open menu via row label click.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open after clicking row label")
	}

	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instMenuComp should be open")
	}

	// Let suppress clear.
	ri := idleFrames(dv, 3, W, H)
	ri()

	closeBtn := comp.CloseBtn()
	if closeBtn == nil {
		t.Fatal("close button missing")
	}
	cr := closeBtn.Rect()
	if cr.Empty() {
		t.Fatal("close button rect is empty")
	}
	closedCalled := false
	origOnClose := comp.props.OnClose
	comp.props.OnClose = func() {
		closedCalled = true
		if origOnClose != nil {
			origOnClose()
		}
	}

	cbx := cr.Min.X + cr.Dx()/2
	cby := cr.Min.Y + cr.Dy()/2

	// Simulate touch press at close button position (captures as deferred tap).
	r := pressDrumView(dv, cbx, cby, W, H)
	dv.Update()
	r()

	// Menu should still be open (touch is held).
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should still be open during press")
	}

	// Simulate touch release at same position (fires deferred tap).
	r = releaseDrumView(dv, cbx, cby, W, H)
	dv.Update()
	r()

	if dv.IsInstMenuOpen() {
		t.Fatal("menu should be closed after tapping close button via deferred tap")
	}
	if !closedCalled {
		t.Fatal("OnClose callback was not called")
	}

	t.Log("close button via deferred tap — PASS")
}

// TestInstMenuCloseButtonViaDeferredTapMobile is the same test but with mobile
// layout where the bottom-sheet covers a larger area.
func TestInstMenuCloseButtonViaDeferredTapMobile(t *testing.T) {
	assertDefaultParityState(t)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, _, _ := newCategoryDrumView(t, W, H)

	// Open menu programmatically.
	dv.openInstMenuForRow(0)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open")
	}

	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instMenuComp should be open")
	}

	// Let suppress clear and layout settle.
	ri := idleFrames(dv, 3, W, H)
	ri()

	closeBtn := comp.CloseBtn()
	if closeBtn == nil {
		t.Fatal("close button missing")
	}
	cr := closeBtn.Rect()
	if cr.Empty() {
		t.Fatal("close button rect is empty")
	}
	closedCalled := false
	origOnClose := comp.props.OnClose
	comp.props.OnClose = func() {
		closedCalled = true
		if origOnClose != nil {
			origOnClose()
		}
	}

	cbx := cr.Min.X + cr.Dx()/2
	cby := cr.Min.Y + cr.Dy()/2

	// Simulate touch press at close button position.
	r := pressDrumView(dv, cbx, cby, W, H)
	dv.Update()
	r()

	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should still be open during press")
	}

	// Simulate touch release (fires deferred tap).
	r = releaseDrumView(dv, cbx, cby, W, H)
	dv.Update()
	r()

	if dv.IsInstMenuOpen() {
		t.Fatal("menu should be closed after mobile close button deferred tap")
	}
	if !closedCalled {
		t.Fatal("OnClose callback was not called on mobile")
	}

	t.Log("mobile close button via deferred tap — PASS")
}

// TestInstMenuAllPopupsHeldMouseStable verifies that for each popup type,
// pressing inside for multiple frames and then releasing does not cause the
// popup to close or change state unexpectedly.
func TestInstMenuAllPopupsHeldMouseStable(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600

	t.Run("instrument_menu", func(t *testing.T) {
		dv, cx, cy := newCategoryDrumView(t, W, H)

		// Open menu.
		clickDrumViewAt(t, dv, cx, cy, W, H)
		if !dv.IsInstMenuOpen() {
			t.Fatal("menu should be open")
		}

		comp := dv.instMenuComp
		if comp == nil {
			t.Fatal("comp is nil")
		}

		// Get a point inside the menu.
		sv := comp.ScrollView()
		if sv.Empty() {
			t.Skip("scroll view empty")
		}
		mx := sv.Min.X + sv.Dx()/2
		my := sv.Min.Y + sv.Dy()/2

		// Let suppress clear.
		ri := idleFrames(dv, 3, W, H)
		ri()

		// Press inside for 5 frames.
		for i := 0; i < 5; i++ {
			r := pressDrumView(dv, mx, my, W, H)
			dv.Update()
			r()
			if !dv.IsInstMenuOpen() {
				t.Fatalf("inst menu closed on held frame %d", i)
			}
		}

		// Release.
		r := releaseDrumView(dv, mx, my, W, H)
		dv.Update()
		r()

		if !dv.IsInstMenuOpen() {
			t.Fatal("inst menu should be open after held+release")
		}
	})

	t.Run("subdiv_menu", func(t *testing.T) {
		dv := NewDrumView(image.Rect(0, 0, W, H), nil, testLogger)
		dv.Rows = []*DrumRow{{
			Name: "Kick", Instrument: "kick",
			Steps: make([]bool, 8), Volume: 1.0,
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

		if dv.subdivBtn() == nil {
			t.Skip("subdivBtn not created")
		}

		// Open subdiv menu.
		dv.openSubdivMenuPortal()
		SuppressClicksUntilMouseUp()
		suppressClicksUntilRelease = false // clear for test

		br := dv.subdivBtn().Rect()
		if br.Empty() {
			t.Skip("subdivBtn rect empty")
		}

		// Build menu buttons.
		menuRect := image.Rect(br.Min.X, br.Max.Y, br.Max.X, br.Max.Y+len(dv.subdivMenuBtns)*dv.rowHeight())
		if menuRect.Empty() || len(dv.subdivMenuBtns) == 0 {
			t.Skip("subdiv menu buttons empty")
		}

		mx := menuRect.Min.X + menuRect.Dx()/2
		my := menuRect.Min.Y + menuRect.Dy()/2

		// Press inside for 5 frames.
		for i := 0; i < 5; i++ {
			r := pressDrumView(dv, mx, my, W, H)
			dv.Update()
			r()
			if !dv.IsSubdivMenuOpen() {
				t.Fatalf("subdiv menu closed on held frame %d", i)
			}
		}

		// Release.
		r := releaseDrumView(dv, mx, my, W, H)
		dv.Update()
		r()

		if !dv.IsSubdivMenuOpen() {
			t.Fatal("subdiv menu should be open after held+release")
		}
	})
}
