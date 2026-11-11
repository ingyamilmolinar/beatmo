# Synth Inline Armed Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the synth tab's popup/in-place-wheel control editing with a unified inline model — click/tap a control to arm it (grows in place), edit with two modern flat inertial wheels (left = resolution, right = value), click/tap again to lock; when not armed, wheel/swipe over a control scrolls the row smoothly.

**Architecture:** A new `BarrelWheel` widget owns only kinetic state (velocity, sub-detent offset, settle) and reads/writes the value model through a `barrelBinding` interface, so the existing `*Knob` (value) and `*KnobStepBadge` (resolution ladder) stay the single source of truth. An `ArmedControlEditor` pairs two `BarrelWheel`s for the currently-armed control and routes input to the hovered lane. The synth `ControlGrid` switches from row-by-row stepped scrolling to the existing continuous-pixel + momentum path already present on `ScrollBehavior`. The barrel popup is dropped from the synth path (kept only for Sampler/EQ).

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), Ebiten 2.9, `-tags test` fast path with `-modfile=go.test.mod`, ebitenstub for headless UI tests, drawRect-interceptor pixel tests, xvfb screenshot harness.

**Spec:** `docs/superpowers/specs/2026-07-22-synth-inline-armed-controls-design.md`

## Global Constraints

- Bundled Go only: `.tools/go/bin/go` (never system go). Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`.
- **Never stage or commit unless the user explicitly asks** (project standing rule). The "Commit" steps below stage nothing — replace each with running the task's tests green and reporting; do the actual `git commit` only on explicit user request. (Kept as checklist markers, not executed.)
- **Mandatory UI screenshot verification**: no UI task is "done" until a rendered PNG (desktop AND `MOBILE=1`) has been opened and inspected. Iterate on the visuals until pristine.
- **No cached `*Knob`/`*KnobStepBadge` pointers across frames** — `buildSynthTab` rebuilds `instEditorControls`/`instEditorStepBadges`/`instEditorBindings` at any Layout. Always resolve by index against `dv.instEditorControls[idx]` etc. (mirrors `synthControlHitAdapter.currentKnob()`).
- Index-aligned slices: `instEditorControls[]*Knob`, `instEditorStepBadges[]*KnobStepBadge` (nil for discrete params), `instEditorBindings[]instParamBinding` (`.def audio.ParamDef`). All declared in `drumview.go:444-447`.
- Direction convention (pin on real device in Task 14): **drag/scroll DOWN → value INCREASES; resolution lane DOWN → coarser (bigger step number)**. Anchor tests to this; verify on the user's uncommitted WASM build.
- Design tokens are density-tied; read via `Profile().DensityValues()`. Do not add `runtime.GOOS=="js"` checks — this feature is platform-neutral except layout, gated via `Profile().IsMobile()`.
- Token forbidden glyphs still apply (use `IconID` + `DrawIcon`, not raw chevrons). Reuse `WithAlpha`/`WithAlphaFromColor` buckets; no new color tokens.

---

## File Structure

**New files (all `src/go/internal/ui/`):**
- `barrel_wheel.go` — `BarrelWheel` kinetic widget + `barrelBinding` interface. (~180 LOC)
- `barrel_wheel_test.go` — inertia/detent/clamp unit tests.
- `synth_barrel_bindings.go` — `synthValueBinding` + `synthResolutionBinding` (resolve-by-index). (~120 LOC)
- `synth_barrel_bindings_test.go`.
- `synth_armed_control.go` — armed-state accessors on DrumView (`instEditorArmedControl` map; `armSynthControl`/`lockSynthControl`/`synthControlArmed`). (~90 LOC)
- `synth_armed_control_test.go`.
- `synth_armed_editor.go` — `ArmedControlEditor`: pairs two `BarrelWheel`s, lane layout, hovered-lane routing, tick, keyboard, numeric-entry hook. (~220 LOC)
- `synth_armed_editor_test.go`.
- `synth_barrel_render.go` — modern flat lane renderer (`drawBarrelLane`). (~120 LOC)
- `synth_barrel_render_test.go` — drawRect-interceptor pixel test.

**Modified files:**
- `DESIGN.md` (×3 density blocks) + regenerated `internal/ui/design_*.gen.go` — 4 new density tokens.
- `control_grid.go:264-274` — swap stepped drag/wheel wrappers for continuous + momentum; add `UpdateMomentum`/`HasMomentum` passthrough; keep thumb-drag direct.
- `synth_panel_zone.go` — armed card height in `layoutSynthSections`/`placeKnobsInSection`; branch `synthControlHitAdapter` handlers on armed state (`:3695-3761`); keyboard + tick in `synthTabUpdate` (`:1914`); numeric-entry reuse; drop synth popup open.
- `synth_selected_control.go` — sibling to selected-control (optional home for armed accessors if preferred over new file; plan uses new file).
- `drumview.go` — declare `instEditorArmedControl map[string]int` + `instEditorArmedEditor *ArmedControlEditor`.
- `drumview_wheel_popup.go` — remove synth open path (Sampler/EQ untouched).
- `scene_catalog.go` — `crop_synth_control_armed` scene (+ `MobileSetup`).

---

### Task 1: Density tokens for the armed editor

**Files:**
- Modify: `DESIGN.md` (the three `densities:` blocks — compact / comfortable / spacious)
- Regenerate: `src/go/internal/ui/design_*.gen.go` (via `make gen-design-tokens`)
- Test: `src/go/internal/ui/synth_armed_tokens_test.go` (Create)

**Interfaces:**
- Produces: `Profile().DensityValues().SynthArmedCardH`, `.SynthWheelLaneW`, `.SynthWheelRowH`, `.SynthWheelVisibleRows` — all `int`.

- [ ] **Step 1: Add the four tokens to each density block in `DESIGN.md`**

Mirror the existing `synthControlIdeal` sizing token exactly (same YAML shape, kebab-case key). Under `densities.compact`, `densities.comfortable`, `densities.spacious` add:

```yaml
    synthArmedCardH: 104        # compact
    synthWheelLaneW: 48
    synthWheelRowH: 22
    synthWheelVisibleRows: 5
```
```yaml
    synthArmedCardH: 120        # comfortable (desktop default)
    synthWheelLaneW: 54
    synthWheelRowH: 26
    synthWheelVisibleRows: 5
```
```yaml
    synthArmedCardH: 148        # spacious (mobile default)
    synthWheelLaneW: 64
    synthWheelRowH: 32
    synthWheelVisibleRows: 5
```

- [ ] **Step 2: Regenerate tokens**

Run: `make gen-design-tokens`
Expected: `internal/ui/design_*.gen.go` now contains the four `SynthArmedCardH`/`SynthWheelLaneW`/`SynthWheelRowH`/`SynthWheelVisibleRows` fields on the density struct; build stays green.

- [ ] **Step 3: Write the token-read test**

```go
package ui

import "testing"

func TestSynthArmedTokensPresent(t *testing.T) {
	for _, d := range []Density{DensityCompact, DensityComfortable, DensitySpacious} {
		restore := SetDensityForTest(d)
		v := Profile().DensityValues()
		if v.SynthArmedCardH <= 0 || v.SynthWheelLaneW <= 0 || v.SynthWheelRowH <= 0 {
			t.Fatalf("density %v: armed tokens must be positive, got card=%d lane=%d row=%d", d, v.SynthArmedCardH, v.SynthWheelLaneW, v.SynthWheelRowH)
		}
		if v.SynthWheelVisibleRows%2 == 0 {
			t.Fatalf("density %v: visible rows must be odd (center + N each side), got %d", d, v.SynthWheelVisibleRows)
		}
		restore()
	}
}
```

- [ ] **Step 4: Run**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSynthArmedTokensPresent ./internal/ui/`
Expected: PASS. Also run `-run 'TestDesignMDDrift|TestTokenDiscipline'` — PASS (density tokens aren't in the drift want-map; only color/spacing/rounded/alpha are).

- [ ] **Step 5: Commit** — `feat(synth): density tokens for inline armed editor`

---

### Task 2: `BarrelWheel` kinetic widget

**Files:**
- Create: `src/go/internal/ui/barrel_wheel.go`
- Test: `src/go/internal/ui/barrel_wheel_test.go`

**Interfaces:**
- Produces:
  - `type barrelBinding interface { labelAt(offset int) string; stepBy(detents int) bool; canStep(dir int) bool }`
  - `func NewBarrelWheel() *BarrelWheel`
  - `func (w *BarrelWheel) Bind(b barrelBinding, detentPx int)`
  - `func (w *BarrelWheel) BeginDrag(y int)`
  - `func (w *BarrelWheel) DragTo(y int) bool` (returns true if a detent crossed)
  - `func (w *BarrelWheel) EndDrag()`
  - `func (w *BarrelWheel) ImpulseNotch(dir int) bool`
  - `func (w *BarrelWheel) Tick() bool` (returns true while still animating)
  - `func (w *BarrelWheel) Animating() bool`
  - `func (w *BarrelWheel) OffsetPx() float64`

- [ ] **Step 1: Write the failing tests**

```go
package ui

import "testing"

// fakeBinding is a test barrelBinding over an int counter with hard clamps.
type fakeBinding struct {
	v, min, max int
}

func (f *fakeBinding) labelAt(off int) string { return "" }
func (f *fakeBinding) stepBy(detents int) bool {
	old := f.v
	f.v += detents
	if f.v < f.min {
		f.v = f.min
	}
	if f.v > f.max {
		f.v = f.max
	}
	return f.v != old
}
func (f *fakeBinding) canStep(dir int) bool {
	if dir > 0 {
		return f.v < f.max
	}
	return f.v > f.min
}

func TestBarrelWheel_SingleSlowNotchMovesOneDetent(t *testing.T) {
	b := &fakeBinding{v: 5, min: 0, max: 100}
	w := NewBarrelWheel()
	w.Bind(b, 20)
	w.ImpulseNotch(+1)
	// One impulse then settle should land exactly one detent up.
	for i := 0; i < 240 && w.Tick(); i++ {
	}
	if b.v != 6 {
		t.Fatalf("one notch = one detent: got %d want 6", b.v)
	}
}

func TestBarrelWheel_FlingDecaysAndSnaps(t *testing.T) {
	b := &fakeBinding{v: 0, min: -1000, max: 1000}
	w := NewBarrelWheel()
	w.Bind(b, 20)
	// Simulate a fast downward drag: y increases 40px/frame for 5 frames.
	w.BeginDrag(0)
	for i := 1; i <= 5; i++ {
		w.DragTo(i * 40)
	}
	w.EndDrag()
	moved := 0
	for i := 0; i < 600 && w.Tick(); i++ {
		moved++
	}
	if w.Animating() {
		t.Fatal("fling must come to rest")
	}
	if b.v <= 10 {
		t.Fatalf("downward fling should coast well past the drag distance, got %d", b.v)
	}
	if w.OffsetPx() != 0 {
		t.Fatalf("must snap to a detent (offset 0), got %v", w.OffsetPx())
	}
}

func TestBarrelWheel_ClampKillsVelocity(t *testing.T) {
	b := &fakeBinding{v: 99, min: 0, max: 100}
	w := NewBarrelWheel()
	w.Bind(b, 20)
	w.BeginDrag(0)
	for i := 1; i <= 10; i++ {
		w.DragTo(i * 60) // huge downward fling toward max
	}
	w.EndDrag()
	for i := 0; i < 600 && w.Tick(); i++ {
	}
	if b.v != 100 {
		t.Fatalf("clamp at max: got %d want 100", b.v)
	}
	if w.OffsetPx() != 0 || w.Animating() {
		t.Fatalf("clamp must kill velocity and settle, offset=%v animating=%v", w.OffsetPx(), w.Animating())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestBarrelWheel ./internal/ui/`
Expected: FAIL (undefined `NewBarrelWheel`).

- [ ] **Step 3: Implement `barrel_wheel.go`**

```go
package ui

// barrelBinding is the value model a BarrelWheel scrolls. The wheel owns no
// value state; it reads labels and asks the binding to step by whole detents.
// Implementations resolve their underlying widget by index every call so a
// panel re-layout can never leave a dangling pointer.
type barrelBinding interface {
	// labelAt returns the display label offset detents from the current
	// selection (0 == current). Used only for the fade stack render.
	labelAt(offset int) string
	// stepBy moves the current selection by detents (signed). Returns whether
	// the underlying value changed (false at a hard clamp).
	stepBy(detents int) bool
	// canStep reports whether one more step in dir (+1/-1) is possible.
	canStep(dir int) bool
}

// Inertia tunables. Starting values; refined in the screenshot loop (Task 14).
const (
	barrelFriction    = 0.90 // velocity retained per tick
	barrelMinVel      = 0.6  // px/frame below which we settle
	barrelSnapEase    = 0.30 // fraction of remaining offset eased away per tick
	barrelNotchVel    = 6.0  // px/frame impulse per wheel notch
	barrelDragSmooth  = 0.5  // velocity smoothing on drag samples
	barrelMaxVelScale = 3.0  // cap fling velocity at detentPx * this
)

// BarrelWheel is an infinite, inertial scroll wheel: a sub-detent pixel offset
// plus a velocity that decays with friction and snaps to the nearest detent at
// rest. Direction: a DOWNWARD gesture (y increasing / positive notch) advances
// the binding in the +1 direction (see plan Global Constraints).
type BarrelWheel struct {
	b        barrelBinding
	detentPx int

	offsetPx float64 // (-detentPx, detentPx); 0 == centered on current detent
	velocity float64 // px/frame

	dragging  bool
	lastDragY int
	settling  bool
}

func NewBarrelWheel() *BarrelWheel { return &BarrelWheel{detentPx: 1} }

func (w *BarrelWheel) Bind(b barrelBinding, detentPx int) {
	w.b = b
	if detentPx < 1 {
		detentPx = 1
	}
	w.detentPx = detentPx
}

func (w *BarrelWheel) Animating() bool { return w.dragging || w.settling || w.velocity != 0 || w.offsetPx != 0 }
func (w *BarrelWheel) OffsetPx() float64 { return w.offsetPx }

func (w *BarrelWheel) BeginDrag(y int) {
	w.dragging = true
	w.settling = false
	w.velocity = 0
	w.lastDragY = y
}

// DragTo shifts by the pointer delta (natural: offset follows the finger) and
// crosses whole detents. Returns whether a detent crossed.
func (w *BarrelWheel) DragTo(y int) bool {
	if !w.dragging {
		return false
	}
	dy := float64(y - w.lastDragY)
	w.lastDragY = y
	// Smooth the velocity estimate so EndDrag flings on recent motion.
	w.velocity = w.velocity*(1-barrelDragSmooth) + dy*barrelDragSmooth
	return w.advance(dy)
}

func (w *BarrelWheel) EndDrag() {
	w.dragging = false
	w.settling = true
	w.clampVel()
}

// ImpulseNotch adds a velocity impulse in dir (+1 down/increase, -1 up).
func (w *BarrelWheel) ImpulseNotch(dir int) bool {
	if dir == 0 {
		return false
	}
	w.settling = true
	w.velocity += float64(sign(dir)) * barrelNotchVel
	w.clampVel()
	return true
}

// Tick integrates one frame of motion. Returns true while still animating.
func (w *BarrelWheel) Tick() bool {
	if w.dragging {
		return true
	}
	if abs(int(w.velocity)) != 0 || w.velocity > barrelMinVel || w.velocity < -barrelMinVel {
		w.advance(w.velocity)
		w.velocity *= barrelFriction
		if w.velocity < barrelMinVel && w.velocity > -barrelMinVel {
			w.velocity = 0
		}
		return true
	}
	// Settle: ease the residual offset to the nearest detent.
	if w.offsetPx != 0 {
		w.offsetPx -= w.offsetPx * barrelSnapEase
		if w.offsetPx < 0.5 && w.offsetPx > -0.5 {
			w.offsetPx = 0
		}
		return w.offsetPx != 0
	}
	w.settling = false
	return false
}

// advance shifts offsetPx by dpx and applies whole-detent crossings, honoring
// clamps (a clamp kills velocity and pins offset to 0).
func (w *BarrelWheel) advance(dpx float64) bool {
	if w.b == nil || w.detentPx <= 0 {
		return false
	}
	w.offsetPx += dpx
	crossed := false
	d := float64(w.detentPx)
	for w.offsetPx >= d {
		if !w.b.canStep(+1) {
			w.offsetPx = 0
			w.velocity = 0
			break
		}
		if w.b.stepBy(+1) {
			crossed = true
		}
		w.offsetPx -= d
	}
	for w.offsetPx <= -d {
		if !w.b.canStep(-1) {
			w.offsetPx = 0
			w.velocity = 0
			break
		}
		if w.b.stepBy(-1) {
			crossed = true
		}
		w.offsetPx += d
	}
	return crossed
}

func (w *BarrelWheel) clampVel() {
	max := float64(w.detentPx) * barrelMaxVelScale
	if w.velocity > max {
		w.velocity = max
	}
	if w.velocity < -max {
		w.velocity = -max
	}
}

func sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestBarrelWheel ./internal/ui/`
Expected: PASS (all three).

- [ ] **Step 5: Commit** — `feat(synth): BarrelWheel inertial widget`

---

### Task 3: Synth value & resolution bindings

**Files:**
- Create: `src/go/internal/ui/synth_barrel_bindings.go`
- Test: `src/go/internal/ui/synth_barrel_bindings_test.go`

**Interfaces:**
- Consumes: `barrelBinding` (Task 2); `DrumView.instEditorControls/instEditorStepBadges/instEditorBindings`; `audio.ParamDef`; `formatSynthParamValueStep`, `formatKnobDisplayStep`, `formatStepValue`, `Knob.StepValueDirect`, `Knob.Value`, `Knob.Scale`, `KnobStepBadge.{Step,Index,SetIndex,LadderLen,Rungs,AdjustResolution}`, `persistKnobStep`, `knobStepResolution`.
- Produces:
  - `type synthValueBinding struct { dv *DrumView; idx int; instID string }` implementing `barrelBinding`
  - `type synthResolutionBinding struct { dv *DrumView; idx int; instID string }` implementing `barrelBinding`
  - `func (b synthResolutionBinding) available() bool` (false when the control has no step badge → discrete param, no resolution lane)

- [ ] **Step 1: Write the failing tests** (functional — real Knob/badge via the modular synth tab game)

```go
package ui

import "testing"

func TestSynthValueBinding_StepChangesKnob(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	instID := dv.synthTabActiveInstrument()
	// Arm-independent: bind directly to control 0 and step it up.
	if len(dv.instEditorControls) == 0 {
		t.Skip("no synth controls for this instrument")
	}
	before := dv.instEditorControls[0].Value
	b := synthValueBinding{dv: dv, idx: 0, instID: instID}
	if !b.stepBy(+3) {
		t.Fatal("stepBy should report a change")
	}
	if dv.instEditorControls[0].Value <= before {
		t.Fatalf("value should increase: before=%v after=%v", before, dv.instEditorControls[0].Value)
	}
	if b.labelAt(0) == "" {
		t.Fatal("labelAt(0) must render the current value")
	}
}

func TestSynthResolutionBinding_WalksLadderAndPersists(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	instID := dv.synthTabActiveInstrument()
	// Find a control that has a step badge (continuous param).
	idx := -1
	for i, badge := range dv.instEditorStepBadges {
		if badge != nil {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("no continuous control with a resolution ladder")
	}
	r := synthResolutionBinding{dv: dv, idx: idx, instID: instID}
	if !r.available() {
		t.Fatal("resolution lane should be available for a badged control")
	}
	start := dv.instEditorStepBadges[idx].Index()
	// +1 detent = coarser (higher index). Guard against already-max.
	if r.canStep(+1) {
		if !r.stepBy(+1) {
			t.Fatal("stepBy(+1) should move the rung")
		}
		if dv.instEditorStepBadges[idx].Index() != start+1 {
			t.Fatalf("rung should advance by one: start=%d now=%d", start, dv.instEditorStepBadges[idx].Index())
		}
	}
}

func TestSynthResolutionBinding_UnavailableForDiscrete(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	// A nil step badge = discrete param = no resolution lane.
	for i, badge := range dv.instEditorStepBadges {
		if badge == nil {
			r := synthResolutionBinding{dv: dv, idx: i, instID: dv.synthTabActiveInstrument()}
			if r.available() {
				t.Fatalf("idx %d has nil badge but reports available", i)
			}
			return
		}
	}
	t.Skip("no discrete control present")
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSynth.*Binding ./internal/ui/`
Expected: FAIL (undefined `synthValueBinding`).

- [ ] **Step 3: Implement `synth_barrel_bindings.go`**

```go
package ui

import (
	"math"

	"beatmo/internal/audio"
)

// synthValueBinding scrolls a synth control's VALUE. Resolves the *Knob by
// index every call (no cached pointer) so a re-layout is safe.
type synthValueBinding struct {
	dv     *DrumView
	idx    int
	instID string
}

func (b synthValueBinding) knob() *Knob {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.instEditorControls) {
		return nil
	}
	return b.dv.instEditorControls[b.idx]
}

func (b synthValueBinding) def() (audio.ParamDef, bool) {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.instEditorBindings) {
		return audio.ParamDef{}, false
	}
	return b.dv.instEditorBindings[b.idx].def, true
}

// realAt returns the real-unit value `offset` detents from current, plus the
// active step, without mutating the knob.
func (b synthValueBinding) realAt(offset int) (val, step float64, def audio.ParamDef, ok bool) {
	k := b.knob()
	def, ok = b.def()
	if k == nil || !ok {
		return 0, 0, def, false
	}
	span := def.Max - def.Min
	step = knobStepResolution(k, def)
	real := def.Min + k.Value*span
	return real + float64(offset)*step, step, def, true
}

func (b synthValueBinding) labelAt(offset int) string {
	real, step, def, ok := b.realAt(offset)
	if !ok {
		return ""
	}
	// Enum params show labels; the offset walks the enum index directly.
	if len(def.Enum) > 0 {
		k := b.knob()
		span := float64(len(def.Enum) - 1)
		cur := int(math.Round(k.Value * span))
		i := cur + offset
		if i < 0 || i >= len(def.Enum) {
			return ""
		}
		return def.Enum[i]
	}
	return formatSynthParamValueStep(def, real, step)
}

func (b synthValueBinding) stepBy(detents int) bool {
	k := b.knob()
	if k == nil || detents == 0 {
		return false
	}
	dir := sign(detents)
	changed := false
	for i := 0; i < abs(detents); i++ {
		if k.StepValueDirect(dir) {
			changed = true
		}
	}
	return changed
}

func (b synthValueBinding) canStep(dir int) bool {
	k := b.knob()
	if k == nil {
		return false
	}
	if dir > 0 {
		return k.Value < 1.0
	}
	return k.Value > 0.0
}

// synthResolutionBinding scrolls a synth control's RESOLUTION rung. +1 detent =
// coarser (higher ladder index). Nil badge (discrete param) = unavailable.
type synthResolutionBinding struct {
	dv     *DrumView
	idx    int
	instID string
}

func (b synthResolutionBinding) badge() *KnobStepBadge {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.instEditorStepBadges) {
		return nil
	}
	return b.dv.instEditorStepBadges[b.idx]
}

func (b synthResolutionBinding) available() bool { return b.badge() != nil }

func (b synthResolutionBinding) def() audio.ParamDef {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.instEditorBindings) {
		return audio.ParamDef{}
	}
	return b.dv.instEditorBindings[b.idx].def
}

func (b synthResolutionBinding) labelAt(offset int) string {
	bg := b.badge()
	if bg == nil {
		return ""
	}
	i := bg.Index() + offset
	rungs := bg.Rungs()
	if i < 0 || i >= len(rungs) {
		return ""
	}
	return formatStepValue(b.def(), rungs[i])
}

func (b synthResolutionBinding) stepBy(detents int) bool {
	bg := b.badge()
	if bg == nil || detents == 0 {
		return false
	}
	// +1 detent = coarser = higher index. AdjustResolution(+1) is FINER, so
	// invert: coarser needs a negative AdjustResolution delta.
	moved := bg.AdjustResolution(-detents)
	if moved {
		// Keep the knob's per-notch step in sync with the chosen rung and
		// persist the choice (mirrors persistKnobStep call sites).
		if k := synthValueBinding{dv: b.dv, idx: b.idx, instID: b.instID}.knob(); k != nil {
			k.StepMul = bg.Step()
		}
		b.dv.persistKnobStep(bg)
	}
	return moved
}

func (b synthResolutionBinding) canStep(dir int) bool {
	bg := b.badge()
	if bg == nil {
		return false
	}
	i := bg.Index()
	// dir>0 = coarser = higher index; guard against the ladder ends.
	if dir > 0 {
		return i < bg.LadderLen()-1
	}
	return i > 0
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSynth.*Binding ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Commit** — `feat(synth): value & resolution barrel bindings`

---

### Task 4: Armed-control state on DrumView

**Files:**
- Modify: `src/go/internal/ui/drumview.go` (add `instEditorArmedControl map[string]int` near `instEditorSelectedControl` at `:523`)
- Create: `src/go/internal/ui/synth_armed_control.go`
- Test: `src/go/internal/ui/synth_armed_control_test.go`

**Interfaces:**
- Consumes: `setSynthSelectedControl` (`synth_selected_control.go:53`).
- Produces:
  - `func (dv *DrumView) synthControlArmed(instID string) (idx int, armed bool)`
  - `func (dv *DrumView) armSynthControl(instID string, idx int)` (arms idx, locks any prior, selects idx)
  - `func (dv *DrumView) lockSynthControl(instID string)` (disarms; no-op if none)
  - `func (dv *DrumView) toggleSynthControlArm(instID string, idx int)` (arm if not armed-here, else lock)

- [ ] **Step 1: Add the field**

In `drumview.go`, immediately after `instEditorSelectedControl map[string]int` (`:523`):

```go
	// instEditorArmedControl is the per-instrument index of the control that is
	// currently ARMED for inline barrel editing (at most one). Absent = none
	// armed. Arming also selects (focus graph); locking removes the entry.
	instEditorArmedControl map[string]int
	// instEditorArmedEditor owns the two barrel wheels for the armed control.
	instEditorArmedEditor *ArmedControlEditor
```

- [ ] **Step 2: Write the failing tests**

```go
package ui

import "testing"

func TestArmSynthControl_OneAtATime(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("nothing should be armed initially")
	}
	dv.armSynthControl(inst, 2)
	idx, armed := dv.synthControlArmed(inst)
	if !armed || idx != 2 {
		t.Fatalf("expected armed idx 2, got idx=%d armed=%v", idx, armed)
	}
	// Arming a second control replaces the first.
	dv.armSynthControl(inst, 5)
	idx, armed = dv.synthControlArmed(inst)
	if !armed || idx != 5 {
		t.Fatalf("expected armed idx 5, got idx=%d armed=%v", idx, armed)
	}
	// Arming also selects for the focus graph.
	if got := dv.instEditorSelectedControl[inst]; got != 5 {
		t.Fatalf("arm should select: selected=%d", got)
	}
}

func TestToggleSynthControlArm_LocksOnSecondTap(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	dv.toggleSynthControlArm(inst, 3) // arm
	if idx, armed := dv.synthControlArmed(inst); !armed || idx != 3 {
		t.Fatalf("first toggle should arm 3, got %d/%v", idx, armed)
	}
	dv.toggleSynthControlArm(inst, 3) // lock
	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("second toggle on same idx should lock")
	}
	// Toggling a different idx while armed re-arms it.
	dv.toggleSynthControlArm(inst, 3)
	dv.toggleSynthControlArm(inst, 4)
	if idx, armed := dv.synthControlArmed(inst); !armed || idx != 4 {
		t.Fatalf("toggle other idx should arm it, got %d/%v", idx, armed)
	}
}
```

- [ ] **Step 3: Implement `synth_armed_control.go`**

```go
package ui

// synth_armed_control.go — which control is ARMED for inline barrel editing.
//
// At most one control is armed per instrument. Arming selects the control
// (focus graph) and locks any previously-armed one. Locking removes the entry.
// The ArmedControlEditor (rebuilt on arm) owns the kinetic barrel state.

func (dv *DrumView) synthControlArmed(instID string) (int, bool) {
	if dv.instEditorArmedControl == nil || instID == "" {
		return -1, false
	}
	idx, ok := dv.instEditorArmedControl[instID]
	return idx, ok
}

func (dv *DrumView) armSynthControl(instID string, idx int) {
	if instID == "" || idx < 0 {
		return
	}
	if dv.instEditorArmedControl == nil {
		dv.instEditorArmedControl = map[string]int{}
	}
	dv.instEditorArmedControl[instID] = idx
	dv.setSynthSelectedControl(instID, idx)
	// Fresh editor bound to the new index; kinetic state starts at rest.
	dv.instEditorArmedEditor = newArmedControlEditor(dv, instID, idx)
}

func (dv *DrumView) lockSynthControl(instID string) {
	if dv.instEditorArmedControl != nil {
		delete(dv.instEditorArmedControl, instID)
	}
	dv.instEditorArmedEditor = nil
}

func (dv *DrumView) toggleSynthControlArm(instID string, idx int) {
	if cur, armed := dv.synthControlArmed(instID); armed && cur == idx {
		dv.lockSynthControl(instID)
		return
	}
	dv.armSynthControl(instID, idx)
}
```

Note: `newArmedControlEditor` is defined in Task 5. To keep this task independently green, add a **temporary stub** at the bottom of `synth_armed_control.go` and delete it in Task 5:

```go
// TEMP stub — replaced by synth_armed_editor.go in Task 5.
type ArmedControlEditor struct{}

func newArmedControlEditor(dv *DrumView, instID string, idx int) *ArmedControlEditor {
	return &ArmedControlEditor{}
}
```

- [ ] **Step 4: Run**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestArmSynthControl|TestToggleSynthControlArm' ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Commit** — `feat(synth): armed-control state (one-at-a-time, selects)`

---

### Task 5: `ArmedControlEditor` (two-lane editor)

**Files:**
- Create: `src/go/internal/ui/synth_armed_editor.go`
- Modify: `src/go/internal/ui/synth_armed_control.go` (delete the TEMP stub)
- Test: `src/go/internal/ui/synth_armed_editor_test.go`

**Interfaces:**
- Consumes: `BarrelWheel` (Task 2), `synthValueBinding`/`synthResolutionBinding` (Task 3), density tokens (Task 1).
- Produces:
  - `func newArmedControlEditor(dv *DrumView, instID string, idx int) *ArmedControlEditor`
  - `func (e *ArmedControlEditor) SetCardRect(card image.Rectangle)` (splits into value lane + optional res lane)
  - `func (e *ArmedControlEditor) ValueLaneRect() image.Rectangle`
  - `func (e *ArmedControlEditor) ResLaneRect() image.Rectangle` (empty when no resolution lane)
  - `func (e *ArmedControlEditor) laneAt(x int) *BarrelWheel` (which wheel a pointer x hits; value lane is the default)
  - `func (e *ArmedControlEditor) BeginDrag(x, y int)`, `DragTo(y int) bool`, `EndDrag()`
  - `func (e *ArmedControlEditor) Wheel(x, y, steps int) bool`
  - `func (e *ArmedControlEditor) StepValue(dir int) bool`, `StepResolution(dir int) bool` (keyboard)
  - `func (e *ArmedControlEditor) Tick() bool` (ticks both wheels + fires debounced commit; returns animating)
  - `func (e *ArmedControlEditor) value() *BarrelWheel`, `res() *BarrelWheel`

- [ ] **Step 1: Write the failing tests**

```go
package ui

import (
	"image"
	"testing"
)

func TestArmedEditor_ValueLaneSpinsChangesValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	dv.armSynthControl(inst, 0)
	e := dv.instEditorArmedEditor
	e.SetCardRect(image.Rect(0, 0, 120, 148))

	before := dv.instEditorControls[0].Value
	// Spin the value lane (right side) with a notch, then settle.
	vx := e.ValueLaneRect().Min.X + 4
	e.Wheel(vx, 40, +1)
	for i := 0; i < 240 && e.Tick(); i++ {
	}
	if dv.instEditorControls[0].Value <= before {
		t.Fatalf("value lane notch should increase value: before=%v after=%v", before, dv.instEditorControls[0].Value)
	}
}

func TestArmedEditor_HoveredLaneRouting(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	// pick a badged (continuous) control so a res lane exists
	idx := 0
	for i, b := range dv.instEditorStepBadges {
		if b != nil {
			idx = i
			break
		}
	}
	dv.armSynthControl(inst, idx)
	e := dv.instEditorArmedEditor
	e.SetCardRect(image.Rect(0, 0, 120, 148))
	if e.ResLaneRect().Empty() {
		t.Skip("selected control has no resolution lane")
	}
	// A point in the left lane routes to the resolution wheel.
	rx := e.ResLaneRect().Min.X + 2
	if e.laneAt(rx) != e.res() {
		t.Fatal("left-lane x should route to the resolution wheel")
	}
	// A point in the right lane routes to the value wheel.
	vx := e.ValueLaneRect().Min.X + 2
	if e.laneAt(vx) != e.value() {
		t.Fatal("right-lane x should route to the value wheel")
	}
}

func TestArmedEditor_KeyboardStepsValueAndResolution(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	dv.armSynthControl(inst, 0)
	e := dv.instEditorArmedEditor
	before := dv.instEditorControls[0].Value
	if !e.StepValue(+1) {
		t.Fatal("StepValue(+1) should change value")
	}
	if dv.instEditorControls[0].Value <= before {
		t.Fatal("value should increase")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestArmedEditor ./internal/ui/`
Expected: FAIL (stub `ArmedControlEditor` has no methods).

- [ ] **Step 3: Delete the TEMP stub** in `synth_armed_control.go` (the `type ArmedControlEditor struct{}` + `newArmedControlEditor` block).

- [ ] **Step 4: Implement `synth_armed_editor.go`**

```go
package ui

import "image"

// ArmedControlEditor pairs two BarrelWheels for the armed synth control: the
// value lane (right, always present) and the resolution lane (left, present
// only for continuous params with a step badge). It owns the lane geometry,
// routes pointer/keyboard input to the correct wheel, and coalesces edits into
// one debounced undo step via the shared synthWheelNudge.
type ArmedControlEditor struct {
	dv     *DrumView
	instID string
	idx    int

	valWheel *BarrelWheel
	resWheel *BarrelWheel // nil when the control has no resolution ladder

	card     image.Rectangle
	valRect  image.Rectangle
	resRect  image.Rectangle
	active   *BarrelWheel // wheel currently captured by a drag
}

func newArmedControlEditor(dv *DrumView, instID string, idx int) *ArmedControlEditor {
	e := &ArmedControlEditor{dv: dv, instID: instID, idx: idx}
	detent := Profile().DensityValues().SynthWheelRowH
	e.valWheel = NewBarrelWheel()
	e.valWheel.Bind(synthValueBinding{dv: dv, idx: idx, instID: instID}, detent)
	if rb := (synthResolutionBinding{dv: dv, idx: idx, instID: instID}); rb.available() {
		e.resWheel = NewBarrelWheel()
		e.resWheel.Bind(rb, detent)
	}
	return e
}

func (e *ArmedControlEditor) value() *BarrelWheel { return e.valWheel }
func (e *ArmedControlEditor) res() *BarrelWheel   { return e.resWheel }

// SetCardRect splits the grown card into a left resolution lane (when present)
// and a wider right value lane, leaving the card header untouched at the top.
func (e *ArmedControlEditor) SetCardRect(card image.Rectangle) {
	e.card = card
	v := Profile().DensityValues()
	// Reserve a header band at the top (param name) == one wheel row.
	body := card
	body.Min.Y = card.Min.Y + v.SynthWheelRowH
	if e.resWheel != nil {
		laneW := v.SynthWheelLaneW
		if laneW > body.Dx()/2 {
			laneW = body.Dx() / 2
		}
		e.resRect = image.Rect(body.Min.X, body.Min.Y, body.Min.X+laneW, body.Max.Y)
		e.valRect = image.Rect(body.Min.X+laneW, body.Min.Y, body.Max.X, body.Max.Y)
	} else {
		e.resRect = image.Rectangle{}
		e.valRect = body
	}
}

func (e *ArmedControlEditor) ValueLaneRect() image.Rectangle { return e.valRect }
func (e *ArmedControlEditor) ResLaneRect() image.Rectangle   { return e.resRect }

// laneAt returns the wheel under pointer x. The value lane is the default so a
// press just outside the narrow res lane still edits the value.
func (e *ArmedControlEditor) laneAt(x int) *BarrelWheel {
	if e.resWheel != nil && !e.resRect.Empty() && x < e.resRect.Max.X {
		return e.resWheel
	}
	return e.valWheel
}

func (e *ArmedControlEditor) BeginDrag(x, y int) {
	e.active = e.laneAt(x)
	if e.active != nil {
		e.active.BeginDrag(y)
	}
}

func (e *ArmedControlEditor) DragTo(y int) bool {
	if e.active == nil {
		return false
	}
	if e.active.DragTo(y) {
		e.onEdit()
		return true
	}
	return false
}

func (e *ArmedControlEditor) EndDrag() {
	if e.active != nil {
		e.active.EndDrag()
	}
	e.active = nil
}

func (e *ArmedControlEditor) Wheel(x, y, steps int) bool {
	w := e.laneAt(x)
	if w == nil || steps == 0 {
		return false
	}
	// Positive steps (scroll down on target) = +1 = increase / coarser.
	changed := w.ImpulseNotch(sign(steps))
	if changed {
		e.onEdit()
	}
	return changed
}

func (e *ArmedControlEditor) StepValue(dir int) bool {
	if e.valWheel == nil {
		return false
	}
	ok := e.valWheel.ImpulseNotch(sign(dir))
	if ok {
		e.onEdit()
	}
	return ok
}

func (e *ArmedControlEditor) StepResolution(dir int) bool {
	if e.resWheel == nil {
		return false
	}
	ok := e.resWheel.ImpulseNotch(sign(dir))
	if ok {
		e.onEdit()
	}
	return ok
}

// Tick advances both wheels; whenever a value detent crosses, onEdit fires the
// live-propagate + debounced commit. Returns true while either wheel animates.
func (e *ArmedControlEditor) Tick() bool {
	anim := false
	if e.valWheel != nil && e.valWheel.Tick() {
		anim = true
		e.onEdit()
	}
	if e.resWheel != nil && e.resWheel.Tick() {
		anim = true
		e.onEdit()
	}
	return anim
}

// onEdit mirrors synthControlHitAdapter.OnWheel's commit path: live slider sync
// + propagate now, one debounced undo step for the whole spin.
func (e *ArmedControlEditor) onEdit() {
	dv := e.dv
	dv.syncSliderFromKnob(e.idx)
	dv.propagateSynthSliderValue(e.idx, e.instID)
	instID := e.instID
	dv.synthWheelNudge.bump(func() {
		dv.commitInstrumentParams(instID)
		dv.requestSynthMirror(instID)
	})
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.Invalidate()
	}
}
```

- [ ] **Step 5: Run**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestArmedEditor|TestArmSynthControl|TestToggleSynthControlArm' ./internal/ui/`
Expected: PASS.

- [ ] **Step 6: Commit** — `feat(synth): ArmedControlEditor two-lane editor`

---

### Task 6: Modern flat lane renderer

**Files:**
- Create: `src/go/internal/ui/synth_barrel_render.go`
- Test: `src/go/internal/ui/synth_barrel_render_test.go`

**Interfaces:**
- Consumes: `barrelBinding.labelAt`, `BarrelWheel.OffsetPx`, density tokens, `fillVerticalGradient`, `WithAlpha`/`WithAlphaFromColor`, `drawText` role helpers, `colConceptStroke`, surface/accent tokens.
- Produces:
  - `func drawBarrelLane(dst *ebiten.Image, rect image.Rectangle, w *BarrelWheel, b barrelBinding, style barrelLaneStyle)`
  - `type barrelLaneStyle struct { RowH, VisibleRows int; Accent color.Color; Emphasis bool }`

Renderer contract (modern, flat — NOT the cylinder popup):
- Draw `VisibleRows` labels from `b.labelAt(-n..+n)`, vertically centered, shifted by `w.OffsetPx()` for sub-pixel motion.
- Center row: `RoleBody`, bold, accent-tinted; a thin accent hairline under it (1px, `WithAlpha(accent, AlphaStrong)`). (Hairline-vs-pill final choice made in Task 14 screenshot loop; start with the hairline.)
- Neighbors: `RoleCaption`, alpha falls off with distance from center; top/bottom soft fade via `fillVerticalGradient` toward the card edges.
- No cylinder gradient, ridges, specular, keycap, or rails. Uses the card's surface tokens and radius only.

- [ ] **Step 1: Write the failing drawRect/label test** (interceptor pattern from `audio_panel_render_pixels_test.go`)

```go
package ui

import (
	"image"
	"testing"
)

func TestDrawBarrelLane_RendersCenterAndNeighbors(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	dv.armSynthControl(inst, 0)
	e := dv.instEditorArmedEditor
	e.SetCardRect(image.Rect(0, 0, 120, 148))

	var texts []string
	restore := setTestTextSink(func(s string, x, y int) { texts = append(texts, s) })
	defer restore()

	dst := newTestImage(120, 148)
	drawBarrelLane(dst, e.ValueLaneRect(), e.value(), synthValueBinding{dv: dv, idx: 0, instID: inst},
		barrelLaneStyle{RowH: 26, VisibleRows: 5, Accent: colConceptStroke, Emphasis: true})

	if len(texts) < 3 {
		t.Fatalf("expected center + neighbor labels drawn, got %d: %v", len(texts), texts)
	}
}
```

> If no `setTestTextSink`/`newTestImage` helper exists, use the existing drawRect-interceptor + `testAnalyzerStateOverride` scaffolding in `audio_panel_render_pixels_test.go` as the model (count non-empty label draws inside the lane rect). Match whatever text-capture hook that file already uses; do not invent a new one.

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDrawBarrelLane ./internal/ui/`
Expected: FAIL (undefined `drawBarrelLane`).

- [ ] **Step 3: Implement `synth_barrel_render.go`** (flat renderer; see contract above). Center label bold+accent, neighbors alpha-faded, `fillVerticalGradient` edge fades, sub-pixel offset from `w.OffsetPx()`. Reuse existing `drawText`/role + gradient helpers — no new primitives.

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDrawBarrelLane ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Commit** — `feat(synth): modern flat barrel lane renderer`

---

### Task 7: Armed card grows in place

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` — `placeKnobsInSection` (rect assignment ~`:2119-2126`) and the `ControlGrid` row-height/`Layout` path so the armed row reserves `SynthArmedCardH`.
- Modify: `src/go/internal/ui/synth_panel_zone.go` — `drawSynthDetailPane` (`:2703`, card loop ~`:2796-2808`) to draw the armed card via the editor + `drawBarrelLane` instead of `drawParamCard`.
- Test: `src/go/internal/ui/synth_armed_layout_test.go` (Create)

**Interfaces:**
- Consumes: `synthControlArmed`, `ArmedControlEditor.SetCardRect`, `drawBarrelLane`, `SynthArmedCardH`.

Approach: the armed control's cell is taller than the others. Simplest correct integration that respects `ControlGrid` windowing: when a control in the open section is armed, grow **that cell's height** to `SynthArmedCardH` during placement and feed the editor the grown rect via `SetCardRect`. Because `ControlGrid` uses a uniform `rowH`, reserve the extra height by treating the armed card as spanning enough rows to cover `SynthArmedCardH` (ceil division), or special-case its rect after the grid computes the base slot (extend `slot.Max.Y` and shift subsequent cells down within the section's content rect). Pick the post-slot extension — it keeps `ControlGrid`'s scroll math untouched and only nudges draw/hit rects.

- [ ] **Step 1: Write the failing test**

```go
package ui

import "testing"

func TestArmedCardGrowsInPlace(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	// Lay out once un-armed; capture control 0's height.
	forceSynthLayout(g) // helper: drive one Update+Draw so rects are assigned
	restRect := dv.instEditorControls[0].Rect()
	restH := restRect.Dy()

	dv.armSynthControl(inst, 0)
	forceSynthLayout(g)
	armedRect := dv.instEditorControls[0].Rect()
	if armedRect.Dy() <= restH {
		t.Fatalf("armed card should be taller: rest=%d armed=%d", restH, armedRect.Dy())
	}
	if want := Profile().DensityValues().SynthArmedCardH; armedRect.Dy() < want-2 {
		t.Fatalf("armed height ~%d expected, got %d", want, armedRect.Dy())
	}
}
```

> `forceSynthLayout` = drive the panel through one Layout (call the same path `synth_panel_zone_test.go` already uses to lay out the tab; reuse that helper rather than adding a new one).

- [ ] **Step 2: Run to verify failure** — FAIL (armed height == rest height).

- [ ] **Step 3: Implement** the post-slot height extension in `placeKnobsInSection` + route the armed card's draw through the editor in `drawSynthDetailPane` (call `e.SetCardRect(cardRect)` then `drawBarrelLane` for each lane; keep drawing the param name in the reserved header band).

- [ ] **Step 4: Run** — PASS. Also run `-run 'TestSynthLayoutFingerprintStable'` and confirm the fingerprint test still holds for the **un-armed** default (arming is a runtime state, not the default layout).

- [ ] **Step 5: Commit** — `feat(synth): armed card grows in place with barrels`

---

### Task 8: Synth ControlGrid → smooth continuous + momentum scroll

**Files:**
- Modify: `src/go/internal/ui/control_grid.go:264-274` (drag/wheel wrappers + add momentum passthrough)
- Test: `src/go/internal/ui/control_grid_momentum_test.go` (Create)

**Interfaces:**
- Consumes: `ScrollBehavior.HandleTouchBegin/Move/End`, `HandleWheel`, `UpdateMomentum`, `HasMomentum`, `applyPixelDelta` (already on `ScrollBehavior`).
- Produces (new/changed on `*ControlGrid`): `BeginContentDrag`/`DragTo`/`EndDrag` become pixel+momentum; `WheelStep(steps)` becomes continuous (`HandleWheel`); `UpdateMomentum() bool`; `HasMomentum() bool`. **Thumb** grab (`BeginDrag`, the non-natural scrollbar-thumb path) stays stepped/direct.

- [ ] **Step 1: Write the failing test**

```go
package ui

import "testing"

func TestControlGrid_ContentDragHasMomentum(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.Layout(rect0(0, 0, 200, 100), 40, 60, 26, 6, 6) // 40 cells, small viewport
	// Fast upward content drag then release should keep coasting.
	g.BeginContentDrag(90)
	for y := 90; y >= 10; y -= 20 {
		g.DragTo(y)
	}
	g.EndDrag()
	if !g.HasMomentum() {
		t.Fatal("a flick should leave momentum")
	}
	ticks := 0
	for g.UpdateMomentum() && ticks < 600 {
		ticks++
	}
	if ticks == 0 {
		t.Fatal("momentum should coast for several frames")
	}
}

func TestControlGrid_WheelIsContinuousNotOneRow(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.Layout(rect0(0, 0, 200, 100), 40, 60, 26, 6, 6)
	first := g.Scroll().VS.First
	g.WheelStep(3) // a 3-notch spin
	// Continuous wheel should move more than the single row a stepped wheel did.
	if g.Scroll().VS.First <= first {
		t.Fatal("wheel should advance the window")
	}
}
```

> `rect0` = whatever the existing `control_grid_test.go` uses to build an `image.Rectangle` (reuse it; don't add a helper).

- [ ] **Step 2: Run to verify failure** — FAIL (`HasMomentum`/`UpdateMomentum` undefined on ControlGrid; stepped wheel moves exactly one row).

- [ ] **Step 3: Implement** — repoint the content wrappers:

```go
// control_grid.go (replace the stepped content-drag + wheel wrappers)

// BeginContentDrag starts a natural pixel drag (content follows the finger)
// with momentum on release. The scrollbar THUMB grab keeps BeginDrag (stepped,
// direct) so the thumb still follows the finger exactly.
func (g *ControlGrid) BeginContentDrag(y int) bool { g.scroll.HandleTouchBegin(0, y); return true }
func (g *ControlGrid) DragTo(y int) bool           { return g.scroll.HandleTouchMove(0, y) }
func (g *ControlGrid) EndDrag()                    { g.scroll.HandleTouchEnd(0, 0) }

// WheelStep now scrolls continuously (respects notch magnitude, no one-row cap).
func (g *ControlGrid) WheelStep(steps int) bool { return g.scroll.HandleWheel(steps) }

// UpdateMomentum advances any fling; call once per frame while HasMomentum.
func (g *ControlGrid) UpdateMomentum() bool { return g.scroll.UpdateMomentum() }
func (g *ControlGrid) HasMomentum() bool    { return g.scroll.HasMomentum() }
```

Keep `BeginDrag`/`Tick` (thumb path) as-is. Note: the natural direction is preserved because `HandleTouchMove` already applies `applyPixelDelta(-delta)` (content-follows-finger). Verify sign against `TestControlGrid_ContentDragHasMomentum` and the synth body-drag natural-direction test.

- [ ] **Step 4: Drive momentum each frame** — in `synthTabUpdate` (`synth_panel_zone.go:1914`), where it currently calls `g.Tick()` per section grid (`:1921-1925`), also pump momentum:

```go
for _, g := range dv.instEditorSectionGrids {
	g.Tick()
	if g.UpdateMomentum() {
		changed = true
	}
}
```

- [ ] **Step 5: Run** — `-run 'TestControlGrid_|TestSynthSection_BodyDragIsNatural|TestSynthSection_ThumbDragAdvancesOneRowPerStep'`. PASS (body natural + momentum; thumb still one-row direct).

- [ ] **Step 6: Commit** — `feat(synth): smooth continuous + momentum control-grid scroll`

---

### Task 9: Wire `synthControlHitAdapter` to armed model

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go:3695-3761` (`OnPress`/`OnDrag`/`OnRelease`/`OnWheel`)
- Test: `src/go/internal/ui/synth_armed_input_test.go` (Create)

**Interfaces:**
- Consumes: `synthControlArmed`, `armSynthControl`/`toggleSynthControlArm`/`lockSynthControl`, `ArmedControlEditor` methods, `ownerGrid()`, `dragScrollArbiter`.

Behavior matrix (replaces today's "tap→popup, wheel→nudge"):
- `OnPress`: if this idx is armed → `editor.BeginDrag(x,y)`, return `InputCaptured`. Else → begin `dragScrollArbiter` (unchanged) → `InputCaptured`.
- `OnDrag`: if armed(this) → `editor.DragTo(y)` (+ invalidate). Else → arbiter drag (scrolls the grid, now momentum-capable).
- `OnRelease`: if armed(this) → `editor.EndDrag()`; a stationary release (arbiter says tap, or editor moved nothing) → `toggleSynthControlArm` (lock). Else → arbiter end: **tap → `armSynthControl` (was: open popup)**; scroll → nothing.
- `OnWheel`: if armed(this) → `editor.Wheel(x,y,steps)`. **Else → scroll the owner grid** (`ownerGrid().WheelStep(steps)`), NOT `nudgeKnobByWheel`. This is the desktop behavior change.

- [ ] **Step 1: Write the failing tests**

```go
package ui

import "testing"

func TestNotArmedWheelScrollsRow(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	forceSynthLayout(g)
	sec := dv.synthSelectedSection()
	grid := dv.sectionGrid(sec.id)
	if !grid.HasScroll() {
		t.Skip("section doesn't overflow; no scroll to test")
	}
	first := grid.Scroll().VS.First
	h := &synthControlHitAdapter{dv: dv, idx: sec.knobIdxs[0], instID: dv.synthTabActiveInstrument()}
	r := dv.instEditorControls[sec.knobIdxs[0]].Rect()
	before := dv.instEditorControls[sec.knobIdxs[0]].Value
	h.OnWheel(r.Min.X+2, r.Min.Y+2, +3)
	if grid.Scroll().VS.First == first {
		t.Fatal("not-armed wheel over a control must scroll the row")
	}
	if dv.instEditorControls[sec.knobIdxs[0]].Value != before {
		t.Fatal("not-armed wheel must NOT change the value")
	}
}

func TestTapArmsThenWheelEditsValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	forceSynthLayout(g)
	idx := dv.synthSelectedSection().knobIdxs[0]
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: inst}
	r := dv.instEditorControls[idx].Rect()
	// Stationary press+release = arm.
	h.OnPress(r.Min.X+2, r.Min.Y+2)
	h.OnRelease(r.Min.X+2, r.Min.Y+2)
	if _, armed := dv.synthControlArmed(inst); !armed {
		t.Fatal("stationary tap should arm the control")
	}
	before := dv.instEditorControls[idx].Value
	// Now the wheel edits the value (right lane).
	e := dv.instEditorArmedEditor
	e.SetCardRect(r) // ensure lane rects
	vx := e.ValueLaneRect().Min.X + 2
	h.OnWheel(vx, r.Min.Y+r.Dy()/2, +2)
	for i := 0; i < 240 && e.Tick(); i++ {
	}
	if dv.instEditorControls[idx].Value <= before {
		t.Fatal("armed wheel should change the value")
	}
	// Tap again → lock.
	h.OnPress(vx, r.Min.Y+2)
	h.OnRelease(vx, r.Min.Y+2)
	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("second tap should lock")
	}
}
```

- [ ] **Step 2: Run to verify failure** — FAIL (old code opens popup on release / nudges value on wheel).

- [ ] **Step 3: Implement** the branch matrix in the four handlers. Reference the existing bodies at `:3695-3761`; keep the numeric-editor swallow guards. For the not-armed wheel branch:

```go
func (h *synthControlHitAdapter) OnWheel(x, y, steps int) InputResult {
	if h.dv.paramEditor != nil && h.dv.paramEditor.Active() {
		return InputConsumed
	}
	if idx, armed := h.dv.synthControlArmed(h.instID); armed && idx == h.idx {
		if h.dv.instEditorArmedEditor != nil && h.dv.instEditorArmedEditor.Wheel(x, y, steps) {
			h.invalidatePanel()
		}
		return InputConsumed
	}
	// NOT armed: wheel over a control scrolls the row (was: nudgeKnobByWheel).
	if g := h.ownerGrid(); g != nil && g.WheelStep(steps) {
		h.invalidatePanel()
	}
	return InputConsumed
}
```

Apply the analogous armed/not-armed branch to `OnPress`/`OnDrag`/`OnRelease` per the matrix. Ensure `OnRelease`'s not-armed tap calls `armSynthControl` (delete the `openSynthControlWheelPopup` call).

- [ ] **Step 4: Run** — `-run 'TestNotArmedWheelScrollsRow|TestTapArmsThenWheelEditsValue|TestSynthControl'`. PASS.

- [ ] **Step 5: Commit** — `feat(synth): route control input through armed model`

---

### Task 10: Keyboard editing while armed (arrows + Esc)

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go:1914` (`synthTabUpdate`) — poll arrow/Esc when a control is armed.
- Test: `src/go/internal/ui/synth_armed_keys_test.go` (Create)

**Interfaces:**
- Consumes: `ArmedControlEditor.StepValue`/`StepResolution`, `lockSynthControl`, `isKeyJustPressed` (the repo's input wrapper — mirror how `mobile_wheel_popup_keys_test.go` drives keys).

- [ ] **Step 1: Write the failing test** (drive via the same key-injection helper `mobile_wheel_popup_keys_test.go` uses)

```go
package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestArmedArrowKeysEditValueAndEsc(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	forceSynthLayout(g)
	idx := dv.synthSelectedSection().knobIdxs[0]
	dv.armSynthControl(inst, idx)

	before := dv.instEditorControls[idx].Value
	pressKeyForTest(ebiten.KeyArrowUp) // helper mirrors existing key tests
	dv.synthTabUpdate()
	for i := 0; i < 120 && dv.instEditorArmedEditor.Tick(); i++ {
	}
	if dv.instEditorControls[idx].Value <= before {
		t.Fatal("ArrowUp should increase the armed value")
	}

	pressKeyForTest(ebiten.KeyEscape)
	dv.synthTabUpdate()
	if _, armed := dv.synthControlArmed(inst); armed {
		t.Fatal("Esc should lock the armed control")
	}
}
```

> Use whatever key-injection helper the existing `mobile_wheel_popup_keys_test.go` uses instead of `pressKeyForTest` if the name differs.

- [ ] **Step 2: Run to verify failure** — FAIL (no key handling).

- [ ] **Step 3: Implement** in `synthTabUpdate`, when `synthControlArmed(instID)` is true:

```go
if idx, armed := dv.synthControlArmed(instID); armed && dv.instEditorArmedEditor != nil {
	e := dv.instEditorArmedEditor
	switch {
	case isKeyJustPressed(ebiten.KeyArrowUp):
		e.StepValue(+1); changed = true
	case isKeyJustPressed(ebiten.KeyArrowDown):
		e.StepValue(-1); changed = true
	case isKeyJustPressed(ebiten.KeyArrowRight):
		e.StepResolution(+1); changed = true
	case isKeyJustPressed(ebiten.KeyArrowLeft):
		e.StepResolution(-1); changed = true
	case isKeyJustPressed(ebiten.KeyEscape):
		dv.lockSynthControl(instID); changed = true
	}
	_ = idx
}
```

Direction: ArrowUp = increase (matches "up = higher" for the discrete keyboard path; independent of the scroll-sign convention). Also tick the editor each frame here (Task 12 formalizes the tick; a minimal tick call is fine now).

- [ ] **Step 4: Run** — `-run TestArmedArrowKeys` PASS.

- [ ] **Step 5: Commit** — `feat(synth): arrow/Esc keyboard editing while armed`

---

### Task 11: Numeric entry while armed

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` — a stationary click/double-click on the armed value readout, or typing a digit while armed, opens `openSynthParamEditor(idx, instID)`.
- Test: `src/go/internal/ui/synth_armed_numeric_test.go` (Create)

**Interfaces:**
- Consumes: `openSynthParamEditor` (`:3167`), `paramEditor.Active()`, `synthControlReadoutRect`.

Simplest reliable trigger that fits the armed model: while armed, a **double-click / double-tap** on the value lane opens the numeric editor (desktop and mobile). Detect via the existing double-tap gate the panel already uses (or a small timestamp check mirroring other double-tap sites). Keep the numeric editor's own commit path (it already syncs+propagates+commits).

- [ ] **Step 1: Write the failing test**

```go
package ui

import "testing"

func TestArmedDoubleTapOpensNumericEditor(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	forceSynthLayout(g)
	idx := dv.synthSelectedSection().knobIdxs[0]
	dv.armSynthControl(inst, idx)
	dv.instEditorArmedEditor.SetCardRect(dv.instEditorControls[idx].Rect())

	openArmedNumericEditor(dv, idx, inst) // the new helper the double-tap calls
	if dv.paramEditor == nil || !dv.paramEditor.Active() {
		t.Fatal("numeric editor should be active after the armed numeric-entry trigger")
	}
}
```

- [ ] **Step 2: Run** — FAIL (undefined `openArmedNumericEditor`).

- [ ] **Step 3: Implement** a thin `openArmedNumericEditor(dv, idx, instID)` that calls `dv.openSynthParamEditor(idx, instID)` (guarded on armed state), and wire the armed double-tap in `synthControlHitAdapter.OnRelease` (a second stationary release within the double-tap window on an already-armed control opens numeric entry instead of locking).

- [ ] **Step 4: Run** — PASS.

- [ ] **Step 5: Commit** — `feat(synth): numeric entry for armed control`

---

### Task 12: Per-frame animation tick + render-cache bypass

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go:1914` (`synthTabUpdate`) — tick the armed editor; return `changed=true` while animating.
- Modify: `src/go/internal/ui/synth_panel_zone.go` — in `drawSynthDetailPane`, invalidate the armed card's `TraceSprite` while the editor animates so the kinetic lanes bypass the cache.
- Test: `src/go/internal/ui/synth_armed_anim_test.go` (Create)

**Interfaces:**
- Consumes: `ArmedControlEditor.Tick`, `synthSpriteCache.card(id)`, `TraceSprite.Invalidate`.

- [ ] **Step 1: Write the failing test**

```go
package ui

import "testing"

func TestArmedEditorTicksUntilSettled(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	forceSynthLayout(g)
	idx := dv.synthSelectedSection().knobIdxs[0]
	dv.armSynthControl(inst, idx)
	dv.instEditorArmedEditor.value().ImpulseNotch(+1)

	// synthTabUpdate must report "changed" while the editor animates, then stop.
	sawChange := false
	for i := 0; i < 300; i++ {
		if dv.synthTabUpdate() {
			sawChange = true
		} else if sawChange {
			return // settled after animating — good
		}
	}
	t.Fatal("armed editor animation should drive synthTabUpdate and then settle")
}
```

- [ ] **Step 2: Run** — FAIL (editor not ticked by `synthTabUpdate`).

- [ ] **Step 3: Implement** — in `synthTabUpdate`, after the section-grid tick loop:

```go
if dv.instEditorArmedEditor != nil {
	if dv.instEditorArmedEditor.Tick() {
		changed = true
		// Bypass the card cache so the kinetic lanes redraw every frame.
		if idx, armed := dv.synthControlArmed(instID); armed {
			if sp := dv.synp().sprites.card(armedCardCacheKey(instID, idx)); sp != nil {
				sp.Invalidate()
			}
		}
	}
}
```

Use whatever the card-sprite key convention is (`synthSpriteCache.card(id string)`); add a tiny `armedCardCacheKey` helper matching existing card keys. If the armed card is drawn outside the per-card `TraceSprite` (drawn directly in `drawSynthDetailPane`), instead ensure `drawSynthDetailPane` renders the armed card straight to `dst` (never cached) while `editor.Animating()`.

- [ ] **Step 4: Run** — `-run 'TestArmedEditorTicksUntilSettled'` PASS. Then run the alloc/fingerprint discipline: `-run 'TestSynthLayoutFingerprintStable|TestSynthTabDrag'` and any `perTabAllocBudget`/drag byte-budget test — confirm budgets hold (armed animation is a transient runtime state, not the default draw path).

- [ ] **Step 5: Commit** — `feat(synth): tick armed editor + bypass card cache while animating`

---

### Task 13: Retire the synth wheel popup path

**Files:**
- Modify: `src/go/internal/ui/drumview_wheel_popup.go` — remove the synth open path (`openSynthControlWheelPopup` body → no-op or delete + callers), keep `MobileWheelPopup` type for Sampler/EQ.
- Modify: any remaining synth caller of `openSynthControlWheelPopup` (should be gone after Task 9).
- Test: `src/go/internal/ui/synth_no_popup_test.go` (Create)

**Interfaces:**
- Produces: no synth path opens `synthWheelPopup`.

- [ ] **Step 1: Write the failing test**

```go
package ui

import "testing"

func TestSynthTapDoesNotOpenWheelPopup(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	forceSynthLayout(g)
	idx := dv.synthSelectedSection().knobIdxs[0]
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: inst}
	r := dv.instEditorControls[idx].Rect()
	h.OnPress(r.Min.X+2, r.Min.Y+2)
	h.OnRelease(r.Min.X+2, r.Min.Y+2)
	if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
		t.Fatal("synth tap must arm inline, never open the barrel popup")
	}
	if _, armed := dv.synthControlArmed(inst); !armed {
		t.Fatal("synth tap should arm")
	}
}
```

- [ ] **Step 2: Run** — should already PASS if Task 9 removed the popup call; if it FAILS, a stray caller remains.

- [ ] **Step 3: Implement** — grep `openSynthControlWheelPopup` / `synthWheelPopup` in the synth path; remove synth usages. Leave `sampler_wheel_popup.go` / `eq_wheel_popup.go` untouched. Update/delete now-stale tests (`drumview_wheel_test.go` synth cases; the JS `synthControlRects` note in `project_synth_control_wheel_model_stale_js_tests` — no JS change needed unless an export shape changed).

- [ ] **Step 4: Run** — `-run 'TestSynthTapDoesNotOpenWheelPopup'` PASS; full `./internal/ui` build clean; sampler/eq popup tests still green (`-run 'Sampler.*Wheel|EQWheel'`).

- [ ] **Step 5: Commit** — `refactor(synth): retire barrel popup from synth path`

---

### Task 14: Scenes + mandatory visual verification (iterate to pristine)

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go` — add `crop_synth_control_armed` with `Subject` + `MobileSetup`.
- Modify: `src/go/internal/ui/screenshot_subjects.go` — subject rect for the armed card (if a new subject is needed; else reuse the synth-tab subject).
- Modify: `src/go/internal/ui/audio_panel_render_pixels_test.go` — extend to color-count the armed lane rects (armed = new audio-panel data pixels).

**Interfaces:**
- Consumes: `armSynthControl`, the scene helpers (`scopeZoneOf`, modular synth tab setup).

- [ ] **Step 1: Add the scene**

In `scene_catalog.go`, mirror the one-liner style:

```go
{Name: "crop_synth_control_armed", Subject: SubjectSynthArmedCard, Mobile: true,
	Setup: func(g *Game) {
		openModularSynthTab(g) // reuse the existing synth-tab scene setup
		inst := g.drum.synthTabActiveInstrument()
		sec := g.drum.synthSelectedSection()
		g.drum.armSynthControl(inst, sec.knobIdxs[0])
	},
	MobileSetup: func(g *Game) { /* same, with mobile audio-panel exposed */ }},
```

- [ ] **Step 2: Build the screenshot binary**

Run:
```bash
make build/libdrums.a
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_ss ./cmd
```

- [ ] **Step 3: Capture desktop + mobile**

```bash
xvfb-run -a /tmp/beatmo_ss -log ERROR -scene crop_synth_control_armed -screenshot /tmp/armed_desktop.png
xvfb-run -a /tmp/beatmo_ss -mobile -win-w 414 -win-h 896 -scene crop_synth_control_armed -scene-pass mobile -screenshot /tmp/armed_mobile.png
```

- [ ] **Step 4: OPEN both PNGs and inspect** (mandatory). Confirm:
  - Card visibly grew; both lanes render (res left / value right; value-only when discrete).
  - Center value bold + accent; a thin accent hairline (or the pill, if you switch) under it.
  - Neighbor values fade toward the card edges; no cylinder/keycap/ridge chrome.
  - No overlap with neighboring cards; header (param name) still legible in the reserved band.
  - **This is NOT the popup look** — flat, in-touch with the card.
  Fix the renderer/layout/tokens and **re-capture until pristine**. Iterate.

- [ ] **Step 5: Pixel regression test** — extend `audio_panel_render_pixels_test.go` to color-count non-background pixels inside the armed value-lane rect (>0 data pixels). Run it.

- [ ] **Step 6: Full sweep + device-direction check**
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/
```
Then have the user run an uncommitted WASM build (`make wasm && make run`) on their device to pin the **scroll/drag direction sign** (down = increase; res down = coarser). Flip the `sign(steps)` in `ArmedControlEditor.Wheel` / the drag sign only if the device disagrees; re-verify against `TestBarrelWheel_FlingDecaysAndSnaps` and the arm tests.

- [ ] **Step 7: Commit** — `feat(synth): armed-control scene + visual verification`

---

## Self-Review

**Spec coverage:**
- §3.1 arm/lock state machine → Task 4 (+ toggle/lock via Task 9 handlers, Esc via Task 10).
- §3.2 expand-in-place layout → Task 7.
- §3.3 two inertial wheels + no duplicated state → Tasks 2, 3, 5; commit pipeline reused in Task 5 `onEdit`.
- §3.3 numeric entry kept → Task 11.
- §3.4 input mapping (desktop hover-lane wheel, not-armed→scroll; mobile swipe; arrows) → Tasks 9 (pointer/wheel), 10 (keyboard).
- §3.5 smooth continuous + momentum row scroll → Task 8.
- §4.1–4.2 BarrelWheel + inertia → Task 2.
- §4.3 modern flat renderer (NOT barrel popup) → Task 6 + Task 14 visual gate.
- §4.4 adapter wiring / drop popup open → Tasks 9, 13.
- §4.5 per-frame tick + cache bypass + budgets → Task 12.
- §5 input isolation (capture-on-press, no modal portal) → Task 9 (`InputCaptured`); no new portal introduced.
- §6 density tokens → Task 1.
- §7 testing + mandatory screenshots → every task's tests + Task 14.
- §9 out-of-scope (Sampler/EQ popups) → Task 13 explicitly leaves them.

**Placeholder scan:** No "TBD/TODO". Two deferred *visual* choices (hairline-vs-pill, exact inertia constants) are explicitly resolved in the Task 14 screenshot loop with concrete starting values given — not placeholders in requirements. Helper-name fallbacks (`forceSynthLayout`, `pressKeyForTest`, `rect0`, text-sink) instruct reuse of the existing test helper whose exact name the implementer confirms in-file — this avoids inventing parallel helpers.

**Type consistency:** `barrelBinding` (labelAt/stepBy/canStep) identical across Tasks 2, 3, 6. `BarrelWheel` methods (Bind/BeginDrag/DragTo/EndDrag/ImpulseNotch/Tick/Animating/OffsetPx) consistent Tasks 2, 5, 12, 14. `ArmedControlEditor` API (SetCardRect/ValueLaneRect/ResLaneRect/laneAt/BeginDrag/DragTo/EndDrag/Wheel/StepValue/StepResolution/Tick/value/res) consistent Tasks 5, 9, 10, 12. `synthControlArmed/armSynthControl/lockSynthControl/toggleSynthControlArm` consistent Tasks 4, 9, 10, 12, 13. `instEditorArmedControl`/`instEditorArmedEditor` fields declared once (Task 4), used consistently. Density fields `SynthArmedCardH/SynthWheelLaneW/SynthWheelRowH/SynthWheelVisibleRows` consistent Tasks 1, 5, 7.
