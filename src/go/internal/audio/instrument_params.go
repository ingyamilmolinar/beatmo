package audio

import (
	"errors"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// instrument_params.go — the flat-field adapter over instrument documents
// (spec 2026-10-04 §6): the knob schema types the Synth tab and tools share,
// and SetInstrumentParam / GetInstrumentParams / ResetInstrumentParams, which
// read and edit an instrument's project document field by field.

// ParamDef declares one editable parameter on a SynthRecipe. The schema is
// shipped to UI editors and serialized as a plugin manifest, so the shape is a
// v1 freezing surface — see plan section "Plugin-readiness".
type ParamDef struct {
	Name    string  `json:"name"`
	Label   string  `json:"label,omitempty"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	Unit    string  `json:"unit,omitempty"` // "", "ms", "Hz", "dB", "%", "st"
	Curve   string  `json:"curve,omitempty"`
	Group   string  `json:"group,omitempty"`
	// Enum (v2, additive + omitempty) carries display labels for a DISCRETE
	// param. The param stays float-valued (Min:0, Max:len-1, Default:idx) so all
	// clamp/hash/merge/persist machinery is unchanged; Enum only tells the UI to
	// render a selector instead of a knob. A ParamDef without an enum serializes
	// exactly as v1 (no "enum" key), so existing recipe goldens are unaffected.
	Enum []string `json:"enum,omitempty"`
	// Step (v2, additive + omitempty) marks a non-enum param as DISCRETE with
	// detents at Min, Min+Step, ... Max. Enum params are implicitly Step:1.
	// A zero Step serializes identically to v1 (no "step" key), so existing
	// recipe goldens are unaffected.
	Step float64 `json:"step,omitempty"`
	// Tier (v3, additive + omitempty) is the P4 exposure priority in the Synth
	// tab: "" (TierEssential) = always shown; "advanced" (TierAdvanced) =
	// collapsed under the owning stage's "Advanced" expander so a beginner sees
	// only the 3-5 character knobs per stage. A zero Tier serializes identically
	// to v1/v2 (no "tier" key), so existing recipe goldens are unaffected.
	Tier string `json:"tier,omitempty"`
}

// ParamDef.Tier values (P4 tiered-knob exposure).
const (
	TierEssential = ""         // always shown (default)
	TierAdvanced  = "advanced" // collapsed under the stage's Advanced expander
)

// RecipeParams is the IPC-shaped parameter payload. Keys are ParamDef.Name.
type RecipeParams map[string]float64

// voiceCacheInvalidate drops cached voices for a single instrument. The
// native voice_cache.go (Phase 1.4) sets this to globalVoiceCache.ClearInstrument;
// under -tags test || js it stays a no-op (no real cache).
var voiceCacheInvalidate = func(instrumentID instrument.ID) {}

// SetInstrumentParam edits one flat field of an instrument's project document
// (spec §6, the sub-project-1 adapter: instrument.SetField). The value is run
// through sanitizeParamValue first — NaN/Inf are dropped, out-of-range values
// are clamped to the knob's range (widened to contain the document's own
// value). A field of a module the instrument does not have (an inactive
// source/layer type) or a type constant is ignored. Followed by the hook
// publish and the off-thread warm of the new document.
func SetInstrumentParam(instrumentID instrument.ID, name string, value float64) {
	start := time.Now()
	if !knownParamName(instrumentID, name) {
		// A name the engine does not have (retired / misspelled): refuse it.
		recordParamRejected()
		return
	}
	stored, ok := sanitizeParamValue(lookupParamDef(instrumentID, name), value)
	if !ok {
		recordParamRejected()
		return
	}
	doc, ok := EnsureInstrumentDoc(instrumentID)
	if !ok {
		recordParamRejected()
		return
	}
	reg := BuiltinModuleRegistry()
	// No-op skip: stationary drag bursts produce many same-value writes per
	// second; collapsing them here covers every caller (UI, JS bridge).
	if cur, ok := instrument.FieldValues(doc, reg)[name]; ok && cur == stored {
		if _, owned := instrument.OwnedFields(doc, reg)[name]; owned {
			recordParamCoalesced()
			return
		}
	}
	next, err := instrument.SetField(doc, reg, name, stored)
	if err != nil {
		if errors.Is(err, instrument.ErrInactive) || errors.Is(err, instrument.ErrNotSettable) {
			recordParamCoalesced()
			return
		}
		recordParamRejected()
		return
	}
	if err := SetInstrumentDoc(instrumentID, next); err != nil {
		recordParamRejected()
		return
	}
	// The voice cache is keyed on the flattened block, so the new value maps to
	// a DIFFERENT key — no invalidation (old-value entries survive for the
	// predictive warm; see SetInstrumentDoc).
	hooks.PublishKind(hooks.EventInstrumentParamChanged, hooks.InstrumentParamPayload{
		Instrument: instrumentID,
		Origin:     instrumentDocOrigin(instrumentID),
		Param:      name,
		Value:      stored,
	})
	notifyInstrumentBindingChanged(instrumentID, next)
	recordParamDispatch(time.Since(start))
}

// SetInstrumentParamsLive applies several fields to instrumentID's CURRENT
// project document as ONE edit — one document write, one push to the platform,
// one binding change (so the voice prewarmer renders one new sound) — for a
// control that moves several params per step (a Synth-tab macro dial). Each
// value is checked exactly as SetInstrumentParam checks it, a field that cannot
// land is skipped the same way, and every field whose value changed is
// announced like SetInstrumentParam announces it.
//
// N SetInstrumentParam calls are NOT equivalent: each publishes a half-applied
// document (only some of the fields moved), and the prewarmer renders every
// one of them at every pitch the row plays — sounds nobody will hear.
func SetInstrumentParamsLive(instrumentID instrument.ID, values map[string]float64) {
	start := time.Now()
	doc, ok := EnsureInstrumentDoc(instrumentID)
	if !ok {
		recordParamRejected()
		return
	}
	reg := BuiltinModuleRegistry()
	clean := make(map[string]float64, len(values))
	for name, v := range values {
		if !knownParamName(instrumentID, name) {
			recordParamRejected()
			continue
		}
		stored, ok := sanitizeParamValue(lookupParamDef(instrumentID, name), v)
		if !ok {
			recordParamRejected()
			continue
		}
		clean[name] = stored
	}
	next, _, err := applyFieldPasses(doc, clean, true)
	if err != nil {
		recordParamRejected()
		return
	}
	before, ownedBefore := instrument.FieldValues(doc, reg), instrument.OwnedFields(doc, reg)
	after, ownedAfter := instrument.FieldValues(next, reg), instrument.OwnedFields(next, reg)
	var changed []string
	for _, name := range sortedKeys(clean) {
		_, was := ownedBefore[name]
		_, is := ownedAfter[name]
		if after[name] != before[name] || was != is {
			changed = append(changed, name)
		}
	}
	if len(changed) == 0 {
		recordParamCoalesced()
		return
	}
	if err := SetInstrumentDoc(instrumentID, next); err != nil {
		recordParamRejected()
		return
	}
	origin := instrumentDocOrigin(instrumentID)
	for _, name := range changed {
		hooks.PublishKind(hooks.EventInstrumentParamChanged, hooks.InstrumentParamPayload{
			Instrument: instrumentID,
			Origin:     origin,
			Param:      name,
			Value:      after[name],
		})
	}
	notifyInstrumentBindingChanged(instrumentID, next)
	recordParamDispatch(time.Since(start))
}

// SetInstrumentParams makes the instrument's project document its LIBRARY
// document with these flat fields applied (each sanitized like
// SetInstrumentParam; unknown fields and fields that cannot land skipped). Used by the JS bridge
// and offline renders; an empty map resets to the library document.
func SetInstrumentParams(instrumentID instrument.ID, p RecipeParams) {
	doc, ok := libraryCopyFor(instrumentID)
	if !ok {
		return
	}
	clean := make(map[string]float64, len(p))
	for name, v := range p {
		if !knownParamName(instrumentID, name) {
			continue
		}
		if stored, ok := sanitizeParamValue(lookupParamDef(instrumentID, name), v); ok {
			clean[name] = stored
		}
	}
	// Multi-pass: a field of a layer the set itself adds lands after its source.
	doc, _, _ = applyFieldPasses(doc, clean, false)
	if cur, ok := InstrumentDoc(instrumentID); ok && len(instrument.Diff(cur, doc)) == 0 {
		return // no-op skip (undo/redo re-applies whole sets)
	}
	if err := SetInstrumentDoc(instrumentID, doc); err != nil {
		return
	}
	// Bulk replace wants a clean slate (as it always did): drop the old voices.
	voiceCacheInvalidate(instrumentID)
	notifyInstrumentBindingChanged(instrumentID, doc)
}

// libraryCopyFor is the library document id's project copy starts from: the
// entry named by its current Origin (or the id / its instance base).
func libraryCopyFor(instrumentID instrument.ID) (instrument.Doc, bool) {
	origin := instrumentID
	if cur, ok := InstrumentDoc(instrumentID); ok && cur.Origin != "" {
		origin = cur.Origin
	}
	d, ok := LibraryDoc(origin)
	if !ok {
		base := instrumentID.Base()
		if d, ok = LibraryDoc(base); !ok {
			return instrument.Doc{}, false
		}
		origin = base
	}
	d.ID, d.Origin = instrumentID, origin
	return d, true
}

// ModifiedInstrumentDocCountForTest returns how many project documents differ from
// their library entry — the soak tests' leak gauge (bounded by the instruments
// actually edited).
func ModifiedInstrumentDocCountForTest() int {
	n := 0
	docStore.RLock()
	ids := make([]instrument.ID, 0, len(docStore.docs))
	for id := range docStore.docs {
		ids = append(ids, id)
	}
	docStore.RUnlock()
	for _, id := range ids {
		if InstrumentDocModified(id) {
			n++
		}
	}
	return n
}

// GetInstrumentParams returns the flat fields the instrument's document owns
// (instrument.OwnedFields). Callers may mutate the returned map.
func GetInstrumentParams(instrumentID instrument.ID) RecipeParams {
	d, ok := InstrumentDoc(instrumentID)
	if !ok {
		return RecipeParams{}
	}
	return RecipeParams(instrument.OwnedFields(d, BuiltinModuleRegistry()))
}

// ResetInstrumentParams returns the instrument's project document to its
// library entry (spec §6 Reset).
func ResetInstrumentParams(instrumentID instrument.ID) {
	d, ok := libraryCopyFor(instrumentID)
	if !ok {
		return
	}
	if cur, ok := InstrumentDoc(instrumentID); ok && len(instrument.Diff(cur, d)) == 0 {
		return
	}
	if err := SetInstrumentDoc(instrumentID, d); err != nil {
		return
	}
	voiceCacheInvalidate(instrumentID)
	notifyInstrumentBindingChanged(instrumentID, d)
}

// InstrumentDocModified reports whether the instrument's project document
// differs from the library entry it came from (the Synth tab's dirty dot).
// Cached by (document rev, library rev): the Synth tab asks every frame.
func InstrumentDocModified(instrumentID instrument.ID) bool {
	key := modifiedKey{doc: InstrumentDocRev(instrumentID), lib: libraryRev.Load()}
	modifiedMu.Lock()
	if e, ok := modifiedCache[instrumentID]; ok && e.key == key {
		modifiedMu.Unlock()
		return e.modified
	}
	modifiedMu.Unlock()
	cur, ok := InstrumentDoc(instrumentID)
	if !ok {
		return false
	}
	lib, ok := libraryCopyFor(instrumentID)
	modified := ok && len(instrument.Diff(cur, lib)) > 0
	modifiedMu.Lock()
	modifiedCache[instrumentID] = modifiedEntry{key: key, modified: modified}
	modifiedMu.Unlock()
	return modified
}

type modifiedKey struct{ doc, lib uint64 }

type modifiedEntry struct {
	key      modifiedKey
	modified bool
}

var (
	modifiedMu    sync.Mutex
	modifiedCache = map[instrument.ID]modifiedEntry{}
)
