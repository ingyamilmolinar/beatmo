package audio

import (
	"reflect"
	"sort"
	"testing"
)

// Phase 1 100% replication guarantee. These tests prove that the unified
// RecipeDoc shape can express every shipped builtin (drums AND FM) without
// loss, and that every shipped instrument id is reachable through the doc
// projection. They iterate the full registry — no allow-list / skip-list —
// so adding a new C renderer without a matching RecipeDoc fails the build.

// TestRecipeDocRoundtripAllRecipes proves BuildBuiltinRecipeDocs covers
// every entry in builtinRecipeDescriptors, and that ToRegistration produces
// a RecipeRegistration whose Params slice is field-for-field identical to
// the production-registered one. Subtest names are the recipe id so the
// failure log says "fm-bass" (or whichever) when it breaks.
func TestRecipeDocRoundtripAllRecipes(t *testing.T) {
	docs := BuildBuiltinRecipeDocs()
	if got, want := len(docs), len(builtinRecipeDescriptors); got != want {
		t.Fatalf("BuildBuiltinRecipeDocs len = %d, want %d (matches builtinRecipeDescriptors)", got, want)
	}
	regs := RecipeRegistrations()
	docByID := make(map[string]RecipeDoc, len(docs))
	for _, d := range docs {
		docByID[d.ID] = d
	}
	for _, d := range builtinRecipeDescriptors {
		t.Run(d.ID, func(t *testing.T) {
			doc, ok := docByID[d.ID]
			if !ok {
				t.Fatalf("BuildBuiltinRecipeDocs missing entry for %q", d.ID)
			}
			if doc.DisplayName != d.Display {
				t.Errorf("doc.DisplayName = %q want %q", doc.DisplayName, d.Display)
			}
			if doc.Category != d.Category {
				t.Errorf("doc.Category = %q want %q", doc.Category, d.Category)
			}
			if doc.Origin != OriginBuiltin {
				t.Errorf("doc.Origin = %q want %q", doc.Origin, OriginBuiltin)
			}
			if doc.BaseRecipe != "" {
				t.Errorf("doc.BaseRecipe = %q want \"\" (builtins own their renderer)", doc.BaseRecipe)
			}
			if len(doc.ParamSeed) != 0 {
				t.Errorf("doc.ParamSeed = %v want empty (builtins have no seed)", doc.ParamSeed)
			}

			reg := regs[d.ID]
			if reg == nil {
				t.Fatalf("registry has no entry for %q (production registration missing)", d.ID)
			}
			if len(reg.Params) != len(doc.ParamDefs) {
				t.Fatalf("registry Params len = %d, doc ParamDefs len = %d", len(reg.Params), len(doc.ParamDefs))
			}
			for i := range reg.Params {
				if !reflect.DeepEqual(reg.Params[i], doc.ParamDefs[i]) {
					t.Errorf("param[%d]: registry=%+v doc=%+v", i, reg.Params[i], doc.ParamDefs[i])
				}
			}

			// ToRegistration with a no-op factory must reproduce the same
			// Params shape the production registration uses. New is opaque
			// (factory closure) and is intentionally not compared.
			roundTrip := doc.ToRegistration(func() SynthRecipe { return nil })
			if roundTrip.ID != reg.ID || roundTrip.DisplayName != reg.DisplayName || roundTrip.Category != reg.Category {
				t.Errorf("ToRegistration shape: got id=%q display=%q category=%q; want id=%q display=%q category=%q",
					roundTrip.ID, roundTrip.DisplayName, roundTrip.Category,
					reg.ID, reg.DisplayName, reg.Category)
			}
			if len(roundTrip.Params) != len(reg.Params) {
				t.Fatalf("ToRegistration Params len = %d want %d", len(roundTrip.Params), len(reg.Params))
			}
			for i := range roundTrip.Params {
				if !reflect.DeepEqual(roundTrip.Params[i], reg.Params[i]) {
					t.Errorf("ToRegistration param[%d]: got %+v want %+v", i, roundTrip.Params[i], reg.Params[i])
				}
			}

			// Defaults derived from the doc must hash identically to the
			// registry-resolved defaults. Catches silent drift between the
			// doc projection and the registry contents.
			docDefaults := make(RecipeParams, len(doc.ParamDefs))
			for _, p := range doc.ParamDefs {
				docDefaults[p.Name] = p.Default
			}
			if got, want := hashRecipeParams(docDefaults), hashRecipeParams(RecipeDefaultParams(d.ID)); got != want {
				t.Errorf("defaults hash mismatch: doc=%d registry=%d", got, want)
			}
		})
	}
}

// TestRecipeDocCoversEveryInstrumentBinding proves that every shipped
// instrument id (including FM variants like fm-bass-1 and drum aliases like
// kick-tight) is reachable through the unified doc projection. For each
// binding entry it asserts (1) the recipe id resolves through
// RecipeForInstrument, (2) NewRecipe(recipeID) returns a non-nil recipe,
// and (3) that recipe id appears in BuildBuiltinRecipeDocs.
func TestRecipeDocCoversEveryInstrumentBinding(t *testing.T) {
	docIDs := make(map[string]bool, len(builtinRecipeDescriptors))
	for _, d := range BuildBuiltinRecipeDocs() {
		docIDs[d.ID] = true
	}
	// Sort for deterministic subtest order in failure logs.
	instIDs := make([]string, 0, len(builtinInstrumentRecipeBindings))
	for id := range builtinInstrumentRecipeBindings {
		instIDs = append(instIDs, id)
	}
	sort.Strings(instIDs)

	for _, instID := range instIDs {
		t.Run(instID, func(t *testing.T) {
			wantRecipe := builtinInstrumentRecipeBindings[instID]
			gotRecipe := RecipeForInstrument(instID)
			if gotRecipe != wantRecipe {
				t.Errorf("RecipeForInstrument(%q) = %q, want %q", instID, gotRecipe, wantRecipe)
			}
			if NewRecipe(wantRecipe) == nil {
				t.Errorf("NewRecipe(%q) = nil — recipe bound to instrument %q is not registered", wantRecipe, instID)
			}
			if !docIDs[wantRecipe] {
				t.Errorf("recipe %q (bound to %q) is not present in BuildBuiltinRecipeDocs", wantRecipe, instID)
			}
		})
	}
}
