# Knob Interaction Overhaul — Design Spec

**Date:** 2026-06-13
**Branch:** add-core-node-types
**Status:** Approved design, pre-implementation

## Goal

Make Beatmo's rotary knobs feel like real hardware and give the user precise control:

1. **Clicky discrete knobs** — params with a fixed set of positions (enum + integer-step)
   snap to detents during the turn, with no in-between values.
2. **Endless, adjustable-resolution knobs** — every continuous knob becomes an endless
   encoder (drag never hits a travel wall) with a user-tappable per-knob step multiplier
   that controls how much each little turn changes the value.
3. **Manual numeric entry** — every numeric value across all tabs gets BPM-style
   click/tap-to-type editing with per-param validation and clamping.

## Background — current state (audited)

- `Knob` (`internal/ui/knob.go`) holds a normalized `Value` (0..1). Drag is
  **absolute from the press point**: `Value = pressVal + dxPixels/dragPixels`, clamped
  to [0,1]. `FineDrag` (Shift) widens the sweep (120px → 360px). A scroll wheel steps
  `knobWheelStep` (2.5%). **No snapping at the widget.**
- `ParamDef` (`internal/audio/synth_recipe.go:18-33`) carries `Min/Max/Default/Unit/Curve/
  Group/Enum`. `Enum != nil` marks a discrete param (stored as a float index; Min:0,
  Max:len-1). There is **no `Step` field** today.
- Enum params currently render as **continuous-sweep knobs**; the value only snaps to an
  integer index at *write time* (`synth_panel_zone.go:2019-2021`), so the **motion feels
  smooth, not clicky**. Integer params with no enum (`osc_octave` −2..2) are fully
  continuous with no snapping at all.
- Knobs exist only in the **Synth tab** (`synth_panel_zone.go`, per-`ParamDef`) and the
  **Sampler tab** (`sampler_panel_zone.go`, 5 hardcoded continuous knobs: Start, End,
  Transpose, Detune, Gain). **EQ and Chain tabs use `Slider`** (`slider.go`), not knobs.
- The knob value readout is drawn as a caption under the dial via `synthKnobCaption` →
  `enumLabelFor` / `formatSynthParamValue` (`synth_panel_zone.go:1727-1745, 1865-1887`).
- **BPM** is the only control with click-to-type numeric entry: `transport.go:54-98` +
  the reusable `TextInput` widget (`textinput.go`), with focus→clear→parse→commit-on-blur
  and revert-on-invalid.
- Synth knob edits are **config-only / next-trigger** with **no auto-audition** (a removed
  Phase-10 feature that caused crackle; pinned by `synth_no_auto_audition_test.go`).

## Scope (confirmed with user)

| Feature | Applies to |
|---|---|
| Clicky discrete | Synth knobs only (enum + `Step>0` params). Sampler has none. EQ/Chain "discrete" controls are already pills/buttons — untouched. |
| Endless + adjustable resolution | **All** continuous knobs (Synth + Sampler). |
| Numeric entry | **Every value, all tabs**: Synth knobs, Sampler knobs, EQ sliders, Chain sliders, and discrete params (type index or enum label). |

User decisions:
- Resolution control = **per-knob step badge + endless drag** (not modifier keys, not dual-ring).
- Step badge shows the step in **real units** (`10 Hz`, `5 ms`); abstract `×N` only when unitless.
- Step rung is keyed by **param name globally** (shared across instruments).
- Numeric-entry trigger = **tap the value readout** (dial drag still turns).
- Step rung **persists across restarts** in userprefs.
- No auto-audition: endless drag and numeric commit stay config-only / next-trigger.

## Architecture

### Component 1 — `Knob` behavior modes (`internal/ui/knob.go`)

Add opt-in fields; **defaults preserve today's behavior** so any bare `NewKnob` (and the
existing knob tests) is unchanged.

```go
type KnobScale struct {
    Min, Max float64
    Unit     string   // "", "ms", "Hz", "dB", "%", "st"
    Enum     []string // non-nil => discrete
    Step     float64  // >0 => discrete detents at Min, Min+Step, ... Max
}

type Knob struct {
    // ... existing fields (Value, Bipolar, ZeroFrac, FineDrag, ...) ...
    Scale    KnobScale
    Discrete bool    // = Scale.Enum != nil || Scale.Step > 0
    Endless  bool    // continuous knobs only; mutually exclusive with Discrete
    StepMul  float64 // current step in PARAM UNITS, used by endless drag + wheel
}
```

**Detent count** `N`:
- enum → `len(Enum)`
- else `Step>0` → `int((Max-Min)/Step) + 1`

**Discrete drag/wheel** (clicky): map gesture to the nearest detent, quantize before
storing so the indicator snaps and there is no in-between:
```
raw   = clamp(pressVal + dxPixels/dragPixels, 0, 1)   // existing absolute-from-press
idx   = round(raw * (N-1))
Value = idx / (N-1)
```
Wheel = ±1 index. Render `N` tick marks along the 270° arc; the indicator sits on a tick.

**Endless drag** (continuous): incremental accumulation so there's no travel wall, the arc
still fills proportionally so the user sees where they are in range:
```
onPress: lastX = pressX
onDrag:  d        = mx - lastX ; lastX = mx
         real     = Min + Value*(Max-Min)
         real     = clamp(real + (float64(d)/knobEndlessPxPerNotch)*StepMul, Min, Max)
         Value    = (real - Min) / (Max - Min)
```
Wheel = ±1 `StepMul`. (Incremental accumulation has no post-clamp dead-zone: dragging back
immediately moves `real` off the clamp — strictly better than absolute-from-press here.)

`ParamDef` gains an additive `Step float64 \`json:"step,omitempty"\`` field
(`internal/audio/synth_recipe.go`). `osc_octave` is marked `Step:1`; enum params are
implicitly `Step:1`. This is explicit, not a fragile "looks integer" heuristic, and
serializes identically to v1 when zero (no golden churn).

### Component 2 — per-knob step badge (`internal/ui/knob_step.go`, new)

A small tappable pill under each continuous knob showing the current step in real units
(`10 Hz`, `5 ms`, `0.1` for unitless). Tap cycles a 4–5 rung ladder.

- `stepLadder(def ParamDef) []float64` — derives rungs from range+unit: a **default rung
  giving ~200–1000 notches across the range** (nicely nuanced), with rungs stepping by
  decades around it, snapped to 1/2/5×10^k "nice" numbers. `defaultStepIndex(def)` selects
  the default rung.
- `KnobStepBadge` component: draws the pill, registers a touch-compliant hit area
  (`HitArea.Touch + ClipRect`) at the knob's z+1, cycles the rung on tap, writes the new
  `StepMul` to the knob, and persists it (Component 5).
- Display via `formatStepValue(def, step)` (compact; unit suffix or `×`).

### Component 3 — shared numeric editor (`internal/ui/param_value_editor.go`, new)

One `TextInput`-backed editor, rendered as an **overlay portal** so it floats over the
tapped readout without layout reflow. DrumView holds a **single active editor** (one field
edits at a time), targeting whichever readout was tapped.

```go
type ParamValueEditor struct {
    ti       *TextInput
    def      audio.ParamDef
    getValue func() float64
    setValue func(float64)   // clamps + writes to audio + syncs the control
    anchor   image.Rectangle
}
```

Shared helpers:
- `formatParamForEntry(def, v) string` — number in real units; enum → label.
- `parseParamEntry(def, text) (float64, bool)` — continuous: parse + clamp to [Min,Max];
  discrete: accept an index **or** case-insensitive enum-label match.

Flow mirrors BPM: focus → clear → live-parse → commit on Enter/blur, revert + error anim
on invalid. The **value readout becomes the tap target** (knob caption + slider `%` label).
Dragging the dial/track still turns; tapping the readout types. Mobile uses the soft
keyboard (`TextInput` already supports it).

### Component 4 — wiring per tab

- **Synth** (`synth_panel_zone.go`): at knob creation set `Scale` from the `ParamDef`,
  `Discrete = Enum!=nil || Step>0`, `Endless = !Discrete`, and `StepMul` from the persisted
  rung (or default). Render the step badge (continuous only) and make the readout a tap
  target → active `ParamValueEditor`. Keep the parallel slider mirror
  (`instEditorSliders[i].Value = knob.Value`). Edits remain config-only/next-trigger — **no
  auto-audition**.
- **Sampler** (`sampler_panel_zone.go`): same for its 5 continuous knobs (endless + badge +
  numeric entry).
- **EQ / Chain** (`eq_panel_zone.go`, `slider.go`, `chain_*`): sliders stay bounded (no
  endless/clicky); add readout → numeric entry only.

### Component 5 — persistence (`internal/userprefs/store.go`)

New `KnobStepStore`: `map[paramName]float64` (chosen step rung), behind a prefs version
bump (current latest is V5 per project memory → V6), with a marshal/round-trip test.
Keyed by `ParamDef.Name` globally (shared across instruments).

### Component 6 — design tokens

New dimension tokens in `DESIGN.md` (`densities:`): step-badge height/padding, detent tick
stroke width/length, numeric-editor box metrics. Run `make gen-design-tokens`; update the
`design_md_drift_test.go` want-map and the `token_discipline_test.go` budgets. **No new
colors** — reuse existing accent/secondary/surface tokens. Respect the forbidden-glyph rule
(use `IconID` + `DrawIcon`, single-char labels OK).

## Data flow

```
user gesture / type
   │
   ├─ drag dial ───────► Knob (discrete: snap to detent | endless: += StepMul) ─┐
   ├─ wheel ───────────► Knob (±1 detent | ±1 StepMul) ─────────────────────────┤
   ├─ tap step badge ──► KnobStepBadge cycles rung → Knob.StepMul + persist      │
   └─ tap readout ─────► ParamValueEditor (parse+clamp) ─────────────────────────┤
                                                                                 ▼
                                            actual = Scale.Min + Value*(Max-Min)
                                            (discrete: snap to int index)
                                                                                 ▼
                          audio.SetInstrumentParam / EQ / chain setter (config-only)
                                                                                 ▼
                                          parallel slider mirror + readout refresh
```

## Error handling

- Numeric parse failure / out-of-range → revert to previous value + error anim (BPM parity).
  Non-finite (NaN/±Inf) already rejected by `sanitizeParamValue`.
- Endless clamp at [Min,Max]; reverse drag immediately leaves the clamp (no dead-zone).
- Discrete index always rounded + clamped to [0, N-1].
- Step ladder always has ≥1 rung; `defaultStepIndex` clamps to a valid index.
- Persisted step that no longer maps to a valid rung → snap to nearest valid rung.

## Testing (TDD, functional-first)

Fast path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ ./internal/userprefs/`

**Unit**
- Detent snap: N positions, no in-between value reachable, tick count = N, indicator sits on
  a detent, wheel = ±1 index.
- Endless: incremental accumulation, clamp at both ends, reverse-off-clamp, wheel = ±1
  `StepMul`, arc fill proportional to value.
- Step ladder: rung count, default rung notch-count in [~200,1000], decade spacing, nice
  numbers; badge tap cycles and writes `StepMul`.
- `parseParamEntry`/`formatParamForEntry`: round-trip per unit, clamp, enum label + index,
  invalid → ok=false.

**Functional** (drive the real `Game.Update` input loop on a synth scene; reproduce-first,
per project convention):
- Tap a continuous knob's readout → type a value → commit → audio param updates.
- Drag an endless `filter_cutoff` knob → value traverses 20→20000 Hz.
- Tap the step badge → step rung cycles → subsequent drag moves by the new step.
- Drag a discrete `osc_type` knob → snaps between waveforms, no intermediate value.
- EQ/Chain slider readout → numeric entry commits in dB/value.

**Persistence**: userprefs `KnobStepStore` marshal/round-trip; invalid persisted rung snaps
to nearest.

**Regression guards**
- No immediate audio on knob edit / numeric commit (`synth_no_auto_audition_test.go` family).
- Default `NewKnob` (Discrete=false, Endless=false) keeps absolute-from-press behavior
  (existing `knob_test.go`).
- Token drift + discipline guards pass after token additions.
- Zone input isolation: badge + readout hit areas at z+1 don't leak; editor overlay portal
  doesn't steal input from siblings.

## Non-goals / trade-offs

- **No log-curve arc remapping** for wide params — the adjustable step multiplier solves the
  nuance need directly and is simpler.
- Step rungs keyed by **param name globally**, not per-instrument (easy to make
  per-instrument later).
- EQ/Chain sliders stay **bounded** (no endless/clicky) — only gain numeric entry.
- One active numeric editor at a time (no multi-field editing).

## Files touched

New: `internal/ui/knob_step.go`, `internal/ui/param_value_editor.go`, and their tests
(`knob_step_test.go`, `param_value_editor_test.go`, functional test file).

Changed: `internal/ui/knob.go`, `internal/audio/synth_recipe.go` (+ `Step` field),
`internal/audio/modular_recipe.go` (mark `osc_octave` `Step:1`),
`internal/ui/synth_panel_zone.go`, `internal/ui/sampler_panel_zone.go`,
`internal/ui/eq_panel_zone.go`, `internal/ui/slider.go`, chain slider call sites,
`internal/userprefs/store.go`, `DESIGN.md` (+ generated tokens),
`design_md_drift_test.go`, `token_discipline_test.go`, existing knob/slider/transport tests
as needed.
