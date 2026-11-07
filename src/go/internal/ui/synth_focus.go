package ui

// The one control the graph explains: session UI state only, VALIDATED on read (a switched instrument or a
// hidden dial reads as no focus), so no switch or toggle site has to clear it.

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// synthFocus names a focused macro dial (card + dial id) on one instrument.
type synthFocus struct {
	inst       instrument.ID
	card, dial string
}

// Every focus is a gesture start, so it also snapshots the graph's BEFORE ghost.
func (dv *DrumView) setSynthFocusDial(inst instrument.ID, card, dial string) {
	dv.synthFocusState = synthFocus{inst: inst, card: card, dial: dial}
	dv.captureGraphBefore(inst, dialCtl(card, dial))
}

// clearSynthFocus drops the focus (the graph returns to the overview).
func (dv *DrumView) clearSynthFocus() { dv.synthFocusState = synthFocus{} }

func (dv *DrumView) synthFocusedDial(inst instrument.ID) (card, dial string, ok bool) {
	f := dv.synthFocusState
	if f.inst == "" || f.inst != inst {
		return "", "", false
	}
	if _, ok := dv.synthDialByID(f.card, f.dial); !ok {
		return "", "", false
	}
	return f.card, f.dial, true
}

func (dv *DrumView) focusCardFirstDial(inst instrument.ID, card string) {
	cg := dv.macroConceptByID(card)
	if cg == nil || len(cg.Dials) == 0 {
		return
	}
	dv.setSynthFocusDial(inst, card, cg.Dials[0].Dial.ID)
}

func (dv *DrumView) clearSynthFocusIfShown() bool {
	if dv.eqPanelZone == nil || dv.eqPanelZone.ActiveTab() != TabSynth {
		return false
	}
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if _, _, ok := dv.synthFocusedDial(inst); !ok {
		return false
	}
	dv.clearSynthFocus()
	dv.invalidateSynthPanel()
	return true
}

// Behind the cards: a tap on empty space drops the focus, a sweep scrolls.
type synthFocusClearAdapter struct {
	dv  *DrumView
	arb dragScrollArbiter
}

func (h *synthFocusClearAdapter) OnPress(x, y int) InputResult {
	h.arb.begin(y, h.dv.instEditorMacroGrid)
	return InputCaptured
}

func (h *synthFocusClearAdapter) OnDrag(x, y int) {
	if h.arb.drag(y) {
		h.dv.invalidateSynthPanel()
	}
}

func (h *synthFocusClearAdapter) OnRelease(x, y int) {
	if !h.arb.end() {
		return
	}
	h.dv.clearSynthFocus()
	h.dv.invalidateSynthPanel()
}

func (h *synthFocusClearAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }
