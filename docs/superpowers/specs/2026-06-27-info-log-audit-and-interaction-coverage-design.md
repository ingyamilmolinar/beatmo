# INFO-log audit & full user-interaction coverage

**Date:** 2026-06-27
**Status:** Approved design, pending spec review → implementation plan
**Area:** `internal/hooks`, `internal/eventlogger`, `internal/ui`, `internal/log`, `cmd/`

## Problem

The project already routes nearly all user actions through `hooks.Bus` →
`internal/eventlogger`, which emits **one INFO line per event**
(`[tag] message  src=pkg/file.go:line`). The system is mature (54 non-verbose
event kinds, each with a registry entry + formatter, coverage-enforced). But an
audit found four classes of problem:

1. **Inconsistent tag casing.** The eventlogger narrative tags are all
   lowercase (`[play]`, `[node]`, `[row]`, `[audio]`…). The direct
   infrastructure `log.Infof` calls are mostly UPPERCASE (`[HOOKS]`, `[BENCH]`,
   `[DEMO]`, `[RUNTIME]`, `[SCREENSHOT]`, `[PYROSCOPE]`, `[RECORD-BENCH]`) with a
   few lowercase stragglers (`[heap]`, `[eventlog]`, `[synthdrag]`). No single
   convention.

2. **`[audio]` is an overloaded catch-all.** A single `[audio]` tag covers EQ,
   insert-FX, sends, synth params, sampler, WAV load, instrument rename, recipe,
   and kit — features the user wants independently greppable.

3. **Pan / zoom / drag are never logged.** `EventCameraPan`,
   `EventCameraZoom`, `EventDragProgress` have registry entries + formatters but
   **no code emits them**, and they are classed `IsVerbose` (filtered from the
   default narrative). Today panning/zooming produce zero log lines.

4. **Coverage gaps in discrete actions and interaction telemetry.**
   - Discrete actions with no event at all: **language change**
     (`game_input_shortcuts.go:302`), **undo / redo** (`performUndo` /
     `performRedo` raise UI notifications but emit no event).
   - **No interaction-layer logging at all**: opening/closing menus, popups,
     dialogs; generic button/pill/toggle taps; scrollbars/scroll gestures;
     search queries; generic text-input commits; mobile view-mode switches;
     audio-panel tab switches. The user requires every interactive component to
     log the action taken (selection, toggle, close menu, value update, …).

(Density change was considered but has **no production commit site** today — only
`SetDensityForTest`. It is intentionally out of scope until a settings-UI commit
site lands.)

## Decisions (locked with the user)

- **Casing:** lowercase everywhere.
- **Tag granularity:** split `[audio]` by feature.
- **Pan/zoom/drag:** emit **one line per finished gesture** (gesture/scroll
  end), promoted into the default narrative.
- **Scope:** also add the missing discrete actions (language, undo, redo).
- **Button logging:** **blanket** — instrument the single `Button.applyPress`
  core so every button/pill/toggle/keycap emits a generic `[ui] tap "<label>"`.
  Action buttons therefore show two complementary lines: the tap (intent) and
  the existing domain event (effect). This is one-per-gesture, not per-frame.
- **Overlay logging:** log **both** open and close (incl. Esc / click-outside
  dismiss) from the shared portal chokepoint.

## Architecture

Two complementary layers, both flowing through the existing
`hooks.Bus → eventlogger` pipeline. No new logging pipeline is introduced.

### Layer A — Domain narrative (existing, audited & extended)

The canonical user-action narrative. Each `hooks.Kind` describes a **state
change / effect**.

### Layer B — Interaction telemetry (new)

A small set of new `hooks.Kind`s describing **what the user touched**
(tap / open / close / scroll / search / text-commit / view-switch), emitted at
shared widget chokepoints. Delivered through the same bus/eventlogger so they
appear inline in the narrative and in the JSONL eventstream.

### The tag becomes a registry field (single source of truth)

`internal/hooks/actions.go` `ActionMeta` is documented as *"the single place
that names and classifies every state-mutating user action."* We add a `Tag`
field so the bracket tag lives in that same table next to `Label` and `Scope`:

```go
type ActionMeta struct {
    Kind           Kind
    Label          string      // human label, e.g. "add node"
    Tag            string      // INFO bracket tag, lowercase, e.g. "node"
    Scope          ActionScope
    ExcludedReason string
}
```

`eventlogger.writeEvent` reads the tag from the registry
(`hooks.MetaFor(e.Kind).Tag`) instead of each formatter hardcoding it. Formatters
return only the message string (the `formatted.tag` field is removed; formatters
become `func(payload any) string`). A new test
`TestEveryActionHasLowercaseTag` (in `internal/hooks`) asserts every entry has a
non-empty, all-lowercase `Tag`.

Rationale: tag + label + scope in one row per Kind is maximally greppable and
"parametrized, looked up easily in code" (the user's requirement), and removes
the tag/label duplication that let the two drift.

### New scope: `ScopeInteraction`

Interaction-telemetry kinds are never undoable and not document/session state.
Add `ScopeInteraction` to the `ActionScope` enum. `Undoable()` already returns
true only for `ScopeDocument`, so the new scope is non-undoable automatically. No
exhaustive `switch` on `ActionScope` exists in the tree, so the addition is
safe.

## Tag taxonomy (final, all lowercase)

| Tag | Events |
|---|---|
| `play` | play start/stop, pause, resume |
| `seek` | transport seek |
| `bpm` | BPM change |
| `transport` | subdiv, length |
| `record` | recording start/stop/dropped |
| `import` / `export` | project I/O |
| `scene` / `uistate` | scene apply; UI-state apply |
| `node` / `edge` | graph edits |
| `row` | row add/delete/instrument/mute/solo/volume/pan/color |
| `master` | master volume |
| `eq` | EQ band + HPF/LPF filter |
| `fx` | insert-effect add/remove/param/move/toggle |
| `send` | delay/reverb sends |
| `synth` | synth committed/reset, live synth param, instrument rename |
| `sampler` | sample saved/created/reset/edit, WAV load |
| `recipe` / `kit` | recipe lifecycle; kit apply |
| `favorite` | favorite toggle |
| `panel` | audio-panel state + tab switch (was `uistate`) |
| `camera` | pan / zoom (gesture-end) |
| `drag` | per-frame drag telemetry (stays verbose) |
| `lang` | **new** — UI language change |
| `undo` / `redo` | **new** — undo / redo |
| `ui` | **new** — generic button/pill/toggle tap |
| `popup` | **new** — menu/popup/dialog open & close |
| `scroll` | **new** — scroll gesture end |
| `search` | **new** — search query (coalesced) |
| `input` | **new** — generic text-input commit |
| `view` | **new** — mobile view-mode switch |

## New event kinds

All non-verbose (appear in default narrative) unless noted. Each gets:
`events.go` const + payload type + `KindAll` entry + `ActionRegistry`
entry (`Tag`/`Label`/`Scope`) + formatter + (if high-rate) a `coalesceKinds`
entry. `NumNonVerbose` is recomputed.

### Layer A (discrete actions)

| Kind | Tag | Scope | Payload | Emit site |
|---|---|---|---|---|
| `EventLanguageChanged` | `lang` | Session | `{Old, New string}` | `game_input_shortcuts.go` settings `onPick` (the `i18n.SetLocale` site) |
| `EventUndo` | `undo` | Session | `{Label string}` | `undo_seam.go performUndo` |
| `EventRedo` | `redo` | Session | `{Label string}` | `undo_seam.go performRedo` |

### Layer B (interaction telemetry)

| Kind | Tag | Scope | Payload | Emit site | Coalesced? |
|---|---|---|---|---|---|
| `EventUITap` | `ui` | Interaction | `{Label, Icon string}` | `Button.applyPress` press edge (`held==1`) | no |
| `EventPopupOpened` | `popup` | Interaction | `{ID string}` | `portal.go Open` | no |
| `EventPopupClosed` | `popup` | Interaction | `{ID, Reason string}` | `portal.go Close` / `CloseTop` / `CleanupClosed` | no |
| `EventScroll` | `scroll` | Interaction | `{Surface string}` | `scroll_behavior.go HandleTouchEnd`/`HandleDragEnd`/`EndStepDrag` + `HandleWheel` | **yes** (per-surface key, see below) |
| `EventSearchChanged` | `search` | Interaction | `{Surface, Query string}` | inst-menu search rebuild | **yes** |
| `EventTextCommitted` | `input` | Interaction | `{Field, Value string}` | generic text-input commit not already covered by a domain rename/BPM event | no |
| `EventViewModeChanged` | `view` | Interaction | `{Mode string}` | `SegmentedControl` view-switch callback | no |

Audio-panel **tab switch** reuses `EventAudioPanelStateChanged` with
`Field:"tab:<name>"`, emitted only on the user-driven `onTab` path (not the
programmatic `SetActiveTab` from scene/init). Its tag moves `uistate` → `panel`.

## Pan / zoom / drag (Layer A, gesture-end)

- Add emit sites that fire **once at gesture end**:
  - Camera **pan**: emit `EventCameraPan{DX,DY}` when the pan gesture releases
    (cumulative delta for the gesture).
  - Camera **zoom**: emit `EventCameraZoom{Factor}` at the end of a
    wheel/pinch zoom; coalesced so a continuous wheel-zoom collapses to one line.
- **Promote** `EventCameraPan` / `EventCameraZoom` out of the `IsVerbose` list
  so they show in the default narrative. Their registry `Scope` stays
  `ScopeVerbose` (scope and `IsVerbose` are independent; `Undoable()` is false
  for `ScopeVerbose`). `EventDragProgress` stays in the `IsVerbose` list.
- `EventDragProgress` **stays verbose** per-frame telemetry. Discrete drag
  outcomes are already covered: node drag-end → `EventNodeMoved` (`[node]`),
  slider/knob drag-end → domain commit events, timeline scrub → `EventSeek`.

## Anti-pollution rules

- **One line per discrete action / gesture, never per-frame.** Every high-rate
  emit site fires only at the gesture boundary (release / commit), or is routed
  through the `eventlogger` coalescer (debounce-trailing).
- **`Button.applyPress` logs only on the press edge** (`held==1`), never on
  repeat-tick or release, so a held auto-repeat button is one tap line.
- **Coalescer is keyed by `Kind`.** A single generic interaction kind would
  wrongly collapse unrelated interactions, so interaction telemetry uses
  **per-surface kinds**. `EventScroll` and `EventSearchChanged` are the only
  interaction kinds added to `coalesceKinds` (they are inherently high-rate);
  `EventUITap` / `EventPopupOpened` / `EventPopupClosed` are discrete and are
  **not** coalesced (coalescing taps by kind would drop distinct taps).
  - Note: the coalescer collapses by `Kind` only, so two different scroll
    surfaces within one window collapse to the latest. Acceptable: scroll is
    telemetry, and concurrent scrolls of two surfaces do not occur.
- **Accepted duplication:** an action button emits a `[ui] tap` line *and* its
  domain effect line. This is intentional (intent vs. effect) and bounded to two
  lines per gesture.

## Infrastructure direct-call tags (lowercase)

Mechanical rename of the remaining UPPERCASE tags in direct `log.Infof` calls:
`[HOOKS]→[hooks]`, `[BENCH]→[bench]`, `[DEMO]→[demo]`, `[RUNTIME]→[runtime]`,
`[SCREENSHOT]→[screenshot]`, `[PYROSCOPE]→[pyroscope]`,
`[RECORD-BENCH]→[record-bench]` (and `[RECORD-BENCH/hook]→[record-bench/hook]`),
`[BENCH]` in `game_record_bench.go`/`game_update.go`. Already-lowercase tags
(`[heap]`, `[eventlog]`, `[synthdrag]`) are unchanged. The `[HOOKS]` bench
formatter strings in `internal/bench/hooks_config.go` and
`cmd/beatmo_record_bench.go` are included.

## Emit-helper conventions

New Layer-A/B emit sites follow the existing `internal/ui/event_helpers.go`
pattern: a thin `emitX(...)` helper calling
`hooks.PublishWithSource(kind, payload, hooks.CaptureSource(1))` so the `src=`
column points at the user-code caller. For chokepoints **outside** `internal/ui`
that need source capture (`portal.go` is in `internal/ui`; OK), the same helper
pattern applies. `Button.applyPress` is in `internal/ui`, so `EventUITap` uses an
`emitUITap(label, icon)` helper there; `CaptureSource` will resolve to the
press-routing seam — acceptable because the human label, not the src, identifies
the button.

## Testing

All in the fast `-tags test` path; these guards already gate PRs touching the
area:

- **`internal/hooks`**: `TestEveryKindClassified` (registry completeness — new
  kinds must be added), new `TestEveryActionHasLowercaseTag`, `NumNonVerbose`
  recomputed and its doc-comment updated, `IsVerbose` list updated (pan/zoom
  removed).
- **`internal/eventlogger`**: `coverage_test.go` (every non-verbose Kind needs a
  formatter), `golden_session_test.go` fixtures regenerated for the new
  tags/lines, `coalesce_test.go` for the two new coalesced kinds.
- **`internal/ui`**: new tests asserting emit at each new chokepoint —
  `Button.applyPress` → one `EventUITap` per press edge; `portal.Open/Close` →
  open/close events; scroll-end → `EventScroll`; search → coalesced
  `EventSearchChanged`; language/undo/redo emit; pan/zoom gesture-end emit. The
  undo-set derivation test (`TestUndoableSetMatchesRegistry`) must still pass
  (new kinds are non-document, so the derived set is unchanged).
- **`internal/log`**: `forbidden_at_info_test.go` unaffected (we add no
  per-frame INFO).
- **Browser bridge**: `wasm_bridge_smoke` export catalogue + Go-side
  `js_exports_catalogue_drift_test.go` unaffected (no new JS exports).

## Documentation

- `CLAUDE.md` — update the "Event Notification" + "Log levels" sections: the new
  tag taxonomy, the registry `Tag` field as single source of truth, Layer-B
  interaction telemetry, and the anti-pollution rules.
- `DESIGN.md` — only if it references log tags (verify during implementation).

## Out of scope

- Density-change logging (no production commit site yet).
- Any new JS/WASM bridge export.
- Reworking the coalescer to sub-key by payload (per-surface kinds suffice).
- Per-keystroke text logging (commit-only).

## Implementation phasing (for the plan)

1. Registry `Tag` field + `eventlogger` reads tag from registry; formatters
   return message-only; lowercase-tag test. (Pure refactor; golden fixtures
   updated.)
2. Split `[audio]` tags via the new `Tag` field; retag `panel`.
3. Infra direct-call tag lowercasing.
4. Layer-A gaps: language, undo, redo; pan/zoom gesture-end emit + de-verbose.
5. Layer-B interaction kinds + chokepoint emit sites (portal, button, scroll,
   search, text, view, tab).
6. Docs.
