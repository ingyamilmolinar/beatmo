# Keyboard Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **REPO RULE — DO NOT COMMIT:** This repo carries a large uncommitted WIP and the user owns all commits. **Never run `git add`/`git commit`/`git stash`/`git checkout`/`git restore`/`git reset`.** Each task ends with a "Checkpoint" (run tests, leave changes in the working tree) instead of a commit. To revert a change, re-edit the lines back manually with the Edit tool — never `git`.

**Goal:** Add keyboard control for common actions (Space = play/pause; arrows pan; `[`/`]` zoom; `0` reset zoom; `+`/`−` BPM; `1`–`7` audio tabs; `?` help overlay) plus a universal Esc that cancels any pending interaction — all additive to existing mouse/touch.

**Architecture:** One new UI-thread handler `(*Game).handleGlobalShortcuts()` called from `game_update.go` right after `handleUndoRedoKeys()` (before `seqMu`). It mutates only UI-layer state (camera, transport pulse flags, audio-tab state, overlay portal). A single `(*Game).handleEscape()` becomes the sole Esc authority with a precedence ladder ending in Stop, consolidating today's scattered Esc handlers.

**Tech Stack:** Go 1.23, Ebiten v2, the project's stubbed-Ebiten test harness (`-tags test`). Tests drive the production input seam (`isKeyPressed`/`isKeyJustPressed` function variables) via the existing `stubKeys()` helper.

---

## Conventions used in every task

- **Bundled Go:** all test commands use `.tools/go/bin/go`.
- **Fast test command (from repo root):**
  ```bash
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <TestName> -v
  ```
- **Input seam:** `isKeyPressed` (level) and `isKeyJustPressed` (edge) are package-level function variables in `internal/ui/input.go`. `stubKeys(pressed, justPressed map[ebiten.Key]bool)` (in `undo_keyboard_test.go`) swaps both and returns a restore func.
- **Test game:** `newTestGameForUndo(t)` (in `undo_seam_test.go`) builds a laid-out `*Game` (800×600) with a start node, registered for cleanup. Use it for all new tests.

## File structure

| File | Responsibility | New/Modified |
|---|---|---|
| `internal/ui/game_input_shortcuts.go` | `handleGlobalShortcuts`, action handlers (pan/zoom/bpm/tabs/help-toggle), `handleEscape` ladder | **New** |
| `internal/ui/shortcuts_overlay.go` | `ShortcutsOverlay` (PortalOverlay) — the `?` help panel | **New** |
| `internal/ui/game_input_shortcuts_test.go` | All functional tests for the above | **New** |
| `internal/ui/camera.go` | add `(*Camera).Reset()` | Modify |
| `internal/ui/drumview_transport.go` | add `TriggerPlayPause()` / `TriggerStop()` | Modify |
| `internal/ui/game_update.go:263` | one call site: `g.handleGlobalShortcuts()` | Modify |
| `internal/ui/game_input_editor.go:66-106` | remove the migrated Esc checks (sidebar/connect/move) | Modify |
| `internal/ui/transport_zone.go` | desktop "?" help chip (button + hit area + accessor) | Modify |

---

## Task 1: Foundation helpers (Camera.Reset + transport trigger methods)

These are tiny seams the shortcut handlers call. TDD each.

**Files:**
- Modify: `internal/ui/camera.go`
- Modify: `internal/ui/drumview_transport.go`
- Test: `internal/ui/game_input_shortcuts_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/ui/game_input_shortcuts_test.go`:

```go
package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestCameraReset(t *testing.T) {
	c := NewCamera()
	c.Scale = 0.37
	c.OffsetX = 123
	c.OffsetY = -456
	c.Reset()
	if c.Scale != 2.0 {
		t.Fatalf("Reset Scale = %v, want 2.0", c.Scale)
	}
	if c.OffsetX != 0 || c.OffsetY != 0 {
		t.Fatalf("Reset Offset = (%v,%v), want (0,0)", c.OffsetX, c.OffsetY)
	}
}

func TestTriggerPlayPauseSetsPulse(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.TriggerPlayPause()
	if !g.drum.PlayPressed() {
		t.Fatal("TriggerPlayPause should make PlayPressed() return true once")
	}
	if g.drum.PlayPressed() {
		t.Fatal("PlayPressed() should be a one-shot (cleared after read)")
	}
}

func TestTriggerStopSetsPulse(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.TriggerStop()
	if !g.drum.StopPressed() {
		t.Fatal("TriggerStop should make StopPressed() return true once")
	}
	if g.drum.StopPressed() {
		t.Fatal("StopPressed() should be a one-shot (cleared after read)")
	}
}
```

(The unused `ebiten` import is consumed by later tasks in this same file; if running Task 1 in isolation, the import will be flagged — proceed to Step 3 which leaves it unused only transiently. If strict, temporarily drop the import and re-add in Task 2.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestCameraReset|TestTriggerPlayPauseSetsPulse|TestTriggerStopSetsPulse' -v`
Expected: FAIL — `c.Reset undefined`, `g.drum.TriggerPlayPause undefined`, `g.drum.TriggerStop undefined`.

- [ ] **Step 3: Add `Camera.Reset`**

In `internal/ui/camera.go`, immediately after `func NewCamera()` (line 18):

```go
// Reset restores the default zoom and recenters the view (the keyboard "0"
// shortcut). Default scale matches NewCamera (2.0).
func (c *Camera) Reset() {
	c.Scale = 2.0
	c.OffsetX = 0
	c.OffsetY = 0
	c.Snap()
}
```

- [ ] **Step 4: Add transport trigger methods**

In `internal/ui/drumview_transport.go`, after `StopPressed()` (line 54):

```go
// TriggerPlayPause sets the one-frame play/pause pulse, exactly as the
// on-screen Play button does. Consumed by Game.Update via PlayPressed().
func (dv *DrumView) TriggerPlayPause() { dv.playPressed = true }

// TriggerStop sets the one-frame stop pulse, exactly as the Stop button does.
// Consumed by Game.Update via StopPressed().
func (dv *DrumView) TriggerStop() { dv.stopPressed = true }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestCameraReset|TestTriggerPlayPauseSetsPulse|TestTriggerStopSetsPulse' -v`
Expected: PASS.

- [ ] **Step 6: Checkpoint** — leave changes in the working tree (do NOT commit).

---

## Task 2: `handleGlobalShortcuts` skeleton + text-input gate + Space play/pause

Wire the handler into `Update`, add the focused-text gate, and implement Space.

**Files:**
- Create: `internal/ui/game_input_shortcuts.go`
- Modify: `internal/ui/game_update.go:263`
- Test: `internal/ui/game_input_shortcuts_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/game_input_shortcuts_test.go`:

```go
func TestSpaceTogglesPlayPulse(t *testing.T) {
	g := newTestGameForUndo(t)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySpace: true})
	defer restore()

	g.handleGlobalShortcuts()
	if !g.drum.PlayPressed() {
		t.Fatal("Space should trigger the play/pause pulse")
	}
}

func TestActionKeysIgnoredWhileTextInputFocused(t *testing.T) {
	g := newTestGameForUndo(t)
	// Simulate a focused text field (BPM box / name input).
	g.drum.tree.SetFocus("transport")

	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySpace: true})
	defer restore()

	g.handleGlobalShortcuts()
	if g.drum.PlayPressed() {
		t.Fatal("Space must NOT trigger play while a text input owns the keyboard")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSpaceTogglesPlayPulse|TestActionKeysIgnoredWhileTextInputFocused' -v`
Expected: FAIL — `g.handleGlobalShortcuts undefined`.

- [ ] **Step 3: Create the handler file**

Create `internal/ui/game_input_shortcuts.go`:

```go
package ui

import "github.com/hajimehoshi/ebiten/v2"

// handleGlobalShortcuts dispatches keyboard shortcuts for common actions. It is
// called from Game.Update immediately after handleUndoRedoKeys (BEFORE seqMu is
// acquired), so — like undo/redo — it works everywhere (over popups, during
// drags) but must touch ONLY UI-layer state: camera, transport pulse flags,
// audio-tab state, and the overlay portal. It must never mutate the graph or
// predictor (those require seqMu) and must never call into drum.Update.
//
// Order matters: handleEscape runs first (so Esc still cancels even with a text
// field focused), then a focused text input swallows all remaining action keys
// (otherwise Space could not be typed into the BPM box / name fields).
func (g *Game) handleGlobalShortcuts() {
	if g.drum == nil {
		return
	}

	// Esc is the universal cancel; it owns its own focus handling.
	if g.handleEscape() {
		return
	}

	// A focused text input owns the keyboard this frame.
	if g.drum.tree != nil && g.drum.tree.focusedZone != "" {
		return
	}

	if isKeyJustPressed(ebiten.KeySpace) {
		g.drum.TriggerPlayPause()
	}
}

// handleEscape is the single authority for the Esc key. Filled in by Task 6.
// For now it does nothing and reports "not handled".
func (g *Game) handleEscape() bool { return false }
```

- [ ] **Step 4: Wire it into Update**

In `internal/ui/game_update.go`, after line 263 (`g.handleUndoRedoKeys()`), add:

```go
	g.handleUndoRedoKeys()
	g.handleGlobalShortcuts()
```

(Insert `g.handleGlobalShortcuts()` on the line directly below the existing `g.handleUndoRedoKeys()` call.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSpaceTogglesPlayPulse|TestActionKeysIgnoredWhileTextInputFocused' -v`
Expected: PASS.

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

## Task 3: Pan (arrows), Zoom (`[`/`]`), Reset zoom (`0`)

**Files:**
- Modify: `internal/ui/game_input_shortcuts.go`
- Test: `internal/ui/game_input_shortcuts_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/game_input_shortcuts_test.go`:

```go
func TestArrowKeysPan(t *testing.T) {
	g := newTestGameForUndo(t)
	g.cam.OffsetX = 0
	g.cam.OffsetY = 0

	// Right arrow scrolls content; left arrow pans the camera so content moves
	// right, i.e. OffsetX increases. We only assert direction + nonzero.
	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	g.handleGlobalShortcuts()
	restore()
	if g.cam.OffsetX <= 0 {
		t.Fatalf("ArrowLeft should increase OffsetX, got %v", g.cam.OffsetX)
	}

	g.cam.OffsetX, g.cam.OffsetY = 0, 0
	restore = stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowUp: true}, nil)
	g.handleGlobalShortcuts()
	restore()
	if g.cam.OffsetY <= 0 {
		t.Fatalf("ArrowUp should increase OffsetY, got %v", g.cam.OffsetY)
	}
}

func TestBracketKeysZoom(t *testing.T) {
	g := newTestGameForUndo(t)
	g.cam.Scale = 2.0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyBracketRight: true}, nil)
	g.handleGlobalShortcuts()
	restore()
	if g.cam.Scale <= 2.0 {
		t.Fatalf("] should zoom in (increase scale), got %v", g.cam.Scale)
	}
	zoomedIn := g.cam.Scale

	restore = stubKeys(map[ebiten.Key]bool{ebiten.KeyBracketLeft: true}, nil)
	g.handleGlobalShortcuts()
	restore()
	if g.cam.Scale >= zoomedIn {
		t.Fatalf("[ should zoom out (decrease scale), got %v", g.cam.Scale)
	}
}

func TestZoomStaysInBounds(t *testing.T) {
	g := newTestGameForUndo(t)
	g.cam.Scale = 9.95
	for i := 0; i < 200; i++ {
		restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyBracketRight: true}, nil)
		g.handleGlobalShortcuts()
		restore()
	}
	if g.cam.Scale > 10.0 {
		t.Fatalf("zoom in must clamp at 10.0, got %v", g.cam.Scale)
	}
}

func TestZeroResetsZoom(t *testing.T) {
	g := newTestGameForUndo(t)
	g.cam.Scale = 0.5
	g.cam.OffsetX = 999
	g.cam.OffsetY = -999
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.Key0: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.cam.Scale != 2.0 || g.cam.OffsetX != 0 || g.cam.OffsetY != 0 {
		t.Fatalf("0 should reset camera: scale=%v off=(%v,%v)", g.cam.Scale, g.cam.OffsetX, g.cam.OffsetY)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestArrowKeysPan|TestBracketKeysZoom|TestZoomStaysInBounds|TestZeroResetsZoom' -v`
Expected: FAIL (camera unchanged — no pan/zoom logic yet).

- [ ] **Step 3: Implement pan/zoom/reset**

In `internal/ui/game_input_shortcuts.go`, add a `panStep` const and extend `handleGlobalShortcuts` (after the Space block, before the closing brace):

```go
// panStep is the per-frame pan distance (screen px) for arrow-key panning.
// Continuous (held) for smooth scrolling at 60fps.
const panStep = 24.0

// zoomKeyStep mirrors a single wheel notch's strength in zoomAtScreen.
const zoomKeyStep = 8.0
```

```go
	// Arrow keys pan the grid (continuous — held repeats every frame).
	// Panning the camera "toward" a direction moves content the opposite way,
	// matching drag-pan semantics in Camera.HandleMouse.
	panned := false
	if isKeyPressed(ebiten.KeyArrowLeft) {
		g.cam.OffsetX += panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowRight) {
		g.cam.OffsetX -= panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowUp) {
		g.cam.OffsetY += panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowDown) {
		g.cam.OffsetY -= panStep
		panned = true
	}
	if panned {
		g.cam.Snap()
	}

	// Zoom anchored at the grid-pane center (continuous w/ repeat).
	if isKeyPressed(ebiten.KeyBracketRight) || isKeyPressed(ebiten.KeyBracketLeft) {
		cx := float64(g.split.GridW(g.winW)) / 2
		cy := float64(gridTopOffset()) + float64(g.split.GridH(g.winH))/2
		steps := zoomKeyStep
		if isKeyPressed(ebiten.KeyBracketLeft) {
			steps = -zoomKeyStep
		}
		g.zoomAtScreen(cx, cy, steps)
	}

	// 0 resets zoom + recenters (edge-triggered).
	if isKeyJustPressed(ebiten.Key0) {
		g.cam.Reset()
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestArrowKeysPan|TestBracketKeysZoom|TestZoomStaysInBounds|TestZeroResetsZoom' -v`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

## Task 4: BPM nudge (`+`/`−`, Shift = ±5)

`+` is `Shift+=` on most layouts, so bind `ebiten.KeyEqual` for up and `ebiten.KeyMinus` for down; also accept numpad `+`/`-`. Shift makes the step 5. Edge-triggered (one nudge per press); auto-repeat is handled naturally by holding + re-pressing, and is fine to leave as edge for v1.

**Files:**
- Modify: `internal/ui/game_input_shortcuts.go`
- Test: `internal/ui/game_input_shortcuts_test.go`

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestEqualKeyRaisesBPM(t *testing.T) {
	g := newTestGameForUndo(t)
	start := g.drum.BPM()
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEqual: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.BPM() != start+1 {
		t.Fatalf("'+' should raise BPM by 1: %d -> %d", start, g.drum.BPM())
	}
}

func TestMinusKeyLowersBPM(t *testing.T) {
	g := newTestGameForUndo(t)
	start := g.drum.BPM()
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyMinus: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.BPM() != start-1 {
		t.Fatalf("'-' should lower BPM by 1: %d -> %d", start, g.drum.BPM())
	}
}

func TestShiftPlusRaisesBPMBy5(t *testing.T) {
	g := newTestGameForUndo(t)
	start := g.drum.BPM()
	restore := stubKeys(
		map[ebiten.Key]bool{ebiten.KeyShiftLeft: true},
		map[ebiten.Key]bool{ebiten.KeyEqual: true},
	)
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.BPM() != start+5 {
		t.Fatalf("Shift+'+' should raise BPM by 5: %d -> %d", start, g.drum.BPM())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEqualKeyRaisesBPM|TestMinusKeyLowersBPM|TestShiftPlusRaisesBPMBy5' -v`
Expected: FAIL (BPM unchanged).

- [ ] **Step 3: Implement BPM nudge**

In `handleGlobalShortcuts`, after the `0` reset block:

```go
	// BPM nudge: +/- by 1, Shift = 5. KeyEqual is the unshifted '+' key;
	// KeyNumpadAdd/Subtract cover the numpad. Edge-triggered.
	bpmStep := 1
	if isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight) {
		bpmStep = 5
	}
	if isKeyJustPressed(ebiten.KeyEqual) || isKeyJustPressed(ebiten.KeyNumpadAdd) {
		g.drum.SetBPM(g.drum.BPM() + bpmStep)
	}
	if isKeyJustPressed(ebiten.KeyMinus) || isKeyJustPressed(ebiten.KeyNumpadSubtract) {
		g.drum.SetBPM(g.drum.BPM() - bpmStep)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEqualKeyRaisesBPM|TestMinusKeyLowersBPM|TestShiftPlusRaisesBPMBy5' -v`
Expected: PASS.

- [ ] **Step 5: Verify SetBPM is seqMu-safe from this call site**

`handleGlobalShortcuts` runs before `seqMu.Lock()` (game_update.go:391). `dv.SetBPM` (drumview_transport.go:86) sets UI fields and fires `onStructuralMutation("bpm-change")`. Confirm `onStructuralMutation` does NOT acquire `g.seqMu` (grep it):

Run: `cd src/go && grep -rn "onStructuralMutation" internal/ui/*.go | grep -i "func\|seqMu"`
Expected: the callback re-derives paths / seeds timeline but does not lock `seqMu` synchronously from the UI thread. If it DOES, change the implementation to set a `g.pendingBPMDelta int` field here and apply it under `seqMu` later in Update. (Default expectation: direct call is safe, matching the BPM spinner which also runs on the UI thread.)

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

## Task 5: Audio panel tabs (`1`–`7`)

Keys `1`–`7` map to `PanelTab(0..6)` = `TabWave, TabSpectrum, TabMeters, TabEQ, TabScope, TabSynth, TabSampler` (enum definition order). Route through `(*EQPanelZone).SetActiveTab`, which already calls `EnsureAnalyzersForTab` (eq_panel_zone.go:426-438) — the mandatory dispatcher chokepoint.

**Files:**
- Modify: `internal/ui/game_input_shortcuts.go`
- Test: `internal/ui/game_input_shortcuts_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestNumberKeysSwitchTabs(t *testing.T) {
	cases := []struct {
		key ebiten.Key
		tab PanelTab
	}{
		{ebiten.Key1, TabWave},
		{ebiten.Key2, TabSpectrum},
		{ebiten.Key3, TabMeters},
		{ebiten.Key4, TabEQ},
		{ebiten.Key5, TabScope},
		{ebiten.Key6, TabSynth},
		{ebiten.Key7, TabSampler},
	}
	for _, c := range cases {
		g := newTestGameForUndo(t)
		restore := stubKeys(nil, map[ebiten.Key]bool{c.key: true})
		g.handleGlobalShortcuts()
		restore()
		if got := g.drum.eqPanelZone.tabState.ActiveTab(); got != c.tab {
			t.Fatalf("key %v: active tab = %v, want %v", c.key, got, c.tab)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNumberKeysSwitchTabs -v`
Expected: FAIL (tab unchanged for keys 1..7).

- [ ] **Step 3: Implement tab switching**

In `handleGlobalShortcuts`, after the BPM block:

```go
	// Number keys 1-7 select audio-panel tabs (enum definition order).
	// SetActiveTab routes through EnsureAnalyzersForTab — the dispatcher
	// chokepoint that enables the right analyzer before the next frame.
	if g.drum.eqPanelZone != nil {
		for k := ebiten.Key1; k <= ebiten.Key7; k++ {
			if isKeyJustPressed(k) {
				g.drum.eqPanelZone.SetActiveTab(PanelTab(int(k - ebiten.Key1)))
				break
			}
		}
	}
```

Note: `ebiten.Key1 .. ebiten.Key7` are consecutive in Ebiten's key enum, and `PanelTab(0..6)` matches the spec's Wave…Sampler order.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNumberKeysSwitchTabs -v`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave changes in the working tree.

---

## Task 6: Universal Esc ladder (`handleEscape`) + consolidate scattered Esc handlers

Replace the stub `handleEscape` with the full precedence ladder, and remove the now-duplicated Esc checks from `handleEditor` so there is exactly one Esc authority.

**Files:**
- Modify: `internal/ui/game_input_shortcuts.go`
- Modify: `internal/ui/game_input_editor.go:66-106`
- Test: `internal/ui/game_input_shortcuts_test.go`

- [ ] **Step 1: Write the failing tests (one per ladder rung + fallback)**

Append:

```go
func TestEscFallbackStopsPlayback(t *testing.T) {
	g := newTestGameForUndo(t)
	// Nothing pending → Esc should request Stop.
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if !g.drum.StopPressed() {
		t.Fatal("Esc with nothing pending should request Stop")
	}
}

func TestEscClosesPortalNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	// Open a portal overlay (use the help overlay as a stand-in for any menu).
	g.toggleShortcutsOverlay()
	if !g.drum.tree.Portal().IsOpen() {
		t.Fatal("setup: portal should be open")
	}
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.tree.Portal().IsOpen() {
		t.Fatal("Esc should close the open portal")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that closed a portal must NOT also Stop")
	}
}

func TestEscCancelsConnectModeNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.connectMode = true
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.connectMode {
		t.Fatal("Esc should cancel connect mode")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that cancelled connect mode must NOT also Stop")
	}
}

func TestEscCancelsMoveModeNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.moveMode = true
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.moveMode {
		t.Fatal("Esc should cancel move mode")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that cancelled move mode must NOT also Stop")
	}
}

func TestEscCancelsLinkDragNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.linkDrag.active = true
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.linkDrag.active {
		t.Fatal("Esc should cancel an in-progress link drag")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that cancelled link drag must NOT also Stop")
	}
}

func TestEscBlursTextInputNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.tree.SetFocus("transport")
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.tree.focusedZone != "" {
		t.Fatal("Esc should blur the focused text input")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that blurred a field must NOT also Stop")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEsc' -v`
Expected: FAIL — `g.toggleShortcutsOverlay undefined` and Esc ladder rungs not implemented. (Task 7 defines `toggleShortcutsOverlay`; if running strictly task-by-task, implement Step 3 here first, then the portal test passes once Task 7 lands. To keep Task 6 self-contained, temporarily comment out `TestEscClosesPortalNotStop` until Task 7, or do Task 7 Step 3 — the overlay file — before this step.)

- [ ] **Step 3: Implement the Esc ladder**

In `internal/ui/game_input_shortcuts.go`, replace the stub `handleEscape` with:

```go
// handleEscape is the SINGLE authority for the Esc key. It walks a precedence
// ladder and performs exactly ONE cancel action, then returns true. With
// nothing pending, the final rung requests Stop. This consolidates what used to
// be scattered Esc checks (sidebar/connect/move in handleEditor, portal in the
// tree) into one coherent order. Edge-triggered so a held Esc fires once.
func (g *Game) handleEscape() bool {
	if !isKeyJustPressed(ebiten.KeyEscape) {
		return false
	}

	// 1+2. Any open portal overlay (help panel, instrument menu, dialogs,
	// dropdowns, tooltips) — close the topmost.
	if g.drum.tree != nil && g.drum.tree.Portal() != nil && g.drum.tree.Portal().IsOpen() {
		g.drum.tree.Portal().CloseTop()
		return true
	}

	// 3. Node sidebar.
	if g.sidebar.IsOpen() {
		g.sidebar.Close()
		return true
	}

	// 4. Focused text input — revert + blur.
	if g.drum.tree != nil && g.drum.tree.focusedZone != "" {
		if g.drum.transportZone != nil {
			g.drum.transportZone.BlurInputs()
		}
		g.drum.tree.SetFocus("")
		return true
	}

	// 5. Connect mode.
	if g.connectMode {
		g.cancelConnectMode()
		return true
	}

	// 6. Move mode.
	if g.moveMode {
		g.cancelMoveMode()
		return true
	}

	// 7. In-progress link drag.
	if g.linkDrag.active {
		g.linkDrag = dragLink{}
		return true
	}

	// 8. In-flight tree capture (a drag handler holds capture).
	if g.drum.tree != nil && g.drum.tree.Capturing() {
		g.drum.tree.ClearCapture()
		return true
	}

	// 9. Active touch gesture (pinch/pan in flight).
	if globalTouchState.ActiveTouchCount() > 0 {
		globalTouchState.Reset()
		return true
	}

	// 10. Pending editor click state.
	if g.pendingClick || g.camDragged || g.pendingStartRow >= 0 {
		g.pendingClick = false
		g.camDragged = false
		g.pendingStartRow = -1
		return true
	}

	// 11. Fallback: nothing pending → Stop playback.
	g.drum.TriggerStop()
	return true
}
```

- [ ] **Step 4: Remove the migrated Esc checks from `handleEditor`**

In `internal/ui/game_input_editor.go`, delete the three Esc blocks now owned by `handleEscape`:

Remove lines 65-70 (sidebar Esc):
```go
	// ESC closes node sidebar
	if g.sidebar.IsOpen() && isKeyPressed(ebiten.KeyEscape) {
		g.sidebar.Close()
		g.leftPrev = left
		return
	}
```

Remove lines 72-77 (connect-mode Esc):
```go
	// ESC cancels connect mode
	if g.connectMode && isKeyPressed(ebiten.KeyEscape) {
		g.cancelConnectMode()
		g.leftPrev = left
		return
	}
```

Remove lines 101-106 (move-mode Esc), the block:
```go
		// ESC cancels move mode
		if isKeyPressed(ebiten.KeyEscape) {
			g.cancelMoveMode()
			g.leftPrev = left
			return
		}
```

Leave the rest of `handleEditor` (move-mode placement, confirm dialog, occupied-destination cancel) untouched — those are mouse-driven, not Esc.

- [ ] **Step 5: Run the Esc tests + the move/connect mode tests to verify no regression**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestEsc|ConnectMode|MoveMode' -v`
Expected: PASS. (If existing connect/move tests relied on `handleEditor` handling Esc, they continue to pass because `handleGlobalShortcuts` runs every frame in `Update` and is also exercised directly. If any existing test calls `handleEditor` in isolation and asserts Esc cancels, update it to call `g.handleEscape()` instead — note such changes in the checkpoint.)

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

## Task 7: `?` help overlay (ShortcutsOverlay)

A modal, scrimmed portal overlay listing the key map. Toggled by `?` (Shift+`/`) and the plain `/` key; closed by `?`, Esc (Task 6 rung 1), or by toggling again.

**Files:**
- Create: `internal/ui/shortcuts_overlay.go`
- Modify: `internal/ui/game_input_shortcuts.go`
- Test: `internal/ui/game_input_shortcuts_test.go`

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestSlashTogglesHelpOverlay(t *testing.T) {
	g := newTestGameForUndo(t)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySlash: true})
	g.handleGlobalShortcuts()
	restore()
	if !g.drum.tree.Portal().IsOpen() {
		t.Fatal("'?' should open the shortcuts overlay")
	}

	// Press again to close.
	restore = stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySlash: true})
	g.handleGlobalShortcuts()
	restore()
	if g.drum.tree.Portal().IsOpen() {
		t.Fatal("'?' again should close the shortcuts overlay")
	}
}

func TestToggleShortcutsOverlayDirect(t *testing.T) {
	g := newTestGameForUndo(t)
	g.toggleShortcutsOverlay()
	if !g.drum.tree.Portal().IsOpen() {
		t.Fatal("toggleShortcutsOverlay should open the portal")
	}
	g.toggleShortcutsOverlay()
	if g.drum.tree.Portal().IsOpen() {
		t.Fatal("toggleShortcutsOverlay should close the portal")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSlashTogglesHelpOverlay|TestToggleShortcutsOverlayDirect' -v`
Expected: FAIL — `g.toggleShortcutsOverlay undefined`.

- [ ] **Step 3: Create the overlay**

Create `internal/ui/shortcuts_overlay.go`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// shortcutRow is one line in the help overlay: a key glyph and its action.
type shortcutRow struct{ keys, action string }

var shortcutRows = []shortcutRow{
	{"Space", "Play / Pause"},
	{"Esc", "Cancel / close — or Stop"},
	{"Arrows", "Pan the grid"},
	{"[  ]", "Zoom out / in"},
	{"0", "Reset zoom"},
	{"+  -", "BPM  (Shift = 5)"},
	{"1 - 7", "Wave / Spectrum / Levels / EQ / Chain / Synth / Sampler"},
	{"?", "Show / hide this help"},
}

// ShortcutsOverlay is a modal portal panel listing the keyboard shortcuts.
// Presentational only (no hit areas); dismissed via the ? key, Esc, or toggling.
type ShortcutsOverlay struct {
	rect   image.Rectangle
	closed bool
}

func NewShortcutsOverlay() *ShortcutsOverlay { return &ShortcutsOverlay{} }

func (o *ShortcutsOverlay) Close() { o.closed = true }

func (o *ShortcutsOverlay) Layout(_, screenBounds image.Rectangle) {
	const w, lineH, padX, padY = 460, 26, 24, 20
	h := padY*2 + lineH*(len(shortcutRows)+1)
	cx := (screenBounds.Min.X + screenBounds.Max.X) / 2
	cy := (screenBounds.Min.Y + screenBounds.Max.Y) / 2
	o.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
}

func (o *ShortcutsOverlay) HitAreas() []HitArea { return nil }

func (o *ShortcutsOverlay) Draw(screen *ebiten.Image) {
	if o.rect.Empty() {
		return
	}
	const lineH, padX, padY = 26, 24, 20
	drawRoundedRect(screen, o.rect, colSurface2, RadiusSM, true)
	drawRoundedRect(screen, o.rect, colButtonBorder, RadiusSM, false)

	x := o.rect.Min.X + padX
	y := o.rect.Min.Y + padY
	DrawTextStyled(screen, "Keyboard shortcuts", x, y, RoleBody, colTextPrimary)
	y += lineH
	keyCol := o.rect.Min.X + padX
	actCol := o.rect.Min.X + padX + 110
	for _, r := range shortcutRows {
		DrawTextStyled(screen, r.keys, keyCol, y, RoleCaption, colTextPrimary)
		DrawTextStyled(screen, r.action, actCol, y, RoleCaption, colTextSecondary)
		y += lineH
	}
}

func (o *ShortcutsOverlay) ShouldClose() bool { return o.closed }
```

- [ ] **Step 4: Add the toggle + `?` binding**

In `internal/ui/game_input_shortcuts.go`, add the toggle helper and bind it.

Add a field reference via a method (no struct change needed — the portal owns the entry by ID):

```go
// shortcutsOverlayID identifies the help overlay portal entry.
const shortcutsOverlayID = "shortcuts"

// toggleShortcutsOverlay opens the keyboard-shortcuts help panel, or closes it
// if already open.
func (g *Game) toggleShortcutsOverlay() {
	if g.drum == nil || g.drum.tree == nil {
		return
	}
	p := g.drum.tree.Portal()
	if p == nil {
		return
	}
	if p.IsOpen() {
		// If our overlay is the open one, close it; otherwise close topmost.
		p.Close(shortcutsOverlayID)
		if !p.IsOpen() {
			return
		}
	}
	p.Open(PortalEntry{
		ID:      shortcutsOverlayID,
		Overlay: NewShortcutsOverlay(),
		Modal:   true,
		Scrim:   true,
	})
}
```

In `handleGlobalShortcuts`, after the tab-switch block, add:

```go
	// ? (Shift+/) or plain / toggles the keyboard-shortcuts help overlay.
	if isKeyJustPressed(ebiten.KeySlash) {
		g.toggleShortcutsOverlay()
	}
```

> Note: detecting the literal `?` character requires Shift+`/`; binding `ebiten.KeySlash` (with or without Shift) is the pragmatic, layout-robust choice and is what users press.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSlashTogglesHelpOverlay|TestToggleShortcutsOverlayDirect|TestEscClosesPortalNotStop' -v`
Expected: PASS (this also un-blocks the Task 6 portal test).

- [ ] **Step 6: Checkpoint** — leave changes in the working tree.

---

## Task 8: Desktop discoverability — transport-bar "?" help chip

A small button in the transport bar that toggles the same overlay, shown desktop-only. This gives mouse users an entry point.

**Files:**
- Modify: `internal/ui/transport_zone.go`
- Modify: `internal/ui/drumview_ctor.go` (wire the chip's OnClick to `toggleShortcutsOverlay`)
- Test: `internal/ui/game_input_shortcuts_test.go`

- [ ] **Step 1: Investigate the transport zone button wiring**

Read how an existing transport button (e.g. the Stop button) is constructed in `initButtons` (transport_zone.go:165), laid out in `Layout` (line 402), drawn in `Draw` (line 514), and how its `HitAreas` (line 496) are published. Mirror that exact pattern for a new `helpBtn`. Also read `TransportCallbacks` (line 17) to add an `OnHelp func()` callback, and find where callbacks are populated for the transport zone (search `TransportCallbacks{` in `drumview_ctor.go`).

Run:
```bash
cd src/go && grep -n "stopBtn\|TabBtn\|recordBtn\|OnStop" internal/ui/transport_zone.go | head -30
cd src/go && grep -rn "TransportCallbacks{" internal/ui/*.go
```

- [ ] **Step 2: Write the failing test**

Append to `internal/ui/game_input_shortcuts_test.go`:

```go
func TestHelpChipTogglesOverlay(t *testing.T) {
	g := newTestGameForUndo(t)
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone")
	}
	if tz.callbacks.OnHelp == nil {
		t.Fatal("transport zone should expose an OnHelp callback")
	}
	tz.callbacks.OnHelp()
	if !g.drum.tree.Portal().IsOpen() {
		t.Fatal("OnHelp should open the shortcuts overlay")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestHelpChipTogglesOverlay -v`
Expected: FAIL — `OnHelp` undefined on `TransportCallbacks`.

- [ ] **Step 4: Add the OnHelp callback + help button**

1. In `transport_zone.go`, add to the `TransportCallbacks` struct (near line 17):
```go
	OnHelp          func()            // help "?" chip pressed
```

2. Add a `helpBtn *Button` field to `TransportZone` (near the other button fields ~line 99). In `initButtons` (line 165), construct it mirroring the Stop button, with a `?` text label (single-char text labels are permitted per DESIGN.md — `?` is allowed; if not, use an `IconID` for help). Bind its press:
```go
	if z.helpBtn.OnPress(mx, my) == InputConsumed {
		if z.callbacks.OnHelp != nil {
			z.callbacks.OnHelp()
		}
	}
```
3. In `Layout` (line 402), position `helpBtn` at the right edge of the bar **only on desktop** (`if !Profile().IsMobile()`); on mobile, give it a zero rect so it isn't drawn/hit.
4. In `Draw` (line 514) and `HitAreas` (line 496), include `helpBtn` guarded by the same `!Profile().IsMobile()` condition.

> Follow the established button construction/layout/draw/hit pattern exactly; do not roll bespoke chrome (shared-widget-reuse rule).

- [ ] **Step 5: Wire the callback in the ctor**

In `drumview_ctor.go`, where `TransportCallbacks{...}` is populated (found in Step 1), add:
```go
		OnHelp: func() {
			if dv.onHelp != nil {
				dv.onHelp()
			}
		},
```
If a `dv.onHelp` hook does not exist, instead wire it directly to the Game via the existing pattern other transport callbacks use to reach `Game` (e.g. they call `dv`-level methods). The simplest correct wiring: have `Game` set `dv`'s transport `OnHelp` to `g.toggleShortcutsOverlay` after construction — search for where `g.drum` callbacks are finalized in `game_new.go` and add `g.drum.transportZone.callbacks.OnHelp = g.toggleShortcutsOverlay` there. Use whichever matches the existing OnStop/OnPlayToggle wiring style.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestHelpChipTogglesOverlay -v`
Expected: PASS.

- [ ] **Step 7: Checkpoint** — leave changes in the working tree.

---

## Task 9: Full-suite verification + token/design guards

**Files:** none (verification only).

- [ ] **Step 1: Run the full keyboard test file**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestCameraReset|TestTrigger|TestSpace|TestActionKeys|TestArrow|TestBracket|TestZoom|TestZero|TestEqual|TestMinus|TestShiftPlus|TestNumberKeys|TestEsc|TestSlash|TestToggleShortcuts|TestHelpChip' -v`
Expected: ALL PASS.

- [ ] **Step 2: Run the broader UI suite to catch regressions from the Esc consolidation**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -40`
Expected: No NEW failures introduced by this work. **Cross-check against the branch baseline** — per memory `project_add_core_node_types_preexisting_failures`, this branch carries pre-existing failures (concurrent Synth-tab redesign, full-suite timeout). Before blaming your diff, run the same command on the pre-change tree (re-edit your additions out manually if needed) OR diff the failing test names against the known pre-existing set. Only investigate failures that are NEW.

- [ ] **Step 3: Run the design-token guards (the new overlay uses tokens, not literals)**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|TestNoBareRadiusLiterals' ./internal/ui/
```
Expected: PASS. `shortcuts_overlay.go` uses only `colSurface2`/`colButtonBorder`/`colTextPrimary`/`colTextSecondary`/`RadiusSM` — no inline `color.RGBA{}` literals and no bare radius literals, so neither ratchet trips.

- [ ] **Step 4: Confirm no new `runtime.GOOS == "js"` branch was added**

Run: `cd src/go && grep -rn "GOOS\|GOARCH" internal/ui/game_input_shortcuts.go internal/ui/shortcuts_overlay.go`
Expected: no matches. Keyboard handling is uniform across desktop/WASM via Ebiten; no platform branch needed.

- [ ] **Step 5: Final checkpoint** — all changes remain in the working tree, uncommitted. Report the test results to the user.

---

## Self-review notes (author)

- **Spec coverage:** Space play/pause (T2), Esc-stop + universal Esc ladder (T6), arrows-pan / `[`/`]`-zoom / `0`-reset (T3), `+`/`−` BPM with Shift (T4), `1`–`7` tabs via dispatcher (T5), `?` overlay (T7), desktop discoverability chip (T8), text-input gate (T2), functional tests driving the production seam (all), no-GOOS-branch + token guards (T9). All spec sections mapped.
- **Type consistency:** `handleGlobalShortcuts`, `handleEscape`, `toggleShortcutsOverlay`, `TriggerPlayPause`/`TriggerStop`, `Camera.Reset`, `ShortcutsOverlay`, `shortcutsOverlayID` are referenced consistently across tasks. `PanelTab(int(k - ebiten.Key1))` relies on enum order documented in T5.
- **Known risk:** the Esc consolidation (T6) may require touching a small number of existing tests that called `handleEditor` for Esc — T6 Step 5 flags this and says to repoint them at `handleEscape`. The BPM seqMu-safety check (T4 Step 5) has an explicit fallback (deferred delta) if the direct call turns out to lock.
- **DO NOT COMMIT** rule is honored — every task ends in a non-git "Checkpoint".
