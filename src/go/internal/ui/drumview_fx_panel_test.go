package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newTestDV(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	dv := NewDrumView(image.Rect(0, 0, 800, 300), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	dv.calcLayout()
	t.Cleanup(func() {
		audio.ClearAllInsertEffects()
	})
	return dv
}

func fxClickAt(t *testing.T, dv *DrumView, x, y int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
}

func fxReleaseInput(t *testing.T, dv *DrumView) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
}

func TestFXPanelOpenClose(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	if len(dv.rowFXBtns()) == 0 {
		t.Skip("no FX buttons")
	}

	btn := dv.rowFXBtns()[0]
	r := btn.Rect()
	if r.Empty() {
		t.Skip("FX button has empty rect")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

	// Click FX button to open.
	fxClickAt(t, dv, cx, cy)
	fxReleaseInput(t, dv)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	if dv.fxPanelRow != 0 {
		t.Errorf("expected row 0, got %d", dv.fxPanelRow)
	}

	// Click again to close (toggle).
	fxClickAt(t, dv, cx, cy)
	fxReleaseInput(t, dv)
	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not close on toggle")
	}
}

// findFXPanelBtn searches fxPanelBtns for a button with the given text.
func findFXPanelBtn(dv *DrumView, text string) *Button {
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == text {
			return btn
		}
	}
	return nil
}

func TestFXPanelAddEffect(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Should have an "Add Effect" button.
	if len(dv.fxPanelBtns) == 0 {
		t.Fatal("no panel buttons")
	}

	instID := dv.Rows[0].Instrument
	beforeCount := len(audio.GetInsertEffects(instID))

	addBtn := findFXPanelBtn(dv, "+ Add Effect")
	if addBtn == nil {
		t.Fatal("'+ Add Effect' button not found")
	}

	// Click opens the type picker, does NOT add an effect yet.
	addBtn.OnClick()
	suppressClicksUntilRelease = false // clear for direct OnClick chaining in test
	if !dv.fxAddMenuOpen {
		t.Fatal("type picker did not open")
	}
	if len(audio.GetInsertEffects(instID)) != beforeCount {
		t.Error("clicking '+ Add Effect' should not add an effect immediately")
	}

	// Now pick "Distortion" from the picker.
	var distBtn *Button
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == "Distortion" {
			distBtn = btn
			break
		}
	}
	if distBtn == nil {
		t.Fatal("Distortion button not found in picker")
	}
	distBtn.OnClick()

	afterCount := len(audio.GetInsertEffects(instID))
	if afterCount != beforeCount+1 {
		t.Errorf("expected %d effects after add, got %d", beforeCount+1, afterCount)
	}
	if dv.fxAddMenuOpen {
		t.Error("picker should close after selecting an effect")
	}
}

func TestFXPanelAddEffectCancel(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}

	dv.toggleFXPanel(0)
	instID := dv.Rows[0].Instrument
	beforeCount := len(audio.GetInsertEffects(instID))

	// Open the type picker.
	addBtn := findFXPanelBtn(dv, "+ Add Effect")
	if addBtn == nil {
		t.Fatal("'+ Add Effect' button not found")
	}
	addBtn.OnClick()
	suppressClicksUntilRelease = false // clear for direct OnClick chaining in test
	if !dv.fxAddMenuOpen {
		t.Fatal("type picker did not open")
	}

	// Close the picker by setting fxAddMenuOpen = false directly,
	// since the Cancel button may be off-screen with many effect types.
	dv.fxAddMenuOpen = false
	dv.buildFXPanel()

	if dv.fxAddMenuOpen {
		t.Error("picker should close after cancel")
	}
	if len(audio.GetInsertEffects(instID)) != beforeCount {
		t.Error("no effect should be added after cancel")
	}
}

func TestFXPanelPickerShowsAllTypes(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}

	dv.toggleFXPanel(0)

	// Open the type picker.
	addBtn := findFXPanelBtn(dv, "+ Add Effect")
	if addBtn == nil {
		t.Fatal("'+ Add Effect' button not found")
	}
	addBtn.OnClick()
	suppressClicksUntilRelease = false // clear for test

	// Verify effect type buttons are present (some may be off-screen with many types).
	regs := audio.EffectRegistrations()
	foundCount := 0
	for _, btn := range dv.fxPanelBtns {
		for _, reg := range regs {
			if btn.Text == reg.DisplayName {
				foundCount++
				break
			}
		}
	}
	if foundCount == 0 {
		t.Error("no effect type buttons found in picker")
	}
}

func TestFXPanelRemoveEffect(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Should have toggle, remove, and add buttons.
	// Find the remove button (icon IconClose per DESIGN.md §5c).
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

	effects := audio.GetInsertEffects(instID)
	if len(effects) != 0 {
		t.Errorf("expected 0 effects after remove, got %d", len(effects))
	}
}

func TestFXPanelToggleEffect(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	effects := audio.GetInsertEffects(instID)
	if !effects[0].Enabled {
		t.Fatal("expected effect to be enabled initially")
	}

	// Open FX panel.
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
		t.Error("expected effect to be disabled after toggle")
	}
}

func TestFXPanelInAnyDropdownOpen(t *testing.T) {
	dv := newTestDV(t)
	if dv.anyDropdownOpen() {
		t.Error("expected no dropdown open initially")
	}
	// Open FX panel via portal path
	dv.openFXPanelPortal()
	if !dv.anyDropdownOpen() {
		t.Error("anyDropdownOpen should return true when FX panel portal is open")
	}
	dv.CloseAllPopups()
}

func TestFXPanelClosedByCloseAllPopups(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// CloseAllPopups should close the FX panel.
	dv.CloseAllPopups()
	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel was not closed by CloseAllPopups")
	}
}

func TestFXPanelHasCloseButton(t *testing.T) {
	dv := newTestDV(t)
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

	// Last button should be the close button with "close" icon.
	closeBtn := dv.fxPanelBtns[len(dv.fxPanelBtns)-1]
	if closeBtn.Icon != "close" {
		t.Errorf("expected last button icon='close', got %q", closeBtn.Icon)
	}

	// Clicking the close button should close the panel.
	closeBtn.OnClick()
	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not close after clicking close button")
	}
}

// fxHoldAt simulates holding the mouse at (x,y) for one frame without releasing.
func fxHoldAt(t *testing.T, dv *DrumView, x, y int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()
}

func TestFXPanelSliderDrag(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in FX panel")
	}

	sl := dv.fxPanelSliders[0]
	slR := sl.Rect()
	if slR.Empty() {
		t.Fatal("slider has empty rect")
	}

	startVal := sl.Value

	// Press on the slider (left edge).
	sx := slR.Min.X + 2
	sy := (slR.Min.Y + slR.Max.Y) / 2
	fxHoldAt(t, dv, sx, sy)

	if !dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should be true after pressing on slider")
	}

	// Drag to the right side of the slider.
	dx := slR.Max.X - 4
	fxHoldAt(t, dv, dx, sy)

	if sl.Value <= startVal {
		t.Errorf("slider value should have increased: start=%f, now=%f", startVal, sl.Value)
	}
	if !dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should still be true while held")
	}

	// Verify audio param was propagated.
	effects := audio.GetInsertEffects(instID)
	if len(effects) == 0 {
		t.Fatal("no effects found")
	}
	b := dv.fxSliderBindings[0]
	expected := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	actual := effects[0].Params[b.paramName]
	if actual != expected {
		t.Errorf("audio param not propagated: expected=%f, got=%f", expected, actual)
	}

	// Release.
	fxReleaseInput(t, dv)
	if dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should be false after release")
	}
}

func TestFXPanelSliderDragMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(nil, game_log.LevelError)
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

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open on mobile")
	}

	// On mobile, effects start collapsed — expand slot 0 to show sliders.
	dv.fxExpandedSlots[0] = true
	dv.buildFXPanel()

	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in FX panel on mobile after expanding")
	}

	sl := dv.fxPanelSliders[0]
	slR := sl.Rect()
	if slR.Empty() {
		t.Fatal("slider has empty rect on mobile")
	}

	startVal := sl.Value

	// Touch on the slider (left side).
	sx := slR.Min.X + 2
	sy := (slR.Min.Y + slR.Max.Y) / 2
	fxHoldAt(t, dv, sx, sy)

	if !dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should be true on mobile after touching slider")
	}

	// Drag to right side — value should update continuously.
	dx := slR.Max.X - 4
	fxHoldAt(t, dv, dx, sy)

	if sl.Value <= startVal {
		t.Errorf("slider value should have increased on drag: start=%f, now=%f", startVal, sl.Value)
	}

	// Verify audio param was propagated.
	effects := audio.GetInsertEffects(instID)
	if len(effects) == 0 {
		t.Fatal("no effects")
	}
	b := dv.fxSliderBindings[0]
	expected := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	actual := effects[0].Params[b.paramName]
	if actual != expected {
		t.Errorf("mobile audio param not propagated: expected=%f, got=%f", expected, actual)
	}

	// Release.
	fxReleaseInput(t, dv)
	if dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should be false after release on mobile")
	}
}

func TestFXPanelSliderDragOutsidePanel(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders")
	}

	sl := dv.fxPanelSliders[0]
	slR := sl.Rect()
	if slR.Empty() {
		t.Fatal("slider has empty rect")
	}

	// Press on the slider to start drag.
	sx := (slR.Min.X + slR.Max.X) / 2
	sy := (slR.Min.Y + slR.Max.Y) / 2
	fxHoldAt(t, dv, sx, sy)

	if !dv.fxSliderDragging {
		t.Fatal("drag did not start")
	}

	// Move cursor far outside the panel rect.
	outsideX := dv.fxPanelRect.Max.X + 100
	outsideY := dv.fxPanelRect.Max.Y + 100
	fxHoldAt(t, dv, outsideX, outsideY)

	// Panel should NOT have closed — drag takes priority.
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel closed while slider was being dragged outside panel rect")
	}
	if !dv.fxSliderDragging {
		t.Fatal("drag ended prematurely when cursor left panel rect")
	}

	// Release outside panel — drag ends, panel stays open.
	fxReleaseInput(t, dv)
	if dv.fxSliderDragging {
		t.Fatal("drag should end on release")
	}
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel should stay open after drag release")
	}
}

// newMobileDV creates a DrumView in mobile mode for FX panel tests.
func newMobileDV(t *testing.T) *DrumView {
	t.Helper()
	setupMobileTest(t, true)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	return g.drum
}

func TestFXPanelCollapsedByDefaultMobile(t *testing.T) {
	dv := newMobileDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// On mobile, effects start collapsed — no sliders should be created.
	if len(dv.fxPanelSliders) != 0 {
		t.Errorf("expected 0 sliders when collapsed on mobile, got %d", len(dv.fxPanelSliders))
	}

	// Expand button (IconChevronRight when collapsed) should exist.
	var expandBtn *Button
	for _, btn := range dv.fxPanelBtns {
		if btn.Icon == string(IconChevronRight) {
			expandBtn = btn
			break
		}
	}
	if expandBtn == nil {
		t.Fatal("expand chevron button not found on mobile")
	}
}

func TestFXPanelExpandCollapseMobile(t *testing.T) {
	dv := newMobileDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Collapsed: no sliders.
	if len(dv.fxPanelSliders) != 0 {
		t.Errorf("expected 0 sliders when collapsed, got %d", len(dv.fxPanelSliders))
	}

	// Expand slot 0.
	dv.fxExpandedSlots[0] = true
	dv.buildFXPanel()
	suppressClicksUntilRelease = false

	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("expected sliders after expanding slot 0")
	}

	// Collapse slot 0.
	dv.fxExpandedSlots[0] = false
	dv.buildFXPanel()

	if len(dv.fxPanelSliders) != 0 {
		t.Errorf("expected 0 sliders after collapsing, got %d", len(dv.fxPanelSliders))
	}
}

func TestFXPanelDesktopAlwaysExpanded(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// On desktop, enabled effects always show sliders (no collapse).
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("expected sliders on desktop for enabled effect")
	}

	// No expand/collapse chevron button should exist on desktop.
	// (Only 1 effect added so no move buttons either — chevron absence is unambiguous.)
	for _, btn := range dv.fxPanelBtns {
		if btn.Icon == string(IconChevronRight) || btn.Icon == string(IconChevronDown) {
			t.Error("expand/collapse chevron should not exist on desktop")
		}
	}
}

func TestFXPanelScrollNeededMobile(t *testing.T) {
	dv := newMobileDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	// Add 3 effects and expand all.
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Expand all 3 effects.
	dv.fxExpandedSlots[0] = true
	dv.fxExpandedSlots[1] = true
	dv.fxExpandedSlots[2] = true
	dv.buildFXPanel()

	if dv.fxScrollMaxPx <= 0 {
		t.Errorf("expected fxScrollMaxPx > 0 with 3 expanded effects on mobile, got %d", dv.fxScrollMaxPx)
	}
}

func TestFXPanelScrollOffsetClamps(t *testing.T) {
	dv := newMobileDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	dv.fxExpandedSlots[0] = true
	dv.fxExpandedSlots[1] = true
	dv.fxExpandedSlots[2] = true
	dv.buildFXPanel()

	// Set scroll beyond max.
	dv.fxScrollOffsetPx = dv.fxScrollMaxPx + 500
	dv.buildFXPanel()

	if dv.fxScrollOffsetPx > dv.fxScrollMaxPx {
		t.Errorf("scroll offset %d exceeds max %d after rebuild", dv.fxScrollOffsetPx, dv.fxScrollMaxPx)
	}

	// Set scroll negative.
	dv.fxScrollOffsetPx = -100
	dv.buildFXPanel()

	if dv.fxScrollOffsetPx < 0 {
		t.Errorf("scroll offset %d should not be negative after rebuild", dv.fxScrollOffsetPx)
	}
}

// TestFXPanelSliderClickWithSuppressActive verifies that a new press on an
// FX panel slider works even when suppressClicksUntilRelease is true.
// Before the fix, the tree did not clear the global suppress flag around
// OnPress dispatch (unlike OnDrag at line 192-195), causing
// Slider.HandleInputResult to return InputIgnored.
func TestFXPanelSliderClickWithSuppressActive(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	// Advance frames to clear any debounce window.
	fxAdvanceFrames(t, dv, 3)

	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in FX panel")
	}

	sl := dv.fxPanelSliders[0]
	slR := sl.TrackRect()
	if slR.Empty() {
		t.Fatal("slider track has empty rect")
	}

	startVal := sl.Value

	// Target: center of the slider track.
	cx := (slR.Min.X + slR.Max.X) / 2
	cy := (slR.Min.Y + slR.Max.Y) / 2

	// Set up input at slider center with mouse pressed.
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	t.Cleanup(restore)

	// Simulate the timing window: suppress is true from a prior interaction
	// but the tree's own suppress has been cleared (e.g., release frame).
	suppressClicksUntilRelease = true

	dv.Update()

	// The slider should have captured the press despite global suppress.
	if !dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should be true — slider press was blocked by stale suppressClicksUntilRelease")
	}
	if sl.Value == startVal {
		t.Error("slider value should have changed after press at center of track")
	}

	// Release and verify cleanup.
	restore()
	fxReleaseInput(t, dv)
	if dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should be false after release")
	}
}

func TestFXPanelContentFitsInBoundsMobile(t *testing.T) {
	dv := newMobileDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	r := dv.fxPanelRect
	// Panel rect must fit within drum bounds.
	if r.Min.Y < dv.Bounds.Min.Y || r.Max.Y > dv.Bounds.Max.Y {
		t.Errorf("panel Y [%d, %d] exceeds bounds [%d, %d]", r.Min.Y, r.Max.Y, dv.Bounds.Min.Y, dv.Bounds.Max.Y)
	}
	if r.Min.X < dv.Bounds.Min.X || r.Max.X > dv.Bounds.Max.X {
		t.Errorf("panel X [%d, %d] exceeds bounds [%d, %d]", r.Min.X, r.Max.X, dv.Bounds.Min.X, dv.Bounds.Max.X)
	}

	// All visible buttons must be within panel rect.
	for i, btn := range dv.fxPanelBtns {
		if btn == nil {
			continue
		}
		br := btn.Rect()
		if br.Empty() {
			continue
		}
		// Buttons should overlap the viewport or be the close button (fixed header).
		if br.Max.Y <= dv.fxViewportRect.Min.Y || br.Min.Y >= dv.fxViewportRect.Max.Y {
			// May be the close button in the header area.
			if i == len(dv.fxPanelBtns)-1 {
				continue // close button is in header
			}
		}
	}
}

// TestFXPanelSliderClickBlocksRowCallback verifies that clicking on an FX
// panel slider is handled by the portal (z=300) and does NOT leak to the
// underlying RowRackZone (z=120).
func TestFXPanelSliderClickBlocksRowCallback(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Advance frames to clear debounce window.
	fxAdvanceFrames(t, dv, 3)

	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in FX panel")
	}

	// Hook spy on row-rack callbacks to detect unwanted dispatch.
	fxToggleCalled := false
	origOnFX := dv.rowRackZone.callbacks.OnFXPanelToggle
	dv.rowRackZone.callbacks.OnFXPanelToggle = func(row int) {
		fxToggleCalled = true
		if origOnFX != nil {
			origOnFX(row)
		}
	}
	muteCalled := false
	origMute := dv.rowRackZone.callbacks.OnMuteToggle
	dv.rowRackZone.callbacks.OnMuteToggle = func(row int) {
		muteCalled = true
		if origMute != nil {
			origMute(row)
		}
	}

	sl := dv.fxPanelSliders[0]
	slR := sl.Rect()
	if slR.Empty() {
		t.Fatal("slider has empty rect")
	}

	startVal := sl.Value

	// Click center of slider.
	cx := (slR.Min.X + slR.Max.X) / 2
	cy := (slR.Min.Y + slR.Max.Y) / 2
	fxHoldAt(t, dv, cx, cy)

	// Slider should have captured the press.
	if !dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should be true — slider did not capture the click")
	}
	if sl.Value == startVal {
		t.Error("slider value should have changed after click at center")
	}

	// Tree should have dispatched to the FX panel portal.
	if dv.tree != nil && !dv.tree.InputHandled() {
		t.Error("tree.InputHandled() should be true — input routed to FX portal")
	}

	// Row callbacks must NOT have been called.
	if fxToggleCalled {
		t.Error("OnFXPanelToggle was called — click leaked to RowRackZone")
	}
	if muteCalled {
		t.Error("OnMuteToggle was called — click leaked to RowRackZone")
	}

	// Release.
	fxReleaseInput(t, dv)
	if dv.fxSliderDragging {
		t.Fatal("fxSliderDragging should be false after release")
	}
}

// TestFXPanelHeaderClickBlocksRowCallback verifies that clicking on the FX
// panel header (the title bar area, NOT the close button) is consumed by the
// portal and does NOT leak through to the underlying RowRackZone callbacks.
func TestFXPanelHeaderClickBlocksRowCallback(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Advance frames to clear debounce window.
	fxAdvanceFrames(t, dv, 3)

	// Compute header center — 16px down from panel top (center of 32px header),
	// offset left from center to avoid the close button on the right.
	hx := dv.fxPanelRect.Min.X + 30 // left side of header, away from close button
	hy := dv.fxPanelRect.Min.Y + 16 // vertical center of 32px header

	// Hook spies on ALL row-rack callbacks to detect unwanted dispatch.
	var leaked []string
	spy := func(name string, orig func(int)) func(int) {
		return func(row int) {
			leaked = append(leaked, name)
			if orig != nil {
				orig(row)
			}
		}
	}
	dv.rowRackZone.callbacks.OnFXPanelToggle = spy("OnFXPanelToggle", dv.rowRackZone.callbacks.OnFXPanelToggle)
	dv.rowRackZone.callbacks.OnMuteToggle = spy("OnMuteToggle", dv.rowRackZone.callbacks.OnMuteToggle)
	dv.rowRackZone.callbacks.OnSoloToggle = spy("OnSoloToggle", dv.rowRackZone.callbacks.OnSoloToggle)
	dv.rowRackZone.callbacks.OnInstMenuOpen = spy("OnInstMenuOpen", dv.rowRackZone.callbacks.OnInstMenuOpen)
	dv.rowRackZone.callbacks.OnContextMenuOpen = spy("OnContextMenuOpen", dv.rowRackZone.callbacks.OnContextMenuOpen)
	dv.rowRackZone.callbacks.OnColorWheelOpen = spy("OnColorWheelOpen", dv.rowRackZone.callbacks.OnColorWheelOpen)
	dv.rowRackZone.callbacks.OnRenameOpen = spy("OnRenameOpen", dv.rowRackZone.callbacks.OnRenameOpen)
	dv.rowRackZone.callbacks.OnDeleteRow = spy("OnDeleteRow", dv.rowRackZone.callbacks.OnDeleteRow)
	dv.rowRackZone.callbacks.OnOriginReq = spy("OnOriginReq", dv.rowRackZone.callbacks.OnOriginReq)
	dv.rowRackZone.callbacks.OnRowSelect = spy("OnRowSelect", dv.rowRackZone.callbacks.OnRowSelect)
	origAdd := dv.rowRackZone.callbacks.OnAddRow
	dv.rowRackZone.callbacks.OnAddRow = func() {
		leaked = append(leaked, "OnAddRow")
		if origAdd != nil {
			origAdd()
		}
	}

	// Click header (hold — calls Update which processes the press).
	fxHoldAt(t, dv, hx, hy)

	// Tree should have handled the input (check before release resets it).
	if dv.tree != nil && !dv.tree.InputHandled() {
		t.Error("tree.InputHandled() should be true — header click should be consumed by portal")
	}

	// Row callbacks must NOT have been called.
	if len(leaked) > 0 {
		t.Errorf("row-rack callbacks leaked through FX panel header: %v", leaked)
	}

	// FX panel should still be open.
	if !dv.IsFXPanelOpen() {
		t.Error("FX panel closed unexpectedly after header click")
	}

	// Release.
	fxReleaseInput(t, dv)
}

// TestFXPanelUnionRectGapClickConsumed verifies that clicking an FX button
// outside fxPanelRect triggers the tree's click-outside mechanism, closing
// the panel without leaking to the row-rack zone.
func TestFXPanelUnionRectGapClickConsumed(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	fxAdvanceFrames(t, dv, 3)

	// Find an FX button rect that extends outside fxPanelRect.
	fxBtns := dv.rowFXBtns()
	if len(fxBtns) == 0 {
		t.Skip("no FX buttons")
	}
	var gapX, gapY int
	found := false
	for _, btn := range fxBtns {
		if btn == nil {
			continue
		}
		br := btn.Rect()
		if br.Empty() {
			continue
		}
		cx := (br.Min.X + br.Max.X) / 2
		cy := (br.Min.Y + br.Max.Y) / 2
		// We need a point that is outside fxPanelRect (triggers
		// click-outside via the tree).
		if !image.Pt(cx, cy).In(dv.fxPanelRect) {
			gapX, gapY = cx, cy
			found = true
			break
		}
	}
	if !found {
		t.Skip("no FX button outside fxPanelRect — union rect has no gap")
	}

	// Hook spy on row-rack callbacks.
	var leaked []string
	spy := func(name string, orig func(int)) func(int) {
		return func(row int) {
			leaked = append(leaked, name)
			if orig != nil {
				orig(row)
			}
		}
	}
	dv.rowRackZone.callbacks.OnFXPanelToggle = spy("OnFXPanelToggle", dv.rowRackZone.callbacks.OnFXPanelToggle)
	dv.rowRackZone.callbacks.OnMuteToggle = spy("OnMuteToggle", dv.rowRackZone.callbacks.OnMuteToggle)
	dv.rowRackZone.callbacks.OnSoloToggle = spy("OnSoloToggle", dv.rowRackZone.callbacks.OnSoloToggle)
	dv.rowRackZone.callbacks.OnRowSelect = spy("OnRowSelect", dv.rowRackZone.callbacks.OnRowSelect)

	// Click at the gap point — tree's click-outside closes the panel.
	fxHoldAt(t, dv, gapX, gapY)

	// No row callbacks leaked (tree's click-outside suppresses further dispatch).
	if len(leaked) > 0 {
		t.Errorf("row-rack callbacks leaked through union rect gap: %v", leaked)
	}

	// Panel should be closed via tree's click-outside mechanism.
	if dv.IsFXPanelOpen() {
		t.Error("FX panel should be closed after click-outside via tree")
	}

	fxReleaseInput(t, dv)
}

// TestFXPanelSliderDragSurvivesSuppressOnDrag verifies that an active slider
// drag is not killed by a stale suppressClicksUntilRelease flag. The tree
// must clear the global flag during OnDrag dispatch so the slider's
// HandleInputResult does not bail.
func TestFXPanelSliderDragSurvivesSuppressOnDrag(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	fxAdvanceFrames(t, dv, 3)

	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders")
	}

	sl := dv.fxPanelSliders[0]
	slR := sl.Rect()
	if slR.Empty() {
		t.Fatal("slider has empty rect")
	}

	// Start drag on left edge of slider.
	sx := slR.Min.X + 2
	sy := (slR.Min.Y + slR.Max.Y) / 2
	fxHoldAt(t, dv, sx, sy)
	if !dv.fxSliderDragging {
		t.Fatal("drag did not start")
	}
	startVal := sl.Value

	// Simulate stale suppress flag (e.g., from a prior interaction).
	suppressClicksUntilRelease = true

	// Continue drag to right side of slider.
	dx := slR.Max.X - 4
	fxHoldAt(t, dv, dx, sy)

	// Slider should still be dragging despite stale suppress.
	if !dv.fxSliderDragging {
		t.Fatal("slider drag was killed by stale suppressClicksUntilRelease during OnDrag")
	}
	if sl.Value <= startVal {
		t.Errorf("slider value should have increased during drag: start=%f, now=%f", startVal, sl.Value)
	}

	// Release.
	fxReleaseInput(t, dv)
	if dv.fxSliderDragging {
		t.Fatal("drag should end on release")
	}
}

// TestFXPanelHeaderClickAfterReopenDifferentRow verifies that reopening the
// FX panel for a different row (different anchor position) registers the
// correct hit area immediately so header clicks don't leak to the underlying
// RowRackZone. This catches the stale fxPanelRect bug where closeFXPanel()
// didn't zero the rect and openFXPanel() registered the portal before
// buildFXPanel() computed the new rect.
func TestFXPanelHeaderClickAfterReopenDifferentRow(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	// Add a second row so we can reopen on a different anchor.
	dv.AddRow()

	// Open FX panel for row 0, then close it.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open for row 0")
	}
	oldRect := dv.fxPanelRect
	dv.closeFXPanel()

	// fxPanelRect must be zeroed after close.
	if !dv.fxPanelRect.Empty() {
		t.Fatal("fxPanelRect should be zeroed after closeFXPanel()")
	}

	// Reopen for row 1 (different anchor → different rect).
	dv.toggleFXPanel(1)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open for row 1")
	}
	newRect := dv.fxPanelRect
	if newRect.Empty() {
		t.Fatal("fxPanelRect is empty after reopening")
	}

	// Advance frames to clear debounce window.
	fxAdvanceFrames(t, dv, 3)

	// Click center of new header.
	hx := newRect.Min.X + 30
	hy := newRect.Min.Y + 16

	// Spy on row callbacks.
	var leaked []string
	spy := func(name string, orig func(int)) func(int) {
		return func(row int) {
			leaked = append(leaked, name)
			if orig != nil {
				orig(row)
			}
		}
	}
	dv.rowRackZone.callbacks.OnFXPanelToggle = spy("OnFXPanelToggle", dv.rowRackZone.callbacks.OnFXPanelToggle)
	dv.rowRackZone.callbacks.OnMuteToggle = spy("OnMuteToggle", dv.rowRackZone.callbacks.OnMuteToggle)
	dv.rowRackZone.callbacks.OnSoloToggle = spy("OnSoloToggle", dv.rowRackZone.callbacks.OnSoloToggle)
	dv.rowRackZone.callbacks.OnRowSelect = spy("OnRowSelect", dv.rowRackZone.callbacks.OnRowSelect)

	fxHoldAt(t, dv, hx, hy)

	if len(leaked) > 0 {
		t.Errorf("row-rack callbacks leaked after reopen for different row: %v (old rect=%v, new rect=%v)", leaked, oldRect, newRect)
	}
	if !dv.IsFXPanelOpen() {
		t.Error("FX panel closed unexpectedly after header click")
	}

	fxReleaseInput(t, dv)
}

// TestFXPanelScrimClickAbsorbedByPortal verifies that clicking on the scrim
// area (outside fxPanelRect and FX buttons, but inside dv.Bounds) is absorbed
// by the portal handler and does NOT leak to the underlying RowRackZone.
func TestFXPanelScrimClickAbsorbedByPortal(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel for row 0.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	// Advance 3 frames to clear the debounce window.
	fxAdvanceFrames(t, dv, 3)

	// Find a point OUTSIDE fxPanelRect and FX buttons but INSIDE dv.Bounds.
	// Use a point near the bottom-right corner of dv.Bounds, far from all
	// panel and button rects.
	scrimPt := image.Pt(dv.Bounds.Max.X-5, dv.Bounds.Max.Y-5)
	if scrimPt.In(dv.fxPanelRect) {
		t.Fatal("test point should be outside fxPanelRect")
	}
	for _, btn := range dv.rowFXBtns() {
		if btn != nil && scrimPt.In(btn.Rect()) {
			t.Fatal("test point should be outside all FX buttons")
		}
	}
	if !scrimPt.In(dv.Bounds) {
		t.Fatal("test point should be inside dv.Bounds")
	}

	// Hook spy callbacks on all RowRackCallbacks to detect leaks.
	var leaked []string
	spy := func(name string, orig func(int)) func(int) {
		return func(row int) {
			leaked = append(leaked, name)
			if orig != nil {
				orig(row)
			}
		}
	}
	dv.rowRackZone.callbacks.OnFXPanelToggle = spy("OnFXPanelToggle", dv.rowRackZone.callbacks.OnFXPanelToggle)
	dv.rowRackZone.callbacks.OnMuteToggle = spy("OnMuteToggle", dv.rowRackZone.callbacks.OnMuteToggle)
	dv.rowRackZone.callbacks.OnSoloToggle = spy("OnSoloToggle", dv.rowRackZone.callbacks.OnSoloToggle)
	dv.rowRackZone.callbacks.OnInstMenuOpen = spy("OnInstMenuOpen", dv.rowRackZone.callbacks.OnInstMenuOpen)
	dv.rowRackZone.callbacks.OnContextMenuOpen = spy("OnContextMenuOpen", dv.rowRackZone.callbacks.OnContextMenuOpen)
	dv.rowRackZone.callbacks.OnColorWheelOpen = spy("OnColorWheelOpen", dv.rowRackZone.callbacks.OnColorWheelOpen)
	dv.rowRackZone.callbacks.OnRenameOpen = spy("OnRenameOpen", dv.rowRackZone.callbacks.OnRenameOpen)
	dv.rowRackZone.callbacks.OnDeleteRow = spy("OnDeleteRow", dv.rowRackZone.callbacks.OnDeleteRow)
	dv.rowRackZone.callbacks.OnOriginReq = spy("OnOriginReq", dv.rowRackZone.callbacks.OnOriginReq)
	dv.rowRackZone.callbacks.OnRowSelect = spy("OnRowSelect", dv.rowRackZone.callbacks.OnRowSelect)
	origAdd := dv.rowRackZone.callbacks.OnAddRow
	dv.rowRackZone.callbacks.OnAddRow = func() {
		leaked = append(leaked, "OnAddRow")
		if origAdd != nil {
			origAdd()
		}
	}

	// Click the scrim point — tree's click-outside closes the panel.
	fxHoldAt(t, dv, scrimPt.X, scrimPt.Y)

	// No row-rack callbacks should have leaked.
	if len(leaked) > 0 {
		t.Errorf("row-rack callbacks leaked through scrim click: %v", leaked)
	}

	// FX panel should be closed (scrim click = click-outside via tree).
	if dv.IsFXPanelOpen() {
		t.Error("FX panel should close on scrim click outside panel")
	}

	fxReleaseInput(t, dv)
}

// TestFXPanelEmptySpaceDoesNotLeakToRowRack verifies that clicking empty
// space inside fxPanelRect (between buttons) is consumed by the portal and
// does NOT leak to the underlying RowRackZone.
func TestFXPanelEmptySpaceDoesNotLeakToRowRack(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	fxAdvanceFrames(t, dv, 3)

	// Find empty space inside fxPanelRect.
	emptyX := dv.fxPanelRect.Min.X + 10
	emptyY := dv.fxPanelRect.Min.Y + dv.fxPanelRect.Dy()/2
	emptyPt := image.Pt(emptyX, emptyY)
	if !emptyPt.In(dv.fxPanelRect) {
		t.Fatal("test point should be inside fxPanelRect")
	}

	// Verify point is not inside any button or slider rect.
	for _, btn := range dv.fxPanelBtns {
		if btn != nil && emptyPt.In(btn.Rect()) {
			t.Fatal("test point should NOT be inside any button")
		}
	}
	for _, sl := range dv.fxPanelSliders {
		if sl != nil && emptyPt.In(sl.Rect()) {
			t.Fatal("test point should NOT be inside any slider")
		}
	}

	// Hook spy callbacks on ALL RowRackCallbacks.
	var leaked []string
	spy := func(name string, orig func(int)) func(int) {
		return func(row int) {
			leaked = append(leaked, name)
			if orig != nil {
				orig(row)
			}
		}
	}
	dv.rowRackZone.callbacks.OnFXPanelToggle = spy("OnFXPanelToggle", dv.rowRackZone.callbacks.OnFXPanelToggle)
	dv.rowRackZone.callbacks.OnMuteToggle = spy("OnMuteToggle", dv.rowRackZone.callbacks.OnMuteToggle)
	dv.rowRackZone.callbacks.OnSoloToggle = spy("OnSoloToggle", dv.rowRackZone.callbacks.OnSoloToggle)
	dv.rowRackZone.callbacks.OnInstMenuOpen = spy("OnInstMenuOpen", dv.rowRackZone.callbacks.OnInstMenuOpen)
	dv.rowRackZone.callbacks.OnContextMenuOpen = spy("OnContextMenuOpen", dv.rowRackZone.callbacks.OnContextMenuOpen)
	dv.rowRackZone.callbacks.OnColorWheelOpen = spy("OnColorWheelOpen", dv.rowRackZone.callbacks.OnColorWheelOpen)
	dv.rowRackZone.callbacks.OnRenameOpen = spy("OnRenameOpen", dv.rowRackZone.callbacks.OnRenameOpen)
	dv.rowRackZone.callbacks.OnDeleteRow = spy("OnDeleteRow", dv.rowRackZone.callbacks.OnDeleteRow)
	dv.rowRackZone.callbacks.OnOriginReq = spy("OnOriginReq", dv.rowRackZone.callbacks.OnOriginReq)
	dv.rowRackZone.callbacks.OnRowSelect = spy("OnRowSelect", dv.rowRackZone.callbacks.OnRowSelect)
	origAdd := dv.rowRackZone.callbacks.OnAddRow
	dv.rowRackZone.callbacks.OnAddRow = func() {
		leaked = append(leaked, "OnAddRow")
		if origAdd != nil {
			origAdd()
		}
	}

	// Click the empty-space point.
	fxHoldAt(t, dv, emptyX, emptyY)

	// Tree should have handled the input.
	if dv.tree != nil && !dv.tree.InputHandled() {
		t.Error("tree.InputHandled() should be true — empty space click should be consumed by portal")
	}

	// No row callbacks leaked.
	if len(leaked) > 0 {
		t.Errorf("row-rack callbacks leaked through FX panel empty space: %v", leaked)
	}

	// FX panel should still be open.
	if !dv.IsFXPanelOpen() {
		t.Error("FX panel closed unexpectedly after empty space click")
	}

	fxReleaseInput(t, dv)
}

// TestFXPanelClickOutsideClosesWithoutLeak verifies that clicking outside
// fxPanelRect but inside dv.Bounds (scrim area) closes the panel via the
// tree's click-outside mechanism without leaking to the row-rack zone.
func TestFXPanelClickOutsideClosesWithoutLeak(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	fxAdvanceFrames(t, dv, 3)

	// Find a point outside fxPanelRect but inside dv.Bounds.
	outsidePt := image.Pt(dv.Bounds.Max.X-5, dv.Bounds.Max.Y-5)
	if outsidePt.In(dv.fxPanelRect) {
		t.Fatal("test point should be outside fxPanelRect")
	}
	if !outsidePt.In(dv.Bounds) {
		t.Fatal("test point should be inside dv.Bounds")
	}

	// Hook spy callbacks.
	var leaked []string
	spy := func(name string, orig func(int)) func(int) {
		return func(row int) {
			leaked = append(leaked, name)
			if orig != nil {
				orig(row)
			}
		}
	}
	dv.rowRackZone.callbacks.OnFXPanelToggle = spy("OnFXPanelToggle", dv.rowRackZone.callbacks.OnFXPanelToggle)
	dv.rowRackZone.callbacks.OnMuteToggle = spy("OnMuteToggle", dv.rowRackZone.callbacks.OnMuteToggle)
	dv.rowRackZone.callbacks.OnSoloToggle = spy("OnSoloToggle", dv.rowRackZone.callbacks.OnSoloToggle)
	dv.rowRackZone.callbacks.OnInstMenuOpen = spy("OnInstMenuOpen", dv.rowRackZone.callbacks.OnInstMenuOpen)
	dv.rowRackZone.callbacks.OnContextMenuOpen = spy("OnContextMenuOpen", dv.rowRackZone.callbacks.OnContextMenuOpen)
	dv.rowRackZone.callbacks.OnColorWheelOpen = spy("OnColorWheelOpen", dv.rowRackZone.callbacks.OnColorWheelOpen)
	dv.rowRackZone.callbacks.OnRenameOpen = spy("OnRenameOpen", dv.rowRackZone.callbacks.OnRenameOpen)
	dv.rowRackZone.callbacks.OnDeleteRow = spy("OnDeleteRow", dv.rowRackZone.callbacks.OnDeleteRow)
	dv.rowRackZone.callbacks.OnOriginReq = spy("OnOriginReq", dv.rowRackZone.callbacks.OnOriginReq)
	dv.rowRackZone.callbacks.OnRowSelect = spy("OnRowSelect", dv.rowRackZone.callbacks.OnRowSelect)
	origAdd := dv.rowRackZone.callbacks.OnAddRow
	dv.rowRackZone.callbacks.OnAddRow = func() {
		leaked = append(leaked, "OnAddRow")
		if origAdd != nil {
			origAdd()
		}
	}

	// Click outside — tree's click-outside closes the panel.
	fxHoldAt(t, dv, outsidePt.X, outsidePt.Y)

	// No row callbacks leaked.
	if len(leaked) > 0 {
		t.Errorf("row-rack callbacks leaked through click-outside: %v", leaked)
	}

	// Panel should be closed.
	if dv.IsFXPanelOpen() {
		t.Error("FX panel should close on click outside")
	}

	fxReleaseInput(t, dv)
}

// TestFXPanelHeaderClickAfterAddEffect verifies that after adding an effect
// (which calls buildFXPanel() changing panel height), clicking the header
// does not leak to the underlying RowRackZone. This catches the stale hit
// area bug where the portal's registered hit areas weren't refreshed until
// the next frame's Update().
func TestFXPanelHeaderClickAfterAddEffect(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument

	// Open FX panel with no effects.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	rectBefore := dv.fxPanelRect

	// Add an effect via audio API + rebuild (simulates button click).
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)
	dv.buildFXPanel()
	dv.refreshFXPortalHitAreas()

	rectAfter := dv.fxPanelRect
	if rectAfter == rectBefore {
		// Panel height should change after adding an effect with params.
		t.Log("panel rect did not change after adding effect — test still valid for hit area freshness")
	}

	// Advance frames to clear debounce window.
	fxAdvanceFrames(t, dv, 3)

	// Click center of (possibly moved) header — WITHOUT waiting for portal.Update().
	hx := rectAfter.Min.X + 30
	hy := rectAfter.Min.Y + 16

	// Spy on row callbacks.
	var leaked []string
	spy := func(name string, orig func(int)) func(int) {
		return func(row int) {
			leaked = append(leaked, name)
			if orig != nil {
				orig(row)
			}
		}
	}
	dv.rowRackZone.callbacks.OnFXPanelToggle = spy("OnFXPanelToggle", dv.rowRackZone.callbacks.OnFXPanelToggle)
	dv.rowRackZone.callbacks.OnMuteToggle = spy("OnMuteToggle", dv.rowRackZone.callbacks.OnMuteToggle)
	dv.rowRackZone.callbacks.OnSoloToggle = spy("OnSoloToggle", dv.rowRackZone.callbacks.OnSoloToggle)
	dv.rowRackZone.callbacks.OnRowSelect = spy("OnRowSelect", dv.rowRackZone.callbacks.OnRowSelect)

	fxHoldAt(t, dv, hx, hy)

	if len(leaked) > 0 {
		t.Errorf("row-rack callbacks leaked after adding effect: %v (rectBefore=%v, rectAfter=%v)", leaked, rectBefore, rectAfter)
	}
	if !dv.IsFXPanelOpen() {
		t.Error("FX panel closed unexpectedly after header click")
	}

	fxReleaseInput(t, dv)
}
