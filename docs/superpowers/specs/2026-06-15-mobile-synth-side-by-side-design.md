# Mobile Synth Tab — Side-by-Side Knobs + Waves

**Date:** 2026-06-15
**Branch:** add-core-node-types
**Status:** Design — approved, pending spec review
**Builds on:** the synth focus-graph redesign + phase-2 mobile always-on band (`2026-06-15-synth-focus-graph-redesign-design.md`, Task 19).

## Problem

On mobile the synth-tab knobs are unusable. Captured `mobile_bottom_nav_synth`: the node graph hogs the top ~46% of the screen, and the synth panel below stacks ~226px of fixed chrome (channel pill + Save/Save As/Reset header + the always-on focus-wave band + a 2-row chip strip + the "VOICE" detail header) before any knob — leaving **~46px for knobs**: one clipped, untappable row.

The phase-2 mobile "always-on focus band" (Task 19) is itself part of the problem: it's a full-width band stacked *above* the knob grid, consuming vertical space the knobs need.

## Goal

Re-arrange the mobile synth panel to mirror the desktop split, using horizontal space efficiently:
- **Pipeline (chip strip) on top, full width** (unchanged).
- **Knobs in a left column** that owns the full body height → at least one good-sized, tappable dial (scrolls for more).
- **Both wave graphs in a right column**, stacked: "Your sound" mirror on top, the selected-knob focus graph below — same as desktop.

## Decisions (locked with user)

- Move both waves from the top band into a **right column** beside the knobs.
- Knobs get a **left column** spanning the full body height (no wave band stealing vertical room above them).
- Chip strip stays on top, full width.
- Node graph above is **left unchanged** (out of scope; a separate follow-up if more room is wanted later).

## Architecture

The draw path already does exactly what we need — `drawSynthTab`'s mobile block (`synth_panel_zone.go:1536`) draws the mirror + focus graph stacked into `dv.instEditorMobileFocusRect` via `splitSynthRightPane` (top mirror / bottom focus). **We only change where that rect is** (top band → right column). The change is in layout, not draw.

### 1. `buildSynthTab` mobile branch (`synth_panel_zone.go:720-767`)

Currently carves a top band from `sectionsRect` and shrinks `sectionsRect.Min.Y` below it. **Remove the top-band carve.** The mobile branch becomes simply:

```go
	} else if mobile {
		dv.instEditorPreviewRect = image.Rectangle{}
		// Mobile: the "Your sound" mirror + focus graph live in a RIGHT column
		// beside the knob grid (set in layoutSynthSections once detailR is known),
		// NOT a top band. Cleared here; layoutSynthSections fills it on mobile.
		dv.instEditorMobileFocusRect = image.Rectangle{}
	} else {
```

`sectionsRect` is no longer shrunk from the top → the chip strip + detail/knob area get the full mobile panel height. The `minBandH`/`minGridH`/`SynthFocusGraphH` band-sizing block is deleted.

### 2. `layoutSynthSections` — split `detailR` into knob (left) + wave (right) on mobile

After `detailR` is computed (`synth_panel_zone.go:1245-1249`, full width below the chip strip) and BEFORE `dv.instEditorDetailR = detailR`, on mobile carve the right column:

```go
	if mobile && !detailR.Empty() {
		// Right column for the stacked waves (Your sound + focus graph); the knob
		// grid keeps the left column. Width ~43% of the detail body, clamped so the
		// knob column keeps room for one good-sized dial + label and the wave column
		// stays legible. If the body is too narrow for both, skip the split (knobs
		// take the whole body; waves are omitted on that pathological width).
		const minKnobColW, minWaveColW = 150, 110
		waveW := detailR.Dx() * 43 / 100
		if waveW > detailR.Dx()-minKnobColW {
			waveW = detailR.Dx() - minKnobColW
		}
		if waveW >= minWaveColW {
			dv.instEditorMobileFocusRect = image.Rect(
				detailR.Max.X-waveW, detailR.Min.Y, detailR.Max.X, detailR.Max.Y,
			)
			detailR.Max.X = dv.instEditorMobileFocusRect.Min.X - SpaceSM
		}
	}
	dv.instEditorDetailR = detailR
```

- `instEditorDetailR` (knob area + header) is now the **left column**; `placeKnobsInSection` lays knobs into it automatically (it reads `instEditorDetailR`; `ControlGrid.SetMaxCols(1)` on mobile keeps one dial per row in the narrower column, scrolling for more).
- `instEditorMobileFocusRect` is the **right column**, spanning the full body height (chip-bottom → panel-bottom) → the two stacked waves get real height.
- The detail header (`drawSynthDetailPane`) draws within `instEditorDetailR` → it sits over the knob (left) column. (Header-over-left, waves full-height on the right — gives the waves maximum height; acceptable per the mockup.)

### 3. `drawSynthTab` mobile draw block (`synth_panel_zone.go:1536-1543`)

**No logic change** — it already does `splitSynthRightPane(mb)` → mirror top / focus bottom into `instEditorMobileFocusRect`. Update the stale comment (lines 1534-1535) from "stacked band reserved at the top" to "right column beside the knob grid".

### 4. The chip-compaction floor (`minDetailH`, `synthChipWrapRows`, `layoutSynthSections:1194-1214`)

Keep as-is — the chip strip is still full-width on top and may wrap; the compaction guard still protects the detail body. Because the top band is gone, the body has more height, so deep wraps are less likely to trigger compaction.

## Testing (TDD)

- **Right-column geometry** (`internal/ui`, `//go:build test`): rebuild the mobile synth tab (existing `newMobileSynthTabGameForTest`/the `SetForceMobileProfile`+`Layout(390,…)` pattern). Replace `TestMobileSynthTab_HasFocusBandAboveGrid` with `TestMobileSynthTab_WavesRightOfKnobs`: assert `dv.synthMobileFocusRect()` is non-empty, its `Min.X >= dv.instEditorDetailR.Max.X` (right of the knob column), and it shares the detail body's Y-range (roughly full body height, not a thin top strip).
- **Knobs get real height**: assert the knob column (`instEditorDetailR`) body height (minus header) is now much larger than before (e.g. ≥ a configurable floor like `2×SynthKnobMin`), and that the first section's first knob lays out with a non-empty rect whose diameter ≥ a usable minimum.
- **Waves render in the right column**: keep `TestMobileSynthTab_FocusGraphRespondsToKnobSelect` but assert the ink lands inside the right-column rect (and that the mirror band is non-empty when the column is tall enough).
- **Desktop unaffected**: `instEditorMobileFocusRect` stays empty on desktop; the desktop preview-pane tests stay green.
- **Existing mobile guards**: `TestSynthChips_MobileWrapAllStagesVisible` + `TestSynthTab_KnobDragViaTreeDispatch_Mobile` must still pass (all stages visible; a knob drag via tree dispatch still changes the param). The detail-floor / chip-compaction code is preserved, so they should — update only if the increased body height changes a pinned expectation, with a one-line rationale.
- **Visual**: recapture `mobile_bottom_nav_synth` (full mobile synth tab) and confirm: chip strip on top full-width; a big tappable dial on the left; "Your sound" + focus graph stacked on the right. Also recapture the `crop_synth_focus_*` mobile variants (their `MobileSetup` now produces a right-column band) and confirm the focus graph renders there.

## Out of scope / YAGNI
- The node graph pane height (left unchanged per user).
- Header chrome compaction (Save/Save As/Reset, channel pill) — not needed to fix the knobs; leave as-is.
- No new density tokens (reuse the 43%/min-width constants inline; promote to tokens only if a second consumer appears).

## Notes
- Per repo policy the implementer does **not** `git add`/commit; changes stay in the working tree.
- Concurrent i18n/Settings/wave-tab WIP intermittently flap-breaks the `-tags test` build in shared files; verify with `-run` filters + `go vet`, never a clean full-suite run. Tell agents: do the work, verify with `-run`, STOP — no background log-watch loops.
- `instEditorMobileFocusRect`'s semantics change (top band → right column); update its doc comment + the `synthMobileFocusRect()` accessor comment. The field name is kept to avoid churn; the accessor stays.
