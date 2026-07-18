package ui

// synthPanel is the Synth audio-panel tab, extracted from the DrumView
// God-object (Phase 2b). It mirrors samplerPanel (see sampler_panel.go): the
// synth methods are grouped onto this collaborator via a `dv *DrumView`
// back-reference, and every relocated method keeps a one-line *DrumView
// forwarding shim so all 262 existing synth tests drive it through DrumView
// unchanged. Synth state fields stay on DrumView for now (Rung-A cohesion move,
// zero behavior change).
//
// Cross-tab helpers deliberately stay on DrumView (NOT relocated here) because
// non-synth code calls them: knobStepPref, persistKnobStep, masterChannelSelected,
// samplerViewDisabled, synthViewDisabled, activeInstrumentHasSynth. The synth
// methods reach them via the dv back-reference.
type synthPanel struct {
	dv *DrumView // back-reference

	// sprites holds the Synth tab's TraceSprite render caches (Task 2 of the
	// render-caching plan, synth_sprite_cache.go): the full-note/mirror/focus
	// trace draws blit a cached texture on steady frames instead of re-walking
	// their pixel loops. Released on tab-hide via EQCallbacks.ReleaseSynthSprites
	// (releaseHiddenTabSprites chokepoint in eq_panel_zone.go).
	sprites synthSpriteCache
}

// synp returns the DrumView's synth panel, lazily creating it if absent. The
// ctor sets dv.synthPanel eagerly; this getter is the self-healing path for the
// many test files that build *DrumView via a bare &DrumView{} literal bypassing
// the ctor. Every synth forwarding shim calls dv.synp().X() so those tests (and
// any future bare-literal construction) never nil-panic on a relocated method.
func (dv *DrumView) synp() *synthPanel {
	if dv.synthPanel == nil {
		p := &synthPanel{dv: dv}
		// Drag-rate throttle for the three trace sprites, matching the EQ
		// curve sprite's cap (traceSpriteDragInterval — ~30 Hz, the
		// user-approved staleness for surfaces whose key churns during a
		// live knob drag).
		p.sprites.fullNote = TraceSprite{tag: "synthFullNote", minInterval: traceSpriteDragInterval}
		p.sprites.mirror = TraceSprite{tag: "synthMirror", minInterval: traceSpriteDragInterval}
		p.sprites.focus = TraceSprite{tag: "synthFocus", minInterval: traceSpriteDragInterval}
		dv.synthPanel = p
	}
	return dv.synthPanel
}
