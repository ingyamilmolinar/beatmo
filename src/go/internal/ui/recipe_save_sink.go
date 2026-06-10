package ui

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// RecipeSaveSink is the persistence shim the Synth-tab Save / Save-As
// buttons write through. internal/userprefs.RecipeStore satisfies it;
// tests use a stub. The interface deliberately omits Load operations —
// the Synth tab only writes; reading happens at startup via
// audio.ApplyUserRecipeOverrides (Phase 3) which takes a separate
// audio.UserRecipeSource view.
//
// Keeping the surface small lets the UI test the Save flow with a
// minimal in-memory stub without pulling the whole RecipeStore API.
type RecipeSaveSink interface {
	SaveRecipeOverride(recipeID string, params map[string]float64) error
	SaveUserRecipe(recipeID string, doc []byte) error
}

// recipeSinkMu guards the global recipe sink. Mirrors the favoritesGuard
// pattern in favorites.go: the production bootstrap calls SetRecipeSink
// once at startup; tests swap a stub via t.Cleanup. Reads happen on the
// UI goroutine inside SaveActiveRecipe / SaveActiveRecipeAs.
var (
	recipeSinkMu sync.RWMutex
	recipeSink   RecipeSaveSink
)

// SetRecipeSink registers the global recipe persistence sink. Production
// bootstraps (cmd/beatmo.go for desktop, js_bootstrap_wasm.go for
// browser) call this after constructing the userprefs store. Nil-safe:
// passing nil disables Save / Save-As (the buttons still render but
// their handlers become no-ops).
func SetRecipeSink(s RecipeSaveSink) {
	recipeSinkMu.Lock()
	recipeSink = s
	recipeSinkMu.Unlock()
}

// activeRecipeSink returns the registered sink (or nil). Internal to
// the synth-tab Save flow; callers must tolerate a nil return.
func activeRecipeSink() RecipeSaveSink {
	recipeSinkMu.RLock()
	defer recipeSinkMu.RUnlock()
	return recipeSink
}

// SaveActiveRecipe is the Save-button entry point. Snapshots the active
// instrument's currently-effective params (registered defaults overlaid
// with per-instrument overrides), mutates the registered recipe's defaults
// so the next trigger uses the new tone, persists the override via the sink,
// and publishes EventRecipeSaved. The per-instrument overlay is intentionally
// kept (see saveRecipeForInstrument).
//
// Returns the recipe id that was saved, or empty string when there's
// nothing to save (no active instrument, no recipe binding).
func (dv *DrumView) SaveActiveRecipe() string {
	return saveRecipeForInstrument(dv.synthTabActiveInstrument())
}

// saveRecipeForInstrument is the instrument-keyed core of the Save flow,
// shared by the Synth-tab Save button (via SaveActiveRecipe) and the WASM
// saveActiveRecipe JS export so both platforms run identical logic.
func saveRecipeForInstrument(instID string) string {
	if instID == "" {
		return ""
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return ""
	}
	effective := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	asFloats := map[string]float64(effective)
	if sink := activeRecipeSink(); sink != nil {
		_ = sink.SaveRecipeOverride(recipeID, asFloats)
	}
	audio.UpdateRecipeDefaultsAndInvalidate(recipeID, asFloats)
	// Intentionally DO NOT clear the per-instrument overlay here. Although its
	// values now equal the recipe defaults (so MergeRecipeDefaults yields the
	// same numbers either way), clearing it would push an empty param map to
	// the WASM bridge (platformInstrumentParamsChanged → updateInstrumentParams),
	// dropping the just-saved tone from the browser's WebAudio voice cache and
	// reverting the sound. Keeping the overlay leaves the liked params in sync
	// on both platforms; InstrumentParamsDiffer still reports not-dirty because
	// the overlay matches the new defaults, so the Save button un-highlights.
	hooks.PublishKind(hooks.EventRecipeSaved, hooks.RecipePayload{
		RecipeID:     recipeID,
		InstrumentID: instID,
	})
	return recipeID
}

// ResetActiveRecipe is the Reset-button entry point. It restores the active
// instrument's recipe to its original shipped defaults — undoing any prior
// Save that overwrote them — and clears the per-instrument overlay. After this
// the instrument renders exactly as it shipped: on desktop the recipe is no
// longer customized so the bare legacy render is used, and on browser the
// emptied overlay is pushed to JS. The knob positions follow because they read
// the (restored) recipe defaults.
//
// Returns the recipe id that was reset, or empty string when there's no active
// instrument or recipe binding.
func (dv *DrumView) ResetActiveRecipe() string {
	return resetRecipeForInstrument(dv.synthTabActiveInstrument())
}

// resetRecipeForInstrument is the instrument-keyed core of the Reset flow,
// shared by the Synth-tab Reset button (via ResetActiveRecipe) and the WASM
// resetActiveRecipe JS export so both platforms run identical logic.
func resetRecipeForInstrument(instID string) string {
	if instID == "" {
		return ""
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return ""
	}
	audio.ResetRecipeToShipped(recipeID)
	audio.ResetInstrumentParams(instID)
	emitInstrumentParamsReset(instID, recipeID)
	recordUndo(hooks.EventInstrumentParamsReset)
	return recipeID
}

// ResetActiveSampler is the Sampler-tab Reset-button entry point. It reverts the
// loaded instrument to its factory state via audio.ResetSampleToFactory: a
// built-in that was converted to a sample is re-bound to its original synth
// recipe (user sample discarded); a user WAV / Save-As sample is restored to its
// pristine first-loaded buffer. Publishes EventSampleReset. Returns the
// instrument id reset, or "" when there's no loaded sampler instrument.
func (dv *DrumView) ResetActiveSampler() string {
	return resetSampleForInstrument(dv.sampler.captureID)
}

// resetSampleForInstrument is the instrument-keyed core of the Sampler Reset,
// mirroring resetRecipeForInstrument. The EventSampleReset payload carries
// enough (instrument + the recipe it reverted to) for a future undo journal to
// invert the action.
func resetSampleForInstrument(instID string) string {
	if instID == "" {
		return ""
	}
	// Drop any non-destructive sample-edit descriptor (and its persisted copy)
	// so the synth renders factory-clean again. The recipe binding is untouched
	// — a descriptor-bearing instrument was never converted to a sample.
	audio.ClearSampleEdit(instID)
	samplerEditDeleteFn(instID)
	priorRecipe, _ := audio.ResetSampleToFactory(instID)
	hooks.PublishKind(hooks.EventSampleReset, hooks.SamplePayload{
		SampleID: instID,
		SourceID: priorRecipe,
	})
	return instID
}

// SaveActiveRecipeAs is the Save-As-button entry point. Snapshots the
// active instrument's effective params, generates a fresh user recipe
// id (user.<base>.<short>), registers it via the plugin path delegating
// to the base recipe's renderer, persists the doc via the sink, rebinds
// the active row to the new recipe so the next trigger uses the cloned
// tone, and publishes EventRecipeCreated.
//
// Rebind semantics: the per-instrument overlay is cleared (its values
// just became the new recipe's defaults), the binding is repointed via
// audio.BindInstrumentToRecipe, and the voice cache for the instrument
// is dropped via ResetInstrumentParams's invalidation path. The user
// hears the new tone on the next beat without needing to navigate the
// instrument menu.
//
// displayName, when non-empty, overrides the auto-generated "<base> (saved)"
// label. Pass the user-typed value from the Save As dialog; pass "" to keep
// the auto-suggestion (test paths that bypass the dialog rely on this).
//
// Returns the new recipe id, or empty string when nothing was saved.
func (dv *DrumView) SaveActiveRecipeAs(displayName string) string {
	instID := dv.synthTabActiveInstrument()
	if instID == "" {
		return ""
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return ""
	}
	baseReg := audio.RecipeRegistrations()[recipeID]
	if baseReg == nil {
		return ""
	}
	effective := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	asFloats := map[string]float64(effective)

	newID := generateUserRecipeID(recipeID, asFloats)
	if strings.TrimSpace(displayName) == "" {
		displayName = fmt.Sprintf("%s (saved)", baseReg.DisplayName)
	}

	doc, err := audio.RegisterUserRecipeFromBase(newID, displayName, recipeID, asFloats)
	if err != nil {
		return ""
	}
	if sink := activeRecipeSink(); sink != nil {
		raw, marshalErr := json.Marshal(doc)
		if marshalErr == nil {
			_ = sink.SaveUserRecipe(newID, raw)
		}
	}
	// Repoint this row's instrument to the freshly-cloned recipe and
	// drop the overlay that just got baked into the new defaults. Order
	// matters: bind first (so any param read between here and the next
	// trigger sees the new recipe), then reset (which invalidates the
	// voice cache for instID so the next trigger re-renders from the
	// new defaults rather than reusing a cached buffer keyed by the old
	// recipe's params).
	audio.BindInstrumentToRecipe(instID, newID)
	audio.ResetInstrumentParams(instID)
	hooks.PublishKind(hooks.EventRecipeCreated, hooks.RecipePayload{
		RecipeID:     newID,
		BaseRecipe:   recipeID,
		InstrumentID: instID,
		DisplayName:  displayName,
	})
	return newID
}

// generateUserRecipeID produces a deterministic-yet-unique id for a
// Save-As clone. Pattern: `user.<base-trimmed>.<short>` where short is
// a 6-char hex hash of the param snapshot + current Unix nano so two
// clones of the same base with the same params from different times
// produce different ids (the user might intentionally save the same
// preset twice). Hash collisions across saves are not load-bearing —
// duplicate ids would just overwrite the prior save in userprefs.
func generateUserRecipeID(baseRecipeID string, params map[string]float64) string {
	trim := strings.TrimPrefix(baseRecipeID, "drum-")
	trim = strings.TrimPrefix(trim, "fm-")
	h := fnv.New64a()
	keys := sortedFloatKeys(params)
	var buf [8]byte
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		bits := mathFloat64bits(params[k])
		for i := 0; i < 8; i++ {
			buf[i] = byte(bits >> (8 * i))
		}
		h.Write(buf[:])
	}
	h.Write([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	return fmt.Sprintf("user.%s.%06x", trim, h.Sum64()&0xFFFFFF)
}

// sortedFloatKeys returns the keys of m in lexical order so the user-id
// hash stays deterministic for the same params snapshot taken twice in
// the same nanosecond (defensive; the time component already makes
// collisions vanishingly unlikely).
func sortedFloatKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Inline sort to avoid an import — small set; insertion sort beats
	// pulling sort just for the slice size we ever see (8-knob set).
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// mathFloat64bits forwards to math.Float64bits. Kept as a separate name
// so callers in this file read uniformly; the audio package uses
// hashRecipeParams for its own keying, and this is the UI-side mirror.
func mathFloat64bits(f float64) uint64 {
	return math.Float64bits(f)
}
