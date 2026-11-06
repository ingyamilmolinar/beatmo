# Node Quick-Action Modes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Centralize grid interaction modes (move/copy/connect) into one `gridModeManager`, add a Copy option + desktop mouse-hold entry to the node long-press popup, make interactive move placement edge-only with immediate commit + stitch, add blink/ghost-validity visuals, and emit hooks.Bus events for every step.

**Architecture:** A new `gridModeManager` (single mutation chokepoint: `Enter`/`Cancel`/`Commit`, sync observers, centralized event emission) absorbs the scattered `moveMode`/`movingNode`/`connectMode`/`connectFromNode` fields. The existing long-press popup gains a Copy keycap and popup/tap telemetry; `handleTapInGrid` and `handleEditor` route armed-mode taps through one shared `handleArmedModeTap`. The move-confirm dialog is retired.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), Ebiten + ebitenstub tests, hooks.Bus/eventlogger, Playwright browser tests.

**Spec:** `docs/superpowers/specs/2026-07-17-node-quick-action-modes-design.md`

## Global Constraints

- **NEVER run `git add` or `git commit`** — standing user preference. Where this plan says "Checkpoint", run the listed tests and report; the user commits.
- Go commands: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>` (fast path, stubbed Ebiten).
- Browser tests: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/<name>.browser.test.js` (absolute GO path).
- All hooks bracket tags lowercase. Every new `hooks.Kind` needs: KindAll entry, ActionRegistry row, eventlogger formatter, `NumNonVerbose` bump.
- No new `color.RGBA{}` literals in `internal/ui/` (token discipline ratchet); use existing `genColor*`/`genAlpha*` tokens + `WithAlpha`.
- No forbidden glyphs (`▶`, `✕`, …) in `internal/ui/` source.
- Functional tests drive real `Game.Update`/input paths (see `internal/ui/testutil_test.go` header + existing `TestHandleTapInGrid_*` for setup patterns); do not hand-poke private state except via existing test seams.
- `hooks.Bus` delivery is async (worker pool) — event tests use the subscribe-channel pattern from `internal/ui/uitap_log_test.go` with timeouts.

## File Structure

| File | Responsibility |
|---|---|
| Create `internal/ui/grid_mode.go` | `GridModeKind`, `GridModeObserver`, `gridModeManager` (Enter/Cancel/Commit/Tick/blinkOn/Register/Active/Is/Subject) |
| Create `internal/ui/grid_mode_test.go` | Manager transition matrix + observer + event-emission unit tests |
| Create `internal/ui/grid_mode_discipline_test.go` | Structural guard: no manager field writes outside `grid_mode.go` |
| Create `internal/ui/grid_mode_flow_test.go` | End-to-end functional tests (popup→arm→tap matrix, mouse-hold, event sequences) |
| Modify `internal/hooks/events.go`, `internal/hooks/actions.go`, `internal/eventlogger/format.go` | New kinds `EventGridModeArmed`/`EventGridModeCanceled` + payload + registry + formatters |
| Modify `internal/ui/event_helpers.go` | `emitGridModeArmed` / `emitGridModeCanceled` |
| Modify `internal/ui/game_input_gesture_state.go` | Remove loose mode fields; add `modes gridModeManager` + mouse-hold fields |
| Modify `internal/ui/game_longpress_popup.go` | 4th Copy button, popup open/close events, UITap emission, dismiss reasons |
| Modify `internal/ui/game_input_touch.go`, `internal/ui/game_input_editor.go` | Route armed-mode taps through `handleArmedModeTap`; remove confirm-dialog branches; mouse-hold detector |
| Modify `internal/ui/game_connect_mode.go` | Connect mode state via manager |
| Modify `internal/ui/game_graph_nodes.go` | `moveNode` stitches destination edge; delete `cancelMoveMode`, `moveNodeEdgeLoss` |
| Modify `internal/ui/grid_pane_draw.go`, `grid_pane_layers.go`, `grid_tree.go` | Blink + ghost validity + copy ghost; delete `drawGridMoveConfirm` + `GZMoveConfirm` |
| Modify `internal/ui/game_node_sidebar.go`, `game_input_shortcuts.go`, `internal/ui/import.go`, `js_exports_graph_ui.go`, `js_exports_eq_widgets.go` | Manager call sites, generic ESC cancel, new JS exports |
| Modify `internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go` | `KeyCapCopy` |
| Modify `src/js/touch_gestures.browser.test.js`, `src/js/wasm_bridge_smoke.browser.test.js` | Real-touch flow tests + export catalogue entries |

---

### Task 1: hooks kinds `mode.armed` / `mode.canceled`

**Files:**
- Modify: `src/go/internal/hooks/events.go` (Kind consts near `EventEdgeDeleted` ~line 45; payload structs near `PopupPayload` ~line 592; `KindAll` ~line 195; `NumNonVerbose` ~line 250)
- Modify: `src/go/internal/hooks/actions.go` (registry rows after `EventEdgeDeleted` ~line 76)
- Modify: `src/go/internal/eventlogger/format.go` (formatters after `EventEdgeDeleted` ~line 120)

**Interfaces:**
- Produces: `hooks.EventGridModeArmed Kind = "mode.armed"`, `hooks.EventGridModeCanceled Kind = "mode.canceled"`, `hooks.GridModePayload{Mode string; ID, I, J int; Reason string}`. Later tasks emit these.

- [ ] **Step 1: Add the kinds and payload (this makes the existing guard tests the failing tests)**

In `events.go`, after the `EventEdgeDeleted` const:

```go
	// Armed grid interaction modes (quick-action popup: move/copy/connect).
	// Interaction-scoped: arming/canceling mutates nothing; the committed
	// operation emits its own domain event (node.moved / node.pasted / edge.added).
	EventGridModeArmed    Kind = "mode.armed"
	EventGridModeCanceled Kind = "mode.canceled"
```

After the `PopupPayload` struct:

```go
// GridModePayload describes an armed grid-mode transition (move/copy/connect)
// on the subject node.
type GridModePayload struct {
	Mode   string `json:"mode"`
	ID     int    `json:"id"`
	I      int    `json:"i"`
	J      int    `json:"j"`
	Reason string `json:"reason,omitempty"`
}
```

In `KindAll`, extend the graph-edit line:

```go
	EventEdgeAdded, EventEdgeDeleted,
	EventGridModeArmed, EventGridModeCanceled,
```

- [ ] **Step 2: Run guard tests to verify they fail**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`
Expected: FAIL — `TestEveryKindClassified` (no registry entry), `TestHooks_NumNonVerboseUpdated` (count drift), eventlogger coverage test (missing formatter).

- [ ] **Step 3: Add registry rows, formatter, NumNonVerbose bump**

`actions.go`, after the `EventEdgeDeleted` row:

```go
	{EventGridModeArmed, "arm grid mode", "node", ScopeInteraction, ""},
	{EventGridModeCanceled, "cancel grid mode", "node", ScopeInteraction, ""},
```

`events.go` — append to the `NumNonVerbose` doc comment and bump:

```go
// + mode.armed + mode.canceled (grid quick-action modes) = 74.
const NumNonVerbose = 74
```

`eventlogger/format.go`, after the `EventEdgeDeleted` formatter:

```go
	hooks.EventGridModeArmed: func(p any) string {
		m, _ := p.(hooks.GridModePayload)
		return fmt.Sprintf("%s mode armed on node id=%d at (%d,%d)", m.Mode, m.ID, m.I, m.J)
	},
	hooks.EventGridModeCanceled: func(p any) string {
		m, _ := p.(hooks.GridModePayload)
		if m.Reason != "" {
			return fmt.Sprintf("%s mode canceled (%s)", m.Mode, m.Reason)
		}
		return fmt.Sprintf("%s mode canceled", m.Mode)
	},
```

- [ ] **Step 4: Run guard tests to verify they pass**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — report task done. Do NOT commit.

---

### Task 2: `gridModeManager` core

**Files:**
- Create: `src/go/internal/ui/grid_mode.go`
- Modify: `src/go/internal/ui/event_helpers.go` (append emitters)
- Test: `src/go/internal/ui/grid_mode_test.go`, `src/go/internal/ui/grid_mode_discipline_test.go`

**Interfaces:**
- Consumes: `hooks.EventGridModeArmed/Canceled`, `hooks.GridModePayload` (Task 1).
- Produces (used by Tasks 3–9):
  - `type GridModeKind int` with `GridModeNone/GridModeMove/GridModeCopy/GridModeConnect`, `func (k GridModeKind) String() string` → `"none"/"move"/"copy"/"connect"`.
  - `type GridModeObserver interface { OnGridModeChanged(prev, next GridModeKind, subject *uiNode) }`
  - `gridModeManager` methods: `Register(o GridModeObserver)`, `Enter(kind GridModeKind, subject *uiNode)`, `Cancel(reason string)`, `Commit()`, `Active() GridModeKind`, `Is(k GridModeKind) bool`, `Subject() *uiNode`, `Tick()`, `blinkOn() bool`.
  - Const `gridModeBlinkHalfPeriod = 15` (frames; ~250 ms at 60 TPS).

- [ ] **Step 1: Write the failing unit tests** (`grid_mode_test.go`)

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

type recObserver struct{ calls []string }

func (r *recObserver) OnGridModeChanged(prev, next GridModeKind, subject *uiNode) {
	r.calls = append(r.calls, prev.String()+"->"+next.String())
}

func TestGridModeManager_TransitionMatrix(t *testing.T) {
	n := &uiNode{ID: 1, I: 2, J: 3}
	var m gridModeManager
	obs := &recObserver{}
	m.Register(obs)

	// None → Move
	m.Enter(GridModeMove, n)
	if !m.Is(GridModeMove) || m.Subject() != n {
		t.Fatalf("expected move armed with subject, got %v %v", m.Active(), m.Subject())
	}
	// Enter with nil subject or None kind is a no-op.
	m.Enter(GridModeCopy, nil)
	m.Enter(GridModeNone, n)
	if !m.Is(GridModeMove) {
		t.Fatalf("no-op Enters must not change state, got %v", m.Active())
	}
	// Move armed while Connect requested: implicit cancel ("switched") then
	// re-arm — observers see move->none followed by none->connect.
	m.Enter(GridModeConnect, n)
	if !m.Is(GridModeConnect) {
		t.Fatalf("expected connect after switch, got %v", m.Active())
	}
	// Cancel clears.
	m.Cancel("esc")
	if m.Active() != GridModeNone || m.Subject() != nil {
		t.Fatalf("cancel must clear state")
	}
	// Cancel when None is a no-op (no observer call, no panic).
	before := len(obs.calls)
	m.Cancel("esc")
	if len(obs.calls) != before {
		t.Fatalf("cancel-when-none must not notify")
	}
	// Commit clears.
	m.Enter(GridModeCopy, n)
	m.Commit()
	if m.Active() != GridModeNone {
		t.Fatalf("commit must clear state")
	}
	// Commit when None is a no-op.
	m.Commit()

	want := []string{"none->move", "move->none", "none->connect", "connect->none", "none->copy", "copy->none"}
	if len(obs.calls) != len(want) {
		t.Fatalf("observer calls = %v, want %v", obs.calls, want)
	}
	for i := range want {
		if obs.calls[i] != want[i] {
			t.Fatalf("observer call %d = %q, want %q", i, obs.calls[i], want[i])
		}
	}
}

func TestGridModeManager_EmitsArmedAndCanceled(t *testing.T) {
	armed := make(chan hooks.GridModePayload, 4)
	canceled := make(chan hooks.GridModePayload, 4)
	u1 := hooks.GlobalBus().Subscribe(hooks.EventGridModeArmed, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			armed <- p
		}
	})
	defer u1()
	u2 := hooks.GlobalBus().Subscribe(hooks.EventGridModeCanceled, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			canceled <- p
		}
	})
	defer u2()

	n := &uiNode{ID: 7, I: 4, J: 5}
	var m gridModeManager
	m.Enter(GridModeMove, n)
	select {
	case p := <-armed:
		if p.Mode != "move" || p.ID != 7 || p.I != 4 || p.J != 5 {
			t.Fatalf("armed payload = %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventGridModeArmed")
	}
	m.Cancel("invalid-target")
	select {
	case p := <-canceled:
		if p.Mode != "move" || p.Reason != "invalid-target" {
			t.Fatalf("canceled payload = %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventGridModeCanceled")
	}
	// Commit emits neither armed nor canceled.
	m.Enter(GridModeCopy, n)
	<-armed
	m.Commit()
	select {
	case p := <-canceled:
		t.Fatalf("commit must not emit canceled: %+v", p)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestGridModeManager_BlinkPhase(t *testing.T) {
	n := &uiNode{ID: 1}
	var m gridModeManager
	m.Enter(GridModeMove, n)
	if !m.blinkOn() {
		t.Fatal("blink must start ON at phase 0")
	}
	for i := 0; i < gridModeBlinkHalfPeriod; i++ {
		m.Tick()
	}
	if m.blinkOn() {
		t.Fatal("blink must be OFF after one half period")
	}
	for i := 0; i < gridModeBlinkHalfPeriod; i++ {
		m.Tick()
	}
	if !m.blinkOn() {
		t.Fatal("blink must be ON again after a full period")
	}
	// Re-arming resets the phase.
	m.Cancel("test")
	m.Enter(GridModeMove, n)
	if !m.blinkOn() {
		t.Fatal("phase must reset on Enter")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGridModeManager'`
Expected: FAIL (compile error: `gridModeManager` undefined).

- [ ] **Step 3: Implement `grid_mode.go` + emitters**

`grid_mode.go`:

```go
package ui

// GridModeKind identifies an armed grid interaction mode entered from the node
// quick-action popup (or the sidebar's Move button). While a mode is armed the
// user can pan/zoom freely; the next grid tap places (move/copy), connects
// (connect), or cancels the mode.
type GridModeKind int

const (
	GridModeNone GridModeKind = iota
	GridModeMove
	GridModeCopy
	GridModeConnect
)

func (k GridModeKind) String() string {
	switch k {
	case GridModeMove:
		return "move"
	case GridModeCopy:
		return "copy"
	case GridModeConnect:
		return "connect"
	}
	return "none"
}

// GridModeObserver is the opt-in API for components that must react to mode
// transitions (e.g. the node sidebar closes when a mode arms). Observers run
// synchronously on the UI Update goroutine — they must not re-acquire seqMu.
type GridModeObserver interface {
	OnGridModeChanged(prev, next GridModeKind, subject *uiNode)
}

// gridModeBlinkHalfPeriod is the blink half-period in Update ticks (~250 ms at
// 60 TPS): the armed subject node's body is highlighted for one half period,
// plain for the next.
const gridModeBlinkHalfPeriod = 15

// gridModeManager is the single owner of armed grid-mode state. Enter, Cancel
// and Commit are the ONLY mutation paths (grid_mode_discipline_test.go guards
// this structurally): they emit the armed/canceled hooks events centrally and
// notify observers synchronously, so call sites cannot forget either. Armed
// modes mutate nothing until the committed operation runs, so Cancel is always
// a safe rollback: clear state, notify, done.
type gridModeManager struct {
	kind      GridModeKind
	subject   *uiNode
	phase     int // blink phase, advanced by Tick() once per Update while armed
	observers []GridModeObserver
}

// Register adds an observer. Registration is permanent (startup wiring only).
func (m *gridModeManager) Register(o GridModeObserver) {
	m.observers = append(m.observers, o)
}

// Enter arms kind on subject. Entering with kind==GridModeNone or a nil
// subject is a no-op (use Cancel to disarm). Entering while another mode is
// armed cancels it first (reason "switched") and then arms the new mode.
func (m *gridModeManager) Enter(kind GridModeKind, subject *uiNode) {
	if kind == GridModeNone || subject == nil {
		return
	}
	if m.kind != GridModeNone {
		m.Cancel("switched")
	}
	m.kind = kind
	m.subject = subject
	m.phase = 0
	emitGridModeArmed(kind.String(), subject.ID, subject.I, subject.J)
	m.notify(GridModeNone, kind, subject)
}

// Cancel rolls back the armed mode without touching the document. No-op when
// nothing is armed.
func (m *gridModeManager) Cancel(reason string) {
	if m.kind == GridModeNone {
		return
	}
	prev, subject := m.kind, m.subject
	m.kind = GridModeNone
	m.subject = nil
	m.phase = 0
	emitGridModeCanceled(prev.String(), subject.ID, subject.I, subject.J, reason)
	m.notify(prev, GridModeNone, subject)
}

// Commit clears the armed mode after its operation ran. It emits no event of
// its own — the committed operation (moveNode / pasteNodeFrom / addEdge)
// publishes the domain event. No-op when nothing is armed.
func (m *gridModeManager) Commit() {
	if m.kind == GridModeNone {
		return
	}
	prev, subject := m.kind, m.subject
	m.kind = GridModeNone
	m.subject = nil
	m.phase = 0
	m.notify(prev, GridModeNone, subject)
}

func (m *gridModeManager) notify(prev, next GridModeKind, subject *uiNode) {
	for _, o := range m.observers {
		o.OnGridModeChanged(prev, next, subject)
	}
}

func (m *gridModeManager) Active() GridModeKind    { return m.kind }
func (m *gridModeManager) Is(k GridModeKind) bool  { return m.kind == k }
func (m *gridModeManager) Subject() *uiNode        { return m.subject }

// Tick advances the blink phase; called once per Game.Update while armed.
func (m *gridModeManager) Tick() {
	if m.kind != GridModeNone {
		m.phase++
	}
}

// blinkOn reports whether the armed subject's blink highlight is in its ON
// half period this tick.
func (m *gridModeManager) blinkOn() bool {
	return (m.phase/gridModeBlinkHalfPeriod)%2 == 0
}
```

Append to `event_helpers.go`:

```go
// emitGridModeArmed / emitGridModeCanceled publish the armed grid-mode
// lifecycle. Called ONLY from gridModeManager.Enter/Cancel — skip=2 attributes
// the event to the manager's caller (the user-action site: popup activation,
// sidebar button, tap handler).
func emitGridModeArmed(mode string, id model.NodeID, i, j int) {
	hooks.PublishWithSource(hooks.EventGridModeArmed, hooks.GridModePayload{
		Mode: mode, ID: int(id), I: i, J: j,
	}, hooks.CaptureSource(2))
}

func emitGridModeCanceled(mode string, id model.NodeID, i, j int, reason string) {
	hooks.PublishWithSource(hooks.EventGridModeCanceled, hooks.GridModePayload{
		Mode: mode, ID: int(id), I: i, J: j, Reason: reason,
	}, hooks.CaptureSource(2))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGridModeManager'`
Expected: PASS (3 tests).

- [ ] **Step 5: Write the discipline test** (`grid_mode_discipline_test.go`)

```go
package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// gridModeManager fields may only be written inside grid_mode.go — every
// other file must go through Enter/Cancel/Commit/Tick/Register. This keeps
// the mode domain centralized (see the design spec § Centralized mode domain).
func TestGridModeManagerFieldWritesOnlyInOwnFile(t *testing.T) {
	writeRe := regexp.MustCompile(`\.modes\.(kind|subject|phase|observers)\s*(=|\+\+|--|\+=|-=)`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "grid_mode.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for ln, line := range strings.Split(string(data), "\n") {
			if writeRe.MatchString(line) {
				t.Errorf("%s:%d writes gridModeManager state directly: %s", f, ln+1, strings.TrimSpace(line))
			}
		}
	}
}
```

- [ ] **Step 6: Run it**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGridModeManagerFieldWrites'`
Expected: PASS.

- [ ] **Step 7: Checkpoint** — full `./internal/ui/` package still compiles: run `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`. Report. Do NOT commit.

---

### Task 3: Migrate move + connect state into the manager (pure refactor)

Behavior must NOT change in this task — every existing test stays green. This is the mechanical field migration; placement-rule changes come in Task 4.

**Files:**
- Modify: `src/go/internal/ui/game_input_gesture_state.go` (fields)
- Modify: `src/go/internal/ui/game_graph_nodes.go` (`cancelMoveMode` deleted)
- Modify: `src/go/internal/ui/game_connect_mode.go`
- Modify: `src/go/internal/ui/game_longpress_popup.go` (`updateLongPressPopup` switch)
- Modify: `src/go/internal/ui/game_node_sidebar.go:862` (Move button)
- Modify: `src/go/internal/ui/game_input_touch.go` (branch conditions)
- Modify: `src/go/internal/ui/game_input_editor.go` (move-mode block conditions)
- Modify: `src/go/internal/ui/game_input_shortcuts.go:433-443` (ESC)
- Modify: `src/go/internal/ui/import.go:128`
- Modify: `src/go/internal/ui/grid_pane_layers.go:41` + `grid_pane_draw.go` (`drawGridMoveMode` reads)
- Modify: `src/go/internal/ui/grid_guide.go:228` (`updateGuideAim` gate)
- Modify: `src/go/internal/ui/js_exports_graph_ui.go:2237` (state export reads)
- Modify: `src/go/internal/ui/game_new.go` (register sidebar observer; call `g.modes.Tick()` — see Step 4)
- Modify: `src/go/internal/ui/game_update.go` (Tick call, ~line 364 before the longPressPopup intercept)

**Interfaces:**
- Consumes: `gridModeManager` (Task 2).
- Produces: `g.modes` field on `inputGestureState` (promoted to `Game`); `sidebarModeObserver`. All later tasks read mode state via `g.modes.Is(...)` / `g.modes.Subject()`.

**Field mapping (apply everywhere; `grep -n "moveMode\|movingNode\|connectMode\|connectFromNode" internal/ui/*.go` to enumerate):**

| Old | New |
|---|---|
| `g.moveMode && g.movingNode != nil` (and bare `g.moveMode`) | `g.modes.Is(GridModeMove)` |
| `g.movingNode` | `g.modes.Subject()` |
| `g.moveMode = true; g.movingNode = node` | `g.modes.Enter(GridModeMove, node)` |
| `g.cancelMoveMode()` | `g.modes.Cancel(reason)` + `g.moveSkipRelease = false` where the old func reset it (see Step 3) — EXCEPT after a successful `moveNode`, which becomes `g.modes.Commit()` |
| `g.connectMode && g.connectFromNode != nil` / `g.connectMode` | `g.modes.Is(GridModeConnect)` |
| `g.connectFromNode` | `g.modes.Subject()` |
| `g.moveMode \|\| g.connectMode` (guideAim gate) | `g.modes.Active() != GridModeNone` |

- [ ] **Step 1: Update `inputGestureState`** — delete the `moveMode`, `movingNode`, `connectMode`, `connectFromNode` fields (KEEP `moveConfirm`/`moveConfirmI/J`/`moveEdgeLoss` until Task 4; KEEP `moveSkipRelease`), and add:

```go
	// modes is the centralized armed grid-mode state (move/copy/connect) —
	// see grid_mode.go. Mutated ONLY via Enter/Cancel/Commit/Tick.
	modes gridModeManager
```

- [ ] **Step 2: Migrate every call site per the mapping table.** Specific reason strings:
  - ESC (`game_input_shortcuts.go`): replace both the connect-mode and move-mode branches with one generic branch **placed where the connect-mode branch is today** (higher-priority position):

```go
	// 5. Any armed grid mode (move/copy/connect).
	if g.modes.Active() != GridModeNone {
		g.modes.Cancel("esc")
		g.moveSkipRelease = false
		return true
	}
```

  - `import.go:128`: `g.cancelMoveMode()` → `g.modes.Cancel("import"); g.moveSkipRelease = false`
  - `game_input_touch.go` move branch: occupied-target cancel → `g.modes.Cancel("invalid-target")`; after successful `g.moveNode(...)` → `g.modes.Commit()`; confirm-dialog cancels → `g.modes.Cancel("dialog-cancel")` (branch dies in Task 4 anyway).
  - `game_input_editor.go` move block: same substitutions.
  - `game_connect_mode.go`:

```go
func (g *Game) enterConnectMode(node *uiNode) {
	g.modes.Enter(GridModeConnect, node)
	// Visually select the source node.
	if g.sel != nil {
		g.sel.Selected = false
	}
	g.sel = node
	node.Selected = true
	g.computeSelNeighbors()
}
```

  In `handleConnectModeTap`: replace `g.connectFromNode` reads with a local `from := g.modes.Subject()` at the top; empty-grid tap → `g.modes.Cancel("invalid-target")`; successful `g.addEdge(from, target)` → `g.modes.Commit()`. Delete `cancelConnectMode` and update its remaining callers (grep) to `g.modes.Cancel(...)`. In `drawConnectMode` the guard becomes `if !g.modes.Is(GridModeConnect) { return }` and the source node is `g.modes.Subject()`.
  - `game_longpress_popup.go` switch case "move": `g.modes.Enter(GridModeMove, node)` (keep `g.moveSkipRelease = true`); the `showLongPressPopup` guard `if g.moveMode || g.connectMode || g.longPressPopup` → `if g.modes.Active() != GridModeNone || g.longPressPopup`.
  - `game_node_sidebar.go` Move button:

```go
	mk("move", i18n.T(i18n.KeyCapMoveNode), func() {
		g.modes.Enter(GridModeMove, node)
		sb.Close()
		g.moveSkipRelease = true
	})
```

  - `grid_pane_layers.go`: move-mode layer `vis: func() bool { return g.modes.Is(GridModeMove) }`.
  - `js_exports_graph_ui.go:2237`: `obj.Set("connectMode", g.modes.Is(GridModeConnect))`, `obj.Set("moveMode", g.modes.Is(GridModeMove))`.
  - Delete `cancelMoveMode` from `game_graph_nodes.go` (its `moveConfirm*` resets move into the touch/editor cancel sites until Task 4 removes them; the `moveSkipRelease=false` reset moves to the explicit sites listed above).

- [ ] **Step 3: Register the sidebar observer + Tick.** In `grid_mode.go` add:

```go
// sidebarModeObserver closes the node sidebar whenever a grid mode arms — the
// armed-mode overlay owns the grid until placement. Registered once at Game
// construction; the first proof of the opt-in observer API.
type sidebarModeObserver struct{ g *Game }

func (o sidebarModeObserver) OnGridModeChanged(prev, next GridModeKind, _ *uiNode) {
	if next != GridModeNone && o.g.sidebar != nil && o.g.sidebar.IsOpen() {
		o.g.sidebar.Close()
	}
}
```

In `game_new.go`, immediately after `g.sidebar` is constructed (grep `g.sidebar = `), add: `g.modes.Register(sidebarModeObserver{g: g})`.
In `game_update.go`, immediately BEFORE the `if g.longPressPopup {` intercept (~line 364), add: `g.modes.Tick()`.

- [ ] **Step 4: Compile + run the full ui fast suite**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: Tests referencing the deleted fields fail to compile. Update test files mechanically per the same mapping table (grep in `*_test.go`; typical: `g.moveMode = true; g.movingNode = n` → `g.modes.Enter(GridModeMove, n)`; assertions `g.moveMode == false` → `!g.modes.Is(GridModeMove)`). Behavior assertions must not change. Re-run until PASS.

- [ ] **Step 5: Run the neighboring suites**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/hooks/ ./internal/eventlogger/ ./internal/timeline/`
Expected: PASS. (Note: `TestGridModeManagerFieldWrites` from Task 2 now guards all the migrated files.)

- [ ] **Step 6: Checkpoint** — report. Do NOT commit.

---

### Task 4: Edge-only placement, immediate commit, stitch, retire confirm dialog

**Files:**
- Modify: `src/go/internal/ui/game_input_touch.go` (move branch → `handleArmedModeTap`)
- Modify: `src/go/internal/ui/game_input_editor.go` (move block → `handleArmedModeTap`; delete confirm branch)
- Modify: `src/go/internal/ui/game_graph_nodes.go` (`moveNode` stitch; delete `moveNodeEdgeLoss`)
- Modify: `src/go/internal/ui/game_input_gesture_state.go` (delete `moveConfirm`, `moveConfirmI`, `moveConfirmJ`, `moveEdgeLoss`)
- Modify: `src/go/internal/ui/grid_pane_draw.go` (delete `drawGridMoveConfirm`)
- Modify: `src/go/internal/ui/grid_pane_layers.go` (delete `grid-move-confirm` registration), `src/go/internal/ui/grid_tree.go` (delete `GZMoveConfirm`)
- Modify: `src/go/internal/ui/move_node_test.go`, `game_input_touch_test.go`, `grid_layer_zorder_test.go` (+ any test referencing the deleted state — grep `moveConfirm|moveNodeEdgeLoss|MoveConfirm|EdgeLoss`)
- Test: `src/go/internal/ui/grid_mode_flow_test.go` (new)

**Interfaces:**
- Consumes: `g.modes`, `guideTargetAt` (`GuideTarget.Kind == GuideEdge` ⇒ interior edge cell, guaranteed unoccupied by `guideEdgeTarget`), `moveNode`, `stitchEdgesAt`, `pasteNodeFrom`, `beginPasteConfirm`.
- Produces: `func (g *Game) handleArmedModeTap(x, y int) bool` — consumed by both input paths and extended with the copy case in Task 7.

- [ ] **Step 1: Write the failing functional tests** (`grid_mode_flow_test.go`). Follow the setup conventions of `game_input_touch_test.go` / `move_node_test.go` (read their helpers first; use the same Game construction + node/edge builders they use — e.g. `newTestGame`-style helper, `g.tryAddNode`, `g.addEdge`, `g.handleTapInGrid`, and screen-coord helpers already present in those files).

Test cases (one function each, table where natural):

```go
// Placement matrix for the armed MOVE mode. Grid layout used by all cases:
//   A(2,2) ── E ── B(2,8)   (vertical edge, interior cells (2,3)..(2,7))
//   M(6,2)                  (the node being moved; no edges)
//   O(2,8) occupied endpoint; C(10,10) free empty cell.
//
// TestMoveMode_TapOnEdgePointCommitsImmediately:
//   arm via g.modes.Enter(GridModeMove, M) (entry paths tested separately),
//   tap screen coords of cell (2,5) → expect:
//   - M now at (2,5) (g.nodeAt(2,5).ID == M.ID, g.nodeAt(6,2) == nil)
//   - target edge stitched: edge A–B gone; edges A–M and M–B exist
//   - g.modes.Active() == GridModeNone (committed)
//   - NO confirm dialog state (type has no field anymore — compile guard)
//
// TestMoveMode_TapEmptyCellCancels:
//   tap screen coords of free cell (10,10) → M stays at (6,2), mode canceled,
//   graph unchanged (node count, edge count).
//
// TestMoveMode_TapOccupiedNodeCancels:
//   tap on node B → M stays, mode canceled.
//
// TestMoveMode_MoveWithOwnEdgesDropsNonOrthogonalSilently:
//   M2 at (2,12) with edge M2–D where D(2,16) (vertical). Move M2 to (2,5)
//   (on edge A–B): M2–D stays orthogonal (same I) → still present;
//   add M3(4,12) with edge M3–M2; after moving M2 to (2,5) the M3 edge is
//   non-orthogonal → dropped, and NO dialog appears (commit is immediate).
//
// TestMoveMode_SingleUndoStep:
//   perform the edge-point move, then undo once → M back at origin, A–B edge
//   restored, stitch reverted. (Use the undo entry point existing tests use —
//   grep "undoShortcut\|Undo()" in internal/ui tests.)
//
// TestMoveMode_EventSequence:
//   subscribe to EventGridModeArmed + EventNodeMoved + EventGridModeCanceled
//   (uitap_log_test.go channel pattern). Valid placement → armed then moved,
//   no canceled. Invalid placement → armed then canceled(reason
//   "invalid-target"), no moved.
```

Write these as real code against the existing helpers; the assertions above are the required outcomes.

- [ ] **Step 2: Run to verify the new tests fail**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMoveMode_'`
Expected: FAIL (current code places on empty cells and shows the confirm dialog; no stitch on move).

- [ ] **Step 3: Implement.**

(a) `game_input_touch.go` — new shared handler (place above `handleTapInGrid`); replace the whole move-mode branch (lines 40–75) and the connect branch with:

```go
	// Armed grid mode (move/copy/connect): the tap is the placement/target.
	if g.handleArmedModeTap(x, y) {
		return
	}
```

```go
// handleArmedModeTap resolves a grid tap while a grid mode is armed. Returns
// true when the tap was consumed (whether it committed or canceled the mode).
// Move placement is edge-only: the tapped cell must lie on an existing edge
// segment (GuideEdge); anything else cancels the mode and the node stays put.
func (g *Game) handleArmedModeTap(x, y int) bool {
	switch g.modes.Active() {
	case GridModeMove:
		gt := g.guideTargetAt(x, y)
		n := g.modes.Subject()
		if gt.Kind != GuideEdge {
			g.modes.Cancel("invalid-target")
			return true
		}
		if g.moveNode(n, gt.I, gt.J) {
			g.modes.Commit()
		} else {
			g.modes.Cancel("move-failed")
		}
		return true
	case GridModeConnect:
		g.handleConnectModeTap(x, y)
		return true
	}
	return false
}
```

(b) `game_input_editor.go` — replace the whole `if g.moveMode …` block (the confirm dialog, skip-release, and release-placement logic, current lines ~55–110) with:

```go
	// Armed grid mode: mouse release in the grid places/cancels via the same
	// shared handler the touch path uses.
	if g.modes.Active() != GridModeNone {
		x, y := cursorPosition()
		// Skip the mouse release that corresponds to the popup/sidebar button
		// press that armed the mode.
		if g.moveSkipRelease {
			if !left {
				g.moveSkipRelease = false
			}
			g.leftPrev = left
			return
		}
		if !left && g.leftPrev && g.split.InGridPane(x, y) && y >= gridTopOffset() {
			g.handleArmedModeTap(x, y)
		}
		g.leftPrev = left
		return
	}
```

NOTE: connect mode previously did NOT flow through this editor block — verify with `git log`/current code that mouse connect taps arrive via `handleTapInGrid` only… they arrive via the pendingClick release path. Guard against double-handling: the block above intercepts ALL armed modes before pendingClick logic, which matches the old move-mode structure; connect-on-desktop now resolves here instead of the node-click path. Run `TestHandleTapInGrid_ConnectMode*` and the connect-mode tests — they must stay green; if a desktop connect test drove `handleEditor` clicks, this block now handles it identically (tap → `handleConnectModeTap`).

(c) `game_graph_nodes.go` — in `moveNode`, after the re-add-orthogonal-edges loop (line ~423), insert:

```go
	// Splice into any edge passing through the destination cell — same
	// semantics as add/paste (tryAddNode/pasteNodeFrom → stitchEdgesAt):
	// edge A–B through the new cell becomes A–n + n–B, so a node dropped
	// onto an edge joins the traversal path there. Own edges (terminating
	// at n) are never split. Inside the same "move node" undo group.
	if !g.importing {
		g.stitchEdgesAt(n)
	}
```

Delete `moveNodeEdgeLoss` (lines ~358–375).

(d) Delete confirm-dialog remnants: the four `moveConfirm*`/`moveEdgeLoss` fields, `drawGridMoveConfirm`, its layer registration + `GZMoveConfirm` const, and every branch referencing them (grep). Update `grid_layer_zorder_test.go`'s expected table (remove `grid-move-confirm`). Update the `drawGridMoveMode` banner text: `"MOVING NODE (%d,%d) — TAP A POINT ON AN EDGE"`.

- [ ] **Step 4: Run the new tests + full ui suite; fix legacy tests**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: `TestMoveMode_*` PASS. Legacy failures to update (behavior intentionally changed — rewrite, don't delete coverage):
- `TestHandleTapInGrid_MoveMode_EmptyTarget` → expects cancel now.
- `TestHandleTapInGrid_MoveMode_EdgeLoss_Confirm` / `_ConfirmDialog*` → delete (dialog retired); their edge-loss coverage is `TestMoveMode_MoveWithOwnEdgesDropsNonOrthogonalSilently`.
- `move_node_test.go`: `TestMoveNodeEdgeLossPreview` → delete; `TestMoveNodeBasic/PreservesEdges/DropsNonOrthogonalEdges/OccupiedBlocked` exercise `g.moveNode` directly (programmatic semantics, still place-anywhere) → keep, but add stitch assertions where the destination lies on an edge.

- [ ] **Step 5: Checkpoint** — report, including the exact list of rewritten/deleted legacy tests. Do NOT commit.

---

### Task 5: Blink + ghost validity rendering

**Files:**
- Modify: `src/go/internal/ui/grid_pane_draw.go` (`drawGridMoveMode`)
- Test: append to `src/go/internal/ui/grid_mode_flow_test.go`

**Interfaces:**
- Consumes: `g.modes.blinkOn()`, `g.modes.Tick()` (wired in Task 3), `guideTargetCached`.
- Produces: none new.

- [ ] **Step 1: Write the failing render tests.** Use the ebitenstub pixel/structural conventions from `audio_panel_render_pixels_test.go` / existing grid draw tests (measure DELTA between two draws, not absolute pixels — see memory: `At()` panics under real Ebiten, stub-only):

```go
// TestMoveModeBlinkAlternates: arm move mode on a node, draw once at phase 0
// (blink ON) and once after gridModeBlinkHalfPeriod Ticks (blink OFF); assert
// the two draws differ inside the subject node's screen rect (count pixels of
// the glow color, ON-count > OFF-count).
//
// TestMoveModeGhostValidityStates: with the cursor (SetInputForTest) over an
// edge-interior cell, draw → ghost fill present (filled glow pixels inside the
// ghost rect above a threshold). Move cursor over a free empty cell, draw →
// outline-only (filled-glow pixel count strictly lower than the valid case;
// nonzero outline pixels).
```

Write real code against the stub helpers those tests use (`RENDER_SAFE`-style drawRect interception or direct stub-image pixel reads — follow whichever pattern the neighboring grid draw tests use).

- [ ] **Step 2: Run to verify failure**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMoveModeBlink|TestMoveModeGhost'`
Expected: FAIL (no blink; ghost always filled).

- [ ] **Step 3: Implement in `drawGridMoveMode`.** Replace the outline+ghost section:

```go
		// Pulsing outline + blinking body on the subject node: the blink
		// (Update-ticked, gridModeBlinkHalfPeriod) signals "this node is
		// about to move" while every other behavior stays live.
		n := g.modes.Subject()
		mx1, my1, mx2, my2 := g.nodeScreenRect(n)
		hlCol := WithAlpha(genColorVizGlow, 255)
		var idm ebiten.GeoM
		DrawLineCam(dst, mx1-1, my1-1, mx2+1, my1-1, &idm, hlCol, 2)
		DrawLineCam(dst, mx2+1, my1-1, mx2+1, my2+1, &idm, hlCol, 2)
		DrawLineCam(dst, mx2+1, my2+1, mx1-1, my2+1, &idm, hlCol, 2)
		DrawLineCam(dst, mx1-1, my2+1, mx1-1, my1-1, &idm, hlCol, 2)
		if g.modes.blinkOn() {
			blinkRect := image.Rect(int(mx1), int(my1), int(mx2), int(my2))
			drawRect(dst, blinkRect, WithAlpha(genColorVizGlow, genAlphaMedium), true)
		}

		// Ghost indicator at cursor position (snapped to grid). Valid target
		// (interior point on an existing edge) = filled glow ghost; invalid =
		// faint outline only, signalling release-here-cancels.
		cmx, cmy := cursorPosition()
		if g.split.InGridPane(cmx, cmy) && cmy >= gridTopOffset() {
			gt := g.guideTargetCached(cmx, cmy)
			gi, gj := gt.I, gt.J
			gsx := g.cam.OffsetX + unitPx*float64(gi)
			gsy := g.cam.OffsetY + unitPx*float64(gj) + float64(gridTopOffset())
			gr := g.grid.NodeRadius(g.cam.Scale) * g.cam.Scale
			ghostRect := image.Rect(int(gsx-gr), int(gsy-gr), int(gsx+gr), int(gsy+gr))
			if gt.Kind == GuideEdge {
				drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaAccentOverlay), true)
				drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaStrong), false)
			} else {
				drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaSubtle), false)
			}
		}
```

(The surrounding banner code stays; the whole method's guard is already the layer `vis` predicate + `if g.modes.Is(GridModeMove)`.)

- [ ] **Step 4: Run tests**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMoveModeBlink|TestMoveModeGhost'`
Expected: PASS. Then full `./internal/ui/` — PASS.

- [ ] **Step 5: Checkpoint.** Do NOT commit.

---

### Task 6: Popup lifecycle events + option-tap telemetry

**Files:**
- Modify: `src/go/internal/ui/game_longpress_popup.go`
- Test: append to `src/go/internal/ui/grid_mode_flow_test.go`

**Interfaces:**
- Consumes: `emitPopupOpened(id)` / `emitPopupClosed(id, reason)` / `emitUITap(label, icon)` (all exist in `interaction_events.go`).
- Produces: popup surface id `"node-longpress"`; `dismissLongPressPopup(reason string)` (new signature) — Task 7/8 call it.

- [ ] **Step 1: Write the failing test:**

```go
// TestLongPressPopupEmitsLifecycleEvents: subscribe to EventPopupOpened,
// EventPopupClosed, EventUITap. showLongPressPopup(node,…) → popup.opened
// {ID:"node-longpress"}. Simulate slide-release on Move (drive
// updateLongPressPopup with coords inside longPressPopupMove: one frame
// left=true, next frame left=false) → EventUITap{Label:"Move"} AND
// popup.closed{Reason:"action"} AND EventGridModeArmed{Mode:"move"}.
// Separately: release outside all buttons → popup.closed{Reason:"dismiss"},
// no UITap, no mode armed.
```

- [ ] **Step 2: Run to verify failure** (`-run 'TestLongPressPopupEmits'`). Expected: FAIL.

- [ ] **Step 3: Implement.**
- `showLongPressPopup`: after the state is set (i.e. after the early-return guards, at the end of the function): `emitPopupOpened("node-longpress")`.
- Change `dismissLongPressPopup()` → `dismissLongPressPopup(reason string)`; body appends `emitPopupClosed("node-longpress", reason)` (emit BEFORE clearing state). Update every caller (grep): activation cases pass `"action"`, the default no-hover release passes `"dismiss"`, any ESC/other sites pass `"esc"`/`"cancel"` as appropriate.
- In the `updateLongPressPopup` release switch, before each mode entry/delete: `emitUITap(i18n.T(i18n.KeyCapMove), "")`, `emitUITap(i18n.T(i18n.KeyCapConnect), "")`, `emitUITap(i18n.T(i18n.KeyMenuDelete), "")` respectively.

- [ ] **Step 4: Run tests** (`-run 'TestLongPressPopupEmits'`, then full ui). Expected: PASS.

- [ ] **Step 5: Checkpoint.** Do NOT commit.

---

### Task 7: Copy option + armed copy mode

**Files:**
- Modify: `src/go/internal/i18n/keys.go` (+`KeyCapCopy Key = "cap.copy"` next to `KeyCapMove`), `catalog_en.go` (`KeyCapCopy: "Copy",`), `catalog_es.go` (`KeyCapCopy: "Copiar",`)
- Modify: `src/go/internal/ui/game_input_gesture_state.go` (add `longPressPopupCopy image.Rectangle` after `longPressPopupMove`)
- Modify: `src/go/internal/ui/game_longpress_popup.go` (layout, hover, draw, activation)
- Modify: `src/go/internal/ui/game_input_touch.go` (`handleArmedModeTap` copy case)
- Modify: `src/go/internal/ui/grid_pane_layers.go` + `grid_pane_draw.go` (ghost for copy mode)
- Modify: `src/go/internal/ui/js_exports_graph_ui.go` (state export + popup rect exports + `gridModeState`)
- Test: append to `grid_mode_flow_test.go`; update `longpress_popup*_test.go` for 4 buttons

**Interfaces:**
- Consumes: `snapshotNodeToClipboard`, `emitNodeCopied`, `pasteNodeFrom`, `beginPasteConfirm`, `g.clipboard`, `GridModeCopy`.
- Produces: JS exports `longPressMoveRect()`, `longPressCopyRect()`, `gridModeState()` (Task 9's browser test consumes them).

- [ ] **Step 1: Write the failing tests:**

```go
// TestLongPressPopupCopyArmsCopyMode: open popup, slide-release on the Copy
// rect → clipboard has the node's type/params, g.modes.Is(GridModeCopy),
// EventNodeCopied emitted, EventGridModeArmed{Mode:"copy"}.
//
// TestCopyMode_TapFreeCellPastes: arm copy on node M (params: set a
// distinctive Volume via M's model params first). Tap free cell (10,4) →
// new node exists at (10,4) with M's type+params, M unchanged, mode
// committed (None), EventNodePasted emitted. Node count +1.
//
// TestCopyMode_TapOnEdgeCellPastesAndStitches: arm copy, tap an interior
// edge cell → pasted node stitched into that edge (pasteNodeFrom semantics).
//
// TestCopyMode_TapOccupiedOpensOverwriteConfirm: arm copy, tap node B →
// g.pasteConfirmActive() true, mode committed (the confirm portal owns the
// rest; ConfirmPasteOverwrite flow already covered by node_clipboard_test.go).
//
// TestCopyMode_EscCancels: arm copy, ESC → mode None,
// EventGridModeCanceled{Mode:"copy", Reason:"esc"}.
```

- [ ] **Step 2: Run to verify failure** (`-run 'TestLongPressPopupCopy|TestCopyMode_'`). Expected: FAIL.

- [ ] **Step 3: Implement.**

(a) Popup layout in `showLongPressPopup` — four buttons, width-clamped for narrow screens:

```go
	btnW := 80
	btnH := Profile().DensityValues().PopupBtnH
	gap := SpaceSM
	pad := SpaceMD
	gridW := g.split.GridW(g.winW)
	// Four keycaps must fit narrow mobile panes: shrink from the ideal 80px
	// down to whatever the grid pane allows.
	if maxBtnW := (gridW - 2*pad - 3*gap) / 4; maxBtnW < btnW {
		btnW = maxBtnW
	}
	panelW := 4*btnW + 3*gap + 2*pad
	panelH := btnH + 2*pad
```

(the `gridW := g.split.GridW(g.winW)` already computed later for clamping — hoist it, don't compute twice). Button rects (order Move · Copy · Connect · Delete):

```go
	bx := px + pad
	by := py + pad
	g.longPressPopupMove = image.Rect(bx, by, bx+btnW, by+btnH)
	g.longPressPopupCopy = image.Rect(bx+(btnW+gap), by, bx+2*btnW+gap, by+btnH)
	g.longPressPopupConn = image.Rect(bx+2*(btnW+gap), by, bx+3*btnW+2*gap, by+btnH)
	g.longPressPopupDel = image.Rect(bx+3*(btnW+gap), by, bx+4*btnW+3*gap, by+btnH)
```

(b) `updateLongPressPopup`: add hover case `pt.In(g.longPressPopupCopy)` → `"copy"` (between move and connect); activation case:

```go
		case "copy":
			node := g.longPressPopupNode
			emitUITap(i18n.T(i18n.KeyCapCopy), "")
			g.dismissLongPressPopup("action")
			if g.snapshotNodeToClipboard(node) {
				emitNodeCopied(node.ID, node.I, node.J, g.clipboard.typ)
				g.modes.Enter(GridModeCopy, node)
			}
```

(c) `dismissLongPressPopup`: clear `g.longPressPopupCopy = image.Rectangle{}` too.

(d) `drawLongPressPopup`: add the Copy keycap between Move and Connect, identical styling to Move/Connect (neutral keycap, instrument-accent hover text), label `i18n.T(i18n.KeyCapCopy)`.

(e) `handleArmedModeTap` — insert the copy case:

```go
	case GridModeCopy:
		gt := g.guideTargetAt(x, y)
		if existing := g.nodeAt(gt.I, gt.J); existing != nil {
			// Existing paste semantics: occupied cell → overwrite confirm
			// (portal). The confirm takes over; the armed mode's job is done.
			g.beginPasteConfirm(gt.I, gt.J)
			g.modes.Commit()
			return true
		}
		g.pasteNodeFrom(gt.I, gt.J, g.clipboard)
		g.modes.Commit()
		return true
```

(f) Ghost for copy mode: change the move-mode layer's `vis` to `func() bool { return g.modes.Is(GridModeMove) || g.modes.Is(GridModeCopy) }` (keep layer id `grid-move-mode`; update `grid_layer_zorder_test.go` only if it pins vis semantics). In `drawGridMoveMode`, restructure: banner + blink + outline render only for `GridModeMove` (guard `if g.modes.Is(GridModeMove)`); for `GridModeCopy` draw a banner `"PASTING NODE — TAP TO PLACE"` and skip the subject outline/blink; the ghost block runs for both modes, with copy validity = `g.nodeAt(gt.I, gt.J) == nil` (free cell → filled ghost; occupied → faint outline):

```go
		valid := false
		switch g.modes.Active() {
		case GridModeMove:
			valid = gt.Kind == GuideEdge
		case GridModeCopy:
			valid = g.nodeAt(gt.I, gt.J) == nil
		}
```

(g) JS exports (`js_exports_graph_ui.go`): in the input-state object (~line 2237) add `obj.Set("copyMode", g.modes.Is(GridModeCopy))`; next to `longPressDeleteRect` add:

```go
	// longPressMoveRect() / longPressCopyRect() -> {x,y,w,h} or null
	js.Global().Set("longPressMoveRect", jsFn(func(args jsArgs) any {
		if !g.longPressPopup {
			return nil
		}
		return rectToJS(g.longPressPopupMove)
	}))
	js.Global().Set("longPressCopyRect", jsFn(func(args jsArgs) any {
		if !g.longPressPopup {
			return nil
		}
		return rectToJS(g.longPressPopupCopy)
	}))

	// gridModeState() -> {mode:"none|move|copy|connect", subjectId:int}
	js.Global().Set("gridModeState", jsFn(func(args jsArgs) any {
		obj := js.Global().Get("Object").New()
		obj.Set("mode", g.modes.Active().String())
		if n := g.modes.Subject(); n != nil {
			obj.Set("subjectId", int(n.ID))
		} else {
			obj.Set("subjectId", -1)
		}
		return obj
	}))
```

Add all three export names wherever `internal/ui/js_exports_catalogue_drift_test.go` requires (run it; it will name the catalogue location).

- [ ] **Step 4: Run tests**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/i18n/`
Expected: PASS, including `TestCatalogCompleteness` (i18n) and updated `longpress_popup*_test.go` (4 buttons — update rect-count/layout assertions).

- [ ] **Step 5: Checkpoint.** Do NOT commit.

---

### Task 8: Desktop mouse-hold opens the popup

**Files:**
- Modify: `src/go/internal/ui/game_input_gesture_state.go` (hold fields)
- Modify: `src/go/internal/ui/gesture.go` (const) — add `mouseHoldPopupFrames`
- Modify: `src/go/internal/ui/game_input_editor.go` (detector)
- Test: append to `grid_mode_flow_test.go`

**Interfaces:**
- Consumes: `showLongPressPopup`, `touchOverrideActive` (package var, `touch.go`), `tapMaxMovePx`, `longPressThresholdMS` (`gesture.go`).
- Produces: nothing new for later tasks.

- [ ] **Step 1: Write the failing tests:**

```go
// All drive handleEditor via SetInputForTest-style seams used by existing
// editor tests (grep "SetInputForTest" in internal/ui for the canonical
// pattern), with touchOverrideActive false (desktop).
//
// TestMouseHoldOnNodeOpensPopup: press-and-hold left on a node's screen
// coords; run g.Update()/handleEditor for mouseHoldPopupFrames+1 frames
// without moving → g.longPressPopup true, popup node == pressed node, and
// pendingClick consumed (a subsequent release must NOT open the sidebar:
// g.sidebar.IsOpen() == false after release frame).
//
// TestMouseHoldCanceledByDrag: press on node, move cursor > tapMaxMovePx
// after 10 frames, keep holding past the threshold → popup NEVER opens.
//
// TestMouseShortClickStillOpensSidebar: press+release on node within a few
// frames → sidebar opens (existing behavior intact).
//
// TestMouseHoldThenSlideReleaseArmsMove: after popup opens via hold, move
// cursor into longPressPopupMove rect (left still true → updateLongPressPopup
// tracks hover), then release → g.modes.Is(GridModeMove).
//
// TestMouseHoldOnEmptySpaceDoesNothing: hold on empty grid → no popup.
```

- [ ] **Step 2: Run to verify failure** (`-run 'TestMouseHold|TestMouseShortClick'`). Expected: FAIL.

- [ ] **Step 3: Implement.**

`gesture.go` — add below the existing constants:

```go
// mouseHoldPopupFrames is the desktop mouse equivalent of the touch
// long-press threshold, in Update ticks (60 TPS): click-and-hold a node this
// long without moving opens the quick-action popup.
const mouseHoldPopupFrames = longPressThresholdMS * 60 / 1000
```

`game_input_gesture_state.go` — add to `inputGestureState`:

```go
	// Desktop mouse-hold: press-and-hold on a node (no drag) for
	// mouseHoldPopupFrames opens the long-press quick-action popup. -1 =
	// hold dead for this press (moved, or already fired).
	holdFrames int
	holdStartX int
	holdStartY int
```

`game_input_editor.go` — in the press-edge block (`if left && !g.leftPrev {`), after `g.camDragged = false`:

```go
		g.holdFrames = 0
		g.holdStartX, g.holdStartY = x, y
```

Immediately after the whole press-edge block (before the release block `if !left && g.leftPrev {`), add:

```go
	// Desktop mouse-hold on a node → quick-action popup (touch reaches the
	// same popup via GestureLongPress; gate on !touchOverrideActive so the
	// two detectors never double-fire on one touch).
	if left && g.leftPrev && g.pendingClick && g.clickNode != nil && !touchOverrideActive {
		if g.camDragged || abs(x-g.holdStartX) > tapMaxMovePx || abs(y-g.holdStartY) > tapMaxMovePx {
			g.holdFrames = -1 // moved: hold is dead for this press
		} else if g.holdFrames >= 0 {
			g.holdFrames++
			if g.holdFrames >= mouseHoldPopupFrames {
				g.holdFrames = -1
				g.pendingClick = false // the hold consumed this press: no click on release
				g.showLongPressPopup(g.clickNode, x, y)
				g.leftPrev = left
				return
			}
		}
	}
```

(Once the popup is open, `game_update.go:366`'s intercept routes frames to `updateLongPressPopup`, so slide+release works with the mouse for free.)

- [ ] **Step 4: Run tests** (`-run 'TestMouseHold|TestMouseShortClick'`, then full `./internal/ui/`). Expected: PASS.

- [ ] **Step 5: Checkpoint.** Do NOT commit.

---

### Task 9: Browser real-input tests + bridge catalogue

**Files:**
- Modify: `src/js/touch_gestures.browser.test.js` (append two tests before the summary block)
- Modify: `src/js/wasm_bridge_smoke.browser.test.js` (catalogue entries for `longPressMoveRect`, `longPressCopyRect`, `gridModeState`)

**Interfaces:**
- Consumes: JS exports from Task 7; `cdpLongPressAndSlide`, `cdpTap`, `canvasToPageCoords` from `src/js/touch_cdp_helpers.js`; existing exports `addNode`, `addEdgeGrid`, `nodeRect`, `nodeIdAt`, `totalNodes`.

- [ ] **Step 1: Add catalogue entries.** Open `wasm_bridge_smoke.browser.test.js`, find the export catalogue, and add `longPressMoveRect`, `longPressCopyRect` (existence + callable, returns null when popup closed) and `gridModeState` (callable, shape `{mode, subjectId}`, `mode === "none"` at idle), following the file's existing entry format exactly.

- [ ] **Step 2: Run the smoke test to verify it passes**

Run: `cd /home/ymolinar/Repos/beatmo && GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: PASS (also proves `js_exports_catalogue_drift_test.go` alignment — run it Go-side too).

- [ ] **Step 3: Append the gesture tests** to `touch_gestures.browser.test.js` (before the pass/fail summary). First change the imports: use `cdpLongPressAndSlide` (drop the `_` alias). Then:

```js
// Test: long-press a node, slide to MOVE, release; tap an edge point → node moves
console.log("Test: long-press slide-to-Move relocates node onto edge point");
{
  // Fresh mini-circuit away from the default path: endpoints A(6,2)–B(6,6)
  // with a vertical edge (interior cells (6,3)..(6,5)); mover M at (8,3).
  await page.evaluate(() => {
    addNode(6, 2, 'regular'); addNode(6, 6, 'regular');
    addEdgeGrid(6, 2, 6, 6);
    addNode(8, 3, 'regular');
  });
  await page.waitForTimeout(150);
  const mRect = await page.evaluate(() => nodeRect(8, 3));
  const aRect = await page.evaluate(() => nodeRect(6, 2));
  const bRect = await page.evaluate(() => nodeRect(6, 6));
  assert(mRect && aRect && bRect, `test nodes visible (m=${JSON.stringify(mRect)})`);
  const moverId = await page.evaluate(() => nodeIdAt(8, 3));

  // 1) Long-press the mover to open the popup (canvas coords → page coords).
  const mC = await canvasToPageCoords(page, mRect.x + mRect.w / 2, mRect.y + mRect.h / 2);
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: mC.x, y: mC.y }],
  });
  // Poll for the popup's Move rect instead of a fixed wait (CPU contention).
  let moveRect = null;
  for (let i = 0; i < 30 && !moveRect; i++) {
    await page.waitForTimeout(100);
    moveRect = await page.evaluate(() =>
      typeof longPressMoveRect === 'function' ? longPressMoveRect() : null);
  }
  assert(moveRect, `long-press popup opened with Move rect (${JSON.stringify(moveRect)})`);

  if (moveRect) {
    // 2) Slide the same touch onto Move and release → arms move mode.
    const mv = await canvasToPageCoords(page, moveRect.x + moveRect.w / 2, moveRect.y + moveRect.h / 2);
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove', touchPoints: [{ x: mv.x, y: mv.y }],
    });
    await page.waitForTimeout(150);
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await page.waitForTimeout(300);
    const armed = await page.evaluate(() => gridModeState());
    assert(armed.mode === 'move' && armed.subjectId === moverId,
      `move mode armed on mover (${JSON.stringify(armed)})`);

    // 3) Tap the edge midpoint cell (6,4): screen midpoint of A and B centers.
    const tgt = await canvasToPageCoords(page,
      (aRect.x + aRect.w / 2 + bRect.x + bRect.w / 2) / 2,
      (aRect.y + aRect.h / 2 + bRect.y + bRect.h / 2) / 2);
    await cdpTap(page, tgt.x, tgt.y);
    await page.waitForTimeout(400);
    const moved = await page.evaluate(() => ({
      atOld: nodeIdAt(8, 3), atNew: nodeIdAt(6, 4), mode: gridModeState().mode,
    }));
    assert(moved.atOld === -1 && moved.atNew === moverId,
      `node relocated (8,3)→(6,4) (${JSON.stringify(moved)})`);
    assert(moved.mode === 'none', `move mode committed (mode=${moved.mode})`);
  }
  await page.waitForTimeout(200);
}

// Test: armed move mode + tap on empty cell cancels (node stays)
console.log("Test: move-mode tap on empty cell cancels");
{
  await page.evaluate(() => { addNode(12, 3, 'regular'); });
  await page.waitForTimeout(150);
  const nRect = await page.evaluate(() => nodeRect(12, 3));
  assert(nRect, "cancel-test node visible");
  const nC = await canvasToPageCoords(page, nRect.x + nRect.w / 2, nRect.y + nRect.h / 2);
  const cdp2 = await page.context().newCDPSession(page);
  await cdp2.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: nC.x, y: nC.y }] });
  let mr = null;
  for (let i = 0; i < 30 && !mr; i++) {
    await page.waitForTimeout(100);
    mr = await page.evaluate(() => longPressMoveRect());
  }
  if (assert(mr, "popup opened for cancel test")) {
    const mv = await canvasToPageCoords(page, mr.x + mr.w / 2, mr.y + mr.h / 2);
    await cdp2.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: mv.x, y: mv.y }] });
    await page.waitForTimeout(150);
    await cdp2.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await page.waitForTimeout(300);
    // Tap a far empty cell (no edge there): mode must cancel, node must stay.
    const empty = await canvasToPageCoords(page, nRect.x + nRect.w / 2 + 120, nRect.y + nRect.h / 2 + 80);
    await cdpTap(page, empty.x, empty.y);
    await page.waitForTimeout(400);
    const after = await page.evaluate(() => ({ still: nodeIdAt(12, 3), mode: gridModeState().mode }));
    assert(after.still !== -1, `node stayed at (12,3) after invalid drop (${JSON.stringify(after)})`);
    assert(after.mode === 'none', `mode canceled (mode=${after.mode})`);
  }
  await page.waitForTimeout(200);
}

// Test: long-press slide-to-Copy, tap free cell pastes a copy
console.log("Test: long-press slide-to-Copy pastes at tapped cell");
{
  const before = await page.evaluate(() => totalNodes());
  const srcRect = await page.evaluate(() => nodeRect(6, 2)); // endpoint A from earlier test
  assert(srcRect, "copy source visible");
  const sC = await canvasToPageCoords(page, srcRect.x + srcRect.w / 2, srcRect.y + srcRect.h / 2);
  const cdp3 = await page.context().newCDPSession(page);
  await cdp3.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: sC.x, y: sC.y }] });
  let cr = null;
  for (let i = 0; i < 30 && !cr; i++) {
    await page.waitForTimeout(100);
    cr = await page.evaluate(() =>
      typeof longPressCopyRect === 'function' ? longPressCopyRect() : null);
  }
  if (assert(cr, "popup opened with Copy rect")) {
    const cv = await canvasToPageCoords(page, cr.x + cr.w / 2, cr.y + cr.h / 2);
    await cdp3.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: cv.x, y: cv.y }] });
    await page.waitForTimeout(150);
    await cdp3.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await page.waitForTimeout(300);
    const armed = await page.evaluate(() => gridModeState());
    assert(armed.mode === 'copy', `copy mode armed (${JSON.stringify(armed)})`);
    // Paste on a free cell left of the source.
    const paste = await canvasToPageCoords(page, srcRect.x + srcRect.w / 2 - 120, srcRect.y + srcRect.h / 2);
    await cdpTap(page, paste.x, paste.y);
    await page.waitForTimeout(400);
    const afterCount = await page.evaluate(() => totalNodes());
    assert(afterCount === before + 1, `paste added one node (${before} → ${afterCount})`);
    assert((await page.evaluate(() => gridModeState())).mode === 'none', 'copy mode committed');
  }
}
```

NOTE for the implementer: if the fresh nodes land outside the visible grid pane on the iPhone-12 viewport (a `nodeRect` assert fails), lower the grid coordinates (e.g. shift all `i` by −2) rather than adding camera manipulation.

- [ ] **Step 4: Run the browser tests**

Run: `cd /home/ymolinar/Repos/beatmo && GO=$(pwd)/.tools/go/bin/go node src/js/touch_gestures.browser.test.js`
Expected: all existing + new tests PASS.

- [ ] **Step 5: Checkpoint.** Do NOT commit.

---

### Task 10: Full verification sweep

- [ ] **Step 1: Full fast Go suite**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
Expected: PASS (pre-existing known-red tests per memory — node-logic volume tests at HEAD — may stay red; anything else red must be triaged as caused-by-this-change, per the no-preexisting-baselines rule: prove causation with `git stash` if in doubt).

- [ ] **Step 2: Design/token/log guards**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestNoUppercaseInfoTags|TestEveryActionHasLowercaseTag|TestEveryKindClassified|TestHooks_NumNonVerbose' ./internal/ui/ ./internal/hooks/ ./internal/log/`
Expected: PASS.

- [ ] **Step 3: Bridge + parity browser smoke**

Run:
```bash
cd /home/ymolinar/Repos/beatmo
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/touch_gestures.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/e2e_node_click.browser.test.js
```
Expected: PASS (e2e_node_click guards the tap-opens-menu decoupling).

- [ ] **Step 4: Real-Ebiten spot check (optional but recommended)**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/ui/ -run 'TestMoveMode_|TestGridModeManager|TestMouseHold'`
Expected: PASS (blink/ghost pixel tests are stub-only and must skip themselves under real Ebiten, following the neighboring tests' build-tag/skip pattern).

- [ ] **Step 5: Report** a summary of everything run + results to the user. Do NOT commit — the user commits.
