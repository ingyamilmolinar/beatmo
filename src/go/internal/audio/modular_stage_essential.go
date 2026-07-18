package audio

import "fmt"

// modular_stage_essential.go — "which pipeline stage is the sound's sole
// source?" No build tag: it reads only the merged param map (cross-platform,
// available under test, js/wasm, and native), never the cgo engine.
//
// The Synth tab lets the user bypass any stage that carries a per-stage
// `*_enabled` toggle. For a SHAPER stage (filter, envelope, LFO, drive, …) the
// off-state is identity — the sound survives. For a GENERATOR stage the off
// state is silence: "the generator is the sound source, so bypass means no
// source" (src/c/modular.c). StageDisableSilences distinguishes the two so the
// UI can grey out (make non-interactive) an enable pill whose stage the sound
// cannot exist without — a beginner should not be able to silence FM Bass by
// toggling off the one oscillator that is its whole voice.

// StageDisableSilences reports whether disabling the pipeline stage gated by
// enableParam would leave instID with no audible sound source — i.e. the stage
// is the instrument's SOLE generator, so bypassing it is silence, not shaping.
//
// The modular pipeline has two kinds of sound source:
//   - the OSC stage (`osc_enabled`, an oscillator/FM/physical voice), and
//   - the parallel gen-bank slots (`gen{N}_source` > 0 with `gen{N}_gain` > 0);
//     the harmonic-bank kick voice (source 5) is additionally gated by
//     `kick_enabled`.
//
// Every other `*_enabled` toggle gates a processor/shaper and can always be
// bypassed, so this returns false for them. A generator gate returns true only
// when it is the LAST remaining source: OSC layered over a gen-slot (e.g.
// cello) stays disableable, and an already-disabled OSC is disableable so it
// can be turned back on.
func StageDisableSilences(instID, enableParam string) bool {
	if instID == "" || enableParam == "" {
		return false
	}
	switch enableParam {
	case "osc_enabled", "kick_enabled":
		// Generator gate — decide by counting the other audible sources below.
	default:
		// Processor / shaper stage: off-state is identity, never silence.
		return false
	}

	merged := MergedInstrumentParamsRO(instID)
	if len(merged) == 0 {
		return false
	}

	// OSC is a source only when the recipe declares the stage AND it is on.
	oscAudible := false
	if v, ok := merged["osc_enabled"]; ok {
		oscAudible = v >= 0.5
	}
	// The gen-bank kick voice runs unless explicitly gated off; when the gate is
	// absent the source==5 slot still sounds (matches the engine default).
	kickGate := true
	if v, ok := merged["kick_enabled"]; ok {
		kickGate = v >= 0.5
	}

	// Inventory the gen-bank: audible non-kick slots vs. audible kick slots.
	nonKick, kick := 0, 0
	for k := 1; k <= modularGenSlots; k++ {
		src := int(merged[fmt.Sprintf("gen%d_source", k)] + 0.5)
		if src <= 0 || merged[fmt.Sprintf("gen%d_gain", k)] <= 0 {
			continue
		}
		if src == 5 { // harmonic-bank kick voice
			if kickGate {
				kick++
			}
			continue
		}
		nonKick++
	}

	switch enableParam {
	case "osc_enabled":
		// The oscillator is the sole source iff it is on and nothing else sounds.
		return oscAudible && nonKick == 0 && kick == 0
	case "kick_enabled":
		// The kick voice is the sole source iff it is the only thing audible.
		return kick > 0 && !oscAudible && nonKick == 0
	}
	return false
}
