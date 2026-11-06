# Engine: LFO Routing (vibrato / cutoff sweep) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `lfo_target` parameter to the modular synth voice so the existing LFO can drive **amplitude tremolo (target 0, unchanged), pitch vibrato (target 1), or filter-cutoff sweep (target 2)** — the foundation for realistic bowed strings and winds.

**Architecture:** One new scalar param threaded end-to-end (C struct → Go struct → both marshalling directions → flat schema tail → recipe ParamDef → UI stage group). The post-mix amp-LFO stage is gated to `target==0`; vibrato injects into the existing wavetable per-sample frequency loop (where pitch-env already lives); cutoff sweep injects into the filter stage via a new state-preserving biquad coeff setter. Every change is identity-gated so all existing instruments render bit-for-bit unchanged.

**Tech Stack:** C (miniaudio-style DSP, CGo), Go 1.23, bundled toolchain at `.tools/go/bin/go`.

## Global Constraints

- **Byte-identity invariant (load-bearing):** `lfo_target` defaults to `0`; with `lfo_target==0` every existing instrument MUST render bit-for-bit identical to before this change. This is asserted by the existing modular byte-identity tests — they must stay green at every checkpoint.
- **Append at the flat tail:** add the new scalar param at the END of the modulator-stage list (`synth_param_schema.go` `modularModulatorStages`), the END of the C `modular_params` struct, and the END of the Go `ModularParams` struct + both marshalling blocks. Inserting mid-struct/mid-list shifts the WASM flat param-block indices and breaks the browser bridge.
- **Bundled Go only:** `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`.
- **Engine tests use the REAL CGo build** (the modular render files are `//go:build !test && !js`). Run them as: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run <name> -v`. Do NOT add `-tags test` / `-modfile=go.test.mod` for these — that path stubs out the C renderer.
- **Do NOT `git add` or `git commit`** — the user reviews and commits all work themselves (standing project rule). Every "Checkpoint" step means *run the listed tests and pause for review*, leaving the work uncommitted.
- **LFO depth units:** target 1 (pitch) interprets `lfo_depth` as ± semitones (0.30 → ±0.30 st = ±30 cents); target 2 (cutoff) as ± octaves (0.5 → ±0.5 oct). Target 0 (amp) keeps the existing 0..1 dip meaning.
- **Vibrato scope:** target 1 modulates ONLY the wavetable OSC path (`osc_type` 0–3). FM (`osc_type==4`) and noise (`5/6`) are unaffected by design.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `src/c/modular.h` | `modular_params` struct | Add `float lfo_target;` at struct tail |
| `src/c/modular.c` | `render_modular_p` voice | Gate amp-LFO on target 0; vibrato in wavetable loop (target 1); cutoff sweep in filter stage (target 2) |
| `src/c/modular_stages.h` | biquad API | Declare `mod_biquad_set_coeffs` |
| `src/c/modular_stages.c` | biquad impl | Add state-preserving coeff-only setter |
| `src/go/internal/audio/modular_c.go` | Go↔C marshalling | Add `LfoTarget` field + both directions |
| `src/go/internal/audio/synth_param_schema.go` | flat param schema | Append `{"lfo_target", 0}` at modulator-stage tail |
| `src/go/internal/audio/modular_recipe.go` | recipe ParamDefs | Add `lfo_target` ParamDef (Group "lfo", enum 0..2) |
| `src/go/internal/audio/modular_stage_params.go` | UI stage grouping | Add `lfo_target` to the LFO numeric stage |
| `src/go/internal/audio/modular_render_test.go` | engine tests | New behavior tests (vibrato, cutoff, byte-identity) |

---

### Task 1: Plumb `lfo_target` end-to-end and gate the amp-LFO on target 0

Adds the parameter through every layer with **no audible change** (amp-LFO now reads `target==0`, which is the default, so behavior is preserved), proven by a byte-identity test.

**Files:**
- Modify: `src/c/modular.h` (struct tail, ~line 367 before closing `}`)
- Modify: `src/c/modular.c` (read `lfo_target` near line 169; gate amp-LFO at line 270)
- Modify: `src/go/internal/audio/modular_c.go` (struct ~line 226; `toCModularParams` ~line 306; `fromRecipeParams` get-block ~line 484)
- Modify: `src/go/internal/audio/synth_param_schema.go` (modulator-stage list, line 287, after `{"burst4_off", 0}, {"burst4_amp", 0},`)
- Modify: `src/go/internal/audio/modular_recipe.go` (after the `lfo_depth` ParamDef, ~line 299)
- Modify: `src/go/internal/audio/modular_stage_params.go` (LFO stage, line 69)
- Test: `src/go/internal/audio/modular_render_test.go`

**Interfaces:**
- Produces: C struct field `lfo_target`; Go field `ModularParams.LfoTarget float64`; RecipeParams key `"lfo_target"` (identity 0.0); ParamDef `lfo_target` (Min 0, Max 2, Default 0, Group "lfo").
- Consumes: existing `renderModularP(buf []float32, sr, n int, p ModularParams)`, `defaultModularParams()`, `recipeParamsToModular(map[string]float64)` test/runtime seams.

- [ ] **Step 1: Write the failing test** (round-trip + byte-identity)

Add to `src/go/internal/audio/modular_render_test.go`:

```go
func TestLfoTarget_RoundTripsThroughParams(t *testing.T) {
	mp := recipeParamsToModular(map[string]float64{"lfo_target": 2})
	if mp.LfoTarget != 2 {
		t.Fatalf("LfoTarget round-trip = %v, want 2", mp.LfoTarget)
	}
}

func TestLfoTarget_DefaultZeroKeepsAmpLFOByteIdentical(t *testing.T) {
	const sr, n = 48000, 24000
	// An instrument using the amp-wobble LFO at the default target (0).
	base := defaultModularParams()
	base.OscType = 1 // saw
	base.LfoEnabled = 1
	base.LfoRate = 6
	base.LfoDepth = 0.5
	// LfoTarget defaults to 0 (amp). Render it.
	withTarget := base
	withTarget.LfoTarget = 0
	a := make([]float32, n)
	b := make([]float32, n)
	renderModularP(a, sr, n, base)       // field present, zero value
	renderModularP(b, sr, n, withTarget) // explicit 0
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("amp-LFO at target 0 not byte-identical at sample %d: %v vs %v", i, a[i], b[i])
		}
	}
	// And it must actually wobble (sanity: not silent / not flat).
	var mn, mx float32 = a[0], a[0]
	for _, v := range a {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	if mx-mn < 0.01 {
		t.Fatalf("amp-LFO produced near-flat output (range %v)", mx-mn)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'TestLfoTarget_RoundTripsThroughParams|TestLfoTarget_DefaultZeroKeepsAmpLFOByteIdentical' -v`
Expected: FAIL — `mp.LfoTarget` undefined (compile error), because the field does not exist yet.

- [ ] **Step 3: Add the C struct field**

In `src/c/modular.h`, change the struct tail (currently ending with `burst4_amp;` then `} modular_params;`):

```c
    float burst4_off;
    float burst4_amp;
    /* LFO routing target: 0 = amplitude tremolo (default, identity), 1 = pitch
     * vibrato (wavetable OSC path), 2 = filter-cutoff sweep. Appended at the
     * struct tail so the WASM flat param-block indices stay frozen. */
    float lfo_target;
} modular_params;
```

- [ ] **Step 4: Read and gate `lfo_target` in `render_modular_p`**

In `src/c/modular.c`, after the `lfo_depth` read (line 169) add:

```c
    int   lfo_target = (int)lrintf(mp_get(p, 0.0f, p->lfo_target, have));
```

Then change the post-mix amp-LFO guard (line 270) from:

```c
    if (lfo_on && lfo_depth > 0.0f && lfo_rate > 0.0f) {
```

to (amp wobble ONLY when target selects amplitude):

```c
    if (lfo_on && lfo_target == 0 && lfo_depth > 0.0f && lfo_rate > 0.0f) {
```

- [ ] **Step 5: Add the Go marshalling**

In `src/go/internal/audio/modular_c.go`:
- After `Burst4Amp float64` (line 226) add to the `ModularParams` struct:

```go
	Burst4Amp float64
	// LFO routing target: 0 = amp (default/identity), 1 = pitch vibrato, 2 = cutoff sweep.
	LfoTarget float64
```

- In `toCModularParams`, after `burst4_amp: C.float(p.Burst4Amp),` (line 306):

```go
		burst4_amp: C.float(p.Burst4Amp),
		lfo_target: C.float(p.LfoTarget),
```

- In `fromRecipeParams` (the `get(...)` block), after `Burst4Amp: get("burst4_amp"),` (line 484):

```go
		Burst4Amp: get("burst4_amp"),
		LfoTarget: get("lfo_target"),
```

- [ ] **Step 6: Append to the flat schema tail**

In `src/go/internal/audio/synth_param_schema.go`, change the modulator-stage list (lines 283–287) to append `lfo_target` LAST:

```go
		{"burst_enabled", 0}, {"burst_sharp", 0},
		{"burst1_off", 0}, {"burst1_amp", 0},
		{"burst2_off", 0}, {"burst2_amp", 0},
		{"burst3_off", 0}, {"burst3_amp", 0},
		{"burst4_off", 0}, {"burst4_amp", 0},
		{"lfo_target", 0},
```

- [ ] **Step 7: Add the recipe ParamDef**

In `src/go/internal/audio/modular_recipe.go`, immediately after the `lfo_depth` ParamDef (line 299) add:

```go
		ParamDef{Name: "lfo_depth", Label: "Depth", Group: "lfo", Min: 0, Max: 1, Default: 0.5},
		ParamDef{Name: "lfo_target", Label: "LFO Target", Group: "lfo", Min: 0, Max: 2, Default: 0,
			Enum: []string{"Amp", "Pitch", "Cutoff"}},
```

> If `ParamDef` has no `Enum` field in this codebase, drop the `Enum: …` clause — the
> Min/Max/Default 0..2 integer range is sufficient; the value is `lrintf`-rounded in C.
> Check the `ParamDef` struct definition in `synth_recipe.go` before writing this line and
> match its exact fields.

- [ ] **Step 8: Add `lfo_target` to the LFO UI stage group**

In `src/go/internal/audio/modular_stage_params.go`, line 69, extend the LFO stage's numeric list:

```go
	{numeric: []string{"lfo_rate", "lfo_depth", "lfo_target"}, toggle: "lfo_enabled"},
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'TestLfoTarget_RoundTripsThroughParams|TestLfoTarget_DefaultZeroKeepsAmpLFOByteIdentical' -v`
Expected: PASS both.

- [ ] **Step 10: Checkpoint — full modular byte-identity + schema suite (do NOT commit)**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'Modular|Schema|ByteIdent|Recipe' -v`
Expected: PASS (all existing instruments still byte-identical; schema length grew by exactly 1). Pause for user review.

---

### Task 2: LFO → pitch vibrato on the wavetable OSC path (target 1)

**Files:**
- Modify: `src/c/modular.c` (the wavetable OSC branch, lines 232–257)
- Test: `src/go/internal/audio/modular_render_test.go`

**Interfaces:**
- Consumes: `lfo_target`, `lfo_on`, `lfo_rate`, `lfo_depth` (Task 1); existing `wt_osc_init` / `wt_osc_set_freq` / `wt_osc_tick`.
- Produces: pitch vibrato on `osc_type` 0–3 when `lfo_enabled==1 && lfo_target==1`.

- [ ] **Step 1: Write the failing test**

```go
func TestLfoTarget_PitchVibratoBendsAndIsIdentityAtDepthZero(t *testing.T) {
	const sr, n = 48000, 48000
	mk := func(target, depth float64) []float32 {
		p := defaultModularParams()
		p.OscType = 0 // sine — easy to measure pitch via zero crossings
		p.FilterEnabled = 0
		p.EnvEnabled = 0
		p.LfoEnabled = 1
		p.LfoRate = 6
		p.LfoDepth = depth
		p.LfoTarget = target
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		return buf
	}
	plain := mk(1, 0)      // vibrato selected but depth 0 → must equal no-LFO baseline
	base := defaultModularParams()
	base.OscType = 0
	base.FilterEnabled = 0
	base.EnvEnabled = 0
	baseBuf := make([]float32, n)
	renderModularP(baseBuf, sr, n, base)
	for i := range plain {
		if plain[i] != baseBuf[i] {
			t.Fatalf("vibrato at depth 0 not identity at %d: %v vs %v", i, plain[i], baseBuf[i])
		}
	}
	vib := mk(1, 0.5) // ±0.5 st vibrato → output must differ from baseline
	diff := 0
	for i := range vib {
		if vib[i] != baseBuf[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("vibrato at depth 0.5 produced no change vs baseline")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestLfoTarget_PitchVibratoBendsAndIsIdentityAtDepthZero -v`
Expected: FAIL — `vib` equals baseline (`diff == 0`), because vibrato is not implemented yet.

- [ ] **Step 3: Implement vibrato in the wavetable branch**

In `src/c/modular.c`, replace the two wavetable branches (the `else if (pe_on && ...)` block at lines 232–248 and the trailing `else { plain loop }` at 249–257) with a single combined-modulation loop. Add a vibrato flag just above the OSC stage (near line 196, after `int osc_wrote = 0;`):

```c
    int vib_on = lfo_on && lfo_target == 1 && lfo_depth > 0.0f && lfo_rate > 0.0f;
```

Then replace lines 232–257 with:

```c
    } else if ((pe_on && pe_amt != 0.0f && pe_decay > 0.0f) || vib_on) {
        /* Wavetable OSC with per-sample frequency modulation: pitch-env
         * (exp semitone sweep) and/or LFO vibrato (sinusoidal semitone wobble),
         * summed in semitones then applied as a 2^(st/12) multiplier. When
         * vib_on is false this is bit-identical to the legacy pitch-env branch;
         * when both are off control never reaches here (the plain branch below
         * runs), preserving byte-identity for every pre-change instrument. */
        ensure_modular_tables();
        wt_osc_t osc;
        wt_osc_init(&osc, modular_table_for(osc_type), freq, sampleRate);
        int pe_active = pe_on && pe_amt != 0.0f && pe_decay > 0.0f;
        for (int i = 0; i < samples; i++) {
            double t = (double)i / (double)sampleRate;
            double semis = 0.0;
            if (pe_active) semis += (double)pe_amt * exp(-t / (double)pe_decay);
            if (vib_on)    semis += (double)lfo_depth
                                  * sin(2.0 * M_PI * (double)lfo_rate * t);
            double mult = pow(2.0, semis / 12.0);
            wt_osc_set_freq(&osc, freq * mult, sampleRate);
            out[i] = wt_osc_tick(&osc);
        }
        osc_wrote = 1;
    } else {
        ensure_modular_tables();
        wt_osc_t osc;
        wt_osc_init(&osc, modular_table_for(osc_type), freq, sampleRate);
        for (int i = 0; i < samples; i++) {
            out[i] = wt_osc_tick(&osc);
        }
        osc_wrote = 1;
    }
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestLfoTarget_PitchVibratoBendsAndIsIdentityAtDepthZero -v`
Expected: PASS.

- [ ] **Step 5: Checkpoint — byte-identity for all existing instruments (do NOT commit)**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'Modular|ByteIdent|Recipe|OscShapesDiffer' -v`
Expected: PASS (target-0 instruments unchanged; the legacy pitch-env branch still byte-identical because `vib_on` is false for them). Pause for review.

---

### Task 3: State-preserving biquad coeff setter (`mod_biquad_set_coeffs`)

`mod_biquad_set` zeroes the filter delay line (`x1=x2=y1=y2=0`) on every call, so it cannot be used for per-block cutoff modulation (it would click). Add a coeff-only variant.

**Files:**
- Modify: `src/c/modular_stages.h` (declare, after line 17)
- Modify: `src/c/modular_stages.c` (implement, near `mod_biquad_set`, line 22–65)
- Test: `src/go/internal/audio/modular_render_test.go` (indirect — exercised by Task 4; add a direct C-behavior test here)

**Interfaces:**
- Produces: `void mod_biquad_set_coeffs(mod_biquad *f, int type, double cutoff, double q, int sr);` — identical coefficient math to `mod_biquad_set` but does NOT touch `x1/x2/y1/y2`.

- [ ] **Step 1: Write the failing test**

Add a test that a continuous filtered tone with coeff-only updates has no discontinuity that a state-zeroing update would introduce. Because this is C-internal, assert it via the Task-4-style path: render a filtered saw with `lfo_target==2` and confirm finiteness + no NaN (full behavioral assertion lands in Task 4). Add the minimal guard here:

```go
func TestModBiquadCoeffOnly_FilterStaysFinite(t *testing.T) {
	const sr, n = 48000, 24000
	p := defaultModularParams()
	p.OscType = 1 // saw
	p.FilterEnabled = 1
	p.FilterType = 0 // low-pass
	p.FilterCutoff = 2000
	p.LfoEnabled = 1
	p.LfoRate = 5
	p.LfoDepth = 0.5
	p.LfoTarget = 2 // cutoff sweep — drives the coeff-only path (Task 4)
	buf := make([]float32, n)
	renderModularP(buf, sr, n, p)
	for i, v := range buf {
		if v != v || v > 4 || v < -4 {
			t.Fatalf("non-finite/exploded filter output at %d: %v", i, v)
		}
	}
}
```

> This test will only meaningfully exercise the coeff setter once Task 4 wires it in. It
> is placed here so the setter has a home and compiles; it PASSES trivially until Task 4
> activates the `lfo_target==2` path, then guards stability.

- [ ] **Step 2: Run to verify current state**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestModBiquadCoeffOnly_FilterStaysFinite -v`
Expected: PASS trivially (lfo_target==2 not yet handled in the filter stage → behaves as a static filter; output finite). This baseline guards against regressions when Task 4 lands.

- [ ] **Step 3: Declare the coeff-only setter**

In `src/c/modular_stages.h`, after line 17 (`void mod_biquad_set(...)`):

```c
void mod_biquad_set(mod_biquad *f, int type, double cutoff, double q, int sr);
/* Update coefficients WITHOUT resetting the delay line (x1/x2/y1/y2). For
 * per-block cutoff modulation where zeroing state would click. */
void mod_biquad_set_coeffs(mod_biquad *f, int type, double cutoff, double q, int sr);
```

- [ ] **Step 4: Implement it**

In `src/c/modular_stages.c`, refactor so both functions share the math. Replace the body of `mod_biquad_set` (lines 22–65) so it delegates:

```c
void mod_biquad_set_coeffs(mod_biquad *f, int type, double cutoff, double q, int sr) {
    if (cutoff < 20.0) cutoff = 20.0;
    double nyq = 0.5 * (double)sr;
    if (cutoff > nyq * 0.99) cutoff = nyq * 0.99;
    if (q < 0.25) q = 0.25;

    double w0 = 2.0 * M_PI * cutoff / (double)sr;
    double cw = cos(w0), sw = sin(w0);
    double alpha = sw / (2.0 * q);
    double a0, b0, b1, b2, a1, a2;

    switch (type) {
    case 1: /* high-pass */
        b0 = (1.0 + cw) / 2.0; b1 = -(1.0 + cw); b2 = (1.0 + cw) / 2.0;
        a0 = 1.0 + alpha; a1 = -2.0 * cw; a2 = 1.0 - alpha;
        break;
    case 2: /* band-pass (constant 0 dB peak) */
        b0 = alpha; b1 = 0.0; b2 = -alpha;
        a0 = 1.0 + alpha; a1 = -2.0 * cw; a2 = 1.0 - alpha;
        break;
    default: /* low-pass */
        b0 = (1.0 - cw) / 2.0; b1 = 1.0 - cw; b2 = (1.0 - cw) / 2.0;
        a0 = 1.0 + alpha; a1 = -2.0 * cw; a2 = 1.0 - alpha;
        break;
    }
    f->b0 = (float)(b0 / a0);
    f->b1 = (float)(b1 / a0);
    f->b2 = (float)(b2 / a0);
    f->a1 = (float)(a1 / a0);
    f->a2 = (float)(a2 / a0);
}

void mod_biquad_set(mod_biquad *f, int type, double cutoff, double q, int sr) {
    mod_biquad_set_coeffs(f, type, cutoff, q, sr);
    f->x1 = f->x2 = f->y1 = f->y2 = 0.0f;
}
```

> This keeps `mod_biquad_set` byte-identical (same coeff math, still zeroes state) — so
> every existing filter user is unchanged — while exposing the state-preserving variant.

- [ ] **Step 5: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'TestModBiquadCoeffOnly_FilterStaysFinite|Biquad|Filter' -v`
Expected: PASS.

- [ ] **Step 6: Checkpoint — full audio suite (do NOT commit)**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -v`
Expected: PASS. Pause for review.

---

### Task 4: LFO → cutoff sweep on the filter stage (target 2)

**Files:**
- Modify: `src/c/modular.c` (filter stage, lines 328–334)
- Test: `src/go/internal/audio/modular_render_test.go`

**Interfaces:**
- Consumes: `lfo_target`, `lfo_on`, `lfo_rate`, `lfo_depth` (Task 1); `mod_biquad_set` + `mod_biquad_set_coeffs` (Task 3).
- Produces: cutoff sweep on the FILTER stage when `lfo_enabled==1 && lfo_target==2`.

- [ ] **Step 1: Write the failing test**

```go
func TestLfoTarget_CutoffSweepChangesFilteredOutput(t *testing.T) {
	const sr, n = 48000, 48000
	mkFiltered := func(target float64) []float32 {
		p := defaultModularParams()
		p.OscType = 1 // saw (harmonically rich → cutoff changes are audible)
		p.EnvEnabled = 0
		p.FilterEnabled = 1
		p.FilterType = 0 // low-pass
		p.FilterCutoff = 2000
		p.LfoEnabled = 1
		p.LfoRate = 4
		p.LfoDepth = 0.6
		p.LfoTarget = target
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		return buf
	}
	staticCut := mkFiltered(0) // amp target → filter cutoff is static
	sweptCut := mkFiltered(2)  // cutoff target → filter cutoff sweeps
	diff := 0
	for i := range staticCut {
		if sweptCut[i] != staticCut[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("cutoff sweep (target 2) produced no change vs static-cutoff filter")
	}
	for i, v := range sweptCut {
		if v != v || v > 4 || v < -4 {
			t.Fatalf("cutoff sweep non-finite at %d: %v", i, v)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestLfoTarget_CutoffSweepChangesFilteredOutput -v`
Expected: FAIL — `diff == 0` (target 2 not yet handled; filter is static).

- [ ] **Step 3: Implement the cutoff sweep**

In `src/c/modular.c`, replace the filter stage (lines 328–334) with:

```c
    /* ── Filter stage ── */
    if (filt_on) {
        mod_biquad filt;
        int cut_sweep = lfo_on && lfo_target == 2 && lfo_depth > 0.0f && lfo_rate > 0.0f;
        if (!cut_sweep) {
            mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
            for (int i = 0; i < samples; i++) {
                out[i] = mod_biquad_tick_f(&filt, out[i]);
            }
        } else {
            /* LFO cutoff sweep: ± lfo_depth octaves around the base cutoff,
             * recomputed every 64 samples with the state-preserving coeff
             * setter (no click). Target 0/1 take the static path above, so this
             * branch is reached ONLY for cutoff-target instruments — every
             * pre-change instrument is byte-identical. */
            mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
            for (int i = 0; i < samples; i++) {
                if ((i & 63) == 0) {
                    double t = (double)i / (double)sampleRate;
                    double oct = (double)lfo_depth
                               * sin(2.0 * M_PI * (double)lfo_rate * t);
                    double c = (double)cutoff * pow(2.0, oct);
                    mod_biquad_set_coeffs(&filt, filt_type, c, (double)q, sampleRate);
                }
                out[i] = mod_biquad_tick_f(&filt, out[i]);
            }
        }
    }
    /* filter disabled → passthrough. */
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'TestLfoTarget_CutoffSweepChangesFilteredOutput|TestModBiquadCoeffOnly_FilterStaysFinite' -v`
Expected: PASS both.

- [ ] **Step 5: Checkpoint — full audio suite (do NOT commit)**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -v`
Expected: PASS (all three LFO targets work; every target-0 instrument byte-identical). Pause for review.

---

### Task 5: WASM build + cross-platform parity verification

Confirms the new param flows through the browser bridge and renders identically native↔WASM.

**Files:**
- No source changes expected (the param is generic). If the bridge smoke catalogue or a flat-schema length assertion fails, fix the indicated file.

**Interfaces:**
- Consumes: everything from Tasks 1–4.

- [ ] **Step 1: Build the WASM module**

Run: `make wasm`
Expected: builds `src/js/main.wasm` with the updated modular schema (no C symbol added, so `EXPORTED_FUNCTIONS` is unchanged).

- [ ] **Step 2: Run the bridge smoke (canonical export catalogue)**

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: PASS — `render_modular` / `updateInstrumentParams` still callable; no new top-level export required (the param rides the existing modular param block).

- [ ] **Step 3: Run cross-platform audio parity**

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js`
Expected: PASS — WASM render matches native for the existing goldens (byte-identity holds across the bridge because target-0 is unchanged).

- [ ] **Step 4: Spot-check a vibrato instrument in the browser (manual/optional)**

Build/run the browser app, register a scratch instrument with `recipe: "synth-modular"` and
`synth_params: {"osc_type":1,"lfo_enabled":1,"lfo_rate":6,"lfo_depth":0.3,"lfo_target":1}`,
play a sustained note, and confirm audible vibrato. (This is the first real audition of the
feature; not a gating automated test.)

- [ ] **Step 5: Checkpoint — leave uncommitted for user review**

Run the engine suite once more: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -v`
Expected: PASS. Pause for the user to review the whole LFO-routing change before it is committed by them.

---

## Self-Review

**Spec coverage (§3 of the design):**
- `lfo_target` param with amp/pitch/cutoff — Tasks 1/2/4. ✓
- Pitch vibrato on the wavetable OSC path, ±semitones = `lfo_depth` — Task 2. ✓
- Cutoff sweep ±octaves, per-block, state-preserving — Tasks 3+4. ✓
- Byte-identity invariant (target 0 unchanged) — asserted in every task's checkpoint + dedicated test in Task 1. ✓
- Flat-tail append (WASM index freeze) — Task 1 Steps 3/5/6; verified Task 5. ✓
- Amp LFO unchanged when target 0 — Task 1 Step 4 + test. ✓

**Placeholder scan:** no "TBD"/"handle edge cases"; every code step shows full code; the one
conditional (`ParamDef.Enum`) carries an explicit "check the struct, drop the clause if
absent" instruction. ✓

**Type consistency:** `LfoTarget float64` (Go) ↔ `lfo_target` (C float) ↔ `"lfo_target"`
(RecipeParams key) ↔ ParamDef `Name: "lfo_target"` — consistent across Tasks 1–4.
`mod_biquad_set_coeffs` signature identical in declaration (Task 3 Step 3) and call site
(Task 4 Step 3). ✓

**Out of scope (correctly deferred to v2 per the design):** FM-path vibrato; attack-brightening
filter envelope; LFO shape selection / onset delay.
