//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// ResetInstruments restores the built-in instrument set.
func ResetInstruments() {
	// Every synth instrument is an instrument DOCUMENT (spec 2026-10-04): the
	// project store is re-initialised from the library, and each library id
	// plays through docInstrument (doc_voice_native.go).
	ResetInstrumentDocs()
	instMu.Lock()
	instruments = map[instrument.ID]Instrument{}
	for _, id := range LibraryIDs() {
		instruments[id] = docInstrument{id: id}
	}
	instOrder = append(append([]instrument.ID(nil), roster.All...), userOnlyLibraryIDs()...)
	// Snapshot the as-shipped instrument set so a factory Reset can restore a
	// single instrument's built-in render after the Sampler's Save overwrote it
	// with a chopped Sample (UnregisterSamplePCM).
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
