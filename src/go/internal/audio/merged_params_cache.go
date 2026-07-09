package audio

import (
	"sync"
	"sync/atomic"
)

// merged_params_cache.go — revision-gated cache for the merged
// (recipe defaults ⊕ per-instrument overrides) param map.
//
// MergeRecipeDefaults clones the recipe's FULL schema per call — ~1030
// entries / ~115 KB for a modular recipe. The Synth tab used to re-derive it
// a dozen-plus times per frame (buildSynthTab, the mirror's params hash,
// every preview/concept wave, per-stage enable checks), producing ~1.8 MB of
// garbage per frame; on the single-threaded WASM heap that churn drove the
// GC into 100 ms+ stalls whenever the tab was open, and worse during a live
// knob drag ("editing the kick Punch knob during playback hangs the game").
// Guarded by TestSynthTab_LiveKnobEditFrameByteBudget in internal/ui.
//
// The cache is invalidated by two monotonic revision counters instead of by
// content comparison:
//   - a per-instrument revision, bumped by every mutation that changes the
//     instrument's contribution to the merge (SetInstrumentParam[s],
//     ResetInstrumentParams, BindInstrumentToRecipe), and
//   - a process-wide recipe-defaults revision, bumped by every mutation that
//     can change ANY recipe's registered defaults (RegisterRecipe,
//     updateRecipeDefaults, unregisterRecipeForTest). Coarse on purpose:
//     defaults changes are user-rare (Save/Reset/import), so a global bump
//     costs one extra re-merge per instrument, not correctness.

// recipeDefaultsRev increments whenever any registered recipe's defaults can
// have changed. Read via RecipeDefaultsRev.
var recipeDefaultsRev atomic.Uint64

// bumpRecipeDefaultsRev marks all cached merged params stale. Call after any
// mutation of a RecipeRegistration's ParamDef defaults (or the set of
// registered recipes).
func bumpRecipeDefaultsRev() { recipeDefaultsRev.Add(1) }

// RecipeDefaultsRev returns the process-wide recipe-defaults revision. It
// changes whenever a Save / Reset / plugin (re-)registration may have changed
// some recipe's default params.
func RecipeDefaultsRev() uint64 { return recipeDefaultsRev.Load() }

// InstrumentParamsRev returns the per-instrument params revision: it changes
// whenever the instrument's overlay params or recipe binding change. Unknown
// instruments return 0. Together with RecipeDefaultsRev this is a stable
// O(1) identity for "did the merged param map change?" — the Synth tab's
// per-frame re-render gates key on it instead of hashing ~1030 floats.
func InstrumentParamsRev(instrumentID string) uint64 {
	m := instrumentParamsMgr
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.revs[instrumentID]
}

type mergedCacheEntry struct {
	instRev uint64
	defsRev uint64
	params  RecipeParams
}

var (
	mergedCacheMu sync.RWMutex
	mergedCache   = map[string]mergedCacheEntry{}
)

// MergedInstrumentParamsRO returns the instrument's merged (defaults ⊕
// overrides) param map, cached until either revision moves. READ-ONLY
// CONTRACT: callers must never mutate the returned map — it is shared across
// callers and frames. Paths that need to overlay or mutate (trigger-time
// pitch injection, export deltas) must keep using MergeRecipeDefaults, which
// always returns a fresh map.
func MergedInstrumentParamsRO(instrumentID string) RecipeParams {
	instRev := InstrumentParamsRev(instrumentID)
	defsRev := recipeDefaultsRev.Load()
	mergedCacheMu.RLock()
	e, ok := mergedCache[instrumentID]
	mergedCacheMu.RUnlock()
	if ok && e.instRev == instRev && e.defsRev == defsRev {
		return e.params
	}
	// Revisions are captured BEFORE the merge: if a mutation lands in between,
	// the entry is stored under the pre-mutation revs, fails the next revision
	// check, and re-merges — one wasted merge, never a stale serve.
	merged := MergeRecipeDefaults(RecipeForInstrument(instrumentID), GetInstrumentParams(instrumentID))
	mergedCacheMu.Lock()
	mergedCache[instrumentID] = mergedCacheEntry{instRev: instRev, defsRev: defsRev, params: merged}
	mergedCacheMu.Unlock()
	return merged
}
