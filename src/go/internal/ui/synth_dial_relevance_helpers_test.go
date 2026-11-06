package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// shownRootMacros is the category cards a shipped instrument's Synth root
// shows: its config-live dials minus the ones the DSP found inert on its
// sound (the generated verdict, synth_dial_relevance.go), at voiceSecs.
func shownRootMacros(id instrument.ID, voiceSecs float64) []macroGroup {
	schema := audio.InstrumentParamDefs(id)
	merged := audio.MergedInstrumentParamsRO(id)
	macros := buildMacroGroupsForVoice(schema, merged, voiceSecs)
	if d, ok := audio.InstrumentDoc(id); ok {
		if hidden, ok := shippedDialRelevance(audio.SoundSignature(d)); ok {
			return withoutDials(macros, hidden)
		}
	}
	return macros
}
