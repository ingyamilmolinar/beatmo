package audio

import "fmt"

// instrument_clone.go is the production, config-first CLONE / DELETE API for
// instruments. It composes the existing runtime primitives so an instrument can
// be duplicated (with param overrides) or removed at runtime without any
// per-instrument code — the same config-first principle as the modular
// instrument table. Cloning is how "copy an instrument's configuration" works
// programmatically; the UI Save-As flow is a specialization of the same idea.
//
// Tag-neutral: every primitive it calls (registerInstanceAlias,
// BindInstrumentToRecipe, RegisterUserRecipeFromBase, Register/Unregister,
// platformInstrumentDefaultsPush) has a per-build implementation, so clone and
// delete work on desktop, under -tags test, and on WASM. (On WASM a brand-new
// runtime instrument id also needs a render entry on the JS side to be audible
// there; built-in/table instruments get that from modular_instruments.gen.js.)

// CloneInstrument creates a new instrument newID that is a copy of srcID's
// current effective configuration, with overrides applied on top. The clone is
// a first-class, playable, recipe-bound instrument defined purely by config —
// no code. Returns the registered RecipeDoc.
//
//   - srcID must be an existing recipe-bound instrument.
//   - newID must be unused (not already a registered instrument or recipe).
//   - display is the clone's display name ("" → keep the auto label).
//   - overrides overlay srcID's effective params (nil → an exact clone).
func CloneInstrument(srcID, newID, display string, overrides RecipeParams) (RecipeDoc, error) {
	if newID == "" {
		return RecipeDoc{}, fmt.Errorf("audio.CloneInstrument: newID is required")
	}
	if newID == srcID {
		return RecipeDoc{}, fmt.Errorf("audio.CloneInstrument: newID must differ from srcID %q", srcID)
	}
	if instanceAlreadyRegistered(newID) || RecipeForInstrument(newID) != "" {
		return RecipeDoc{}, fmt.Errorf("audio.CloneInstrument: instrument %q already exists", newID)
	}
	srcRecipeID := RecipeForInstrument(srcID)
	if srcRecipeID == "" {
		return RecipeDoc{}, fmt.Errorf("audio.CloneInstrument: source %q is not recipe-bound", srcID)
	}

	// The clone's config = srcID's effective params (recipe defaults + any live
	// per-instrument edits) with the overrides overlaid. This is the whole
	// "configuration" that defines the new instrument.
	effective := MergeRecipeDefaults(srcRecipeID, GetInstrumentParams(srcID))
	for k, v := range overrides {
		effective[k] = v
	}

	newRecipeID := "clone." + newID
	doc, err := RegisterUserRecipeFromBase(newRecipeID, display, srcRecipeID, map[string]float64(effective))
	if err != nil {
		return RecipeDoc{}, fmt.Errorf("audio.CloneInstrument: %w", err)
	}

	// Make newID a playable instrument that renders the CLONED config. On native
	// this bakes the effective seed into the voice (bakedModularRender), the same
	// path the shipped seeded instruments use — so the override actually changes
	// the sound even though it lives in the recipe defaults (the dispatch's fast
	// path skips the recipe when defaults aren't "customized"). Bind the new
	// recipe and seed its defaults to the platform layer (browser + recipe path).
	registerClonedVoice(newID, effective, srcID)
	// Inherit the source's loudness-normalized amplitude. The clone id has no
	// offline measurement, so without this it would fall back to
	// DefaultAmplitude and an exact clone would not render identically to its
	// source (normalizeAndScale scales by amplitudeForInstrument(id)).
	setRuntimeLoudnessAmp(newID, ConfigForInstrument(srcID).Amplitude)
	BindInstrumentToRecipe(newID, newRecipeID)
	platformInstrumentDefaultsPush(newID, RecipeDefaultParams(newRecipeID))
	if display != "" {
		SetInstrumentDisplayName(newID, display)
	}
	return doc, nil
}

// DeleteInstrument removes a runtime-registered instrument: it drops the
// playable voice, clears the recipe binding + name override, and unregisters the
// instrument's recipe UNLESS another instrument still shares it (deleting a
// shared/base recipe would silence its other users). Deleting an unknown id is
// an error; deleting a shipped built-in is allowed but session-scoped (a fresh
// process / ResetInstruments restores it).
func DeleteInstrument(id string) error {
	if id == "" {
		return fmt.Errorf("audio.DeleteInstrument: id is required")
	}
	if !instanceAlreadyRegistered(id) && RecipeForInstrument(id) == "" {
		return fmt.Errorf("audio.DeleteInstrument: instrument %q is not registered", id)
	}
	recipeID := RecipeForInstrument(id)

	Unregister(id)
	BindInstrumentToRecipe(id, "") // unbind
	SetInstrumentDisplayName(id, "")
	clearRuntimeLoudnessAmp(id) // drop any inherited clone amp

	// Only drop the recipe if nothing else is bound to it — a builtin recipe is
	// shared by its base instrument (and possibly instance variants), so removing
	// it would break them.
	if recipeID != "" && !recipeBoundByOtherInstrument(recipeID, id) {
		unregisterRecipe(recipeID)
	}
	return nil
}

// recipeBoundByOtherInstrument reports whether any instrument other than exceptID
// is bound to recipeID.
func recipeBoundByOtherInstrument(recipeID, exceptID string) bool {
	m := instrumentParamsMgr
	m.mu.RLock()
	defer m.mu.RUnlock()
	for inst, rec := range m.bindings {
		if rec == recipeID && inst != exceptID {
			return true
		}
	}
	return false
}
