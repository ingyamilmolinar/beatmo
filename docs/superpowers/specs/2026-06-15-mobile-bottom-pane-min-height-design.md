# Mobile Bottom-Pane Minimum Height (fit content, keep grid ≥ 40%)

**Date:** 2026-06-15
**Branch:** add-core-node-types
**Status:** Design — approved (pending spec review)
**Builds on:** the mobile synth side-by-side layout (`2026-06-15-mobile-synth-side-by-side-design.md`).

## Problem

On mobile, audio-tab content (Synth knobs, Sampler controls) is cramped: the synth knob shrinks to `SynthKnobMin` (~40px) instead of `SynthKnobIdeal` (~88px), and the Sampler's knob sub-labels ("Start 0%", "Pitch +0 st") overlap the Rev/Norm/Fade/Preview/Save row.

**Root cause (corrected from the initial mapping):** the audio panel lives inside the **drum pane**, whose height is set at the **Game level** by `adaptiveMobilePortraitSplitY` (`game_layout.go:176`). On mobile that function sizes the drum pane from **drum-row count** (`headerH + (numRows+1)*rowH + barH`, capped 65% / floored 30%) — it ignores that on an audio tab the pane hosts the synth/sampler panel, which needs more height than the rows. So the pane is too short and the content compresses. (The DrumView-internal `eqRect` already expands to fill the pane in `MobileEQMode`; the pane itself is the constraint.)

## Goal

On a mobile audio tab, size the drum pane to comfortably fit the active tab's content (full-size knob + chrome; un-overlapped sampler buttons) — **and no larger than necessary** — while **keeping the node graph always visible at ≥ 40% of the screen height**.

## Decisions (locked with user)

- **Grid always ≥ 40%** of total height → drum pane ≤ 60%.
- **Don't over-expand**: the drum pane takes exactly the content's comfortable minimum, capped at 60%. If the content needs less than 60%, the grid gets *more* than 40%.
- Applies to the **mobile audio-tab path** (`MobileEQMode()` true: Synth/Sampler/EQ/Wave/Spectrum/Levels/Chain). The **Pads** view (drum rows) keeps its existing rows-based sizing — out of scope.
- Synth is the tallest content, so its comfortable minimum drives the floor; the same floor is applied on every audio tab (consistent, and the analysis tabs already want a tall panel).

## Architecture

All changes in `internal/ui`, centered on `game_layout.go`'s mobile split.

### 1. `mobileAudioPanelMinH(tab PanelTab) int` (new, `internal/ui` — near `adaptiveMobilePortraitSplitY` or in `panel_tab.go`)

Returns the comfortable minimum **content** height (excluding the transport header + bottom bar) for the active mobile audio tab, computed from density tokens (`Profile().DensityValues()`):

- **Synth**: chip-strip allowance + `SynthDetailHeaderH` + one **full** knob cell (`SynthKnobIdeal + SynthKnobCaptionH`) + section paddings. This is the value that guarantees the knob renders at `SynthKnobIdeal`, not the shrunk `SynthKnobMin`. (The waves are a side column — they don't add height.)
- **Sampler**: waveform band + one knob-row (full) + the control-button row (`TouchMinTarget`) + the readout line + paddings, so the sub-labels clear the button row.
- **Other audio tabs (EQ/Wave/Spectrum/Levels/Chain)**: a sensible shared default (they're not the constraint; use the Synth value so every audio tab is consistently roomy).

Keep the composition readable and token-derived; document each addend with a one-line comment. The exact constants come from existing tokens — no new design tokens unless a value genuinely has no token.

### 2. `adaptiveMobilePortraitSplitY` (`game_layout.go:176`) — honor the audio-tab floor + grid cap

When `g.drum != nil && g.drum.MobileEQMode()`:

```go
	transportH := headerH // the mobile transport header already computed above
	barH := TouchMinTarget()
	tab := g.drum.eqPanelZone.ActiveTab() // guard nils
	contentNeed := transportH + mobileAudioPanelMinH(tab) + barH
	maxDrum := h * 60 / 100 // grid always >= 40%
	drumH := contentNeed
	if drumH > maxDrum {
		drumH = maxDrum // content can't push the grid below 40%
	}
	// "Don't over-expand": when content needs less than the cap, the grid keeps
	// the surplus (drumH stays at contentNeed, grid = h - drumH > 40%).
	return h - drumH
```

For the **non-audio (Pads)** path, the existing rows-based computation is unchanged.

Notes:
- `maxDrum = 60%` replaces the audio-path use of the old 65% cap (the 65% stays for Pads). The grid therefore never drops below 40% on audio tabs.
- No explicit min-drum floor is needed on the audio path — `contentNeed` is substantial; but keep the result ≥ the old `minDrum` (30%) defensively so a pathologically small `contentNeed` can't shrink the panel.
- `MobileEQMode()` / `eqPanelZone.ActiveTab()` must be reachable from `game_layout.go` (same package). Guard `g.drum.eqPanelZone == nil`.

### 3. The pane already fills its space

`DrumView`'s `eqRect` already expands to `headerH → bottomActionBar` in `MobileEQMode` (`drumview_layout.go:755`), and the synth side-by-side layout (`layoutSynthSections`) lays knobs into the taller column automatically. So once the pane is taller, the knob cell gets `SynthKnobIdeal` (the cell-height clamp in `placeKnobsInSection` stops shrinking the dial) and the sampler button row gets its own row below the knobs. **No change needed in the panel-internal layout** beyond what already exists — this is purely the Game-split height.

## Testing (TDD — the QoL guarantee)

- **Grid ≥ 40% on audio tabs** (`game_layout` test): at 390×720 and 390×844, build the mobile Synth tab and Sampler tab; assert `gridHeight >= 0.40*h` (i.e. `split.Y >= 0.40*h`) and the grid pane is non-empty (always visible).
- **Don't over-expand**: assert `drumPaneHeight <= max(contentNeed, …)` — i.e. when content fits under 60%, the grid keeps the surplus (`split.Y > 0.40*h` strictly, drum == contentNeed). Pick a tall viewport where contentNeed < 60% to prove the pane doesn't balloon to 60%.
- **Full-size synth knob** (`internal/ui`): on the mobile Synth tab at 390×844, the first laid-out knob's diameter == `Profile().DensityValues().SynthKnobIdeal` (NOT shrunk to `SynthKnobMin`). At 390×720 (short), assert it's ≥ a usable threshold even if the 60% cap prevents full ideal — document the floor.
- **Sampler buttons don't overlap** (extend `TestSamplerButtonsDoNotOverlapMobile`, `sampler_panel_zone.go` test): at 390×720/844, no two laid-out control buttons intersect, AND the button-row rect does not intersect the knob-row rect (the sub-label/button overlap this fixes).
- **Pads unchanged**: the Pads (rows) split is byte-for-byte the same — a regression test asserting `adaptiveMobilePortraitSplitY` for `viewModeRows` returns the same value as before.
- **Visual**: recapture `mobile_bottom_nav_synth` (full-size dial, grid still ≥40% visible on top) and `mobile_bottom_nav_sampler` (buttons clear of the knob labels). Read both and confirm.

## Out of scope / YAGNI
- Pads (drum-rows) sizing — unchanged.
- Per-tab *different* floors beyond Synth-drives-all (revisit only if a tab needs more than Synth).
- User-draggable mobile grid/panel divider — not requested.

## Notes
- Per repo policy the implementer does **not** `git add`/commit; changes stay in the working tree.
- Active concurrent i18n/wave WIP intermittently flap-breaks the `-tags test` build and a background cleanup deletes `zz_*` scratch files — verify with `-run` filters + `go vet`; do the work, verify, STOP (no background log-watch loops, no throwaway probe files that the cleanup will eat).
- `headerH` inside `adaptiveMobilePortraitSplitY` is the mobile transport header (`Profile().HeaderMaxH` / `mobileHeaderH`) already computed there — reuse it; don't recompute.
