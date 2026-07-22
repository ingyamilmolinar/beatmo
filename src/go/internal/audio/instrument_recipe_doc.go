package audio

import (
	"fmt"
	"math"
	"sync"
)

// Recipe → builtin-base tracking. Populated by RegisterPluginRecipeFromDoc for
// every user/plugin recipe (which delegates rendering to a builtin base).
// Builtins never register through that path, so RecipeBaseOf returns "" for them
// and callers treat the builtin as its own base.
var (
	recipeBaseMu sync.RWMutex
	recipeBase   = map[string]string{}
)

func recordRecipeBase(id, base string) {
	if id == "" || base == "" {
		return
	}
	recipeBaseMu.Lock()
	recipeBase[id] = base
	recipeBaseMu.Unlock()
}

// RecipeBaseOf returns the builtin base recipe a user/plugin recipe delegates
// to, or "" when the recipe is itself a builtin (its own base) or untracked.
func RecipeBaseOf(id string) string {
	recipeBaseMu.RLock()
	defer recipeBaseMu.RUnlock()
	return recipeBase[id]
}

// RecipeExists reports whether a recipe id is currently registered.
func RecipeExists(id string) bool {
	return RecipeRegistrations()[id] != nil
}

// RecipeIsBuiltin reports whether a registered recipe is a shipped builtin (it
// has no tracked user base). Import must never overwrite one — a colliding id is
// re-namespaced to a fresh user recipe instead.
func RecipeIsBuiltin(id string) bool {
	return RecipeExists(id) && RecipeBaseOf(id) == ""
}

// BuildInstrumentRecipeDoc reconstructs a fully portable RecipeDoc for the
// instrument's current tone: a base recipe the recipient is guaranteed to have
// (a builtin) plus a ParamSeed = the effective params' delta vs that base's
// as-shipped defaults. Importing via RegisterUserRecipeFromBase(newID, name,
// doc.BaseRecipe, doc.ParamSeed) reproduces the tone on a machine that never saw
// the source recipe.
//
// This is the CORRECT export the legacy exportUserRecipe lacked (that one
// dropped BaseRecipe and ParamSeed, encoding the tone only in ParamDefs
// defaults, which the import path cannot read back).
func BuildInstrumentRecipeDoc(instID string) (RecipeDoc, error) {
	recipeID := RecipeForInstrument(instID)
	if recipeID == "" {
		return RecipeDoc{}, fmt.Errorf("instrument %q has no recipe binding", instID)
	}
	reg := RecipeRegistrations()[recipeID]
	if reg == nil {
		return RecipeDoc{}, fmt.Errorf("recipe %q not registered", recipeID)
	}

	// Resolve a base the recipient will have: a user recipe → its tracked builtin
	// base; a builtin → itself.
	base := RecipeBaseOf(recipeID)
	if base == "" {
		base = recipeID
	}
	baseReg := RecipeRegistrations()[base]
	if baseReg == nil {
		// Base not registered (defensive) — fall back to the current recipe so
		// at least same-machine round-trips work.
		base, baseReg = recipeID, reg
	}

	// Seed = effective params where they differ from the base's AS-SHIPPED
	// defaults (portable: RecipeShippedDefaults ignores later Saves).
	effective := MergeRecipeDefaults(recipeID, GetInstrumentParams(instID))
	baseShipped := RecipeShippedDefaults(base)
	seed := map[string]float64{}
	for k, v := range effective {
		if s, ok := baseShipped[k]; !ok || math.Abs(v-s) > 1e-9 {
			seed[k] = v
		}
	}

	defs := make([]ParamDef, len(baseReg.Params))
	copy(defs, baseReg.Params)

	return RecipeDoc{
		ID:          recipeID,
		DisplayName: reg.DisplayName,
		Category:    reg.Category,
		BaseRecipe:  base,
		ParamDefs:   defs,
		ParamSeed:   seed,
		Origin:      OriginUser,
	}, nil
}
