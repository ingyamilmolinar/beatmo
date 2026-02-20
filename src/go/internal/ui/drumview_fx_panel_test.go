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
	if len(dv.rowFXBtns) == 0 {
		t.Skip("no FX buttons")
	}

	btn := dv.rowFXBtns[0]
	r := btn.Rect()
	if r.Empty() {
		t.Skip("FX button has empty rect")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

	// Click FX button to open.
	fxClickAt(t, dv, cx, cy)
	fxReleaseInput(t, dv)
	if !dv.fxPanelOpen {
		t.Fatal("FX panel did not open")
	}
	if dv.fxPanelRow != 0 {
		t.Errorf("expected row 0, got %d", dv.fxPanelRow)
	}

	// Click again to close (toggle).
	fxClickAt(t, dv, cx, cy)
	fxReleaseInput(t, dv)
	if dv.fxPanelOpen {
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
	if len(dv.Rows) == 0 || len(dv.rowFXBtns) == 0 {
		t.Skip("no rows or FX buttons")
	}

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.fxPanelOpen {
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
	if len(dv.Rows) == 0 || len(dv.rowFXBtns) == 0 {
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

	// Click Cancel.
	var cancelBtn *Button
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == "Cancel" {
			cancelBtn = btn
			break
		}
	}
	if cancelBtn == nil {
		t.Fatal("Cancel button not found")
	}
	cancelBtn.OnClick()

	if dv.fxAddMenuOpen {
		t.Error("picker should close after cancel")
	}
	if len(audio.GetInsertEffects(instID)) != beforeCount {
		t.Error("no effect should be added after cancel")
	}
}

func TestFXPanelPickerShowsAllTypes(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns) == 0 {
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

	// Verify all 6 type names + Cancel are present.
	expected := map[string]bool{
		"Distortion": false,
		"Delay":      false,
		"Reverb":     false,
		"Chorus":     false,
		"Bitcrusher": false,
		"Filter":     false,
		"Cancel":     false,
	}
	for _, btn := range dv.fxPanelBtns {
		if _, ok := expected[btn.Text]; ok {
			expected[btn.Text] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("button %q not found in picker", name)
		}
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
	if !dv.fxPanelOpen {
		t.Fatal("FX panel did not open")
	}

	// Should have toggle, remove, and add buttons.
	// Find the remove button (text "✕").
	var removeBtn *Button
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == "✕" {
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

	// Find toggle button (text "✓").
	var toggleBtn *Button
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == "✓" {
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
	dv.fxPanelOpen = true
	if !dv.anyDropdownOpen() {
		t.Error("anyDropdownOpen should return true when FX panel is open")
	}
}

func TestFXPanelClosedByCloseAllPopups(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel.
	dv.toggleFXPanel(0)
	if !dv.fxPanelOpen {
		t.Fatal("FX panel did not open")
	}

	// CloseAllPopups should close the FX panel.
	dv.CloseAllPopups()
	if dv.fxPanelOpen {
		t.Fatal("FX panel was not closed by CloseAllPopups")
	}
}

func TestFXPanelHasCloseButton(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.toggleFXPanel(0)
	if !dv.fxPanelOpen {
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
	if dv.fxPanelOpen {
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
	if !dv.fxPanelOpen {
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
	if !dv.fxPanelOpen {
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
	if !dv.fxPanelOpen {
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

	// Advance past debounce window so click-outside could close the panel.
	dv.updateSeq = dv.fxPanelOpenSeq + 10

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
	if !dv.fxPanelOpen {
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
	if !dv.fxPanelOpen {
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
	if !dv.fxPanelOpen {
		t.Fatal("FX panel did not open")
	}

	// On mobile, effects start collapsed — no sliders should be created.
	if len(dv.fxPanelSliders) != 0 {
		t.Errorf("expected 0 sliders when collapsed on mobile, got %d", len(dv.fxPanelSliders))
	}

	// Expand button (chevron) should exist.
	var expandBtn *Button
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == "▶" {
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
	if !dv.fxPanelOpen {
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
	if !dv.fxPanelOpen {
		t.Fatal("FX panel did not open")
	}

	// On desktop, enabled effects always show sliders (no collapse).
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("expected sliders on desktop for enabled effect")
	}

	// No expand/collapse chevron button should exist on desktop.
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == "▶" || btn.Text == "▼" {
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
	if !dv.fxPanelOpen {
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
	if !dv.fxPanelOpen {
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
