# Config-only Synth Engine — Phase B (delete all legacy) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **Repo rule:** never `git add` / `git commit` / `git stash` (the user commits). Every "commit" step is a VERIFY CHECKPOINT. Ledger: `.superpowers/sdd/2026-10-02-config-only-synth-engine/progress.md` (same plan family as Phase A — append, prefix lines with `B<n>`).

**Goal:** Delete every legacy / obsolete / backward-compat path from the synth engine, the import boundary, the C sources and the JS bridge, and make `sub-bass` / `sub-bass-drive` ordinary config-first table rows with an editable Synth-tab stage — while every shipped instrument stays BIT-IDENTICAL.

**Architecture:** Shipped instruments already render through `modularInstrumentDefs` seeds (`synth-modular-*`). Everything else — the 24 legacy recipes, the family legacy→modular translation layer (~4.6k LOC), the 7-field `synth_params` ABI, retired-id / gen_type / schema-less import migrations, test-only and dead C, unused WASM exports, dead JS arms — exists only for old projects or old tests. Order: modernize fixtures (no behavior change) → convert sub-bass (bit-exact) → delete layer by layer, each step gated bit-exact against the Phase A render-parity baseline.

**Tech Stack:** Go (`src/go`), C (`src/c` → `build/libdrums.a` native, `src/js/drums.single.js` via emcc), JS (`src/js/audio.js`, `synth_render_worker.js`), generated `synth_param_abi.gen.js`, `modular_instruments.gen.js`, `instrument_ids_gen.go`.

**Spec:** `docs/superpowers/plans/2026-10-02-config-only-synth-engine.md` (Roadmap row B + user decisions: drop old-project import support entirely; keep a foreign-JSON reject; every shipped sound bit-identical) + the Phase B deletion map (this session, 2026-10-02; facts quoted inline below).

## Global Constraints

- Bundled Go only: `.tools/go/bin/go`; fast suite `-tags test -modfile=go.test.mod`; real-build suite untagged (needs `build/libdrums.a`). `make test-real` is the only runner of `!test` tests — most deleted legacy tests are `!test`, so the fast suite will NOT notice coverage loss.
- After ANY `.c`/`.h` edit: `make build/libdrums.a` (else native tests run stale C) and, before browser tests, `make wasm`.
- Generated files are regenerated, never hand-edited: `make gen-modular-instruments`, `cd src/go && ../../.tools/go/bin/go generate ./internal/audio/` (ids), `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-synth-abi > ../js/synth_param_abi.gen.js` (STDOUT — redirect), `make gen-fundamentals`, `make measure-loudness` (only if its drift test trips).
- **Parity gate (every task):** `BEATMO_RENDER_PARITY_DIR=$HOME/.beatmo/render-parity`; dump `BEATMO_RENDER_PARITY_LABEL=<task>` then `BEATMO_RENDER_PARITY_A=<baseline> BEATMO_RENDER_PARITY_B=<task> go test -count=1 -v -run TestRenderParityCompare ./internal/audio/` must report every instrument and every knob/pitch case BIT-IDENTICAL (no `MIN_SNR`). Baseline until Task B2 = `phaseA-final`; after B2 = `phaseB-subbass`.
- Never read failure lists through `head`/`tail -n` — grep the full `^--- FAIL` set (Phase A lesson: a truncated list hid a failure).
- The user's real profile must never be touched: tests already isolate prefs; captures use `XDG_CONFIG_HOME=<scratch>`.

## Review Focus

1. **A project/bundle/prefs file without `schema_version` (or a foreign JSON) after B7** must be rejected BEFORE any state is mutated — the current circuit must survive a bad import untouched. Pinned by `TestImportRejectsForeignJSONWipe` (keep) + the flipped row in `TestImportProjectVersionRule` (B7).
2. **An instrument id with a single hyphen + number (`snare-1`)** after `legacy_instrument_ids.go` goes: must NOT be read as an instance of `snare` (the `instanceBaseID` single-hyphen arm is deleted in the same task). Pinned in B7 Step 3.
3. **A project binding a now-deleted recipe id** (e.g. `drum-snare`) must fail loudly, not silently play the base voice (`import.go` `BindInstrumentToRecipe` with an unknown id). Pinned in B7 Step 5.
4. **Browser total silence** if `audio.js` imports a symbol the regenerated ABI no longer exports — the ABI regeneration and the audio.js edit land in the SAME task (B6), verified by `synth_param_block_double` + `wasm_bridge_smoke`.
5. **C coverage floor** (`make coverage-c-check`, 99%) relies on tests B4/B8 delete — re-measure and either move live-C cases or re-ratchet with a dated note (B4 Step 4, B8 Step 6).

---

## File map (by task)

| Task | Deletes | Edits | Creates |
|---|---|---|---|
| B1 fixtures | `src/go/beatmo-export.json` | ~45 Go project fixtures (+`schema_version`), 92 test files re-pointed from legacy recipe ids, 8 browser tests (+`schema_version`), `cmd/export_audio/main.go` legacy cases → `synth-modular-*`, regenerate `internal/ui/testdata/import_embedded_pcm.json` | — |
| B2 sub-bass rows | `engine_instruments.go:23-36` rows, `synth_recipe.go:604-605` bindings, audio.js hand `RENDER`/`RENDER_INFO` sub-bass entries | `instrument_seeds.go` (+`subBassSeed`), `modular_instruments.go` (+2 rows), `render_parity_tool_test.go` (+`BEATMO_RENDER_PARITY_REKEYED`) | — |
| B3 analytic stage | — | `modular_family_stages.go` (source-4 stage rows), `ui/scene_catalog.go` (+2 scenes) | screenshots |
| B4 legacy tests | item-I test files (list in B4) | — | — |
| B5 legacy recipes + family layer | 24 legacy descriptors, `legacyCompatRecipes`, 18 `{kick,snare,tom,cymbal,fm,bass}_modular_{binding,push,native}.go`, `kick.go kick_deep.go snare.go rimshot.go sidestick.go clap.go`, `family_modular_{binding,push}.go`, `synth_recipe_wired.go`, `drum_gen_defaults.go`, `modular_stage_params.go:194-392` | `synth_recipe.go`, `synth_registry_entries_native.go`, `synth_recipe_wasm.go`, `modular_defaults_seed.go`, `cmd/export_audio/main.go`, 3 guard tests | — |
| B6 7-field path | `synth_params.go`, `SynthParamSchema*`, dead `variants.go` methods, audio.js 7-field arm + family scaffolding, worker bpe-4 branch | `cmd/gen-synth-abi`, regenerate `synth_param_abi.gen.js` | — |
| B7 import boundary | `legacy_instrument_ids.go`, `migrate_gen_type.go`, `LegacyVersion`×2, `schema.IsLegacy`, v1 node `LegacySynth*`, absent `master_volume`/`rules_enabled` inference, un-gzipped PCM read | `schema/schema.go`, `ui/project_file.go`, `ui/import.go`, `instrument_bundle.go`, `instance_instrument.go`, userprefs, `cmd/refs/instrument_name.go` | — |
| B8 C cleanup | `softsat`/`osc_wave` aliases, `sp_fundamental/sp_freq/sp_env_decay/sp_filter_cutoff`, tombstones, `pan.c/.h`, `adsr_reset/adsr_process`, `wt_osc_process*`, `dsp_units_c.go` + test | `synth_params.h` → internal `post_params`, Makefile `C_SRC/C_HDRS/C_OBJS/clean/lint-c` | — |
| B9 WASM exports + JS leftovers | unused `-sEXPORTED_FUNCTIONS` (`_result_description`, `_delay_*`, `_reverb_*`, 36 `_ifx_*_reset/_set_param`), runtime `cwrap`, `MODULAR_ALLOC_FLOOR`, gitignored `src/js/drums.js` + eslint entry | Makefile:34, `eslint.config.js`, `.gitignore` | — |
| B10 decisions | (pending user) non-modal hihat/ride voices; waveguide osc_type 8-11 | — | — |
| B11 docs + final | — | CLAUDE.md, AGENTS*.md, README.md, schema spec | — |

---

### Task B1: Modernize every fixture (no behavior change)

**Interfaces:** Produces: every test and fixture uses current ids, `synth-modular-*` recipes and `schema_version`, so later deletions break nothing they shouldn't. Consumes: nothing.

- [ ] **Step 1: `schema_version` on every project fixture.** For each Go test that builds a project with `"version":1` / a `Version int` struct and no `schema_version` (sites: `import_test:34,41`, `import_edge_cases_test` (13), `import_edges_missing_targets_test:22,32`, `import_more_validation_test:26-40`, `import_unknown_type_bpm_test:25,30`, `import_sequencer_hang_test` (7), `import_ui_flow_test:184,191`, `import_validation_test:34,51,124`, `parity_switch_instrument_replay_test:14`, `import_export_logic_test:95`, `import_readd_playback_test:852`, `connect_mode_test:235`, `demo_config_test:18,62,98`, `project_pins_test:105`, `export_synth_test:142,158-209`, `synth_render_roundtrip_test:268`, `import_button_flow_test:17`, `import_timeline_reset_test:26`, `uistate/apply_test:145`), add `"schema_version": "0.0.1"` (use `schema.Version` in Go literals). Same for browser fixtures `e2e_workflow:144,200`, `mobile_file_picker:97,225`, `device_user_actions:58`, `synth_generator_live_circuit:58`, `mobile_import_realdevice:67`, `mobile_import_gesture:74`. Find stragglers:
```bash
cd src/go && grep -rln '"version"' --include=*_test.go internal | xargs grep -L schema_version
```
- [ ] **Step 2: re-point legacy recipe ids in tests.** Mapping (recipe ids): `drum-snare→synth-modular-snare`, `drum-kick→synth-modular-kick-base`, `drum-hihat→synth-modular-hihat`, `drum-kick-punchy→synth-modular-kick-snap`, `drum-open-hihat→synth-modular-hihat-open`, `drum-cowbell→synth-modular-cowbell`, `fm-bass→synth-modular-fm-bass`, `fm-bell→synth-modular-fm-bell`, `fm-lead→synth-modular-fm-lead` (verify each target exists: `grep -n 'RecipeID: "synth-modular-<x>"' internal/audio/modular_instruments.go`). List the 92 files: `grep -rlE 'drum-(snare|kick|hihat|open-hihat|cowbell|kick-punchy|clap|ride|crash|tom)|"fm-(bass|bell|lead|epiano|pluck)"' --include=*_test.go internal cmd`. For each: replace the recipe id; where the test then asserts LEGACY param names (`snare_tone_freq`, `bass_*`, the 7 `pitch/decay/tone/…` knobs) or a knob count from `WiredParamsForRecipe` (`ui/synth_panel_drag_e2e_test:30,44,152,181`, `ui/synth_panel_tree_dispatch_test:34,44`), rewrite the expectation against the modular recipe's ParamDefs (`audio.RecipeRegistrations()[id].Params`). Tests whose ONLY purpose is legacy behavior are NOT re-pointed — they are deleted in B4/B5 (list there). `recipe_rebind_defaults_push_test:33,106` rebinds `hihat-open-bright`→`drum-open-hihat`: rebind to another shipped recipe (e.g. `synth-modular-hihat-lofi`) so the rebind check keeps its value. `engine_test:15-16`, `latency_test:19`: replace `Snare{}` with the `snare` voice (`audio.NewVoice("snare", …)` / existing helper).
- [ ] **Step 3: browser test re-points.** `wasm_bridge_smoke:434,444` → `synth-modular-kick-base` / `synth-modular-snare`; `instrument_params.browser.test.js:177-223,290-310` → a modular recipe (or delete those blocks: covered by Go `recipe_shipped_defaults_test`).
- [ ] **Step 4: `cmd/export_audio` cases.** Replace the ~44 legacy `drum-*`/`fm-*` cases (`main.go:237-312`) and the 3 `synth_params` cases (`:217-219`) with equivalent `synth-modular-*` cases (same knob intent, modular names). The browser replays these in JSON order on shared noise state: run `xplat_audio_compare` after (it renders the native reference fresh).
- [ ] **Step 5: regenerate `internal/ui/testdata/import_embedded_pcm.json`** with current ids/recipes (`kick-1`→`kick--1` instance form or a shipped id; `drum-kick-punchy/drum-clap/drum-cowbell/fm-epiano` → `synth-modular-*`), via the test's own generator if present, else export from a test Game. Update `import_embedded_pcm_test:27,66-73`, `import_sample_override_replace_test:26`, `import_embedded_pcm.browser.test.js:28-42` (drop the id-rename assertion). Delete `src/go/beatmo-export.json` (unreferenced).
- [ ] **Step 6: VERIFY** — fast suite, real suite and the touched browser tests green (only the pre-existing unrelated set may fail; list it in full); parity compare `phaseA-final` vs `B1` bit-identical (no production change).

---

### Task B2: `sub-bass` / `sub-bass-drive` become table rows (bit-exact)

**Interfaces:** Consumes B1. Produces: rows `{ID: "sub-bass", RecipeID: "synth-modular-sub-bass"}` and `{ID: "sub-bass-drive", RecipeID: "synth-modular-sub-bass-drive", DefaultFX: distortion}`; seed var `subBassSeed`; parity baseline `phaseB-subbass`.

- [ ] **Step 1: parity tool can re-key an instrument.** In `render_parity_tool_test.go` `compareParityCases`, read `BEATMO_RENDER_PARITY_REKEYED` (comma list of ids): for those ids skip case comparison and log "re-keyed (recipe changed): N cases in A, M in B". Base renders are still compared bit-exact.
- [ ] **Step 2: write the failing test** (`sub_bass_row_test.go`, `!test && !js`):
```go
// TestSubBassIsAConfigRow: sub-bass is an ordinary config-first table row — no
// legacy recipe, no hand-written CVariantInstrument — and renders exactly what
// the legacy drum-sub-bass binding rendered (golden in synth_golden_pinned_test).
func TestSubBassIsAConfigRow(t *testing.T) {
	for _, id := range []string{"sub-bass", "sub-bass-drive"} {
		found := false
		for _, d := range modularInstrumentDefs {
			if d.ID == id {
				found = true
				if d.RecipeID != "synth-modular-"+id {
					t.Errorf("%s: RecipeID %q", id, d.RecipeID)
				}
			}
		}
		if !found {
			t.Errorf("%s is not a modularInstrumentDefs row", id)
		}
		if r := RecipeForInstrument(id); r != "synth-modular-"+id {
			t.Errorf("%s bound to %q, want synth-modular-%s", id, r, id)
		}
	}
}
```
Run: FAIL (rows absent; bound to `drum-sub-bass`).
- [ ] **Step 3: generate the exact seed.** Scratch (delete after) `zz_subbass_seed_test.go`: `p, _ := ModularPushParams("drum-sub-bass", MergeRecipeDefaults("drum-sub-bass", RecipeParams{}))`, print every key sorted as `"key": value,` (`%v` of float64 prints the shortest exact repr). Expected keys (deletion map): `voice_freq_hz 45`, `gen1_source 4`, `gen1_freq_mode 0`, `gen1_freq 1`, `gen1_phase_mode 1`, `gen1_phase π/2` (print with `%.17g` — keep `math.Pi * 0.5` in source), `gen1_wave 0`, `gen1_harm_mix 0.08`, `gen1_pitch_env_amt 0.15`, `gen1_pitch_env_rate 40`, `gen1_atk_amt 0.2`, `gen1_atk_rate 60`, `gen1_env_fast_rate 2`, `gen1_sat_k 1.2`, `gen1_out_scale 0.95`, `osc/env/filter/drive_enabled 0`, `post_enabled 1`, `post_decay_rate 4`, POST on-gates per `subBassPostConfig` (pitch/decay/body/drive on, brightness/tone off), `post_decay 1`.
- [ ] **Step 4: add the seed + rows.** In `instrument_seeds.go`:
```go
// subBassSeed: the analytic dual-oscillator sub bass (gen source 4) as a plain
// config seed — formerly the legacy drum-sub-bass recipe + bass translation
// layer; values are exactly what that binding sent (bit-identical render).
var subBassSeed = RecipeParams{ /* the Step 3 keys, gen1_phase: math.Pi * 0.5 */ }
```
In `modular_instruments.go` (after the kick block): `{ID: "sub-bass", RecipeID: "synth-modular-sub-bass", Display: "Sub Bass", Category: modularRecipeCategory, Seed: subBassSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8}` and `{ID: "sub-bass-drive", RecipeID: "synth-modular-sub-bass-drive", Display: "Sub Bass Drive", Category: modularRecipeCategory, Seed: subBassSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8, DefaultFX: []EffectSlot{{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 2.0, "tone": 8000, "mix": 1}}}}`. Delete `engine_instruments.go:15-36` (the two CVariantInstrument rows + comment), `synth_recipe.go:604-605` bindings, audio.js hand `RENDER` `sub-bass`/`sub-bass-drive` (~1659,1661) and `RENDER_INFO` (~1726-1727). Run `make gen-modular-instruments`, `go generate ./internal/audio/` (id ORDER changes — sub-bass is no longer first), `make gen-fundamentals` (expect sub-bass entries unchanged at 45 Hz "declared"/"voice_freq_hz").
- [ ] **Step 5: GREEN + bit-exact.** `TestSubBassIsAConfigRow` PASS; `TestSynthGoldenByteIdentity` PASS **unchanged** for `sub-bass`/`sub-bass-drive` (if they move, the seed is wrong — diff with the Step 3 dump; do not re-pin); parity: `BEATMO_RENDER_PARITY_REKEYED=sub-bass,sub-bass-drive` compare `phaseA-final` vs `B2` → 150/150 base renders bit-identical. Then dump the new baseline label `phaseB-subbass` (with cases). `make wasm`; browser `synth_param_block_double`, `xplat_audio_compare`, `wasm_bridge_smoke` PASS. Loudness drift test: if the id order moved the median, regenerate (`make measure-loudness`) only if the drift test fails, and confirm no amp changed (median over a set is order-independent).

---

### Task B3: Synth-tab stage for the analytic voice (source 4) — UI

**Interfaces:** Consumes B2. Produces: the sub-bass knobs visible and editable (user requirement: every part of every instrument editable).

- [ ] **Step 1: failing test** in `ui/knob_interaction_functional_test.go`:
```go
// TestSubBassKnobsAreEditable: the analytic dual-osc voice (gen source 4) has a
// Synth-tab stage — every sound-shaping field the seed sets is a visible knob.
func TestSubBassKnobsAreEditable(t *testing.T) {
	reg := audio.RecipeRegistrations()["synth-modular-sub-bass"]
	if reg == nil {
		t.Fatal("recipe synth-modular-sub-bass not registered")
	}
	claimed := map[string]bool{}
	for _, g := range buildMacroGroupsAll(reg.Params, audio.RecipeDefaultParams("synth-modular-sub-bass")) {
		for _, p := range g.Params {
			claimed[p] = true
		}
	}
	for _, name := range []string{"gen1_harm_mix", "gen1_pitch_env_amt", "gen1_pitch_env_rate", "gen1_atk_amt", "gen1_atk_rate", "gen1_env_fast_rate", "gen1_sat_k", "gen1_out_scale", "gen1_wave", "voice_freq_hz"} {
		if !claimed[name] {
			t.Errorf("%s: no Synth-tab knob for the sub-bass voice", name)
		}
	}
}
```
Run: FAIL (fields are `SynthHiddenGroup`).
- [ ] **Step 2: implement** — in `audio/modular_family_stages.go` add a source-4 (analytic) family stage (same shape as the kick/snare entries: group, label, enum, macro category) for `harm_mix` ("Harmonics", tone), `pitch_env_amt` ("Pitch Drop", pitchdrop), `pitch_env_rate` ("Drop Speed", pitchdrop), `atk_amt` ("Punch", punch), `atk_rate` ("Punch Speed", punch), `env_fast_rate` ("Body Decay", length), `sat_k` ("Drive", grit), `out_scale` ("Level", boom); route visibility to slots whose `gen_source == 4` (mirror `synthFamilyParamHidden`). Bounds: confirm each field's ParamDef Min/Max contains the seed value.
- [ ] **Step 3: GREEN**; run `-tags test` ui suite subset `Synth|Macro|Knob`.
- [ ] **Step 4: screenshots (MANDATORY, CLAUDE.md).** Add scenes `crop_synth_sub_bass_root` (`macroSceneSetup("sub-bass", "")`) and `crop_synth_sub_bass_punch_more` (`synthMoreScrolledSceneSetup("sub-bass", "punch")`); build `CGO_ENABLED=1 .tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd` and capture with `XDG_CONFIG_HOME=<scratch>`; open the PNGs; labels legible, values = seed.

---

### Task B4: Delete tests that only pin legacy behavior

- [ ] **Step 1: measure C coverage first:** `make coverage-c-check` → record %.
- [ ] **Step 2: delete** (all `!test && !js` unless noted): `legacy_oracle_golden_test.go` + `testdata/legacy_oracle_golden.json`, `family_push_binding_parity_test.go`, `native_knob_effect_test.go`, `native_wave_knob_test.go`, `native_golden_test.go`, `legacy_render_native_test.go`, `legacy_render_determinism_test.go`, `config_first_seed_equivalence_test.go`, `{bass,cymbal,fm,kick,snare,tom}_migration_test.go`, `family_migration_helpers_test.go`, `{bass,cymbal,fm,kick,snare,tom}_native_fastpath_test.go`, `snare_postorder_test.go`; untagged: `synth_recipe_wired_test.go`, `drum_gen_defaults_test.go`, `stage_params_test.go`, `recipe_param_effect_test.go`, `synth_registry_entries_test.go`, `fm_golden_test.go`. Partial: `synth_recipe_phase2_native_test.go:14-75` (keep `:76`, `:99`), `voice_cache_bench_test.go:35-38`, `variants_native_test.go:84-138`, `variants_no_topup_test.go:36-56`, `instrument_seeds_test.go:368,373`. Before deleting `modular_noedit_fastpath_test.go` / `sax_native_fastpath_test.go`, read them: if they assert shipped-id behavior, keep them.
- [ ] **Step 3: VERIFY** — real + fast suites green (full FAIL list); parity unchanged (no production change).
- [ ] **Step 4: C coverage** — `make coverage-c-check`; if below the floor, move the live-C cases the deleted tests covered (`load_audio` error paths, FM long buffer) into a shipped-id test, else re-ratchet the floor with a dated Makefile comment naming the deleted legacy-only lines.

---

### Task B5: Delete the legacy recipes and the family translation layer (one change)

**Ordering constraint (deletion map):** the `*_native.go` package-var inits call `MergeRecipeDefaults("drum-*")` and `BuildBuiltinRecipeDocs` reads `WiredParamsForRecipe` → all of this goes in ONE change.

- [ ] **Step 1: guards first** — edit `synth_recipe_completeness_test.go:26`, `synth_recipe_discipline_test.go:118`, `recipe_overrides_defaults_push_test.go:88` to drop the `IsLegacyCompatRecipe` clause; add to `synth_recipe_discipline_test.go`:
```go
// TestNoLegacyRecipes: every registered recipe is the modular base or a table row.
func TestNoLegacyRecipes(t *testing.T) {
	for id := range RecipeRegistrations() {
		if id != "synth-modular" && !strings.HasPrefix(id, "synth-modular-") && !strings.HasPrefix(id, "user.") {
			t.Errorf("legacy recipe %q still registered", id)
		}
	}
}
```
Run: FAIL (24 legacy ids).
- [ ] **Step 2: delete** `synth_recipe.go` legacy descriptors `:504-529`, `legacyCompatRecipes`/`IsLegacyCompatRecipe` `:548-589`, comments `:498-502,593-602`, the non-modular `else` arm of `BuildBuiltinRecipeDocs` (`:701-707`); files `{kick,snare,tom,cymbal,fm,bass}_modular_{binding,push,native}.go`, `kick.go`, `kick_deep.go`, `snare.go`, `rimshot.go`, `sidestick.go`, `clap.go`, `family_modular_binding.go`, `family_modular_push.go`, `synth_recipe_wired.go`, `drum_gen_defaults.go`; `modular_stage_params.go:194-392` (keep `:1-192`; simplify `isAppendedStageName`); in `synth_registry_entries_native.go` delete `nativeDrumRecipe` `:11-34`, `cRecipeRenderer`/`builtinFamilyRenderers` `:39-68`, the non-modular init arm `:179-188`. Fix callers: `synth_recipe_wasm.go:33,87` (no translation — push the overlay as-is), `modular_defaults_seed.go:76` (drop the `!IsModularMigratedRecipe` branch), `cmd/export_audio/main.go:377` (B1 already moved its cases; delete the `ModularPushParams` use). `go build ./... && go vet ./...` (native + `-tags test` + `GOOS=js GOARCH=wasm`).
- [ ] **Step 3: VERIFY** — `TestNoLegacyRecipes` PASS; real + fast suites (full FAIL list); parity `phaseB-subbass` vs `B5` bit-identical (base + 50,976 cases); `make wasm`; browser `synth_param_block_double`, `xplat_audio_compare`, `synth_save_export_import`, `wasm_bridge_smoke`, `import_synth_render` PASS.

---

### Task B6: Delete the 7-field `synth_params` ABI path (Go + JS + ABI together)

- [ ] **Step 1: failing guard** in `modular_abi_drift_test.go`:
```go
// TestOnlyTheModularParamBlockCrossesTheABI: the generated JS ABI carries the
// modular block only — no 7-field synth_params block.
func TestOnlyTheModularParamBlockCrossesTheABI(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootForC(t), "src/js/synth_param_abi.gen.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SYNTH_PARAM_") {
		t.Fatal("synth_param_abi.gen.js still exports the 7-field SYNTH_PARAM_* block")
	}
}
```
Run: FAIL.
- [ ] **Step 2: Go** — delete `synth_params.go`; `synth_registry_entries_native.go` `recipeParamsToSynth` `:87-109`, `builtinRecipeRenderers` `:111-138`, dead fallback `:33`; `variants.go` `RenderParam` `:21`, `NewVoiceWithParams` `:84-102`, `renderParamAndCache` `:104-125`; `synth_param_schema.go:30-104` (`SynthParamSchema*`); `cmd/gen-synth-abi/main.go:29-64,123-152`; tests `synth_params_test.go`, `synth_params_nan_test.go`, `synth_param_schema_test.go:77-~100`, `modular_abi_drift_test.go` `TestSynthParamsSchemaMatchesCStruct` + the embedded-struct branch, `config_p3_test.go:31-35`; re-point `voice_cache_test.go:356-357` to `RecipeParams`. Rewrite stale comments `modular_c.go:20,618`, `synth_recipe_plugin.go:101`, `synth_recipe.go:85`.
- [ ] **Step 3: JS (same task)** — `audio.js`: imports `:2-4`, `synthCount` `:128`, `FAMILY_ABI_OK` term `:129`, the render arm `:4155-4204` (`familyBlock`, `SYNTH_PARAM_*`, `Float32Array`, `'_p'`) → the modular block is the only block (`Float64Array`, `render_modular_p`); `renderVoiceSync` HEAPF32 param branch `:1605-1613`; `synth_render_worker.js:66-75` bpe-4 branch (keep the output read); `FAMILY_PARAM_BLOCKS` `:38-46`, `FAMILY_REQUIRED_FIELDS`/`validateFamilyABI`/`FAMILY_ABI_OK` `:91-122`. `xplat_audio_compare.browser.test.js`: remove `:345-347,362-369,379,397-411` and the hardcoded floor `:403`. Regenerate `synth_param_abi.gen.js`.
- [ ] **Step 4: VERIFY** — guard PASS; suites; parity bit-identical; `make wasm`; browser `synth_param_block_double`, `xplat_audio_compare`, `wasm_bridge_smoke`, `synth_abi_consistency`, `instrument_params` PASS (a module-load failure = total silence — check the page console in `synth_param_block_double`).

---

### Task B7: Old-project read boundary — strict `schema_version`, no migrations

- [ ] **Step 1: failing tests** — flip `schema_test.go:12-44` "absent" row to `wantErr`; flip `project_schema_version_test.go` `TestImportProjectVersionRule` row `:38-40` (absent version + `"version":1`) to REJECT; add to `ui/project_schema_version_test.go`:
```go
// TestImportRejectsSchemaLessProjectBeforeMutating: a file without
// schema_version is rejected and the current circuit is left untouched.
func TestImportRejectsSchemaLessProjectBeforeMutating(t *testing.T) {
	g := newTestGame(t)
	before := g.ExportJSONForTest()
	err := g.ImportJSONForTest([]byte(`{"version":1,"bpm":90,"instruments":[],"nodes":[]}`))
	if err == nil {
		t.Fatal("schema-less project accepted")
	}
	if after := g.ExportJSONForTest(); after != before {
		t.Fatal("rejected import mutated the circuit")
	}
}
```
(use the existing test helpers for game construction / export / import — `grep -n "func newTestGame\|ForTest(" internal/ui/testutil_test.go`.) Run: FAIL.
- [ ] **Step 2: schema** — `schema/schema.go`: delete `IsLegacy` `:62-67`; `Validate` `:70-82` rejects empty; rewrite docs `:7-19,41-53`. `ui/project_file.go`: delete `LegacyVersion` `:21-40`, collapse `validateProjectVersion` `:101-113` to "present and == schema.Version, else reject" (called at `import.go:134` BEFORE any reset — keep that position). `instrument_bundle.go`: delete `LegacyVersion` `:99-105`, simplify `ValidateBundleHeader` `:127-133` (keep the `Format == "beatmo.instrument"` check). userprefs `store.go:351-375`, `instrument_store.go:108-120`: drop the `IsLegacy` logs (schema-less prefs now load empty, WARN). Tests: delete `instrument_bundle_test.go:57-61`; update `schema_version_test.go:109`.
- [ ] **Step 3: retired ids** — delete `legacy_instrument_ids.go` and its callers' resolution (`ui/import.go:148-156,196-202,563-568`, `instance_instrument.go:81-90`, `synth_recipe.go:426-435,643-650`, `cmd/refs/instrument_name.go:46-56`) AND the single-hyphen arm of `instanceBaseID` (`instance_instrument.go:30-37`) in the same edit. Failing-first guard in `instance_instrument_test.go`:
```go
// TestSingleHyphenNumberIsNotAnInstance: "snare-1" is just an (unknown) id —
// only "--N" marks an instance since the retired-id table is gone.
func TestSingleHyphenNumberIsNotAnInstance(t *testing.T) {
	if base := instanceBaseID("snare-1"); base == "snare" {
		t.Fatalf("instanceBaseID(snare-1) = %q, want it NOT read as an instance of snare", base)
	}
	if base := instanceBaseID("snare--1"); base != "snare" {
		t.Fatalf("instanceBaseID(snare--1) = %q, want snare", base)
	}
}
```
Move `instance_instrument_test.go:25-192`, `factory_recipe_export_test.go:25`, `recipe_base_id_fallback_test.go:10` fixtures to `--N`; delete `legacy_instrument_ids_test.go`, `instance_instrument_test.go:191,~215-240`, `ui/legacy_instrument_import_test.go`, `cmd/refs/instrument_name_test.go:27`, `ui/repro_import_beatmojson_test.go`.
- [ ] **Step 4: gen_type migration** — delete `migrate_gen_type.go`, callers `ui/import.go:735-740`, `recipe_overrides.go:108-116`, JS `audio.js:3276-3279`; tests `migrate_gen_type_test.go`, `ui/import_gen_type_migration_test.go`, `import_synth_render.browser.test.js` case 3 (`~173-205`); comments `synth_recipe_wasm.go:59`, `synth_recipe_binding_hook.go:9`, `scene_catalog.go:2827`.
- [ ] **Step 5: unknown recipe id fails loudly** — failing test in `ui/import_validation_test.go`: importing a project whose instrument has `"recipe": "drum-snare"` returns an error naming the id (today `import.go:741` silently binds and plays the base voice). Implement: in the import path, `if _, ok := audio.RecipeRegistrations()[recipe]; !ok && !isUserRecipe(recipe) { return fmt.Errorf("instrument %q: unknown recipe %q", …) }` before mutation (`user.*` recipes come from `projectFile.Recipes`, registered first).
- [ ] **Step 6: other legacy reads** — delete v1 node `LegacySynth*` (`ui/export.go:169-212`, `import.go:432-436`), absent-`master_volume`→1.0 (`import.go:787-805`; absent now means the file is invalid only if schema says so — keep the field required), `rules_enabled` inference (`import.go:530-541`), un-gzipped PCM read (`export.go:85-112`); delete their tests (`export_synth_test:154`, `export_import_audio_test:747`, `group_import_export_test:176`, `sampler_export_test:137`).
- [ ] **Step 7: VERIFY** — all new guards GREEN; `TestImportRejectsForeignJSONWipe` unchanged GREEN; suites; parity bit-identical; `make test-real` export→import identity suite GREEN; browser import tests (`e2e_workflow`, `import_embedded_pcm`, `import_synth_render`, `synth_save_export_import`).

---

### Task B8: C cleanup + test-only C

- [ ] **Step 1:** `synth_params.h` → internal POST type: rename struct `synth_params` → `post_params` (internal, no ABI), drop the unused `fundamental` field, delete `sp_fundamental` `:35`, `sp_freq` `:38-43`, `sp_env_decay` `:45-49`, `sp_filter_cutoff` `:52-55`; keep `kp_get`. Update `synth_post.h:28-29`, `drums.c:182-281`, `modular.c:1361-1430`. Drop the `synth_params.h` include from `drums.h:4`, `fmsynth.h:4`, and `synth_post.h` from `fmsynth.c:4` (re-add only where the compiler asks).
- [ ] **Step 2:** delete aliases `drums.c:25` (`softsat`), `:55-57` (`osc_wave`); tombstone comments `drums.c:86-121,157-174,283-320`, `fmsynth.c:189-205`, `fmsynth.h:38-44`, `synth_params.h:88-117`, `drums.h:6-36`, `modular_stages.c:2805`.
- [ ] **Step 3:** test-only C — delete `dsp_units_c.go` + `dsp_units_native_test.go` (first move `TestSendDelayInitDampingBranches` if `send_delay_damping_native_test` does not cover it), `pan.c`/`pan.h` (Makefile `C_SRC:5`, `C_HDRS:11`, `C_OBJS:48`, clean `:72`, `lint-c:528`), `adsr.c:56-66` + `adsr.h:46-50` (`adsr_reset`, `adsr_process`), `wavetable.c:126-168` + `wavetable.h:68-74` (`wt_osc_process*`).
- [ ] **Step 4:** `make build/libdrums.a` warning-free; `TestModularParamsAreNeverNarrowed` still 0.
- [ ] **Step 5: VERIFY** — suites; parity bit-identical; `make wasm`; browser smoke + xplat.
- [ ] **Step 6:** `make coverage-c-check` — same handling as B4 Step 4.

---

### Task B9: WASM export trim + JS leftovers

- [ ] **Step 1:** Makefile:34 `-sEXPORTED_FUNCTIONS`: remove `_result_description`, `_delay_init/_process/_reset`, `_reverb_buffer_size/_init/_process/_reset`, the 36 `_ifx_*_reset`/`_ifx_*_set_param`; keep `_render_modular`, `_render_modular_p`, `_ifx_*_init`, `_ifx_*_process`, `_ifx_reverb_mem_size` (used by `xplat_insert_fx_parity`), `_malloc`, `_free`; runtime: drop `cwrap`, keep `ccall`, `HEAPF32`, `HEAPF64`. (C stays: native cgo uses `result_description` / delay / reverb.)
- [ ] **Step 2:** audio.js `MODULAR_ALLOC_FLOOR` (`:65`, use at `~4188`) → `MODULAR_PARAM_COUNT`; stale comments `:1424-1428,3052,4150-4158`; delete gitignored `src/js/drums.js`, its `eslint.config.js:8` entry and `.gitignore:5` line. Keep the hand `RENDER`/`RENDER_INFO` entry for `modular` (the base voice; not legacy) and delete every other hand entry now duplicated by the generated table.
- [ ] **Step 3: VERIFY** — `make wasm`; full browser set used above + `xplat_insert_fx_parity`, `webaudio_drums_consistency`, `modular_synth_audible`; `TestWasmAssetSizeBudget` (expect smaller).

---

### Task B10: User decisions (do NOT execute without them)

1. **Non-modal hi-hat/ride voices** (`gen_cymbal_hihat` `modular_stages.c:2099-2161`, `gen_cymbal_ride` `:2363-2438`): no shipped seed reaches them, but a user edit can (Variant Hi-Hat/Ride with Plate Modes 0). Options: (a) delete them and make variants 0/4 always run the modal plate (Plate Modes min 1 for those variants; edits that set 0 clamp to 1); (b) keep them as a selectable "classic" voice. Recommendation: (a).
2. **Waveguide oscillators `osc_type` 8-11** (brass/reed/flute/sax, ~240 LOC + 6 `osc_sax_*` params): no seed uses them, but they are user-selectable in the Synth tab (enum labels cannot be removed without shifting "Voice" = 12). Recommendation: keep — they are a feature of the editable pipeline, not legacy.

### Task B11: Docs + final verification

- [ ] **Step 1:** CLAUDE.md — the "SynthRecipe is the canonical instrument path" gotcha, Schema versioning (three-way rule → strict), "The browser's synth mirror…" (stale `drum-open-hihat` line), "The native audio reference…" (`TestFamilyPushMatchesBindingRender`), `pan.c`; AGENTS.synth.md `:98,190-191,235-245,405,412`; AGENTS.synth.instruments.md `:12-14,272,342,376,676,703,1477-1486,1520-1540,1563`; AGENTS.md `:83`; README.md `:49`; `docs/superpowers/specs/2026-07-27-schema-versioning-design.md`; Makefile comment `:1000-1007`.
- [ ] **Step 2:** full verification — real + fast suites (full FAIL list, each remaining failure attributed), `make test-real`, browser set, parity `phaseB-subbass` vs final bit-identical, `make coverage-c-check`; dump baseline `phaseB-final` for Phase C.
- [ ] **Step 3:** final whole-branch review (fresh reviewer, most capable model) of Phase B; fix Critical/Important with RED→GREEN; ledger minors.

## Results

(filled in as tasks land)
