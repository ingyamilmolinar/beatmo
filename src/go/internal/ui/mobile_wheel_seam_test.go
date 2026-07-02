package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestOpenSynthKnobWheelPopup verifies that openSynthKnobWheelPopup opens the
// mobile wheel popup and closeSynthKnobWheelPopup closes it.
//
// Approach: uses the newModularSynthTabGame + expandSynthPanelForTest harness
// (same as knob_interaction_functional_test.go) to get a fully-wired DrumView
// with populated instEditorKnobs/instEditorBindings, then calls open/close.
// "ut-chip-modular" bound to "synth-modular" provides a continuous knob
// (filter_cutoff) at a known param name.
func TestOpenSynthKnobWheelPopup(t *testing.T) {
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

	dv.openSynthKnobWheelPopup(idx, instID)
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		t.Fatal("openSynthKnobWheelPopup should open the wheel popup")
	}

	dv.closeSynthKnobWheelPopup()
	if dv.synthWheelPopup.IsOpen() {
		t.Fatal("closeSynthKnobWheelPopup should close it")
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

// firstContinuousSynthKnobIdx returns the binding index of filter_cutoff (the
// canonical continuous knob used by the wheel-seam tests).
func (dv *DrumView) firstContinuousSynthKnobIdx(t *testing.T) int {
	t.Helper()
	for i, b := range dv.instEditorBindings {
		if b.def.Name == "filter_cutoff" {
			return i
		}
	}
	t.Fatal("firstContinuousSynthKnobIdx: filter_cutoff not found in instEditorBindings")
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
	k := dv.instEditorKnobs[idx]
	kr := k.Rect()
	if kr.Empty() {
		t.Fatal("filter_cutoff knob has empty rect on mobile; check expandSynthPanelForTest layout")
	}

	h := &synthKnobHitAdapter{dv: dv, idx: idx, instID: instID}
	res := h.OnPress(kr.Min.X+kr.Dx()/2, kr.Min.Y+kr.Dy()/2)
	if res != InputConsumed {
		t.Fatalf("mobile knob tap should be consumed to open the wheel, got %v", res)
	}
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		t.Fatal("mobile knob tap must open the wheel popup")
	}
}

// TestDesktopTapKnobStartsDrag verifies the desktop path is unchanged: OnPress
// must return InputCaptured and must NOT open the wheel popup.
func TestDesktopTapKnobStartsDrag(t *testing.T) {
	// withSmallScreen(t, false) keeps desktop profile and satisfies the
	// precondition check (forceSmallScreen==false → prev==false, ok).
	withSmallScreen(t, false)
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)

	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)
	audio.BindInstrumentToRecipe(instID, "synth-modular")

	dv := g.drum
	k := dv.instEditorKnobs[idx]
	kr := k.Rect()
	if kr.Empty() {
		t.Fatal("filter_cutoff knob has empty rect on desktop; check expandSynthPanelForTest layout")
	}

	h := &synthKnobHitAdapter{dv: dv, idx: idx, instID: instID}
	res := h.OnPress(kr.Min.X+kr.Dx()/2, kr.Min.Y+kr.Dy()/2)
	if res != InputCaptured {
		t.Fatalf("desktop knob tap should start a drag (InputCaptured), got %v", res)
	}
	if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
		t.Fatal("desktop knob tap must NOT open the wheel popup")
	}
}

// TestMobileWheelEndToEndDragChangesAudioValue opens the mobile scroll-wheel
// for a continuous knob (filter_cutoff), drags up by 20 notches, then asserts
// the knob's value has increased. The assertion validates the full path:
// HandleInput → applyValueDelta → Knob.NudgeEndless → value change.
//
// Setup order: build game first (assertDefaultParityState requires
// forceSmallScreenForTest==false), then switch to mobile, then expand the
// panel — same pattern as TestOpenSynthKnobWheelPopup.
func TestMobileWheelEndToEndDragChangesAudioValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	withSmallScreen(t, true)
	expandSynthPanelForTest(t, g)

	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)
	audio.BindInstrumentToRecipe(instID, "synth-modular")

	dv := g.drum
	startVal := dv.instEditorKnobs[idx].Value
	dv.openSynthKnobWheelPopup(idx, instID)
	w := dv.synthWheelPopup

	// Press in the lower quarter of the barrel — well outside the center-value
	// band (centerH spans [cy-pillH/2, cy+pillH/2] around the vertical midpoint)
	// so the first press starts a drag rather than opening the numeric editor.
	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()*3/4
	w.HandleInput(cx, y0, true)
	// Drag up by 20 notches (y decreases → positive delta → value increases).
	y1 := y0 - knobEndlessPxPerNotch*20
	w.HandleInput(cx, y1, true)
	w.HandleInput(cx, y1, false)

	if dv.instEditorKnobs[idx].Value <= startVal {
		t.Fatalf("end-to-end drag should raise the knob value from %v", startVal)
	}
}

// TestMobileWheelCenterTapOpensNumericEditor verifies that a tap on the
// center value box of the wheel popup opens the numeric param editor.
//
// Setup order: build game first (assertDefaultParityState requires
// forceSmallScreenForTest==false), then switch to mobile — same pattern
// as TestOpenSynthKnobWheelPopup.
func TestMobileWheelCenterTapOpensNumericEditor(t *testing.T) {
	g := newModularSynthTabGame(t)
	withSmallScreen(t, true)
	expandSynthPanelForTest(t, g)

	const instID = "ut-chip-modular"
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, instID, idx)
	audio.BindInstrumentToRecipe(instID, "synth-modular")

	dv := g.drum
	dv.openSynthKnobWheelPopup(idx, instID)
	w := dv.synthWheelPopup

	cx := (w.centerH.Min.X + w.centerH.Max.X) / 2
	cy := (w.centerH.Min.Y + w.centerH.Max.Y) / 2
	w.HandleInput(cx, cy, true)

	if dv.paramEditor == nil || !dv.paramEditor.Active() {
		t.Fatal("tapping the center value should open the numeric editor")
	}
}
