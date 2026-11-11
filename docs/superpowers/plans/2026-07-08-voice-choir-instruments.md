# Voice/Choir Instruments + Synth Stage Modularization — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Voice category with six voice/choir instruments driven by a new FORMANT stage, a humanized ENSEMBLE stage, a surfaced RESONATOR stage, and a glottal-pulse oscillator — plus a byte-identical decomposition of the oversized C stage functions.

**Architecture:** Source–filter voice synthesis inside the existing modular pipeline: glottal/saw source (with per-voice humanized unison) → existing stages → new parallel 5-band formant bank (Csound Appendix D vowel tables) → existing body resonator. All new params append at the `modular_params` ABI tail and default off, so the 42 existing instruments render byte-identically. Instruments are config-only rows in `modularInstrumentDefs`.

**Tech Stack:** C (src/c/modular.c, modular_stages.c, wavetable.c), Go 1.23 (internal/audio, internal/ui, internal/i18n), generated JS ABI (gen-synth-abi, gen-modular-instruments).

**Spec:** `docs/superpowers/specs/2026-07-08-voice-choir-instruments-design.md`

## Global Constraints

- **NEVER `git add` / `git commit` / `git revert` / `rm`** — leave every change uncommitted in the working tree (standing user rule; overrides this skill's usual commit-per-task cadence). "Commit" steps are replaced by verification steps.
- **Go binary:** always `/home/ymolinar/Repos/beatmo/.tools/go/bin/go` (referred to as `$GO` below; set `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go`).
- **After ANY C edit:** run `make build/libdrums.a` from the repo root before running Go tests (the rule force-relinks cgo).
- **Modular render tests are real CGo:** run `cd src/go && $GO test ./internal/audio/ -run <X>` with NO `-tags test` and NO `-modfile` (the C synth stubs to zero under `-tags test`).
- **UI tests are the fast path:** `cd src/go && $GO test -tags test -modfile=go.test.mod ./internal/ui/ -run <X>`.
- **ABI is append-only.** New `modular_params` fields go at the VERY tail, after `osc_bow_loss` (`src/c/modular.h:481`). Never reorder anything above.
- **Byte-identity gate:** `TestSynthGoldenByteIdentity` must stay green through Tasks 1–9 (no golden changes until the new instruments are added in Task 10).
- **No seed may set `"gain": 0.5`** (TestPhase2Native mutates gain→0.5 and needs a delta). Use 0.55/0.6.
- **No new C files** (avoids Makefile C_SRC/C_OBJS + emcc manifest churn). New C code goes into the existing modular.c / modular_stages.c / wavetable.c.
- Param naming: lowercase snake_case; discrete values are integer-valued floats.
- Every new UI-visible string needs EN + es-419 i18n entries.

---

### Task 1: C decomposition — extract the OSC source ladder into static helpers (byte-identical)

**Files:**
- Modify: `src/c/modular.c` (the `render_modular_p` OSC stage, lines ~508–677)

**Interfaces:**
- Produces: static helpers inside modular.c — `modular_source_noise`, `modular_source_fm`, `modular_source_physical`, `modular_source_wt_modulated`, `modular_source_unison`, `modular_source_plain`. Signatures below. Task 6 modifies `modular_source_unison`; Task 7 relies on the wavetable branches dispatching any `osc_type` via `modular_table_for`.

- [ ] **Step 1: Capture the pre-refactor goldens as the failing/passing baseline**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && $GO test ./internal/audio/ -run 'TestSynthGoldenByteIdentity' -v
```
Expected: PASS (this is the baseline; if it fails, STOP — the branch is already broken, report it).

- [ ] **Step 2: Extract the six source branches into static functions**

Immediately above `render_modular_p` in `src/c/modular.c`, add six static helpers, then replace each `else if` body in the ladder (lines ~508–677) with a call. Move the bodies VERBATIM — no arithmetic/ordering changes. The ladder's conditions stay in `render_modular_p`; only the bodies move:

```c
/* ── Source-stage primitives (Phase-15 decomposition). Each renders the OSC
 * stage's buffer for one branch of the osc_type dispatch. Bodies are verbatim
 * transplants from the render_modular_p ladder — byte-identity is gated by
 * TestSynthGoldenByteIdentity. ── */
static void modular_source_noise(float *out, int samples, int osc_type,
                                 unsigned int noise_seed);
static void modular_source_fm(float *out, int sampleRate, int samples,
                              const modular_params *p, double freq,
                              int fm_on, int pe_on, float pe_amt, float pe_decay);
static void modular_source_physical(float *out, int sampleRate, int samples,
                                    const modular_params *p, int osc_type,
                                    double freq, unsigned int noise_seed);
static void modular_source_wt_modulated(float *out, int sampleRate, int samples,
                                        int osc_type, double freq,
                                        int pe_active, float pe_amt, float pe_decay,
                                        int vib_on, float lfo_depth, float lfo_rate,
                                        float lfo_delay);
static void modular_source_unison(float *out, int sampleRate, int samples,
                                  const modular_params *p, int have,
                                  int osc_type, double freq,
                                  int unison_nv, float unison_detune, float unison_mix,
                                  float unison_drift_rate, float unison_drift_depth,
                                  unsigned int noise_seed);
static void modular_source_plain(float *out, int sampleRate, int samples,
                                 int osc_type, double freq);
```

`modular_source_physical` contains the osc_type 7/8/9/10/11 sub-dispatch (the five `render_*` waveguide calls with their `kp_get` argument lists, verbatim). The `p`/`have` args on `modular_source_unison` are unused in this task (they exist for Task 6's ensemble reads) — mark them `(void)p; (void)have;` for now.

- [ ] **Step 3: Rebuild and verify byte identity**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && $GO test ./internal/audio/ -run 'TestSynthGoldenByteIdentity|TestModularNoEditFastPathMatchesRecipe|TestCompleteness' -v 2>&1 | tail -20
```
Expected: PASS, zero golden drift. If ANY golden fails, the transplant changed arithmetic — diff your extraction against the original body (float promotion order matters; keep `double` intermediates exactly as they were).

- [ ] **Step 4: Run the wider modular test net**

```bash
cd src/go && $GO test ./internal/audio/ 2>&1 | tail -5
```
Expected: `ok` (same failures as pre-existing baseline, if any — record them first via `git stash && $GO test ./internal/audio/ 2>&1 | tail -5 && git stash pop` ONLY if unsure; do not use stash otherwise).

---

### Task 2: C decomposition — split `modular_gen_slot_kick` into per-variant functions (byte-identical)

**Files:**
- Modify: `src/c/modular_stages.c` (`modular_gen_slot_kick`, ~lines 384–1060)

**Interfaces:**
- Produces: static per-variant functions in modular_stages.c: `gen_kick_base`, `gen_kick_deep`, `gen_kick_punchy`, `gen_kick_lofi`, `gen_kick_tight`, `gen_kick_hybrid`, `gen_kick_modal`, `gen_kick_acoustic`. Each takes the same slot-render context the variant branch already consumes (pass the existing locals as parameters — read the function first and mirror what each branch actually touches).

- [ ] **Step 1: Read `modular_gen_slot_kick` end-to-end and map the variant dispatch**

Identify the variant selector read (`kick_variant`, values 0–7) and the exact line span of each variant's branch. List which locals each branch reads/writes (envelope state, noise taps, harmonic banks, output accumulation).

- [ ] **Step 2: Extract one variant at a time, rebuilding + golden-checking after EACH**

For each variant (start with the smallest branch), move its body verbatim into a `static void gen_kick_<variant>(...)` function directly above `modular_gen_slot_kick`, passing every local the branch consumes. After each extraction:

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && $GO test ./internal/audio/ -run 'TestSynthGoldenByteIdentity' 2>&1 | tail -3
```
Expected: PASS after every single extraction. Never batch two extractions between golden runs — a drift found after one move is trivially localized; after four it is not.

- [ ] **Step 3: Repeat the golden + kick-specific nets at the end**

```bash
cd src/go && $GO test ./internal/audio/ -run 'Kick|TestSynthGoldenByteIdentity|Golden' 2>&1 | tail -10
```
Expected: PASS (includes the dnb-kick and drum-gen-defaults drift guards).

---

### Task 3: C decomposition — split cymbal and snare gen slots (byte-identical)

**Files:**
- Modify: `src/c/modular_stages.c` (`modular_gen_slot_cymbal` ~1544–1964; `modular_gen_slot_snare` ~1182–1465)

**Interfaces:**
- Produces: `gen_cymbal_hihat`, `gen_cymbal_open`, `gen_cymbal_cowbell`, `gen_cymbal_shaker`, `gen_cymbal_ride`, `gen_cymbal_crash`; `gen_snare_main`, `gen_snare_rimshot`, `gen_snare_sidestick`. Same extraction discipline as Task 2.

- [ ] **Step 1: Extract cymbal variants one at a time, golden-check after each**

Same procedure and commands as Task 2 Step 2, applied to the 6 cymbal variant branches.

- [ ] **Step 2: Extract snare variants one at a time, golden-check after each**

Same procedure for the 3 snare variant branches.

- [ ] **Step 3: Full modular net**

```bash
cd src/go && $GO test ./internal/audio/ 2>&1 | tail -5
```
Expected: same pass/fail set as the Task 1 baseline.

---

### Task 4: ABI append — Phase-15 voice params (C struct + Go schema + ParamDefs + JS ABI)

No DSP reads yet — this task only plumbs the 13 new params end-to-end so the shape tests pin them. Every default/identity keeps renders byte-identical.

**Files:**
- Modify: `src/c/modular.h` (append after `osc_bow_loss`, line 481, inside the struct)
- Modify: `src/go/internal/audio/synth_param_schema.go` (new Phase-15 table after `modularGlobalsPhase14Bow` :379; schema append after :493; identity append after :681)
- Modify: `src/go/internal/audio/modular_recipe.go` (ParamDefs append after the Phase-14 bow block :488; tier map :505)
- Regen: `src/js/synth_param_abi.gen.js` via `make gen-synth-abi`
- Test: `src/go/internal/audio/synth_param_schema_test.go` (existing shape tests must pass; extend counts if pinned)

**Interfaces:**
- Produces: `modular_params` fields + schema names, consumed by Tasks 5/6 (C reads) and Task 8 (UI groups): `formant_enabled, formant_vowel, formant_voice_type, formant_mix, formant_shift, formant_breath, formant_sing, formant_morph_rate, formant_morph_to, ens_scatter, ens_vib_rate, ens_vib_depth, ens_humanize`.
- Identity/default/C-fallback triples MUST agree: all 0.0 except `formant_shift` = 1.0 (mp_get fallback 1.0; 0-ish reads as 1.0).
- `formant_vowel`, `formant_voice_type`, `formant_morph_to` are read DIRECTLY (`lrintf`/raw), not via mp_get, because 0 is a valid choice (soprano / vowel "a") — same pattern as `osc_type`.

- [ ] **Step 1: Append the C struct fields**

At `src/c/modular.h`, after `float osc_bow_loss;` (:481), before the closing `} modular_params;`:

```c
    /* ── Phase-15 VOICE/CHOIR (formant vowel bank + ensemble humanization).
     * APPEND-ONLY at the VERY tail. formant_enabled 0 (identity) = the stage
     * never runs; all ens_* 0 (identity) = the unison path is byte-identical.
     * formant_vowel/voice_type/morph_to are read directly (0 is a valid enum
     * value: vowel "a" / soprano); formant_shift is mp_get(1.0) so the
     * zero-struct reads as no shift. ── */
    float formant_enabled;    /* >=0.5 runs the formant stage (default 0) */
    float formant_vowel;      /* 0..4 continuous: a → e → i → o → u */
    float formant_voice_type; /* 0 soprano / 1 alto / 2 tenor / 3 bass */
    float formant_mix;        /* 0..1 wet blend (0 = exact bypass) */
    float formant_shift;      /* 0.5..2 formant-frequency scale (vocal-tract size) */
    float formant_breath;     /* 0..1 white-noise mix injected PRE-ADSR (whisper=1) */
    float formant_sing;       /* 0..1 singer's-formant boost (~2.4-3.1 kHz by voice) */
    float formant_morph_rate; /* Hz, 0..8: vowel-morph LFO toward morph_to (0 = static) */
    float formant_morph_to;   /* 0..4 morph target vowel */
    float ens_scatter;        /* cents, 0..20: per-voice constant pitch offset */
    float ens_vib_rate;       /* Hz, 0..8: per-voice vibrato LFO rate */
    float ens_vib_depth;      /* cents, 0..100: per-voice vibrato depth */
    float ens_humanize;       /* 0..1: randomizes per-voice vibrato phase/rate */
```

- [ ] **Step 2: Add the Phase-15 Go schema table + schema/identity appends**

In `src/go/internal/audio/synth_param_schema.go` after `modularGlobalsPhase14Bow` (:379):

```go
// modularGlobalsPhase15Voice is the Phase-15 voice/choir block: the FORMANT
// vowel-bank stage + the ENSEMBLE humanization knobs. APPEND-ONLY at the VERY
// tail (after Phase-14 bow) — matching the C struct's last fields. Identities
// are 0 (stage off / no humanization = byte-identical) except formant_shift,
// whose identity 1.0 matches the C mp_get(1.0) no-shift fallback.
var modularGlobalsPhase15Voice = []struct {
	Name     string
	Identity float64
}{
	{"formant_enabled", 0}, {"formant_vowel", 0}, {"formant_voice_type", 0},
	{"formant_mix", 0}, {"formant_shift", 1.0}, {"formant_breath", 0},
	{"formant_sing", 0}, {"formant_morph_rate", 0}, {"formant_morph_to", 0},
	{"ens_scatter", 0}, {"ens_vib_rate", 0}, {"ens_vib_depth", 0}, {"ens_humanize", 0},
}
```

In the `modularParamSchema` builder, after the Phase-14 loop (:489–493):

```go
	// Phase-15 voice/choir globals append at the VERY END (after Phase-14 bow) —
	// matching the C struct's last fields.
	for _, g := range modularGlobalsPhase15Voice {
		s = append(s, g.Name)
	}
```

In `ModularParamSchemaIdentity()`, after the Phase-14 loop (:679–682):

```go
	// Phase-15 voice/choir globals (new tail; identity 0 = stage off /
	// no humanization, formant_shift identity 1.0 = no shift).
	for _, g := range modularGlobalsPhase15Voice {
		out[g.Name] = g.Identity
	}
```

- [ ] **Step 3: Add the ParamDefs (user-facing groups "formant" / "unison")**

In `src/go/internal/audio/modular_recipe.go`, after the Phase-14 bow loop (:482–488), before the tier-marking loop:

```go
	// ── Phase-15 voice/choir: the FORMANT stage (group "formant" → its own
	// Synth-tab section) + ENSEMBLE humanization (group "unison", joining the
	// existing unison knobs in the ENSEMBLE section). formant_enabled defaults
	// 0 (off = byte-identity polarity for new stages); the numeric defaults are
	// audible starting points gated behind the pill (same carve-out rationale
	// as Phase-8C). Order MUST mirror modularGlobalsPhase15Voice. ──
	defs = append(defs,
		ParamDef{Name: "formant_enabled", Label: "Formant", Group: "formant", Min: 0, Max: 1, Default: 0,
			Enum: []string{"Off", "On"}},
		ParamDef{Name: "formant_vowel", Label: "Vowel", Group: "formant", Min: 0, Max: 4, Default: 0,
			Enum: []string{"Ah", "Eh", "Ee", "Oh", "Oo"}},
		ParamDef{Name: "formant_voice_type", Label: "Voice Type", Group: "formant", Min: 0, Max: 3, Default: 0,
			Enum: []string{"Soprano", "Alto", "Tenor", "Bass"}},
		ParamDef{Name: "formant_mix", Label: "Mix", Group: "formant", Min: 0, Max: 1, Default: 0},
		ParamDef{Name: "formant_shift", Label: "Head Size", Group: "formant", Min: 0.5, Max: 2, Default: 1},
		ParamDef{Name: "formant_breath", Label: "Breath", Group: "formant", Min: 0, Max: 1, Default: 0},
		ParamDef{Name: "formant_sing", Label: "Shine", Group: "formant", Min: 0, Max: 1, Default: 0},
		ParamDef{Name: "formant_morph_rate", Label: "Morph Speed", Group: "formant", Min: 0, Max: 8, Default: 0, Unit: "Hz"},
		ParamDef{Name: "formant_morph_to", Label: "Morph To", Group: "formant", Min: 0, Max: 4, Default: 0,
			Enum: []string{"Ah", "Eh", "Ee", "Oh", "Oo"}},
		ParamDef{Name: "ens_scatter", Label: "Scatter", Group: "unison", Min: 0, Max: 20, Default: 0, Unit: "¢"},
		ParamDef{Name: "ens_vib_rate", Label: "Vibrato Rate", Group: "unison", Min: 0, Max: 8, Default: 0, Unit: "Hz"},
		ParamDef{Name: "ens_vib_depth", Label: "Vibrato Depth", Group: "unison", Min: 0, Max: 100, Default: 0, Unit: "¢"},
		ParamDef{Name: "ens_humanize", Label: "Humanize", Group: "unison", Min: 0, Max: 1, Default: 0},
	)
```

Note: `formant_vowel` and `formant_morph_to` carry Enum labels but are CONTINUOUS (Min/Max 0–4, no Step) — the enum labels name the integer stops; verify the UI renders a knob (it keys sliders vs selectors off Enum+integer-Default heuristics — if it renders a 5-way selector instead, that is acceptable for v1; note it in the task report).

Add to `advancedTierParams` (:505):

```go
	// FORMANT — Essential: Vowel, Voice Type, Mix, Breath. Advanced: the rest.
	"formant_shift": true, "formant_sing": true,
	"formant_morph_rate": true, "formant_morph_to": true,
```

- [ ] **Step 4: Regenerate the JS ABI and run the shape tests**

```bash
cd /home/ymolinar/Repos/beatmo && make gen-synth-abi && make build/libdrums.a
cd src/go && $GO test ./internal/audio/ -run 'TestModularParamSchemaShape|TestSynthParamSchemaShape|SchemaIdentity|ParamDef|Recipe' 2>&1 | tail -15
```
Expected: PASS. If a shape test pins an exact param COUNT, bump it by 13 with a comment citing Phase-15.

- [ ] **Step 5: Byte-identity + fast UI net still green**

```bash
cd src/go && $GO test ./internal/audio/ -run 'TestSynthGoldenByteIdentity' 2>&1 | tail -3
cd src/go && $GO test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -5
```
Expected: PASS (new params are `Group: "formant"`/`"unison"` — "formant" has no section mapping yet, so its knobs don't render until Task 8; that is fine and must not crash the section router — `modularStageGroupSection` returns `(0,false)` and the def falls through to the generic router. If a UI discipline test objects to an unmapped group, TEMPORARILY set the formant defs' Group to `SynthHiddenGroup` and flip to "formant" in Task 8 — note which route you took).

---

### Task 5: FORMANT stage DSP (vowel bank + singer's formant + breath)

**Files:**
- Modify: `src/c/modular_stages.h` (declare `modular_formant_process`)
- Modify: `src/c/modular_stages.c` (vowel tables + implementation; mod_biquad already lives here)
- Modify: `src/c/modular.c` (param reads ~line 469; breath injection after gen-bank call :683; stage call between filter :804 and body-resonator :807)
- Test: `src/go/internal/audio/modular_formant_test.go` (new, real-CGo: `//go:build !test && !js`)

**Interfaces:**
- Consumes: Task 4's `modular_params` fields.
- Produces: `void modular_formant_process(float *out, int sampleRate, int samples, int voice_type, float vowel, float mix, float shift, float sing, float morph_rate, float morph_to);`

- [ ] **Step 1: Write the failing Go test**

Create `src/go/internal/audio/modular_formant_test.go`:

```go
//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

func renderModularSeed(t *testing.T, seed RecipeParams, secs float64) []float32 {
	t.Helper()
	// Mirror how existing modular tests render a seeded voice (see
	// modular_golden_test.go for the canonical call): render via the recipe
	// path with the seed overlaid on the schema identity.
	return renderModularParamsForTest(t, seed, secs) // reuse/adapt the existing helper in modular_golden_test.go; if none is exported, copy its ~10-line body here
}

// Formant stage off (identity) must be an exact bypass: byte-identical to a
// render with no formant params at all.
func TestFormantStage_OffIsExactBypass(t *testing.T) {
	base := RecipeParams{"osc_type": 1, "osc_enabled": 1}
	withOff := RecipeParams{"osc_type": 1, "osc_enabled": 1, "formant_mix": 0.9} // enabled ABSENT
	a := renderModularSeed(t, base, 0.5)
	b := renderModularSeed(t, withOff, 0.5)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs with formant disabled: %v vs %v", i, a[i], b[i])
		}
	}
}

// Enabling the stage with mix > 0 must change the output and stay finite.
func TestFormantStage_OnChangesOutputFinite(t *testing.T) {
	base := RecipeParams{"osc_type": 1, "osc_enabled": 1}
	on := RecipeParams{"osc_type": 1, "osc_enabled": 1,
		"formant_enabled": 1, "formant_mix": 0.9, "formant_vowel": 0, "formant_voice_type": 2}
	a := renderModularSeed(t, base, 0.5)
	b := renderModularSeed(t, on, 0.5)
	diff := false
	for i := range b {
		if math.IsNaN(float64(b[i])) || math.IsInf(float64(b[i]), 0) {
			t.Fatalf("non-finite sample %d", i)
		}
		if a[i] != b[i] {
			diff = true
		}
	}
	if !diff {
		t.Fatal("formant stage enabled but output identical")
	}
}

// Different vowels must produce different spectra (coarse: different bytes).
func TestFormantStage_VowelsDiffer(t *testing.T) {
	mk := func(vowel float64) RecipeParams {
		return RecipeParams{"osc_type": 1, "osc_enabled": 1,
			"formant_enabled": 1, "formant_mix": 0.9, "formant_vowel": vowel, "formant_voice_type": 3}
	}
	a := renderModularSeed(t, mk(0), 0.5)
	u := renderModularSeed(t, mk(4), 0.5)
	same := true
	for i := range a {
		if a[i] != u[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("vowel a and vowel u rendered identically")
	}
}

// Breath: noise-only whisper (osc off, breath 1) must be non-silent and finite.
func TestFormantStage_WhisperNonSilent(t *testing.T) {
	w := RecipeParams{"osc_enabled": 0,
		"formant_enabled": 1, "formant_mix": 1, "formant_breath": 1, "formant_vowel": 2}
	b := renderModularSeed(t, w, 0.5)
	peak := float32(0)
	for _, s := range b {
		if s > peak {
			peak = s
		}
		if math.IsNaN(float64(s)) {
			t.Fatal("NaN in whisper render")
		}
	}
	if peak < 0.01 {
		t.Fatalf("whisper render is silent (peak %v)", peak)
	}
}
```

Before writing, open `src/go/internal/audio/modular_golden_test.go` and reuse its render helper (adapt the name `renderModularParamsForTest` to whatever exists — the test must go through the same recipe render path the goldens use).

- [ ] **Step 2: Run to verify it fails**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && $GO test ./internal/audio/ -run 'TestFormantStage' -v
```
Expected: FAIL — `TestFormantStage_OnChangesOutputFinite` and later ("output identical": params exist but nothing reads them). `_OffIsExactBypass` should already PASS.

- [ ] **Step 3: Implement the vowel tables + `modular_formant_process` in modular_stages.c**

Declare in `src/c/modular_stages.h`:

```c
/* Phase-15 FORMANT stage: parallel 5-band vowel bandpass bank + singer's
 * formant. Runs between the static filter and the body resonator. Exact no-op
 * when mix <= 0 (caller also gates on formant_enabled). */
void modular_formant_process(float *out, int sampleRate, int samples,
                             int voice_type, float vowel, float mix,
                             float shift, float sing,
                             float morph_rate, float morph_to);
```

Implement in `src/c/modular_stages.c` (bottom of file). Data: Csound Appendix D (spec § Formant tables — copy ALL numbers from the spec, they are reproduced there in full):

```c
/* ── Phase-15 FORMANT stage ────────────────────────────────────────────────
 * Csound Appendix D formant tables: [voice][vowel][formant] for voices
 * soprano/alto/tenor/bass and vowels a/e/i/o/u. Amplitudes are dB relative
 * to F1; bandwidths are -6 dB widths (Q = Fc/BW). */
static const float form_hz[4][5][5] = {
    {{800,1150,2900,3900,4950},{350,2000,2800,3600,4950},{270,2140,2950,3900,4950},
     {450,800,2830,3800,4950},{325,700,2700,3800,4950}},   /* soprano */
    {{800,1150,2800,3500,4950},{400,1600,2700,3300,4950},{350,1700,2700,3700,4950},
     {450,800,2830,3500,4950},{325,700,2530,3500,4950}},   /* alto */
    {{650,1080,2650,2900,3250},{400,1700,2600,3200,3580},{290,1870,2800,3250,3540},
     {400,800,2600,2800,3000},{350,600,2700,2900,3300}},   /* tenor */
    {{600,1040,2250,2450,2750},{400,1620,2400,2800,3100},{250,1750,2600,3050,3340},
     {400,750,2400,2600,2900},{350,600,2400,2675,2950}},   /* bass */
};
static const float form_db[4][5][5] = {
    {{0,-6,-32,-20,-50},{0,-20,-15,-40,-56},{0,-12,-26,-26,-44},{0,-11,-22,-22,-50},{0,-16,-35,-40,-60}},
    {{0,-4,-20,-36,-60},{0,-24,-30,-35,-60},{0,-20,-30,-36,-60},{0,-9,-16,-28,-55},{0,-12,-30,-40,-64}},
    {{0,-6,-7,-8,-22},{0,-14,-12,-14,-20},{0,-15,-18,-20,-30},{0,-10,-12,-12,-26},{0,-20,-17,-14,-26}},
    {{0,-7,-9,-9,-20},{0,-12,-9,-12,-18},{0,-30,-16,-22,-28},{0,-11,-21,-20,-40},{0,-20,-32,-28,-36}},
};
static const float form_bw[4][5][5] = {
    {{80,90,120,130,140},{60,100,120,150,200},{60,90,100,120,120},{40,80,100,120,120},{50,60,170,180,200}},
    {{80,90,120,130,140},{60,80,120,150,200},{50,100,120,150,200},{70,80,100,130,135},{50,60,170,180,200}},
    {{80,90,120,130,140},{70,80,100,120,120},{40,90,100,120,120},{70,80,100,130,135},{40,60,100,120,120}},
    {{60,70,110,120,130},{40,80,100,120,120},{60,90,100,120,120},{40,80,100,120,120},{40,80,100,120,120}},
};
/* Singer's-formant center per voice type (Sundberg). */
static const float form_sing_hz[4] = {3092.0f, 3000.0f, 2750.0f, 2400.0f};

/* Interpolate the 5-band (Fc, linear gain, Q) set at a continuous vowel
 * position v in [0,4], scaled by shift. init!=0 uses mod_biquad_set (fresh
 * state); init==0 uses mod_biquad_set_coeffs (state-preserving — the same
 * block-rate morph pattern as the filter-env sweep in modular.c). */
static void formant_bank_tune(mod_biquad *bank, float *g, int voice_type,
                              float v, float shift, int sampleRate, int init) {
    if (v < 0.0f) v = 0.0f; else if (v > 4.0f) v = 4.0f;
    int   i0 = (int)v; if (i0 > 3) i0 = 3;
    float fr = v - (float)i0;
    for (int k = 0; k < 5; k++) {
        double hz = ((1.0f - fr) * form_hz[voice_type][i0][k] + fr * form_hz[voice_type][i0 + 1][k])
                  * (double)shift;
        double db = (1.0f - fr) * form_db[voice_type][i0][k] + fr * form_db[voice_type][i0 + 1][k];
        double bw = ((1.0f - fr) * form_bw[voice_type][i0][k] + fr * form_bw[voice_type][i0 + 1][k])
                  * (double)shift;
        double nyq = 0.49 * (double)sampleRate;
        if (hz > nyq) hz = nyq;
        double q = hz / bw; if (q < 0.5) q = 0.5;
        g[k] = (float)pow(10.0, db / 20.0);
        if (init) mod_biquad_set(&bank[k], 2 /* band-pass */, hz, q, sampleRate);
        else      mod_biquad_set_coeffs(&bank[k], 2, hz, q, sampleRate);
    }
}

void modular_formant_process(float *out, int sampleRate, int samples,
                             int voice_type, float vowel, float mix,
                             float shift, float sing,
                             float morph_rate, float morph_to) {
    if (mix <= 0.0f || samples <= 0) return;
    if (voice_type < 0) voice_type = 0; else if (voice_type > 3) voice_type = 3;
    if (shift <= 0.0f) shift = 1.0f;
    mod_biquad bank[5];
    float g[5];
    formant_bank_tune(bank, g, voice_type, vowel, shift, sampleRate, 1);
    mod_biquad singbq;
    if (sing > 0.0f) {
        double shz = (double)form_sing_hz[voice_type] * (double)shift;
        double nyq = 0.49 * (double)sampleRate;
        if (shz > nyq) shz = nyq;
        mod_biquad_set(&singbq, 2, shz, 10.0, sampleRate);
    }
    int morph_on = morph_rate > 0.0f;
    /* Suppress the dry as mix grows (same shaping rationale as the body
     * resonator's dryAmt — the harmonics should move THROUGH the formants,
     * not sit under them). dryAmt(0)=1 → exact bypass at mix 0. */
    double dryAmt = 1.0 - 0.85 * fmin((double)mix, 1.0);
    for (int i = 0; i < samples; i++) {
        if (morph_on && (i & 63) == 0) {
            double t = (double)i / (double)sampleRate;
            double m = 0.5 - 0.5 * cos(2.0 * M_PI * (double)morph_rate * t);
            float v = (float)((1.0 - m) * (double)vowel + m * (double)morph_to);
            formant_bank_tune(bank, g, voice_type, v, shift, sampleRate, 0);
        }
        double x = (double)out[i];
        double wet = 0.0;
        for (int k = 0; k < 5; k++)
            wet += (double)mod_biquad_tick_f(&bank[k], (float)x) * (double)g[k];
        if (sing > 0.0f)
            wet += (double)mod_biquad_tick_f(&singbq, (float)x) * (double)sing * 0.7;
        out[i] = (float)(x * dryAmt + wet * (double)mix);
    }
}
```

(Adapt `mod_biquad_set_coeffs`'s exact name/signature to what modular_stages.h declares — it is the state-preserving setter the LFO cutoff sweep uses at modular.c:768.)

- [ ] **Step 4: Wire the stage + breath into `render_modular_p`**

In `src/c/modular.c`:

(a) Param reads, after the `bow_dynamics` read (:469):

```c
    /* Phase-15 formant stage. enabled/mix gate everything; vowel/voice_type/
     * morph_to read raw (0 = a valid enum value); shift falls back to 1.0. */
    int   formant_on   = mp_get(p, 0.0f, p->formant_enabled, have) >= 0.5f;
    float formant_vowel = p->formant_vowel;
    int   formant_vt   = (int)lrintf(p->formant_voice_type);
    float formant_mix  = mp_get(p, 0.0f, p->formant_mix,   have);
    float formant_shift = mp_get(p, 1.0f, p->formant_shift, have);
    float formant_breath = mp_get(p, 0.0f, p->formant_breath, have);
    float formant_sing = mp_get(p, 0.0f, p->formant_sing,  have);
    float formant_mrate = mp_get(p, 0.0f, p->formant_morph_rate, have);
    float formant_mto  = p->formant_morph_to;
```

(b) Breath injection — AFTER the gen-bank call (:683), BEFORE the LFO stage (:690), so the noise is shaped by the amp ADSR and colored by the filter + formant bank downstream:

```c
    /* ── Phase-15 breath: white noise mixed into the source PRE-envelope so the
     * ADSR shapes it and the filter + formant bank color it. Whisper = osc off
     * + breath 1. Gated on the formant stage (it is that card's knob). ── */
    if (formant_on && formant_breath > 0.0f) {
        noise_gen bng;
        noise_init(&bng, noise_seed ^ 0x9E3779B9u);
        for (int i = 0; i < samples; i++)
            out[i] += noise_white_tick(&bng) * formant_breath * 0.5f;
    }
```

(c) Stage call — AFTER the filter stage's closing brace (:804 `/* filter disabled → passthrough. */`), BEFORE the body-resonator block (:807):

```c
    /* ── Phase-15 FORMANT stage (vowel bank) — before the body resonator so
     * FORMANT (mouth) and RESONATOR (instrument body) compose in that order. */
    if (formant_on) {
        modular_formant_process(out, sampleRate, samples, formant_vt,
                                formant_vowel, formant_mix, formant_shift,
                                formant_sing, formant_mrate, formant_mto);
    }
```

- [ ] **Step 5: Rebuild, run the new tests + byte identity + extreme-max finiteness**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && $GO test ./internal/audio/ -run 'TestFormantStage' -v
cd src/go && $GO test ./internal/audio/ -run 'TestSynthGoldenByteIdentity|TestCompleteness_RecipeRenderPresetsHaveFiniteOutput' 2>&1 | tail -5
```
Expected: all PASS. The extreme-max preset now runs the formant stage at Max everything — it must stay finite (the bank is a linear filter; the dry/wet sum is bounded).

---

### Task 6: ENSEMBLE humanization DSP (scatter + per-voice vibrato + humanize)

**Files:**
- Modify: `src/c/modular.c` (`modular_source_unison` from Task 1)
- Test: `src/go/internal/audio/modular_ensemble_test.go` (new, `//go:build !test && !js`)

**Interfaces:**
- Consumes: Task 4's `ens_*` fields; Task 1's `modular_source_unison(..., const modular_params *p, int have, ...)`.
- Determinism rule: per-voice randomness comes ONLY from an LCG seeded by `(noise_seed, voice index)` — never rand()/time. Same seed ⇒ same bytes (xplat parity depends on it).

- [ ] **Step 1: Write the failing test**

```go
//go:build !test && !js

package audio

import "testing"

// All ens_* at identity must leave the unison render byte-identical.
func TestEnsemble_IdentityIsExactBypass(t *testing.T) {
	base := RecipeParams{"osc_type": 1, "osc_enabled": 1, "unison_voices": 4, "unison_detune": 10}
	withZeros := RecipeParams{"osc_type": 1, "osc_enabled": 1, "unison_voices": 4, "unison_detune": 10,
		"ens_scatter": 0, "ens_vib_rate": 0, "ens_vib_depth": 0, "ens_humanize": 0}
	a := renderModularSeed(t, base, 0.5)
	b := renderModularSeed(t, withZeros, 0.5)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs at ens identity", i)
		}
	}
}

// Scatter and vibrato must change the output; two renders with the same seed
// must be identical (determinism).
func TestEnsemble_HumanizeChangesOutputDeterministically(t *testing.T) {
	hum := RecipeParams{"osc_type": 1, "osc_enabled": 1, "unison_voices": 5, "unison_detune": 8,
		"ens_scatter": 10, "ens_vib_rate": 5.5, "ens_vib_depth": 8, "ens_humanize": 0.8}
	base := RecipeParams{"osc_type": 1, "osc_enabled": 1, "unison_voices": 5, "unison_detune": 8}
	a := renderModularSeed(t, base, 0.5)
	b1 := renderModularSeed(t, hum, 0.5)
	b2 := renderModularSeed(t, hum, 0.5)
	diff := false
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Fatalf("non-deterministic ensemble render at sample %d", i)
		}
		if a[i] != b1[i] {
			diff = true
		}
	}
	if !diff {
		t.Fatal("ens params set but output identical to base")
	}
}
```

- [ ] **Step 2: Run to verify the second test fails**

```bash
cd src/go && $GO test ./internal/audio/ -run 'TestEnsemble' -v
```
Expected: `_IdentityIsExactBypass` PASS, `_HumanizeChangesOutputDeterministically` FAIL ("output identical to base").

- [ ] **Step 3: Implement inside `modular_source_unison`**

Read the ens params at the top of the function (`p`/`have` are already parameters from Task 1):

```c
    float ens_scatter = mp_get(p, 0.0f, p->ens_scatter,  have);
    float ens_vr      = mp_get(p, 0.0f, p->ens_vib_rate,  have);
    float ens_vd      = mp_get(p, 0.0f, p->ens_vib_depth, have);
    float ens_hum     = mp_get(p, 0.0f, p->ens_humanize,  have);
    int   ens_vib_on  = (ens_vr > 0.0f && ens_vd > 0.0f);
```

Deterministic per-voice values (voice 0 = center, 1..ns = sides), computed once before the render loop:

```c
    /* Deterministic per-voice randoms: LCG seeded by (noise_seed, voice).
     * lcg_norm ∈ [-1,1], lcg_unit ∈ [0,1). Never rand()/time — byte-identity
     * and xplat parity depend on renders being pure functions of the seed. */
    double scat_cents[7] = {0}, vib_rate_v[7] = {0}, vib_phase_v[7] = {0};
    if (ens_scatter > 0.0f || ens_vib_on) {
        for (int v = 0; v < unison_nv; v++) {
            unsigned int r = (noise_seed + 1u) * 2654435761u + (unsigned int)(v + 1) * 40503u;
            r = r * 1664525u + 1013904223u;
            double n1 = ((double)(r >> 8) / (double)(1u << 24)) * 2.0 - 1.0;
            r = r * 1664525u + 1013904223u;
            double n2 = ((double)(r >> 8) / (double)(1u << 24)) * 2.0 - 1.0;
            r = r * 1664525u + 1013904223u;
            double u3 = (double)(r >> 8) / (double)(1u << 24);
            /* Center voice keeps exact pitch center: scatter applies to sides only. */
            scat_cents[v] = (v == 0) ? 0.0 : (double)ens_scatter * n1;
            /* humanize spreads rate ±15% and randomizes phase; at 0 all voices
             * share rate ens_vr and phase 0 (one synchronized singer). */
            vib_rate_v[v]  = (double)ens_vr * (1.0 + 0.15 * (double)ens_hum * n2);
            vib_phase_v[v] = 2.0 * M_PI * (double)ens_hum * u3;
        }
    }
```

Extend the per-sample frequency-update path: the existing loop only calls `wt_osc_set_freq` on side voices when `drift_on`. Generalize the condition to `int mod_on = drift_on || ens_scatter > 0.0f || ens_vib_on;` and inside the per-sample update compute, for side voice v (1-based side index maps to voice v+1 in the tables):

```c
    double cents = drift_base_cents[v]                                  /* static spread (existing) */
                 + (drift_on ? drift_cents : 0.0)                        /* existing drift term */
                 + scat_cents[v + 1]                                     /* Phase-15 scatter */
                 + (ens_vib_on ? (double)ens_vd
                       * sin(2.0 * M_PI * vib_rate_v[v + 1] * t + vib_phase_v[v + 1]) : 0.0);
    wt_osc_set_freq(&side[v], freq * pow(2.0, cents / 1200.0), sampleRate);
```

The CENTER voice also gets vibrato (a choir's every singer vibrates; scatter stays 0 so the section stays centered):

```c
    if (ens_vib_on) {
        double c0 = (double)ens_vd * sin(2.0 * M_PI * vib_rate_v[0] * t + vib_phase_v[0]);
        wt_osc_set_freq(&osc, freq * pow(2.0, c0 / 1200.0), sampleRate);
    }
```

CRITICAL identity check: when all ens_* are 0, `mod_on == drift_on`, `scat_cents` all 0, no center set_freq call, and the cents expression reduces to the pre-change formula — byte-identical. Do NOT call `wt_osc_set_freq` when `mod_on` is false (the call itself may perturb phase state).

- [ ] **Step 4: Rebuild + run**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && $GO test ./internal/audio/ -run 'TestEnsemble|TestSynthGoldenByteIdentity' -v 2>&1 | tail -10
```
Expected: PASS all (goldens prove the identity path).

---

### Task 7: Glottal-pulse oscillator (osc_type 12)

**Files:**
- Modify: `src/c/wavetable.h` + `src/c/wavetable.c` (add `wt_generate_glottal`)
- Modify: `src/c/modular.c` (table statics :30–53: new buf/wt, ensure/table_for entries)
- Modify: `src/go/internal/audio/modular_recipe.go:20-21` (osc_type Max 11→12, Enum + "Voice")
- Test: `src/go/internal/audio/modular_glottal_test.go` (new); update `src/go/internal/audio/modular_saveas_test.go:39,71` (12→13); `modular_recipe_test.go:98` enum-count map if it lists osc_type

**Interfaces:**
- Produces: `void wt_generate_glottal(wavetable_t *wt, float *buf, int length, int harmonics);` — band-limited all-harmonic table with 1/n² amplitude (the Rosenberg −12 dB/oct glottal slope; a saw is 1/n = −6 dB/oct). osc_type 12 flows through ALL wavetable branches (plain / unison / pitch-env+vibrato) automatically, so glottal + ensemble + formant compose with zero extra dispatch.

- [ ] **Step 1: Write the failing test**

```go
//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// osc_type 12 (glottal) must render non-silent, finite, and differ from saw.
func TestGlottalOsc_RendersDistinctFinite(t *testing.T) {
	glottal := RecipeParams{"osc_type": 12, "osc_enabled": 1}
	saw := RecipeParams{"osc_type": 1, "osc_enabled": 1}
	g := renderModularSeed(t, glottal, 0.5)
	s := renderModularSeed(t, saw, 0.5)
	peak, diff := float32(0), false
	for i := range g {
		if math.IsNaN(float64(g[i])) || math.IsInf(float64(g[i]), 0) {
			t.Fatalf("non-finite glottal sample %d", i)
		}
		if a := float32(math.Abs(float64(g[i]))); a > peak {
			peak = a
		}
		if g[i] != s[i] {
			diff = true
		}
	}
	if peak < 0.01 {
		t.Fatal("glottal render silent")
	}
	if !diff {
		t.Fatal("glottal identical to saw — table not wired")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd src/go && $GO test ./internal/audio/ -run 'TestGlottalOsc' -v
```
Expected: FAIL — osc_type 12 currently falls to `modular_table_for`'s `default:` (sine)… which means it renders a SINE, non-silent but "identical to saw" is false too — the failure will be on neither branch; if it passes accidentally (sine ≠ saw and finite), tighten the test: also assert `g` differs from a sine render (`osc_type: 0`). Add that third comparison now if needed so the test genuinely fails pre-implementation.

- [ ] **Step 3: Implement the table**

`src/c/wavetable.h` — declare next to `wt_generate_saw`:

```c
/* Band-limited glottal-pulse approximation: all harmonics at 1/n^2 amplitude
 * (−12 dB/oct — the Rosenberg glottal source slope; saw's 1/n is −6 dB/oct).
 * The classic vocal source for formant synthesis. */
void wt_generate_glottal(wavetable_t *wt, float *buf, int length, int harmonics);
```

`src/c/wavetable.c` — mirror `wt_generate_saw`'s body exactly (same normalization pass), changing the per-harmonic amplitude from `1.0/n` to `1.0/(n*n)`.

`src/c/modular.c` — extend the statics (:30–34), `ensure_modular_tables` (:37–44), and `modular_table_for` (:46–53):

```c
static float       modular_glottal_buf[WT_DEFAULT_LENGTH + 1];
static wavetable_t modular_glottal_wt;
/* in ensure_modular_tables(), before modular_tables_ready = 1: */
    wt_generate_glottal(&modular_glottal_wt, modular_glottal_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
/* in modular_table_for(): */
    case 12: return &modular_glottal_wt;
```

- [ ] **Step 4: Bump the enum ceiling (the SIGSEGV trap)**

`src/go/internal/audio/modular_recipe.go:20-21`:

```go
		{Name: "osc_type", Label: "Oscillator", Group: "osc", Min: 0, Max: 12, Default: 0,
			Enum: []string{"Sine", "Saw", "Square", "Triangle", "FM", "Noise White", "Noise Pink", "Bowed String", "Brass", "Reed", "Flute", "Sax", "Voice"}},
```

osc_type Max == highest IMPLEMENTED model (12 = wavetable branch, inherently finite) — the extreme-max completeness preset will now render osc_type 12, which dispatches to the glottal wavetable through the standard branches. Update the pinned enum counts:
- `modular_saveas_test.go:39`: `len(oscDef.Enum) != 12` → `!= 13` (and `Enum[1] != "Saw"` stays).
- `modular_saveas_test.go:71`: `len(d.Enum) != 12` → `!= 13`.
- `modular_recipe_test.go:98` enum-count map: if it has an `osc_type` entry, bump to 13.

- [ ] **Step 5: Rebuild + full gate**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && $GO test ./internal/audio/ -run 'TestGlottalOsc|TestSynthGoldenByteIdentity|TestCompleteness|SaveAs|ModularRecipe' -v 2>&1 | tail -15
```
Expected: PASS all.

---

### Task 8: UI stage cards — ENSEMBLE, FORMANT, RESONATOR sections + i18n

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (section consts :42-79; `unifiedSynthSectionOrder` :106; `sectionLabel` :175; `sectionLabelKey` :207; `sectionSubtitleKey` :281; `enableToggleSection` :326; `modularStageGroupSection` :359)
- Modify: `src/go/internal/audio/modular_recipe.go` (body_model/body_mix/bow_dynamics Group `SynthHiddenGroup` → `"resonator"` :366-368; formant Group if parked hidden in Task 4)
- Modify: `src/go/internal/i18n/keys.go` (~:391), `catalog_en.go` (~:363), `catalog_es.go` (~:366)
- Modify: `src/go/internal/ui/plain_english.go` (glosses for new labels)
- Test: update `src/go/internal/ui/synth_unified_sections_test.go`, `synth_stage_i18n_test.go`, `synth_modular_toggle_test.go`, `synth_panel_zone_test.go` expectations

**Interfaces:**
- Consumes: Task 4's Groups (`formant`, `unison`) and param names.
- Produces: sections `synthSectionEnsemble`, `synthSectionFormant`, `synthSectionResonator`; final card order `VOICE · OSC · ENSEMBLE · FM · PITCH · LFO · BURST · ENVELOPE · FILTER · FILTER ENV · FORMANT · RESONATOR · POST`.
- ENSEMBLE and RESONATOR have NO enable pill (their off states are `unison_voices==1` / `body_model==0` — the "Off" enum entry); FORMANT's pill is `formant_enabled`.

- [ ] **Step 1: Write the failing test (extend the section-order test)**

In `src/go/internal/ui/synth_unified_sections_test.go`, find the test that asserts the modular recipe's section order and update its expected slice to include the three new sections in the order above. Also add:

```go
// Phase-15: the ENSEMBLE/FORMANT/RESONATOR cards route their groups.
func TestVoiceStagesRouting(t *testing.T) {
	if sec, ok := modularStageGroupSection("unison"); !ok || sec != synthSectionEnsemble {
		t.Fatalf("unison group must route to ENSEMBLE, got %v ok=%v", sec, ok)
	}
	if sec, ok := modularStageGroupSection("formant"); !ok || sec != synthSectionFormant {
		t.Fatalf("formant group must route to FORMANT, got %v ok=%v", sec, ok)
	}
	if sec, ok := modularStageGroupSection("resonator"); !ok || sec != synthSectionResonator {
		t.Fatalf("resonator group must route to RESONATOR, got %v ok=%v", sec, ok)
	}
	if sec, ok := enableToggleSection("formant_enabled"); !ok || sec != synthSectionFormant {
		t.Fatalf("formant_enabled pill must own FORMANT, got %v ok=%v", sec, ok)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd src/go && $GO test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestVoiceStagesRouting|UnifiedSections' -v 2>&1 | tail -10
```
Expected: FAIL (undefined `synthSectionEnsemble` etc. — compile error counts as the failing state).

- [ ] **Step 3: Implement the sections**

(a) Append consts after `synthSectionKick` (:78):

```go
	// Phase-15 voice/choir stages. ENSEMBLE hosts the unison knobs (moved out
	// of OSC) + the humanized-vibrato knobs; no enable pill (voices==1 is its
	// off state). FORMANT is the vowel bank (formant_enabled pill). RESONATOR
	// surfaces the body-resonator bank (body_model "Off" is its off state).
	synthSectionEnsemble
	synthSectionFormant
	synthSectionResonator
```

(b) `unifiedSynthSectionOrder` (:106) — insert `synthSectionEnsemble` after `synthSectionOsc`; insert `synthSectionFormant, synthSectionResonator` after `synthSectionFilterEnv`:

```go
var unifiedSynthSectionOrder = []synthSectionID{
	synthSectionVoice,
	synthSectionOsc,
	synthSectionEnsemble,
	synthSectionFM,
	synthSectionPitch,
	synthSectionLFO,
	synthSectionBurst,
	synthSectionEnvelope,
	synthSectionFilter,
	synthSectionFilterEnv,
	synthSectionFormant,
	synthSectionResonator,
	synthSectionPost,
}
```

(c) `sectionLabel` (:175): add cases returning `"ENSEMBLE"`, `"FORMANT"`, `"RESONATOR"`.
(d) `sectionLabelKey` (:207) + `sectionSubtitleKey` (:281): add cases returning the new keys from step 4.
(e) `enableToggleSection` (:326): add `case "formant_enabled": return synthSectionFormant, true`.
(f) `modularStageGroupSection` (:359): change `case "unison"` to return `synthSectionEnsemble, true` (delete the "lives in OSC" comment); add `case "formant": return synthSectionFormant, true` and `case "resonator": return synthSectionResonator, true`.

(g) In `src/go/internal/audio/modular_recipe.go` (:366-368) surface the resonator knobs:

```go
		ParamDef{Name: "body_model", Label: "Body Model", Group: "resonator", Min: 0, Max: 4, Default: 0, Enum: []string{"Off", "Violin", "Guitar", "Cello", "Steel Guitar"}},
		ParamDef{Name: "body_mix", Label: "Body Mix", Group: "resonator", Min: 0, Max: 1.5, Default: 0},
		ParamDef{Name: "bow_dynamics", Label: "Bow Dynamics", Group: "resonator", Min: 0, Max: 1, Default: 0},
```

If Task 4 parked the formant defs in `SynthHiddenGroup`, flip them to `"formant"` now.

- [ ] **Step 4: i18n keys + catalogs**

`src/go/internal/i18n/keys.go` (next to `KeySynthStageBurst`, :391):

```go
	KeySynthStageEnsemble     Key = "synth.stage.ensemble"
	KeySynthStageFormant      Key = "synth.stage.formant"
	KeySynthStageResonator    Key = "synth.stage.resonator"
	KeySynthStageEnsembleSub  Key = "synth.stage.ensemble.sub"
	KeySynthStageFormantSub   Key = "synth.stage.formant.sub"
	KeySynthStageResonatorSub Key = "synth.stage.resonator.sub"
```

`catalog_en.go` (:363 area):

```go
	KeySynthStageEnsemble:     "ENSEMBLE",
	KeySynthStageFormant:      "FORMANT",
	KeySynthStageResonator:    "RESONATOR",
	KeySynthStageEnsembleSub:  "one singer or a whole group",
	KeySynthStageFormantSub:   "the mouth shape of the sound",
	KeySynthStageResonatorSub: "the instrument's wooden body",
```

`catalog_es.go` (:366 area):

```go
	KeySynthStageEnsemble:     "CONJUNTO",
	KeySynthStageFormant:      "FORMANTE",
	KeySynthStageResonator:    "RESONADOR",
	KeySynthStageEnsembleSub:  "una voz o un grupo entero",
	KeySynthStageFormantSub:   "la forma de la boca del sonido",
	KeySynthStageResonatorSub: "el cuerpo de madera del instrumento",
```

If an i18n completeness test pins the key count, update it.

- [ ] **Step 5: PlainEnglish glosses**

In `src/go/internal/ui/plain_english.go`, add entries to the gloss map (match the existing map's exact shape/casing conventions — read the file first):

```go
	"Vowel":         "ah, eh, ee, oh or oo",
	"Voice Type":    "high voice or low voice",
	"Breath":        "airy whisper in the sound",
	"Shine":         "the bright singer sparkle",
	"Head Size":     "small head or big head",
	"Morph Speed":   "how fast the mouth moves",
	"Morph To":      "the mouth shape it moves to",
	"Scatter":       "how spread apart the singers are",
	"Vibrato Rate":  "how fast the voices wobble",
	"Vibrato Depth": "how much the voices wobble",
	"Humanize":      "make each singer a little different",
	"Body Model":    "which instrument body it rings in",
	"Body Mix":      "how much the body colors the sound",
	"Bow Dynamics":  "the bow pressing harder and softer",
```

- [ ] **Step 6: Run the UI net and fix every pinned expectation**

```bash
cd src/go && $GO test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -20
```
Expected failures to fix by updating expectations (each with a one-line rationale comment): `synth_unified_sections_test.go` (order), `synth_stage_i18n_test.go` (new sections need catalog entries — already added), `synth_modular_toggle_test.go` (formant pill), `synth_panel_zone_test.go` (section counts). Re-run until PASS. Then re-run the audio fast net:

```bash
cd src/go && $GO test -tags test -modfile=go.test.mod ./internal/audio/... 2>&1 | tail -5
```
Expected: PASS.

---

### Task 9: Voice category

**Files:**
- Modify: `src/go/internal/audio/category.go` (:20-35 consts, :40-46 order, :155-188 text rules)
- Test: extend the existing category test file (grep `-l "CategoryOf" internal/audio/*_test.go`)

**Interfaces:**
- Produces: `CatVoice CategoryID = "voice"`, display "Voice", placed between Brass and Lead/Synth. Text rules MUST precede the `"bass"` case (else `voice-bass` → CatBass).

- [ ] **Step 1: Write the failing test**

In the existing category test file:

```go
func TestVoiceCategoryClassification(t *testing.T) {
	for _, id := range []string{"choir-ahh", "choir-ooh", "voice-soprano", "voice-bass", "voice-pad", "voice-whisper"} {
		if got := CategoryOf(id); got != CatVoice {
			t.Fatalf("CategoryOf(%q) = %q, want %q", id, got, CatVoice)
		}
	}
	// The trap: "voice-bass" contains "bass" — the voice rule must win.
	if got := CategoryOf("voice-bass"); got == CatBass {
		t.Fatal("voice-bass fell through to the bass substring rule")
	}
	// And plain bass ids must still classify as bass.
	if got := CategoryOf("bass-acid"); got != CatBass {
		t.Fatalf("bass-acid = %q, want %q", got, CatBass)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd src/go && $GO test -tags test -modfile=go.test.mod ./internal/audio/ -run 'TestVoiceCategory' -v
```
Expected: FAIL (`CatVoice` undefined → compile error is the failing state).

- [ ] **Step 3: Implement**

(a) Const (:34, before `CatOther`): `CatVoice CategoryID = "voice"`.
(b) Order (:44-45): `{CatBrass, "Brass"}, {CatVoice, "Voice"}, {CatLead, "Lead/Synth"}, ...`.
(c) Text rule — insert BEFORE the `"bass"` case (:172), with the ordering comment style the switch already uses:

```go
	// Voice/choir before "bass"/"string"/"lead": "voice-bass" must hit Voice,
	// not the bass substring rule.
	case strings.Contains(lc, "choir"), strings.Contains(lc, "vocal"), strings.Contains(lc, "voice"),
		strings.Contains(lc, "soprano"), strings.Contains(lc, "whisper"):
		return CatVoice, true
```

(d) Category display-name i18n: grep how the categories menu localizes names (`grep -rn "Percussion\|Woodwind" src/go/internal/i18n/catalog_es.go`). If category labels have catalog entries, add "Voice" → EN `"Voice"` / ES `"Voz"` following the exact existing key pattern; if they are keyed by CategoryID, mirror that.

- [ ] **Step 4: Run**

```bash
cd src/go && $GO test -tags test -modfile=go.test.mod ./internal/audio/ -run 'Category' -v 2>&1 | tail -10
```
Expected: PASS (fix any pinned category-count test with a one-line rationale).

---

### Task 10: The six voice instruments (seeds + table rows + regens)

**Files:**
- Modify: `src/go/internal/audio/instrument_seeds.go` (six new seeds at the end)
- Modify: `src/go/internal/audio/modular_instruments.go` (six rows appended to `modularInstrumentDefs` :110)
- Modify: `src/go/internal/audio/melodic_recipes.go` (five entries in `melodicRecipeIDs` :15 — NOT whisper)
- Modify: `src/go/internal/audio/synth_golden_pinned_test.go` (six new golden entries)
- Regen: `src/js/modular_instruments.gen.js` (`make gen-modular-instruments`), `instrument_ids_gen.go` (`go generate`), `instrument_loudness_gen.go` (measure-loudness)

**Interfaces:**
- Consumes: everything from Tasks 4–9.
- Produces: instrument ids `choir-ahh, choir-ooh, voice-soprano, voice-bass, voice-pad, voice-whisper`; recipe ids `synth-modular-<id>`.

- [ ] **Step 1: Write the failing check (the discipline tests ARE the test)**

```bash
cd src/go && $GO test ./internal/audio/ -run 'TestSynthGoldenByteIdentity' -v 2>&1 | tail -3
```
Expected currently PASS; after adding table rows (step 3) it will FAIL with "missing golden for choir-ahh" — that failure→fix cycle is this task's TDD loop.

- [ ] **Step 2: Add the seeds to `instrument_seeds.go`**

```go
// ── Phase-15 voice/choir family. Source–filter vocal synthesis: glottal pulse
// (osc_type 12) or detuned saws → formant vowel bank (Csound Appendix D
// tables) + humanized ensemble. Values are research-derived starting points
// (spec 2026-07-08); final values are set by the EAR-TUNING pass — the user's
// ear is the acceptance gate, not these numbers.

// choirAhhSeed: mixed "ahh" choir — 5 humanized glottal voices, tenor "a"
// formants, moderate singer's formant, soft 100 ms attack.
var choirAhhSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1,
	"unison_voices": 5, "unison_detune": 8, "unison_mix": 0.7,
	"unison_drift_rate": 0.3, "unison_drift_depth": 4,
	"ens_scatter": 10, "ens_vib_rate": 5.5, "ens_vib_depth": 8, "ens_humanize": 0.8,
	"filter_type": 0, "filter_cutoff": 6000, "filter_resonance": 0.5,
	"amp_attack": 0.10, "amp_decay": 0.3, "amp_sustain": 0.85, "amp_release": 0.5,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 2,
	"formant_mix": 0.85, "formant_breath": 0.06, "formant_sing": 0.35,
	"gain": 0.55, // not 0.5 — TestPhase2Native mutates gain->0.5 and needs a delta
}

// choirOohSeed: darker sustained "ooh" pad — alto "u" vowel, more breath,
// slower attack, long release.
var choirOohSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1,
	"unison_voices": 5, "unison_detune": 7, "unison_mix": 0.75,
	"unison_drift_rate": 0.25, "unison_drift_depth": 4,
	"ens_scatter": 9, "ens_vib_rate": 5.0, "ens_vib_depth": 7, "ens_humanize": 0.85,
	"filter_type": 0, "filter_cutoff": 4200, "filter_resonance": 0.5,
	"amp_attack": 0.15, "amp_decay": 0.4, "amp_sustain": 0.9, "amp_release": 1.0,
	"formant_enabled": 1, "formant_vowel": 4, "formant_voice_type": 1,
	"formant_mix": 0.85, "formant_breath": 0.12, "formant_sing": 0.2,
	"gain": 0.55,
}

// voiceSopranoSeed: solo soprano — single glottal voice, expressive ±60¢
// vibrato after a 0.3 s onset, onset scoop via a small negative pitch env,
// bright 3.1 kHz singer's formant.
var voiceSopranoSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1, "osc_octave": 1,
	"unison_voices": 1,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 5.5, "lfo_depth": 0.6, "lfo_delay": 0.3,
	"pitchenv_enabled": 1, "pitchenv_amt": -0.5, "pitchenv_decay": 0.09,
	"filter_type": 0, "filter_cutoff": 8000, "filter_resonance": 0.5,
	"amp_attack": 0.06, "amp_decay": 0.2, "amp_sustain": 0.9, "amp_release": 0.4,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 0,
	"formant_mix": 0.9, "formant_breath": 0.05, "formant_sing": 0.5,
	"gain": 0.6,
}

// voiceBassSeed: solo bass singer — glottal an octave down, bass "o" vowel,
// restrained ±30¢ vibrato, low 2.4 kHz singer's formant.
var voiceBassSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1, "osc_octave": -1,
	"unison_voices": 1,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 5.0, "lfo_depth": 0.3, "lfo_delay": 0.35,
	"pitchenv_enabled": 1, "pitchenv_amt": -0.4, "pitchenv_decay": 0.1,
	"filter_type": 0, "filter_cutoff": 5000, "filter_resonance": 0.5,
	"amp_attack": 0.09, "amp_decay": 0.25, "amp_sustain": 0.9, "amp_release": 0.45,
	"formant_enabled": 1, "formant_vowel": 3, "formant_voice_type": 3,
	"formant_mix": 0.9, "formant_breath": 0.05, "formant_sing": 0.3,
	"gain": 0.6,
}

// voicePadSeed: lush vowel-morphing pad — 7 detuned SAWS (deliberately synthy),
// slow ah↔oh morph, wide humanized ensemble.
var voicePadSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1,
	"unison_voices": 7, "unison_detune": 12, "unison_mix": 0.85,
	"unison_drift_rate": 0.4, "unison_drift_depth": 6,
	"ens_scatter": 8, "ens_vib_rate": 5.0, "ens_vib_depth": 6, "ens_humanize": 0.6,
	"filter_type": 0, "filter_cutoff": 4500, "filter_resonance": 0.6,
	"amp_attack": 0.25, "amp_decay": 0.5, "amp_sustain": 0.9, "amp_release": 1.2,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 1,
	"formant_mix": 0.9, "formant_morph_rate": 0.15, "formant_morph_to": 3,
	"formant_breath": 0.04, "formant_sing": 0.15,
	"gain": 0.55,
}

// voiceWhisperSeed: unvoiced texture — NO pitch source (osc off), pure breath
// noise through "ee" formants. Unpitched: NOT in melodicRecipeIDs.
var voiceWhisperSeed = RecipeParams{
	"osc_enabled": 0,
	"formant_enabled": 1, "formant_vowel": 2, "formant_voice_type": 1,
	"formant_mix": 1.0, "formant_breath": 1.0,
	"filter_type": 0, "filter_cutoff": 9000, "filter_resonance": 0.5,
	"amp_attack": 0.08, "amp_decay": 0.3, "amp_sustain": 0.9, "amp_release": 0.4,
	"gain": 0.6,
}
```

- [ ] **Step 3: Append the table rows**

At the end of `modularInstrumentDefs` (:110):

```go
	// ── Phase-15 voice/choir family (Voice category). Sustained vocal
	// instruments — same Beats/Duration shape as organ/sax. Amplitude 0.8 is
	// the fallback; the real level comes from the measure-loudness regen. ──
	{ID: "choir-ahh", RecipeID: "synth-modular-choir-ahh", Display: "Ahh Choir", Category: modularRecipeCategory, Seed: choirAhhSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "choir-ooh", RecipeID: "synth-modular-choir-ooh", Display: "Ooh Choir", Category: modularRecipeCategory, Seed: choirOohSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "voice-soprano", RecipeID: "synth-modular-voice-soprano", Display: "Soprano", Category: modularRecipeCategory, Seed: voiceSopranoSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "voice-bass", RecipeID: "synth-modular-voice-bass", Display: "Bass Singer", Category: modularRecipeCategory, Seed: voiceBassSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "voice-pad", RecipeID: "synth-modular-voice-pad", Display: "Vowel Pad", Category: modularRecipeCategory, Seed: voicePadSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "voice-whisper", RecipeID: "synth-modular-voice-whisper", Display: "Whisper", Category: modularRecipeCategory, Seed: voiceWhisperSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
```

Add the five PITCHED recipes to `melodicRecipeIDs` (:15) — whisper is unpitched noise, leave it out:

```go
	// Voice/choir family — pitched vocal instruments (whisper is unpitched).
	"synth-modular-choir-ahh":     true,
	"synth-modular-choir-ooh":     true,
	"synth-modular-voice-soprano": true,
	"synth-modular-voice-bass":    true,
	"synth-modular-voice-pad":     true,
```

- [ ] **Step 4: Regenerate everything**

```bash
cd /home/ymolinar/Repos/beatmo && make gen-modular-instruments
cd src/go && $GO generate ./internal/audio/
```
Then loudness (read `src/go/cmd/measure-loudness/main.go`'s usage header first for the exact invocation — it regenerates `instrument_loudness_gen.go`):

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && AUDIO_SAMPLE_RATE=48000 $GO run ./cmd/measure-loudness   # adapt flags per its --help/usage
```

- [ ] **Step 5: Pin the six new goldens**

```bash
cd src/go && $GO test ./internal/audio/ -run 'TestSynthGoldenIdentityCapture' -v 2>&1 | grep GOLDEN | grep -E 'choir|voice-'
```
Add the six printed `{n, bits}` entries to `synthGoldens` in `synth_golden_pinned_test.go` (alphabetical position, comment `// Phase-15 voice family — pinned at initial seed values; re-pin after ear-tuning.`). Then:

```bash
cd src/go && $GO test ./internal/audio/ -run 'TestSynthGoldenByteIdentity|TestCompleteness|Recipe|Wired|Melodic' 2>&1 | tail -10
```
Expected: PASS (all 48 goldens; discipline tests see the new descriptors/bindings/configs derived from the table).

- [ ] **Step 6: RTF + JS drift + full nets**

```bash
cd src/go && $GO test ./internal/audio/ -run 'TestP5_RenderRealTimeFactor' -v 2>&1 | tail -3
cd src/go && $GO test ./internal/audio/ -run 'RenderTableJSSrc|JSSrc' -v 2>&1 | tail -3
cd src/go && $GO test ./internal/audio/ 2>&1 | tail -3
cd src/go && $GO test -tags test -modfile=go.test.mod ./... 2>&1 | tail -5
```
Expected: PASS (RTF ≥ 20× aggregate — if the formant instruments drag it under, check that coefficient updates are block-rate, not per-sample).

---

### Task 11: Audition WAVs + end-to-end verification

**Files:**
- Create (scratch, not committed): `tmp/voice_audition/*.wav`

- [ ] **Step 1: Render audition WAVs for the ear gate**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && $GO build -o ../../tmp/synth-analyze ./cmd/synth-analyze
cd /home/ymolinar/Repos/beatmo && mkdir -p tmp/voice_audition
for inst in choir-ahh choir-ooh voice-soprano voice-bass voice-pad voice-whisper; do
  AUDIO_SAMPLE_RATE=48000 tmp/synth-analyze synth -inst "$inst" -pitch 0 -dur 3 -out "tmp/voice_audition/${inst}.wav"
done
ls -la tmp/voice_audition/
```
Expected: six non-trivially-sized WAVs. Also render a short pitch ladder for the choirs (`-pitch -5`, `-pitch 3`) to hear formants staying put while pitch moves (the whole point of the formant bank).

- [ ] **Step 2: Sanity-analyze (metrics guide, ear decides)**

```bash
for f in tmp/voice_audition/*.wav; do tmp/synth-analyze ref -file "$f" -json | head -c 400; echo; done
```
Check: non-zero RMS on all six; soprano/bass F0 roughly where expected (soprano ~440 Hz at `osc_octave:1`, bass ~110 Hz at `-1`); whisper has no stable F0 (expected).

- [ ] **Step 3: Browser bridge + parity smoke**

```bash
cd /home/ymolinar/Repos/beatmo && make wasm
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js
```
Expected: PASS (the generated `modular_instruments.gen.js` carries the six new RENDER/RENDER_INFO entries; no hand-written JS changed, so the deploy manifest is untouched — confirm with `cd src/go && $GO test -tags test -modfile=go.test.mod ./internal/deploy/ 2>&1 | tail -3`).

- [ ] **Step 4: Full verification sweep**

```bash
cd src/go && $GO test -tags test -modfile=go.test.mod ./... 2>&1 | tail -5
cd src/go && $GO test ./internal/audio/ 2>&1 | tail -5
```
Expected: PASS everywhere (modulo the branch's pre-existing failures recorded in Task 1). Then STOP and report to the user: the six audition WAVs in `tmp/voice_audition/` are ready for the ear gate. Do NOT ear-tune autonomously — per the tuning guide, the user's ear is the acceptance gate; expect a tuning round after they listen (re-pin the six goldens + re-run measure-loudness after any seed change).

---

## Self-review notes (already applied)

- **Spec coverage:** stage splits (Tasks 1–3, 8), FORMANT DSP (5), ENSEMBLE (6), glottal (7), ABI (4), category (9), instruments + regens (10), audition + verification (11). Spec's "onset scoop" uses the existing pitchenv stage (negative amt) — no new mechanism, matching the spec.
- **Deviation from spec, deliberate:** `formant_voice_type` default is 0/Soprano (not 2/Tenor) because mp_get treats 0 as "unset" — enums where 0 is a valid choice are read raw with identity 0 (the osc_type pattern). Harmless: the stage is off by default; seeds set it explicitly.
- **Type consistency:** `modular_formant_process` signature identical in Task 5 header/impl/call; `renderModularSeed` helper shared by Tasks 5–7 tests (defined once in modular_formant_test.go); section consts match between Task 8 steps.
- **The kick/cymbal/snare split (Tasks 2–3) intentionally has no new tests** — the 48-golden byte-identity net is the strongest possible test for a byte-identical refactor.
