package audio

import "sync"

// Global synth-recipe registry. Mirrors effect_registry.go in shape.
//
// Built-in recipes register from init() in synth_registry_entries_*.go. The
// registry is process-global and write-rare / read-often; an RWMutex is the
// right shape. Re-registering an existing ID updates the entry in place
// (used by platform-specific overrides), preserving registration order.
var (
	recipeRegMu  sync.RWMutex
	recipeRegMap = map[string]*RecipeRegistration{}
	recipeOrder  []string
	// recipeShipped snapshots each recipe's ParamDef defaults as captured at
	// registration time — the "original" configuration a recipe shipped with.
	// updateRecipeDefaults (Synth-tab Save, disk reload) deliberately does NOT
	// touch this map, so RecipeDefaultsCustomized / ResetRecipeToShipped can
	// tell "customized" from "as-shipped" and restore the original.
	recipeShipped = map[string]RecipeParams{}
)

// paramDefaultsSnapshot builds a fresh RecipeParams from a ParamDef slice's
// Default values. Used to capture the shipped defaults at registration.
func paramDefaultsSnapshot(params []ParamDef) RecipeParams {
	out := make(RecipeParams, len(params))
	for _, d := range params {
		out[d.Name] = d.Default
	}
	return out
}

// RegisterRecipe adds (or updates) a recipe registration. Call from an init()
// function in a synth_registry_entries_*.go file. Panics on invalid input
// because misconfigured recipes are a programming error, not a runtime
// condition.
func RegisterRecipe(reg RecipeRegistration) {
	if reg.ID == "" {
		panic("audio.RegisterRecipe: ID is required")
	}
	if reg.New == nil {
		panic("audio.RegisterRecipe: New is required for " + reg.ID)
	}
	for _, d := range reg.Params {
		if err := validateParamDef(d); err != nil {
			panic("audio.RegisterRecipe " + reg.ID + ": " + err.Error())
		}
	}
	recipeRegMu.Lock()
	defer recipeRegMu.Unlock()
	bumpRecipeDefaultsRev()
	// Capture the shipped defaults on every (re-)registration: a re-register
	// (platform override) is a fresh "ship", whereas updateRecipeDefaults
	// (Save / reload) is a user customization and must not move this baseline.
	recipeShipped[reg.ID] = paramDefaultsSnapshot(reg.Params)
	if existing, ok := recipeRegMap[reg.ID]; ok {
		existing.DisplayName = reg.DisplayName
		existing.Category = reg.Category
		existing.Params = reg.Params
		existing.New = reg.New
		return
	}
	r := reg
	recipeRegMap[reg.ID] = &r
	recipeOrder = append(recipeOrder, reg.ID)
}

// RecipeRegistrations returns a copy of the registry map. The pointer values
// remain shared with the registry, so callers MUST NOT mutate the
// RecipeRegistration through them.
func RecipeRegistrations() map[string]*RecipeRegistration {
	recipeRegMu.RLock()
	defer recipeRegMu.RUnlock()
	out := make(map[string]*RecipeRegistration, len(recipeRegMap))
	for k, v := range recipeRegMap {
		out[k] = v
	}
	return out
}

// RecipeOrder returns recipe IDs in registration order.
func RecipeOrder() []string {
	recipeRegMu.RLock()
	defer recipeRegMu.RUnlock()
	out := make([]string, len(recipeOrder))
	copy(out, recipeOrder)
	return out
}

// RecipeDefaultParams returns a fresh map of the registered defaults for id.
// Unknown IDs return an empty map (never nil) so callers can range freely.
func RecipeDefaultParams(id string) RecipeParams {
	recipeRegMu.RLock()
	reg, ok := recipeRegMap[id]
	recipeRegMu.RUnlock()
	if !ok {
		return RecipeParams{}
	}
	out := make(RecipeParams, len(reg.Params))
	for _, d := range reg.Params {
		out[d.Name] = d.Default
	}
	return out
}

// RecipeShippedDefaults returns a fresh map of the defaults the recipe was
// registered with (its original/as-shipped configuration), independent of any
// later Save or disk-reload customization. Unknown IDs return an empty map.
func RecipeShippedDefaults(id string) RecipeParams {
	recipeRegMu.RLock()
	src, ok := recipeShipped[id]
	recipeRegMu.RUnlock()
	if !ok {
		return RecipeParams{}
	}
	out := make(RecipeParams, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// NewRecipe constructs a SynthRecipe for the given id. Returns nil if id is
// unknown so callers can distinguish "registry miss" from "valid recipe".
func NewRecipe(id string) SynthRecipe {
	recipeRegMu.RLock()
	reg, ok := recipeRegMap[id]
	recipeRegMu.RUnlock()
	if !ok || reg.New == nil {
		return nil
	}
	return reg.New()
}

// MergeRecipeDefaults returns a fresh map containing the recipe's defaults
// overlaid with user-supplied values. Used at trigger time to resolve the
// final parameter set before voice rendering. Unknown IDs return a copy of
// the user map (never the input itself, so mutation is safe).
func MergeRecipeDefaults(id string, user RecipeParams) RecipeParams {
	defaults := RecipeDefaultParams(id)
	out := make(RecipeParams, len(defaults)+len(user))
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range user {
		out[k] = v
	}
	return out
}

// UnregisterRecipeForTest removes a recipe from the registry. Intended
// for test cleanup only; production code never unregisters because
// recipes are process-global singletons. Exported (vs the original
// unexported helper) so cross-package tests in internal/ui can drive
// transient recipe registrations.
func UnregisterRecipeForTest(id string) {
	unregisterRecipeForTest(id)
}

// unregisterRecipe removes a recipe from the registry (production path, used by
// DeleteInstrument to drop a cloned/user recipe). Same mechanics as the test
// helper; separated so the intent reads correctly at the call site.
func unregisterRecipe(id string) {
	unregisterRecipeForTest(id)
}

// unregisterRecipeForTest removes a recipe from the registry. Intended for
// test cleanup only; production code never unregisters because recipes are
// process-global singletons.
func unregisterRecipeForTest(id string) {
	recipeRegMu.Lock()
	defer recipeRegMu.Unlock()
	if _, ok := recipeRegMap[id]; !ok {
		return
	}
	bumpRecipeDefaultsRev()
	delete(recipeRegMap, id)
	out := recipeOrder[:0]
	for _, x := range recipeOrder {
		if x != id {
			out = append(out, x)
		}
	}
	recipeOrder = out
}
