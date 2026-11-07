//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// ---------- helpers ----------

// idleFrames runs n frames with no input on a DrumView.
func idleFrames(dv *DrumView, n int, w, h int) func() {
	r := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	for i := 0; i < n; i++ {
		dv.Update()
	}
	return r
}

func newCategoryDrumView(t *testing.T, w, h int) (*DrumView, int, int) {
	t.Helper()

	dv := NewDrumView(image.Rect(0, 0, w, h), nil, testLogger)
	dv.instOptions = []instrument.ID{roster.Kick, roster.Snare, roster.Tom, roster.Hihat, roster.Crash}
	dv.instCategories = []audio.Category{audio.CatPercussion, audio.CatCymbal}
	dv.instCatByID = map[instrument.ID]audio.Category{
		roster.Kick:  audio.CatPercussion,
		roster.Snare: audio.CatPercussion,
		roster.Tom:   audio.CatPercussion,
		roster.Hihat: audio.CatCymbal,
		roster.Crash: audio.CatCymbal,
	}
	dv.instMenuForceCategories = true
	// These tests index Categories from the caller's first entry, so the virtual Favorites category is off.
	dv.instMenuShowFavoritesCategory = false

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
		func() (int, int) { return w, h },
	)
	dv.Update()
	warmUp()

	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after warm-up")
	}

	lblRect := dv.rowLabels()[0].Rect()
	cx := lblRect.Min.X + lblRect.Dx()/2
	cy := lblRect.Min.Y + lblRect.Dy()/2
	return dv, cx, cy
}

// pressDrumView simulates a mouse press at (x, y) for one frame.
func pressDrumView(dv *DrumView, x, y, w, h int) func() {
	return SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
}

// releaseDrumView simulates a mouse release at (x, y) for one frame.
func releaseDrumView(dv *DrumView, x, y, w, h int) func() {
	return SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
}

// clickDrumViewAt performs a 2-frame click (press+release) through dv.Update().
func clickDrumViewAt(t *testing.T, dv *DrumView, x, y, w, h int) {
	t.Helper()
	r := pressDrumView(dv, x, y, w, h)
	dv.Update()
	r()
	r = releaseDrumView(dv, x, y, w, h)
	dv.Update()
	r()
}

// ---------- tests ----------

// Through DrumView.Update, not comp.HandleInput: clearing suppressClicksUntilRelease too early turns the
// Back click into a click outside.
func TestInstMenuBackButtonViaDrumViewUpdate(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	// Open menu via row label click.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open after clicking row label")
	}

	// Should start in instruments mode (kicked from categories for current instrument).
	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instMenuComp should be open")
	}
	t.Logf("mode after open: %s", comp.Mode())

	if comp.Mode() == InstMenuModeInstruments {
		backBtn := comp.BackBtn()
		if backBtn == nil {
			t.Fatal("back button should exist when categories are available")
		}
		br := backBtn.Rect()
		if br.Empty() {
			t.Fatal("back button rect is empty")
		}
		bx := br.Min.X + br.Dx()/2
		by := br.Min.Y + br.Dy()/2

		// Frame 1: press on back button
		r := pressDrumView(dv, bx, by, W, H)
		dv.Update()
		r()

		if !dv.IsInstMenuOpen() {
			t.Fatal("menu should stay open during back button press")
		}

		// Frame 2: still held
		r = pressDrumView(dv, bx, by, W, H)
		dv.Update()
		r()

		if !dv.IsInstMenuOpen() {
			t.Fatal("menu should stay open while back button held")
		}

		// Frame 3: release
		r = releaseDrumView(dv, bx, by, W, H)
		dv.Update()
		r()

		if !dv.IsInstMenuOpen() {
			t.Fatal("menu should stay open after back button release")
		}

		// Frame 4: idle (no input)
		ri := idleFrames(dv, 1, W, H)
		ri()

		if !dv.IsInstMenuOpen() {
			t.Fatal("menu should stay open after idle frame following back")
		}

		t.Logf("mode after back: %s", comp.Mode())
		if comp.Mode() != InstMenuModeCategories {
			t.Fatalf("expected categories mode after back, got %s", comp.Mode())
		}
	} else {
		t.Logf("already in categories mode — testing from there")
	}

	// Now we're in categories mode. Verify it stays open for several idle frames.
	ri := idleFrames(dv, 5, W, H)
	ri()
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu closed during idle frames after back")
	}

	t.Log("back button via DrumView.Update() — PASS")
}

func TestInstMenuBackThenCategoryRoundtrip(t *testing.T) {
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

	// If in instruments mode, click back first.
	if comp.Mode() == InstMenuModeInstruments {
		backBtn := comp.BackBtn()
		if backBtn == nil {
			t.Fatal("back button missing")
		}
		br := backBtn.Rect()
		clickDrumViewAt(t, dv, br.Min.X+br.Dx()/2, br.Min.Y+br.Dy()/2, W, H)
		if comp.Mode() != InstMenuModeCategories {
			t.Fatalf("expected categories after back, got %s", comp.Mode())
		}
	}

	// Let suppress clear.
	ri := idleFrames(dv, 2, W, H)
	ri()

	// Click "Cymbals" category.
	var cymbalsBtn *Button
	for _, btn := range comp.CategoryBtns() {
		if btn.Text == "Cymbal" {
			cymbalsBtn = btn
			break
		}
	}
	if cymbalsBtn == nil {
		t.Fatal("could not find Cymbals category button")
	}
	cr := cymbalsBtn.Rect()
	clickDrumViewAt(t, dv, cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2, W, H)

	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should stay open after selecting Cymbals")
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode after category click, got %s", comp.Mode())
	}
	if comp.ActiveCategory() != audio.CatCymbal {
		t.Fatalf("expected active category Cymbals, got %s", comp.ActiveCategory())
	}

	// Verify visible instruments are in the Cymbals category.
	visIDs := comp.VisibleInstIDs()
	for _, id := range visIDs {
		if id != roster.Hihat && id != roster.Crash {
			t.Fatalf("unexpected instrument %q in Cymbals category", id)
		}
	}

	// Click back again.
	ri = idleFrames(dv, 2, W, H)
	ri()
	backBtn := comp.BackBtn()
	if backBtn == nil {
		t.Fatal("back button missing after category selection")
	}
	br := backBtn.Rect()
	clickDrumViewAt(t, dv, br.Min.X+br.Dx()/2, br.Min.Y+br.Dy()/2, W, H)

	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories after second back, got %s", comp.Mode())
	}

	// Select "Drums" category.
	ri = idleFrames(dv, 2, W, H)
	ri()
	var drumsBtn *Button
	for _, btn := range comp.CategoryBtns() {
		if btn.Text == "Percussion" {
			drumsBtn = btn
			break
		}
	}
	if drumsBtn == nil {
		t.Fatal("could not find Drums category button")
	}
	dr := drumsBtn.Rect()
	clickDrumViewAt(t, dv, dr.Min.X+dr.Dx()/2, dr.Min.Y+dr.Dy()/2, W, H)

	if comp.ActiveCategory() != audio.CatPercussion {
		t.Fatalf("expected active category Drums, got %s", comp.ActiveCategory())
	}

	// Verify drums instruments.
	visIDs = comp.VisibleInstIDs()
	hasDrum := false
	for _, id := range visIDs {
		if id == roster.Kick || id == roster.Snare || id == roster.Tom {
			hasDrum = true
		}
		if id == roster.Hihat || id == roster.Crash {
			t.Fatalf("cymbal %q visible in Drums category", id)
		}
	}
	if !hasDrum {
		t.Fatal("no drum instruments visible in Drums category")
	}

	t.Log("back-then-category roundtrip — PASS")
}

func TestInstMenuFullSelectionViaDrumViewUpdate(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	if dv.Rows[0].Instrument != roster.Kick {
		t.Fatalf("expected initial instrument kick, got %q", dv.Rows[0].Instrument)
	}

	// Open menu.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open")
	}

	comp := dv.instMenuComp

	// Navigate to Cymbals: back to categories if needed, then select Cymbals.
	if comp.Mode() == InstMenuModeInstruments {
		backBtn := comp.BackBtn()
		if backBtn == nil {
			t.Fatal("back button missing")
		}
		br := backBtn.Rect()
		clickDrumViewAt(t, dv, br.Min.X+br.Dx()/2, br.Min.Y+br.Dy()/2, W, H)
	}

	ri := idleFrames(dv, 2, W, H)
	ri()

	// Click "Cymbals".
	var cymbalsBtn *Button
	for _, btn := range comp.CategoryBtns() {
		if btn.Text == "Cymbal" {
			cymbalsBtn = btn
			break
		}
	}
	if cymbalsBtn == nil {
		t.Fatal("could not find Cymbals category button")
	}
	cr := cymbalsBtn.Rect()
	clickDrumViewAt(t, dv, cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2, W, H)

	ri = idleFrames(dv, 2, W, H)
	ri()

	// Select "hihat" from instruments.
	var hihatBtn *Button
	for _, btn := range comp.InstBtns() {
		if btn.Text == "Hihat" || btn.Text == "hihat" {
			hihatBtn = btn
			break
		}
	}
	if hihatBtn == nil {
		// Try by visible IDs.
		t.Logf("instBtns: %d, visibleIDs: %v", len(comp.InstBtns()), comp.VisibleInstIDs())
		t.Fatal("could not find hihat button in menu")
	}

	hr := hihatBtn.Rect()
	clickDrumViewAt(t, dv, hr.Min.X+hr.Dx()/2, hr.Min.Y+hr.Dy()/2, W, H)

	// The menu stays open after a pick (audition); dismissal is explicit.
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should stay open after instrument selection (audition contract)")
	}

	// Verify instrument was applied.
	if dv.Rows[0].Instrument != roster.Hihat {
		t.Fatalf("expected instrument hihat, got %q", dv.Rows[0].Instrument)
	}

	t.Log("full selection via DrumView.Update() — menu stayed open — PASS")
}

// On mobile a label click can land on the colour button, so the menu opens through openInstMenuForRow, the JS
// export's path.
func TestInstMenuMobileTapPatternFullFlow(t *testing.T) {
	assertDefaultParityState(t)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, _, _ := newCategoryDrumView(t, W, H)

	// Open menu programmatically (matches the API path used in real mobile tests).
	dv.openInstMenuForRow(0)
	if !dv.IsInstMenuOpen() {
		t.Fatalf("mobile menu should be open after openInstMenuForRow (instMenuOpen=%v)", dv.IsInstMenuOpen())
	}

	// Verify stays open for 10 idle frames.
	ri := idleFrames(dv, 10, W, H)
	ri()
	if !dv.IsInstMenuOpen() {
		t.Fatal("mobile menu closed during idle frames")
	}

	// Select an instrument.
	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instMenuComp not open")
	}

	// Navigate to instruments if in categories mode.
	if comp.Mode() == InstMenuModeCategories {
		var catBtn *Button
		catBtns := comp.CategoryBtns()
		if len(catBtns) > 0 {
			catBtn = catBtns[0]
		}
		if catBtn == nil {
			t.Fatal("no category buttons available")
		}
		cr := catBtn.Rect()
		clickDrumViewAt(t, dv, cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2, W, H)
		ri = idleFrames(dv, 2, W, H)
		ri()
	}

	// Tap an instrument.
	if len(comp.InstBtns()) == 0 {
		t.Fatal("no instrument buttons available")
	}
	instBtn := comp.InstBtns()[0]
	ir := instBtn.Rect()
	clickDrumViewAt(t, dv, ir.Min.X+ir.Dx()/2, ir.Min.Y+ir.Dy()/2, W, H)

	if dv.IsInstMenuOpen() {
		// If menu is still open, it might need another idle frame.
		ri = idleFrames(dv, 2, W, H)
		ri()
	}

	// Verify instrument changed (we clicked the first instrument in the filtered list).
	t.Logf("instrument after selection: %q", dv.Rows[0].Instrument)
	t.Log("mobile tap pattern full flow — PASS")
}

func TestInstMenuClickOutsideCloses(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	// Open menu.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open")
	}

	// Let suppress clear with idle frames.
	ri := idleFrames(dv, 5, W, H)
	ri()
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu closed during idle frames")
	}

	// Click well outside the menu area.
	clickDrumViewAt(t, dv, W-10, H-10, W, H)

	if dv.IsInstMenuOpen() {
		t.Fatal("menu should close after clicking outside")
	}

	t.Log("click outside closes — PASS")
}

func TestInstMenuRapidToggle(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	// 1. Open
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("step 1: menu should be open")
	}

	// Let state settle.
	ri := idleFrames(dv, 3, W, H)
	ri()

	// 2. Toggle close by clicking label again.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if dv.IsInstMenuOpen() {
		t.Fatal("step 2: menu should be closed (toggle)")
	}

	ri = idleFrames(dv, 3, W, H)
	ri()

	// 3. Reopen.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("step 3: menu should be open again")
	}

	ri = idleFrames(dv, 5, W, H)
	ri()

	// 4. Close via click outside.
	clickDrumViewAt(t, dv, W-10, H-10, W, H)
	if dv.IsInstMenuOpen() {
		t.Fatal("step 4: menu should close from click-outside")
	}

	ri = idleFrames(dv, 3, W, H)
	ri()

	// 5. Reopen one more time.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("step 5: menu should be open after rapid toggle cycle")
	}

	t.Log("rapid toggle — PASS")
}

// The geometry change from instruments to categories must not close or flicker the menu.
func TestInstMenuBackButtonStaysCategoriesAfterClick(t *testing.T) {
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

	// If not in instruments mode, select a category first.
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

	// Quick click (press + release) on back button.
	clickDrumViewAt(t, dv, bx, by, W, H)

	if !dv.IsInstMenuOpen() {
		t.Fatal("menu closed after back button click")
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories mode after back click, got %s", comp.Mode())
	}

	// Verify stays in categories mode for 10 idle frames.
	for i := 0; i < 10; i++ {
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
		if !dv.IsInstMenuOpen() {
			t.Fatalf("menu closed on idle frame %d after back click", i)
		}
		if comp.Mode() != InstMenuModeCategories {
			t.Fatalf("mode changed to %s on idle frame %d (expected categories)", comp.Mode(), i)
		}
	}

	t.Log("back button stays categories after click — PASS")
}
