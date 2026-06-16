package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// synth_ghost.go — per-knob "ghost" capture/fade state machine (Stage 5).
//
// When a synth knob drag starts, we snapshot the instrument's effective params
// (shipped defaults ⊕ user edits) for that knob index. While the drag is live
// (and for a short fade-out after release) the concept renderers can read the
// snapshot via synthConceptGhost and draw a faint "before" overlay so the user
// sees the delta their drag is making.
//
// State lives on DrumView (lazily initialised). Each entry carries a per-knob
// fade countdown ticked once per frame by advanceSynthGhost (called from the
// Draw decay tick). Capture resets the fade to full; release starts the fade.

// synthGhostFadeFrames is how many frames a ghost lingers after release before
// it is cleared. While a knob is held the fade is pinned at this value (capture
// resets it every press), so the ghost stays visible for the whole drag.
const synthGhostFadeFrames = 30

// synthGhostEntry is the per-knob snapshot plus its fade countdown.
type synthGhostEntry struct {
	params map[string]float64
	fade   int  // frames remaining before the entry is cleared
	held   bool // true between capture and fadeSynthGhost (drag in progress)
}

// synthGhostState maps knob index → ghost entry.
type synthGhostState struct {
	byKnob map[int]*synthGhostEntry
}

// ensureSynthGhostState lazily initialises the per-DrumView ghost store.
func (dv *DrumView) ensureSynthGhostState() *synthGhostState {
	if dv.synthGhost == nil {
		dv.synthGhost = &synthGhostState{byKnob: make(map[int]*synthGhostEntry)}
	}
	if dv.synthGhost.byKnob == nil {
		dv.synthGhost.byKnob = make(map[int]*synthGhostEntry)
	}
	return dv.synthGhost
}

// captureSynthGhost snapshots the instrument's effective params for knob kIdx,
// resetting that knob's fade to full. The map is COPIED so later live-param
// edits don't mutate the captured ghost.
func (dv *DrumView) captureSynthGhost(kIdx int, instID string) {
	if dv == nil || instID == "" {
		return
	}
	merged := audio.MergeRecipeDefaults(audio.RecipeForInstrument(instID), audio.GetInstrumentParams(instID))
	snap := make(map[string]float64, len(merged))
	for k, v := range merged {
		snap[k] = v
	}
	st := dv.ensureSynthGhostState()
	st.byKnob[kIdx] = &synthGhostEntry{params: snap, fade: synthGhostFadeFrames, held: true}
}

// synthConceptGhost returns the captured pre-drag snapshot for knob kIdx while
// it is still active (captured and not yet faded out), else nil.
func (dv *DrumView) synthConceptGhost(kIdx int) map[string]float64 {
	if dv == nil || dv.synthGhost == nil {
		return nil
	}
	e := dv.synthGhost.byKnob[kIdx]
	if e == nil || e.fade <= 0 {
		return nil
	}
	return e.params
}

// fadeSynthGhost begins the fade-out for knob kIdx (called on drag release).
// The entry stays visible until advanceSynthGhost ticks its countdown to zero.
func (dv *DrumView) fadeSynthGhost(kIdx int) {
	if dv == nil || dv.synthGhost == nil {
		return
	}
	if e := dv.synthGhost.byKnob[kIdx]; e != nil {
		e.held = false
	}
}

// advanceSynthGhost ticks every released ghost's fade down by one frame and
// clears entries whose fade reaches zero. Held ghosts (drag in progress) do not
// fade. Cheap no-op when no ghosts are active.
func (dv *DrumView) advanceSynthGhost() {
	if dv == nil || dv.synthGhost == nil || len(dv.synthGhost.byKnob) == 0 {
		return
	}
	for k, e := range dv.synthGhost.byKnob {
		if e == nil {
			delete(dv.synthGhost.byKnob, k)
			continue
		}
		if e.held {
			continue
		}
		e.fade--
		if e.fade <= 0 {
			delete(dv.synthGhost.byKnob, k)
		}
	}
}
