package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Config-live dials minus the ones the DSP found inert on the sound.
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
