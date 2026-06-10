package audio

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestWiredParamsMatchCConfig is the discipline test that catches drift
// between the Go recipeWiredParams table and the C renderer's actual reads.
//
// For every recipe in builtinRecipeDescriptors:
//  1. Locate the matching render_<id>_p function in drums.c or fmsynth.c.
//  2. If the body is a one-line apply_post_params dispatch, parse the
//     post_config initializer to recover the wired set (Phase 2).
//  3. Otherwise scan the body for sp_<name>(params) reader calls (Phase 1
//     hand-rolled variants).
//  4. Assert the recovered set equals recipeWiredParams[id].
//
// Failure means either (a) the Go table is stale (someone added a knob
// reader in C without updating the table) or (b) the C renderer dropped
// a read (the slider would still draw but do nothing). Both are bugs the
// Synth-tab redesign was built to surface; this test prevents reintroduction.
func TestWiredParamsMatchCConfig(t *testing.T) {
	root := findRepoRoot(t)
	drumsC := mustReadFile(t, filepath.Join(root, "src", "c", "drums.c"))
	fmC := mustReadFile(t, filepath.Join(root, "src", "c", "fmsynth.c"))
	sources := drumsC + "\n" + fmC

	for _, d := range builtinRecipeDescriptors {
		if d.Category == modularRecipeCategory {
			// The modular voice reads modular_params (not the generic sp_*
			// knobs) and exposes its own full ParamDef set, so it is exempt
			// from the wired-generic ⇄ C-reader parity check.
			continue
		}
		wantUnsorted, ok := recipeWiredParams[d.ID]
		if !ok {
			t.Errorf("recipe %q has no entry in recipeWiredParams", d.ID)
			continue
		}
		want := append([]string(nil), wantUnsorted...)
		sort.Strings(want)

		var got []string
		var source string
		var err error
		if IsModularMigratedRecipe(d.ID) {
			// Migrated families no longer have a render_<id>_p C function (deleted
			// in the Phase-2 cutover); their POST stage is configured in Go via the
			// modularMigrations registry's familyPostConfig. Verify the wired generic
			// knobs against THAT config instead of the (deleted) C post_config. This
			// is the C-source-verification side adjusting — the ParamDefs /
			// recipeWiredParams table is unchanged.
			got, source = readsForMigratedFamilyFromGo(d.ID), "Go familyPostConfig (modular migrated)"
		} else {
			got, source, err = readsForRecipeFromC(d.ID, sources)
		}
		if err != nil {
			t.Errorf("recipe %q: %v", d.ID, err)
			continue
		}
		sort.Strings(got)

		if !reflect.DeepEqual(got, want) {
			t.Errorf("recipe %q: C reads %v (via %s), recipeWiredParams says %v",
				d.ID, got, source, want)
		}
	}
}

// TestRecipeWiredParamsCoversAllBuiltins guards against silent drift: any
// recipe added to builtinRecipeDescriptors must also appear in
// recipeWiredParams. The test is intentionally separate from
// TestWiredParamsMatchCConfig so a missing entry isn't reported as a
// confusing C-parse failure.
func TestRecipeWiredParamsCoversAllBuiltins(t *testing.T) {
	for _, d := range builtinRecipeDescriptors {
		if d.Category == modularRecipeCategory {
			continue // modular voice uses ModularSynthParamDefs, not wired-generic
		}
		if _, ok := recipeWiredParams[d.ID]; !ok {
			t.Errorf("builtin recipe %q is missing from recipeWiredParams (add the wired knob list)", d.ID)
		}
	}
	known := make(map[string]bool, len(builtinRecipeDescriptors))
	for _, d := range builtinRecipeDescriptors {
		known[d.ID] = true
	}
	for id := range recipeWiredParams {
		if !known[id] {
			t.Errorf("recipeWiredParams has an entry for unknown recipe %q (not in builtinRecipeDescriptors)", id)
		}
	}
}

// TestNoOpKnobsRemoved enforces the schema-level no-op elimination invariant:
// `attack` and `color` are never declared by any recipe today (no C renderer
// reads them). If a future recipe wires them, the C audit must come first
// — uncomment the entry and the test relaxes automatically.
func TestNoOpKnobsRemoved(t *testing.T) {
	for id, wired := range recipeWiredParams {
		for _, name := range wired {
			if name == "attack" || name == "color" {
				t.Errorf("recipe %q declares %q in recipeWiredParams but no C renderer reads it — confirm the C reader exists before adding", id, name)
			}
		}
	}
}

// TestWiredParamsForRecipeRespectsCanonicalOrder ensures WiredParamsForRecipe
// preserves the ParamDef order from GenericSynthParamDefs (pitch, decay,
// tone, attack, drive, body, color, brightness) for the generic-knob block,
// then appends per-recipe extras (Phase 3 addition) at the end. Section
// rendering relies on this stable [generic..., extras...] order so the
// layout fingerprint stays deterministic.
func TestWiredParamsForRecipeRespectsCanonicalOrder(t *testing.T) {
	canon := GenericSynthParamDefs()
	canonIdx := make(map[string]int, len(canon))
	for i, d := range canon {
		canonIdx[d.Name] = i
	}
	for _, d := range builtinRecipeDescriptors {
		if d.Category == modularRecipeCategory {
			continue // modular voice uses ModularSynthParamDefs, not WiredParamsForRecipe
		}
		got := WiredParamsForRecipe(d.ID)
		if got == nil {
			t.Errorf("recipe %q: WiredParamsForRecipe returned nil", d.ID)
			continue
		}
		extras := recipeExtraParams[d.ID]
		extraIdx := make(map[string]int, len(extras))
		for i, ex := range extras {
			extraIdx[ex.Name] = i
		}
		prevCanon := -1
		seenExtra := false
		seenStage := false
		prevExtra := -1
		for _, def := range got {
			// Phase-8A: migrated recipes carry a THIRD trailing block — the appended
			// modular stage params (osc/env/filter + gain + toggles + post_enabled).
			// Block order is [generic..., extras..., stages...]. Once the stage block
			// starts, no generic/extra may follow.
			if isAppendedStageName(d.ID, def.Name) {
				seenStage = true
				continue
			}
			if cIdx, ok := canonIdx[def.Name]; ok {
				if seenExtra || seenStage {
					t.Errorf("recipe %q: generic param %q came after an extra/stage — wrong block order: %v", d.ID, def.Name, got)
					break
				}
				if cIdx <= prevCanon {
					t.Errorf("recipe %q: generic params out of canonical order: %v", d.ID, got)
					break
				}
				prevCanon = cIdx
				continue
			}
			eIdx, ok := extraIdx[def.Name]
			if !ok {
				t.Errorf("recipe %q: WiredParamsForRecipe returned %q which is neither generic nor a declared extra", d.ID, def.Name)
				continue
			}
			if seenStage {
				t.Errorf("recipe %q: extra param %q came after a stage — wrong block order: %v", d.ID, def.Name, got)
				break
			}
			seenExtra = true
			if eIdx <= prevExtra {
				t.Errorf("recipe %q: extras out of declared order: %v", d.ID, got)
				break
			}
			prevExtra = eIdx
		}
	}
}

// ---- helpers ----

// readsForRecipeFromC returns the wired param names recovered from the
// C source for the given recipe id. Returns the matching strategy used
// ("post_config" or "hand-rolled sp_*") for diagnostic messages.
func readsForRecipeFromC(recipeID, sources string) (names []string, source string, err error) {
	fnName := cFunctionNameFor(recipeID)
	body, ok := extractCFunctionBody(sources, fnName)
	if !ok {
		return nil, "", &recipeAuditError{msg: "no C function found named " + fnName}
	}
	if names, ok := parsePostConfig(body); ok {
		return names, "post_config in " + fnName, nil
	}
	names = parseSpReaders(body)
	if len(names) == 0 {
		return nil, "", &recipeAuditError{msg: "no apply_post_params and no sp_*(params) reads in " + fnName}
	}
	return names, "sp_*(params) reads in " + fnName, nil
}

// readsForMigratedFamilyFromGo derives the wired generic-knob set for a migrated
// recipe from its Go-side familyPostConfig (the modularMigrations registry's
// post field) — the single source of truth the binding feeds applyFamilyPostStage.
// This replaces the deleted C post_config parse for migrated families. The
// gate→knob mapping mirrors parsePostConfig: each enabled post sub-effect names
// the generic knob it reads (decay_rate is the per-recipe time constant, not a
// knob, so it is excluded — exactly as parsePostConfig drops it).
func readsForMigratedFamilyFromGo(recipeID string) []string {
	m, ok := modularMigrations[recipeID]
	if !ok {
		return nil
	}
	cfg := m.post
	var out []string
	if cfg.Pitch {
		out = append(out, "pitch")
	}
	if cfg.Decay {
		out = append(out, "decay")
	}
	if cfg.Tone {
		out = append(out, "tone")
	}
	if cfg.Drive {
		out = append(out, "drive")
	}
	if cfg.Body {
		out = append(out, "body")
	}
	if cfg.Brightness {
		out = append(out, "brightness")
	}
	return out
}

// cFunctionNameFor maps a recipe id to the exported C renderer name. The
// "drum-" prefix is dropped (drum-snare → render_snare_p), the "fm-"
// prefix is kept (fm-bass → render_fm_bass_p), and dashes become underscores.
func cFunctionNameFor(recipeID string) string {
	stem := recipeID
	if strings.HasPrefix(stem, "drum-") {
		stem = stem[len("drum-"):]
	}
	stem = strings.ReplaceAll(stem, "-", "_")
	return "render_" + stem + "_p"
}

// extractCFunctionBody returns the brace-balanced body of a top-level C
// function. Matches the first `EXPORT void NAME(...)` and consumes through
// the matching closing brace.
func extractCFunctionBody(src, name string) (string, bool) {
	idx := strings.Index(src, "EXPORT void "+name+"(")
	if idx < 0 {
		return "", false
	}
	open := strings.IndexByte(src[idx:], '{')
	if open < 0 {
		return "", false
	}
	start := idx + open
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1], true
			}
		}
	}
	return "", false
}

// parsePostConfig recovers the wired knob names from a `post_config cfg = {
// .decay_rate = …, .pitch = 1, .decay = 1, … }` initializer. `decay_rate`
// is excluded (it's the per-recipe time constant, not a knob).
func parsePostConfig(body string) ([]string, bool) {
	re := regexp.MustCompile(`post_config\s+\w+\s*=\s*\{([^}]*)\}`)
	m := re.FindStringSubmatch(body)
	if len(m) != 2 {
		return nil, false
	}
	fieldRe := regexp.MustCompile(`\.(\w+)\s*=\s*[^,}]+`)
	var out []string
	seen := map[string]bool{}
	for _, fm := range fieldRe.FindAllStringSubmatch(m[1], -1) {
		name := fm[1]
		if name == "decay_rate" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out, true
}

// parseSpReaders scans a function body for calls to sp_<name>(params) and
// returns the unique parameter names referenced. Used for the 6 Phase-1
// hand-rolled _p variants that predate apply_post_params. Family-block
// variants (native-deprecation migration) receive a wide *_params struct
// and read the generic knobs through `base` (= &params->base), so that
// receiver is accepted too.
func parseSpReaders(body string) []string {
	re := regexp.MustCompile(`sp_(\w+)\(\s*(?:params|base)\s*\)`)
	var out []string
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		name := m[1]
		// sp_saturate / sp_freq / sp_env_decay / sp_filter_cutoff are
		// derived helpers, not direct param readers. Exclude them.
		if !isDirectParamReader(name) {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// isDirectParamReader returns true for sp_<name> helpers that name a
// synth_params field. Keep in sync with synth_params.h sp_* inline
// helpers; the discipline test fails loudly if a new helper is added.
func isDirectParamReader(name string) bool {
	switch name {
	case "pitch", "decay", "tone", "attack", "drive", "body", "color", "brightness":
		return true
	}
	return false
}

type recipeAuditError struct{ msg string }

func (e *recipeAuditError) Error() string { return e.msg }

// findRepoRoot walks up from this test file's directory until it finds a
// `src/c/drums.c`. Returns the repo root. Fails the test if not found.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "src", "c", "drums.c")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not find repo root (src/c/drums.c) walking up from %s", filepath.Dir(file))
	return ""
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
