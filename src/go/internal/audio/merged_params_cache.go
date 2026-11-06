package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// merged_params_cache.go — revision-gated cache of an instrument's flat field
// view (instrument.FieldValues of its project document). The Synth tab reads
// it a dozen-plus times per frame (buildSynthTab, the mirror's params hash,
// every preview/concept wave, per-stage enable checks); re-deriving it each
// time was ~1.8 MB of garbage per frame and drove the WASM GC into 100 ms+
// stalls. Guarded by TestSynthTab_LiveControlEditFrameByteBudget in
// internal/ui. Invalidated by the document revision, never by comparison.

// InstrumentParamsRev returns the instrument's document revision (it changes
// whenever the project document for id changes). Unknown instruments return 0.
// The Synth tab's per-frame re-render gates key on it instead of hashing the
// whole flat view.
func InstrumentParamsRev(instrumentID instrument.ID) uint64 { return InstrumentDocRev(instrumentID) }

type mergedCacheEntry struct {
	rev    uint64
	params RecipeParams
}

var (
	mergedCacheMu sync.RWMutex
	mergedCache   = map[instrument.ID]mergedCacheEntry{}
)

// MergedInstrumentParamsRO returns the instrument's flat field view
// (instrument.FieldValues of its project document), cached until the document
// changes. READ-ONLY CONTRACT: callers must never mutate the returned map — it
// is shared across callers and frames.
func MergedInstrumentParamsRO(instrumentID instrument.ID) RecipeParams {
	rev := InstrumentDocRev(instrumentID)
	mergedCacheMu.RLock()
	e, ok := mergedCache[instrumentID]
	mergedCacheMu.RUnlock()
	if ok && e.rev == rev && e.params != nil {
		return e.params
	}
	d, ok := EnsureInstrumentDoc(instrumentID)
	if !ok {
		return RecipeParams{}
	}
	// The revision is captured BEFORE the read: a mutation landing in between
	// stores under the old rev and re-reads next time — never a stale serve.
	rev = InstrumentDocRev(instrumentID)
	merged := RecipeParams(instrument.FieldValues(d, BuiltinModuleRegistry()))
	mergedCacheMu.Lock()
	mergedCache[instrumentID] = mergedCacheEntry{rev: rev, params: merged}
	mergedCacheMu.Unlock()
	return merged
}
