# ParamCard + universal value-wheel popup — Synth/Sampler control redesign

**Date**: 2026-07-16
**Status**: Approved by user (approach + all design sections)
**Branch**: add-core-node-types (uncommitted, per project convention)

## Problem

The Synth and Sampler audio tabs use rotary dial knobs on desktop and a
value-pill + `MobileWheelPopup` on mobile. Problems:

- **Inconsistent rules.** Desktop sampler: wheel turns the dial, readout tap
  opens a numeric editor. Desktop synth: wheel scrolls the section, caption is
  inert, *horizontal* two-finger changes value. Mobile: tap-pill-to-open-popup.
  Three interaction models for the same concept.
- **Wasted space.** Desktop synth fits **2 knobs per row** in a ~950px pane
  (`SetMaxCols(2)`); name, value, resolution badge, and PlainEnglish gloss are
  four scattered fragments per control.
- **Hidden gestures.** Horizontal-only knob drag, horizontal wheel = value,
  vertical wheel = scroll — none discoverable. Knobs are explicitly hover-blind
  (`hover_glow_overlay.go` exempts drag-only surfaces).
- **Low-affordance Advanced expander.** Tiny per-stage `> Advanced` bar,
  per-instrument hidden state (`dv.synthAdvExpanded`).
- **Ugly/inconsistent.** Dials don't match the keycap/pill design language used
  everywhere else in the game.

## Decisions (user-approved)

1. **Remove rotary knobs on ALL platforms**; unify synth + sampler around
   tap-to-open popup controls (the existing `MobileWheelPopup`, which the EQ
   tab already runs on desktop).
2. **Desktop quick-adjust**: scroll-wheel over the closed control nudges the
   value one resolution step per notch without opening the popup. Click always
   opens the popup. No drag-on-card value gesture.
3. **Closed control = unified ParamCard**: name + bold value/unit + range
   gauge in one compact tappable card. Kills the separate caption band and
   floating step badge.
4. **Resolution UX = visible rung ladder** in the popup: all 1-2-5 ladder
   rungs rendered as tappable pills, active rung highlighted; scroll over the
   ladder steps it; Shift+scroll on desktop = one rung finer temporarily.
5. **Advanced/basic**: no toggle. Essential params first, then a labeled
   non-interactive `— ADVANCED · N —` divider row, then advanced cards.
   Scrolling is only ever needed for advanced controls.
6. **Hover highlight on every clickable control** via the sanctioned one-spring
   hover doctrine (glowTarget / portalHoverer), own-accent colors.

## §1 ParamCard anatomy

New widget `ParamCard` (new file `param_card.go`), replacing the dial cell in
`synth_panel_zone.go` and `sampler_panel_zone.go` on all platforms. Keycap
family styling: raised cap + bevel + contact shadow via the existing
`drawRoundedButton`/`keycapCapRect`/`drawKeycapBevel` primitives; accent rim.

```
┌──────────────────┐
│ CUTOFF           │  name: RoleMicro, dim, uppercase, ellipsized
│ 8.00 kHz       ⌄ │  value+unit: bold RoleBody, resolution-aware precision
│ ▓▓▓▓▓▓▓░░░░░░░░░ │  gauge strip (ParamCardGaugeH px)
└──────────────────┘
```

- **Gauge variants**: continuous → left-fill proportional to normalized value;
  bipolar (`Knob.Bipolar`) → fill from `ZeroFrac` center outward; enum ≤ 8
  items → position dots (filled = current index); enum > 8 → index-proportional
  fill. Rendered with theme tokens, no new colors.
- **Chevron**: small caret icon (`DrawIcon` chevron, pointing down = "opens a
  popup") at the value row's right edge — the "this opens" affordance
  (replaces the pill's caret).
- **States**: idle; hover (desktop: own-accent glow, `glowTarget` +
  `button-hover-fade` spring); pressed (cap sinks, press-driven lift on
  mobile); selected (accent border ring — card selection still drives the
  focus-graph via `setSynthSelectedKnob`); disabled-stage (dim scrim as today,
  cards stay interactive).
- **Value label**: `knobPillLabel(k, def)` machinery (resolution-aware via
  `knobStepResolution` + `formatKnobDisplayStep`); enum shows the localized
  enum label.
- The dial, caption band, floating `KnobStepBadge` pill, and in-grid
  PlainEnglish gloss are all removed from the grid. `Knob` survives strictly as
  the headless value model.

## §2 Grid, density, Advanced divider

- **New density tokens** (DESIGN.md `densities:` ×3 + `make gen-design-tokens`):
  - `paramCardW`: 128 / 148 / 168 (compact/comfortable/spacious)
  - `paramCardH`: 48 / 56 / 64 (≥44px touch-min at spacious=mobile)
  - `paramCardGaugeH`: 3 / 4 / 5
- **Desktop density**: remove the `SetMaxCols(2)` cap in
  `placeKnobsInSection`; `ControlGrid` flows as many cards as fit
  (~5–6/row at comfortable in the ~950px pane). Mobile keeps width-adaptive
  flow (~2/row in the left column); each cell is roughly half the old
  pill+caption height.
- **Cell height** = `ParamCardH` (+ existing vGap). `synthKnobCellHeight()`
  updated; no caption band.
- **Essential-first + divider**: `synthSectionTierSplit` already partitions by
  `ParamDef.Tier`. Render essentials, then a full-width **non-interactive**
  divider row (`— ADVANCED · N —`, hairlines + count, reuse i18n
  `KeySynthAdvanced`), then advanced cards. Delete the expander UI
  (`synthAdvExpanderRect`, `drawSynthAdvExpander`,
  `synthAdvExpanderHitAdapter`, `dv.synthAdvExpanded`,
  `dv.synthAdvScrollPending`). No hidden state; scroll reaches advanced.
- **Sampler**: TRIM/TUNE/LEVEL stay as micro group headers above card
  clusters; all 5 params essential → no divider. Desktop = one row of 5 cards;
  mobile scroll list keeps structure, cells swap pill→card.
- **Panel-height ripple**: `mobileAudioPanelMinContentH()` shrinks (card ≪
  dial cell) → mobile split recomputes; stable-pane-height test expectations
  update (`game_mobile_split_test.go` etc.).

## §3 Value-wheel popup v2

`MobileWheelPopup` becomes the platform-universal value wheel (EQ precedent).
Keep verbatim: transactional model (initialValue/dirty, Accept=commit one undo
step, Cancel=revert), barrel drag with clicky `stepNotcher` detents, wheel
pacing (`wheelStepEventsPerNotch=3`) and the empirically-pinned direction
(`TestWheelDownGesturesAllIncrease`), center-tap numeric entry, Esc/Enter,
click-outside=accept, modal+scrim portal isolation.

Changes:

- **Title band**: name (RoleBody bold) + PlainEnglish gloss (RoleCaption dim):
  "Cutoff — bright or dull". `WheelBinding` gains a `Gloss` (or the Title
  callback returns both) — sampler already passes its gloss as Title; synth
  adds `synthKnobPurpose`.
- **Rung ladder** replaces the chevron stepper (`drawResStrip`): every
  `stepLadder` rung rendered as a tappable pill in a right-hand column, active
  rung accent-highlighted; tap jumps directly (`Badge.SetStep`); scroll over
  the ladder steps one rung (existing pacing); rung persists via the existing
  `knobStepPref`/`persistKnobStep` sink. Hidden for enum/discrete params
  (unchanged). Barrel labels + center value render at active-rung precision
  (existing `formatSynthParamValueStep` path). If ladder rungs exceed the
  column height, compress spacing (ladders are ~7–8 rungs; no scroll needed).
- **Desktop Shift+scroll** anywhere in the popup value area: apply steps one
  rung finer than active, without changing the persisted rung.
- **Hover**: rung pills + center grip + close affordance get desktop hover via
  the `portalHoverer` seam + `hoverFade`.
- **Tokens**: `mobileWheelResStripW` widens (~64/72/80) to fit rung labels;
  `mobileWheelW` grows accordingly. Rename NOT required (keep token names;
  "mobile" prefix is historical).
- The EQ wheel popup (same widget) inherits ladder + hover for free; its tests
  update alongside.

## §4 Interaction rulebook (all platforms)

| Gesture | On a card | Grid whitespace | In popup |
|---|---|---|---|
| Tap/click | open popup + select card | — | barrel / ladder / center-tap per sub-rect |
| Scroll wheel | nudge value ±1 step at active rung (desktop only; live-apply; one undo step per gesture, committed on a ~600ms pause or pointer-leave) | scroll section | value / ladder |
| Shift+scroll | nudge one rung finer (desktop) | — | same |
| Vertical drag | hand off to section scroll (existing axis-lock in the hit adapter) | scroll | barrel value drag |
| Horizontal drag | nothing | — | — |
| Esc / Enter | — | — | cancel / accept |

- Card nudge reuses `Knob.HandleWheel`/`StepValueDirect` semantics so card,
  popup, and EQ share one sign convention; direction is a one-line flip after
  on-device verification.
- Routing is purely hit-rect based: card hit area at `nominal-z+2`, section
  catch-all below — no modes.
- Removed special cases: sampler readout-tap editor + dial wheel; synth
  horizontal-2-finger value (`OnWheel2D` value path); knob press-drag. Numeric
  entry is ONLY popup center-tap.
- Mobile card gestures: tap → popup (unchanged); no wheel.
- `Profile().IsMobile()` gates at `synthKnobHitAdapter.OnPress`
  (`synth_panel_zone.go:3644`), `samplerKnobHitAdapter.OnPress`
  (`sampler_panel_zone.go:1576`), and the two draw sites are removed — one
  path, all platforms.

## §5 Deletions / keeps

**Delete**: `Knob.Draw` + knurled-cap sprite cache + detent ticks + arc
pooling; `updateFromDrag`/`FineDrag`/press-drag capture in
`Knob.HandleInputResult`; `knob_value_pill.go` (superseded by ParamCard);
`KnobStepBadge.Draw` (model + `stepLadder` stay); advanced expander (all
sites); sampler special-case input branches; IsMobile gates listed above.

**Keep**: `Knob` value model (scales, endless math, StepMul, discrete
detents, `NudgeEndless`, `StepValueDirect`, `HandleWheel`); `WheelBinding`;
portal wrappers; step persistence; focus-graph selection; live synth-mirror
OnChange machinery; `synthKnobRects()` JS export name (now returns card
rects — no bridge churn).

## §6 Testing & guardrails

Functional-first (drive real tree dispatch; no isolation-only tests):

- Card tap opens popup on **desktop** and mobile (both tabs).
- Scroll-over-card nudges exactly one step per notch; exactly 1 undo step per
  gesture; scroll-over-whitespace scrolls the section.
- Ladder: tap sets rung, persists, relabels card + barrel precision;
  Shift+scroll finer; hidden for enums.
- Divider: present iff stage has advanced params; essentials render above;
  advanced reachable by scroll; count label correct.
- Enum card renders dots; bipolar gauge fills from center.
- Hover: cards + rung pills join `hover_coverage_test.go`.
- Sampler parity: same card behavior; group headers intact.
- Update/trim pinned rotary tests (~15 files: `knob_test.go` →
  model-level only; `knob_interaction_functional_test.go` rewritten for cards;
  `TestP4_AdvancedExpanderRevealsKnobs` → divider ordering; sampler
  interaction tests; `synth_knob_touch_lifecycle_test.go`; etc.).
- Token discipline: DESIGN.md drift test entries for new tokens; alloc budgets
  re-ratcheted (card render uses per-size cached sprites like keycaps);
  `eq_panel_draw_alloc_discipline_test.go` budget updated with rationale.
- Scenes: existing `crop_synth_tab_*` / `crop_sampler_tab_*` recapture; add
  desktop pass for the wheel popup scene; new `crop_synth_advanced_divider`.
- Mobile split expectations recomputed (`mobileAudioPanelMinContentH`).

## Out of scope

- Volume sliders, EQ curve handles, insert-FX controls (no rotary knobs
  there). The EQ wheel popup inherits §3 upgrades but its opener/flow is
  untouched.
- Popup redesign beyond §3 (barrel rendering, grip, portal behavior stay).
- Any audio/DSP behavior change; params, undo, persistence semantics are
  unchanged.
