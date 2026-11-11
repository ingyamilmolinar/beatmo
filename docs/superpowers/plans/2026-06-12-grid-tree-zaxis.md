# GridTree z-axis (render + input) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the main grid pane a z-ordered render + input tree (`GridTree`) reusing the existing `Zone`/`Layer`/`HitArea`/`HitIndex` primitives, fixing the coordinate-badge-over-popup bug structurally and eliminating the draw-order/click-through bug class.

**Architecture:** A new slim `GridTree` (sibling to `DrumViewTree`, no portal, no keyboard auto-focus) owns the top grid pane. Every former `drawGridPane` block becomes a thin `Layer`/`Zone` adapter holding `*Game` and delegating to a small relocated `(*Game).drawXxx` method. Draw walks ascending z (badge at z=40 below popups at z≥50 → bug impossible). Input is "wrap + arbitrate": grid pointer input runs through `gridTree` first; overlay zones (sidebar/popup/dialog) and a node-canvas catch-all claim presses by z-priority, and legacy gesture code (`handleTapInGrid`, `cam.HandleMouse`, modes) runs only when the tree did not claim the press.

**Tech Stack:** Go 1.23, Ebiten v2, package `internal/ui`. Fast test path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`.

---

## Conventions used throughout

- **Bundled Go:** all `go`/`gofmt` calls use `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`.
- **Run a single test:** from `src/go`:
  `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestName -v`
- **Adapter pattern:** Layers/Zones are thin structs holding `*Game`. Their `Draw`/`HitAreas` delegate to relocated `(*Game)` methods. This keeps each migration a *verbatim cut* of code out of `drawGridPane` into a focused method — low risk, easy to diff.
- **Grid-local z constants** (declared in Task 1), distinct from the drum `Z*` constants:

  | const | value | participant |
  |---|---|---|
  | `GZBackground` | 0 | grid gradient + tile cache |
  | `GZEdges` | 10 | edges + edge cache |
  | `GZCanvas` | 20 | nodes draw + node hit-testing/gestures |
  | `GZPulses` | 30 | edge pulses |
  | `GZCoordBadge` | 40 | `(i,j)` badge |
  | `GZMoveMode` | 42 | move banner + ghost |
  | `GZConnectMode` | 44 | connect-mode visuals |
  | `GZLongPress` | 50 | long-press popup |
  | `GZMoveConfirm` | 52 | move-confirm dialog |
  | `GZSidebar` | 60 | node sidebar |
  | `GZCursorLabel` | 70 | desktop cursor label |

> **Working tree note:** This branch (`add-core-node-types`) already has extensive uncommitted changes. Do NOT `git add -A`. Each task's commit lists exact paths.

---

## Phase 1 — GridTree orchestrator (new, fully tested in isolation)

### Task 1: GridTree core + z constants

**Files:**
- Create: `src/go/internal/ui/grid_tree.go`
- Test: `src/go/internal/ui/grid_tree_test.go`

- [ ] **Step 1: Write the failing test** (`grid_tree_test.go`)

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// fakeGridLayer is a draw-only participant that records draw order.
type fakeGridLayer struct {
	id      string
	z       int
	vis     bool
	drawLog *[]string
}

func (f *fakeGridLayer) ID() string             { return f.id }
func (f *fakeGridLayer) ZIndex() int            { return f.z }
func (f *fakeGridLayer) Visible() bool          { return f.vis }
func (f *fakeGridLayer) Draw(*ebiten.Image)     { *f.drawLog = append(*f.drawLog, f.id) }

func TestGridTreeDrawsAscendingZ(t *testing.T) {
	tr := NewGridTree()
	var log []string
	tr.RegisterLayer(&fakeGridLayer{id: "hi", z: 60, vis: true, drawLog: &log})
	tr.RegisterLayer(&fakeGridLayer{id: "lo", z: 10, vis: true, drawLog: &log})
	tr.RegisterLayer(&fakeGridLayer{id: "mid", z: 40, vis: true, drawLog: &log})
	tr.SetBounds(image.Rect(0, 0, 100, 100))

	dst := ebiten.NewImage(100, 100)
	tr.Draw(dst)

	want := []string{"lo", "mid", "hi"}
	if len(log) != 3 || log[0] != want[0] || log[1] != want[1] || log[2] != want[2] {
		t.Fatalf("draw order: got %v want %v", log, want)
	}
}

func TestGridTreeSkipsInvisibleLayers(t *testing.T) {
	tr := NewGridTree()
	var log []string
	tr.RegisterLayer(&fakeGridLayer{id: "shown", z: 10, vis: true, drawLog: &log})
	tr.RegisterLayer(&fakeGridLayer{id: "hidden", z: 20, vis: false, drawLog: &log})
	tr.SetBounds(image.Rect(0, 0, 50, 50))
	tr.Draw(ebiten.NewImage(50, 50))
	if len(log) != 1 || log[0] != "shown" {
		t.Fatalf("expected only 'shown' drawn, got %v", log)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestGridTreeDraws|TestGridTreeSkips' -v`
Expected: FAIL — `undefined: NewGridTree`.

- [ ] **Step 3: Write minimal implementation** (`grid_tree.go`)

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Grid-local z-index conventions for the GridTree component tree. These
// are independent of the drum-pane Z* constants in drumview_tree.go —
// the two trees own disjoint screen rects. Ascending z = drawn later =
// composites on top. The coordinate badge sits BELOW every popup/sidebar
// so it can never overpaint them (the bug this tree fixes structurally).
const (
	GZBackground  = 0
	GZEdges       = 10
	GZCanvas      = 20
	GZPulses      = 30
	GZCoordBadge  = 40
	GZMoveMode    = 42
	GZConnectMode = 44
	GZLongPress   = 50
	GZMoveConfirm = 52
	GZSidebar     = 60
	GZCursorLabel = 70
)

// GridTree orchestrates the grid pane's Zone/Layer tree. It mirrors
// DrumViewTree's 4-phase loop (Layout → Update → Input → Draw) and its
// capture/suppress lifecycle, but drops the drum-pane specifics
// (OverlayPortal, transport/eq keyboard auto-focus). It reuses the shared
// primitives Zone/Layer/HitArea/HitHandler/HitIndex/zoneAsLayer.
type GridTree struct {
	zones   []*gridZoneEntry
	zoneMap map[string]*gridZoneEntry
	layers  []Layer

	hitIndex *HitIndex

	capturedHandler HitHandler
	capturedTag     string
	suppress        bool
	wasPressed      bool
	inputHandled    bool
	wheelHandled    bool

	dragActive func() bool

	bounds image.Rectangle
}

type gridZoneEntry struct {
	zone        Zone
	rect        image.Rectangle
	zIndex      int
	lastRect    image.Rectangle
	visible     func() bool
	lastVisible bool
}

// NewGridTree creates a tree with a fresh HitIndex (no portal).
func NewGridTree() *GridTree {
	return &GridTree{
		zoneMap:  make(map[string]*gridZoneEntry),
		hitIndex: &HitIndex{},
	}
}

func (t *GridTree) RegisterZone(z Zone, zIndex int) { t.RegisterZoneVisible(z, zIndex, nil) }

func (t *GridTree) RegisterZoneVisible(z Zone, zIndex int, visible func() bool) {
	e := &gridZoneEntry{zone: z, zIndex: zIndex, visible: visible, lastVisible: true}
	t.zones = append(t.zones, e)
	t.zoneMap[z.ID()] = e
	t.insertLayer(zoneAsLayer{zone: z, zIndex: zIndex, visible: visible})
}

func (t *GridTree) RegisterLayer(l Layer) { t.insertLayer(l) }

func (t *GridTree) insertLayer(l Layer) {
	z := l.ZIndex()
	idx := len(t.layers)
	for i, existing := range t.layers {
		if existing.ZIndex() > z {
			idx = i
			break
		}
	}
	t.layers = append(t.layers, nil)
	copy(t.layers[idx+1:], t.layers[idx:])
	t.layers[idx] = l
}

// LayersForTest returns a snapshot of the merged draw slice in render order.
func (t *GridTree) LayersForTest() []Layer {
	out := make([]Layer, len(t.layers))
	copy(out, t.layers)
	return out
}

func (t *GridTree) SetZoneRect(id string, r image.Rectangle) {
	if e, ok := t.zoneMap[id]; ok {
		e.rect = r
	}
}

func (t *GridTree) SetBounds(r image.Rectangle) { t.bounds = r }

func (t *GridTree) HitIndexRef() *HitIndex { return t.hitIndex }

func (t *GridTree) SetDragActive(fn func() bool) { t.dragActive = fn }

// Draw renders the merged slice in ascending z. Zones are clipped to the
// intersection of tree bounds and their rect; plain layers receive the
// unclipped screen and self-clip (mirrors DrumViewTree.Draw).
func (t *GridTree) Draw(screen *ebiten.Image) {
	for _, layer := range t.layers {
		if !layer.Visible() {
			continue
		}
		zl, isZone := layer.(zoneAsLayer)
		if !isZone {
			layer.Draw(screen)
			continue
		}
		clip := screen.Bounds()
		if !t.bounds.Empty() {
			clip = clip.Intersect(t.bounds)
		}
		if e, ok := t.zoneMap[zl.zone.ID()]; ok && !e.rect.Empty() {
			clip = clip.Intersect(e.rect)
		}
		if clip.Empty() {
			continue
		}
		if clip == screen.Bounds() {
			layer.Draw(screen)
		} else {
			layer.Draw(screen.SubImage(clip).(*ebiten.Image))
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestGridTreeDraws|TestGridTreeSkips' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/grid_tree.go src/go/internal/ui/grid_tree_test.go
git commit -m "feat(ui): GridTree orchestrator core (draw order + z constants)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: GridTree layout pass + HitArea publication + visibility gate

**Files:**
- Modify: `src/go/internal/ui/grid_tree.go`
- Test: `src/go/internal/ui/grid_tree_test.go`

- [ ] **Step 1: Write the failing test** (append to `grid_tree_test.go`)

```go
// fakeGridZone implements Zone with controllable hit areas + visibility.
type fakeGridZone struct {
	id        string
	areas     []HitArea
	needs     bool
	layoutHit int
}

func (z *fakeGridZone) ID() string                          { return z.id }
func (z *fakeGridZone) Layout(image.Rectangle)              { z.layoutHit++; z.needs = false }
func (z *fakeGridZone) Update()                             {}
func (z *fakeGridZone) HitAreas() []HitArea                 { return z.areas }
func (z *fakeGridZone) Draw(*ebiten.Image)                  {}
func (z *fakeGridZone) NeedsLayout() bool                   { return z.needs }
func (z *fakeGridZone) Invalidate()                         { z.needs = true }
func (z *fakeGridZone) HandleKey(ebiten.Key) InputResult    { return InputIgnored }
func (z *fakeGridZone) HandleChars([]rune) InputResult      { return InputIgnored }

func TestGridTreePublishesHitAreasOnLayout(t *testing.T) {
	tr := NewGridTree()
	z := &fakeGridZone{id: "z1", needs: true, areas: []HitArea{
		{Rect: image.Rect(10, 10, 30, 30), ZIndex: GZCanvas, Tag: "a"},
	}}
	tr.RegisterZone(z, GZCanvas)
	tr.SetZoneRect("z1", image.Rect(0, 0, 100, 100))
	tr.layoutPass()

	hits := tr.hitIndex.At(20, 20)
	if len(hits) != 1 || hits[0].Tag != "a" {
		t.Fatalf("expected hit 'a' at (20,20), got %+v", hits)
	}
}

func TestGridTreeInvisibleZoneClearsHitAreas(t *testing.T) {
	tr := NewGridTree()
	vis := true
	z := &fakeGridZone{id: "z1", needs: true, areas: []HitArea{
		{Rect: image.Rect(0, 0, 50, 50), ZIndex: GZSidebar, Tag: "panel"},
	}}
	tr.RegisterZoneVisible(z, GZSidebar, func() bool { return vis })
	tr.SetZoneRect("z1", image.Rect(0, 0, 50, 50))
	tr.layoutPass()
	if len(tr.hitIndex.At(10, 10)) != 1 {
		t.Fatal("expected hit area while visible")
	}
	vis = false
	tr.layoutPass()
	if got := tr.hitIndex.At(10, 10); len(got) != 0 {
		t.Fatalf("hidden zone must publish no hit areas, got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestGridTreePublishes|TestGridTreeInvisible' -v`
Expected: FAIL — `tr.layoutPass undefined`.

- [ ] **Step 3: Write minimal implementation** (append methods to `grid_tree.go`)

```go
// layoutPass lays out zones that need it and (re)publishes their hit
// areas. Visibility gate covers HitAreas AND Draw: a hidden zone
// publishes an empty hit-area set so invisible chrome can't take input
// (mirrors DrumViewTree.layoutPass — the "hidden panel swallows taps"
// regression guard).
func (t *GridTree) layoutPass() {
	for i := range t.zones {
		e := t.zones[i]
		nowVisible := e.visible == nil || e.visible()
		layoutChanged := e.zone.NeedsLayout() || e.rect != e.lastRect
		visibilityChanged := nowVisible != e.lastVisible
		switch {
		case !nowVisible:
			if visibilityChanged {
				t.hitIndex.Update(e.zone.ID(), nil)
			}
		case layoutChanged || visibilityChanged:
			e.zone.Layout(e.rect)
			e.lastRect = e.rect
			t.hitIndex.Update(e.zone.ID(), e.zone.HitAreas())
		}
		e.lastVisible = nowVisible
	}
}

// EnsureLayouts re-runs the layout+publish pass (idempotent).
func (t *GridTree) EnsureLayouts() { t.layoutPass() }

// LayoutZoneNow forces an immediate layout + republish for one zone.
func (t *GridTree) LayoutZoneNow(id string) {
	e, ok := t.zoneMap[id]
	if !ok {
		return
	}
	e.zone.Layout(e.rect)
	e.lastRect = e.rect
	if e.visible == nil || e.visible() {
		t.hitIndex.Update(e.zone.ID(), e.zone.HitAreas())
		e.lastVisible = true
	} else {
		t.hitIndex.Update(e.zone.ID(), nil)
		e.lastVisible = false
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestGridTreePublishes|TestGridTreeInvisible' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/grid_tree.go src/go/internal/ui/grid_tree_test.go
git commit -m "feat(ui): GridTree layout pass + visibility-gated hit publication

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: GridTree input dispatch (capture/suppress lifecycle, z-priority)

**Files:**
- Modify: `src/go/internal/ui/grid_tree.go`
- Test: `src/go/internal/ui/grid_tree_input_test.go`

- [ ] **Step 1: Write the failing test** (`grid_tree_input_test.go`)

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

// recordingHandler returns a configurable result and logs calls.
type recordingHandler struct {
	result  InputResult
	presses *[]string
	tag     string
}

func (h *recordingHandler) OnPress(int, int) InputResult { *h.presses = append(*h.presses, h.tag); return h.result }
func (h *recordingHandler) OnDrag(int, int)              {}
func (h *recordingHandler) OnRelease(int, int)           {}
func (h *recordingHandler) OnWheel(int, int, int) InputResult { return InputIgnored }

func TestGridTreeDispatchHighestZFirst(t *testing.T) {
	tr := NewGridTree()
	var presses []string
	// Two overlapping areas at the same point; higher z must get the press.
	tr.hitIndex.Update("low", []HitArea{{
		Rect: image.Rect(0, 0, 100, 100), ZIndex: GZCanvas,
		Handler: &recordingHandler{result: InputIgnored, presses: &presses, tag: "canvas"},
	}})
	tr.hitIndex.Update("high", []HitArea{{
		Rect: image.Rect(0, 0, 100, 100), ZIndex: GZSidebar,
		Handler: &recordingHandler{result: InputConsumed, presses: &presses, tag: "sidebar"},
	}})

	tr.dispatchPressForTest(20, 20)

	// Sidebar (z=60) consumes; canvas (z=20) must NOT be reached.
	if len(presses) != 1 || presses[0] != "sidebar" {
		t.Fatalf("expected only 'sidebar' to receive press, got %v", presses)
	}
	if !tr.inputHandled || !tr.suppress {
		t.Fatalf("consumed press must set inputHandled+suppress")
	}
}

func TestGridTreeIgnoredFallsThrough(t *testing.T) {
	tr := NewGridTree()
	var presses []string
	tr.hitIndex.Update("low", []HitArea{{
		Rect: image.Rect(0, 0, 100, 100), ZIndex: GZCanvas,
		Handler: &recordingHandler{result: InputConsumed, presses: &presses, tag: "canvas"},
	}})
	tr.hitIndex.Update("high", []HitArea{{
		Rect: image.Rect(0, 0, 100, 100), ZIndex: GZSidebar,
		Handler: &recordingHandler{result: InputIgnored, presses: &presses, tag: "sidebar"},
	}})

	tr.dispatchPressForTest(20, 20)

	// Sidebar ignores -> falls through to canvas which consumes.
	if len(presses) != 2 || presses[0] != "sidebar" || presses[1] != "canvas" {
		t.Fatalf("expected sidebar then canvas, got %v", presses)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestGridTreeDispatch|TestGridTreeIgnored' -v`
Expected: FAIL — `tr.dispatchPressForTest undefined`.

- [ ] **Step 3: Write minimal implementation** (append to `grid_tree.go`)

This ports DrumViewTree's press/drag/release/wheel lifecycle minus the portal. `Update()` wires real input; `dispatchPressForTest` is a thin seam so the dispatch logic is unit-testable without Ebiten input globals.

```go
import_marker_for_review_only := 0 // (delete) — see note: add "github.com/hajimehoshi/ebiten/v2" to imports

// Update runs Layout → Update → Input. Draw is separate via Draw().
func (t *GridTree) Update() {
	t.inputHandled = false
	t.wheelHandled = false
	t.layoutPass()
	for i := range t.zones {
		t.zones[i].zone.Update()
	}
	t.handleInput()
}

func (t *GridTree) handleInput() {
	mx, my := cursorPosition()
	pressed := isMouseButtonPressed(ebiten.MouseButtonLeft)

	if t.suppress && !t.wasPressed {
		t.suppress = false
	}

	// Release.
	if !pressed && t.wasPressed {
		if t.capturedHandler != nil {
			t.capturedHandler.OnRelease(mx, my)
			t.capturedHandler = nil
			t.capturedTag = ""
		}
		t.suppress = false
		t.wasPressed = false
		return
	}

	// Ongoing capture (drag).
	if pressed && t.capturedHandler != nil {
		t.capturedHandler.OnDrag(mx, my)
		t.wasPressed = true
		return
	}

	// New press.
	if pressed && !t.wasPressed {
		t.wasPressed = true
		if t.suppress {
			return
		}
		t.dispatchPress(mx, my)
		return
	}

	// Wheel (dispatched unconditionally).
	wx, wy := wheel()
	steps := int(wy)
	if wx != 0 && steps == 0 {
		steps = int(wx)
	}
	if steps != 0 {
		for _, h := range t.hitIndex.At(mx, my) {
			if h.Handler == nil {
				continue
			}
			if h.Handler.OnWheel(mx, my, steps) != InputIgnored {
				t.wheelHandled = true
				break
			}
		}
	}
}

// dispatchPress walks hits z-descending; stops on Captured/Consumed.
func (t *GridTree) dispatchPress(mx, my int) {
	dragBlocked := t.dragActive != nil && t.dragActive()
	hits := t.hitIndex.At(mx, my)
	for _, hit := range hits {
		if hit.Handler == nil {
			continue
		}
		if dragBlocked {
			return
		}
		switch hit.Handler.OnPress(mx, my) {
		case InputCaptured:
			t.capturedHandler = hit.Handler
			t.capturedTag = hit.Tag
			t.suppress = true
			t.inputHandled = true
			return
		case InputConsumed:
			t.suppress = true
			t.inputHandled = true
			return
		case InputIgnored:
			continue
		}
	}
}

// dispatchPressForTest exposes dispatchPress for unit tests without the
// Ebiten input globals.
func (t *GridTree) dispatchPressForTest(mx, my int) { t.dispatchPress(mx, my) }

func (t *GridTree) Suppress() bool      { return t.suppress }
func (t *GridTree) Capturing() bool     { return t.capturedHandler != nil }
func (t *GridTree) CapturedTag() string { return t.capturedTag }
func (t *GridTree) InputHandled() bool  { return t.inputHandled }
func (t *GridTree) WheelHandled() bool  { return t.wheelHandled }
func (t *GridTree) ClearCapture() {
	t.capturedHandler = nil
	t.capturedTag = ""
	t.suppress = false
}
```

> **Implementer note:** delete the `import_marker_for_review_only` line; it only flags that `grid_tree.go`'s import block must now include `"github.com/hajimehoshi/ebiten/v2"` (already present from Task 1) — `cursorPosition`, `isMouseButtonPressed`, `wheel`, `inputChars` are package-local helpers in `input.go`/`touch.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestGridTreeDispatch|TestGridTreeIgnored' -v`
Expected: PASS.

- [ ] **Step 5: Run the full GridTree suite + vet**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestGridTree' -v`
Expected: PASS (all GridTree tests).

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/grid_tree.go src/go/internal/ui/grid_tree_input_test.go
git commit -m "feat(ui): GridTree input dispatch (z-priority, capture/suppress)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 2 — Draw migration (fixes the bug)

> Strategy: split `drawGridPane` into focused `(*Game).drawXxx(dst *ebiten.Image)` methods (verbatim cut of the existing blocks), wrap each in a thin Layer/Zone adapter, register them with a per-`Game` `gridTree`, and make `drawGridPane` dispatch-only. The badge becomes a low-z Layer → the reported bug is fixed at Task 9.

### Task 4: Add `gridTree` field + construct it; grid pane adapters file

**Files:**
- Modify: `src/go/internal/ui/game_struct.go` (add field near `gridPaneSub` block, ~line 306)
- Modify: `src/go/internal/ui/game_new.go` (construct in `New`, ~line 22+)
- Create: `src/go/internal/ui/grid_pane_layers.go` (adapters; filled across Tasks 5–13)
- Test: `src/go/internal/ui/grid_pane_tree_test.go`

- [ ] **Step 1: Write the failing test** (`grid_pane_tree_test.go`)

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestGameHasGridTreeAfterNew(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	if g.gridTree == nil {
		t.Fatal("g.gridTree must be constructed in New()")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestGameHasGridTree -v`
Expected: FAIL — `g.gridTree undefined`.

- [ ] **Step 3: Implement**

In `game_struct.go`, add to the `Game` struct (next to `gridPaneSub` fields ~line 306):

```go
	// gridTree owns the top grid pane's z-ordered draw + input tree
	// (sibling to drum.tree which owns the bottom pane). See grid_tree.go.
	gridTree *GridTree
```

In `game_new.go` inside `New(...)`, after the `g := &Game{...}` is available (place near other sub-component construction, before `return g`):

```go
	g.gridTree = NewGridTree()
	g.registerGridTree()
```

Create `grid_pane_layers.go` with the registration stub (participants added in later tasks):

```go
package ui

// registerGridTree wires the grid pane's Layers and Zones into g.gridTree
// in ascending z-order. Each participant is a thin adapter delegating to a
// (*Game).drawXxx method (see grid_pane_draw.go). Order here is cosmetic —
// GridTree sorts by ZIndex — but kept ascending for readability and to
// mirror the z-order table test.
func (g *Game) registerGridTree() {
	// Participants registered in Phase 2 tasks 5–13.
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestGameHasGridTree -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_struct.go src/go/internal/ui/game_new.go \
        src/go/internal/ui/grid_pane_layers.go src/go/internal/ui/grid_pane_tree_test.go
git commit -m "feat(ui): add g.gridTree field + registration scaffold

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Extract draw blocks into `(*Game).drawXxx` methods (mechanical, no behavior change)

> This task ONLY relocates code. It does not yet route through the tree. After it, `drawGridPane` calls the new methods in the same order — pixels are byte-identical.

**Files:**
- Create: `src/go/internal/ui/grid_pane_draw.go`
- Modify: `src/go/internal/ui/game_draw_grid_pane.go`

- [ ] **Step 1: Create `grid_pane_draw.go` with method shells and move blocks verbatim**

Cut each block listed below **out of** `(*Game).drawGridPane` (in `game_draw_grid_pane.go`) and paste it **into** the corresponding method body in `grid_pane_draw.go`. Each method takes `dst *ebiten.Image` and reuses the existing locals by recomputing the few it needs at the top (the blocks already reference `g.cam`, `g.split`, `gridTopOffset()`, etc.). Where a block used the function-local `top`/`dst` subimage, the method's `dst` parameter replaces it.

```go
package ui

import "github.com/hajimehoshi/ebiten/v2"

// grid_pane_draw.go — the grid pane's draw blocks, relocated verbatim out
// of (*Game).drawGridPane so each can be owned by a GridTree Layer/Zone.
// These methods are the ONLY place grid-pane draw primitives live; the
// dispatcher drawGridPane must stay pixel-free (TestGridRenderPipelineDiscipline).

// drawGridBackground paints the vertical gradient + tiled grid cache.
// (was game_draw_grid_pane.go lines ~43–144)
func (g *Game) drawGridBackground(dst *ebiten.Image) { /* moved block */ }

// drawGridEdges renders edges + edge cache + link preview + screen-edge debug.
// (was lines ~200–431 and the SCREEN_EDGES summary ~972–1051)
func (g *Game) drawGridEdges(dst *ebiten.Image) { /* moved block */ }

// drawGridNodes renders nodes (sprite cache + fallback), state overlays,
// selection halos, glow. (was lines ~485–975 + overlay/halo helpers already
// in separate funcs)
func (g *Game) drawGridNodes(dst *ebiten.Image) { /* moved block */ }

// drawGridPulses renders edge pulse signals. (was lines ~1001–1025)
func (g *Game) drawGridPulses(dst *ebiten.Image) { /* moved block */ }

// drawGridCoordBadge renders the (i,j) badge above the selected node.
// (was lines ~1053–1080)
func (g *Game) drawGridCoordBadge(dst *ebiten.Image) { /* moved block */ }

// drawGridMoveMode renders the move banner + ghost. (was lines ~1082–1121)
func (g *Game) drawGridMoveMode(dst *ebiten.Image) { /* moved block */ }

// drawGridMoveConfirm renders the move-confirm dialog. (was lines ~1123–1140)
func (g *Game) drawGridMoveConfirm(dst *ebiten.Image) { /* moved block */ }

// drawGridCursorLabel renders the desktop cursor coordinate label to the
// UNCLIPPED screen. (was lines ~1148–1169 — note it draws to `screen`, so
// this method receives the unclipped image from its Layer.)
func (g *Game) drawGridCursorLabel(dst *ebiten.Image) { /* moved block */ }
```

> Notes on the relocation:
> - The **node sidebar** (`g.sidebar.Draw(screen)`, ~996), **long-press popup** (`g.drawLongPressPopup(dst)`, ~1143), and **connect-mode** (`g.drawConnectMode(dst)`, ~1146) already live in their own functions — they do NOT need a new `drawGridXxx` wrapper; their Zones/Layers (Tasks 10–13) call the existing funcs directly.
> - `drawGridBackground` must keep using the grid-pane subimage logic. Move the `top := screen.SubImage(gridRect)` setup into the dispatcher (Task 8) and pass the subimage as `dst` to the clipped layers; the cursor-label layer instead gets the raw `screen`.

- [ ] **Step 2: Make `drawGridPane` call the new methods in the same order** (temporary — replaced in Task 8)

In `game_draw_grid_pane.go`, the body of `drawGridPane` becomes a sequence of `g.drawXxx(top)` / `g.drawXxx(screen)` calls in the original order (background, edges, nodes, sidebar, pulses, badge, move-mode, move-confirm, long-press, connect, cursor-label). This is a stepping stone so the build stays green and pixels are unchanged.

- [ ] **Step 3: Run the UI suite to confirm no behavior change**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'Grid|Node|Draw|Pixel' -v`
Expected: PASS (same as before this task — pure relocation).

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/grid_pane_draw.go src/go/internal/ui/game_draw_grid_pane.go
git commit -m "refactor(ui): extract drawGridPane blocks into focused (*Game).drawGrid* methods

Pure relocation — drawGridPane calls them in the same order, pixels unchanged.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Background / edges / pulses / cursor-label Layers (draw-only)

**Files:**
- Modify: `src/go/internal/ui/grid_pane_layers.go`

- [ ] **Step 1: Add the draw-only Layer adapters + register them**

Append to `grid_pane_layers.go`:

```go
import "github.com/hajimehoshi/ebiten/v2"

// gridLayer is a thin draw-only adapter: id + z + a draw func bound to *Game.
type gridLayer struct {
	id   string
	z    int
	vis  func() bool
	draw func(dst *ebiten.Image)
}

func (l gridLayer) ID() string  { return l.id }
func (l gridLayer) ZIndex() int { return l.z }
func (l gridLayer) Visible() bool {
	if l.vis == nil {
		return true
	}
	return l.vis()
}
func (l gridLayer) Draw(dst *ebiten.Image) { l.draw(dst) }
```

Fill `registerGridTree`:

```go
func (g *Game) registerGridTree() {
	g.gridTree.RegisterLayer(gridLayer{id: "grid-bg", z: GZBackground, draw: g.drawGridBackground})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-edges", z: GZEdges, draw: g.drawGridEdges})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-pulses", z: GZPulses, draw: g.drawGridPulses})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-coord-badge", z: GZCoordBadge, draw: g.drawGridCoordBadge})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-move-mode", z: GZMoveMode, vis: func() bool { return g.moveMode && g.movingNode != nil }, draw: g.drawGridMoveMode})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-connect-mode", z: GZConnectMode, draw: g.drawConnectMode})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-move-confirm", z: GZMoveConfirm, vis: func() bool { return g.moveConfirm }, draw: g.drawGridMoveConfirm})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-cursor-label", z: GZCursorLabel, draw: g.drawGridCursorLabel})
	// Zones (canvas/nodes, long-press popup, node sidebar) added in Phase 3
	// (Task 10–13). For Phase 2, nodes + popup + sidebar remain draw-only
	// layers (added below) so the draw migration is independent of input.
	g.gridTree.RegisterLayer(gridLayer{id: "grid-nodes", z: GZCanvas, draw: g.drawGridNodes})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-longpress", z: GZLongPress, vis: func() bool { return g.longPressPopup }, draw: g.drawLongPressPopup})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-sidebar", z: GZSidebar, vis: func() bool { return g.sidebar.IsOpen() }, draw: func(dst *ebiten.Image) { g.sidebar.Draw(dst) }})
}
```

> The `grid-nodes`, `grid-longpress`, and `grid-sidebar` entries are **draw-only Layers in Phase 2** and get **upgraded to input Zones in Phase 3** (Tasks 10–13) by re-registering as Zones and removing these Layer lines. This keeps draw and input migrations independent and each independently revertible.

- [ ] **Step 2: Build only (no behavior wired yet)**

Run: `../../.tools/go/bin/go build -tags test ./internal/ui`
Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/grid_pane_layers.go
git commit -m "feat(ui): grid pane draw-only Layer adapters registered with GridTree

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Z-order table test (pins the grid layer order)

**Files:**
- Test: `src/go/internal/ui/grid_layer_zorder_test.go`

- [ ] **Step 1: Write the test**

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestGridLayerZOrder pins the grid pane's Layer/Zone draw order. Adding,
// removing, or reordering a grid participant MUST update both the GZ*
// constants in grid_tree.go and this expected list. Mirrors
// TestDrumViewLayerZOrder for the top pane.
func TestGridLayerZOrder(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	got := g.gridTree.LayersForTest()
	type expected struct {
		id string
		z  int
	}
	want := []expected{
		{"grid-bg", GZBackground},
		{"grid-edges", GZEdges},
		{"grid-nodes", GZCanvas},
		{"grid-pulses", GZPulses},
		{"grid-coord-badge", GZCoordBadge},
		{"grid-move-mode", GZMoveMode},
		{"grid-connect-mode", GZConnectMode},
		{"grid-longpress", GZLongPress},
		{"grid-move-confirm", GZMoveConfirm},
		{"grid-sidebar", GZSidebar},
		{"grid-cursor-label", GZCursorLabel},
	}
	if len(got) != len(want) {
		for i, l := range got {
			t.Logf("got[%2d] id=%-20q z=%d", i, l.ID(), l.ZIndex())
		}
		t.Fatalf("grid layer count: got %d want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ID() != w.id || got[i].ZIndex() != w.z {
			t.Errorf("grid layer[%d]: got (%q,%d) want (%q,%d)", i, got[i].ID(), got[i].ZIndex(), w.id, w.z)
		}
	}
}
```

- [ ] **Step 2: Run test**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestGridLayerZOrder -v`
Expected: PASS. (If FAIL on order, the `insertLayer` stable-sort places equal-z by insertion order; ensure `grid-nodes` is registered before `grid-pulses` etc. per the want list — adjust registration order to match.)

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/grid_layer_zorder_test.go
git commit -m "test(ui): pin GridTree layer z-order table

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Make `drawGridPane` dispatch-only (`gridTree.Draw`)

**Files:**
- Modify: `src/go/internal/ui/game_draw_grid_pane.go`

- [ ] **Step 1: Replace the body of `drawGridPane` with tree dispatch**

```go
func (g *Game) drawGridPane(screen *ebiten.Image) {
	gridRect := g.split.GridRect(g.winW, g.winH)
	// Reuse the cached grid-pane subimage (clip target for clipped layers).
	var top *ebiten.Image
	if g.gridPaneSubParent == screen && g.gridPaneSubRect == gridRect && g.gridPaneSub != nil {
		top = g.gridPaneSub
	} else {
		top = screen.SubImage(gridRect).(*ebiten.Image)
		g.gridPaneSubParent = screen
		g.gridPaneSubRect = gridRect
		g.gridPaneSub = top
	}
	_ = top // clipped layers are clipped by GridTree via SetZoneRect/SetBounds
	g.gridTree.SetBounds(gridRect)
	g.gridTree.EnsureLayouts()
	g.gridTree.Draw(screen)
}
```

> **Clip policy:** plain Layers self-clip via their draw code (which already references `gridRect`/`gridTopOffset()`), exactly as the pre-refactor blocks did when they drew into `top`. The cursor-label layer intentionally draws to the unclipped `screen`. If any layer relied on the SubImage hard-clip, give that layer a zone rect via `g.gridTree.SetZoneRect`/register as a Zone; for Phase 2 the draw-into-`top` blocks already self-bound. Verify visually in Step 3.

- [ ] **Step 2: Confirm `drawGridPane` is now pixel-free except dispatch**

Visual scan: no `drawRect`/`DrawTextAt`/etc. remain in `drawGridPane`. (Task 14 adds the enforcing test.)

- [ ] **Step 3: Run pixel + render tests**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'Pixel|Render|Grid|Node|Draw' -v`
Expected: PASS. If a layer renders in the wrong place, it's a clip issue from Step 1 — fix by registering that layer's `dst` expectations (most blocks already drew to `top`, so they self-bound).

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/game_draw_grid_pane.go
git commit -m "refactor(ui): drawGridPane is now GridTree.Draw dispatch-only

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Bug regression test — coord badge never paints over the sidebar

**Files:**
- Test: `src/go/internal/ui/grid_coord_badge_zorder_test.go`

- [ ] **Step 1: Write the failing test (run BEFORE removing the desktop guard to prove the test detects the bug, then keep it as a permanent guard)**

```go
//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestCoordBadgeBelowSidebarZ is the structural guard for the reported bug
// (coordinate badge painting over the node pop-up menu). The badge Layer
// must sit strictly below the sidebar and long-press popup Layers/Zones so
// GridTree.Draw composites the popups on top.
func TestCoordBadgeBelowSidebarZ(t *testing.T) {
	if !(GZCoordBadge < GZLongPress && GZCoordBadge < GZSidebar && GZCoordBadge < GZMoveConfirm) {
		t.Fatalf("badge z=%d must be below popup z=%d/%d/%d",
			GZCoordBadge, GZLongPress, GZMoveConfirm, GZSidebar)
	}
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	got := g.gridTree.LayersForTest()
	idx := func(id string) int {
		for i, l := range got {
			if l.ID() == id {
				return i
			}
		}
		return -1
	}
	if idx("grid-coord-badge") > idx("grid-sidebar") {
		t.Errorf("badge draws after sidebar (would overpaint): badge@%d sidebar@%d",
			idx("grid-coord-badge"), idx("grid-sidebar"))
	}
}
```

- [ ] **Step 2: Add a pixel-level guard** (same file) using the existing drawRect interception pattern referenced in CLAUDE.md (`audio_panel_render_pixels_test.go`). Drive desktop profile, open the sidebar, select a node positioned under the sidebar rect, render, and assert the badge text color does not appear inside the sidebar panel rect.

```go
func TestCoordBadgeNotPaintedOverOpenSidebarDesktop(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720) // desktop class

	// Select a node and open the sidebar over it.
	if len(g.nodes) == 0 {
		t.Skip("no nodes in default demo")
	}
	n := g.nodes[0]
	g.sel = n
	n.Selected = true
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame
	g.sidebar.Open(n)
	g.gridTree.EnsureLayouts()

	img := ebitenNewScreenForTest(1280, 720) // helper: ebiten.NewImage(1280,720)
	g.drawGridPane(img)

	// Sidebar panel rect (left-anchored). Sample a band inside it for badge
	// pill-fill color (genColorVizPillFill). It must be absent — the sidebar
	// composites on top.
	panel := image.Rect(0, gridTopOffset(), g.sidebar.width, g.split.GridH(g.winH))
	if rectContainsColor(img, panel, genColorVizPillFill) {
		t.Error("coord badge pill painted inside open sidebar rect (regression)")
	}
}
```

> **Helper note:** if `rectContainsColor` / `ebitenNewScreenForTest` don't exist, add small local helpers in this `_test.go` file: `ebitenNewScreenForTest` = `ebiten.NewImage(w,h)`; `rectContainsColor` scans `img.At` over the rect for an exact RGBA match to the token color. Reuse the existing scan helper from `audio_panel_render_pixels_test.go` if one is exported within the package.

- [ ] **Step 3: Run**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestCoordBadge' -v`
Expected: PASS (bug fixed structurally in Task 8).

- [ ] **Step 4: Optional desktop-guard cleanup.** In `grid_pane_draw.go`'s `drawGridCoordBadge`, the desktop branch's `(g.frame-g.coordBadgeFrame < 180)` may now also gate `&& !g.sidebar.IsOpen()` purely as a UX nicety (don't show a stale badge under a panel). This is no longer load-bearing for the bug. Keep the mobile guard as-is.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/grid_coord_badge_zorder_test.go src/go/internal/ui/grid_pane_draw.go
git commit -m "test(ui): regression guard — coord badge never overpaints sidebar/popup

Fixes the reported badge-over-popup bug structurally via GridTree z-order.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 3 — Input migration (wrap + arbitrate)

> The overlay/canvas zones now publish HitAreas and `Game.Update` routes grid pointer input through `gridTree` first. Legacy paths run only when the tree did not claim the press. The intricate gesture state machines are reused, not rewritten.

### Task 10: NodeSidebarZone — upgrade sidebar Layer → input Zone

**Files:**
- Create: `src/go/internal/ui/grid_sidebar_zone.go`
- Modify: `src/go/internal/ui/grid_pane_layers.go` (remove the `grid-sidebar` Layer line; register the Zone)
- Test: `src/go/internal/ui/grid_sidebar_zone_test.go`

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestSidebarZoneCatchAllOpaque(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if len(g.nodes) == 0 {
		t.Skip("no nodes")
	}
	g.sidebar.Open(g.nodes[0])
	g.gridTree.EnsureLayouts()

	// A press inside the sidebar panel must hit the sidebar catch-all
	// (z=GZSidebar), NOT fall through to the node canvas (z=GZCanvas).
	hits := g.gridTree.HitIndexRef().At(10, gridTopOffset()+40)
	if len(hits) == 0 {
		t.Fatal("expected sidebar hit area inside panel")
	}
	if hits[0].ZIndex != GZSidebar {
		t.Fatalf("topmost hit z=%d want sidebar z=%d (click-through risk)", hits[0].ZIndex, GZSidebar)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestSidebarZoneCatchAll -v`
Expected: FAIL — no sidebar hit area (still a draw-only Layer).

- [ ] **Step 3: Implement `grid_sidebar_zone.go`**

A Zone wrapping the existing sidebar. Draw delegates to `g.sidebar.Draw`. HitAreas publishes a full-panel catch-all at `GZSidebar` (opaque-to-z) whose handler delegates to the existing sidebar input (`g.handleNodeMenuButtons` / `g.sidebar` dispatch). Visibility gate = `g.sidebar.IsOpen()`.

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

type nodeSidebarZone struct {
	g    *Game
	rect image.Rectangle
}

func (z *nodeSidebarZone) ID() string { return "grid-sidebar" }
func (z *nodeSidebarZone) Layout(r image.Rectangle) { z.rect = r }
func (z *nodeSidebarZone) Update() {}
func (z *nodeSidebarZone) Draw(dst *ebiten.Image) {
	if z.g.sidebar.IsOpen() {
		z.g.sidebar.Draw(dst)
	}
}
func (z *nodeSidebarZone) NeedsLayout() bool { return false }
func (z *nodeSidebarZone) Invalidate() {}
func (z *nodeSidebarZone) HandleKey(ebiten.Key) InputResult { return InputIgnored }
func (z *nodeSidebarZone) HandleChars([]rune) InputResult   { return InputIgnored }

func (z *nodeSidebarZone) HitAreas() []HitArea {
	if !z.g.sidebar.IsOpen() {
		return nil
	}
	panel := z.g.sidebar.PanelRect() // add accessor returning rects["panel"] in screen coords
	return []HitArea{{
		Rect:    panel,
		ZIndex:  GZSidebar,
		Handler: &sidebarHitHandler{g: z.g},
		Tag:     "grid-sidebar-capture",
	}}
}

// sidebarHitHandler routes presses to the existing sidebar input path.
type sidebarHitHandler struct{ g *Game }

func (h *sidebarHitHandler) OnPress(x, y int) InputResult {
	// Delegate to the existing sidebar button/scroll dispatch. Returns
	// Consumed so the press never falls through to the node canvas.
	h.g.handleNodeMenuButtons(x, y, true)
	return InputConsumed
}
func (h *sidebarHitHandler) OnDrag(x, y int)              { h.g.handleNodeMenuButtons(x, y, true) }
func (h *sidebarHitHandler) OnRelease(x, y int)           { h.g.handleNodeMenuButtons(x, y, false) }
func (h *sidebarHitHandler) OnWheel(x, y, steps int) InputResult {
	h.g.sidebar.ScrollBy(steps) // reuse existing scroll entry; return Consumed if it scrolled
	return InputConsumed
}
```

> **Accessor additions (small, in `game_node_sidebar.go`):**
> - `func (sb *NodeSidebar) PanelRect() image.Rectangle { return sb.rects["panel"] }`
> - `func (sb *NodeSidebar) ScrollBy(steps int)` if no equivalent exists — wrap the existing wheel handling.
> Check the exact existing method names (`handleNodeMenuButtons` signature, scroll API) and match them; the bodies above are the integration contract, adapt arg lists to the real signatures.

In `grid_pane_layers.go`, replace the `grid-sidebar` Layer registration with:

```go
	g.gridTree.RegisterZoneVisible(&nodeSidebarZone{g: g}, GZSidebar, func() bool { return g.sidebar.IsOpen() })
```

- [ ] **Step 4: Run to verify it passes**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestSidebarZoneCatchAll -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/grid_sidebar_zone.go src/go/internal/ui/grid_pane_layers.go \
        src/go/internal/ui/game_node_sidebar.go src/go/internal/ui/grid_sidebar_zone_test.go
git commit -m "feat(ui): NodeSidebarZone — opaque-to-z input zone for the node sidebar

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: LongPressPopupZone + MoveConfirmZone — upgrade popups to input Zones

**Files:**
- Create: `src/go/internal/ui/grid_popup_zones.go`
- Modify: `src/go/internal/ui/grid_pane_layers.go`
- Test: `src/go/internal/ui/grid_popup_zones_test.go`

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestLongPressPopupZoneOpaque(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if len(g.nodes) == 0 {
		t.Skip("no nodes")
	}
	g.showLongPressPopup(g.nodes[0], 200, 200)
	g.gridTree.EnsureLayouts()
	c := g.longPressPopupRect.Min.Add(g.longPressPopupRect.Max).Div(2) // center
	hits := g.gridTree.HitIndexRef().At(c.X, c.Y)
	if len(hits) == 0 || hits[0].ZIndex < GZLongPress {
		t.Fatalf("press in popup must hit popup zone first, got %+v", hits)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestLongPressPopupZoneOpaque -v`
Expected: FAIL — no popup hit area.

- [ ] **Step 3: Implement `grid_popup_zones.go`**

Two Zones. `longPressPopupZone` publishes a scrim + panel catch-all at `GZLongPress` plus three button HitAreas (Move/Connect/Delete) at `GZLongPress+1`; handlers call the existing `dismissLongPressPopup`/`moveMode`/`enterConnectMode`/`deleteNode` actions (the same switch in `updateLongPressPopup`). `moveConfirmZone` publishes a panel catch-all at `GZMoveConfirm` + Move/Cancel buttons. Draw delegates to the existing `drawLongPressPopup` / `drawGridMoveConfirm`.

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

type longPressPopupZone struct{ g *Game }

func (z *longPressPopupZone) ID() string { return "grid-longpress" }
func (z *longPressPopupZone) Layout(image.Rectangle) {}
func (z *longPressPopupZone) Update() {}
func (z *longPressPopupZone) Draw(dst *ebiten.Image) { z.g.drawLongPressPopup(dst) }
func (z *longPressPopupZone) NeedsLayout() bool { return false }
func (z *longPressPopupZone) Invalidate() {}
func (z *longPressPopupZone) HandleKey(ebiten.Key) InputResult { return InputIgnored }
func (z *longPressPopupZone) HandleChars([]rune) InputResult   { return InputIgnored }

func (z *longPressPopupZone) HitAreas() []HitArea {
	if !z.g.longPressPopup {
		return nil
	}
	g := z.g
	mk := func(r image.Rectangle, action string) HitArea {
		return HitArea{Rect: r, ZIndex: GZLongPress + 1, Tag: "lp-" + action,
			Handler: &longPressBtnHandler{g: g, action: action}}
	}
	return []HitArea{
		// catch-all panel (opaque-to-z) below the buttons:
		NewInputCaptureHitArea(g.longPressPopupRect, GZLongPress, "grid-longpress-capture"),
		mk(g.longPressPopupMove, "move"),
		mk(g.longPressPopupConn, "connect"),
		mk(g.longPressPopupDel, "delete"),
	}
}

type longPressBtnHandler struct {
	g      *Game
	action string
}

func (h *longPressBtnHandler) OnPress(int, int) InputResult { return InputCaptured } // wait for release
func (h *longPressBtnHandler) OnDrag(int, int)              {}
func (h *longPressBtnHandler) OnRelease(int, int) {
	g := h.g
	node := g.longPressPopupNode
	switch h.action {
	case "move":
		g.dismissLongPressPopup()
		g.moveMode = true
		g.movingNode = node
		g.moveSkipRelease = true
	case "connect":
		g.dismissLongPressPopup()
		g.enterConnectMode(node)
	case "delete":
		g.dismissLongPressPopup()
		g.deleteNode(node)
	}
}
func (h *longPressBtnHandler) OnWheel(int, int, int) InputResult { return InputIgnored }
```

Add the analogous `moveConfirmZone` (catch-all at `GZMoveConfirm`, Move/Cancel buttons calling the existing move-confirm handlers). In `grid_pane_layers.go`, replace the `grid-longpress` and `grid-move-confirm` Layer lines with Zone registrations gated by `func() bool { return g.longPressPopup }` and `func() bool { return g.moveConfirm }`.

- [ ] **Step 4: Run to verify it passes**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestLongPressPopupZone|TestMoveConfirm' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/grid_popup_zones.go src/go/internal/ui/grid_pane_layers.go \
        src/go/internal/ui/grid_popup_zones_test.go
git commit -m "feat(ui): LongPressPopupZone + MoveConfirmZone input zones

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: GridCanvasZone — nodes as a z=20 catch-all that arbitrates with pan

**Files:**
- Create: `src/go/internal/ui/grid_canvas_zone.go`
- Modify: `src/go/internal/ui/grid_pane_layers.go` (remove `grid-nodes` Layer; register Zone)
- Test: `src/go/internal/ui/grid_canvas_zone_test.go`

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// A press ON a node is consumed by the canvas; a press on EMPTY grid space
// is ignored so legacy pan can proceed.
func TestGridCanvasConsumesNodeIgnoresEmpty(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if len(g.nodes) == 0 {
		t.Skip("no nodes")
	}
	z := &gridCanvasZone{g: g}
	n := g.nodes[0]
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	cx, cy := int((x1+x2)/2), int((y1+y2)/2)
	if z.hitHandler().OnPress(cx, cy) != InputConsumed {
		t.Error("press on node must be consumed by canvas")
	}
	// Empty space far from any node:
	if got := z.hitHandler().OnPress(g.split.GridW(g.winW)-2, gridTopOffset()+2); got != InputIgnored {
		t.Errorf("press on empty grid must be ignored (let pan run), got %v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestGridCanvasConsumes -v`
Expected: FAIL — `gridCanvasZone` undefined.

- [ ] **Step 3: Implement `grid_canvas_zone.go`**

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// gridCanvasZone owns node drawing + node hit-testing. Its catch-all spans
// the whole grid pane at z=GZCanvas but is CONDITIONALLY opaque: it consumes
// a press only when it lands on a node (delegating to the existing
// handleTapInGrid dispatch); presses on empty space are ignored so the
// legacy pan/zoom path (cam.HandleMouse) still runs. This is the deliberate
// exception to the opaque-to-z catch-all rule — empty grid IS transparent to
// pan (documented in grid_input_isolation_discipline_test.go).
type gridCanvasZone struct {
	g    *Game
	rect image.Rectangle
}

func (z *gridCanvasZone) ID() string { return "grid-nodes" }
func (z *gridCanvasZone) Layout(r image.Rectangle) { z.rect = r }
func (z *gridCanvasZone) Update() {}
func (z *gridCanvasZone) Draw(dst *ebiten.Image) { z.g.drawGridNodes(dst) }
func (z *gridCanvasZone) NeedsLayout() bool { return false }
func (z *gridCanvasZone) Invalidate() {}
func (z *gridCanvasZone) HandleKey(ebiten.Key) InputResult { return InputIgnored }
func (z *gridCanvasZone) HandleChars([]rune) InputResult   { return InputIgnored }

func (z *gridCanvasZone) HitAreas() []HitArea {
	if z.rect.Empty() {
		return nil
	}
	return []HitArea{{
		Rect:    z.rect,
		ZIndex:  GZCanvas,
		Handler: z.hitHandler(),
		Tag:     "grid-canvas",
	}}
}

func (z *gridCanvasZone) hitHandler() *gridCanvasHandler { return &gridCanvasHandler{g: z.g} }

type gridCanvasHandler struct{ g *Game }

func (h *gridCanvasHandler) OnPress(x, y int) InputResult {
	// Only claim the press if it lands on a node; otherwise let pan run.
	if h.g.nodeAtScreen(x, y) == nil {
		return InputIgnored
	}
	h.g.handleTapInGrid(x, y)
	return InputConsumed
}
func (h *gridCanvasHandler) OnDrag(int, int)              {}
func (h *gridCanvasHandler) OnRelease(int, int)           {}
func (h *gridCanvasHandler) OnWheel(int, int, int) InputResult { return InputIgnored }
```

In `grid_pane_layers.go`, replace the `grid-nodes` Layer line with:

```go
	g.gridTree.RegisterZone(&gridCanvasZone{g: g}, GZCanvas)
```

> Set the canvas zone rect each frame so its catch-all spans the grid pane:
> in `drawGridPane` (Task 8 body) add `g.gridTree.SetZoneRect("grid-nodes", g.split.GridRect(g.winW, g.winH))` before `EnsureLayouts()`, and likewise set `grid-sidebar` rect to the panel rect. (Or set them in the Update wiring of Task 13.)

- [ ] **Step 4: Run to verify it passes**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestGridCanvasConsumes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/grid_canvas_zone.go src/go/internal/ui/grid_pane_layers.go \
        src/go/internal/ui/grid_canvas_zone_test.go
git commit -m "feat(ui): GridCanvasZone — node hit-testing in the tree, pan pass-through

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 13: Route `Game.Update` grid input through `gridTree`; gate legacy paths

**Files:**
- Modify: `src/go/internal/ui/game_update.go` (the grid-input region ~lines 263–349)
- Test: `src/go/internal/ui/grid_input_arbitration_test.go`

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// With the sidebar open, a synthesized press inside the sidebar must NOT
// create or select a node beneath it (click-through eliminated).
func TestSidebarPressDoesNotReachCanvas(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if len(g.nodes) == 0 {
		t.Skip("no nodes")
	}
	g.sidebar.Open(g.nodes[0])
	g.gridTree.EnsureLayouts()
	before := len(g.nodes)

	// Press at a point inside the sidebar that also overlaps grid space.
	g.gridTree.dispatchPressForTest(8, gridTopOffset()+30)

	if len(g.nodes) != before {
		t.Errorf("sidebar press created/removed a node (click-through): %d -> %d", before, len(g.nodes))
	}
	if !g.gridTree.InputHandled() {
		t.Error("sidebar press must be claimed by the tree")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestSidebarPressDoesNotReachCanvas -v`
Expected: may PASS already if Task 10 routed the handler; if the legacy path still double-fires, it FAILS — proceed to Step 3.

- [ ] **Step 3: Wire `gridTree.Update()` and gate legacy grid input**

In `game_update.go`, just before the existing grid-input region (the `if gesture != nil` block at ~263 and the `inputDispatcher.Dispatch` at ~336), run the grid tree and short-circuit legacy paths when it claims the press:

```go
	// Grid pane z-ordered input arbitration. The tree owns node hit-testing
	// and the overlay zones (sidebar/popup/dialog). When it claims a press,
	// the legacy gesture/dispatcher/editor paths below are skipped for this
	// frame so input is handled exactly once.
	g.gridTree.SetBounds(g.split.GridRect(g.winW, g.winH))
	g.gridTree.SetZoneRect("grid-nodes", g.split.GridRect(g.winW, g.winH))
	if g.sidebar.IsOpen() {
		g.gridTree.SetZoneRect("grid-sidebar", g.sidebar.PanelRect())
	}
	g.gridTree.Update()
	gridTreeClaimed := g.gridTree.InputHandled() || g.gridTree.Capturing()
```

Then guard the legacy grid dispatch. Wrap the existing grid-tap gesture handling and the `inputDispatcher.Dispatch`/editor block so they are skipped when `gridTreeClaimed`:

```go
	if gesture != nil && !gridTreeClaimed {
		switch gesture.Kind {
		// ... existing cases unchanged ...
		}
	}
```

and at the dispatcher call (~336):

```go
		inputHandled := touchHandled || gridTreeClaimed
		if !inputHandled {
			inputHandled = g.inputDispatcher.Dispatch(mx, my, left)
		}
```

> **Caution:** the long-press popup early-return at ~303 (`if g.longPressPopup { g.updateLongPressPopup(...) }`) is now handled by `LongPressPopupZone`. Remove that branch ONLY after confirming the zone handler reproduces the slide-to-button-release behavior; otherwise keep both and let `gridTreeClaimed` suppress double-fire. Prefer the conservative path: keep `updateLongPressPopup` but gate it `if g.longPressPopup && !gridTreeClaimed`.

- [ ] **Step 4: Run the input arbitration + behavior-preservation tests**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestSidebarPress|TestGridCanvas|Node|Connect|Move|LongPress|Tap' -v`
Expected: PASS. Investigate any node-click/drag/connect/move regressions before continuing.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_update.go src/go/internal/ui/grid_input_arbitration_test.go
git commit -m "feat(ui): route grid input through GridTree; gate legacy paths on claim

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 4 — Discipline tests + docs

### Task 14: Grid opaque-to-z + render-pipeline discipline tests

**Files:**
- Test: `src/go/internal/ui/grid_input_isolation_discipline_test.go`
- Test: `src/go/internal/ui/grid_render_pipeline_discipline_test.go`

- [ ] **Step 1: Opaque-to-z discipline test** (mirror `zone_input_isolation_discipline_test.go`)

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Every OPAQUE grid overlay zone (sidebar, long-press popup, move-confirm)
// must register a full-bounds catch-all at its nominal z so a tap in its
// chrome whitespace can't fall through to a lower-z sibling. The node
// canvas is the documented exception (conditionally opaque — empty grid is
// transparent to pan).
func TestGridOpaqueZonesHaveCatchAll(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if len(g.nodes) == 0 {
		t.Skip("no nodes")
	}

	// Sidebar.
	g.sidebar.Open(g.nodes[0])
	g.gridTree.LayoutZoneNow("grid-sidebar")
	assertCatchAll(t, g, "grid-sidebar", GZSidebar)

	// Long-press popup.
	g.showLongPressPopup(g.nodes[0], 300, 300)
	g.gridTree.LayoutZoneNow("grid-longpress")
	assertCatchAll(t, g, "grid-longpress", GZLongPress)
}

func assertCatchAll(t *testing.T, g *Game, zoneID string, z int) {
	t.Helper()
	for _, e := range g.gridTree.zones {
		if e.zone.ID() != zoneID {
			continue
		}
		for _, a := range e.zone.HitAreas() {
			if a.ZIndex == z { // full-bounds catch-all sits at nominal z
				return
			}
		}
		t.Errorf("zone %q has no catch-all at nominal z=%d", zoneID, z)
		return
	}
	t.Errorf("zone %q not registered", zoneID)
}
```

- [ ] **Step 2: Render-pipeline discipline test** (mirror `render_pipeline_discipline_test.go`)

Add a test that parses `game_draw_grid_pane.go`, finds `(*Game).drawGridPane`, and asserts its body contains no calls in the forbidden draw-primitive set (the same map as `render_pipeline_discipline_test.go`) beyond the GridTree dispatch. Copy the parser walk from that file, swapping the gate to `{"game_draw_grid_pane.go", "drawGridPane"}`.

- [ ] **Step 3: Run**

Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestGridOpaque|TestGridRenderPipeline' -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/grid_input_isolation_discipline_test.go \
        src/go/internal/ui/grid_render_pipeline_discipline_test.go
git commit -m "test(ui): grid opaque-to-z + render-pipeline discipline guards

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 15: Full suite, docs, memory

**Files:**
- Modify: `CLAUDE.md` (grid pane section)
- Modify: project memory (`/home/ymolinar/.claude/projects/-home-ymolinar-Repos-beatmo/memory/`)

- [ ] **Step 1: Run the full fast UI suite + the broader fast suite**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
```
Expected: PASS (modulo the pre-existing unrelated failures noted in project memory `project_add_core_node_types_preexisting_failures.md` — diff against baseline, don't attribute pre-existing fails to this work).

- [ ] **Step 2: Run real-Ebiten grid input/draw tests if present**

Run: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/ui -run 'Grid|Node|Sidebar|LongPress|Connect|Move'`
Expected: PASS.

- [ ] **Step 3: Update `CLAUDE.md`** — under "UI Layout" add a short subsection noting the grid pane is now GridTree-managed (sibling to DrumViewTree), the GZ* z table, the conditionally-opaque canvas exception, and the two new discipline tests. Add a gotcha entry mirroring the DrumViewTree ones.

- [ ] **Step 4: Write a project memory file** summarizing: the bug (badge-over-sidebar = draw-order), the GridTree (shared primitives, no portal/keyboard-focus), the canvas conditional-opacity exception, and the test guards. Add a one-line pointer to `MEMORY.md`.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: GridTree z-axis grid pane architecture + gotchas

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-review (completed by plan author)

- **Spec coverage:** bug fix (Tasks 8–9) ✓; sibling GridTree reusing primitives (Tasks 1–3) ✓; component decomposition table (Tasks 5–13) ✓; wrap+arbitrate input (Tasks 10–13) ✓; render-pipeline discipline test (Task 14) ✓; opaque-to-z + z-order tests (Tasks 7, 14) ✓; behavior preservation (Tasks 5, 13, 15) ✓; phasing matches spec ✓.
- **Type consistency:** `GridTree`, `NewGridTree`, `RegisterLayer/RegisterZone/RegisterZoneVisible`, `SetBounds`, `SetZoneRect`, `EnsureLayouts`, `LayoutZoneNow`, `Draw`, `Update`, `dispatchPress`/`dispatchPressForTest`, `InputHandled`, `Capturing`, `HitIndexRef`, `LayersForTest` used consistently across Tasks 1–14. `GZ*` constants consistent. Adapter types (`gridLayer`, `nodeSidebarZone`, `longPressPopupZone`, `moveConfirmZone`, `gridCanvasZone`) and handlers named consistently.
- **Integration-contract caveats flagged inline:** sidebar accessor names (`PanelRect`, `ScrollBy`, `handleNodeMenuButtons` arg list), long-press release behavior, and the clip policy in Task 8 are marked "match the real signatures / verify visually" — these are the few spots needing the implementer to read the exact existing API before pasting.
```
