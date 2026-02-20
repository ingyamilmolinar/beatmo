//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestInstMenuStaysOpenAfterClick verifies that clicking the row label opens
// the instrument menu and that the menu stays open across subsequent frames.
// This catches the bug where the menu would "pop up and go away immediately."
func TestInstMenuStaysOpenAfterClick(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	if len(dv.rowLabels) == 0 {
		t.Fatal("rowLabels not created after calcLayout")
	}

	// Get row label rect
	lblRect := dv.rowLabels[0].Rect()
	if lblRect.Empty() {
		t.Fatal("row label rect is empty")
	}
	cx := lblRect.Min.X + lblRect.Dx()/2
	cy := lblRect.Min.Y + lblRect.Dy()/2
	t.Logf("Row label rect: %v, click at (%d, %d)", lblRect, cx, cy)

	// Verify menu is initially closed
	if dv.instMenuOpen {
		t.Fatal("inst menu should be closed initially")
	}
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
		t.Fatal("inst menu comp should be closed initially")
	}

	// Frame 1: Mouse down on row label
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()

	t.Logf("After frame 1 (press): instMenuOpen=%v, compOpen=%v, suppress=%v",
		dv.instMenuOpen, dv.instMenuComp != nil && dv.instMenuComp.IsOpen(), suppressClicksUntilRelease)

	if !dv.instMenuOpen {
		t.Fatal("inst menu should be OPEN after clicking row label")
	}
	if dv.instMenuComp != nil && !dv.instMenuComp.IsOpen() {
		t.Fatal("inst menu comp should be OPEN after clicking row label")
	}

	// Frame 2: Mouse still held
	dv.Update()
	t.Logf("After frame 2 (held): instMenuOpen=%v, compOpen=%v, suppress=%v",
		dv.instMenuOpen, dv.instMenuComp != nil && dv.instMenuComp.IsOpen(), suppressClicksUntilRelease)

	if !dv.instMenuOpen {
		t.Fatal("inst menu should STILL be open after frame 2 (held)")
	}

	// Frame 3: Mouse released
	restore()
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()

	t.Logf("After frame 3 (release): instMenuOpen=%v, compOpen=%v, suppress=%v",
		dv.instMenuOpen, dv.instMenuComp != nil && dv.instMenuComp.IsOpen(), suppressClicksUntilRelease)

	if !dv.instMenuOpen {
		t.Fatal("inst menu should STILL be open after mouse release")
	}

	// Frames 4-8: No mouse (verify menu stays open)
	restore()
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	for i := 4; i <= 8; i++ {
		dv.Update()
		open := dv.instMenuOpen
		compOpen := dv.instMenuComp != nil && dv.instMenuComp.IsOpen()
		if !open || !compOpen {
			t.Fatalf("inst menu closed unexpectedly on frame %d: open=%v, compOpen=%v", i, open, compOpen)
		}
	}
	restore()

	t.Log("Menu stayed open for all frames — PASS")
}

// TestInstMenuStaysOpenQuickClick verifies that a fast click (1 frame press
// + 1 frame release) opens and keeps the menu open. This matches the behavior
// of a real quick mouse click or fast CDP tap.
func TestInstMenuStaysOpenQuickClick(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	lblRect := dv.rowLabels[0].Rect()
	cx := lblRect.Min.X + lblRect.Dx()/2
	cy := lblRect.Min.Y + lblRect.Dy()/2

	// Use clickDrumView which does press→release in 2 frames
	clickDrumView(t, dv, cx, cy)

	t.Logf("After quick click: instMenuOpen=%v, compOpen=%v",
		dv.instMenuOpen, dv.instMenuComp != nil && dv.instMenuComp.IsOpen())

	if !dv.instMenuOpen {
		t.Fatal("inst menu should be open after quick click")
	}

	// Verify stays open for 5 more frames
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	for i := 0; i < 5; i++ {
		dv.Update()
		if !dv.instMenuOpen {
			t.Fatalf("menu closed on frame %d after quick click", i)
		}
	}
	restore()

	t.Log("Menu stayed open after quick click — PASS")
}

// TestInstMenuToggleOnSecondClick verifies that clicking the row label while
// the menu is already open for the same row closes it (toggle behavior).
func TestInstMenuToggleOnSecondClick(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	lblRect := dv.rowLabels[0].Rect()
	cx := lblRect.Min.X + lblRect.Dx()/2
	cy := lblRect.Min.Y + lblRect.Dy()/2

	// Helper to run idle frames (no input) to let state settle
	idle := func(n int) {
		r := SetInputForTest(
			func() (int, int) { return 0, 0 },
			func(b ebiten.MouseButton) bool { return false },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 800, 600 },
		)
		for i := 0; i < n; i++ {
			dv.Update()
		}
		r()
	}

	// First click: opens menu
	clickDrumView(t, dv, cx, cy)
	t.Logf("After 1st click: instMenuOpen=%v, compOpen=%v, capturing=%v",
		dv.instMenuOpen,
		dv.instMenuComp != nil && dv.instMenuComp.IsOpen(),
		dv.instMenuComp != nil && dv.instMenuComp.Capturing())
	if !dv.instMenuOpen {
		t.Fatal("menu should be open after first click")
	}

	// Settle
	idle(3)

	// Second click: toggles menu closed
	clickDrumView(t, dv, cx, cy)
	t.Logf("After 2nd click: instMenuOpen=%v, compOpen=%v, capturing=%v",
		dv.instMenuOpen,
		dv.instMenuComp != nil && dv.instMenuComp.IsOpen(),
		dv.instMenuComp != nil && dv.instMenuComp.Capturing())
	if dv.instMenuOpen || (dv.instMenuComp != nil && dv.instMenuComp.IsOpen()) {
		t.Fatal("menu should be CLOSED after second click (toggle)")
	}

	// Settle — let hold state clear
	idle(3)
	t.Logf("After settle: instMenuOpen=%v, compOpen=%v, capturing=%v",
		dv.instMenuOpen,
		dv.instMenuComp != nil && dv.instMenuComp.IsOpen(),
		dv.instMenuComp != nil && dv.instMenuComp.Capturing())

	// Third click: opens menu again
	clickDrumView(t, dv, cx, cy)
	t.Logf("After 3rd click: instMenuOpen=%v, compOpen=%v, capturing=%v",
		dv.instMenuOpen,
		dv.instMenuComp != nil && dv.instMenuComp.IsOpen(),
		dv.instMenuComp != nil && dv.instMenuComp.Capturing())
	if !dv.instMenuOpen {
		t.Fatal("menu should be open again after third click")
	}

	t.Log("Toggle behavior works correctly — PASS")
}

// TestInstMenuMobileStaysOpen tests that on a small screen (mobile layout),
// opening the inst menu via the row label keeps it open.
func TestInstMenuMobileStaysOpen(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	dv := NewDrumView(image.Rect(0, 0, 390, 844), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap", "cowbell", "ride", "crash"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Run a warm-up frame (no input) to let recalcButtons/calcLayout settle
	// the mobile-specific layout. On mobile, the first recalcButtons call
	// triggers mobileEQInited → bgDirty → calcLayout, which changes row positions.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	dv.Update()
	warmUp()

	if len(dv.rowLabels) == 0 {
		t.Fatal("rowLabels not created")
	}

	lblRect := dv.rowLabels[0].Rect()
	cx := lblRect.Min.X + lblRect.Dx()/2
	cy := lblRect.Min.Y + lblRect.Dy()/2

	// Frame 1: press on row label — on mobile, opens context menu (not inst menu)
	restorePress := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	dv.Update()
	restorePress()

	if !dv.contextMenuOpen {
		t.Fatalf("mobile context menu should be open after press on label (contextMenuOpen=%v, renameHold=%v, anyDragActive=%v)",
			dv.contextMenuOpen, dv.renameHold, dv.anyDragActive())
	}

	// Frame 2: release
	restoreRel := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	dv.Update()
	restoreRel()

	if !dv.contextMenuOpen {
		t.Fatal("mobile context menu should still be open after release")
	}

	// Verify stays open for several idle frames
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	for i := 0; i < 10; i++ {
		dv.Update()
		if !dv.contextMenuOpen {
			t.Fatalf("mobile context menu closed on idle frame %d", i)
		}
	}
	restore()
}

// TestInstMenuMobileTapInjectionDoesNotClose verifies that on mobile, the
// Game-level guard prevents tap injection when a drum popup is open. The
// primary defense is !g.drum.anyDropdownOpen() in game_update.go which
// suppresses injectTouchTap entirely.
func TestInstMenuMobileTapInjectionDoesNotClose(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	dv := NewDrumView(image.Rect(0, 0, 390, 844), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.instCategories = nil
	dv.instCatByID = nil

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for mobile layout
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	dv.Update()
	warmUp()

	if len(dv.rowLabels) == 0 {
		t.Fatal("rowLabels not created")
	}

	lblRect := dv.rowLabels[0].Rect()
	cx := lblRect.Min.X + lblRect.Dx()/2
	cy := lblRect.Min.Y + lblRect.Dy()/2

	// --- Phase 1: Open context menu via the row label (mobile behavior) ---
	r1 := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	dv.Update()
	r1()

	if !dv.contextMenuOpen {
		t.Fatal("context menu should be open after press on label")
	}

	// --- Phase 2: Release ---
	r2 := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	dv.Update()
	r2()

	if !dv.contextMenuOpen {
		t.Fatal("context menu should stay open after release")
	}

	// --- Phase 3: Verify the Game-level guard ---
	// anyDropdownOpen() must return true when the context menu is open.
	// This is the condition that prevents tap injection in game_update.go.
	if !dv.anyDropdownOpen() {
		t.Fatal("anyDropdownOpen() should return true when context menu is open")
	}

	// The Game-level guard in game_update.go:
	//   if ... && !g.drum.anyDropdownOpen() { injectTouchTap(...) }
	// ensures injectTouchTap is never called while the menu is open.
	// This prevents the double-fire that causes the menu to toggle closed.
	t.Log("anyDropdownOpen() correctly returns true — tap injection blocked")

	// --- Phase 4: Verify context menu survives idle frames ---
	idle := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	for i := 0; i < 5; i++ {
		dv.Update()
		if !dv.contextMenuOpen {
			t.Fatalf("context menu closed on idle frame %d", i)
		}
	}
	idle()

	t.Log("Mobile tap injection guard works correctly — PASS")
}

// TestInstMenuMobileWithCategories tests mobile layout with forced categories.
// On mobile, the label opens the context menu first; the "Instrument" item
// in the context menu opens the inst menu with categories.
func TestInstMenuMobileWithCategories(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	dv := NewDrumView(image.Rect(0, 0, 390, 844), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.instCategories = []string{"Drums", "Cymbals"}
	dv.instCatByID = map[string]string{
		"kick":  "Drums",
		"snare": "Drums",
		"tom":   "Drums",
		"hihat": "Cymbals",
		"clap":  "Cymbals",
	}
	dv.instMenuForceCategories = true

	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Run warm-up frame for mobile layout init
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	dv.Update()
	warmUp()

	if len(dv.rowLabels) == 0 {
		t.Fatal("rowLabels not created")
	}

	// On mobile, clicking the label opens the context menu.
	// Then clicking "Instrument" in the context menu opens the inst menu.
	dv.openContextMenu(0)
	if !dv.contextMenuOpen {
		t.Fatal("context menu should be open")
	}

	// Find and click the "Instrument" button in the context menu.
	var instBtn *Button
	for _, btn := range dv.contextMenuBtns {
		if btn.Text == "Instrument" {
			instBtn = btn
			break
		}
	}
	if instBtn == nil {
		t.Fatal("Instrument button not found in context menu")
	}
	instBtn.OnClick()

	t.Logf("After Instrument click: instMenuOpen=%v, compOpen=%v",
		dv.instMenuOpen, dv.instMenuComp != nil && dv.instMenuComp.IsOpen())

	if !dv.instMenuOpen {
		t.Fatal("mobile inst menu with categories should be open")
	}

	// Check it's in categories mode
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
		mode := dv.instMenuComp.Mode()
		t.Logf("Menu mode: %s", mode)
		t.Logf("Menu fullRect: %v", dv.instMenuComp.fullRect)
		t.Logf("Scroll view: %v", dv.instMenuComp.ScrollView())
	}

	// Verify stays open for several frames
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	for i := 0; i < 10; i++ {
		dv.Update()
		if !dv.instMenuOpen {
			t.Fatalf("mobile category menu closed on frame %d", i)
		}
	}
	restore()

	t.Log("Mobile menu with categories stayed open — PASS")
}
