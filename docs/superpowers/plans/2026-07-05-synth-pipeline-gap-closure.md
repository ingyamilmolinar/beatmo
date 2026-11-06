# Config-Declarative Synth Pipeline — Gap Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close every gap found in the 2026-07-05 plan-vs-implementation review of the config-declarative synth pipeline (spec: `docs/superpowers/specs/2026-07-04-config-declarative-synth-pipeline-design.md`): green the red tier guard, add the missing P5 performance + composability guards, complete the sp_* DSP primitive library byte-identically, make the declarative config the enforced source of truth for drum-gen defaults, and migrate the remaining 34 seeded-modular instruments into `modularInstrumentDefs`.

**Architecture:** Every change is gated by the existing golden byte-identity net (`TestSynthGoldenByteIdentity`, 83 pinned checksums). C refactors extract *only* arithmetic whose op order is preserved exactly (the proven `sp_osc`/`sp_dcblock` pattern). Config source-of-truth is achieved with declarative Go tables + drift-guard tests that parse the C source (the repo's `design_md_drift_test.go` idiom) — NOT by pushing values through the float32 param ABI, which would silently change double-precision values (`(double)float32(1.12) != 1.12`) and break byte-identity.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), CGo + C (miniaudio DSP, `src/c/`), existing native test path (`!test && !js` build tags).

## Global Constraints

- **NEVER `git add` / `git commit` / `git stash` / `rm` — leave all work uncommitted.** (Standing user rule; overrides this skill's commit steps. Every "Commit" step in the canonical task shape is replaced by a "Verify golden net" step.)
- **Byte-identity is non-negotiable:** after EVERY C edit and EVERY registration migration batch, run the golden net. If it fails, the change is wrong — revert the edit by hand (re-apply the previous text with Edit; never `git checkout`/`git revert`).
- Golden net command (native, from `src/go`):
  `../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity' ./internal/audio/`
  Expected: `ok ... internal/audio` (~1s).
- Fast-path test command (from `src/go`):
  `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 <pkg> -run <name>`
- Native audio test command (from `src/go`):
  `../../.tools/go/bin/go test -count=1 ./internal/audio/ -run <name>`
- All Go work uses `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`. All paths below are relative to `/home/ymolinar/Repos/beatmo` unless absolute.
- Do NOT touch unrelated working-tree scope (loudness normalization, kick tuning, design-review UI fixes are separate in-flight efforts).
- The full `internal/ui -tags test` suite is a known 500s+ flaky run on this branch — verify UI-adjacent changes with targeted `-run` filters only.
- C files (`src/c/*.c`, `src/c/*.h`) compile through CGo automatically on the next `go test` — no separate build step needed natively. WASM verification happens once, in the final task.

---

### Task 1: Green the tier guard (`TestP4_TierClassification`)

The guard fails: 41 params carry `TierAdvanced` against a hard cap of 40. The 10 contextual physical-model knobs (`osc_sax_*`, `osc_bow_*`) are hidden unless the matching oscillator is selected (`synthOscModelParamHidden`), so they never crowd a beginner's default view — counting them against the "beginner sees mostly Essential" budget is a category error.

**Files:**
- Modify: `src/go/internal/audio/param_tier_test.go:40-53`

**Interfaces:**
- Consumes: `ModularSynthParamDefs()`, `TierAdvanced` (existing).
- Produces: nothing new — test-only change.

- [ ] **Step 1: Confirm the current failure**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestP4_TierClassification' ./internal/audio/`
Expected: FAIL with `Advanced set too large (41)`.

- [ ] **Step 2: Exclude contextual osc-model params from the cap count**

In `src/go/internal/audio/param_tier_test.go`, replace the counting block (lines 40-52):

```go
	// Sanity: the Advanced set is small (a beginner still sees mostly Essential).
	// Contextual physical-model knobs (osc_sax_*/osc_bow_*) are excluded from the
	// cap: they are hidden unless the matching oscillator is selected
	// (synthOscModelParamHidden), so they never crowd the default view.
	adv := 0
	for _, d := range defs {
		if d.Tier != TierAdvanced {
			continue
		}
		if strings.HasPrefix(d.Name, "osc_sax_") || strings.HasPrefix(d.Name, "osc_bow_") {
			continue
		}
		adv++
	}
	if adv == 0 {
		t.Error("no Advanced params — tiering classification did not apply")
	}
	if adv > 40 {
		t.Errorf("Advanced set too large (%d) — tiering should hide only fine-tuning, not the bulk", adv)
	}
```

Add `"strings"` to the imports (the file currently imports only `"testing"`).

- [ ] **Step 3: Run the guard — verify green**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestP4_TierClassification' ./internal/audio/`
Expected: PASS (count is now 31 ≤ 40).

---

### Task 2: Real-time-factor performance guard (P5)

The impl report claimed "~160× faster than real-time" with no assertion anywhere. Add a native guard that renders every builtin, computes the aggregate real-time factor, and enforces a CI-safe floor (20×, env-overridable) — machine-independent enough not to flake, strong enough to catch a pipeline-wide regression.

**Files:**
- Create: `src/go/internal/audio/render_rtf_test.go`

**Interfaces:**
- Consumes: `BuiltinInstrumentIDs`, `RenderInstrumentOneShotRaw(id) ([]float32, int)`, `Reset()`, `ResetInstruments()` (all existing).
- Produces: `TestP5_RenderRealTimeFactor` (guard only).

- [ ] **Step 1: Write the guard test**

Create `src/go/internal/audio/render_rtf_test.go`:

```go
//go:build !test && !js

package audio

import (
	"math"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestP5_RenderRealTimeFactor is the performance gate for the one-declarative-
// pipeline refactor (spec P5): rendering EVERY builtin instrument one-shot must
// be dramatically faster than real time, or live param edits and instrument
// switches stall the trigger path. The floor is deliberately conservative
// (20× aggregate — dev machines measure far higher) so CI variance never
// flakes it; override with BEATMO_RTF_MIN for local tightening. Aggregate
// (total audio seconds / total wall seconds) rather than per-instrument so
// one cold-cache outlier cannot flake the gate; the worst single instrument
// is Logf'd for visibility.
func TestP5_RenderRealTimeFactor(t *testing.T) {
	minRTF := 20.0
	if s := os.Getenv("BEATMO_RTF_MIN"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			minRTF = v
		}
	}
	Reset()
	ResetInstruments()

	var audioSec, wallSec float64
	worstID, worstRTF := "", math.Inf(1)
	for _, id := range BuiltinInstrumentIDs {
		start := time.Now()
		buf, sr := RenderInstrumentOneShotRaw(id)
		el := time.Since(start).Seconds()
		if len(buf) == 0 || sr <= 0 {
			t.Fatalf("%q rendered empty (len=%d sr=%d)", id, len(buf), sr)
		}
		aSec := float64(len(buf)) / float64(sr)
		audioSec += aSec
		wallSec += el
		if el > 0 {
			if rtf := aSec / el; rtf < worstRTF {
				worstRTF, worstID = rtf, id
			}
		}
	}
	if wallSec <= 0 {
		t.Fatal("zero wall time — clock did not advance")
	}
	aggregate := audioSec / wallSec
	t.Logf("aggregate RTF %.0fx over %d instruments (%.1fs audio in %.2fs); worst %q %.0fx",
		aggregate, len(BuiltinInstrumentIDs), audioSec, wallSec, worstID, worstRTF)
	if aggregate < minRTF {
		t.Errorf("aggregate render real-time factor %.1fx below floor %.0fx — the pipeline got slower", aggregate, minRTF)
	}
}
```

- [ ] **Step 2: Prove the gate measures (fail with an impossible floor)**

Run: `cd src/go && BEATMO_RTF_MIN=1000000000 ../../.tools/go/bin/go test -count=1 -run 'TestP5_RenderRealTimeFactor' ./internal/audio/ -v`
Expected: FAIL with `aggregate render real-time factor ... below floor 1000000000x`, and a Logf line reporting the true aggregate (note it — this is the honest number to cite instead of "160×").

- [ ] **Step 3: Run with the real floor — verify green**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestP5_RenderRealTimeFactor' ./internal/audio/ -v`
Expected: PASS, Logf shows aggregate well above 20×.

---

### Task 3: Composability matrix guard (P5)

"Every stage toggleable" is covered for the modular recipe/UI but there is no across-all-instruments audit. Add one: for every builtin whose recipe schema carries `*_enabled` toggles, flip each toggle away from its effective default and assert the render (a) stays finite and (b) actually changes when the flip disables an active stage.

**Files:**
- Create: `src/go/internal/audio/stage_matrix_test.go`

**Interfaces:**
- Consumes: `BuiltinInstrumentIDs`, `RecipeForInstrument(id) string`, `RecipeRegistrations() map[string]*RecipeRegistration` (as used by `internal/ui/synth_unified_sections_test.go`), `MergeRecipeDefaults(recipeID, params)`, `SetInstrumentParam(id, name, v)`, `ResetInstrumentParams(id)`, `RenderInstrumentOneShotRaw`.
- Produces: `TestP5_EveryStageToggleablePerInstrument` (guard only).

- [ ] **Step 1: Write the guard test**

Create `src/go/internal/audio/stage_matrix_test.go`:

```go
//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// stageMatrixChecksum is the same order-sensitive bit checksum the golden net
// uses (synth_golden_identity_test.go) — a single changed sample flips it.
func stageMatrixChecksum(buf []float32) uint64 {
	var bits uint64
	for i, v := range buf {
		bits ^= (uint64(i)*2654435761 + 1) * uint64(math.Float32bits(v))
	}
	return bits
}

// stageMatrixNeutralFlips lists (instrumentID, toggle) pairs where flipping the
// toggle is legitimately inaudible — a stage that is enabled but configured
// neutral (e.g. drive_enabled on with drive amount 0). Every entry needs a
// one-line rationale. Populated ONLY from verified runs of this test; an empty
// map is the ideal state.
var stageMatrixNeutralFlips = map[[2]string]string{
	// {"some-id", "some_enabled"}: "stage enabled but neutral because ...",
}

// TestP5_EveryStageToggleablePerInstrument is the composability audit (spec P5:
// "every stage toggleable"): for every builtin instrument and every *_enabled
// stage toggle its recipe schema exposes, flipping the toggle must produce a
// finite render, and must audibly change the render (checksum) unless the pair
// is a documented neutral flip. This pins the promise that instruments differ
// only by config — every stage gate works on every instrument.
func TestP5_EveryStageToggleablePerInstrument(t *testing.T) {
	Reset()
	ResetInstruments()

	assertFinite := func(t *testing.T, id, toggle string, buf []float32) {
		t.Helper()
		for i, v := range buf {
			f := float64(v)
			if math.IsNaN(f) || math.IsInf(f, 0) {
				t.Fatalf("%s with %s flipped: non-finite sample at %d", id, toggle, i)
			}
		}
	}

	for _, id := range BuiltinInstrumentIDs {
		recipeID := RecipeForInstrument(id)
		if recipeID == "" {
			continue
		}
		reg := RecipeRegistrations()[recipeID]
		if reg == nil {
			continue
		}

		base, _ := RenderInstrumentOneShotRaw(id)
		baseSum := stageMatrixChecksum(base)

		merged := MergeRecipeDefaults(recipeID, GetInstrumentParams(id))
		for _, def := range reg.Params {
			name := def.Name
			if len(name) < 9 || name[len(name)-8:] != "_enabled" {
				continue
			}
			cur, ok := merged[name]
			if !ok {
				cur = def.Default
			}
			flipped := 1.0
			if cur >= 0.5 {
				flipped = 0.0
			}

			SetInstrumentParam(id, name, flipped)
			buf, _ := RenderInstrumentOneShotRaw(id)
			ResetInstrumentParams(id)

			if len(buf) == 0 {
				t.Errorf("%s: flipping %s rendered empty", id, name)
				continue
			}
			assertFinite(t, id, name, buf)

			if stageMatrixChecksum(buf) == baseSum {
				if why, exempt := stageMatrixNeutralFlips[[2]string{id, name}]; exempt {
					_ = why
					continue
				}
				t.Errorf("%s: flipping %s (%v -> %v) did not change the render — stage gate inert (add a stageMatrixNeutralFlips entry ONLY with a verified rationale)", id, name, cur, flipped)
			}
		}
	}
}
```

Note: if `RecipeRegistrations()` / `reg.Params` / `GetInstrumentParams` have slightly different names in `internal/audio` (the UI test accesses them as `audio.RecipeRegistrations()`), adapt to the package-internal spelling — check `synth_registry.go` for the unexported registry accessor before writing.

- [ ] **Step 2: Run and triage**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestP5_EveryStageToggleablePerInstrument' ./internal/audio/ -v 2>&1 | tail -40`
Expected: either PASS, or a handful of `did not change the render` failures.

For each failure, investigate whether the stage is genuinely neutral in that instrument's config (e.g. toggle on but the stage's amount param is 0). If genuinely neutral, add a `stageMatrixNeutralFlips` entry with the verified rationale. If the gate is actually inert (flipping does nothing even with non-neutral settings), that is a REAL composability bug — report it in the final summary; do not exempt it silently.

- [ ] **Step 3: Verify green + golden net untouched**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestP5_EveryStageToggleablePerInstrument|TestSynthGoldenByteIdentity' ./internal/audio/`
Expected: PASS (the matrix test resets all params, so goldens are unaffected).

---

### Task 4: Migrate exact-form one-pole sites to `sp_onepole_tick` (P1)

Seven open-coded sites use the exact `state += coef*(in - state)` form that `sp_onepole_tick` already encodes (all `double` state/coef). Replacing them is byte-identical by construction.

**Files:**
- Modify: `src/c/modular_stages.c` (sites at lines ~585, ~749-752, ~762, ~840, ~846-849, ~1244)
- Modify: `src/c/modular.c` (sites at lines ~924, ~929)

**Interfaces:**
- Consumes: `sp_onepole_tick(double *state, double coef, double in)` from `src/c/synth_dsp_primitives.h` (already included by both files).
- Produces: no API change — internal refactor.

**Transformation rule (apply mechanically at each site):**
`X += C * (Y - X);` becomes `sp_onepole_tick(&X, C, Y);` when the updated value is not used in the same expression, or `double v = sp_onepole_tick(&X, C, Y);` when it is. The primitive performs the identical `*state += coef*(in-*state)` and returns `*state` — same float ops, same order.

- [ ] **Step 1: Migrate the `modular_stages.c:585` site**

Current (`modular_stages.c:585`):
```c
            lpOut += lpAlpha * (mixed - lpOut);
```
Replace with:
```c
            sp_onepole_tick(&lpOut, lpAlpha, mixed);
```
(Subsequent reads of `lpOut` are unchanged — the primitive updated it in place.)

- [ ] **Step 2: Golden net**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity' ./internal/audio/`
Expected: PASS. If FAIL: the site's value-use pattern differs — restore the original line and study the surrounding expression before retrying.

- [ ] **Step 3: Migrate the kick noise/click chains (lines ~749-752, ~762, ~840, ~846-849)**

Current (`modular_stages.c:749-752`):
```c
            nzLo  += 0.016 * ((double)nz - nzLo);   /* HP ~120 Hz (drop sub rumble) */
            double nzHP = (double)nz - nzLo;
            nzBand  += 0.065 * (nzHP - nzBand);     /* 1-pole LP ~500 Hz */
            nzBand2 += 0.065 * (nzBand - nzBand2);  /* 2nd pole → steep, kills >800 Hz (no air) */
```
Replace with:
```c
            sp_onepole_tick(&nzLo, 0.016, (double)nz);   /* HP ~120 Hz (drop sub rumble) */
            double nzHP = (double)nz - nzLo;
            sp_onepole_tick(&nzBand, 0.065, nzHP);       /* 1-pole LP ~500 Hz */
            sp_onepole_tick(&nzBand2, 0.065, nzBand);    /* 2nd pole → steep, kills >800 Hz (no air) */
```
Apply the identical rule to `lpClick += 0.017 * ((double)cn - lpClick);` (line ~762), `lpClick += 0.018 * (cn - lpClick);` (line ~840), and the sibling `nzLo`/`nzBand`/`nzBand2` chain at ~846-849 (that variant's operands are already `double`, no casts).

- [ ] **Step 4: Golden net** (same command). Expected: PASS.

- [ ] **Step 5: Migrate the snare LP (`modular_stages.c:1244`) and the two `modular.c` bow smoothers (`:924`, `:929`)**

`modular_stages.c:1244`: `lpState += lpAlpha * ((double)n - lpState);` → `sp_onepole_tick(&lpState, lpAlpha, (double)n);`
`modular.c:924`: `m += 0.0004 * (target - m);` → `sp_onepole_tick(&m, 0.0004, target);`
`modular.c:929`: `lp += a * ((double)out[i] - lp);` → `sp_onepole_tick(&lp, a, (double)out[i]);`

- [ ] **Step 6: Golden net + physical-model params guard**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity|TestRenderModularP_EveryParamMutatesInContext' ./internal/audio/`
Expected: PASS both.

---

### Task 5: Add `sp_onepole_mix` primitive + migrate the lerp-form one-pole sites (P1)

Eight sites use the OTHER one-pole form `y = y*a + x*b` (two independent literals, e.g. `0.97`/`0.03`). This form's op order differs from `sp_onepole_tick`, so it needs its own primitive that reproduces it exactly.

**Files:**
- Modify: `src/c/synth_dsp_primitives.h` (add primitive after `sp_onepole_tick`)
- Modify: `src/c/modular_stages.c` (sites at lines ~434, ~555, ~994, ~1608, ~1665, ~1735, ~1865, ~1936)

**Interfaces:**
- Produces: `static inline double sp_onepole_mix(double *state, double keep, double mix, double in)` — later C code may compose it.

- [ ] **Step 1: Add the primitive**

In `src/c/synth_dsp_primitives.h`, after the `sp_onepole_tick` block (line 28), insert:

```c
/* sp_onepole_mix: the second one-pole LP idiom in the codebase,
 * `*state = *state*keep + in*mix` (keep+mix ~= 1, written as two literals so
 * the historical float rounding is reproduced exactly — do NOT fold to the
 * sp_onepole_tick form, the op order differs and output would drift). */
static inline double sp_onepole_mix(double *state, double keep, double mix, double in) {
    *state = *state * keep + in * mix;
    return *state;
}
```

- [ ] **Step 2: Migrate all eight sites**

Transformation rule: `S = S * A + X * B;` → `sp_onepole_mix(&S, A, B, X);` (add `double v =` prefix only where the expression result is consumed inline — at these eight sites it is not; `lpNoise` is read on later lines).

Worked example (`modular_stages.c:434`):
```c
            lpNoise = lpNoise * 0.97 + (double)n * 0.03;
```
becomes
```c
            sp_onepole_mix(&lpNoise, 0.97, 0.03, (double)n);
```
Apply identically at `:555` (0.96/0.04, `(double)n`), `:994` (0.97/0.03, `(double)n`), `:1608` (0.92/0.08, `xNoise`), `:1665` (0.94/0.06, `xNoise`), `:1735` (0.9/0.1, `n` — keep the operand spelled exactly `n`, its float→double promotion happens identically inside the call), `:1865` (0.92/0.08, `xNoise`), `:1936` (0.93/0.07, `xNoise`).

- [ ] **Step 3: Golden net**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity' ./internal/audio/`
Expected: PASS. If any site fails, revert THAT site only and re-verify — do not revert the batch.

---

### Task 6: Add `sp_biquad` (DF-I tick) + migrate the 11 inline biquad blocks (P1)

Eleven inline RBJ biquads share the identical Direct-Form-I update. Extract ONLY the state struct + tick — coefficients stay computed at each site exactly as-is (their normalization order varies; extracting coefficient math would risk reassociation). This is the tick-only extraction: same arithmetic, same order, byte-identical.

**Files:**
- Modify: `src/c/synth_dsp_primitives.h` (add struct + tick)
- Modify: `src/c/modular_stages.c` (sites: ~616-650 kick ×2, ~1279-1301 snare rimshot, ~1333-1438 snare body ×4, ~1485-1501 clap, ~1710-1712 cymbal, ~1772-1773 cymbal)

**Interfaces:**
- Produces: `sp_biquad` struct (`b0,b1,b2,a1,a2` coeffs + `x1,x2,y1,y2` state), `static inline double sp_biquad_tick(sp_biquad *q, double x)`.

- [ ] **Step 1: Add the primitive**

In `src/c/synth_dsp_primitives.h`, after the `sp_dcblock` block (line 49), insert:

```c
/* sp_biquad: Direct-Form-I biquad TICK. Coefficients are computed AT THE CALL
 * SITE exactly as the historical inline code did (normalization order varies
 * per site and IEEE float math is not associative) and stored pre-normalized;
 * the tick reproduces the shared update
 *   y = b0*x + b1*x1 + b2*x2 - a1*y1 - a2*y2;  x2=x1; x1=x; y2=y1; y1=y;
 * verbatim. ~11 drum/cymbal voices open-coded this exact block. */
typedef struct {
    double b0, b1, b2, a1, a2;
    double x1, x2, y1, y2;
} sp_biquad;

static inline double sp_biquad_tick(sp_biquad *q, double x) {
    double y = q->b0 * x + q->b1 * q->x1 + q->b2 * q->x2 - q->a1 * q->y1 - q->a2 * q->y2;
    q->x2 = q->x1;
    q->x1 = x;
    q->y2 = q->y1;
    q->y1 = y;
    return y;
}
```

- [ ] **Step 2: Migrate ONE site fully and gate it (snare rimshot, ~1279-1301)**

At each site the historical code has (a) per-site coefficient computation into locals like `b0_x, b1_x, b2_x, a1_x, a2_x` (already normalized by `/a0`), (b) state locals `x1,x2,y1,y2` initialized to 0, (c) the DF-I update block in the sample loop. The mechanical migration:

1. Replace the state locals with `sp_biquad bq = {0};`.
2. After the site's existing normalized-coefficient computation, assign: `bq.b0 = b0_x; bq.b1 = b1_x; bq.b2 = b2_x; bq.a1 = a1_x; bq.a2 = a2_x;` (keep the site's own coefficient expressions untouched, whatever their local names are).
3. Replace the in-loop update block
```c
            double y = b0_x * x + b1_x * x1 + b2_x * x2 - a1_x * y1 - a2_x * y2;
            x2 = x1; x1 = x; y2 = y1; y1 = y;
```
with
```c
            double y = sp_biquad_tick(&bq, x);
```
(match the site's actual variable names — some spell the input `n` or `s` rather than `x`; the downstream reads of `y` are unchanged).

- [ ] **Step 3: Golden net**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity' ./internal/audio/`
Expected: PASS. This proves the tick extraction is sound before scaling it.

- [ ] **Step 4: Migrate the remaining 10 sites, golden net after EACH file region**

Order (one golden-net run after each bullet):
1. Kick pair (`modular_stages.c:616-650`, bp + rm biquads — two `sp_biquad` structs).
2. Snare body quad (`:1333-1438`, lp + bp1 + bp2 + bp3 — four structs; NOTE this voice's coefficients are recomputed per-sample for the sweeping filters: keep the per-sample coefficient assignments exactly where they were, updating `bq.b0 = ...` in place of the old locals).
3. Clap (`:1485-1501`).
4. Cymbal pair (`:1710-1712`, `:1772-1773`).

Expected after each: golden net PASS. Any failure → revert only the just-migrated site.

- [ ] **Step 5: Full drum-family verification**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity|TestSnareFamilyKnobContract|TestRenderModularP_EveryParamMutatesInContext' ./internal/audio/`
Expected: PASS all (note: if `TestSnareFamilyKnobContract` was already failing on this branch pre-task — see memory `project_add_core_node_types_preexisting_failures` — compare against a pre-task run, not absolute green).

---

### Task 7: Migrate extractable oscillator sites to `sp_osc_tick` (P1)

~13 phase-accumulator sites evaluate via `osc_wave_shared(wave, phase)` — the exact shape `sp_osc_tick` encodes. Migrating is byte-identical when (a) the phase increment expression is passed verbatim, and (b) the phase variable is used ONLY at the single evaluation point. Sites evaluating via bare `sin(phase)` (crash/china bells at `:1855`, `:1926`) are NOT migrated (different evaluation function; a `sp_osc_sin` variant is not justified by 2 sites — leave them, documented).

**Files:**
- Modify: `src/c/modular_stages.c` (sites: ~722-736 kick harmonic body, ~813-833 kick batter/reso, ~1127-1133 tom, ~1233-1240 snare rimshot, ~1293-1296 snare sidestick, ~1414-1416 snare body, ~1597-1598 / ~1654-1655 / ~1730-1731 / ~1848-1849 / ~1919-1920 cymbal partial loops)

**Interfaces:**
- Consumes: `sp_osc`, `sp_osc_init`, `sp_osc_tick` (existing).

**Transformation rule:** for each phase accumulator: declare `sp_osc oN; sp_osc_init(&oN, <original initial phase>);` where the original `double phaseX = <init>;` was; replace the increment+eval pair
```c
        phaseX += <incExpr>;
        ... osc_wave_shared(w, phaseX) ...
```
with `... sp_osc_tick(&oN, w, <incExpr>) ...` — the increment expression is passed VERBATIM (association preserved). Pre-check each site: `grep` the function body for other uses of `phaseX`; if the phase feeds anything besides the single `osc_wave_shared` call, SKIP the site and note it.

- [ ] **Step 1: Migrate the tom site (worked example, `:1127-1133`)**

Current:
```c
        phaseFund += 2.0 * M_PI * freqFund / (double)sampleRate;
        phaseO1 += 2.0 * M_PI * freqFund * 1.5 / (double)sampleRate;
        phaseO2 += 2.0 * M_PI * freqFund * 2.1 / (double)sampleRate;
        double fund = osc_wave_shared(tWave, phaseFund);
        double o1 = osc_wave_shared(tWave, phaseO1);
        double o2 = osc_wave_shared(tWave, phaseO2);
```
Becomes (with `sp_osc oscFund, oscO1, oscO2;` + three `sp_osc_init(&oscX, 0.0);` replacing the original `double phaseFund = 0.0;` etc. declarations — check each original initial value and reproduce it in the init):
```c
        double fund = sp_osc_tick(&oscFund, tWave, 2.0 * M_PI * freqFund / (double)sampleRate);
        double o1 = sp_osc_tick(&oscO1, tWave, 2.0 * M_PI * freqFund * 1.5 / (double)sampleRate);
        double o2 = sp_osc_tick(&oscO2, tWave, 2.0 * M_PI * freqFund * 2.1 / (double)sampleRate);
```

- [ ] **Step 2: Golden net** (standard command). Expected: PASS.

- [ ] **Step 3: Migrate remaining sites in four gated batches**

Batch order, golden net after each: (1) kick sites `:722-736` + `:813-833`; (2) snare sites `:1233-1240`, `:1293-1296`, `:1414-1416`; (3) cymbal loop sites `:1597`, `:1654`, `:1730` — these run `phase[kk]` ARRAYS: replace with `sp_osc osc[N]` arrays, init loop preserved; (4) cymbal `:1848`, `:1919`. Skip any site where the phase has a second consumer (note which).

Expected after each batch: golden net PASS.

- [ ] **Step 4: Update the primitive header doc**

In `src/c/synth_dsp_primitives.h` header comment (lines 4-11), update "the kick voices (and future instruments)" to reflect reality — e.g. "the kick/tom/snare/cymbal voices and the bowed-string dynamics". Also note the two deliberate non-migrations (crash/china `sin()` bells) in one line.

---

### Task 8: Add `sp_delay` primitive + migrate physical-model delay lines (P1)

The four physical models repeat the identical circular-buffer pattern: `pm_frac_read(buf, LEN, wi, delay); buf[wi] = (float)in; wi++; if (wi >= LEN) wi = 0;`. Extract the write+wrap (the read stays a direct `pm_frac_read` call — it is already a shared helper).

**Files:**
- Modify: `src/c/synth_dsp_primitives.h`
- Modify: `src/c/modular.c` (brass ~237-238, sax ~285, reed bore ~340-341, flute d0/d1 ~407-408)

**Interfaces:**
- Produces: `static inline void sp_delay_write(float *buf, int len, int *wi, double in)`.

- [ ] **Step 1: Add the primitive**

In `src/c/synth_dsp_primitives.h`, after `sp_dcblock_tick` (line 49), insert:

```c
/* sp_delay_write: the shared waveguide delay-line write+wrap. Reads go through
 * the existing pm_frac_read helper; this pairs it with the identical
 * write-then-increment-then-wrap the four physical models each open-coded. */
static inline void sp_delay_write(float *buf, int len, int *wi, double in) {
    buf[*wi] = (float)in;
    (*wi)++;
    if (*wi >= len) *wi = 0;
}
```

- [ ] **Step 2: Migrate the four models**

Worked example (brass, `modular.c:237-238`) — current shape:
```c
        buf[wi] = (float)in;
        wi++; if (wi >= LEN) wi = 0;
```
becomes:
```c
        sp_delay_write(buf, LEN, &wi, in);
```
Match each model's actual buffer/length/index identifiers (reed bore and flute have two lines each — flute has `d0`/`d1` twin lines, migrate both). Where a model's wrap uses a different comparison (e.g. `wi &= mask`), SKIP that site and note it — only the exact `>= len → 0` form migrates.

- [ ] **Step 3: Golden net + physical-model guards**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity|TestSaxNoEditFastPathMatchesRecipe|TestRenderModularP_EveryParamMutatesInContext' ./internal/audio/`
Expected: PASS all. (The brass/reed/flute models are dormant — no shipped instrument uses osc_type 8/9/10 — so the golden net alone doesn't exercise them; `TestRenderModularP_EveryParamMutatesInContext`'s sax/bow subtests plus a compile pass are the coverage. If a dedicated brass/reed/flute render test exists — grep for `render_brass|render_reed|render_flute` in `internal/audio/*_test.go` — run it too.)

---

### Task 9: Drum-gen defaults — declarative Go table + C drift guard (P3d)

Make ONE declarative Go table the source of truth for the snare/clap/cymbal/tom per-variant curated defaults, enforced against the C `kp_get` literals by a source-parsing drift test (the `design_md_drift_test.go` idiom). Values are NOT pushed through the float32 ABI (that would change `(double)float32(1.12)` vs the literal `1.12` and break byte-identity); the C literal stays the runtime fallback, the Go table becomes the authority any change must go through, and the guard makes divergence impossible.

**Files:**
- Create: `src/go/internal/audio/drum_gen_defaults.go`
- Create: `src/go/internal/audio/drum_gen_defaults_test.go`

**Interfaces:**
- Produces: `DrumGenDefaults()` — `map[string]map[int]map[string]float64` keyed family → variant → C param field → default. Consumed by the drift test now; available to future schema/UI surfacing.

- [ ] **Step 1: Write the declarative table**

Create `src/go/internal/audio/drum_gen_defaults.go` (values transcribed from `src/c/modular_stages.c` — snare branches at :1203-1209 (variant 1), :1263-1269 (variant 2), :1315-1323 (variant 0); clap at :1475-1480; cymbal branches at :1567/:1624/:1686/:1753/:1807/:1879; tom `*Def[3]` tables at :1078-1083):

```go
package audio

// drum_gen_defaults.go is the DECLARATIVE source of truth for the drum-gen
// curated per-variant defaults (spec P3: "the value lives in one declarative
// table, not a C literal"). The C voices still carry the same values as kp_get
// fallback literals — deliberately: passing these through the float32 param ABI
// would change their double value ((double)float32(1.12) != 1.12) and break
// byte-identity. Instead this table is the authority and
// TestDrumGenDefaultsMatchCSentinels parses modular_stages.c and fails on any
// divergence, so every default change starts here and the guard forces the C
// sync. Structural per-variant tables (fStartMul, baseFreq partial banks, …)
// are voice character without knobs and stay C-owned.

// drumGenDefaults: family → variant → C param field name → default value.
// Families: "snare" (variants 0 snare / 1 rimshot / 2 sidestick), "clap"
// (single variant, reuses the snare fields), "cymbal" (0 hihat / 1 openhat /
// 2 ride-ish / 3 crash-ish / 4 crash / 5 splash-china), "tom" (0/1/2).
var drumGenDefaults = map[string]map[int]map[string]float64{
	"snare": {
		0: {
			"gen_snare_tone2": 280.0, "gen_snare_tune": 0.55,
			"gen_snare_tone_d": 46.0, "gen_snare_noise_d": 7.0,
			"gen_snare_tail_d": 10.0, "gen_snare_tone_m": 1.12,
			"gen_snare_noise_m": 0.56, "gen_snare_wire_m": 0.8,
			"gen_snare_attack": 0.5,
		},
		1: {
			"gen_snare_tone2": 1050.0, "gen_snare_tune": 1.0,
			"gen_snare_tone_d": 40.0, "gen_snare_noise_d": 200.0,
			"gen_snare_tone_m": 1.12, "gen_snare_noise_m": 0.7,
			"gen_snare_attack": 2.0,
		},
		2: {
			"gen_snare_tone2": 1200.0, "gen_snare_tune": 1.0,
			"gen_snare_tone_d": 100.0, "gen_snare_noise_d": 150.0,
			"gen_snare_tone_m": 0.5, "gen_snare_noise_m": 0.5,
			"gen_snare_attack": 1.0,
		},
	},
	"clap": {
		0: {
			"gen_snare_tune": 1.0, "gen_snare_noise_d": 7.0,
			"gen_snare_tail_d": 4.0, "gen_snare_noise_m": 0.15,
			"gen_snare_attack": 110.0,
		},
	},
	"cymbal": {
		0: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 180.0, "gen_cym_env_tail": 35.0, "gen_cym_tone_m": 0.85, "gen_cym_noise_m": 0.45, "gen_cym_noise_d": 100.0},
		1: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 120.0, "gen_cym_env_tail": 22.0, "gen_cym_tone_m": 0.7, "gen_cym_noise_m": 0.9, "gen_cym_noise_d": 18.0},
		2: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 260.0, "gen_cym_env_tail": 9.0, "gen_cym_tone_m": 1.0, "gen_cym_noise_m": 0.55, "gen_cym_noise_d": 60.0},
		3: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 200.0, "gen_cym_env_tail": 25.0, "gen_cym_tone_m": 0.6, "gen_cym_noise_m": 0.5},
		4: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 40.0, "gen_cym_env_tail": 8.0, "gen_cym_tone_m": 1.0, "gen_cym_noise_m": 0.3, "gen_cym_noise_d": 12.0},
		5: {"gen_cym_tune": 1.0, "gen_cym_env_fast": 10.0, "gen_cym_env_tail": 3.0, "gen_cym_tone_m": 1.0, "gen_cym_noise_m": 0.35, "gen_cym_noise_d": 8.0},
	},
	"tom": {
		0: {"gen_tom_sweep": 18.0, "gen_tom_ring": 2.8, "gen_tom_o1": 0.5, "gen_tom_o2": 0.25, "gen_tom_stick": 0.35, "gen_tom_room": 0.08},
		1: {"gen_tom_sweep": 22.0, "gen_tom_ring": 3.5, "gen_tom_o1": 0.55, "gen_tom_o2": 0.28, "gen_tom_stick": 0.38, "gen_tom_room": 0.06},
		2: {"gen_tom_sweep": 14.0, "gen_tom_ring": 2.2, "gen_tom_o1": 0.45, "gen_tom_o2": 0.22, "gen_tom_stick": 0.32, "gen_tom_room": 0.10},
	},
}

// DrumGenDefaults returns the declarative drum-gen default table (deep enough
// copy for read-only use by tests/tooling).
func DrumGenDefaults() map[string]map[int]map[string]float64 {
	return drumGenDefaults
}
```

IMPORTANT: before finalizing, re-read the actual C branches and verify every value above against the source — the snare/cymbal figures here were transcribed during planning and MUST be re-checked against `modular_stages.c` at implementation time (branch order in the file: snare variant==1 first, then ==2, then the else==0 branch).

- [ ] **Step 2: Write the drift guard (failing first if any transcription is wrong)**

Create `src/go/internal/audio/drum_gen_defaults_test.go`:

```go
package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// TestDrumGenDefaultsMatchCSentinels parses src/c/modular_stages.c and asserts
// every kp_get fallback literal for the drum-gen curated knobs equals the
// declarative Go table (drum_gen_defaults.go). The C literal is the runtime
// fallback (byte-identity: values must NOT round-trip the float32 ABI); this
// guard makes the Go table the enforced source of truth — change a default in
// C without the table (or vice versa) and this fails.
//
// Occurrence order in the C file is load-bearing and pinned here: snare
// variant==1 branch, then ==2, then the else (variant 0) branch, then the clap
// voice; cymbal branches 0..5 in file order. The tom fallbacks are per-variant
// tables (sweepDef[3] etc.), parsed separately.
func TestDrumGenDefaultsMatchCSentinels(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "c", "modular_stages.c"))
	if err != nil {
		t.Fatalf("read modular_stages.c: %v", err)
	}
	text := string(src)

	// --- kp_get scalar fallbacks: field name -> ordered occurrence list.
	kpRe := regexp.MustCompile(`kp_get\(p->(gen_snare_\w+|gen_cym_\w+)\[k\],\s*(-?[0-9.]+)\)`)
	occ := map[string][]float64{}
	for _, m := range kpRe.FindAllStringSubmatch(text, -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			t.Fatalf("unparsable literal %q for %s", m[2], m[1])
		}
		occ[m[1]] = append(occ[m[1]], v)
	}

	// Expected occurrence order per snare/clap field: variant1, variant2,
	// variant0, clap (fields absent from a branch simply skip it).
	snareOrder := [][2]interface{}{{"snare", 1}, {"snare", 2}, {"snare", 0}, {"clap", 0}}
	for _, field := range []string{
		"gen_snare_tone2", "gen_snare_tune", "gen_snare_tone_d", "gen_snare_noise_d",
		"gen_snare_tail_d", "gen_snare_tone_m", "gen_snare_noise_m", "gen_snare_wire_m",
		"gen_snare_attack",
	} {
		var want []float64
		var wantAt []string
		for _, fv := range snareOrder {
			fam, variant := fv[0].(string), fv[1].(int)
			if v, ok := drumGenDefaults[fam][variant][field]; ok {
				want = append(want, v)
				wantAt = append(wantAt, fmt.Sprintf("%s/v%d", fam, variant))
			}
		}
		assertOccurrences(t, field, occ[field], want, wantAt)
	}

	// Cymbal: branches appear in variant order 0..5 in the file.
	for _, field := range []string{
		"gen_cym_tune", "gen_cym_env_fast", "gen_cym_env_tail",
		"gen_cym_tone_m", "gen_cym_noise_m", "gen_cym_noise_d",
	} {
		var want []float64
		var wantAt []string
		for variant := 0; variant <= 5; variant++ {
			if v, ok := drumGenDefaults["cymbal"][variant][field]; ok {
				want = append(want, v)
				wantAt = append(wantAt, fmt.Sprintf("cymbal/v%d", variant))
			}
		}
		assertOccurrences(t, field, occ[field], want, wantAt)
	}

	// --- Tom: per-variant static tables `static const double sweepDef[3] = { a, b, c };`
	tomTables := map[string]string{
		"sweepDef": "gen_tom_sweep", "ringDef": "gen_tom_ring",
		"o1Def": "gen_tom_o1", "o2Def": "gen_tom_o2",
		"stickDef": "gen_tom_stick", "roomDef": "gen_tom_room",
	}
	for cName, field := range tomTables {
		re := regexp.MustCompile(cName + `\[3\]\s*=\s*\{\s*(-?[0-9.]+),\s*(-?[0-9.]+),\s*(-?[0-9.]+)\s*\}`)
		m := re.FindStringSubmatch(text)
		if m == nil {
			t.Errorf("tom table %s not found in modular_stages.c", cName)
			continue
		}
		for variant := 0; variant < 3; variant++ {
			got, _ := strconv.ParseFloat(m[variant+1], 64)
			want := drumGenDefaults["tom"][variant][field]
			if got != want {
				t.Errorf("tom/v%d %s: C literal %v != table %v", variant, field, got, want)
			}
		}
	}
}

func assertOccurrences(t *testing.T, field string, got, want []float64, wantAt []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d kp_get occurrences in C, table expects %d (%v) — branch added/removed without updating drum_gen_defaults.go", field, len(got), len(want), wantAt)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s occurrence %d (%s): C literal %v != table %v", field, i, wantAt[i], got[i], want[i])
		}
	}
}
```

- [ ] **Step 3: Run the drift guard — fix transcription errors**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 -run 'TestDrumGenDefaultsMatchCSentinels' ./internal/audio/`
(This test is tag-neutral — it parses source text, no CGo — so it runs on the FAST path and gates every PR.)
Expected first run: possibly FAIL on transcription mismatches. Correct `drum_gen_defaults.go` (the C file is ground truth today) until PASS. Also check whether the cymbal `wave` fallback (`kp_get(...gen_wave..., 2.0)` in variant 0) should join the table — if the regex catches `gen_wave` occurrences, either add them to the table or narrow the regex to the snare/cym field prefixes as written.

- [ ] **Step 4: Cross-check the existing per-recipe spec lit-maps against the table**

Append to `drum_gen_defaults_test.go` a second test asserting the legacy `snareVariantSpecs` / `tomVariantSpecs` / `cymbalVariantSpecs` lit-maps (in the `*_modular_push.go` files, `//go:build !test && !js`) agree with the table where their param names overlap. Because the spec vars are native-only, put this in a separate NATIVE test file `drum_gen_defaults_native_test.go` with tag `//go:build !test && !js`:

```go
//go:build !test && !js

package audio

import "testing"

// legacyLitToGen maps the legacy per-recipe knob names (the lit maps in
// *_modular_push.go) to the C-side gen field names of the declarative table.
var legacyLitToGen = map[string]string{
	"snare_tone2_freq": "gen_snare_tone2", "snare_noise_tune": "gen_snare_tune",
	"snare_tone_decay": "gen_snare_tone_d", "snare_noise_decay": "gen_snare_noise_d",
	"snare_tail_decay": "gen_snare_tail_d", "snare_tone_mix": "gen_snare_tone_m",
	"snare_noise_mix": "gen_snare_noise_m", "snare_wire_mix": "gen_snare_wire_m",
	"snare_attack": "gen_snare_attack",
	"tom_sweep_rate":   "gen_tom_sweep", "tom_ring_rate": "gen_tom_ring",
	"tom_o1_gain": "gen_tom_o1", "tom_o2_gain": "gen_tom_o2",
	"tom_stick": "gen_tom_stick", "tom_room": "gen_tom_room",
	"cym_tune": "gen_cym_tune", "cym_env_fast": "gen_cym_env_fast",
	"cym_env_tail": "gen_cym_env_tail", "cym_tone_mix": "gen_cym_tone_m",
	"cym_noise_mix": "gen_cym_noise_m", "cym_noise_decay": "gen_cym_noise_d",
}

// TestDrumGenSpecLitsMatchDeclarativeTable keeps the THIRD copy of these
// values (the per-recipe spec lit maps that feed schema defaults) in sync with
// the declarative table, so there is exactly one place a default can change.
func TestDrumGenSpecLitsMatchDeclarativeTable(t *testing.T) {
	check := func(family string, variant int, lits map[string]float64) {
		t.Helper()
		for legacy, v := range lits {
			gen, ok := legacyLitToGen[legacy]
			if !ok {
				continue // knob with no C kp_get fallback (e.g. wave) — table doesn't own it
			}
			want, ok := drumGenDefaults[family][variant][gen]
			if !ok {
				t.Errorf("%s/v%d: spec lit %s (%v) has no declarative-table entry %s", family, variant, legacy, v, gen)
				continue
			}
			if v != want {
				t.Errorf("%s/v%d %s: spec lit %v != declarative table %v", family, variant, legacy, v, want)
			}
		}
	}
	for recipeID, spec := range snareVariantSpecs {
		fam := "snare"
		if spec.variant == 3 {
			fam = "clap"
		}
		variant := int(spec.variant)
		if fam == "clap" {
			variant = 0
		}
		_ = recipeID
		check(fam, variant, spec.lit)
	}
	for _, spec := range tomVariantSpecs {
		check("tom", int(spec.variant), spec.lit)
	}
	for _, spec := range cymbalVariantSpecs {
		check("cymbal", int(spec.variant), spec.lit)
	}
}
```

NOTE: the exact field names of the spec structs (`spec.variant`, `spec.lit`) must be read from `snare_modular_push.go:170-199` / `tom_modular_push.go:126-146` / `cymbal_modular_push.go:159-194` at implementation time — adjust the test to the real struct shape. Cymbal spec `variant` mapping to the C branch index must also be verified (the Go discriminator meanings are: cymbal 0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash — which may NOT equal the C branch order 0..5; reconcile against the C `gen_cym_variant` switch before asserting).

- [ ] **Step 5: Run both guards + golden net**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestDrumGenDefaults|TestSynthGoldenByteIdentity' ./internal/audio/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 -run 'TestDrumGenDefaultsMatchCSentinels' ./internal/audio/`
Expected: PASS (golden net trivially unaffected — no production code changed).

---

### Task 10: Migrate the remaining 34 seeded instruments into `modularInstrumentDefs` (P3d)

Move every seeded-modular instrument from the four scattered registration sites into the declarative table. The fold-in plumbing already exists (`modularTableDescriptors/Bindings/Configs/Voices` + `extractTableIDs`); migration = add rows, delete the now-duplicate hand-written entries, regenerate the two gen files. Byte-identical because every row carries the exact same Seed var, Beats, DurationSec, Amplitude as the entries it replaces.

**Files:**
- Modify: `src/go/internal/audio/modular_instruments.go` (add rows)
- Modify: `src/go/internal/audio/engine_instruments.go` (delete hand-written voice entries)
- Modify: `src/go/internal/audio/synth_recipe.go` (delete hand-written descriptors + bindings)
- Modify: `src/go/internal/audio/config.go` (delete hand-written InstrumentConfigs)
- Regenerate: `src/js/modular_instruments.gen.js`, `src/go/internal/audio/instrument_ids_gen.go`

**Interfaces:**
- Consumes: existing seed vars in `instrument_seeds.go` / `modular_recipe.go` (`modularPadSeed`).
- Produces: table rows only. `melodicRecipeIDs` (`melodic_recipes.go`) is keyed by RecipeID and is NOT touched — organ/sax/violin/… stay in, congas/modular-pad stay out.

**Do it in two gated batches.** The base `"modular"` instrument has NO seed and stays hand-written.

- [ ] **Step 1: Batch 1 — percussion + the non-uniform rows (6 rows: modular-pad, conga, conga-open, conga-tumba, organ, sax)**

Append to `modularInstrumentDefs` in `src/go/internal/audio/modular_instruments.go`:

```go
	// ── Batch 2026-07-05: the remaining seeded-modular instruments absorbed
	// from the four scattered registration sites (engine_instruments.go voice,
	// synth_recipe.go descriptor+binding, config.go config). Values are the
	// EXACT previous registrations — byte-identity is gated by
	// TestSynthGoldenByteIdentity. ──
	{ID: "modular-pad", RecipeID: "synth-modular-pad", Display: "Modular Pad", Category: modularRecipeCategory, Seed: modularPadSeed, Beats: 1.0, DurationSec: 1.0, Amplitude: 0.8},
	{ID: "conga", RecipeID: "synth-modular-conga", Display: "Conga", Category: modularRecipeCategory, Seed: congaSeed, Beats: 0.5, DurationSec: 0.5, Amplitude: 0.8},
	{ID: "conga-open", RecipeID: "synth-modular-conga-open", Display: "Conga Open", Category: modularRecipeCategory, Seed: congaOpenSeed, Beats: 0.5, DurationSec: 0.5, Amplitude: 0.8},
	// conga-tumba: DurationSec 0.6 ≠ Beats-derived 0.5 — preserved exactly from config.go.
	{ID: "conga-tumba", RecipeID: "synth-modular-conga-tumba", Display: "Conga Tumba", Category: modularRecipeCategory, Seed: congaTumbaSeed, Beats: 0.5, DurationSec: 0.6, Amplitude: 0.8},
	// organ/sax: non-uniform amplitudes preserved exactly (organ 0.65 for
	// sustained-voice headroom under sends; sax 0.78) — from config.go.
	{ID: "organ", RecipeID: "synth-modular-organ", Display: "Organ", Category: modularRecipeCategory, Seed: organSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.65},
	{ID: "sax", RecipeID: "synth-modular-sax", Display: "Saxophone", Category: modularRecipeCategory, Seed: saxSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.78},
```

Then DELETE (do not comment out) for exactly these 6 ids:
1. Their `CVariantInstrument` entries in `engine_instruments.go` (conga block ~430-445, organ/sax ~446-465; keep any preceding block comments that explain the family, moving them onto the table rows if load-bearing).
2. Their descriptor lines in `synth_recipe.go` (`:491-496` for congas/organ/sax; the modular-pad descriptor is nearby — find it with `grep -n "synth-modular-pad" synth_recipe.go`).
3. Their binding lines in `synth_recipe.go` (`:592-598` + modular-pad's).
4. Their config lines in `config.go` (`:70-77` + modular-pad's).

- [ ] **Step 2: Regenerate the two gen files**

Run: `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-modular-instruments > ../js/modular_instruments.gen.js`
Then regenerate the id list — find the generator with `grep -rn "go:generate\|extractTableIDs" src/go/internal/audio/gen_instrument_ids.go` and run the command it documents (likely `cd src/go && ../../.tools/go/bin/go generate ./internal/audio/` or a direct `go run`).
Expected: both files change; `git diff --stat src/js/modular_instruments.gen.js` shows added lines for the 6 ids. IMPORTANT: `BuiltinInstrumentIDs` must contain the same set before and after — verify with the drift tests in the next step, and if the generated ORDER changed, confirm no test pins ordering.

- [ ] **Step 3: Gate batch 1**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity|TestModularNoEditFastPathMatchesRecipe|TestSaxNoEditFastPathMatchesRecipe' ./internal/audio/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 -run 'TestModularInstrumentsGenFileIsCurrent|TestSynthRecipe|TestEveryKindClassified' ./internal/audio/ ./internal/hooks/`
Expected: ALL PASS. The golden net is the real gate: any Beats/Duration/Amplitude/Seed transcription slip changes a checksum and fails loudly. Also run the discipline guards: `-run 'TestSynthRecipeDiscipline|TestSynthRecipeInvariants' ./internal/audio/` (fast path, `-tags test`).

- [ ] **Step 4: Batch 2 — the 28 melodic rows**

Append rows for: violin, violin-ensemble, cello, cello-warm, organ-church, scifi-lead, guitar-nylon, guitar-nylon-bright, guitar-steel, guitar-steel-warm, guitar-electric, guitar-electric-neck, harp, piano-grand, piano-felt, flute, flute-breathy, oboe, oboe-full, trumpet, trumpet-mellow, french-horn, french-horn-loud, bass-guitar, bass-acid, bass-reese, bass-fm, bass-808.

Row template (all 28 follow it uniformly — Amplitude 0.8; Beats == DurationSec; 2.0 for the sustained voices, 1.5 for guitars/basses — VERIFY each against its current `engine_instruments.go` Beats and `config.go` DurationSec before writing; the review evidence table has the full list):

```go
	{ID: "violin", RecipeID: "synth-modular-violin", Display: "Violin", Category: modularRecipeCategory, Seed: violinSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "violin-ensemble", RecipeID: "synth-modular-violin-ensemble", Display: "Violin Ensemble", Category: modularRecipeCategory, Seed: violinEnsembleSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "cello", RecipeID: "synth-modular-cello", Display: "Cello", Category: modularRecipeCategory, Seed: celloSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "cello-warm", RecipeID: "synth-modular-cello-warm", Display: "Cello Warm", Category: modularRecipeCategory, Seed: celloWarmSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "organ-church", RecipeID: "synth-modular-organ-church", Display: "Church Organ", Category: modularRecipeCategory, Seed: organChurchSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "scifi-lead", RecipeID: "synth-modular-scifi-lead", Display: "Sci-Fi Lead", Category: modularRecipeCategory, Seed: sciFiLeadSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "guitar-nylon", RecipeID: "synth-modular-guitar-nylon", Display: "Guitar Nylon", Category: modularRecipeCategory, Seed: guitarNylonSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-nylon-bright", RecipeID: "synth-modular-guitar-nylon-bright", Display: "Guitar Nylon Bright", Category: modularRecipeCategory, Seed: guitarNylonBrightSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-steel", RecipeID: "synth-modular-guitar-steel", Display: "Guitar Steel", Category: modularRecipeCategory, Seed: guitarSteelSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-steel-warm", RecipeID: "synth-modular-guitar-steel-warm", Display: "Guitar Steel Warm", Category: modularRecipeCategory, Seed: guitarSteelWarmSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-electric", RecipeID: "synth-modular-guitar-electric", Display: "Guitar Electric", Category: modularRecipeCategory, Seed: guitarElectricSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-electric-neck", RecipeID: "synth-modular-guitar-electric-neck", Display: "Guitar Electric Neck", Category: modularRecipeCategory, Seed: guitarElectricNeckSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "harp", RecipeID: "synth-modular-harp", Display: "Harp", Category: modularRecipeCategory, Seed: harpSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "piano-grand", RecipeID: "synth-modular-piano-grand", Display: "Piano Grand", Category: modularRecipeCategory, Seed: pianoGrandSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "piano-felt", RecipeID: "synth-modular-piano-felt", Display: "Piano Felt", Category: modularRecipeCategory, Seed: pianoFeltSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "flute", RecipeID: "synth-modular-flute", Display: "Flute", Category: modularRecipeCategory, Seed: fluteSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "flute-breathy", RecipeID: "synth-modular-flute-breathy", Display: "Flute Breathy", Category: modularRecipeCategory, Seed: fluteBreathySeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "oboe", RecipeID: "synth-modular-oboe", Display: "Oboe", Category: modularRecipeCategory, Seed: oboeSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "oboe-full", RecipeID: "synth-modular-oboe-full", Display: "Oboe Full", Category: modularRecipeCategory, Seed: oboeFullSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "trumpet", RecipeID: "synth-modular-trumpet", Display: "Trumpet", Category: modularRecipeCategory, Seed: trumpetSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "trumpet-mellow", RecipeID: "synth-modular-trumpet-mellow", Display: "Trumpet Mellow", Category: modularRecipeCategory, Seed: trumpetMellowSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "french-horn", RecipeID: "synth-modular-french-horn", Display: "French Horn", Category: modularRecipeCategory, Seed: frenchHornSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "french-horn-loud", RecipeID: "synth-modular-french-horn-loud", Display: "French Horn Loud", Category: modularRecipeCategory, Seed: frenchHornLoudSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "bass-guitar", RecipeID: "synth-modular-bass-guitar", Display: "Bass Guitar", Category: modularRecipeCategory, Seed: bassGuitarSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-acid", RecipeID: "synth-modular-bass-acid", Display: "Acid Bass", Category: modularRecipeCategory, Seed: bassAcidSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-reese", RecipeID: "synth-modular-bass-reese", Display: "Reese Bass", Category: modularRecipeCategory, Seed: bassReeseSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-fm", RecipeID: "synth-modular-bass-fm", Display: "FM Bass DX", Category: modularRecipeCategory, Seed: bassFMSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-808", RecipeID: "synth-modular-bass-808", Display: "808 Bass", Category: modularRecipeCategory, Seed: bass808Seed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
```

Delete the corresponding 28 hand-written entries from `engine_instruments.go`, `synth_recipe.go` (descriptors + bindings), `config.go`. Regenerate both gen files (Step 2 commands).

- [ ] **Step 5: Gate batch 2 — the full net**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestSynthGoldenByteIdentity|TestModularNoEditFastPathMatchesRecipe|TestP5_RenderRealTimeFactor|TestP5_EveryStageToggleablePerInstrument' ./internal/audio/
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 ./internal/audio/
```
Expected: ALL PASS. If a golden checksum changes for a migrated id, one of its row values doesn't match the deleted registration — diff the row against `git diff` of the deleted entry.

- [ ] **Step 6: Update the table's own header comment**

`modular_instruments.go:33-36` says the table holds only the kick family and absorbs the rest "next" — rewrite to state the table now holds ALL seeded-modular instruments (kick family + percussion + melodic; the base `modular` voice stays hand-written because it has no Seed). Also update the stale `engine_instruments.go` sax-comment block ("~32 other seeded instruments still carry the bare-renderModular one-shot bug") if it survived the deletions — the blanket bake pass fixed that and the entries are now table-derived.

---

### Task 11: Docs, spec status honesty, and final full verification

**Files:**
- Modify: `docs/superpowers/specs/2026-07-04-config-declarative-synth-pipeline-design.md` (status section)
- Modify: `AGENTS.synth.md` (§ Recipe Registry — the add-an-instrument recipe)
- Modify: `CLAUDE.md` (the "SynthRecipe is the canonical instrument path" gotcha)

- [ ] **Step 1: Update the spec's phase status to match reality**

In the spec doc, mark honestly: P0 done; P1 done for the shared idioms (one-pole ×2 forms, biquad tick, oscillator, delay-write, dc-block, pitch-glide, reverb3 — with the documented non-migrations: crash/china `sin()` bells, per-site biquad coefficient math, structural C tables; `sp_adsr` and `sp_noise` are deliberately NOT added — the voices have no duplicated ADSR block, their envelopes are single-expression `exp(-rate*t)` decays, and all noise already flows through the shared `noise.h` generators, so both would be primitives without call sites); P2 done (sax/bow parameterized; brass/reed/flute dormant-by-design); P3 done (pipeline unified + RenderFunc removed + declarative instrument table now covers all seeded-modular instruments + drum-gen defaults declaratively tabled with C drift guard); P4 done (note: the pipeline overview is the chip strip — `drawSynthChipStrip` renders the stages in audio order with connector wires and enable dots; no separate diagram is planned); P5 done via `TestP5_RenderRealTimeFactor` (cite the measured aggregate from Task 2 Step 2, not "160×") + `TestP5_EveryStageToggleablePerInstrument`.

- [ ] **Step 2: Update the instrument-adding recipe in AGENTS.synth.md + CLAUDE.md**

Both currently describe the 4-touchpoint registration (`builtinRecipeDescriptors` entry, `ParamRenderFuncs`, `builtinRecipeRenderers`, `builtinInstrumentRecipeBindings`). Rewrite to the table-first reality: **append one `modularInstrumentDefs` row + regenerate `modular_instruments.gen.js` and `instrument_ids_gen.go`** (drum/FM families still go through their family spec tables). Keep the old path description only for the non-modular legacy families that still use it — verify with `grep -n "builtinRecipeRenderers" src/go/internal/audio/synth_recipe.go` what remains.

- [ ] **Step 3: Final full verification sweep**

```bash
# Native audio: every guard this plan added or touched + the golden net
cd src/go && ../../.tools/go/bin/go test -count=1 ./internal/audio/ -run 'TestSynthGoldenByteIdentity|TestP4_TierClassification|TestP5_RenderRealTimeFactor|TestP5_EveryStageToggleablePerInstrument|TestDrumGenSpecLitsMatchDeclarativeTable|TestModularNoEditFastPathMatchesRecipe|TestRenderModularP_EveryParamMutatesInContext|TestSaxNoEditFastPathMatchesRecipe'

# Fast path: full audio package + the source-parsing drift guards
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 ./internal/audio/ ./internal/i18n/

# UI fast-path guards adjacent to the synth tab (targeted, NOT the full ui suite)
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 ./internal/ui/ -run 'TestSynthUnified|TestP4_|TestSynthTab_ModularSectionsHaveEnablePills'

# WASM: the C edits compile under emscripten and browser output parity holds
cd ../.. && make wasm
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
```
Expected: all PASS (xplat compare proves the C primitive refactors are byte-identical under emscripten too, not just native gcc/clang). Any failure set must be diffed against this branch's documented pre-existing failures before attributing it to this work.

- [ ] **Step 4: Report**

Summarize per task: what changed, guard status, the honest RTF number, any composability exemptions added (with rationale), any oscillator/delay sites skipped (with reason). Leave everything uncommitted.
