# Knob Interaction Overhaul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make discrete synth knobs snap "clicky" to fixed positions, turn every continuous knob into an endless encoder with a user-tappable per-knob step multiplier, and give every numeric value (all tabs) BPM-style click-to-type entry.

**Architecture:** Add opt-in behavior fields to the existing `Knob` widget (`Discrete`/`Endless`/`Scale`/`StepMul`) so bare `NewKnob` keeps today's absolute-drag behavior and existing tests pass. A new `KnobStepBadge` cycles a per-param step ladder; a new shared `ParamValueEditor` (one active at a time, owned by DrumView) wraps the existing `TextInput` for numeric entry. Persistence reuses the `AudioPanelStore` / `marshalPrefsV5` pattern bumped to V6.

**Tech Stack:** Go 1.23, Ebiten, the bundled toolchain at `.tools/go/bin/go`. Fast test path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`.

## Repo conventions baked into this plan

- **Never `git add` / `git commit`** (project rule). Each task ends with a **Checkpoint** step that runs the relevant fast tests and leaves changes in the working tree. The user owns all commits.
- All Go commands use `.tools/go/bin/go` from `src/go`.
- Forbidden glyphs in `internal/ui/`: use `IconID` + `DrawIcon`; single-char text labels OK; `×` is allowed only as a label character in step badges via the multiplication sign — **use the ASCII letter `x` instead** to stay clear of the glyph lint (e.g. `x10`, or unit form `10 Hz`).
- New `color.RGBA`/`NRGBA` literals are budgeted by `token_discipline_test.go`. Reuse `Token*()` accessors — do **not** introduce new color literals.

## File structure

**New files (`src/go/internal/ui/`):**
- `knob_step.go` — `KnobStepBadge`, `stepLadder`, `niceStep`, `defaultStepIndex`, `formatStepValue`.
- `knob_step_test.go` — ladder + badge unit tests.
- `param_value_editor.go` — `ParamValueEditor`, `formatParamForEntry`, `parseParamEntry`, `trimFloat`.
- `param_value_editor_test.go` — format/parse unit tests.
- `knob_interaction_functional_test.go` — functional tests driving the real input loop.

**Modified:**
- `internal/audio/synth_recipe.go` — add `Step float64` to `ParamDef`.
- `internal/audio/modular_recipe.go` — mark `osc_octave` with `Step:1`.
- `internal/ui/knob.go` — discrete/endless modes, `Scale`, `StepMul`, detent rendering.
- `internal/ui/synth_panel_zone.go` — set knob `Scale`/modes, badge, readout-tap → editor.
- `internal/ui/sampler_panel_zone.go`, `internal/ui/sampler_state.go` — sampler knob scales + wiring.
- `internal/ui/eq_panel_zone.go`, `internal/ui/slider.go` + chain slider sites — readout-tap → editor.
- `internal/userprefs/store.go`, `store_js.go`, `store_notjs.go` — `KnobStepStore`, `marshalPrefsV6`.
- `DESIGN.md` + generated tokens; `design_md_drift_test.go`, `token_discipline_test.go`.

---

## Phase 1 — ParamDef.Step + Knob discrete "clicky" mode

### Task 1: Add `Step` to `ParamDef`

**Files:**
- Modify: `src/go/internal/audio/synth_recipe.go:18-33`
- Modify: `src/go/internal/audio/modular_recipe.go` (the `osc_octave` ParamDef)
- Test: `src/go/internal/audio/synth_recipe_test.go` (new test func appended)

- [ ] **Step 1: Write the failing test**

Append to `synth_recipe_test.go`:

```go
func TestParamDefStepFieldRoundTrips(t *testing.T) {
	def := ParamDef{Name: "osc_octave", Min: -2, Max: 2, Default: 0, Step: 1}
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"step":1`) {
		t.Fatalf("expected step in JSON, got %s", b)
	}
	// Zero step must be omitted (v1 goldens unaffected).
	z := ParamDef{Name: "cutoff", Min: 20, Max: 20000}
	bz, _ := json.Marshal(z)
	if strings.Contains(string(bz), "step") {
		t.Fatalf("zero step must be omitted, got %s", bz)
	}
	var back ParamDef
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Step != 1 {
		t.Fatalf("step round-trip = %v, want 1", back.Step)
	}
}
```

Ensure the test file imports `encoding/json` and `strings`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run TestParamDefStepFieldRoundTrips`
Expected: FAIL — `def.Step` undefined.

- [ ] **Step 3: Add the field**

In `synth_recipe.go`, add to the `ParamDef` struct after `Enum`:

```go
	// Step (v2, additive + omitempty) marks a non-enum param as DISCRETE with
	// detents at Min, Min+Step, ... Max. Enum params are implicitly Step:1.
	// A zero Step serializes identically to v1 (no "step" key), so existing
	// recipe goldens are unaffected.
	Step float64 `json:"step,omitempty"`
```

- [ ] **Step 4: Mark `osc_octave` discrete**

In `modular_recipe.go`, find the `osc_octave` ParamDef (`Min:-2, Max:2`) and add `Step: 1` to its literal. Leave all other params unchanged.

- [ ] **Step 5: Run the test + the audio golden suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'TestParamDefStepFieldRoundTrips|Golden|Recipe'`
Expected: PASS (no golden churn — `Step:1` only affects `osc_octave`; goldens store rendered audio, not ParamDef JSON; if a recipe-schema golden exists and fails, regenerate per its documented update flag and confirm the only diff is `osc_octave`'s `"step":1`).

- [ ] **Step 6: Checkpoint** — leave changes in the working tree (no commit).

---

### Task 2: Knob `Scale` + `Discrete` snapping (drag)

**Files:**
- Modify: `src/go/internal/ui/knob.go`
- Test: `src/go/internal/ui/knob_discrete_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `knob_discrete_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func newDiscreteKnob(enum []string) *Knob {
	k := NewKnob(0)
	k.SetRect(image.Rect(0, 0, 100, 100))
	k.Scale = KnobScale{Min: 0, Max: float64(len(enum) - 1), Enum: enum}
	k.Discrete = true
	return k
}

func TestDiscreteKnobSnapsToDetents(t *testing.T) {
	enum := []string{"Sine", "Saw", "Square", "Triangle"} // 4 detents
	k := newDiscreteKnob(enum)
	// Press at left edge, drag slowly right; Value must only ever be one of
	// {0, 1/3, 2/3, 1} — never an in-between fraction.
	k.HandleInputResult(2, 50, true) // press
	allowed := map[float64]bool{0: true, 1.0 / 3: true, 2.0 / 3: true, 1: true}
	for x := 2; x <= 98; x += 3 {
		k.HandleInputResult(x, 50, true)
		if !nearAnyDetent(k.Value, 4) {
			t.Fatalf("x=%d value=%v not on a detent", x, k.Value)
		}
	}
	_ = allowed
	k.HandleInputResult(98, 50, false) // release
	if !nearAnyDetent(k.Value, 4) {
		t.Fatalf("final value=%v not on a detent", k.Value)
	}
}

func TestDiscreteKnobDetentCount(t *testing.T) {
	k := NewKnob(0)
	k.Scale = KnobScale{Min: -2, Max: 2, Step: 1} // 5 detents
	k.Discrete = true
	if got := k.detentCount(); got != 5 {
		t.Fatalf("detentCount=%d want 5", got)
	}
	k2 := newDiscreteKnob([]string{"a", "b", "c"})
	if got := k2.detentCount(); got != 3 {
		t.Fatalf("enum detentCount=%d want 3", got)
	}
}

// nearAnyDetent reports whether v sits (within float slop) on one of the n
// evenly spaced detents in [0,1].
func nearAnyDetent(v float64, n int) bool {
	for i := 0; i < n; i++ {
		d := float64(i) / float64(n-1)
		if v-d < 1e-9 && d-v < 1e-9 {
			return true
		}
	}
	return false
}

var _ = audio.ParamDef{}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDiscreteKnob'`
Expected: FAIL — `KnobScale`, `k.Scale`, `k.Discrete`, `k.detentCount` undefined.

- [ ] **Step 3: Implement `KnobScale`, fields, snapping**

In `knob.go`, add above the `Knob` struct:

```go
// KnobScale describes the param domain a Knob represents so the widget can
// snap discrete params to detents, accumulate endless drags in real units,
// and feed the step badge / numeric editor. Set by the owning panel; the
// zero value leaves the knob in legacy continuous-absolute mode.
type KnobScale struct {
	Min, Max float64
	Unit     string   // "", "ms", "Hz", "dB", "%", "st"
	Enum     []string // non-nil => discrete
	Step     float64  // >0 => discrete detents at Min, Min+Step, ... Max
}
```

Add fields to `Knob` (after `ZeroFrac`):

```go
	// Scale describes the param domain (see KnobScale). Discrete/Endless are
	// derived from it by the owning panel and set explicitly.
	Scale KnobScale
	// Discrete makes drag/wheel snap to evenly spaced detents (clicky). Set
	// when Scale.Enum != nil || Scale.Step > 0.
	Discrete bool
	// Endless makes drag accumulate incrementally in real units (StepMul per
	// knobEndlessPxPerNotch px) instead of mapping the rect to 0..1, so the
	// turn never hits a travel wall. Mutually exclusive with Discrete.
	Endless bool
	// StepMul is the real-unit change applied per knobEndlessPxPerNotch px of
	// endless drag and per wheel notch. Set from the step badge.
	StepMul float64
	// endlessLastX is the previous drag X for incremental accumulation.
	endlessLastX int
```

Add the constant near the others:

```go
	// knobEndlessPxPerNotch is the horizontal pixels that equal one StepMul of
	// value change in endless mode. 8 px = one step gives fine finger control.
	knobEndlessPxPerNotch = 8
```

Add helpers (place after `dragPixels`):

```go
// detentCount returns the number of discrete positions, or 0 if continuous.
func (k *Knob) detentCount() int {
	if len(k.Scale.Enum) > 0 {
		return len(k.Scale.Enum)
	}
	if k.Scale.Step > 0 && k.Scale.Max > k.Scale.Min {
		return int(math.Round((k.Scale.Max-k.Scale.Min)/k.Scale.Step)) + 1
	}
	return 0
}

// snapToDetent rounds a normalized [0,1] value to the nearest detent.
func (k *Knob) snapToDetent(v float64) float64 {
	n := k.detentCount()
	if n < 2 {
		return v
	}
	idx := int(math.Round(v * float64(n-1)))
	if idx < 0 {
		idx = 0
	} else if idx > n-1 {
		idx = n - 1
	}
	return float64(idx) / float64(n-1)
}

// clampRange clamps x to [lo,hi].
func clampRange(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}
```

Modify `updateFromDrag` to branch:

```go
func (k *Knob) updateFromDrag(mx, my int) {
	if k.Endless {
		k.updateEndless(mx)
		return
	}
	pixels := k.dragPixels()
	dxPixels := mx - k.pressX // right = positive
	v := k.pressVal + float64(dxPixels)/float64(pixels)
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if k.Discrete {
		v = k.snapToDetent(v)
	}
	k.Value = v
}

// updateEndless accumulates an incremental drag in real units so the knob has
// no travel limit; the arc still fills proportionally to value-in-range.
func (k *Knob) updateEndless(mx int) {
	span := k.Scale.Max - k.Scale.Min
	if span <= 0 {
		return
	}
	step := k.StepMul
	if step <= 0 {
		step = span / 200
	}
	d := mx - k.endlessLastX
	k.endlessLastX = mx
	real := k.Scale.Min + k.Value*span
	real = clampRange(real+(float64(d)/float64(knobEndlessPxPerNotch))*step, k.Scale.Min, k.Scale.Max)
	k.Value = (real - k.Scale.Min) / span
}
```

In `HandleInputResult`, in the press branch where the drag starts (after `k.pressVal = k.Value`), add:

```go
				k.endlessLastX = mx
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDiscreteKnob'`
Expected: PASS.

- [ ] **Step 5: Verify legacy knob behavior is unchanged**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Knob'`
Expected: PASS (existing `knob_test.go`, `knob_bipolar_test.go`, etc. — bare `NewKnob` has `Discrete=false, Endless=false`).

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

### Task 3: Endless drag + discrete/endless wheel

**Files:**
- Modify: `src/go/internal/ui/knob.go` (`HandleWheel`)
- Test: `src/go/internal/ui/knob_endless_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `knob_endless_test.go`:

```go
package ui

import (
	"image"
	"testing"
)

func newEndlessKnob() *Knob {
	k := NewKnob(0) // value 0 => real == Min
	k.SetRect(image.Rect(0, 0, 100, 100))
	k.Scale = KnobScale{Min: 20, Max: 20000, Unit: "Hz"}
	k.Endless = true
	k.StepMul = 100 // 100 Hz per notch
	return k
}

func realOf(k *Knob) float64 {
	return k.Scale.Min + k.Value*(k.Scale.Max-k.Scale.Min)
}

func TestEndlessKnobAccumulatesInRealUnits(t *testing.T) {
	k := newEndlessKnob()
	k.HandleInputResult(50, 50, true) // press at x=50, real=20
	// Drag right by 8 px (one notch) => +100 Hz.
	k.HandleInputResult(58, 50, true)
	if got := realOf(k); got < 119.999 || got > 120.001 {
		t.Fatalf("after +1 notch real=%v want ~120", got)
	}
	// Continue dragging right 80 px (10 notches) => +1000 Hz.
	k.HandleInputResult(138, 50, true)
	if got := realOf(k); got < 1119.99 || got > 1120.01 {
		t.Fatalf("after +11 notches real=%v want ~1120", got)
	}
}

func TestEndlessKnobClampsThenReversesImmediately(t *testing.T) {
	k := newEndlessKnob()
	k.HandleInputResult(50, 50, true)
	// Drag far left to clamp at Min.
	k.HandleInputResult(-5000, 50, true)
	if realOf(k) > 20.0001 {
		t.Fatalf("expected clamp at Min, got %v", realOf(k))
	}
	// Now one notch right must leave the clamp immediately (no dead zone).
	k.HandleInputResult(-4992, 50, true)
	if got := realOf(k); got < 119.99 || got > 120.01 {
		t.Fatalf("reverse-off-clamp real=%v want ~120", got)
	}
}

func TestEndlessKnobWheelStepsByStepMul(t *testing.T) {
	k := newEndlessKnob()
	if r := k.HandleWheel(50, 50, 1); r != InputConsumed {
		t.Fatalf("wheel result=%v want consumed", r)
	}
	if got := realOf(k); got < 119.99 || got > 120.01 {
		t.Fatalf("wheel +1 real=%v want ~120", got)
	}
}

func TestDiscreteKnobWheelStepsOneIndex(t *testing.T) {
	k := newDiscreteKnob([]string{"a", "b", "c", "d"}) // 4 detents
	if r := k.HandleWheel(50, 50, 1); r != InputConsumed {
		t.Fatalf("wheel result=%v want consumed", r)
	}
	if !nearAnyDetent(k.Value, 4) || k.Value < 0.33 || k.Value > 0.34 {
		t.Fatalf("wheel +1 idx value=%v want 1/3", k.Value)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEndlessKnob|TestDiscreteKnobWheel'`
Expected: FAIL — wheel does not yet honor discrete/endless.

- [ ] **Step 3: Implement discrete/endless wheel**

Replace the body of `HandleWheel` in `knob.go` (keep the rect guard and signature):

```go
func (k *Knob) HandleWheel(mx, my, steps int) InputResult {
	if !image.Pt(mx, my).In(k.r) {
		return InputIgnored
	}
	if k.Discrete {
		n := k.detentCount()
		if n >= 2 {
			idx := int(math.Round(k.Value*float64(n-1))) + steps
			if idx < 0 {
				idx = 0
			} else if idx > n-1 {
				idx = n - 1
			}
			nv := float64(idx) / float64(n-1)
			if nv == k.Value {
				return InputIgnored
			}
			k.Value = nv
			return InputConsumed
		}
	}
	if k.Endless {
		span := k.Scale.Max - k.Scale.Min
		if span > 0 && k.StepMul > 0 {
			real := clampRange(k.Scale.Min+k.Value*span+float64(steps)*k.StepMul, k.Scale.Min, k.Scale.Max)
			nv := (real - k.Scale.Min) / span
			if nv == k.Value {
				return InputIgnored
			}
			k.Value = nv
			return InputConsumed
		}
	}
	step := knobWheelStep
	if k.FineDrag {
		step = knobWheelStepFine
	}
	v := k.Value + float64(steps)*step
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if v == k.Value {
		return InputIgnored
	}
	k.Value = v
	return InputConsumed
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEndlessKnob|TestDiscreteKnob'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

### Task 4: Draw detent tick marks for discrete knobs

**Files:**
- Modify: `src/go/internal/ui/knob.go` (`Draw`)
- Test: `src/go/internal/ui/knob_discrete_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `knob_discrete_test.go`:

```go
func TestDiscreteKnobDrawsDetentTicks(t *testing.T) {
	k := newDiscreteKnob([]string{"a", "b", "c", "d", "e"}) // 5 detents
	var calls int
	restore := swapDetentTickDrawerForTest(func(_ float64) { calls++ })
	defer restore()
	img := newTrackedImage("test.knob", 100, 100)
	defer releaseImage(img)
	k.Draw(img)
	if calls != 5 {
		t.Fatalf("detent ticks drawn=%d want 5", calls)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDiscreteKnobDrawsDetentTicks'`
Expected: FAIL — `swapDetentTickDrawerForTest` undefined.

- [ ] **Step 3: Implement detent drawing with a test seam**

In `knob.go`, add a package-level seam + drawer:

```go
// detentTickDrawer is the per-detent draw hook, swappable in tests to count
// detents without reading pixels.
var detentTickDrawer = func(angleRad float64) {}

func swapDetentTickDrawerForTest(fn func(float64)) func() {
	prev := detentTickDrawer
	detentTickDrawer = fn
	return func() { detentTickDrawer = prev }
}

// drawDetents strokes a short radial tick at each detent position around the
// arc, so a discrete knob visibly reads as N fixed stops.
func (k *Knob) drawDetents(dst *ebiten.Image, cx, cy, radius float64, strokeWidth float32, col color.Color) {
	n := k.detentCount()
	if n < 2 {
		return
	}
	start := knobArcStartDeg * math.Pi / 180
	sweep := knobArcSweepDeg * math.Pi / 180
	innerR := radius - float64(strokeWidth)*1.6
	outerR := radius + float64(strokeWidth)*0.2
	if innerR < 1 {
		innerR = 1
	}
	for i := 0; i < n; i++ {
		a := start + sweep*float64(i)/float64(n-1)
		detentTickDrawer(a)
		ix := cx + math.Cos(a)*innerR
		iy := cy + math.Sin(a)*innerR
		ox := cx + math.Cos(a)*outerR
		oy := cy + math.Sin(a)*outerR
		vector.StrokeLine(dst, float32(ix), float32(iy), float32(ox), float32(oy), 1.5, col, true)
	}
}
```

In `Draw`, after the value-arc block and before the indicator line, add:

```go
	if k.Discrete {
		k.drawDetents(dst, cx, cy, float64(radius), strokeWidth, TokenTextSecondary())
	}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDiscreteKnobDrawsDetentTicks'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

## Phase 2 — Step ladder + step badge

### Task 5: Step ladder math

**Files:**
- Create: `src/go/internal/ui/knob_step.go`
- Test: `src/go/internal/ui/knob_step_test.go`

- [ ] **Step 1: Write the failing test**

Create `knob_step_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestNiceStepRounds(t *testing.T) {
	cases := map[float64]float64{0.011: 0.01, 0.04: 0.05, 0.4: 0.5, 1.1: 1, 3: 2, 7: 5, 9: 10, 90: 100}
	for in, want := range cases {
		if got := niceStep(in); got != want {
			t.Fatalf("niceStep(%v)=%v want %v", in, got, want)
		}
	}
}

func TestStepLadderForHz(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	ladder := stepLadder(def)
	if len(ladder) < 2 {
		t.Fatalf("ladder too short: %v", ladder)
	}
	// Strictly increasing.
	for i := 1; i < len(ladder); i++ {
		if ladder[i] <= ladder[i-1] {
			t.Fatalf("ladder not increasing: %v", ladder)
		}
	}
	// Default rung yields a nuanced ~200-1000 notches over the span.
	def2 := def
	idx := defaultStepIndex(def2, ladder)
	notches := (def.Max - def.Min) / ladder[idx]
	if notches < 100 || notches > 2000 {
		t.Fatalf("default rung notches=%v out of nuanced band", notches)
	}
}

func TestFormatStepValue(t *testing.T) {
	if got := formatStepValue(audio.ParamDef{Unit: "Hz"}, 100); got != "100 Hz" {
		t.Fatalf("got %q", got)
	}
	if got := formatStepValue(audio.ParamDef{Unit: ""}, 0.1); got != "x0.1" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestNiceStep|TestStepLadder|TestFormatStepValue'`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement the ladder math**

Create `knob_step.go`:

```go
package ui

import (
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// niceStep rounds x up to the nearest 1/2/5 x 10^k "nice" number.
func niceStep(x float64) float64 {
	if x <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(x))
	pow := math.Pow(10, exp)
	switch f := x / pow; {
	case f <= 1:
		return pow
	case f <= 2:
		return 2 * pow
	case f <= 5:
		return 5 * pow
	default:
		return 10 * pow
	}
}

// stepLadder derives 4 increasing step rungs (fine -> coarse) for a param,
// spanning roughly span/2000 (very fine) to span/16 (coarse).
func stepLadder(def audio.ParamDef) []float64 {
	span := def.Max - def.Min
	if span <= 0 {
		return []float64{1}
	}
	divisors := []float64{2000, 400, 80, 16}
	out := make([]float64, 0, len(divisors))
	for _, d := range divisors {
		s := niceStep(span / d)
		if len(out) == 0 || s > out[len(out)-1] {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = []float64{niceStep(span / 400)}
	}
	return out
}

// defaultStepIndex selects the rung whose notch-count is closest to the
// "nuanced" target (~400 notches across the span).
func defaultStepIndex(def audio.ParamDef, ladder []float64) int {
	span := def.Max - def.Min
	if span <= 0 || len(ladder) == 0 {
		return 0
	}
	best, bestErr := 0, math.Inf(1)
	for i, s := range ladder {
		notches := span / s
		if e := math.Abs(notches - 400); e < bestErr {
			best, bestErr = i, e
		}
	}
	return best
}

// formatStepValue renders a step for the badge: "100 Hz" with a unit, or
// "x0.1" when unitless (ASCII x, not the multiplication glyph — glyph lint).
func formatStepValue(def audio.ParamDef, step float64) string {
	s := trimFloat(step)
	if def.Unit == "" {
		return "x" + s
	}
	return s + " " + def.Unit
}

// trimFloat formats a float without trailing zeros.
func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

var _ = image.Rectangle{}
var _ = ebiten.Image{}
```

(The `image`/`ebiten` imports are used by Task 6 in this same file; the `var _` lines keep Step-3 compiling and are removed in Task 6.)

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestNiceStep|TestStepLadder|TestFormatStepValue'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

### Task 6: `KnobStepBadge` component

**Files:**
- Modify: `src/go/internal/ui/knob_step.go`
- Test: `src/go/internal/ui/knob_step_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `knob_step_test.go`:

```go
func TestKnobStepBadgeCyclesAndReports(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	b := NewKnobStepBadge(def)
	first := b.Step()
	if first <= 0 {
		t.Fatalf("step not positive: %v", first)
	}
	n := len(b.ladder)
	for i := 0; i < n; i++ {
		b.Cycle()
	}
	if b.Step() != first {
		t.Fatalf("cycling %d times must return to start; got %v want %v", n, b.Step(), first)
	}
	if b.Label() == "" {
		t.Fatalf("label empty")
	}
}

func TestKnobStepBadgeSetStepSnapsToNearestRung(t *testing.T) {
	def := audio.ParamDef{Name: "x", Min: 0, Max: 1000, Unit: ""}
	b := NewKnobStepBadge(def)
	b.SetStep(b.ladder[len(b.ladder)-1] * 1.01) // slightly above coarsest
	if b.idx != len(b.ladder)-1 {
		t.Fatalf("SetStep idx=%d want %d", b.idx, len(b.ladder)-1)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestKnobStepBadge'`
Expected: FAIL — `NewKnobStepBadge` undefined.

- [ ] **Step 3: Implement the badge**

In `knob_step.go`, remove the two `var _ =` lines and append:

```go
// KnobStepBadge is the small tappable pill under a continuous knob showing the
// current step (real units, e.g. "100 Hz"). Tapping cycles a fixed ladder of
// rungs. The chosen rung drives the knob's StepMul and is persisted by name.
type KnobStepBadge struct {
	def    audio.ParamDef
	ladder []float64
	idx    int
	r      image.Rectangle
}

func NewKnobStepBadge(def audio.ParamDef) *KnobStepBadge {
	l := stepLadder(def)
	return &KnobStepBadge{def: def, ladder: l, idx: defaultStepIndex(def, l)}
}

func (b *KnobStepBadge) Step() float64 {
	if b.idx < 0 || b.idx >= len(b.ladder) {
		return 1
	}
	return b.ladder[b.idx]
}

func (b *KnobStepBadge) Cycle()                     { b.idx = (b.idx + 1) % len(b.ladder) }
func (b *KnobStepBadge) Label() string              { return formatStepValue(b.def, b.Step()) }
func (b *KnobStepBadge) SetRect(r image.Rectangle)  { b.r = r }
func (b *KnobStepBadge) Rect() image.Rectangle      { return b.r }

// SetStep snaps the badge to the rung nearest to v (used to restore a
// persisted step).
func (b *KnobStepBadge) SetStep(v float64) {
	best, bestErr := 0, math.Inf(1)
	for i, s := range b.ladder {
		if e := math.Abs(s - v); e < bestErr {
			best, bestErr = i, e
		}
	}
	b.idx = best
}

// Draw renders the pill with the current step label.
func (b *KnobStepBadge) Draw(dst *ebiten.Image) {
	if b.r.Empty() {
		return
	}
	drawRoundedRect(dst, b.r, TokenSurface2(), RadiusXS, true)
	drawRoundedRect(dst, b.r, TokenBorderSubtle(), RadiusXS, false)
	lbl := b.Label()
	tw := TextWidth(lbl)
	tx := b.r.Min.X + (b.r.Dx()-tw)/2
	ty := b.r.Min.Y + (b.r.Dy()-TextHeight())/2
	DrawTextColorAt(dst, lbl, tx, ty, TokenTextSecondary())
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestKnobStepBadge'`
Expected: PASS. If `TokenSurface2`, `RadiusXS`, or `drawRoundedRect` names differ, grep `theme_tokens.go` / `drawing.go` and use the exact accessor — do not add color literals.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

## Phase 3 — Shared numeric editor

### Task 7: `formatParamForEntry` / `parseParamEntry`

**Files:**
- Create: `src/go/internal/ui/param_value_editor.go`
- Test: `src/go/internal/ui/param_value_editor_test.go`

- [ ] **Step 1: Write the failing test**

Create `param_value_editor_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestParseParamEntryContinuousClamps(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	if v, ok := parseParamEntry(def, "  8042 "); !ok || v != 8042 {
		t.Fatalf("got v=%v ok=%v", v, ok)
	}
	if v, ok := parseParamEntry(def, "99999"); !ok || v != 20000 {
		t.Fatalf("clamp-high v=%v ok=%v", v, ok)
	}
	if v, ok := parseParamEntry(def, "0"); !ok || v != 20 {
		t.Fatalf("clamp-low v=%v ok=%v", v, ok)
	}
	if _, ok := parseParamEntry(def, "abc"); ok {
		t.Fatalf("expected parse failure")
	}
	if _, ok := parseParamEntry(def, "NaN"); ok {
		t.Fatalf("expected NaN rejection")
	}
}

func TestParseParamEntryEnumByLabelOrIndex(t *testing.T) {
	def := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Triangle"}}
	if v, ok := parseParamEntry(def, "square"); !ok || v != 2 {
		t.Fatalf("label match v=%v ok=%v", v, ok)
	}
	if v, ok := parseParamEntry(def, "3"); !ok || v != 3 {
		t.Fatalf("index match v=%v ok=%v", v, ok)
	}
	if _, ok := parseParamEntry(def, "9"); ok {
		t.Fatalf("out-of-range index must fail")
	}
}

func TestFormatParamForEntryRoundTrips(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	s := formatParamForEntry(def, 8042)
	if v, ok := parseParamEntry(def, s); !ok || v != 8042 {
		t.Fatalf("round-trip s=%q v=%v ok=%v", s, v, ok)
	}
	enumDef := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Triangle"}}
	if got := formatParamForEntry(enumDef, 2); got != "Square" {
		t.Fatalf("enum format got %q", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParseParamEntry|TestFormatParamForEntry'`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement format/parse**

Create `param_value_editor.go`:

```go
package ui

import (
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// formatParamForEntry renders a param value for the numeric editor: the enum
// label for discrete params, else a bare number (no unit suffix).
func formatParamForEntry(def audio.ParamDef, v float64) string {
	if lbl := enumLabelFor(def, v); lbl != "" {
		return lbl
	}
	return trimFloat(v)
}

// parseParamEntry parses editor text into a param value. Continuous: parse +
// clamp to [Min,Max], reject non-finite. Discrete: case-insensitive enum-label
// match OR an in-range integer index.
func parseParamEntry(def audio.ParamDef, text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	if len(def.Enum) > 0 {
		for i, l := range def.Enum {
			if strings.EqualFold(l, text) {
				return float64(i), true
			}
		}
		if n, err := strconv.Atoi(text); err == nil && n >= 0 && n < len(def.Enum) {
			return float64(n), true
		}
		return 0, false
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if f < def.Min {
		f = def.Min
	} else if f > def.Max {
		f = def.Max
	}
	return f, true
}

var _ = image.Rectangle{}
var _ = ebiten.Image{}
```

(The `image`/`ebiten` imports are used by Task 8 in this same file; the `var _` lines keep Step-3 compiling and are removed in Task 8.)

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParseParamEntry|TestFormatParamForEntry'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

### Task 8: `ParamValueEditor` widget (one active at a time)

**Files:**
- Modify: `src/go/internal/ui/param_value_editor.go`
- Test: `src/go/internal/ui/param_value_editor_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `param_value_editor_test.go`:

```go
func TestParamValueEditorCommitsValidValue(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	var wrote float64 = -1
	ed := NewParamValueEditor()
	ed.Open(def, image.Rect(0, 0, 80, 24), func() float64 { return 1000 }, func(v float64) { wrote = v })
	if !ed.Active() {
		t.Fatalf("editor should be active after Open")
	}
	ed.ti.SetText("8042")
	ed.commit() // simulate blur/Enter
	if wrote != 8042 {
		t.Fatalf("wrote=%v want 8042", wrote)
	}
	if ed.Active() {
		t.Fatalf("editor should close after commit")
	}
}

func TestParamValueEditorRevertsInvalid(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	var wrote float64 = -1
	ed := NewParamValueEditor()
	ed.Open(def, image.Rect(0, 0, 80, 24), func() float64 { return 1000 }, func(v float64) { wrote = v })
	ed.ti.SetText("garbage")
	ed.commit()
	if wrote != -1 {
		t.Fatalf("invalid input must not write; wrote=%v", wrote)
	}
	if ed.errorAnim <= 0 {
		t.Fatalf("invalid input must flag error anim")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParamValueEditor'`
Expected: FAIL — `NewParamValueEditor` undefined.

- [ ] **Step 3: Implement the editor**

In `param_value_editor.go`, remove the two `var _ =` lines and append:

```go
// ParamValueEditor is the single active numeric-entry box shared across tabs.
// It wraps a TextInput, anchored over a tapped value readout. Commit on
// Enter/blur parses + clamps via parseParamEntry and calls setValue; invalid
// input reverts and flashes errorAnim (BPM parity).
type ParamValueEditor struct {
	ti        *TextInput
	def       audio.ParamDef
	setValue  func(float64)
	active    bool
	errorAnim float64
}

func NewParamValueEditor() *ParamValueEditor {
	ti := NewTextInput(image.Rectangle{}, BPMBoxStyle)
	ti.InputMode = "numeric"
	ti.MaxLen = 12
	return &ParamValueEditor{ti: ti}
}

func (e *ParamValueEditor) Active() bool { return e.active }

// Open focuses the editor over anchor, pre-filled with the current value.
func (e *ParamValueEditor) Open(def audio.ParamDef, anchor image.Rectangle, getValue func() float64, setValue func(float64)) {
	e.def = def
	e.setValue = setValue
	e.ti.Rect = anchor
	e.ti.SetText(formatParamForEntry(def, getValue()))
	e.ti.SetFocus(true)
	e.active = true
}

// commit parses the text; on success writes the value and closes, on failure
// reverts (no write) and flashes the error.
func (e *ParamValueEditor) commit() {
	if v, ok := parseParamEntry(e.def, e.ti.Value()); ok {
		if e.setValue != nil {
			e.setValue(v)
		}
		e.close()
		return
	}
	e.errorAnim = 1
	e.close()
}

func (e *ParamValueEditor) close() {
	e.active = false
	e.ti.SetFocus(false)
}

// Update drives the wrapped TextInput; when focus is lost (Enter or click
// outside) it commits. Call once per frame before other input while active.
func (e *ParamValueEditor) Update() {
	if !e.active {
		if e.errorAnim > 0 {
			e.errorAnim *= 0.85
			if e.errorAnim < 0.01 {
				e.errorAnim = 0
			}
		}
		return
	}
	wasFocused := e.ti.Focused()
	e.ti.Update()
	if wasFocused && !e.ti.Focused() {
		e.commit()
	}
}

// Draw renders the editor box (and error flash) when active. Drawn last so it
// floats over the readout.
func (e *ParamValueEditor) Draw(dst *ebiten.Image) {
	if !e.active {
		return
	}
	e.ti.Draw(dst)
	if e.errorAnim > 0 {
		drawRect(dst, e.ti.Rect, fadeColor(colError, e.errorAnim), false)
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParamValueEditor'`
Expected: PASS. If `BPMBoxStyle`, `fadeColor`, or `colError` names differ, grep and substitute.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

## Phase 4 — Wire synth knobs

### Task 9: Build a `KnobScale` from a synth `ParamDef`

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go`
- Test: `src/go/internal/ui/synth_panel_zone_test.go` (or nearest synth test file; append)

- [ ] **Step 1: Write the failing test**

Append to an existing synth test file in `internal/ui/`:

```go
func TestScaleFromParamDefMarksDiscrete(t *testing.T) {
	enum := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Tri"}}
	sc, discrete, endless := scaleFromParamDef(enum)
	if !discrete || endless {
		t.Fatalf("enum must be discrete, not endless (d=%v e=%v)", discrete, endless)
	}
	if len(sc.Enum) != 4 {
		t.Fatalf("scale enum len=%d", len(sc.Enum))
	}
	cont := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	sc2, d2, e2 := scaleFromParamDef(cont)
	if d2 || !e2 {
		t.Fatalf("continuous must be endless, not discrete (d=%v e=%v)", d2, e2)
	}
	if sc2.Unit != "Hz" || sc2.Max != 20000 {
		t.Fatalf("scale not carried: %+v", sc2)
	}
	oct := audio.ParamDef{Name: "osc_octave", Min: -2, Max: 2, Step: 1}
	_, d3, e3 := scaleFromParamDef(oct)
	if !d3 || e3 {
		t.Fatalf("Step>0 must be discrete (d=%v e=%v)", d3, e3)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestScaleFromParamDef'`
Expected: FAIL — `scaleFromParamDef` undefined.

- [ ] **Step 3: Implement the helper**

In `synth_panel_zone.go` (near the other knob helpers), add:

```go
// scaleFromParamDef builds the Knob's KnobScale and behavior flags from a
// ParamDef. Enum or Step>0 => discrete (clicky); otherwise endless.
func scaleFromParamDef(def audio.ParamDef) (sc KnobScale, discrete, endless bool) {
	sc = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit, Enum: def.Enum, Step: def.Step}
	discrete = len(def.Enum) > 0 || def.Step > 0
	endless = !discrete
	return
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestScaleFromParamDef'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

### Task 10: Apply scale + behavior + step badge at synth knob creation

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go:797-801` (knob creation) and the `DrumView` struct (add `instEditorStepBadges []*KnobStepBadge`)
- Test: `src/go/internal/ui/knob_interaction_functional_test.go` (new — Step 1 below also covers Task 11/13)

- [ ] **Step 1: Write the failing functional test**

Create `knob_interaction_functional_test.go`. This drives the real synth tab. Use the existing synth-tab test harness helpers (grep an existing synth functional test, e.g. `synth_chip_strip_*_test.go`, for the exact constructor — reuse `newSynthTestGame`/`openSynthTab` style helpers already present; the snippet below names them generically and MUST be adapted to the real helper names found in the package):

```go
func TestSynthContinuousKnobIsEndlessWithBadge(t *testing.T) {
	dv, instID := newSynthTabForTest(t) // adapt to real helper
	selectSynthSectionForTest(t, dv, "filter")
	idx := synthKnobIndexForParam(t, dv, "filter_cutoff")
	k := dv.instEditorKnobs[idx]
	if !k.Endless || k.Discrete {
		t.Fatalf("cutoff knob should be endless, not discrete")
	}
	if idx >= len(dv.instEditorStepBadges) || dv.instEditorStepBadges[idx] == nil {
		t.Fatalf("cutoff knob has no step badge")
	}
	if k.StepMul <= 0 {
		t.Fatalf("cutoff knob StepMul not initialised")
	}
	_ = instID
}

func TestSynthDiscreteKnobIsClicky(t *testing.T) {
	dv, _ := newSynthTabForTest(t)
	selectSynthSectionForTest(t, dv, "osc")
	idx := synthKnobIndexForParam(t, dv, "osc_type")
	k := dv.instEditorKnobs[idx]
	if !k.Discrete || k.Endless {
		t.Fatalf("osc_type knob should be discrete clicky")
	}
}
```

If no such helpers exist, add minimal ones in this file that: construct a `Game` via the existing fast-path test constructor, open the EQ panel's Synth tab via `SetActiveTab(TabSynth)` + a layout pump, resolve the active synth instrument, and expose `dv.instEditorKnobs`/`instEditorBindings` to locate a param by `def.Name`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthContinuousKnobIsEndless|TestSynthDiscreteKnobIsClicky'`
Expected: FAIL — `instEditorStepBadges` undefined; knob modes unset.

- [ ] **Step 3: Implement creation wiring**

In the `DrumView` struct (grep `instEditorKnobs []*Knob` for the declaration block), add:

```go
	instEditorStepBadges []*KnobStepBadge
```

At the knob-creation site (`synth_panel_zone.go:797-801`), replace:

```go
	k := NewKnob(initial)
	k.Bipolar = bipolar
	k.ZeroFrac = zeroFrac
	dv.instEditorKnobs = append(dv.instEditorKnobs, k)
	dv.instEditorSliders = append(dv.instEditorSliders, NewSlider(initial))
```

with:

```go
	k := NewKnob(initial)
	k.Bipolar = bipolar
	k.ZeroFrac = zeroFrac
	sc, discrete, endless := scaleFromParamDef(def)
	k.Scale = sc
	k.Discrete = discrete
	k.Endless = endless
	var badge *KnobStepBadge
	if endless {
		badge = NewKnobStepBadge(def)
		if persisted, ok := dv.knobStepPref(def.Name); ok {
			badge.SetStep(persisted)
		}
		k.StepMul = badge.Step()
	}
	dv.instEditorKnobs = append(dv.instEditorKnobs, k)
	dv.instEditorStepBadges = append(dv.instEditorStepBadges, badge)
	dv.instEditorSliders = append(dv.instEditorSliders, NewSlider(initial))
```

Add a temporary stub for `knobStepPref` (real implementation lands in Task 16) near the synth helpers:

```go
// knobStepPref returns a persisted step for a param name. Wired to userprefs
// in Task 16; returns (0,false) until then.
func (dv *DrumView) knobStepPref(name string) (float64, bool) { return 0, false }
```

Ensure every other place that appends to `instEditorKnobs` (grep it) also appends to `instEditorStepBadges` (append `nil` for non-knob slots) so the slices stay index-aligned. Reset `instEditorStepBadges` wherever `instEditorKnobs` is reset to empty.

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthContinuousKnobIsEndless|TestSynthDiscreteKnobIsClicky'`
Expected: PASS.

- [ ] **Step 5: Run the synth-tab regression set**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Synth'`
Expected: PASS (account for the ~14 pre-existing concurrent chip-strip Synth-tab failures noted in project memory — diff against the baseline before blaming this change; do not "fix" unrelated pre-existing failures).

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

### Task 11: Draw + hit-test the step badge; cycle on tap

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (the detail-pane knob loop ~1713-1746 for draw + layout; `handleSynthTabInput` ~1930-1940 for hit-test)
- Test: `knob_interaction_functional_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestSynthStepBadgeTapCyclesStep(t *testing.T) {
	dv, instID := newSynthTabForTest(t)
	selectSynthSectionForTest(t, dv, "filter")
	idx := synthKnobIndexForParam(t, dv, "filter_cutoff")
	badge := dv.instEditorStepBadges[idx]
	// Force a layout so the badge has a rect.
	pumpSynthLayout(t, dv)
	before := badge.Step()
	r := badge.Rect()
	if r.Empty() {
		t.Fatalf("badge rect empty after layout")
	}
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	dv.handleSynthTabInput(cx, cy, true, instID)
	if badge.Step() == before {
		t.Fatalf("step did not cycle on tap")
	}
	if dv.instEditorKnobs[idx].StepMul != badge.Step() {
		t.Fatalf("knob StepMul not updated after badge cycle")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthStepBadgeTapCyclesStep'`
Expected: FAIL — badge has no rect / no hit handling.

- [ ] **Step 3: Layout + draw + hit-test the badge**

In the detail-pane knob loop (after the caption is drawn for index `j`/`kIdx`), add badge layout + draw:

```go
		if kIdx < len(dv.instEditorStepBadges) {
			if badge := dv.instEditorStepBadges[kIdx]; badge != nil {
				bw := Profile().DensityValues().KnobStepBadgeW
				bh := Profile().DensityValues().KnobStepBadgeH
				bx := captionR.Min.X + (captionR.Dx()-bw)/2
				by := captionY + TextHeight() + SpaceXS
				badge.SetRect(image.Rect(bx, by, bx+bw, by+bh))
				badge.Draw(dst)
			}
		}
```

(`KnobStepBadgeW`/`KnobStepBadgeH` density tokens are added in Task 17; until then substitute literals `44` and `18` and replace in Task 17.)

In `handleSynthTabInput`, **before** the knob hit-test loop (so a badge tap is not swallowed by the knob's full-rect press), add:

```go
	// Step badge tap: cycle the resolution, never starts a knob drag.
	for i, badge := range dv.instEditorStepBadges {
		if badge == nil || badge.Rect().Empty() || !image.Pt(x, y).In(badge.Rect()) {
			continue
		}
		badge.Cycle()
		if i < len(dv.instEditorKnobs) {
			dv.instEditorKnobs[i].StepMul = badge.Step()
		}
		dv.persistKnobStep(badge) // stub until Task 16; see below
		return true
	}
```

Add a temporary stub near `knobStepPref`:

```go
// persistKnobStep saves a badge's chosen step. Wired to userprefs in Task 16.
func (dv *DrumView) persistKnobStep(b *KnobStepBadge) {}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthStepBadgeTapCyclesStep'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

### Task 12: Readout-tap opens the numeric editor on synth knobs

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (struct: add `paramEditor *ParamValueEditor`; detail-pane loop stores per-knob caption rects; `handleSynthTabInput`; the panel Update + Draw to pump the editor)
- Test: `knob_interaction_functional_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestSynthReadoutTapOpensEditorAndCommits(t *testing.T) {
	dv, instID := newSynthTabForTest(t)
	selectSynthSectionForTest(t, dv, "filter")
	pumpSynthLayout(t, dv)
	idx := synthKnobIndexForParam(t, dv, "filter_cutoff")
	rr := dv.synthKnobReadoutRect(idx)
	if rr.Empty() {
		t.Fatalf("readout rect empty")
	}
	cx, cy := rr.Min.X+rr.Dx()/2, rr.Min.Y+rr.Dy()/2
	dv.handleSynthTabInput(cx, cy, true, instID)
	if dv.paramEditor == nil || !dv.paramEditor.Active() {
		t.Fatalf("editor not opened on readout tap")
	}
	dv.paramEditor.ti.SetText("5000")
	dv.paramEditor.commit()
	got := audio.GetInstrumentParams(instID)["filter_cutoff"]
	if got < 4999 || got > 5001 {
		t.Fatalf("param after edit=%v want ~5000", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthReadoutTapOpensEditorAndCommits'`
Expected: FAIL — `paramEditor`, `synthKnobReadoutRect` undefined.

- [ ] **Step 3: Implement readout rect + editor open + pump**

Add fields to `DrumView`:

```go
	paramEditor       *ParamValueEditor
	instEditorCapRects []image.Rectangle // per-knob readout (caption) rect, index-aligned
```

In the detail-pane knob loop, record the caption rect for each `kIdx` (grow/assign `instEditorCapRects` so it is index-aligned with `instEditorKnobs`; reset it where the knob slice is reset). The readout rect is the caption line you already compute:

```go
		readoutR := image.Rect(captionR.Min.X, captionY, captionR.Max.X, captionY+TextHeight())
		dv.setKnobReadoutRect(kIdx, readoutR)
```

Add:

```go
// synthKnobReadoutRect returns the tappable readout rect for a knob index.
func (dv *DrumView) synthKnobReadoutRect(idx int) image.Rectangle {
	if idx < 0 || idx >= len(dv.instEditorCapRects) {
		return image.Rectangle{}
	}
	return dv.instEditorCapRects[idx]
}

func (dv *DrumView) setKnobReadoutRect(idx int, r image.Rectangle) {
	for len(dv.instEditorCapRects) <= idx {
		dv.instEditorCapRects = append(dv.instEditorCapRects, image.Rectangle{})
	}
	dv.instEditorCapRects[idx] = r
}

// openSynthParamEditor opens the shared numeric editor over a knob's readout.
func (dv *DrumView) openSynthParamEditor(idx int, instID string) {
	if idx < 0 || idx >= len(dv.instEditorBindings) {
		return
	}
	if dv.paramEditor == nil {
		dv.paramEditor = NewParamValueEditor()
	}
	def := dv.instEditorBindings[idx].def
	anchor := dv.synthKnobReadoutRect(idx)
	dv.paramEditor.Open(def, anchor,
		func() float64 {
			return def.Min + dv.instEditorKnobs[idx].Value*(def.Max-def.Min)
		},
		func(v float64) {
			// Normalize back to 0..1, set knob + slider, propagate (config-only).
			span := def.Max - def.Min
			if span <= 0 {
				span = 1
			}
			dv.instEditorKnobs[idx].Value = (v - def.Min) / span
			dv.syncSliderFromKnob(idx)
			dv.propagateSynthSliderValue(idx, instID)
		})
}
```

In `handleSynthTabInput`, **before** the knob hit-test loop (and before/after the badge loop — readout sits below the dial, badge below the readout; order the three loops badge → readout → knob so the dial's full-rect press is the last resort), add a readout loop:

```go
	for i := range dv.instEditorKnobs {
		rr := dv.synthKnobReadoutRect(i)
		if rr.Empty() || !image.Pt(x, y).In(rr) {
			continue
		}
		dv.openSynthParamEditor(i, instID)
		return true
	}
```

Pump the editor: in the synth panel's per-frame Update path (grep where `handleSynthTabInput` is called from / where the synth tab updates each frame), add at the top, before other synth input is processed:

```go
	if dv.paramEditor != nil && dv.paramEditor.Active() {
		dv.paramEditor.Update()
		return // editor owns input this frame
	}
	if dv.paramEditor != nil {
		dv.paramEditor.Update() // drains error-anim decay
	}
```

And draw it last in the synth tab Draw (after `drawSynthFooter`):

```go
	if dv.paramEditor != nil {
		dv.paramEditor.Draw(dst)
	}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthReadoutTapOpensEditorAndCommits'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

## Phase 5 — Wire sampler knobs

### Task 13: Sampler knob scales (endless + numeric entry)

**Files:**
- Modify: `src/go/internal/ui/sampler_state.go` (add `samplerKnobScale`), `sampler_panel_zone.go:270` (apply scale + badge + editor)
- Test: `src/go/internal/ui/sampler_knob_interaction_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `sampler_knob_interaction_test.go`:

```go
package ui

import "testing"

func TestSamplerKnobScalesAreEndless(t *testing.T) {
	for _, idx := range []int{samplerKnobStart, samplerKnobEnd, samplerKnobTranspose, samplerKnobDetune, samplerKnobGain} {
		sc := samplerKnobScale(idx)
		if sc.Max <= sc.Min {
			t.Fatalf("knob %d scale invalid: %+v", idx, sc)
		}
	}
	// Transpose is in semitones over a symmetric range.
	st := samplerKnobScale(samplerKnobTranspose)
	if st.Unit != "st" {
		t.Fatalf("transpose unit=%q want st", st.Unit)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerKnobScales'`
Expected: FAIL — `samplerKnobScale` undefined.

- [ ] **Step 3: Implement sampler scales + wiring**

In `sampler_state.go`, add (using the existing range constants `samplerTransposeRange`, `samplerDetuneRange`, `samplerGainMinDB`, `samplerGainMaxDB`):

```go
// samplerKnobScale returns the real-unit domain for a sampler knob so the
// endless drag, step badge, and numeric editor operate in real units. The
// normalized knob Value still maps through setKnob/knobValue.
func samplerKnobScale(idx int) KnobScale {
	switch idx {
	case samplerKnobStart, samplerKnobEnd:
		return KnobScale{Min: 0, Max: 100, Unit: "%"}
	case samplerKnobTranspose:
		return KnobScale{Min: -samplerTransposeRange, Max: samplerTransposeRange, Unit: "st"}
	case samplerKnobDetune:
		return KnobScale{Min: -samplerDetuneRange, Max: samplerDetuneRange, Unit: "cents"}
	case samplerKnobGain:
		return KnobScale{Min: samplerGainMinDB, Max: samplerGainMaxDB, Unit: "dB"}
	}
	return KnobScale{Min: 0, Max: 1}
}
```

In `sampler_panel_zone.go:270` (knob creation), after `s.knobs[i] = NewKnob(s.knobValue(i))` and the existing bipolar wiring, add:

```go
		s.knobs[i].Scale = samplerKnobScale(i)
		s.knobs[i].Endless = true
		s.knobStepBadges[i] = NewKnobStepBadge(audio.ParamDef{
			Name: samplerStepPrefName(i), Min: s.knobs[i].Scale.Min, Max: s.knobs[i].Scale.Max, Unit: s.knobs[i].Scale.Unit,
		})
		s.knobs[i].StepMul = s.knobStepBadges[i].Step()
```

Add a `knobStepBadges [samplerKnobCount]*KnobStepBadge` field to `samplerState` (grep for the existing `knobs [...]` array declaration to match its sizing constant), and a helper:

```go
func samplerStepPrefName(idx int) string { return "sampler." + samplerGroupLabel(idx) }
```

(If `samplerGroupLabel` returns "" for some indices, fall back to a per-index name like `fmt.Sprintf("sampler.knob%d", idx)`.)

The endless drag uses the normalized `Value` directly, so the existing `samplerKnobHitAdapter.OnDrag` → `setKnob(i, knob.Value)` path keeps working — the endless math in `updateFromDrag` writes `knob.Value`, and `setKnob` maps it to real units exactly as before.

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerKnobScales'`
Expected: PASS.

- [ ] **Step 5: Run the sampler regression set**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sampler'`
Expected: PASS.

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

### Task 14: Sampler badge draw/hit + readout-tap editor

**Files:**
- Modify: `src/go/internal/ui/sampler_panel_zone.go` (knob draw loop ~470 for badge + readout rect; `samplerKnobHitAdapter.OnPress` ~1042 for badge/readout interception; sampler Update/Draw to pump `dv.paramEditor`)
- Test: `sampler_knob_interaction_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append (adapt helper names to the real sampler test harness — grep `sampler_*_test.go`):

```go
func TestSamplerReadoutTapOpensEditor(t *testing.T) {
	dv, s := newSamplerTabForTest(t) // adapt
	pumpSamplerLayout(t, dv)
	rr := s.knobReadoutRect(samplerKnobGain)
	if rr.Empty() {
		t.Fatalf("gain readout rect empty")
	}
	cx, cy := rr.Min.X+rr.Dx()/2, rr.Min.Y+rr.Dy()/2
	dv.handleSamplerTabInput(cx, cy, true) // adapt to real dispatch entry
	if dv.paramEditor == nil || !dv.paramEditor.Active() {
		t.Fatalf("editor not opened on sampler readout tap")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerReadoutTapOpensEditor'`
Expected: FAIL.

- [ ] **Step 3: Implement sampler badge + readout wiring**

Mirror Task 11/12 in the sampler:
- In the knob draw loop (`sampler_panel_zone.go:~470`), lay out + draw `s.knobStepBadges[i]` under each knob caption, and record `s.knobReadoutRects[i]` = the caption rect. Add `knobReadoutRects [samplerKnobCount]image.Rectangle` + accessor `knobReadoutRect(idx)`.
- In `samplerKnobHitAdapter.OnPress` (or the sampler input dispatch that precedes knob hit-testing), check badge rects (cycle + set `StepMul` + `dv.persistKnobStep`) and readout rects (open `dv.paramEditor` with `samplerKnobScale(i)` as a synthetic `ParamDef`, get/set via `s.knobValue(i)` / `s.setKnob` + the existing propagate path) before falling through to the knob drag.
- Pump `dv.paramEditor.Update()`/`.Draw()` in the sampler tab's Update/Draw exactly as Task 12 did for synth (the editor field is shared on `DrumView`, so only the pump call sites are added).

For the editor get/set on the sampler, convert between real units and normalized via the scale:

```go
	sc := samplerKnobScale(i)
	get := func() float64 { return sc.Min + s.knobValue(i)*(sc.Max-sc.Min) }
	set := func(v float64) {
		span := sc.Max - sc.Min
		if span <= 0 { span = 1 }
		s.setKnob(i, (v-sc.Min)/span)
		s.knobs[i].Value = s.knobValue(i)
		s.commitEdit() // adapt: the existing path that pushes SampleEdit to audio
	}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerReadoutTapOpensEditor'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

## Phase 6 — Numeric entry for EQ + Chain sliders

### Task 15: Slider readout-tap opens the numeric editor

**Files:**
- Modify: `src/go/internal/ui/slider.go` (expose a readout/label rect accessor), `eq_panel_zone.go` + chain slider sites (open `dv.paramEditor` on label tap), and the slider hit dispatch
- Test: `src/go/internal/ui/slider_numeric_entry_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `slider_numeric_entry_test.go`:

```go
package ui

import (
	"image"
	"testing"
)

func TestSliderLabelRectIsReported(t *testing.T) {
	s := NewSlider(0.5)
	s.SetRect(image.Rect(0, 0, 200, 24))
	lr := s.LabelRect()
	if lr.Empty() {
		t.Fatalf("label rect empty")
	}
	if !lr.In(s.Rect()) && !lr.Overlaps(s.Rect()) {
		t.Fatalf("label rect %v not within slider %v", lr, s.Rect())
	}
}
```

(Grep `slider.go` for the existing label-draw geometry; `LabelRect()` must return the same rect the `%` label is drawn into. If the slider already exposes a method that yields this, reuse it and assert on that instead.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSliderLabelRect'`
Expected: FAIL — `LabelRect` undefined.

- [ ] **Step 3: Implement `LabelRect` + wire EQ/Chain**

In `slider.go`, add a `LabelRect()` method returning the rect the `%` label occupies (compute from the same geometry used in `Draw` at line ~173). Then, in the EQ panel and chain slider input dispatch, before the slider's own drag press, hit-test `slider.LabelRect()`; on hit open `dv.paramEditor` with a `ParamDef` describing that slider's real range/unit (EQ band gain: `Min:-24, Max:24, Unit:"dB"` — confirm against the EQ model's actual gain range; chain params per their definitions) and get/set closures that read/write the slider value and push through the existing EQ/chain setter (`applyMasterEQ`/`applyRowEQ` for EQ).

Add a focused functional test that opens the editor on an EQ band label tap and commits a dB value, asserting the EQ gain updates (adapt to the EQ test harness; grep `eq_*_test.go`).

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSlider|EQ.*Numeric|Chain.*Numeric'`
Expected: PASS.

- [ ] **Step 5: Run EQ + Chain regression sets**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'EQ|Chain'`
Expected: PASS.

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

## Phase 7 — Persistence

### Task 16: `KnobStepStore` (userprefs V6) + wire badge persistence

**Files:**
- Modify: `src/go/internal/userprefs/store.go` (interface + `marshalPrefsV6`), `store_js.go`, `store_notjs.go`
- Modify: `src/go/internal/ui/synth_panel_zone.go` (replace the Task 10/11 stubs `knobStepPref`/`persistKnobStep` with real userprefs calls)
- Test: `src/go/internal/userprefs/knob_step_store_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `knob_step_store_test.go` (follow the existing `audio_panel_state_test.go` round-trip style):

```go
package userprefs

import "testing"

func TestKnobStepStoreRoundTrips(t *testing.T) {
	steps := map[string]float64{"filter_cutoff": 100, "amp_attack": 0.01}
	b, err := marshalPrefsV6(nil, nil, nil, AudioPanelStateDoc{}, nil, nil, steps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := parseKnobStepsFromPrefs(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got["filter_cutoff"] != 100 || got["amp_attack"] != 0.01 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/userprefs/ -run 'TestKnobStepStore'`
Expected: FAIL — `marshalPrefsV6`/`parseKnobStepsFromPrefs` undefined.

- [ ] **Step 3: Implement V6 + store methods**

In `store.go`:
- Add a `knobSteps map[string]float64` section to `prefsDoc` (JSON key `"knob_steps"`, omitempty).
- Add `marshalPrefsV6` that extends `marshalPrefsV5` with the `knob_steps` section (mirror how `marshalPrefsV5` extends V4 at `store.go:313-318`), and `parseKnobStepsFromPrefs([]byte) (map[string]float64, error)`.
- Add to the store interface:

```go
	LoadKnobSteps() map[string]float64
	SaveKnobStep(name string, step float64) error
```

In `store_notjs.go` and `store_js.go`: hold a `knobSteps map[string]float64` snapshot, implement `LoadKnobSteps`/`SaveKnobStep` mirroring `SaveAudioPanelState`/`SaveSampleEdit` (atomic write on notjs, localStorage on js), and switch the `marshalPrefsV5(...)` call sites to `marshalPrefsV6(..., s.knobSteps)`.

In `synth_panel_zone.go`, replace the Task-10/11 stubs:

```go
func (dv *DrumView) knobStepPref(name string) (float64, bool) {
	if dv.prefs == nil { // use the DrumView's existing userprefs store handle; grep its field name
		return 0, false
	}
	if v, ok := dv.prefs.LoadKnobSteps()[name]; ok {
		return v, true
	}
	return 0, false
}

func (dv *DrumView) persistKnobStep(b *KnobStepBadge) {
	if dv.prefs == nil || b == nil {
		return
	}
	_ = dv.prefs.SaveKnobStep(b.def.Name, b.Step())
}
```

(Grep `DrumView` for the existing userprefs store field — it is the same handle used for `SaveAudioPanelState`/sample edits. Use that exact field name in place of `dv.prefs`.)

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/userprefs/ -run 'TestKnobStepStore'`
Expected: PASS.

- [ ] **Step 5: Run userprefs + a WASM-build smoke**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/userprefs/`
Then verify the js build tag compiles: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/userprefs/`
Expected: both PASS (the `store_js.go` change must compile under js).

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

## Phase 8 — Design tokens

### Task 17: Add badge / detent / editor dimension tokens

**Files:**
- Modify: `DESIGN.md` (YAML `densities:` block — add for all three densities), then regenerate
- Modify: `src/go/internal/ui/design_md_drift_test.go` want-map; `token_discipline_test.go` if any literal counts changed
- Modify: synth/sampler badge layout to read `Profile().DensityValues().KnobStepBadgeW/H` (replace the Task-11 literals)

- [ ] **Step 1: Add tokens to DESIGN.md**

Under each of `densities.compact|comfortable|spacious`, add (compact / comfortable / spacious example values):

```yaml
    knobStepBadgeW: 38   # 38 / 44 / 52
    knobStepBadgeH: 16   # 16 / 18 / 22
```

- [ ] **Step 2: Regenerate tokens**

Run: `make gen-design-tokens`
Expected: `internal/ui/design_*.gen.go` updated with `KnobStepBadgeW`/`KnobStepBadgeH` on the density values struct. (If gopls shows stale "undefined", ignore — the build is authoritative; per project memory.)

- [ ] **Step 3: Replace literals + update drift want-map**

In the synth (Task 11) and sampler (Task 14) badge layout, replace the literal `44`/`18` with `Profile().DensityValues().KnobStepBadgeW`/`.KnobStepBadgeH`.

If `design_md_drift_test.go` enumerates density fields, add `KnobStepBadgeW`/`KnobStepBadgeH` to its want-map per the token-change checklist.

- [ ] **Step 4: Run the token guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/`
Expected: PASS. (`TestTokenDiscipline` fails if a new color literal slipped in — there should be none; if the badge added one, route it through a `Token*()` accessor.)

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

## Phase 9 — Regression guards + full verification

### Task 18: No-auto-audition + legacy-knob invariants hold

**Files:**
- Test: `src/go/internal/ui/knob_interaction_functional_test.go` (append)

- [ ] **Step 1: Write the invariant tests**

Append:

```go
func TestNumericCommitDoesNotAudition(t *testing.T) {
	dv, instID := newSynthTabForTest(t)
	restore := SwapSynthAuditionFnForTest(func(string) { t.Fatalf("numeric commit must not audition") })
	defer restore()
	selectSynthSectionForTest(t, dv, "filter")
	pumpSynthLayout(t, dv)
	idx := synthKnobIndexForParam(t, dv, "filter_cutoff")
	dv.openSynthParamEditor(idx, instID)
	dv.paramEditor.ti.SetText("5000")
	dv.paramEditor.commit()
}

func TestEndlessKnobDragDoesNotAudition(t *testing.T) {
	dv, instID := newSynthTabForTest(t)
	restore := SwapSynthAuditionFnForTest(func(string) { t.Fatalf("knob drag must not audition") })
	defer restore()
	selectSynthSectionForTest(t, dv, "filter")
	pumpSynthLayout(t, dv)
	idx := synthKnobIndexForParam(t, dv, "filter_cutoff")
	r := dv.instEditorKnobs[idx].Rect()
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	dv.handleSynthTabInput(cx, cy, true, instID)
	dv.handleSynthTabInput(cx+40, cy, true, instID)
	dv.handleSynthTabInput(cx+40, cy, false, instID)
}
```

- [ ] **Step 2: Run to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'DoesNotAudition'`
Expected: PASS (the audition fn is only called by the Preview button). Also run the existing `synth_no_auto_audition_test.go`:
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'NoAutoAudition|AutoAudition'`
Expected: PASS.

- [ ] **Step 3: Checkpoint** — leave changes in the working tree.

---

### Task 19: Full fast-suite + cross-platform smoke

**Files:** none (verification only)

- [ ] **Step 1: Run the full UI + audio + userprefs fast suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ ./internal/userprefs/`
Expected: PASS, **except** pre-existing failures already on this branch. Per project memory, the concurrent chip-strip redesign owns ~14 Synth-tab failures and the `internal/ui` full suite can panic/timeout pre-existingly on `add-core-node-types` — **diff against a clean `git stash`-free baseline run captured BEFORE this work** (do not stash; capture the baseline by running the suite on the unmodified tree first if not already known). Only failures introduced by this change must be fixed.

- [ ] **Step 2: WASM bridge + parity smoke (knobs flow through SetInstrumentParam unchanged)**

Run:
```
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_parity.browser.test.js
```
Expected: PASS — no new JS export was added (numeric entry and stepping are pure UI; values still reach the engine via `audio.SetInstrumentParam`). If `xplat_parity` regresses, the `osc_octave` `Step:1` change is the only audio-facing edit — confirm its rendered output is unchanged.

- [ ] **Step 3: Final Checkpoint** — leave all changes in the working tree (no commit). Summarize what changed and which pre-existing failures were confirmed unrelated.

---

## Self-review notes (for the implementer)

- **Spec coverage:** clicky discrete (Tasks 2,4,9,10), endless + step badge (Tasks 3,5,6,10,11,13,14), numeric entry all tabs (Tasks 7,8,12,14,15), persistence (Task 16), tokens (Task 17), no-audition guard (Task 18). All spec sections map to a task.
- **Type consistency:** `KnobScale`/`Knob.Scale`/`Discrete`/`Endless`/`StepMul`/`endlessLastX` (Task 2/3) are reused verbatim in 9–14. `KnobStepBadge` API (`Step`/`Cycle`/`SetStep`/`Label`/`SetRect`/`Rect`/`Draw`) defined in Task 6 and used in 10/11/13/14/16. `ParamValueEditor` (`Open`/`commit`/`Update`/`Draw`/`Active`/`ti`) defined in Task 8, used in 12/14/15/18. `scaleFromParamDef` (Task 9) used in Task 10. `knobStepPref`/`persistKnobStep` stubbed in Task 10/11, replaced in Task 16. `samplerKnobScale` (Task 13) used in Task 14.
- **Adapt-to-harness flags:** the functional tests in Tasks 10–15 name generic helpers (`newSynthTabForTest`, `selectSynthSectionForTest`, `synthKnobIndexForParam`, `pumpSynthLayout`, `newSamplerTabForTest`, etc.). The implementer MUST grep the existing `internal/ui` synth/sampler/eq functional tests and reuse the real helper names (or add thin equivalents) rather than inventing parallel harnesses — see [[feedback_functional_tests_over_isolation]].
