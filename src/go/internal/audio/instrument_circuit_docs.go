package audio

import (
	"maps"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// instrument_circuit_docs.go — opening a circuit (the startup demo, a project
// file, a template). A circuit carries a document for every instrument it uses,
// but the user's SAVED version of an instrument (Synth-tab Save → the user
// library, instruments.json / localStorage) is their personal tone and trumps
// whatever the circuit carries for the SAME instrument. The circuit's own
// documents are remembered: they are what Reset returns a copy to.
//
// Undo/redo re-imports are NOT opens: they restore a snapshot verbatim through
// ReplaceInstrumentDocs (the snapshot already holds the effective documents),
// and swap the remembered circuit state through CircuitSnapshot /
// RestoreCircuit.

// CircuitState is what the open circuit carried for its instruments: each
// one's document and the name its rows were given. Opaque to the undo journal;
// never mutated in place (every change installs new maps).
type CircuitState struct {
	Docs  map[instrument.ID]instrument.Doc
	Names map[instrument.ID]string
}

var circuitDocs = struct {
	sync.RWMutex
	state CircuitState
}{}

// circuitGen counts circuit changes: every open (SetCircuitInstrumentDocs)
// and every undo/redo that swaps the circuit back (RestoreCircuit).
var circuitGen atomic.Uint64

// CircuitGeneration identifies the open circuit: it changes whenever another
// circuit is opened or restored, never while one is being edited. The Synth
// tab anchors what it learns about an instrument (which dials are relevant)
// to it.
func CircuitGeneration() uint64 { return circuitGen.Load() }

// InstrumentLibraryIdentity is the library instrument a project document is a
// copy of: its Origin, else — for a document that names none — the instance
// base of its id ("kick--2" → "kick"). Two documents with the same identity are
// the same instrument (the user's Save of one applies to both).
func InstrumentLibraryIdentity(id instrument.ID, d instrument.Doc) instrument.ID {
	if d.Origin != "" {
		return d.Origin
	}
	return id.Base()
}

// OpenCircuitInstrumentDocs makes a circuit's documents the project's (see
// ReplaceInstrumentDocs), except that an instrument the user has SAVED gets the
// user's saved document instead (same row id, Origin = the saved entry, so it
// reads clean). The circuit's own documents become Reset's target. Nothing
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

// CircuitInstrumentDoc returns a copy of the document the open circuit carried
// for id (false when the circuit did not carry one).
func CircuitInstrumentDoc(id instrument.ID) (instrument.Doc, bool) {
	circuitDocs.RLock()
	defer circuitDocs.RUnlock()
	d, ok := circuitDocs.state.Docs[id]
	if !ok {
		return instrument.Doc{}, false
	}
	return d.Clone(), true
}

// CircuitInstrumentName returns the name the open circuit gave id's rows ("" =
// the instrument's default name) and whether the circuit had a row for id.
func CircuitInstrumentName(id instrument.ID) (string, bool) {
	circuitDocs.RLock()
	defer circuitDocs.RUnlock()
	n, ok := circuitDocs.state.Names[id]
	return n, ok
}

// SetCircuitInstrumentName records the name the open circuit gives id's rows
// (import, while opening a circuit).
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

// RestoreCircuit reinstates a CircuitSnapshot (an undo/redo crossing a circuit
// open restores the circuit that was open at that step).
func RestoreCircuit(s CircuitState) {
	circuitDocs.Lock()
	circuitDocs.state = s
	circuitDocs.Unlock()
	circuitGen.Add(1)
}

// SetCircuitInstrumentDocs replaces the open circuit's documents and forgets
// its row names (a new circuit is being opened; import records the names).
func SetCircuitInstrumentDocs(docs map[instrument.ID]instrument.Doc) {
	circuitDocs.Lock()
	circuitDocs.state = CircuitState{Docs: maps.Clone(docs)}
	circuitDocs.Unlock()
	circuitGen.Add(1)
}

// ResetInstrumentToCircuit is the Synth tab's Reset: id's project copy returns
// to the document the open circuit carried for it, or — for an instrument the
// circuit did not carry (picked this session) — to its library entry.
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
