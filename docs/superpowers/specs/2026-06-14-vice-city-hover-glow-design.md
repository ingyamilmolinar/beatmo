# Vice City Hover Glow — Design

**Date:** 2026-06-14
**Branch:** add-core-node-types
**Status:** approved, implementing (uncommitted per repo working-tree policy)

## Goal

Make every button's hover affordance read as a Vice City neon lift: a soft
cyan glow that **eases in** when the cursor enters and **fades out** when it
leaves — instead of today's static ring that pops on/off with the `hovered`
bool. Not distracting: it ramps to a steady level, no ongoing motion while at
rest on the control.

This implements the "Hover glow *(naming reserved; runtime work is a
follow-up)*" item already reserved in DESIGN.md § Cushioned elevation.

## Palette grounding (DESIGN.md)

- **`primary-bright` `#3FE0E8`** is the designated **"Hover ring; never
  decorative"** token. Base **`primary` `#00C8E0`** is the rest/active accent.
  So hover brightens to `primary-bright`; the active/latched (`toggled`) pulse
  keeps `primary`.
- Stays inside the single **cyan** chrome-accent family (invariant #1). No
  hot-pink — that is reserved for focus/selection rings.
- **Mobile chrome stays flat** (§ Cushioned elevation): the hover glow is a
  desktop-only affordance, so a mobile tap must not flash it.
- Tonal lift surface-2 → surface-3 on hover already ships via the ComponentSpec
  `+12` hover brightness delta — unchanged.

## State / motion table

| State | Glow color | Alpha | Motion |
|---|---|---|---|
| Rest | — | 0 | none |
| Hover (desktop) | `primary-bright` `#3FE0E8` | `togglePulse.AlphaScale × button-glow-rest × hoverAnim` | ease 0→1 in, 1→0 out |
| Active / `toggled` | `primary` `#00C8E0` | `SinPulseAlpha(togglePulse)` | gentle `|sin|` breath (unchanged) |

`toggled` takes precedence over hover (unchanged).

## Revision (post-implementation): overlay, not Button.Draw

The first cut animated the glow inside `Button.Draw`. Root-cause debugging
showed that cannot work: (1) the input dispatcher only runs on
press/drag/release, so `Button.hovered` is never set on plain mouse-over for
tree-dispatched buttons (play/stop, instrument label, …); and (2) zones render
buttons into state-hashed sprite caches (`TransportZone.toolbarCache`), so
`Button.Draw` isn't called per frame and a smooth fade can't render from inside
the cache. The user reported "no hover on any button" — both causes confirmed.

**Final design — a per-frame overlay** (`hover_glow_overlay.go`):
`DrumViewTree.Draw` calls `drawHoverGlow` on top of all zones (beneath
portals). Each frame it queries the live `HitIndex` at the cursor, resolves the
topmost button hit via a `glowButtonProvider { glowButton() *Button }` interface
(implemented by `buttonHitAdapter`, `repeatButtonHitAdapter`,
`samplerButtonHitAdapter` — "buttons only"), eases a single 0..1 progress toward
"over a button?" via `genAnimButtonHoverFade`, and strokes the primary-bright
ring at the button's rect. Immune to zone caching (drawn outside them);
universal (HitIndex knows every control + z-order/occlusion); desktop-only.
`Button.Draw` keeps only the toggled/active pulse; the overlay skips toggled
buttons so they never double-glow. Functional regression test drives the real
`DrumViewTree.Draw` path (`hover_glow_overlay_test.go`).

Not covered (follow-up): Synth-tab header buttons (Save/Save As/Preview/Reset)
use bespoke `*DrumView` adapters with no `*Button` to resolve.

## Implementation (original plan — superseded by the revision above)

Single chokepoint: `(*Button).Draw` (`uigrid.go`) draws the glow ring for every
button, so all buttons are covered.

1. **DESIGN.md token** `animations.button-hover-fade` — `kind: exp-decay`,
   `rate: 0.65`, `threshold: 0.01` (~0.2 s settle @60 fps). Regenerate with
   `make gen-design-tokens` → `genAnimButtonHoverFade`.
2. **`Button.hoverAnim float64`** (0…1) + **`(*Button).AdvanceHoverAnim()`**:
   eases `hoverAnim` toward target each frame; target = 1 when `hovered &&
   !Profile().IsMobile()`, else 0. Allocation-free, no-op when settled. Called
   from `Draw` beside `AdvancePressAnim()`.
3. **`buttonGlowAlphaAnimated(hoverProgress float64, toggled bool, frame int64)
   uint8`** in `button_cushion.go`:
   - `toggled` → `SinPulseAlpha(frame, genAnimButtonTogglePulse)`
   - else → `uint8(AlphaScale × button-glow-rest × hoverProgress)`
   Existing `buttonGlowAlpha(hovered bool, …)` becomes a thin `bool→0/1`
   wrapper so the two pinned tests stay valid.
4. **`drawButtonGlowRing` gains a `col color.Color` arg**: `primary-bright`
   (`TokenAccentBright()`) for the hover ring, `primary` (`TokenAccent()`) for
   the toggled pulse. `Draw` picks the color by state.
5. **DESIGN.md prose**: drop the "naming reserved / follow-up" caveat; state
   hover ring = `primary-bright`, animated via `button-hover-fade`.

## Out of scope

- Converting the 1-px ring stroke into a true 2-px inset *radial* gradient
  (the eventual ideal in the prose) — larger rendering change; the brighter
  hover color + fade already delivers the Vice City feel.
- Extending hover plumbing to tree/HitIndex-dispatched buttons that never
  receive per-frame hover today — separate concern.

## Tests (TDD, fast `-tags test` path)

- `buttonGlowAlphaAnimated`: progress 0 → 0; progress 1 untoggled == token
  steady value; toggled ignores progress (pulse visible across a cycle);
  monotonic non-decreasing in progress.
- `AdvanceHoverAnim`: from 0 hovered+desktop ramps monotonically up and settles
  at 1; from 1 not-hovered ramps to 0; mobile target is 0 even when hovered;
  settled is a no-op.
- Color guard: hover ring resolves to `primary-bright`, toggled pulse to
  `primary`.
- Existing `TestButtonGlowAlpha` / `TestButtonGlowAlphaHoverIsTokenDriven`
  stay green unchanged.
