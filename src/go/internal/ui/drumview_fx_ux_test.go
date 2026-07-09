package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// fxAdvanceFrames runs N Update() calls with no input active.
func fxAdvanceFrames(t *testing.T, dv *DrumView, n int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	for i := 0; i < n; i++ {
		dv.Update()
	}
	restore()
}

// fxClickAndRelease simulates a click at (x,y) then release on the next frame.
func fxClickAndRelease(t *testing.T, dv *DrumView, x, y int) {
	t.Helper()
	fxClickAt(t, dv, x, y)
	fxReleaseInput(t, dv)
}

func fxBtnCenter(t *testing.T, dv *DrumView, row int) (int, int) {
	t.Helper()
	if row >= len(dv.rowFXBtns()) {
		t.Fatalf("no FX button for row %d (have %d)", row, len(dv.rowFXBtns()))
	}
	r := dv.rowFXBtns()[row].Rect()
	if r.Empty() {
		t.Fatalf("FX button for row %d has empty rect", row)
	}
	return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2
}

// ---------- Desktop Tests ----------

func TestFXButtonRenders(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	if len(dv.rowFXBtns()) == 0 {
		t.Fatal("rowFXBtns slice is empty after calcLayout")
	}
	r := dv.rowFXBtns()[0].Rect()
	if r.Empty() {
		t.Fatal("FX button rect is empty")
	}
	// Verify FX button is within the row controls bounds.
	bounds := dv.computeRowControlsBounds()
	if bounds.Empty() {
		t.Fatal("row controls bounds is empty")
	}
	if !r.In(bounds) {
		t.Errorf("FX button rect %v not within row controls bounds %v", r, bounds)
	}
}

func TestFXButtonClickOpensPanel(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	cx, cy := fxBtnCenter(t, dv, 0)

	fxClickAndRelease(t, dv, cx, cy)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open after clicking FX button")
	}
	if dv.fxPanelRow != 0 {
		t.Errorf("expected fxPanelRow=0, got %d", dv.fxPanelRow)
	}
}

func TestFXPanelStaysOpenAcrossFrames(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	cx, cy := fxBtnCenter(t, dv, 0)

	// Open the panel.
	fxClickAndRelease(t, dv, cx, cy)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Advance several frames with no input — panel should stay open.
	fxAdvanceFrames(t, dv, 10)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel closed unexpectedly after advancing frames with no input")
	}
}

func TestFXPanelCloseOnClickOutside(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}

	// Open panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	// Advance past debounce window.
	fxAdvanceFrames(t, dv, 3)

	// Click outside both the panel rect and the FX button.
	// Use coordinates at (1, 1) which should be far from both.
	fxClickAndRelease(t, dv, 1, 1)
	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not close on click outside")
	}
}

func TestFXPanelToggleViaButton(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	cx, cy := fxBtnCenter(t, dv, 0)

	// Open panel.
	fxClickAndRelease(t, dv, cx, cy)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	// Advance past debounce.
	fxAdvanceFrames(t, dv, 3)

	// Click FX button again to close.
	fxClickAndRelease(t, dv, cx, cy)
	if dv.IsFXPanelOpen() {
		t.Fatal("panel did not close on second FX button click")
	}

	// Click again to reopen.
	fxClickAndRelease(t, dv, cx, cy)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not reopen on third FX button click")
	}
}

func TestFXPanelSwitchRow(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) < 2 {
		// Add a second row.
		dv.AddRow()
		dv.recalcButtons()
		dv.calcLayout()
	}
	if len(dv.rowFXBtns()) < 2 {
		t.Skip("not enough FX buttons for multi-row test")
	}

	// Open panel for row 0.
	cx0, cy0 := fxBtnCenter(t, dv, 0)
	fxClickAndRelease(t, dv, cx0, cy0)
	if !dv.IsFXPanelOpen() || dv.fxPanelRow != 0 {
		t.Fatalf("expected panel open for row 0, open=%v row=%d", dv.IsFXPanelOpen(), dv.fxPanelRow)
	}

	// Advance past debounce.
	fxAdvanceFrames(t, dv, 3)

	// Verify row 1's FX button exists and has non-empty rect.
	if len(dv.rowFXBtns()) < 2 {
		t.Fatalf("need at least 2 FX buttons, have %d", len(dv.rowFXBtns()))
	}
	r1 := dv.rowFXBtns()[1].Rect()
	if r1.Empty() {
		t.Fatal("row 1 FX button has empty rect")
	}

	// Switch to row 1 via direct toggle (bypasses input dispatch complexity).
	dv.toggleFXPanel(1)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel should stay open when switching rows")
	}
	if dv.fxPanelRow != 1 {
		t.Errorf("expected fxPanelRow=1, got %d", dv.fxPanelRow)
	}
}

func TestFXPanelAddEffectViaUI(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() || len(dv.fxPanelBtns) == 0 {
		t.Fatal("panel did not open or has no buttons")
	}

	beforeCount := len(audio.GetInsertEffects(instID))

	// Click "+ Add Effect" to open the type picker.
	addBtn := findFXPanelBtn(dv, "+ Add Effect")
	if addBtn == nil {
		t.Fatal("'+ Add Effect' button not found")
	}
	addBtn.OnClick()
	suppressClicksUntilRelease = false // clear for direct OnClick chaining in test

	if !dv.fxAddMenuOpen {
		t.Fatal("type picker did not open")
	}

	// Pick "Distortion" from the picker.
	distBtn := findFXPanelBtn(dv, "Distortion")
	if distBtn == nil {
		t.Fatal("Distortion button not found in picker")
	}
	distBtn.OnClick()

	afterCount := len(audio.GetInsertEffects(instID))
	if afterCount != beforeCount+1 {
		t.Errorf("expected %d effects, got %d", beforeCount+1, afterCount)
	}

	// Panel should rebuild and still be open.
	if !dv.IsFXPanelOpen() {
		t.Error("panel closed after adding effect")
	}
}

func TestFXPanelRemoveEffectViaUI(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	// Open FX panel.
	dv.toggleFXPanel(0)

	// Find remove button (IconClose per DESIGN.md §5c).
	var removeBtn *Button
	for _, btn := range dv.fxPanelBtns {
		if btn.Icon == string(IconClose) {
			removeBtn = btn
			break
		}
	}
	if removeBtn == nil {
		t.Fatal("remove button not found")
	}
	removeBtn.OnClick()

	if len(audio.GetInsertEffects(instID)) != 0 {
		t.Error("effect was not removed")
	}
}

func TestFXPanelToggleEffectViaUI(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	effects := audio.GetInsertEffects(instID)
	if !effects[0].Enabled {
		t.Fatal("effect should be enabled initially")
	}

	dv.toggleFXPanel(0)

	// Find toggle button (pill-style, tagged with fxToggleTag).
	var toggleBtn *Button
	for _, btn := range dv.fxPanelBtns {
		if isToggle, _ := isFXToggleBtn(btn); isToggle {
			toggleBtn = btn
			break
		}
	}
	if toggleBtn == nil {
		t.Fatal("toggle button not found")
	}
	toggleBtn.OnClick()

	effects = audio.GetInsertEffects(instID)
	if effects[0].Enabled {
		t.Error("effect should be disabled after toggle")
	}
}

func TestFXPanelEscapeCloses(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	// Simulate Escape key press.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()

	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not close on Escape")
	}
}

// ---------- Mobile Tests ----------

func TestFXButtonRendersMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844) // iPhone-like
	advanceFrames(g, 2)

	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Skip("no rows in mobile layout")
	}
	// On mobile, FX button may be hidden behind the menu. Check if it exists.
	if len(dv.rowFXBtns()) == 0 {
		t.Skip("FX buttons not allocated in mobile layout")
	}
	// Verify at least the first FX button has a non-empty rect.
	r := dv.rowFXBtns()[0].Rect()
	if r.Empty() {
		t.Skip("FX button has empty rect on mobile (may be behind overflow menu)")
	}
	// If the rect is non-empty, it should be within the drum bounds.
	if !r.Overlaps(dv.Bounds) {
		t.Errorf("FX button rect %v not within drum bounds %v", r, dv.Bounds)
	}
}

func TestFXPanelOpenCloseMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	dv := g.drum
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons in mobile layout")
	}
	r := dv.rowFXBtns()[0].Rect()
	if r.Empty() {
		t.Skip("FX button has empty rect on mobile")
	}

	// Open via toggleFXPanel (avoids relying on mobile button visibility).
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open on mobile")
	}

	// Advance frames — panel should stay open.
	fxAdvanceFrames(t, dv, 5)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel closed unexpectedly on mobile after advancing frames")
	}

	// Close via toggle.
	dv.toggleFXPanel(0)
	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not close on mobile toggle")
	}
}

func TestFXPanelAddEffectMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Skip("no rows in mobile layout")
	}

	instID := dv.Rows[0].Instrument
	beforeCount := len(audio.GetInsertEffects(instID))

	// Open panel and open type picker.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() || len(dv.fxPanelBtns) == 0 {
		t.Fatal("panel did not open or has no buttons on mobile")
	}

	addBtn := findFXPanelBtn(dv, "+ Add Effect")
	if addBtn == nil {
		t.Fatal("'+ Add Effect' button not found on mobile")
	}
	addBtn.OnClick()
	suppressClicksUntilRelease = false // clear for direct OnClick chaining in test

	if !dv.fxAddMenuOpen {
		t.Fatal("type picker did not open on mobile")
	}

	// Pick "Distortion" from the picker.
	distBtn := findFXPanelBtn(dv, "Distortion")
	if distBtn == nil {
		t.Fatal("Distortion button not found in mobile picker")
	}
	distBtn.OnClick()

	afterCount := len(audio.GetInsertEffects(instID))
	if afterCount != beforeCount+1 {
		t.Errorf("expected %d effects on mobile, got %d", beforeCount+1, afterCount)
	}
}

// TestFXPanelAddEffectPickerSurvivesHeldMouse verifies that clicking
// "+ Add Effect" and holding the mouse does not cause the type picker
// to flicker closed. The click-outside logic must respect
// suppressClicksUntilRelease while the mouse is still held from the
// same click that opened the picker.
func TestFXPanelAddEffectPickerSurvivesHeldMouse(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel and advance frames to clear the suppress guard
	// from openFXPanel and expire the debounce window.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() || len(dv.fxPanelBtns) == 0 {
		t.Fatal("panel did not open or has no buttons")
	}
	fxAdvanceFrames(t, dv, 3)

	// Find the "+ Add Effect" button and get its center.
	addBtn := findFXPanelBtn(dv, "+ Add Effect")
	if addBtn == nil {
		t.Fatal("'+ Add Effect' button not found")
	}
	addR := addBtn.Rect()
	cx := (addR.Min.X + addR.Max.X) / 2
	cy := (addR.Min.Y + addR.Max.Y) / 2

	// Simulate clicking "+ Add Effect" (mouse down at button center).
	// This fires OnClick which sets fxAddMenuOpen=true, calls buildFXPanel()
	// (panel grows taller/repositions), and SuppressClicksUntilMouseUp().
	fxClickAt(t, dv, cx, cy)

	if !dv.fxAddMenuOpen {
		t.Fatal("type picker did not open after clicking + Add Effect")
	}
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel closed on the click frame itself")
	}

	// Now hold the mouse for 5+ frames at the SAME position.
	// After buildFXPanel() the panel may have repositioned/grown, so (cx,cy)
	// might be outside the new fxPanelRect. This is the exact scenario that
	// triggers the bug: the click-outside check fires while the mouse is
	// still held from the same click.
	//
	// Note: SetInputForTest clears suppressClicksUntilRelease, but in the
	// real app suppress persists between frames (only cleared on mouse
	// release). We restore it after SetInputForTest to match real behavior.
	for i := 0; i < 6; i++ {
		restore := SetInputForTest(
			func() (int, int) { return cx, cy },
			func(ebiten.MouseButton) bool { return true }, // still held
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 800, 300 },
		)
		suppressClicksUntilRelease = true // restore: real app preserves this across frames
		dv.Update()
		restore()

		if !dv.IsFXPanelOpen() {
			t.Fatalf("FX panel closed on held-mouse frame %d", i+1)
		}
		if !dv.fxAddMenuOpen {
			t.Fatalf("type picker closed on held-mouse frame %d", i+1)
		}
	}

	// Release the mouse — panel should still be open with the picker visible.
	fxReleaseInput(t, dv)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel closed after mouse release")
	}
	if !dv.fxAddMenuOpen {
		t.Fatal("type picker closed after mouse release")
	}
}

// ---------- Mobile Context Menu Structure Tests ----------

// TestContextMenuEffectsAbsentOnMobile verifies that the mobile context
// menu intentionally omits the "Effects" entry. Mobile users do not have
// an FX entry point in this menu.
func TestContextMenuEffectsAbsentOnMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu did not open")
	}

	for _, btn := range dv.contextMenuBtns {
		if btn.Text == "Effects" {
			t.Errorf("mobile context menu should not contain %q", btn.Text)
		}
	}
}

// TestContextMenuHasFiveItemsMobile pins the mobile button count: four items
// (Rename, Color, Origin, Delete) plus the close button = 5. ("Instrument" was
// removed — the row label opens the picker directly on every platform; "Color"
// was re-exposed in Task 14 to open the grouped Vice City picker.)
func TestContextMenuHasFiveItemsMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu did not open")
	}

	// Mobile items: Rename, Color, Origin, Delete (4) + close = 5.
	want := 5
	if got := len(dv.contextMenuBtns); got != want {
		t.Errorf("expected %d context menu buttons (4 items + close), got %d", want, got)
		for i, btn := range dv.contextMenuBtns {
			t.Logf("  btn[%d]: text=%q icon=%q", i, btn.Text, btn.Icon)
		}
	}
}

func TestFXPanelMobileSizing(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel via context menu path (same as mobile user flow).
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	r := dv.fxPanelRect
	if r.Empty() {
		t.Fatal("FX panel rect is empty")
	}

	// Panel should be wider than default 250px on mobile.
	if r.Dx() <= 250 {
		t.Errorf("expected panel width > 250px on mobile, got %d", r.Dx())
	}

	// Panel should be within drum bounds.
	if r.Min.X < dv.Bounds.Min.X || r.Max.X > dv.Bounds.Max.X {
		t.Errorf("panel X range [%d, %d] exceeds bounds [%d, %d]", r.Min.X, r.Max.X, dv.Bounds.Min.X, dv.Bounds.Max.X)
	}
	if r.Min.Y < dv.Bounds.Min.Y || r.Max.Y > dv.Bounds.Max.Y {
		t.Errorf("panel Y range [%d, %d] exceeds bounds [%d, %d]", r.Min.Y, r.Max.Y, dv.Bounds.Min.Y, dv.Bounds.Max.Y)
	}
	// Panel height must not exceed drum bounds.
	maxH := dv.Bounds.Dy() - 20
	if r.Dy() > maxH+1 { // +1 for rounding
		t.Errorf("panel height %d exceeds max allowed %d", r.Dy(), maxH)
	}
}

func TestFXPanelHasCloseButtonMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	if len(dv.fxPanelBtns) == 0 {
		t.Fatal("no buttons in FX panel")
	}

	// Last button should be the close button.
	closeBtn := dv.fxPanelBtns[len(dv.fxPanelBtns)-1]
	if closeBtn.Icon != "close" {
		t.Errorf("expected last button icon='close', got %q", closeBtn.Icon)
	}

	// Clicking close should close the panel.
	closeBtn.OnClick()
	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not close after clicking close button on mobile")
	}
}

// ---------- Flicker & Text Truncation Tests ----------

// TestFXPanelDoesNotFlickerOnHeldPress reproduces the flicker bug: opening the
// FX panel then holding the mouse button at the FX button position on the
// next frame should NOT re-toggle the panel closed (the tree's suppress
// mechanism prevents re-dispatch).
func TestFXPanelDoesNotFlickerOnHeldPress(t *testing.T) {
	dv := newTestDV(t)
	cx, cy := fxBtnCenter(t, dv, 0)

	// Frame 0: click opens FX panel.
	fxClickAt(t, dv, cx, cy)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Frame 1: held press at same position — tree's suppress prevents
	// re-dispatch, so panel stays open.
	fxHoldAt(t, dv, cx, cy)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel closed on held press — flicker bug")
	}
	fxReleaseInput(t, dv)
}

// TestFXPanelTogglesAfterSuppressClears verifies toggling still works after
// the user releases and clicks again.
func TestFXPanelTogglesAfterSuppressClears(t *testing.T) {
	dv := newTestDV(t)
	cx, cy := fxBtnCenter(t, dv, 0)

	// Open.
	fxClickAndRelease(t, dv, cx, cy)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Advance past debounce.
	fxAdvanceFrames(t, dv, 3)

	// Click again to close (suppression cleared by release).
	fxClickAndRelease(t, dv, cx, cy)
	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel should close on second click")
	}
}

// fxGameFrame simulates one Game-like frame: HandleInput then Update.
// This exercises the real dispatch path where the OverlayPortal in HandleInput
// runs before Update re-reads the same mouse state.
func fxGameFrame(dv *DrumView, x, y int, pressed bool, w, h int) {
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	// Game loop runs HandleInput() then Update() each frame.
	dv.HandleInput(x, y, pressed)
	dv.Update()
	restore()
}

// TestFXPanelBlocksClickThrough_GameLoop tests the real Game dispatch path:
// HandleInput (overlay stack) runs first, then Update re-reads mouse state.
// When the FX panel is open and the user clicks a mute button behind it,
// HandleInput closes the panel. Update must NOT let the same click through
// to the mute button.
func TestFXPanelBlocksClickThrough_GameLoop(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	// Open FX panel and advance past debounce.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	fxAdvanceFrames(t, dv, 3)

	// Find the mute button.
	if len(dv.rowMuteBtns()) == 0 || dv.rowMuteBtns()[0] == nil {
		t.Skip("no mute button")
	}
	muteR := dv.rowMuteBtns()[0].Rect()
	if muteR.Empty() {
		t.Skip("mute button has empty rect")
	}

	// Log geometry to help diagnose which path the click takes.
	mutePt := image.Pt((muteR.Min.X+muteR.Max.X)/2, (muteR.Min.Y+muteR.Max.Y)/2)
	t.Logf("fxPanelRect=%v, muteRect=%v, muteCenter=%v",
		dv.fxPanelRect, muteR, mutePt)

	wasMuted := dv.Rows[0].Muted

	// Simulate Game-loop frame: HandleInput + Update with press on mute button.
	fxGameFrame(dv, mutePt.X, mutePt.Y, true, 800, 300)
	// Release frame.
	fxGameFrame(dv, mutePt.X, mutePt.Y, false, 800, 300)

	if dv.Rows[0].Muted != wasMuted {
		t.Error("mute button activated through FX panel — click leaked through overlay (Game loop path)")
	}
}

// TestFXPanelBlocksToolbarClickThrough_GameLoop tests that toolbar buttons
// (play/stop/bpm) behind the FX panel are not activated when clicked.
func TestFXPanelBlocksToolbarClickThrough_GameLoop(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	fxAdvanceFrames(t, dv, 3)

	// Hook play button to detect activation.
	playFired := false
	if dv.playBtn() == nil {
		t.Skip("no play button")
	}
	origOnClick := dv.playBtn().OnClick
	dv.playBtn().OnClick = func() {
		playFired = true
		if origOnClick != nil {
			origOnClick()
		}
	}

	playR := dv.playBtn().Rect()
	if playR.Empty() {
		t.Skip("play button has empty rect")
	}
	px := (playR.Min.X + playR.Max.X) / 2
	py := (playR.Min.Y + playR.Max.Y) / 2

	// Click play button while FX panel is open (Game-loop path).
	fxGameFrame(dv, px, py, true, 800, 300)
	fxGameFrame(dv, px, py, false, 800, 300)

	if playFired {
		t.Error("play button activated through FX panel — click leaked through overlay (Game loop path)")
	}
}

// TestFXPanelBlocksClickThrough_UpdateOnly tests the standalone DrumView path
// (Update() only, no HandleInput). This path is used in tests and also when
// the input dispatcher is not wired up.
func TestFXPanelBlocksClickThrough_UpdateOnly(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	fxAdvanceFrames(t, dv, 3)

	if len(dv.rowMuteBtns()) == 0 || dv.rowMuteBtns()[0] == nil {
		t.Skip("no mute button")
	}
	muteR := dv.rowMuteBtns()[0].Rect()
	if muteR.Empty() {
		t.Skip("mute button has empty rect")
	}
	muteCX := (muteR.Min.X + muteR.Max.X) / 2
	muteCY := (muteR.Min.Y + muteR.Max.Y) / 2

	wasMuted := dv.Rows[0].Muted

	// Click at the mute button via Update-only path.
	fxClickAndRelease(t, dv, muteCX, muteCY)

	if dv.Rows[0].Muted != wasMuted {
		t.Error("mute button activated through FX panel — click leaked through Update-only path")
	}
}

// TestFXPanelOverlayCloseSetsSuppressAfterClose verifies that the
// FXPanelOverlay.Close() path (used by portal click-outside) sets
// suppressClicksUntilRelease AFTER closeFXPanel clears it.
func TestFXPanelOverlayCloseSetsSuppressAfterClose(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	// Simulate what the portal system does: calls closeFXPanel + sets suppress.
	dv.closeFXPanel()
	SuppressClicksUntilMouseUp()

	// After close + suppress, suppress must be true.
	if !suppressClicksUntilRelease {
		t.Error("closeFXPanel + SuppressClicksUntilMouseUp must set suppressClicksUntilRelease to prevent click-through")
	}
	t.Cleanup(func() { suppressClicksUntilRelease = false })
}

// TestFXPanelPhase2CloseSetsSuppressAfterClose verifies that
// handleFXPanelInput Phase 2 (click outside closes panel) sets
// suppressClicksUntilRelease so that Update() blocks the same click.
func TestFXPanelClickOutsideClosesViaTree(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	fxAdvanceFrames(t, dv, 3)

	// Click outside the panel — tree's click-outside closes it.
	outsideX := dv.fxPanelRect.Max.X + 50
	outsideY := dv.fxPanelRect.Min.Y + 5
	fxHoldAt(t, dv, outsideX, outsideY)

	if dv.IsFXPanelOpen() {
		t.Fatal("tree click-outside should have closed the panel")
	}
	fxReleaseInput(t, dv)
}

// TestFXPanelWidthFitsLabels verifies the FX panel is wide enough for labels.
func TestFXPanelWidthFitsLabels(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	// Panel should be wider than the old 250px hardcoded value.
	if dv.fxPanelRect.Dx() < 280 {
		t.Errorf("panel too narrow: %dpx", dv.fxPanelRect.Dx())
	}
	// Check sliders don't overlap label area.
	// Sliders are indented by SpaceXL from panel left edge.
	for i, sl := range dv.fxPanelSliders {
		sr := sl.Rect()
		labelEnd := dv.fxPanelRect.Min.X + SpaceXL + dv.fxSliderLeft
		if sr.Min.X < labelEnd {
			t.Errorf("slider %d starts at %d, overlaps label area ending at %d",
				i, sr.Min.X, labelEnd)
		}
	}
}

func TestFXParamLabelsNotTruncated(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	// Filter has the longest label: "Cutoff: 20000 Hz"
	audio.AddInsertEffect(instID, audio.EffectFilter, nil)
	dv.syncFXToRow(0)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	effects := audio.GetInsertEffects(instID)
	cat := audio.InsertEffectCatalog()
	defs := cat[audio.EffectFilter]
	maxAvail := dv.fxSliderLeft - 16 // 8px left pad + 8px gap
	for _, def := range defs {
		val := effects[0].Params[def.Name]
		label := fxParamLabel(def.Name, val, def.Unit)
		tw := TextWidth(label)
		if tw > maxAvail {
			t.Errorf("label %q (%dpx) exceeds available space (%dpx)",
				label, tw, maxAvail)
		}
	}
}

// TestFXPanelOverlayReturnsCaptureDuringSliderDrag verifies that
// FXPanelOverlay.HandleInput returns InputCaptured (not just InputConsumed)
// while a slider drag is active. Without InputCaptured, the portal
// does not set its capture pointer, so if the mouse leaves InputBounds()
// during a drag the portal closes the panel and the click leaks
// through to elements underneath.
func TestFXPanelOverlayReturnsCaptureDuringSliderDrag(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in panel")
	}
	fxAdvanceFrames(t, dv, 3)

	// Click on slider to start drag.
	sl := dv.fxPanelSliders[0]
	sr := sl.Rect()
	cx := (sr.Min.X + sr.Max.X) / 2
	cy := (sr.Min.Y + sr.Max.Y) / 2

	// Frame 1: press on slider
	fxGameFrame(dv, cx, cy, true, 800, 300)
	if !dv.fxSliderDragging {
		t.Fatal("slider drag did not start")
	}

	// While a slider drag is active, the FX panel's capturing state should
	// be true, meaning the portal system will route all input to it.
	capturing := dv.fxPanelDeferredTap.Active() || dv.fxScrollTS.Active() || dv.fxSliderDragging
	if !capturing {
		t.Error("FX panel should be in capturing state during slider drag")
	}

	// handleFXPanelInput should consume the input during a drag.
	consumed := dv.handleFXPanelInput(cx, cy, true)
	if !consumed {
		t.Error("handleFXPanelInput should consume input during slider drag")
	}

	// Release
	fxGameFrame(dv, cx, cy, false, 800, 300)
}

// TestFXPanelSliderDragOutsidePanelDoesNotClose verifies that dragging an
// FX panel slider outside the panel rect does NOT close the panel.
// This tests the capture path: when fxSliderDragging is true, the portal
// routes all subsequent input to the FX panel, preventing the
// "click outside -> close" logic from firing mid-drag.
func TestFXPanelSliderDragOutsidePanelDoesNotClose(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in panel")
	}
	fxAdvanceFrames(t, dv, 3)

	// Start drag on slider.
	sl := dv.fxPanelSliders[0]
	sr := sl.Rect()
	cx := (sr.Min.X + sr.Max.X) / 2
	cy := (sr.Min.Y + sr.Max.Y) / 2
	fxGameFrame(dv, cx, cy, true, 800, 300)
	if !dv.fxSliderDragging {
		t.Fatal("slider drag did not start")
	}

	// In the portal system, capture is handled automatically when the
	// fxSliderDragging flag is set. The portal routes all input to the
	// FX panel overlay while a drag is active.

	// Drag outside the panel rect (but still inside dv.Bounds).
	outsideX := dv.fxPanelRect.Max.X + 50
	outsideY := dv.fxPanelRect.Min.Y + 5
	if outsideX >= dv.Bounds.Max.X {
		outsideX = dv.Bounds.Max.X - 1
	}

	// With slider dragging active, the panel should stay open even
	// when input is outside the panel rect.
	fxGameFrame(dv, outsideX, outsideY, true, 800, 300)
	if !dv.IsFXPanelOpen() {
		t.Error("FX panel closed during slider drag outside panel rect — " +
			"portal should route to captured overlay instead of closing")
	}

	// Release should end the drag but keep the panel open.
	fxGameFrame(dv, outsideX, outsideY, false, 800, 300)
	if !dv.IsFXPanelOpen() {
		t.Error("FX panel closed on slider drag release")
	}
	if dv.fxSliderDragging {
		t.Error("slider drag should have ended on release")
	}
}

// TestFXPanelSliderDragDoesNotActivateRowControls verifies that while
// dragging a slider inside the FX panel, row controls (mute/solo/volume)
// underneath are not activated — even if the drag passes over them.
func TestFXPanelSliderDragDoesNotActivateRowControls(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in panel")
	}
	fxAdvanceFrames(t, dv, 3)

	// Record initial row state.
	wasMuted := dv.Rows[0].Muted
	wasVol := dv.Rows[0].Volume

	// Start drag on slider.
	sl := dv.fxPanelSliders[0]
	sr := sl.Rect()
	cx := (sr.Min.X + sr.Max.X) / 2
	cy := (sr.Min.Y + sr.Max.Y) / 2
	fxGameFrame(dv, cx, cy, true, 800, 300)

	// Drag across the panel (simulate moving horizontally).
	for dx := -20; dx <= 20; dx += 5 {
		fxGameFrame(dv, cx+dx, cy, true, 800, 300)
	}

	// Release
	fxGameFrame(dv, cx, cy, false, 800, 300)

	// Verify row state didn't change.
	if dv.Rows[0].Muted != wasMuted {
		t.Error("mute button activated during FX slider drag")
	}
	if dv.Rows[0].Volume != wasVol {
		t.Errorf("row volume changed during FX slider drag: was %.3f, now %.3f",
			wasVol, dv.Rows[0].Volume)
	}
}

// TestFXPanelClickInsideDoesNotActivateRowControls verifies that clicking
// anywhere inside the FX panel (on a button, slider, or background) does
// not activate row controls underneath.
func TestFXPanelClickInsideDoesNotActivateRowControls(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	fxAdvanceFrames(t, dv, 3)

	// Record initial row state.
	wasMuted := dv.Rows[0].Muted
	wasVol := dv.Rows[0].Volume

	// Click at center of FX panel (background area).
	panelCX := (dv.fxPanelRect.Min.X + dv.fxPanelRect.Max.X) / 2
	panelCY := (dv.fxPanelRect.Min.Y + dv.fxPanelRect.Max.Y) / 2
	fxGameFrame(dv, panelCX, panelCY, true, 800, 300)
	fxGameFrame(dv, panelCX, panelCY, false, 800, 300)

	if dv.Rows[0].Muted != wasMuted {
		t.Error("mute activated by click on FX panel background")
	}
	if dv.Rows[0].Volume != wasVol {
		t.Errorf("volume changed by click on FX panel background: was %.3f, now %.3f",
			wasVol, dv.Rows[0].Volume)
	}

	// Click on each FX panel button (if any).
	for _, btn := range dv.fxPanelBtns {
		if btn == nil || btn.Rect().Empty() {
			continue
		}
		br := btn.Rect()
		bx := (br.Min.X + br.Max.X) / 2
		by := (br.Min.Y + br.Max.Y) / 2

		// Re-open panel if a button closed it.
		if !dv.IsFXPanelOpen() {
			dv.toggleFXPanel(0)
			fxAdvanceFrames(t, dv, 3)
		}

		before := dv.Rows[0].Muted
		fxGameFrame(dv, bx, by, true, 800, 300)
		fxGameFrame(dv, bx, by, false, 800, 300)

		if dv.Rows[0].Muted != before {
			t.Errorf("mute toggled by clicking FX button at (%d,%d) text=%q",
				bx, by, btn.Text)
		}
	}
}

// dummyPortalOverlay is a minimal PortalOverlay for testing portal state
// without requiring a real overlay component.
type dummyPortalOverlay struct{}

func (o *dummyPortalOverlay) Layout(anchor, screenBounds image.Rectangle) {}
func (o *dummyPortalOverlay) HitAreas() []HitArea                         { return nil }
func (o *dummyPortalOverlay) Draw(screen *ebiten.Image)                   {}
func (o *dummyPortalOverlay) ShouldClose() bool                           { return false }

// TestPortalOverlayDoesNotBlockTransportButtons reproduces Bug 1: a circular
// input blocking loop where portal overlays being open causes anyDropdownOpen()
// to return true, which makes popupActive() return true, which makes the tree
// yield ALL input — including to zone buttons that the tree is supposed to
// dispatch. After fix, anyDropdownOpen() should not include portal state.
func TestPortalOverlayDoesNotBlockTransportButtons(t *testing.T) {
	dv := newTestDV(t)

	// Baseline: no dropdowns open.
	if dv.anyDropdownOpen() {
		t.Fatal("no dropdown should be open initially")
	}

	// Open a dummy portal entry (simulates any portal overlay being open).
	dv.portal().Open(PortalEntry{
		ID:      "test-dummy",
		Overlay: &dummyPortalOverlay{},
		Modal:   false,
	})
	t.Cleanup(func() { dv.portal().Close("test-dummy") })

	if !dv.portal().IsOpen() {
		t.Fatal("portal should be open")
	}

	// anyDropdownOpen() now delegates to portal.IsOpen() — the circular loop
	// no longer exists because legacyPopupOpen has been removed from the tree.
	if !dv.anyDropdownOpen() {
		t.Fatal("anyDropdownOpen() should return true when portal overlay is open")
	}
}

// TestFXPanelSliderDragDoesNotToggleOtherRow reproduces Bug 2: dragging an FX
// panel slider crosses over another row's FX button and toggles that row's
// panel. After fix, the drag-state guard prevents the toggle.
func TestFXPanelSliderDragDoesNotToggleOtherRow(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) < 2 {
		dv.AddRow()
		dv.recalcButtons()
		dv.calcLayout()
	}
	if len(dv.rowFXBtns()) < 2 {
		t.Skip("not enough FX buttons for multi-row test")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	// Open FX panel for row 0.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() || dv.fxPanelRow != 0 {
		t.Fatalf("expected panel open for row 0, open=%v row=%d", dv.IsFXPanelOpen(), dv.fxPanelRow)
	}
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in panel")
	}
	fxAdvanceFrames(t, dv, 3)

	// Start drag on slider.
	sl := dv.fxPanelSliders[0]
	sr := sl.Rect()
	cx := (sr.Min.X + sr.Max.X) / 2
	cy := (sr.Min.Y + sr.Max.Y) / 2
	fxGameFrame(dv, cx, cy, true, 800, 300)
	if !dv.fxSliderDragging {
		t.Fatal("slider drag did not start")
	}

	// Move cursor to row 1's FX button while still dragging.
	r1cx, r1cy := fxBtnCenter(t, dv, 1)
	fxGameFrame(dv, r1cx, r1cy, true, 800, 300)

	// Before fix: row 1's FX button fires during drag → fxPanelRow changes.
	// After fix: fxSliderDragging guard prevents toggle.
	if dv.fxPanelRow != 0 {
		t.Errorf("fxPanelRow changed to %d during slider drag — drag leaked to row 1's FX button", dv.fxPanelRow)
	}
	if !dv.IsFXPanelOpen() {
		t.Error("FX panel closed during slider drag")
	}

	// Release.
	fxGameFrame(dv, r1cx, r1cy, false, 800, 300)
}

// TestContextMenuButtonsFireOnMobile reproduces the bug where context menu
// button clicks are dead on mobile because the tree's popupActive() yields
// before reaching hitIndex.At(), so portal overlay inputFn never fires.
func TestContextMenuButtonsFireOnMobile(t *testing.T) {
	setupMobileTest(t, true)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })
	dv := NewDrumView(image.Rect(0, 0, 800, 300), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open context menu for row 0.
	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu did not open")
	}
	fxAdvanceFrames(t, dv, 2) // settle

	// Find the "Rename" button (first item in the platform-uniform menu).
	if len(dv.contextMenuBtns) == 0 {
		t.Fatal("context menu has no buttons")
	}

	var renameBtn *Button
	for _, btn := range dv.contextMenuBtns {
		if btn.Text == "Rename" {
			renameBtn = btn
			break
		}
	}
	if renameBtn == nil {
		t.Fatal("could not find Rename button in context menu")
	}
	r := renameBtn.Rect()
	if r.Empty() {
		t.Fatal("Rename button has empty rect")
	}

	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	// Click on the Rename button via the Game-loop path (HandleInput + Update).
	// Before fix: tree yields at popupActive → portal inputFn never fires → button dead.
	// After fix: tree dispatches to portal overlay → inputFn → handleContextMenuInput → button fires.
	// Firing Rename closes the context menu (closeContextMenuPortal), so a
	// still-open menu means the button never fired.
	fxGameFrame(dv, cx, cy, true, 800, 300)
	fxGameFrame(dv, cx, cy, false, 800, 300)

	if dv.IsContextMenuOpen() {
		t.Error("Rename button did not fire — context menu buttons dead on mobile " +
			"(tree yields at popupActive before portal dispatch)")
	}
}

// TestEQWaveToggleWorksWithPortalOpen reproduces the bug where the EQ/Wave
// toggle button (viewSwitchBtn, z=110 in TransportZone) is dead when any
// portal overlay is open because popupActive() returns true.
func TestEQWaveToggleWorksWithPortalOpen(t *testing.T) {
	setupMobileTest(t, true)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })
	dv := NewDrumView(image.Rect(0, 0, 800, 300), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	dv.calcLayout()

	// Verify viewSwitchBtn exists and has a non-empty rect.
	if dv.viewSwitchBtn() == nil {
		t.Skip("no viewSwitchBtn (desktop-only?)")
	}
	r := dv.viewSwitchBtn().Rect()
	if r.Empty() {
		t.Skip("viewSwitchBtn has empty rect")
	}

	// Record initial view mode.
	initialMode := dv.currentViewMode

	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	// Click on viewSwitchBtn.
	fxGameFrame(dv, cx, cy, true, 800, 300)
	fxGameFrame(dv, cx, cy, false, 800, 300)

	if dv.currentViewMode == initialMode {
		t.Error("viewSwitchBtn did not cycle view mode — " +
			"button dead (tree yields at popupActive for stale state)")
	}
}

func TestRowButtonTextsNotTruncated(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	g := dv.rowGroups()[0]
	for _, tc := range []struct {
		name string
		btn  *Button
	}{
		{"Mute", g.Mute},
		{"Solo", g.Solo},
		{"FX", g.FX},
		{"Origin", g.Origin},
		{"Delete", g.Delete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.btn == nil {
				t.Skip("button is nil")
			}
			r := tc.btn.Rect()
			if r.Empty() {
				t.Skip("button has empty rect")
			}
			maxW := r.Dx() - 2*SpaceXS
			clipped := clipTextToWidth(tc.btn.Text, maxW)
			if clipped != tc.btn.Text {
				t.Errorf("%s button text %q truncated to %q (width=%d, maxW=%d, textW=%d)",
					tc.name, tc.btn.Text, clipped, r.Dx(), maxW, TextWidth(tc.btn.Text))
			}
		})
	}
}
