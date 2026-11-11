# Value wheel + single TextField — go-to redesign and numeric-input audit

Date: 2026-10-03 · Status: approved in conversation (sections §1–§6)

## Intent

- The playback **go-to editor** (beat counter → navigation menu → "Go to beat / Go to
  time") is a tiny type-in box that does not work on mobile (no native input id) and
  disagrees with its own title (row says beat `0` while the title says `Beat 1`).
  Replace it with a **wheel**, inline in the navigation card, on both platforms.
- **One way** to enter numbers (a wheel), **one way** to enter text (`TextField`).
  Typed numeric entry is removed everywhere.
- DESIGN.md gains **menu sizing & layout** rules; the nav card gets bigger on desktop.
- Exhaustive audit of numeric inputs, decided one by one (table below).

Hard requirement from the user: **no disruption to current functionality** — every
existing behaviour is pinned by tests before it is touched (§6).

## Audit and decisions

| Surface | Today | Decision |
|---|---|---|
| Nav → Go to beat | typed "B.S", desktop only | **inline wheel** (§2) |
| Nav → Go to time | typed M:SS, desktop only | **inline wheel** (§2) |
| Transport BPM | tap readout → type; ± | **wheel popup** on tap; keep ± |
| EQ band dB | wheel; centre-tap → type | keep wheel, **drop typing** |
| Sampler knobs | wheel; centre-tap → type | keep wheel, **drop typing** |
| Synth params | typing disabled, dead code | **delete** dead code |
| Legacy `Transport` BPM box | tests only | **delete** |
| Node volume / duration / logic N / logic P / groove % | ± only | **wheel popup** on tapping the value; keep ± |
| Logs time filter (hour/minute ±) | ± | **two inline wheels** HH / MM |
| Node pitch, row/master volume sliders, FX sliders, jump-amount + subdiv pick-lists, Synth armed wheel | — | unchanged |
| Text: rename, WAV name, Save As ×2, inst/template/FX/logs search | TextInput, per-site native wiring | **`TextField`** (§4) |

## §1 Shared `ValueWheel`

```go
type WheelModel interface {
    Label(offset int) (string, bool) // label `offset` detents from current; false = out of range
    StepBy(detents int) bool         // move by detents × current rung; false at clamp
    Rungs() []string                 // ladder labels coarse→fine (top→bottom); nil = no ladder
    Rung() int
    SetRung(i int) bool
}
```

- `NumberModel` — `{Min, Max, Value, Ladder []float64, Rung, Format func(v, step float64) string}`;
  integer and float; used by go-to beat (subdivision steps), go-to time (seconds), BPM,
  node values, logs HH/MM.
- `knobModel` — adapter over the existing `Knob` + `audio.ParamDef` + `KnobStepBadge`, so
  Sampler/EQ behave **identically** (enum params, distinct-label walk, StepMul).
- `ValueWheel` (view) — barrel + optional ladder strip + drag / notch-scroll / click-a-row /
  clicky `stepNotcher` cadence, all **moved** out of `MobileWheelPopup` unchanged. No frame,
  no title; embeddable in any card. It is the partial keyboard claimant
  (Up/Down value, Left/Right resolution, both arrow spellings, claims only while focused).
- `WheelPopup` (host) — today's popup frame around a `ValueWheel`, keeping the
  transactional Accept / Cancel (snapshot at open; Esc restores; Enter / tap-away commits).
- Commit mode per host: **live** (apply each change, one commit at close — Sampler, EQ,
  BPM, node) or **preview** (apply only on Go / Enter — go-to).
- Removed: `ParamValueEditor`, `ValueSpec`, `positionValueSpec`, `parseClock`, the wheel's
  centre "tap to type", `openSynthParamEditor`, `openArmedNumericEditor`, legacy
  `Transport`, numeric native ids (`bpm`, `sampler-param`, `eq-db-N`).
- Not in scope: the Synth armed `BarrelWheel` (in-card inertial editor) — unifying its
  physics is a follow-up.

## §2 Go-to nav card

```
┌ ⌖ Beat 13 · 0:24 ─────────────── × ┐  title band: live playhead (unchanged)
│     [◀◀]   [ ×4 ]   [▶▶]           │  jog row (unchanged, immediate)
│     [  Beats  |  Subdiv  ]          │  jump unit (unchanged)
│ ─────────────────────────────────── │
│ GO TO               [ Beat | Time ] │  mode tabs
│   ┌──────────────────┐ ┌────────┐   │
│   │ ▸ Beat 13 · 3/8 ◂│ │ [1]    │   │  value barrel · ladder
│   └──────────────────┘ └────────┘   │
│   0:24                     [ Go ]   │  secondary readout · primary action
└─────────────────────────────────────┘
```

- **Beat mode**: value = absolute subdivision step; labels **1-based** ("Beat 13",
  "Beat 13 · 3/8" off-beat, reduced fraction as `BeatSlot`/`BeatAxisLabel`); ladder
  1/div · 1 beat · 4 beats · 16 beats (fine rung hidden when div = 1); default 1 beat;
  range Beat 1 … end of timeline (`timelineBeats*div`).
- **Time mode**: value = whole seconds, "M:SS"; ladder 1 s · 5 s · 15 s · 1 min; the
  secondary readout shows the resolved beat via `timeToSteps` (closest beat at/after).
- Opens at the current playhead; does **not** follow the playhead while open.
- Scrolling = **preview**: the existing timeline scrub ghost marks the pending position
  (its chip becomes 1-based too).
- **Go** / **Enter** → `RequestSeekSteps(steps, seekOriginCounter)` (unchanged seek path:
  undo, log, audio lane); menu stays open, wheel re-seats, ghost clears.
- **Esc** / **tap-away** cancel (no seek). Esc closes the card as today.
- Keys while open: wheel owns arrows; everything else passes (Space plays).
- Beat ↔ Time switch converts the pending value.
- Desktop width `menu-w-editor`; 5 wheel rows. Mobile: full-width bottom sheet.
- Removed: the "Go to beat / Go to time" rows, `openGotoBeatEditor`, `openGotoTimeEditor`.

## §3 Other surfaces

All **live**: each detent writes the engine; closing emits **one** log line and **one**
undo step (the `drumview_release_commit.go` pattern).

| Surface | Opens by | Range / ladder |
|---|---|---|
| BPM | tap BPM readout → `WheelPopup` under it | 1–1000; ±1/±5/±10/±50, default ±1 |
| Node volume | tap value | Muted, −40…+12 dB; 0.5/1/3/6 dB (same dB curve as `stepNodeVolume`) |
| Node duration | tap value | 0.25–4; 0.05/0.1/0.25/1 |
| Logic N | tap value | 1–64; 1/4/16 |
| Logic P | tap value | 0–100 %; 1/5/10/25 % (stored 0–1) |
| Groove % | tap value | 0–100 %; 1/5/25 % (stored 0–1) |
| Logs time | inline in date picker | HH 0–23, MM 0–59; no ladder |
| Sampler, EQ | unchanged | unchanged (centre tap now inert) |

- One `nodeParamLimits` table read by sidebar ±, wheels, and the JS bridge. Deliberate
  change: sidebar ± can no longer produce negative duration / N = 0 / volume > +12 dB.
- `setNodeParamsLive` (SetNodeParams + notifyPredictorNode + RebaseAt for logic) — no emit;
  `commitNodeParams` emits + records undo. ± buttons keep their per-tap emit.

## §4 `TextField`

```go
type TextField struct {
    *TextInput
    ID        string                 // stable native id
    Owner     string                 // tree owner (topmost-owner check)
    InputRect func() image.Rectangle // optional: native input placed elsewhere
    OnCommit  func(string)
}
```

- Registers in DrumView's `textFields` registry while visible; `syncNativeGestures`
  builds desktop soft-keyboard and mobile native-input candidates from it (replacing the
  hand-kept per-field lists). Generic `mobileInputPollResult(ID)` polling and
  `softKeyboardShow/Hide` move inside.
- Migrated: rename row, WAV name, Synth Save As, Sampler Save As, inst search, template
  search, FX search, logs search (`menu_search_field.go` wraps it).
- Fixes Save As ×2 and FX search on mobile (and proxy on desktop WASM).
- Guards: no `NewTextInput(` outside the TextField file; unique non-empty IDs; every
  visible field → exactly one candidate per platform.

## §5 DESIGN.md — menu sizing & layout

Widths by **role** (desktop; every class is a full-width bottom sheet on mobile):

| Token | Desktop | Role |
|---|---|---|
| `menu-w-action` | content-fit, 160–240 | context / action / subdiv menus |
| `menu-w-list` | 300 | searchable lists |
| `menu-w-editor` | 360 | cards hosting controls: nav, wheel popups, FX, Settings, Theme |
| `menu-w-wide` | ¾ window, ≤ 960 | Logs |

- Height = content, capped 80 % (desktop) / 85 % (mobile sheet); body scrolls with
  `MenuScroll`, title pinned. Rows ≥ `touchMinTargetPx`. Title band → body inset
  `spacing.md` → hairline + uppercase caption between sections → primary action
  bottom-right, secondary readout bottom-left. Wheels: 5 rows, ladder ≈ ⅓ width.
- Tokens in DESIGN.md `profileOverrides` (layout-tied), regenerated; dead `popupPanelW`
  removed.
- Adopted now: nav card, `WheelPopup`, Settings/Theme (340 → 360). Others on a shrinking
  ratchet list in `TestMenusUseWidthClass`.

## §6 Testing

0. Baseline: fast suite, `make test-real`, touched browser suites; BEFORE screenshots of
   every affected surface (desktop + mobile, isolated `XDG_CONFIG_HOME`).
1. Characterization tape for Sampler/EQ wheels (real `Game.Update`: drag, notch, row
   click, keys, ladder, Enter/Esc/tap-away → per-frame value/step/undo count), captured
   **before** the extraction and replayed identically after. Existing `mobile_wheel_*`
   tests pass unchanged.
2. New real-input tests for go-to, keys, BPM, node values, logs, TextField; browser tests
   extended (no new files) for Save As / FX search / CDP-touch go-to.
3. Every deleted test mapped to a replacement or a reason.
4. Gates per phase: fast `./...`, `make test-real`, touched browser suites, design
   drift/discipline/lint, isolation / MenuCard / keys guards, alloc budgets.
5. New scenes `nav_goto_beat`, `nav_goto_time`, `nav_goto_preview_ghost`, `crop_nav_goto`,
   `bpm_wheel_open`, `node_volume_wheel_open`, `logs_time_wheels`, `save_as_mobile`; AFTER
   captures compared with BEFORE.

Phases: (1) characterization + pure `ValueWheel` extraction, (2) `TextField`, (3) go-to
card, (4) BPM / node / logs, (5) typed-entry deletions, (6) DESIGN.md + tokens + ratchet.
