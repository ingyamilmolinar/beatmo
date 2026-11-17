# Instrument Seed Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add 20 synthesized melodic instruments (guitars, bowed strings, piano, winds) as parameter "seeds" of the existing modular synth voice, surfaced in new Strings/Winds/Keys picker categories and playable on desktop + browser.

**Architecture:** Each instrument is a `RecipeParams` map (a seed) bound to a seed-bearing modular recipe, registered like the existing `synth-modular-pad`. No new C/DSP — they render through `render_modular_p`. Bowed strings + winds use LFO→pitch vibrato (`lfo_target=1`, from the LFO-routing plan, already implemented). Per-instrument wiring is ~6 mechanical one-liners.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), the modular C voice (already built), `audio.js` bridge.

**Depends on:** `docs/superpowers/plans/2026-06-18-engine-lfo-routing.md` (DONE — `lfo_target` 0/1/2 implemented, byte-identity verified, WASM parity green).

## Global Constraints

- **Seeds are TUNING STARTING POINTS.** The param values below are research-grounded first drafts; the user auditions and we refine by ear. A task is "done" when the instrument is wired, registered, categorized, render-smoke-passing, and audibly distinct — NOT when it sounds perfect.
- **Render-smoke bar (every instrument):** rendered output must be finite (no NaN/Inf), non-silent (peak > 0.01), and distinct from the bare modular default (the seed actually changes the sound).
- **No byte-identity risk:** these are NEW instruments/recipes; they add entries, they do not modify existing instruments. Existing instruments must stay untouched (the recipe-registry discipline tests enforce no orphans/dupes).
- **Vibrato lives on the wavetable OSC path only** (`osc_type` 0–3). Bowed strings + winds are authored as subtractive saw/sine + filter (NOT FM) so vibrato works. Guitars use the Karplus-Strong gen-bank slot (no vibrato needed). Piano uses additive sine gen-bank slots (no vibrato).
- **`lfo_depth` semantics by target:** target 1 (vibrato) = ± semitones (0.18 → ±18 cents); target 2 (cutoff) = ± octaves; target 0 (amp) = 0..1 dip. Seeds set `lfo_target` explicitly.
- **Bundled Go only:** `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`. Engine/recipe tests run on the REAL CGo build (`cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run … -v`, NOT `-tags test`). After any seed/binding change that affects browser playback, `make wasm` regenerates `synth_param_abi.gen.js` (no new export needed).
- **Do NOT `git add` / `git commit`** — leave all work uncommitted for the user (standing project rule). "Checkpoint" = run tests + pause for review.

---

## The seed vocabulary (verified against the modular schema)

Seeds are `map[string]float64`. Available keys:

**Global stages:** `osc_type` (0 sine,1 saw,2 square,3 triangle,5 white-noise,6 pink), `osc_detune` (±100 cents), `osc_octave`, `osc_enabled` (0/1), `amp_attack/amp_decay/amp_sustain/amp_release` (s / 0..1), `amp_curve`, `filter_type` (0 LP,1 HP,2 BP), `filter_cutoff` (20–20000 Hz), `filter_resonance` (0.5–16 Q), `drive` (0..1), `gain` (0–1.5), `lfo_enabled` (0/1), `lfo_rate` (Hz), `lfo_depth`, `lfo_target` (0/1/2), `pitchenv_enabled/pitchenv_amt/pitchenv_decay`.

**Gen-bank per-slot keys** (`gen<k>_<field>`, k = 1..12): `source` (1 wavetable-osc, 2 noise-tap, 3 Karplus-Strong, …), `wave` (0 sine,1 saw,2 sq,3 tri), `freq_mode` (0 ratio×voicefreq, 1 absolute Hz), `freq`, `gain`, `env_fast_rate`, `env_tail_rate`, `env_fast_mix`, `env_tail_mix`, `filt_type` (0 none,1 1pole-LP,2 1pole-HP,3 RBJ-LP,4 RBJ-HP,5 RBJ-BP), `filt_alpha`, `filt_freq`, `filt_q`, `noise_offset`, and (KS) `ks_sustain`, `ks_pluck`.

When a gen-bank slot is the sound source (KS guitars, additive piano), set `osc_enabled: 0` so the legacy OSC stage is silent and the slots carry the voice.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `src/go/internal/audio/instrument_seeds.go` (NEW) | The 20 `RecipeParams` seed vars + their recipe-descriptor IDs | Create |
| `src/go/internal/audio/synth_recipe.go` | recipe descriptors + instrument→recipe bindings | Add 20 `builtinRecipeDescriptor{…, Seed:}` + 20 bindings |
| `src/go/internal/audio/engine_instruments.go` | builtin instrument map | Add 20 `CVariantInstrument{Render: renderModular, Beats:…}` |
| `src/go/internal/audio/config.go` | browser instrument configs | Add 20 `InstrumentConfigs` entries |
| `src/go/internal/audio/catalog_desktop.go` | picker categories | Extend `synthCategory()` → Strings/Winds/Keys |
| `src/js/audio.js` | browser render dispatch | Add 20 `RENDER` + 20 `RENDER_INFO` entries (`render_modular`, `paramBlock:'modular'`) |
| `src/go/internal/audio/instrument_seeds_test.go` (NEW) | render-smoke tests | Create |

The seed vars + descriptor IDs live together in `instrument_seeds.go` (one responsibility: the seed data). Wiring touches the existing registry files.

---

## Per-instrument wiring recipe (identical for every instrument — apply per row)

For instrument id `X`, recipe id `synth-modular-X`, category `CAT`, seed var `XSeed`:
1. `instrument_seeds.go`: `var XSeed = RecipeParams{…}` (the seed) + reference it in the descriptor list.
2. `synth_recipe.go` `builtinRecipeDescriptors`: `{ID: "synth-modular-X", Display: "<Name>", Category: modularRecipeCategory, Seed: XSeed}`.
3. `synth_recipe.go` `builtinInstrumentRecipeBindings`: `"X": "synth-modular-X"`.
4. `engine_instruments.go` map: `"X": CVariantInstrument{Name: "X", Render: renderModular, Beats: <dur>}`.
5. `config.go` `InstrumentConfigs`: `"X": {ID: "X", DurationSec: <dur>, Amplitude: 0.8, RenderFunc: "render_modular"}`.
6. `audio.js`: `RENDER['X']='render_modular'`; `RENDER_INFO['X']={seconds:<dur>, amp:0.8, paramBlock:'modular'}`.
7. `catalog_desktop.go` `synthCategory()`: ensure id prefix → `CAT` (once per category, not per instrument).

`instrument_ids_gen.go` regenerates on build.

---

### Task 1: Category scaffolding + Bowed Strings (violin, cello — the vibrato showcase)

Validates the entire seed→recipe→category→render→browser pipeline on the family that best demonstrates the new vibrato, before mass-producing the rest.

**Files:** create `instrument_seeds.go`, `instrument_seeds_test.go`; modify `synth_recipe.go`, `engine_instruments.go`, `config.go`, `catalog_desktop.go`, `src/js/audio.js`.

**Instruments (Beats: 2.0 — sustained):** `violin`, `violin-ensemble`, `cello`, `cello-warm`. Category **"Strings (Synth)"**.

Seeds (starting points — subtractive saw + LP + vibrato + bow-noise tap):
```
violin:          {osc_type:1, osc_enabled:1, filter_type:0, filter_cutoff:3400, filter_resonance:1.1,
                  amp_attack:0.07, amp_decay:0.09, amp_sustain:0.88, amp_release:0.22,
                  lfo_enabled:1, lfo_rate:6.0, lfo_depth:0.18, lfo_target:1,
                  gen1_source:2, gen1_gain:0.10, gen1_filt_type:5, gen1_filt_freq:4000, gen1_filt_q:0.8, gen1_env_fast_rate:8, gain:0.9}
violin-ensemble: {osc_type:1, osc_detune:6, filter_cutoff:3000, filter_resonance:1.0,
                  amp_attack:0.10, amp_sustain:0.85, amp_release:0.30,
                  lfo_enabled:1, lfo_rate:5.5, lfo_depth:0.12, lfo_target:1,
                  gen1_source:2, gen1_gain:0.08, gen1_filt_type:5, gen1_filt_freq:3500, gen1_filt_q:0.8, gen1_env_fast_rate:6, gain:0.9}
cello:           {osc_type:1, filter_cutoff:2000, filter_resonance:1.0,
                  amp_attack:0.12, amp_decay:0.16, amp_sustain:0.90, amp_release:0.38,
                  lfo_enabled:1, lfo_rate:4.8, lfo_depth:0.13, lfo_target:1,
                  gen1_source:2, gen1_gain:0.10, gen1_filt_type:5, gen1_filt_freq:2800, gen1_filt_q:0.8, gen1_env_fast_rate:6, gain:0.95}
cello-warm:      {osc_type:1, filter_cutoff:1500, filter_resonance:1.2,
                  amp_attack:0.16, amp_sustain:0.92, amp_release:0.5,
                  lfo_enabled:1, lfo_rate:4.6, lfo_depth:0.11, lfo_target:1,
                  gen1_source:2, gen1_gain:0.08, gen1_filt_type:5, gen1_filt_freq:2200, gen1_filt_q:0.8, gen1_env_fast_rate:5, gain:0.95}
```

- [ ] **Step 1: Write the failing render-smoke test** (`instrument_seeds_test.go`)

```go
package audio

import "testing"

// renderSeedInstrument renders one builtin instrument id through its bound recipe
// and returns the buffer. Helper for seed render-smoke tests.
func renderSeedInstrument(t *testing.T, id string, n int) []float32 {
	t.Helper()
	rec := RecipeForInstrument(id)
	if rec == "" {
		t.Fatalf("instrument %q has no recipe binding", id)
	}
	p := RecipeDefaultParams(rec) // includes the seed (baked into recipe defaults)
	buf := make([]float32, n)
	renderModularP(buf, 48000, n, recipeParamsToModular(p))
	return buf
}

func assertAudible(t *testing.T, id string, buf []float32) {
	t.Helper()
	var peak float32
	for _, v := range buf {
		if v != v || v > 8 || v < -8 {
			t.Fatalf("%s: non-finite/exploded sample %v", id, v)
		}
		a := v
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}
	if peak < 0.01 {
		t.Fatalf("%s: silent (peak %v)", id, peak)
	}
}

func TestSeed_BowedStrings_Audible(t *testing.T) {
	for _, id := range []string{"violin", "violin-ensemble", "cello", "cello-warm"} {
		buf := renderSeedInstrument(t, id, 48000)
		assertAudible(t, id, buf)
	}
}

func TestSeed_BowedStrings_DistinctFromDefault(t *testing.T) {
	def := make([]float32, 48000)
	renderModularP(def, 48000, 48000, defaultModularParams())
	for _, id := range []string{"violin", "cello"} {
		buf := renderSeedInstrument(t, id, 48000)
		diff := 0
		for i := range buf {
			if buf[i] != def[i] {
				diff++
			}
		}
		if diff == 0 {
			t.Fatalf("%s identical to bare modular default — seed not applied", id)
		}
	}
}

func TestSeed_StringsCategory(t *testing.T) {
	for _, id := range []string{"violin", "cello"} {
		if got := synthCategory(id); got != "Strings (Synth)" {
			t.Fatalf("synthCategory(%q)=%q, want Strings (Synth)", id, got)
		}
	}
}
```

- [ ] **Step 2: Run — verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'TestSeed_BowedStrings|TestSeed_StringsCategory' -v`
Expected: FAIL — `RecipeForInstrument("violin")` empty / `synthCategory` wrong (instruments not registered yet).

- [ ] **Step 3: Create `instrument_seeds.go`** with the four seed vars above (`violinSeed`, `violinEnsembleSeed`, `celloSeed`, `celloWarmSeed`) as `RecipeParams{…}` literals (transcribe the maps verbatim).

- [ ] **Step 4: Register descriptors + bindings** in `synth_recipe.go`: append to `builtinRecipeDescriptors` the four `{ID:"synth-modular-violin", Display:"Violin", Category: modularRecipeCategory, Seed: violinSeed}` (etc.), and to `builtinInstrumentRecipeBindings` the four `"violin":"synth-modular-violin"` (etc.). Match the exact `modularRecipeCategory` constant + descriptor struct fields already used by `synth-modular-pad`.

- [ ] **Step 5: Add instrument entries** in `engine_instruments.go` (`"violin": CVariantInstrument{Name:"violin", Render: renderModular, Beats: 2.0}`, etc. — match the existing `modular-pad` entry's shape), `config.go` (`InstrumentConfigs`), and `audio.js` (`RENDER` + `RENDER_INFO`).

- [ ] **Step 6: Extend `synthCategory()`** in `catalog_desktop.go`: add a case mapping `violin`/`cello` (and `viola` for future) prefixes → `"Strings (Synth)"`, placed so it doesn't collide with existing cases.

- [ ] **Step 7: Run — verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'TestSeed_BowedStrings|TestSeed_StringsCategory' -v`
Expected: PASS.

- [ ] **Step 8: Recipe-registry discipline + drift guards**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'Recipe|Synth|Catalog|Instrument' -v`
Expected: PASS (no orphan recipes, every binding resolves, NaN/Inf boundary sweep clean, instrument-id gen current).

- [ ] **Step 9: Browser playability**

Run: `make wasm` then `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: build OK; bridge smoke PASS (the four ids resolve through the modular block).

- [ ] **Step 10: Checkpoint (do NOT commit).** Pause for the user to audition violin/cello (load via a scratch circuit or the picker) and tune the seeds together. Record tuning learnings in memory.

---

### Task 2: Plucked Strings (guitars — Karplus-Strong)

**Instruments (Beats: 1.5):** `guitar-nylon`, `guitar-nylon-bright`, `guitar-steel`, `guitar-steel-warm`, `guitar-electric`, `guitar-electric-neck`. Category **"Strings (Synth)"** (extend the existing case with `guitar` prefix).

Seeds (KS gen-bank slot; `osc_enabled:0` so the slot is the source; global LP filter for body):
```
guitar-nylon:         {osc_enabled:0, gen1_source:3, gen1_gain:1.0, gen1_ks_sustain:0.955, gen1_ks_pluck:0.18,
                       filter_type:0, filter_cutoff:3500, amp_attack:0.002, amp_decay:1.2, amp_sustain:0.0, amp_release:0.2, gain:0.95}
guitar-nylon-bright:  {osc_enabled:0, gen1_source:3, gen1_gain:1.0, gen1_ks_sustain:0.965, gen1_ks_pluck:0.27,
                       filter_cutoff:5000, amp_attack:0.002, amp_decay:1.4, amp_sustain:0.0, amp_release:0.2, gain:0.95}
guitar-steel:         {osc_enabled:0, gen1_source:3, gen1_gain:1.0, gen1_ks_sustain:0.985, gen1_ks_pluck:0.6,
                       filter_cutoff:8000, amp_attack:0.001, amp_decay:2.5, amp_sustain:0.0, amp_release:0.3, gain:0.9}
guitar-steel-warm:    {osc_enabled:0, gen1_source:3, gen1_gain:1.0, gen1_ks_sustain:0.978, gen1_ks_pluck:0.45,
                       filter_cutoff:6000, amp_attack:0.001, amp_decay:2.0, amp_sustain:0.0, amp_release:0.3, gain:0.9}
guitar-electric:      {osc_enabled:0, gen1_source:3, gen1_gain:1.0, gen1_ks_sustain:0.99, gen1_ks_pluck:0.5,
                       filter_cutoff:5000, filter_resonance:1.2, amp_attack:0.001, amp_decay:4.0, amp_sustain:0.0, amp_release:0.3, gain:0.9}
guitar-electric-neck: {osc_enabled:0, gen1_source:3, gen1_gain:1.0, gen1_ks_sustain:0.988, gen1_ks_pluck:0.4,
                       filter_cutoff:3800, amp_attack:0.001, amp_decay:4.0, amp_sustain:0.0, amp_release:0.3, gain:0.9}
```

- [ ] **Step 1: Failing test** — append `TestSeed_Guitars_Audible` (the six ids through `renderSeedInstrument`+`assertAudible`) and `TestSeed_Guitars_NylonDarkerThanSteel` (assert steel has more high-frequency energy than nylon — e.g. compare summed abs first-difference `Σ|buf[i]-buf[i-1]|`, steel > nylon, since brighter = faster sample-to-sample change).
- [ ] **Step 2: Run — verify fail** (`cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestSeed_Guitars -v`; FAIL — no bindings).
- [ ] **Step 3: Add seeds** to `instrument_seeds.go` (six vars).
- [ ] **Step 4: Wire all six** per the per-instrument recipe (descriptors+bindings, engine_instruments Beats:1.5, config.go, audio.js); extend `synthCategory()` `guitar` prefix → "Strings (Synth)".
- [ ] **Step 5: Run — verify pass** (same `-run TestSeed_Guitars -v`).
- [ ] **Step 6: Discipline guards** (`-run 'Recipe|Synth|Catalog|Instrument'`).
- [ ] **Step 7: Checkpoint (no commit)** — audition + tune; record learnings.

---

### Task 3: Keys (acoustic piano — additive)

**Instruments (Beats: 2.0):** `piano-grand`, `piano-felt`. Category **"Keys (Synth)"**.

Seeds (additive sine partials via gen slots 1–4 at ratios 1/2/3/4 with faster-decaying highs; hammer noise tap slot 5; ±1.5¢ unison via `osc_detune` on a doubled OSC; `osc_enabled:1` carrying the fundamental, slots adding partials):
```
piano-grand: {osc_type:0, osc_enabled:1, osc_detune:1.5, amp_attack:0.002, amp_decay:2.5, amp_sustain:0.0, amp_release:0.3,
              filter_type:0, filter_cutoff:9000,
              gen1_source:1, gen1_wave:0, gen1_freq_mode:0, gen1_freq:2, gen1_gain:0.6, gen1_env_fast_rate:1.2,
              gen2_source:1, gen2_wave:0, gen2_freq_mode:0, gen2_freq:3, gen2_gain:0.4, gen2_env_fast_rate:2.0,
              gen3_source:1, gen3_wave:0, gen3_freq_mode:0, gen3_freq:4, gen3_gain:0.25, gen3_env_fast_rate:3.0,
              gen5_source:2, gen5_gain:0.12, gen5_filt_type:2, gen5_filt_alpha:0.85, gen5_env_fast_rate:120, gain:0.9}
piano-felt:  {osc_type:0, osc_enabled:1, osc_detune:1.2, amp_attack:0.004, amp_decay:1.8, amp_sustain:0.0, amp_release:0.3,
              filter_type:0, filter_cutoff:4500,
              gen1_source:1, gen1_wave:0, gen1_freq_mode:0, gen1_freq:2, gen1_gain:0.45, gen1_env_fast_rate:1.6,
              gen2_source:1, gen2_wave:0, gen2_freq_mode:0, gen2_freq:3, gen2_gain:0.22, gen2_env_fast_rate:2.6,
              gen5_source:2, gen5_gain:0.06, gen5_filt_type:1, gen5_filt_alpha:0.6, gen5_env_fast_rate:160, gain:0.9}
```

- [ ] **Step 1: Failing test** — `TestSeed_Piano_Audible` + `TestSeed_KeysCategory` + `TestSeed_Piano_DecaysNotSustained` (assert the tail half has lower RMS than the first half — percussive decay, sustain≈0).
- [ ] **Step 2: Run — verify fail.**
- [ ] **Step 3: Add seeds** (two vars).
- [ ] **Step 4: Wire both**; `synthCategory()` `piano` prefix → "Keys (Synth)".
- [ ] **Step 5: Run — verify pass.**
- [ ] **Step 6: Discipline guards.**
- [ ] **Step 7: Checkpoint (no commit)** — audition + tune.

---

### Task 4: Winds — Woodwind (flute, oboe)

**Instruments (Beats: 2.0):** `flute`, `flute-breathy`, `oboe`, `oboe-full`. Category **"Winds (Synth)"**.

Seeds:
```
flute:         {osc_type:0, osc_enabled:1, filter_type:0, filter_cutoff:4000,
                amp_attack:0.06, amp_decay:0.1, amp_sustain:0.80, amp_release:0.12,
                lfo_enabled:1, lfo_rate:5.5, lfo_depth:0.10, lfo_target:1,
                gen1_source:2, gen1_gain:0.10, gen1_filt_type:5, gen1_filt_freq:2800, gen1_filt_q:0.7, gen1_env_fast_rate:0.5, gain:0.9}
flute-breathy: {osc_type:0, filter_cutoff:4000, amp_attack:0.05, amp_sustain:0.78, amp_release:0.12,
                lfo_enabled:1, lfo_rate:5.5, lfo_depth:0.10, lfo_target:1,
                gen1_source:2, gen1_gain:0.20, gen1_filt_type:5, gen1_filt_freq:2800, gen1_filt_q:0.7, gen1_env_fast_rate:0.5, gain:0.9}
oboe:          {osc_type:1, osc_enabled:1, filter_type:2, filter_cutoff:1400, filter_resonance:3.0,
                amp_attack:0.02, amp_decay:0.06, amp_sustain:0.85, amp_release:0.09,
                lfo_enabled:1, lfo_rate:5.5, lfo_depth:0.22, lfo_target:1, gain:0.9}
oboe-full:     {osc_type:1, filter_type:2, filter_cutoff:1800, filter_resonance:2.5,
                amp_attack:0.02, amp_sustain:0.88, amp_release:0.09,
                lfo_enabled:1, lfo_rate:5.5, lfo_depth:0.22, lfo_target:1, gain:0.9}
```

- [ ] **Step 1: Failing test** — `TestSeed_Woodwind_Audible` + `TestSeed_WindsCategory` + `TestSeed_Flute_HasVibrato` (render flute, assert it differs from the same seed with `lfo_enabled:0` → vibrato is active).
- [ ] **Step 2: Run — verify fail.**
- [ ] **Step 3: Add seeds** (four vars).
- [ ] **Step 4: Wire all four**; `synthCategory()` `flute`/`oboe` prefix → "Winds (Synth)".
- [ ] **Step 5: Run — verify pass.**
- [ ] **Step 6: Discipline guards.**
- [ ] **Step 7: Checkpoint (no commit)** — audition + tune.

---

### Task 5: Winds — Brass (trumpet, french horn)

**Instruments (Beats: 2.0):** `trumpet`, `trumpet-mellow`, `french-horn`, `french-horn-loud`. Category **"Winds (Synth)"**.

Seeds (subtractive saw + LP + amp attack-overshoot via short attack + vibrato):
```
trumpet:          {osc_type:1, osc_enabled:1, filter_type:0, filter_cutoff:3500, filter_resonance:1.0,
                   amp_attack:0.025, amp_decay:0.07, amp_sustain:0.55, amp_release:0.10,
                   lfo_enabled:1, lfo_rate:5.8, lfo_depth:0.20, lfo_target:1, drive:0.1, gain:0.9}
trumpet-mellow:   {osc_type:1, filter_cutoff:2200, amp_attack:0.035, amp_sustain:0.55, amp_release:0.12,
                   lfo_enabled:1, lfo_rate:5.5, lfo_depth:0.16, lfo_target:1, gain:0.9}
french-horn:      {osc_type:1, filter_cutoff:1800, filter_resonance:1.0,
                   amp_attack:0.07, amp_decay:0.12, amp_sustain:0.5, amp_release:0.2,
                   lfo_enabled:1, lfo_rate:5.0, lfo_depth:0.10, lfo_target:1, gain:0.95}
french-horn-loud: {osc_type:1, filter_cutoff:2800, amp_attack:0.04, amp_sustain:0.55, amp_release:0.2,
                   lfo_enabled:1, lfo_rate:5.0, lfo_depth:0.12, lfo_target:1, drive:0.1, gain:0.95}
```

- [ ] **Step 1: Failing test** — `TestSeed_Brass_Audible` + `TestSeed_Brass_TrumpetBrighterThanHorn` (summed abs first-difference: trumpet > french-horn).
- [ ] **Step 2: Run — verify fail.**
- [ ] **Step 3: Add seeds** (four vars).
- [ ] **Step 4: Wire all four** (Winds category already covers via prefix — add `trumpet`/`french-horn` prefixes to `synthCategory()`).
- [ ] **Step 5: Run — verify pass.**
- [ ] **Step 6: Discipline guards.**
- [ ] **Step 7: Checkpoint (no commit)** — audition + tune.

---

### Task 6: Full-suite verification + WASM parity + docs

- [ ] **Step 1: Full audio suite** — `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -v` (all 20 instruments registered, all discipline guards green; full suite may be slow — if it times out under external load, scope to `-run 'Seed|Recipe|Synth|Catalog|Instrument'` and note it).
- [ ] **Step 2: WASM build + bridge smoke + xplat parity** — `make wasm`; `GO=…/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`; `GO=…/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js`. Expect PASS (no new exports; modular path generic).
- [ ] **Step 3: Docs** — add a "Synthesized Melodic Instruments (Strings/Keys/Winds)" section to `AGENTS.synth.instruments.md` documenting the seed approach + the per-family recipes (so the knowledge persists).
- [ ] **Step 4: Checkpoint (no commit).** Hand off the full set for audition. Capture tuned-seed learnings in memory.

---

## Self-Review

**Spec coverage:** All 20 instruments from design §4 across 3 categories — Tasks 1–5. WASM parity — Task 6. Docs — Task 6. ✓
**Vibrato dependency:** bowed strings/winds use `lfo_target:1` (engine-LFO-routing plan, DONE). ✓
**Placeholder scan:** every seed is a concrete literal; every test has real code. (Seed VALUES are explicitly starting points per Global Constraints — that is intended, not a placeholder.) ✓
**Type consistency:** `renderSeedInstrument`/`assertAudible` helpers (Task 1) reused by all later tasks; `RecipeForInstrument`/`RecipeDefaultParams`/`recipeParamsToModular`/`synthCategory`/`defaultModularParams` are existing audio-package functions. ✓
**Known follow-up (from engine review):** the UI `lfo_depth` knob shows one 0..1 range for three target semantics — note in Task 6 docs; a dedicated per-target range is optional future polish.
