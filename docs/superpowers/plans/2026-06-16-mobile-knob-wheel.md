# Mobile Knob → Vertical Scroll-Wheel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On mobile, replace each rotary synth/sampler knob with a tap-to-open vertical scroll-wheel popup (barrel picker) that changes value with endless/relative vertical drag and a draggable resolution strip; desktop keeps the rotary knob unchanged.

**Architecture:** The value math already lives on `*Knob` (`Endless`/`StepMul` accumulation clamped at `Scale.Min/Max`) and `*KnobStepBadge` (persisted resolution rungs). We add a delta-based driver method on `Knob`, a new `MobileWheelPopup` (sibling to `SliderPopup`) that is purely a *view + vertical gesture* over the **same** `*Knob` bound in `instEditorKnobs[idx]`, a portal overlay mirroring `slider_popup_portal.go`, and a single `Profile().IsMobile()` branch in `synthKnobHitAdapter.OnPress` + the knob draw site. One value model, two views.

**Tech Stack:** Go 1.23, Ebiten, `internal/ui` package. Bundled Go: `.tools/go/bin/go`.

## Global Constraints

- **Bundled Go only:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/` is the fast test path. Never `go test ./...` (a phantom duplicate `internal/ui` package breaks it — use explicit package paths).
- **NEVER `git add`/commit/stash/restore/rm** in this repo (this is a hard user rule — the branch carries large uncommitted WIP). Each task's final step is a **verification checkpoint**, not a commit. Leave all changes in the working tree.
- **Desktop must stay byte-for-byte unchanged.** Every new behavior is gated on `Profile().IsMobile()`. The only desktop edit is one added `if Profile().IsMobile() { … return }` branch ahead of the existing desktop path.
- **Single value model.** The wheel never stores its own value; it reads/writes `dv.instEditorKnobs[idx].Value` and resolution through `dv.instEditorStepBadges[idx]`. No duplicated value state.
- **No new opacity/color literals.** Reuse `Token*()` accessors and existing draw helpers (`drawSliderRail`, `KnobStepBadge.Draw`, `formatSynthParamValue`). New sizing → DESIGN.md density tokens.
- **Functional tests over isolation** (project rule): drive the real `dv.Update()` input loop on a mobile profile and assert user-visible outcomes; back model logic with unit tests.
- The full `internal/ui` suite has **pre-existing unrelated failures** on this branch. Verify your work with `-run` filters, never by a clean full-suite pass.

---

## File Structure

**New files:**
- `src/go/internal/ui/mobile_wheel_popup.go` — `MobileWheelPopup` struct, `WheelBinding`, constructor, `Open/Close/IsOpen/Rect/Anchor/IsDragging`, `HandleInput` (vertical value drag + resolution-strip drag + center-value tap).
- `src/go/internal/ui/mobile_wheel_render.go` — `(*MobileWheelPopup).Draw` (barrel ticks with alpha/spacing falloff, center value box, resolution strip, accent).
- `src/go/internal/ui/mobile_wheel_popup_portal.go` — `mobileWheelPopupPortalOverlay` + `mobileWheelHitHandler` (mirrors `slider_popup_portal.go`).
- `src/go/internal/ui/drumview_wheel_popup.go` — `openSynthKnobWheelPopup` / `closeSynthKnobWheelPopup` + portal open/close helpers.
- Test files: `mobile_wheel_value_test.go`, `mobile_wheel_popup_test.go`, `mobile_wheel_resolution_test.go`, `mobile_wheel_render_test.go`, `mobile_wheel_seam_test.go`.

**Modified files:**
- `src/go/internal/ui/knob.go` — add `NudgeEndless`.
- `src/go/internal/ui/synth_panel_zone.go` — mobile branch in `synthKnobHitAdapter.OnPress`; mobile Pill draw at the knob draw site; `openSynthKnobWheelPopup` builder.
- `src/go/internal/ui/drumview_ctor.go` — construct the popup instance + portal wiring fields.
- `DESIGN.md` — density tokens for wheel sizing; then `make gen-design-tokens`.
- A scene catalog file (`scene_catalog.go`) — a `crop_*` mobile scene for the open wheel.

---

### Task 1: Delta-based endless driver on Knob

**Files:**
- Modify: `src/go/internal/ui/knob.go` (add method near `updateEndless`, ~line 431)
- Test: `src/go/internal/ui/mobile_wheel_value_test.go`

**Interfaces:**
- Produces: `func (k *Knob) NudgeEndless(deltaPx int)` — accumulate an endless value change by `deltaPx` pixels (positive = increase), clamped at `Scale.Min/Max`. Mirrors `updateEndless` math but is delta-based and stateless (caller computes per-frame delta), so it needs no `endlessLastX`.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/mobile_wheel_value_test.go
package ui

import (
	"math"
	"testing"
)

func newEndlessKnobForTest(min, max, stepMul float64) *Knob {
	k := &Knob{Endless: true, StepMul: stepMul}
	k.Scale = KnobScale{Min: min, Max: max}
	k.Value = 0.5 // midpoint
	return k
}

func realValue(k *Knob) float64 {
	return k.Scale.Min + k.Value*(k.Scale.Max-k.Scale.Min)
}

func TestNudgeEndless_IncreasesByStep(t *testing.T) {
	// span 200, stepMul 1.0, knobEndlessPxPerNotch px = 1 step.
	k := newEndlessKnobForTest(0, 200, 1.0)
	start := realValue(k) // 100
	k.NudgeEndless(knobEndlessPxPerNotch * 5)
	got := realValue(k)
	if math.Abs(got-(start+5)) > 1e-6 {
		t.Fatalf("NudgeEndless(+5 notches): got %v want %v", got, start+5)
	}
}

func TestNudgeEndless_NegativeDecreases(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	start := realValue(k)
	k.NudgeEndless(-knobEndlessPxPerNotch * 3)
	got := realValue(k)
	if math.Abs(got-(start-3)) > 1e-6 {
		t.Fatalf("NudgeEndless(-3 notches): got %v want %v", got, start-3)
	}
}

func TestNudgeEndless_ClampsAtMax(t *testing.T) {
	k := newEndlessKnobForTest(0, 10, 1.0)
	k.NudgeEndless(knobEndlessPxPerNotch * 10000)
	if got := realValue(k); math.Abs(got-10) > 1e-6 {
		t.Fatalf("clamp at max: got %v want 10", got)
	}
}

func TestNudgeEndless_ClampsAtMin(t *testing.T) {
	k := newEndlessKnobForTest(0, 10, 1.0)
	k.NudgeEndless(-knobEndlessPxPerNotch * 10000)
	if got := realValue(k); math.Abs(got-0) > 1e-6 {
		t.Fatalf("clamp at min: got %v want 0", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestNudgeEndless ./internal/ui/`
Expected: FAIL — `k.NudgeEndless undefined`.

- [ ] **Step 3: Add the method** (in `knob.go`, immediately after `updateEndless`)

```go
// NudgeEndless accumulates an endless value change by deltaPx pixels of motion
// (positive = increase). Delta-based sibling of updateEndless: the caller owns
// the per-frame delta, so this carries no latched state and is safe to drive
// from the mobile wheel popup's vertical drag. Clamps at Scale.Min/Max.
func (k *Knob) NudgeEndless(deltaPx int) {
	span := k.Scale.Max - k.Scale.Min
	if span <= 0 {
		return
	}
	step := k.StepMul
	if step <= 0 {
		step = span / 200
	}
	real := k.Scale.Min + k.Value*span
	real = clampF64(real+(float64(deltaPx)/float64(knobEndlessPxPerNotch))*step, k.Scale.Min, k.Scale.Max)
	k.Value = (real - k.Scale.Min) / span
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestNudgeEndless ./internal/ui/`
Expected: PASS (4 tests).

- [ ] **Step 5: Verification checkpoint** (do NOT commit — leave in working tree)

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
Expected: no errors. Confirm desktop knob behavior untouched (no existing method changed).

---

### Task 2: Density tokens for wheel sizing

**Files:**
- Modify: `DESIGN.md` (YAML front matter, `densities:` block — all three densities)
- Generated (do not hand-edit): `src/go/internal/ui/design_density.gen.go`

**Interfaces:**
- Produces (read via `Profile().DensityValues()`): `MobileWheelW`, `MobileWheelH`, `MobileWheelTickGap`, `MobileWheelResStripW`, `MobileWheelPillH` (all `int`).

- [ ] **Step 1: Add tokens to DESIGN.md `densities:` block**

For EACH of `densities.compact`, `densities.comfortable`, `densities.spacious`, add (kebab-case keys, matching the existing block's style — see `synthKnobIdeal`/`knobStepBadgeW` neighbors). Suggested values:

```yaml
        mobileWheelW: 200          # compact
        mobileWheelH: 260
        mobileWheelTickGap: 30
        mobileWheelResStripW: 36
        mobileWheelPillH: 34
```
```yaml
        mobileWheelW: 220          # comfortable
        mobileWheelH: 300
        mobileWheelTickGap: 34
        mobileWheelResStripW: 40
        mobileWheelPillH: 38
```
```yaml
        mobileWheelW: 240          # spacious (mobile default)
        mobileWheelH: 340
        mobileWheelTickGap: 38
        mobileWheelResStripW: 44
        mobileWheelPillH: 44       # >= 44px touch-min
```

- [ ] **Step 2: Regenerate tokens**

Run: `make gen-design-tokens`
Expected: `src/go/internal/ui/design_density.gen.go` now declares `MobileWheelW`, `MobileWheelH`, `MobileWheelTickGap`, `MobileWheelResStripW`, `MobileWheelPillH` on the density-values struct.

- [ ] **Step 3: Write a guard test that the fields are populated**

```go
// append to mobile_wheel_value_test.go
func TestMobileWheelDensityTokensPresent(t *testing.T) {
	dvals := Profile().DensityValues()
	if dvals.MobileWheelW <= 0 || dvals.MobileWheelH <= 0 ||
		dvals.MobileWheelTickGap <= 0 || dvals.MobileWheelResStripW <= 0 ||
		dvals.MobileWheelPillH <= 0 {
		t.Fatalf("wheel density tokens must be positive, got %+v", dvals)
	}
}
```

- [ ] **Step 4: Run it**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestMobileWheelDensityTokensPresent ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Verification checkpoint** (do NOT commit)

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline' ./internal/ui/`
Expected: PASS (density additions don't affect color/spacing drift; if `TestDesignMDLintSnapshot` warns, that's network-gated — ignore if `npx` unavailable).

---

### Task 3: MobileWheelPopup value core (open/close + vertical value drag)

**Files:**
- Create: `src/go/internal/ui/mobile_wheel_popup.go`
- Test: `src/go/internal/ui/mobile_wheel_popup_test.go`

**Interfaces:**
- Consumes: `Knob.NudgeEndless` (Task 1); `Knob.StepValueByWheel(steps int) bool`; `KnobScale`; `audio.ParamDef`; `Profile()`, `AnchorPopupRect`, `PopupBelow` (existing popup placement helpers used by `SliderPopup.Open`).
- Produces:
  - `type WheelBinding struct { Knob *Knob; Badge *KnobStepBadge; Def audio.ParamDef; Discrete bool; OnChange func(); OnCommit func(); OnResolution func(); OpenEditor func(); Title func() string; Accent func() color.Color }`
  - `func NewMobileWheelPopup() *MobileWheelPopup`
  - `func (w *MobileWheelPopup) Open(b WheelBinding, anchor, bounds image.Rectangle, headerH int)`
  - `func (w *MobileWheelPopup) Close()`
  - `func (w *MobileWheelPopup) IsOpen() bool`
  - `func (w *MobileWheelPopup) Rect() image.Rectangle`
  - `func (w *MobileWheelPopup) Anchor() image.Rectangle`
  - `func (w *MobileWheelPopup) IsDragging() bool`
  - `func (w *MobileWheelPopup) HandleInput(mx, my int, pressed bool) bool`

- [ ] **Step 1: Write the failing test** (drag up increases; clamps; release commits)

```go
// src/go/internal/ui/mobile_wheel_popup_test.go
package ui

import (
	"image"
	"math"
	"testing"

	"beatmo/internal/audio"
)

func wheelTestBinding(k *Knob) (*WheelBinding, *int) {
	commits := 0
	b := &WheelBinding{
		Knob:     k,
		Def:      audio.ParamDef{Name: "test", Min: k.Scale.Min, Max: k.Scale.Max},
		OnChange: func() {},
		OnCommit: func() { commits++ },
		Title:    func() string { return "Test" },
	}
	return b, &commits
}

func TestWheelDragUpIncreasesValue(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0) // value 0.5 -> real 100
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	cx := w.Rect().Min.X + 20
	// press near vertical center, then drag the finger UP (y decreases) = increase.
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0-knobEndlessPxPerNotch*10, true)
	w.HandleInput(cx, y0-knobEndlessPxPerNotch*10, false)

	if got := realValue(k); got <= 100 {
		t.Fatalf("drag up should increase value past 100, got %v", got)
	}
}

func TestWheelDragDownDecreasesValue(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0+knobEndlessPxPerNotch*10, true)
	w.HandleInput(cx, y0+knobEndlessPxPerNotch*10, false)
	if got := realValue(k); got >= 100 {
		t.Fatalf("drag down should decrease value below 100, got %v", got)
	}
}

func TestWheelReleaseCommitsOnce(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	b, commits := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0-20, true)
	w.HandleInput(cx, y0-20, false)
	if *commits != 1 {
		t.Fatalf("expected exactly 1 commit on release, got %d", *commits)
	}
}

func TestWheelClampsAtMax(t *testing.T) {
	k := newEndlessKnobForTest(0, 10, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0-knobEndlessPxPerNotch*100000, true)
	w.HandleInput(cx, y0-knobEndlessPxPerNotch*100000, false)
	if got := realValue(k); math.Abs(got-10) > 1e-6 {
		t.Fatalf("clamp at max: got %v want 10", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheel ./internal/ui/`
Expected: FAIL — `NewMobileWheelPopup`/`WheelBinding` undefined.

- [ ] **Step 3: Implement the popup core**

```go
// src/go/internal/ui/mobile_wheel_popup.go
package ui

import (
	"image"
	"image/color"

	"beatmo/internal/audio"
)

// wheelDragKind disambiguates which sub-control the active drag belongs to.
type wheelDragKind int

const (
	wheelDragNone wheelDragKind = iota
	wheelDragValue
	wheelDragRes
)

// WheelBinding wires a MobileWheelPopup to the single shared knob value-model.
// The popup stores NO value of its own — it reads/writes Knob and Badge.
type WheelBinding struct {
	Knob         *Knob
	Badge        *KnobStepBadge // nil for discrete/enum params (no resolution strip)
	Def          audio.ParamDef
	Discrete     bool // enum or Step>0: step detents instead of endless accumulation
	OnChange     func()
	OnCommit     func()
	OnResolution func()
	OpenEditor   func()
	Title        func() string
	Accent       func() color.Color
}

// MobileWheelPopup is the mobile vertical scroll-wheel view over a single knob.
type MobileWheelPopup struct {
	binding  WheelBinding
	open     bool
	rect     image.Rectangle
	anchor   image.Rectangle
	resRect  image.Rectangle // resolution strip sub-rect (set in layout)
	valRect  image.Rectangle // barrel sub-rect
	centerH  image.Rectangle // center value box (tap → numeric entry)
	dragging bool
	dragKind wheelDragKind
	lastY    int
	discAcc  int // pixel accumulator for discrete detent stepping
	resAcc   int // pixel accumulator for resolution-strip drag
}

func NewMobileWheelPopup() *MobileWheelPopup { return &MobileWheelPopup{} }

func (w *MobileWheelPopup) Open(b WheelBinding, anchor, bounds image.Rectangle, headerH int) {
	w.binding = b
	w.anchor = anchor
	dvals := Profile().DensityValues()
	pw, ph := dvals.MobileWheelW, dvals.MobileWheelH
	w.rect = AnchorPopupRect(bounds, anchor, pw, ph, PopupBelow)
	w.layout()
	w.open = true
	w.dragging = false
	w.dragKind = wheelDragNone
	w.discAcc = 0
	w.resAcc = 0
}

// layout splits the panel into a barrel column, an optional resolution strip,
// and a center value box. Discrete params hide the strip.
func (w *MobileWheelPopup) layout() {
	dvals := Profile().DensityValues()
	inner := w.rect.Inset(SpaceSM)
	resW := 0
	if w.binding.Badge != nil && !w.binding.Discrete {
		resW = dvals.MobileWheelResStripW
	}
	w.valRect = image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X-resW, inner.Max.Y)
	if resW > 0 {
		w.resRect = image.Rect(inner.Max.X-resW, inner.Min.Y, inner.Max.X, inner.Max.Y)
	} else {
		w.resRect = image.Rectangle{}
	}
	// center value box: a band centered vertically in the barrel column.
	pillH := dvals.MobileWheelPillH
	cy := (w.valRect.Min.Y + w.valRect.Max.Y) / 2
	w.centerH = image.Rect(w.valRect.Min.X, cy-pillH/2, w.valRect.Max.X, cy+pillH/2)
}

func (w *MobileWheelPopup) Close()                { w.open = false; w.dragging = false; w.dragKind = wheelDragNone }
func (w *MobileWheelPopup) IsOpen() bool          { return w.open }
func (w *MobileWheelPopup) Rect() image.Rectangle { return w.rect }
func (w *MobileWheelPopup) Anchor() image.Rectangle { return w.anchor }
func (w *MobileWheelPopup) IsDragging() bool      { return w.dragging && w.dragKind == wheelDragValue }

// HandleInput routes a pointer event. Returns true if the popup consumed it.
func (w *MobileWheelPopup) HandleInput(mx, my int, pressed bool) bool {
	if !w.open {
		return false
	}
	if !pressed {
		if w.dragging && w.dragKind == wheelDragValue && w.binding.OnCommit != nil {
			w.binding.OnCommit()
		}
		w.dragging = false
		w.dragKind = wheelDragNone
		return true
	}
	pt := image.Pt(mx, my)
	if !w.dragging {
		// Press: decide which sub-control this gesture owns.
		switch {
		case !w.binding.Discrete && pt.In(w.centerH) && w.binding.OpenEditor != nil:
			w.binding.OpenEditor()
			return true
		case !w.resRect.Empty() && pt.In(w.resRect):
			w.dragging = true
			w.dragKind = wheelDragRes
			w.lastY = my
			w.resAcc = 0
		default:
			w.dragging = true
			w.dragKind = wheelDragValue
			w.lastY = my
			w.discAcc = 0
		}
		return true
	}
	// Continue drag.
	switch w.dragKind {
	case wheelDragValue:
		delta := w.lastY - my // finger up (y decreases) => positive => increase
		w.lastY = my
		w.applyValueDelta(delta)
		if w.binding.OnChange != nil {
			w.binding.OnChange()
		}
	case wheelDragRes:
		w.applyResDelta(w.lastY - my)
		w.lastY = my
	}
	return true
}

// applyValueDelta accumulates a vertical delta into the bound knob: endless
// for continuous params, detent stepping for discrete/enum params.
func (w *MobileWheelPopup) applyValueDelta(delta int) {
	k := w.binding.Knob
	if k == nil {
		return
	}
	if w.binding.Discrete {
		w.discAcc += delta
		gap := Profile().DensityValues().MobileWheelTickGap
		for w.discAcc >= gap {
			k.StepValueByWheel(1)
			w.discAcc -= gap
		}
		for w.discAcc <= -gap {
			k.StepValueByWheel(-1)
			w.discAcc += gap
		}
		return
	}
	k.NudgeEndless(delta)
}

// applyResDelta drags the resolution strip across the KnobStepBadge rungs and
// keeps the knob's StepMul in sync. One rung shift per tick-gap of travel.
func (w *MobileWheelPopup) applyResDelta(delta int) {
	b := w.binding.Badge
	if b == nil {
		return
	}
	w.resAcc += delta
	gap := Profile().DensityValues().MobileWheelTickGap
	changed := false
	for w.resAcc >= gap { // drag up = finer
		if b.AdjustResolution(1) {
			changed = true
		}
		w.resAcc -= gap
	}
	for w.resAcc <= -gap { // drag down = coarser
		if b.AdjustResolution(-1) {
			changed = true
		}
		w.resAcc += gap
	}
	if changed {
		if w.binding.Knob != nil {
			w.binding.Knob.StepMul = b.Step()
		}
		if w.binding.OnResolution != nil {
			w.binding.OnResolution()
		}
	}
}
```

> NOTE: `AnchorPopupRect`, `PopupBelow`, `SpaceSM` are existing symbols (used by `SliderPopup.Open` at `slider_popup.go:118`). If `AnchorPopupRect`'s signature differs from `(bounds, anchor, w, h, dir)`, read `slider_popup.go:88-126` and match it exactly.

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheel ./internal/ui/`
Expected: PASS (5 tests).

- [ ] **Step 5: Verification checkpoint** (do NOT commit)

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
Expected: no errors.

---

### Task 4: Resolution-strip persistence + discrete (enum) stepping tests

**Files:**
- Test: `src/go/internal/ui/mobile_wheel_resolution_test.go`
- (No new production code expected — this task proves Task 3's `applyResDelta`/`applyValueDelta` against a real `KnobStepBadge` and a discrete knob. If a gap is found, fix it in `mobile_wheel_popup.go`.)

**Interfaces:**
- Consumes: `NewKnobStepBadge(def)`, `KnobStepBadge.Step()`, `KnobStepBadge.AdjustResolution(delta int) bool`, `Knob.StepValueByWheel`.

- [ ] **Step 1: Write the failing tests**

```go
// src/go/internal/ui/mobile_wheel_resolution_test.go
package ui

import (
	"image"
	"testing"

	"beatmo/internal/audio"
)

func TestWheelResolutionDragChangesStepMul(t *testing.T) {
	def := audio.ParamDef{Name: "cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	k := &Knob{Endless: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	k.Value = 0.5
	badge := NewKnobStepBadge(def)
	k.StepMul = badge.Step()
	startStep := k.StepMul

	resolved := 0
	b := WheelBinding{Knob: k, Badge: badge, Def: def,
		OnChange: func() {}, OnResolution: func() { resolved++ }, Title: func() string { return "Cutoff" }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	rx := w.resRect.Min.X + 2
	y0 := w.resRect.Min.Y + w.resRect.Dy()/2
	gap := Profile().DensityValues().MobileWheelTickGap
	w.HandleInput(rx, y0, true)
	w.HandleInput(rx, y0-gap*3, true) // drag up = finer, several rungs
	w.HandleInput(rx, y0-gap*3, false)

	if k.StepMul == startStep {
		t.Fatalf("resolution drag should change StepMul from %v", startStep)
	}
	if k.StepMul != badge.Step() {
		t.Fatalf("knob StepMul %v out of sync with badge.Step() %v", k.StepMul, badge.Step())
	}
	if resolved == 0 {
		t.Fatal("OnResolution callback never fired")
	}
}

func TestWheelEnumStepsLabels(t *testing.T) {
	def := audio.ParamDef{Name: "wave", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Tri"}}
	k := &Knob{Discrete: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Enum: def.Enum}
	k.Value = 0 // Sine
	b := WheelBinding{Knob: k, Def: def, Discrete: true,
		OnChange: func() {}, OnCommit: func() {}, Title: func() string { return "Wave" }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	// no resolution strip for enums
	if !w.resRect.Empty() {
		t.Fatal("enum wheel must not show a resolution strip")
	}

	cx := w.valRect.Min.X + 4
	y0 := w.valRect.Min.Y + w.valRect.Dy()/2
	gap := Profile().DensityValues().MobileWheelTickGap
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0+gap, true) // one detent worth, drag down
	w.HandleInput(cx, y0+gap, false)

	if k.Value == 0 {
		t.Fatalf("enum wheel drag should advance the detent; value still %v", k.Value)
	}
}
```

- [ ] **Step 2: Run to verify failure (or pass)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestWheelResolutionDragChangesStepMul|TestWheelEnumStepsLabels' ./internal/ui/`
Expected: If Task 3 was complete, these may PASS directly. If they FAIL, the failure pinpoints a gap in `applyResDelta`/`applyValueDelta` — fix it in `mobile_wheel_popup.go` (e.g. enum sign direction) and re-run.

- [ ] **Step 3: Fix any gap surfaced** (only if Step 2 failed; otherwise skip)

Adjust `applyResDelta`/`applyValueDelta` direction or accumulator logic minimally to satisfy the assertions.

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestWheelResolution|TestWheelEnum' ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Verification checkpoint** (do NOT commit)

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheel ./internal/ui/`
Expected: all wheel value/resolution tests PASS together.

---

### Task 5: Barrel + strip rendering

**Files:**
- Create: `src/go/internal/ui/mobile_wheel_render.go`
- Test: `src/go/internal/ui/mobile_wheel_render_test.go`

**Interfaces:**
- Produces: `func (w *MobileWheelPopup) Draw(dst *ebiten.Image)`.
- Consumes: `formatSynthParamValue(def audio.ParamDef, value float64) string` (param_registry.go), `KnobStepBadge.Draw`, existing draw helpers. Study `slider_popup.go:185-280` (`SliderPopup.Draw`) and `knob_step.go:236` (`KnobStepBadge.Draw`) and mirror their helper usage (panel fill, rounded rect, `DrawTextStyled`/`TextRole`, accent fill, token colors). Do not introduce raw `color.RGBA` literals.

**Rendering spec:**
- Panel background + border: copy the panel chrome from `SliderPopup.Draw`.
- Title (`binding.Title()`) at the top.
- Barrel in `w.valRect`: for tick offsets `n` in `-N..N` (where `N` fills the column at `MobileWheelTickGap` spacing), draw the tick label centered at `centerY + n*gap`:
  - Continuous: real value = `def.Min + Value*span + n*StepMul`; skip ticks outside `[Min,Max]`; label via `formatSynthParamValue(def, real)`.
  - Discrete: enum index = `round(Value*(len-1)) + n`; skip out-of-range; label = `def.Enum[idx]`.
  - Alpha falloff: `alpha = 1 - |n|/(N+1)` (ticks fade toward edges). Center tick (`n==0`) is brightest.
- Center value box (`w.centerH`): bright boxed live value (accent fill from `binding.Accent()` if non-nil, else primary).
- Resolution strip (`w.resRect`, when non-empty): a dim rail with the active rung highlighted; render `binding.Badge.Label()` rotated/stacked or as a short text. Simplest acceptable: draw the rail + the badge via `binding.Badge.Draw(dst)` positioned inside `w.resRect` (set the badge's rect first if an exported setter exists; otherwise draw rail + `Label()` text).

- [ ] **Step 1: Write the failing test** (draw-recorder: center value text drawn; multiple ticks drawn)

```go
// src/go/internal/ui/mobile_wheel_render_test.go
package ui

import (
	"image"
	"testing"

	"beatmo/internal/audio"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestWheelDrawRendersCenterAndTicks(t *testing.T) {
	def := audio.ParamDef{Name: "cutoff", Min: 0, Max: 200, Unit: "Hz"}
	k := &Knob{Endless: true, StepMul: 1.0}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	k.Value = 0.5
	b := WheelBinding{Knob: k, Def: def, OnChange: func() {}, OnCommit: func() {}, Title: func() string { return "Cutoff" }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	rec := newTextDrawRecorder(t) // see helper note below
	defer rec.restore()

	dst := ebiten.NewImage(400, 800)
	w.Draw(dst)

	if rec.count("100") == 0 && rec.count("100 Hz") == 0 {
		t.Fatalf("center value (~100) not rendered; saw texts: %v", rec.texts())
	}
	if rec.distinctTexts() < 3 {
		t.Fatalf("expected several tick labels, saw %d distinct", rec.distinctTexts())
	}
}
```

> HELPER NOTE: This package already has a draw-interception pattern (see `audio_panel_render_pixels_test.go`, which intercepts `drawRect`; and text tests that count `DrawTextStyled`). Use the SAME mechanism the package already uses to capture drawn text. If a text recorder helper does not exist, instead assert on a non-text signal that the package already exposes (e.g. count filled rects via the existing `drawRect` interceptor used in `audio_panel_render_pixels_test.go`), and assert the center box + ≥3 tick rows draw. Match the existing harness — do not invent a new global hook.

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheelDrawRenders ./internal/ui/`
Expected: FAIL — `w.Draw` undefined.

- [ ] **Step 3: Implement `Draw`** in `mobile_wheel_render.go`, following the Rendering spec above and mirroring `SliderPopup.Draw`. Use `formatSynthParamValue` for labels, alpha falloff per tick, accent for the center box, and `binding.Badge` for the strip. Guard `if !w.open { return }`.

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheelDrawRenders ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Verification checkpoint** (do NOT commit)

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestWheel|TestTokenDiscipline' ./internal/ui/`
Expected: PASS (no new color literals tripping `TestTokenDiscipline`; if it trips, replace literals with `Token*()`/`WithAlpha`).

---

### Task 6: Portal overlay + hit handler

**Files:**
- Create: `src/go/internal/ui/mobile_wheel_popup_portal.go`
- Test: `src/go/internal/ui/mobile_wheel_popup_test.go` (append)

**Interfaces:**
- Produces: `type mobileWheelPopupPortalOverlay struct { popup *MobileWheelPopup; tag string }` implementing the same overlay interface as `sliderPopupPortalOverlay` (`Layout`, `HitAreas`, `Draw`, `ShouldClose`); and `type mobileWheelHitHandler struct { popup *MobileWheelPopup }` implementing `OnPress/OnDrag/OnRelease/OnWheel`.
- Consumes: `HitArea`, `InputResult` constants, `PortalEntry`.

- [ ] **Step 1: Write the failing test**

```go
// append to mobile_wheel_popup_test.go
func TestWheelPortalOverlayHitAreasAndClose(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(5, 5, 15, 15), image.Rect(0, 0, 400, 800), 0)

	o := &mobileWheelPopupPortalOverlay{popup: w, tag: "synth-wheel-popup"}
	areas := o.HitAreas()
	if len(areas) == 0 {
		t.Fatal("open wheel must publish at least one hit area")
	}
	if o.ShouldClose() {
		t.Fatal("ShouldClose must be false while open")
	}
	w.Close()
	if !o.ShouldClose() {
		t.Fatal("ShouldClose must be true after Close")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheelPortalOverlay ./internal/ui/`
Expected: FAIL — overlay type undefined.

- [ ] **Step 3: Implement, mirroring `slider_popup_portal.go:10-75`**

```go
// src/go/internal/ui/mobile_wheel_popup_portal.go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

type mobileWheelPopupPortalOverlay struct {
	popup *MobileWheelPopup
	tag   string
}

func (o *mobileWheelPopupPortalOverlay) Layout(anchor, screenBounds image.Rectangle) {}

func (o *mobileWheelPopupPortalOverlay) HitAreas() []HitArea {
	r := o.popup.Rect()
	if r.Empty() || !o.popup.IsOpen() {
		return nil
	}
	areas := []HitArea{
		{Rect: r, Handler: &mobileWheelHitHandler{popup: o.popup}, Tag: o.tag, Touch: true},
	}
	if anchor := o.popup.Anchor(); !anchor.Empty() {
		areas = append(areas, HitArea{
			Rect:    anchor,
			Handler: &mobileWheelAnchorCloseHandler{popup: o.popup},
			Tag:     o.tag + "-anchor-close",
		})
	}
	return areas
}

func (o *mobileWheelPopupPortalOverlay) Draw(screen *ebiten.Image) { o.popup.Draw(screen) }
func (o *mobileWheelPopupPortalOverlay) ShouldClose() bool         { return !o.popup.IsOpen() }

type mobileWheelHitHandler struct{ popup *MobileWheelPopup }

func (h *mobileWheelHitHandler) OnPress(x, y int) InputResult {
	if h.popup.HandleInput(x, y, true) {
		if h.popup.IsDragging() {
			return InputCaptured
		}
		return InputConsumed
	}
	return InputIgnored
}
func (h *mobileWheelHitHandler) OnDrag(x, y int)    { h.popup.HandleInput(x, y, true) }
func (h *mobileWheelHitHandler) OnRelease(x, y int) { h.popup.HandleInput(x, y, false) }
func (h *mobileWheelHitHandler) OnWheel(x, y, steps int) InputResult { return InputConsumed }

type mobileWheelAnchorCloseHandler struct{ popup *MobileWheelPopup }

func (h *mobileWheelAnchorCloseHandler) OnPress(x, y int) InputResult {
	h.popup.Close()
	return InputConsumed
}
func (h *mobileWheelAnchorCloseHandler) OnDrag(x, y int)                {}
func (h *mobileWheelAnchorCloseHandler) OnRelease(x, y int)             {}
func (h *mobileWheelAnchorCloseHandler) OnWheel(x, y, steps int) InputResult { return InputConsumed }
```

> If the overlay/handler interfaces have additional required methods, read `slider_popup_portal.go` in full and match every method (e.g. an `OnWheel2D` if the package's handler interface requires it).

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheelPortalOverlay ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Verification checkpoint** (do NOT commit)

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
Expected: no errors (confirms the overlay/handler satisfy the real interfaces).

---

### Task 7: DrumView open/close helpers + ctor wiring + numeric-entry binding

**Files:**
- Create: `src/go/internal/ui/drumview_wheel_popup.go`
- Modify: `src/go/internal/ui/drumview.go` (add `synthWheelPopup *MobileWheelPopup` field near `instEditorKnobs`, ~line 377)
- Modify: `src/go/internal/ui/drumview_ctor.go` (construct `dv.synthWheelPopup = NewMobileWheelPopup()`)
- Modify: `src/go/internal/ui/drumview_portal_open.go` (add `openSynthWheelPortal`/`closeSynthWheelPortal` mirroring `openVolPopupPortal:335-356`)
- Test: `src/go/internal/ui/mobile_wheel_seam_test.go`

**Interfaces:**
- Produces: `func (dv *DrumView) openSynthKnobWheelPopup(idx int, instID string)`, `func (dv *DrumView) closeSynthKnobWheelPopup()`.
- Consumes: `dv.instEditorKnobs`, `dv.instEditorStepBadges`, `dv.instEditorBindings[idx].def`, `dv.synthKnobReadoutRect(idx)`, `dv.syncSliderFromKnob`, `dv.propagateSynthSliderValue`, `dv.requestSynthMirror`, `dv.commitInstrumentParams`, `dv.persistKnobStep`, `dv.openSynthParamEditor`, `dv.CloseAllPopups`, `dv.Bounds`, `dv.headerH`, `dv.tree.Portal()`, `paramSpec`, `formatSynthParamValue`.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/mobile_wheel_seam_test.go
package ui

import "testing"

func TestOpenSynthKnobWheelPopup(t *testing.T) {
	withSmallScreen(t, true)
	dv := synthWheelTestDV(t) // helper builds a DrumView with a synth instrument selected; see note
	idx := dv.firstContinuousSynthKnobIdx(t)

	dv.openSynthKnobWheelPopup(idx, dv.activeSynthInstID(t))
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		t.Fatal("openSynthKnobWheelPopup should open the wheel popup")
	}
	dv.closeSynthKnobWheelPopup()
	if dv.synthWheelPopup.IsOpen() {
		t.Fatal("closeSynthKnobWheelPopup should close it")
	}
}
```

> HELPER NOTE: Build `synthWheelTestDV`, `firstContinuousSynthKnobIdx`, `activeSynthInstID` in the test file, following the existing synth-tab test setup (search for tests that populate `instEditorKnobs`/`instEditorBindings` — e.g. knob-overhaul tests). They must drive `dv.Update()` once so the synth tab lays out its knobs. If building a fully-populated synth DrumView in a unit test is heavy, instead seed `dv.instEditorKnobs`, `dv.instEditorStepBadges`, and `dv.instEditorBindings` directly to a one-knob fixture (these are package-private slices reachable from the test).

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestOpenSynthKnobWheelPopup ./internal/ui/`
Expected: FAIL — `openSynthKnobWheelPopup`/`synthWheelPopup` undefined.

- [ ] **Step 3: Add the field + ctor**

In `drumview.go` near line 377:
```go
	instEditorKnobs      []*Knob
	instEditorStepBadges []*KnobStepBadge
	synthWheelPopup      *MobileWheelPopup // mobile vertical scroll-wheel for synth knobs
```
In `drumview_ctor.go` (where other popups like `volPopup` are constructed):
```go
	dv.synthWheelPopup = NewMobileWheelPopup()
```

- [ ] **Step 4: Implement the open/close helpers**

```go
// src/go/internal/ui/drumview_wheel_popup.go
package ui

import (
	"color" // remove — placeholder; use image/color
)

// (use: import ( "image"; "image/color" ))

// openSynthKnobWheelPopup opens the mobile scroll-wheel for synth knob idx.
func (dv *DrumView) openSynthKnobWheelPopup(idx int, instID string) {
	if idx < 0 || idx >= len(dv.instEditorKnobs) || idx >= len(dv.instEditorBindings) {
		return
	}
	dv.CloseAllPopups()
	def := dv.instEditorBindings[idx].def
	k := dv.instEditorKnobs[idx]
	var badge *KnobStepBadge
	if idx < len(dv.instEditorStepBadges) {
		badge = dv.instEditorStepBadges[idx]
	}
	discrete := len(def.Enum) > 0 || def.Step > 0

	anchor := dv.instEditorKnobs[idx].Rect()
	binding := WheelBinding{
		Knob:     k,
		Badge:    badge,
		Def:      def,
		Discrete: discrete,
		Title:    func() string { return def.Label },
		OnChange: func() {
			dv.syncSliderFromKnob(idx)
			dv.propagateSynthSliderValue(idx, instID)
		},
		OnCommit: func() {
			dv.requestSynthMirror(instID)
			dv.commitInstrumentParams(instID)
		},
		OnResolution: func() { dv.persistKnobStep(badge) },
		OpenEditor:   func() { dv.openSynthParamEditor(idx, instID) },
		Accent:       func() color.Color { return dv.synthKnobAccent(idx) }, // see note
	}
	dv.synthWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)
	dv.openSynthWheelPortal()
}

func (dv *DrumView) closeSynthKnobWheelPopup() {
	if dv.synthWheelPopup != nil {
		dv.synthWheelPopup.Close()
	}
	dv.closeSynthWheelPortal()
}
```

> CORRECTIONS REQUIRED while implementing:
> - Use real imports `"image"`, `"image/color"` (the `"color"` line above is intentionally wrong to force you to fix it).
> - `def.Label` — confirm the field name on `audio.ParamDef` (the explore showed `Label`). If absent, use `synthParamFacet(def.Name)` for the title.
> - `Accent`: if there is no `synthKnobAccent` helper, pass `Accent: nil` (the renderer falls back to primary). Do NOT invent a helper.
> - Add `openSynthWheelPortal`/`closeSynthWheelPortal` in `drumview_portal_open.go` by copying `openVolPopupPortal`/`closeVolPopupPortal` (id `"synth-wheel-popup"`, overlay `&mobileWheelPopupPortalOverlay{popup: dv.synthWheelPopup, tag: "synth-wheel-popup"}`, `Modal: true`, `Scrim: true`, `OnClose: func(){ dv.synthWheelPopup.Close() }`).

- [ ] **Step 5: Run to verify pass, then verification checkpoint** (do NOT commit)

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestOpenSynthKnobWheelPopup ./internal/ui/`
Expected: PASS.
Then: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` → no errors.

---

### Task 8: The platform swap seam (mobile OnPress opens popup; mobile draws a Pill; desktop unchanged)

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` — `synthKnobHitAdapter.OnPress` (line 3105); knob draw site (lines 2000-2008); add `drawKnobValuePill`.
- Test: `src/go/internal/ui/mobile_wheel_seam_test.go` (append)

**Interfaces:**
- Consumes: `Profile().IsMobile()`, `dv.openSynthKnobWheelPopup` (Task 7), `formatSynthParamValue`.
- Produces: `func (dv *DrumView) drawKnobValuePill(dst *ebiten.Image, idx int)`.

- [ ] **Step 1: Write the failing functional test** (real mobile Update loop)

```go
// append to mobile_wheel_seam_test.go
func TestMobileTapKnobOpensWheel_DesktopStillDrags(t *testing.T) {
	withSmallScreen(t, true)
	dv := synthWheelTestDV(t)
	idx := dv.firstContinuousSynthKnobIdx(t)
	instID := dv.activeSynthInstID(t)

	h := &synthKnobHitAdapter{dv: dv, idx: idx, instID: instID}
	kr := dv.instEditorKnobs[idx].Rect()
	res := h.OnPress(kr.Min.X+kr.Dx()/2, kr.Min.Y+kr.Dy()/2)
	if res != InputConsumed {
		t.Fatalf("mobile knob tap should be consumed to open the wheel, got %v", res)
	}
	if !dv.synthWheelPopup.IsOpen() {
		t.Fatal("mobile knob tap must open the wheel popup")
	}
}

func TestDesktopTapKnobStartsDrag(t *testing.T) {
	withSmallScreen(t, false) // desktop
	dv := synthWheelTestDV(t)
	idx := dv.firstContinuousSynthKnobIdx(t)
	instID := dv.activeSynthInstID(t)

	h := &synthKnobHitAdapter{dv: dv, idx: idx, instID: instID}
	kr := dv.instEditorKnobs[idx].Rect()
	res := h.OnPress(kr.Min.X+kr.Dx()/2, kr.Min.Y+kr.Dy()/2)
	if res != InputCaptured {
		t.Fatalf("desktop knob tap should start a drag (InputCaptured), got %v", res)
	}
	if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
		t.Fatal("desktop knob tap must NOT open the wheel popup")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestMobileTapKnobOpensWheel|TestDesktopTapKnobStartsDrag' ./internal/ui/`
Expected: FAIL — mobile branch not present (mobile tap currently returns `InputCaptured`).

- [ ] **Step 3: Add the mobile branch to `OnPress`** — insert immediately after the `paramEditor.Active()` guard (after line 3109), BEFORE the step-badge block:

```go
	// Mobile: a tap on the knob cell opens the vertical scroll-wheel popup,
	// which owns value + resolution + numeric entry. Desktop keeps the
	// in-place rotary drag (the code below).
	if Profile().IsMobile() {
		h.dv.openSynthKnobWheelPopup(h.idx, h.instID)
		return InputConsumed
	}
```

- [ ] **Step 4: Add the mobile Pill draw at the knob draw site** (lines 2000-2008) — replace `k.Draw(dst)` with:

```go
		if Profile().IsMobile() {
			dv.drawKnobValuePill(dst, kIdx)
		} else {
			k.Draw(dst)
		}
```

And add the helper (new method, same file or `mobile_wheel_render.go`):

```go
// drawKnobValuePill draws the compact mobile affordance in place of the rotary
// knob: a rounded value pill (label + current value) that opens the wheel on tap.
func (dv *DrumView) drawKnobValuePill(dst *ebiten.Image, idx int) {
	if idx < 0 || idx >= len(dv.instEditorKnobs) || idx >= len(dv.instEditorBindings) {
		return
	}
	k := dv.instEditorKnobs[idx]
	r := k.Rect()
	if r.Empty() {
		return
	}
	def := dv.instEditorBindings[idx].def
	real := def.Min + k.Value*(def.Max-def.Min)
	label := formatSynthParamValue(def, real)
	// Mirror the rounded-button chrome used elsewhere (e.g. drawRoundedButton)
	// and DrawTextStyled for the value; accent border optional. No raw literals.
	dv.drawValuePillChrome(dst, r, label) // implement via existing rounded-rect + text helpers
}
```

> NOTE: implement `drawValuePillChrome` using the SAME rounded-rect + text helpers the package already uses (search `drawRoundedButton`, `DrawTextStyled`). Keep it allocation-light (cache nothing per-frame beyond what buttons already do).

- [ ] **Step 5: Run to verify pass, then checkpoint** (do NOT commit)

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestMobileTapKnobOpensWheel|TestDesktopTapKnobStartsDrag' ./internal/ui/`
Expected: PASS.
Then: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEQPanelDrawAllocDiscipline ./internal/ui/` → PASS (Pill draw must stay within alloc budget; if it trips, cache the pill chrome like the knurled-cap cache).

---

### Task 9: End-to-end mobile wheel test, numeric-entry hookup proof, and screenshot scene

**Files:**
- Test: `src/go/internal/ui/mobile_wheel_seam_test.go` (append)
- Modify: `src/go/internal/ui/scene_catalog.go` (add a `crop_mobile_synth_wheel` scene with `MobileSetup`)

**Interfaces:**
- Consumes: everything above; the scene catalog `crop_*` + `Subject` conventions (see CLAUDE.md "Screenshot Subjects").

- [ ] **Step 1: Write the failing E2E + numeric-entry tests**

```go
// append to mobile_wheel_seam_test.go
func TestMobileWheelEndToEndDragChangesAudioValue(t *testing.T) {
	withSmallScreen(t, true)
	dv := synthWheelTestDV(t)
	idx := dv.firstContinuousSynthKnobIdx(t)
	instID := dv.activeSynthInstID(t)

	startVal := dv.instEditorKnobs[idx].Value
	dv.openSynthKnobWheelPopup(idx, instID)
	w := dv.synthWheelPopup

	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0-knobEndlessPxPerNotch*20, true)
	w.HandleInput(cx, y0-knobEndlessPxPerNotch*20, false)

	if dv.instEditorKnobs[idx].Value <= startVal {
		t.Fatalf("end-to-end drag should raise the knob value from %v", startVal)
	}
}

func TestMobileWheelCenterTapOpensNumericEditor(t *testing.T) {
	withSmallScreen(t, true)
	dv := synthWheelTestDV(t)
	idx := dv.firstContinuousSynthKnobIdx(t)
	instID := dv.activeSynthInstID(t)
	dv.openSynthKnobWheelPopup(idx, instID)
	w := dv.synthWheelPopup

	cx := (w.centerH.Min.X + w.centerH.Max.X) / 2
	cy := (w.centerH.Min.Y + w.centerH.Max.Y) / 2
	w.HandleInput(cx, cy, true)

	if dv.paramEditor == nil || !dv.paramEditor.Active() {
		t.Fatal("tapping the center value should open the numeric editor")
	}
}
```

- [ ] **Step 2: Run to verify failure/pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestMobileWheelEndToEnd|TestMobileWheelCenterTap' ./internal/ui/`
Expected: PASS if Tasks 3/7 are correct; if FAIL, fix the wiring (e.g. `OpenEditor` not set, or `centerH` empty for the bound knob).

- [ ] **Step 3: Add the screenshot scene** in `scene_catalog.go` — a `crop_mobile_synth_wheel` entry whose `MobileSetup` selects a synth instrument, lays out the synth tab, and calls `dv.openSynthKnobWheelPopup(idx, instID)` on a continuous knob; set `Subject` to the wheel popup rect (add a `Subject` const + `SubjectRect` case + a public bounds accessor returning `dv.synthWheelPopup.Rect()`, per CLAUDE.md "Adding a subject"). Follow `mobileAudioPanelPlaySetup` as the model for a mobile `MobileSetup`.

- [ ] **Step 4: Run the scene + crop guard**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSceneCrop|Subject' ./internal/ui/`
Expected: PASS — `crop_mobile_synth_wheel` produces a visible Subject rect strictly inside the framebuffer.

- [ ] **Step 5: Final verification checkpoint** (do NOT commit)

Run the whole feature's tests together:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestNudgeEndless|TestWheel|TestMobileWheel|TestOpenSynthKnobWheelPopup|TestMobileTapKnobOpensWheel|TestDesktopTapKnobStartsDrag|TestMobileWheelDensityTokensPresent|TestSceneCrop' \
  ./internal/ui/
```
Expected: all PASS.
Then desktop-regression sanity (the rotary path must be untouched):
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Knob' ./internal/ui/
```
Expected: pre-existing knob tests still PASS (no regressions from the added `NudgeEndless` method or the gated seam).

Leave ALL changes uncommitted in the working tree (per the repo rule). Report the final test output verbatim.

---

## Self-Review (performed against the spec)

- **Spec coverage:** endless/relative value via wheel → Tasks 1,3,9. Barrel/mouse-wheel visual → Task 5. Draggable resolution rung, persisted → Tasks 3(applyResDelta),4,7(persistKnobStep). Enum knobs scroll labels, no strip → Tasks 3,4. Tap-to-open popup like volume → Tasks 6,7,8. Numeric-entry escape hatch → Tasks 7,9. Platform component-swap seam, desktop unchanged → Task 8. Density tokens → Task 2. Tests (functional + platform-swap + alloc + scene) → Tasks 8,9. ✅ All spec sections map to a task.
- **Placeholder scan:** the only intentional placeholder is the wrong `"color"` import in Task 7, explicitly flagged with a CORRECTION note so the implementer fixes it. Render pixel-calls in Task 5 and `drawValuePillChrome` in Task 8 point at concrete existing helpers to mirror rather than guessing signatures — per the project's "real API ≠ plan guesses" rule, the implementing subagent reads those helpers in-repo.
- **Type consistency:** `WheelBinding`, `MobileWheelPopup`, `NewMobileWheelPopup`, `openSynthKnobWheelPopup`, `synthWheelPopup`, `mobileWheelPopupPortalOverlay` are used identically across Tasks 3–9. `NudgeEndless(deltaPx int)` signature consistent (Tasks 1,3,9). Density field names consistent (Tasks 2,3,5).
- **Risks:** test-helper construction of a populated synth DrumView (Task 7/8 `synthWheelTestDV`) is the main unknown — the plan offers a direct-seed fallback. `AnchorPopupRect`/overlay-interface signatures must be matched to the real ones (flagged inline).
