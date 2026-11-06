package ui

// synth_focus.go — the Synth tab's FOCUS: the one control the graph explains
// (spec docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md
// §4.2). Pressing, dragging or opening the wheel on a dial focuses it in both
// modes; in the production UI a card tap focuses the card's first dial (the
// card no longer drills). Focus is sticky until another control takes it.
//
// It is session UI state only (never persisted, never undoable), and it is
// VALIDATED on read: a focus whose instrument is no longer the one shown, or
// whose card/dial the current card set does not carry (an instrument switch, a
// dial hidden by its stage), reads as no focus. So no instrument-switch or
// stage-toggle site has to remember to clear it.

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// synthFocus names a focused macro dial (card + dial id) on one instrument.
type synthFocus struct {
	inst       instrument.ID
	card, dial string
}

// setSynthFocusDial focuses dial `dial` of card `card` on inst.
//
// Every focus is a gesture start (a press, a wheel open, a card tap), so it
// also snapshots the graph's BEFORE ghost: the sound as it was.
func (dv *DrumView) setSynthFocusDial(inst instrument.ID, card, dial string) {
	dv.synthFocusState = synthFocus{inst: inst, card: card, dial: dial}
	dv.captureGraphBefore(inst, dialCtl(card, dial))
}

// clearSynthFocus drops the focus (the graph returns to the overview).
func (dv *DrumView) clearSynthFocus() { dv.synthFocusState = synthFocus{} }

// synthFocusedDial returns the focused dial on inst, when the focus names inst
// and the current card set still carries that dial.
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

// focusCardFirstDial focuses the first live dial of card on inst (the
// production card tap). No-op when the card has no dial.
func (dv *DrumView) focusCardFirstDial(inst instrument.ID, card string) {
	cg := dv.macroConceptByID(card)
	if cg == nil || len(cg.Dials) == 0 {
		return
	}
	dv.setSynthFocusDial(inst, card, cg.Dials[0].Dial.ID)
}

// clearSynthFocusIfShown drops the focus when the Synth tab is showing a
// focused dial for its instrument (Esc). Reports whether it did.
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

// synthFocusClearAdapter covers the root's card area BEHIND the cards: a tap
// on empty space drops the focus (the graph returns to the overview); a
// vertical sweep scrolls the cards like the plain scroll body it replaces.
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
