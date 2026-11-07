package audio

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A generator stage's off state is silence, not identity, so the Synth tab greys out a pill whose stage the sound
// cannot exist without: a beginner should not silence FM Bass with its only oscillator. Reads only the param map.

// The sources are the OSC stage and the gen-bank slots (gain > 0; the kick voice also gated by kick_enabled). A
// generator gate is essential only while it is the LAST source; an already-off OSC stays disableable so it can come back.
func StageDisableSilences(instID instrument.ID, enableParam string) bool {
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
	// Absent the gate, the source 5 slot still sounds.
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
