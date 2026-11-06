# Controls & Interactive-Components Overhaul — Design v2 (2026-07-08)

Evidence: fresh full capture in `screenshots/review_2026_07_08/` (114 desktop +
51 mobile), reviewed via the curated basics set (`make screenshots-basic`),
plus code inspection of the widget layer (`knob.go`, `knob_step.go`,
`mobile_wheel_popup.go`, `mobile_wheel_render.go`, `slider_popup.go`,
`transport_zone.go`, `row_rack_zone.go`).

## Thesis — the problem is grammar, not pixels

Beatmo has ~7 different ways to "change a number" and ~5 different ways to
"turn a thing on":

| Operation | Today's affordances (all different) |
|---|---|
| Edit a number | BPM text box + micro-chevrons · node-sidebar −/+ keycaps · desktop knob drag/scroll · mobile wheel popup (gesture-only) · EQ dB invisible click-to-type text · FX horizontal sliders · volume slider popups |
| Toggle a thing | FX iOS-switch · sampler latched amber keycap (Norm) · synth stage pill-with-dot · mute/solo colored keycaps · AG/FIT latched chips |
| Choose one of N | audio-tab pills · bottom-nav keycaps · OVR/SPL/DIF segmented · Language segmented · subdiv dropdown |

Each is individually defensible; together they mean a user never transfers
learning from one surface to the next, and several controls expose only ONE
input method (gesture-only wheel, type-only BPM, drag-only knobs) — which is
exactly why the wheel and knobs feel underpowered. The ambition of this design:
**one value-editing grammar, one toggle taxonomy, one selection pattern**,
implemented as shared components and rolled out to every surface.

Non-negotiable interaction rule adopted throughout (user requirement):
**every continuous control offers BOTH gesture and buttons** — touch-scroll
/ drag for speed, discrete ± buttons for precision, type-in for exactness.
44px is the hit floor, not the visual size.

---

## P0 — Broken / defective (unchanged from v1, fix first)

1. **Synth channel-chooser dropdown renders without a panel** (native +
   browser: overlapping floating pills, chip strip bleeds through). Render via
   shared menu chrome (`drawMenuRow` in an opaque panel, `AnchorPopupRect`).
2. **Color-wheel title band not opaque** (row label ghosts through).
3. **Synth header thumbnail renders half-empty** (both platforms; A7 leftover,
   now reliably reproduced).
4. **Sampler LEVEL group (Gain) missing on mobile** (no third group, no scroll
   affordance).
5. **Five `Mobile:true` scenes capture identical default Pads views**
   (`transport_recording`, `mobile_track_chip_*`, `multi_row_full_grid`,
   `mobile_transport_bottom_bar`) — Setup doesn't apply on the mobile path.
   Add `MobileSetup`s + a manifest dedup guard (fail any Mobile scene whose
   capture is pixel-identical to `mobile_default`).
6. **Right-edge clipping** of axis/scale labels in mobile Levels ("80") and
   Chain/Wave ("11ms") — reserve measured text width.

---

## Workstream A — The Universal Value Editor

### A1. Wheel popup v2 (mobile numeric editor; also EQ's desktop editor)

Current state (code-verified): barrel drag/scroll moves the value; resolution
strip drag/scroll shifts the step rung; the strip's chevrons are **drawn but
not pressable** (`mobile_wheel_render.go:354-356`, no hit areas); commit is
Enter/tap-away, cancel is Esc — both invisible on touch; the center value box
is tap-to-type with no input affordance.

Target layout (within `MobileWheelW × MobileWheelH`, grown to ~240×380
spacious, still viewport-clamped):

```
┌──────────────────────────────┐
│  ✕   Cutoff — bright or dull  ✓ │  title band: cancel · name+gloss · accept
│ ┌────────────┐  ┌─────┐      │
│ │     ▲      │  │STEP │      │  ▲ value +1 step (auto-repeat)
│ │  8.10 kHz  │  │  ▲  │      │  res rail: finer
│ │ [8.00 kHz] │  │±50Hz│      │  step badge (pressable = cycle rungs)
│ │  7.95 kHz  │  │  ▼  │      │  res rail: coarser
│ │     ▼      │  └─────┘      │  ▼ value −1 step (auto-repeat)
│ └────────────┘               │
└──────────────────────────────┘
```

- **Value ▲/▼ buttons** capping the barrel column: one press = exactly one
  `StepMul` (same as one wheel notch / one barrel detent, via the existing
  `NudgeEndless(knobEndlessPxPerNotch)` path so grid-snap semantics are
  identical for continuous, endless, and enum params). Press-and-hold
  auto-repeats with acceleration (see A4). Enum/discrete params: next/prev
  option. Buttons disable (dim) at min/max.
- **Resolution ▲/▼ buttons**: the existing decorative chevrons become real
  buttons calling `AdjustResolution(±1)` + `Knob.StepMul` sync (same code the
  strip-drag uses), auto-repeating. A `STEP` micro-caption (PlainEnglish
  gloss) labels the rail; the badge stays drag/scrollable AND becomes
  pressable (tap = cycle to next rung, long-press = reset to the param's
  default rung). Hidden for discrete/enum params, and the barrel column takes
  the full width in that case.
- **Both gestures kept**: barrel flick keeps momentum + detent snap
  (add flick inertia — currently drag is 1:1 only); res strip keeps
  drag/scroll. Buttons and gestures drive the same notcher so the tick
  cadence/undo behavior is shared.
- **Explicit commit**: ✓ (accept) and ✕ (cancel) in the title band, 44px hits,
  wired to the existing accept/cancel transaction paths. Tap-away still
  accepts; Esc/✕ restores the snapshot.
- **Value box** gets the shared text-input affordance (boxed, caret on focus)
  and opens numeric type-in on tap (existing path, now visibly an input).
- **Default marker**: a subtle tick on the barrel at the param default; a
  small `RST`-style chip appears in the title band only when value ≠ default
  (one tap = reset to default, previewed, committed on accept).
- **Range rails**: min/max values ghost-rendered at the barrel's ends when
  within a screen of travel, so users see the bounds coming.

The EQ dB wheel (the one cross-platform user) inherits all of this on desktop
too — giving desktop its first precise stepper for EQ gains.

### A2. Knob overhaul — knobs become self-documenting (desktop parity)

Sampler knobs already carry the full grammar visually (amber value arc, name,
value, ±step badge). Synth knobs are near-invisible dark discs with scattered
captions, and desktop knobs expose no type-in, no reset, no fine mode.
Unify on one `KnobCell` renderer + interaction contract, used by Synth,
Sampler, and the FX drawer (replacing its bare sliders' inconsistency):

- **Render**: value arc from the default position (not from min — so the arc
  shows *deviation from default*, which is what musicians scan for), default
  tick, centered caption stack (name / value+unit / step badge), disabled
  (bypassed-stage) scrim raised to keep captions readable.
- **Gestures (desktop)**: drag = continuous; scroll = ±1 step (exists);
  **Shift+drag / Shift+scroll = fine** (StepMul ÷ 10, floor at the finest
  rung); **double-click = reset to default** (undoable, one commit);
  **click on the value text = type-in** via `ParamValueEditor`.
- **Buttons**: hover reveals compact ‹ › nudge chevrons flanking the value
  text (desktop only — mobile has the wheel; keycap-styled, auto-repeat).
  This is the knob-level answer to "buttons AND gestures" on desktop.
- **Mobile**: value pill unchanged as the wheel launcher, but gains a
  a long-press shortcut = reset-to-default (with the standard long-press
  progress ring), mirroring double-click on desktop.

### A3. One Stepper component

BPM's micro-chevron stack, the node sidebar's −/+ keycaps, the wheel's new
value/res buttons, and the knob hover-nudges are all the same widget. Extract
`Stepper` (orientation, size, auto-repeat policy, disabled-at-bound states)
and re-skin the call sites:

- **BPM (transport)**: replace the tiny two-arrow stack with −/+ Stepper cells
  matching transport button height (mobile already lays out −/BPM/+ as cells —
  desktop gets the same); box stays type-in.
- **Node sidebar rows**: keep −/value/+ arrangement, adopt Stepper (gains
  auto-repeat + bound-dimming for free); value boxes get the input affordance
  (they are editable-looking today but inert).
- **Subdiv**: stays a dropdown (it's a choice, not a number).

### A4. Auto-repeat & acceleration as design tokens

One policy for every press-and-hold control, declared in DESIGN.md
(`stepperRepeatDelayMs` 350, `stepperRepeatIntervalMs` 80, acceleration ×4
after 2s — exact values tuned once, shared): wheel value/res buttons, BPM,
node ±, knob nudges, zoom ±. Today each site hand-rolls or lacks repeat
entirely.

### A5. Sliders (volume popups, FX drawer)

- Double-tap (mobile) / double-click (desktop) thumb = reset to default.
- Value bubble above the thumb while dragging (the popup title already shows
  %, keep; FX drawer sliders show `name: value` inline — keep but format
  through the unit formatters so values never truncate: "4000…" → "4.0k").
- Per-row volume steps stay perceptual (±3 dB — already done); popup gains
  −/+ Stepper ends so volume also satisfies gesture+buttons.

### A6. Text inputs — one visible affordance

Boxed field + caret + focus ring everywhere text can be typed: BPM, rename,
search, Save As, EQ dB cells (today plain "0.0" text), wheel value box,
node-sidebar value boxes. On mobile every one of these opens the native
keyboard via the existing `MobileInputID` path. The rule: **if you can type
in it, it looks like an input; if it looks like an input, you can type in
it.**

---

## Workstream B — Toggle & selection taxonomy

Define three roles in DESIGN.md and re-map every control:

1. **Bypass switch** (signal flows or not): FX enable, synth stage enable,
   sampler Rev/Fade, EQ band mute, HP/LP. → One switch visual (the FX pill
   switch, sized by density). The synth stage "pill with tiny dot" and the
   sampler's latched keycaps for Rev/Fade migrate here. State must be legible
   at a glance (on = accent track, off = neutral) — fixes the mobile
   Rev/Norm/Fade "which is on?" problem.
2. **Mode latch** (a view/behavior stays engaged): AG, FIT, K20, Freeze,
   Norm(alize), mute/solo keycaps. → Latched keycap (amber = engaged), the
   existing signature style. Mute keeps its dedicated red fill per the
   three-reds rule.
3. **Exclusive choice**: audio tabs, bottom nav, OVR/SPL/DIF, Language,
   subdiv. → One `SegmentedControl` (already exists for view switching) with
   a single active treatment (amber pill), used by all five sites.

Deliverable: a one-page table in DESIGN.md mapping every toggle-like control
to its role, plus a discipline test that new `Button`s with latch behavior
declare a role.

---

## Workstream C — Mobile surface rebalance

(Keeps v1 items, now framed by the grammar work.)

1. **Transport**: uniform 44px cells, uniform IconSizeMD glyphs
   (play/stop/rec/lock), Record demoted by height only; volume + kebab as two
   clean cells (kill the stray "⋯"); beat counter restyled as passive text;
   BPM cell group becomes − / [120] / + (A3).
2. **Row rack**: control pills to 36px visual / 44 hit; volume micro-bar
   under the speaker replaced by a level tint on the icon (readout belongs to
   the popup); freed width goes to the row label (fixes "Shak…"/"Orga…").
3. **Synth/Sampler headers**: Preview + Save visible (content-sized), Save As
   + Reset into a header kebab; drop the "SAMPLER" title band; freed rows go
   to knob grid / waveform. Save keeps dirty-state Primary.
4. **FX rows**: reorder ▲▼ and delete ✕ to 36px visual/44 hit with SpaceSM
   separation, delete rightmost; drawer sliders adopt A5.
5. **Bottom sheets**: safe-area bottom inset in `drawBottomSheetPanel`;
   shared `MenuScroll` scrollbar whenever content overflows; color wheel
   becomes a bottom sheet (swatches ≥40px, family labels above rows, no
   clipping — fixes both platforms' truncated labels).
6. **Bottom nav**: caption-floor label size, distinct disabled treatment
   (Syn/Smpl under Master), amber = active only. Structural regrouping
   deliberately deferred.

## Workstream D — Popup system consistency

1. Every popup renders an opaque panel via shared chrome (P0.1/P0.2 are the
   two current escapes).
2. Desktop instrument menu anchors to its invoking row (not screen-left);
   categories gain trailing chevron + count.
3. One close-button size/placement (`closeButtonRect` + `CloseButtonSize`)
   across all popups; one destructive-row treatment (menu rows: red text +
   icon; standalone buttons: destructive fill) documented in the red-roles
   table.
4. Undo/redo icons raised to standard on-surface tone (desktop dark-on-dark).

---

## Multi-device robustness (mobile "multiple layouts" requirement)

- Extend the browser capture matrix beyond 390×844: add **360×640** (small
  Android floor) and **414×896** viewports for the basics set (`MOBILE_SIZES`
  env in `capture_browser_runner.mjs`; manifest records viewport).
- Wheel v2, bottom sheets, and the FX panel get layout tests at 360×640
  asserting: all wheel chrome visible (both button pairs + badge + ✓/✕),
  sheet last-row clearance ≥ safe inset, no clipped axis labels.
- The existing `AnchorPopupRect` clamp stays the single positioning
  primitive; new chrome must live inside the clamped rect (no absolute
  offsets).

## Testing approach

House rules: functional tests through real `Game.Update` + ebitenstub pixel
harness; `SetDensityForTest` for both densities; every interaction gains a
behavior test (auto-repeat cadence, bound-dimming, reset-to-default undo
grouping, fine-mode step math), every visual change a pinning test; wheel v2
gets a dedicated interaction suite (buttons and gestures asserted to produce
identical value sequences). Re-capture via `make screenshots-basic` after each
workstream. No commits without explicit approval.

## Implementation order

1. **P0.1–P0.6** (small, independent bug fixes)
2. **A1 wheel v2** (the flagship: buttons + gestures, ✓/✕, STEP rail, reset,
   flick inertia) + A4 tokens — immediately reused by EQ desktop
3. **A3 Stepper** + BPM/node-sidebar/volume-popup retrofit + **A6 inputs**
4. **A2 KnobCell** (synth knobs visible + full grammar; FX drawer adoption)
5. **B toggle taxonomy** (switch/latch/segmented re-map)
6. **C mobile rebalance** (transport → row rack → headers → FX → sheets → nav)
7. **D popup consistency** + multi-viewport matrix

Each stage lands with its tests and a before/after basics capture.
