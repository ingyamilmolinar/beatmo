# Portal Dead Code & Legacy Pattern Cleanup

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove dead click-outside code from context menu and overflow menu handlers, clean up stale `fxPanelOpenSeq` references, and fix tests that bypass the portal system by setting private fields directly.

**Architecture:** The portal/tree system handles click-outside closing for all overlays via `DrumViewTree.handleInput()` (lines 228-249 in `drumview_tree.go`). Individual handler functions like `handleContextMenuInput()` and `handleOverflowMenuInput()` should NOT contain their own click-outside blocks — the tree closes the portal before the handler is ever called for outside clicks. Tests that call these handlers directly with outside coords are testing dead production code and should be updated to test through the tree or removed.

**Tech Stack:** Go 1.23, Ebiten (game UI framework), custom portal/tree input system

---

### Task 1: Remove dead click-outside from `handleContextMenuInput`

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go:395-414`

The portal `rectFn` returns `dv.contextMenuRect` (portal_open.go:190), so the tree only dispatches to the handler for clicks inside that rect. The `if left && !pt.In(dv.contextMenuRect)` check (line 402) is unreachable in production — the tree's click-outside (tree.go:245-249) closes the portal before this handler runs.

- [ ] **Step 1: Remove the dead click-outside block**

In `src/go/internal/ui/drumview_context_menu.go`, replace lines 395-414:

```go
	// Click outside closes — but NOT during the suppression window set by
	// openContextMenu(). Without this guard, the touch that opens the menu
	// (via lbl.OnClick in drum.Update, which runs after the InputDispatcher)
	// causes the dispatcher to fire "click outside" on the very next frame
	// while the touch is still held, closing the menu immediately and
	// producing a 1-frame flicker. DeferredTap.Begin already honors this
	// guard; the click-outside path must too.
	if left && !pt.In(dv.contextMenuRect) {
		dv.closeContextMenuPortal()
		dv.contextMenuDeferredTap.Cancel()
		if scroll != nil {
			scroll.ResetTouch()
		}
		return true
	}
	// Consume all input when open and inside.
	if pt.In(dv.contextMenuRect) {
		return true
	}
	return false
```

With:

```go
	// Consume all input when open and inside the rect.
	if pt.In(dv.contextMenuRect) {
		return true
	}
	return false
```

- [ ] **Step 2: Run context menu tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run "TestContextMenu" ./internal/ui/ -v -count=1`
Expected: Some tests FAIL (tests that call handleContextMenuInput directly with outside coords)

---

### Task 2: Update context menu tests that test dead click-outside code

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu_test.go:1600-1638`
- Modify: `src/go/internal/ui/context_menu_flicker_test.go:183-198`
- Modify: `src/go/internal/ui/mobile_overlay_tap_test.go:215-235`

These tests call `handleContextMenuInput()` directly with coordinates outside `contextMenuRect`, testing the now-removed click-outside block. Update them to either test through the tree (which is the real click-outside path) or test that the handler correctly returns `false` for outside coords.

- [ ] **Step 1: Update `TestContextMenuClickOutsideCloses`**

In `drumview_context_menu_test.go`, replace `TestContextMenuClickOutsideCloses` (lines 1600-1638):

```go
// TestContextMenuClickOutsideCloses verifies that handleContextMenuInput
// returns false for clicks outside the menu rect (click-outside is handled
// by the tree, not the handler).
func TestContextMenuClickOutsideCloses(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	// Warm-up frame for layout init.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	// Manually open the context menu.
	dv.openContextMenuPortal()
	dv.contextMenuRow = 0
	dv.contextMenuRect = image.Rect(100, 100, 300, 400)

	suppressClicksUntilRelease = false

	// Click well outside the menu rect — handler should NOT consume
	// (click-outside is handled by the tree).
	got := dv.handleContextMenuInput(500, 500, true)
	if got {
		t.Fatal("handleContextMenuInput should return false for clicks outside contextMenuRect")
	}
}
```

- [ ] **Step 2: Update `TestContextMenuSuppressGuardPreventsFlicker`**

In `context_menu_flicker_test.go`, the test at line 192 calls `handleContextMenuInput(outsideX, outsideY, true)` to test closing. Since click-outside is now tree-managed, update the test to use `dv.Update()` with input at the outside point (going through the tree). Replace lines 190-198:

```go
	// The tree's click-outside mechanism handles closing the context menu.
	// Simulate the click via Update (portal/tree path).
	resetOutside := SetInputForTest(
		func() (int, int) { return outsideX, outsideY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetOutside()

	if dv.IsContextMenuOpen() {
		t.Errorf("context menu should close when tapping outside after suppression cleared (tap pos: %d,%d, menu rect: %v)",
			outsideX, outsideY, dv.contextMenuRect)
	}
```

- [ ] **Step 3: Update `TestMobileContextMenuTapOutsideCloses`**

In `mobile_overlay_tap_test.go`, the test at line 227 calls `handleContextMenuInput(outsideX, outsideY, true)` directly. Update lines 224-235 to go through the tree:

```go
	// The tree's click-outside mechanism handles closing.
	// Simulate the click via Update (portal/tree path).
	r := mobileInputAt(t, outsideX, outsideY, true)
	dv.Update()
	r()

	if dv.IsContextMenuOpen() {
		t.Error("context menu should be closed after tapping outside")
	}
```

- [ ] **Step 4: Run all context menu tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run "TestContextMenu|TestMobileContextMenu|TestContextMenuSuppress" ./internal/ui/ -v -count=1`
Expected: ALL PASS

---

### Task 3: Remove dead click-outside from `handleOverflowMenuInput`

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go:555-563`

Same pattern as context menu. Portal `rectFn` returns `dv.overflowPopupRect()` (portal_open.go:144), so the `if left && !pt.In(popupRect) && !pt.In(dv.overflowBtn().Rect())` block is unreachable.

- [ ] **Step 1: Remove the dead click-outside block**

In `drumview_context_menu.go`, replace lines 555-563:

```go
	// Click outside closes.
	if left && !pt.In(popupRect) && !pt.In(dv.overflowBtn().Rect()) {
		dv.closeOverflowMenu()
		return true
	}
	if pt.In(popupRect) {
		return true
	}
	return false
```

With:

```go
	// Consume all input when open and inside the popup rect.
	if pt.In(popupRect) {
		return true
	}
	return false
```

- [ ] **Step 2: Run overflow menu tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run "TestOverflow" ./internal/ui/ -v -count=1`
Expected: Some tests FAIL (tests that call handleOverflowMenuInput directly with outside coords)

---

### Task 4: Update overflow menu tests that test dead click-outside code

**Files:**
- Modify: `src/go/internal/ui/overflow_menu_regressions_test.go:402-421`

- [ ] **Step 1: Update `TestOverflowMenuSuppressGuardPreventsFlicker`**

In `overflow_menu_regressions_test.go`, the test at line 415 calls `handleOverflowMenuInput(outsideX, outsideY, true)`. Update lines 413-421 to go through the tree:

```go
	// The tree's click-outside mechanism handles closing the overflow menu.
	// Simulate the click via Update (portal/tree path).
	resetOutside := SetInputForTest(
		func() (int, int) { return outsideX, outsideY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetOutside()

	if dv.IsOverflowMenuOpen() {
		t.Errorf("overflow menu should close when tapping outside after suppression cleared (tap pos: %d,%d, popup rect: %v)",
			outsideX, outsideY, popupRect)
	}
```

- [ ] **Step 2: Run overflow menu tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run "TestOverflow" ./internal/ui/ -v -count=1`
Expected: ALL PASS

---

### Task 5: Remove stale `fxPanelOpenSeq` field and all references

**Files:**
- Modify: `src/go/internal/ui/drumview.go:218` — remove field declaration
- Modify: `src/go/internal/ui/drumview_fx_panel.go:29` — remove assignment
- Modify: `src/go/internal/ui/drumview_fx_panel_test.go:552` — remove dead line
- Modify: `src/go/internal/ui/fx_panel_lifecycle_test.go:557` — remove dead line
- Modify: `src/go/internal/ui/mobile_overlay_tap_test.go:284` — remove dead line

The `fxPanelOpenSeq` field was used by the now-removed Phase 2 debounce check in `handleFXPanelInput`. Nothing reads it anymore.

- [ ] **Step 1: Remove the field declaration**

In `drumview.go:218`, remove the line:
```go
	fxPanelOpenSeq   int // updateSeq when panel opened (for click debounce)
```

- [ ] **Step 2: Remove the assignment**

In `drumview_fx_panel.go:29`, remove:
```go
	dv.fxPanelOpenSeq = dv.updateSeq
```

- [ ] **Step 3: Remove stale test references**

In `drumview_fx_panel_test.go:552`, remove:
```go
	dv.updateSeq = dv.fxPanelOpenSeq + 10
```

In `fx_panel_lifecycle_test.go:557`, remove:
```go
	dv.updateSeq = dv.fxPanelOpenSeq + 10
```

In `mobile_overlay_tap_test.go:284`, remove:
```go
	dv.fxPanelOpenSeq = dv.updateSeq - 5 // past debounce window
```

- [ ] **Step 4: Run FX panel tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run "TestFXPanel|TestMobileOverlayTap" ./internal/ui/ -v -count=1`
Expected: ALL PASS

---

### Task 6: Fix tests that bypass portal by setting `SliderPopup.open = true` directly

**Files:**
- Modify: `src/go/internal/ui/drumview_update_test.go:404-407`

These tests set the private `.open` field directly, which makes `IsOpen()` return true but does NOT register a portal entry. This means the tree has no hit areas for the popup and click-outside won't work. The test is checking that popups block drag initiation in `Update()`, so we need to open them through the portal instead.

- [ ] **Step 1: Update popup setters to use portal opening**

In `drumview_update_test.go`, replace the three direct `.open = true` lines:

```go
		{"volPopupOpen", func(dv *DrumView) { dv.volPopup.open = true }},
		{"eqPopupOpen", func(dv *DrumView) { dv.eqPopup.open = true }},
		...
		{"masterVolPopupOpen", func(dv *DrumView) { dv.masterVolPopup.open = true }},
```

With portal-based opening:

```go
		{"volPopupOpen", func(dv *DrumView) {
			dv.volPopup.Open(0.5, image.Pt(100, 100), dv.Bounds)
			dv.openVolPopupPortal()
		}},
		{"eqPopupOpen", func(dv *DrumView) {
			dv.eqPopup.Open(0.5, image.Pt(100, 100), dv.Bounds)
			dv.openEQPopupPortal()
		}},
		...
		{"masterVolPopupOpen", func(dv *DrumView) {
			dv.masterVolPopup.Open(0.5, image.Pt(100, 100), dv.Bounds)
			dv.openMasterVolPopupPortal()
		}},
```

- [ ] **Step 2: Run the update test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run "TestDrumViewUpdatePopupBlocksAll" ./internal/ui/ -v -count=1`
Expected: PASS

---

### Task 7: Full test suite verification

- [ ] **Step 1: Run ALL UI tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -count=1`
Expected: ALL PASS (no regressions)

- [ ] **Step 2: Commit**

```bash
git add src/go/internal/ui/drumview_context_menu.go \
       src/go/internal/ui/drumview_context_menu_test.go \
       src/go/internal/ui/context_menu_flicker_test.go \
       src/go/internal/ui/mobile_overlay_tap_test.go \
       src/go/internal/ui/overflow_menu_regressions_test.go \
       src/go/internal/ui/drumview.go \
       src/go/internal/ui/drumview_fx_panel.go \
       src/go/internal/ui/drumview_fx_panel_test.go \
       src/go/internal/ui/fx_panel_lifecycle_test.go \
       src/go/internal/ui/drumview_update_test.go
git commit -m "refactor: remove dead click-outside code from context/overflow menu handlers

Remove unreachable click-outside blocks from handleContextMenuInput and
handleOverflowMenuInput — the tree's click-outside mechanism (drumview_tree.go:245-249)
handles portal closing before these handlers run. Remove stale fxPanelOpenSeq
field (debounce no longer checked). Fix tests that bypassed portal system
by setting SliderPopup.open directly."
```
