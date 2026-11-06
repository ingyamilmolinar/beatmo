# Config-only Synth Engine — Phase C (one ABI table, strict reads, slot-scoped families, FINISH stage) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **Repo rule:** never `git add` / `git commit` / `git stash` (the user commits). Every "commit" step is a VERIFY CHECKPOINT: run the named suite, read the output, append a ledger line.

**Goal:** The modular param block has ONE source of truth (a Go table that generates the C struct, the JS ABI and the identity map), the C engine reads every param directly (no NaN / NULL / "0 means default" fallback path), family blocks are sized to what a voice can actually use, and the post-processing knobs become a visible, editable FINISH stage.

**Architecture:** `modularFields` (Go) lists every field once — name, slot count, identity, doc. `cmd/gen-synth-abi` emits `src/c/modular_params.gen.h` (the `modular_params` struct + a size assert) and `src/js/synth_param_abi.gen.js`; Go marshals by copying a flat `[]float64` in schema order straight into C (no hand-written Go struct, no field-by-field marshal). The C engine becomes a pure function of that block: every fallback literal that used to stand in for an absent/zero value is spelled out in the instrument seeds instead (bit-exact), and the NULL-params entry point is deleted. Family-specific per-slot fields shrink from 12 slots to the family's capacity (1, cymbal 3). The post stage's per-op gates, op-order switch and `post_enabled` collapse into always-on ops (each a no-op at identity) plus an explicit `out_clamp`; those knobs surface in a new FINISH Synth-tab stage.

**Tech Stack:** C (`src/c`, native via cgo `build/libdrums.a`, browser via emscripten `src/js/drums.single.js`), Go (`src/go/internal/audio`, `src/go/internal/ui`, `src/go/cmd/gen-synth-abi`, `src/go/cmd/gen-modular-instruments`), JS (`src/js/audio.js`, `src/js/synth_render_worker.js`).

**Spec:** roadmap in `docs/superpowers/plans/2026-10-02-config-only-synth-engine.md` (Phase C row: "ABI reshape: family blocks slot-scoped; Go struct + marshal GENERATED from the schema (7 hand edit points → 1); `kp_get`/`cym_modal_param`/`mp_get` strict — no NaN/0 'use the C default' path, every field required (kills silent zeros)") + user decisions 2026-10-03: surface the POST `post_*` knobs (Phase B review #5) in Phase C; keep sub-bass at its normalised level (review #7). Phase B ledger: `.superpowers/sdd/2026-10-02-config-only-synth-engine/progress.md`.

## Global Constraints

- Bundled Go only: `.tools/go/bin/go`. Fast path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ...`; real path: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ...`.
- Concurrent sessions edit the tree: run Go tests with `-overlay=$S/overlay/overlay.json` after `python3 $S/refresh_overlay.py` (`$S` = this session's scratchpad). Never touch the audio-lane, groove-template/gen-showcase or startup-demo files.
- After ANY C edit: `make build/libdrums.a` (cgo links a PREBUILT archive). Browser: `rm -f src/js/drums.single.js && make wasm` (a Makefile/header-only change does not rebuild the glue on its own).
- Regenerate both ABI files with `make gen-synth-abi` (or `go run ./cmd/gen-synth-abi [-c] -o <file>`; never redirect into the header — the shell truncates it before the cgo build that includes it).
- **No sound changes in Phase C.** Every default render stays BIT-IDENTICAL. The only allowed engine-case changes are the exact sets a task names (a value that used to be silently replaced by a C default now means itself).
- Never commit audio; render dumps live under `$BEATMO_RENDER_PARITY_DIR` (`~/.beatmo/render-parity`).
- UI changes need a real screenshot, opened and inspected (CLAUDE.md), captured with `XDG_CONFIG_HOME=$S/xdg`; mobile single scene: `-mobile -scene-pass mobile -win-w 390 -win-h 844` on a `500x900` xvfb screen.

## Review Focus

1. **A value of 0 that used to mean "default" now means 0.** Cymbal `rattle` 0 → previously 1.0, now no rattle; transplanted-voice `gen_gain` 0 → previously 1.0, now silent; FM op ratio 0 → previously 1. A user at a knob's minimum must hear the minimum, and a shipped instrument must not move. Pinned by Task C2's `TestZeroIsARealValue` (engine render at 0 differs from the old default) + the engine parity gate (default renders unchanged).
2. **The browser renders a voice with no param block** (an id whose params are not known yet, the engine warm-up). With `render_modular` (NULL params) deleted, JS must build an identity block, or the voice is silent / throws. Pinned by Task C2's browser run (`modular_synth_audible`, `wasm_bridge_smoke`, `synth_param_block_double`).
3. **Generated C header and JS ABI drift apart** (someone edits `modular_params.gen.h` by hand, or regenerates one file and not the other). Pinned by Task C1's `TestGeneratedCHeaderIsCurrent` + the existing JS gen drift test, both fed from `modularFields`.
4. **A project / instrument bundle / user recipe naming a param that no longer exists** (`post_enabled`, `post_tone_on`, `gen2_kick_h2`) or placing a family voice in a slot beyond its capacity. Must be rejected with a message naming the param (explicit opens) or skipped with a WARN (boot stores) — never silently dropped and never rendered as a wrong voice. Pinned by Task C6.
5. **The FINISH stage's knobs collide with POST's** ("Pitch", "Drive", "Decay" twice on one screen) or a FINISH knob is dead (post_decay with rate 0, post_tone > 0). Pinned by Task C5's display-name uniqueness test + Task C4's identity changes (rate default 8, tone range −1..0).

---

## File map

- **Modify** `src/go/internal/audio/render_parity_tool_test.go` — engine knob cases + drop/allow globs (C0).
- **Create** `src/go/internal/audio/modular_fields.go` — the one ABI table (C1).
- **Modify** `src/go/internal/audio/synth_param_schema.go` — schema / index / identity derived from `modularFields`; phase tables deleted (C1).
- **Create** `src/c/modular_params.gen.h` (generated); **Modify** `src/c/modular.h` — the hand-written struct replaced by `#include "modular_params.gen.h"` (C1).
- **Modify** `src/go/cmd/gen-synth-abi/main.go` — `-c` mode emits the C header (C1).
- **Modify** `src/go/internal/audio/modular_c.go` — `ModularParams` struct, `toCModularParams`, field-by-field `recipeParamsToModular` deleted; `modularBlock` flat marshal (C1).
- **Modify** tests using `ModularParams` (modular_render_test, modular_stage_io_test, modular_genbank_render_test, modular_golden_test, modular_gen_slot_count_test, instrument_seeds_test, modular_modulator_stages_test, modular_blown_test, filtenv_monotonic_test, all_instruments_pitch_table_test, modular_param_schema_test) + `modular_params_f32_round.go`, `drums_c.go`, `synth_registry_entries_native.go` (C1).
- **Modify** `src/c/modular.c`, `src/c/modular_stages.c`, `src/c/synth_params.h` — strict reads (C2), slot capacity guards (C3), post fold + `out_clamp` (C4).
- **Modify** `src/js/audio.js`, `src/go/cmd/gen-modular-instruments/gen.go` (+ regenerated `src/js/modular_instruments.gen.js`), `Makefile` — no NULL render (C2).
- **Modify** `src/go/internal/audio/modular_instruments.go` / `instrument_seeds.go` — seeds spell former C fallbacks (C2), post keys folded (C4).
- **Modify** `src/go/internal/audio/modular_recipe.go`, `modular_family_stages.go` — post ParamDefs visible (C4/C5).
- **Modify** `src/go/internal/ui/synth_panel_zone.go`, `synth_macro_groups.go`, `synth_param_display.go`, `plain_english.go`, `synth_focus_graph.go`, `src/go/internal/i18n/{keys.go,catalog_en.go,catalog_es.go}`, `scene_catalog.go` — FINISH stage (C5).
- **Modify** `src/go/internal/audio/synth_recipe.go` (`SetInstrumentParam`), `recipe_overrides.go`, `src/go/internal/ui/import.go` / `project_file.go`, `internal/ui/instrument_import.go` — unknown-name rejection (C6).
- **Modify** docs: `CLAUDE.md`, `AGENTS.synth.md`, `AGENTS.synth.instruments.md` (C7).

---

### Task C0: The parity gate renders the ENGINE at knob extremes

**Why:** the Phase A/B "51,316 cases" are `RenderInstrumentPreviewWithOverrides` — the pure-Go preview approximator, not the C engine. Only the 150 default renders exercise C. Phase C changes behaviour exactly at knob extremes (a 0 that used to mean "default"), so the gate must render the engine there.

**Files:**
- Modify: `src/go/internal/audio/render_parity_tool_test.go`
- Test: same file (new `TestEngineParityCasesCoverSeedAndVisibleParams`, `!test && !js`, not env-gated)

**Interfaces:**
- Produces: `engineParityCases(id string) map[string]string` (keys `engine:<param>@min|max|zero`); env `BEATMO_RENDER_PARITY_DROPPED` (comma list of `path.Match` globs over param names whose cases may disappear) and `BEATMO_RENDER_PARITY_ALLOW` (comma list of `path.Match` globs over `<id> <case>` that may change). Used by every later task's verify step.

- [ ] **Step 1: Write the failing test**

```go
// TestEngineParityCasesCoverSeedAndVisibleParams pins what the engine half of
// the parity gate covers: every visible knob AND every param the seed sets,
// rendered through the C engine (not the Go preview) at Min, Max and 0.
func TestEngineParityCasesCoverSeedAndVisibleParams(t *testing.T) {
	cases := engineParityCases("hihat")
	for _, want := range []string{
		"engine:gen1_cym_rattle@zero", // visible modal knob, 0 in range
		"engine:gen1_cym_rattle@max",
		"engine:post_enabled@min",     // hidden, but the seed sets it
	} {
		if _, ok := cases[want]; !ok {
			t.Errorf("engine case %q missing (have %d cases)", want, len(cases))
		}
	}
	again := engineParityCases("hihat")
	for k, h := range cases {
		if again[k] != h {
			t.Errorf("%s not deterministic", k)
		}
	}
}
```

- [ ] **Step 2: Run it — expect a compile failure (`engineParityCases` undefined)**

Run: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test -overlay=$O -run TestEngineParityCasesCoverSeedAndVisibleParams ./internal/audio/`
Expected: FAIL `undefined: engineParityCases`.

- [ ] **Step 3: Implement**

```go
// engineParityCases renders the C ENGINE (NewRecipe(...).Render, the path every
// note takes) for one instrument with a single param overridden, for every
// visible ParamDef and every param the merged seed sets away from identity, at
// the def's Min, Max and — when 0 lies in [Min,Max] and is not the default — 0.
// 250 ms each keeps the full dump to a few minutes.
func engineParityCases(id string) map[string]string {
	out := map[string]string{}
	recipeID := RecipeForInstrument(id)
	reg := RecipeRegistrations()[recipeID]
	if reg == nil {
		return out
	}
	merged := MergedInstrumentParamsRO(id)
	ident := ModularParamSchemaIdentity()
	sr := SampleRate()
	n := sr / 4
	render := func(over string, v float64) string {
		p := make(RecipeParams, len(merged)+1)
		for k, x := range merged {
			p[k] = x
		}
		p[over] = v
		buf := make([]float32, n)
		NewRecipe(recipeID).Render(buf, sr, n, 0, p)
		f := make([]float64, n)
		for i, s := range buf {
			f[i] = float64(s)
		}
		return parityHash(f)
	}
	for _, d := range reg.Params {
		if d.Name == "noise_seed" {
			continue
		}
		seeded := merged[d.Name] != ident[d.Name]
		if d.Group == SynthHiddenGroup && !seeded {
			continue
		}
		out["engine:"+d.Name+"@min"] = render(d.Name, d.Min)
		out["engine:"+d.Name+"@max"] = render(d.Name, d.Max)
		// Always when 0 is in range — including a seed value of 0, which is
		// exactly where a "0 means the C default" fallback hides.
		if d.Min <= 0 && 0 <= d.Max {
			out["engine:"+d.Name+"@zero"] = render(d.Name, 0)
		}
	}
	return out
}
```

In `renderParityCases(id)` merge the engine cases into the returned map (`for k, v := range engineParityCases(id) { out[k] = v }`). In `compareParityCases`, before reporting a lost case, skip it when its param name (the part between `engine:`/start and `@`) matches a glob in `BEATMO_RENDER_PARITY_DROPPED`; before reporting a changed case, skip it when `id+" "+case` matches a glob in `BEATMO_RENDER_PARITY_ALLOW` (count it as `allowed`). Log `allowed` and `dropped` counts on the summary line. Use `path.Match` for globs.

- [ ] **Step 4: Run it — expect PASS**

Run: same command as Step 2. Expected: `ok`.

- [ ] **Step 5: Baseline + checkpoint**

Run: `make build/libdrums.a && cd src/go && BEATMO_RENDER_PARITY_DIR=$HOME/.beatmo/render-parity BEATMO_RENDER_PARITY_LABEL=phaseC-base xvfb-run -a ../../.tools/go/bin/go test -overlay=$O -run TestRenderParityDump -count=1 -timeout 3600s ./internal/audio/`
Then compare it to itself (`_A=phaseC-base _B=phaseC-base`) — expected: 150 bit-identical, cases all identical, 0 lost. Record the engine-case count in the ledger. This label is the Phase C gate for every later task.

---

### Task C1: One ABI table generates the C struct, the JS ABI and the Go marshal

**Files:**
- Create: `src/go/internal/audio/modular_fields.go`, `src/c/modular_params.gen.h` (generated)
- Modify: `src/go/internal/audio/synth_param_schema.go`, `src/go/cmd/gen-synth-abi/main.go`, `src/c/modular.h`, `src/go/internal/audio/modular_c.go`, `src/go/internal/audio/modular_params_f32_round.go`, `src/go/internal/audio/synth_registry_entries_native.go`, `src/go/internal/audio/drums_c.go`, every test listed in the file map, `src/go/internal/audio/modular_abi_drift_test.go`
- Test: `src/go/internal/audio/modular_abi_drift_test.go` (`TestGeneratedCHeaderIsCurrent`), `src/go/internal/audio/modular_fields_test.go` (`TestModularFieldsReproduceTheShippedLayout`)

**Interfaces:**
- Produces:
  - `type modularField struct { Name string; Slots int; Identity float64; Doc string }` — `Slots == 0` is a global scalar `Name`; `Slots == N` is the per-slot array `gen_<Name>[N]` flattening to `gen1_<Name>..genN_<Name>`.
  - `var modularFields []modularField` (ABI order).
  - `type modularBlock []float64`; `newModularBlock(p RecipeParams) modularBlock` (missing names → identity); `(b modularBlock) set(name string, v float64)`; `(b modularBlock) get(name string) float64`; `zeroModularBlock() modularBlock` (every field 0, for tests that relied on Go zero values); `renderModularBlock(buf []float32, sampleRate, samples int, b modularBlock)`.
  - `ModularParamSchema()`, `ModularParamSchemaIndexMap()`, `ModularParamSchemaIdentity()` keep their signatures, now derived from `modularFields`.
  - `go run ./cmd/gen-synth-abi -c` writes the C header.

- [ ] **Step 1: Write the failing tests**

`modular_fields_test.go` (no build tag):
```go
// The table must reproduce the shipped layout exactly before anything moves:
// same names, same order, same identities as the pre-table schema (frozen copy
// in testdata/modular_schema_pre_c1.json, written by Step 3a).
func TestModularFieldsReproduceTheShippedLayout(t *testing.T) {
	raw, err := os.ReadFile("testdata/modular_schema_pre_c1.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []struct {
		Name     string
		Identity float64
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	got := ModularParamSchema()
	ident := ModularParamSchemaIdentity()
	if len(got) != len(want) {
		t.Fatalf("schema has %d fields, shipped layout %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w.Name || ident[w.Name] != w.Identity {
			t.Fatalf("field %d: got %s=%v, want %s=%v", i, got[i], ident[got[i]], w.Name, w.Identity)
		}
	}
}
```

`modular_abi_drift_test.go` (add):
```go
// TestGeneratedCHeaderIsCurrent regenerates modular_params.gen.h and diffs it.
func TestGeneratedCHeaderIsCurrent(t *testing.T) {
	out, err := exec.Command(goBin(t), "run", "../../cmd/gen-synth-abi", "-c").Output()
	if err != nil {
		t.Fatalf("gen-synth-abi -c: %v", err)
	}
	have, err := os.ReadFile("../../../c/modular_params.gen.h")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, have) {
		t.Fatal("src/c/modular_params.gen.h is stale: run `go run ./cmd/gen-synth-abi -c -o ../c/modular_params.gen.h`")
	}
}
```
(`goBin` = the helper the existing JS-gen drift test uses; reuse it — find it with `grep -n "gen-synth-abi" internal/audio/*_test.go`.) Point `TestModularParamsSchemaMatchesCStruct`'s header path at `modular_params.gen.h`.

- [ ] **Step 2: Freeze the shipped layout, then run — expect FAIL**

Step 2a (one-off, before any edit): write `testdata/modular_schema_pre_c1.json` from the CURRENT code with a scratch test that marshals `[]{Name, Identity}` over `ModularParamSchema()`; delete the scratch test.
Step 2b: Run `-run 'TestModularFieldsReproduceTheShippedLayout|TestGeneratedCHeaderIsCurrent'`.
Expected: the layout test PASSES (old code), the header test FAILS (`no such file modular_params.gen.h` / unknown flag `-c`). The layout test is the refactor's safety net — it must stay green through Step 3.

- [ ] **Step 3: Build the table and derive everything from it**

3a. Generate the table literal from the current phase tables with a scratch test (walk `modularParamSchema` in order; a run of `gen1_X..gen12_X` becomes one row `{Name: "X", Slots: 12}`; a global becomes `{Name: name}`; identity from `ModularParamSchemaIdentity()`; `Doc` = the C comment for that field in `modular.h`, copied by hand for the globals and per-field for slot arrays). Paste it as `var modularFields = []modularField{...}` in `modular_fields.go`, grouped with section comments (`// oscillator`, `// gen bank (generic per-slot)`, `// analytic bass voice`, `// Karplus-Strong`, `// kick`, …). Delete the scratch test.

3b. In `synth_param_schema.go` replace `modularParamSchema`, `ModularParamSchemaIdentity` and every `modularGenSlotFields*` / `modularGlobalsPhase*` table with derivations over `modularFields`:
```go
var modularParamSchema = func() []string {
	var s []string
	for _, f := range modularFields {
		if f.Slots == 0 {
			s = append(s, f.Name)
			continue
		}
		for k := 1; k <= f.Slots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	return s
}()

func ModularParamSchemaIdentity() map[string]float64 {
	out := make(map[string]float64, len(modularParamSchema))
	for _, f := range modularFields {
		if f.Slots == 0 {
			out[f.Name] = f.Identity
			continue
		}
		for k := 1; k <= f.Slots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Identity
		}
	}
	return out
}
```
Callers that ranged over a phase table (e.g. `modular_recipe.go`'s `for _, f := range modularGenSlotFieldsPhase3Kick`, `IsKickPhysicalField`, `IsCymbalModalField`) range over a named subset instead — add `modularFieldsIn(names ...string) []modularField` or keep the field-name lists they need as plain `[]string`; the bounds maps in `modular_recipe.go` stay (they are UI metadata).

3c. `cmd/gen-synth-abi`: add `-c`. Header output:
```c
/* Code generated by cmd/gen-synth-abi -c. DO NOT EDIT.
 * Source of truth: src/go/internal/audio/modular_fields.go */
#ifndef MODULAR_PARAMS_GEN_H
#define MODULAR_PARAMS_GEN_H

#define MODULAR_PARAM_COUNT 1380

typedef struct {
    double osc_type; /* 0=sine 1=saw ... */
    ...
    double gen_source[12]; /* 0=off 1=osc 2=noise-tap ... */
    ...
} modular_params;

_Static_assert(sizeof(modular_params) == MODULAR_PARAM_COUNT * sizeof(double),
               "modular_params must be a dense array of doubles");
#endif
```
The generator needs the table: export `audio.ModularFieldsForCodegen() []struct{Name string; Slots int; Doc string}`.

3d. `modular.h`: delete the hand-written `typedef struct {...} modular_params;` and `#include "modular_params.gen.h"` in its place (keep every other declaration). Regenerate the header and the JS ABI; `git diff src/js/synth_param_abi.gen.js` must be EMPTY.

3e. `modular_c.go`: delete `type ModularParams`, `toCModularParams`, the field-by-field `recipeParamsToModular`; add
```go
// modularBlock is modular_params as a flat array: element i is
// ModularParamSchema()[i]. The C struct is a dense array of doubles
// (static-asserted in modular_params.gen.h), so the block is passed as-is.
type modularBlock []float64

var modularSchemaIndex = ModularParamSchemaIndexMap()

func newModularBlock(p RecipeParams) modularBlock {
	ident := sharedModularParamSchemaIdentity()
	b := make(modularBlock, len(modularParamSchema))
	for i, name := range modularParamSchema {
		if v, ok := p[name]; ok {
			b[i] = v
		} else {
			b[i] = ident[name]
		}
	}
	return b
}

func zeroModularBlock() modularBlock { return make(modularBlock, len(modularParamSchema)) }

func (b modularBlock) set(name string, v float64) { b[modularSchemaIndex[name]] = v }
func (b modularBlock) get(name string) float64    { return b[modularSchemaIndex[name]] }

func renderModularBlock(buf []float32, sampleRate, samples int, b modularBlock) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 || len(b) != len(modularParamSchema) {
		return
	}
	C.render_modular_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples),
		(*C.modular_params)(unsafe.Pointer(&b[0])))
}
```
`nativeModularRecipe.Render`: `b := newModularBlock(p); b.set("noise_seed", float64(variant)); renderModularBlock(buf, sampleRate, samples, b)`. `modularPadVoiceParams`, `bakedModularRender`, `roundModularParamsToFloat32` (now: round every element of a copy of the block) follow.

3f. Tests: rewrite `ModularParams` uses mechanically — `p := identityModularParams()` → `p := newModularBlock(nil)`; a `ModularParams{...}` literal → `p := zeroModularBlock()` + one `p.set(...)` per listed field (Go zero values for the rest, exactly as before); `p.OscType = 1` → `p.set("osc_type", 1)`; `renderModularP(buf, sr, n, p)` → `renderModularBlock(buf, sr, n, p)`; `func(*ModularParams)` mutators → `func(modularBlock)`. The Go-field → schema-name mapping is the deleted `recipeParamsToModular` body (`OscType: get("osc_type")` …): keep a copy in the scratchpad while rewriting.

- [ ] **Step 4: Run — expect PASS**

Run: `make build/libdrums.a && cd src/go && ../../.tools/go/bin/go generate ./internal/audio/ 2>/dev/null; ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -overlay=$O ./internal/audio/ && xvfb-run -a ../../.tools/go/bin/go test -overlay=$O ./internal/audio/ ./cmd/gen-synth-abi/`
Expected: `ok` both (every golden unchanged — the layout did not move).

- [ ] **Step 5: Checkpoint**

Parity `phaseC-base` vs `C1`: 150 bit-identical, all cases identical, 0 lost. `make wasm`; browser `wasm_bridge_smoke`, `synth_param_block_double`, `xplat_audio_compare` PASS. Ledger: table rows, header lines, LOC deleted from `modular_c.go`.

---

### Task C2: The C engine reads every param directly — no NULL, NaN or "0 means default" path

**Files:**
- Modify: `src/c/modular.c` (`render_modular` + `have`/`zero` path, `mp_get` ×68, FM `ratio > 0 ? ratio : 1`), `src/c/modular_stages.c` (`kp_get` ×217, `cym_modal_param` ×22, `cym_modal_level` ×2, `slotGain`, formant `shift <= 0 → 1`), `src/c/synth_params.h` (`kp_get` deleted), `Makefile` (`_render_modular` dropped from `EXPORTED_FUNCTIONS`), `src/js/audio.js` (no-params and engine-warm paths build an identity block), `src/go/cmd/gen-modular-instruments/gen.go` (RENDER value `'render_modular_p'`), `src/go/internal/audio/modular_c.go` (`renderModular` → identity block), seeds in `modular_instruments.go` / `instrument_seeds.go`
- Test: `src/go/internal/audio/strict_params_test.go` (`TestZeroIsARealValue`, `TestNoFallbackHelpersRemainInC`), existing identity tests updated (`TestGenBankIdentityIsByteNeutral`, `TestModularIdentityGolden`, `TestModulatorStagesIdentityIsByteNeutral`, `modular_golden_test.go`'s NULL arm)

**Interfaces:**
- Consumes: `newModularBlock`, `renderModularBlock` (C1); `engineParityCases` + `BEATMO_RENDER_PARITY_ALLOW` (C0).
- Produces: C has no default-substitution helper; `render_modular_p(out, sr, n, NULL)` is no longer accepted (NULL → silent output + early return, documented). JS always passes a block.

**The rule** for every C site touched: a branch that maps a VALID input (0, a positive value) to a different value is a *default fallback* → delete it and read the param. A branch that maps an INVALID input (negative rate, NaN from nowhere) to the nearest valid value is a *structural clamp* → keep it, and say so in a comment. `cym_modal_param(v, def)` is a fallback for `v == 0` and a clamp for `v < 0` → becomes `fmax(v, 0.0)` where 0 is valid, and a seed value where the default was > 0.

- [ ] **Step 1: Write the failing tests**

```go
//go:build !test && !js

// TestZeroIsARealValue: a knob turned to 0 must sound like 0, not like the C
// default it used to be swapped for (Phase C review focus 1).
func TestZeroIsARealValue(t *testing.T) {
	for _, c := range []struct{ id, param string }{
		{"hihat", "gen1_cym_rattle"}, // was: 0 → 1.0 (full rattle)
		{"ride", "gen1_cym_rattle"},
	} {
		zero := engineCaseRender(c.id, c.param, 0)
		dflt := engineCaseRender(c.id, c.param, 1.0)
		if equalF32(zero, dflt) {
			t.Errorf("%s %s=0 renders exactly like %s=1: 0 is still swapped for the C default", c.id, c.param, c.param)
		}
	}
}

// TestNoFallbackHelpersRemainInC: the default-substitution helpers are gone.
func TestNoFallbackHelpersRemainInC(t *testing.T) {
	for _, f := range []string{"../../../c/modular.c", "../../../c/modular_stages.c", "../../../c/synth_params.h"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"kp_get(", "mp_get(", "cym_modal_param(", "cym_modal_level(", "slotGain = (gain > 0.0)", "const modular_params zero"} {
			if bytes.Contains(src, []byte(banned)) {
				t.Errorf("%s still contains %q", f, banned)
			}
		}
	}
}
```
`engineCaseRender(id, param, v) []float32` = the render closure of `engineParityCases` lifted into a helper (refactor C0's closure into it). `equalF32` compares bit patterns.

- [ ] **Step 2: Run — expect FAIL** (rattle 0 renders like 1; every banned helper present).

- [ ] **Step 3: Spell every fallback the shipped seeds rely on**

Probe (scratch test, delete after): for every instrument, for every slot `k` whose source reaches a fallback, print the (param, value, fallback) triples where the merged value triggers the fallback:
- cymbal modal (`gen_cym_variant` 0/4, or 1/5 with `gen_cym_modes >= 1`): every `cym_modal_param` field with value `<= 0` and fallback `!= 0` (tune 1.0, mode_lo 300, mode_hi 18000, env_tail 1.7, hf_corner 10000, tone_f 3000, tone_w 0.6, body_f 1500, body_w 0.15, spark_f 11000, spark_w 0.3, attack 0.0005, noise_d 4.0, rattle 1.0, rattle_rate 2000, rattle_decay 0.06) and the two `cym_modal_level` fields with NaN;
- transplanted voices (source 3–10): `gen_gain <= 0`;
- `osc_type == 4`: `fm_opN_ratio <= 0`;
- formant on (`formant_enabled >= 0.5`, `formant_mix > 0`): `formant_shift <= 0`.
Add each triple to that instrument's seed (`gen<k>_<field>: <fallback>`). Expect a short list (the probe output goes in the ledger). Re-run the parity dump for those ids: default renders must stay bit-identical.

- [ ] **Step 4: Make the C strict**

- `modular.c`: delete `render_modular`, the `have`/`zero` locals and `mp_get`; every `mp_get(p, fb, p->x, have)` → `p->x`; at the top of `render_modular_p`: `if (!params) { memset(out, 0, (size_t)samples * sizeof(float)); return; }`. FM: `out->ops[i].freq_ratio = ratio[i];` and clamp negative ratios to 0 (structural).
- `modular_stages.c`: `kp_get(p->x[k], lit)` → `p->x[k]` (all 217; a scripted rewrite `sed -E 's/kp_get\(([^,]+), [^)]+\)/\1/'` then `make build/libdrums.a` and review the diff by eye for nested parentheses); `cym_modal_param(v, def)` → `fmax(v, 0.0)` (the seeds now carry every positive default); `cym_modal_level(v, def)` → `v`; `slotGain` → `gain`; formant `if (shift <= 0.0f) shift = 1.0f;` → delete (seed carries it) and clamp `shift < 0.01` to 0.01 (structural, a 0 shift has no meaning).
- `synth_params.h`: delete `kp_get`.
- `Makefile`: remove `_render_modular,` from `EXPORTED_FUNCTIONS`.
- `audio.js`: in the voice-render path, when `instrumentParamsFor(id)` returns nothing, build the block from `MODULAR_PARAM_IDENTITY` (same loop, `params = {}`), and always call `'render_modular_p'`; the engine warm (`renderVoiceInWorker('render_modular', …)` / `renderVoiceSync(m, 'render_modular', …)`) passes an identity block to `'render_modular_p'`.
- `gen-modular-instruments`: RENDER value `'render_modular_p'`; regenerate `modular_instruments.gen.js`.
- Go: `renderModular(buf, sr, n)` → `renderModularBlock(buf, sr, n, newModularBlock(nil))`; the identity-vs-NULL tests become identity-block tests (assert the identity block render equals the committed golden).

- [ ] **Step 5: Run — expect PASS**

`make build/libdrums.a`, then `xvfb-run -a go test -overlay=$O -run 'TestZeroIsARealValue|TestNoFallbackHelpersRemainInC|Identity|Golden' ./internal/audio/` → `ok`; then the full audio package (fast + real).

- [ ] **Step 6: Checkpoint**

Parity `phaseC-base` vs `C2` with `BEATMO_RENDER_PARITY_ALLOW` = exactly the cases that set a former-fallback value (`* engine:gen1_cym_*@zero`, `* engine:gen1_cym_*@min` for modal fields with Min 0, `* engine:gen1_gain@zero`, `* engine:fm_op*_ratio@min` when Min 0, `* engine:formant_shift@min` when Min 0). Expected: 150 bit-identical; every changed case is in the allow list; 0 lost. Rebuild WASM (`rm -f src/js/drums.single.js && make wasm`); browser `modular_synth_audible`, `wasm_bridge_smoke`, `synth_param_block_double`, `xplat_audio_compare`, `webaudio_drums_consistency` PASS.

---

### Task C3: Family blocks sized to the family's slot capacity

**Files:**
- Modify: `src/go/internal/audio/modular_fields.go` (per-family `Slots`, rows regrouped into family blocks), `src/c/modular_stages.c` (capacity guard per family voice), `src/c/modular_stages.h` (`MODULAR_GEN_SLOTS` stays 12 for the generic bank), `src/go/internal/audio/modular_recipe.go` (`appendGenSlotDefs` takes the slot count), generated header + JS ABI
- Test: `src/go/internal/audio/slot_capacity_test.go` (`TestFamilyFieldsHaveTheirFamilyCapacity`, `TestEverySeedRespectsSlotCapacity`, `TestFamilyVoiceBeyondCapacityRendersSilence`)

**Interfaces:**
- Produces: `familySlotCapacity(source int) int` (0 for sources without a family block: 1 osc, 2 noise-tap) and generated C `#define MODULAR_<FAMILY>_SLOTS n` for analytic(4), ks(3), kick(5), tom(6), snare(7/8), cym(9), fm(10).

Capacities (from the seed survey 2026-10-03, max slot any shipped seed uses): analytic bass fields (`pitch_env_amt`, `pitch_env_rate`, `harm_mix`, `atk_amt`, `atk_rate`, `sat_k`, `out_scale`) **1**; `ks_*` **1**; `kick_*` **1**; `tom_*` **1**; `snare_*` **1**; `fm_*` (gen-bank FM voice) **1**; `cym_*` **3**. The generic per-slot fields (`source` … `noise_offset`, used up to slot 12) keep **12**. Result: 1,380 → ~410 doubles.

- [ ] **Step 1: Write the failing tests**

```go
func TestFamilyFieldsHaveTheirFamilyCapacity(t *testing.T) {
	want := map[string]int{"kick_h2": 1, "snare_tone2": 1, "tom_sweep": 1, "fm_r1": 1,
		"ks_sustain": 1, "harm_mix": 1, "cym_rattle": 3, "source": 12, "filt_freq": 12}
	for _, f := range modularFields {
		if n, ok := want[f.Name]; ok && f.Slots != n {
			t.Errorf("%s: %d slots, want %d", f.Name, f.Slots, n)
		}
	}
	if n := len(ModularParamSchema()); n > 450 {
		t.Errorf("block is %d doubles; slot-scoped families should leave ~410", n)
	}
}

// Every shipped seed places a family voice only in a slot that family has.
func TestEverySeedRespectsSlotCapacity(t *testing.T) {
	for _, id := range BuiltinInstrumentIDs {
		if err := validateSlotCapacity(MergedInstrumentParamsRO(id)); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}
```
`validateSlotCapacity(p RecipeParams) error` returns an error naming `gen<k>_source` when `k > familySlotCapacity(int(source))` for a family source.
(`!test && !js`) `TestFamilyVoiceBeyondCapacityRendersSilence`: identity block with `gen2_source = 5` (kick, capacity 1) renders all-zero samples.

- [ ] **Step 2: Run — expect FAIL** (every family field has 12 slots; `validateSlotCapacity` undefined).

- [ ] **Step 3: Implement**

- `modular_fields.go`: set `Slots` per the capacity list; move the analytic bass rows out of the generic block into an `// analytic bass voice (source 4)` block (rows may now move freely — every side is generated).
- C voices: at the top of each family slot voice (`modular_gen_slot_bass`, `_ks`, `_kick`, `_tom`, `_snare`, `_clap`, `_cymbal`, `_fm` — names per `grep -n "^static.*modular_gen_slot_" src/c/modular_stages.c`) add `if (k >= MODULAR_<FAMILY>_SLOTS) return;` (render nothing — `out` untouched). Arrays are now `gen_kick_h2[MODULAR_KICK_SLOTS]`; the generated header defines the constants.
- `appendGenSlotDefs(defs, field, slots, bounds, ident)`; `modular_recipe.go` passes each field's `Slots`.
- `validateSlotCapacity` in `slot_capacity.go`; Task C6 wires it into every write boundary.
- Regenerate header + JS ABI, `make build/libdrums.a`.

- [ ] **Step 4: Run — expect PASS** (fast + real audio package).

- [ ] **Step 5: Checkpoint**

Parity `phaseC-C2` vs `C3`: 150 bit-identical, every case identical; lost cases must be 0 (no seed or visible knob used slots beyond capacity — if any is lost, that is a capacity mistake, not a drop). `rm -f src/js/drums.single.js && make wasm`; browser `synth_param_block_double`, `xplat_audio_compare`, `modular_synth_audible`, `wasm_bridge_smoke` PASS; `TestWasmAssetSizeBudget` PASS (the JS ABI shrank).

---

### Task C4: The post stage is a pure function of its knobs

Every shipped instrument has every post value at identity (survey 2026-10-03: 46 instruments set `post_enabled = 1`, none sets a post value; 0 gate-off-with-value conflicts), so the post ops change nothing today — `post_enabled`'s only audible effect is to SKIP the ±1.5 output clamp.

**Files:**
- Modify: `src/c/modular.c` (post block), `src/go/internal/audio/modular_fields.go` (remove `post_enabled`, `post_pitch_on`, `post_decay_on`, `post_body_on`, `post_brightness_on`, `post_tone_on`, `post_drive_on`, `post_order`; add `out_clamp` identity 1.5; `post_decay_rate` identity 8), `src/go/internal/audio/modular_recipe.go` (bounds: `post_tone` −1..0, `out_clamp` 0..4), seeds (46 instruments: `post_enabled: 1` → `out_clamp: 0`; drop `post_*_on`, `post_order`), `src/go/internal/ui/synth_panel_zone.go` (`enableToggleSection`: drop `post_enabled`)
- Test: `src/go/internal/audio/post_stage_test.go` (`TestPostStageHasNoGatesOrderOrEnable`, `TestOutClampIsTheOnlyDifferenceFormerlyPostEnabledMade`)

**Interfaces:**
- Produces: params `post_pitch`, `post_decay`, `post_decay_rate`, `post_body`, `post_brightness`, `post_tone`, `post_drive`, `out_clamp` — the whole post surface Task C5 shows.

- [ ] **Step 1: Write the failing tests**

```go
func TestPostStageHasNoGatesOrderOrEnable(t *testing.T) {
	idx := ModularParamSchemaIndexMap()
	for _, gone := range []string{"post_enabled", "post_order", "post_pitch_on", "post_decay_on",
		"post_body_on", "post_brightness_on", "post_tone_on", "post_drive_on"} {
		if _, ok := idx[gone]; ok {
			t.Errorf("%s still in the param block", gone)
		}
	}
	if _, ok := idx["out_clamp"]; !ok {
		t.Error("out_clamp missing")
	}
}
```
(`!test && !js`) `TestOutClampIsTheOnlyDifferenceFormerlyPostEnabledMade`: for `kick` (formerly post-enabled), rendering with `out_clamp` 0 equals the `phaseC-base` dump bytes (read `$BEATMO_RENDER_PARITY_DIR/phaseC-base/kick.f32` when the env is set, else skip loudly), and with `out_clamp` 1.5 every sample is within ±1.5.

- [ ] **Step 2: Run — expect FAIL** (gates present, `out_clamp` missing).

- [ ] **Step 3: Implement**

`modular.c` post block becomes:
```c
    /* ── POST (finishing ops). Always runs; every op is a no-op at its identity
     * (pitch 0, decay 1, body 0, brightness 0, tone 0, drive 0), so an
     * instrument that does not use them renders exactly as before. Then the
     * output clamp: out_clamp > 0 limits to ±out_clamp, 0 disables it. ── */
    post_params base;
    base.pitch      = p->post_pitch;
    base.decay      = p->post_decay;
    base.tone       = p->post_tone;
    base.drive      = p->post_drive;
    base.body       = p->post_body;
    base.brightness = p->post_brightness;
    post_config cfg = { .decay_rate = p->post_decay_rate,
                        .pitch = 1, .decay = 1, .drive = 1, .tone = 1, .body = 1, .brightness = 1 };
    apply_post_params(out, sampleRate, samples, &base, &cfg);
    double clamp = p->out_clamp;
    if (clamp > 0.0) {
        for (int i = 0; i < samples; i++) {
            if (out[i] > clamp) out[i] = (float)clamp;
            else if (out[i] < -clamp) out[i] = (float)-clamp;
        }
    }
```
(check `post_config`'s field names in `synth_params.h` and write the initializer to match). Delete the `post_order` 1/2 branches. Seeds: the 46 instruments trade `post_enabled: 1` for `out_clamp: 0` and lose every `post_*_on` / `post_order` key; `post_decay_rate` stays where set. `TestSeedParamsAreLiveInTheirOwnInstrumentContext` must stay green (`out_clamp: 0` is live: those instruments exceed ±1.5 or the probe will say so — if a seed's `out_clamp: 0` is inert, drop it from that seed). Regenerate header + JS ABI + `modular_instruments.gen.js`.

- [ ] **Step 4: Run — expect PASS** (fast + real audio + ui packages; `TestSynthGoldenByteIdentity` unchanged).

- [ ] **Step 5: Checkpoint**

Parity `C3` vs `C4` with `BEATMO_RENDER_PARITY_DROPPED=post_enabled,post_order,post_*_on` and `BEATMO_RENDER_PARITY_ALLOW` = `* engine:post_decay@*` (rate identity 0 → 8 makes decay audible on formerly rate-0 instruments) and `* engine:post_tone@max` (range now ends at 0). Expected: 150 bit-identical; changes only in the allow list. WASM rebuild + browser `modular_synth_audible`, `xplat_audio_compare` PASS.

---

### Task C5: FINISH — the post knobs become a visible Synth-tab stage

**Files:**
- Modify: `src/go/internal/audio/modular_recipe.go` (post ParamDefs: `Group: "finish"`, labels below), `src/go/internal/audio/modular_stage_params.go` (`modularStageGroups` gains `"finish"` so the names count as stage params), `src/go/internal/ui/synth_panel_zone.go` (`synthSectionFinish`: const, `unifiedSynthSectionOrder` after POST, `sectionLabel` "FINISH", `sectionLabelKey`, `sectionSubtitleKey`, `modularStageGroupSection("finish")`), `src/go/internal/ui/synth_macro_groups.go` (melodic claims + `Stages` lists), `src/go/internal/ui/synth_param_display.go` (`synthParamNameKeys`), `src/go/internal/ui/plain_english.go`, `src/go/internal/ui/synth_focus_graph.go` (`synthFocusRenderers["finish"]`, `post_pitch` in `isSynthPitchControl`), `src/go/internal/i18n/{keys.go,catalog_en.go,catalog_es.go}`, `src/go/internal/ui/scene_catalog.go`, the stage-order test maps (`synth_unified_sections_test.go` rank map; `synth_stage_test.go` key lists)
- Test: `src/go/internal/ui/synth_finish_stage_test.go` (`TestFinishStageShowsEveryPostKnob`, `TestFinishKnobNamesDoNotCollide`, `TestFinishKnobEditIsAudible`)

Labels (EN / ES), units, ranges:
| param | label EN | label ES | unit | range |
|---|---|---|---|---|
| post_pitch | Resample | Remuestreo | st | −24..24 |
| post_decay | Fade | Desvanecer | × | 0..4 |
| post_decay_rate | Fade Speed | Velocidad | /s | 0..64 |
| post_body | Low Boost | Graves | % | 0..1 |
| post_brightness | Air | Aire | % | 0..1 |
| post_tone | Darken | Oscurecer | — | −1..0 |
| post_drive | Saturate | Saturación | % | 0..1 |
| out_clamp | Clamp | Límite | — | 0..4 (More tier) |

Stage: FINISH (ES "ACABADO" — POST is already "FINAL" in Spanish), subtitle "Shape the finished sound" / "Da forma al sonido final". No enable param (`enableNone`) → always shown in the strip, after POST. Melodic category claims: `post_decay`, `post_decay_rate` → length; `post_body`, `post_drive` → warmth; `post_brightness`, `post_tone` → brightness; `post_pitch` → sound; `out_clamp` → warmth. Percussion: move `post_decay_rate` from pitchdrop to length; add `synthSectionFinish` to the `Stages` of boom/pitchdrop/tone/length/grit; `out_clamp` → grit. Focus graph: `"finish": conceptFocusLevelWave`; `post_pitch` joins `isSynthPitchControl`.

- [ ] **Step 1: Write the failing tests** (real input through `Game.Update`, the `newSynthTabGame` helper)

```go
// Every post knob is reachable on a melodic AND a percussion instrument.
func TestFinishStageShowsEveryPostKnob(t *testing.T) {
	for _, inst := range []string{"kick", "violin"} {
		g := newSynthTabGame(t)
		g.drum.SetInstrument(0, inst)
		g.drum.eqPanelZone.SetActiveTab(TabSynth)
		g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
		if !g.drum.SelectSynthSectionByLabel("FINISH") {
			t.Fatalf("%s: no FINISH stage", inst)
		}
		have := map[string]bool{}
		for _, k := range g.drum.synthSelectedSection().knobIdxs {
			have[g.drum.instEditorBindings[k].def.Name] = true
		}
		for _, p := range []string{"post_pitch", "post_decay", "post_decay_rate", "post_body",
			"post_brightness", "post_tone", "post_drive", "out_clamp"} {
			if !have[p] {
				t.Errorf("%s: FINISH lacks %s", inst, p)
			}
		}
	}
}

// No two visible knobs on one instrument share a display name.
func TestFinishKnobNamesDoNotCollide(t *testing.T) {
	for _, inst := range []string{"kick", "snare", "violin", "fm-bass"} {
		seen := map[string]string{}
		reg := audio.RecipeRegistrations()[audio.RecipeForInstrument(inst)]
		for _, d := range reg.Params {
			if synthParamHiddenInContext(reg.ID, d, audio.MergedInstrumentParamsRO(inst)) {
				continue
			}
			n := synthParamDisplayName(d)
			if other, dup := seen[n]; dup {
				t.Errorf("%s: %q names both %s and %s", inst, n, other, d.Name)
			}
			seen[n] = d.Name
		}
	}
}
```
(Match the real signatures of `synthParamHiddenInContext` / `synthParamDisplayName` — check them first with `grep -n "^func synthParamHiddenInContext\|^func synthParamDisplayName" internal/ui/*.go`.) `TestFinishKnobEditIsAudible` (`!test`): turn `post_body` to 1 on `kick` through `audio.SetInstrumentParam`, render, compare to the default render — must differ.

- [ ] **Step 2: Run — expect FAIL** (no FINISH section; display names collide: "Pitch", "Drive", "Decay").

- [ ] **Step 3: Implement** the wiring listed under Files, in this order: ParamDefs (group/label/unit/tier) → `modularStageGroups["finish"]` → section const/order/labels/i18n keys (EN+ES, `AllKeys()`) → `modularStageGroupSection` → category tables → display-name keys + plain-English map entries (`resample`, `fade`, `fade speed`, `low boost`, `air`, `darken`, `saturate`, `clamp`) → focus renderer → guard-test maps (`TestSynthUnified_EverySynthRecipeUsesStandardOrder` rank, `TestSynthStageKeysPresentBothLocales` lists). Scenes in `scene_catalog.go`:
```go
	{Name: "crop_synth_finish_kick", Description: "Synth tab — Kick drilled into FINISH: Resample, Fade, Fade Speed, Low Boost, Air, Darken, Saturate (+ Clamp under More)",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabStageSetup("kick", "FINISH", ""),
		MobileSetup: mobileSynthTabStageSetup("kick", "FINISH", "")},
	{Name: "crop_synth_finish_violin_air", Description: "Synth tab focus graph: violin FINISH stage, Air knob selected",
		Subject:     SubjectSynthPanel,
		Setup:       synthTabFocusControlSetup("violin", "FINISH", "post_brightness"),
		MobileSetup: mobileSynthTabFocusControlSetup("violin", "FINISH", "post_brightness")},
```

- [ ] **Step 4: Run — expect PASS**: the three new tests + the full ui + i18n packages (fast) + `TestFinishKnobEditIsAudible` (real).

- [ ] **Step 5: Screenshots (MANDATORY)** — capture `crop_synth_finish_kick`, `crop_synth_finish_violin_air`, `crop_synth_sub_bass_root` (strip now carries FINISH), desktop + mobile; open each PNG; check labels legible, no blank cards, no overlap, the strip pill reads FINISH. Re-capture until right.

- [ ] **Step 6: Checkpoint** — parity `C4` vs `C5`: 150 bit-identical, cases identical except NEW (FINISH knobs now visible → preview cases added). Ledger the scene names.

---

### Task C6: A name the engine does not know is rejected at every write boundary

**Files:**
- Modify: `src/go/internal/audio/synth_recipe.go` (`SetInstrumentParam`), `src/go/internal/audio/recipe_overrides.go` (`ApplyUserRecipeOverrides`, `RegisterUserRecipeFromBase`), `src/go/internal/ui/project_file.go` (`validateProjectRecipes` → also `validateProjectParams`), `src/go/internal/ui/import.go` (call it with the other validators, before any mutation), `src/go/internal/ui/instrument_import.go` (`ApplyInstrumentBundle`: validate every doc first, then register — also fixes the Phase B deferred minor "partial bundle import")
- Test: `src/go/internal/audio/unknown_param_test.go`, `src/go/internal/ui/import_unknown_param_test.go`

**Interfaces:**
- Consumes: `validateSlotCapacity` (C3).
- Produces: `audio.ValidateRecipeParams(p map[string]float64) error` — unknown name (not in `ModularParamSchemaIndexMap()`), non-finite value, or slot-capacity violation → error naming the param.

- [ ] **Step 1: Write the failing tests**

```go
func TestSetInstrumentParamRejectsUnknownNames(t *testing.T) {
	const inst = "kick"
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	SetInstrumentParam(inst, "post_tone_on", 0) // retired in C4
	SetInstrumentParam(inst, "gen2_kick_h2", 1) // beyond capacity since C3
	if _, ok := GetInstrumentParams(inst)["post_tone_on"]; ok {
		t.Error("post_tone_on stored")
	}
	if _, ok := GetInstrumentParams(inst)["gen2_kick_h2"]; ok {
		t.Error("gen2_kick_h2 stored")
	}
}

func TestValidateRecipeParams(t *testing.T) {
	if err := ValidateRecipeParams(map[string]float64{"post_enabled": 1}); err == nil || !strings.Contains(err.Error(), "post_enabled") {
		t.Errorf("unknown name: err = %v", err)
	}
	if err := ValidateRecipeParams(map[string]float64{"gen2_source": 5}); err == nil {
		t.Error("kick voice in slot 2 accepted")
	}
	if err := ValidateRecipeParams(map[string]float64{"gen1_kick_h2": 0.4}); err != nil {
		t.Errorf("valid params rejected: %v", err)
	}
}
```
UI: `TestImportRejectsUnknownSynthParam` — a valid project whose instrument carries `"synth_params": {"post_enabled": 1}` → `Import` returns an error containing `post_enabled`, and the previous circuit is untouched (reuse the side-effect assertions of `TestRejectedImportHasNoSideEffects`). `TestBundleImportValidatesEveryDocFirst` — a two-doc bundle whose second doc is on a dead base registers NOTHING.

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement** `ValidateRecipeParams`; call it in `SetInstrumentParam` (reject → return without storing, WARN log like the non-finite branch), `ApplyUserRecipeOverrides` (boot store: skip the entry + WARN), `RegisterUserRecipeFromBase` (error), `validateProjectParams` (import: hard error before mutation, covering `instruments[].synth_params` and `recipes[].param_seed`), `ApplyInstrumentBundle` (validate all docs, then apply).

- [ ] **Step 4: Run — expect PASS** (audio + ui fast; `import_*` tests real).

- [ ] **Step 5: Checkpoint** — fast + real suites; browser `synth_save_export_import`, `import_synth_render` PASS.

---

### Task C7: Docs, full verification, final review

**Files:**
- Modify: `CLAUDE.md` (the one-config-row gotcha: param block generated from `modular_fields.go`; no C defaults — a seed spells every value; FINISH stage), `AGENTS.synth.md` (ABI section: `modular_fields.go` → `gen-synth-abi` / `-c`; capacity table; strict-read rule), `AGENTS.synth.instruments.md` (post stage: always-on ops + `out_clamp`; seeds must spell former fallbacks), `docs/superpowers/plans/2026-10-02-config-only-synth-engine.md` (Phase C row → link this plan)

- [ ] **Step 1: Docs** as listed; `grep -rn "kp_get\|mp_get\|post_enabled\|post_order\|render_modular\b" CLAUDE.md AGENTS*.md` must return only historical mentions.
- [ ] **Step 2: Full verification** — parity `phaseC-base` vs `phaseC-final` with the C2/C4 allow lists and the C4 drop list (150 bit-identical); fast + real Go suites (failure set must equal the Phase B final set or be explained); `rm -f src/js/drums.single.js && make wasm`; `WASM_PREBUILT=1 make test-browser`; `make coverage-c-check` then `make build/libdrums.a`; `TestWasmAssetSizeBudget`. Dump `phaseC-final` as the Phase D baseline.
- [ ] **Step 3: Final review** — whole-branch reviewer (most capable model) over the Phase C diff with this plan, the Review Focus and the ledger rulings; one fix pass, each fix RED→GREEN.
