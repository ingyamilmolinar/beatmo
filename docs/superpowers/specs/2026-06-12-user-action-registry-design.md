# Centralized User-Action Registry + Undo/Redo Coverage Lock

**Date:** 2026-06-12
**Branch:** add-core-node-types
**Status:** Design approved, pending implementation plan

## Problem

The names and undo classification of user actions that mutate game/audio state
are spread across three places, and some mutating actions have no name at all:

1. `internal/hooks/events.go` — `hooks.Kind` constants (the closest thing to a
   central name registry; 53 non-verbose + 3 verbose kinds).
2. `internal/ui/event_helpers.go` — `emit*` helpers that publish a `Kind` and
   tap `recordUndo`.
3. `internal/ui/undo.go` — `documentScopeKinds`, a *separately* hand-maintained
   26-entry map deciding which kinds are undoable.

Consequences of the split:

- A `Kind` can exist without being classified undoable/not.
- A user mutation can exist with **no `Kind` and no undo step at all**.
- The undoable set drifts from the emit-helper taps with nothing to catch it.

### Verified gaps (motivating evidence)

These were confirmed by reading the source, not inferred:

- **`emitSendChanged` and `emitRowPan` exist but are never called from any real
  UI path** (only from tests) — names without wiring.
- **Pan, `delay_send`, `reverb_send`, and the global send-bus config ARE in the
  export schema** (`export.go:54-56`, `207-221`, exported when non-default) — so
  they are genuine *document* state, yet absent from `documentScopeKinds`.
- **Master EQ HPF/LPF toggles** (`setActiveHPF`/`setActiveLPF` in
  `drumview_audio_eq.go:471,488`) mutate exported master-EQ state but emit no
  event and tap no undo.

So a registry is not mere bookkeeping: it surfaces real undo gaps in exported
document state (HPF/LPF, row pan, per-row sends) and makes them test-pinned.

## Goal

One authoritative table that names every user action which mutates
game/audio/transport state, annotated with **scope** and **undoability**, such
that:

- `documentScopeKinds` is *derived* from it (single source of truth).
- A discipline test makes undo/redo coverage provable and drift-proof.
- The non-exported / not-yet-wired gaps are classified honestly and visibly.

Ephemeral UI chrome that mutates no game state (which menu/dropdown is open,
view-mode switch, panel expand, node selection) is **out of scope**.

## Decisions (locked with the user)

1. **Scope:** every state-mutating user action — document, runtime/session
   audio, transport, recording, project-I/O. Exclude ephemeral UI chrome.
2. **Form:** a single canonical registry; derive `documentScopeKinds` and lock
   with a discipline test. Do not keep two parallel structures.
3. **Gaps:** classify now (assign a name/scope, record the reason). Actually
   *wiring* not-yet-undoable document actions (HPF/LPF, pan, sends) to produce
   undo steps — and any export/import goldens churn — is a deliberate follow-up.

## Architecture

### 1. The canonical registry — `internal/hooks/actions.go`

`hooks.Kind` already holds the action *names*; this adds the single
classification table over them. It lives in `hooks` (not `ui`) because both the
`ui` and `audio` packages emit kinds and should consult one table; the
`ExcludedReason` field is a static fact, so the table needs no dependency on the
UI-side export schema.

```go
type ActionScope int

const (
    ScopeDocument  ActionScope = iota // in export schema → undoable unless ExcludedReason set
    ScopeSession                      // runtime audio not exported (mute/solo, audio-panel prefs)
    ScopeTransport                    // play/stop/seek/pause/resume
    ScopeProjectIO                    // import/export (handled by OnExternalLoad, not journaled)
    ScopeRecording                    // record start/stop/dropped
    ScopeVerbose                      // camera/drag/per-frame param — not an undo action
)

// ActionMeta is one row of the registry: the canonical metadata for a Kind.
type ActionMeta struct {
    Kind           Kind
    Label          string      // human label, e.g. "add node"
    Scope          ActionScope
    ExcludedReason string      // non-empty ⇒ document-scope but deliberately not undoable yet
}

// ActionRegistry has exactly one entry per Kind in KindAll.
var ActionRegistry = []ActionMeta{ /* ... one per Kind ... */ }

func MetaFor(k Kind) (ActionMeta, bool)
func Undoable(k Kind) bool          // Scope==ScopeDocument && ExcludedReason==""
func AllActions() []ActionMeta
```

**Undoability rule:** `Undoable(k) == meta.Scope == ScopeDocument &&
meta.ExcludedReason == ""`.

### 2. Derive, don't duplicate

`internal/ui/undo.go`'s hand-maintained `documentScopeKinds` literal map is
replaced by a derived builder:

```go
var documentScopeKinds = buildDocumentScopeKinds()

func buildDocumentScopeKinds() map[hooks.Kind]string {
    m := map[hooks.Kind]string{}
    for _, a := range hooks.AllActions() {
        if hooks.Undoable(a.Kind) {
            m[a.Kind] = a.Label
        }
    }
    return m
}
```

`recordUndo` continues to key off `documentScopeKinds`, so **no emit-helper call
site changes**. The labels move from the literal map into the registry entries.

### 3. Classify the gaps now (names centralized, wiring deferred)

Every state-mutating action gets a registry entry. For actions with no `Kind`
today, assign one (so the name lives in one place) and an emit helper to carry
it; classification records the current reality.

- **Exported document state, no undo today** → `ScopeDocument` with
  `ExcludedReason: "not yet wired to undo (follow-up)"`:
  - master EQ **HPF toggle**, **LPF toggle**
  - **row pan** (`emitRowPan` exists, unwired)
  - **per-row delay/reverb sends** (`emitSendChanged` exists, unwired)
  - **global send-bus config** (`ConfigureSendDelay`/`ConfigureSendReverb`)

  These need a `hooks.Kind` (reuse `EventRowPan`/`EventSendChanged` where they
  already exist; add new kinds for HPF/LPF toggle and send-bus config) so the
  name is centralized even though undo wiring is deferred.

- **Session (not exported)** → `ScopeSession`, not undoable, reasoned:
  - mute/solo (`EventRowMute`/`EventRowSolo` — `Row.Muted/Solo` session-only)
  - audio-panel prefs (`EventAudioPanelStateChanged`)

- **Transport / recording / project-I/O / verbose** → their respective scopes,
  not undoable: `EventPlayStart/Stop`, `EventPaused/Resumed`, `EventSeek`,
  `EventRecordStart/Stop/Dropped`, `EventImport/Export`, the verbose kinds, and
  `EventLengthChange` (derived display state, `ScopeSession`/reasoned).

The exact scope assignment for all 53 non-verbose kinds is enumerated during the
implementation plan against the live `KindAll`.

### 4. Tests that lock it

All run in the fast `-tags test` path.

- **`TestEveryKindClassified`** — every `hooks.Kind` in `KindAll` has exactly one
  `ActionRegistry` entry; no unclassified name, no orphan entry. (New
  drift-guard, analogous to the eventlogger coverage test.)
- **`TestUndoableSetMatchesRegistry`** — derived `documentScopeKinds` equals
  `{k : hooks.Undoable(k)}`.
- **`TestExcludedDocumentActionsHaveReason`** — every `ScopeDocument` entry with
  `Undoable==false` has a non-empty `ExcludedReason`.
- **`TestEmitTapsMatchRegistry`** — the set of emit helpers in `event_helpers.go`
  that call `recordUndo` equals the undoable set, catching drift between the
  helpers and the registry.
- **Refactor `undo_all_actions_test.go`** to iterate the registry's undoable set
  rather than a hand-written list, so adding a future undoable action forces a
  per-action proof automatically: changes the doc, exactly 1 undo step, undo is
  byte-identical, redo restores.

Existing undo tests (`undo_coverage_test.go`, `undo_mute_solo_test.go`, etc.)
are updated to assert against the derived set / registry where they currently
assert against the literal `documentScopeKinds`.

### 5. Out of scope (this pass)

- Wiring HPF/LPF, row pan, and sends to actually emit on their UI commit and
  produce undo steps (and any export/import/parity goldens churn). Tracked by
  the `ExcludedReason` entries and the discipline tests, which keep the gaps
  visible rather than silent.
- Ephemeral UI chrome cataloguing (menu/dropdown open, view-mode, selection,
  panel expand) — mutates no game state.

## Files touched

- **New:** `internal/hooks/actions.go` (registry + scope enum + helpers).
- **New:** `internal/hooks/actions_test.go` (`TestEveryKindClassified` and
  registry-internal invariants).
- **Modified:** `internal/ui/undo.go` (derive `documentScopeKinds`; drop the
  literal map; labels now sourced from the registry).
- **Modified:** `internal/ui/event_helpers.go` (add emit helpers for any new
  gap kinds: HPF/LPF toggle, send-bus config — publish only, no undo tap yet).
- **Modified:** `internal/hooks/events.go` (add the new gap `Kind` constants +
  `KindAll` + `NumNonVerbose` bump; add formatters if the eventlogger coverage
  test requires them).
- **Modified:** `internal/ui/undo_all_actions_test.go`,
  `internal/ui/undo_coverage_test.go` (iterate/assert against the registry).
- **New:** `internal/ui/undo_registry_drift_test.go`
  (`TestUndoableSetMatchesRegistry`, `TestExcludedDocumentActionsHaveReason`,
  `TestEmitTapsMatchRegistry`).

## Risks / notes

- Adding new `Kind` constants bumps `NumNonVerbose` and may require eventlogger
  formatters (forced by `internal/eventlogger/coverage_test.go`). Account for
  this in the plan.
- The `TestEmitTapsMatchRegistry` test must parse `event_helpers.go` for
  `recordUndo(...)` calls (or use a runtime registry of taps) — pick the
  approach that is robust to formatting during the plan.
- Built on `add-core-node-types`, which has pre-existing unrelated test
  breakage and concurrent uncommitted work (see
  `project_add_core_node_types_preexisting_failures`). Stage own hunks only.
