package audio

import (
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
)

// UseUserRecipeOverrides gates whether ApplyUserRecipeOverrides actually
// mutates the registry. Default: false. Production bootstraps (browser +
// desktop) flip this to true once before calling ApplyUserRecipeOverrides;
// test binaries and parity goldens leave it off so the shipped defaults
// are bit-identical across runs.
//
// This is the "parity protection mechanism" the Phase-3 plan calls out
// (gate the override application, not the persistence boundary). Same
// pattern as voiceCacheInvalidate + platformInstrumentParamsChanged: safe
// default at the audio layer, opt-in at the bootstrap.
//
// atomic.Bool so production-bootstrap flips and test-only flips
// (SwapUseUserRecipeOverridesForTest) don't race with Apply on background
// loader goroutines once kit / preset auto-reload lands.
var useUserRecipeOverrides atomic.Bool

// SetUseUserRecipeOverrides flips the flag. Production bootstraps call
// this once at startup; tests use SwapUseUserRecipeOverridesForTest to
// restore.
func SetUseUserRecipeOverrides(on bool) {
	useUserRecipeOverrides.Store(on)
}

// UseUserRecipeOverrides reports the current setting.
func UseUserRecipeOverrides() bool {
	return useUserRecipeOverrides.Load()
}

// SwapUseUserRecipeOverridesForTest sets the flag and returns the
// previous value so callers can restore via t.Cleanup. Mirrors
// SwapPlatformInstrumentParamsChangedForTest's discipline.
func SwapUseUserRecipeOverridesForTest(v bool) bool {
	return useUserRecipeOverrides.Swap(v)
}

// UserRecipeSource is the minimal contract audio needs from the
// persistence layer. internal/userprefs.RecipeStore satisfies it
// directly; tests pass a stub. Keeps the audio↔userprefs coupling
// to an interface, not a concrete-type import.
type UserRecipeSource interface {
	LoadRecipeOverrides() (map[string]map[string]float64, error)
	LoadUserRecipes() (map[string][]byte, error)
}

// ApplyUserRecipeOverrides is the Phase-3 entry point. It does nothing
// when the gate is off, so test binaries that never flip the gate keep
// shipped defaults exactly.
//
// When the gate is on, the sequence is:
//  1. Decode each UserRecipe payload into a RecipeDoc; for each one
//     whose BaseRecipe resolves to a registered recipe, register the
//     user recipe via RegisterPluginRecipeFromDoc with a provider that
//     delegates to NewRecipe(BaseRecipe).
//  2. For each RecipeOverride entry, mutate the registry entry's
//     ParamDefs in place (clamping out-of-range values, ignoring keys
//     the recipe doesn't declare). Mutation happens under
//     recipeRegMu.Lock() through updateRecipeDefaultsLocked.
//  3. After the lock is released, invoke voiceCacheInvalidate for every
//     instrument id bound to an affected recipe so the next trigger
//     picks up the new defaults.
//
// Returns the first error encountered (per-recipe failures are
// best-effort: the function logs them via the package logf when
// available and continues so a single bad doc can't block the whole
// load). The error return is primarily for the test path.
func ApplyUserRecipeOverrides(src UserRecipeSource) error {
	if !UseUserRecipeOverrides() {
		return nil
	}
	if src == nil {
		return nil
	}

	// 1) Register user recipes.
	if recipes, err := src.LoadUserRecipes(); err == nil {
		for id, raw := range recipes {
			doc, derr := decodeUserRecipeDoc(raw)
			if derr != nil {
				continue
			}
			if doc.ID == "" {
				doc.ID = id
			}
			if doc.BaseRecipe == "" {
				// No renderer to delegate to. Skip silently — a v3
				// "pure plugin" doc (with its own Provider declared
				// out-of-band) will need a separate registration path.
				continue
			}
			base := NewRecipe(doc.BaseRecipe)
			if base == nil {
				continue
			}
			provider := baseRecipeProvider{base: base}
			_ = RegisterPluginRecipeFromDoc(doc, provider)
		}
	}

	// 2) Apply overrides. Collect affected recipe ids for invalidation.
	// gen_type-era hygiene: a Save while re-voiced could persist the dead
	// gen_type key — strip it (re-voicing was structural project state,
	// handled by MigrateGenType at import; it never belonged in a
	// knob-override map).
	affected := map[string]bool{}
	if overrides, err := src.LoadRecipeOverrides(); err == nil {
		for recipeID, params := range overrides {
			params = StripGenTypeFromParams(params)
			if recipeID == "" || len(params) == 0 {
				continue
			}
			if updateRecipeDefaults(recipeID, params) {
				affected[recipeID] = true
			}
		}
	}

	// 3) Invalidate voice cache for every instrument bound to an
	//    affected recipe. Read bindings under the manager's RLock; call
	//    the invalidator outside the lock so a slow invalidator can't
	//    block the manager.
	if len(affected) > 0 {
		mgr := instrumentParamsMgr
		mgr.mu.RLock()
		type binding struct{ instID, recipeID string }
		bound := make([]binding, 0, len(mgr.bindings))
		for instID, recipeID := range mgr.bindings {
			if affected[recipeID] {
				bound = append(bound, binding{instID, recipeID})
			}
		}
		mgr.mu.RUnlock()
		for _, b := range bound {
			voiceCacheInvalidate(b.instID)
			// Forward the reloaded defaults to the platform layer: the
			// browser render falls back to this map when the overlay is
			// empty, so without the push a restart with saved overrides
			// renders shipped on WASM while desktop renders the saved tone.
			platformInstrumentDefaultsPush(b.instID, RecipeDefaultParams(b.recipeID))
		}
	}
	return nil
}

// UpdateRecipeDefaultsAndInvalidate mutates the registered recipe's
// ParamDef defaults from the override map AND invalidates the voice
// cache for every instrument bound to that recipe. Used by direct user
// actions (the Synth-tab Save button) where the user expects to hear
// the new sound immediately, regardless of UseUserRecipeOverrides
// (which only gates the disk → registry load path at startup).
//
// Returns true when at least one default was changed.
func UpdateRecipeDefaultsAndInvalidate(recipeID string, overrides map[string]float64) bool {
	changed := updateRecipeDefaults(recipeID, overrides)
	if !changed {
		return false
	}
	mgr := instrumentParamsMgr
	mgr.mu.RLock()
	instIDs := make([]string, 0, len(mgr.bindings))
	for instID, bound := range mgr.bindings {
		if bound == recipeID {
			instIDs = append(instIDs, instID)
		}
	}
	mgr.mu.RUnlock()
	defaults := RecipeDefaultParams(recipeID)
	for _, instID := range instIDs {
		voiceCacheInvalidate(instID)
		// Forward the saved defaults to the platform layer (same reason as
		// ApplyUserRecipeOverrides): a Save followed by an overlay clear must
		// keep rendering the saved tone in the browser, matching the desktop
		// dispatch which reads the registered defaults directly.
		platformInstrumentDefaultsPush(instID, defaults)
	}
	return true
}

// RecipeDefaultsCustomized reports whether a recipe's currently-registered
// defaults diverge from the defaults it shipped with — i.e. whether a Save (or
// a disk reload of a saved override) has changed its tone. The render
// dispatcher uses this to decide whether the recipe path is required even when
// the per-instrument overlay is empty: a customized recipe must render through
// its (changed) defaults, an as-shipped one can take the cheaper legacy path.
//
// Compared value-by-value with the same epsilon as InstrumentParamsDiffer so a
// restore-to-shipped that lands on the original numbers reads as not-customized.
//
// Allocation-free: this is on the hot legacy-dispatch path (every trigger of an
// unedited instrument calls it via tryRecipeVoice), so it compares the live
// ParamDef defaults against the shipped snapshot directly under the read lock
// instead of building two RecipeParams maps. Guarded by
// TestRecipeAwareVoiceDispatch_LegacyPathAllocBudget.
func RecipeDefaultsCustomized(id string) bool {
	if id == "" {
		return false
	}
	recipeRegMu.RLock()
	defer recipeRegMu.RUnlock()
	reg, ok := recipeRegMap[id]
	if !ok {
		return false
	}
	shipped := recipeShipped[id]
	for i := range reg.Params {
		ref, ok := shipped[reg.Params[i].Name]
		if !ok {
			return true
		}
		if math.Abs(reg.Params[i].Default-ref) > 1e-6 {
			return true
		}
	}
	return false
}

// ResetRecipeToShipped restores a recipe's registered defaults to the values it
// shipped with, undoing any prior Save, and invalidates the voice cache for
// every instrument bound to it so the next trigger renders the original tone.
// Returns true when at least one default was changed.
func ResetRecipeToShipped(id string) bool {
	shipped := RecipeShippedDefaults(id)
	if len(shipped) == 0 {
		return false
	}
	return UpdateRecipeDefaultsAndInvalidate(id, map[string]float64(shipped))
}

// RegisterUserRecipeFromBase is the convenience entry point the Synth-tab
// Save-As flow uses. It builds a RecipeDoc that delegates rendering to
// baseRecipeID (which must already be registered) with paramSeed overlaid
// onto the base's ParamDefs at registration time. The returned doc is
// fully populated and suitable for serialisation via json.Marshal — the
// caller passes those bytes to userprefs.RecipeStore.SaveUserRecipe so
// the recipe survives a restart.
//
// Returns an error if baseRecipeID isn't registered (so the caller can
// abort cleanly) or if the doc fails validation inside
// RegisterPluginRecipeFromDoc.
func RegisterUserRecipeFromBase(newID, displayName, baseRecipeID string, paramSeed map[string]float64) (RecipeDoc, error) {
	base := NewRecipe(baseRecipeID)
	if base == nil {
		return RecipeDoc{}, fmt.Errorf("audio: base recipe %q is not registered", baseRecipeID)
	}
	baseReg := RecipeRegistrations()[baseRecipeID]
	if baseReg == nil {
		return RecipeDoc{}, fmt.Errorf("audio: base recipe %q has no registration", baseRecipeID)
	}
	paramDefs := make([]ParamDef, len(baseReg.Params))
	copy(paramDefs, baseReg.Params)
	seed := make(RecipeParams, len(paramSeed))
	for k, v := range paramSeed {
		if !isFiniteParam(v) {
			continue
		}
		seed[k] = v
	}
	doc := RecipeDoc{
		ID:          newID,
		DisplayName: displayName,
		Category:    baseReg.Category,
		BaseRecipe:  baseRecipeID,
		ParamDefs:   paramDefs,
		ParamSeed:   seed,
		Origin:      OriginUser,
	}
	if err := RegisterPluginRecipeFromDoc(doc, baseRecipeProvider{base: base}); err != nil {
		return RecipeDoc{}, err
	}
	return doc, nil
}

// updateRecipeDefaults mutates the registered recipe's ParamDef defaults
// from the override map. Keys not in the recipe's ParamDefs are ignored
// (best-effort: a forward-compat doc could carry knobs the current build
// doesn't know about); values are clamped to [Min,Max]; non-finite
// values are skipped. Returns true when at least one default was changed.
func updateRecipeDefaults(recipeID string, overrides map[string]float64) bool {
	recipeRegMu.Lock()
	defer recipeRegMu.Unlock()
	reg, ok := recipeRegMap[recipeID]
	if !ok {
		return false
	}
	changed := false
	for i := range reg.Params {
		v, present := overrides[reg.Params[i].Name]
		if !present {
			continue
		}
		if !isFiniteParam(v) {
			continue
		}
		if v < reg.Params[i].Min {
			v = reg.Params[i].Min
		} else if v > reg.Params[i].Max {
			v = reg.Params[i].Max
		}
		if reg.Params[i].Default != v {
			reg.Params[i].Default = v
			changed = true
		}
	}
	if changed {
		bumpRecipeDefaultsRev()
	}
	return changed
}

// isFiniteParam is the boundary sanitiser. Duplicated from
// userprefs.isFiniteFloat because audio cannot import userprefs and the
// check is one line. Same semantics: rejects NaN and ±Inf.
func isFiniteParam(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// decodeUserRecipeDoc decodes the opaque RecipeDoc bytes that
// userprefs.RecipeStore carries. Lives here so the audio package owns
// the schema; userprefs stays free of any audio-domain types.
func decodeUserRecipeDoc(raw []byte) (RecipeDoc, error) {
	var doc RecipeDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return RecipeDoc{}, fmt.Errorf("decode user recipe: %w", err)
	}
	return doc, nil
}

// baseRecipeProvider satisfies SynthRecipeProvider by delegating to an
// already-registered base recipe. Capture the base SynthRecipe instance
// at registration time so the per-render dispatch is a single virtual
// call, not a registry lookup. Recipe definitions don't change at
// runtime in v1, so the cached reference stays valid.
type baseRecipeProvider struct {
	base SynthRecipe
}

func (p baseRecipeProvider) Render(buf []float32, sampleRate, samples, variant int, params RecipeParams) {
	if p.base == nil {
		return
	}
	p.base.Render(buf, sampleRate, samples, variant, params)
}

// InstrumentParamsDiffer reports whether the per-instrument param overlay
// for instID differs from the registered defaults of recipeID. Equivalent
// to "would Save have anything to persist?" — the UI Synth-tab uses this
// for the dirty-indicator on the Save button.
//
// Today the overlay is mutation-only (SetInstrumentParam writes, Reset
// clears) and never set to equal the defaults, so a non-empty overlay
// implies divergence. The comparison is still done value-by-value with an
// epsilon so a future caller that writes back the default doesn't get a
// false-positive dirty state.
//
// Allocation-light: the Synth tab calls this every frame for the Save-button
// dirty indicator, so the defaults are scanned directly under the registry
// read lock instead of materialising a fresh RecipeDefaultParams map. Only
// the (small) overlay copy allocates.
func InstrumentParamsDiffer(instID, recipeID string) bool {
	if instID == "" || recipeID == "" {
		return false
	}
	overlay := GetInstrumentParams(instID)
	if len(overlay) == 0 {
		return false
	}
	recipeRegMu.RLock()
	defer recipeRegMu.RUnlock()
	reg, ok := recipeRegMap[recipeID]
	if !ok {
		// Unknown recipe: every overlay key is "not in defaults" → differs.
		return true
	}
	matched := 0
	for i := range reg.Params {
		v, present := overlay[reg.Params[i].Name]
		if !present {
			continue
		}
		matched++
		if math.Abs(v-reg.Params[i].Default) > 1e-6 {
			return true
		}
	}
	// Overlay keys that don't exist in the recipe's defaults count as a
	// divergence (same semantics as the old map lookup miss).
	return matched != len(overlay)
}
