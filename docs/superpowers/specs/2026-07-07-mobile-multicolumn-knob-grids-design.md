# Mobile multi-column knob grids — Sampler + Synth tabs

**Date**: 2026-07-07
**Status**: Approved (design), pending implementation
**Scope**: Mobile layout only. Desktop is untouched on both tabs.

## Problem

On mobile, both audio-panel knob sections are hard-capped to a single column
(`SetMaxCols(1)`), leaving most of the horizontal panel width empty:

- **Sampler tab**: `layoutSamplerKnobsScrolling` (`sampler_panel_zone.go:630`)
  lays the five knobs (Start / End / Pitch / Gain / …, groups TRIM/TUNE/LEVEL)
  in one vertical scrolling column of value-pill buttons.
- **Synth tab**: the per-stage `ControlGrid` layout (`synth_panel_zone.go:1708-1712`)
  caps mobile to `maxCols = 1` (desktop to 2), so each stage card shows one
  knob per row and scrolls heavily.

## Decision

Let `ControlGrid`'s native width-adaptive column selection run on mobile,
capped at **4** columns, on both tabs:

- `cols = clamp((rectW + hGap) / (idealCellW + hGap), 2, 4)` — this is the
  existing `ControlGrid.Layout` logic; only the cap changes (1 → 4 on mobile).
- `idealCellW` stays `dv2.SynthKnobIdeal` (already what both call sites pass),
  so a typical phone lands at 3–4 columns depending on width and density.
- Desktop caps unchanged: sampler keeps its bespoke one-row desktop branch;
  synth keeps `maxCols = 2`.

### What does NOT change

- Cell internal structure on both tabs (label band / pill band / caption band;
  synth's caption row + purpose gloss + concept-viz band).
- The value-pill button (`knobValuePillRect`) — it already self-sizes to its
  label and clamps to the cell width.
- Wheel popup, numeric editor, undo/commit wiring, hit-area z-scheme.
- Scroll machinery: `HasScroll()` naturally goes false when the shorter grid
  fits; the scrollbar and scroll-body catch-all hit areas are already gated on
  it (`sampler_panel_zone.go:1412`, synth equivalent).

## Implementation sketch

1. `sampler_panel_zone.go` `layoutSamplerKnobsScrolling`: `SetMaxCols(1)` →
   `SetMaxCols(4)`; update the function comment (no longer "single vertical
   column").
2. `synth_panel_zone.go` stage-grid layout: `maxCols := 2; if mobile { maxCols = 1 }`
   → `maxCols := 2; if mobile { maxCols = 4 }` (comment update likewise).

## Risks & mitigations

- **Pill text truncation at 4 columns on narrow phones** ("Ganancia +0 dB" is
  the long case). Pills self-clamp; verify with mobile screenshots in EN + ES.
  Fallback if cramped: raise the effective ideal cell width floor so narrow
  phones settle at 3 columns.
- **Synth per-cell chrome** (plain-English purpose gloss + concept mini-visual
  under the caption) was the stated reason for the fewer-columns cap; narrow
  cells may squeeze it. Verify visually; if a renderer misbehaves at narrow
  widths, prefer its existing width guards / clamping over re-widening the
  grid.
- **Group labels (TRIM/TUNE/LEVEL)** render in each cell's own label band —
  verify per-cell placement reads correctly in a grid row.

## Testing

- Update single-column assumptions: `sampler_scroll_test.go`,
  `sampler_knob_size_i18n_test.go` (and any synth grid tests asserting 1
  column on mobile).
- New functional tests (real `Game.Update`, mobile profile):
  - Sampler: grid reports 2–4 columns; all five knobs land in visible,
    non-overlapping cells on a standard mobile viewport (no scroll needed).
  - Synth: a stage card's grid reports 2–4 columns on mobile.
- Visual verification: `crop_sampler_tab` + synth-tab mobile screenshot
  scenes, EN + ES.
