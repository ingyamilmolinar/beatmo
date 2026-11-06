# Sampler armed inline controls + control-wheel removal — design

**Date:** 2026-07-27
**Branch:** `add-core-node-types`
**Status:** approved design, not yet implemented

## Goal

Make the **Sampler** tab's five param controls look and behave **exactly** like the
Synth tab's controls — the click-to-arm inline two-lane inertial barrel model — and
then **delete** the legacy `MobileWheelPopup` ("control wheel") entirely, including
its design-token config.

Deleting the wheel requires removing its second consumer, the **EQ** tab's per-band
dB wheel. Per user decision the EQ tab loses *all* direct value entry: it becomes
**drag-only**, controlled solely by its draggable curve handles.

## Background — the two models

**Synth (target).** `synthControlHitAdapter` routes a per-card gesture through the
armed model:

- **Not armed** — a stationary tap ARMS the card on release; a vertical sweep or a
  wheel notch scrolls the stage's `ControlGrid` instead (never a value edit).
- **Arming** — the card grows in place into a ≥2×2 block over ~7 frames
  (`armGrow*` eased progress → `growArmedCard` → `reflowArmedAnchored`), displacing
  only same-column neighbours downward. It renders two kinetic `BarrelWheel` lanes:
  **left = resolution** ladder, **right = value** (`ArmedControlEditor`,
  `drawBarrelLane`).
- **Armed** — press/drag/wheel spin whichever lane the pointer is over; a stationary
  click *anywhere* on the card LOCKS it; ↑↓ steps value, ←→ steps resolution; Esc
  locks via the `handleEscape` rung.
- Manual numeric text entry is deliberately **disabled**.

**Sampler (current).** Cards already render through `drawParamCard`, so the resting
look already matches. Only the interaction differs: `samplerKnobHitAdapter.OnRelease`
calls `openSamplerKnobWheelPopup` → a modal `MobileWheelPopup` portal with a scrim;
`OnWheel` nudges the value in place via `nudgeKnobByWheel`.

**EQ (current).** `eqDBOpenAdapter.OnPress` → `OnOpenValueWheel` callback →
`openEQKnobWheelPopup`, with a fallback to the inline numeric editor
`openEQDBEditor` when the callback is nil.

## Non-goals

- No change to the Synth tab's **behavior**. The generalization below must be
  behavior-preserving there; every existing synth armed-control test stays green
  unchanged.
- No change to the Sampler's waveform, trim handles, transform toggles, footer
  actions, group labels (TRIM/TUNE/LEVEL), or metadata strip.
- No change to the EQ curve, handles, HPF/LPF, mute buttons, or any other tab.
- No new density tokens. The sampler reuses the existing `synthArmedCard{W,H}`,
  `synthWheelLaneW`, `synthWheelRowH`, `synthWheelVisibleRows`.

## Architecture

The armed model is already almost entirely generic. This is a
**generalize-then-adopt**, not a re-implementation — which is what makes the two
tabs behaviorally identical by construction rather than by parallel maintenance.

### Reused unchanged

`BarrelWheel` (already speaks the `barrelBinding` interface), `drawBarrelLane`,
`reflowArmed` / `reflowArmedAnchored`, `growArmedCard` (+ `growArmedCardLegacy`),
the `armGrow*` animation, `drawParamCard`, `ControlGrid`, `synthWheelNudge`'s
sibling `samplerWheelNudge`.

### Seam 1 — `ArmedControlEditor` takes its bindings

`newArmedControlEditor(dv, instID, idx)` hard-wires `synthValueBinding` /
`synthResolutionBinding` and a synth-specific `onEdit`. Widen to:

```go
func newArmedControlEditor(valB barrelBinding, resB barrelBinding, detentPx int, onEdit func()) *ArmedControlEditor
```

with two thin factories:

- `newSynthArmedControlEditor(dv, instID, idx)` — today's bindings + today's
  `onEdit` (sync slider → propagate → `synthWheelNudge.bump(commit + mirror)` →
  `eqPanelZone.Invalidate()`).
- `newSamplerArmedControlEditor(dv, idx)` — new `samplerValueBinding` /
  `samplerResolutionBinding` + a sampler `onEdit` (`applySamplerKnob(idx)` →
  `samplerWheelNudge.bump(commitSamplerEdit)` → `Invalidate()`).

`ArmedControlEditor` keeps `instID`/`idx` only for the reconcile check in
`buildSynthTab`; the sampler passes `""`/`idx`.

**New sampler bindings** (`sampler_barrel_bindings.go`), mirroring the synth pair
and resolving the `*Knob` / `*KnobStepBadge` **by index every call** (never a cached
pointer — a re-layout must stay safe):

- `samplerValueBinding{dv, idx}` — `labelAt` via `formatSynthParamValueStep(def, real, step)`
  where `def = dv.samplerKnobDef(idx)`; `stepBy` via `Knob.StepValueDirect`;
  `canStep` via the knob's 0..1 clamp. All five sampler params are continuous, so
  the enum branch is not needed.
- `samplerResolutionBinding{dv, idx}` — `badge()` from `s.knobStepBadges[idx]`,
  `stepBy` inverting `AdjustResolution` (+1 detent = coarser) and persisting via
  `persistKnobStep(badge)`, exactly as the synth binding does.

### Seam 2 — arm state and grow subject get a scope

`dv.instEditorArmedControl map[string]int` and the single grow subject
(`armGrowIdx` / `armGrowInst` / `armGrowT` / `armGrowTarget`) are keyed by
instrument id only. The Sampler and Synth tabs edit the **same instrument id** with
**different index spaces**, so unscoped state would let a synth arm leak into the
sampler's grid (and vice versa) and would let one tab's grow animation drive the
other's reflow.

Introduce:

```go
type armedScope int
const (armedScopeSynth armedScope = iota; armedScopeSampler)
```

- The arm map becomes `map[armedScope]map[string]int` (or two fields — chosen at
  implementation time, whichever reads cleaner; the observable contract is that the
  two scopes are independent).
- The grow subject gains an `armGrowScope armedScope` field; `setArmGrowSubject`,
  `armGrowControl`, and `advanceArmGrow` take/compare it.
- `dv.instEditorArmedEditor` gains a sibling `dv.samplerArmedEditor`.
- `synthArmedActive()` generalizes to `armedControlActive()` returning true when
  *either* scope is armed on its own active tab (`TabSynth` for synth,
  `TabSampler` for sampler). Both `KeyboardClaimed()` (`keyboard_focus.go`) and
  the `handleEscape` rung (`game_input_shortcuts.go`) switch to it, so arrow keys
  never also pan the camera and Esc never falls through to `TriggerStop`.

Only one audio-panel tab is active at a time, so at most one scope is ever
*interactive*; the scope exists to keep their **state** from aliasing.

### Seam 3 — `drawArmedControlCard` becomes panel-agnostic

Today it is a `*synthPanel` method reading `dv.instEditorArmedEditor` and building
the synth bindings inline. Lift to package level:

```go
func drawArmedControlCard(dst *ebiten.Image, r image.Rectangle, name string,
    e *ArmedControlEditor, valB, resB barrelBinding)
```

Both panels call it. The chrome (flat body + accent selection bar + header band +
two lanes + gutter hairline) is unchanged, so the armed card is pixel-identical
across tabs at equal size.

### Seam 4 — the desktop Sampler adopts `ControlGrid`

`growArmedCard` needs a `ControlGrid` for its cell lattice (`Cols`, `CellAt`,
`CellAtUnclipped`, `SetContentRows`, `ClipBottom`) and returns early when the grid
is nil. The desktop sampler currently hand-rolls its layout
(`cellW := ctrlR.Dx() / samplerKnobCount`, no grid); mobile already uses
`layoutSamplerKnobsScrolling`.

Collapse both into the one grid path. The cell formula is **already identical**
(`labelBandH + ParamCardH + SpaceXS`); the only divergence is the column cap —
`SetMaxCols(4)` on mobile, `SetMaxCols(samplerKnobCount)` on desktop so five cards
sit in one row as they do today. Desktop keeps its card-width clamp
(`min(cellW - SpaceSM, ParamCardW*3/2)`, floored at `ParamCardW/2`) applied to the
grid slot, centred in the cell.

This is the change that makes the grow/reflow behavior *inherently* the same on
both platforms and both tabs rather than a second implementation to keep in sync.

## Behavior table — Sampler after the change

| Gesture | Not armed | Armed (this card) |
|---|---|---|
| Stationary tap / click | ARMS on release (grow animation starts) | LOCKS (any point on the card, including the value readout band) |
| Vertical drag | scrolls the sampler `ControlGrid` (tap-vs-scroll via `dragScrollArbiter`) | spins the lane under the pointer x (`laneAt`); ≥ `tapMaxMovePx` of travel sets `armedMoved` so the release does not lock |
| Wheel notch | scrolls the `ControlGrid` | `ImpulseNotch` on the hovered lane, kinetic |
| ↑ / ↓ | — | `StepValue(±1)` (immediate) |
| ← / → | — | `StepResolution(±1)` (immediate) |
| Esc | — | locks, via the `handleEscape` rung (must NOT stop playback) |
| Tap another card | arms that one | locks this, arms that one |
| Numeric text entry | **removed** | **removed** |

The armed-vs-drag lock test uses **cumulative pointer displacement** from the press
point versus `tapMaxMovePx`, *not* `DragTo`'s detent-crossed return — a sub-detent
drag (12–20 px, past the 10 px tap slop but under the 22–32 px detent) must spin,
not lock. This is the same rule and the same `armedStartX/Y` + `armedMoved` fields
the synth adapter carries.

Undo remains **one step per gesture**: the barrel's `onEdit` fires
`samplerWheelNudge.bump(commitSamplerEdit)` per detent, and the debouncer coalesces
the whole spin. A tab switch or view switch must flush a pending commit (the
existing `wheelNudger` flush contract).

## Removals

### Sampler

- `sampler_wheel_popup.go` (`openSamplerKnobWheelPopup`, `closeSamplerKnobWheelPopup`,
  `SamplerWheelPopupRect`, `samplerMobileKnobButtonRect` if unused).
- `dv.samplerWheelPopup` field + its ctor init; `open/closeSamplerWheelPortal`
  (portal id `"sampler-wheel-popup"`) and the forwarding shims.
- `samplerKnobHitAdapter.OnWheel`'s `nudgeKnobByWheel` value-nudge path (becomes a
  grid scroll).
- `openSamplerParamEditor` + `samplerKnobReadoutRect` + `s.readoutRects` (numeric
  entry is disabled for exact synth parity — see § No legacy retained for tests).
- `sampler_wheel_popup_test.go`; the wheel-specific cases in `sampler_card_test.go`,
  `sampler_control_scroll_drag_test.go`, `sampler_knob_interaction_test.go`,
  `undo_sampler_test.go` — **migrated to the armed path, not deleted**, so the
  invariants they pin (one undo step per gesture, no auto-audition, modal
  isolation, scroll-vs-tap arbitration) survive.

### EQ — drag-only

- `eq_wheel_popup.go` entirely; `dv.eqWheelPopup`, `dv.eqWheelStepBadge`,
  `dv.eqWheelStepRestored`; `open/closeEQWheelPortal` (portal id `"eq-wheel-popup"`);
  `EQWheelPopupRect`; the `closeEQKnobWheelPopup()` call in `CloseAllPopups`.
- `EQCallbacks.OnOpenValueWheel` + its `drumview_ctor.go` wiring.
- `applyBandGainLive` — becomes dead. (The curve-drag path fires
  `callbacks.OnGainChange` directly at `eq_panel_zone.go:1991`; only the wheel
  used `applyBandGainLive`.)
- `eqDBOpenAdapter` (+ its `glowRect`), the `eq-db-*` hit-area loop,
  `openEQDBEditor`, `eqDBSpec`, and `EQPanelZone.paramEditor` (its ctor init,
  `Update`/`Draw` pumps, and the `Active()` input gates in `eq_panel_zone.go` and
  `drumview_tree.go:365`). Grep confirms `openEQDBEditor` was the only
  `z.paramEditor.OpenValue` caller. See § No legacy retained for tests.
- `eqDBMin` / `eqDBMax` / `eqDBSpan` move to wherever the curve clamp needs them, or
  go away if the curve already has its own bounds.
- Stale orphan doc comment for the long-deleted `stepSelectedBand`
  (`eq_panel_zone.go:1272`).
- `eq_db_input_test.go` is **inverted**, not deleted: it now asserts there are
  **zero** `eq-db-*` hit areas and that a press on a dB readout cell opens no
  editor. `eq_wheel_popup_test.go`, `eq_wheel_popup_isolation_test.go`,
  `eq_wheel_seam_test.go` are deleted (their subject is gone).
- The dB readout cells remain as **read-only** value labels — they still show each
  band's gain, they are just not interactive.

### The wheel itself

- **Relocate first:** `formatKnobDisplay`, `formatKnobDisplayStep`, and
  `knobPillLabel` live in `mobile_wheel_render.go` but are used by the **synth**
  (`synth_panel_zone.go:3165`, `synth_macro_render.go:271`). Move them to a new
  `knob_format.go` **before** deleting the file.
- Delete `mobile_wheel_popup.go`, `mobile_wheel_render.go` (incl.
  `wheelCylinderSprite`, `wheelGripSprite`), `mobile_wheel_popup_portal.go`
  (`mobileWheelPopupPortalOverlay`, `mobileWheelHitHandler`,
  `mobileWheelAnchorCloseHandler`), `WheelBinding`.
- Delete `step_notcher.go` (`stepNotcher`, `applyKnobSteps`,
  `wheelStepEventsPerNotch`) and `knob_wheel_nudge.go`'s `nudgeKnobByWheel` once
  their last callers are gone. `Knob.NudgeEndless` likewise. `Knob.StepValueDirect`
  **stays** — the barrel bindings use it.
- Delete the wheel test files: `mobile_wheel_popup_test.go`,
  `mobile_wheel_render_test.go`, `mobile_wheel_clicky_test.go`,
  `mobile_wheel_precision_test.go`, `mobile_wheel_resolution_test.go`,
  `mobile_wheel_popup_keys_test.go`, `mobile_wheel_value_test.go`,
  `wheel_ladder_test.go`, `wheel_row_click_test.go`,
  `mobile_knob_caption_dupe_test.go`. `hover_coverage_test.go`'s
  `TestMobileWheelPopupPortalOverlayIsPortalHoverer` is deleted; verify no other
  portal overlay loses its hover-seam guard.

## No legacy retained for tests

**Rule: nothing survives solely to keep a test compiling.** A test whose subject is
deleted is either re-pointed at a real production surface or deleted with its
subject. Two consequences, both larger than they first look.

### The audio panel loses numeric text entry entirely

The decisions above (Sampler = exact synth parity → no numeric entry; EQ =
drag-only) mean **no tab of the audio panel has a numeric text editor**. Grep
confirms `DrumView.paramEditor` is *opened* from exactly two places —
`openSynthParamEditor` and `openSamplerParamEditor` — and every other reference is
an `Update`/`Draw` pump or an `Active()` gate. So the whole apparatus is dead, not
merely unused:

- `openSynthParamEditor` (both the `*synthPanel` method and the `*DrumView`
  forwarder) — already reachable **only from tests**; no production call site exists.
- `openArmedNumericEditor` and `TestArmedNumericEditorHelperRetained` /
  `synth_armed_numeric_test.go`'s retention case — the textbook "kept alive by a
  test" pattern. Both go.
- `openSamplerParamEditor` + the `*DrumView` forwarder.
- `DrumView.paramEditor` itself: the field, its lazy inits, the `Update` pumps in
  `synthTabUpdate` / `samplerTabUpdate`, the `Draw` pumps in `synth_panel_zone.go`
  and `sampler_panel_zone.go`, and the `Active()` early-return gates in
  `synthControlHitAdapter.OnPress/OnWheel` and `samplerKnobHitAdapter.OnPress/OnWheel`
  (with no editor there is nothing to swallow a tap for).
- `EQPanelZone.paramEditor` (see EQ removals above).
- `game_input_shortcuts.go`'s Esc/arrow rungs for `dv.paramEditor` and
  `dv.eqPanelZone.paramEditor`.
- `activeParamEditorNativeRects` + its `kept = append(...)` call in
  `syncNativeRects`, plus `TestParamEditorNativeRectArmedBySync`. If the
  native-gesture discipline test carries a `param_value_editor.go` exemption, that
  exemption must be **removed** too (these guards ratchet — a stale exemption is
  itself drift).

The `ParamValueEditor` **type stays**: the transport BPM box
(`transportZone.paramEditor`) is a real, wired production user of it.

### Test re-pointing

Numeric and alphanumeric text-input coverage moves onto **already-wired production
surfaces**, which the same two files already exercise in sibling tests — so this is
re-pointing at proven harness code, not inventing a new fixture:

| Coverage | New vehicle |
|---|---|
| Numeric input: caret ←/→, Esc-cancels, Enter-commits, soft-keyboard proxy forwarding | **BPM box** — `g.drum.bpmBox()` → `transportZone.paramEditor` (see the existing `TestFunctional_Esc_BPMBox`, `..._BPMBox_Commits`, `..._SoftKeyboardProxy`) |
| Alphanumeric input: typing, Esc-cancels | **Instrument rename** — `rowRackZone.callbacks.OnRenameOpen(0)` → `g.drum.renameComp` (see the existing `TestFunctional_Esc_Rename`) |

- `arrow_key_navigation_functional_test.go`: `TestFunctional_ArrowKeys_EQdBEditor`
  and `TestFunctional_SoftKeyboardArrowMovesCaret` re-point at the BPM box
  (numeric caret + soft-keyboard proxy). Add an alphanumeric caret case on the
  rename field, since a text field is where ←/→ navigation actually matters to a
  user. `TestFunctional_ArrowKeys_SynthControlEditor` is **deleted** — its subject
  (a synth numeric editor) no longer exists; the armed control's ↑↓/←→ coverage
  lives in the sampler/synth armed keyboard tests instead.
- `text_input_escape_functional_test.go`: `TestFunctional_Esc_EQdBInput` is
  **deleted** (subject gone). The BPM, rename, and instrument-search Esc cases are
  untouched and already carry the Esc contract. `fakeInput` and `newDesktopGame`
  live in this file and are used package-wide — they stay.
- `knob_interaction_functional_test.go`: `TestSynthControlCaptionIsDisplayOnly`
  deleted (it exists to prove the adapter swallows taps *while an editor is
  active*); `TestSynthNumericCommitDoesNotAudition` deleted — verify the armed
  path's no-auto-audition assertion covers the same invariant first, and write it
  if it does not.
- `undo_audio_panel_test.go`: `TestUndoSynthNumericEntry` deleted; the
  one-undo-step-per-edit invariant is already pinned by
  `TestSynthControlWheelRecordsExactlyOneUndoStep` in the same file.

**Verification for this section:** `grep -rn "paramEditor" src/go/internal/ui` must
return only `transportZone`-scoped hits and `param_value_editor.go` itself.
- `synth_no_popup_test.go` (asserts the synth path opens no wheel) is superseded by
  the type no longer existing. Replace it with a single discipline test asserting
  no `internal/ui` source references a wheel-popup symbol, so a future
  reintroduction fails loudly.

### Config

- DESIGN.md: remove `mobileWheelW`, `mobileWheelH`, `mobileWheelTickGap`,
  `mobileWheelResStripW`, `mobileWheelPillH` from all three `densities:` blocks
  (compact / comfortable / spacious).
- `make gen-design-tokens` to regenerate `internal/ui/design_density.gen.go`.
- Drop `TestMobileWheelDensityTokensPresent` (`mobile_wheel_value_test.go`).
- Update the `AGENTS*.md` / `CLAUDE.md` prose that describes the wheel popup as the
  sampler/EQ control model.

## Testing

Following the project convention of **functional tests driving real `Game.Update`**
over isolated unit calls, and the discipline that a gesture is tested through the
tree (`OnPress` → `OnDrag` → `OnRelease`), never by calling a handler's internals.

**Sampler — new / migrated (`internal/ui`):**

- Tap a card → armed; `samplerArmedEditor != nil`; grow animation runs and settles.
- Tap again → locked; shrink animation runs; neighbours return to rest.
- Vertical sweep on a card → grid scrolls, card does **not** arm.
- Not-armed wheel over a card → grid scrolls, value unchanged.
- Armed wheel over the value lane → value changes; over the res lane → rung changes
  and persists.
- Armed drag of 12–20 px (sub-detent, past tap slop) → spins, does **not** lock.
- Arrow keys ↑↓ / ←→ while armed → value / resolution step; camera does not pan.
- Esc while armed → locks and **does not** stop playback.
- One undo step per spin gesture; a tab switch flushes a pending commit.
- No auto-audition on a sampler param edit (sampler edits apply on Save/Preview).
- Desktop and mobile both exercised (`withSmallScreen` **after** the game ctor —
  the ctor's `assertDefaultParityState` requires the flag false).
- Switching the active instrument while armed reconciles/clears the editor (the
  synth's instrument-switch-freeze bug class).

**Per-frame animation invariants** — extend the existing
`synth_armed_anim_invariants_test.go` matrix to cover the sampler grid across
several window sizes, including a **single-visible-row** grid. Assert per frame,
not just settled: never escapes the grid column, nothing moves up, an
off-window-at-rest control stays hidden, never paints outside the sampler control
column, never moves sideways, neighbours never overlap, never paints below the fold.
This is where the `CellAt`-empty-rect-is-both-a-sentinel-and-the-screen-origin bug
class lives, and settled-only assertions are blind to it.

**EQ:** zero `eq-db-*` hit areas; a press on a dB readout cell opens no editor and
mutates nothing; curve-handle drag and HPF/LPF still work and still commit one undo
step.

**Removal guards:** the package compiles with no reference to `MobileWheelPopup`;
`grep -r MobileWheel src/go` is empty; `grep -rn paramEditor src/go/internal/ui`
returns only `transportZone`-scoped hits and `param_value_editor.go`;
`TestDesignMDDrift` passes after the token removal.

**Re-pointed text-input coverage** (§ No legacy retained for tests): BPM box —
caret ←/→ moves without panning the grid, Esc cancels, Enter commits,
`SignalSoftKeyboardArrowLeft/Right` + `SignalSoftKeyboardEscape` are honoured with
no canvas key. Instrument rename — typing registers, caret ←/→ moves, Esc cancels
and leaves the name unchanged.

## Visual verification (mandatory)

Per the project's UI rule, green tests are not sufficient. New crop scenes in
`scene_catalog.go`:

- `crop_sampler_control_armed` — a sampler card armed, both lanes readable. Must
  arm an **on-window** control (non-empty `Rect()`), or the crop is invisible.
- `crop_sampler_control_armed_lastcol` — the rightmost card armed, to prove the
  right-edge clamp keeps the clicked cell as the block's right column.
- `sampler_arming_offwindow` — deliberately **full-screen**, not cropped: an
  origin-flight regression leaves the panel entirely, so a crop cannot show it.
- Mobile variants via `MobileSetup` (a Setup-only scene silently falls back to an
  invisible panel under `runSceneMobile`).
- An EQ tab scene re-captured to confirm the dB cells still render their values and
  nothing is visually missing where the interactive affordance used to be.

Capture desktop with `xvfb-run -a /tmp/beatmo_ss -scene <name> -screenshot out.png`
and mobile with `-mobile -win-w 390 -win-h 844 -scene-pass mobile` (the native
binary needs `-mobile`; the `MOBILE=1` env var does nothing to the binary). **Open
every PNG and inspect it**; re-capture until it is right.

## Risks

| Risk | Mitigation |
|---|---|
| Desktop sampler layout regresses when it moves onto `ControlGrid` | The cell formula is already shared; pin the desktop 5-in-a-row result with a layout test and a re-captured `crop_sampler_tab_captured` screenshot. |
| Scope aliasing between synth and sampler arm state | The `armedScope` key, plus a test that arming a synth control leaves the sampler unarmed for the same instrument id and vice versa. |
| Mid-animation origin-flight (the sentinel-as-geometry bug class) | Extend the per-frame invariant matrix to the sampler grid, including a single-visible-row window. |
| Deleting `mobile_wheel_render.go` breaks the synth | Relocate `knobPillLabel` / `formatKnobDisplay*` to `knob_format.go` as the first step of the removal. |
| Losing arrow-key / Esc / soft-keyboard-proxy coverage that used the EQ wheel as its vehicle | Re-point onto the BPM box (numeric) and instrument rename (alphanumeric) — both already-wired production text inputs exercised by sibling tests in the same files. See § No legacy retained for tests. |
| EQ users lose the only way to type an exact dB value | Explicit user decision: EQ is drag-only. The dB cells remain as read-only readouts. |
| The audio panel loses numeric entry on **all** tabs, which is wider than "migrate the sampler" | Called out explicitly in § No legacy retained for tests. It is the direct consequence of the two approved decisions plus the no-legacy-for-tests rule; every removed opener was already unreachable from production. Flag it at review, not after. |
