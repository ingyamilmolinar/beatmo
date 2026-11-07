//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// ResetInstruments restores the built-in instrument set.
func ResetInstruments() {
	// Every synth instrument is a DOCUMENT: the project store is re-initialised from the library.
	ResetInstrumentDocs()
	instMu.Lock()
	instruments = map[instrument.ID]Instrument{}
	for _, id := range LibraryIDs() {
		instruments[id] = docInstrument{id: id}
	}
	instOrder = append(append([]instrument.ID(nil), roster.All...), userOnlyLibraryIDs()...)
	// So one instrument's built-in render can be restored after a Sample replaced it.
	factoryInstruments = make(map[instrument.ID]Instrument, len(instruments))
	for id, inst := range instruments {
		factoryInstruments[id] = inst
	}
	instMu.Unlock()
	globalVoiceCache.Clear()
	bumpInstrumentsVersion()
	ClearAllInsertEffects()
	resetInstrumentChannels()
	clearInstrumentDisplayNames()
}
