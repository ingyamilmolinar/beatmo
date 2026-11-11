# FM Time-Domain Param Visualization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the FM time-domain knobs (operator decay, FM pitch-env) and the trivial `fm_base_freq`/`fm_op1_level` cases visibly move the synth focus graph, via a pure-math schematic time-curve renderer — closing all but one (genuinely-inert) FM exemption.

**Architecture:** A new pure-math `conceptFMEnvelope` renderer draws a schematic decay-vs-time or pitch-vs-time curve from the selected knob's value (like the existing `conceptMotion`/`conceptEnvelope` pictures). Per-knob routing in `synthFocusRendererForKnob` sends FM-time params to it, `fm_base_freq` to the existing pitch wave, and `fm_op1_level` to the amplitude wave. The strict sweep gate then drops the now-closed exemptions.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), `internal/ui` only — NO `internal/audio`/DSP changes.

**Conventions (read once):**
- Bundled Go from `src/go`: `../../.tools/go/bin/go`. Test: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <Name>`.
- **Do NOT `git add`/commit/stash/restore/`rm`** (repo policy). Each "Commit" step is a **Checkpoint**: run the test, leave changes in the tree, report.
- The full `internal/ui` suite has PRE-EXISTING failures + active concurrent i18n WIP that intermittently flap-breaks the `-tags test` build in shared files (`synth_panel_zone.go`, `synth_preview_layer.go`). Verify ONLY your own tests with `-run`; trust `go test`/`go vet` over IDE diagnostics. If the build fails ONLY in an `i18n_*`/`wave_*` file, move it aside + restore unconditionally in one command. **Do the work, verify with `-run`, then STOP — never spawn background log-watch loops.**
- All concept renderers share the type `conceptRenderer = func(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64)`. Helpers (all in `synth_concept_viz.go`/`synth_focus_graph.go`): `conceptBgFill`, `conceptMergedParams(instID) audio.RecipeParams` (map of effective params), `conceptDrawFilledCurve(dst, rect, samples []float64, yAt func(float64) int, fillBaseline int, fillCol, strokeCol)`, colors `colConceptFill/colConceptStroke/colConceptGhost`, `colAccent`, `WithAlpha`, `AlphaSubtle/AlphaStrong`.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `src/go/internal/ui/synth_concept_viz.go` | concept renderers | Add `conceptFMEnvelope` + `fmDecaySamples`/`fmPitchEnvSamples`/`fmPitchEnvParams` helpers |
| `src/go/internal/ui/synth_focus_graph.go` | per-knob routing | Extend `synthFocusRendererForKnob`; add `isFMTimeDomainKnob`/`isFMPitchEnvKnob`/`isFMDecayKnob`; extend `isSynthPitchKnob` |
| `src/go/internal/ui/synth_fm_envelope_test.go` | **NEW** renderer + routing tests | Create |
| `src/go/internal/ui/synth_focus_sweep_test.go` | strict gate | Remove now-closed FM exemptions |
| `src/go/internal/ui/scene_catalog.go` | screenshot scenes | Add `crop_synth_focus_fm_decay` + `crop_synth_focus_fm_penv` |
| `src/go/internal/ui/synth_focus_scene_test.go` | scene guard | Add guard rows for the 2 new scenes |

---

## Task 1: `conceptFMEnvelope` renderer + helpers

**Files:**
- Modify: `src/go/internal/ui/synth_concept_viz.go`
- Test: `src/go/internal/ui/synth_fm_envelope_test.go` (Create)

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/synth_fm_envelope_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// fmEnvFingerprint renders conceptFMEnvelope for inst with knob `name`=v and
// returns a hash of the painted rects (changes when the picture changes).
func fmEnvFingerprint(t *testing.T, inst, name string, v float64) int {
	t.Helper()
	audio.ResetInstrumentParams(inst)
	audio.SetInstrumentParam(inst, name, v)
	def := audio.ParamDef{Name: name, Group: "fm"}
	rects := collectFilledRects(t, func() {
		conceptFMEnvelope(ebiten.NewImage(260, 110), image.Rect(0, 0, 260, 110), inst, def, nil)
	})
	h := len(rects)
	for _, r := range rects {
		h = h*31 + r.Rect.Min.Y + r.Rect.Max.X*7
	}
	return h
}

func TestConceptFMEnvelope_RespondsToDecayAndPitchEnv(t *testing.T) {
	inst := "fm-env-modular"
	bindModularConceptInst(t, inst)
	reg := audio.RecipeRegistrations()[audio.RecipeForInstrument(inst)]

	// Decay + pitch-env knobs the recipe actually exposes.
	for _, name := range []string{"fm_op2_decay", "fm_pitch_env_amount", "fm_pitch_env_decay"} {
		d := fmFindParam(reg, name)
		if d == nil {
			t.Logf("recipe lacks %s, skipping", name)
			continue
		}
		if fmEnvFingerprint(t, inst, name, d.Min) == fmEnvFingerprint(t, inst, name, d.Max) {
			t.Errorf("conceptFMEnvelope identical at Min/Max for %s — not reflected", name)
		}
	}
}

func fmFindParam(reg *audio.RecipeRegistration, name string) *audio.ParamDef {
	if reg == nil {
		return nil
	}
	for i := range reg.Params {
		if reg.Params[i].Name == name {
			return &reg.Params[i]
		}
	}
	return nil
}
```

> Confirm the exact type name of `audio.RecipeRegistrations()`'s value (the registration struct) and its params field by reading `synth_focus_sweep_test.go` (it already iterates `reg.Params`). Match `fmFindParam`'s signature to the real type. `bindModularConceptInst`, `collectFilledRects`, `drawnRect{Rect, Color}` are the same helpers used across the existing focus tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestConceptFMEnvelope_RespondsToDecayAndPitchEnv`
Expected: FAIL — `conceptFMEnvelope` undefined.

- [ ] **Step 3: Implement the renderer + helpers**

Append to `src/go/internal/ui/synth_concept_viz.go` (ensure `math` is imported — it already is in this file):

```go
// ── conceptFMEnvelope — schematic time-curves for time-domain FM params ──────
//
// The steady, cycle-normalized conceptFM wave cannot show params whose effect
// evolves over the note: per-operator amplitude decay and the FM pitch sweep.
// conceptFMEnvelope draws a PURE-MATH schematic of the param's meaning over a
// normalized time window — an exp decay curve for an operator's decay knob, a
// pitch-vs-time sweep for the FM pitch-env — exactly as conceptMotion/conceptEnvelope
// draw schematic time pictures. No audio render; live during drag.

// fmDecaySamples returns n samples of an exponential amplitude fade exp(-t/tau)
// over [0, windowSec]. Larger tau ⇒ slower fall ⇒ flatter curve.
func fmDecaySamples(tau, windowSec float64, n int) []float64 {
	if n < 2 {
		n = 2
	}
	if tau <= 0.02 {
		tau = 0.02
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1) * windowSec
		out[i] = math.Exp(-t / tau)
	}
	return out
}

// fmPitchEnvSamples returns n samples of a pitch-vs-time sweep
// (amt/maxAmt)·exp(-t/dec) over [0, windowSec], normalized to ~[-1,1].
func fmPitchEnvSamples(amt, dec, maxAmt, windowSec float64, n int) []float64 {
	if n < 2 {
		n = 2
	}
	if dec <= 0 {
		dec = 0.06
	}
	if maxAmt <= 0 {
		maxAmt = 24
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1) * windowSec
		out[i] = (amt / maxAmt) * math.Exp(-t/dec)
	}
	return out
}

// fmPitchEnvParams reads the FM pitch-env amount + decay from a params map,
// handling both the modular (fm_pitch_env_*) and bespoke (fm_pe_*) naming.
func fmPitchEnvParams(m map[string]float64) (amt, dec float64) {
	amt = m["fm_pitch_env_amount"]
	if amt == 0 {
		amt = m["fm_pe_amt"]
	}
	dec = m["fm_pitch_env_decay"]
	if dec == 0 {
		dec = m["fm_pe_decay"]
	}
	if dec <= 0 {
		dec = 0.06
	}
	return amt, dec
}

// conceptFMEnvelope draws the schematic time-curve for a time-domain FM knob.
func conceptFMEnvelope(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	merged := conceptMergedParams(instID)
	n := rect.Dx()
	if n < 2 {
		n = 2
	}

	if isFMPitchEnvKnob(def.Name) {
		// Bipolar pitch sweep around the centerline.
		const windowSec, maxAmt = 0.4, 24.0
		midYf := float64(rect.Min.Y+rect.Max.Y) / 2
		half := float64(rect.Dy()-2) / 2
		yAt := func(v float64) int {
			if v > 1 {
				v = 1
			} else if v < -1 {
				v = -1
			}
			return int(math.Round(midYf - v*half))
		}
		mid := int(math.Round(midYf))
		amt, dec := fmPitchEnvParams(merged)
		if ghost != nil {
			gAmt, gDec := fmPitchEnvParams(ghost)
			if gAmt != amt || gDec != dec {
				conceptDrawFilledCurve(dst, rect, fmPitchEnvSamples(gAmt, gDec, maxAmt, windowSec, n), yAt, mid, nil, colConceptGhost)
			}
		}
		conceptDrawFilledCurve(dst, rect, fmPitchEnvSamples(amt, dec, maxAmt, windowSec, n), yAt, mid,
			WithAlpha(colAccent, AlphaSubtle), WithAlpha(colAccent, AlphaStrong))
		return
	}

	// Decay mode: amplitude fade from the SELECTED operator's decay value.
	const windowSec = 1.0
	baseY := rect.Max.Y - 1
	topY := rect.Min.Y + 1
	yAt := func(v float64) int {
		if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		return baseY - int(v*float64(baseY-topY))
	}
	tau := merged[def.Name]
	if ghost != nil {
		if gv, ok := ghost[def.Name]; ok && gv != tau {
			conceptDrawFilledCurve(dst, rect, fmDecaySamples(gv, windowSec, n), yAt, baseY, nil, colConceptGhost)
		}
	}
	conceptDrawFilledCurve(dst, rect, fmDecaySamples(tau, windowSec, n), yAt, baseY, colConceptFill, colConceptStroke)
}
```

The predicates `isFMPitchEnvKnob`/`isFMDecayKnob`/`isFMTimeDomainKnob` are added in Task 2 (in `synth_focus_graph.go`, same package) — this file references `isFMPitchEnvKnob`, so Task 2 must land for the package to compile. To keep Task 1 independently compilable, ALSO add the three predicates now at the top of the appended block (Task 2 will then NOT re-add them — it only edits the router + `isSynthPitchKnob`). Add (needs `strings` import in `synth_concept_viz.go` — add it):

```go
// isFMPitchEnvKnob / isFMDecayKnob / isFMTimeDomainKnob classify the FM knobs
// whose effect is time-domain (shown by conceptFMEnvelope). Handles both the
// modular (fm_opN_decay / fm_pitch_env_*) and bespoke (fm_decN / fm_pe_*) naming.
func isFMPitchEnvKnob(name string) bool {
	switch name {
	case "fm_pitch_env_amount", "fm_pitch_env_decay", "fm_pe_amt", "fm_pe_decay":
		return true
	}
	return false
}

func isFMDecayKnob(name string) bool {
	if strings.HasPrefix(name, "fm_op") && strings.HasSuffix(name, "_decay") {
		return true
	}
	return strings.HasPrefix(name, "fm_dec")
}

func isFMTimeDomainKnob(name string) bool {
	return isFMPitchEnvKnob(name) || isFMDecayKnob(name)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestConceptFMEnvelope_RespondsToDecayAndPitchEnv`
Expected: PASS.

- [ ] **Step 5: vet**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | grep -iE 'concept_viz|fm_envelope'`
Expected: empty (ignore unrelated concurrent-WIP lines).

- [ ] **Step 6: Checkpoint** — report; do NOT commit.

---

## Task 2: Route FM time-domain + pitch + level knobs

**Files:**
- Modify: `src/go/internal/ui/synth_focus_graph.go` (`synthFocusRendererForKnob:197`, `isSynthPitchKnob:186`)
- Test: `src/go/internal/ui/synth_fm_envelope_test.go` (append)

- [ ] **Step 1: Write the failing routing test**

Append to `synth_fm_envelope_test.go`:

```go
func TestSynthFocusRendererForKnob_FMRouting(t *testing.T) {
	cases := []struct {
		name string
		want conceptRenderer
	}{
		{"fm_op2_decay", conceptFMEnvelope},
		{"fm_pitch_env_amount", conceptFMEnvelope},
		{"fm_pitch_env_decay", conceptFMEnvelope},
		{"fm_dec2", conceptFMEnvelope},
		{"fm_pe_amt", conceptFMEnvelope},
		{"fm_base_freq", conceptPitchWave},
		{"fm_base", conceptPitchWave},
		{"fm_op2_ratio", conceptFM}, // unchanged: timbre, group routing
	}
	for _, c := range cases {
		got := synthFocusRendererForKnob(audio.ParamDef{Name: c.name, Group: "fm"})
		if rendererName(got) != rendererName(c.want) {
			t.Errorf("%s routed to %s, want %s", c.name, rendererName(got), rendererName(c.want))
		}
	}
}
```

> `rendererName` already exists (added in the pitch-routing test `synth_focus_pitch_test.go`). If it's in a different test file in the same package it's reachable; if not, add the tiny helper here (reflect.ValueOf(fn).Pointer() → runtime.FuncForPC(...).Name()).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSynthFocusRendererForKnob_FMRouting`
Expected: FAIL — `fm_op2_decay` routes to `conceptFM` (group), `fm_base_freq` to `conceptFM`, not the new targets.

- [ ] **Step 3: Extend the router + pitch predicate**

In `synth_focus_graph.go`, `isSynthPitchKnob` (line ~186) currently lists pitch names. Add `fm_base_freq` and `fm_base`:

```go
func isSynthPitchKnob(name string) bool {
	switch name {
	case "osc_octave", "osc_detune", "pitch", "tune", "fm_base", "fm_base_freq", "fundamental", "base_freq":
		return true
	}
	return false
}
```

(If the existing list differs, add `fm_base` and `fm_base_freq` to it without removing existing entries.)

Then `synthFocusRendererForKnob` (line ~197):

```go
func synthFocusRendererForKnob(def audio.ParamDef) conceptRenderer {
	if isSynthPitchKnob(def.Name) {
		return conceptPitchWave
	}
	if isFMTimeDomainKnob(def.Name) {
		return conceptFMEnvelope
	}
	if def.Name == "fm_op1_level" && fmOp1IsCarrier(def) {
		return conceptFocusLevelWave
	}
	return synthFocusRendererForGroup(def.Group)
}
```

- [ ] **Step 4: Decide `fm_op1_level` carrier handling**

Add `fmOp1IsCarrier`. Investigate whether op1 is the carrier (its level = output amplitude) for the recipes exposing `fm_op1_level`. Read how the FM algorithm routes operators (`grep -rn "fm_algorithm\|carrier\|op1" internal/audio/*fm* internal/audio/modular*`). Two acceptable outcomes:

- **If op1 is reliably the carrier** (modular FM uses op1 as carrier across its algorithms): implement
  ```go
  // fmOp1IsCarrier reports whether operator 1's level maps to output amplitude
  // (carrier) so conceptFocusLevelWave is a faithful picture for fm_op1_level.
  func fmOp1IsCarrier(def audio.ParamDef) bool { return true }
  ```
  with a comment citing what you found in the engine.
- **If it's algorithm-dependent / not reliably the carrier**: make `fmOp1IsCarrier` return `false` (so `fm_op1_level` falls through to `conceptFM` and stays a sweep exemption), and document why. Do NOT show a misleading amplitude.

Report which path you took and the evidence.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthFocusRendererForKnob_FMRouting|TestConceptFMEnvelope'`
Expected: PASS.

- [ ] **Step 6: vet + checkpoint** — `go vet -tags test ./internal/ui/` clean for your files; report the `fm_op1_level` decision. Do NOT commit.

---

## Task 3: Tighten the strict sweep gate

**Files:**
- Modify: `src/go/internal/ui/synth_focus_sweep_test.go`

- [ ] **Step 1: Identify the now-closed exemptions**

Read `synth_focus_sweep_test.go`'s exemption structures (`focusSweepExemptByName` + `focusSweepExemptPair` from the redesign plan). The FM exemptions to REMOVE (now responsive): every `fm_opN_decay` (`fm_op1_decay`/`fm_op2_decay`/`fm_op3_decay`/`fm_op4_decay`), `fm_pitch_env_amount`, `fm_pitch_env_decay`, `fm_dec*`/`fm_pe_*` if present, `fm_base_freq`, `fm_base`, and `fm_op1_level` **iff Task 2 routed it** (if Task 2 kept `fmOp1IsCarrier` false, KEEP the `fm_op1_level` exemption with its reason).

KEEP: the genuinely-inert `fm-epiano / fm_op3_*` per-recipe exemption(s).

- [ ] **Step 2: Run the sweep to see what's now required**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFocusGraph_RespondsToEveryKnob -v 2>&1 | tail -40`
Expected before edits: PASS (the old exemptions still cover the now-responsive knobs, but they're stale — the bidirectional check will flag them as "EXEMPT but now RESPONSIVE" once you... actually, remove them and re-run).

- [ ] **Step 3: Remove the closed exemption entries**

Delete the identified FM-time/pitch/level entries from the exemption maps. Keep the inert `fm-epiano/op3` entry. (The bidirectional gate from the redesign already fails if an exempt knob is responsive, so leaving them would FAIL — removal is required, not optional.)

- [ ] **Step 4: Run the strict gate**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFocusGraph_RespondsToEveryKnob -v 2>&1 | grep -iE 'responsive|exempt|PASS|FAIL|ok'`
Expected: PASS, with the summary line showing a HIGHER responsive count and a LOWER exempt count (ideally just the `fm-epiano/op3` inert case, plus `fm_op1_level` only if Task 2 kept it). If any FM-time knob is still flat (unexpected), investigate — likely a naming-scheme mismatch in `isFMDecayKnob`/`fmPitchEnvParams`; fix the predicate/helper (Task 1/2), not by re-adding the exemption.

- [ ] **Step 5: Checkpoint** — report the before/after responsive+exempt counts and the final exemption list. Do NOT commit.

---

## Task 4: Scenes + visual verification

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go`, `src/go/internal/ui/synth_focus_scene_test.go`

- [ ] **Step 1: Add the crop scenes**

In `scene_catalog.go`, next to the existing `crop_synth_focus_*` entries, add two using the existing `synthTabFocusKnobSetup` + `mobileSynthTabFocusKnobSetup` helpers (copy the exact struct fields `Name`/`Description`/`Subject`/`Setup`/`MobileSetup` from a neighbouring entry):

```go
	{Name: "crop_synth_focus_fm_decay", Description: "FM operator decay schematic", Subject: SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "FM", "fm_op2_decay"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "FM", "fm_op2_decay")},
	{Name: "crop_synth_focus_fm_penv", Description: "FM pitch-env schematic", Subject: SubjectSynthPanel,
		Setup:       synthTabFocusKnobSetup("modular", "FM", "fm_pitch_env_amount"),
		MobileSetup: mobileSynthTabFocusKnobSetup("modular", "FM", "fm_pitch_env_amount")},
```

Verify the modular FM stage actually exposes `fm_op2_decay` and `fm_pitch_env_amount` as grid knobs (the focus setup falls back to the section main knob otherwise). If the modular recipe doesn't expose pitch-env, use a bespoke FM recipe that does (e.g. `fm-bell`) for the `_fm_penv` scene — pick one whose FM stage has `fm_pitch_env_amount`, confirmed via `audio.RecipeRegistrations()`.

- [ ] **Step 2: Add scene guard rows**

In `synth_focus_scene_test.go`, extend `TestSynthFocusSceneSelectsIntendedKnob` (or its data table) with rows asserting `crop_synth_focus_fm_decay`→`fm_op2_decay` and `crop_synth_focus_fm_penv`→`fm_pitch_env_amount` actually select that knob (so a silent fallback fails). Mirror the existing rows' shape.

- [ ] **Step 3: Run scene tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSceneCrop|TestSynthFocusScene'`
Expected: PASS (both new scenes produce a visible Subject rect; the guard confirms knob selection).

- [ ] **Step 4: Capture + visually confirm**

Run (repo root): `make screenshots-all SCENES=crop_synth_focus_fm_decay,crop_synth_focus_fm_penv` then `make screenshots-all MOBILE=1 SCENES=crop_synth_focus_fm_decay,crop_synth_focus_fm_penv`
Open the PNGs under `screenshots/`. Confirm: the focus graph shows a **decay curve** (high→low exponential fall) for `crop_synth_focus_fm_decay` and a **pitch sweep** (bipolar spike decaying to center) for `crop_synth_focus_fm_penv`; on mobile the always-on band shows them above the grid. Report what you saw per image. If the harness can't run here (no display), say so — Step 3's scene test is the gating check.

- [ ] **Step 5: Final regression + vet**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'SynthFocus|FocusGraph|ConceptFM|SceneCrop|TestFocusGraph_RespondsToEveryKnob'`
Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | head -5`
Expected: PASS; vet clean for your files.

- [ ] **Step 6: Final checkpoint** — summarize: new responsive/exempt counts, the `fm_op1_level` decision, screenshots confirmed. Hand the working tree to the user. Do NOT commit.

---

## Self-Review — spec coverage

| Spec requirement | Task |
|---|---|
| `conceptFMEnvelope` schematic decay curve (`fm_op*_decay`/`fm_dec*`) | 1 |
| `conceptFMEnvelope` schematic pitch-env curve (`fm_pitch_env_*`/`fm_pe_*`) | 1 |
| Reads both modular + bespoke naming | 1 (`fmPitchEnvParams`, `isFMDecayKnob`) |
| Ghost overlay | 1 |
| Route FM-time knobs → `conceptFMEnvelope` | 2 |
| `fm_base_freq`/`fm_base` → pitch wave | 2 |
| `fm_op1_level` → level wave iff carrier (else exempt) | 2 (`fmOp1IsCarrier`) |
| Strict sweep drops closed exemptions; keeps inert op3 | 3 |
| Bidirectional gate stays green | 3 |
| Renderer responsiveness tests (TDD) | 1 |
| Routing tests | 2 |
| Scenes + desktop/mobile visual confirm | 4 |
