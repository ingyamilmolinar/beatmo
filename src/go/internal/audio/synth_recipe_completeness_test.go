package audio

import (
	"math"
	"testing"
)

// Phase 6 completeness tests. These cover the cross-platform "every
// recipe renders cleanly at every preset" surface that the Phase 6 plan
// freezes for plugin parity. The native-only render quality check lives
// in synth_recipe_phase2_native_test.go (Render NaN/Inf + decay-changed);
// here we add the surface-level (registry-wide) coverage.

// TestCompleteness_EveryRecipeHasInstrument inverts the existing
// TestPhase2_EveryInstrumentBindingResolvesToRegisteredRecipe: every
// recipe must be bound to at least one shipped instrument. A registered
// recipe with no bound instrument is dead code (it would also be a UI
// bug — the Synth tab is per-instrument). Plugin recipes in v2 will be
// exempt via a flag we'll add when needed.
func TestCompleteness_EveryRecipeHasInstrument(t *testing.T) {
	boundRecipes := make(map[string]bool, len(builtinInstrumentRecipeBindings))
	for _, recipeID := range builtinInstrumentRecipeBindings {
		boundRecipes[recipeID] = true
	}
	for _, d := range builtinRecipeDescriptors {
		if !boundRecipes[d.ID] {
			t.Errorf("recipe %q is registered but no instrument binds to it (orphan)", d.ID)
		}
	}
}

// TestCompleteness_EveryRecipeParamHasDeclaredRange asserts every
// ParamDef has a self-consistent range. The validateParamDef call inside
// RegisterRecipe already catches Min>Max and Default-out-of-range, but
// this test makes the contract surface-visible: registry-wide assertion
// after init().
func TestCompleteness_EveryRecipeParamHasDeclaredRange(t *testing.T) {
	for _, id := range RecipeOrder() {
		regs := RecipeRegistrations()
		r := regs[id]
		if r == nil {
			continue
		}
		for _, p := range r.Params {
			if p.Min > p.Max {
				t.Errorf("recipe %q param %q: Min=%v > Max=%v", id, p.Name, p.Min, p.Max)
			}
			if p.Default < p.Min || p.Default > p.Max {
				t.Errorf("recipe %q param %q: Default=%v outside [%v,%v]", id, p.Name, p.Default, p.Min, p.Max)
			}
		}
	}
}

// TestCompleteness_RecipeRenderPresetsHaveFiniteOutput renders every
// recipe at three representative parameter presets (default, half,
// extreme) and asserts the output buffer contains no NaN/Inf. Catches
// future C-side regressions where a knob at the boundary produces
// pathological math (e.g. log(0), divide-by-zero).
//
// Native-only because the stub Render is a no-op — actual CGo render
// quality is what we're guarding here.
func TestCompleteness_RecipeRenderPresetsHaveFiniteOutput(t *testing.T) {
	// Under -tags test, recipes use stub Render which is a no-op.
	// We still iterate the registry to assert no panics, but the
	// NaN/Inf check is meaningful only on the native build (which
	// runs this test under `go test ./internal/audio/`).
	const samples = 22050 // 0.5s @ 44.1k

	presets := []struct {
		name string
		// take=0 → ParamDef.Default; take=1 → midpoint; take=2 → Max.
		take int
	}{
		{"default", 0},
		{"mid", 1},
		{"extreme-max", 2},
	}

	for _, id := range RecipeOrder() {
		r := NewRecipe(id)
		if r == nil {
			t.Errorf("NewRecipe(%q) returned nil", id)
			continue
		}
		schema := r.ParamSchema()
		for _, preset := range presets {
			params := RecipeParams{}
			for _, def := range schema {
				switch preset.take {
				case 0:
					params[def.Name] = def.Default
				case 1:
					params[def.Name] = (def.Min + def.Max) / 2
				case 2:
					params[def.Name] = def.Max
				}
			}
			buf := make([]float32, samples)
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("recipe %q preset %s panicked: %v", id, preset.name, rec)
					}
				}()
				r.Render(buf, 44100, samples, 0, params)
			}()
			for i, v := range buf {
				f := float64(v)
				if math.IsNaN(f) {
					t.Errorf("recipe %q preset %s produced NaN at sample %d", id, preset.name, i)
					break
				}
				if math.IsInf(f, 0) {
					t.Errorf("recipe %q preset %s produced Inf at sample %d", id, preset.name, i)
					break
				}
			}
		}
	}
}

// TestCompleteness_RecipeOrderMatchesDescriptorOrder asserts the order
// in which RecipeOrder() returns ids matches the declaration order in
// builtinRecipeDescriptors. Important for the UI: the Synth tab and
// any future preset browser iterate RecipeOrder() to render the list,
// and a stable order keeps muscle memory + tests deterministic.
func TestCompleteness_RecipeOrderMatchesDescriptorOrder(t *testing.T) {
	want := make([]string, 0, len(builtinRecipeDescriptors))
	for _, d := range builtinRecipeDescriptors {
		want = append(want, d.ID)
	}
	got := RecipeOrder()
	// got may contain test-registered transient recipes; we only require
	// the prefix of got to match the descriptor order.
	if len(got) < len(want) {
		t.Fatalf("RecipeOrder len=%d < descriptor len=%d", len(got), len(want))
	}
	// Find the position of each builtin in got; they must appear in
	// the same relative order. Test-registered recipes can interleave.
	pos := func(id string) int {
		for i, x := range got {
			if x == id {
				return i
			}
		}
		return -1
	}
	for i := 1; i < len(want); i++ {
		prev, cur := pos(want[i-1]), pos(want[i])
		if prev < 0 || cur < 0 {
			t.Errorf("missing recipe in order: prev=%q (%d), cur=%q (%d)", want[i-1], prev, want[i], cur)
			continue
		}
		if cur <= prev {
			t.Errorf("builtin order violated: %q (pos %d) must appear before %q (pos %d)", want[i-1], prev, want[i], cur)
		}
	}
}

// TestCompleteness_HashRecipeParamsCollisionResistanceSpotCheck samples
// pairs of "different but similar" param maps and asserts they hash to
// distinct values. Not a cryptographic proof — just a sanity check that
// a one-byte difference in a float doesn't collide with a known
// neighbor.
func TestCompleteness_HashRecipeParamsCollisionResistanceSpotCheck(t *testing.T) {
	cases := []struct {
		name string
		a    RecipeParams
		b    RecipeParams
	}{
		{"different value", RecipeParams{"x": 0.1}, RecipeParams{"x": 0.10001}},
		{"different key", RecipeParams{"x": 0.5}, RecipeParams{"y": 0.5}},
		{"extra key", RecipeParams{"x": 0.5}, RecipeParams{"x": 0.5, "y": 0}},
		{"different signs", RecipeParams{"x": 0.5}, RecipeParams{"x": -0.5}},
		{"large vs small", RecipeParams{"x": 1e-10}, RecipeParams{"x": 1e10}},
	}
	for _, c := range cases {
		if hashRecipeParams(c.a) == hashRecipeParams(c.b) {
			t.Errorf("collision: %s → a=%v b=%v hash=%x", c.name, c.a, c.b, hashRecipeParams(c.a))
		}
	}
}
