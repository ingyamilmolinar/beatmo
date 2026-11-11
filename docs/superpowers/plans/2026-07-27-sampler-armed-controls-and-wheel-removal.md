# Sampler Armed Controls + Control-Wheel Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Sampler tab's five param controls behave and render exactly like the Synth tab's click-to-arm inline barrel controls, then delete the legacy `MobileWheelPopup` and all numeric-text-entry code it kept alive.

**Architecture:** Generalize the four synth-specific seams in the existing armed-control machinery (editor construction, arm-state scoping, card rendering, grid layout), then point the Sampler at them. The Synth tab's *behavior* must not change at all. Finally remove the wheel popup, the EQ tab's value-entry affordances, and the now-dead `DrumView.paramEditor` apparatus.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebitengine 2.9.9, `internal/ui` package, ebitenstub test harness (`-tags test`), xvfb screenshot harness.

## Global Constraints

- **NEVER stage or commit.** This repository's standing rule. Every task ends with a **verification step**, not a commit. Leave all work uncommitted in the working tree.
- **Go binary:** always `.tools/go/bin/go` (never system `go`). From `src/go`, that is `../../.tools/go/bin/go`.
- **Fast test command:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1500s ./internal/ui/`
  **`-timeout 1500s` is mandatory for the whole-package run.** `internal/ui` takes ~11 minutes and blows Go's 600s default, which surfaces as a goroutine-dump panic that looks like a hang but is not one. Every whole-package command below implies this flag even where it is written without it.
- **Single-test command:** append `-run 'TestName'` to the above. Filtered runs are fast and do not need the timeout flag.
- **No behavior change to the Synth tab.** Every existing synth armed-control test must stay green **without edits** through Tasks 1–4. If one needs editing, stop and re-read the task.
- **No new density tokens.** The sampler reuses `SynthArmedCardW`, `SynthArmedCardH`, `SynthWheelLaneW`, `SynthWheelRowH`, `SynthWheelVisibleRows`.
- **Bindings resolve widgets by index every call.** Never cache a `*Knob` or `*KnobStepBadge` pointer in a binding — a panel re-layout would leave it dangling.
- **No legacy retained for tests.** If a production symbol's only callers are tests, delete the symbol and the tests. Never add a "retained helper" test.
- **UI work requires a screenshot you have opened and looked at.** Green Go tests are not sufficient (Task 11 and Task 17).
- **Never dismiss a red test as "pre-existing".** Prove causation or fix it.
- Log-tag rule if you add any log line: lowercase bracket tag, sourced from `hooks.ActionRegistry`.

## Reference: the spec

`docs/superpowers/specs/2026-07-27-sampler-armed-controls-and-wheel-removal-design.md`

## File Structure

**Phase A — generalize (Tasks 1–4), behavior-preserving:**

| File | Responsibility |
|---|---|
| Create `src/go/internal/ui/knob_format.go` | Wheel-independent knob value formatters, relocated out of `mobile_wheel_render.go` so that file can be deleted later. |
| Modify `src/go/internal/ui/synth_armed_editor.go` | `ArmedControlEditor` takes its two bindings + an `onEdit` closure instead of hard-wiring synth ones. |
| Create `src/go/internal/ui/armed_control_editor_factories.go` | `newSynthArmedControlEditor` / `newSamplerArmedControlEditor`. |
| Modify `src/go/internal/ui/synth_armed_grow_anim.go` | Grow subject gains an `armedScope`. |
| Create `src/go/internal/ui/armed_scope.go` | The `armedScope` enum. |
| Modify `src/go/internal/ui/synth_armed_control.go` | Synth arm state passes its scope through; `armedControlActive()` replaces `synthArmedActive()`. |
| Create `src/go/internal/ui/armed_control_card.go` | Package-level `drawArmedControlCard`. |

**Phase B — sampler adopts (Tasks 5–11):**

| File | Responsibility |
|---|---|
| Create `src/go/internal/ui/sampler_barrel_bindings.go` | `samplerValueBinding` / `samplerResolutionBinding`. |
| Create `src/go/internal/ui/sampler_armed_control.go` | Sampler arm/lock/toggle state + `samplerArmedActive`. |
| Modify `src/go/internal/ui/sampler_state.go` | Extract the pure `samplerStateValueAt` mapping; add `samplerKnobEditKey`. |
| Modify `src/go/internal/ui/sampler_panel_zone.go` | One grid layout path (desktop + mobile); armed hit-adapter branches; armed card draw; update loop. |
| Create `src/go/internal/ui/sampler_armed_test.go` etc. | New behavior tests. |

**Phase C — removals (Tasks 12–17):** deletions across `sampler_panel_zone.go`, `eq_panel_zone.go`, `synth_panel_zone.go`, `game_input_shortcuts.go`, `native_gesture.go`, `drumview*.go`, `DESIGN.md`, plus ~15 test-file deletions and 2 test-file re-pointings.

---

## Task 1: Relocate wheel-independent knob formatters

`mobile_wheel_render.go` will be deleted in Task 15, but it hosts five package-level helpers the **synth** depends on today (`knobPillLabel` at `synth_panel_zone.go:3165` and `synth_macro_render.go:271`; `knobStepResolution` at `synth_barrel_bindings.go:39`). Move them first so the later deletion is a clean file removal.

**Files:**
- Create: `src/go/internal/ui/knob_format.go`
- Modify: `src/go/internal/ui/mobile_wheel_render.go` (delete lines defining the five funcs; keep the file otherwise)
- Test: existing `src/go/internal/ui/mobile_wheel_precision_test.go` already asserts `knobPillLabel` behavior — reuse it as the regression check.

**Interfaces:**
- Consumes: nothing.
- Produces (unchanged signatures, new home):
  - `func enumIndex(norm float64, n int) int`
  - `func formatKnobDisplay(def audio.ParamDef, norm float64) string`
  - `func formatKnobDisplayStep(def audio.ParamDef, norm, step float64) string`
  - `func knobStepResolution(k *Knob, def audio.ParamDef) float64`
  - `func knobPillLabel(k *Knob, def audio.ParamDef) string`

- [ ] **Step 1: Read the five functions you are about to move**

Run: `cd src/go/internal/ui && sed -n '450,515p' mobile_wheel_render.go`

Read the whole block including doc comments. You will move them **verbatim** — no logic changes in this task.

- [ ] **Step 2: Create `knob_format.go` with the five functions moved verbatim**

Create `src/go/internal/ui/knob_format.go`:

```go
package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// knob_format.go — formatting a *Knob's normalized value for display.
//
// These helpers are independent of any particular control VIEW (they were
// previously housed in the retired mobile wheel popup's renderer): the synth
// ParamCard, the armed barrel lanes, and the sampler card all format through
// them, so a value shown in one place can never disagree with the same value
// shown in another.
```

Then append the five functions **exactly as they appear** in `mobile_wheel_render.go`, doc comments included. Do not reformat or "improve" them.

- [ ] **Step 3: Delete the five functions from `mobile_wheel_render.go`**

Remove the `enumIndex`, `formatKnobDisplay`, `formatKnobDisplayStep`, `knobStepResolution`, and `knobPillLabel` definitions (lines ~458–512). Leave every `MobileWheelPopup` method in place.

- [ ] **Step 4: Fix imports in `mobile_wheel_render.go`**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`

Expected: either a clean build, or "imported and not used" for `math` / `audio`. Remove whichever import became unused. Re-run until clean.

- [ ] **Step 5: Verify no behavior changed**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestKnobPill|TestMobileWheel|TestSynthComponentSprite|TestMobileKnobCaption'
```
Expected: PASS. This is a pure relocation; a failure means you changed a function body.

- [ ] **Step 6: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS (same set as before your change — capture the baseline first with `git stash` if you are unsure).

---

## Task 2: `ArmedControlEditor` takes its bindings and commit closure

Today `newArmedControlEditor(dv, instID, idx)` constructs `synthValueBinding` / `synthResolutionBinding` internally and `onEdit` is a synth-specific method. Widen both so a second panel can supply its own.

**Files:**
- Modify: `src/go/internal/ui/synth_armed_editor.go`
- Create: `src/go/internal/ui/armed_control_editor_factories.go`
- Modify: `src/go/internal/ui/synth_armed_control.go:27` and `src/go/internal/ui/synth_panel_zone.go:1100` (the two `newArmedControlEditor` call sites)
- Test: `src/go/internal/ui/armed_control_editor_generic_test.go` (new)

**Interfaces:**
- Consumes: `barrelBinding` (`barrel_wheel.go`), `Profile().DensityValues().SynthWheelRowH`.
- Produces:
  - `func newArmedControlEditor(instID string, idx int, valB, resB barrelBinding, onEdit func()) *ArmedControlEditor`
  - `func newSynthArmedControlEditor(dv *DrumView, instID string, idx int) *ArmedControlEditor`
  - `func (dv *DrumView) onSynthArmedEdit(instID string, idx int)`
  - `ArmedControlEditor` loses its `dv` field; keeps `instID`, `idx`, and gains `onEditFn func()`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/armed_control_editor_generic_test.go`:

```go
//go:build test

package ui

import "testing"

// fakeBarrelBinding is a minimal barrelBinding over an int counter, so the
// editor can be exercised with no DrumView at all — proving it is generic.
type fakeBarrelBinding struct {
	val  int
	min  int
	max  int
	name string
}

func (b *fakeBarrelBinding) labelAt(offset int) string {
	v := b.val + offset
	if v < b.min || v > b.max {
		return ""
	}
	return b.name
}

func (b *fakeBarrelBinding) stepBy(detents int) bool {
	next := b.val + detents
	if next < b.min {
		next = b.min
	}
	if next > b.max {
		next = b.max
	}
	if next == b.val {
		return false
	}
	b.val = next
	return true
}

func (b *fakeBarrelBinding) canStep(dir int) bool {
	if dir > 0 {
		return b.val < b.max
	}
	return b.val > b.min
}

// TestArmedControlEditorIsPanelAgnostic proves ArmedControlEditor can be built
// from arbitrary bindings plus an onEdit closure, with no DrumView — the seam
// the Sampler needs. Keyboard steps must move the bound value and fire onEdit.
func TestArmedControlEditorIsPanelAgnostic(t *testing.T) {
	val := &fakeBarrelBinding{val: 5, min: 0, max: 10, name: "v"}
	res := &fakeBarrelBinding{val: 2, min: 0, max: 4, name: "r"}
	edits := 0

	e := newArmedControlEditor("inst-x", 3, val, res, func() { edits++ })

	if e.instID != "inst-x" || e.idx != 3 {
		t.Fatalf("identity not carried: instID=%q idx=%d", e.instID, e.idx)
	}
	if e.res() == nil {
		t.Fatal("resolution lane should exist when a non-nil res binding is supplied")
	}
	if !e.StepValue(+1) {
		t.Fatal("StepValue(+1) should move the value binding")
	}
	if val.val != 6 {
		t.Fatalf("value binding = %d, want 6", val.val)
	}
	if !e.StepResolution(+1) {
		t.Fatal("StepResolution(+1) should move the resolution binding")
	}
	if res.val != 3 {
		t.Fatalf("resolution binding = %d, want 3", res.val)
	}
	if edits != 2 {
		t.Fatalf("onEdit fired %d times, want 2 (one per successful step)", edits)
	}
}

// TestArmedControlEditorNilResBindingHasNoResLane pins that a param with no
// resolution ladder gets a single (value-only) lane — the synth's
// discrete-param case, and the shape the sampler inherits.
func TestArmedControlEditorNilResBindingHasNoResLane(t *testing.T) {
	val := &fakeBarrelBinding{val: 1, min: 0, max: 3, name: "v"}
	e := newArmedControlEditor("", 0, val, nil, func() {})
	if e.res() != nil {
		t.Fatal("nil res binding must leave the resolution wheel nil")
	}
	if e.StepResolution(+1) {
		t.Fatal("StepResolution must be a no-op with no resolution lane")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestArmedControlEditor(IsPanelAgnostic|NilResBinding)'
```
Expected: FAIL to compile — `not enough arguments in call to newArmedControlEditor` (the current signature is `(dv, instID, idx)`).

- [ ] **Step 3: Widen the editor**

In `src/go/internal/ui/synth_armed_editor.go`, replace the struct's `dv` field with an `onEditFn`, and replace the constructor:

```go
type ArmedControlEditor struct {
	instID string
	idx    int

	onEditFn func()

	valWheel *BarrelWheel
	resWheel *BarrelWheel // nil when the control has no resolution ladder

	card    image.Rectangle
	valRect image.Rectangle
	resRect image.Rectangle
	active  *BarrelWheel // wheel currently captured by a drag
}

// newArmedControlEditor pairs a value binding (always) with an optional
// resolution binding, and coalesces every detent crossing into the caller's
// onEdit closure. It knows nothing about which panel owns it: the Synth and
// Sampler tabs both build one through the factories in
// armed_control_editor_factories.go, which is what makes the two tabs' editing
// behavior identical by construction rather than by parallel maintenance.
// A nil resB means "this control has no resolution ladder" (discrete params).
func newArmedControlEditor(instID string, idx int, valB, resB barrelBinding, onEdit func()) *ArmedControlEditor {
	e := &ArmedControlEditor{instID: instID, idx: idx, onEditFn: onEdit}
	detent := Profile().DensityValues().SynthWheelRowH
	e.valWheel = NewBarrelWheel()
	e.valWheel.Bind(valB, detent)
	if resB != nil {
		e.resWheel = NewBarrelWheel()
		e.resWheel.Bind(resB, detent)
	}
	return e
}
```

Replace the `onEdit` method at the bottom of the file with:

```go
// onEdit fires the owning panel's live-propagate + debounced-commit closure.
func (e *ArmedControlEditor) onEdit() {
	if e.onEditFn != nil {
		e.onEditFn()
	}
}
```

Leave `SetCardRect`, `LaneGapRect`, `laneAt`, `BeginDrag`, `DragTo`, `EndDrag`, `Wheel`, `StepValue`, `StepResolution`, and `Tick` untouched.

- [ ] **Step 4: Add the synth factory**

Create `src/go/internal/ui/armed_control_editor_factories.go`:

```go
package ui

// armed_control_editor_factories.go — one factory per panel that hosts armed
// inline barrel controls. Each supplies the panel's two bindings plus the
// live-propagate + debounced-commit closure; ArmedControlEditor itself stays
// panel-agnostic (see synth_armed_editor.go).

// newSynthArmedControlEditor binds the armed editor to synth control idx of
// instID. The resolution lane is present only for params that carry a step
// badge (continuous params) — a discrete param gets a value lane only.
func newSynthArmedControlEditor(dv *DrumView, instID string, idx int) *ArmedControlEditor {
	var resB barrelBinding
	if rb := (synthResolutionBinding{dv: dv, idx: idx, instID: instID}); rb.available() {
		resB = rb
	}
	return newArmedControlEditor(instID, idx,
		synthValueBinding{dv: dv, idx: idx, instID: instID}, resB,
		func() { dv.onSynthArmedEdit(instID, idx) })
}

// onSynthArmedEdit is the Synth tab's per-detent commit path: live slider sync +
// propagate now, ONE debounced undo step for the whole spin.
func (dv *DrumView) onSynthArmedEdit(instID string, idx int) {
	dv.syncSliderFromKnob(idx)
	dv.propagateSynthSliderValue(idx, instID)
	dv.synthWheelNudge.bump(func() {
		dv.commitInstrumentParams(instID)
		dv.requestSynthMirror(instID)
	})
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.Invalidate()
	}
}
```

- [ ] **Step 5: Update the two existing call sites**

In `src/go/internal/ui/synth_armed_control.go`, line ~27:
```go
	dv.instEditorArmedEditor = newSynthArmedControlEditor(dv, instID, idx)
```

In `src/go/internal/ui/synth_panel_zone.go`, line ~1100:
```go
		dv.instEditorArmedEditor = newSynthArmedControlEditor(dv, resolved, aidx)
```

- [ ] **Step 6: Run the new tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestArmedControlEditor(IsPanelAgnostic|NilResBinding)'
```
Expected: PASS.

- [ ] **Step 7: Verify the Synth tab is untouched**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Armed|Barrel|SynthControl'
```
Expected: PASS with **no edits to any existing test file**. If a test needed editing, you changed behavior — revert and re-read Step 3.

- [ ] **Step 8: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

---

## Task 3: Scope the arm state and grow subject

The Sampler and Synth tabs edit the **same instrument id** with **different index spaces**. The grow animation is a single subject on `DrumView` (`armGrowIdx` / `armGrowInst` / `armGrowT` / `armGrowTarget`) keyed by instrument id only — unscoped, a synth arm would drive the sampler's reflow. Add a scope.

**Files:**
- Create: `src/go/internal/ui/armed_scope.go`
- Modify: `src/go/internal/ui/synth_armed_grow_anim.go`
- Modify: `src/go/internal/ui/synth_armed_control.go`
- Modify: `src/go/internal/ui/drumview.go` (add `armGrowScope` field)
- Modify: `src/go/internal/ui/keyboard_focus.go:72`, `src/go/internal/ui/game_input_shortcuts.go:696`
- Modify: `src/go/internal/ui/synth_panel_zone.go:2212`, `src/go/internal/ui/synth_macro_render.go:160`
- Test: `src/go/internal/ui/armed_scope_test.go` (new)

**Interfaces:**
- Consumes: `newSynthArmedControlEditor` (Task 2).
- Produces:
  - `type armedScope int`, consts `armedScopeSynth`, `armedScopeSampler`
  - `func (dv *DrumView) setArmGrowSubject(scope armedScope, instID string, idx int, target float64)`
  - `func (dv *DrumView) armGrowControl(scope armedScope, instID string) (int, bool)`
  - `func (dv *DrumView) armedControlActive() bool`
  - `synthArmedActive()` is **removed**; `armedControlActive()` replaces it.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/armed_scope_test.go`:

```go
//go:build test

package ui

import "testing"

// TestArmGrowSubjectIsScoped pins that the grow animation's single subject is
// keyed by SCOPE as well as instrument id. Without the scope, a synth arm on
// instrument X would drive the sampler's reflow for the same X (and vice
// versa) because both tabs edit the same instrument with different index
// spaces.
func TestArmGrowSubjectIsScoped(t *testing.T) {
	g := newSamplerTabGame(t)
	dv := g.drum

	dv.setArmGrowSubject(armedScopeSynth, "kick", 4, 1)

	if idx, ok := dv.armGrowControl(armedScopeSynth, "kick"); !ok || idx != 4 {
		t.Fatalf("synth scope should see its own subject: idx=%d ok=%v", idx, ok)
	}
	if _, ok := dv.armGrowControl(armedScopeSampler, "kick"); ok {
		t.Fatal("sampler scope must NOT see the synth's grow subject for the same instrument")
	}

	dv.setArmGrowSubject(armedScopeSampler, "kick", 1, 1)
	if idx, ok := dv.armGrowControl(armedScopeSampler, "kick"); !ok || idx != 1 {
		t.Fatalf("sampler scope should see its own subject: idx=%d ok=%v", idx, ok)
	}
	if _, ok := dv.armGrowControl(armedScopeSynth, "kick"); ok {
		t.Fatal("synth scope must NOT see the sampler's grow subject after the switch")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestArmGrowSubjectIsScoped'
```
Expected: FAIL to compile — `undefined: armedScopeSynth`, and `setArmGrowSubject` takes 3 args.

- [ ] **Step 3: Add the scope enum**

Create `src/go/internal/ui/armed_scope.go`:

```go
package ui

// armed_scope.go — which panel owns an armed inline control.
//
// The Synth and Sampler tabs both host click-to-arm barrel controls, and both
// address their controls by an INDEX into their own widget slice. A given
// instrument id therefore means "synth control 7" in one tab and "sampler knob
// 2" in the other. The arm bookkeeping and the single grow-animation subject
// are keyed by scope as well as instrument id so the two tabs can never read
// each other's state.
type armedScope int

const (
	armedScopeSynth armedScope = iota
	armedScopeSampler
)
```

- [ ] **Step 4: Thread the scope through the grow animation**

In `src/go/internal/ui/drumview.go`, next to the other `armGrow*` fields (~line 535), add:
```go
	armGrowScope            armedScope
```

In `src/go/internal/ui/synth_armed_grow_anim.go`, change the two subject functions:

```go
func (dv *DrumView) setArmGrowSubject(scope armedScope, instID string, idx int, target float64) {
	if dv.armGrowIdx != idx || dv.armGrowInst != instID || dv.armGrowScope != scope {
		dv.armGrowT = 0
	}
	dv.armGrowIdx = idx
	dv.armGrowInst = instID
	dv.armGrowScope = scope
	dv.armGrowTarget = target
}

func (dv *DrumView) armGrowControl(scope armedScope, instID string) (int, bool) {
	return dv.armGrowIdx, dv.armGrowIdx >= 0 && dv.armGrowInst != "" &&
		dv.armGrowInst == instID && dv.armGrowScope == scope
}
```

Leave `advanceArmGrow`, `armGrowProgress`, `smoothstep`, `lerpInt`, `lerpRect` unchanged.

- [ ] **Step 5: Update the synth call sites to pass their scope**

- `synth_armed_control.go:29` → `dv.setArmGrowSubject(armedScopeSynth, instID, idx, 1)`
- `synth_armed_control.go:38` → `dv.setArmGrowSubject(armedScopeSynth, instID, idx, 0)`
- `synth_panel_zone.go:2212` → `if idx, active := p.dv.armGrowControl(armedScopeSynth, instID); active {`
- `synth_macro_render.go:160` → `if idx, active := dv.armGrowControl(armedScopeSynth, resolved); active {`

- [ ] **Step 6: Rename `synthArmedActive` to `armedControlActive`**

In `src/go/internal/ui/synth_armed_control.go`, replace `synthArmedActive` with a version that will grow a sampler arm in Task 7. For now it covers only the synth so behavior is identical:

```go
// armedControlActive reports whether the ACTIVE audio-panel tab has a control
// armed for inline barrel editing. Nil-safe. Used by KeyboardClaimed (so armed
// arrow keys don't also pan the camera) and by handleEscape's armed rung (so Esc
// locks the control instead of falling through to Stop playback). The Sampler
// branch is added in the sampler-armed task; keeping ONE predicate is what
// guarantees both tabs get the same keyboard semantics.
func (dv *DrumView) armedControlActive() bool {
	if dv == nil || dv.eqPanelZone == nil {
		return false
	}
	switch dv.eqPanelZone.ActiveTab() {
	case TabSynth:
		if len(dv.instEditorArmedControl) == 0 || dv.instEditorArmedEditor == nil {
			return false
		}
		_, armed := dv.synthControlArmed(dv.synthTabActiveInstrument())
		return armed
	}
	return false
}
```

Update the two consumers:
- `keyboard_focus.go:72` → `if dv.armedControlActive() {`
- `game_input_shortcuts.go:696` → `if g.drum != nil && g.drum.armedControlActive() {`

- [ ] **Step 7: Run the new test**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestArmGrowSubjectIsScoped'
```
Expected: PASS.

- [ ] **Step 8: Fix any test that referenced `synthArmedActive`**

Run: `cd src/go/internal/ui && grep -rn "synthArmedActive" .`
Expected: no hits. If a test file references it, rename the reference to `armedControlActive` — that is a pure rename, not a behavior change.

- [ ] **Step 9: Verify the Synth tab is untouched**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

---

## Task 4: Make `drawArmedControlCard` panel-agnostic

**Files:**
- Create: `src/go/internal/ui/armed_control_card.go`
- Modify: `src/go/internal/ui/synth_panel_zone.go` (delete the `*synthPanel` method at ~3200; update the call at ~3157)
- Modify: `src/go/internal/ui/synth_macro_render.go:267` (the other call site)
- Test: existing `src/go/internal/ui/synth_armed_pixels_test.go` and `src/go/internal/ui/synth_barrel_render_test.go` are the regression check.

**Interfaces:**
- Consumes: `ArmedControlEditor`, `barrelBinding`, `drawBarrelLane`, `barrelLaneStyle`.
- Produces: `func drawArmedControlCard(dst *ebiten.Image, r image.Rectangle, name string, e *ArmedControlEditor, valB, resB barrelBinding)`

- [ ] **Step 1: Read the current method**

Run: `cd src/go/internal/ui && sed -n '3192,3240p' synth_panel_zone.go`

- [ ] **Step 2: Create the package-level renderer**

Create `src/go/internal/ui/armed_control_card.go`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// armed_control_card.go — the ARMED control's grown card, shared by the Synth
// and Sampler tabs so an armed card is pixel-identical on both at equal size.
//
// Layout mirrors ArmedControlEditor.SetCardRect exactly: a header band one
// wheel row tall carrying the param name, then the body split into an optional
// left resolution lane and a wider right value lane with a gutter between them.
// `r` is the GROWN rect the caller's grow/reflow pass already assigned.
//
// This bypasses the per-card ParamCard sprite cache on purpose: the lanes
// animate every frame, and a cached sprite would freeze mid-scroll.
func drawArmedControlCard(dst *ebiten.Image, r image.Rectangle, name string,
	e *ArmedControlEditor, valB, resB barrelBinding) {
	if e == nil || r.Empty() {
		return
	}
	rad := RadiusControl
	// Flat-tonal armed card: one filled body plus the accent bar marking it as
	// the armed control. No shell/bevel/rim.
	capR := r
	drawFlatButtonBody(dst, capR, TokenSurface3(), rad)
	drawFlatSelectionBar(dst, capR, colConceptStroke, selectionEdgeBottom, 1)

	e.SetCardRect(capR)

	dv2 := Profile().DensityValues()
	label := truncCaptionStyled(name, capR.Dx()-2*SpaceXS, RoleMicro)
	nameY := capR.Min.Y + (dv2.SynthWheelRowH-StyledTextHeight(RoleMicro))/2
	DrawTextStyled(dst, label, capR.Min.X+SpaceXS, nameY, RoleMicro, TokenTextSecondary())

	style := barrelLaneStyle{
		RowH:        dv2.SynthWheelRowH,
		VisibleRows: dv2.SynthWheelVisibleRows,
		Accent:      colConceptStroke,
		Emphasis:    true,
	}
	if vr := e.ValueLaneRect(); !vr.Empty() {
		drawBarrelLane(dst, vr, e.value(), valB, style)
	}
	if rr := e.ResLaneRect(); !rr.Empty() && resB != nil {
		drawBarrelLane(dst, rr, e.res(), resB, style)
	}
	// A subtle 1px hairline in the gutter between the two lanes so their label
	// stacks read as two distinct columns rather than merging.
	if gap := e.LaneGapRect(); !gap.Empty() {
		divX := gap.Min.X + gap.Dx()/2
		div := image.Rect(divX, gap.Min.Y+SpaceXS, divX+1, gap.Max.Y-SpaceXS)
		drawRect(dst, div, WithAlphaFromColor(TokenBorderMedium(), AlphaSubtle), true)
	}
}
```

- [ ] **Step 3: Delete the `*synthPanel` method and update both synth call sites**

Delete `func (p *synthPanel) drawArmedControlCard(...)` from `synth_panel_zone.go`.

At `synth_panel_zone.go:3157`, replace:
```go
			p.drawArmedControlCard(dst, r, b, instID, kIdx)
```
with:
```go
			var resB barrelBinding
			if rb := (synthResolutionBinding{dv: dv, idx: kIdx, instID: instID}); rb.available() {
				resB = rb
			}
			drawArmedControlCard(dst, r, synthParamDisplayName(b.def.Name),
				dv.instEditorArmedEditor,
				synthValueBinding{dv: dv, idx: kIdx, instID: instID}, resB)
```

At `synth_macro_render.go:267`, apply the identical replacement (the local variable names there are `dv`, `resolved`, `kIdx`, `b` — use `resolved` in place of `instID`).

- [ ] **Step 4: Build**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`
Expected: clean.

- [ ] **Step 5: Verify the rendered pixels did not change**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthArmedPixels|TestBarrelLane|TestSynthBarrelRender|Armed'
```
Expected: PASS with no test edits.

- [ ] **Step 6: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

**Phase A is now complete. The Synth tab must be bit-identical in behavior. Before moving on, confirm you edited ZERO existing test files in Tasks 1–4.**

**Do NOT use `git diff` for this gate.** This branch already carries the entire uncommitted armed-controls feature, so `git status` shows ~100 dirty test files that have nothing to do with this work. Instead, list the test files *you* touched this session by mtime:

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go/internal/ui && \
  find . -name '*_test.go' -newermt '-6 hours' -printf '%T@ %p\n' | sort -rn | head -20
```
Expected: only the new files you created (`armed_control_editor_generic_test.go`, `armed_scope_test.go`) and any file whose ONLY change is the `synthArmedActive` → `armedControlActive` rename. Anything else means you changed Synth behavior — revert and re-read the task.

---

## Task 5: One grid layout path for the Sampler (desktop + mobile)

`growArmedCard` needs a `ControlGrid` for its cell lattice and returns early when the grid is nil. Desktop currently hand-rolls the layout; mobile already uses `layoutSamplerKnobsScrolling`. Collapse to one path — the cell formula is already identical, so this is a deletion plus a column cap.

**Files:**
- Modify: `src/go/internal/ui/sampler_panel_zone.go:542-589` (delete the desktop branch) and `:607-646` (`layoutSamplerKnobsScrolling`)
- Test: `src/go/internal/ui/sampler_grid_layout_test.go` (new)

**Interfaces:**
- Consumes: `ControlGrid.SetMaxCols`, `ControlGrid.Layout`, `ControlGrid.CellRect`.
- Produces: `func (p *samplerPanel) layoutSamplerKnobsScrolling(gridR image.Rectangle, dv2 densityValues)` now serves both platforms; `s.knobGrid` is non-nil on desktop too.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/sampler_grid_layout_test.go`:

```go
//go:build test

package ui

import "testing"

// TestSamplerDesktopUsesControlGrid pins that the DESKTOP sampler lays its
// cards out through the shared ControlGrid, not a hand-rolled row. The armed
// grow/reflow reads the grid's cell lattice (ControlGrid.CellAt /
// CellAtUnclipped / SetContentRows / ClipBottom) and no-ops without it, so a
// nil grid on desktop would silently disable arming there.
func TestSamplerDesktopUsesControlGrid(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick") // knobs only lay out once a buffer exists
	layoutSamplerTab(t, g)
	s := &g.drum.sampler

	if s.knobGrid == nil {
		t.Fatal("desktop sampler must own a ControlGrid (armed grow/reflow needs its lattice)")
	}
	if s.knobGrid.Cols() < 1 {
		t.Fatalf("grid has no columns: %d", s.knobGrid.Cols())
	}
	for i, k := range s.knobs {
		if k == nil || k.Rect().Empty() {
			t.Errorf("knob %d has an empty rect after the grid layout", i)
		}
	}
}

// TestSamplerDesktopKeepsFiveInOneRow pins the visual status quo: on a
// desktop-width panel all five cards still sit side by side in a single row.
// The column cap (samplerKnobCount) is what preserves that after the move onto
// ControlGrid, whose adaptive count would otherwise wrap.
func TestSamplerDesktopKeepsFiveInOneRow(t *testing.T) {
	g := newSamplerTabGame(t)
	g.Layout(1280, 720)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	s := &g.drum.sampler

	if got := s.knobGrid.Cols(); got != samplerKnobCount {
		t.Fatalf("desktop grid cols = %d, want %d (all five cards in one row)", got, samplerKnobCount)
	}
	// Every card's vertical band must overlap the first card's: one row.
	first := s.knobs[0].Rect()
	for i := 1; i < len(s.knobs); i++ {
		r := s.knobs[i].Rect()
		if r.Empty() {
			t.Fatalf("knob %d empty", i)
		}
		if r.Min.Y != first.Min.Y {
			t.Errorf("knob %d top = %d, want %d (same row as knob 0)", i, r.Min.Y, first.Min.Y)
		}
		if r.Min.X <= s.knobs[i-1].Rect().Min.X {
			t.Errorf("knob %d is not to the right of knob %d", i, i-1)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerDesktop(UsesControlGrid|KeepsFiveInOneRow)'
```
Expected: FAIL — `desktop sampler must own a ControlGrid` (the desktop branch returns before `ensureKnobGrid`).

- [ ] **Step 3: Route desktop through the grid**

In `src/go/internal/ui/sampler_panel_zone.go`, replace the whole desktop tail of the layout function (from the `if mobile { ... return }` block's closing brace through the `dv.layoutSamplerControlButtons(...)` call at ~588) with:

```go
	// Both platforms lay the cards out through the SAME ControlGrid. The armed
	// card's grow + neighbour reflow (growArmedCard) reads the grid's cell
	// lattice, so a hand-rolled desktop row would silently disable arming
	// there. The only divergence is the column cap: mobile wraps at 4, desktop
	// keeps all five cards side by side in one row.
	gridBottom := toggleY0 - SpaceXS
	dv.layoutSamplerKnobsScrolling(image.Rect(ctrlR.Min.X, ctrlR.Min.Y, ctrlR.Max.X, gridBottom), dv2)
	if mobile {
		dv.layoutSamplerMobileToggleRow(image.Rect(ctrlR.Min.X, toggleY0, ctrlR.Max.X, toggleY1))
		return
	}
	dv.layoutSamplerControlButtons(image.Rect(ctrlR.Min.X, toggleY0, ctrlR.Max.X, toggleY1))
```

Delete the now-unreachable `labelH` / `cellW` / `cardH` / `cardW` / `cardTop` / `cellBottom` desktop block entirely. Keep `labelH` only if `metaH`/`toggleY0` still need it — check the compiler.

- [ ] **Step 4: Add the platform-dependent column cap and the desktop card-width clamp**

In `layoutSamplerKnobsScrolling`, replace `grid.SetMaxCols(4)` with:

```go
	// Mobile wraps at 4 columns; desktop keeps all five cards in one row.
	if Profile().IsMobile() {
		grid.SetMaxCols(4)
	} else {
		grid.SetMaxCols(samplerKnobCount)
	}
```

Then inside the per-knob loop, after `cardH` is clamped, replace the unconditional full-slot-width `SetRect` with a width clamp that preserves the desktop look:

```go
		// Mobile cards fill the slot. Desktop keeps the previous proportions:
		// wider than a bare ParamCard in the roomy control column, but never
		// filling the whole cell, and never overhanging into a neighbour.
		cardW := slot.Dx()
		if !Profile().IsMobile() {
			cardW = slot.Dx() - SpaceSM
			if maxW := dv2.ParamCardW * 3 / 2; cardW > maxW {
				cardW = maxW
			}
			if minW := dv2.ParamCardW / 2; cardW < minW {
				cardW = minW
			}
			if cardW > slot.Dx() {
				cardW = slot.Dx()
			}
		}
		cardX0 := slot.Min.X + (slot.Dx()-cardW)/2
		k.SetRect(image.Rect(cardX0, slot.Min.Y+labelBandH, cardX0+cardW, slot.Min.Y+labelBandH+cardH))
		s.knobCells[i] = slot
```

- [ ] **Step 5: Run the new tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerDesktop(UsesControlGrid|KeepsFiveInOneRow)'
```
Expected: PASS.

- [ ] **Step 6: Run every sampler test**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sampler'
```
Expected: PASS. A layout-geometry test that fails here is a **real** regression from the grid move — fix the layout, do not relax the test.

- [ ] **Step 7: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

---

## Task 6: Sampler barrel bindings

**Files:**
- Create: `src/go/internal/ui/sampler_barrel_bindings.go`
- Modify: `src/go/internal/ui/sampler_state.go` (extract the pure value mapping)
- Modify: `src/go/internal/ui/sampler_panel_zone.go` (`samplerKnobValueText` delegates to the new shared formatter)
- Test: `src/go/internal/ui/sampler_barrel_bindings_test.go` (new)

**Interfaces:**
- Consumes: `barrelBinding`, `knobStepResolution` (Task 1), `samplerKnobScale`, `sampleEditFacet`, `KnobStepBadge`, `formatStepValue`.
- Produces:
  - `func samplerStateValueAt(idx int, norm float64) float64`
  - `func samplerKnobEditKey(idx int) (string, bool)`
  - `func samplerKnobValueTextAt(idx int, norm float64) string`
  - `type samplerValueBinding struct{ dv *DrumView; idx int }` implementing `barrelBinding`
  - `type samplerResolutionBinding struct{ dv *DrumView; idx int }` implementing `barrelBinding`, plus `available() bool`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/sampler_barrel_bindings_test.go`:

```go
//go:build test

package ui

import "testing"

// TestSamplerBarrelLabelsMatchTheCard is the single-source-of-truth guard: the
// barrel's centre label (offset 0) must be the EXACT string the resting
// ParamCard shows. Two independent formatters would let the card read "15 %"
// while the barrel under the user's finger read something else.
func TestSamplerBarrelLabelsMatchTheCard(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	dv := g.drum
	s := &dv.sampler

	for idx := 0; idx < samplerKnobCount; idx++ {
		b := samplerValueBinding{dv: dv, idx: idx}
		want := samplerKnobValueText(idx, s)
		if got := b.labelAt(0); got != want {
			t.Errorf("knob %d: barrel centre label = %q, card value = %q", idx, got, want)
		}
	}
}

// TestSamplerValueBindingStepsTheKnob pins that a detent moves the knob and
// that the label follows, and that the binding clamps at the param ends
// instead of wrapping.
func TestSamplerValueBindingStepsTheKnob(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	dv := g.drum

	idx := samplerKnobGain
	b := samplerValueBinding{dv: dv, idx: idx}
	before := dv.sampler.knobs[idx].Value

	if !b.stepBy(+1) {
		t.Fatal("stepBy(+1) should move the gain knob")
	}
	if dv.sampler.knobs[idx].Value <= before {
		t.Fatalf("value did not increase: %v -> %v", before, dv.sampler.knobs[idx].Value)
	}

	// Clamp at the top: canStep must go false and stepBy must report no change.
	dv.sampler.knobs[idx].Value = 1.0
	if b.canStep(+1) {
		t.Error("canStep(+1) must be false at the maximum")
	}
	dv.sampler.knobs[idx].Value = 0.0
	if b.canStep(-1) {
		t.Error("canStep(-1) must be false at the minimum")
	}
}

// TestSamplerResolutionBindingWalksTheLadder pins that +1 detent is COARSER
// (the same direction contract the synth resolution lane uses: down/positive
// shows a bigger step number) and that the rung persists.
func TestSamplerResolutionBindingWalksTheLadder(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	dv := g.drum

	idx := samplerKnobGain
	rb := samplerResolutionBinding{dv: dv, idx: idx}
	if !rb.available() {
		t.Fatal("every sampler knob is continuous and carries a step badge")
	}
	badge := dv.sampler.knobStepBadges[idx]
	badge.SetIndex(1)
	stepBefore := badge.Step()

	if !rb.stepBy(+1) {
		t.Fatal("stepBy(+1) should coarsen the resolution")
	}
	if badge.Step() <= stepBefore {
		t.Fatalf("+1 detent must be COARSER: step %v -> %v", stepBefore, badge.Step())
	}
	if dv.sampler.knobs[idx].StepMul != badge.Step() {
		t.Errorf("knob StepMul %v out of sync with badge rung %v",
			dv.sampler.knobs[idx].StepMul, badge.Step())
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSampler(BarrelLabels|ValueBinding|ResolutionBinding)'
```
Expected: FAIL to compile — `undefined: samplerValueBinding`.

- [ ] **Step 3: Extract the pure normalized→state mapping**

In `src/go/internal/ui/sampler_state.go`, add above `setKnob`:

```go
// samplerStateValueAt maps a normalized [0,1] knob value to the sample_edit
// param value for idx WITHOUT touching samplerState. It is the pure core that
// setKnob writes through, so the barrel's label preview and the committed value
// can never disagree about what a given knob position means.
func samplerStateValueAt(idx int, v float64) float64 {
	v = clampUnit(v)
	switch idx {
	case samplerKnobStart, samplerKnobEnd:
		return v
	case samplerKnobTranspose:
		return v*2*samplerTransposeRange - samplerTransposeRange
	case samplerKnobDetune:
		return v*2*samplerDetuneRange - samplerDetuneRange
	case samplerKnobGain:
		return samplerGainMinDB + v*(samplerGainMaxDB-samplerGainMinDB)
	}
	return 0
}
```

Then rewrite `setKnob` to use it (one mapping, not two):

```go
// setKnob maps a normalized [0,1] knob value to the corresponding edit param.
func (s *samplerState) setKnob(idx int, v float64) {
	out := samplerStateValueAt(idx, v)
	switch idx {
	case samplerKnobStart:
		s.startFrac = out
	case samplerKnobEnd:
		s.endFrac = out
	case samplerKnobTranspose:
		s.transposeSemis = out
	case samplerKnobDetune:
		s.detuneCents = out
	case samplerKnobGain:
		s.gainDB = out
	}
}
```

- [ ] **Step 4: Add the key-only accessor and the shared value formatter**

In `src/go/internal/ui/sampler_panel_zone.go`, add next to `samplerKnobEditField`:

```go
// samplerKnobEditKey returns just the sample_edit map key for a knob index —
// the state-free half of samplerKnobEditField, used by the barrel bindings to
// format a HYPOTHETICAL value (a row above/below the current one) that is not in
// samplerState yet.
func samplerKnobEditKey(idx int) (string, bool) {
	switch idx {
	case samplerKnobStart:
		return "start_frac", true
	case samplerKnobEnd:
		return "end_frac", true
	case samplerKnobTranspose:
		return "transpose_semis", true
	case samplerKnobDetune:
		return "detune_cents", true
	case samplerKnobGain:
		return "gain_db", true
	}
	return "", false
}

// samplerKnobValueTextAt formats the value a knob would show at normalized
// position norm, through the SAME paramFacet formatter the resting card and the
// undo notification use. The single source of truth for every sampler value
// string on screen.
func samplerKnobValueTextAt(idx int, norm float64) string {
	key, ok := samplerKnobEditKey(idx)
	if !ok {
		return ""
	}
	f, ok := sampleEditFacet(key)
	if !ok {
		return ""
	}
	return f.format(samplerStateValueAt(idx, norm))
}
```

Rewrite `samplerKnobValueText` to delegate, so card and barrel are guaranteed identical:

```go
// samplerKnobValueText returns just the formatted value ("15 %", "+0 st") for
// the knob's CURRENT position. Delegates to samplerKnobValueTextAt so the
// resting card and the armed barrel lane format through one code path.
func samplerKnobValueText(idx int, s *samplerState) string {
	return samplerKnobValueTextAt(idx, s.knobValue(idx))
}
```

- [ ] **Step 5: Create the bindings**

Create `src/go/internal/ui/sampler_barrel_bindings.go`:

```go
package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// sampler_barrel_bindings.go — the value model the Sampler tab's armed barrel
// lanes scroll. Mirrors synth_barrel_bindings.go one-for-one.
//
// Both bindings resolve their *Knob / *KnobStepBadge BY INDEX on every call and
// never cache a pointer: the sampler panel re-lays-out on every buffer change,
// scroll, and instrument switch, and a cached pointer would go stale.

// samplerValueBinding scrolls a sampler knob's VALUE.
type samplerValueBinding struct {
	dv  *DrumView
	idx int
}

func (b samplerValueBinding) knob() *Knob {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.sampler.knobs) {
		return nil
	}
	return b.dv.sampler.knobs[b.idx]
}

func (b samplerValueBinding) def() audio.ParamDef {
	if b.dv == nil {
		return audio.ParamDef{}
	}
	return b.dv.samplerKnobDef(b.idx)
}

// labelAt formats the value `offset` detents from current WITHOUT mutating the
// knob. One detent is knobStepResolution in the param's DISPLAY units, so it is
// converted to the knob's normalized domain before being applied.
func (b samplerValueBinding) labelAt(offset int) string {
	k := b.knob()
	if k == nil {
		return ""
	}
	def := b.def()
	span := def.Max - def.Min
	if span <= 0 {
		return ""
	}
	stepNorm := knobStepResolution(k, def) / span
	norm := k.Value + float64(offset)*stepNorm
	if norm < 0 || norm > 1 {
		return "" // past a hard clamp: the lane fades out instead of repeating
	}
	return samplerKnobValueTextAt(b.idx, norm)
}

func (b samplerValueBinding) stepBy(detents int) bool {
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
	if changed {
		// Push the new knob position into samplerState so the resting card, the
		// waveform trim handles, and the commit all see it immediately.
		b.dv.applySamplerKnob(b.idx)
	}
	return changed
}

func (b samplerValueBinding) canStep(dir int) bool {
	k := b.knob()
	if k == nil {
		return false
	}
	if dir > 0 {
		return k.Value < 1.0
	}
	return k.Value > 0.0
}

// samplerResolutionBinding scrolls a sampler knob's RESOLUTION rung. +1 detent =
// coarser (higher ladder index), matching the synth lane's direction contract.
type samplerResolutionBinding struct {
	dv  *DrumView
	idx int
}

func (b samplerResolutionBinding) badge() *KnobStepBadge {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.sampler.knobStepBadges) {
		return nil
	}
	return b.dv.sampler.knobStepBadges[b.idx]
}

func (b samplerResolutionBinding) available() bool { return b.badge() != nil }

func (b samplerResolutionBinding) labelAt(offset int) string {
	bg := b.badge()
	if bg == nil {
		return ""
	}
	i := bg.Index() + offset
	rungs := bg.Rungs()
	if i < 0 || i >= len(rungs) {
		return ""
	}
	return formatStepValue(b.dv.samplerKnobDef(b.idx), rungs[i])
}

func (b samplerResolutionBinding) stepBy(detents int) bool {
	bg := b.badge()
	if bg == nil || detents == 0 {
		return false
	}
	// AdjustResolution(+1) is FINER, so coarser needs a negative delta.
	moved := bg.AdjustResolution(-detents)
	if moved {
		if k := (samplerValueBinding{dv: b.dv, idx: b.idx}).knob(); k != nil {
			k.StepMul = bg.Step()
		}
		b.dv.persistKnobStep(bg)
	}
	return moved
}

func (b samplerResolutionBinding) canStep(dir int) bool {
	bg := b.badge()
	if bg == nil {
		return false
	}
	i := bg.Index()
	if dir > 0 {
		return i < bg.LadderLen()-1
	}
	return i > 0
}
```

- [ ] **Step 6: Run the new tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSampler(BarrelLabels|ValueBinding|ResolutionBinding)'
```
Expected: PASS.

- [ ] **Step 7: Verify the value-text refactor changed nothing**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sampler'
```
Expected: PASS. `samplerKnobValueText` and `setKnob` were both refactored to route through the new pure helpers; any caption/undo-notification test failure means the mapping extraction is wrong.

---

## Task 7: Sampler arm state, editor, and grow wiring

**Files:**
- Create: `src/go/internal/ui/sampler_armed_control.go`
- Modify: `src/go/internal/ui/armed_control_editor_factories.go` (add the sampler factory)
- Modify: `src/go/internal/ui/drumview.go` (two new fields)
- Modify: `src/go/internal/ui/sampler_panel_zone.go` (`layoutSamplerKnobsScrolling` calls `growArmedCard`)
- Modify: `src/go/internal/ui/synth_armed_control.go` (`armedControlActive` gains its sampler branch)
- Test: `src/go/internal/ui/sampler_armed_state_test.go` (new)

**Interfaces:**
- Consumes: `armedScope` (Task 3), `newArmedControlEditor` (Task 2), `growArmedCard`, `setArmGrowSubject`.
- Produces:
  - `dv.samplerArmedControl map[string]int`, `dv.samplerArmedEditor *ArmedControlEditor`
  - `func (dv *DrumView) samplerControlArmed(instID string) (int, bool)`
  - `func (dv *DrumView) armSamplerControl(instID string, idx int)`
  - `func (dv *DrumView) lockSamplerControl(instID string)`
  - `func (dv *DrumView) toggleSamplerControlArm(instID string, idx int)`
  - `func (dv *DrumView) samplerArmedActive() bool`
  - `func newSamplerArmedControlEditor(dv *DrumView, instID string, idx int) *ArmedControlEditor`
  - `func (dv *DrumView) onSamplerArmedEdit(idx int)`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/sampler_armed_state_test.go`:

```go
//go:build test

package ui

import "testing"

// samplerArmedGame returns a laid-out sampler tab with a buffer, plus the
// instrument id the sampler is editing.
func samplerArmedGame(t *testing.T) (*Game, string) {
	t.Helper()
	g := newSamplerTabGame(t)
	g.Layout(1280, 720)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	return g, g.drum.samplerActiveInstrument()
}

// TestArmSamplerControlBuildsEditorAndGrowSubject pins the arm entry point:
// the arm map records the index, an editor is built for it, and the grow
// animation is aimed at 1 in the SAMPLER scope.
func TestArmSamplerControlBuildsEditorAndGrowSubject(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum

	dv.armSamplerControl(inst, samplerKnobGain)

	idx, armed := dv.samplerControlArmed(inst)
	if !armed || idx != samplerKnobGain {
		t.Fatalf("samplerControlArmed = (%d,%v), want (%d,true)", idx, armed, samplerKnobGain)
	}
	if dv.samplerArmedEditor == nil {
		t.Fatal("arming must build a sampler armed editor")
	}
	if dv.samplerArmedEditor.res() == nil {
		t.Error("a continuous sampler knob must get a resolution lane")
	}
	if gi, active := dv.armGrowControl(armedScopeSampler, inst); !active || gi != samplerKnobGain {
		t.Fatalf("grow subject = (%d,%v), want (%d,true) in the sampler scope", gi, active, samplerKnobGain)
	}
	if _, active := dv.armGrowControl(armedScopeSynth, inst); active {
		t.Error("arming a SAMPLER control must not create a SYNTH grow subject")
	}
}

// TestLockSamplerControlShrinksThenClears pins that locking aims the grow
// animation back at 0 (so the card visibly shrinks) and drops the editor.
func TestLockSamplerControlShrinksThenClears(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	dv.armSamplerControl(inst, samplerKnobStart)

	dv.lockSamplerControl(inst)

	if _, armed := dv.samplerControlArmed(inst); armed {
		t.Error("lock must clear the arm entry")
	}
	if dv.samplerArmedEditor != nil {
		t.Error("lock must drop the editor")
	}
	if dv.armGrowTarget != 0 {
		t.Errorf("lock must aim the grow animation at 0 (shrink), got %v", dv.armGrowTarget)
	}
	// The subject is retained until the shrink settles, so the card animates out.
	if _, active := dv.armGrowControl(armedScopeSampler, inst); !active {
		t.Error("grow subject must be retained through the shrink")
	}
}

// TestToggleSamplerControlArmLocksSameIndexArmsOther mirrors the synth's
// toggle contract exactly.
func TestToggleSamplerControlArmLocksSameIndexArmsOther(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum

	dv.toggleSamplerControlArm(inst, samplerKnobDetune)
	if idx, armed := dv.samplerControlArmed(inst); !armed || idx != samplerKnobDetune {
		t.Fatalf("first toggle should arm: (%d,%v)", idx, armed)
	}
	dv.toggleSamplerControlArm(inst, samplerKnobDetune)
	if _, armed := dv.samplerControlArmed(inst); armed {
		t.Fatal("toggling the SAME index should lock")
	}
	dv.toggleSamplerControlArm(inst, samplerKnobDetune)
	dv.toggleSamplerControlArm(inst, samplerKnobGain)
	if idx, armed := dv.samplerControlArmed(inst); !armed || idx != samplerKnobGain {
		t.Fatalf("toggling a DIFFERENT index should move the arm: (%d,%v)", idx, armed)
	}
}

// TestSamplerAndSynthArmStatesAreIndependent is the scope guard: the same
// instrument id armed in one tab must be invisible to the other.
func TestSamplerAndSynthArmStatesAreIndependent(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum

	dv.armSamplerControl(inst, samplerKnobGain)
	if _, armed := dv.synthControlArmed(inst); armed {
		t.Error("a sampler arm must not appear as a synth arm for the same instrument")
	}

	dv.armSynthControl(inst, 0)
	if idx, armed := dv.samplerControlArmed(inst); !armed || idx != samplerKnobGain {
		t.Errorf("arming a synth control must not disturb the sampler arm: (%d,%v)", idx, armed)
	}
}

// TestSamplerArmedGrowsTheCard drives the REAL layout path frame by frame and
// asserts the armed card actually gets bigger than its resting slot — the
// growArmedCard call has to be wired into the sampler's layout, not just the
// synth's.
func TestSamplerArmedGrowsTheCard(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	idx := samplerKnobGain
	resting := dv.sampler.knobs[idx].Rect()
	if resting.Empty() {
		t.Fatal("precondition: knob must be laid out at rest")
	}

	dv.armSamplerControl(inst, idx)
	for i := 0; i < 20 && dv.advanceArmGrow(); i++ {
		layoutSamplerTab(t, g)
	}
	layoutSamplerTab(t, g)

	grown := dv.sampler.knobs[idx].Rect()
	if grown.Empty() {
		t.Fatal("armed card must not be empty after the grow")
	}
	if grown.Dx() <= resting.Dx() && grown.Dy() <= resting.Dy() {
		t.Fatalf("armed card did not grow: resting %v -> grown %v", resting, grown)
	}
	if grown.Min.Y < resting.Min.Y {
		t.Errorf("armed card grew UPWARD (top %d -> %d); growth must be down+right only",
			resting.Min.Y, grown.Min.Y)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestArmSamplerControl|TestLockSamplerControl|TestToggleSamplerControlArm|TestSamplerAndSynthArmStates|TestSamplerArmedGrows'
```
Expected: FAIL to compile — `undefined: dv.armSamplerControl`.

- [ ] **Step 3: Add the DrumView fields**

In `src/go/internal/ui/drumview.go`, next to `instEditorArmedControl` / `instEditorArmedEditor` (~line 523):

```go
	// samplerArmedControl / samplerArmedEditor are the Sampler tab's half of the
	// armed-control model. They are DELIBERATELY separate from the synth's pair
	// (rather than one map keyed by armedScope): both tabs address controls by an
	// index into their own widget slice, so the same instrument id means
	// different things in each. The shared BEHAVIOR lives in ArmedControlEditor,
	// drawArmedControlCard, and growArmedCard — only this ~30 lines of map
	// bookkeeping is per-tab.
	samplerArmedControl map[string]int
	samplerArmedEditor  *ArmedControlEditor
```

- [ ] **Step 4: Add the sampler arm state**

Create `src/go/internal/ui/sampler_armed_control.go`:

```go
package ui

// sampler_armed_control.go — which Sampler knob is ARMED for inline barrel
// editing. Mirrors synth_armed_control.go one-for-one; see armed_scope.go for
// why the two tabs keep separate bookkeeping.
//
// At most one knob is armed per instrument. Arming locks any previously-armed
// one and rebuilds the editor; locking removes the entry but RETAINS the grow
// subject so the card visibly shrinks.

func (dv *DrumView) samplerControlArmed(instID string) (int, bool) {
	if dv.samplerArmedControl == nil || instID == "" {
		return -1, false
	}
	idx, ok := dv.samplerArmedControl[instID]
	return idx, ok
}

func (dv *DrumView) armSamplerControl(instID string, idx int) {
	if instID == "" || idx < 0 || idx >= samplerKnobCount {
		return
	}
	if dv.samplerArmedControl == nil {
		dv.samplerArmedControl = map[string]int{}
	}
	dv.samplerArmedControl[instID] = idx
	dv.samplerArmedEditor = newSamplerArmedControlEditor(dv, instID, idx)
	dv.setArmGrowSubject(armedScopeSampler, instID, idx, 1)
}

func (dv *DrumView) lockSamplerControl(instID string) {
	if dv.samplerArmedControl != nil {
		if idx, ok := dv.samplerArmedControl[instID]; ok {
			// Aim the animation back at rest; the subject is retained by
			// setArmGrowSubject/advanceArmGrow until it settles so the card
			// visibly shrinks and neighbours slide home after it is unarmed.
			dv.setArmGrowSubject(armedScopeSampler, instID, idx, 0)
		}
		delete(dv.samplerArmedControl, instID)
	}
	dv.samplerArmedEditor = nil
}

func (dv *DrumView) toggleSamplerControlArm(instID string, idx int) {
	if cur, armed := dv.samplerControlArmed(instID); armed && cur == idx {
		dv.lockSamplerControl(instID)
		return
	}
	dv.armSamplerControl(instID, idx)
}

// samplerArmedActive reports whether a Sampler knob is armed for the sampler's
// active instrument. Tab-agnostic on purpose — armedControlActive() adds the
// active-tab test.
func (dv *DrumView) samplerArmedActive() bool {
	if dv == nil || len(dv.samplerArmedControl) == 0 || dv.samplerArmedEditor == nil {
		return false
	}
	_, armed := dv.samplerControlArmed(dv.samplerActiveInstrument())
	return armed
}
```

- [ ] **Step 5: Add the sampler editor factory**

Append to `src/go/internal/ui/armed_control_editor_factories.go`:

```go
// newSamplerArmedControlEditor binds the armed editor to sampler knob idx.
// Every sampler param is continuous and carries a step badge, so the resolution
// lane is always present — but the availability check stays so the shape
// matches the synth factory exactly.
func newSamplerArmedControlEditor(dv *DrumView, instID string, idx int) *ArmedControlEditor {
	var resB barrelBinding
	if rb := (samplerResolutionBinding{dv: dv, idx: idx}); rb.available() {
		resB = rb
	}
	return newArmedControlEditor(instID, idx,
		samplerValueBinding{dv: dv, idx: idx}, resB,
		func() { dv.onSamplerArmedEdit(idx) })
}

// onSamplerArmedEdit is the Sampler tab's per-detent commit path. The value is
// already pushed into samplerState by samplerValueBinding.stepBy; this records
// ONE debounced undo step for the whole spin. Sampler edits must NOT audition —
// they apply on Save/Preview — so there is no audition call here.
func (dv *DrumView) onSamplerArmedEdit(idx int) {
	dv.samplerWheelNudge.bump(func() { dv.commitSamplerEdit() })
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.Invalidate()
	}
}
```

- [ ] **Step 6: Wire the grow into the sampler layout**

At the very end of `layoutSamplerKnobsScrolling` in `src/go/internal/ui/sampler_panel_zone.go`, after the per-knob `SetRect` loop, append:

```go
	// The ARMED knob's card grows in place to host the two barrel lanes, and its
	// neighbours reflow down out of the way. Same call, same reflow, same
	// animation the Synth tab uses — the sampler gets identical behavior by
	// sharing the implementation rather than mirroring it. `visible` is the
	// identity slice: the sampler has no phantom (divider) cells.
	inst := dv.samplerActiveInstrument()
	if inst != "" {
		if idx, active := dv.armGrowControl(armedScopeSampler, inst); active {
			visible := make([]int, samplerKnobCount)
			for i := range visible {
				visible[i] = i
			}
			dv.growArmedCard(visible, grid, gridR, idx)
		}
	}
```

**Important:** `growArmedCard` reads and writes `dv.instEditorControls` / `dv.instEditorSliders`, which are the *synth's* widget slices. Generalize it to take a rect getter and a rect setter, so both panels share one reflow implementation.

Rename the existing function body to `growArmedCardInto` with this exact signature:

```go
func (dv *DrumView) growArmedCardInto(visible []int, grid *ControlGrid, innerR image.Rectangle,
	armedKIdx int, rectOf func(k int) image.Rectangle, setRect func(k int, r image.Rectangle))
```

Inside that body, make exactly two substitutions and change nothing else:
1. The resting-rect read `resting[i] = dv.instEditorControls[k].Rect()` becomes `resting[i] = rectOf(k)`.
2. The final write block
   ```go
   if k >= 0 && k < len(dv.instEditorControls) {
       dv.instEditorControls[k].SetRect(r)
       if k < len(dv.instEditorSliders) {
           dv.instEditorSliders[k].SetRect(r)
       }
   }
   ```
   becomes `setRect(k, r)`.

Leave the `spanRect` / `CellAtUnclipped` / `ClipBottom` / height-reach logic untouched — that is the hard-won geometry and it is panel-independent.

Add the two per-panel accessors:

```go
// setSynthControlRect assigns a reflowed rect to synth control k (and its
// mirrored legacy slider, which keeps hit-tests and JS export rects landing on
// the widget).
func (dv *DrumView) setSynthControlRect(k int, r image.Rectangle) {
	if k < 0 || k >= len(dv.instEditorControls) {
		return
	}
	dv.instEditorControls[k].SetRect(r)
	if k < len(dv.instEditorSliders) {
		dv.instEditorSliders[k].SetRect(r)
	}
}

func (dv *DrumView) synthControlRect(k int) image.Rectangle {
	if k < 0 || k >= len(dv.instEditorControls) {
		return image.Rectangle{}
	}
	return dv.instEditorControls[k].Rect()
}

// setSamplerControlRect assigns a reflowed rect to sampler knob k.
func (dv *DrumView) setSamplerControlRect(k int, r image.Rectangle) {
	if k < 0 || k >= len(dv.sampler.knobs) || dv.sampler.knobs[k] == nil {
		return
	}
	dv.sampler.knobs[k].SetRect(r)
}

func (dv *DrumView) samplerControlRect(k int) image.Rectangle {
	if k < 0 || k >= len(dv.sampler.knobs) || dv.sampler.knobs[k] == nil {
		return image.Rectangle{}
	}
	return dv.sampler.knobs[k].Rect()
}
```

Keep `growArmedCard` as the synth's thin wrapper so its ~10 existing call sites and tests are untouched:

```go
func (dv *DrumView) growArmedCard(visible []int, grid *ControlGrid, innerR image.Rectangle, armedKIdx int) {
	dv.growArmedCardInto(visible, grid, innerR, armedKIdx,
		dv.synthControlRect, dv.setSynthControlRect)
}
```

And the sampler call in `layoutSamplerKnobsScrolling` becomes:

```go
			dv.growArmedCardInto(visible, grid, gridR, idx,
				dv.samplerControlRect, dv.setSamplerControlRect)
```

`growArmedCardLegacy` stays **synth-only**: the sampler has no phantom `-1` cells, so it can never reach that dispatch. Leave `growArmedCardInto`'s existing early `for _, k := range visible { if k < 0 { ...Legacy...; return } }` guard exactly as it is.

- [ ] **Step 7: Add the sampler branch to `armedControlActive`**

In `src/go/internal/ui/synth_armed_control.go`, extend the switch:

```go
	case TabSampler:
		return dv.samplerArmedActive()
```

- [ ] **Step 8: Run the new tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestArmSamplerControl|TestLockSamplerControl|TestToggleSamplerControlArm|TestSamplerAndSynthArmStates|TestSamplerArmedGrows'
```
Expected: PASS.

- [ ] **Step 9: Verify the synth grow is unchanged by the signature split**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Armed|Reflow|Grow'
```
Expected: PASS with no test edits.

- [ ] **Step 10: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

---

## Task 8: Sampler hit adapter — the armed gesture model

**Files:**
- Modify: `src/go/internal/ui/sampler_panel_zone.go:1476-1560` (`samplerKnobHitAdapter`)
- Test: `src/go/internal/ui/sampler_armed_input_test.go` (new)

**Interfaces:**
- Consumes: `dv.samplerControlArmed`, `dv.armSamplerControl`, `dv.toggleSamplerControlArm`, `dv.samplerArmedEditor`, `dragScrollArbiter`, `tapMaxMovePx`.
- Produces: `samplerKnobHitAdapter` gains `armedMoved`, `armedStartX`, `armedStartY`, and an `isArmed()` helper. `OnWheel` no longer nudges the value.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/sampler_armed_input_test.go`:

```go
//go:build test

package ui

import "testing"

// samplerAdapterFor returns a fresh hit adapter for sampler knob idx, driven
// through its real OnPress/OnDrag/OnRelease/OnWheel lifecycle (never internals).
func samplerAdapterFor(dv *DrumView, idx int) *samplerKnobHitAdapter {
	return &samplerKnobHitAdapter{dv: dv, idx: idx}
}

// TestSamplerStationaryTapArmsOnRelease pins the not-armed contract: a tap ARMS
// on release (not on press — pressing must stay capturable so a sweep can
// become a scroll instead).
func TestSamplerStationaryTapArmsOnRelease(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	idx := samplerKnobGain
	r := dv.sampler.knobs[idx].Rect()
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	a := samplerAdapterFor(dv, idx)
	if got := a.OnPress(cx, cy); got != InputCaptured {
		t.Fatalf("OnPress = %v, want InputCaptured (a sweep must be able to scroll)", got)
	}
	if _, armed := dv.samplerControlArmed(inst); armed {
		t.Fatal("press alone must not arm — arming happens on release")
	}
	a.OnRelease(cx, cy)
	if i, armed := dv.samplerControlArmed(inst); !armed || i != idx {
		t.Fatalf("stationary tap must ARM: (%d,%v)", i, armed)
	}
}

// TestSamplerVerticalSweepScrollsAndDoesNotArm pins the swipe-scrolls gate.
func TestSamplerVerticalSweepScrollsAndDoesNotArm(t *testing.T) {
	g, inst := samplerArmedGame(t)
	// A narrow, short panel makes the grid actually overflow so a sweep scrolls.
	g.Layout(420, 900)
	withSmallScreen(t, true)
	layoutSamplerTab(t, g)
	dv := g.drum

	idx := 0
	r := dv.sampler.knobs[idx].Rect()
	if r.Empty() {
		t.Skip("knob 0 not laid out at this size")
	}
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	a := samplerAdapterFor(dv, idx)
	a.OnPress(cx, cy)
	for dy := 0; dy <= 4*tapMaxMovePx; dy += tapMaxMovePx {
		a.OnDrag(cx, cy-dy) // sweep upward past the tap slop
	}
	a.OnRelease(cx, cy-4*tapMaxMovePx)

	if _, armed := dv.samplerControlArmed(inst); armed {
		t.Fatal("a committed vertical sweep must NOT arm the control")
	}
}

// TestSamplerNotArmedWheelScrollsGridNotValue pins the wheel semantics change:
// a wheel notch over a NOT-armed card scrolls the grid; it never edits the
// value (arm the control first). This is the synth's rule.
func TestSamplerNotArmedWheelScrollsGridNotValue(t *testing.T) {
	g, _ := samplerArmedGame(t)
	g.Layout(420, 900)
	withSmallScreen(t, true)
	layoutSamplerTab(t, g)
	dv := g.drum

	idx := 0
	r := dv.sampler.knobs[idx].Rect()
	if r.Empty() {
		t.Skip("knob 0 not laid out at this size")
	}
	before := dv.sampler.knobs[idx].Value

	a := samplerAdapterFor(dv, idx)
	a.OnWheel(r.Min.X+2, r.Min.Y+2, +1)

	if dv.sampler.knobs[idx].Value != before {
		t.Fatalf("a not-armed wheel notch must NOT change the value: %v -> %v",
			before, dv.sampler.knobs[idx].Value)
	}
}

// TestSamplerArmedStationaryClickLocks pins that any stationary click on the
// armed card locks it — including on the value readout band (manual numeric
// entry is disabled, matching the synth).
func TestSamplerArmedStationaryClickLocks(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	idx := samplerKnobGain
	dv.armSamplerControl(inst, idx)
	for i := 0; i < 20 && dv.advanceArmGrow(); i++ {
		layoutSamplerTab(t, g)
	}
	r := dv.sampler.knobs[idx].Rect()
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	a := samplerAdapterFor(dv, idx)
	a.OnPress(cx, cy)
	a.OnRelease(cx, cy)

	if _, armed := dv.samplerControlArmed(inst); armed {
		t.Fatal("a stationary click on the armed card must LOCK it")
	}
}

// TestSamplerArmedSubDetentDragSpinsAndDoesNotLock is the bug this rule exists
// for: a drag past the 10px tap slop but SHORTER than one barrel detent
// (22-32px) must spin the lane, not read as "stationary" and lock.
func TestSamplerArmedSubDetentDragSpinsAndDoesNotLock(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	idx := samplerKnobGain
	dv.armSamplerControl(inst, idx)
	for i := 0; i < 20 && dv.advanceArmGrow(); i++ {
		layoutSamplerTab(t, g)
	}
	r := dv.sampler.knobs[idx].Rect()
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	detent := Profile().DensityValues().SynthWheelRowH
	sub := tapMaxMovePx + 2 // past the tap slop, well under one detent
	if sub >= detent {
		t.Skipf("tap slop %d >= detent %d at this density", sub, detent)
	}

	a := samplerAdapterFor(dv, idx)
	a.OnPress(cx, cy)
	a.OnDrag(cx, cy+sub)
	a.OnRelease(cx, cy+sub)

	if _, armed := dv.samplerControlArmed(inst); !armed {
		t.Fatal("a sub-detent DRAG must keep the control armed (it spins, it does not lock)")
	}
}

// TestSamplerArmedWheelSpinsTheValueLane pins that an armed wheel notch over
// the value lane changes the value, and collapses into ONE undo step.
func TestSamplerArmedWheelSpinsTheValueLane(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	idx := samplerKnobGain
	dv.armSamplerControl(inst, idx)
	for i := 0; i < 20 && dv.advanceArmGrow(); i++ {
		layoutSamplerTab(t, g)
	}
	e := dv.samplerArmedEditor
	vr := e.ValueLaneRect()
	if vr.Empty() {
		t.Fatal("armed editor has no value lane rect after the grow")
	}
	before := dv.sampler.knobs[idx].Value

	a := samplerAdapterFor(dv, idx)
	a.OnWheel(vr.Min.X+vr.Dx()/2, vr.Min.Y+vr.Dy()/2, +1)
	// Kinetic: the impulse resolves across Ticks.
	for i := 0; i < 60 && e.Tick(); i++ {
	}

	if dv.sampler.knobs[idx].Value == before {
		t.Fatalf("armed wheel over the value lane must change the value (still %v)", before)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSampler(StationaryTap|VerticalSweep|NotArmedWheel|Armed(StationaryClick|SubDetent|Wheel))'
```
Expected: several FAIL — `press alone must not arm` passes today by accident, but `a not-armed wheel notch must NOT change the value` and the armed cases fail (the adapter opens the wheel popup and nudges the value).

- [ ] **Step 3: Rewrite the adapter**

Replace `samplerKnobHitAdapter` and its five methods in `src/go/internal/ui/sampler_panel_zone.go` with:

```go
// samplerKnobHitAdapter is the per-card hit handler, routed through the same
// armed-control model the Synth tab uses (see sampler_armed_control.go /
// synth_armed_editor.go):
//   - NOT armed: a stationary tap ARMS this knob; a vertical sweep scrolls the
//     sampler's ControlGrid via dragScrollArbiter; a wheel notch also scrolls.
//   - ARMED (this knob): press/drag/wheel route straight to the
//     ArmedControlEditor, spinning whichever barrel lane (value/resolution) the
//     pointer is over; a stationary tap LOCKS the knob back out.
//
// The adapter never caches the *Knob — the sampler re-lays-out on buffer
// change, scroll, and instrument switch. Lookup by index stays correct.
type samplerKnobHitAdapter struct {
	dv  *DrumView
	idx int
	arb dragScrollArbiter // tap-vs-scroll gate for the current gesture (NOT-armed path only)

	// armedMoved tracks whether the current armed gesture's CUMULATIVE pointer
	// displacement from the press point has crossed tapMaxMovePx. This is
	// deliberately NOT "did DragTo cross a whole barrel detent": BarrelWheel only
	// reports true once a full SynthWheelRowH (22-32px) detent is crossed, so a
	// sub-detent drag (12-20px, well past the 10px tap slop) would otherwise look
	// "stationary" and wrongly LOCK the control instead of spinning it partway.
	armedMoved  bool
	armedStartX int
	armedStartY int
}

func (h *samplerKnobHitAdapter) knob() *Knob {
	if h.dv == nil || h.idx < 0 || h.idx >= len(h.dv.sampler.knobs) {
		return nil
	}
	return h.dv.sampler.knobs[h.idx]
}

// isArmed reports whether THIS knob is the currently armed one.
func (h *samplerKnobHitAdapter) isArmed() bool {
	idx, armed := h.dv.samplerControlArmed(h.dv.samplerActiveInstrument())
	return armed && idx == h.idx
}

func (h *samplerKnobHitAdapter) grid() *ControlGrid {
	if h.dv == nil {
		return nil
	}
	return h.dv.sampler.knobGrid
}

// glowRect implements glowTarget: hovering a sampler param card lifts it
// through the shared tree hover overlay like every other clickable.
func (h *samplerKnobHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}

func (h *samplerKnobHitAdapter) invalidate() {
	if h.dv != nil && h.dv.eqPanelZone != nil {
		h.dv.eqPanelZone.Invalidate()
	}
}

func (h *samplerKnobHitAdapter) OnPress(x, y int) InputResult {
	if h.isArmed() {
		// Armed: press starts a barrel drag on whichever lane the pointer is
		// over. A stationary press+release LOCKS (see OnRelease).
		h.armedMoved = false
		h.armedStartX = x
		h.armedStartY = y
		if h.dv.samplerArmedEditor != nil {
			h.dv.samplerArmedEditor.BeginDrag(x, y)
		}
		return InputCaptured
	}
	// NOT armed: capture and begin tap-vs-scroll arbitration. Capturing (not
	// consuming) is what lets a sweep become a scroll instead of arming on the
	// down frame.
	h.arb.begin(y, h.grid())
	return InputCaptured
}

func (h *samplerKnobHitAdapter) OnDrag(x, y int) {
	if h.isArmed() {
		if abs(y-h.armedStartY) >= tapMaxMovePx {
			h.armedMoved = true
		}
		if h.dv.samplerArmedEditor != nil && h.dv.samplerArmedEditor.DragTo(y) {
			h.invalidate()
		}
		return
	}
	if h.arb.drag(y) {
		h.invalidate()
	}
}

// OnRelease finishes whichever gesture OnPress started. Armed: ends the barrel
// drag; a gesture that stayed STATIONARY LOCKS the knob back out — a click
// ANYWHERE on the armed card locks, including the value-lane readout (manual
// numeric text entry is disabled, matching the Synth tab); a dragged spin stays
// armed. Not armed: ARMS only when the gesture stayed a tap.
func (h *samplerKnobHitAdapter) OnRelease(x, y int) {
	inst := h.dv.samplerActiveInstrument()
	if h.isArmed() {
		if h.dv.samplerArmedEditor != nil {
			h.dv.samplerArmedEditor.EndDrag()
		}
		if !h.armedMoved {
			h.dv.toggleSamplerControlArm(inst, h.idx)
		}
		return
	}
	if !h.arb.end() {
		return
	}
	h.dv.armSamplerControl(inst, h.idx)
}

// OnWheel routes the notch to whichever surface owns it. Armed: spins the
// hovered barrel lane by one detent, coalesced into ONE debounced undo step by
// the ArmedControlEditor. Not armed: scrolls the ControlGrid — a not-armed
// wheel over a control is a row scroll, never a value edit (arm it first).
func (h *samplerKnobHitAdapter) OnWheel(x, y, steps int) InputResult {
	if h.isArmed() {
		if h.dv.samplerArmedEditor != nil && h.dv.samplerArmedEditor.Wheel(x, y, steps) {
			h.invalidate()
		}
		return InputConsumed
	}
	if g := h.grid(); g != nil && g.WheelStep(steps) {
		h.invalidate()
	}
	return InputConsumed
}
```

Note what is gone: the `paramEditor.Active()` guards (there is no numeric editor any more — Task 14 removes it, and nothing here needs to swallow a tap for it), the `nudgeKnobByWheel` value nudge, and the `openSamplerKnobWheelPopup` call.

- [ ] **Step 4: Run the new tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSampler(StationaryTap|VerticalSweep|NotArmedWheel|Armed(StationaryClick|SubDetent|Wheel))'
```
Expected: PASS.

- [ ] **Step 5: Expect and fix wheel-popup test fallout**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sampler'`

Expected: FAIL in `sampler_wheel_popup_test.go`, `sampler_card_test.go`, and possibly `sampler_control_scroll_drag_test.go` / `undo_sampler_test.go` — they assert the retired wheel-popup behavior. **These are real fallout from this task, not pre-existing failures.** Task 12 deletes/migrates them. For now, note each failing test name in your task report; do not "fix" them by restoring the popup.

- [ ] **Step 6: Verify nothing outside the sampler broke**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Synth|Armed|EQ'
```
Expected: PASS.

---

## Task 9: Draw the armed sampler card

**Files:**
- Modify: `src/go/internal/ui/sampler_panel_zone.go:961-1012` (`drawSamplerKnobs`)
- Test: `src/go/internal/ui/sampler_armed_render_test.go` (new)

**Interfaces:**
- Consumes: `drawArmedControlCard` (Task 4), `samplerValueBinding` / `samplerResolutionBinding` (Task 6), `dv.samplerControlArmed`.
- Produces: no new symbols.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/sampler_armed_render_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

// TestSamplerArmedCardDrawsBarrelLanes pins that the armed knob's card renders
// the two kinetic lanes instead of the resting ParamCard: after a Draw, the
// editor's lane rects must be populated (drawArmedControlCard is what calls
// SetCardRect) and must lie inside the grown card.
func TestSamplerArmedCardDrawsBarrelLanes(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	idx := samplerKnobGain
	dv.armSamplerControl(inst, idx)
	for i := 0; i < 20 && dv.advanceArmGrow(); i++ {
		layoutSamplerTab(t, g)
	}
	layoutSamplerTab(t, g)

	img := newTrackedImage("test.samplerarmed", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	e := dv.samplerArmedEditor
	if e == nil {
		t.Fatal("editor disappeared during draw")
	}
	card := dv.sampler.knobs[idx].Rect()
	vr := e.ValueLaneRect()
	if vr.Empty() {
		t.Fatal("value lane rect empty after Draw — drawArmedControlCard did not run")
	}
	if !vr.In(card) {
		t.Errorf("value lane %v escapes the armed card %v", vr, card)
	}
	rr := e.ResLaneRect()
	if rr.Empty() {
		t.Fatal("resolution lane rect empty after Draw (continuous param must have one)")
	}
	if !rr.In(card) {
		t.Errorf("resolution lane %v escapes the armed card %v", rr, card)
	}
	if rr.Max.X > vr.Min.X {
		t.Errorf("lanes overlap: res %v must sit left of value %v", rr, vr)
	}
}

// TestSamplerNotArmedCardStillDrawsParamCard pins that the resting cards keep
// their ParamCard rendering — only the armed one swaps.
func TestSamplerNotArmedCardStillDrawsParamCard(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	dv.armSamplerControl(inst, samplerKnobGain)
	for i := 0; i < 20 && dv.advanceArmGrow(); i++ {
		layoutSamplerTab(t, g)
	}

	img := newTrackedImage("test.samplerresting", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	for i := 0; i < samplerKnobCount; i++ {
		if i == samplerKnobGain {
			continue
		}
		r := dv.sampler.knobs[i].Rect()
		if r.Empty() {
			continue // reflowed off-window: legitimately hidden
		}
		if r == (image.Rectangle{}) {
			t.Errorf("knob %d has a zero rect but was not skipped", i)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSampler(ArmedCardDraws|NotArmedCardStill)'
```
Expected: FAIL — `value lane rect empty after Draw`.

- [ ] **Step 3: Swap the armed card's renderer**

In `drawSamplerKnobs`, inside the per-knob loop, replace the single `drawParamCard(...)` call with an armed branch:

```go
		// The ARMED knob renders the two kinetic barrel lanes instead of the
		// resting ParamCard — the same renderer the Synth tab uses, so an armed
		// card is pixel-identical on both tabs at equal size. It deliberately
		// bypasses any sprite cache: the lanes animate every frame.
		if armed && armedIdx == i && dv.samplerArmedEditor != nil {
			var resB barrelBinding
			if rb := (samplerResolutionBinding{dv: dv, idx: i}); rb.available() {
				resB = rb
			}
			drawArmedControlCard(dst, r, samplerKnobNameText(i, s),
				dv.samplerArmedEditor,
				samplerValueBinding{dv: dv, idx: i}, resB)
			continue
		}
		drawParamCard(dst, r, samplerKnobNameText(i, s), samplerKnobValueText(i, s),
			paramCardGaugeFor(k, dv.samplerKnobDef(i)), paramCardState{})
```

Add, just above the loop:

```go
	armedIdx, armed := dv.samplerControlArmed(dv.samplerActiveInstrument())
```

Also delete the two `s.readoutRects[i] = image.Rectangle{}` writes and the readout-clearing loop at the top of the function — Task 12 removes the field entirely, and it is already always empty.

- [ ] **Step 4: Run the new tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSampler(ArmedCardDraws|NotArmedCardStill)'
```
Expected: PASS.

- [ ] **Step 5: Package check (wheel-popup fallout still expected)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS except the wheel-popup fallout catalogued in Task 8 Step 5. No NEW failures.

---

## Task 10: Sampler update loop — keyboard, Tick, grow

**Files:**
- Modify: `src/go/internal/ui/sampler_panel_zone.go:1208-1228` (`samplerTabUpdate`)
- Test: `src/go/internal/ui/sampler_armed_keys_test.go` (new)

**Interfaces:**
- Consumes: `dv.samplerArmedEditor`, `dv.advanceArmGrow`, `isKeyJustPressed`, `armedControlActive` (Task 3/7).
- Produces: no new symbols.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/sampler_armed_keys_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSamplerArmedArrowKeysStepValueAndResolution drives the REAL game loop:
// with a sampler knob armed, Up/Down step the value and Left/Right step the
// resolution, exactly as on the Synth tab.
func TestSamplerArmedArrowKeysStepValueAndResolution(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	idx := samplerKnobGain
	dv.armSamplerControl(inst, idx)

	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)

	valBefore := dv.sampler.knobs[idx].Value
	fi.pressKey(t, g, ebiten.KeyArrowUp)
	if dv.sampler.knobs[idx].Value <= valBefore {
		t.Fatalf("ArrowUp must raise the armed value: %v -> %v", valBefore, dv.sampler.knobs[idx].Value)
	}

	badge := dv.sampler.knobStepBadges[idx]
	resBefore := badge.Step()
	fi.pressKey(t, g, ebiten.KeyArrowRight)
	if badge.Step() == resBefore {
		t.Fatalf("ArrowRight must change the armed resolution rung (still %v)", resBefore)
	}
}

// TestSamplerArmedArrowsDoNotPanTheGrid pins the KeyboardClaimed contract: an
// armed control owns the arrow keys, so the camera must not also pan.
func TestSamplerArmedArrowsDoNotPanTheGrid(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	dv.armSamplerControl(inst, samplerKnobGain)

	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)

	camX, camY := g.cam.OffsetX, g.cam.OffsetY
	fi.pressKey(t, g, ebiten.KeyArrowUp)
	fi.pressKey(t, g, ebiten.KeyArrowRight)

	if g.cam.OffsetX != camX || g.cam.OffsetY != camY {
		t.Fatalf("armed arrows panned the grid: (%v,%v) -> (%v,%v)", camX, camY, g.cam.OffsetX, g.cam.OffsetY)
	}
}

// TestSamplerArmedEscLocksAndDoesNotStopPlayback pins the handleEscape rung:
// Esc must lock the armed control WITHOUT falling through to the Stop fallback.
func TestSamplerArmedEscLocksAndDoesNotStopPlayback(t *testing.T) {
	g, inst := samplerArmedGame(t)
	dv := g.drum
	dv.armSamplerControl(inst, samplerKnobGain)

	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)

	g.TriggerPlay()
	fi.frame(t, g)
	if !g.drum.IsPlaying() {
		t.Skip("playback did not start in this harness")
	}

	fi.pressKey(t, g, ebiten.KeyEscape)

	if _, armed := dv.samplerControlArmed(inst); armed {
		t.Error("Esc must lock the armed sampler control")
	}
	if !g.drum.IsPlaying() {
		t.Error("Esc must NOT stop playback when it is consumed locking an armed control")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerArmed(ArrowKeys|Arrows|Esc)'
```
Expected: FAIL — `ArrowUp must raise the armed value` (no keyboard rung yet).

- [ ] **Step 3: Add the keyboard rung, editor Tick, and grow advance**

Replace `samplerTabUpdate` in `src/go/internal/ui/sampler_panel_zone.go` with:

```go
// samplerTabUpdate pumps the Sampler tab's per-frame work: hover refresh, the
// debounced wheel-nudge commit, the knob grid's scroll cooldown, arrow-key
// editing of the ARMED control, that control's kinetic barrel Tick, and the
// grow/shrink animation. Returns true when something changed and the zone needs
// a re-layout + redraw. Mirrors synthTabUpdate.
func (p *samplerPanel) samplerTabUpdate() bool {
	dv := p.dv
	changed := false

	// Refresh which trim handle the cursor is hovering (desktop only) so the
	// hovered handle blooms in its own color.
	p.updateSamplerHandleHover()
	// Advance the debounced wheel/barrel commit once per frame while the tab is
	// active (mirrors synthWheelNudge.tick in synthTabUpdate).
	dv.samplerWheelNudge.tick()
	// Tick the knob-scroll cooldown clock (see ControlGrid.WheelStep/Tick).
	if dv.sampler.knobGrid != nil {
		dv.sampler.knobGrid.Tick()
	}

	// Arrow keyboard editing while a knob is ARMED. KeyboardClaimed (see
	// keyboard_focus.go / armedControlActive) suppresses the grid's arrow-pan for
	// the same duration, so arrows only ever do one thing at a time.
	//
	// Esc is NOT handled here: handleEscape (game_input_shortcuts.go) is the
	// SINGLE authority for Esc and runs before this Update, so it consumes Esc
	// for the armed case (locking it) before reaching its Stop-playback
	// fallback. A duplicate branch here would fire the same frame
	// (isKeyJustPressed is non-consuming) and reintroduce the "Esc also stopped
	// playback" bug.
	if e := dv.samplerArmedEditor; e != nil && dv.samplerArmedActive() {
		switch {
		case isKeyJustPressed(ebiten.KeyArrowUp):
			if e.StepValue(+1) {
				changed = true
			}
		case isKeyJustPressed(ebiten.KeyArrowDown):
			if e.StepValue(-1) {
				changed = true
			}
		case isKeyJustPressed(ebiten.KeyArrowRight):
			if e.StepResolution(+1) {
				changed = true
			}
		case isKeyJustPressed(ebiten.KeyArrowLeft):
			if e.StepResolution(-1) {
				changed = true
			}
		}
		if e.Tick() {
			changed = true
		}
	}

	// Advance the armed-card grow/shrink animation (independent of whether a
	// control is still armed — the shrink runs after lock). A running animation
	// forces a redraw + relayout each frame so growArmedCard re-interpolates.
	if dv.advanceArmGrow() {
		changed = true
	}
	return changed
}
```

- [ ] **Step 4: Add the sampler Esc rung**

In `src/go/internal/ui/game_input_shortcuts.go`, the armed rung (~line 696) currently locks only the synth. Make it scope-aware:

```go
	// 9a. Armed inline control (Synth or Sampler). Arming opens no portal/focus/
	// mode, so without this rung Esc would fall through to the Stop fallback
	// below and reset playback as an unrequested side effect of locking. Lock it
	// and consume the Esc here instead.
	if g.drum != nil && g.drum.armedControlActive() {
		if g.drum.eqPanelZone != nil && g.drum.eqPanelZone.ActiveTab() == TabSampler {
			g.drum.lockSamplerControl(g.drum.samplerActiveInstrument())
		} else {
			g.drum.lockSynthControl(g.drum.synthTabActiveInstrument())
		}
		return true
	}
```

- [ ] **Step 5: Ensure `samplerTabUpdate`'s return value drives a re-layout**

Run: `cd src/go/internal/ui && grep -n "samplerTabUpdate" eq_panel_zone.go`

Confirm the caller treats a `true` return as "request re-layout / redraw" (it previously meant "the numeric editor just closed"). If it only sets a narrow flag, widen it to the same path `synthTabUpdate`'s `changed` return takes. Read both call sites side by side before editing.

- [ ] **Step 6: Run the new tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerArmed(ArrowKeys|Arrows|Esc)'
```
Expected: PASS.

- [ ] **Step 7: Verify the synth's Esc and arrow behavior is unchanged**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Esc|ArrowKey|Armed'
```
Expected: PASS except the catalogued wheel-popup fallout.

---

## Task 11: Per-frame animation invariants + scenes + visual verification

The sentinel-as-geometry bug class (`ControlGrid.CellAt` returns `image.Rectangle{}` for an off-window cell, which is simultaneously "not visible" **and** a zero-size rect at the screen origin) only shows up **mid-animation**. Settled-frame assertions are blind to it. Extend the existing per-frame matrix to the sampler grid.

**Files:**
- Create: `src/go/internal/ui/sampler_armed_anim_invariants_test.go`
- Modify: `src/go/internal/ui/scene_catalog.go` (three new scenes)
- Modify: `src/go/internal/ui/screenshot_subjects.go` (one new subject)
- Modify: `scripts/basic_scenes.txt` (register the cropped scenes)

**Interfaces:**
- Consumes: everything from Tasks 5–10.
- Produces: scene names `crop_sampler_control_armed`, `crop_sampler_control_armed_lastcol`, `sampler_arming_offwindow`; subject const `SubjectSamplerArmedControl`.

- [ ] **Step 1: Read the existing synth invariants test as your template**

Run: `cd src/go/internal/ui && sed -n '1,120p' synth_armed_anim_invariants_test.go`

You are writing the sampler sibling. Do **not** copy the macro-grid setup — the sampler uses `layoutSamplerTab` + `dv.sampler.knobGrid`.

- [ ] **Step 2: Write the failing per-frame test**

Create `src/go/internal/ui/sampler_armed_anim_invariants_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

// sampler_armed_anim_invariants_test.go — per-FRAME geometry invariants for the
// Sampler tab's armed grow/shrink animation.
//
// ControlGrid's "off-window" return value (image.Rectangle{}) is simultaneously
// a NOT-VISIBLE sentinel AND a zero-size rect at the SCREEN ORIGIN, so feeding
// it to lerpRect makes a control travel to or from (0,0). Both instances of that
// bug were invisible to settled-frame tests, which is why these assertions run
// every frame across several window sizes — including sizes where the grid has a
// single visible row and the reflow genuinely pushes neighbours off-window.

// samplerAnimLayouts covers a roomy desktop grid, mid sizes, and narrow/short
// grids where visRows floors to 1.
var samplerAnimLayouts = [][2]int{
	{1280, 900}, {900, 1000}, {700, 900}, {560, 820}, {420, 760},
}

// runSamplerArmFrames arms knob `pick` and yields every frame of the grow to
// fn, with the resting rects and the grid's content rect.
func runSamplerArmFrames(t *testing.T, w, h, pick int,
	fn func(frame int, rects map[int]image.Rectangle, rest map[int]image.Rectangle, content image.Rectangle)) {
	t.Helper()
	g := newSamplerTabGame(t)
	g.Layout(w, h)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	dv := g.drum

	grid := dv.sampler.knobGrid
	if grid == nil || grid.Cols() < 1 {
		t.Skipf("[%dx%d] no sampler knob grid", w, h)
	}
	rest := map[int]image.Rectangle{}
	for i, k := range dv.sampler.knobs {
		if k != nil {
			rest[i] = k.Rect()
		}
	}
	if rest[pick].Empty() {
		t.Skipf("[%dx%d] knob %d is off-window at rest", w, h, pick)
	}
	content := grid.ContentRect()

	dv.armSamplerControl(dv.samplerActiveInstrument(), pick)
	for frame := 0; frame < 30; frame++ {
		layoutSamplerTab(t, g)
		cur := map[int]image.Rectangle{}
		for i, k := range dv.sampler.knobs {
			if k != nil {
				cur[i] = k.Rect()
			}
		}
		fn(frame, cur, rest, content)
		if !dv.advanceArmGrow() {
			return
		}
	}
}

// TestSamplerArmAnim_NothingFliesToTheOrigin is the direct guard for the
// sentinel-as-geometry bug: no control may ever be positioned at or near the
// screen origin mid-animation.
func TestSamplerArmAnim_NothingFliesToTheOrigin(t *testing.T) {
	for _, wh := range samplerAnimLayouts {
		w, h := wh[0], wh[1]
		for pick := 0; pick < samplerKnobCount; pick++ {
			runSamplerArmFrames(t, w, h, pick, func(frame int, cur, rest map[int]image.Rectangle, content image.Rectangle) {
				for i, r := range cur {
					if r.Empty() {
						continue // legitimately hidden
					}
					if r.Min.X < content.Min.X || r.Min.Y < content.Min.Y {
						t.Fatalf("[%dx%d pick=%d frame=%d] knob %d at %v escapes the grid content %v (origin flight)",
							w, h, pick, frame, i, r, content)
					}
				}
			})
		}
	}
}

// TestSamplerArmAnim_NothingMovesUp pins the down-only growth rule: the armed
// card anchors at the clicked cell's top-left and only ever grows DOWN and
// RIGHT. No control may rise above its resting row on any frame.
func TestSamplerArmAnim_NothingMovesUp(t *testing.T) {
	for _, wh := range samplerAnimLayouts {
		w, h := wh[0], wh[1]
		for pick := 0; pick < samplerKnobCount; pick++ {
			runSamplerArmFrames(t, w, h, pick, func(frame int, cur, rest map[int]image.Rectangle, _ image.Rectangle) {
				for i, r := range cur {
					if r.Empty() || rest[i].Empty() {
						continue
					}
					if r.Min.Y < rest[i].Min.Y {
						t.Fatalf("[%dx%d pick=%d frame=%d] knob %d rose above its resting row: %v -> %v",
							w, h, pick, frame, i, rest[i], r)
					}
				}
			})
		}
	}
}

// TestSamplerArmAnim_OffWindowAtRestStaysHidden pins the second instance of the
// origin bug: a control already off-window before the grow must stay hidden the
// whole animation (a down-only reflow can never bring it into view).
func TestSamplerArmAnim_OffWindowAtRestStaysHidden(t *testing.T) {
	for _, wh := range samplerAnimLayouts {
		w, h := wh[0], wh[1]
		for pick := 0; pick < samplerKnobCount; pick++ {
			runSamplerArmFrames(t, w, h, pick, func(frame int, cur, rest map[int]image.Rectangle, _ image.Rectangle) {
				for i, r := range cur {
					if rest[i].Empty() && !r.Empty() {
						t.Fatalf("[%dx%d pick=%d frame=%d] knob %d was off-window at rest but appeared at %v",
							w, h, pick, frame, i, r)
					}
				}
			})
		}
	}
}

// TestSamplerArmAnim_NeighboursNeverOverlap pins that the reflow never leaves
// two visible cards on top of each other mid-flight.
func TestSamplerArmAnim_NeighboursNeverOverlap(t *testing.T) {
	for _, wh := range samplerAnimLayouts {
		w, h := wh[0], wh[1]
		for pick := 0; pick < samplerKnobCount; pick++ {
			runSamplerArmFrames(t, w, h, pick, func(frame int, cur, _ map[int]image.Rectangle, _ image.Rectangle) {
				for i := 0; i < samplerKnobCount; i++ {
					for j := i + 1; j < samplerKnobCount; j++ {
						a, b := cur[i], cur[j]
						if a.Empty() || b.Empty() {
							continue
						}
						if a.Overlaps(b) {
							t.Fatalf("[%dx%d pick=%d frame=%d] knobs %d %v and %d %v overlap",
								w, h, pick, frame, i, a, j, b)
						}
					}
				}
			})
		}
	}
}

// TestSamplerArmAnim_NeverPaintsBelowTheFold pins that a control travelling
// toward an off-window row is clipped at the grid's bottom edge rather than
// painting over whatever follows the grid.
func TestSamplerArmAnim_NeverPaintsBelowTheFold(t *testing.T) {
	for _, wh := range samplerAnimLayouts {
		w, h := wh[0], wh[1]
		for pick := 0; pick < samplerKnobCount; pick++ {
			runSamplerArmFrames(t, w, h, pick, func(frame int, cur, _ map[int]image.Rectangle, content image.Rectangle) {
				for i, r := range cur {
					if r.Empty() {
						continue
					}
					if r.Max.Y > content.Max.Y {
						t.Fatalf("[%dx%d pick=%d frame=%d] knob %d %v paints below the grid fold (content %v)",
							w, h, pick, frame, i, r, content)
					}
				}
			})
		}
	}
}
```

- [ ] **Step 3: Run the invariants and fix real failures**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerArmAnim'
```
Expected: PASS. **If a case fails, it is a real geometry bug in the sampler's grow wiring — fix `growArmedCardInto`'s clipping / off-window substitution for the sampler path. Do NOT relax an assertion or add a skip.**

- [ ] **Step 4: Add the screenshot subject**

In `src/go/internal/ui/screenshot_subjects.go`, declare the const and return its bounds from `(*Game).SubjectRect`:

```go
	SubjectSamplerArmedControl Subject = "sampler_armed_control"
```

```go
	case SubjectSamplerArmedControl:
		return g.drum.SamplerArmedControlBounds()
```

Add the public accessor in `src/go/internal/ui/sampler_panel_zone.go` (never reach into private fields from `screenshot_subjects.go`):

```go
// SamplerArmedControlBounds returns the screen rect of the currently armed
// sampler card (empty when none is armed). Used by the screenshot harness to
// crop to the armed control on both platforms.
func (dv *DrumView) SamplerArmedControlBounds() image.Rectangle {
	idx, armed := dv.samplerControlArmed(dv.samplerActiveInstrument())
	if !armed || idx < 0 || idx >= len(dv.sampler.knobs) || dv.sampler.knobs[idx] == nil {
		return image.Rectangle{}
	}
	return dv.sampler.knobs[idx].Rect()
}
```

- [ ] **Step 5: Add the three scenes**

In `src/go/internal/ui/scene_catalog.go`, add next to the existing `crop_sampler_tab_*` entries. Reuse `samplerTabSceneSetup(true)` and `mobileSamplerTabSceneSetup(true)` (the existing helpers at ~line 2093/2127), then arm and settle:

```go
	{Name: "crop_sampler_control_armed",
		Description: "Sampler tab — a control card ARMED: grown in place with the resolution and value barrel lanes",
		Subject:     SubjectSamplerArmedControl, SettleFrames: 60,
		Setup:       samplerArmedSceneSetup(samplerKnobGain),
		MobileSetup: mobileSamplerArmedSceneSetup(samplerKnobGain)},
	{Name: "crop_sampler_control_armed_lastcol",
		Description: "Sampler tab — the RIGHTMOST control armed: the block keeps the clicked cell as its right column and grows left",
		Subject:     SubjectSamplerArmedControl, SettleFrames: 60,
		Setup:       samplerArmedSceneSetup(samplerKnobCount - 1),
		MobileSetup: mobileSamplerArmedSceneSetup(samplerKnobCount - 1)},
	// Deliberately NOT subject-cropped: an origin-flight regression leaves the
	// panel entirely, so a crop cannot show it. Capture full-screen.
	{Name: "sampler_arming_offwindow",
		Description: "Sampler tab — MID grow animation (armGrowT≈0.35) so neighbours are caught sliding; full-screen to expose any origin flight",
		SettleFrames: 0,
		Setup:        samplerArmingMidAnimSceneSetup(samplerKnobGain),
		MobileSetup:  mobileSamplerArmingMidAnimSceneSetup(samplerKnobGain)},
```

And the four setup helpers, next to `samplerTabSceneSetup`:

```go
// samplerArmedSceneSetup applies the sampler-tab setup, arms knob idx, and runs
// the grow animation to completion so the scene shows a SETTLED armed card.
func samplerArmedSceneSetup(idx int) func(*Game) {
	return func(g *Game) {
		samplerTabSceneSetup(true)(g)
		dv := g.drum
		dv.armSamplerControl(dv.samplerActiveInstrument(), idx)
		for i := 0; i < 50 && dv.advanceArmGrow(); i++ {
			dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		}
	}
}

func mobileSamplerArmedSceneSetup(idx int) func(*Game) {
	return func(g *Game) {
		mobileSamplerTabSceneSetup(true)(g)
		dv := g.drum
		dv.armSamplerControl(dv.samplerActiveInstrument(), idx)
		for i := 0; i < 50 && dv.advanceArmGrow(); i++ {
			dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		}
	}
}

// samplerArmingMidAnimSceneSetup freezes the grow partway (armGrowT ≈ 0.35) so
// neighbours are caught sliding to their reflowed slots. Both armGrowT and
// armGrowTarget are pinned so the harness's settle frames leave it put.
func samplerArmingMidAnimSceneSetup(idx int) func(*Game) {
	return func(g *Game) {
		samplerTabSceneSetup(true)(g)
		dv := g.drum
		dv.armSamplerControl(dv.samplerActiveInstrument(), idx)
		dv.armGrowT = 0.35
		dv.armGrowTarget = 0.35
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	}
}

func mobileSamplerArmingMidAnimSceneSetup(idx int) func(*Game) {
	return func(g *Game) {
		mobileSamplerTabSceneSetup(true)(g)
		dv := g.drum
		dv.armSamplerControl(dv.samplerActiveInstrument(), idx)
		dv.armGrowT = 0.35
		dv.armGrowTarget = 0.35
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	}
}
```

Add `crop_sampler_control_armed` and `crop_sampler_control_armed_lastcol` to `scripts/basic_scenes.txt`.

- [ ] **Step 6: Verify the crop scenes produce a visible subject**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSceneCrop'
```
Expected: PASS. `scene_crop_test.go` automatically exercises every `crop_*` scene and requires a non-empty Subject rect that fits inside the framebuffer and is strictly smaller than the full screen. A failure means the scene armed an off-window control.

- [ ] **Step 7: Build the screenshot binary**

Run:
```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a && \
  (cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_ss ./cmd)
```
Expected: clean build.

- [ ] **Step 8: Capture desktop**

Run:
```bash
cd /home/ymolinar/Repos/beatmo && for s in crop_sampler_control_armed crop_sampler_control_armed_lastcol; do
  xvfb-run -a /tmp/beatmo_ss -log ERROR -scene "$s" -screenshot "/tmp/ss_$s.png"; done
xvfb-run -a /tmp/beatmo_ss -log ERROR -win-w 900 -win-h 1000 -scene sampler_arming_offwindow -screenshot /tmp/ss_sampler_offwindow.png
```

- [ ] **Step 9: Capture mobile**

Run:
```bash
cd /home/ymolinar/Repos/beatmo && for s in crop_sampler_control_armed crop_sampler_control_armed_lastcol; do
  xvfb-run -a /tmp/beatmo_ss -log ERROR -mobile -win-w 390 -win-h 844 -scene-pass mobile -scene "$s" -screenshot "/tmp/ss_m_$s.png"; done
xvfb-run -a /tmp/beatmo_ss -log ERROR -mobile -win-w 390 -win-h 844 -scene-pass mobile -scene sampler_arming_offwindow -screenshot /tmp/ss_m_sampler_offwindow.png
```

Note: the native binary needs `-mobile`; the `MOBILE=1` env var does nothing to it. Portrait is required.

- [ ] **Step 10: OPEN every PNG and inspect it**

Use the Read tool on each of the six PNGs. Confirm, for each:
- The armed card is visibly **larger** than the resting cards around it.
- **Two** barrel lanes are present and readable — the narrow resolution ladder on the left, the wider value column on the right, with a visible gutter hairline between them.
- The param name renders in the header band and is **not blank**.
- The last-column scene's armed block keeps the rightmost card's cell as its RIGHT edge and grew leftward; nothing is clipped off-screen.
- In the mid-animation full-screen captures: no stray square near the top-left origin, no card overlapping unrelated chrome, no card above its resting row.

Fix and **re-capture until it looks right**. Do not proceed on green tests alone.

- [ ] **Step 11: Compare against the Synth tab**

Capture `crop_synth_control_armed` the same way and open it alongside `crop_sampler_control_armed`. The two armed cards must read as the same control: same body fill, same accent selection bar, same lane proportions, same typography. Any visible difference is a bug in Task 4's shared renderer or in the density tokens being read.

---

## Task 12: Remove the sampler wheel wiring

**Files:**
- Delete: `src/go/internal/ui/sampler_wheel_popup.go`, `src/go/internal/ui/sampler_wheel_popup_test.go`
- Modify: `src/go/internal/ui/drumview.go` (drop `samplerWheelPopup`), `src/go/internal/ui/drumview_ctor.go:205`, `src/go/internal/ui/drumview_portal_open.go:483-509,687-689`, `src/go/internal/ui/sampler_state.go` (drop `readoutRects`), `src/go/internal/ui/sampler_panel_zone.go` (drop `samplerKnobReadoutRect`, `openSamplerParamEditor`, `samplerMobileKnobButtonRect` if unused)
- Modify: the sampler tests that asserted wheel behavior

**Interfaces:**
- Consumes: nothing new.
- Produces: nothing. Pure removal.

- [ ] **Step 1: Catalogue what currently fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sampler' 2>&1 | grep -E '^(---|\s+---) FAIL'`

Write the list down. Every one must be either migrated to the armed path or deleted with its subject by the end of this task.

- [ ] **Step 2: Delete the wheel files and fields**

```bash
cd /home/ymolinar/Repos/beatmo/src/go/internal/ui
rm sampler_wheel_popup.go sampler_wheel_popup_test.go
```

Then remove:
- `drumview.go:449` — the `samplerWheelPopup` field.
- `drumview_ctor.go:205` — `dv.samplerWheelPopup = NewMobileWheelPopup()`.
- `drumview_portal_open.go` — `openSamplerWheelPortal`, `closeSamplerWheelPortal` (both the `*samplerPanel` methods at 483–509 and the `*DrumView` shims at 687–689).
- `sampler_state.go:87` — the `readoutRects` field.
- `sampler_panel_zone.go` — `samplerKnobReadoutRect` (both the panel method and any `*DrumView` shim), `openSamplerParamEditor` (both), and the `Draw` pump `if dv.paramEditor != nil { dv.paramEditor.Draw(dst) }` at ~907.
- `sampler_panel.go:6` — the doc comment listing `dv.samplerWheelPopup`.

- [ ] **Step 3: Migrate the surviving sampler tests to the armed path**

For each test in the Step 1 list, decide by its **invariant**, not its mechanism:

- **`sampler_card_test.go`** — the "tap opens the wheel popup" assertions become "tap arms the control": replace `dv.samplerWheelPopup.IsOpen()` with `dv.samplerControlArmed(inst)`. Keep the modal-isolation case only if the armed model has an equivalent (it does not — arming opens no portal), otherwise delete that case and note it in your report.
- **`sampler_control_scroll_drag_test.go`** — the swipe-scrolls-vs-tap-opens cases are still valid; retarget the "opens" assertion to "arms". These overlap `TestSamplerVerticalSweepScrollsAndDoesNotArm` from Task 8; if a case is now an exact duplicate, delete the duplicate rather than keeping both.
- **`sampler_knob_interaction_test.go`** — the `openSamplerParamEditor` case is deleted (subject gone).
- **`undo_sampler_test.go`** — the one-undo-step invariant is the valuable part. Rewrite the numeric-editor case as an armed-barrel spin: arm, spin the value lane, tick until the debounce fires, assert exactly one undo step. Use `TestSamplerArmedWheelSpinsTheValueLane` (Task 8) as the driving pattern and `assertOneUndoStepRoundTrip` for the assertion.

- [ ] **Step 4: Verify the sampler suite is green**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sampler|Undo'`
Expected: PASS.

- [ ] **Step 5: Verify no sampler wheel references remain**

Run: `cd src/go/internal/ui && grep -rn "samplerWheelPopup\|SamplerWheelPopupRect\|openSamplerKnobWheelPopup\|sampler-wheel-popup\|openSamplerParamEditor\|readoutRects" .`
Expected: no hits.

- [ ] **Step 6: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS except EQ-wheel fallout (Task 13's subject), which should not have changed.

---

## Task 13: EQ becomes drag-only

**Files:**
- Delete: `src/go/internal/ui/eq_wheel_popup.go`, `src/go/internal/ui/eq_wheel_popup_test.go`, `src/go/internal/ui/eq_wheel_popup_isolation_test.go`, `src/go/internal/ui/eq_wheel_seam_test.go`
- Modify: `src/go/internal/ui/eq_panel_zone.go` (drop `eqDBOpenAdapter`, the `eq-db-*` hit-area loop, `openEQDBEditor`, `eqDBSpec`, `applyBandGainLive`, `paramEditor`, and the stale `stepSelectedBand` comment), `src/go/internal/ui/drumview.go`, `src/go/internal/ui/drumview_ctor.go:397`, `src/go/internal/ui/drumview_close_popups.go:59`, `src/go/internal/ui/drumview_portal_open.go:510-533`, `src/go/internal/ui/drumview_tree.go:365`
- Modify: `src/go/internal/ui/eq_db_input_test.go` — **inverted**, not deleted

**Interfaces:**
- Consumes: nothing new.
- Produces: `EQCallbacks` loses `OnOpenValueWheel`. `EQPanelZone` loses `paramEditor`.

- [ ] **Step 1: Write the inverted guard first**

Replace the body of `src/go/internal/ui/eq_db_input_test.go` with:

```go
//go:build test

package ui

import "testing"

// eq_db_input_test.go — the EQ tab is DRAG-ONLY.
//
// Band gains are set exclusively by the draggable curve handles. The per-band dB
// cells are READ-ONLY readouts: they publish no hit area, and pressing one opens
// nothing. This file used to assert the opposite (a tap opened a numeric editor,
// later a precision wheel); it is inverted deliberately so a reintroduced value
// editor fails loudly instead of silently re-adding a second input path.
func TestEQDBReadoutsPublishNoHitAreas(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 4; i++ { // settle + draw so the dB readout rects exist
		fi.frame(t, g)
	}
	ez := g.drum.eqPanelZone
	if ez == nil {
		t.Skip("no eq panel zone")
	}
	laidOut := false
	for i := 0; i < 10; i++ {
		if !ez.dbReadoutRect(i).Empty() {
			laidOut = true
			break
		}
	}
	if !laidOut {
		t.Skip("no EQ dB readout rect after draw (panel collapsed in this layout)")
	}

	for _, ha := range ez.HitAreas() {
		for i := range eqCenterLabels {
			if ha.Tag == "eq-db-"+eqCenterLabels[i] {
				t.Errorf("EQ dB readout %q must publish NO hit area (EQ is drag-only)", ha.Tag)
			}
		}
	}
}

// TestEQDBReadoutPressChangesNothing drives a real click on a dB readout cell
// through the tree and asserts the band gain is untouched and no editor opened.
func TestEQDBReadoutPressChangesNothing(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 4; i++ {
		fi.frame(t, g)
	}
	ez := g.drum.eqPanelZone
	if ez == nil {
		t.Skip("no eq panel zone")
	}
	idx := -1
	for i := 0; i < 10; i++ {
		if !ez.dbReadoutRect(i).Empty() {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(ez.bandGainsDB) {
		t.Skip("no EQ dB readout rect after draw")
	}
	before := ez.bandGainsDB[idx]

	r := ez.dbReadoutRect(idx)
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)

	if ez.bandGainsDB[idx] != before {
		t.Errorf("clicking a dB readout changed band %d: %v -> %v", idx, before, ez.bandGainsDB[idx])
	}
	if g.drum.valueEditorActive() {
		t.Error("clicking a dB readout must not open any value editor (EQ is drag-only)")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEQDBReadouts?(PublishNoHitAreas|PressChangesNothing)'
```
Expected: FAIL — the hit areas still exist and the click still opens the wheel.

- [ ] **Step 3: Delete the EQ wheel**

```bash
cd /home/ymolinar/Repos/beatmo/src/go/internal/ui
rm eq_wheel_popup.go eq_wheel_popup_test.go eq_wheel_popup_isolation_test.go eq_wheel_seam_test.go
```

Then remove:
- `drumview.go:451-453` — `eqWheelPopup`, `eqWheelStepBadge`, `eqWheelStepRestored`.
- `drumview_ctor.go:206` and the `eqWheelStepBadge` init + its explanatory comment at ~210.
- `drumview_ctor.go:397` — `OnOpenValueWheel: func(band int) { dv.openEQKnobWheelPopup(band) },`
- `eq_panel_zone.go:20`-ish — the `OnOpenValueWheel` field from `EQCallbacks`.
- `drumview_close_popups.go:59` — `dv.closeEQKnobWheelPopup()`.
- `drumview_portal_open.go:510-533` — `openEQWheelPortal` / `closeEQWheelPortal`.

- [ ] **Step 4: Delete the EQ value-entry path**

In `src/go/internal/ui/eq_panel_zone.go` remove:
- `eqDBOpenAdapter` (the struct, `OnPress`, `OnDrag`, `OnRelease`, `OnWheel`, `glowRect`).
- The `if eqTab { for i := range z.dbReadoutRects { ... } }` hit-area loop at ~1668–1684. Keep `dbReadoutRects` itself: the cells still **render** their values.
- `openEQDBEditor` (~1399–1426) and `eqDBSpec` (~1371).
- `applyBandGainLive` (~1432–1445) — its only production caller was the wheel; the curve drag fires `callbacks.OnGainChange` directly at ~1991.
- `z.paramEditor`: the field (~269 area), the ctor init at ~480, the `Update` pump at ~605, the `Draw` pump at ~846, and the `Active()` gates at ~858 and ~865.
- The orphaned `stepSelectedBand` doc comment at ~1272 (the function it documents was deleted long ago).

In `src/go/internal/ui/drumview_tree.go:365`, remove the `ez.paramEditor` gate.

In `src/go/internal/ui/game_input_shortcuts.go`, remove the `dv.eqPanelZone.paramEditor` branches from `valueEditorActive` (~105) and the Esc cancel path (~123).

- [ ] **Step 5: Relocate or delete the dB bounds**

`eqDBMin` / `eqDBMax` / `eqDBSpan` lived in the deleted `eq_wheel_popup.go`.

Run: `cd src/go/internal/ui && grep -rn "eqDBMin\|eqDBMax\|eqDBSpan" .`

If any surviving code references them, move the three consts into `eq_panel_zone.go` next to the curve clamp. If nothing does, they are gone with the file — good.

- [ ] **Step 6: Run the inverted guard**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEQDB'
```
Expected: PASS.

- [ ] **Step 7: Verify the curve handles still work**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'EQ'
```
Expected: PASS. Curve-handle drag, HPF/LPF, mute, and the one-undo-step-per-drag contract must be untouched. Any failure there means you removed more than the value-entry path.

- [ ] **Step 8: Verify no EQ wheel references remain**

Run: `cd src/go/internal/ui && grep -rn "eqWheelPopup\|OnOpenValueWheel\|openEQDBEditor\|eqDBSpec\|applyBandGainLive\|eq-wheel-popup" .`
Expected: no hits.

---

## Task 14: Delete the dead `DrumView.paramEditor` apparatus and re-point the text-input tests

With Sampler and EQ done, no audio-panel tab has a numeric editor. `DrumView.paramEditor` was opened only by `openSynthParamEditor` (already test-only) and the now-deleted `openSamplerParamEditor`.

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (delete `openSynthParamEditor` ×2, `openArmedNumericEditor`, the `paramEditor` pumps and gates), `src/go/internal/ui/drumview.go`, `src/go/internal/ui/game_input_shortcuts.go`, `src/go/internal/ui/native_gesture.go`
- Delete: `src/go/internal/ui/synth_armed_numeric_test.go`
- Modify: `src/go/internal/ui/arrow_key_navigation_functional_test.go`, `src/go/internal/ui/text_input_escape_functional_test.go`, `src/go/internal/ui/knob_interaction_functional_test.go`, `src/go/internal/ui/undo_audio_panel_test.go`
- Modify: the native-gesture discipline test (drop the `param_value_editor.go` exemption)

**Interfaces:**
- Consumes: nothing new.
- Produces: `ParamValueEditor` the **type** survives (the transport BPM box uses it). `DrumView.paramEditor` is gone.

- [ ] **Step 1: Re-point the arrow-key tests onto BPM and rename FIRST**

These must be green before you delete anything, so the coverage never lapses. Replace `TestFunctional_ArrowKeys_EQdBEditor` and `TestFunctional_SoftKeyboardArrowMovesCaret` in `src/go/internal/ui/arrow_key_navigation_functional_test.go`, and delete `TestFunctional_ArrowKeys_SynthControlEditor` (its subject is the synth numeric editor):

```go
//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// arrow_key_navigation_functional_test.go — caret navigation inside the app's
// real text inputs, driven through the REAL Game.Update+Draw loop.
//
// The audio panel has no numeric text entry (Synth/Sampler controls are armed
// inline barrels; EQ is drag-only), so the vehicles here are the two production
// text inputs that DO exist: the transport BPM box (numeric) and the
// instrument-rename field (alphanumeric).

// bpmEditorOpen clicks the BPM readout and returns the opened editor.
func bpmEditorOpen(t *testing.T, g *Game, fi *fakeInput) *ParamValueEditor {
	t.Helper()
	box := g.drum.bpmBox()
	if box == nil || box.Rect.Empty() {
		t.Skip("no BPM box rect")
	}
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	ed := g.drum.transportZone.paramEditor
	if ed == nil || !ed.Active() {
		t.Fatal("clicking the BPM readout did not open the editor")
	}
	return ed
}

// TestFunctional_ArrowKeys_BPMBox: Left/Right must move the caret inside the
// numeric BPM editor and must NOT pan the grid.
func TestFunctional_ArrowKeys_BPMBox(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)
	fi.frame(t, g)

	ed := bpmEditorOpen(t, g, fi)
	startCursor := ed.ti.cursor
	if startCursor == 0 {
		t.Fatalf("precondition: prefilled value should put the caret at the end (>0), got %d", startCursor)
	}
	camBefore := g.cam.OffsetX

	fi.pressKey(t, g, ebiten.KeyLeft)

	if g.cam.OffsetX != camBefore {
		t.Errorf("Left arrow panned the grid while editing (OffsetX %v -> %v)", camBefore, g.cam.OffsetX)
	}
	if !ed.Active() {
		t.Fatal("Left arrow unexpectedly closed the editor")
	}
	if ed.ti.cursor != startCursor-1 {
		t.Fatalf("Left arrow did NOT move the caret: cursor=%d want %d", ed.ti.cursor, startCursor-1)
	}

	fi.pressKey(t, g, ebiten.KeyRight)
	if ed.ti.cursor != startCursor {
		t.Fatalf("Right arrow did NOT move the caret back: cursor=%d want %d", ed.ti.cursor, startCursor)
	}
}

// TestFunctional_SoftKeyboardArrowMovesCaret reproduces the WASM bug: while a
// text field is focused the hidden soft-keyboard proxy holds keyboard focus, so
// the canvas never sees the arrow keydown (isKeyPressed stays false). The proxy
// must forward Left/Right the way it forwards Esc. This fires ONLY the forwarded
// signal (no canvas arrow key) and asserts the caret moves.
func TestFunctional_SoftKeyboardArrowMovesCaret(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)
	fi.frame(t, g)

	ed := bpmEditorOpen(t, g, fi)
	startCursor := ed.ti.cursor
	if startCursor == 0 {
		t.Fatalf("precondition: caret at end (>0), got %d", startCursor)
	}

	// Crucially: NO canvas arrow key (fi.keys is empty). Only the proxy's
	// forwarded signal, exactly as the WASM keydown handler delivers it.
	SignalSoftKeyboardArrowLeft()
	fi.frame(t, g)
	if ed.ti.cursor != startCursor-1 {
		t.Fatalf("soft-keyboard Left did NOT move the caret: cursor=%d want %d", ed.ti.cursor, startCursor-1)
	}

	SignalSoftKeyboardArrowRight()
	fi.frame(t, g)
	if ed.ti.cursor != startCursor {
		t.Fatalf("soft-keyboard Right did NOT move the caret back: cursor=%d want %d", ed.ti.cursor, startCursor)
	}
}

// TestFunctional_ArrowKeys_RenameField covers caret navigation in an
// ALPHANUMERIC field — where left/right navigation actually matters to a user.
func TestFunctional_ArrowKeys_RenameField(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)
	fi.frame(t, g)

	if len(g.drum.Rows) == 0 {
		t.Skip("no rows to rename")
	}
	if g.drum.rowRackZone == nil || g.drum.rowRackZone.callbacks.OnRenameOpen == nil {
		t.Skip("no rename trigger")
	}
	g.drum.rowRackZone.callbacks.OnRenameOpen(0)
	fi.frame(t, g)
	if g.drum.renameComp == nil || !g.drum.renameComp.IsOpen() {
		t.Skip("rename field did not open (mobile/no-textbox build)")
	}

	fi.chars = []rune{'A', 'B', 'C'}
	fi.frame(t, g)

	ti := g.drum.renameComp.textInput()
	if ti == nil {
		t.Skip("rename component exposes no text input in this build")
	}
	startCursor := ti.cursor
	if startCursor == 0 {
		t.Fatalf("precondition: typing should advance the caret, got %d", startCursor)
	}
	camBefore := g.cam.OffsetX

	fi.pressKey(t, g, ebiten.KeyLeft)

	if g.cam.OffsetX != camBefore {
		t.Errorf("Left arrow panned the grid while renaming (OffsetX %v -> %v)", camBefore, g.cam.OffsetX)
	}
	if ti.cursor != startCursor-1 {
		t.Fatalf("Left arrow did NOT move the rename caret: cursor=%d want %d", ti.cursor, startCursor-1)
	}

	fi.pressKey(t, g, ebiten.KeyRight)
	if ti.cursor != startCursor {
		t.Fatalf("Right arrow did NOT move the rename caret back: cursor=%d want %d", ti.cursor, startCursor)
	}
}
```

**If `renameComp` exposes no `textInput()` accessor**, add one (a two-line getter returning the component's `*TextInput`) rather than reaching into private state from the test — check `drumview_rename*.go` first for an existing accessor and use that name instead.

- [ ] **Step 2: Delete the EQ dB Esc case**

In `src/go/internal/ui/text_input_escape_functional_test.go`, delete `TestFunctional_Esc_EQdBInput` (its subject is gone). Leave `fakeInput`, `installFakeInput`, `newDesktopGame`, and the BPM / rename / instrument-search Esc cases exactly as they are — they are used package-wide and already carry the Esc contract.

- [ ] **Step 3: Run the re-pointed tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestFunctional_(ArrowKeys|SoftKeyboardArrow|Esc)'
```
Expected: PASS. Coverage is now on production surfaces; only after this is green do you delete the old apparatus.

- [ ] **Step 4: Delete the tests whose subject is the numeric editor**

```bash
cd /home/ymolinar/Repos/beatmo/src/go/internal/ui && rm synth_armed_numeric_test.go
```

In `knob_interaction_functional_test.go`, delete `TestSynthControlCaptionIsDisplayOnly` and `TestSynthNumericCommitDoesNotAudition`. **Before deleting the latter**, confirm the armed path already pins no-auto-audition:

Run: `cd src/go/internal/ui && grep -rn "must NOT audition\|SwapSynthAuditionFnForTest" synth_no_auto_audition_test.go`

If no armed-path case exists, write one first (arm a control, spin the value lane, tick to commit, assert the audition fn is never called), then delete.

In `undo_audio_panel_test.go`, delete `TestUndoSynthNumericEntry`. **Before deleting**, confirm `TestSynthControlWheelRecordsExactlyOneUndoStep` exists in the same file and covers the one-undo-step invariant.

- [ ] **Step 5: Delete the apparatus**

Remove:
- `synth_panel_zone.go` — `openSynthParamEditor` (the `*synthPanel` method at ~3582 and the `*DrumView` forwarder at ~4616), `openArmedNumericEditor` (~4627), the `paramEditor` `Update` pump (~1948–1955), the `paramEditor.Active()` guard in the armed keyboard block (~1974), the `Draw` pump (~2523), and the `paramEditor.Active()` early returns in `synthControlHitAdapter.OnPress` (~4127) and `OnWheel` (~4207). Also fix the stale comment at ~4187 that mentions `openArmedNumericEditor`.
- `drumview.go` — the `paramEditor` field.
- `game_input_shortcuts.go` — the `dv.paramEditor` branches in `valueEditorActive` (~102) and the Esc cancel (~119–121). `valueEditorActive` should now consult only `transportZone.paramEditor`.
- `native_gesture.go` — `activeParamEditorNativeRects` (~103–130), its `kept = append(...)` call at ~87, and the "Documented exception" comment block at ~78–86 that describes it.

- [ ] **Step 6: Drop the now-stale discipline exemption**

Run: `cd src/go/internal/ui && grep -rn "param_value_editor" native_gesture_discipline_test.go`

Remove the `param_value_editor.go` exemption and delete `TestParamEditorNativeRectArmedBySync`. These guards ratchet — a stale exemption is itself drift.

- [ ] **Step 7: Verify the apparatus is gone**

Run: `cd src/go/internal/ui && grep -rn "paramEditor" . | grep -v transportZone | grep -v "^./param_value_editor.go"`
Expected: no hits.

- [ ] **Step 8: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

---

## Task 15: Delete `MobileWheelPopup`

**Files:**
- Delete: `src/go/internal/ui/mobile_wheel_popup.go`, `mobile_wheel_render.go`, `mobile_wheel_popup_portal.go`, `step_notcher.go`, plus `mobile_wheel_popup_test.go`, `mobile_wheel_render_test.go`, `mobile_wheel_clicky_test.go`, `mobile_wheel_precision_test.go`, `mobile_wheel_resolution_test.go`, `mobile_wheel_popup_keys_test.go`, `mobile_wheel_value_test.go`, `wheel_ladder_test.go`, `wheel_row_click_test.go`, `mobile_knob_caption_dupe_test.go`, `synth_no_popup_test.go`
- Modify: `src/go/internal/ui/knob.go` (drop `NudgeEndless` if dead), `knob_wheel_nudge.go` (drop `nudgeKnobByWheel` if dead), `hover_coverage_test.go`, `game_input_shortcuts.go` (`handleEnter`'s wheel-popup authority)
- Create: `src/go/internal/ui/no_wheel_popup_discipline_test.go`

**Interfaces:**
- Consumes: `knob_format.go` (Task 1) already holds everything the synth needs.
- Produces: `func TestNoWheelPopupSymbolsRemain(t *testing.T)`.

- [ ] **Step 1: Write the discipline guard first**

Create `src/go/internal/ui/no_wheel_popup_discipline_test.go`:

```go
//go:build test

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoWheelPopupSymbolsRemain replaces synth_no_popup_test.go, which asserted
// the synth path opened no wheel popup. The popup type no longer exists at all:
// Synth and Sampler controls are armed inline barrels, and EQ is drag-only. This
// greps the package so a reintroduced wheel popup fails loudly rather than
// quietly re-adding a second, divergent editing model.
func TestNoWheelPopupSymbolsRemain(t *testing.T) {
	banned := []string{
		"MobileWheelPopup",
		"WheelBinding",
		"mobileWheelHitHandler",
		"mobileWheelPopupPortalOverlay",
		"openSynthKnobWheelPopup",
		"openSamplerKnobWheelPopup",
		"openEQKnobWheelPopup",
		"stepNotcher",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if name == filepath.Base("no_wheel_popup_discipline_test.go") {
			continue // this file names the banned symbols on purpose
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(b)
		for _, sym := range banned {
			if strings.Contains(src, sym) {
				t.Errorf("%s references retired wheel-popup symbol %q — the wheel model is gone; use the armed inline barrel controls", name, sym)
			}
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestNoWheelPopupSymbolsRemain'
```
Expected: FAIL, listing the wheel files.

- [ ] **Step 3: Delete the wheel implementation and its tests**

```bash
cd /home/ymolinar/Repos/beatmo/src/go/internal/ui
rm mobile_wheel_popup.go mobile_wheel_render.go mobile_wheel_popup_portal.go step_notcher.go
rm mobile_wheel_popup_test.go mobile_wheel_render_test.go mobile_wheel_clicky_test.go \
   mobile_wheel_precision_test.go mobile_wheel_resolution_test.go mobile_wheel_popup_keys_test.go \
   mobile_wheel_value_test.go wheel_ladder_test.go wheel_row_click_test.go \
   mobile_knob_caption_dupe_test.go synth_no_popup_test.go
```

- [ ] **Step 4: Build and remove whatever is now dead**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`

Fix each error in turn:
- `hover_coverage_test.go` — delete `TestMobileWheelPopupPortalOverlayIsPortalHoverer`. **Before deleting**, check whether any OTHER portal overlay lost its hover-seam guard as collateral; if the test was table-driven over several overlays, keep the table and drop only the wheel row.
- `game_input_shortcuts.go` — `handleEnter`'s doc comment says it exists for "the precision wheel popups". Run `grep -n "portalEnterHandler" *.go`; if no overlay implements it any more, delete `handleEnter` and its call site, otherwise just fix the comment.
- `knob_wheel_nudge.go` — if `nudgeKnobByWheel` has no callers left, delete it and the file if it becomes empty.
- `knob.go` — if `NudgeEndless` has no callers left, delete it. **Keep `StepValueDirect`** — both barrel bindings use it.

Re-run the build after each removal until clean.

- [ ] **Step 5: Run the discipline guard**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestNoWheelPopupSymbolsRemain'
```
Expected: PASS.

- [ ] **Step 6: Verify with grep too**

Run: `cd /home/ymolinar/Repos/beatmo && grep -rn "MobileWheel" src/go src/js`
Expected: no hits.

- [ ] **Step 7: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

---

## Task 16: Remove the `mobileWheel*` design tokens and update the docs

**Files:**
- Modify: `DESIGN.md` (lines ~636–640, ~735–739, ~834–838)
- Regenerate: `src/go/internal/ui/design_density.gen.go`
- Modify: `CLAUDE.md`, `AGENTS.md`, `AGENTS.synth.md` (whichever describe the wheel as the sampler/EQ control model)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing.

- [ ] **Step 1: Remove the five tokens from all three density blocks**

In `DESIGN.md`, delete these five lines from **each** of the `compact`, `comfortable`, and `spacious` blocks under `densities:`:

```yaml
    mobileWheelW:         ...
    mobileWheelH:         ...
    mobileWheelTickGap:    ...
    mobileWheelResStripW:  ...
    mobileWheelPillH:      ...
```

Leave `synthArmedCardH`, `synthArmedCardW`, `synthWheelLaneW`, `synthWheelRowH`, `synthWheelVisibleRows` in place — the armed barrel controls use all five.

- [ ] **Step 2: Regenerate the tokens**

Run: `cd /home/ymolinar/Repos/beatmo && make gen-design-tokens`

- [ ] **Step 3: Verify the generated file lost exactly the five fields**

Run: `cd /home/ymolinar/Repos/beatmo && grep -n "MobileWheel" src/go/internal/ui/design_density.gen.go`
Expected: no hits.

Run: `git diff --stat src/go/internal/ui/design_density.gen.go`
Expected: only removals (5 struct fields + 15 value lines). If the generator reordered or reformatted anything else, investigate before proceeding.

- [ ] **Step 4: Run the token guards**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/
```
Expected: PASS. `TestDesignMDLintSnapshot` may need its pinned warning set updated in `testdata/design_md_lint.expected.json` if removing the tokens changes the lint output — regenerate that snapshot only if the diff is clearly attributable to the removed keys.

- [ ] **Step 5: Update the prose docs**

Run: `cd /home/ymolinar/Repos/beatmo && grep -rn "wheel popup\|MobileWheelPopup\|value-wheel\|control wheel" CLAUDE.md AGENTS*.md DESIGN.md`

For each hit, rewrite the description to the armed inline barrel model. In particular:
- Any Sampler-tab description that says a knob tap opens a wheel popup.
- Any EQ-tab description that mentions a precision wheel or a dB numeric editor — EQ is now drag-only.
- The Synth-tab row of the per-tab table if it still mentions numeric entry.

- [ ] **Step 6: Full package check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

---

## Task 17: Whole-change verification

**Files:** none modified — this task only verifies.

- [ ] **Step 1: Full Go fast suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
Expected: PASS. Any failure must be traced to a specific change in this branch — never dismissed as pre-existing. If you believe a failure predates this work, prove it by stashing your changes and re-running that one test.

- [ ] **Step 2: Removal greps**

Run:
```bash
cd /home/ymolinar/Repos/beatmo
grep -rn "MobileWheel" src/go src/js ; echo "--- (want: empty)"
grep -rn "paramEditor" src/go/internal/ui | grep -v transportZone | grep -v "param_value_editor.go" ; echo "--- (want: empty)"
grep -rn "eqWheelPopup\|OnOpenValueWheel\|openEQDBEditor\|applyBandGainLive" src/go ; echo "--- (want: empty)"
grep -rn "samplerWheelPopup\|openSamplerParamEditor\|openSynthParamEditor\|openArmedNumericEditor" src/go ; echo "--- (want: empty)"
```
Expected: all four empty.

- [ ] **Step 3: Rebuild the screenshot binary**

Run:
```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a && \
  (cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_ss ./cmd)
```

- [ ] **Step 4: Capture the full regression set, desktop**

Run:
```bash
cd /home/ymolinar/Repos/beatmo
for s in crop_sampler_control_armed crop_sampler_control_armed_lastcol crop_sampler_tab_captured \
         crop_sampler_tab_empty crop_synth_control_armed; do
  xvfb-run -a /tmp/beatmo_ss -log ERROR -scene "$s" -screenshot "/tmp/v_$s.png"
done
xvfb-run -a /tmp/beatmo_ss -log ERROR -win-w 900 -win-h 1000 -scene sampler_arming_offwindow -screenshot /tmp/v_sampler_offwindow.png
```

Also capture an EQ-tab scene. Run `/tmp/beatmo_ss -list-scenes | grep -i eq` and pick the one that shows the dB readout row, then capture it to `/tmp/v_eq.png`.

- [ ] **Step 5: Capture mobile**

Run:
```bash
cd /home/ymolinar/Repos/beatmo
for s in crop_sampler_control_armed crop_sampler_control_armed_lastcol mobile_bottom_nav_sampler; do
  xvfb-run -a /tmp/beatmo_ss -log ERROR -mobile -win-w 390 -win-h 844 -scene-pass mobile -scene "$s" -screenshot "/tmp/vm_$s.png"
done
```

- [ ] **Step 6: OPEN and inspect every PNG**

Use the Read tool on each. Confirm:
- **Sampler armed (desktop + mobile):** card visibly grown; two readable barrel lanes with a gutter; param name present in the header; no blank controls.
- **Sampler armed, last column:** the block's RIGHT edge is the clicked cell; it grew leftward; nothing runs off-screen.
- **Sampler resting (`crop_sampler_tab_captured` / `_empty` / `mobile_bottom_nav_sampler`):** five ParamCards, group labels (TRIM/TUNE/LEVEL) intact, waveform + trim handles + toggles + footer unchanged from before this work. Compare against the committed baselines in `screenshots/` if present.
- **Mid-animation full-screen:** no stray square near the top-left origin; no card above its resting row; nothing painted over the waveform or the footer.
- **Synth armed:** unchanged, and visually identical in style to the sampler armed card.
- **EQ tab:** the per-band dB values still render as readouts; the curve and handles are unchanged; nothing looks missing where the interactive affordance used to be.

Fix and re-capture until every one is right.

- [ ] **Step 7: Browser bridge smoke (the JS export catalogue must be unaffected)**

Run:
```bash
cd /home/ymolinar/Repos/beatmo && GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
```
Expected: PASS. No JS export referenced the wheel popups, so this should be untouched — a failure means a Go export was removed as collateral.

- [ ] **Step 8: Report, do not commit**

Leave everything uncommitted. In your final report, list:
- Every file created, modified, and deleted.
- Every test deleted, with the invariant it pinned and where that invariant now lives (or an explicit statement that it is gone and why that is correct).
- The screenshots you opened and what you confirmed in each.
- Anything you could not verify.

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Seam 1 — `ArmedControlEditor` takes its bindings | 2 |
| Seam 2 — arm state and grow subject get a scope | 3, 7 |
| Seam 3 — `drawArmedControlCard` panel-agnostic | 4 |
| Seam 4 — desktop Sampler adopts `ControlGrid` | 5 |
| Sampler bindings (label parity with the card) | 6 |
| Behavior table (arm / lock / sweep / wheel / arrows / Esc / one-undo-step) | 8, 10 |
| Armed card rendering on the Sampler | 9 |
| Per-frame animation invariants | 11 |
| Visual verification (scenes, subjects, both platforms) | 11, 17 |
| Removals — Sampler | 12 |
| Removals — EQ drag-only | 13 |
| No legacy retained for tests (paramEditor apparatus + test re-pointing) | 14 |
| Removals — the wheel itself (+ relocate formatters) | 1, 15 |
| Removals — config (DESIGN.md tokens) + docs | 16 |
| Removal guards (greps, `TestDesignMDDrift`) | 15, 16, 17 |

No spec requirement is unassigned.

**Placeholder scan:** every code step carries real code; every run step carries a real command and an expected result. Three steps deliberately say "check first, then decide" (Task 14 Step 1's `renameComp` accessor, Task 15 Step 4's `handleEnter`, Task 12 Step 3's duplicate-test judgement) — each states exactly what to grep for and what each outcome implies, which is a decision procedure, not a placeholder.

**Type consistency:** `growArmedCardInto` is introduced in Task 7 Step 6 with the signature `(visible []int, grid *ControlGrid, innerR image.Rectangle, armedKIdx int, rectOf func(int) image.Rectangle, setRect func(int, image.Rectangle))` and both call sites in that same step match it. `newArmedControlEditor(instID, idx, valB, resB, onEdit)` is defined in Task 2 and called with that arity in Tasks 2 and 7. `drawArmedControlCard(dst, r, name, e, valB, resB)` is defined in Task 4 and called with that arity in Tasks 4 and 9. `setArmGrowSubject(scope, instID, idx, target)` and `armGrowControl(scope, instID)` are defined in Task 3 and used with the scope argument in Tasks 3, 7, and 11. `samplerKnobValueTextAt(idx, norm)`, `samplerStateValueAt(idx, norm)`, and `samplerKnobEditKey(idx)` are defined in Task 6 and used in Tasks 6 and 9.
