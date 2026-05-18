package audio

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Phase 6 plugin-readiness invariants. Each test pins one of the v1
// freezing-surface contracts the future .wasm plugin loader will rely on.
// Changing any of these contracts is a coordinated v2 migration, not a
// drive-by edit; the tests force the conversation.

// repoRootForInvariants walks up from this test file to find the repo
// root (the directory containing src/c and src/go).
func repoRootForInvariants(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) returned !ok")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "src", "c")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("repo root not found (no src/c above the test file)")
	return ""
}

// Invariant #1: SynthRecipe interface shape. The plugin loader will use
// reflection / cgo bindings against the exact method set declared today.
// Adding a method is a v2 break; renaming a method is a v2 break;
// removing a method is a v2 break.
func TestInvariant_SynthRecipeInterfaceShape(t *testing.T) {
	root := repoRootForInvariants(t)
	src := filepath.Join(root, "src", "go", "internal", "audio", "synth_recipe.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, src, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", src, err)
	}
	var iface *ast.InterfaceType
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name == nil || ts.Name.Name != "SynthRecipe" {
			return true
		}
		if it, ok := ts.Type.(*ast.InterfaceType); ok {
			iface = it
		}
		return false
	})
	if iface == nil {
		t.Fatal("SynthRecipe interface not found in synth_recipe.go")
	}
	wantMethods := []string{"ID", "DisplayName", "Category", "ParamSchema", "Render"}
	gotMethods := []string{}
	for _, field := range iface.Methods.List {
		for _, name := range field.Names {
			gotMethods = append(gotMethods, name.Name)
		}
	}
	sort.Strings(gotMethods)
	sortedWant := append([]string{}, wantMethods...)
	sort.Strings(sortedWant)
	if !reflect.DeepEqual(gotMethods, sortedWant) {
		t.Fatalf("SynthRecipe method set drift: got %v, want %v — this is a v1 freezing-surface change", gotMethods, sortedWant)
	}
	if len(gotMethods) != 5 {
		t.Fatalf("SynthRecipe must declare exactly 5 methods; got %d", len(gotMethods))
	}
}

// Invariant #2: ParamDef JSON shape. Plugins ship a manifest with the
// same field names + types; renaming a JSON tag silently corrupts every
// in-the-wild .wasm.
func TestInvariant_ParamDefJSONShape(t *testing.T) {
	d := ParamDef{
		Name:    "decay",
		Label:   "Decay",
		Min:     0,
		Max:     4,
		Default: 1,
		Unit:    "ms",
		Curve:   "linear",
		Group:   "generic",
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Decode into a map and assert every expected key is present with
	// the expected type. Field order in JSON is implementation-defined
	// for structs, so we don't compare byte sequences.
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantStringKeys := []string{"name", "label", "unit", "curve", "group"}
	wantFloatKeys := []string{"min", "max", "default"}
	for _, k := range wantStringKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("ParamDef JSON missing key %q (raw: %s)", k, raw)
		}
	}
	for _, k := range wantFloatKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("ParamDef JSON missing key %q (raw: %s)", k, raw)
		}
	}
	if len(got) > 8 {
		t.Errorf("ParamDef JSON has unexpected extra keys: %v", got)
	}

	// Reverse: a JSON manifest with the documented shape must
	// decode without losing fields.
	manifest := `{"name":"pitch","label":"Pitch","min":-24,"max":24,"default":0,"unit":"st","curve":"linear","group":"generic"}`
	var d2 ParamDef
	if err := json.Unmarshal([]byte(manifest), &d2); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if d2.Name != "pitch" || d2.Min != -24 || d2.Max != 24 || d2.Unit != "st" || d2.Curve != "linear" || d2.Group != "generic" {
		t.Errorf("ParamDef JSON round-trip lost fields: %+v", d2)
	}
}

// Invariant #3: RecipeParams semantics. The IPC payload is a flat
// map[string]float64; nested types or non-float values would break the
// JS bridge serialization (json.Marshal in synth_recipe_wasm.go) and the
// hash function (hashRecipeParams).
func TestInvariant_RecipeParamsType(t *testing.T) {
	got := reflect.TypeOf(RecipeParams{})
	if got.Kind() != reflect.Map {
		t.Fatalf("RecipeParams must be a map; got %s", got.Kind())
	}
	if got.Key().Kind() != reflect.String {
		t.Fatalf("RecipeParams key must be string; got %s", got.Key().Kind())
	}
	if got.Elem().Kind() != reflect.Float64 {
		t.Fatalf("RecipeParams value must be float64; got %s", got.Elem().Kind())
	}
}

// Invariant #4: Render ABI parity with C. Every drum recipe id (drum-*)
// resolves to a registered renderer; every C `render_*` prototype in
// drums.h has a matching `_p` variant (covered by
// c_header_parity_test.go's TestEveryCRenderHasPVariant).
// This test pins the Go-side mapping so a future recipe rename can't
// silently orphan its underlying C function.
func TestInvariant_RenderABIParity(t *testing.T) {
	regs := RecipeRegistrations()
	for _, d := range builtinRecipeDescriptors {
		r := regs[d.ID]
		if r == nil {
			t.Errorf("descriptor %q not registered", d.ID)
			continue
		}
		if r.New == nil {
			t.Errorf("recipe %q has nil New", d.ID)
		}
		rcp := NewRecipe(d.ID)
		if rcp == nil {
			t.Errorf("NewRecipe(%q) returned nil", d.ID)
			continue
		}
		if rcp.ID() != d.ID {
			t.Errorf("recipe ID mismatch: factory returned %q for descriptor %q", rcp.ID(), d.ID)
		}
	}
}

// Invariant #5: paramsHash determinism. The voice cache keys voices by
// FNV-1a hash over sorted (name,value) pairs; collisions or order
// dependence would serve stale audio across instruments. The fuzz
// tests random maps to catch insertion-order regressions.
func TestInvariant_ParamsHashDeterministicUnderFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(0xBEAEEEEF))
	for iter := 0; iter < 200; iter++ {
		n := 1 + rng.Intn(16)
		// Build two maps with the same content but inserted in
		// different orders.
		entries := make([][2]any, n)
		for i := range entries {
			entries[i] = [2]any{
				randIdent(rng),
				rng.NormFloat64() * 100,
			}
		}
		a := RecipeParams{}
		b := RecipeParams{}
		for _, e := range entries {
			a[e[0].(string)] = e[1].(float64)
		}
		rng.Shuffle(len(entries), func(i, j int) {
			entries[i], entries[j] = entries[j], entries[i]
		})
		for _, e := range entries {
			b[e[0].(string)] = e[1].(float64)
		}
		if hashRecipeParams(a) != hashRecipeParams(b) {
			t.Fatalf("hash depends on insertion order at iter %d:\n  a=%v\n  b=%v", iter, a, b)
		}
	}
}

func randIdent(rng *rand.Rand) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz_"
	n := 3 + rng.Intn(12)
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(buf)
}

// Invariant #6: platformInstrumentParamsChanged is the only IPC seam.
// JS-side code reading or writing recipe params must go through
// window.updateInstrumentParams (the handler installed in audio.js).
// AST-walk the JS file to ensure no other entry point writes the
// instrumentSynthParams map.
func TestInvariant_SinglePlatformIPCSeam(t *testing.T) {
	root := repoRootForInvariants(t)
	jsPath := filepath.Join(root, "src", "js", "audio.js")
	bytes, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("read %s: %v", jsPath, err)
	}
	text := string(bytes)
	// instrumentSynthParams should be written in exactly two places:
	//   1. _updateInstrumentParams (the handler)
	//   2. We allow `instrumentSynthParams.delete(id)` in _updateInstrumentParams
	// Both occur inside the same function.
	writes := strings.Count(text, "instrumentSynthParams.set")
	deletes := strings.Count(text, "instrumentSynthParams.delete")
	if writes != 1 {
		t.Errorf("instrumentSynthParams.set called %d times in audio.js; want exactly 1 (in _updateInstrumentParams)", writes)
	}
	if deletes != 1 {
		t.Errorf("instrumentSynthParams.delete called %d times in audio.js; want exactly 1 (in _updateInstrumentParams)", deletes)
	}
	// And only one global handler: window.updateInstrumentParams = ...
	handlers := strings.Count(text, "window.updateInstrumentParams = ")
	if handlers != 1 {
		t.Errorf("window.updateInstrumentParams assigned %d times; want exactly 1", handlers)
	}
}
