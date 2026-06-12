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

| Tab | New owner | Controls it renders itself | Content render fn it calls |
|---|---|---|---|
| Wave | `waveTabZone` | freeze | `drawAnalyzerWaveform` |
| Spectrum | `spectrumTabZone` | freeze, freq-scale (log/lin), slope (0/3/4.5 dB/oct), pre overlay, reset-hold | `drawAnalyzerSpectrumWithScale` |
| Levels | `levelsTabZone` | freeze, Clear-Clips, K-20 | `drawLevelsMultiChannel` |
| EQ | *(unchanged — dispatcher, fast-follow)* | HPF/LPF, band mutes, dB inputs, curve drag | `drawSpectrumBars` + `drawEQCurve` |
| Chain | `ChainPanelZone` *(unchanged)* | OVR/SPL/DIF, AG, FIT, freeze, close | `drawChainTraces` |
| Synth / Sampler | DrumView header *(unchanged)* | Save/Save As/Reset/Preview | `DrawSynthTab` |

Each tab zone receives the full rect below the switcher row and arranges its own
header strip of buttons + content however it likes. There is **no shared "control
sub-row height" abstraction** — each tab owns its own window geometry. The existing
`drawAnalyzer*` / `drawLevels*` render functions are unchanged and are simply called
by their owning tab zone, so this moves *ownership*, not render logic.

### Freeze button

Freeze *state* is global analyzer state (lives in the analyzer service, reached via
the existing `OnFreezeToggle` callback returning the new frozen bool). Each tab that
shows a freeze button constructs **its own** `*Button` instance wired to that
callback — no shared button widget. Tabs stay independent; the underlying freeze
stays coherent.

## Dispatcher (`EQPanelZone`)

Becomes thin for the extracted tabs:

- Owns the slimmed `AudioStickyBar`, the channel dropdown portal, and the legend
  popover.
- Holds a `PanelTab → tabZone` registry for the extracted tabs (Wave/Spectrum/Levels).
- Routes `Layout / Draw / Update / HitAreas` to the active tab zone with
  `contentRect = area below the switcher row`. For EQ/Chain/Synth/Sampler the
  existing dispatch paths remain.
- `contentRect()` is unchanged (`panel top + stickyBarHeight()` … bottom): each tab
  zone now lays out its own header *inside* that rect rather than the bar reserving
  a strip. (The shared bar shrinks because its per-tab buttons leave, but
  `stickyBarHeight()` stays 26 — the row still holds tabs/channel/?/expander.)

### `tabZone` interface

```go
type tabZone interface {
    Layout(content image.Rectangle) // full area below the switcher row
    Draw(dst *ebiten.Image)
    HitAreas() []HitArea            // empty when this zone is not the active tab
    Update()                        // optional per-frame (button anim, hover)
    Buttons() []*Button             // for wiring/tests
}
```

Inactive tab zones publish **no** hit areas (reusing the existing
`TestZoneInvisibleEqualsNoInput` discipline), and are not drawn.

## State-accessor redirects

Renderers currently read tab state from the sticky bar; redirect them to the owning
tab zone (keep accessor names where JS exports/tests depend on them, delegating
through `EQPanelZone`):

- `render_spectrum.go` — `SlopeDBPerOct()`, `PreOverlay()`, `freqScaleLog` → `spectrumTabZone`.
- `render_meters.go` — `K20View()` → `levelsTabZone`.
- `eq_panel_zone.go initButtons` — slope/pre/reset/freq/clear/K20 OnClick wiring
  moves into the owning tab zones' constructors (callbacks passed from the
  dispatcher); legend/expander/channel wiring stays in the dispatcher.

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

1. **Seam + Wave.** Introduce the `tabZone` interface + dispatcher registry; extract
   `waveTabZone` (only the freeze button). Wave's freeze button leaves the bar.
2. **Levels.** Extract `levelsTabZone` (freeze, Clear-Clips, K-20). Those buttons
   leave the bar.
3. **Spectrum.** Extract `spectrumTabZone` (freeze, freq-scale, slope, pre,
   reset-hold). Those buttons leave the bar.
4. **Slim the bar + remove X.** With all per-tab buttons relocated, delete
   `closeBtn`, `freezeBtn`, `freqScaleBtn`, `slopeBtn`, `preBtn`, `resetHoldBtn`,
   `clearClipsBtn`, `k20Btn` and their layout/draw/hit/accessor code from
   `AudioStickyBar`. Remove `EQCallbacks.OnClose` + its `initButtons` wiring.

(EQ extraction is a documented fast-follow, same pattern, not in this effort.)

## TDD (tests first, red→green, per phase)

Per extracted tab zone:
- `Test<Tab>TabZoneButtons` — the zone owns exactly its expected buttons; their
  rects sit inside the content rect (below the switcher row).
- `Test<Tab>ControlsWired` — clicking each button has the right effect: slope cycles
  0→3→4.5→0, K-20 toggles, Clear-Clips invokes its callback + `audio.ResetClipsWindow`,
  freq-scale toggles log/lin, freeze invokes `OnFreezeToggle`.
- `Test<Tab>ZoneInactiveNoHitAreas` — when another tab is active, this zone
  publishes no hit areas.

Cross-cutting:
- `TestMainTabBarOnlyHasTabsChannelLegendExpander` — on every tab, `AudioStickyBar`
  publishes hit areas for only channel/tabs/legend/expander — never freeze/close/
  slope/pre/reset/freq/clear/K20.
- `TestCloseButtonGone` — no `eq-close-btn` hit area on any tab; `EQCallbacks` has no
  `OnClose`.
- `TestContentDispatchedToActiveTabZone` — the dispatcher routes layout/draw/hit to
  the active tab zone.
- `TestMobileControlVisibilityPreserved` — on mobile, desktop-only pills
  (slope/pre/reset/freq-scale/clear-clips/K-20) are absent; the freeze button is
  still present on Wave/Spectrum/Levels (no mobile freeze regression).
- Update/trim `audio_sticky_bar_test.go` + `audio_sticky_bar_phase1_test.go` to the
  new ownership; delete the close-button test (`audio_sticky_bar_test.go:313`+).

## Guards to honor

- `eq_panel_draw_alloc_discipline_test.go` — Levels has the tightest per-frame draw
  budget. Moving its pills into `levelsTabZone` should be draw-neutral; if the draw
  count shifts, update `perTabAllocBudget[TabMeters]` with a one-line rationale.
- `zone_input_isolation_discipline_test.go` (`TestZoneInvisibleEqualsNoInput`) — each
  tab zone's controls sit inside the panel bounds, so the EQ-panel catch-all still
  covers them; inactive zones must publish nil hit areas.
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
