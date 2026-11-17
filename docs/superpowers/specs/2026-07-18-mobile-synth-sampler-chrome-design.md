# Mobile Synth & Sampler chrome redesign

**Date:** 2026-07-18
**Status:** Design — awaiting review
**Scope:** `internal/ui/` mobile layout of the Synth and Sampler audio-panel tabs.

## Overview

On a 390×844 mobile viewport, the Synth and Sampler editor tabs spend most of the
bottom pane on chrome — a redundant instrument-name banner, a full-width row of four
equal action buttons, and truncated stage-pill labels — leaving only ~20% of the
screen for the actual sound-editing surface. This redesign removes that chrome and
relocates the primary actions onto the always-present sticky bar, so the editor reads
as a clean, spacious, kid-friendly surface without touching the pane-split algorithm
or desktop layout.

Measured vertical bands (390×844, current build):

| Band | Synth | Sampler |
|---|---|---|
| Graph/node pane (top) | ~41% | ~41% |
| Identity banner (`dnb-kick — synth-modular-kick-d…` / `SAMPLER`) | ~40px | ~40px |
| Action buttons (Preview/Save/Save As/Reset, 4× full width) | ~45px | ~45px (+Rev/Norm/Fade ~63px) |
| Actual knob/preview editing | ~20% | ~20% |

## Goals

1. Remove the redundant instrument-identity banner on both tabs; identity lives in the
   channel pill, which already shows the friendly name.
2. Reduce the action buttons to a **primary pair (Preview, Save) + a `⋯` overflow**
   holding Save As and Reset, and host them on the sticky bar beside the channel pill
   (Approach B), eliminating the separate in-panel action row entirely.
3. Eliminate ellipsis truncation on stage-pill labels via consistent short labels.
4. Reflect the design system: correct action hierarchy, `destructive` red role for
   Reset, no forbidden glyphs, touch-min compliant hit targets.

## Non-goals

- **No change to the pane-split algorithm** (`game_layout.go:adaptiveMobilePortraitSplitY`,
  `mobileAudioPanelMinContentH`). The grid stays at its ~40% floor. Removing chrome
  shrinks the bottom pane's content floor, so the grid may drift slightly *larger* —
  never smaller. This is acceptable and expected.
- No change to desktop layout (the desktop header/action row and desktop sticky-bar
  tab strip are untouched).
- No change to the stage-detail knob rendering, the synth preview pane, or the sampler
  waveform/TRIM controls.
- No new mobile scroll mechanism beyond what already exists
  (`layoutSamplerKnobsScrolling`, chip-strip scroll).

## Current-state anchors (from code map)

- **Synth banner:** `synth_panel_zone.go` — `drawSynthHeader` (:2186), `layoutSynthHeader`
  (:1381), `synthHeaderCaptionText` (:2212), waveform thumbnail `drawSynthHeaderWaveform`
  (:2243). Header height in `buildSynthTab` (:1135) via `Profile().SynthHeaderH`
  (mobile 38) / `SynthHeaderButtonH`.
- **Synth actions:** `layoutSynthHeader` mobile branch (:1411) lays four equal cells;
  built by `buildSynthHeaderButtons` (:1491) with callbacks `previewActiveSynth`,
  `SaveActiveRecipe`, `openSaveAsDialog`, `ResetActiveRecipe`.
- **Sampler banner + actions:** `sampler_panel_zone.go` — title in `drawSamplerTab`
  (:859); mobile two-row action layout `layoutSamplerMobileButtonRows` (:678) →
  `toggleR` (Rev/Norm/Fade) + `actionR` (Preview/Save/Save As/Reset).
- **Channel pill (identity home):** sticky bar `audio_sticky_bar.go` — `channelBtn`
  placed at `Layout` (:126–134); label refreshed by
  `eq_panel_zone.go:refreshChannelPillLabel` (:1118) to the active instrument's row name.
- **Sticky bar mobile right region is empty:** on mobile the legend chip (:143) and the
  entire tab strip (:153) are collapsed to empty rects, so the sticky bar's right side
  is unused — the home for the relocated actions.
- **Stage labels/truncation:** `sectionLabel` table (:192), `sectionShortLabelKey`
  (:267), fit-or-ellipsis in `sectionLabelChipFitted` (:282), drawn at
  `drawSynthChipStripContent` (:2580).
- **Mobile axis:** `Profile().IsMobile()` (`layout_profile.go:201`).

## Design

**All changes below are gated on `Profile().IsMobile()`. Desktop layout — the synth
header banner + inline action buttons, the sampler header buttons, and the desktop
sticky-bar tab strip — is untouched.** The synth banner (`drawSynthHeader`) and its
layout currently render on both classes, so the mobile behavior is added as an
`IsMobile()` branch rather than an unconditional deletion.

### 1. Suppress the identity banner on mobile (both tabs)

- **Synth:** on mobile, `buildSynthTab` reserves no `SynthHeaderH` band and
  `layoutSynthHeader`/`drawSynthHeader` skip the caption card + waveform thumbnail; the
  synth tab content begins with the chip strip. On desktop the banner and its inline
  actions render exactly as today.
- **Sampler:** on mobile, `drawSamplerTab` skips the `SAMPLER` title and `buildSamplerTab`
  reserves no band for it; the waveform becomes the top of the sampler body. (Sampler
  already branches mobile↔desktop for its header buttons via `layoutSamplerHeaderButtons`.)
- **Identity is unchanged** — `refreshChannelPillLabel` already shows the friendly
  instrument name in the always-visible channel pill. The dropped recipe-ID string
  (`synth-modular-kick-…`) is dev-facing and intentionally gone from the mobile view.

### 2. Primary actions + `⋯` overflow, hosted on the sticky bar (Approach B)

**Ownership contract (matches the codebase's "bar never reaches into parent" rule):**
the action buttons keep their existing owners and callbacks in the synth/sampler
panels; the sticky bar only *lays them out and registers their hit areas*. Concretely,
`AudioStickyBar` gains an optional, per-frame-settable list of "trailing action
buttons" (mirroring how `SetActiveTab` is pushed in before `Layout`). The parent
(EQ/synth/sampler dispatch) supplies:

- **Synth active + mobile:** `[Preview] [Save] [⋯]`
- **Sampler active + mobile:** `[Preview] [Save] [⋯]`
- **Any other tab, or desktop:** empty (desktop keeps its existing in-panel/right-of-
  caption layout — unchanged).

Layout: the buttons pack right-to-left in the sticky bar's right cluster (the region
vacated by the hidden legend/tab pills), left-clamped so they never overlap the channel
pill (same clamp discipline as the desktop tab strip). Hit areas register at
`parentZIndex+1` with `Touch=true` + `ClipRect=expandToTouchMin(rect)` so the ~32px
pills meet the 44px touch minimum, exactly as the channel pill already does.

**The `⋯` overflow menu** opens a small popup (existing overlay-portal infra) with two
rows:
- **Save As…** → `openSaveAsDialog` / sampler equivalent.
- **Reset** → `ResetActiveRecipe` / sampler reset, rendered with the design system's
  `destructive` red role. Reset follows the existing destructive-confirm pattern if the
  action already has one; otherwise it at minimum reads as destructive (red).

**Removed:** the synth in-panel action row (`layoutSynthHeader` mobile branch) and the
sampler `actionR` row (`layoutSamplerMobileButtonRows`) are deleted on mobile. The
sampler **`toggleR` (Rev / Norm / Fade) transforms row stays** in the sampler body — it
is a set of distinct sample operations, not save-actions.

### 3. Fix truncated stage-pill labels (synth)

For `ENSEMBLE`, `FORMANT`, `RESONATOR` (and any label that overflows its chip), add
designed short labels — **ENS / FRM / RES** — via `sectionShortLabelKey`, so
`sectionLabelChipFitted` chooses the short label before ever falling through to
`truncCaptionStyled` (the `…`). This matches the existing OSC/FM/LFO/ENV/`F.ENV`
abbreviation convention. Add the corresponding `i18n.KeySynthStage*Short` keys to the
en/es catalogs.

## Design-system compliance

- **Action hierarchy:** primary (Preview, Save) vs. secondary/destructive (Save As,
  Reset in overflow) now visually distinct — resolves the "four equal buttons" flatness.
- **Reds/roles:** Reset uses the `destructive` role (button fill), not neutral — matches
  DESIGN.md's "three reds, three roles" invariant.
- **Glyphs:** the `⋯` overflow affordance uses an `IconID` (e.g. an ellipsis/kebab icon
  via `DrawIcon`), never a raw glyph, per the forbidden-glyph rule. No new
  `color.RGBA` literals — reuse `Token*` accessors.
- **Touch-min:** relocated actions use `Touch`+`ClipRect` expansion, not geometric rect
  growth, per the zone-input-isolation discipline.

## Testing

- **Screenshots (visual):** re-capture `mobile_bottom_nav_synth` and
  `mobile_bottom_nav_sampler` scenes; verify banner gone, single-row actions on the
  sticky bar, no `…` on stage pills, and a visibly larger editor/knob region.
- **Go fast-path tests:**
  - Synth/sampler layout: assert no banner rect is reserved; assert the sticky bar
    exposes the three action hit areas (`preview`/`save`/overflow) on mobile when the
    tab is active, and none on desktop / other tabs.
  - Overflow menu: opening `⋯` surfaces Save As + Reset; Reset carries the destructive
    role.
  - Stage labels: assert `sectionLabelChipFitted` returns ENS/FRM/RES (never an
    ellipsis) at the mobile chip width.
  - Sticky-bar isolation: relocated action hit areas do not leak to sibling zones
    (extend `zone_input_isolation_discipline_test.go` coverage if needed).
- **i18n:** short-label keys present in en + es catalogs (drift guard).
- Follow existing functional-test doctrine — drive `Game.Update`, not hand-built structs.

## Risks

- **Sticky-bar coupling.** Adding trailing actions to `AudioStickyBar` must preserve its
  "never reaches into parent" contract — mitigated by the parent supplying pre-built
  buttons (behavior stays in the panel; only layout moves to the bar).
- **Split drift.** Dropping `SynthHeaderH` from `mobileAudioPanelMinContentH` lowers the
  content floor, so the grid may grow a few percent. Acceptable per Non-goals; verify no
  tab clips its content after the change.
- **Sampler desktop parity.** Sampler currently moves buttons to the control column on
  mobile and keeps them in the header on desktop — the mobile relocation must not
  disturb the desktop header path.

## Out of scope (possible follow-ups)

- Stacking the VOICE-detail two-column (knobs | preview) into a single full-width column
  on mobile — raised in the critique, deferred to keep this change minimal.
- Any grid-pane collapse behavior (explicitly declined in favor of keeping the grid).
