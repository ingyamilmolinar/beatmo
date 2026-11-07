package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The Synth tab reads an instrument's flat view many times per frame; re-deriving it each time drove the WASM GC into
// 100 ms+ stalls. Invalidated by the document revision, never by comparison.

// 0 for an unknown instrument. Per-frame re-render gates key on it instead of hashing the flat view.
func InstrumentParamsRev(instrumentID instrument.ID) uint64 { return InstrumentDocRev(instrumentID) }

type mergedCacheEntry struct {
	rev    uint64
	params RecipeParams
}

var (
	mergedCacheMu sync.RWMutex
	mergedCache   = map[instrument.ID]mergedCacheEntry{}
)

// READ-ONLY: shared across callers and frames.
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
	// Captured BEFORE the read: a mutation in between stores under the old rev and re-reads next time, never a stale serve.
	rev = InstrumentDocRev(instrumentID)
	merged := RecipeParams(instrument.FieldValues(d, BuiltinModuleRegistry()))
	mergedCacheMu.Lock()
	mergedCache[instrumentID] = mergedCacheEntry{rev: rev, params: merged}
	mergedCacheMu.Unlock()
	return merged
}
