package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestOpenSynthControlWheelPopup verifies that openSynthControlWheelPopup opens the
// mobile wheel popup and closeSynthControlWheelPopup closes it.
//
// Approach: uses the newModularSynthTabGame + expandSynthPanelForTest harness
// (same as knob_interaction_functional_test.go) to get a fully-wired DrumView
// with populated instEditorControls/instEditorBindings, then calls open/close.
// "ut-chip-modular" bound to "synth-modular" provides a continuous knob
// (filter_cutoff) at a known param name.
func TestOpenSynthControlWheelPopup(t *testing.T) {
	// Build the game first (newModularSynthTabGame calls assertDefaultParityState
	// which requires forceSmallScreenForTest==false), then override to mobile.
	g := newModularSynthTabGame(t)
	withSmallScreen(t, true)
	expandSynthPanelForTest(t, g)

	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)

	dv := g.drum
	audio.BindInstrumentToRecipe(instID, "synth-modular")

	dv.openSynthControlWheelPopup(idx, instID)
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		t.Fatal("openSynthControlWheelPopup should open the wheel popup")
	}

	dv.closeSynthControlWheelPopup()
	if dv.synthWheelPopup.IsOpen() {
		t.Fatal("closeSynthControlWheelPopup should close it")
	}
}

// ── Test-only helpers on *DrumView ───────────────────────────────────────────

// synthWheelTestDV builds the standard modular synth-tab DrumView used by the
// wheel-seam tests: modular synth bound to "ut-chip-modular", synth tab
// expanded, filter_cutoff section selected so the knob has a non-empty rect.
// Call withSmallScreen BEFORE calling this if you want mobile layout.
func synthWheelTestDV(t *testing.T) *DrumView {
	t.Helper()
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)
	audio.BindInstrumentToRecipe(instID, "synth-modular")
	return g.drum
}

// firstContinuousSynthControlIdx returns the binding index of filter_cutoff (the
// canonical continuous knob used by the wheel-seam tests).
func (dv *DrumView) firstContinuousSynthControlIdx(t *testing.T) int {
	t.Helper()
	for i, b := range dv.instEditorBindings {
		if b.def.Name == "filter_cutoff" {
			return i
		}
	}
	t.Fatal("firstContinuousSynthControlIdx: filter_cutoff not found in instEditorBindings")
	return -1
}

// activeSynthInstID returns the instrument ID of the first drum row (the row
// configured by the wheel-seam test harness).
func (dv *DrumView) activeSynthInstID(t *testing.T) string {
	t.Helper()
	if len(dv.Rows) == 0 {
		t.Fatal("activeSynthInstID: no rows")
	}
	return dv.Rows[0].Instrument
}

// ── Platform-swap seam tests ─────────────────────────────────────────────────

// TestMobileTapKnobOpensWheel_DesktopStillDrags verifies the mobile knob-tap
// seam: on mobile, OnPress must return InputConsumed and open the wheel popup.
func TestMobileTapKnobOpensWheel_DesktopStillDrags(t *testing.T) {
	// Build game with assertDefaultParityState (forceSmallScreen==false), THEN
	// switch to mobile so withSmallScreen's precondition passes.
	g := newModularSynthTabGame(t)
	withSmallScreen(t, true)
	expandSynthPanelForTest(t, g)

	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)
	audio.BindInstrumentToRecipe(instID, "synth-modular")

	dv := g.drum
	k := dv.instEditorControls[idx]
	kr := k.Rect()
	if kr.Empty() {
		t.Fatal("filter_cutoff knob has empty rect on mobile; check expandSynthPanelForTest layout")
	}

	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: instID}
	cx, cy := kr.Min.X+kr.Dx()/2, kr.Min.Y+kr.Dy()/2
	res := h.OnPress(cx, cy)
	if res != InputCaptured {
		t.Fatalf("mobile control tap should CAPTURE the gesture (so a sweep can scroll), got %v", res)
	}
	if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
		t.Fatal("mobile control tap must not open the wheel on press; it opens on release")
	}
	h.OnRelease(cx, cy)
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		t.Fatal("mobile control tap must open the wheel popup on release")
	}
}

// TestMobileWheelEndToEndDragChangesAudioValue opens the mobile scroll-wheel
// for a continuous knob (filter_cutoff), drags down by 20 notches, then asserts
// the knob's value has increased. The assertion validates the full path:
// HandleInput → applyValueDelta → Knob.NudgeEndless → value change.
//
// Setup order: build game first (assertDefaultParityState requires
// forceSmallScreenForTest==false), then switch to mobile, then expand the
// panel — same pattern as TestOpenSynthControlWheelPopup.
func TestMobileWheelEndToEndDragChangesAudioValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	withSmallScreen(t, true)
	expandSynthPanelForTest(t, g)

	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)
	audio.BindInstrumentToRecipe(instID, "synth-modular")

	dv := g.drum
	startVal := dv.instEditorControls[idx].Value
	dv.openSynthControlWheelPopup(idx, instID)
	w := dv.synthWheelPopup

	// Press in the lower quarter of the barrel — well outside the center-value
	// band (centerH spans [cy-pillH/2, cy+pillH/2] around the vertical midpoint)
	// so the first press starts a drag rather than opening the numeric editor.
	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()*3/4
	w.HandleInput(cx, y0, true)
	// Drag down by 20 notches (y increases → higher rows to center → value increases).
	y1 := y0 + knobEndlessPxPerNotch*20
	w.HandleInput(cx, y1, true)
	w.HandleInput(cx, y1, false)

	if dv.instEditorControls[idx].Value <= startVal {
		t.Fatalf("end-to-end drag should raise the knob value from %v", startVal)
	}
}

// TestMobileWheelCenterTapOpensNumericEditor verifies that a tap on the
// center value box of the wheel popup opens the numeric param editor.
//
// Setup order: build game first (assertDefaultParityState requires
// forceSmallScreenForTest==false), then switch to mobile — same pattern
// as TestOpenSynthControlWheelPopup.
func TestMobileWheelCenterTapOpensNumericEditor(t *testing.T) {
	g := newModularSynthTabGame(t)
	withSmallScreen(t, true)
	expandSynthPanelForTest(t, g)

	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)
	audio.BindInstrumentToRecipe(instID, "synth-modular")

	dv := g.drum
	dv.openSynthControlWheelPopup(idx, instID)
	w := dv.synthWheelPopup

	cx := (w.centerH.Min.X + w.centerH.Max.X) / 2
	cy := (w.centerH.Min.Y + w.centerH.Max.Y) / 2
	w.HandleInput(cx, cy, true)

	if dv.paramEditor == nil || !dv.paramEditor.Active() {
		t.Fatal("tapping the center value should open the numeric editor")
	}
}

// TestDesktopTapKnobOpensWheel verifies the Task-5 universal tap-to-popup: on
// desktop OnPress now returns InputConsumed and OPENS the wheel popup (the old
// in-place rotary drag was retired), mirroring mobile.
func TestDesktopTapKnobOpensWheel(t *testing.T) {
	withSmallScreen(t, false)
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)
	audio.BindInstrumentToRecipe(instID, "synth-modular")
	dv := g.drum
	k := dv.instEditorControls[idx]
	kr := k.Rect()
	if kr.Empty() {
		t.Fatal("filter_cutoff knob has empty rect on desktop")
	}
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: instID}
	cx, cy := kr.Min.X+kr.Dx()/2, kr.Min.Y+kr.Dy()/2
	res := h.OnPress(cx, cy)
	if res != InputCaptured {
		t.Fatalf("desktop control tap should CAPTURE the gesture (so a drag can scroll), got %v", res)
	}
	if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
		t.Fatal("desktop control tap must not open the wheel on press; it opens on release")
	}
	h.OnRelease(cx, cy)
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		t.Fatal("desktop control tap must open the wheel popup on release (universal tap-to-popup)")
	}
}
