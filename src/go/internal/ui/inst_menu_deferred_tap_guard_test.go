//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Back switches mode and suppresses clicks until mouse-up: the held pointer then sits over the new view and
// must not capture a deferred tap that flips the mode back on release.
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

	// One SetInputForTest for the whole hold: each call clears suppressClicksUntilRelease, which real input never does.
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

	// Suppress must block the deferred-tap capture while the press is held.
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

// The mobile bottom-sheet view covers a larger area.
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

	// One SetInputForTest preserves suppressClicksUntilRelease across frames.
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

	// One SetInputForTest preserves suppressClicksUntilRelease across frames.
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

// Defence in depth: Open clears any stale deferred tap.
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
			{ID: roster.Kick, Label: "Kick"},
			{ID: roster.Snare, Label: "Snare"},
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

// The mobile bottom sheet has no close ×: it dismisses on a tap away.
func TestInstMenuCloseButtonAbsentOnMobile(t *testing.T) {
	assertDefaultParityState(t)

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

	// Let suppress clear + layout settle so the close × (if any) would be built.
	ri := idleFrames(dv, 3, W, H)
	ri()

	if closeBtn := comp.CloseBtn(); closeBtn != nil {
		t.Fatalf("mobile instrument menu must have no close ×; got rect %+v", closeBtn.Rect())
	}
}

func TestInstMenuCloseButtonAbsentOnMobileProgrammatic(t *testing.T) {
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

	if closeBtn := comp.CloseBtn(); closeBtn != nil {
		t.Fatalf("mobile instrument menu must have no close ×; got rect %+v", closeBtn.Rect())
	}
}

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
			Name: "Kick", Instrument: roster.Kick,
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
