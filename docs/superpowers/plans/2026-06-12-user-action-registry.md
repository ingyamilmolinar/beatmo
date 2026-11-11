# User-Action Registry + Undo Coverage-Lock Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Centralize every state-mutating user action into one authoritative `hooks.ActionRegistry` (name + scope + undoability), derive `documentScopeKinds` from it, and lock undo/redo coverage with discipline tests.

**Architecture:** A single data table in `internal/hooks/actions.go` annotates each `hooks.Kind` with an `ActionScope` and (for document-scope gaps) an `ExcludedReason`. `internal/ui/undo.go`'s hand-maintained `documentScopeKinds` map becomes a derived view of the registry. Discipline tests assert: every `Kind` is classified, the undoable set equals the registry's undoable subset, every excluded document-action carries a reason, and every undoable kind has a behavioral per-action undo test.

**Tech Stack:** Go 1.23, bundled toolchain at `.tools/go/bin/go`, fast test path `-tags test -modfile=go.test.mod`.

**Branch note:** Built on `add-core-node-types`, which carries heavy uncommitted work from other agents and has pre-existing unrelated test breakage (see memory `project_add_core_node_types_preexisting_failures`). **Stage only your own files in every commit** (explicit `git add <paths>`, never `git add -A`).

**Test command (use everywhere below):**
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkg> -run <TestName> -v
```

---

## File Structure

- **Create** `src/go/internal/hooks/actions.go` — the registry: `ActionScope`, `ActionMeta`, `ActionRegistry`, `MetaFor`, `Undoable`, `AllActions`.
- **Create** `src/go/internal/hooks/actions_test.go` — registry-internal invariants (`TestEveryKindClassified`, `TestRegistryNoDuplicateKinds`, `TestExcludedReasonOnlyOnDocument`, `TestUndoableImpliesDocument`).
- **Modify** `src/go/internal/ui/undo.go` — replace the literal `documentScopeKinds` with a builder derived from `hooks.AllActions()`.
- **Create** `src/go/internal/ui/undo_registry_drift_test.go` — `TestUndoableSetMatchesRegistry`.
- **Modify** `src/go/internal/ui/undo_coverage_test.go` — derive coverage assertions from the registry instead of hand-written lists.
- **Modify** `src/go/internal/ui/undo_all_actions_test.go` — add `TestEveryUndoableKindHasActionCase` cross-check (forces a behavioral test per undoable kind).
- **(Task 6, naming-gap closure)** Modify `src/go/internal/hooks/events.go`, `src/go/internal/hooks/actions.go`, `src/go/internal/eventlogger/format.go`, `src/go/internal/ui/synth_panel_save_test.go` to add `EventEQFilterToggled`.

**Note on emit-tap parity:** A source-scan test asserting emit helpers tap `recordUndo` is deliberately NOT included — it is fully subsumed by Task 5's behavioral guarantee. A forgotten tap makes that kind's per-action test record 0 undo steps (fails), and an extra tap on an excluded kind is a no-op because `recordUndo` gates on `documentScopeKinds` membership. The behavioral test is strictly stronger than a string scan.

---

## Task 1: Create the registry (`hooks/actions.go`)

**Files:**
- Create: `src/go/internal/hooks/actions.go`
- Test: `src/go/internal/hooks/actions_test.go`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/hooks/actions_test.go`:

```go
package hooks

import "testing"

// TestEveryKindClassified asserts every Kind in KindAll has exactly one
// ActionRegistry entry and vice versa. This is the central completeness guard:
// a new Kind without a registry entry (or a stale entry) trips it.
func TestEveryKindClassified(t *testing.T) {
	inReg := map[Kind]int{}
	for _, a := range ActionRegistry {
		inReg[a.Kind]++
	}
	for _, k := range KindAll {
		switch inReg[k] {
		case 0:
			t.Errorf("Kind %q has no ActionRegistry entry — classify it in actions.go", k)
		case 1:
			// ok
		default:
			t.Errorf("Kind %q has %d ActionRegistry entries — must be exactly one", k, inReg[k])
		}
	}
	known := map[Kind]bool{}
	for _, k := range KindAll {
		known[k] = true
	}
	for _, a := range ActionRegistry {
		if !known[a.Kind] {
			t.Errorf("ActionRegistry has %q which is not in KindAll", a.Kind)
		}
	}
}

// TestRegistryNoDuplicateKinds guards against copy-paste duplicates.
func TestRegistryNoDuplicateKinds(t *testing.T) {
	seen := map[Kind]bool{}
	for _, a := range ActionRegistry {
		if seen[a.Kind] {
			t.Errorf("duplicate ActionRegistry entry for %q", a.Kind)
		}
		seen[a.Kind] = true
	}
}

// TestExcludedReasonOnlyOnDocument asserts ExcludedReason is only set on
// ScopeDocument entries (it means "document state, deliberately not undoable
// yet" — meaningless on non-document scopes, which are never undoable).
func TestExcludedReasonOnlyOnDocument(t *testing.T) {
	for _, a := range ActionRegistry {
		if a.ExcludedReason != "" && a.Scope != ScopeDocument {
			t.Errorf("%q has ExcludedReason on non-document scope %v", a.Kind, a.Scope)
		}
	}
}

// TestUndoableImpliesDocument asserts the Undoable helper only ever returns
// true for document-scope, non-excluded entries.
func TestUndoableImpliesDocument(t *testing.T) {
	for _, a := range ActionRegistry {
		if Undoable(a.Kind) {
			if a.Scope != ScopeDocument || a.ExcludedReason != "" {
				t.Errorf("Undoable(%q)=true but scope=%v reason=%q", a.Kind, a.Scope, a.ExcludedReason)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails (does not compile)**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ -run TestEveryKindClassified -v
```
Expected: FAIL — build error `undefined: ActionRegistry`, `undefined: ScopeDocument`, `undefined: Undoable`.

- [ ] **Step 3: Create the registry**

Create `src/go/internal/hooks/actions.go`:

```go
package hooks

// ActionScope classifies a user-action Kind by the kind of state it mutates,
// which determines whether it participates in the snapshot-journal undo system.
type ActionScope int

const (
	// ScopeDocument mutates the EXPORTED project document. Undoable unless
	// ExcludedReason is set (a known gap pending a real UI commit site / wiring).
	ScopeDocument ActionScope = iota
	// ScopeSession mutates runtime audio/UI state NOT in the export schema
	// (mute/solo, audio-panel prefs, user-asset loads). A snapshot can't
	// restore it, so it is never undoable.
	ScopeSession
	// ScopeTransport is playback control (play/stop/seek/pause/resume).
	ScopeTransport
	// ScopeProjectIO replaces or reads the whole document (import/export/scene).
	// Undo handles these via OnExternalLoad, not the per-action journal.
	ScopeProjectIO
	// ScopeRecording is the audio recording lifecycle.
	ScopeRecording
	// ScopeVerbose is high-frequency telemetry (camera/drag/per-frame param),
	// never an undo action.
	ScopeVerbose
)

// ActionMeta is one row of the canonical user-action registry: the single
// source of truth for a Kind's human label, scope, and undo classification.
type ActionMeta struct {
	Kind           Kind
	Label          string      // human label, e.g. "add node"
	Scope          ActionScope
	ExcludedReason string      // non-empty ⇒ ScopeDocument but deliberately not undoable yet
}

// ActionRegistry has exactly one entry per Kind in KindAll. It is the single
// place that names and classifies every state-mutating user action. The undo
// recorded-set (internal/ui documentScopeKinds) is DERIVED from this; do not
// hand-maintain a parallel list. Completeness is pinned by TestEveryKindClassified.
//
// Labels for undoable (ScopeDocument, no ExcludedReason) entries are the
// user-facing undo labels and must stay stable — they are asserted against the
// derived documentScopeKinds in internal/ui/undo_coverage_test.go.
var ActionRegistry = []ActionMeta{
	// ── Transport ───────────────────────────────────────────────────────
	{EventPlayStart, "start playback", ScopeTransport, ""},
	{EventPlayStop, "stop playback", ScopeTransport, ""},
	{EventPaused, "pause", ScopeTransport, ""},
	{EventResumed, "resume", ScopeTransport, ""},
	{EventSeek, "seek", ScopeTransport, ""},

	// ── Recording ───────────────────────────────────────────────────────
	{EventRecordStart, "start recording", ScopeRecording, ""},
	{EventRecordStop, "stop recording", ScopeRecording, ""},
	{EventRecordDropped, "recording dropped", ScopeRecording, ""},

	// ── Time settings ───────────────────────────────────────────────────
	{EventBPMChange, "change BPM", ScopeDocument, ""},
	{EventSubdivChange, "change subdivision", ScopeDocument, ""},
	// Length is derived display state recomputed from the graph beat-path; it
	// is not in the export schema, so a snapshot cannot restore it independently.
	{EventLengthChange, "change length", ScopeSession, ""},

	// ── Project I/O ─────────────────────────────────────────────────────
	{EventImport, "import project", ScopeProjectIO, ""},
	{EventExport, "export project", ScopeProjectIO, ""},

	// ── Graph edits (all undoable) ──────────────────────────────────────
	{EventNodeAdded, "add node", ScopeDocument, ""},
	{EventNodeDeleted, "delete node", ScopeDocument, ""},
	{EventNodeMoved, "move node", ScopeDocument, ""},
	{EventNodeTypeChanged, "change node type", ScopeDocument, ""},
	{EventNodeParamsChanged, "edit node", ScopeDocument, ""},
	{EventStartNodeChanged, "set start node", ScopeDocument, ""},
	{EventEdgeAdded, "add edge", ScopeDocument, ""},
	{EventEdgeDeleted, "delete edge", ScopeDocument, ""},

	// ── Drum rows ───────────────────────────────────────────────────────
	{EventRowAdded, "add row", ScopeDocument, ""},
	{EventRowDeleted, "delete row", ScopeDocument, ""},
	{EventRowInstrumentChange, "change instrument", ScopeDocument, ""},
	// Mute/solo are session state (Row.Muted/Solo), not in exportBytes.
	{EventRowMute, "toggle mute", ScopeSession, ""},
	{EventRowSolo, "toggle solo", ScopeSession, ""},

	// ── Audio settings ──────────────────────────────────────────────────
	{EventMasterVolumeChange, "set master volume", ScopeDocument, ""},
	{EventEQBandChange, "adjust EQ", ScopeDocument, ""},
	{EventInsertEffectAdded, "add effect", ScopeDocument, ""},
	{EventInsertEffectRemoved, "remove effect", ScopeDocument, ""},
	{EventInsertEffectParam, "adjust effect", ScopeDocument, ""},
	{EventRowVolume, "set row volume", ScopeDocument, ""},
	// Row pan IS in the export schema, but no UI control writes it today (import
	// + display only). Re-classify as undoable when a commit site lands.
	{EventRowPan, "set row pan", ScopeDocument, "import-only; no UI commit site yet"},
	{EventInsertEffectMoved, "reorder effect", ScopeDocument, ""},
	{EventInsertEffectToggled, "toggle effect", ScopeDocument, ""},
	// Per-row delay/reverb sends ARE in the export schema, but no UI control
	// writes them today (import + display only).
	{EventSendChanged, "adjust send", ScopeDocument, "import-only; no UI commit site yet"},
	{EventInstrumentParamsCommitted, "edit synth", ScopeDocument, ""},
	{EventInstrumentParamsReset, "reset synth", ScopeDocument, ""},
	// Audio-panel prefs persist to userprefs, not the project document.
	{EventAudioPanelStateChanged, "change audio-panel view", ScopeSession, ""},

	// ── Round 2 narrative events ────────────────────────────────────────
	{EventRowColorChanged, "recolor row", ScopeDocument, ""},
	// Loading a user WAV is a library/asset op, not a journaled document edit.
	{EventCustomWAVLoaded, "load WAV", ScopeSession, ""},
	{EventInstrumentRenamed, "rename instrument", ScopeDocument, ""},
	{EventSceneApplied, "apply scene", ScopeProjectIO, ""},
	{EventUIStateApplied, "apply UI state", ScopeSession, ""},
	{EventFavoriteToggled, "toggle favorite", ScopeSession, ""},

	// ── Recipe / kit lifecycle (library ops, not journaled) ─────────────
	{EventRecipeSaved, "save recipe", ScopeSession, ""},
	{EventRecipeCreated, "create recipe", ScopeSession, ""},
	{EventRecipeDeleted, "delete recipe", ScopeSession, ""},
	{EventKitApplied, "apply kit", ScopeSession, ""},

	// ── Sampler lifecycle ───────────────────────────────────────────────
	{EventSampleSaved, "save sample", ScopeSession, ""},
	{EventSampleCreated, "create sample", ScopeSession, ""},
	{EventSampleReset, "reset sample", ScopeSession, ""},
	// The non-destructive sample-edit descriptor IS exported (sample_edit).
	{EventSampleEditChanged, "edit sample", ScopeDocument, ""},

	// ── Verbose (never undo actions) ────────────────────────────────────
	{EventCameraPan, "camera pan", ScopeVerbose, ""},
	{EventCameraZoom, "camera zoom", ScopeVerbose, ""},
	{EventDragProgress, "drag", ScopeVerbose, ""},
	{EventInstrumentParamChanged, "edit synth (live)", ScopeVerbose, ""},
}

// actionByKind indexes ActionRegistry for O(1) lookup. Built once at init.
var actionByKind = func() map[Kind]ActionMeta {
	m := make(map[Kind]ActionMeta, len(ActionRegistry))
	for _, a := range ActionRegistry {
		m[a.Kind] = a
	}
	return m
}()

// MetaFor returns the registry metadata for a Kind.
func MetaFor(k Kind) (ActionMeta, bool) {
	a, ok := actionByKind[k]
	return a, ok
}

// Undoable reports whether a Kind produces a snapshot-journal undo step:
// document-scope and not deliberately excluded.
func Undoable(k Kind) bool {
	a, ok := actionByKind[k]
	return ok && a.Scope == ScopeDocument && a.ExcludedReason == ""
}

// AllActions returns the full registry (read-only; do not mutate the slice).
func AllActions() []ActionMeta { return ActionRegistry }
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ -run 'TestEveryKindClassified|TestRegistryNoDuplicateKinds|TestExcludedReasonOnlyOnDocument|TestUndoableImpliesDocument' -v
```
Expected: PASS (4 tests). If `TestEveryKindClassified` reports a missing/extra Kind, the live `KindAll` drifted from this plan's enumeration — add/remove the registry entry to match `KindAll` exactly (the registry must mirror it 1:1).

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/hooks/actions.go src/go/internal/hooks/actions_test.go
git commit -m "feat(hooks): canonical user-action registry over hooks.Kind

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Derive `documentScopeKinds` from the registry

**Files:**
- Modify: `src/go/internal/ui/undo.go:218-248` (replace literal map)
- Create: `src/go/internal/ui/undo_registry_drift_test.go`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/undo_registry_drift_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestUndoableSetMatchesRegistry asserts the derived undo recorded-set is
// exactly the registry's undoable subset — the single-source-of-truth invariant.
func TestUndoableSetMatchesRegistry(t *testing.T) {
	want := map[hooks.Kind]string{}
	for _, a := range hooks.AllActions() {
		if hooks.Undoable(a.Kind) {
			want[a.Kind] = a.Label
		}
	}
	if len(want) != len(documentScopeKinds) {
		t.Fatalf("documentScopeKinds size=%d, registry undoable size=%d", len(documentScopeKinds), len(want))
	}
	for k, label := range want {
		got, ok := documentScopeKinds[k]
		if !ok {
			t.Errorf("registry says %q is undoable but documentScopeKinds lacks it", k)
			continue
		}
		if got != label {
			t.Errorf("label drift for %q: documentScopeKinds=%q registry=%q", k, got, label)
		}
	}
	for k := range documentScopeKinds {
		if !hooks.Undoable(k) {
			t.Errorf("documentScopeKinds has %q but registry says it is not undoable", k)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it passes against the OLD literal map**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestUndoableSetMatchesRegistry -v
```
Expected: PASS — the registry's 26 undoable labels already match the existing literal `documentScopeKinds`. (This proves the registry is faithful BEFORE we delete the literal.) If it fails on a label mismatch, fix the registry `Label` in `actions.go` to match the existing `documentScopeKinds` literal exactly, then re-run.

- [ ] **Step 3: Replace the literal map with a derived builder**

In `src/go/internal/ui/undo.go`, replace the entire `var documentScopeKinds = map[hooks.Kind]string{ ... }` block (the comment + literal, lines 218-248) with:

```go
// documentScopeKinds is the recorded-set: committed document-state kinds that
// produce an undo step. It is DERIVED from hooks.ActionRegistry (the single
// source of truth) — do not hand-edit. Pinned by TestUndoableSetMatchesRegistry.
var documentScopeKinds = buildDocumentScopeKinds()

func buildDocumentScopeKinds() map[hooks.Kind]string {
	m := make(map[hooks.Kind]string)
	for _, a := range hooks.AllActions() {
		if hooks.Undoable(a.Kind) {
			m[a.Kind] = a.Label
		}
	}
	return m
}
```

Leave `undoLabelFor`, `recordUndo`, and `recordUndoStep` (below the map) unchanged.

- [ ] **Step 4: Run the undo test suite to verify nothing broke**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestUndoableSetMatchesRegistry|TestUndoRecordedSetMatchesDeclared|TestUndoRecordedSetExcludesNonDocument' -v
```
Expected: PASS (3 tests). `documentScopeKinds` now equals the registry-derived map and still satisfies the legacy assertions (which Task 3 then re-bases).

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/undo.go src/go/internal/ui/undo_registry_drift_test.go
git commit -m "refactor(ui): derive documentScopeKinds from hooks.ActionRegistry

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Re-base `undo_coverage_test.go` on the registry

The existing `wantUndoableKinds` (26 hand-written) and `excluded` (hand-written) lists are exactly the duplication the registry eliminates. Re-express them as registry queries so the file can never drift.

**Files:**
- Modify: `src/go/internal/ui/undo_coverage_test.go` (replace entire contents)

- [ ] **Step 1: Rewrite the file to derive from the registry**

Replace the entire contents of `src/go/internal/ui/undo_coverage_test.go` with:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestUndoRecordedSetMatchesDeclared asserts every undoable kind in the registry
// has a non-empty label in the derived documentScopeKinds, and no extra keys
// exist. (Focused label-presence guard; the deeper equality check is
// TestUndoableSetMatchesRegistry in undo_registry_drift_test.go.)
func TestUndoRecordedSetMatchesDeclared(t *testing.T) {
	for _, a := range hooks.AllActions() {
		if !hooks.Undoable(a.Kind) {
			continue
		}
		if documentScopeKinds[a.Kind] == "" {
			t.Errorf("undoable kind %q missing a label in documentScopeKinds", a.Kind)
		}
	}
	for k := range documentScopeKinds {
		if !hooks.Undoable(k) {
			t.Errorf("documentScopeKinds has %q but registry says not undoable", k)
		}
	}
}

// TestUndoRecordedSetExcludesNonDocument asserts kinds that emit events for
// coverage but cannot be snapshot-undone are NOT in the recorded-set. The
// excluded set is derived from the registry (everything not Undoable), so this
// can never drift from the classification.
func TestUndoRecordedSetExcludesNonDocument(t *testing.T) {
	for _, a := range hooks.AllActions() {
		if hooks.Undoable(a.Kind) {
			continue
		}
		if _, ok := documentScopeKinds[a.Kind]; ok {
			t.Errorf("%q is not undoable in the registry but appears in documentScopeKinds", a.Kind)
		}
	}
}

// TestDocumentGapsAreReasoned asserts every document-scope kind that is NOT
// undoable carries an ExcludedReason — the honest record of a known gap
// (e.g. row pan / sends are exported but have no UI commit site yet).
func TestDocumentGapsAreReasoned(t *testing.T) {
	for _, a := range hooks.AllActions() {
		if a.Scope == hooks.ScopeDocument && !hooks.Undoable(a.Kind) {
			if a.ExcludedReason == "" {
				t.Errorf("document-scope gap %q must carry an ExcludedReason", a.Kind)
			}
		}
	}
}
```

- [ ] **Step 2: Run to verify it passes**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestUndoRecordedSetMatchesDeclared|TestUndoRecordedSetExcludesNonDocument|TestDocumentGapsAreReasoned' -v
```
Expected: PASS (3 tests).

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/undo_coverage_test.go
git commit -m "test(ui): base undo coverage assertions on the action registry

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Force a behavioral test for every undoable kind

`undo_all_actions_test.go` proves each undoable action {changes doc, 1 step, undo byte-identical, redo} with bespoke per-action mutation closures. Add a cross-check so a *future* undoable kind cannot be added to the registry without also adding a behavioral test case. This also subsumes emit-tap parity: a missing `recordUndo` tap makes the per-action test record 0 steps and fail.

**Files:**
- Modify: `src/go/internal/ui/undo_all_actions_test.go` (add cross-check test)

- [ ] **Step 1: Inspect the existing case table**

Run:
```bash
cd src/go && grep -n "hooks.Event\|kind\b\|Kind\b\|name:\|cases :=\|:= \[\]\|range " internal/ui/undo_all_actions_test.go | head -60
```
Read the result and note: (a) the exact slice variable holding the test cases (e.g. `cases`), and (b) whether each case already carries a `hooks.Kind` field, and its name. The next step references them as `<casesVar>` and `<kindField>` — substitute the real identifiers. If no kind field exists, add one (Step 2b).

- [ ] **Step 2: Add the coverage cross-check**

Append to `src/go/internal/ui/undo_all_actions_test.go` (substitute `<casesVar>` / `<kindField>` from Step 1):

```go
// TestEveryUndoableKindHasActionCase asserts the per-action test table covers
// exactly the registry's undoable set. Adding a new undoable kind to the
// registry without a corresponding bespoke test case fails here — so undo
// coverage can never silently regress.
func TestEveryUndoableKindHasActionCase(t *testing.T) {
	covered := map[hooks.Kind]bool{}
	for _, c := range <casesVar> {
		covered[c.<kindField>] = true
	}
	for _, a := range hooks.AllActions() {
		if !hooks.Undoable(a.Kind) {
			continue
		}
		if !covered[a.Kind] {
			t.Errorf("undoable kind %q has no case in undo_all_actions_test.go — add one", a.Kind)
		}
	}
	for k := range covered {
		if k != "" && !hooks.Undoable(k) {
			t.Errorf("test case references %q which the registry says is not undoable", k)
		}
	}
}
```

Ensure `undo_all_actions_test.go` imports `github.com/ingyamilmolinar/beatmo/internal/hooks` (it almost certainly already does; add it if not).

- [ ] **Step 2b (only if Step 1 found no kind field): add a `kind` field to the case struct**

If the case struct lacks a `hooks.Kind`, add a field `kind hooks.Kind` to its definition and set it on every existing case to the kind that case exercises (e.g. the add-node case gets `kind: hooks.EventNodeAdded`). Each case's kind is the one its mutation emits. Do not invent kinds — use the one the case's action actually records.

- [ ] **Step 3: Run to verify it passes**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestEveryUndoableKindHasActionCase -v
```
Expected: PASS. If it lists missing kinds, the existing table genuinely lacks a case for them (the registry's undoable set is 26; the existing table has ~17). For each missing undoable kind, add a bespoke case following the file's existing pattern: perform the real mutation via the same UI path, then assert exactly 1 undo step + byte-identical undo + working redo. Re-run until green.

- [ ] **Step 4: Run the full undo suite as a regression check**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Undo' -v 2>&1 | tail -40
```
Expected: all `Undo*` tests PASS. Ignore any unrelated pre-existing failures from concurrent branch work — confirm by name that any failure is not in a file this plan touched.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/undo_all_actions_test.go
git commit -m "test(ui): force a per-action undo case for every undoable kind

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Package gate checkpoint

**Files:** none (verification only)

- [ ] **Step 1: Run the hooks + eventlogger packages**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/... ./internal/eventlogger/... 2>&1 | tail -30
```
Expected: PASS (Tasks 1-4 add no new Kind, so `NumNonVerbose` and eventlogger coverage are unaffected).

- [ ] **Step 2: Run the targeted ui registry/undo tests together**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Undo|Registry|DocumentGaps|ActionCase' -v 2>&1 | tail -40
```
Expected: all listed tests PASS. This is a checkpoint — nothing to commit if clean.

---

## Task 6 (naming-gap closure): give the HPF/LPF toggle a centralized name

Master EQ HPF/LPF toggles are user-reachable (`eq_panel_zone.go:361-368`), mutate exported master-EQ state, but have no `hooks.Kind` — so they cannot appear in the registry and the discipline tests cannot see them. Add one reserved Kind so the name is centralized and the gap is test-visible. **Per the spec, this names + classifies only — it does NOT wire the emit at the UI commit site (deferred follow-up).**

**Files:**
- Modify: `src/go/internal/hooks/events.go` (add Kind, payload, KindAll entry, NumNonVerbose)
- Modify: `src/go/internal/hooks/actions.go` (add registry entry)
- Modify: `src/go/internal/eventlogger/format.go` (add formatter)
- Modify: `src/go/internal/ui/synth_panel_save_test.go:405` (bump NumNonVerbose assertion)

- [ ] **Step 1: Run the coverage tests to confirm the current green baseline**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ -run TestEventLoggerCoversAllNonVerboseKinds -v
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestHooks_NumNonVerboseUpdated -v
```
Expected: PASS — establishes the baseline we are about to intentionally trip.

- [ ] **Step 2: Add the Kind, payload, KindAll entry, and bump NumNonVerbose**

In `src/go/internal/hooks/events.go`:

(a) Add the constant in the "Round 2 additions" const group (after `EventFavoriteToggled`):
```go
	// EventEQFilterToggled names the master/channel EQ HPF or LPF enable toggle.
	// Reserved name only: the UI toggle (eq_panel_zone.go OnToggleHPF/OnToggleLPF)
	// does not emit it yet. Classified ScopeDocument with an ExcludedReason in
	// the action registry until a commit site + undo tap are wired.
	EventEQFilterToggled Kind = "audio.eq_filter_toggled"
```

(b) Add to `KindAll` (the non-verbose section, right after the `EventRowColorChanged, EventCustomWAVLoaded, EventInstrumentRenamed,` line), on its own line:
```go
	EventEQFilterToggled,
```

(c) Bump the count constant (currently `const NumNonVerbose = 53`):
```go
const NumNonVerbose = 54
```
Append to that constant's doc comment: `The HPF/LPF filter-toggle naming gap added 1 (eq filter toggled): 53 + 1 = 54.`

(d) Add a payload type near the other EQ payloads (after `EQBandPayload`):
```go
// EQFilterPayload describes a master/channel EQ HPF/LPF enable toggle. Channel
// is the instrument id ("main", "kick", …), Filter is "hpf" or "lpf".
type EQFilterPayload struct {
	Channel  string  `json:"channel"`
	Filter   string  `json:"filter"`
	Enabled  bool    `json:"enabled"`
	CutoffHz float64 `json:"cutoff_hz,omitempty"`
}
```

- [ ] **Step 3: Add the registry entry**

In `src/go/internal/hooks/actions.go`, add to `ActionRegistry` in the Round 2 group (right after the `EventRowColorChanged` entry):
```go
	// HPF/LPF toggle mutates exported master-EQ state and IS user-reachable, but
	// the UI toggle does not emit/record undo yet.
	{EventEQFilterToggled, "toggle EQ filter", ScopeDocument, "user-reachable but emit + undo tap not wired yet"},
```

- [ ] **Step 4: Add the eventlogger formatter**

In `src/go/internal/eventlogger/format.go`, add to the `formatters` map (near the `EventEQBandChange` entry):
```go
	hooks.EventEQFilterToggled: func(p any) formatted {
		f, _ := p.(hooks.EQFilterPayload)
		state := "off"
		if f.Enabled {
			state = "on"
		}
		return formatted{tag: "eq", msg: fmt.Sprintf("%s %s %s", f.Channel, f.Filter, state)}
	},
```

- [ ] **Step 5: Bump the NumNonVerbose test assertion**

In `src/go/internal/ui/synth_panel_save_test.go` around line 405, change the guard from `!= 53` to `!= 54` and update the `want 53` portion of the error message to `want 54`, appending `; the HPF/LPF filter-toggle naming gap added 1`.

- [ ] **Step 6: Run all affected coverage tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ -run 'TestEveryKindClassified|TestDocumentGapsAreReasoned|TestExcludedReasonOnlyOnDocument' -v
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ -run 'TestEventLoggerCovers' -v
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestHooks_NumNonVerboseUpdated|TestUndoableSetMatchesRegistry|TestDocumentGapsAreReasoned' -v
```
Expected: PASS all. `EventEQFilterToggled` is now classified (ScopeDocument + reason), formatter present, counts consistent, and it does NOT enter the undoable set (it has an ExcludedReason, so `TestUndoableSetMatchesRegistry` still shows 26 undoable kinds).

- [ ] **Step 7: Commit**

```bash
git add src/go/internal/hooks/events.go src/go/internal/hooks/actions.go src/go/internal/eventlogger/format.go src/go/internal/ui/synth_panel_save_test.go
git commit -m "feat(hooks): name the HPF/LPF EQ-filter toggle gap (classify, defer wiring)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Final verification

**Files:** none (verification only)

- [ ] **Step 1: Run hooks + eventlogger in full**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/... ./internal/eventlogger/...
```
Expected: ok for both packages.

- [ ] **Step 2: Run the ui registry/undo guards in full**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Undo|Registry|DocumentGaps|ActionCase|KindClassified' 2>&1 | tail -50
```
Expected: all listed tests PASS. Any failure outside the files this plan touched is pre-existing branch breakage — confirm by test name before attributing.

- [ ] **Step 3: Confirm no stray files were committed**

Run:
```bash
cd /home/ymolinar/Repos/beatmo && git log --oneline -7 && git show --stat --format= HEAD~6..HEAD | grep -vE '^\s*$' | sort -u
```
Expected: only files named in Tasks 1-6 appear across the new commits (`actions.go`, `actions_test.go`, `undo.go`, `undo_registry_drift_test.go`, `undo_coverage_test.go`, `undo_all_actions_test.go`, `events.go`, `format.go`, `synth_panel_save_test.go`). If any unrelated file appears, it captured another agent's work — `git reset` and recommit with explicit paths.

---

## Self-Review notes (addressed in this plan)

- **Spec coverage:** registry (Task 1) ✓; derive documentScopeKinds (Task 2) ✓; `TestEveryKindClassified` (Task 1) ✓; `TestUndoableSetMatchesRegistry` (Task 2) ✓; excluded-have-reason → `TestDocumentGapsAreReasoned` + `TestExcludedReasonOnlyOnDocument` (Tasks 3, 1) ✓; per-action coverage → `TestEveryUndoableKindHasActionCase` (Task 4) ✓; classify gaps now (pan/sends ExcludedReason in Task 1; HPF/LPF naming in Task 6) ✓; eventlogger formatter + NumNonVerbose handled (Task 6) ✓.
- **Emit-tap parity:** intentionally covered behaviorally by Task 4 rather than a brittle source-scan (a missing tap → 0 undo steps → per-action test fails; an extra tap → harmless no-op via `recordUndo`'s `documentScopeKinds` gate).
- **Out of scope (per spec):** actually wiring HPF/LPF/pan/sends emit + undo, and ephemeral UI chrome — intentionally not in any task.
- **Type consistency:** `ActionScope`, `ActionMeta{Kind,Label,Scope,ExcludedReason}`, `MetaFor`, `Undoable`, `AllActions`, `EQFilterPayload` used identically across tasks.
- **Explicit inspect-then-fill (not a placeholder):** Task 4 Step 1 references `<casesVar>` / `<kindField>` because the existing `undo_all_actions_test.go` identifiers must be read from source first; the step says exactly how to find and substitute them.
- **Drift safety net:** if live `KindAll` differs from this plan's 57-entry enumeration (concurrent branch work added a Kind), `TestEveryKindClassified` (Task 1 Step 4) flags it; add the matching registry entry before proceeding.
