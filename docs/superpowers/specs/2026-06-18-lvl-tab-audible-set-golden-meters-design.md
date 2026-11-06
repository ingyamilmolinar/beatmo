# Lvl tab: audible-set strips, golden/orange/red meters, mobile bar room

**Date:** 2026-06-18
**Branch:** add-core-node-types
**Status:** Approved — ready for implementation plan

## Problem

The audio-panel **Lvl** (Levels) tab renders one vertical Peak/RMS strip for
**every** instrument plus master, regardless of what is actually audible. Three
issues:

1. It shows all instruments even when one is soloed or several are muted —
   noise, and the strips get cramped when there are many rows.
2. The green/yellow/red meter ramp, while technically on-palette, uses a cool
   mint green that does not read as part of the game's golden Vice City identity.
3. On mobile the Headroom/Clips/Loudest readout column eats horizontal space
   that would be better spent on the bars.

## Goals

- Lvl tab shows only the **audible** instruments + master, adapting to solo/mute.
- Meter zones use a **golden → orange → red** ramp, defined by codegen (DESIGN.md
  YAML → `make gen-design-tokens`), all exact Vice City palette members.
- On mobile, the bars get more width; the metrics column is compact/smaller.
- All fonts and sizes route through the design-system type scale + density tokens.
- Thorough TDD coverage for every change.

## Non-goals

- No change to the analyzer service, mixer, or how per-instrument metrics are
  computed. We only filter what the renderer draws.
- No change to the single-channel `drawLevelsDetail` empty/boot fallback paths
  beyond what falls out of the shared helper.
- No change to tab dispatch (`EnsureAnalyzersForTab` stays as is — all
  instruments remain analyzed; we filter at render time only).

## Current state (facts)

- Renderer: `drawLevelsMultiChannel(dst, rect, state *analyzer.State, latches)` in
  `src/go/internal/ui/render_meters.go:447`. Iterates `state.Instruments`
  (`[]analyzer.InstrumentMetrics{ID,Name,PeakDB,RMSDB,ClipCount,Active}`) and draws
  `state.Master` last at ~2× width. Strip width = `stripSpace / (nInst+2)`.
- Call site: `eq_panel_zone.go:638` — `drawLevelsMultiChannel(screen, z.bodyRect(), state, z.levelsLatches)`.
- Audibility predicate (canonical, duplicated inline): `!r.Muted && (!anySolo || r.Solo)`
  at `drumview_rows.go:267` (inside `autoSelectSoleAudibleChannel`).
- `EQCallbacks.ActiveRows() []*DrumRow` is wired to `dv.Rows` (ALL rows, not
  filtered) at `drumview_ctor.go:269`. Each `DrumRow` has `Solo bool`,
  `Muted bool`, `Instrument string` (instrument ID).
- Meter colors are already codegen:
  `genColorVizMeterGreen/Yellow/Red/Bg/Clip` in `design_tokens.gen.go`, sourced
  from `viz-meter-*` in DESIGN.md YAML (lines 112-116). Go aliases
  `meterGreen/meterYellow/meterRed` in `render_meters.go:17-19`.
- Current hexes are all palette members already: green `#3FD67A` = mint-300,
  yellow `#FFD319` = sunset-gold-200, red `#FF5C2A` = tangerine-400. So
  `TestPaletteMembership` already passes; the request is aesthetic.
- Density tokens (DESIGN.md): `levelsReadoutWFull` (compact 120 / comfortable 180
  / spacious 220), `levelsReadoutWIcons` (28 / 36 / 44), `levelsReadoutFooterH`.
  Mobile uses **Spacious**.
- Type scale: `FontSizeCaption=10, Small=12, Body=14, Label=16, Title=18,
  Heading=20` (`textcache.go:16`). The aggregate column currently uses raw
  `headlineScale = 2.0` and `bodyScale = 1.0` multipliers (`render_meters.go:974`).

## Design

### 1. Audible-set filtering

Extract the canonical predicate into shared, unit-testable helpers (UI package):

```go
func anySoloActive(rows []*DrumRow) bool          // true if any row.Solo
func rowAudible(r *DrumRow, anySolo bool) bool     // !r.Muted && (!anySolo || r.Solo)
func audibleInstrumentIDs(rows []*DrumRow) map[string]bool // set of audible row.Instrument
```

Reuse them in `autoSelectSoleAudibleChannel` (removes the duplicated inline scan).

Add a `visibleIDs map[string]bool` parameter to `drawLevelsMultiChannel`:

```go
func drawLevelsMultiChannel(dst, rect, state, latches, visibleIDs map[string]bool)
```

- `visibleIDs == nil` → show all (preserves existing callers/tests + the
  single-channel fallback paths).
- Otherwise the instrument loop skips any `inst.ID` not in `visibleIDs`. An
  instrument ID not present in any row (shouldn't happen) is treated as visible
  (don't hide unclassifiable data). Master is always drawn.
- The existing `nInst`-based width math already widens strips when the filtered
  count drops — no extra layout work.

`EQPanelZone.Draw` (Lvl case) computes the set from `z.callbacks.ActiveRows()`
and passes it down. Idle (no solo, no mute) → predicate true for all → all show.

### 2. Golden/orange/red meter colors (codegen)

Rename the three role tokens in DESIGN.md so the name matches the new hue (a
green-named golden token is a footgun), keeping `viz-meter-bg`/`viz-meter-clip`:

| Old token | New token | Hex | Palette member | Role |
|---|---|---|---|---|
| `viz-meter-green` | `viz-meter-low` | `#FFB30A` | sunset-gold-300 (game primary) | safe/low |
| `viz-meter-yellow` | `viz-meter-mid` | `#FF7A3D` | tangerine-300 | warn/mid |
| `viz-meter-red` | `viz-meter-high` | `#E84A1F` | tangerine-500 | hot/high |

Then:
- `make gen-design-tokens` regenerates `design_tokens.gen.go`
  (`genColorVizMeterLow/Mid/High`).
- Rename Go aliases `meterGreen/Yellow/Red` → `meterLow/Mid/High` in
  `render_meters.go` and update all references (~10 call sites incl.
  `meterColor`, `meterColorAlpha`, headroom color logic, segmented bar).
- Update the want-map in `design_md_drift_test.go` to the renamed tokens/hexes.
- All three hexes are exact palette members → `TestPaletteMembership` stays green.

`meterColor(db)` thresholds unchanged (`> meterRedDB` → high, `> meterYellowDB`
→ mid, else low). Headroom color logic in `drawLevelsAggregates` /
`drawLevelsAggregatesIconRow` keeps its good/warn/bad mapping (now gold/orange/red).

### 3. Mobile: wider bars, smaller metrics

- Reduce Spacious `levelsReadoutWIcons` 44 → 34 (DESIGN.md `densities.spacious`,
  regenerate) so the strips reclaim the width. Tighten the icon-row padding in
  `drawLevelsAggregatesIconRow`.
- The strip-width math then yields wider bars automatically (more space left of
  the readout column).
- Verify the readout-mode thresholds still land the mobile panel in icon mode
  (not chevron) at typical mobile widths after the token change.

### 4. Type-scale / token compliance

- Replace raw multipliers in `levelsAggregateColumnRows`: `headlineScale = 2.0`
  → `FontSizeHeading / FontSizeBody` (≈1.43); `bodyScale` stays `1.0` (= Body)
  but expressed via the constant; caption rows keep `FontSizeCaption/FontSizeBody`.
- This also shrinks the "big headroom" number, helping the mobile compactness goal.
- No new inline `color.RGBA` literals (keep `TestTokenDiscipline` green); all
  colors via tokens.

## Testing (TDD — written before implementation)

1. **Helper unit tests** (fast path): `audibleInstrumentIDs` / `rowAudible` /
   `anySoloActive`:
   - no solo, none muted → all IDs present
   - single solo → only soloed ID
   - multi-solo → exactly the soloed set
   - mute (no solo) → excludes muted
   - solo + mute interaction (muted-and-soloed row excluded)
   - empty rows / row with empty Instrument ID
2. **Renderer filter test**: `drawLevelsMultiChannel` with a `visibleIDs` set
   draws only matching instrument strips + master. Assert via the strip/`drawRect`
   interceptor pattern (`audio_panel_render_pixels_test.go` / `drawRect` hook).
   Include `visibleIDs == nil` → all strips (back-compat).
3. **Color codegen tests**: extend `design_md_drift_test.go` want-map for
   `viz-meter-low/mid/high`; assert `meterLow/Mid/High` equal the regenerated gen
   constants; run `TestPaletteMembership` + `TestDesignMDDrift` +
   `TestTokenDiscipline`.
4. **Mobile layout tests**: density-token test for reduced spacious
   `levelsReadoutWIcons`; a layout assertion that filtered/mobile bar width is
   greater than the pre-change baseline width; readout-mode-selection test at a
   representative mobile width.
5. **Type-scale test**: assert the aggregate "headline" row scale derives from
   the `FontSize*` constants (no literal `2.0`).
6. **Integration (ebitenstub)**: drive the real EQ panel with soloed/muted rows
   through the Draw path and assert the rendered strip set matches the audible set.

## Validation commands

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|TestPaletteMembership' ./internal/ui/
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/analyzer/
make gen-design-tokens   # after DESIGN.md edits; confirm gen file matches
```

## Risks / notes

- Renaming `meterGreen/Yellow/Red` touches ~10 call sites — mechanical but must
  be complete; the build catches misses.
- `make gen-design-tokens` must be run and the generated file committed; the
  pre-commit hook rejects stale `*.gen.go`.
- Heavy concurrent external churn on this branch has historically deleted
  untracked test/doc files mid-task (see project memory) — re-check `git status`
  of own files before finishing; rewrite rather than `git restore`.
