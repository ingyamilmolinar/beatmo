# Cross-Tab Pause/Play (Freeze) — Wave · Spectrum · Levels · Chain

Date: 2026-06-15
Branch: add-core-node-types
Status: Approved design — ready for implementation plan

## Goal

Give every signal-display audio-panel tab (Wave, Spectrum, Levels, Chain) a
**pause/play button** that freezes/unfreezes the signal shown *in that tab only*
(independent per-tab freeze), rendered with **pause/play icons** (pause while
live → click to freeze; play while frozen → click to resume). Freezing is
**visual only** — it never affects audio output.

## Decisions (from brainstorming)

- **Independent per-tab freeze.** Pausing one tab does not freeze the others.
  (Today Wave/Spectrum/Levels share one global analyzer freeze; this changes.)
- **Icon mapping:** live → `IconPause` (action = pause), frozen → `IconPlay`
  (action = resume). Replaces the `||` / `>` text glyphs.
- Levels must *visibly* hold when frozen.

## Current state (as found)

- **Icons already exist.** `IconPause`, `IconPlay` in `icons.go`. `Button`
  supports icons: set `btn.Icon = string(IconPause)` + clear `Text`; `Button.Draw`
  sprite-renders it (transport bar already does this). `IconColor` overrides tint.
- **Freeze pill (text).** `audio_tab_controls.go`: `newFreezePill` builds a
  `NewButton("||", …)`; `applyFreezeVisual(b, frozen)` toggles text `||`↔`>` and
  color `colTextSecondary`↔`colAccent`. `freezeControls` (embedded by
  `levelsControls`, `spectrumControls`) owns `freezeBtn`, `frozen bool`,
  `SyncFreeze`, `layoutFreezePill`. **`waveControls` has NO freeze pill** (it was
  replaced by the AUTO pill); its `SyncFreeze` is a no-op.
- **Analyzer freeze is GLOBAL today.** `EQCallbacks.OnFreezeToggle`
  (`eq_panel_zone.go:62`) → wired in `drumview_ctor.go:315` to
  `audio.AnalyzerService().Freeze()/Unfreeze()` (desktop) or a single zone-local
  `frozenAnalyzer *analyzer.State` snapshot (WASM). One global
  `analyzer.State.Capture.Frozen` is shared by Wave/Spectrum/Levels.
- **Renderers read `*analyzer.State`** via `getAnalyzerStateForTab(tab)`
  (`eq_panel_zone.go:2291`), which pulls live data from
  `z.callbacks.AnalyzerState()` / `AnalyzerMetricsOnly()` and memoizes per-frame.
- **`analyzer.State` is an atomically-published immutable snapshot**
  (`analyzer/state.go:7`): the service hands back a fresh pointer per refresh and
  does not mutate held ones. ⇒ holding the current pointer = a stable frozen
  moment (no deep clone required).
- **Chain already has independent scope freeze.** `chain_panel_zone.go`:
  `freezeBtn` + `frozen` + `SetFrozen` → its own `OnFreezeToggle`
  (`drumview_ctor.go:481`) → `audio.ScopeService().Freeze()` (a separate capture
  only Chain consumes). Drawn via text glyph.
- **Levels freeze is not visibly obvious.** `render_meters.go` reads peak-hold
  latch ballistics (`MultiLevelsLatch`), which keep decaying regardless of the
  frozen flag, so a freeze doesn't clearly hold the meters.

## Design

### 1. Icon-ize the freeze pill (all four tabs)

`applyFreezeVisual(b, frozen)` (shared by Levels/Spectrum/Wave) sets:
```
if frozen { b.Icon = string(IconPlay);  b.Text = ""; b.IconColor = colAccent }       // resume
else      { b.Icon = string(IconPause); b.Text = ""; b.IconColor = colTextSecondary } // pause
```
Chain's button visual (in `SetFrozen` / its draw path) does the same. The
`||`/`>` text glyphs are removed (one fewer DESIGN.md text-glyph exception).
Pill width keeps using `AudioPillIconW` (square-ish icon pill).

### 2. Re-add a freeze pill to the Wave tab

`waveControls` gains a freeze pill **alongside** the AUTO pill. Layout: freeze
flush-right via `layoutRightPill` (returns the running `rightEdge`), AUTO placed
to its left with the same helper. `waveControls.SyncFreeze` stops being a no-op:
it drives the freeze pill icon/visual from the Wave tab's frozen flag. Hit area
tag `wave-freeze-btn` (alongside the existing `wave-auto-btn`). Auto-gain and
freeze are orthogonal — a frozen Wave still honors the AUTO scale on the frozen
samples.

### 3. Independent per-tab analyzer freeze (the core change)

Replace the single global analyzer freeze with per-tab frozen **snapshots** held
in `EQPanelZone`:

- New field: `frozenByTab map[PanelTab]*analyzer.State` (entries only for
  Wave/Spectrum/Levels). Replaces the single `frozenAnalyzer`.
- `getAnalyzerStateForTab(tab)`: **if `frozenByTab[tab] != nil`, return it**
  (the frozen snapshot); else resolve live as today. (Insert ahead of the live
  resolution; keep the per-frame memo keyed by tab.)
- New method `toggleTabFreeze(tab PanelTab) bool`:
  - frozen now → `delete(frozenByTab, tab)`; return `false`.
  - else → snapshot the current live state and store it; return `true`.
  - snapshot = the pointer returned by `z.callbacks.AnalyzerState()` (or
    `AnalyzerMetricsOnly()` for Levels) — an immutable published snapshot, so
    holding it freezes that tab's data while the service keeps producing fresh
    states for the live tabs. *(Plan verifies pointer stability on both desktop
    and WASM; falls back to a deep `analyzer.State` snapshot helper only if the
    published pointer is not stable.)*
- The audio analyzer service is **never frozen** for these tabs — audio and the
  other tabs stay live. (Strictly safer than the old `AnalyzerService().Freeze()`.)

### 4. Per-tab freeze wiring

Each analyzer tab's freeze pill toggles **its own** freeze, not a global one:

- `newWaveControls` / `newLevelsControls` / `newSpectrumControls` freeze buttons
  call a per-tab toggle the zone supplies, e.g. a closure
  `func() bool { return z.toggleTabFreeze(TabWave) }`.
- The control component keeps its own `frozen bool` (the button's highlight
  source of truth), set from the toggle's return value, and kept in sync each
  draw via `SyncFreeze(z.frozenByTab[activeTab] != nil)`.
- The old global `EQCallbacks.OnFreezeToggle` is no longer used by the analyzer
  tabs. Keep the callback field if other code references it, but the analyzer
  freeze pills route through `toggleTabFreeze`. **Audit** all callers of
  `OnFreezeToggle`, `AnalyzerService().Freeze/Unfreeze`, and
  `Capture.Frozen` (scene catalog, JS exports, parity/freeze tests) and migrate
  or preserve them so nothing silently depends on the global behavior.

### 5. Chain tab

Chain keeps its existing independent scope freeze mechanism (`ScopeService`) —
only its button visual changes to the pause/play icon (item 1). No data-path
change. (Chain is already independent of the analyzer tabs.)

### 6. Levels visibly holds when frozen

When `TabMeters` is frozen, `getAnalyzerStateForTab(TabMeters)` returns the same
frozen snapshot every frame, so the latches are fed constant values. To make the
hold **immediate** (not a multi-frame ballistic settle), on the frozen branch:
snap each `MultiLevelsLatch` entry to the frozen snapshot's peak/RMS instead of
running the decay update (a "freeze = hold displayed value" path in the Levels
draw block). On resume, normal latch updates continue.

### 7. Renderer frozen indicator

The Wave renderer currently draws a `FROZEN` tag from `capture.Frozen`. With
per-tab snapshots the capture's own flag is no longer the freeze signal. Drive
the indicator (and any Spectrum/Levels "frozen" affordance) from the tab's frozen
state instead — simplest: when a tab is frozen, the pause→play icon + accent
highlight already communicate it, so the in-panel `FROZEN` text tag is optional.
Keep the Wave `FROZEN` tag but source it from the tab-frozen flag (pass a
`frozen bool` the zone already knows) rather than `capture.Frozen`, so it stays
correct. The gain badge (`AUTO xN`) yields the slot to `FROZEN` when frozen, as
today.

## Components / boundaries

| Unit | Responsibility |
|---|---|
| `applyFreezeVisual` | Map frozen bool → pause/play icon + color on a `*Button`. Pure-ish, shared. |
| `waveControls` / `levelsControls` / `spectrumControls` | Own a freeze pill + `frozen` flag; call `toggleTabFreeze`; sync icon. |
| `EQPanelZone.frozenByTab` + `toggleTabFreeze` + `getAnalyzerStateForTab` | Per-tab frozen snapshot store + routing. |
| `ChainPanelZone` | Unchanged scope freeze; icon visual only. |
| Levels draw block | On frozen, snap latches to held values. |

## Testing

Go fast path (`-tags test`):
- **Icon toggle:** `applyFreezeVisual(b,false)` → `b.Icon==IconPause`, `Text==""`;
  `(b,true)` → `b.Icon==IconPlay`. Chain button likewise.
- **Per-tab independence:** freeze Wave via `toggleTabFreeze(TabWave)` →
  `getAnalyzerStateForTab(TabWave)` returns the held snapshot while
  `getAnalyzerStateForTab(TabSpectrum)`/`TabMeters` still return live; freezing
  Chain leaves all analyzer tabs live and vice versa.
- **Snapshot is stable:** after freezing a tab, advancing the live state
  (new pointer from the callback) does not change what that tab renders.
- **Wave has both pills:** `wave-auto-btn` and `wave-freeze-btn` hit areas both
  present; freezing doesn't disable AUTO.
- **Levels hold:** with a frozen snapshot, the Levels latch values equal the
  frozen peak/RMS and don't drift across frames.
- **Audio untouched:** freezing never calls an audio-output path (assert the UI
  freeze toggles only the zone snapshot / scope service, not playback).
- **Resume:** unfreezing a tab returns it to live state.
- Update existing freeze tests that asserted the global/text-glyph behavior
  (`TestWaveControlsOwnFreeze` already updated to AUTO — re-add freeze
  assertions; Levels/Spectrum freeze tests; any `Capture.Frozen` global tests).

Browser: re-run `audio_panel_render.browser.test.js` (icon render + tabs still
draw). Bridge smoke if a JS freeze export changes shape.

## Out of scope (YAGNI)

- No persistence of frozen state across sessions (session-only).
- No freeze for non-signal tabs (EQ, Synth, Sampler).
- No change to Chain's freeze data path (icon only).
- No new design tokens (reuse `IconPause`/`IconPlay`, existing colors/widths).

## Risks / notes

- **`OnFreezeToggle` / `AnalyzerService().Freeze` callers** must be audited
  (scenes, JS exports, parity). Migrating the analyzer tabs off the global freeze
  is the main risk; Chain and audio are untouched.
- **Pointer-stability assumption** for the snapshot — verified in the plan; deep
  snapshot fallback specified if needed.
- Token discipline: icon-only changes add no color literals (use existing `col*`).
- Concurrency: another agent is actively editing `internal/ui`/`internal/audio`;
  verify own work via `-run` filters and ride out transient build breaks.
