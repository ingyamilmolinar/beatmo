# Event Coverage + Snapshot Undo/Redo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every state-mutating user action emit a `hooks` event, then add session-only undo/redo for document/musical state via a snapshot journal that records exactly once per deterministic commit (drag-release or discrete action).

**Architecture:** A self-contained `UndoManager` stores byte snapshots (`exportBytes()` output) on bounded undo/redo rings. It is fed by a synchronous, in-process tap (`recordUndo(kind)`) co-located with the committed-event emit sites — never the async `hooks.Bus` (unordered). Restore re-imports a snapshot through the goldens-tested `Import` path, preserving the playhead via `Seek`. Continuous controls (sliders/knobs) commit one event at drag-release using the existing `sliderGroupHitAdapter.onRelease` hook and the slider/knob `InputConsumed` signal; discrete actions commit immediately.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), the existing `internal/hooks` bus + `internal/eventlogger` formatters.

**Test command (fast path):**
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/hooks/ ./internal/eventlogger/ ./internal/audio/
```

**Conventions:**
- All Go commands use the bundled toolchain: `../../.tools/go/bin/go` from `src/go`.
- New test files use `package ui` (or the package under test) and the `-tags test` fast path unless they exercise real audio (those use `!test`).
- Commit after every task with the message shown.

---

## File Structure

**New files:**
- `src/go/internal/ui/undo.go` — `UndoManager` (pure logic + package-level observer indirection + recorded-set/labels).
- `src/go/internal/ui/undo_test.go` — unit tests for `UndoManager` with fake capture/restore.
- `src/go/internal/ui/undo_seam.go` — `Game.undoCapture` / `Game.undoRestore` (playhead-preserving) + `recordUndo` forwarding.
- `src/go/internal/ui/undo_seam_test.go` — seam + integration tests (round-trip, cascade, playhead).
- `src/go/internal/ui/undo_coverage_test.go` — discipline test pinning the recorded-set.

**Modified files:**
- `src/go/internal/hooks/events.go` — 8 new non-verbose `Kind`s + payload field additions + `KindAll` + `NumNonVerbose`.
- `src/go/internal/eventlogger/format.go` — 8 new formatters.
- `src/go/internal/ui/event_helpers.go` — new emit helpers; committed helpers call `recordUndo`.
- `src/go/internal/ui/js_exports_insert_effects.go` — emit move/toggle.
- `src/go/internal/ui/drumview_ctor.go` — relocate per-frame EQ/master emits to release; wire send/volume/pan commit emits.
- `src/go/internal/ui/row_rack_zone.go`, `transport_zone.go`, `eq_panel_zone.go` — set `onRelease` on slider adapters.
- `src/go/internal/ui/drumview_fx_panel.go` — move FX-param emit to release; send emits.
- `src/go/internal/ui/synth_panel_zone.go` — committed synth-param emit at knob release.
- `src/go/internal/ui/recipe_save_sink.go` — synth reset emit + record.
- `src/go/internal/ui/game_struct.go`, `game_new.go` — `undoManager` field + construction + observer registration.
- `src/go/internal/ui/import.go` — `g.undoManager.OnExternalLoad()` at entry.
- `src/go/internal/ui/input.go` — injectable `isKeyJustPressed`.
- `src/go/internal/ui/game_input_editor.go` — Ctrl/Cmd+Z / +Shift+Z / +Y dispatch.
- `src/go/internal/ui/transport_zone.go` — undo/redo buttons + callbacks + layout (desktop + mobile).

---

## Phase 0 — UndoManager core (pure, no wiring)

### Task 0.1: UndoManager struct + record/undo/redo/labels/ring/guard

**Files:**
- Create: `src/go/internal/ui/undo.go`
- Test: `src/go/internal/ui/undo_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ui

import "testing"

func TestUndoManager_RecordUndoRedo(t *testing.T) {
	state := "A"
	snaps := []string{}
	m := NewUndoManager(
		func() []byte { return []byte(state) },          // capture
		func(b []byte) error { state = string(b); return nil }, // restore
	)
	m.maxDepth = 100

	// Record a transition A -> B.
	state = "B"
	m.record("edit-1")
	// Record B -> C.
	state = "C"
	m.record("edit-2")

	if !m.CanUndo() {
		t.Fatal("expected CanUndo true")
	}
	if got := m.UndoLabel(); got != "edit-2" {
		t.Fatalf("UndoLabel = %q, want edit-2", got)
	}

	m.Undo() // -> B
	if state != "B" {
		t.Fatalf("after undo state=%q want B", state)
	}
	m.Undo() // -> A
	if state != "A" {
		t.Fatalf("after 2nd undo state=%q want A", state)
	}
	if m.CanUndo() {
		t.Fatal("expected CanUndo false at bottom")
	}
	m.Redo() // -> B
	if state != "B" {
		t.Fatalf("after redo state=%q want B", state)
	}
	_ = snaps
}

func TestUndoManager_NoopGestureDropped(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	m.record("noop") // state unchanged since construction baseline
	if m.CanUndo() {
		t.Fatal("identical snapshot must not push a step")
	}
}

func TestUndoManager_NewActionClearsRedo(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	state = "B"
	m.record("b")
	m.Undo() // back to A, redo has B
	if !m.CanRedo() {
		t.Fatal("expected CanRedo true")
	}
	state = "C"
	m.record("c") // new action clears redo
	if m.CanRedo() {
		t.Fatal("new action must clear redo stack")
	}
}

func TestUndoManager_RingBound(t *testing.T) {
	state := "0"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	m.maxDepth = 3
	for i := 1; i <= 10; i++ {
		state = string(rune('0' + i))
		m.record("e")
	}
	// Only the last 3 transitions are retained.
	count := 0
	for m.CanUndo() {
		m.Undo()
		count++
	}
	if count != 3 {
		t.Fatalf("retained %d steps, want 3 (ring bound)", count)
	}
}

func TestUndoManager_RestoringGuardAndExternalLoad(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	state = "B"
	m.record("b")
	// OnExternalLoad clears history (real import).
	m.OnExternalLoad()
	if m.CanUndo() || m.CanRedo() {
		t.Fatal("OnExternalLoad must clear both stacks")
	}
	// While restoring, record is a no-op and OnExternalLoad does NOT clear.
	state = "C"
	m.record("c")
	m.Undo() // baseline is C now; undo to ... nothing recorded after clear except c
	// Set up a redo entry then assert restoring blocks recording during restore.
	state = "D"
	m.record("d")
	m.restoring = true
	state = "E"
	m.record("should-be-ignored")
	m.restoring = false
	if m.UndoLabel() == "should-be-ignored" {
		t.Fatal("record during restoring must be ignored")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestUndoManager -v`
Expected: FAIL — `undefined: NewUndoManager`.

- [ ] **Step 3: Write minimal implementation**

```go
package ui

import "bytes"

// undoEntry is one history step: a label for the UI + the document snapshot
// to restore to (the BEFORE state of the transition this entry represents).
type undoEntry struct {
	label    string
	snapshot []byte
}

// UndoManager is a session-only, bounded snapshot journal. It is decoupled
// from the mutation sites: it knows only how to capture the current document
// (Capture), how to restore one (Restore), and receives a synchronous
// record() call at each deterministic commit. See docs/superpowers/specs/
// 2026-06-10-event-coverage-and-undo-redo-design.md.
type UndoManager struct {
	capture   func() []byte
	restore   func([]byte) error
	committed []byte // snapshot as of the current state
	undo      []undoEntry
	redo      []undoEntry
	maxDepth  int
	restoring bool // true while a restore is applying; suppresses record + external-clear
}

// NewUndoManager constructs a manager. capture must return a deterministic
// byte snapshot of the document; restore must apply one.
func NewUndoManager(capture func() []byte, restore func([]byte) error) *UndoManager {
	return &UndoManager{
		capture:   capture,
		restore:   restore,
		committed: capture(),
		maxDepth:  100,
	}
}

// record snapshots the current (post-change) document and, if it differs from
// the committed baseline, pushes the previous baseline as an undo step. No-op
// gestures (snapshot == baseline) are dropped. Clears redo on a real change.
func (m *UndoManager) record(label string) {
	if m.restoring {
		return
	}
	snap := m.capture()
	if bytes.Equal(snap, m.committed) {
		return
	}
	m.undo = append(m.undo, undoEntry{label: label, snapshot: m.committed})
	if len(m.undo) > m.maxDepth {
		m.undo = m.undo[len(m.undo)-m.maxDepth:]
	}
	m.committed = snap
	m.redo = m.redo[:0]
}

// CanUndo / CanRedo report stack non-emptiness.
func (m *UndoManager) CanUndo() bool { return len(m.undo) > 0 }
func (m *UndoManager) CanRedo() bool { return len(m.redo) > 0 }

// UndoLabel / RedoLabel return the next step's label (or "").
func (m *UndoManager) UndoLabel() string {
	if len(m.undo) == 0 {
		return ""
	}
	return m.undo[len(m.undo)-1].label
}
func (m *UndoManager) RedoLabel() string {
	if len(m.redo) == 0 {
		return ""
	}
	return m.redo[len(m.redo)-1].label
}

// Undo restores the previous document state.
func (m *UndoManager) Undo() {
	if len(m.undo) == 0 {
		return
	}
	e := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.redo = append(m.redo, undoEntry{label: e.label, snapshot: m.committed})
	_ = m.restore(e.snapshot)
	m.committed = e.snapshot
}

// Redo re-applies the most recently undone state.
func (m *UndoManager) Redo() {
	if len(m.redo) == 0 {
		return
	}
	e := m.redo[len(m.redo)-1]
	m.redo = m.redo[:len(m.redo)-1]
	m.undo = append(m.undo, undoEntry{label: e.label, snapshot: m.committed})
	_ = m.restore(e.snapshot)
	m.committed = e.snapshot
}

// OnExternalLoad clears history (called by a real user import/scene-apply).
// No-op while restoring so an undo-driven re-import does not wipe the stacks.
func (m *UndoManager) OnExternalLoad() {
	if m.restoring {
		return
	}
	m.undo = m.undo[:0]
	m.redo = m.redo[:0]
	m.committed = m.capture()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestUndoManager -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/undo.go src/go/internal/ui/undo_test.go
git commit -m "feat(undo): UndoManager snapshot journal core (record/undo/redo/ring/guard)"
```

---

## Phase 1 — Capture/Restore seam on Game

### Task 1.1: Game.undoManager field + playhead-preserving restore seam

**Files:**
- Modify: `src/go/internal/ui/game_struct.go:24` (add field after `drum *DrumView`)
- Create: `src/go/internal/ui/undo_seam.go`
- Modify: `src/go/internal/ui/game_new.go` (construct + wire after `g.drum.game = g`, ~line 116)
- Test: `src/go/internal/ui/undo_seam_test.go`

- [ ] **Step 1: Add the struct field**

In `game_struct.go`, immediately after the `drum *DrumView` line (line 24):

```go
	drum                      *DrumView
	undoManager               *UndoManager
```

- [ ] **Step 2: Write the seam (undo_seam.go)**

```go
package ui

// undoCapture returns a deterministic byte snapshot of the whole document
// (nodes/edges, rows, instruments, BPM/length/subdiv, EQ, insert FX, sends,
// synth params, sample edits) via the goldens-tested export serializer.
func (g *Game) undoCapture() []byte {
	if g.drum == nil {
		return nil
	}
	b, err := g.drum.exportBytes()
	if err != nil {
		// On error return the last committed snapshot shape (nil) so record()
		// treats it as "no change" rather than corrupting history.
		return nil
	}
	return b
}

// undoRestore re-imports a snapshot while preserving the playhead. The
// restoring guard (set by the manager around this call) keeps Import from
// clearing the undo stack and keeps recordUndo a no-op during the rebuild.
func (g *Game) undoRestore(snapshot []byte) error {
	if len(snapshot) == 0 {
		return nil
	}
	div := max1(g.grid.MaxDiv())
	beat := g.playheadAbsSubdiv() / div // back to whole beats
	g.undoManager.restoring = true
	defer func() { g.undoManager.restoring = false }()
	if err := g.Import(snapshot); err != nil {
		return err
	}
	g.Seek(beat) // Import resets the playhead; put it back.
	return nil
}
```

> `max1` already exists in the package (used by `playheadAbsSubdiv`), so no `math` import is needed here — drop the `import "math"` line if the compiler flags it unused. The synchronous tap (`recordUndo`) is introduced in Phase 3 Task 3.1, not here; the seam only needs capture/restore.

- [ ] **Step 3: Construct + register in game_new.go**

After `g.drum.game = g` (game_new.go:116), add:

```go
	// Undo/redo: snapshot journal fed by committed-event taps. Capture/Restore
	// ride the export/import path; the manager is registered as the package
	// observer so emit sites can record without a *Game reference.
	g.undoManager = NewUndoManager(g.undoCapture, g.undoRestore)
	registerUndoObserver(g.undoManager)
```

- [ ] **Step 4: Write the failing seam test**

```go
package ui

import "testing"

func TestUndoSeam_RoundTripPreservesDocument(t *testing.T) {
	assertDefaultParityState(t)
	g := newTestGameForUndo(t) // helper added below
	before := g.undoCapture()

	// Mutate: add a node via the canonical path.
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")

	after := g.undoCapture()
	if string(before) == string(after) {
		t.Fatal("expected document to change after AddRow")
	}

	g.undoManager.Undo()
	restored := g.undoCapture()
	if string(restored) != string(before) {
		t.Fatalf("undo did not restore byte-identical document\nbefore=%s\nrestored=%s", before, restored)
	}
}
```

Add this helper to the same file (reuse existing test scaffolding — see `testutil_test.go` header for the canonical constructor; adapt the name used there):

```go
func newTestGameForUndo(t *testing.T) *Game {
	t.Helper()
	g := New(nil) // matches game_new.go signature: func New(logger *game_log.Logger) *Game
	g.updateBeatInfos()
	return g
}
```

> If `New(nil)` panics under the test stub, use the same constructor the existing `internal/ui` tests use (grep `func Test` in `e2e_workflow_test.go` for the canonical setup, e.g. `newGameForTest(t)`), and replace `newTestGameForUndo` body with that.

- [ ] **Step 5: Run test to verify it fails, then passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestUndoSeam' -v`
Expected first: FAIL — `undefined: registerUndoObserver` / `undoObserver` (added in Phase 3 Task 3.1). 

> **Ordering note:** `recordUndo`, `undoObserver`, and `registerUndoObserver` are defined in Task 3.1. To keep Phase 1 compiling, add the observer scaffold now as a tiny stub at the top of `undo.go` (it is fleshed out in Task 3.1):

```go
// undoObserver is the process-wide sink for committed-mutation taps. One Game
// per process, so a package global is safe (mirrors input.go's var pattern).
var undoObserver interface{ recordKind(label string) }

func registerUndoObserver(m *UndoManager) { undoObserver = undoManagerObserver{m} }

// undoManagerObserver adapts *UndoManager to the observer interface.
type undoManagerObserver struct{ m *UndoManager }

func (o undoManagerObserver) recordKind(label string) { o.m.record(label) }
```

Add the above to `undo.go` now, then re-run. Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/game_struct.go src/go/internal/ui/game_new.go src/go/internal/ui/undo_seam.go src/go/internal/ui/undo_seam_test.go src/go/internal/ui/undo.go
git commit -m "feat(undo): Game capture/restore seam + observer registration (playhead-preserving)"
```

---

## Phase 2 — Close event-coverage gaps

### Task 2.1: New hooks Kinds + payload fields

**Files:**
- Modify: `src/go/internal/hooks/events.go`
- Test: (covered by existing `internal/eventlogger/coverage_test.go` in Task 2.2)

- [ ] **Step 1: Add the Kind constants**

In `events.go`, inside the `const (...)` block, after the `EventInsertEffectParam` line (line 57), add:

```go
	// Audio settings — release-committed (added for full event coverage +
	// undo). These fire once per gesture at drag-release, never per-frame.
	EventRowVolume           Kind = "row.volume"
	EventRowPan              Kind = "row.pan"
	EventInsertEffectMoved   Kind = "audio.insert_moved"
	EventInsertEffectToggled Kind = "audio.insert_toggled"
	EventSendChanged         Kind = "audio.send"
	// Synth params committed once at knob-release (the per-frame edit stays
	// the verbose EventInstrumentParamChanged below). Reset clears overrides.
	EventInstrumentParamsCommitted Kind = "audio.synth_committed"
	EventInstrumentParamsReset     Kind = "audio.synth_reset"
	// Userpref coverage only (NOT undoable): audio-panel state persisted.
	EventAudioPanelStateChanged Kind = "uistate.audio_panel"
```

- [ ] **Step 2: Add them to KindAll**

In the `KindAll` slice, after the `EventInsertEffectAdded, EventInsertEffectRemoved, EventInsertEffectParam,` line (line 130), add:

```go
	EventRowVolume, EventRowPan,
	EventInsertEffectMoved, EventInsertEffectToggled, EventSendChanged,
	EventInstrumentParamsCommitted, EventInstrumentParamsReset,
	EventAudioPanelStateChanged,
```

- [ ] **Step 3: Bump NumNonVerbose**

Change the constant (line 147) and append a one-line note:

```go
// ... The non-destructive sample-edit descriptor added 1 (45). The
// event-coverage + undo pass added 8 (row volume/pan, insert moved/toggled,
// send, synth committed/reset, audio-panel state): 45 + 8 = 53.
const NumNonVerbose = 53
```

- [ ] **Step 4: Extend reused payloads + add two new payloads**

Add `Volume`/`Pan` to `RowChangePayload` (after the `Solo` field, line 278):

```go
	Mute          bool    `json:"mute,omitempty"`
	Solo          bool    `json:"solo,omitempty"`
	Volume        float64 `json:"volume,omitempty"`
	Pan           float64 `json:"pan,omitempty"`
```

Add `Enabled`/`FromSlot`/`ToSlot` to `InsertEffectPayload` (after `Value`, line 331):

```go
	Value    float64 `json:"value,omitempty"`
	Enabled  bool    `json:"enabled,omitempty"`
	FromSlot int     `json:"from_slot,omitempty"`
	ToSlot   int     `json:"to_slot,omitempty"`
```

Add two new payload types at the end of the file (after `KitPayload`):

```go
// SendPayload describes a delay/reverb send change. Kind is "delay" or "reverb".
type SendPayload struct {
	Channel string  `json:"channel"`
	Kind    string  `json:"kind"`
	Value   float64 `json:"value"`
}

// AudioPanelStatePayload describes an audio-panel preference change (spectrum
// slope, Pre overlay, K-20 view, chain A/B taps). Userpref coverage only.
type AudioPanelStatePayload struct {
	Field string `json:"field"`
}
```

- [ ] **Step 5: Build to verify it compiles**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/hooks/`
Expected: builds clean.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/hooks/events.go
git commit -m "feat(hooks): 8 new non-verbose kinds for full event coverage + undo"
```

### Task 2.2: Formatters for the new kinds

**Files:**
- Modify: `src/go/internal/eventlogger/format.go`
- Test: `src/go/internal/eventlogger/coverage_test.go` (existing — will now fail until formatters added)

- [ ] **Step 1: Run the coverage test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ -run TestEventLoggerCoversAllNonVerboseKinds -v`
Expected: FAIL — `missing formatter for non-verbose kind "row.volume"` (and the other 7).

- [ ] **Step 2: Add the formatters**

In `format.go`, inside the `formatters` map, after the `hooks.EventInsertEffectParam` entry (line 182), add:

```go
		hooks.EventRowVolume: func(p any) formatted {
			r, _ := p.(hooks.RowChangePayload)
			return formatted{tag: "row", msg: fmt.Sprintf("row %d volume = %.2f", r.Row, r.Volume)}
		},
		hooks.EventRowPan: func(p any) formatted {
			r, _ := p.(hooks.RowChangePayload)
			return formatted{tag: "row", msg: fmt.Sprintf("row %d pan = %+.2f", r.Row, r.Pan)}
		},
		hooks.EventInsertEffectMoved: func(p any) formatted {
			e, _ := p.(hooks.InsertEffectPayload)
			return formatted{tag: "audio", msg: fmt.Sprintf("insert FX %s moved %d → %d", e.Channel, e.FromSlot, e.ToSlot)}
		},
		hooks.EventInsertEffectToggled: func(p any) formatted {
			e, _ := p.(hooks.InsertEffectPayload)
			state := "off"
			if e.Enabled {
				state = "on"
			}
			return formatted{tag: "audio", msg: fmt.Sprintf("insert FX %s slot=%d %s", e.Channel, e.Slot, state)}
		},
		hooks.EventSendChanged: func(p any) formatted {
			s, _ := p.(hooks.SendPayload)
			return formatted{tag: "audio", msg: fmt.Sprintf("%s send %s = %.2f", s.Kind, s.Channel, s.Value)}
		},
		hooks.EventInstrumentParamsCommitted: func(p any) formatted {
			e, _ := p.(hooks.InstrumentParamPayload)
			return formatted{tag: "audio", msg: fmt.Sprintf("synth params committed %s (recipe=%s)", e.Channel, fallback(e.Recipe, "?"))}
		},
		hooks.EventInstrumentParamsReset: func(p any) formatted {
			e, _ := p.(hooks.InstrumentParamPayload)
			return formatted{tag: "audio", msg: fmt.Sprintf("synth params reset %s", e.Channel)}
		},
		hooks.EventAudioPanelStateChanged: func(p any) formatted {
			s, _ := p.(hooks.AudioPanelStatePayload)
			return formatted{tag: "uistate", msg: fmt.Sprintf("audio panel %s changed", s.Field)}
		},
```

- [ ] **Step 3: Run the coverage tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/eventlogger/ -v`
Expected: PASS (all three coverage tests).

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/eventlogger/format.go
git commit -m "feat(eventlogger): formatters for 8 new event kinds"
```

### Task 2.3: New emit helpers

**Files:**
- Modify: `src/go/internal/ui/event_helpers.go`
- Test: `src/go/internal/ui/event_helpers_new_test.go` (create)

- [ ] **Step 1: Write a failing test that the helpers publish the right kind**

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestNewEmitHelpersPublish(t *testing.T) {
	got := map[hooks.Kind]int{}
	unsub := hooks.Subscribe(hooks.EventRowVolume, func(e hooks.Event) { got[e.Kind]++ })
	defer unsub()
	unsub2 := hooks.Subscribe(hooks.EventSendChanged, func(e hooks.Event) { got[e.Kind]++ })
	defer unsub2()

	emitRowVolume(2, 0.7)
	emitSendChanged("kick", "delay", 0.3)

	// hooks.Bus is async; flush by closing+reopening is overkill — use the
	// test bus drain helper if present, else poll briefly.
	hooks.GlobalBus().FlushForTest() // see note below
	if got[hooks.EventRowVolume] == 0 {
		t.Error("emitRowVolume did not publish EventRowVolume")
	}
	if got[hooks.EventSendChanged] == 0 {
		t.Error("emitSendChanged did not publish EventSendChanged")
	}
}
```

> **Note on async flush:** the bus delivers off-thread. If `hooks.GlobalBus()` has no `FlushForTest`, grep `internal/hooks` for an existing synchronous test helper (e.g. `Bus.Stats()` + a poll loop, or a `drain` test hook). If none exists, replace the assertion with an `eventually(t, func() bool { return got[...] > 0 })` poll (50×10ms). Do NOT add production flush APIs just for the test.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNewEmitHelpersPublish -v`
Expected: FAIL — `undefined: emitRowVolume`.

- [ ] **Step 3: Add the emit helpers**

Append to `event_helpers.go` (before `nodeTypeName`):

```go
// emitRowVolume publishes EventRowVolume once at slider release.
func emitRowVolume(row int, vol float64) {
	hooks.PublishWithSource(hooks.EventRowVolume, hooks.RowChangePayload{Row: row, Volume: vol}, hooks.CaptureSource(1))
}

// emitRowPan publishes EventRowPan once at slider release.
func emitRowPan(row int, pan float64) {
	hooks.PublishWithSource(hooks.EventRowPan, hooks.RowChangePayload{Row: row, Pan: pan}, hooks.CaptureSource(1))
}

// emitInsertEffectMoved publishes EventInsertEffectMoved on reorder.
func emitInsertEffectMoved(channel string, from, to int) {
	hooks.PublishWithSource(hooks.EventInsertEffectMoved, hooks.InsertEffectPayload{Channel: channel, FromSlot: from, ToSlot: to}, hooks.CaptureSource(1))
}

// emitInsertEffectToggled publishes EventInsertEffectToggled on enable/disable.
func emitInsertEffectToggled(channel string, slot int, enabled bool) {
	hooks.PublishWithSource(hooks.EventInsertEffectToggled, hooks.InsertEffectPayload{Channel: channel, Slot: slot, Enabled: enabled}, hooks.CaptureSource(1))
}

// emitSendChanged publishes EventSendChanged once at slider release. kind is
// "delay" or "reverb".
func emitSendChanged(channel, kind string, value float64) {
	hooks.PublishWithSource(hooks.EventSendChanged, hooks.SendPayload{Channel: channel, Kind: kind, Value: value}, hooks.CaptureSource(1))
}

// emitInstrumentParamsCommitted publishes EventInstrumentParamsCommitted once
// at knob release (the per-frame edit stays EventInstrumentParamChanged).
func emitInstrumentParamsCommitted(channel, recipe string) {
	hooks.PublishWithSource(hooks.EventInstrumentParamsCommitted, hooks.InstrumentParamPayload{Channel: channel, Recipe: recipe}, hooks.CaptureSource(1))
}

// emitInstrumentParamsReset publishes EventInstrumentParamsReset.
func emitInstrumentParamsReset(channel, recipe string) {
	hooks.PublishWithSource(hooks.EventInstrumentParamsReset, hooks.InstrumentParamPayload{Channel: channel, Recipe: recipe}, hooks.CaptureSource(1))
}

// emitAudioPanelStateChanged publishes EventAudioPanelStateChanged (userpref
// coverage only — not undoable).
func emitAudioPanelStateChanged(field string) {
	hooks.PublishWithSource(hooks.EventAudioPanelStateChanged, hooks.AudioPanelStatePayload{Field: field}, hooks.CaptureSource(1))
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNewEmitHelpersPublish -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/event_helpers.go src/go/internal/ui/event_helpers_new_test.go
git commit -m "feat(ui): emit helpers for new committed event kinds"
```

### Task 2.4: Emit move/toggle insert-effect events at the UI callers

**Files:**
- Modify: `src/go/internal/ui/js_exports_insert_effects.go:66-77` (moveInsertEffect) and `:98-109` (toggleInsertEffect)
- Test: `src/go/internal/ui/insert_fx_event_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestInsertFXMoveToggleEmit(t *testing.T) {
	seen := map[hooks.Kind]int{}
	u1 := hooks.Subscribe(hooks.EventInsertEffectMoved, func(e hooks.Event) { seen[e.Kind]++ })
	defer u1()
	u2 := hooks.Subscribe(hooks.EventInsertEffectToggled, func(e hooks.Event) { seen[e.Kind]++ })
	defer u2()

	emitInsertEffectMoved("kick", 0, 1)
	emitInsertEffectToggled("kick", 1, false)

	eventually(t, func() bool { return seen[hooks.EventInsertEffectMoved] > 0 && seen[hooks.EventInsertEffectToggled] > 0 })
}
```

> Reuse the `eventually` helper from Task 2.3 (define it once in a shared `*_test.go` if absent: `func eventually(t *testing.T, cond func() bool) { for i:=0;i<50;i++ { if cond() { return }; time.Sleep(10*time.Millisecond) }; t.Fatal("condition not met") }`).

- [ ] **Step 2: Run to verify it fails** (it will pass for the emit functions but we need the wiring; this test guards the helpers — proceed to wire the callers).

- [ ] **Step 3: Wire the JS export callers**

In `js_exports_insert_effects.go`, in the `moveInsertEffect` handler after `audio.MoveInsertEffect(id, from, to)`:

```go
		audio.MoveInsertEffect(id, from, to)
		emitInsertEffectMoved(id, from, to)
		g.drum.recordUndoStep(hooks.EventInsertEffectMoved)
```

In the `toggleInsertEffect` handler after `audio.ToggleInsertEffect(id, slot, enabled)`:

```go
		audio.ToggleInsertEffect(id, slot, enabled)
		emitInsertEffectToggled(id, slot, enabled)
		g.drum.recordUndoStep(hooks.EventInsertEffectToggled)
```

> `recordUndoStep` is the DrumView convenience wrapper added in Task 3.1. If this file lacks a `hooks` import, add `"github.com/ingyamilmolinar/beatmo/internal/hooks"`.

- [ ] **Step 4: Run the package build + test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestInsertFXMoveToggleEmit' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/js_exports_insert_effects.go src/go/internal/ui/insert_fx_event_test.go
git commit -m "feat(ui): emit + record insert-FX move/toggle"
```

### Task 2.5: Relocate per-frame emits to release; add volume/pan/send/synth commit emits

This task makes continuous controls emit (and record) **once at drag-release**, not per-frame. The live `audio.Set*` calls stay per-frame; only the `emit*` + `recordUndoStep` move to the release hook.

**Files:**
- Modify: `src/go/internal/ui/drumview_ctor.go` (OnGainChange ~192-209; SetMainVolume ~580-584; OnVolumeChange ~674)
- Modify: `src/go/internal/ui/row_rack_zone.go:1071` (set `onRelease` on the row-vol slider adapter)
- Modify: `src/go/internal/ui/transport_zone.go:965` (set `onRelease` on the master-vol slider adapter)
- Modify: `src/go/internal/ui/eq_panel_zone.go` (set `onRelease` on the EQ band slider adapter)
- Modify: `src/go/internal/ui/drumview_fx_panel.go:176-186, 838-840` (move FX-param emit to the `!left` release branch)
- Modify: `src/go/internal/ui/synth_panel_zone.go:2628-2649` (committed synth emit in `OnRelease`)
- Test: `src/go/internal/ui/release_commit_test.go` (create)

- [ ] **Step 1: EQ band — remove per-frame emit, capture last (band, db) for release**

In `drumview_ctor.go` `OnGainChange` (line 208), DELETE the `emitEQBandChange(ch, band, db)` line and instead stash the pending commit on DrumView. Add fields to the DrumView struct (find `type DrumView struct` and add near other eq fields):

```go
	eqPendingChannel string
	eqPendingBand    int
	eqPendingGainDB  float64
	eqPendingDirty   bool
```

Replace the deleted emit with:

```go
				dv.eqPendingChannel, dv.eqPendingBand, dv.eqPendingGainDB, dv.eqPendingDirty = ch, band, db, true
```

Then set the EQ slider adapter's `onRelease` (in `eq_panel_zone.go` where the band-gain `sliderGroupHitAdapter` is constructed — grep for the EQ band slider group; set `onRelease: zone.commitEQBand` or wire through a callback). Add a commit method to DrumView:

```go
// commitEQBand emits + records the EQ band change once, at slider release.
func (dv *DrumView) commitEQBand() {
	if !dv.eqPendingDirty {
		return
	}
	dv.eqPendingDirty = false
	emitEQBandChange(dv.eqPendingChannel, dv.eqPendingBand, dv.eqPendingGainDB)
	dv.recordUndoStep(hooks.EventEQBandChange)
}
```

> The EQ panel's band sliders go through the zone; wire the zone's slider-group adapter `onRelease` to call back into DrumView's `commitEQBand`. If the EQ band sliders are handled by a different adapter (e.g. `curveHandleHitAdapter` for the curve handles at `eq_panel_zone.go:1418`), set `commitEQBand` there too so dragging a curve handle also commits once on release.

- [ ] **Step 2: Master volume — move emit to release**

In `drumview_ctor.go` `SetMainVolume` (line 583), DELETE `emitMasterVolumeChange(v)` (keep `audio.SetMainVolume(v)`). Stash the value:

```go
		SetMainVolume: func(v float64) {
			audio.SetMainVolume(v)
			dv.mainVolPending, dv.mainVolPendingDirty = v, true
		},
```

Add DrumView fields `mainVolPending float64`, `mainVolPendingDirty bool` and a commit method:

```go
func (dv *DrumView) commitMainVolume() {
	if !dv.mainVolPendingDirty {
		return
	}
	dv.mainVolPendingDirty = false
	emitMasterVolumeChange(dv.mainVolPending)
	dv.recordUndoStep(hooks.EventMasterVolumeChange)
}
```

In `transport_zone.go:965`, change the master-vol adapter construction to set `onRelease`. The TransportZone needs a callback to DrumView; add `OnMainVolCommit func()` to `TransportCallbacks` and set the adapter:

```go
				Handler: &sliderGroupHitAdapter{group: z.mainVolGroup, onRelease: func() {
					if z.callbacks.OnMainVolCommit != nil {
						z.callbacks.OnMainVolCommit()
					}
				}},
```

Wire `OnMainVolCommit: dv.commitMainVolume` in `drumview_ctor.go` where `TransportCallbacks` is constructed.

- [ ] **Step 3: Row volume — wire OnVolumeChange to fire at release + add pan**

In `row_rack_zone.go:280`, the `rowVolGroup` onChange fires per-frame (live). Set the adapter `onRelease` (line 1071) to commit. Add `OnVolumeCommit func(row int)` to `RowRackCallbacks` (grep the callbacks struct) and:

```go
				Handler: &sliderGroupHitAdapter{group: z.rowVolGroup, onRelease: func() {
					if z.callbacks.OnVolumeCommit != nil {
						z.callbacks.OnVolumeCommit(z.rowVolGroup.Active())
					}
				}},
```

> `SliderGroup.Active()` returns -1 after release (capture cleared). Instead capture the active index BEFORE release: store `z.lastVolRow = idx` inside the `rowVolGroup` onChange callback (line 280), and pass `z.lastVolRow` to `OnVolumeCommit`.

In `drumview_ctor.go`, wire `OnVolumeCommit` to a DrumView method:

```go
func (dv *DrumView) commitRowVolume(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	emitRowVolume(row, dv.Rows[row].Volume)
	dv.recordUndoStep(hooks.EventRowVolume)
}
```

> **Pan:** if a pan control exists, mirror this exactly with `emitRowPan` + `hooks.EventRowPan`. Grep `internal/ui` for where `Rows[*].Pan` / `SetChannelPan` is driven from a UI control; if there is no pan UI today, add a `// TODO(pan-ui)` note and skip — the event/undo wiring lands when the control does. Do NOT fabricate a control.

- [ ] **Step 4: FX insert param — move emit to release branch**

In `drumview_fx_panel.go` `propagateFXSliderValue` (line 185), DELETE the `emitInsertEffectParam(...)` line (keep `audio.SetInsertEffectParam(...)`). In the desktop release branch (line 838-840, the `if !left { dv.fxSliderDragging = false }`), add a commit:

```go
	if dv.fxSliderDragging {
		idx := dv.fxSliderDragIdx
		if idx < len(dv.fxPanelSliders) {
			sl := dv.fxPanelSliders[idx]
			sl.HandleInputResult(mx, my, left)
			dv.propagateFXSliderValue(idx)
		}
		if !left {
			dv.fxSliderDragging = false
			dv.commitFXSlider(idx)
		}
		return true
	}
```

Add the commit method:

```go
func (dv *DrumView) commitFXSlider(idx int) {
	if idx < 0 || idx >= len(dv.fxSliderBindings) {
		return
	}
	b := dv.fxSliderBindings[idx]
	sl := dv.fxPanelSliders[idx]
	actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	instID := dv.Rows[dv.fxPanelRow].Instrument
	emitInsertEffectParam(instID, b.slotIndex, b.paramName, actual)
	dv.recordUndoStep(hooks.EventInsertEffectParam)
}
```

> The mobile deferred-tap path (line 992) is a single discrete set, not a drag — keep its `emitInsertEffectParam(...)` and add `dv.recordUndoStep(hooks.EventInsertEffectParam)` right after it.
> **Sends:** if the FX overlay has delay/reverb send sliders (grep `SetDelaySend`/`SetReverbSend` callers in `internal/ui`), wire them with the same release pattern using `emitSendChanged(instID, "delay"|"reverb", v)` + `dv.recordUndoStep(hooks.EventSendChanged)`. If sends are import-only today, add the emit at whatever UI control sets them when that control exists; note it inline.

- [ ] **Step 5: Synth knob — committed emit at release**

In `synth_panel_zone.go` `synthKnobHitAdapter.OnRelease` (line 2628), after `h.propagateIfStable()`:

```go
	k := h.currentKnob()
	if k != nil {
		k.HandleInputResult(x, y, false)
		h.propagateIfStable()
		recipe := audio.RecipeForInstrument(h.instID)
		emitInstrumentParamsCommitted(h.instID, recipe)
		h.dv.recordUndoStep(hooks.EventInstrumentParamsCommitted)
	}
```

> Confirm `h.instID` and `audio.RecipeForInstrument` are in scope here (the agent confirmed `audio.RecipeForInstrument` exists; `instID` is a field on the adapter per the OnRelease body). If `recordUndoStep` needs `h.dv`, the adapter already holds `dv` (used in the existing `h.dv.sdbg(...)` line).

- [ ] **Step 6: Synth reset — emit + record**

In `recipe_save_sink.go` `resetRecipeForInstrument` (line 129), after `audio.ResetInstrumentParams(instID)`:

```go
	audio.ResetRecipeToShipped(recipeID)
	audio.ResetInstrumentParams(instID)
	emitInstrumentParamsReset(instID, recipeID)
	recordUndoForReset() // see note
	return recipeID
```

> This function may not have a `*DrumView`/`*Game`. Use the free-function tap: `recordUndo(hooks.EventInstrumentParamsReset, undoLabelFor(hooks.EventInstrumentParamsReset))` (defined in Task 3.1). Replace `recordUndoForReset()` with that call. Add the `hooks` import if missing.

- [ ] **Step 7: Write the release-commit test**

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestEQBandCommitsOnceOnRelease(t *testing.T) {
	count := 0
	u := hooks.Subscribe(hooks.EventEQBandChange, func(e hooks.Event) { count++ })
	defer u()
	g := newTestGameForUndo(t)
	// Simulate a per-frame drag: many OnGainChange calls, then one release.
	for i := 0; i < 5; i++ {
		g.drum.eqPendingChannel, g.drum.eqPendingBand, g.drum.eqPendingGainDB, g.drum.eqPendingDirty = "main", 3, float64(i), true
	}
	g.drum.commitEQBand()
	eventually(t, func() bool { return count == 1 })
	// A second commit with no new drag must be a no-op.
	g.drum.commitEQBand()
	if count != 1 {
		t.Fatalf("commitEQBand fired %d times, want 1 (no-op when clean)", count)
	}
}
```

- [ ] **Step 8: Run the UI package tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEQBandCommitsOnceOnRelease|TestNewEmitHelpersPublish' -v`
Expected: PASS. Then run the broader UI suite to catch relocated-emit fallout:
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS (fix any test that asserted per-frame EQ/master/FX emits — update it to assert on release).

- [ ] **Step 9: Commit**

```bash
git add -A src/go/internal/ui/
git commit -m "feat(ui): relocate continuous-control emits to drag-release; add volume/pan/send/synth commit emits"
```

---

## Phase 3 — Undo tap wiring

### Task 3.1: Observer indirection, recorded-set, labels, DrumView wrapper

**Files:**
- Modify: `src/go/internal/ui/undo.go` (flesh out the Task 1.1 stub)
- Modify: `src/go/internal/ui/event_helpers.go` (committed discrete helpers call `recordUndo`)
- Test: `src/go/internal/ui/undo_coverage_test.go` (create)

- [ ] **Step 1: Replace the Task 1.1 observer stub with the full version**

In `undo.go`, replace the stub block with:

```go
import "github.com/ingyamilmolinar/beatmo/internal/hooks"

// undoObserver is the process-wide sink for committed-mutation taps. One Game
// per process, so a package global is safe (mirrors input.go's var pattern).
var undoObserver interface{ recordKind(label string) }

func registerUndoObserver(m *UndoManager) { undoObserver = undoManagerObserver{m} }

type undoManagerObserver struct{ m *UndoManager }

func (o undoManagerObserver) recordKind(label string) { o.m.record(label) }

// documentScopeKinds is the recorded-set: committed document-state kinds that
// produce an undo step. Transport, import/scene, app prefs, library ops, and
// verbose kinds are intentionally absent. Pinned by undo_coverage_test.go.
var documentScopeKinds = map[hooks.Kind]string{
	hooks.EventNodeAdded:                 "add node",
	hooks.EventNodeDeleted:               "delete node",
	hooks.EventNodeMoved:                 "move node",
	hooks.EventNodeTypeChanged:           "change node type",
	hooks.EventNodeParamsChanged:         "edit node",
	hooks.EventStartNodeChanged:          "set start node",
	hooks.EventEdgeAdded:                 "add edge",
	hooks.EventEdgeDeleted:               "delete edge",
	hooks.EventRowAdded:                  "add row",
	hooks.EventRowDeleted:                "delete row",
	hooks.EventRowInstrumentChange:       "change instrument",
	hooks.EventRowMute:                   "mute row",
	hooks.EventRowSolo:                   "solo row",
	hooks.EventRowColorChanged:           "recolor row",
	hooks.EventRowVolume:                 "set row volume",
	hooks.EventRowPan:                    "set row pan",
	hooks.EventInstrumentRenamed:         "rename instrument",
	hooks.EventBPMChange:                 "change BPM",
	hooks.EventSubdivChange:              "change subdivision",
	hooks.EventLengthChange:              "change length",
	hooks.EventMasterVolumeChange:        "set master volume",
	hooks.EventEQBandChange:              "adjust EQ",
	hooks.EventInsertEffectAdded:         "add effect",
	hooks.EventInsertEffectRemoved:       "remove effect",
	hooks.EventInsertEffectParam:         "adjust effect",
	hooks.EventInsertEffectMoved:         "reorder effect",
	hooks.EventInsertEffectToggled:       "toggle effect",
	hooks.EventSendChanged:               "set send",
	hooks.EventInstrumentParamsCommitted: "edit synth",
	hooks.EventInstrumentParamsReset:     "reset synth",
	hooks.EventSampleEditChanged:         "edit sample",
}

// undoLabelFor returns the UI label for a recorded kind ("" if not recorded).
func undoLabelFor(k hooks.Kind) string { return documentScopeKinds[k] }

// recordUndo is the synchronous tap. Free-function form so emit helpers can
// call it without a *Game. No-op when kind is out of the recorded-set.
func recordUndo(k hooks.Kind) {
	label, ok := documentScopeKinds[k]
	if !ok || undoObserver == nil {
		return
	}
	undoObserver.recordKind(label)
}

// recordUndoStep is the DrumView-scoped convenience wrapper used at UI commit
// sites that already hold a *DrumView.
func (dv *DrumView) recordUndoStep(k hooks.Kind) { recordUndo(k) }
```

Remove the old `recordUndo(kind interface{ String() string }, label string)` from `undo_seam.go` (superseded by this typed version). Update its one caller note in Task 2.5 Step 6 to `recordUndo(hooks.EventInstrumentParamsReset)`.

- [ ] **Step 2: Make committed discrete emit helpers record**

For the discrete graph/row helpers that are committed-by-nature, append a `recordUndo(<kind>)` call inside each helper in `event_helpers.go`. Add to the end of these helper bodies (one line each): `emitNodeAdded` → `recordUndo(hooks.EventNodeAdded)`; `emitNodeDeleted`, `emitNodeMoved`, `emitNodeTypeChanged`, `emitNodeParamsChanged`, `emitStartNodeChanged`, `emitEdgeAdded`, `emitEdgeDeleted`, `emitRowAdded`, `emitRowDeleted`, `emitRowInstrumentChange`, `emitRowMute`, `emitRowSolo`, `emitRowColorChanged`, `emitInstrumentRenamed`, `emitBPMChange`, `emitSubdivChange`, `emitLengthChange`, `emitInsertEffectAdded`, `emitInsertEffectRemoved` — each gets its matching `recordUndo(hooks.Event…)` as the last statement.

Example (`emitNodeAdded`):

```go
func emitNodeAdded(n *uiNode, t model.NodeType) {
	if n == nil {
		return
	}
	hooks.PublishWithSource(hooks.EventNodeAdded, hooks.NodeEdit{
		ID: int(n.ID), I: n.I, J: n.J, Type: nodeTypeName(t),
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeAdded)
}
```

> The new committed helpers from Task 2.3 (`emitRowVolume`, `emitRowPan`, `emitSendChanged`, `emitInstrumentParamsCommitted`, `emitInstrumentParamsReset`) are recorded at their **call sites** (Task 2.5) via `recordUndoStep`, NOT inside the helper — because those helpers fire only from release handlers and we want the record co-located with the gesture commit. Do not double-record. `emitEQBandChange`, `emitMasterVolumeChange`, `emitInsertEffectParam` are likewise recorded at their relocated release call sites (Task 2.5), so do NOT add `recordUndo` inside those three helpers. (`emitSampleEditChanged` lives in the audio package; record it at the UI sampler-save site — grep `internal/ui` for the Sampler Save handler and add `recordUndo(hooks.EventSampleEditChanged)` there.)

- [ ] **Step 3: Write the discipline test**

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// wantUndoableKinds is the hand-maintained source of truth for the undo
// recorded-set. Changing the recorded-set requires updating this list, which
// is the point: it forces a deliberate decision.
var wantUndoableKinds = []hooks.Kind{
	hooks.EventNodeAdded, hooks.EventNodeDeleted, hooks.EventNodeMoved,
	hooks.EventNodeTypeChanged, hooks.EventNodeParamsChanged, hooks.EventStartNodeChanged,
	hooks.EventEdgeAdded, hooks.EventEdgeDeleted,
	hooks.EventRowAdded, hooks.EventRowDeleted, hooks.EventRowInstrumentChange,
	hooks.EventRowMute, hooks.EventRowSolo, hooks.EventRowColorChanged,
	hooks.EventRowVolume, hooks.EventRowPan, hooks.EventInstrumentRenamed,
	hooks.EventBPMChange, hooks.EventSubdivChange, hooks.EventLengthChange,
	hooks.EventMasterVolumeChange, hooks.EventEQBandChange,
	hooks.EventInsertEffectAdded, hooks.EventInsertEffectRemoved, hooks.EventInsertEffectParam,
	hooks.EventInsertEffectMoved, hooks.EventInsertEffectToggled, hooks.EventSendChanged,
	hooks.EventInstrumentParamsCommitted, hooks.EventInstrumentParamsReset,
	hooks.EventSampleEditChanged,
}

func TestUndoRecordedSetMatchesDeclared(t *testing.T) {
	want := map[hooks.Kind]bool{}
	for _, k := range wantUndoableKinds {
		want[k] = true
		if documentScopeKinds[k] == "" {
			t.Errorf("recorded kind %q missing a label in documentScopeKinds", k)
		}
	}
	for k := range documentScopeKinds {
		if !want[k] {
			t.Errorf("documentScopeKinds has %q not in wantUndoableKinds — deliberate? update the list", k)
		}
	}
	for _, k := range wantUndoableKinds {
		if _, ok := documentScopeKinds[k]; !ok {
			t.Errorf("wantUndoableKinds has %q but documentScopeKinds does not", k)
		}
	}
}

func TestUndoRecordedSetExcludesNonDocument(t *testing.T) {
	excluded := []hooks.Kind{
		hooks.EventPlayStart, hooks.EventPlayStop, hooks.EventSeek,
		hooks.EventImport, hooks.EventExport, hooks.EventFavoriteToggled,
		hooks.EventAudioPanelStateChanged, hooks.EventInstrumentParamChanged,
		hooks.EventRecipeSaved, hooks.EventSceneApplied,
	}
	for _, k := range excluded {
		if _, ok := documentScopeKinds[k]; ok {
			t.Errorf("%q must NOT be in the undo recorded-set", k)
		}
	}
}
```

- [ ] **Step 4: Run discipline + undo tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestUndo' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/undo.go src/go/internal/ui/undo_seam.go src/go/internal/ui/event_helpers.go src/go/internal/ui/undo_coverage_test.go
git commit -m "feat(undo): observer indirection + recorded-set + discipline test; tap committed emit helpers"
```

### Task 3.2: Import clears history at entry

**Files:**
- Modify: `src/go/internal/ui/import.go:69` (add after the deferred recover handler)
- Test: `src/go/internal/ui/undo_seam_test.go` (add)

- [ ] **Step 1: Write the failing test**

```go
func TestImportClearsUndoHistory(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected undo available")
	}
	snap := g.undoCapture()
	if err := g.Import(snap); err != nil {
		t.Fatalf("import: %v", err)
	}
	if g.undoManager.CanUndo() || g.undoManager.CanRedo() {
		t.Fatal("real Import must clear undo history")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestImportClearsUndoHistory -v`
Expected: FAIL (history survives).

- [ ] **Step 3: Add the clear call**

In `import.go`, immediately after the deferred `recover()` block (before the "Stop sequencer during import" comment), add:

```go
	// Drop undo history on a real project load — you cannot undo across a
	// document swap. No-op while an undo-restore is re-importing (guarded by
	// the manager's restoring flag).
	if g.undoManager != nil {
		g.undoManager.OnExternalLoad()
	}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestImportClearsUndoHistory -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/import.go src/go/internal/ui/undo_seam_test.go
git commit -m "feat(undo): clear history on real import (restore-guarded)"
```

---

## Phase 4 — UI surface

### Task 4.1: Transport bar undo/redo buttons + callbacks

**Files:**
- Modify: `src/go/internal/ui/transport_zone.go` (struct fields ~42-144; `initButtons` ~169; `TransportCallbacks`; desktop layout ~807; mobile layout ~683)
- Modify: `src/go/internal/ui/drumview_ctor.go` (wire `OnUndo`/`OnRedo` in TransportCallbacks)
- Test: `src/go/internal/ui/transport_undo_buttons_test.go` (create)

- [ ] **Step 1: Add callbacks + buttons + wiring**

In `transport_zone.go`, add to the `TransportCallbacks` struct: `OnUndo func()`, `OnRedo func()`, `CanUndo func() bool`, `CanRedo func() bool`, plus the `OnMainVolCommit func()` from Task 2.5.

Add struct fields after `overflowBtn *Button` (line ~63): `undoBtn *Button`, `redoBtn *Button`.

In `initButtons()` (after `recordBtn` init, ~line 197):

```go
	z.undoBtn = NewButton("", nil, func() {
		hapticTransportTap()
		if z.callbacks.OnUndo != nil {
			z.callbacks.OnUndo()
		}
	})
	z.undoBtn.Icon = string(IconUndo) // see icon note
	z.redoBtn = NewButton("", nil, func() {
		hapticTransportTap()
		if z.callbacks.OnRedo != nil {
			z.callbacks.OnRedo()
		}
	})
	z.redoBtn.Icon = string(IconRedo)
```

> **Icon note:** grep `internal/ui/icons.go` for `IconUndo`/`IconRedo`. If absent, either (a) add two `IconID` enum values + glyph paths in `icons.go` (preferred — forbidden-glyph rule bans raw `↶`/`↷` text), or (b) temporarily use text labels `NewButton("Undo", …)` / `NewButton("Redo", …)` and file a follow-up to add icons. Pick (a) if the icon system is data-driven; the discipline tests forbid raw chrome glyphs in source.

In `drumview_ctor.go`, where `TransportCallbacks{...}` is constructed, add:

```go
		OnUndo:        func() { if dv.game != nil { dv.game.undoManager.Undo() } },
		OnRedo:        func() { if dv.game != nil { dv.game.undoManager.Redo() } },
		CanUndo:       func() bool { return dv.game != nil && dv.game.undoManager.CanUndo() },
		CanRedo:       func() bool { return dv.game != nil && dv.game.undoManager.CanRedo() },
		OnMainVolCommit: dv.commitMainVolume,
```

- [ ] **Step 2: Lay out the buttons (desktop)**

In the desktop layout (`layoutDesktop` near line 807), extend the transport button grid to include two more cells for undo/redo (follow the exact `play/stop/record` pattern at lines 816-826). Add after the record button placement:

```go
	undoRect := safeInsetTransport(z.transportGroup.Cell(3, 0), pad)
	undoRect = enforceMinSize(undoRect, minBtn, minBtn)
	z.undoBtn.SetRect(undoRect)

	redoRect := safeInsetTransport(z.transportGroup.Cell(4, 0), pad)
	redoRect = enforceMinSize(redoRect, minBtn, minBtn)
	z.redoBtn.SetRect(redoRect)
```

> Adjust the `transportGroup` column count where it is constructed (grep `transportGroup =` / `NewGridLayout`) to add 2 columns. Shift subsequent buttons' cell indices accordingly. Mirror the same in `layoutMobile` (line 683): extend the `weights` arrays and add `z.undoBtn.SetRect(...)`, `z.redoBtn.SetRect(...)`. Keep undo/redo on the primary toolbar (not the overflow menu) — they are important live-use controls.

- [ ] **Step 3: Draw + dim + register hit areas**

In the zone's `Draw` (grep where `z.recordBtn.Draw(...)` is called), add `z.undoBtn.Draw(dst)` / `z.redoBtn.Draw(dst)`. Before drawing, set the dim state each frame (in Layout or Draw):

```go
	if z.callbacks.CanUndo != nil && !z.callbacks.CanUndo() {
		z.undoBtn.IconColor = colDisabledIcon // grep theme for the dim token; e.g. WithAlpha(colOnSurface, AlphaSubtle)
	} else {
		z.undoBtn.IconColor = colOnSurface
	}
	// same for redoBtn via CanRedo
```

In the hit-area registration (grep where `recordBtn` hit area is added), register undo/redo button hit areas the same way.

- [ ] **Step 4: Write the test**

```go
package ui

import "testing"

func TestTransportUndoRedoButtonsWired(t *testing.T) {
	g := newTestGameForUndo(t)
	// Make a change so undo is available.
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")

	tz := g.drum.transportZone() // grep for the accessor; else reach via tree
	if tz == nil || tz.undoBtn == nil {
		t.Fatal("undo button not constructed")
	}
	before := g.undoCapture()
	// Simulate the button click via its OnClick.
	tz.undoBtn.OnClick()
	if !g.undoManager.CanRedo() {
		t.Fatal("after undo click, redo should be available")
	}
	_ = before
}
```

> If there is no `transportZone()` accessor on DrumView, add a small test-only accessor in a `_test.go` file or reach the zone through the existing DrumView field (grep `transportZone` / `TransportZone` field name in `drumview_*.go`).

- [ ] **Step 5: Run + commit**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTransportUndoRedoButtonsWired' -v`
Expected: PASS.

```bash
git add -A src/go/internal/ui/ src/go/internal/ui/icons.go
git commit -m "feat(ui): transport undo/redo buttons (desktop + mobile, dim when empty)"
```

### Task 4.2: Keyboard shortcuts

**Files:**
- Modify: `src/go/internal/ui/input.go` (add injectable `isKeyJustPressed`)
- Modify: `src/go/internal/ui/game_input_editor.go` (dispatch)
- Test: `src/go/internal/ui/undo_keyboard_test.go` (create)

- [ ] **Step 1: Add injectable just-pressed var**

In `input.go`, in the `var (...)` block with the other input vars (line ~54), add:

```go
	isKeyJustPressed = inpututil.IsKeyJustPressed
```

Add `"github.com/hajimehoshi/ebiten/v2/inpututil"` to the imports if not present.

- [ ] **Step 2: Dispatch in handleEditor**

In `game_input_editor.go`, near the top of `handleEditor()` (after the existing `shift := ...` line), add:

```go
	ctrl := isKeyPressed(ebiten.KeyControlLeft) || isKeyPressed(ebiten.KeyControlRight) ||
		isKeyPressed(ebiten.KeyMetaLeft) || isKeyPressed(ebiten.KeyMetaRight)
	if ctrl && isKeyJustPressed(ebiten.KeyZ) {
		if shift {
			g.undoManager.Redo()
		} else {
			g.undoManager.Undo()
		}
		return
	}
	if ctrl && isKeyJustPressed(ebiten.KeyY) {
		g.undoManager.Redo()
		return
	}
```

> Confirm `shift` is computed before this block; if `handleEditor` computes `shift` lower down, hoist this block below it. Confirm `ebiten.KeyMetaLeft/Right` exist in the vendored ebiten version (grep `KeyMeta` in the module); if not, drop the Meta clause (Ebiten maps Cmd→Control on macOS in many builds).

- [ ] **Step 3: Write the test**

```go
package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestUndoKeyboardShortcut(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")

	// Stub input: Ctrl held, Z just-pressed.
	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyControlLeft: true}, map[ebiten.Key]bool{ebiten.KeyZ: true})
	defer restore()

	before := g.undoCapture()
	g.handleEditor()
	if !g.undoManager.CanRedo() {
		t.Fatal("Ctrl+Z should have triggered undo")
	}
	_ = before
}

// stubKeys overrides the input vars for the test and returns a restore func.
func stubKeys(pressed, justPressed map[ebiten.Key]bool) func() {
	oldP, oldJ := isKeyPressed, isKeyJustPressed
	isKeyPressed = func(k ebiten.Key) bool { return pressed[k] }
	isKeyJustPressed = func(k ebiten.Key) bool { return justPressed[k] }
	return func() { isKeyPressed, isKeyJustPressed = oldP, oldJ }
}
```

> If `handleEditor` early-returns before reaching the new block under the test stub (e.g. it checks sidebar/cursor first), the test may need additional stubs. Read `handleEditor` and place the undo block early enough that it is reachable, or call a smaller extracted `g.handleUndoRedoKeys()` directly and test that.

- [ ] **Step 4: Run + commit**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestUndoKeyboardShortcut -v`
Expected: PASS.

```bash
git add src/go/internal/ui/input.go src/go/internal/ui/game_input_editor.go src/go/internal/ui/undo_keyboard_test.go
git commit -m "feat(ui): Ctrl/Cmd+Z / +Shift+Z / +Y undo-redo keyboard shortcuts"
```

---

## Phase 5 — Integration tests

### Task 5.1: Per-action round-trip + cascade + playhead

**Files:**
- Test: `src/go/internal/ui/undo_integration_test.go` (create)

- [ ] **Step 1: Write the integration tests**

```go
package ui

import "testing"

// TestUndoRoundTripPerAction drives each undoable action through its canonical
// mutation site and asserts undo restores a byte-identical document, redo
// re-applies it.
func TestUndoRoundTripPerAction(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(g *Game)
		kind   string
	}{
		{"add-row", func(g *Game) { g.drum.AddRow() }, "add row"},
		{"bpm", func(g *Game) { g.drum.SetBPM(140) }, "change BPM"},
		// extend with delete-row, mute, instrument change, EQ commit, etc.
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGameForUndo(t)
			before := g.undoCapture()
			tc.mutate(g)
			g.updateBeatInfos()
			g.undoManager.record(tc.kind)
			after := g.undoCapture()
			if string(after) == string(before) {
				t.Fatalf("%s did not change the document", tc.name)
			}
			g.undoManager.Undo()
			if got := g.undoCapture(); string(got) != string(before) {
				t.Fatalf("%s: undo not byte-identical\nwant=%s\ngot =%s", tc.name, before, got)
			}
			g.undoManager.Redo()
			if got := g.undoCapture(); string(got) != string(after) {
				t.Fatalf("%s: redo not byte-identical", tc.name)
			}
		})
	}
}

// TestUndoNodeDeleteCascade asserts undoing a node delete restores the node
// AND its incident edges (the snapshot captures cascades for free).
func TestUndoNodeDeleteCascade(t *testing.T) {
	g := newTestGameForUndo(t)
	// Build: two nodes + an edge between them, then delete one node.
	// (Use the canonical add/edge/delete sites — grep tryAddNode / addEdge /
	// deleteNodeInternal for the exact helpers.)
	t.Skip("fill in with canonical node/edge add + delete; assert undo restores node+edge")
}

// TestUndoPreservesPlayheadDuringPlayback asserts an undo while playing keeps
// the playhead near its pre-undo beat.
func TestUndoPreservesPlayheadDuringPlayback(t *testing.T) {
	g := newTestGameForUndo(t)
	g.SetPlaying(true)
	g.Seek(3)
	g.drum.SetBPM(150)
	g.undoManager.record("change BPM")
	beatBefore := g.playheadAbsSubdiv() / max1(g.grid.MaxDiv())
	g.undoManager.Undo()
	beatAfter := g.playheadAbsSubdiv() / max1(g.grid.MaxDiv())
	if beatAfter != beatBefore {
		t.Fatalf("playhead beat moved across undo: before=%d after=%d", beatBefore, beatAfter)
	}
}
```

- [ ] **Step 2: Fill in the cascade test**

Read `game_graph_nodes.go` (`tryAddNode`), `game_graph_edges.go` (`addEdge`), and the node-delete path; replace the `t.Skip` with: add node A, add node B, add edge A→B, snapshot, delete A (which removes the edge), `record("delete node")`, then `Undo()` and assert both node A and edge A→B exist again by inspecting `g.graph`.

- [ ] **Step 3: Run + commit**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestUndo' -v`
Expected: PASS.

```bash
git add src/go/internal/ui/undo_integration_test.go
git commit -m "test(undo): per-action round-trip, cascade, playhead-preservation integration tests"
```

### Task 5.2: Full suite + cross-platform sanity

**Files:** none (verification only)

- [ ] **Step 1: Run the full fast UI/audio/hooks/eventlogger suite**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/hooks/ ./internal/eventlogger/ ./internal/audio/
```
Expected: PASS. (Note from memory: `internal/ui` had pre-existing flakes on this branch — diff against the baseline before blaming this change; see project memory `add_core_node_types_preexisting_failures`.)

- [ ] **Step 2: Bridge smoke (no new JS export added, but confirm no drift)**

Run:
```bash
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
```
Expected: PASS. (We added no JS exports; this confirms the catalogue is unaffected.)

- [ ] **Step 3: Commit any test fixups, then finish**

```bash
git add -A
git commit -m "test(undo): full-suite verification fixups" --allow-empty
```

---

## Self-Review Notes (addressed during authoring)

- **Spec coverage:** Part 1 (event gaps) → Tasks 2.1–2.5. Part 2 (undo engine) → Tasks 0.1, 1.1, 3.1–3.2. Part 3 (UI) → Tasks 4.1–4.2 (keyboard + buttons + mobile). Testing section → Tasks 5.1–5.2 + per-task TDD. Decoupling seam → Task 3.1 observer indirection. Deterministic commit (no timers) → Task 2.5 release-driven emits.
- **Type consistency:** `record(label string)` (manager) vs `recordKind(label string)` (observer) vs `recordUndo(k hooks.Kind)` (free tap) vs `recordUndoStep(k hooks.Kind)` (DrumView wrapper) — distinct names, distinct roles, used consistently. `documentScopeKinds` + `undoLabelFor` + `wantUndoableKinds` align in Task 3.1/3.3.
- **Open discovery items flagged inline (not placeholders — they are "locate exact site X using pattern Y"):** row pan UI control existence; send-slider UI existence; `IconUndo`/`IconRedo` presence; `transportZone()` accessor; `handleEditor` early-return ordering; async-bus test-flush helper. Each carries the fallback to use if the primary site is absent.
- **Risk:** relocating EQ/master/FX per-frame emits to release (Task 2.5) may break tests asserting per-frame emit — Step 8 runs the full UI suite to catch and fix them.
