# Mobile Bottom-Pane Stable Height (tab-independent, row-snapped)

**Date:** 2026-06-15
**Branch:** add-core-node-types
**Status:** Design — approved (pending spec review)
**Builds on / amends:** `2026-06-15-mobile-bottom-pane-min-height-design.md` (the per-tab content floor).

## Problem

The mobile drum/audio pane height is currently computed **per active tab**: `adaptiveMobilePortraitSplitY` only applies the audio content floor when `MobileEQMode()` is true. So switching Pads ↔ an audio tab (or any tab whose content need differs) **resizes the bottom panel mid-use** — a confusing, janky jump.

Additionally, when the pane is sized to a content floor, the drum-rack can be left with a **partial-row blank strip** (e.g. room for 4.5 rows shows 4 rows + a dead gap).

## Goal

Compute **one stable pane height** that fits the tallest tab, apply it to **every** tab at **startup and on window resize** (never on tab switch), and **snap** it so the drum-rack renders a **whole number of rows** (no half-blank space).

## Decisions (locked with user)

- **Same height for all tabs**, including Pads (user choice). The split depends only on window size + row count — never on the active tab → zero resize on tab switch.
- **Snap to whole rows**: bump the height so the rack fills with the next full row when it fits (under the grid-≥-40% cap); otherwise trim down to the last whole row. No partial-row blank strip.
- Computed in `Game.Layout` (runs at startup + on resize), not gated on tab entry.
- Grid stays ≥ 40% on every tab (drum ≤ 60%).
- Accepted trade-off: on Pads with few rows, the rack may show one extra (empty) row slot to keep the height stable + clean — better than a mid-use resize.

## Architecture

All in `game_layout.go`, in `adaptiveMobilePortraitSplitY(h, g)`.

### 1. Remove the per-tab gate; unify the height

Replace the `if g.drum != nil && g.drum.MobileEQMode() { … }`-gated branch with an **always-applied** computation:

```go
	barH := TouchMinTarget()
	rowsNeed := headerH + (numRows+1)*rh + barH
	audioNeed := headerH + mobileAudioPanelMinContentH() + barH
	needed := rowsNeed
	if audioNeed > needed {
		needed = audioNeed
	}
	maxDrum := h * 60 / 100 // grid always >= 40% on EVERY tab
	minDrum := h * 30 / 100
	drumH := needed
	if drumH > maxDrum {
		drumH = maxDrum
	}
	if drumH < minDrum {
		drumH = minDrum
	}
	drumH = snapDrumHToWholeRows(drumH, h, headerH, barH, rh, maxDrum) // §2
	return h - drumH
```

`needed = max(rowsNeed, audioNeed)` and `maxDrum = 60%` are **tab-independent** (neither reads `currentViewMode`/`MobileEQMode`), so the split is identical across tabs. Because it runs inside `Layout`, it's (re)applied at startup and on every resize.

### 2. Row-snapping helper `snapDrumHToWholeRows`

```go
// snapDrumHToWholeRows adjusts the drum-pane height so its rack region (the area
// below the transport header and above the bottom action bar) holds a WHOLE number
// of drum rows — no partial-row blank strip. It rounds UP to include the next full
// row when that still fits under maxDrum; otherwise it rounds DOWN to the last whole
// row. Returns drumH unchanged when the rack region is already row-aligned or too
// small to hold a row.
func snapDrumHToWholeRows(drumH, h, headerH, barH, rh, maxDrum int) int {
	if rh <= 0 {
		return drumH
	}
	rackH := drumH - headerH - barH
	if rackH < rh {
		return drumH // not even one row — leave as is
	}
	rem := rackH % rh
	if rem == 0 {
		return drumH // already clean
	}
	up := drumH + (rh - rem) // bump to the next whole row
	if up <= maxDrum {
		return up
	}
	return drumH - rem // can't grow past the 40%-grid floor: trim to whole rows
}
```

> The rack region = `drumH - headerH - barH`. If the real rack also reserves a timeline strip inside the drum pane on mobile, subtract that too — the plan confirms the exact chrome the rack sits under by reading the rack/timeline layout; the helper takes the chrome as a parameter so the composition stays explicit and testable.

### 3. Cleanup of the prior per-tab behavior

- The old `PadsUnchanged` test is now wrong (Pads intentionally uses the unified height) — replace it with the stability test below.
- `mobileAudioPanelMinContentH()` is unchanged (still the synth-driven floor).

## Testing (TDD — write failing first)

- **`TestMobileSplit_StableAcrossTabs`** (the core no-resize guarantee): at a fixed `w,h` and fixed row count, `adaptiveMobilePortraitSplitY` returns the **same** value for `viewModeRows`, `viewModeSynth`, `viewModeSampler`, `viewModeEQ`, `viewModeWave`, `viewModeSpectrum`, `viewModeMeters`, `viewModeChain`. Fails today (Pads differs from audio tabs).
- **`TestMobileSplit_AppliedAtStartupAndOnResize`**: a fresh `Game.Layout(390, 844)` (forced mobile, auto-size) sets `split.Y` to the stable value immediately (not the default `h/2`); a second `Layout(390, 720)` recomputes it for the new height. Asserts it's driven by Layout, not tab entry.
- **`TestMobileSplit_RackShowsWholeRows`**: at several heights, the rack region (`drumH - headerH - barH`, with the real chrome) is an exact multiple of `rowHeight` (no partial-row remainder), and the snap never pushes the grid below 40%.
- **Keep** (now tab-independent): `GridAtLeast40Percent` (extended to Pads + all audio tabs), `DoesNotOverExpand` (drum ≤ the snapped max-tab-need), `KnobRendersAtIdealSize`, `SamplerControlsClearKnobRowMobile`.
- **Replace** `PadsUnchanged` with `StableAcrossTabs`.
- **Visual**: recapture `mobile_bottom_nav_pads`, `mobile_bottom_nav_synth`, `mobile_bottom_nav_sampler` — confirm the grid/pane proportion is identical across all three (no resize), the rack shows whole rows on Pads (no blank strip), the synth dial is full-size, and the sampler buttons are clear.

## Out of scope / YAGNI
- Desktop layout — unchanged (`adaptiveMobilePortraitSplitY` is mobile-only).
- User-draggable mobile divider — not requested.
- Per-tab *different* heights — explicitly removed (that's the bug).

## Notes
- Per repo policy the implementer does **not** `git add`/commit; changes stay in the working tree.
- Concurrent i18n/wave WIP intermittently flap-breaks the `-tags test` build and a background cleanup deletes `zz_*` scratch files — verify with `-run` filters + `go vet`, do NOT create throwaway probe files, do the work, verify, STOP.
- The test helper `newMobileSynthTabGameForTestSize` already sets `forceAutoSize` so `Layout` exercises `adaptiveMobilePortraitSplitY` under `go test` (added in the prior task) — reuse it; the startup/resize test depends on it.
