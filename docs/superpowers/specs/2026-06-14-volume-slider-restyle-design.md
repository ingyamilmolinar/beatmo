# Volume Slider Restyle — Design

**Date:** 2026-06-14
**Branch:** add-core-node-types
**Status:** Approved (visual direction), pending implementation plan

## Problem

The volume controls look out of place against the Vice City / Neon Horizon aesthetic
documented in DESIGN.md. Concretely:

- **In-row level indicator** (`slider_level_indicator.go`): a flat, faint 2px straight
  `drawRect` underline beneath the speaker icon.
- **Volume popup** (`slider_popup.go`): a cramped panel with a hairline 2px track and a
  flat **12px square** thumb (`drawRect`, not even rounded), which also clips at the
  bottom screen edge when opened from a low row.
- **Horizontal param slider** (`slider.go`): a thin track with an off-white pill thumb —
  plain and flat, no neon treatment.

None use the rounded surfaces, neon-cyan fills, or glow/cushion depth that the rest of the
UI (buttons, chrome) uses.

The user likes the existing **interaction** ("tap the speaker to toggle a slider open, drag
to set") and wants it kept — only the **style** restyled and the control made **easier to
use, especially on mobile**.

## Decisions (from brainstorming)

| Decision | Choice |
|---|---|
| Interaction model | **A — Polished popup.** Keep tap-to-open + drag exactly as today. |
| In-row indicator | **A1 — Rounded underline.** Same position, rounded ~4px bar with a dim rail + glowing fill. |
| Popup orientation | **H — Horizontal on mobile, vertical on desktop.** |
| Glow implementation | Reuse the existing `button-glow-*` tokens + button-cushion glow primitives; adapt slightly where it reads better (calmer glow on the many-on-screen param sliders). |
| Scope | All three surfaces: per-row volume, master volume (shares the same code), and the horizontal param slider. |

## Aesthetic target

Shared visual language across all three surfaces:

- **Rails / tracks:** rounded (radius = height/2), dim base on `surface-3` (`#382354`) so the
  full range always reads as a rail.
- **Fill:** neon cyan — `primary` (`#00C8E0`) / `primary-dim` (`#00A8C9`) — with a subtle
  glow bloom. The in-row indicator fill is tinted to the **row's instrument color** instead
  of cyan (as today).
- **Thumb:** a round knob — `on-surface` (`#FDF2FF`) core with a cyan glow ring
  (`buttonGlowRingColor` / `viz-glow` `#9AF0FF`), brightening while dragging. Replaces the
  square popup thumb and the off-white pill on the horizontal slider.
- **Popup panel:** rounded `surface-2` with a cyan keyline + soft bloom (same primitives as
  `drawButtonHoverGlow` / `drawButtonGlowRing`), not a literal gradient.

Gradients shown in the mockups are approximated with a solid accent fill + layered
`drawRoundedRect` glow strokes — the established way this codebase renders neon.

## Components & changes

The three surfaces share drawing helpers so a tweak propagates everywhere — mirroring the
existing `drawVolumeButton` "single source of truth" comment.

### 1. In-row level indicator — `slider_level_indicator.go`

- Replace the flat `drawRect` track + fill with **rounded** bars (`drawRoundedRect`,
  radius = height/2), thickness bumped from 2px to ~4px (token-governed, see Tokens).
- Always draw the dim rail (`surface-3`); draw the fill (row color) on top with a subtle
  glow. Off/muted = rail only (preserve current semantics).
- Keep: footprint = icon glyph width, clamped inside `btnR`; per-row color tint;
  `indicatorMinIconWidth` no-op guard.

### 2. Volume popup — `slider_popup.go`

Keep the `SliderPopup` type, `SliderPopupConfig`, `OnRelease` commit semantics, portal
wiring, and tap-to-toggle-close (`Anchor()`). Add **orientation**:

- New internal orientation (vertical | horizontal), selected at `Open()` time from
  `Profile().IsMobile()` (mobile = horizontal). Desktop behavior is unchanged.
- `Open()` sizes the panel per orientation (horizontal = wider/shorter), places via
  `AnchorPopupRect` with clamping so it **never runs off-screen** in the active axis —
  fixing the bottom-edge clip.
- `Draw()`: rounded panel + keyline/bloom; rounded rail; neon fill; round glowing thumb
  (~24px desktop, larger on mobile per density). Title + percentage readout retained.
- `HandleInput()`: map the **active axis** to value (X for horizontal: left=0,right=1;
  Y for vertical: unchanged top=1,bottom=0). Whole-panel drag retained so the user need not
  hit the thumb precisely (key mobile ergonomics). `OnRelease` still fires once on pointer-up
  after ≥1 `SetValue`.
- Helpers `trackTop`/`trackBot` generalize to `trackStart`/`trackEnd` along the active axis.

Master volume popup comes along for free (same type, opened via `openMasterVolumePopup`).

### 3. Horizontal param slider — `slider.go`

- Thicken the rounded track; keep neon fill.
- Replace the off-white pill thumb with the same round glowing thumb, **adapted** to a
  calmer glow alpha since many can appear on one screen (FX/synth param rows). Drag/label
  logic unchanged.

## Tokens (DESIGN.md → `make gen-design-tokens`)

- Bump `sliderTrackH` per density (thicker, touch-friendlier).
- Indicator thickness: promote the `2px` literal to a density/spacing token (~4px).
- Thumb: reuse `SliderThumbW/H` (already per-density: 10–14 / 16–24). Glow: reuse
  `button-glow-*` (`button-glow-rest`, glow-spread) + `viz-glow`; add a single adapted
  "slider thumb glow" alpha only if reuse looks too hot on param rows.
- Horizontal-popup dimensions (width/height/track length) as tokens.

Follow the **token-change checklist** in CLAUDE.md:
- Update the want-map in `design_md_drift_test.go` for any new named token.
- Decrement per-file budgets in `token_discipline_test.go` as `drawRect`/`color.RGBA`
  literals become token-backed (ratchet — the test fails if you forget).
- Use `WithAlpha(token, Alpha*)` buckets, not raw opacity values.

## Testing (fast `-tags test` path)

- **`slider_popup_test.go`** — add orientation coverage:
  - Horizontal (mobile profile via `SetRuntimeProfileForTest` / mobile layout): value maps
    from X (left=0, right=1); placement stays on-screen (no bottom clip); `OnRelease` fires
    once on release.
  - Vertical (desktop) existing behavior stays green.
- **`slider_level_indicator.go`** — add a render test: rail always present; fill length ∝
  value; off/muted → rail only; per-row color tint applied.
- **`slider.go`** — keep `slider_test.go` / `slider_handler_test.go` green; assert the new
  thumb still resolves value across full range.
- Keep `slider_group_test.go`, `volume_slider_realtime_test.go`, `volume_popup_test.go`
  green (capture/exclusion/realtime-cache semantics unchanged).
- No new manual-test steps — every behavior is covered by an automated test.

## Out of scope

- Mute/solo interaction (separate buttons; untouched).
- The "tap-to-open / tap-to-close" gesture itself (kept as-is).
- Audio behavior, undo/redo wiring (volume already has its `OnRelease` commit point).

## Risks

- **Horizontal popup placement** is the main new code path: anchor flip + clamp in the X
  axis. Mitigated by the placement/no-clip test and reuse of `AnchorPopupRect`.
- **Token ratchet**: forgetting a budget decrement fails `token_discipline_test.go` — caught
  automatically.
- Param-slider glow could be visually noisy with many on screen — mitigated by the calmer
  adapted glow alpha.
