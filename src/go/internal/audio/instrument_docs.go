package audio

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"slices"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// instrument_docs.go — the PROJECT's instrument documents (spec §6): one copy
// per instrument id in use (rows are keyed by instrument id). The only input
// of a synth voice render. A copy's Origin names the library entry it came
// from (Save / Reset / the dirty dot).

// docEntry is a stored document plus its flattened block and its voice hash,
// computed once per change (the dispatch path reads them per trigger). The
// hash covers everything a voice render reads: the block AND the render
// metadata + voice FX (salt), which Flatten does not write.
type docEntry struct {
	doc   instrument.Doc
	block []float64
	salt  uint64
	hash  uint64
}

func newDocEntry(d instrument.Doc) docEntry {
	b := instrument.Flatten(d, BuiltinModuleRegistry())
	salt := renderMetaHash(d)
	return docEntry{doc: d, block: b, salt: salt, hash: instrument.BlockHash(b) ^ salt}
}

// renderMetaHash hashes the parts of a document a voice render reads that the
// flattened block does not carry: render metadata (beats, seconds, amplitude,
// gate tail, melodic) and the voice FX chain. Two documents with the same
// block but a different amplitude must not share a cached voice.
func renderMetaHash(d instrument.Doc) uint64 {
	b, err := json.Marshal(struct {
		R instrument.Render
		F []instrument.Module
	}{d.Render, d.FX})
	if err != nil {
		panic("audio: renderMetaHash: " + err.Error())
	}
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}

var docStore = struct {
	sync.RWMutex
	docs map[instrument.ID]docEntry
	rev  map[instrument.ID]uint64
}{docs: map[instrument.ID]docEntry{}, rev: map[instrument.ID]uint64{}}

// platformInstrumentDocChanged is replaced on js/wasm to push the flattened
// block to audio.js (instrument_docs_wasm.go).
var platformInstrumentDocChanged = func(id instrument.ID, d instrument.Doc) {}

// SwapPlatformInstrumentDocChangedForTest replaces the platform push hook and
// returns the previous one (a nil fn leaves it unchanged). Cross-package tests
// count document pushes through it; production never calls it.
func SwapPlatformInstrumentDocChangedForTest(fn func(id instrument.ID, d instrument.Doc)) func(id instrument.ID, d instrument.Doc) {
	prev := platformInstrumentDocChanged
	if fn != nil {
		platformInstrumentDocChanged = fn
	}
	return prev
}

// InstrumentDoc returns a copy of the project's document for id.
func InstrumentDoc(id instrument.ID) (instrument.Doc, bool) {
	docStore.RLock()
	defer docStore.RUnlock()
	e, ok := docStore.docs[id]
	if !ok {
		return instrument.Doc{}, false
	}
	return e.doc.Clone(), true
}

// HasInstrumentDoc reports whether id has a project document (no copy).
func HasInstrumentDoc(id instrument.ID) bool {
	docStore.RLock()
	defer docStore.RUnlock()
	_, ok := docStore.docs[id]
	return ok
}

// InstrumentDocRev bumps on every change of id's document (cache keys).
func InstrumentDocRev(id instrument.ID) uint64 {
	docStore.RLock()
	defer docStore.RUnlock()
	return docStore.rev[id]
}

// SetInstrumentDoc validates d and makes it id's project document. The voice
// cache is NOT invalidated: it is keyed on the flattened block's hash, so the
// next trigger misses on its own and other pitches' entries survive (the
// same contract SetInstrumentParam always had).
func SetInstrumentDoc(id instrument.ID, d instrument.Doc) error {
	d = d.Clone()
	d.ID = id
	if err := instrument.Validate(d, BuiltinModuleRegistry()); err != nil {
		return fmt.Errorf("instrument %s: %w", id, err)
	}
	e := newDocEntry(d)
	docStore.Lock()
	docStore.docs[id] = e
	docStore.rev[id]++
	docStore.Unlock()
	platformInstrumentDocChanged(id, d)
	return nil
}

// ResetInstrumentDocs re-initialises the store with a copy of every library
// document (Origin = its id) and forgets the open circuit's documents (no
// circuit is open). Called by ResetInstruments.
func ResetInstrumentDocs() {
	SetCircuitInstrumentDocs(nil)
	fresh := map[instrument.ID]docEntry{}
	for _, id := range LibraryIDs() {
		if d, ok := LibraryDoc(id); ok {
			d.Origin = id
			fresh[id] = newDocEntry(d)
		}
	}
	docStore.Lock()
	docStore.docs = fresh
	for id := range fresh {
		docStore.rev[id]++
	}
	docStore.Unlock()
	for id, e := range fresh {
		platformInstrumentDocChanged(id, e.doc)
	}
}

// ReplaceInstrumentDocs makes entries the project's documents (a project
// import — and so every undo/redo, which re-imports): each entry replaces its
// id's copy, and every other library instrument (and instance "x--N") returns
// to its library copy, so nothing from the previous project survives (replace,
// not merge). Only documents that actually change are stored and pushed — a
// push re-renders that voice in the browser, and one undo must not re-render
// the whole roster. An entry whose id this machine has never seen (a Save-As
// instrument from another library) becomes a playable instrument. Every entry
// is validated before anything changes.
func ReplaceInstrumentDocs(entries map[instrument.ID]instrument.Doc) error {
	reg := BuiltinModuleRegistry()
	for id, d := range entries {
		d.ID = id
		if err := instrument.Validate(d, reg); err != nil {
			return fmt.Errorf("instrument %s: %w", id, err)
		}
	}
	docStore.RLock()
	ids := make(map[instrument.ID]bool, len(docStore.docs))
	for id := range docStore.docs {
		ids[id] = true
	}
	docStore.RUnlock()
	for _, id := range LibraryIDs() {
		ids[id] = true
	}
	for id := range entries {
		ids[id] = true
	}
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		want, ok := entries[id]
		if !ok {
			if want, ok = libraryCopyOf(id); !ok {
				continue // not the file's, not the library's: left as it is
			}
		}
		want.ID = id
		if cur, had := InstrumentDoc(id); had && cur.Origin == want.Origin && len(instrument.Diff(cur, want)) == 0 {
			continue
		}
		if err := SetInstrumentDoc(id, want); err != nil {
			return err
		}
	}
	for id := range entries {
		if !instanceAlreadyRegistered(id) {
			registerDocInstrument(id)
		}
	}
	return nil
}

// libraryCopyOf is the copy a pick of id starts from: its library entry, or —
// for an instance id "x--N" — x's entry (Origin = the entry it came from).
func libraryCopyOf(id instrument.ID) (instrument.Doc, bool) {
	if d, ok := LibraryDoc(id); ok {
		d.Origin = id
		return d, true
	}
	base := id.Base()
	if base == id {
		return instrument.Doc{}, false
	}
	d, ok := LibraryDoc(base)
	if !ok {
		return instrument.Doc{}, false
	}
	d.Origin = base
	return d, true
}

// EnsureInstrumentDoc returns id's project document, creating it from the
// library when absent: the library entry for id, or — for an instance id
// "x--N" — x's entry under the new id (Origin = x).
func EnsureInstrumentDoc(id instrument.ID) (instrument.Doc, bool) {
	if d, ok := InstrumentDoc(id); ok {
		return d, true
	}
	d, ok := libraryCopyOf(id)
	if !ok {
		return instrument.Doc{}, false
	}
	if err := SetInstrumentDoc(id, d); err != nil {
		return instrument.Doc{}, false
	}
	return InstrumentDoc(id)
}

// IsMelodicInstrument reports whether id renders AT the node pitch (its
// document's render.melodic): the project copy when there is one, else the
// library document for id or its instance base — tools ask before any project
// exists. False for an id with no document (a sample, an unknown id).
func IsMelodicInstrument(id instrument.ID) bool {
	docStore.RLock()
	e, ok := docStore.docs[id]
	docStore.RUnlock()
	if ok {
		return e.doc.Render.Melodic
	}
	d, ok := libraryCopyOf(id)
	return ok && d.Render.Melodic
}

// instrumentDocOrigin returns id's project document Origin ("" when none),
// without copying the document.
func instrumentDocOrigin(id instrument.ID) instrument.ID {
	docStore.RLock()
	defer docStore.RUnlock()
	return docStore.docs[id].doc.Origin
}

// instrumentDocMelodic reports whether id's project document renders at the
// node pitch, without copying the document (UI draw loop safe).
func instrumentDocMelodic(id instrument.ID) bool {
	docStore.RLock()
	defer docStore.RUnlock()
	return docStore.docs[id].doc.Render.Melodic
}

// docBlockForDispatch returns id's stored flattened block (READ-ONLY, shared),
// its voice hash and its render-metadata salt without copying anything — the
// dispatch hot path.
func docBlockForDispatch(id instrument.ID) ([]float64, uint64, uint64, bool) {
	docStore.RLock()
	defer docStore.RUnlock()
	e, ok := docStore.docs[id]
	return e.block, e.hash, e.salt, ok
}

// removeInstrumentDoc drops id's project document.
func removeInstrumentDoc(id instrument.ID) {
	docStore.Lock()
	_, had := docStore.docs[id]
	delete(docStore.docs, id)
	docStore.rev[id]++
	docStore.Unlock()
	if had {
		platformInstrumentDocRemoved(id)
	}
}

// platformInstrumentDocRemoved is replaced on js/wasm (clearInstrumentBlock).
var platformInstrumentDocRemoved = func(id instrument.ID) {}

// InstallInstrumentDocForTest gives instrument id a project document copied
// from library entry libraryID (Origin = libraryID). Test seam.
func InstallInstrumentDocForTest(id, libraryID instrument.ID) error {
	d, ok := LibraryDoc(libraryID)
	if !ok {
		return fmt.Errorf("no library instrument %q", libraryID)
	}
	d.Origin = libraryID
	return SetInstrumentDoc(id, d)
}

// RemoveInstrumentDocForTest drops id's project document. Test seam.
func RemoveInstrumentDocForTest(id instrument.ID) { removeInstrumentDoc(id) }

// MigrateAndValidateInstrumentDoc is the read boundary for a document that
// arrives from outside the running app (a project file, a bundle): migrate it
// to current module versions, then validate it.
func MigrateAndValidateInstrumentDoc(d instrument.Doc) (instrument.Doc, error) {
	reg := BuiltinModuleRegistry()
	m, err := instrument.Migrate(d, reg)
	if err != nil {
		return instrument.Doc{}, err
	}
	if err := instrument.Validate(m, reg); err != nil {
		return instrument.Doc{}, err
	}
	return m, nil
}

// InstrumentDocWithFields returns id's project document (created from the
// library when absent) with each field set through it (instrument.SetField,
// sorted by name). A field the document does not have a module for, or a type
// constant, is skipped — the same tolerance SetInstrumentParam has; an unknown
// field name is an error, so a typo in a tool's override map cannot silently
// render the unedited voice.
func InstrumentDocWithFields(id instrument.ID, fields map[string]float64) (instrument.Doc, error) {
	d, ok := EnsureInstrumentDoc(id)
	if !ok {
		return instrument.Doc{}, fmt.Errorf("instrument %q has no document", id)
	}
	d, _, err := ApplyInstrumentFields(d, fields)
	if err != nil {
		return instrument.Doc{}, fmt.Errorf("instrument %q: %w", id, err)
	}
	return d, nil
}

// ApplyInstrumentFields writes every field onto d through instrument.SetField
// and returns the fields that did not land (inactive or not settable on the
// resulting document, or discarded by a later type switch). Any other error
// aborts.
//
// A structural field can make others settable — gen9_source creates layer 9,
// whose gen9_freq / gen9_env_* sort BEFORE it — so the inactive fields are
// retried, pass after pass, until a pass lands nothing new.
func ApplyInstrumentFields(d instrument.Doc, fields map[string]float64) (instrument.Doc, []string, error) {
	return applyFieldPasses(d, fields, true)
}

// applyFieldPasses is ApplyInstrumentFields; strict=false treats EVERY SetField
// error as "did not land" instead of aborting (the bulk writer's per-field skip).
func applyFieldPasses(d instrument.Doc, fields map[string]float64, strict bool) (instrument.Doc, []string, error) {
	reg := BuiltinModuleRegistry()
	pending := sortedKeys(fields)
	for {
		var skipped []string
		for _, name := range pending {
			next, err := instrument.SetField(d, reg, name, fields[name])
			switch {
			case err == nil:
				d = next
			case !strict || errors.Is(err, instrument.ErrInactive) || errors.Is(err, instrument.ErrNotSettable):
				skipped = append(skipped, name)
			default:
				return instrument.Doc{}, nil, err
			}
		}
		if len(skipped) == 0 || len(skipped) == len(pending) {
			// A field that landed and was then discarded by a later type
			// switch (it belongs only to the old type) did not land either.
			got := instrument.FieldValues(d, reg)
			var lost []string
			for _, name := range sortedKeys(fields) {
				if got[name] != fields[name] {
					lost = append(lost, name)
				}
			}
			return d, lost, nil
		}
		pending = skipped
	}
}

// ShippedSoundKey identifies the SOUND of a shipped instrument (its flattened
// block plus render metadata): two ids with the same key render identically.
// "" when id is not shipped. Tools use it to ask "is this variant just its
// base's sound?".
func ShippedSoundKey(id instrument.ID) string {
	d, ok := ShippedDoc(id)
	if !ok {
		return ""
	}
	b := instrument.Flatten(d, BuiltinModuleRegistry())
	return fmt.Sprintf("%016x|%+v|%d", instrument.BlockHash(b), d.Render, len(d.FX))
}

// FlatFieldMap is d's flattened param block keyed by ABI field name — EVERY
// field, so a consumer that rebuilds the block from the map (the browser half
// of the cross-platform reference) needs no identity table of its own.
func FlatFieldMap(d instrument.Doc) map[string]float64 {
	b := instrument.Flatten(d, BuiltinModuleRegistry())
	out := make(map[string]float64, len(b))
	for i, name := range modularParamSchema {
		out[name] = b[i]
	}
	return out
}

// ModularParamNames is the engine's param-block field order (ABI names).
func ModularParamNames() []string { return append([]string(nil), modularParamSchema...) }

// LibraryFieldValue is field's value in the library entry id's project copy
// comes from (its Origin, or the id / its instance base): what the instrument
// ships (or was saved) with. ok=false when there is no library entry or the
// entry's modules do not hold field.
func LibraryFieldValue(id instrument.ID, field string) (float64, bool) {
	origin := id
	if o := instrumentDocOrigin(id); o != "" {
		origin = o
	}
	d, ok := libraryCopyOf(origin)
	if !ok {
		return 0, false
	}
	v, ok := instrument.FieldValues(d, BuiltinModuleRegistry())[field]
	return v, ok
}

// InstrumentSoundHash identifies what id sounds like with overrides set through
// its document: the flattened block's hash plus render metadata, as hex. ""
// when id has no document or an override names an unknown field. Tools record
// it next to an ear verdict to tell later whether the sound has changed.
func InstrumentSoundHash(id instrument.ID, overrides map[string]float64) string {
	d, err := InstrumentDocWithFields(id, overrides)
	if err != nil {
		return ""
	}
	b := instrument.Flatten(d, BuiltinModuleRegistry())
	return fmt.Sprintf("%016x-%x", instrument.BlockHash(b), instrument.BlockHash([]float64{d.Render.Seconds, d.Render.Amplitude, d.Render.GateTail, float64(len(d.FX))}))
}
