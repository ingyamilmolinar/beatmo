# Event Coverage + Snapshot Undo/Redo — Design

**Date:** 2026-06-10
**Branch:** add-core-node-types
**Status:** Approved design, pending implementation plan

## Goal

Two coupled deliverables:

1. **Complete event coverage** — every state-mutating *user action* (game, project, **and** userpref state) emits a `hooks` event. Reads, tab/view switches, hover, and focus changes are out of scope.
2. **Undo/redo** for **document/musical state**, built as a snapshot journal that *reuses* the existing eventing infrastructure as its trigger/label layer while staying decoupled from the mutation sites.

## Non-goals

- Undo/redo of transport (play/stop/pause/seek), import/scene-apply, or app preferences (favorites, audio-panel state, saved recipes/samples). These still **emit** events (deliverable 1) but are **not** on the undo stack.
- Persisting undo history into the project file or across reloads (session-only, bounded ring).
- Streaming/in-flight synth modulation. Undo restores discrete committed states only.

## Scope of "document/musical state" (undoable)

Graph (nodes, edges, node params/type/logic/start), drum rows (add/delete/instrument/mute/solo/color/volume/pan), BPM, length, subdivision, master volume, EQ (band gains, HPF/LPF), insert effects (add/remove/move/toggle/param), delay/reverb sends, per-instrument synth params (+ reset), and sample-edit descriptors.

This is exactly the set captured by `(*DrumView).exportBytes()` (`internal/ui/export.go:346`), confirmed to include nodes+edges, rows, instruments, BPM/length/subdiv, master + per-instrument EQ, insert effects (read from the audio layer), delay/reverb sends, per-instrument synth params (delta-from-shipped), and sample-edit descriptors.

---

## Part 1 — Close the event-coverage gaps

### Principle

Emit at the **user-action call site** (UI commit handlers), not at the low-level audio setters. The audio setters (`SetChannelVolume`, `SetChannelPan`, `SetInstrumentParams`, `SetDelaySend`, …) are also driven by `Import`, so emitting there would fire spurious events during a project load. This also satisfies the existing "events carry the original emitter source" rule (capture `file:line` of the user-action site).

### Gaps to fill

These mutations currently emit no `hooks` event. Each new `Kind` requires: an enum const + a `KindAll` entry (`IsVerbose=false`), a payload struct in `internal/hooks/events.go`, an emit helper in `internal/ui/event_helpers.go`, and a formatter in `internal/eventlogger/format.go`. The existing `TestEventLoggerCoversAllNonVerboseKinds` **forces** the formatter to exist, so a missing one fails the build.

| Action | New event `Kind` | Payload | Commit point |
|---|---|---|---|
| Row volume | `EventRowVolume` | `{Row, Volume}` | slider `OnRelease` |
| Row/channel pan | `EventRowPan` | `{Row, Pan}` | slider `OnRelease` |
| Move insert effect | `EventInsertEffectMoved` | `{Channel, From, To}` | discrete |
| Toggle insert effect | `EventInsertEffectToggled` | `{Channel, Slot, Enabled}` | discrete |
| Delay/reverb send | `EventSendChanged` | `{Channel, Kind (delay\|reverb), Value}` | slider `OnRelease` |
| Synth params committed | `EventInstrumentParamsCommitted` | `{Channel, Recipe}` | knob `OnRelease` |
| Reset synth params | `EventInstrumentParamsReset` | `{Channel, Recipe}` | discrete |
| Audio-panel state | `EventAudioPanelStateChanged` | `{Field, ...}` | discrete (userpref coverage only — **not** undoable) |

Notes:
- The existing per-frame `EventInstrumentParamChanged` stays **verbose** (live drag feedback / logging). The new `EventInstrumentParamsCommitted` is the non-verbose, once-per-gesture event fired at knob release.
- Master volume, channel volume (via row volume), favorites, and the graph/row/transport/EQ/insert-add/remove/param actions already emit.

### Coverage discipline test

Add a test enumerating the full set of state-mutating actions and asserting each emits its expected `Kind` (extending the existing emit-helper test patterns). Combined with the formatter-coverage test, this prevents silent regressions.

---

## Part 2 — Undo engine (snapshot journal)

### Component: `UndoManager` (in `internal/ui`)

Self-contained. Holds:

- `committed []byte` — snapshot of the document as of the current state (`= exportBytes()`).
- `undo`, `redo` — bounded ring stacks of `{label string, snapshot []byte}` (cap ~100).
- `restoring bool` — re-entrancy guard.

No coalescing state, **no timer, no `Update` tick**.

### Decoupling boundary (three seams only)

1. `Capture func() []byte` ← `(*DrumView).exportBytes`
2. `Restore func(snapshot []byte) error` ← a small `Game` method doing playhead-preserving import (below)
3. `Notify(kind hooks.Kind, target any, label string)` ← called from the emit chokepoint on **committed** document-scope events only

The hooks/event layer must not hard-depend on the undo engine. The manager registers itself into a package-level indirection (`var undoObserver interface{ Notify(...) }`, dependency-inverted). Mutation sites and the async bus stay ignorant of undo; undo stays ignorant of mutation internals — it knows only the `Kind` taxonomy and a `kind→label` table.

### Why synchronous, not the async `hooks.Bus`

The async bus is unordered and runs off the publisher goroutine, so it cannot define step boundaries reliably. `Notify` is a **synchronous** tap on the same chokepoint, always invoked on the UI/JS goroutine (under `seqMu`), so ordering is exact. The async bus continues serving observers (`eventlogger`, `eventstream`) unchanged. One chokepoint, two consumers: async observers + the sync undo tap.

### Deterministic step boundaries (no timing)

Steps are bracketed structurally by the input system, never by elapsed time:

- **Continuous controls** (volume/pan/EQ-band/send/insert-param/synth-knob/node-move drags): `Notify` fires once at `OnRelease` (drag end). The live `OnDrag` mutations emit verbose events for logging but are **not** recorded. Value commits via other means (e.g. a text field) fire `Notify` at end-of-edit.
- **Discrete actions** (add/delete node·edge, toggle mute/solo, set instrument, add/remove/move/toggle FX, BPM ±, length/subdiv, node type, synth reset, sample-edit toggle): `Notify` fires immediately — each is already a single commit.

So the manager records **exactly on committed events**. The recorded set = the committed (non-live, non-verbose) document-scope `Kind`s.

### Recording (the whole algorithm)

```
Notify(kind, target, label):
    if restoring: return                 # restore must not record
    if kind not in recordedSet: return   # live/verbose/out-of-scope kinds ignored
    snap = Capture()
    if bytes.Equal(snap, committed): return   # no-op gesture (e.g. dragged back)
    undo.push({label, snapshot: committed})    # before-state = previous baseline
    committed = snap
    redo.clear()
```

The `before` snapshot for any step is simply the previous `committed` baseline. It stays in sync with the live document because nothing is recorded mid-drag and **every** committed mutation calls `Notify` (enforced by Part 1 coverage). A debug-build assertion verifies `committed == Capture()` at the start of each recording when no gesture is in flight, surfacing any coverage gap loudly rather than silently corrupting a step.

### Undo / Redo

```
Undo():
    if undo.empty: return
    redo.push({label, snapshot: committed})
    e = undo.pop()
    restore(e.snapshot)
    committed = e.snapshot

Redo(): symmetric (swap undo/redo)
```

### Playhead-preserving restore (the one `Game` method the manager calls)

```
RestoreSnapshot(snapshot):
    beat = playheadAbsSubdiv() -> beats
    wasPlaying = isPlaying()
    restoring = true
    Import(snapshot)          # tested replace path; stops playback, resets timeline
    Seek(beat)                # restore playhead (Import otherwise resets it)
    if wasPlaying: resume
    restoring = false
```

- `restoring = true` makes `Notify` a no-op (the restore is not a new step) and makes `Import`'s clear-on-load a no-op.
- `Import` gains one line at entry: `g.undo.OnExternalLoad()`, which **clears** both stacks *unless* `restoring`. A real user import/scene-apply therefore wipes history (chosen behavior); an undo-restore does not.
- Re-import fires no per-field hooks, so a restore does not pollute the event stream or re-trigger recording.

### Snapshot representation & memory

Snapshots are raw `exportBytes()` output. They are byte-exact (synth params are delta-from-shipped, goldens-locked), so equality checks and round-trips are reliable, and restore rides the most-tested path (`Import`). A bounded ring (~100 steps × ~10–50 KB) caps memory at a few MB worst case. Identical-to-baseline snapshots are dropped, so no-op gestures cost nothing.

---

## Part 3 — UI surface

- **Keyboard:** Ctrl/Cmd+Z = undo; Ctrl/Cmd+Shift+Z and Ctrl+Y = redo. Desktop (Ebiten key events) + browser (canvas keydown — may need a small input-layer addition for modifier+key, flagged as a plan task).
- **Buttons:** undo/redo buttons in the transport bar, built with the shared `*Button` widget (no bespoke chrome). Disabled when the respective stack is empty; tooltip shows the next step's label.
- **Mobile:** the same undo/redo buttons surface in the mobile layout for parity.

---

## Testing

- **Unit** (`UndoManager` with fake `Capture`/`Restore`): push/undo/redo ordering; recorded-set filtering (live/verbose kinds ignored); no-op-gesture drop; ring bound; `OnExternalLoad` clears stacks; `restoring` re-entrancy (restore records nothing, doesn't clear).
- **Integration** (Go fast path, ebitenstub): for each document action, mutate via the canonical site → assert `exportBytes()` after undo is byte-identical to before the action; redo re-applies; a simulated drag (`OnPress`→several `OnDrag`→`OnRelease`) yields exactly **one** step; cascade (delete a node with incident edges → undo restores node **and** edges); playhead preserved across undo while playing.
- **Coverage discipline:** a table of document-scope `Kind`s the manager records, with a test (mirroring `internal/eventlogger/coverage_test.go`) asserting no gaps vs the declared recorded-set. New event formatters auto-enforced by the existing coverage test.
- **Wiring** (ebitenstub): Ctrl+Z triggers undo; Ctrl+Shift+Z / Ctrl+Y trigger redo; buttons disabled when empty; mobile presence.

## Risks / notes

- Brief audio discontinuity on undo-during-play; anti-pop covers transients and the playhead is preserved.
- `exportBytes` runs once per committed step (never per live event) — cheap at this project scale.
- Correctness of the running baseline depends on **complete** committed-event coverage; the debug assertion + coverage discipline test guard this invariant.
- All `Notify`/restore activity is on the UI/JS goroutine under `seqMu`; no new locks. `exportBytes` and `Import` are already safe on that goroutine.
