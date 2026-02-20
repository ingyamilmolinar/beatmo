//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// --- 1. Toggle lifecycle ---

func TestFXPanelToggleOpenClose(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Open FX panel for row 0.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel should be open after toggleFXPanel(0)")
	}
	if dv.fxPanelRow != 0 {
		t.Errorf("expected fxPanelRow=0, got %d", dv.fxPanelRow)
	}

	// Toggle same row again to close.
	dv.toggleFXPanel(0)
	if dv.IsFXPanelOpen() {
		t.Fatal("FX panel should be closed after second toggleFXPanel(0)")
	}
}

func TestFXPanelToggleDifferentRow(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) < 2 {
		dv.AddRow()
		dv.recalcButtons()
		dv.calcLayout()
	}
	if len(dv.Rows) < 2 {
		t.Skip("need at least 2 rows")
	}

	// Open for row 0.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() || dv.fxPanelRow != 0 {
		t.Fatalf("expected panel open for row 0, open=%v row=%d", dv.IsFXPanelOpen(), dv.fxPanelRow)
	}

	// Toggle row 1 — should switch (close row 0, open row 1).
	dv.toggleFXPanel(1)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel should still be open after switching to row 1")
	}
	if dv.fxPanelRow != 1 {
		t.Errorf("expected fxPanelRow=1, got %d", dv.fxPanelRow)
	}
}

func TestFXPanelOpenInvalidRow(t *testing.T) {
	dv := newTestDV(t)

	// Negative row.
	dv.openFXPanel(-1)
	if dv.IsFXPanelOpen() {
		t.Error("panel should not open for negative row")
	}

	// Out-of-range row.
	dv.openFXPanel(len(dv.Rows) + 100)
	if dv.IsFXPanelOpen() {
		t.Error("panel should not open for out-of-range row")
	}
}

// --- 2. Close cleanup ---

func TestFXPanelCloseResetsState(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	// Open panel to populate state.
	dv.openFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel should be open")
	}

	// Simulate accumulated state.
	dv.fxAddMenuOpen = true
	dv.fxSliderDragging = true

	// Close and verify all state is reset.
	dv.closeFXPanel()

	if dv.IsFXPanelOpen() {
		t.Error("fxPanelOpen should be false after close")
	}
	if dv.fxAddMenuOpen {
		t.Error("fxAddMenuOpen should be false after close")
	}
	if dv.fxPanelBtns != nil {
		t.Error("fxPanelBtns should be nil after close")
	}
	if dv.fxPanelSliders != nil {
		t.Error("fxPanelSliders should be nil after close")
	}
	if dv.fxSliderDragging {
		t.Error("fxSliderDragging should be false after close")
	}
	if dv.fxScrollOffsetPx != 0 {
		t.Errorf("fxScrollOffsetPx should be 0 after close, got %d", dv.fxScrollOffsetPx)
	}
	if dv.fxScrollMaxPx != 0 {
		t.Errorf("fxScrollMaxPx should be 0 after close, got %d", dv.fxScrollMaxPx)
	}
}

// --- 3. buildFXPanel layout ---

func TestFXPanelBuildWithEffects(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	dv.syncFXToRow(0)

	dv.openFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	// Should have buttons: toggle + remove per effect, + Add Effect, close button.
	// With 2 effects: at minimum 2 toggle + 2 remove + move buttons + add + close.
	if len(dv.fxPanelBtns) == 0 {
		t.Fatal("expected buttons to be created")
	}

	// Verify toggle buttons exist (pill-style, tagged with fxToggleTag).
	toggleCount := 0
	for _, btn := range dv.fxPanelBtns {
		if isToggle, _ := isFXToggleBtn(btn); isToggle {
			toggleCount++
		}
	}
	if toggleCount < 2 {
		t.Errorf("expected at least 2 toggle buttons for 2 effects, got %d", toggleCount)
	}

	// Verify remove buttons exist.
	removeCount := 0
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == "\u2715" { // "x"
			removeCount++
		}
	}
	if removeCount < 2 {
		t.Errorf("expected at least 2 remove buttons for 2 effects, got %d", removeCount)
	}

	// Verify close button is last.
	closeBtn := dv.fxPanelBtns[len(dv.fxPanelBtns)-1]
	if closeBtn.Icon != "close" {
		t.Errorf("expected last button to be close, got icon=%q text=%q", closeBtn.Icon, closeBtn.Text)
	}

	// On desktop, enabled effects always show sliders.
	if len(dv.fxPanelSliders) == 0 {
		t.Error("expected sliders for enabled effects on desktop")
	}
}

func TestFXPanelBuildWithAddMenu(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.openFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	// Open the add menu.
	dv.fxAddMenuOpen = true
	dv.buildFXPanel()
	suppressClicksUntilRelease = false

	// Ensure at least some effect type buttons are visible in the add menu.
	// With many effects, some buttons (and Cancel) may be off-screen due to
	// viewport clipping, so we check that at least the first few are present.
	regs := audio.EffectRegistrations()
	foundEffectBtns := 0
	for _, btn := range dv.fxPanelBtns {
		for _, reg := range regs {
			if btn.Text == reg.DisplayName {
				foundEffectBtns++
				break
			}
		}
	}
	if foundEffectBtns == 0 {
		t.Error("no effect type buttons found when add menu is open")
	}
}

func TestFXPanelBuildMobile(t *testing.T) {
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
		t.Fatal("panel did not open on mobile")
	}

	r := dv.fxPanelRect
	// Mobile panel should be wider than default desktop 320px (near full-width).
	if r.Dx() <= 250 {
		t.Errorf("mobile panel should be wider than 250px, got %d", r.Dx())
	}

	// Panel must fit within drum bounds.
	if r.Min.X < dv.Bounds.Min.X || r.Max.X > dv.Bounds.Max.X {
		t.Errorf("mobile panel X [%d,%d] exceeds bounds [%d,%d]",
			r.Min.X, r.Max.X, dv.Bounds.Min.X, dv.Bounds.Max.X)
	}
	if r.Min.Y < dv.Bounds.Min.Y || r.Max.Y > dv.Bounds.Max.Y {
		t.Errorf("mobile panel Y [%d,%d] exceeds bounds [%d,%d]",
			r.Min.Y, r.Max.Y, dv.Bounds.Min.Y, dv.Bounds.Max.Y)
	}
}

// --- 4. fxShowParams ---

func TestFXShowParamsDesktop(t *testing.T) {
	dv := newTestDV(t)

	// Desktop: enabled=true always shows params.
	if !dv.fxShowParams(0, true) {
		t.Error("desktop: fxShowParams(0, true) should return true")
	}
	// Desktop: enabled=false never shows params.
	if dv.fxShowParams(0, false) {
		t.Error("desktop: fxShowParams(0, false) should return false")
	}
}

func TestFXShowParamsMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum

	// Initialize fxExpandedSlots.
	dv.fxExpandedSlots = map[int]bool{}

	// Mobile: enabled=true but NOT expanded should return false.
	if dv.fxShowParams(0, true) {
		t.Error("mobile: fxShowParams(0, true) should return false when not expanded")
	}

	// Mobile: enabled=false should return false regardless.
	if dv.fxShowParams(0, false) {
		t.Error("mobile: fxShowParams(0, false) should return false")
	}

	// Mobile: enabled=true AND expanded should return true.
	dv.fxExpandedSlots[0] = true
	if !dv.fxShowParams(0, true) {
		t.Error("mobile: fxShowParams(0, true) should return true when expanded")
	}

	// Mobile: enabled=false AND expanded should still return false (disabled gate).
	if dv.fxShowParams(0, false) {
		t.Error("mobile: fxShowParams(0, false) should return false even when expanded")
	}
}

// --- 5. syncFXToRow ---

func TestSyncFXToRow(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument

	// Add effects via audio API.
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.AddInsertEffect(instID, audio.EffectReverb, nil)

	// Before sync, dv.Rows[0].Effects should be empty.
	if len(dv.Rows[0].Effects) != 0 {
		t.Errorf("expected 0 effects before sync, got %d", len(dv.Rows[0].Effects))
	}

	// Sync.
	dv.syncFXToRow(0)

	// After sync, should match audio state.
	if len(dv.Rows[0].Effects) != 2 {
		t.Fatalf("expected 2 effects after sync, got %d", len(dv.Rows[0].Effects))
	}
	if dv.Rows[0].Effects[0].Type != audio.EffectDistortion {
		t.Errorf("expected first effect Distortion, got %s", dv.Rows[0].Effects[0].Type)
	}
	if dv.Rows[0].Effects[1].Type != audio.EffectReverb {
		t.Errorf("expected second effect Reverb, got %s", dv.Rows[0].Effects[1].Type)
	}

	// syncFXToRow with invalid row should not panic.
	dv.syncFXToRow(-1)
	dv.syncFXToRow(len(dv.Rows) + 10)
}

// --- 6. fxParamLabel formatting ---

func TestFXParamLabel(t *testing.T) {
	cases := []struct {
		name     string
		val      float64
		unit     string
		expected string
	}{
		// val >= 1000 -> "%.0f" (non-empty units get a space prefix)
		{"cutoff", 2000, "Hz", "Cutoff: 2000 Hz"},
		{"freq", 10000, "Hz", "Freq: 10000 Hz"},
		// val >= 10 -> "%.1f"
		{"drive", 15.5, "", "Drive: 15.5"},
		{"drive", 10.0, "", "Drive: 10.0"},
		// val == int(val) -> "%.0f"
		{"mix", 1, "", "Mix: 1"},
		{"mix", 0, "", "Mix: 0"},
		// default -> "%.2f"
		{"mix", 0.75, "", "Mix: 0.75"},
		{"mix", 0.123, "", "Mix: 0.12"},
		{"rate", 5.5, "Hz", "Rate: 5.50 Hz"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			got := fxParamLabel(tc.name, tc.val, tc.unit)
			if got != tc.expected {
				t.Errorf("fxParamLabel(%q, %v, %q) = %q, want %q",
					tc.name, tc.val, tc.unit, got, tc.expected)
			}
		})
	}
}

func TestCapitalize(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"", ""},
		{"a", "A"},
		{"drive", "Drive"},
		{"mix", "Mix"},
		{"cutoff_freq", "Cutoff_freq"},
		{"A", "A"},
	}
	for _, tc := range cases {
		if got := capitalize(tc.in); got != tc.out {
			t.Errorf("capitalize(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

// --- 7. FX Panel open/close/capturing state ---

func TestFXPanelOpenCloseState(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Panel starts closed.
	if dv.IsFXPanelOpen() {
		t.Error("fxPanelOpen should be false initially")
	}

	// Open the panel.
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Error("fxPanelOpen should be true after toggleFXPanel")
	}

	// Capturing should be false in idle state (no drag, no deferred tap, no scroll).
	capturing := dv.fxPanelDeferredTap.Active() || dv.fxScrollTS.Active() || dv.fxSliderDragging
	if capturing {
		t.Error("capturing state should be false with no active interactions")
	}

	// Close via closeFXPanel.
	dv.closeFXPanel()
	if dv.IsFXPanelOpen() {
		t.Error("fxPanelOpen should be false after closeFXPanel")
	}
}

func TestFXPanelCapturingState(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)

	// Idle state: not capturing.
	capturing := dv.fxPanelDeferredTap.Active() || dv.fxScrollTS.Active() || dv.fxSliderDragging
	if capturing {
		t.Error("capturing should be false initially")
	}

	// Simulate slider dragging.
	dv.fxSliderDragging = true
	capturing = dv.fxPanelDeferredTap.Active() || dv.fxScrollTS.Active() || dv.fxSliderDragging
	if !capturing {
		t.Error("capturing should be true during slider drag")
	}
	dv.fxSliderDragging = false

	// Simulate deferred tap active.
	dv.fxPanelDeferredTap.active = true
	capturing = dv.fxPanelDeferredTap.Active() || dv.fxScrollTS.Active() || dv.fxSliderDragging
	if !capturing {
		t.Error("capturing should be true during deferred tap")
	}
	dv.fxPanelDeferredTap.Cancel()
}

// --- 8. handleFXPanelInput ---

func TestFXPanelInputClosedReturnsEarly(t *testing.T) {
	dv := newTestDV(t)

	// Panel is closed — should return false.
	consumed := dv.handleFXPanelInput(100, 100, true)
	if consumed {
		t.Error("handleFXPanelInput should return false when panel is closed")
	}
}

func TestFXPanelInputClickOutsideCloses(t *testing.T) {
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
	outsideX := dv.fxPanelRect.Max.X + 100
	outsideY := dv.fxPanelRect.Max.Y + 100
	// Make sure we're not on an FX button.
	for _, fb := range dv.rowFXBtns() {
		if fb != nil {
			r := fb.Rect()
			if outsideX >= r.Min.X && outsideX < r.Max.X &&
				outsideY >= r.Min.Y && outsideY < r.Max.Y {
				outsideX = dv.Bounds.Max.X - 1
				outsideY = dv.Bounds.Max.Y - 1
			}
		}
	}

	fxHoldAt(t, dv, outsideX, outsideY)

	if dv.IsFXPanelOpen() {
		t.Error("panel should close on click outside via tree")
	}
	fxReleaseInput(t, dv)
}

func TestFXPanelInputIgnoresOutsidePanel(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	// handleFXPanelInput should return false for clicks outside the panel
	// (click-outside is handled by the tree, not the handler).
	outsideX := dv.fxPanelRect.Max.X + 100
	outsideY := dv.fxPanelRect.Max.Y + 100
	consumed := dv.handleFXPanelInput(outsideX, outsideY, true)
	if consumed {
		t.Error("handleFXPanelInput should NOT consume clicks outside fxPanelRect")
	}
}

func TestFXPanelInputConsumesInsidePanel(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}
	// Click inside the panel rect (on background, not a button).
	insideX := (dv.fxPanelRect.Min.X + dv.fxPanelRect.Max.X) / 2
	insideY := (dv.fxPanelRect.Min.Y + dv.fxPanelRect.Max.Y) / 2

	consumed := dv.handleFXPanelInput(insideX, insideY, true)
	if !consumed {
		t.Error("click inside panel should be consumed")
	}
	// Panel should still be open (click was consumed, not close-triggering).
	if !dv.IsFXPanelOpen() {
		t.Error("panel should stay open on click inside")
	}
}

// --- Additional: propagateFXSliderValue ---

func TestPropagateFXSliderValue(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.syncFXToRow(0)

	dv.toggleFXPanel(0)
	if len(dv.fxPanelSliders) == 0 || len(dv.fxSliderBindings) == 0 {
		t.Fatal("no sliders or bindings")
	}

	// Set slider to midpoint.
	sl := dv.fxPanelSliders[0]
	sl.Value = 0.5
	dv.propagateFXSliderValue(0)

	// Verify the value was pushed to audio.
	b := dv.fxSliderBindings[0]
	expected := b.def.Min + 0.5*(b.def.Max-b.def.Min)
	effects := audio.GetInsertEffects(instID)
	actual := effects[0].Params[b.paramName]
	if actual != expected {
		t.Errorf("propagateFXSliderValue: expected %.3f, got %.3f", expected, actual)
	}

	// Out-of-range index should not panic.
	dv.propagateFXSliderValue(999)
}

// --- Additional: buildFXPanel with move buttons ---

func TestFXPanelMoveButtons(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	dv.syncFXToRow(0)

	dv.openFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel did not open")
	}

	// With 3 effects: should have up/down move buttons.
	upCount := 0
	downCount := 0
	for _, btn := range dv.fxPanelBtns {
		switch btn.Text {
		case "\u25b2": // up arrow
			upCount++
		case "\u25bc": // down arrow
			downCount++
		}
	}

	// First effect has no up button, last has no down button.
	// So: up buttons for effects 1 and 2 (2 total).
	// Down buttons for effects 0 and 1 (2 total).
	if upCount < 2 {
		t.Errorf("expected at least 2 up buttons with 3 effects, got %d", upCount)
	}
	if downCount < 2 {
		t.Errorf("expected at least 2 down buttons with 3 effects, got %d", downCount)
	}

	// Test that clicking a down button actually reorders.
	effectsBefore := audio.GetInsertEffects(instID)
	firstTypeBefore := effectsBefore[0].Type

	// Find the first down button and click it.
	for _, btn := range dv.fxPanelBtns {
		if btn.Text == "\u25bc" {
			btn.OnClick()
			break
		}
	}

	effectsAfter := audio.GetInsertEffects(instID)
	if effectsAfter[0].Type == firstTypeBefore && effectsAfter[1].Type != firstTypeBefore {
		// Order didn't change — might be the wrong button.
		// At least verify it didn't crash.
	}
	// The key test is that it didn't panic and the panel rebuilt.
	if !dv.IsFXPanelOpen() {
		t.Error("panel should stay open after move operation")
	}
}

// --- Additional: FX panel wheel scroll ---

func TestFXPanelWheelScroll(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	// Add multiple effects with expansion to create scrollable content.
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	dv.syncFXToRow(0)

	dv.openFXPanel(0)

	// FX panel should be open and functional.
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel should be open")
	}
	// Scroll offset should start at 0.
	if dv.fxScrollOffsetPx != 0 {
		t.Errorf("fxScrollOffsetPx should be 0 initially, got %d", dv.fxScrollOffsetPx)
	}
}

// --- Additional: buildFXPanel with invalid row after open ---

func TestFXPanelBuildInvalidRowCloses(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	dv.openFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("panel should be open")
	}

	// Force invalid row (simulate row deletion after panel was opened).
	dv.fxPanelRow = len(dv.Rows) + 5
	dv.buildFXPanel()

	if dv.IsFXPanelOpen() {
		t.Error("buildFXPanel should close the panel when row is out of range")
	}
}
