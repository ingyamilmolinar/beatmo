# Synth Refinement (v2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make the crappy-sounding synthesized instruments (violin, cello, trumpet, bass, ride) sound good, and make the genre showcase circuits musically interesting.

**Architecture:** Two engine changes un-freeze and de-munchkin the subtractive modular voices: (1) a **filter envelope** (cutoff brightens on attack, decays to base — the classic acoustic attack), and (2) **per-pitch re-render** for melodic recipes (render at the played fundamental so the filter formant stays at absolute Hz, instead of resampling one A3 buffer and sliding the spectrum). Then rework the bad seeds (filter env + layered body/sub-osc + fixed bow noise + octave), fix the ride (seed retune + a small bell C param + longer buffer), and rewrite the showcase compositions richer.

**Tech Stack:** C modular voice (`src/c/modular.c`, `modular_stages.c`), Go audio (`internal/audio`), the `cmd/gen-showcase` circuit builder.

**Depends on:** the LFO-routing plan (DONE — `mod_biquad_set_coeffs`, `lfo_target`, the tail-append param pattern) and the instrument-seed + showcase plans (DONE).

## Global Constraints

- **Byte-identity for existing instruments/drums:** every engine change is identity-gated (new params default to no-op) so all drum goldens + existing instruments render bit-for-bit identical. The per-pitch re-render is scoped to MELODIC recipes only — drums keep the resample path unchanged.
- **Tail-append params** (struct / both marshalling directions / `modularGlobalsPhase8` schema / `genGlobalsPhase8` test / ParamDef / stage group) — exactly like `lfo_target`. New scalar params go at the END.
- **After any `src/c/modular*.c` edit, rebuild `build/libdrums.a`** (`make build/libdrums.a`) before Go tests; the audio package links the prebuilt lib.
- **Engine/recipe tests run on the REAL CGo build:** `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run X` (NOT `-tags test`). Schema param additions require regenerating `synth_param_abi.gen.js` (`go run ./cmd/gen-synth-abi > ../js/synth_param_abi.gen.js`, or `make wasm`).
- **Seeds are tuning starting points** — the user auditions and we keep refining. "Done" = audibly improved + tests green, not perfect.
- **Template colors follow `templates.InstrumentSequence[row]`** (enforced). Circuit edits go through `cmd/gen-showcase` (`go run ./cmd/gen-showcase`) + the drift test.
- **Bundled Go;** **Do NOT `git add`/`git commit`** (standing rule). "Checkpoint" = run tests + pause.

---

### Task 1: Filter envelope (engine)

Cutoff brightens by `filtenv_amt` octaves at onset, decaying with `filtenv_decay` to the base cutoff. Separate stage from the LFO cutoff sweep (own enable gate) so it composes with vibrato.

**Files:** `src/c/modular.h` (params at struct tail), `src/c/modular.c` (filter stage third branch), `modular_c.go` (struct + both marshalling), `synth_param_schema.go` (`modularGlobalsPhase8`), `modular_genbank_schema_test.go` (`genGlobalsPhase8`), `modular_recipe.go` (3 ParamDefs), `modular_stage_params.go` (stage group), `modular_render_test.go` (tests).

**Interfaces:** new params `filtenv_enabled` (0/1, default 0 = off/identity), `filtenv_amt` (octaves, e.g. 0..4), `filtenv_decay` (s, e.g. 0.01..2). Reuses `mod_biquad_set_coeffs`.

- [ ] **Step 1: Failing test** (`modular_render_test.go`):

```go
func TestFiltEnv_BrightenThenSettleAndIdentityWhenOff(t *testing.T) {
	const sr, n = 48000, 48000
	mk := func(enabled, amt float64) []float32 {
		p := defaultModularParams()
		p.OscType = 1 // saw (rich harmonics so cutoff motion is audible)
		p.EnvEnabled = 0
		p.FilterEnabled = 1
		p.FilterType = 0
		p.FilterCutoff = 1200
		p.FiltEnvEnabled = enabled
		p.FiltEnvAmt = amt
		p.FiltEnvDecay = 0.2
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		return buf
	}
	off := mk(0, 3) // disabled → identity (amt ignored)
	base := defaultModularParams()
	base.OscType = 1
	base.EnvEnabled = 0
	base.FilterEnabled = 1
	base.FilterType = 0
	base.FilterCutoff = 1200
	baseBuf := make([]float32, n)
	renderModularP(baseBuf, sr, n, base)
	for i := range off {
		if off[i] != baseBuf[i] {
			t.Fatalf("filtenv disabled not byte-identical at %d", i)
		}
	}
	on := mk(1, 3) // enabled → differs (brighter attack)
	diff := 0
	for i := range on {
		if on[i] != baseBuf[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("filter envelope produced no change")
	}
	// Attack window should be brighter (more HF energy = larger sample-to-sample delta) than the tail.
	d := func(b []float32, a, z int) float64 {
		var s float64
		for i := a + 1; i < z; i++ {
			s += math.Abs(float64(b[i] - b[i-1]))
		}
		return s
	}
	if d(on, 0, 2000) <= d(on, 40000, 42000) {
		t.Fatal("filter envelope attack not brighter than tail")
	}
}
```

- [ ] **Step 2: Run → fails** (`FiltEnvEnabled` undefined). `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestFiltEnv -v`.
- [ ] **Step 3: C params** — in `src/c/modular.h`, after `float lfo_target;` (struct tail):

```c
    /* Filter envelope: cutoff *= 2^(amt * exp(-t/decay)) — brightens on attack,
     * decays to base. Separate from the LFO cutoff sweep so it composes with
     * vibrato. Default 0 = off = byte-identical. */
    float filtenv_enabled;
    float filtenv_amt;
    float filtenv_decay;
```

- [ ] **Step 4: C filter stage** — in `src/c/modular.c`, read the params near `lfo_target` (~line 170):

```c
    int   filtenv_on    = mp_get(p, 0.0f, p->filtenv_enabled, have) >= 0.5f;
    float filtenv_amt   = mp_get(p, 0.0f, p->filtenv_amt,   have);
    float filtenv_decay = mp_get(p, 0.0f, p->filtenv_decay, have);
```

Then extend the filter stage. The `!cut_sweep` static path becomes: if `filtenv_on && filtenv_amt>0 && filtenv_decay>0`, run a per-64-sample envelope branch (mirroring the cut_sweep branch but with `c = cutoff * pow(2, amt*exp(-t/decay))`); else the existing static path. (cut_sweep keeps priority or compose — simplest: filter-envelope branch only when `!cut_sweep`.) Use `mod_biquad_set_coeffs`. Byte-identity: when `filtenv_on` is false, the existing static path runs unchanged.

```c
    if (filt_on) {
        mod_biquad filt;
        int cut_sweep   = lfo_on && lfo_target == 2 && lfo_depth > 0.0f && lfo_rate > 0.0f;
        int filt_env_on = filtenv_on && filtenv_amt > 0.0f && filtenv_decay > 0.0f;
        if (cut_sweep) {
            /* existing LFO cutoff-sweep branch (unchanged) */
            ...
        } else if (filt_env_on) {
            mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
            for (int i = 0; i < samples; i++) {
                if ((i & 63) == 0) {
                    double t = (double)i / (double)sampleRate;
                    double oct = (double)filtenv_amt * exp(-t / (double)filtenv_decay);
                    double c = (double)cutoff * pow(2.0, oct);
                    mod_biquad_set_coeffs(&filt, filt_type, c, (double)q, sampleRate);
                }
                out[i] = mod_biquad_tick_f(&filt, out[i]);
            }
        } else {
            mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
            for (int i = 0; i < samples; i++) out[i] = mod_biquad_tick_f(&filt, out[i]);
        }
    }
```

- [ ] **Step 5: Go plumbing** — `modular_c.go`: `ModularParams` fields `FiltEnvEnabled/FiltEnvAmt/FiltEnvDecay` (after `LfoTarget`); `toCModularParams` (`filtenv_enabled: C.float(p.FiltEnvEnabled)`, etc.); `recipeParamsToModular` (`FiltEnvEnabled: get("filtenv_enabled")`, etc.). `synth_param_schema.go`: append `{"filtenv_enabled",0},{"filtenv_amt",0},{"filtenv_decay",0}` to the `modularGlobalsPhase8` list tail. `modular_genbank_schema_test.go`: same 3 to `genGlobalsPhase8`. `modular_recipe.go`: 3 ParamDefs (group "filter", Min/Max/Default: enabled 0/1/0, amt 0/4/0, decay 0.01/2/0.3). `modular_stage_params.go`: add `filtenv_amt`,`filtenv_decay` to a stage with toggle `filtenv_enabled`.
- [ ] **Step 6: Rebuild + run** — `make build/libdrums.a` then the test → PASS. Then byte-identity checkpoint: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'Modular|ByteIdent|Recipe|Schema|FiltEnv' -v` → PASS.
- [ ] **Step 7: ABI regen** — `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-synth-abi > ../js/synth_param_abi.gen.js`; confirm `TestSynthParamABIGenFileIsCurrent` passes.
- [ ] **Step 8: Checkpoint (no commit).**

---

### Task 2: Per-pitch re-render for melodic recipes (de-munchkin)

Render melodic recipe voices at the node's pitch (so the filter formant stays at absolute Hz) instead of resampling one base-pitch buffer. Scoped to melodic recipes; drums unchanged (byte-identical goldens).

**Files:** `synth_recipe_dispatch.go` (`tryRecipeVoiceOpts` — pass node pitch into merged modular `pitch` param), `voice_cache.go` (cache key includes pitch for pitch-aware recipes), `engine_play.go` (`PlayParams` — skip the pitch resample when the voice was rendered at-pitch), a `pitchAwareRecipe(id) bool` predicate (the new melodic recipe ids), `engine_play_test.go` / a new test.

**Interfaces:** `pitchAwareRecipe(recipeID string) bool` — true for the synth-modular melodic instrument recipes (violin/cello/guitars/piano/flute/oboe/trumpet/french-horn families), false for drums + bass-family + the base modular. Voice cache key gains a `pitch` field (0 for non-pitch-aware).

- [ ] **Step 1: Failing test** — assert formant preservation. Render a melodic instrument (e.g. violin) at pitch 0 and at pitch +12 via the full play path; the +12 version's spectral-centroid-to-fundamental RATIO should stay ~constant (formant fixed), whereas the OLD resample path shifts the whole spectrum. Concretely: compare `renderRecipeAtPitch("violin", 0)` and `renderRecipeAtPitch("violin", 12)` (a test helper that exercises the new at-pitch render) and assert the +12 buffer's fundamental is ~2× while its high-frequency rolloff knee (a fixed filter cutoff) is NOT 2× (i.e. the filter stayed at absolute Hz). If a spectral test is too heavy, assert: the at-pitch render of pitch+12 differs from a resample-by-+12 of the pitch-0 buffer (proving re-render ≠ resample). Also assert a drum (`kick`) is UNCHANGED (still resampled, byte-identical).
- [ ] **Step 2: Run → fails.**
- [ ] **Step 3: `pitchAwareRecipe` predicate** — list the melodic recipe ids (the synth-modular-<instrument> set; exclude drum-*, fm-bass, sub-bass, bass-guitar, synth-modular base/pad).
- [ ] **Step 4: Render at-pitch** — in `tryRecipeVoiceOpts`, when the recipe is pitch-aware and a node pitch is supplied, set the merged modular `pitch` param to the node pitch (so `freq = base·2^(pitch/12)`) before `recipe.Render`; thread the pitch through from the play path.
- [ ] **Step 5: Cache key** — add `pitch` to `voiceCacheKey` (rounded to nearest semitone) for pitch-aware recipes; 0 otherwise (drum keys unchanged → drum goldens unaffected).
- [ ] **Step 6: Skip resample** — in `PlayParams`, when the resolved voice is pitch-aware/rendered-at-pitch, do NOT apply the `2^(pitch/12)` resample factor (apply only the duration factor). Keep the full resample for non-pitch-aware.
- [ ] **Step 7: Rebuild + run** — test PASS; drum byte-identity + xplat goldens green (`-run 'Modular|ByteIdent|Recipe|Drum|Golden'`).
- [ ] **Step 8: Checkpoint (no commit).**

> Risk note: this is the most invasive change. If the duration-vs-pitch coupling proves hard, the minimal correct version is: render at-pitch with `samples` already set to the note duration (so no resample at all for pitch-aware voices), caching by (pitch, durationBucket). Keep drums on the untouched path.

---

### Task 3: Fix the bad instruments (seeds) + the ride

**Files:** `instrument_seeds.go` (rework violin/cello/trumpet + add a dedicated bass), `synth_recipe.go` (bindings if a new bass recipe), `cymbal_modular_push.go` (ride cym param retune) + `modular_stages.c` (bell params, small) + `config.go` (ride DurationSec), `modular.h`/`modular_c.go`/schema (bell params tail-append).

- [ ] **Step 1: Rework violin/cello/trumpet seeds** — for each: enable the filter envelope (`filtenv_enabled:1`, amt ~1.5–2.5 strings / ~3 brass, decay ~0.15–0.3); add a detuned-unison body slot (`gen2_source:1, gen2_wave:1, gen2_freq_mode:0, gen2_freq:1.005, gen2_gain:0.5`) and a sub-octave slot for cello/trumpet (`gen3_source:1, gen3_wave:0, gen3_freq:0.5, gen3_gain:0.3`); fix bow noise (BP center 4000→~2000, Q 0.8→~2, add `gen1_env_tail_mix:0.2` for sustained scrape); **cello `osc_octave:-1`** (drop register). Add/extend audibility + "distinct from default" tests; keep them passing.
- [ ] **Step 2: Dedicated bass seed** — add `synth-bass` (e.g. id `synth-bass`): saw + strong LP + filter-envelope pluck + sub-octave sine, lower register (`osc_octave:-1`), short-ish decay. Wire it (6 touchpoints) + category "Bass (Synth)". Render-smoke test.
- [ ] **Step 3: Ride retune (seed/binding)** — in `cymbal_modular_push.go` `cymbalVariantSpecs["drum-ride"]`: `cym_env_tail` 8→~2.5, `cym_noise_mix` 0.3→~0.15, faster `cym_noise_decay`. `config.go`: ride `DurationSec` 1.0→2.5. Test: ride tail RMS at 1.5s > a threshold (rings longer than a hat).
- [ ] **Step 4: Ride bell param (small C)** — add `gen_cym_bell_m`/`gen_cym_bell_d` (tail-append) read at the bell line in `modular_gen_slot_cymbal` (replace the hard-coded 0.15 / exp(-6t)); set the ride binding to a prominent bell (`cym_bell_m ~0.4`, `cym_bell_d ~3`). Rebuild. Test: ride with bell differs from bell-off + still finite.
- [ ] **Step 5: Checkpoint (no commit)** — audition the reworked instruments.

---

### Task 4: Richer, more interesting compositions (all showcases)

Rewrite the showcase circuits to be musically interesting: longer phrases (2–4 bars) with variation between bars (not 1-bar loops), denser/develop­ing melodic lines, fills, counter-melodies, dynamics (per-note volume), and groove (per-note `Groove`/swing). Keep melodic lines well within range now that re-render (Task 2) lifts the ±7 limit — use the full musical range.

**Files:** `cmd/gen-showcase/main.go` (extend `Hit` with `Vol`/`Dur`/groove already present; optionally support per-bar variation by just authoring more bars), `cmd/gen-showcase/specs.go` (rewrite all 10 circuits richer), regenerate, drift + Template suite.

- [ ] **Step 1: Builder capability** — confirm `Hit` supports `Vol`, `Dur` (it does); add optional `Groove`/`GroovePct` passthrough to nodes for swing if not present. Bump default `Bars` to 2–4 per circuit as authored.
- [ ] **Step 2: Rewrite classical** (canon/ode) — fuller arpeggios, the complete recognizable phrase (Canon ≥4 bars descending sequence; Ode full A-phrase), violin with dynamics + the dotted-rhythm Ode ending.
- [ ] **Step 3: Rewrite rock/funk/pop** — rock: 2-bar riff with a turnaround + lead fill; funk: syncopated 16th bass + guitar chank interplay; pop: verse-y 4-bar piano+flute with a counter-line.
- [ ] **Step 4: Rewrite jazz/salsa** — jazz: 2-bar ii-V-I with a walking bass that moves + a trumpet bebop line; salsa: 2-bar son-clave with montuno development + tumbao + trumpet mambo hits.
- [ ] **Step 5: Rewrite electronic (house/techno/hiphop)** — evolving arps, filter-envelope stabs, 2–4 bar bass movement, hat variation/rolls.
- [ ] **Step 6: Regenerate + validate** — `go run ./cmd/gen-showcase`; drift + `Template` suite green.
- [ ] **Step 7: Checkpoint (no commit)** — audition the full refreshed set.

---

### Task 5: WASM parity + docs/memory

- [ ] **Step 1:** `make wasm`; bridge smoke + xplat parity green (new params ride the modular block).
- [ ] **Step 2:** Update `AGENTS.synth.instruments.md` with the filter-envelope + per-pitch-render + the tuned recipes (persist the knowledge once auditioned). Update the project memory.
- [ ] **Step 3: Checkpoint (no commit).** Hand off for audition.

---

## Self-Review
- Root causes from the diagnosis → Task 1 (frozen spectrum / filter env) + Task 2 (resample munchkin / re-render). ✓
- Bad instruments (violin/cello/trumpet/bass) → Task 3 seeds; ride → Task 3 seed+bell. ✓
- "More interesting, less square" compositions → Task 4. ✓
- Byte-identity for drums/existing preserved (identity-gated engine changes; re-render scoped to melodic recipes). ✓
- Placeholders: Task 2's spectral test + the pitch-aware threading are described, not fully coded — the implementer designs the cleanest correct version per the risk note (the diagnosis Q1 Option A/B has the mechanism). Flag for careful review.
