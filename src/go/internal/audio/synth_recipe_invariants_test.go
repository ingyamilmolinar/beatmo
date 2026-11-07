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

// Contracts a future .wasm plugin loader relies on: changing one is a coordinated migration, and these force the
// conversation.

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

// Plugins ship a manifest with these names: renaming a JSON tag silently corrupts every one in the wild.
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
	// Struct field order in JSON is not a contract, so keys and types are compared, not bytes.
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

	manifest := `{"name":"pitch","label":"Pitch","min":-24,"max":24,"default":0,"unit":"st","curve":"linear","group":"generic"}`
	var d2 ParamDef
	if err := json.Unmarshal([]byte(manifest), &d2); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if d2.Name != "pitch" || d2.Min != -24 || d2.Max != 24 || d2.Unit != "st" || d2.Curve != "linear" || d2.Group != "generic" {
		t.Errorf("ParamDef JSON round-trip lost fields: %+v", d2)
	}
}

// A flat map[string]float64: nested or non-float values would break the bridge serialization and the hash.
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

// audio.js keeps no parameter maps: the block map is written in exactly one place.
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
