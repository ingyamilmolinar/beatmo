package audio

import (
	"maps"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The user's SAVED version of an instrument trumps whatever an opened circuit carries for it. Undo/redo re-imports
// are not opens: they restore a snapshot verbatim and swap the remembered circuit state back.

// Opaque to the undo journal; never mutated in place.
type CircuitState struct {
	Docs  map[instrument.ID]instrument.Doc
	Names map[instrument.ID]string
}

var circuitDocs = struct {
	sync.RWMutex
	state CircuitState
}{}

var circuitGen atomic.Uint64

// Changes when another circuit is opened or restored, never while one is edited; the Synth tab anchors dial relevance
// to it.
func CircuitGeneration() uint64 { return circuitGen.Load() }

// Origin, else the instance base of its id ("kick--2" → "kick"). Two documents with one identity are the same
// instrument, so a Save of one applies to both.
func InstrumentLibraryIdentity(id instrument.ID, d instrument.Doc) instrument.ID {
	if d.Origin != "" {
		return d.Origin
	}
	return id.Base()
}

// An instrument the user has saved gets the saved document instead (same row id, Origin = the saved entry). Nothing
// changes when an entry fails validation.
func OpenCircuitInstrumentDocs(entries map[instrument.ID]instrument.Doc) error {
	effective := make(map[instrument.ID]instrument.Doc, len(entries))
	for id, d := range entries {
		key := InstrumentLibraryIdentity(id, d)
		if saved, ok := UserLibraryDoc(key); ok {
			saved.ID, saved.Origin = id, key
			effective[id] = saved
			continue
		}
		effective[id] = d
	}
	if err := ReplaceInstrumentDocs(effective); err != nil {
		return err
	}
	circuit := make(map[instrument.ID]instrument.Doc, len(entries))
	for id, d := range entries {
		d = d.Clone()
		d.ID = id
		circuit[id] = d
	}
	SetCircuitInstrumentDocs(circuit)
	return nil
}

func CircuitInstrumentDoc(id instrument.ID) (instrument.Doc, bool) {
	circuitDocs.RLock()
	defer circuitDocs.RUnlock()
	d, ok := circuitDocs.state.Docs[id]
	if !ok {
		return instrument.Doc{}, false
	}
	return d.Clone(), true
}

// "" means the instrument's default name.
func CircuitInstrumentName(id instrument.ID) (string, bool) {
	circuitDocs.RLock()
	defer circuitDocs.RUnlock()
	n, ok := circuitDocs.state.Names[id]
	return n, ok
}

func SetCircuitInstrumentName(id instrument.ID, name string) {
	circuitDocs.Lock()
	next := maps.Clone(circuitDocs.state.Names)
	if next == nil {
		next = map[instrument.ID]string{}
	}
	next[id] = name
	circuitDocs.state.Names = next
	circuitDocs.Unlock()
}

// CircuitSnapshot returns the open circuit's state (for the undo journal).
func CircuitSnapshot() CircuitState {
	circuitDocs.RLock()
	defer circuitDocs.RUnlock()
	return circuitDocs.state
}

// An undo/redo crossing a circuit open restores the circuit open at that step.
func RestoreCircuit(s CircuitState) {
	circuitDocs.Lock()
	circuitDocs.state = s
	circuitDocs.Unlock()
	circuitGen.Add(1)
}

// Forgets the row names: import records the new circuit's.
func SetCircuitInstrumentDocs(docs map[instrument.ID]instrument.Doc) {
	circuitDocs.Lock()
	circuitDocs.state = CircuitState{Docs: maps.Clone(docs)}
	circuitDocs.Unlock()
	circuitGen.Add(1)
}

// An instrument the circuit did not carry returns to its library entry.
func ResetInstrumentToCircuit(id instrument.ID) {
	d, ok := CircuitInstrumentDoc(id)
	if !ok {
		ResetInstrumentParams(id)
		return
	}
	if cur, ok := InstrumentDoc(id); ok && cur.Origin == d.Origin && len(instrument.Diff(cur, d)) == 0 {
		return
	}
	if err := SetInstrumentDoc(id, d); err != nil {
		return
	}
	voiceCacheInvalidate(id)
	notifyInstrumentBindingChanged(id, d)
}
