# Vice City Palette — single-source color system + instrument color picker

**Date:** 2026-06-13
**Branch:** add-core-node-types
**Status:** Design approved (brainstorm). Implementation plan to follow via writing-plans.

## 1. Overview

Two coupled deliverables:

1. **A single Vice City color system** that governs every color in the UI. The codebase is already fully token-driven (no stray inline `color.RGBA` literals — the `TestTokenDiscipline` ratchet enforces this), so the work is to **repaint the 111 named color tokens in `DESIGN.md` so each one is a member of an authentic GTA-Vice-City / outrun palette** (plus a small neon-night neutral ramp), and to add a guard test that makes off-palette color impossible.
2. **An instrument-row color picker** that lets the user pick a row's color from that palette, presented as labelled gradient ramps grouped by hue family. (Re-exposes and upgrades the existing, currently scene-only `ColorWheelComponent`.)

The aesthetic target is the real Vice City signature — hot-pink→magenta (the logo color), electric cyan/turquoise, vivid violet, and the outrun sunset ramp (gold→orange→pink→magenta→purple) — with each hue family rendered as a **gradient ramp** where every shade melts into the next. References: color-hex Vice City (#ee00dd ramp), Synthwave Sunset (#ff2975/#f222ff/#8c1eff), Miami Vice turquoise/coral/flamingo.

## 2. Goals / Non-goals

**Goals**
- Every user-visible color derives from one canonical palette declared in `DESIGN.md`.
- A build-failing guard rejects any color token outside the palette.
- Authentic Vice City look: violet-black surfaces, cyan + hot-pink accents, neon instrument data colors.
- Contrast guidance codified and (where checkable) test-enforced.
- A grouped, gradient color picker on the instrument row ellipsis menu.

**Non-goals (explicit)**
- **No geometry, layout, spacing, typography, or component-shape changes.** This is a *colors-only* repaint. Every token's *role* and every call site stays identical; only hex values move. Density/profile/spacing/rounded tokens are untouched.
- No freeform/RGB-wheel color selection — palette membership only.
- No new audio-panel tabs or controls; the audio tabs change color only.
- Full multi-preset/HSV picker is out of scope.

## 3. The palette (canonical, declared in DESIGN.md)

### Tier 1 — Chromatic ramps (35 colors)

Seven hue families × five shades, ordered light→deep (`-100`…`-500`). Family order is itself a continuous spectrum.

| Family | -100 | -200 | -300 | -400 | -500 |
|---|---|---|---|---|---|
| `sunset-gold` | #FFE98A | #FFD319 | #FFB30A | #FF9E1F | #F58A12 |
| `tangerine` | #FFC299 | #FF9E54 | #FF7A3D | #FF5C2A | #E84A1F |
| `hot-pink` | #FFA8E0 | #FF6AD5 | #FF2D9E | #EE00DD | #C400B5 |
| `orchid` | #E0A8FF | #C24FFF | #A030FF | #8C1EFF | #6E12E0 |
| `electric-blue` | #A8C8FF | #5E8AFF | #3A6AF0 | #2A5FD6 | #1E47B5 |
| `cyan` | #9AF0FF | #3FE0E8 | #00C8E0 | #00A8C9 | #0E7391 |
| `mint` | #B3F5C2 | #6BE89A | #3FD67A | #2BC85C | #1FA84A |

`hot-pink-400` (#EE00DD) is the Vice City signature.

### Tier 2 — Neon-night neutrals (violet-tinted, derived from the palette)

| Token | Hex | Role |
|---|---|---|
| `background` | #120A1C | app background |
| `surface-1` | #1C1230 | row rack / base panels |
| `surface-2` | #281A40 | cards, buttons |
| `surface-3` | #382354 | hover / raised |
| `surface-overlay` | #1F1438 | popups, menus |
| `text-hi` | #FDF2FF | high-emphasis text/icons |
| `text-mid` | #C9B8D6 | secondary text |
| `text-dim` | #8A7A9C | disabled / hints |
| `white` | #FFFFFF | **alpha-only** anchor: borders, highlights |
| `black` | #000000 | **alpha-only** anchor: shadow, scrim |

`white`/`black` are achromatic and may appear **only** via `WithAlpha(...)` (never as a solid fill), so they introduce no hue.

### Tier 3 — Semantic roles → palette member

| Role (token) | Member | Hex | Notes |
|---|---|---|---|
| `primary` | cyan-300 | #00C8E0 | interactive surfaces, sliders, active borders |
| `primary-bright` | cyan-200 | #3FE0E8 | high-contrast variant |
| `primary-dim` | cyan-400 | #00A8C9 | muted fill |
| `focus` / selection | hot-pink-300 | #FF2D9E | selection rings, focus |
| `success` / play | mint-300 | #3FD67A | play, good-state |
| `warning` | sunset-gold-200 | #FFD319 | caution |
| `error` (text + stop) | tangerine-400 | #FF5C2A | the reddest legible member |
| `destructive` (delete fill) | tangerine-500 | #E84A1F | deep ember |
| `mute` | hot-pink-500 | #C400B5 | deep magenta (moves off the old rust-brown) |
| timeline accent | sunset-gold / tangerine | gold ramp | view/cursor/hi |

**Accent model change:** this replaces the old *single azure-accent* invariant with a deliberate **two-accent system: cyan (primary) + hot-pink (focus/selection)** — the quintessential Vice City pairing. The previous `splitter-handle` cyan "exception" is now on-palette and no longer an exception. The "three reds" roles survive but are repainted: error→vermillion, destructive→ember, mute→magenta.

## 4. Repainting the 111 existing tokens

Mechanical re-point of every `colors:` entry in `DESIGN.md` to a palette member or alpha-of-anchor. No call sites change (they already read named tokens). Representative mapping:

- **Surfaces / grid / drum stripes** → neon-night ramp. `grid-line/half/quarter/eighth/…` become stepped `white`-over-`surface` alphas; `drum-stripe-even/odd`, `drum-cell-off` → violet-black surfaces.
- **Text / borders** → `text-hi/mid/dim`; all `border*` → `white` via existing alpha buckets.
- **Viz / data (audio tabs)** — see §5. `viz-meter-green/yellow/red` → mint-300 / sunset-gold-200 / tangerine-400; `viz-wave-a`/`viz-curve`/`scope-trace-a` → cyan; `viz-wave-b`/`viz-mids` → `text-dim`; `viz-bass`/`viz-treble` → tangerine / electric-blue; `scope-trigger` → sunset-gold-200; `meter-clip` → tangerine-500.
- **Timeline** amber family → sunset-gold/tangerine ramp.
- **Transport / play / stop / record** → mint (play), tangerine (stop/record).
- **Node / edge / splitter** → `node-fill` = surface-2, `node-border`/`edge` = `text-dim` via alpha, `splitter-handle` = cyan-300.
- **Instrument swatches + sequence** → replaced by the 35 (see §6).

Naming follows existing codegen: `hot-pink-300` → `genColorHotPink300`; semantic aliases (`genColorPrimary`, etc.) re-point to the chosen member. `theme.go` `col*` aliases are unchanged in name; only the `gen*` value they reference moves.

## 5. Audio-panel tabs are colors-only and fully token-sourced

Acceptance criterion: **every audio-panel tab (Wave, Spectrum, Levels, EQ, Chain, Synth, Sampler, Scope) renders all color from `viz-*`/semantic tokens — no inline literals, no geometry change.**

- The renderers (`render_waveform.go`, `render_spectrum.go`, `render_meters.go`, `render_scope.go`, `render_eq_curve.go`, `eq_panel_zone.go`, `chain_panel_zone.go`, `synth_panel_zone.go`) are already at literal-budget 0; repainting the `viz-*` tokens recolors them automatically.
- Implementation step: audit each renderer to confirm zero non-token color usage; route any stray through a palette token (none expected per inventory, but verified, not assumed).
- Pixel coverage already exists: `audio_panel_render_pixels_test.go` (Go fast path) + `audio_panel_render.browser.test.js`. These keep passing as content; the repaint must not change geometry, only color. EQ curve handles use `primary` (cyan); EQ bands/meters follow the viz mapping above.

## 6. Instrument swatches + auto-assign sequence

- `instrumentSwatches:` (currently 12) → the **35** ramp colors.
- `instrumentSequence:` → a curated walk maximizing adjacent-row hue contrast, e.g.: `hot-pink-400, cyan-300, sunset-gold-200, orchid-300, mint-300, electric-blue-300, tangerine-300, hot-pink-200, cyan-100, …`. Exact order tuned during implementation; the invariant is that consecutive entries are from different families.
- `startup_demo.json` and genre templates re-sequenced to on-palette sequential colors (existing `ResequenceRowColors` + the JSON-authoritative template flow).
- Compliance tests (`vice_city_palette_compliance_test.go`, `instrument_palette_test.go`) re-pointed to the new set.

## 7. Contrast & usage guidance (normative DESIGN.md prose + tests)

1. **Two safe text/icon patterns only:** near-white/near-black on a dark surface, **or** near-black (`#120A1C`) on a neon fill. Never mid-neon on mid-neon.
2. **Targets:** text & icons ≥ **4.5:1**; large text & graphical/control strokes ≥ **3:1**.
3. **Controls:** filled = neon fill + `#120A1C` label/icon; secondary = ghost outline (neon stroke + neon label on surface); icons default to `text-hi`, flip to `#120A1C` on an active neon fill.
4. **Pale swatches always get a 1px `white`-low-alpha border** so light shades don't vanish on the dark background (instrument swatches + picker).
5. **Glow is decoration, never contrast:** neon bloom (box-shadow / glow ring) is aesthetic; legibility must hold with glow removed.

## 8. The instrument color picker

- **Entry point:** a **"Color"** item returns to the row ellipsis/kebab menu in `drumview_context_menu.go` (`contextMenuItems`), uniformly on every platform. On click it opens the existing `ColorWheelComponent` through `openColorWheelPortal()` (`drumview_portal_open.go`) — the portal plumbing already exists (reachable for scenes today); we re-expose the user-facing entry.
- **Picker UI (upgrade `drumview_overlay_color_comp.go`):** render the 7 ramps as labelled rows (family name in `text-mid`), each a left→right gradient row of 5 shade swatches. Current color shows a `focus` (hot-pink) selection ring. Pale swatches bordered per §7.4. **No geometry philosophy change** — reuse existing popup chrome (`menu_chrome.go` header band, `AnchorPopupRect` positioning, portal scrim handling).
- **Pick behavior:** clicking a swatch calls existing `SetRowColor(idx, c)` → `emitRowColorChanged` → `recordUndoStep(EventRowColorChanged)`.
- **Uniqueness:** relax `SetRowColor` so a **deliberate manual pick wins even if it duplicates another row** (drop the auto-substitute on the manual path); keep uniqueness/substitution for *auto*-assigned new rows. Update `TestSetRowColorRejectsDuplicate` / `TestUniqueColorsAcrossRows` to reflect manual-pick-wins while auto-assign stays distinct.

## 9. Enforcement (tests)

- **`TestPaletteMembership` (new):** parse `DESIGN.md` `colors:`; assert every hex ∈ AllowedColorSet = {35 ramp members} ∪ {neon-night neutrals} ∪ {`#FFFFFF`,`#000000`}. Failure names the offending token + nearest legal member. *This is the lock.*
- **`TestColorContrast` (new):** compute WCAG ratios for semantic pairings (text-hi on each surface; `#120A1C` on each neon fill; error/play/warning legibility). Fail below §7.2 targets.
- **`TestDesignMDDrift`:** want-map updated to repainted values (mechanical).
- **`TestTokenDiscipline`:** unchanged mechanism; literal ratchet stays at current budgets. Membership + ratchet together ⇒ nothing user-visible is off-palette.
- **Vice City compliance tests:** re-pointed to the new 35 + new sequence.
- **Lint snapshot:** `design_md_lint.expected.json` regenerated.
- **Picker functional test (new):** drive the real `Game.Update()` loop — open kebab → Color → click a ramp swatch → assert the row recolors and an undo step records (functional-over-isolation).

## 10. Phasing

1. **Palette foundation** — add the 35 ramps + neutrals + semantic aliases to `DESIGN.md`; `make gen-design-tokens`; add `TestPaletteMembership` + `TestColorContrast`. No visual change yet.
2. **Repaint tokens** — re-point all 111 tokens; update drift want-map + lint snapshot; regenerate. *(UI visibly turns Vice City: violet-black surfaces, cyan/pink accents.)*
3. **Instrument swatches/sequence** — swap to the 35; re-point compliance tests; re-sequence `startup_demo.json` + templates.
4. **Color picker** — re-expose ellipsis "Color" entry; grouped gradient picker UI; uniqueness relax; functional test.

Each phase leaves the build green and is independently testable.

## 11. Risks / notes

- **Visual shift is large by design** (blue-black → violet-black, azure → cyan+pink). Approved to proceed unflagged.
- **Drift/lint want-maps are large mechanical edits** — keep them in lockstep with the DESIGN.md change in the same phase or the build breaks.
- **Branch hygiene:** `add-core-node-types` carries heavy uncommitted WIP. Per standing instruction, **do not stage or commit** — all changes (including this spec) stay in the working tree; the user owns commits.
- Manual revert of any change uses the Edit tool, never `git checkout/restore/reset`.
