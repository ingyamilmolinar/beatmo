package audio

import (
	"errors"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Shipped to UI editors and serialized as a plugin manifest: a frozen surface, extended only additively.
type ParamDef struct {
	Name    string  `json:"name"`
	Label   string  `json:"label,omitempty"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	Unit    string  `json:"unit,omitempty"` // "", "ms", "Hz", "dB", "%", "st"
	Curve   string  `json:"curve,omitempty"`
	Group   string  `json:"group,omitempty"`
	// Display labels for a DISCRETE param, which stays float-valued (0..len-1) so clamp/hash/persist are unchanged.
	Enum []string `json:"enum,omitempty"`
	// Detents at Min, Min+Step, … Max; enum params are implicitly Step 1.
	Step float64 `json:"step,omitempty"`
	// "" (TierEssential) is always shown; "advanced" collapses under the stage's More expander.
	Tier string `json:"tier,omitempty"`
}

// ParamDef.Tier values (P4 tiered-knob exposure).
const (
	TierEssential = ""         // always shown (default)
	TierAdvanced  = "advanced" // collapsed under the stage's Advanced expander
)

// RecipeParams is the IPC-shaped parameter payload. Keys are ParamDef.Name.
type RecipeParams map[string]float64

// A no-op under -tags test || js, which have no voice cache.
var voiceCacheInvalidate = func(instrumentID instrument.ID) {}

// NaN/Inf are dropped and out-of-range values clamped to the knob's range; a field of a module the instrument does not
// have, or a type constant, is ignored.
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
	// A stationary drag produces many same-value writes per second; collapsing them here covers every caller.
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
	// The new value is a different cache key, so nothing is invalidated: old entries survive for the predictive warm.
	hooks.PublishKind(hooks.EventInstrumentParamChanged, hooks.InstrumentParamPayload{
		Instrument: instrumentID,
		Origin:     instrumentDocOrigin(instrumentID),
		Param:      name,
		Value:      stored,
	})
	notifyInstrumentBindingChanged(instrumentID, next)
	recordParamDispatch(time.Since(start))
}

// ONE edit for a control that moves several params per step (a macro dial). N SetInstrumentParam calls would each
// publish a half-applied document, and the prewarmer renders every one at every pitch the row plays.
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

// The LIBRARY document with these fields applied; an empty map resets to it. For the JS bridge and offline renders.
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

// The entry named by its Origin, or the id / its instance base.
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

// The soak tests' leak gauge: bounded by the instruments actually edited.
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

// Callers may mutate the returned map.
func GetInstrumentParams(instrumentID instrument.ID) RecipeParams {
	d, ok := InstrumentDoc(instrumentID)
	if !ok {
		return RecipeParams{}
	}
	return RecipeParams(instrument.OwnedFields(d, BuiltinModuleRegistry()))
}

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

// Cached by (document rev, library rev): asked every frame.
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
