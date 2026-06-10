package audio

import "testing"

// The gen_type "Generator" selector is removed by the native-deprecation
// migration: every instrument's own engine is fully knob-parameterized, and
// generic re-voicing is the Modular recipe's job. Saved projects from the
// gen_type era carry `gen_type` in synth_params; MigrateGenType converts
// that intent losslessly:
//
//   - gen_type >= 1 (re-voiced): rebind to synth-modular with
//     osc_type = gen_type-1; overlapping generic knobs (pitch/drive) carry
//     over; the gen_type key is dropped.
//   - gen_type 0/absent (Native): keep the recipe, drop the key.
func TestMigrateGenType(t *testing.T) {
	cases := []struct {
		name       string
		recipe     string
		params     RecipeParams
		wantRecipe string
		wantParams RecipeParams
	}{
		{
			name:       "absent gen_type is a no-op",
			recipe:     "drum-snare",
			params:     RecipeParams{"decay": 2},
			wantRecipe: "drum-snare",
			wantParams: RecipeParams{"decay": 2},
		},
		{
			name:       "native gen_type 0 drops the key",
			recipe:     "drum-snare",
			params:     RecipeParams{"gen_type": 0, "drive": 0.5},
			wantRecipe: "drum-snare",
			wantParams: RecipeParams{"drive": 0.5},
		},
		{
			name:       "re-voiced saw rebinds to modular",
			recipe:     "drum-snare",
			params:     RecipeParams{"gen_type": 2, "pitch": 3, "drive": 0.4},
			wantRecipe: "synth-modular",
			wantParams: RecipeParams{"osc_type": 1, "pitch": 3, "drive": 0.4},
		},
		{
			name:       "re-voiced noise keeps modular knobs already present",
			recipe:     "drum-kick",
			params:     RecipeParams{"gen_type": 6, "amp_decay": 0.8},
			wantRecipe: "synth-modular",
			wantParams: RecipeParams{"osc_type": 5, "amp_decay": 0.8},
		},
		{
			name:       "nil params stays nil",
			recipe:     "fm-bass",
			params:     nil,
			wantRecipe: "fm-bass",
			wantParams: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRecipe, gotParams := MigrateGenType(tc.recipe, tc.params)
			if gotRecipe != tc.wantRecipe {
				t.Errorf("recipe = %q, want %q", gotRecipe, tc.wantRecipe)
			}
			if len(gotParams) != len(tc.wantParams) {
				t.Fatalf("params = %v, want %v", gotParams, tc.wantParams)
			}
			for k, v := range tc.wantParams {
				if gotParams[k] != v {
					t.Errorf("params[%q] = %v, want %v", k, gotParams[k], v)
				}
			}
		})
	}
}
