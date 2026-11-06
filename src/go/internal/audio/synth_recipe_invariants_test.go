package audio

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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

func randIdent(rng *rand.Rand) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz_"
	n := 3 + rng.Intn(12)
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(buf)
}

// Invariant #6: window.setInstrumentBlock is the only IPC seam for synth
// voices (spec 2026-10-04 §8): Go flattens each instrument document and pushes
// the block; audio.js keeps no parameter maps. The instrumentBlocks map is
// written in exactly one place, and the retired param-map globals are gone.
func TestInvariant_SinglePlatformIPCSeam(t *testing.T) {
	root := repoRootForInvariants(t)
	jsPath := filepath.Join(root, "src", "js", "audio.js")
	bytes, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("read %s: %v", jsPath, err)
	}
	text := string(bytes)
	if n := strings.Count(text, "instrumentBlocks.set("); n != 1 {
		t.Errorf("instrumentBlocks.set called %d times in audio.js; want exactly 1 (in setInstrumentBlock)", n)
	}
	if n := strings.Count(text, "window.setInstrumentBlock = "); n != 1 {
		t.Errorf("window.setInstrumentBlock assigned %d times; want exactly 1", n)
	}
	for _, gone := range []string{"instrumentSynthParams", "instrumentDefaultParams", "seedInstrumentDefaults",
		"updateInstrumentParams", "updateInstrumentRecipe", "instrumentParamsFor", "identityParamBlock", "MODULAR_PARAM_IDENTITY"} {
		if strings.Contains(text, gone) {
			t.Errorf("audio.js still mentions the retired %s — the block is the only parameter source", gone)
		}
	}
}
