# Timeline Navigation UX — Menu-hosted Precise Navigation

**Date:** 2026-07-18
**Status:** Approved design, pre-implementation
**Supersedes (in part):** the inline FF/RW buttons + `×N` pill added in the timeline-navigation feature — those move into the Navigation panel described here.

## Problem

The transport bar is crowded, and the current seek model — single-clicking a spot on the timeline ribbon — is imprecise ("clicking at random places is not good UX"). Navigation should be **intentional and precise**: jump a chosen number of beats/subdivisions, go to a specific beat, or go to the beat nearest a timestamp. The controls should live in a more sophisticated overflow/kebab menu that works identically on desktop and mobile, decluttering the transport.

## Goals

1. Precise, intentional timeline navigation on **both platforms**, unified (no desktop/mobile divergence).
2. Host navigation in a **sectioned overflow menu** that scales to more option groups over time.
3. Declutter the transport bar.
4. The timeline ribbon becomes a small **progress indicator**, not an imprecise seek surface.

## Decisions (settled with user)

| Question | Decision |
|---|---|
| Menu structure | Sectioned bottom sheet / popover ("Options"): NAVIGATION section on top, FILE section below. Replaces today's flat "File" list. |
| "Timeline selection" | Point-seek (single playhead position). No range/loop selection. |
| Platform scope | Both platforms use the same menu-hosted navigation. |
| Transport/ribbon | Remove inline RW/FF + `×N`; add one **Navigate** button that opens the Options sheet at NAVIGATION. Ribbon = drag-to-scrub only (no single-click seek). |
| Ribbon role | Keep the moving playhead, but **smaller** and **decoupled from the drum-grid window length** — a compact "progress through the song from the beginning" bar. |

## Architecture

### The Options menu (sectioned)

Today's overflow menu (`OpenOverflowMenu`, `overflowItems`, `drumview_context_menu.go`) renders a flat list (Upload/Import/Export/Load template). It becomes a **sectioned** surface: an ordered list of `(sectionTitle, []entry)` groups rendered with a section header + divider between groups. Section 1 = NAVIGATION (interactive controls), Section 2 = FILE (the existing action rows, unchanged). The sheet gains a title ("Options") and keeps the drag handle + close (X). Same bottom-sheet on mobile, same anchored popover on desktop.

The NAVIGATION section hosts **interactive controls**, not plain action rows — so the section renderer must support an embedded control panel, not only icon+label buttons. This is the one piece of new menu machinery.

### The NAVIGATION panel

A self-contained component (`NavigationPanel`) rendered inside the NAVIGATION section, top-to-bottom:

1. **Position readout** — `Beat <b> · <m:ss>`. Reads `playheadAbsSubdiv()` (beat) and derives elapsed time from the clock authority (`beat / bpm * 60`). Read-only display.
2. **Jump by amount** — an editable **amount** (default 4) + a **unit toggle** with two states, **Beats** and **Subdivisions**, plus `◀ Prev` / `Next ▶` buttons. Prev/Next jump backward/forward by `amount` in the chosen unit:
   - Beats: `RequestSeekDeltaBeats(±amount)`.
   - Subdivisions: a new `RequestSeekDeltaSteps(±amount)` (delta in subdivision steps), resolved in the drain against the current playhead.
   The amount is a small dropdown of `{1,2,4,8,16}` reusing the `SubdivMenuComponent` idiom (the same control the old `×N` pill used), so it works identically on touch and desktop.
3. **Go to beat** — a numeric field accepting `beat` or `beat.subdivision`, parsed by the existing `positionValueSpec` grammar; on commit → `RequestSeekSteps(beat*div + sub)`.
4. **Go to time** — an `M:SS` field. On commit, parse to seconds, compute `targetSteps = ceil(seconds * bpm / 60) * div` (the closest beat **at or after** the timestamp), clamp to `[0, timelineBeats*div]`, → `RequestSeekSteps(targetSteps)`.

All jumps funnel through the existing `DrumView.RequestSeek*` → drain → `SeekSteps` chokepoint, so clock/timeline/predictor stay consistent and the O(idx) seek perf fix applies. The stopped-tracking + lock behavior applies unchanged (a menu jump follows the playhead when the tracking lock is on).

### Transport bar changes

- **Remove**: `rwBtn`, `ffBtn`, `jumpBtn` (`×N`) from `TransportZone` (construction, layout, hit areas, Draw). Their layout slots are reclaimed (transport declutters). The `SubdivMenuComponent` jump-size instance and its wiring move to the panel.
- **Add**: a single `navBtn` (a target/crosshair `IconID`, new) in the transport run on both platforms; `OnClick` opens the Options sheet with the NAVIGATION section in view.
- **Keep**: the tappable timeline **beat counter** — it also opens the Options sheet at NAVIGATION (instead of the standalone position popup).
- The keyboard seek shortcuts (`.` `,` Home) and the JS `seekSteps`/`playheadStep` exports are unaffected (they call the same chokepoint).

### Timeline ribbon changes

- Keep the moving **playhead cursor**, but render the ribbon **smaller** (reduced height) and as a **whole-song progress bar**: its scale spans `[0, timelineBeats]` (or the song's committed range) with the cursor at `playhead / total`, **decoupled from `dv.Length`** (it no longer mirrors the editable drum-grid window). Remove the view-rect that highlighted the pattern window.
- **Remove single-click-to-seek** (`timelineScrubHitAdapter` OnPress/OnRelease seek + the ghost). **Keep drag-to-scrub** for coarse movement (drag still commits a seek through the chokepoint). Grid-drag / two-finger view pan unchanged.

## Cross-platform behavior

Identical navigation on both platforms: the Navigate button + Options sheet's NAVIGATION section are the single home for precise navigation. Desktop and mobile differ only in the sheet's presentation (anchored popover vs. bottom sheet) — already handled by the existing overflow/portal layout.

## Reuse / what changes

- **Reuses:** `SeekSteps` chokepoint + `RequestSeek*` drain, `positionValueSpec` (beat.subdiv parse), the overflow menu + portal, the `SubdivMenuComponent` dropdown idiom, the FF/RW icons (now Prev/Next inside the panel).
- **Removes from transport:** inline RW/FF/`×N` (superseding that part of the prior feature).
- **New:** sectioned-menu rendering, `NavigationPanel` component, `RequestSeekDeltaSteps`, a `navBtn` + navigate icon, the progress-bar ribbon simplification.

## Testing

Functional tests driving `Game.Update` (per the repo's functional-test convention):
- Jump-by-N in **Beats** vs **Subdivisions** (prev/next move the playhead by the right amount/unit).
- **Go to beat** (`17`, `17.3`) and **Go to time** (`0:30` → closest beat at/after; verify the `ceil(sec*bpm/60)` rounding and clamping at song bounds).
- Sectioned menu renders both NAVIGATION and FILE sections; Navigate button and beat counter both open it at NAVIGATION.
- Ribbon single-click no longer seeks; drag-to-scrub still seeks; ribbon height reduced and progress independent of `dv.Length`.
- Layout guards: transport no longer contains rw/ff/jump; the navBtn is present and input-reachable on both platforms.
- Visual screenshots (desktop + mobile): the Options sheet with the NAVIGATION panel, and the smaller progress ribbon.

## Risks

- **Sectioned menu with embedded interactive controls** is the main new surface — the overflow renderer currently assumes uniform action rows. Mitigation: keep FILE rows exactly as-is; add a section-with-panel path only for NAVIGATION.
- **Removing single-click ribbon seek** changes an existing behavior (and its e2e/tests) — the tests that asserted click-seek move to assert the panel/drag paths.
- **Removing inline FF/RW/`×N`** reverts recent transport work; the underlying jump logic and icons are retained, so it's a relocation, not a loss.
