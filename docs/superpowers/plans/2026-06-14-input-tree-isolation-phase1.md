# Input Tree Isolation — Phase 1: Keyboard-Ownership Contract — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the ad-hoc denylist that makes grid keyboard shortcuts "back off" with a single ownership predicate — every keyboard-owning surface declares it owns the keyboard, and the grid keyboard handler yields whenever anything claims it. This turns the three Phase-0 failing tests green and removes the class of bug entirely (new text inputs are covered by construction).

**Architecture:** Introduce a `KeyboardClaimant` interface (`ClaimsKeyboard() bool`). The portal-hosted modal components (rename, instrument menu) and the inline Save As dialog implement it; the `compPortalOverlay` adapter forwards it (exactly like it already forwards `HandleEscape`); `OverlayPortal` and `DrumViewTree` expose `OwnsKeyboard()`; `DrumView.KeyboardClaimed()` aggregates both trees + value editors + the Save As dialog + the WAV-naming box. `handleGlobalShortcuts`'s gate becomes `if g.drum.KeyboardClaimed() { return }`.

**Tech Stack:** Go 1.23, Ebiten v2, project test harness (`-tags test -modfile=go.test.mod`).

**Repo policy (IMPORTANT):** Do **NOT** `git add`, stage, or commit in this repo — leave all changes in the working tree; the user owns commits. Every task below ends with a verification run, **not** a commit.

**Bundled Go:** always use `../../.tools/go/bin/go` from `src/go`.

**Fast test command (run from `src/go`):**
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run '<Pattern>' ./internal/ui/
```
Logs are noisy; append `2>&1 | grep -E '^(=== RUN|--- (FAIL|PASS)|PASS|FAIL|ok|.*\.go:[0-9]+:)' | grep -v DEBUG` to isolate results.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `src/go/internal/ui/keyboard_focus.go` | The contract: `KeyboardClaimant` interface, `DrumView.KeyboardClaimed()`, `DrumViewTree.OwnsKeyboard()`, `OverlayPortal.ClaimsKeyboard()` | **Create** |
| `src/go/internal/ui/comp_portal_overlay.go` | Forward `ClaimsKeyboard()` to the wrapped component (mirror of `HandleEscape`) | Modify (+1 method) |
| `src/go/internal/ui/drumview_overlay_rename_comp.go` | `RenameComponent.ClaimsKeyboard()` | Modify (+1 method) |
| `src/go/internal/ui/drumview_overlay_inst_comp.go` | `InstrumentMenuComponent.ClaimsKeyboard()` | Modify (+1 method) |
| `src/go/internal/ui/synth_panel_save_as_dialog.go` | `synthSaveAsDialog.ClaimsKeyboard()` | Modify (+1 method) |
| `src/go/internal/ui/game_input_shortcuts.go` | Replace the denylist gate (lines 123-127) with `KeyboardClaimed()` | Modify |
| `src/go/internal/ui/keyboard_focus_test.go` | Unit + contract tests for the new predicates | **Create** |
| `src/go/internal/ui/arrow_key_text_input_isolation_test.go` | Existing Phase-0 RED tests + new per-surface gate tests | Modify (append) |
| `src/go/internal/ui/keyboard_focus_discipline_test.go` | Source-scan guard: the gate is the single predicate | **Create** |

---

## Task 1: `ClaimsKeyboard()` on the three components

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_rename_comp.go` (add method near `IsOpen` at line 120)
- Modify: `src/go/internal/ui/drumview_overlay_inst_comp.go` (add method near `IsOpen` at line 292)
- Modify: `src/go/internal/ui/synth_panel_save_as_dialog.go` (add method near `Focused` at line 175)
- Test: `src/go/internal/ui/keyboard_focus_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/keyboard_focus_test.go`:

```go
package ui

import (
	"image"
	"testing"
)

// A focused Save As dialog claims the keyboard; an empty one does not.
func TestClaimsKeyboard_SaveAsDialog(t *testing.T) {
	var nilDlg *synthSaveAsDialog
	if nilDlg.ClaimsKeyboard() {
		t.Fatal("nil dialog must not claim the keyboard")
	}
	ti := NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	ti.SetText("kick (saved)")
	ti.SetFocus(true)
	dlg := &synthSaveAsDialog{textInput: ti}
	if !dlg.ClaimsKeyboard() {
		t.Fatal("a focused Save As dialog must claim the keyboard")
	}
	ti.SetFocus(false)
	if dlg.ClaimsKeyboard() {
		t.Fatal("an unfocused Save As dialog must not claim the keyboard")
	}
}

// An open rename component claims the keyboard; a closed one does not.
func TestClaimsKeyboard_Rename(t *testing.T) {
	r := &RenameComponent{}
	if r.ClaimsKeyboard() {
		t.Fatal("a closed rename component must not claim the keyboard")
	}
	r.SetProps(RenameProps{InitialText: "kick", MaxLen: 32, AnchorRect: image.Rect(0, 0, 120, 24)})
	r.Open()
	if !r.IsOpen() {
		t.Skip("rename did not open in this environment")
	}
	if !r.ClaimsKeyboard() {
		t.Fatal("an open rename component must claim the keyboard")
	}
}

// An open instrument menu claims the keyboard; a closed one does not.
func TestClaimsKeyboard_InstrumentMenu(t *testing.T) {
	g := newTestGameForUndo(t)
	m := g.drum.instMenuComp
	if m == nil {
		t.Skip("no instrument menu component")
	}
	if m.ClaimsKeyboard() {
		t.Fatal("a closed instrument menu must not claim the keyboard")
	}
	m.Open()
	if !m.ClaimsKeyboard() {
		t.Fatal("an open instrument menu must claim the keyboard")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestClaimsKeyboard_' ./internal/ui/ 2>&1 | grep -v DEBUG | tail -20
```
Expected: **build failure** — `r.ClaimsKeyboard undefined`, `dlg.ClaimsKeyboard undefined`, `m.ClaimsKeyboard undefined`.

- [ ] **Step 3: Add the three methods**

In `drumview_overlay_rename_comp.go`, immediately after the `IsOpen()` method (ends ~line 126):

```go
// ClaimsKeyboard reports whether this rename field currently owns the keyboard.
// While open, all keys (typed name, caret moves) belong to the rename box, so
// the grid must not act on them. Part of the keyboard-ownership contract
// (keyboard_focus.go).
func (r *RenameComponent) ClaimsKeyboard() bool {
	return r != nil && r.IsOpen()
}
```

In `drumview_overlay_inst_comp.go`, immediately after the `IsOpen()` method (ends ~line 294):

```go
// ClaimsKeyboard reports whether the instrument menu currently owns the
// keyboard. While open, the search box (and pagination jump field) consume
// typed/caret keys, so the grid must not act on them. Part of the
// keyboard-ownership contract (keyboard_focus.go).
func (m *InstrumentMenuComponent) ClaimsKeyboard() bool {
	return m != nil && m.IsOpen()
}
```

In `synth_panel_save_as_dialog.go`, immediately after the `Focused()` method (ends ~line 180):

```go
// ClaimsKeyboard reports whether the Save As dialog currently owns the
// keyboard. While its text input is focused, caret/typed keys belong to it, so
// the grid must not act on them. Part of the keyboard-ownership contract
// (keyboard_focus.go).
func (d *synthSaveAsDialog) ClaimsKeyboard() bool {
	return d != nil && d.textInput != nil && d.textInput.Focused()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestClaimsKeyboard_' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS)|ok|FAIL)' | grep -v DEBUG
```
Expected: PASS for all three (`TestClaimsKeyboard_Rename` may log `SKIP` if rename can't open headless — acceptable).

- [ ] **Step 5: Verify (no commit — repo policy)**

Confirm working tree holds the new test file + three method additions. Do NOT `git add`/commit.

---

## Task 2: `KeyboardClaimant` interface + tree/portal ownership plumbing

**Files:**
- Create: `src/go/internal/ui/keyboard_focus.go`
- Modify: `src/go/internal/ui/comp_portal_overlay.go` (add `ClaimsKeyboard` forwarder)
- Test: `src/go/internal/ui/keyboard_focus_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/keyboard_focus_test.go`:

```go
// An open rename portal makes its owning tree report OwnsKeyboard()==true.
func TestTreeOwnsKeyboard_RenamePortal(t *testing.T) {
	g := newTestGameForUndo(t)
	rc := g.drum.renameComp
	if rc == nil {
		t.Skip("no rename component")
	}
	if g.drum.tree.OwnsKeyboard() {
		t.Fatal("tree must not own the keyboard at rest")
	}
	rc.SetProps(RenameProps{InitialText: "kick", MaxLen: 32})
	rc.Open()
	g.drum.openRenamePortal()
	if !g.drum.tree.Portal().IsOpen() {
		t.Skip("rename portal did not open")
	}
	if !g.drum.tree.OwnsKeyboard() {
		t.Fatal("tree must own the keyboard while a rename portal is open")
	}
}

// focusedZone alone makes a tree own the keyboard (existing transport/EQ path).
func TestTreeOwnsKeyboard_FocusedZone(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.drum.tree.OwnsKeyboard() {
		t.Fatal("tree must not own the keyboard at rest")
	}
	g.drum.tree.SetFocus("transport")
	if !g.drum.tree.OwnsKeyboard() {
		t.Fatal("a focused zone must make the tree own the keyboard")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestTreeOwnsKeyboard_' ./internal/ui/ 2>&1 | grep -v DEBUG | tail -20
```
Expected: build failure — `g.drum.tree.OwnsKeyboard undefined`.

- [ ] **Step 3: Create the contract file and the portal forwarder**

Create `src/go/internal/ui/keyboard_focus.go`:

```go
package ui

// keyboard_focus.go — the keyboard-ownership contract.
//
// Keyboard input is not spatial, so "who gets the keys?" cannot be answered by
// the HitIndex. Instead, any surface that accepts typed/caret keys *claims* the
// keyboard while it is active. Grid-level keyboard shortcuts (arrow-pan, zoom,
// Space, BPM±, tab-select, "?") run only when nothing claims the keyboard —
// see handleGlobalShortcuts. This replaces the previous denylist that had to
// enumerate every text-input source (and missed several).

// KeyboardClaimant is implemented by anything that owns the keyboard while it is
// open/focused: portal-hosted modal components (rename, instrument menu) and the
// inline Save As dialog. Implementations MUST be nil-safe on the receiver.
type KeyboardClaimant interface {
	ClaimsKeyboard() bool
}

// OwnsKeyboard reports whether this tree (a focused zone, or a portal overlay
// that claims the keyboard) currently owns keyboard input.
func (t *DrumViewTree) OwnsKeyboard() bool {
	if t == nil {
		return false
	}
	if t.focusedZone != "" {
		return true
	}
	if t.portal != nil && t.portal.ClaimsKeyboard() {
		return true
	}
	return false
}

// ClaimsKeyboard reports whether this portal's top overlay claims the keyboard.
func (p *OverlayPortal) ClaimsKeyboard() bool {
	if p == nil {
		return false
	}
	if c, ok := p.TopOverlay().(KeyboardClaimant); ok {
		return c.ClaimsKeyboard()
	}
	return false
}

// KeyboardClaimed reports whether ANY bottom-panel surface currently owns the
// keyboard: either subtree (focused zone or claiming portal overlay), the shared
// inline numeric editor (transport BPM / EQ dB / synth knob), the Save As
// dialog, or the WAV-naming box. The grid keyboard handler yields when true.
func (dv *DrumView) KeyboardClaimed() bool {
	if dv == nil {
		return false
	}
	if dv.tree != nil && dv.tree.OwnsKeyboard() {
		return true
	}
	if dv.audioTree != nil && dv.audioTree.OwnsKeyboard() {
		return true
	}
	if dv.valueEditorActive() {
		return true
	}
	if dv.saveAsDialog != nil && dv.saveAsDialog.ClaimsKeyboard() {
		return true
	}
	if dv.nameBox != nil && dv.nameBox.Focused() {
		return true
	}
	return false
}
```

In `comp_portal_overlay.go`, add this method immediately after `HandleEscape` (ends line 67):

```go
// ClaimsKeyboard forwards to the wrapped component if it owns the keyboard while
// open (rename, instrument menu). Mirrors HandleEscape forwarding. Part of the
// keyboard-ownership contract (keyboard_focus.go).
func (o *compPortalOverlay) ClaimsKeyboard() bool {
	if c, ok := o.comp.(KeyboardClaimant); ok {
		return c.ClaimsKeyboard()
	}
	return false
}
```

> Note: `DrumViewTree.Portal()` already exists (used by `handleEscape`); it returns `t.portal`. If a getter is needed in the test and is absent, confirm via `grep -n "func (t \*DrumViewTree) Portal" drumview_tree.go` — it is present.

- [ ] **Step 4: Run test to verify it passes**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestTreeOwnsKeyboard_|TestClaimsKeyboard_' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS)|ok|FAIL)' | grep -v DEBUG
```
Expected: all PASS (rename-portal test may SKIP headless).

- [ ] **Step 5: Verify (no commit)**

Confirm the new file + forwarder compile and tests pass. Do NOT commit.

---

## Task 3: `KeyboardClaimed()` aggregator contract test

**Files:**
- Test: `src/go/internal/ui/keyboard_focus_test.go` (append)

This task adds no production code — it pins `KeyboardClaimed()` behaviour for every surface so future surfaces must be added to the predicate, not to a denylist.

- [ ] **Step 1: Write the test**

Append to `src/go/internal/ui/keyboard_focus_test.go`:

```go
// KeyboardClaimed is false at rest and true for each text-input surface.
func TestKeyboardClaimed_AllSurfaces(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.drum.KeyboardClaimed() {
		t.Fatal("KeyboardClaimed must be false at rest")
	}

	// 1. Save As dialog.
	ti := NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	ti.SetFocus(true)
	g.drum.saveAsDialog = &synthSaveAsDialog{textInput: ti}
	if !g.drum.KeyboardClaimed() {
		t.Fatal("Save As dialog must claim the keyboard")
	}
	g.drum.saveAsDialog = nil

	// 2. WAV-naming box.
	g.drum.nameBox = NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	g.drum.nameBox.SetFocus(true)
	if !g.drum.KeyboardClaimed() {
		t.Fatal("WAV-naming box must claim the keyboard")
	}
	g.drum.nameBox = nil

	// 3. Focused transport zone (BPM editing).
	g.drum.tree.SetFocus("transport")
	if !g.drum.KeyboardClaimed() {
		t.Fatal("focused transport zone must claim the keyboard")
	}
	g.drum.tree.SetFocus("")

	if g.drum.KeyboardClaimed() {
		t.Fatal("KeyboardClaimed must return to false once all surfaces cleared")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestKeyboardClaimed_AllSurfaces' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS)|ok|FAIL)' | grep -v DEBUG
```
Expected: PASS. (This passes immediately because Task 2 already implemented `KeyboardClaimed`; it is a regression lock, not RED-first. That is acceptable for a pure contract-pinning test.)

- [ ] **Step 3: Verify (no commit)**

---

## Task 4: Replace the denylist gate — turn Phase-0 RED green

**Files:**
- Modify: `src/go/internal/ui/game_input_shortcuts.go:123-127`
- Test: `src/go/internal/ui/arrow_key_text_input_isolation_test.go` (append per-surface tests)

- [ ] **Step 1: Add per-surface RED tests**

Append to `src/go/internal/ui/arrow_key_text_input_isolation_test.go`:

```go
// Arrows must not pan while the instrument menu (search box) is open.
func TestArrowKeysYieldToInstrumentMenu(t *testing.T) {
	g := newTestGameForUndo(t)
	m := g.drum.instMenuComp
	if m == nil {
		t.Skip("no instrument menu component")
	}
	m.Open()
	g.drum.openInstMenuPortal()
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowLeft must NOT pan the grid while the instrument menu is open (OffsetX=%v)", g.cam.OffsetX)
	}
}

// Arrows must not pan while the WAV-naming box is focused.
func TestArrowKeysYieldToNamingBox(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.nameBox = NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	g.drum.nameBox.SetText("loop")
	g.drum.nameBox.SetFocus(true)
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowRight: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowRight must NOT pan the grid while the WAV-naming box is focused (OffsetX=%v)", g.cam.OffsetX)
	}
}
```

- [ ] **Step 2: Run the full isolation suite — confirm the leak**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestArrowKeysYieldTo' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS)|ok|FAIL|.*\.go:[0-9]+:)' | grep -v DEBUG
```
Expected: `TestArrowKeysYieldToSaveAsDialog`, `...Right`, `...RenameTextInput`, `...InstrumentMenu`, `...NamingBox` **FAIL** (panned ±24). `...SynthValueEditor` / `...EQValueEditor` PASS (already gated).

- [ ] **Step 3: Replace the gate**

In `game_input_shortcuts.go`, replace the block at lines 118-127:

```go
	// A focused text input owns the keyboard this frame. The focusedZone gates
	// cover BPM (transport) and any tree-routed field; valueEditorActive() also
	// covers the shared inline numeric editor (synth/sampler + EQ dB) directly,
	// so Left/Right/Enter/Esc reach its caret instead of panning the grid — and
	// without depending on autoFocusPass having run first this frame.
	if (g.drum.tree != nil && g.drum.tree.focusedZone != "") ||
		(g.drum.audioTree != nil && g.drum.audioTree.focusedZone != "") ||
		g.drum.valueEditorActive() {
		return
	}
```

with:

```go
	// A surface that accepts typed/caret keys owns the keyboard this frame, so
	// the grid yields entirely (arrows reach the caret, not the camera). This is
	// the single keyboard-ownership predicate — every text-input surface declares
	// itself via KeyboardClaimant (keyboard_focus.go). Do NOT reintroduce an
	// inline denylist here; add new surfaces to KeyboardClaimed instead.
	if g.drum.KeyboardClaimed() {
		return
	}
```

- [ ] **Step 4: Run the isolation suite — confirm all green**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestArrowKeysYieldTo' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS)|ok|FAIL)' | grep -v DEBUG
```
Expected: all PASS (any may SKIP only if its component can't open headless).

- [ ] **Step 5: Run the broader keyboard-shortcut suite — no regressions**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestArrowKeysPan|TestActionKeysIgnoredWhileTextInputFocused|TestSpaceTogglesPlayPulse|TestNumberKeysSwitchTabs|TestSlashIgnoredWhileTextInputFocused|TestEsc' ./internal/ui/ 2>&1 | grep -E '^(--- FAIL|ok|FAIL)' | grep -v DEBUG
```
Expected: no `--- FAIL` lines; package `ok`. (`TestArrowKeysPan` still passes — at rest nothing claims the keyboard.)

- [ ] **Step 6: Verify (no commit)**

---

## Task 5: Discipline guard — the gate is the single predicate

**Files:**
- Create: `src/go/internal/ui/keyboard_focus_discipline_test.go`

- [ ] **Step 1: Write the discipline test**

Create `src/go/internal/ui/keyboard_focus_discipline_test.go`:

```go
package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The grid keyboard gate must be the single KeyboardClaimed() predicate. This
// guards against a denylist creeping back into handleGlobalShortcuts (the
// original bug: an inline `focusedZone != "" || valueEditorActive()` check that
// had to enumerate every text input and missed several).
func TestGridKeyboardGateUsesSinglePredicate(t *testing.T) {
	src, err := os.ReadFile("game_input_shortcuts.go")
	if err != nil {
		t.Fatalf("read game_input_shortcuts.go: %v", err)
	}
	body := extractFuncBody(t, string(src), "handleGlobalShortcuts")

	if !strings.Contains(body, "KeyboardClaimed()") {
		t.Fatal("handleGlobalShortcuts must gate grid shortcuts via g.drum.KeyboardClaimed()")
	}
	if strings.Contains(body, "focusedZone") {
		t.Fatal("handleGlobalShortcuts must NOT inspect focusedZone directly — route ownership through KeyboardClaimed()")
	}
	if strings.Contains(body, "valueEditorActive()") {
		t.Fatal("handleGlobalShortcuts must NOT inspect valueEditorActive() directly — it belongs inside KeyboardClaimed()")
	}
}

// extractFuncBody returns the source text of the named top-level method/func
// body (between its first '{' and the matching '}').
func extractFuncBody(t *testing.T, src, name string) string {
	t.Helper()
	re := regexp.MustCompile(`func (\([^)]*\) )?` + regexp.QuoteMeta(name) + `\(`)
	loc := re.FindStringIndex(src)
	if loc == nil {
		t.Fatalf("func %s not found", name)
	}
	open := strings.IndexByte(src[loc[1]:], '{')
	if open < 0 {
		t.Fatalf("no opening brace for %s", name)
	}
	start := loc[1] + open
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	t.Fatalf("unbalanced braces for %s", name)
	return ""
}
```

- [ ] **Step 2: Run test to verify it passes**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestGridKeyboardGateUsesSinglePredicate' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS)|ok|FAIL)' | grep -v DEBUG
```
Expected: PASS (Task 4 already replaced the gate). To confirm the guard actually bites, temporarily re-add `focusedZone` to the gate, re-run (expect FAIL), then revert.

- [ ] **Step 3: Verify (no commit)**

---

## Task 6: Functional test through the real `Game.Update` loop

**Files:**
- Modify: `src/go/internal/ui/arrow_key_text_input_isolation_test.go` (append a `//go:build test` functional test in a new file to avoid mixing build tags)
- Create: `src/go/internal/ui/arrow_key_save_as_functional_test.go`

The gate-level tests above call `handleGlobalShortcuts()` directly. This adds one end-to-end test that drives the real input loop (per the project's functional-tests-over-isolation guidance), proving the Save As caret moves while the grid stays put.

- [ ] **Step 1: Write the functional test**

Create `src/go/internal/ui/arrow_key_save_as_functional_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Open the Save As dialog on the Synth tab and drive Left/Right through the real
// Game.Update loop: the caret moves and the grid must NOT pan.
func TestFunctional_ArrowKeys_SaveAsDialog(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	g.Update()

	g.drum.openSaveAsDialog()
	if g.drum.saveAsDialog == nil || !g.drum.saveAsDialog.Focused() {
		t.Skip("Save As dialog did not open/focus in this environment")
	}

	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	// One settling frame so the dialog's TextInput caret is initialised.
	fi.frame(t, g)

	startCursor := g.drum.saveAsDialog.textInput.cursor
	if startCursor == 0 {
		t.Skip("precondition: prefilled caret should sit at end (>0)")
	}
	camBefore := g.cam.OffsetX

	fi.pressKey(t, g, ebiten.KeyLeft)

	if g.cam.OffsetX != camBefore {
		t.Errorf("Left arrow panned the grid while the Save As dialog was open (%v -> %v)", camBefore, g.cam.OffsetX)
	}
	if g.drum.saveAsDialog == nil {
		t.Fatalf("Left arrow unexpectedly closed the Save As dialog")
	}
	if g.drum.saveAsDialog.textInput.cursor != startCursor-1 {
		t.Fatalf("Left arrow did NOT move the dialog caret: cursor=%d want %d", g.drum.saveAsDialog.textInput.cursor, startCursor-1)
	}
}
```

> If `textInput.cursor` is unexported and unreadable from this test file (same package `ui`, so it is readable), no accessor is needed. `synthSaveAsDialog.textInput` and `TextInput.cursor` are both package-private and reachable from `package ui` tests.

- [ ] **Step 2: Run test to verify it passes**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestFunctional_ArrowKeys_SaveAsDialog' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS|SKIP)|ok|FAIL)' | grep -v DEBUG
```
Expected: PASS (or SKIP if the dialog cannot open headless — the gate-level Save As tests in Task 4 already cover the behaviour deterministically).

- [ ] **Step 3: Verify (no commit)**

---

## Task 7: Full `internal/ui` suite — no regressions

- [ ] **Step 1: Run the whole package**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | grep -E '^(--- FAIL|ok|FAIL|panic)' | grep -v DEBUG | tail -40
```
Expected: package `ok`. If there are FAILs, isolate each with `-run '<Name>'` and confirm whether it is pre-existing on this branch (the memory notes several flaky/pre-existing failures): re-run the same test on a clean checkout of the file set you did not touch. Only failures involving keyboard/shortcuts/`KeyboardClaimed`/arrow tests are yours to fix.

- [ ] **Step 2: Run the `vet` build guard (branch has a phantom `./...` package — use the explicit path)**

Run from `src/go`:
```bash
../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | grep -v DEBUG | tail -20
```
Expected: clean (no build errors). Ignore stale gopls "undefined" flapping — trust this command.

- [ ] **Step 3: Report**

Summarise: Phase-0 tests now green, per-surface tests green, discipline guard green, full package status (and any pre-existing failures explicitly identified). Leave everything in the working tree for the user to commit.

---

## Self-Review (completed by plan author)

- **Spec coverage:** Phase-1 row of the umbrella spec (`KeyboardClaimant`, `OwnsKeyboard()`, `KeyboardClaimed()`, denylist removal, Phase-0 green) → Tasks 1-5. Functional-test guidance → Task 6. Discipline lock for this phase → Task 5. Deferred items (key delivery / `TextInput` global reads) correctly excluded — they are Phase 5.
- **Placeholder scan:** none — every step has exact code, exact paths, exact commands, expected output.
- **Type consistency:** `ClaimsKeyboard()` (components + `compPortalOverlay`), `OverlayPortal.ClaimsKeyboard()`, `DrumViewTree.OwnsKeyboard()`, `DrumView.KeyboardClaimed()`, `KeyboardClaimant` interface — names identical across all tasks. `Portal()`, `IsOpen()`, `openRenamePortal()`, `openInstMenuPortal()`, `openSaveAsDialog()`, `Focused()`, `SetFocus()` confirmed against current source line numbers.
- **Repo policy:** every task ends in verification, never a commit.
```
