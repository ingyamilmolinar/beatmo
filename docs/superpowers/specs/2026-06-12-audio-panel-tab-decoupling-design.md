# Audio-Panel Tab Decoupling — Design

**Date:** 2026-06-12
**Branch:** add-core-node-types
**Status:** Approved (pending spec review)

## Problem

On desktop, the audio panel's `AudioStickyBar` crams everything into one 26px row
and reveals/hides controls per active tab via visibility-gating:

```
[Master▾] [EQ][Wave][Spec][Lvl][Chn][Syn]   …spacer…   [slope][pre][R][log] [freeze] [X] [?] [⌄]
```

This couples unrelated concerns into one shared component. The slope/pre/reset/
freq-scale pills (Spectrum), Clear-Clips/K-20 pills (Levels), and the freeze
toggle (Wave/Spectrum/Levels) all live as fields on `AudioStickyBar` with
`if activeTab == TabXxx` layout/draw branches. Reasoning about any one tab means
reading the whole shared bar. The X close button is a **dead no-op on desktop**
(`EQCallbacks.OnClose` is never wired in `drumview_ctor.go`, so the handler's
nil-check skips it).

By contrast, `ChainPanelZone` and the Synth header already own their own controls
as self-contained component trees — the model this design generalizes.

## Principle

The audio panel is a **dispatcher + N independent tab windows**. The dispatcher
owns only the shared switcher row. The entire area below that row is handed to the
active tab's component, which owns *everything* inside it: its own control buttons,
layout, draw, and hit areas. No tab reaches into another; no component is
"shown/hidden per tab" — a tab's chrome exists only inside that tab's own tree.

## What stays shared (the switcher row only)

`AudioStickyBar`, slimmed to four elements, all genuinely cross-tab:

- **channel pill** (`channelBtn`) — global single-source-of-truth channel selector
  (`selectAudioChannel`); every tab follows it.
- **tab pills** (`tabBtns`) — the switcher itself.
- **`?` legend** (`legendBtn`) — contextual help; text is per-tab data
  (`audioPanelLegendText[PanelTab]`) but the chrome is one shared button.
- **expander chevron** (`expanderBtn`) — panel collapse/expand
  (`PanelTabState.ToggleExpanded`).

The channel dropdown portal/overlay and the legend popover remain dispatcher-owned
(switcher-row concerns).

### Removed

- **X close button** (`closeBtn`) — deleted entirely (dead no-op on desktop). The
  `EQCallbacks.OnClose` field and its `initButtons` wiring are removed; the
  Chain zone keeps its own independent close (it switches back to the EQ tab —
  `drumview_ctor.go:472`, unaffected).

## What each tab owns (self-contained, modeled on `ChainPanelZone`)

This effort extracts **Wave, Spectrum, Levels**. EQ/Chain/Synth/Sampler are
unchanged (Chain/Synth already self-contained; EQ's controls already live inside
its content area, not on the shared bar, so slimming the bar leaves EQ untouched).

| Tab | New owner (controls component) | Controls it owns | Content (dispatcher draws into body) |
|---|---|---|---|
| Wave | `waveControls` | freeze | `drawAnalyzerWaveform` |
| Spectrum | `spectrumControls` | freeze, freq-scale (log/lin), slope (0/3/4.5 dB/oct), pre overlay, reset-hold | `drawAnalyzerSpectrumWithScale` |
| Levels | `levelsControls` | freeze, Clear-Clips, K-20 | `drawLevelsMultiChannel` |
| EQ | *(unchanged — dispatcher, fast-follow)* | HPF/LPF, band mutes, dB inputs, curve drag | `drawSpectrumBars` + `drawEQCurve` |
| Chain | `ChainPanelZone` *(unchanged)* | OVR/SPL/DIF, AG, FIT, freeze, close | `drawChainTraces` |
| Synth / Sampler | DrumView header *(unchanged)* | Save/Save As/Reset/Preview | `DrawSynthTab` |

The `tabControls` components are implemented in a new `audio_tab_controls.go`.
Each owns its own freeze `*Button` instance (no shared widget); the dispatcher
calls `SyncFreeze(frozen)` on the active component each Draw so the icon reflects
the global analyzer freeze state.

Each tab's component **owns its controls outright**: the button widgets, their
toggle state (slope index, pre overlay, freq-scale log, K-20), layout, draw, and
hit areas. There is no shared button bar and no per-tab visibility-gating inside a
shared component — a tab's chrome exists only inside that tab's own component.

The component renders its controls in a **header row directly below the
tab-switcher row** (the geometric strip is not "shared chrome" — only the active
tab's own component ever draws into it). The dispatcher renders that tab's
**content** (`drawAnalyzer*` / `drawLevels*`, unchanged free functions) into the
**body region below the header**, reading any needed toggle state from the owning
component via accessors (`SlopeDBPerOct()`, `PreOverlay()`, `FreqScaleLog()`,
`K20View()`). This removes the shared button bar (the actual coupling) without
relocating the deeply-entangled content/render state (levels latches, spectrum
peak-hold, cursor) that currently lives on the dispatcher — keeping the diff
low-risk. Relocating that render state into the components is a later refinement,
not required to decouple the buttons.

### Freeze button

Freeze *state* is global analyzer state (lives in the analyzer service, reached via
the existing `OnFreezeToggle` callback returning the new frozen bool). Each tab that
shows a freeze button constructs **its own** `*Button` instance wired to that
callback — no shared button widget. Tabs stay independent; the underlying freeze
stays coherent.

## Dispatcher (`EQPanelZone`)

- Owns the slimmed `AudioStickyBar`, the channel dropdown portal, and the legend
  popover.
- Holds the three `tabControls` components (`waveControls`, `spectrumControls`,
  `levelsControls`) and an `activeTabControls() tabControls` selector returning the
  active tab's component (nil for EQ/Chain/Synth/Sampler).
- Reserves a **control-header strip** at the top of `contentRect()` whose height is
  `activeTabControls().HeaderH()` (0 when nil). The active component lays out/draws
  its buttons there; the dispatcher draws the tab's **content** into the **body
  region** below (`bodyRect()` = `contentRect()` inset by the header height).
- Routes the active component's `HitAreas()` into `HitAreas()`; inactive components
  publish none (they aren't asked).
- `contentRect()` and `stickyBarHeight()` are unchanged (the switcher row stays 26
  and still holds channel/tabs/?/expander). The body shrinks by the header height.

### `tabControls` interface (new file `audio_tab_controls.go`)

```go
type tabControls interface {
    HeaderH() int                    // control-header height (0 = none)
    Layout(header image.Rectangle)   // position buttons in the reserved strip
    Draw(dst *ebiten.Image)          // render the buttons
    HitAreas() []HitArea             // buttons' hit areas (z above the body)
    SyncFreeze(frozen bool)          // update freeze icon from analyzer state
}
```

## State-accessor redirects

The dispatcher's content draw reads toggle state from the active component instead
of the sticky bar:

- `eq_panel_zone.go Draw` TabSpectrum case — `FreqScaleLog()`, `SlopeDBPerOct()`,
  `PreOverlay()` → `z.spectrumControls`.
- `eq_panel_zone.go Draw` TabMeters case — `K20View()` → `z.levelsControls`.
- slope/pre/freq-scale/K-20 toggles are self-contained in their component
  constructors; reset-hold (`spectrumPeaks.ResetMax`) and clear-clips
  (`levelsLatches.Clear` + `audio.ResetClipsWindow`) are passed in as callbacks
  from the dispatcher. Legend/expander/channel wiring stays on the sticky bar +
  dispatcher.

## Mobile

Each migrated control must preserve its **current per-platform visibility** — this
is a behavior-preserving refactor, not a mobile redesign:

- **slope / pre / reset-hold** (`audio_sticky_bar.go:251` `&& !Profile().IsMobile()`)
  and **clear-clips / K-20** (`:310` `&& !Profile().IsMobile()`) are **desktop-only**
  today. The owning tab zone lays them out only when `!Profile().IsMobile()`.
- **freq-scale** is Spectrum-tab + desktop-only today (`:251`-adjacent block).
- **freeze** has **no mobile gate** today (`:222`-`:230`) — it shows on
  Wave/Spectrum/Levels/Chain on *both* platforms. The owning tab zone renders its
  freeze button on mobile too, so mobile freeze is not lost.

Mobile tab switching stays on the bottom-nav (the bar's tab pills are already
mobile-hidden, `:330`); legend/expander are already mobile-hidden (`:423`). None of
that changes. Verified by `TestMobileControlVisibilityPreserved` (desktop-only pills
absent on mobile; freeze present on mobile for Wave/Spectrum/Levels).

## Phasing (each phase TDD'd, independently shippable)

During phases 1–3 the sticky bar keeps its old per-tab buttons but **suppresses**
them for already-migrated tabs (a temporary `SetMigratedTabs(...)` scaffold), so no
tab ever shows a control twice. Phase 4 deletes the now-dead bar buttons + the
scaffold.

0. **Seam.** New `audio_tab_controls.go` with the `tabControls` interface and
   dispatcher plumbing: `waveControls/spectrumControls/levelsControls` fields,
   `activeTabControls()`, `headerRect()/bodyRect()`. All three components return
   `HeaderH()==0` initially → zero behavior change. Body == content.
1. **Wave.** `waveControls` owns its own freeze button; dispatcher draws its header
   on TabWave + insets the body; bar suppresses freeze on TabWave.
2. **Levels.** `levelsControls` owns freeze + Clear-Clips + K-20; content K-20 read
   redirects to it; bar suppresses freeze/clear/K-20 on TabMeters.
3. **Spectrum.** `spectrumControls` owns freeze + freq-scale + slope + pre + reset;
   content slope/pre/freq-scale reads redirect to it; bar suppresses these on
   TabSpectrum.
4. **Slim the bar + remove X.** Delete `closeBtn`, `freezeBtn`, `freqScaleBtn`,
   `slopeBtn`, `preBtn`, `resetHoldBtn`, `clearClipsBtn`, `k20Btn`, the
   `SetMigratedTabs` scaffold, and all their layout/draw/hit/accessor code from
   `AudioStickyBar`. Remove `EQCallbacks.OnClose` + its `initButtons` wiring.

(EQ extraction is a documented fast-follow, same pattern, not in this effort.)

## TDD (tests first, red→green, per phase)

Per controls component:
- `Test<Tab>ControlsButtons` — the component owns exactly its expected buttons; on
  its tab their rects sit in the header strip (Y ≥ switcher-row bottom, < body top).
- `Test<Tab>ControlsWired` — clicking each button has the right effect: slope cycles
  0→3→4.5→0, K-20 toggles, Clear-Clips invokes its callback + `audio.ResetClipsWindow`,
  freq-scale toggles log/lin, freeze invokes `OnFreezeToggle`.
- `Test<Tab>ControlsInactiveNoHitAreas` — when another tab is active, the dispatcher
  publishes none of this component's hit areas.

Cross-cutting:
- `TestMainTabBarOnlyHasTabsChannelLegendExpander` — on every tab, `AudioStickyBar`
  publishes hit areas for only channel/tabs/legend/expander — never freeze/close/
  slope/pre/reset/freq/clear/K20.
- `TestCloseButtonGone` — no `eq-close-btn` hit area on any tab; `EQCallbacks` has no
  `OnClose`.
- `TestBodyRectBelowControlHeader` — on tabs with a header, `bodyRect().Min.Y ==
  contentRect().Min.Y + activeTabControls().HeaderH()`.
- `TestMobileControlVisibilityPreserved` — on mobile, desktop-only pills
  (slope/pre/reset/freq-scale/clear-clips/K-20) are absent; the freeze button is
  still present on Wave/Spectrum/Levels (no mobile freeze regression).
- Update/trim `audio_sticky_bar_test.go` + `audio_sticky_bar_phase1_test.go` to the
  new ownership; delete the close-button test (`audio_sticky_bar_test.go:313`+).

## Guards to honor

- `eq_panel_draw_alloc_discipline_test.go` — Levels has the tightest per-frame draw
  budget. Moving its pills into `levelsControls` should be draw-neutral; if the draw
  count shifts, update `perTabAllocBudget[TabMeters]` with a one-line rationale.
- `zone_input_isolation_discipline_test.go` (`TestZoneInvisibleEqualsNoInput`) — the
  control header sits inside the panel bounds, so the EQ-panel catch-all still
  covers it; inactive components publish no hit areas.
- Scene/pixel tests reading sticky-bar button rects (e.g. `audio_panel_render_pixels_test.go`,
  scene crops, `wasm_bridge_smoke`) — redirect to the new owners or update.
- `js_exports_catalogue_drift_test.go` — if any removed accessor backed a JS export,
  update the catalogue.

## Non-goals

- EQ tab extraction (fast-follow, same pattern).
- Any change to Chain/Synth/Sampler ownership.
- Mobile bottom-nav or mobile control surfacing (behavior preserved as-is).
- Renaming `EQPanelZone` (kept to limit churn, despite it now being a general
  audio-panel dispatcher).
