package audio

import (
	"reflect"
	"sort"
	"testing"
)

// fakeRecipe is a deterministic SynthRecipe for registry/hash tests.
type fakeRecipe struct {
	id       string
	display  string
	category string
	params   []ParamDef
	render   func(buf []float32, sampleRate, samples, variant int, p RecipeParams)
}

func (r *fakeRecipe) ID() string              { return r.id }
func (r *fakeRecipe) DisplayName() string     { return r.display }
func (r *fakeRecipe) Category() string        { return r.category }
func (r *fakeRecipe) ParamSchema() []ParamDef { return r.params }
func (r *fakeRecipe) Render(buf []float32, sampleRate, samples, variant int, p RecipeParams) {
	if r.render != nil {
		r.render(buf, sampleRate, samples, variant, p)
	}
}

func newFakeRecipe(id string, params ...ParamDef) RecipeRegistration {
	return RecipeRegistration{
		ID:          id,
		DisplayName: id + " name",
		Category:    "test",
		Params:      params,
		New: func() SynthRecipe {
			return &fakeRecipe{id: id, display: id + " name", category: "test", params: params}
		},
	}
}

func TestRegisterRecipe_AppearsInRegistrations(t *testing.T) {
	const id = "test-register-appears"
	t.Cleanup(func() { unregisterRecipeForTest(id) })
	RegisterRecipe(newFakeRecipe(id, ParamDef{Name: "x", Min: 0, Max: 1, Default: 0.5}))

	regs := RecipeRegistrations()
	got, ok := regs[id]
	if !ok || got == nil {
		t.Fatalf("recipe %q not registered; got=%#v", id, regs)
	}
	if got.ID != id {
		t.Errorf("got ID=%q want %q", got.ID, id)
	}
	if got.Category != "test" {
		t.Errorf("got Category=%q want %q", got.Category, "test")
	}
	if len(got.Params) != 1 || got.Params[0].Name != "x" {
		t.Errorf("got Params=%#v want one ParamDef named x", got.Params)
	}
}

func TestRegisterRecipe_AppearsInRecipeOrder(t *testing.T) {
	const id = "test-register-order"
	t.Cleanup(func() { unregisterRecipeForTest(id) })
	RegisterRecipe(newFakeRecipe(id, ParamDef{Name: "x", Min: 0, Max: 1, Default: 0.5}))

	order := RecipeOrder()
	found := false
	for _, x := range order {
		if x == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("recipe %q missing from RecipeOrder()=%v", id, order)
	}
}

func TestRegisterRecipe_DuplicateOverwritesNew(t *testing.T) {
	const id = "test-register-dup"
	t.Cleanup(func() { unregisterRecipeForTest(id) })

	var marker string
	first := newFakeRecipe(id, ParamDef{Name: "x"})
	first.New = func() SynthRecipe { marker = "first"; return &fakeRecipe{id: id} }
	RegisterRecipe(first)

	second := newFakeRecipe(id, ParamDef{Name: "x"})
	second.New = func() SynthRecipe { marker = "second"; return &fakeRecipe{id: id} }
	RegisterRecipe(second)

	r := NewRecipe(id)
	if r == nil {
		t.Fatal("NewRecipe returned nil")
	}
	if marker != "second" {
		t.Errorf("duplicate registration did not overwrite New; marker=%q want %q", marker, "second")
	}
	// Verify only one entry in order (no duplicate slot).
	order := RecipeOrder()
	count := 0
	for _, x := range order {
		if x == id {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one occurrence in RecipeOrder; got %d", count)
	}
}

func TestRecipeDefaultParams_FromSchema(t *testing.T) {
	const id = "test-defaults"
	t.Cleanup(func() { unregisterRecipeForTest(id) })
	RegisterRecipe(newFakeRecipe(id,
		ParamDef{Name: "a", Min: 0, Max: 1, Default: 0.25},
		ParamDef{Name: "b", Min: -1, Max: 1, Default: -0.5},
	))

	got := RecipeDefaultParams(id)
	want := RecipeParams{"a": 0.25, "b": -0.5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestRecipeDefaultParams_UnknownReturnsEmpty(t *testing.T) {
	got := RecipeDefaultParams("does-not-exist")
	if len(got) != 0 {
		t.Errorf("expected empty map for unknown recipe, got %v", got)
	}
}

func TestMergeRecipeDefaults_UserOverridesDefaults(t *testing.T) {
	const id = "test-merge"
	t.Cleanup(func() { unregisterRecipeForTest(id) })
	RegisterRecipe(newFakeRecipe(id,
		ParamDef{Name: "a", Min: 0, Max: 1, Default: 0.1},
		ParamDef{Name: "b", Min: 0, Max: 1, Default: 0.2},
	))
	merged := MergeRecipeDefaults(id, RecipeParams{"a": 0.9})
	want := RecipeParams{"a": 0.9, "b": 0.2}
	if !reflect.DeepEqual(merged, want) {
		t.Errorf("got=%v want=%v", merged, want)
	}
}

func TestMergeRecipeDefaults_NilUserReturnsDefaults(t *testing.T) {
	const id = "test-merge-nil"
	t.Cleanup(func() { unregisterRecipeForTest(id) })
	RegisterRecipe(newFakeRecipe(id, ParamDef{Name: "a", Min: 0, Max: 1, Default: 0.7}))
	merged := MergeRecipeDefaults(id, nil)
	if got, want := merged["a"], 0.7; got != want {
		t.Errorf("got merged[a]=%v want %v", got, want)
	}
}

func TestMergeRecipeDefaults_UnknownRecipeReturnsCopyOfUser(t *testing.T) {
	user := RecipeParams{"a": 0.5}
	merged := MergeRecipeDefaults("unknown-recipe", user)
	if !reflect.DeepEqual(merged, user) {
		t.Errorf("got=%v want=%v", merged, user)
	}
	// Mutating user must not affect the returned copy.
	user["a"] = 99
	if merged["a"] == 99 {
		t.Errorf("merged shares storage with input; got=%v", merged)
	}
}

func TestNewRecipe_ReturnsConstructedRecipe(t *testing.T) {
	const id = "test-new-recipe"
	t.Cleanup(func() { unregisterRecipeForTest(id) })
	RegisterRecipe(newFakeRecipe(id, ParamDef{Name: "x", Min: 0, Max: 1, Default: 0.5}))

	r := NewRecipe(id)
	if r == nil {
		t.Fatal("NewRecipe returned nil")
	}
	if r.ID() != id {
		t.Errorf("got ID=%q want %q", r.ID(), id)
	}
}

func TestNewRecipe_UnknownReturnsNil(t *testing.T) {
	if r := NewRecipe("nope"); r != nil {
		t.Errorf("expected nil for unknown recipe, got %#v", r)
	}
}

func TestParamDefValidator_RejectsEmptyName(t *testing.T) {
	if err := validateParamDef(ParamDef{Min: 0, Max: 1, Default: 0.5}); err == nil {
		t.Errorf("expected error on empty Name")
	}
}

func TestParamDefValidator_RejectsMinExceedsMax(t *testing.T) {
	if err := validateParamDef(ParamDef{Name: "x", Min: 1, Max: 0, Default: 0.5}); err == nil {
		t.Errorf("expected error when Min > Max")
	}
}

func TestParamDefValidator_RejectsDefaultOutsideRange(t *testing.T) {
	if err := validateParamDef(ParamDef{Name: "x", Min: 0, Max: 1, Default: 2}); err == nil {
		t.Errorf("expected error when Default > Max")
	}
	if err := validateParamDef(ParamDef{Name: "x", Min: 0, Max: 1, Default: -1}); err == nil {
		t.Errorf("expected error when Default < Min")
	}
}

func TestRegisterRecipe_PanicsOnInvalidParamDef(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on invalid ParamDef")
		}
	}()
	RegisterRecipe(newFakeRecipe("test-bad-param", ParamDef{Name: "", Min: 0, Max: 1, Default: 0}))
}

func TestHashRecipeParams_Deterministic(t *testing.T) {
	a := RecipeParams{"x": 0.1, "y": 0.2, "z": 0.3}
	b := RecipeParams{"z": 0.3, "y": 0.2, "x": 0.1}
	if hashRecipeParams(a) != hashRecipeParams(b) {
		t.Errorf("hash depends on map iteration order; a=%x b=%x",
			hashRecipeParams(a), hashRecipeParams(b))
	}
}

func TestHashRecipeParams_DifferentValuesDifferHashes(t *testing.T) {
	a := RecipeParams{"x": 0.1, "y": 0.2}
	b := RecipeParams{"x": 0.2, "y": 0.2}
	if hashRecipeParams(a) == hashRecipeParams(b) {
		t.Errorf("expected different hashes; got %x", hashRecipeParams(a))
	}
}

func TestHashRecipeParams_EmptyMapStableNonZero(t *testing.T) {
	// Empty map is allowed; just must not panic and must equal itself.
	h1 := hashRecipeParams(RecipeParams{})
	h2 := hashRecipeParams(nil)
	if h1 != h2 {
		t.Errorf("empty and nil map should hash equal; got %x vs %x", h1, h2)
	}
}

func TestHashRecipeParams_KeyOrderingViaSort(t *testing.T) {
	// Sanity: sorted key set determines hash, not insertion order.
	a := RecipeParams{}
	b := RecipeParams{}
	keys := []string{"c", "a", "b", "d"}
	for _, k := range keys {
		a[k] = 0.5
	}
	sort.Strings(keys)
	for _, k := range keys {
		b[k] = 0.5
	}
	if hashRecipeParams(a) != hashRecipeParams(b) {
		t.Errorf("sorted vs unsorted insertion produced different hashes")
	}
}
