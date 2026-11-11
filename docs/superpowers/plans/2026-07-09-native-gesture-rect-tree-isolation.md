# Native-Gesture Rect Tree Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make out-of-tree JS gesture rects (file picker, soft keyboard, mobile native input) a per-frame projection of the composed component tree's topmost hit, so a rect can never fire for a component that isn't the visible topmost one — fixing the "tap a template row → file picker opens" leak and making the whole class impossible.

**Architecture:** A single per-frame phase in `(*DrumView).Update` collects native-rect *candidates* from pure producers, gates each by `RootTree.TopmostOwnerAt(center) == candidate.Owner` (reusing the real dispatch precedence), diffs vs. the last synced set, and pushes the delta to JS through a platform seam. Producers replace the three imperative `*RegisterRect`/`*Clear` sites.

**Tech Stack:** Go 1.23, `syscall/js` (WASM), Ebiten, the existing `DrumViewTree`/`RootTree`/`HitIndex`/`OverlayPortal` input system.

## Global Constraints

- Bundled Go only: `/home/ymolinar/Repos/beatmo/.tools/go/bin/go` (alias `GO` below). Never system `go`.
- Fast test invocation (from `src/go`): `$GO test -tags test -modfile=go.test.mod ./internal/ui/ -run <Name>`.
- Both build tags must compile: native (`!js`) and WASM (`GOOS=js GOARCH=wasm`), each with and without `-tags test`.
- Mobile+WASM test profile: `restore := SetRuntimeProfileForTest(browserRuntimeProfile()); defer restore()` + `g.SetForceMobileProfile(true)` before `g.Layout(w,h)`.
- Per repo convention: do NOT `git add`/`git commit`. Where a task says "Commit", instead leave the working tree dirty and run the task's verification. (This overrides the skill's commit steps.)
- Pre-existing failing tests on this branch are documented; do not attribute them to this work. The only NEW failures allowed are RED steps you then turn GREEN.
- Owner IDs: the overflow menu portal ID is `"overflow-menu"` (see `openOverflowMenuPortal`). Zone IDs come from each zone's `ID()`. Overlay/portal owner IDs are the `PortalEntry.ID` passed to `portal().Open`.

---

## File Structure

- Create `src/go/internal/ui/native_gesture.go` — types (`NativeChannel`, `NativeIntent`, `NativeRectCandidate`, `NativeRect`), `(*DrumView).collectNativeRectCandidates`, the per-channel candidate producers, `(*DrumView).syncNativeGestures`, and `(*DrumView).lastNativeRects` state. Build-tag-free (compiles everywhere).
- Create `src/go/internal/ui/native_gesture_wasm.go` (`//go:build js && !test`) — `platformSyncNativeRects` dispatching to the existing low-level JS bindings.
- Create `src/go/internal/ui/native_gesture_stub.go` (`//go:build !js || test`) — `platformSyncNativeRects` recording into a test-inspectable var.
- Modify `src/go/internal/ui/root_tree.go` — add `TopmostOwnerAt` + `(*DrumViewTree).topOwnerAt` helper.
- Modify `src/go/internal/ui/drumview_update.go` — call `dv.syncNativeGestures()` once, right after `dv.rootTree.Update()`.
- Modify `src/go/internal/ui/drumview_context_menu.go` — delete `registerFilePickerRects` body/usage; file-picker candidates move to `native_gesture.go`.
- Modify `src/go/internal/ui/drumview_ctor.go` — drop the mobile `registerFilePickerRects()` call in `OnOverflowOpen`.
- Modify `src/go/internal/ui/drumview_portal_open.go` — drop `filePickerClearRects()` from overflow `OnClose`.
- Modify `src/go/internal/ui/drumview_layout.go` — delete the soft-keyboard + mobile-native-input registration blocks (moved to producers).
- Create tests: `native_gesture_test.go`, `native_gesture_topmost_test.go`, `template_row_no_filepicker_leak_test.go`, `native_gesture_discipline_test.go`.

---

### Task 1: Native-gesture types + platform seam

**Files:**
- Create: `src/go/internal/ui/native_gesture.go` (types only, this task)
- Create: `src/go/internal/ui/native_gesture_stub.go`
- Create: `src/go/internal/ui/native_gesture_wasm.go`
- Test: `src/go/internal/ui/native_gesture_test.go`

**Interfaces:**
- Produces: types `NativeChannel` (`NativeFilePicker`, `NativeSoftKeyboard`, `NativeTextInput`), `NativeIntent{Channel,ID,Accept,InputMode,Text,MaxLen,InputRect}`, `NativeRect{Rect,Intent}`, `NativeRectCandidate{Owner,Rect,Intent}`; func `platformSyncNativeRects([]NativeRect)`; test var `testCapturedNativeRects *[]NativeRect`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/native_gesture_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

func TestPlatformSyncNativeRects_RecordsInStub(t *testing.T) {
	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	in := []NativeRect{
		{Rect: image.Rect(0, 0, 10, 10), Intent: NativeIntent{Channel: NativeFilePicker, ID: "import", Accept: ".json"}},
	}
	platformSyncNativeRects(in)

	if len(captured) != 1 || captured[0].Intent.ID != "import" {
		t.Fatalf("stub did not record synced rects: %+v", captured)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestPlatformSyncNativeRects_RecordsInStub`
Expected: FAIL — undefined: `NativeRect`, `platformSyncNativeRects`, `testCapturedNativeRects`.

- [ ] **Step 3: Write the types**

Create `src/go/internal/ui/native_gesture.go`:

```go
package ui

import "image"

// NativeChannel identifies which browser-gesture registry a native rect targets.
type NativeChannel int

const (
	NativeFilePicker   NativeChannel = iota // OS file dialog on touchend
	NativeSoftKeyboard                      // focusable soft-keyboard rect (desktop browser)
	NativeTextInput                         // mobile native <input> overlay
)

// NativeIntent describes the browser gesture to arm at a rect.
type NativeIntent struct {
	Channel   NativeChannel
	ID        string
	Accept    string // file picker
	InputMode string // "numeric" | "text"
	Text      string // native text input initial value
	MaxLen    int    // native text input max length
	// InputRect: trigger/input split for NativeTextInput. Zero → the input
	// appears at the (gated) trigger rect. Non-zero → the trigger rect is what
	// the user taps, InputRect is where the <input> is placed (rename glue).
	InputRect image.Rectangle
}

// NativeRectCandidate is a producer's pre-gating output. Rect is the TRIGGER
// rect (what the user taps, the thing gated by z-order). Owner is the ownerID
// TopmostOwnerAt(center) must return for the candidate to survive gating.
type NativeRectCandidate struct {
	Owner  string
	Rect   image.Rectangle
	Intent NativeIntent
}

// NativeRect is a gated, screen-space rect synced to the JS registries.
type NativeRect struct {
	Rect   image.Rectangle
	Intent NativeIntent
}
```

Create `src/go/internal/ui/native_gesture_stub.go`:

```go
//go:build !js || test

package ui

// testCapturedNativeRects, when non-nil, receives every platformSyncNativeRects
// call (last write wins the slice contents) for test inspection.
var testCapturedNativeRects *[]NativeRect

func platformSyncNativeRects(rects []NativeRect) {
	if testCapturedNativeRects != nil {
		*testCapturedNativeRects = append((*testCapturedNativeRects)[:0], rects...)
	}
}
```

Create `src/go/internal/ui/native_gesture_wasm.go`:

```go
//go:build js && !test

package ui

// platformSyncNativeRects clears each JS gesture registry and re-arms exactly
// the gated set. Called only when the set changed (see syncNativeGestures), so
// the clear+register churn cannot interrupt a steady-state gesture.
func platformSyncNativeRects(rects []NativeRect) {
	filePickerClearRects()
	softKeyboardClearRects()
	mobileInputClear()
	for _, r := range rects {
		switch r.Intent.Channel {
		case NativeFilePicker:
			filePickerRegisterRect(r.Intent.ID, r.Rect.Min.X, r.Rect.Min.Y, r.Rect.Dx(), r.Rect.Dy(), r.Intent.Accept)
		case NativeSoftKeyboard:
			softKeyboardRegisterRect(r.Intent.ID, r.Rect.Min.X, r.Rect.Min.Y, r.Rect.Dx(), r.Rect.Dy(), r.Intent.InputMode)
		case NativeTextInput:
			if r.Intent.InputRect.Empty() {
				mobileInputRegister(r.Intent.ID, r.Rect.Min.X, r.Rect.Min.Y, r.Rect.Dx(), r.Rect.Dy(), r.Intent.Text, r.Intent.MaxLen, r.Intent.InputMode)
			} else {
				ir := r.Intent.InputRect
				mobileInputRegisterTrigger(r.Intent.ID,
					r.Rect.Min.X, r.Rect.Min.Y, r.Rect.Dx(), r.Rect.Dy(),
					ir.Min.X, ir.Min.Y, ir.Dx(), ir.Dy(),
					r.Intent.Text, r.Intent.MaxLen, r.Intent.InputMode)
			}
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestPlatformSyncNativeRects_RecordsInStub`
Expected: PASS.

- [ ] **Step 5: Verify WASM build compiles**

Run: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/`
Expected: no output (success). (This proves `native_gesture_wasm.go` references the real JS bindings correctly.)

- [ ] **Step 6: Checkpoint** — leave dirty (no commit per Global Constraints); re-run Step 4 to confirm green.

---

### Task 2: `RootTree.TopmostOwnerAt` gate

**Files:**
- Modify: `src/go/internal/ui/root_tree.go`
- Test: `src/go/internal/ui/native_gesture_topmost_test.go`

**Interfaces:**
- Consumes: `RootTree`, `(*DrumViewTree).hitIndex`, `HitIndex.At`, `indexedHitArea.ownerID`, `r.capturer()`, `r.blockingPortalOwner()`.
- Produces: `(*RootTree).TopmostOwnerAt(x, y int) string`; `(*DrumViewTree).topOwnerAt(x, y int) string`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/native_gesture_topmost_test.go`:

```go
//go:build test

package ui

import (
	"testing"
)

// A press over the open overflow menu resolves to the menu's overlay owner, and
// a press over a base-zone control resolves to that zone — proving TopmostOwnerAt
// mirrors real dispatch precedence (blocking portal wins).
func TestTopmostOwnerAt_BlockingPortalOwnsPoint(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)
	dv := g.drum

	dv.OpenOverflowMenu()
	g.Update() // publish portal hit areas into the overlay subtree index

	r := dv.overflowPopupRect()
	if r.Empty() {
		t.Fatal("overflow popup rect empty after open")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	if got := dv.rootTree.TopmostOwnerAt(cx, cy); got != "overflow-menu" {
		t.Fatalf("TopmostOwnerAt over open menu = %q, want overflow-menu", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTopmostOwnerAt_BlockingPortalOwnsPoint`
Expected: FAIL — `dv.rootTree.TopmostOwnerAt` undefined.

- [ ] **Step 3: Implement TopmostOwnerAt + topOwnerAt**

Add to `src/go/internal/ui/root_tree.go`:

```go
// TopmostOwnerAt returns the ownerID that a fresh press at (x, y) would be
// dispatched to, using the SAME precedence as dispatchInput: capture-holder →
// blocking-portal owner → top-down by z. Within the chosen subtree it returns
// that subtree's topmost hit area's ownerID. "" if nothing is there. This is the
// gate for native-gesture rects: a rect is armable only where a real press would
// reach its owning component.
func (r *RootTree) TopmostOwnerAt(x, y int) string {
	if c := r.capturer(); c != nil {
		return c.topOwnerAt(x, y)
	}
	if c := r.blockingPortalOwner(); c != nil {
		return c.topOwnerAt(x, y)
	}
	for i := len(r.children) - 1; i >= 0; i-- {
		if o := r.children[i].tree.topOwnerAt(x, y); o != "" {
			return o
		}
	}
	return ""
}
```

Add to `src/go/internal/ui/drumview_tree.go` (near `HitIndexRef`):

```go
// topOwnerAt returns the ownerID of this subtree's topmost hit area at (x, y),
// or "" if none. Mirrors what OnPress dispatch would pick first.
func (t *DrumViewTree) topOwnerAt(x, y int) string {
	hits := t.hitIndex.At(x, y)
	if len(hits) == 0 {
		return ""
	}
	return hits[0].ownerID
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTopmostOwnerAt_BlockingPortalOwnsPoint`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — leave dirty; confirm green.

---

### Task 3: Sync phase wired into the frame loop (empty collector)

**Files:**
- Modify: `src/go/internal/ui/native_gesture.go` (add `lastNativeRects`, `collectNativeRectCandidates` returning nil for now, `syncNativeGestures`)
- Modify: `src/go/internal/ui/drumview.go` (add `lastNativeRects []NativeRect` field to the `DrumView` struct)
- Modify: `src/go/internal/ui/drumview_update.go` (call `dv.syncNativeGestures()` after `dv.rootTree.Update()`)
- Test: `src/go/internal/ui/native_gesture_test.go` (add case)

**Interfaces:**
- Consumes: `(*RootTree).TopmostOwnerAt`, `platformSyncNativeRects`.
- Produces: `(*DrumView).collectNativeRectCandidates() []NativeRectCandidate`, `(*DrumView).syncNativeGestures()`, field `DrumView.lastNativeRects`.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/native_gesture_test.go`:

```go
func TestSyncNativeGestures_NoCandidates_ClearsAndNoChurn(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	// With no menus/inputs open there are no candidates; a frame must leave the
	// synced set empty.
	g.Update()
	if len(g.drum.lastNativeRects) != 0 {
		t.Fatalf("expected no armed native rects with nothing open, got %d", len(g.drum.lastNativeRects))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSyncNativeGestures_NoCandidates_ClearsAndNoChurn`
Expected: FAIL — `g.drum.lastNativeRects` undefined.

- [ ] **Step 3: Add the field**

In `src/go/internal/ui/drumview.go`, add to the `DrumView` struct (near other transient input state):

```go
	// lastNativeRects is the set of native-gesture rects synced to JS last frame.
	// syncNativeGestures diffs against it to avoid per-frame arm/clear churn.
	lastNativeRects []NativeRect
```

- [ ] **Step 4: Add collector + sync (empty collector for now)**

Append to `src/go/internal/ui/native_gesture.go`:

```go
// collectNativeRectCandidates returns the native-gesture rects the current UI
// state wants armed, PRE-gating. Pure function of current state; no JS side
// effects. Producers are filled in by later tasks.
func (dv *DrumView) collectNativeRectCandidates() []NativeRectCandidate {
	if dv == nil {
		return nil
	}
	var out []NativeRectCandidate
	out = append(out, dv.filePickerCandidates()...)
	out = append(out, dv.softKeyboardCandidates()...)
	out = append(out, dv.nativeInputCandidates()...)
	return out
}

// syncNativeGestures is the single per-frame native-rect projection: collect
// candidates, keep only those whose trigger rect resolves (via the composed
// dispatch precedence) to their own owner, diff against last frame, and sync the
// delta to the JS registries. Called once from (*DrumView).Update after the
// RootTree has ticked and dispatched (indexes final).
func (dv *DrumView) syncNativeGestures() {
	if dv == nil || dv.rootTree == nil {
		return
	}
	cands := dv.collectNativeRectCandidates()
	kept := make([]NativeRect, 0, len(cands))
	for _, c := range cands {
		cx := (c.Rect.Min.X + c.Rect.Max.X) / 2
		cy := (c.Rect.Min.Y + c.Rect.Max.Y) / 2
		if dv.rootTree.TopmostOwnerAt(cx, cy) == c.Owner {
			kept = append(kept, NativeRect{Rect: c.Rect, Intent: c.Intent})
		}
	}
	if nativeRectsEqual(kept, dv.lastNativeRects) {
		return
	}
	platformSyncNativeRects(kept)
	dv.lastNativeRects = kept
}

func nativeRectsEqual(a, b []NativeRect) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Add three temporary empty producers at the end of `native_gesture.go` (each replaced in Tasks 4-6):

```go
func (dv *DrumView) filePickerCandidates() []NativeRectCandidate  { return nil }
func (dv *DrumView) softKeyboardCandidates() []NativeRectCandidate { return nil }
func (dv *DrumView) nativeInputCandidates() []NativeRectCandidate  { return nil }
```

Note: `NativeRect` contains an `image.Rectangle` and a `NativeIntent` (which contains an `image.Rectangle`); both are comparable structs, so `a[i] != b[i]` compiles.

- [ ] **Step 5: Wire the single call site**

In `src/go/internal/ui/drumview_update.go`, immediately after the `dv.rootTree.Update()` call (around line 52):

```go
	if dv.rootTree != nil {
		dv.rootTree.SetBounds(dv.Bounds)
		// ... existing lines ...
		dv.rootTree.Update()
		dv.syncNativeGestures() // ADD: single native-gesture projection site
	}
```

(Place `dv.syncNativeGestures()` as the last statement inside the existing `if dv.rootTree != nil` block that calls `dv.rootTree.Update()`.)

- [ ] **Step 6: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSyncNativeGestures_NoCandidates_ClearsAndNoChurn`
Expected: PASS.

- [ ] **Step 7: Verify both builds**

Run:
```
cd src/go && ../../.tools/go/bin/go build ./internal/ui/
cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/
```
Expected: both succeed.

- [ ] **Step 8: Checkpoint** — leave dirty; confirm green.

---

### Task 4: Migrate the file-picker channel + fix the reported bug

**Files:**
- Modify: `src/go/internal/ui/native_gesture.go` (implement `filePickerCandidates`)
- Modify: `src/go/internal/ui/drumview_context_menu.go` (delete `registerFilePickerRects`)
- Modify: `src/go/internal/ui/drumview_ctor.go` (remove the mobile `registerFilePickerRects()` call in `OnOverflowOpen`)
- Modify: `src/go/internal/ui/drumview_portal_open.go` (remove `filePickerClearRects()` from overflow `OnClose`)
- Test: `src/go/internal/ui/template_row_no_filepicker_leak_test.go`

**Interfaces:**
- Consumes: `dv.IsOverflowMenuOpen()`, `dv.overflowPage`, `dv.overflowPopupRect()`, `dv.overflowItems()`, `dv.configureOverflowScroll()`, `dv.overflowMenuScroll.OffsetPx()`, `closeButtonRect`, `insetRect`, `touchMinTargetPx`, `SpaceXS`, `SpaceSM`, owner `"overflow-menu"`.
- Produces: `(*DrumView).filePickerCandidates() []NativeRectCandidate`.

- [ ] **Step 1: Write the failing reported-bug regression test**

Create `src/go/internal/ui/template_row_no_filepicker_leak_test.go`:

```go
//go:build test

package ui

import (
	"testing"
)

// Reproduces the reported WASM-mobile bug: on the overflow menu's Templates
// page, a stale File-page file-picker rect used to sit under a template row, so
// tapping a template row opened the OS file picker. With the tree-owned
// projection, no file-picker rect may be armed while the Templates page is
// showing; the File page still arms Upload/Import.
func TestOverflowTemplatePage_NoFilePickerRectArmed(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)
	dv := g.drum

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	// Open menu on the File page → Upload+Import file-picker rects armed.
	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()
	fileArmed := 0
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeFilePicker {
			fileArmed++
		}
	}
	if fileArmed == 0 {
		t.Fatal("File page armed no file-picker rects — precondition failed")
	}

	// Switch to Templates page. No file-picker rect may remain armed.
	dv.overflowPage = 1
	g.Update()
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeFilePicker {
			t.Fatalf("file-picker rect %q armed on Templates page (id=%q rect=%v) — leaks onto template rows",
				r.Intent.ID, r.Intent.ID, r.Rect)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestOverflowTemplatePage_NoFilePickerRectArmed`
Expected: FAIL at the precondition (`filePickerCandidates` still returns nil → `fileArmed == 0`). This confirms the test drives the real path and the producer is not yet implemented.

- [ ] **Step 3: Implement `filePickerCandidates`**

Replace the placeholder `filePickerCandidates` in `src/go/internal/ui/native_gesture.go` with the File-page-gated producer (mirrors the deleted `registerFilePickerRects`, but returns candidates and is page-gated):

```go
// filePickerCandidates returns the Upload/Import file-picker triggers, owner
// "overflow-menu", ONLY when the overflow menu is open on the File page
// (overflowPage 0). On the Templates page (or closed) it returns nil, so the
// tree-owned sync clears the JS registry and no rect can leak under a template
// row. The close-button crop is kept here (menu-internal; the menu is a single
// hit area, so it is not derivable from z-order).
func (dv *DrumView) filePickerCandidates() []NativeRectCandidate {
	if dv == nil || !dv.IsOverflowMenuOpen() || dv.overflowPage != 0 {
		return nil
	}
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		return nil
	}
	dv.configureOverflowScroll()
	offset := 0
	if dv.overflowMenuScroll != nil {
		offset = dv.overflowMenuScroll.OffsetPx()
	}
	rowH := touchMinTargetPx
	accept := map[string]string{"Upload": ".wav", "Import": "application/json,.json"}
	idByLabel := map[string]string{"Upload": "upload", "Import": "import"}
	closeR := closeButtonRect(popupRect, SpaceXS)

	var out []NativeRectCandidate
	for i, item := range dv.overflowItems() {
		fileID, ok := idByLabel[item.label]
		if !ok {
			continue
		}
		y0 := popupRect.Min.Y + i*rowH - offset
		r := insetRect(image.Rect(popupRect.Min.X, y0, popupRect.Max.X, y0+rowH), SpaceXS)
		if r.Max.Y > closeR.Min.Y && r.Min.Y < closeR.Max.Y {
			if r.Max.X > closeR.Min.X {
				r.Max.X = closeR.Min.X - SpaceSM
			}
		}
		out = append(out, NativeRectCandidate{
			Owner:  "overflow-menu",
			Rect:   r,
			Intent: NativeIntent{Channel: NativeFilePicker, ID: fileID, Accept: accept[item.label]},
		})
	}
	return out
}
```

Ensure `native_gesture.go` imports `image` (already needed for the types).

- [ ] **Step 4: Delete the imperative file-picker sites**

In `src/go/internal/ui/drumview_context_menu.go`, delete the entire `registerFilePickerRects` function (the `func (dv *DrumView) registerFilePickerRects() { ... }` block, ~lines 1046-1087).

In `src/go/internal/ui/drumview_ctor.go`, in `OnOverflowOpen` (~lines 762-773), remove the mobile branch that calls `dv.registerFilePickerRects()` (delete the `if Profile().IsMobile() { dv.registerFilePickerRects() }` lines).

In `src/go/internal/ui/drumview_portal_open.go`, in the overflow menu `OnClose` (~line 188), remove the `filePickerClearRects()` line (the tree sync now clears it when candidates vanish). Also remove the `filePickerClearRects()` call in `closeOverflowMenu` (`drumview_context_menu.go` ~line 1042) if present.

- [ ] **Step 5: Run the bug regression test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestOverflowTemplatePage_NoFilePickerRectArmed`
Expected: PASS (File page arms Upload+Import; Templates page arms none).

- [ ] **Step 6: Verify no compile refs to the deleted function remain + both builds**

Run:
```
cd src/go && ../../.tools/go/bin/go build ./internal/ui/
cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Overflow|Template|FilePicker|MobileFilePicker'
```
Expected: builds succeed; overflow/template/file-picker tests pass (fix any test that asserted the old imperative `registerFilePickerRects`/`filePickerClearRects` behavior by re-pointing it at `dv.lastNativeRects` — the new source of truth. If a test used `testCapturedFilePickerRects`, switch it to `testCapturedNativeRects` filtered by `NativeFilePicker`).

- [ ] **Step 7: Checkpoint** — leave dirty; confirm the bug test + overflow suite green.

---

### Task 5: Migrate the soft-keyboard channel

**Files:**
- Modify: `src/go/internal/ui/native_gesture.go` (implement `softKeyboardCandidates`)
- Modify: `src/go/internal/ui/drumview_layout.go` (delete the soft-keyboard registration block, ~lines 859-881)
- Test: `src/go/internal/ui/native_gesture_test.go` (add case)

**Interfaces:**
- Consumes: `Profile().IsMobile()`, `dv.bpmBox()`, `dv.IsInstMenuOpen()`, `dv.instSearchBox`, `dv.instSearchRect`, `dv.IsNamingOpen()`, `dv.nameBox`, `dv.renameBox`, `instSearchMobileInputID`, the owning zone/overlay IDs of each widget.
- Produces: `(*DrumView).softKeyboardCandidates() []NativeRectCandidate`.

**Owner resolution:** each widget's `Owner` is the ownerID that the hit index reports at that widget's rect. Determine each once by reading the publishing zone/overlay: the BPM box is published by the transport zone (`Owner = <transportZone>.ID()`); inst-search by the instrument-menu overlay (`Owner = "inst-menu"` — confirm via the `portal().Open` ID in `openInstMenuPortal`); wav-name by the naming overlay (`Owner = "naming"` — confirm via its portal Open ID); rename box by the rename overlay (`Owner = "rename"`). Confirm each ID by grep before writing (`grep -n 'ID: *"' drumview_portal_open.go` and the transport zone's `ID()`), then hardcode the confirmed constant.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/native_gesture_test.go` (desktop-browser profile, BPM box present):

```go
func TestSoftKeyboardCandidate_BPMArmedOnDesktopBrowser(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Desktop browser (NOT mobile) → soft-keyboard channel is the active one.
	g.Layout(1200, 800)

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	g.Update()
	found := false
	for _, r := range g.drum.lastNativeRects {
		if r.Intent.Channel == NativeSoftKeyboard && r.Intent.ID == "bpm" {
			found = true
		}
	}
	if !found {
		t.Fatal("BPM soft-keyboard rect not armed on desktop browser — soft-keyboard channel migration incomplete")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSoftKeyboardCandidate_BPMArmedOnDesktopBrowser`
Expected: FAIL — `softKeyboardCandidates` returns nil.

- [ ] **Step 3: Implement `softKeyboardCandidates`**

Replace the placeholder in `native_gesture.go` (transcribe the deleted block from `drumview_layout.go` 859-881; `"transport"` etc. resolved per "Owner resolution" above):

```go
// softKeyboardCandidates returns focusable soft-keyboard rects for DESKTOP
// browser only (on mobile the native-input channel handles text). Mirrors the
// former drumview_layout.go registration block, page/open-state gates intact.
func (dv *DrumView) softKeyboardCandidates() []NativeRectCandidate {
	if dv == nil || Profile().IsMobile() {
		return nil
	}
	var out []NativeRectCandidate
	add := func(owner, id, mode string, r image.Rectangle) {
		if r.Empty() {
			return
		}
		out = append(out, NativeRectCandidate{Owner: owner, Rect: r,
			Intent: NativeIntent{Channel: NativeSoftKeyboard, ID: id, InputMode: mode}})
	}
	if dv.bpmBox() != nil {
		add("transport", "bpm", "numeric", dv.bpmBox().Rect)
	}
	if dv.IsInstMenuOpen() && dv.instSearchBox != nil {
		add("inst-menu", instSearchMobileInputID, "text", dv.instSearchRect)
	}
	if dv.IsNamingOpen() && dv.nameBox != nil {
		add("naming", "wav-name", "text", dv.nameBox.Rect)
	}
	if dv.renameBox != nil {
		add("rename", "rename", "text", dv.renameBox.Rect)
	}
	return out
}
```

- [ ] **Step 4: Delete the imperative soft-keyboard block**

In `src/go/internal/ui/drumview_layout.go`, delete lines ~859-881 (the comment + `softKeyboardClearRects()` + the `if !p.IsMobile() { ... }` block with the four `softKeyboardRegisterRect` calls).

- [ ] **Step 5: Run test + builds**

Run:
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSoftKeyboardCandidate_BPMArmedOnDesktopBrowser|SoftKeyboard|BPM'
cd src/go && ../../.tools/go/bin/go build ./internal/ui/ && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/
```
Expected: new test PASS; existing soft-keyboard/BPM tests PASS (re-point any that asserted `softKeyboardRegisterRect` at `dv.lastNativeRects` filtered by `NativeSoftKeyboard`); both builds succeed.

- [ ] **Step 6: Checkpoint** — leave dirty; confirm green.

---

### Task 6: Migrate the mobile native-input channel (highest risk)

**Files:**
- Modify: `src/go/internal/ui/native_gesture.go` (implement `nativeInputCandidates`)
- Modify: `src/go/internal/ui/drumview_layout.go` (delete the mobile native-input block, ~lines 883-937)
- Modify: `src/go/internal/ui/param_value_editor.go` (remove the direct `mobileInputRegister` at ~line 148; fold into `nativeInputCandidates` if that path is still needed, else confirm it is covered by the BPM candidate)
- Test: `src/go/internal/ui/native_gesture_test.go` (add cases: BPM direct + rename trigger)

**Interfaces:**
- Consumes: `dv.bpmBox()`, the rename-trigger path (`dv.IsContextMenuOpen()`, `dv.contextMenuRow`, `dv.contextMenuRenameBtn()`, `dv.rowLabels()`, `dv.Rows`), `dv.IsInstMenuOpen()`/`instSearchBox`/`instSearchRect`, `dv.IsNamingOpen()`/`nameBox`. Owners resolved as in Task 5, plus the rename trigger's owner is the **context-menu** overlay (`Owner = "context-menu"` — confirm via its portal Open ID).
- Produces: `(*DrumView).nativeInputCandidates() []NativeRectCandidate`.

**Note on the rename trigger/input split:** the Rename menu item is the TRIGGER (gated by z-order → `Owner = "context-menu"`), and the `<input>` is glued to the row-label rect (`Intent.InputRect = labelR`). Set `Rect = trigR` (Rename item), `Intent.InputRect = labelR`.

- [ ] **Step 1: Write the failing tests**

Add to `native_gesture_test.go`:

```go
func TestNativeInputCandidate_BPMArmedOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	g.Update()
	found := false
	for _, r := range g.drum.lastNativeRects {
		if r.Intent.Channel == NativeTextInput && r.Intent.ID == "bpm" {
			found = true
		}
	}
	if !found {
		t.Fatal("BPM native-input rect not armed on mobile — native-input migration incomplete")
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNativeInputCandidate_BPMArmedOnMobile`
Expected: FAIL — `nativeInputCandidates` returns nil.

- [ ] **Step 3: Implement `nativeInputCandidates`**

Replace the placeholder in `native_gesture.go` (transcribe the deleted mobile block from `drumview_layout.go` 883-937; owners resolved per Task 5/6 notes; `"transport"` as in Task 5):

```go
// nativeInputCandidates returns mobile native <input> triggers. Mirrors the
// former drumview_layout.go mobile block: BPM direct rect, rename trigger→label
// glue, inst-search, wav-name. Gates + open-state guards preserved.
func (dv *DrumView) nativeInputCandidates() []NativeRectCandidate {
	if dv == nil || !Profile().IsMobile() {
		return nil
	}
	var out []NativeRectCandidate
	if dv.bpmBox() != nil && !dv.bpmBox().Rect.Empty() {
		out = append(out, NativeRectCandidate{Owner: "transport", Rect: dv.bpmBox().Rect,
			Intent: NativeIntent{Channel: NativeTextInput, ID: "bpm", Text: dv.bpmBox().Text, MaxLen: 4, InputMode: "numeric"}})
	}
	if dv.IsContextMenuOpen() && dv.contextMenuRow >= 0 &&
		dv.contextMenuRow < len(dv.Rows) && dv.contextMenuRow < len(dv.rowLabels()) {
		if renameBtn := dv.contextMenuRenameBtn(); renameBtn != nil {
			trigR := renameBtn.Rect()
			labelR := dv.rowLabels()[dv.contextMenuRow].Rect()
			if !trigR.Empty() && !labelR.Empty() {
				out = append(out, NativeRectCandidate{Owner: "context-menu", Rect: trigR,
					Intent: NativeIntent{Channel: NativeTextInput,
						ID: fmt.Sprintf("rename-%d", dv.contextMenuRow),
						Text: dv.Rows[dv.contextMenuRow].Name, MaxLen: 32, InputMode: "text",
						InputRect: labelR}})
			}
		}
	}
	if dv.IsInstMenuOpen() && dv.instSearchBox != nil && !dv.instSearchRect.Empty() {
		out = append(out, NativeRectCandidate{Owner: "inst-menu", Rect: dv.instSearchRect,
			Intent: NativeIntent{Channel: NativeTextInput, ID: instSearchMobileInputID, Text: dv.instSearchBox.Text, MaxLen: 40, InputMode: "text"}})
	}
	if dv.IsNamingOpen() && dv.nameBox != nil && !dv.nameBox.Rect.Empty() {
		out = append(out, NativeRectCandidate{Owner: "naming", Rect: dv.nameBox.Rect,
			Intent: NativeIntent{Channel: NativeTextInput, ID: "wav-name", Text: dv.nameBox.Text, MaxLen: 32, InputMode: "text"}})
	}
	return out
}
```

Add `"fmt"` to `native_gesture.go` imports.

- [ ] **Step 4: Delete the imperative mobile block + editor site**

In `src/go/internal/ui/drumview_layout.go`, delete lines ~883-937 (the `if p.IsMobile() { mobileInputClear(); ... }` block). If deleting it empties the enclosing `func`, leave the function returning normally.

In `src/go/internal/ui/param_value_editor.go` line ~148, remove the direct `mobileInputRegister(o.MobileInputID, ...)` call. (The BPM editor rect is now produced by `nativeInputCandidates`. If `param_value_editor` covers editors beyond BPM that are NOT in the candidate list, instead of deleting, add a producer for those editors — verify by grepping `MobileInputID` usages before deleting; if BPM is the only one, delete.)

- [ ] **Step 5: Run tests + builds + the delicate mobile input suites**

Run:
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'NativeInput|MobileBPM|Rename|MobileNative|MobileText|ColorButtonTriggersRename'
cd src/go && ../../.tools/go/bin/go build ./internal/ui/ && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/
```
Expected: new test PASS; the mobile BPM / rename-isolation / native-input tests PASS. Any test that asserted `mobileInputRegister*` capture must be re-pointed at `dv.lastNativeRects` filtered by `NativeTextInput`. If the documented "tapping Color opens rename" isolation test exists, it must stay green (the trigger owner `"context-menu"` + trigger rect = Rename item preserves that guarantee).

- [ ] **Step 6: WASM browser parity check**

Run the mobile native-input + file-picker browser suites (these exercise the real JS bindings through the new seam):
```
cd src/go && GO=$(pwd)/../../.tools/go/bin/go node ../js/mobile_file_picker.browser.test.js
cd src/go && GO=$(pwd)/../../.tools/go/bin/go node ../js/mobile_native_input.browser.test.js
cd src/go && GO=$(pwd)/../../.tools/go/bin/go node ../js/mobile_text_input.browser.test.js
```
Expected: PASS (behavior unchanged; only the Go caller moved). If a suite is flaky per the documented mobile-native-input race, re-run in isolation.

- [ ] **Step 7: Checkpoint** — leave dirty; confirm green.

---

### Task 7: Discipline + isolation invariant tests (lock the class shut)

**Files:**
- Test: `src/go/internal/ui/native_gesture_discipline_test.go`
- Test: `src/go/internal/ui/native_gesture_test.go` (add occlusion invariant)

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: Write the discipline test**

Create `src/go/internal/ui/native_gesture_discipline_test.go`:

```go
//go:build test

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// No production file outside the platform seam (native_gesture*.go) may call the
// low-level JS gesture registrars directly. This keeps every native rect flowing
// through the tree-owned, z-gated syncNativeGestures — so the file-picker leak
// class cannot be reintroduced.
func TestNativeGesture_NoDirectRegistrarCallsOutsideSeam(t *testing.T) {
	banned := regexp.MustCompile(`\b(filePickerRegisterRect|softKeyboardRegisterRect|mobileInputRegister|mobileInputRegisterTrigger)\s*\(`)
	seam := map[string]bool{
		"native_gesture.go": true, "native_gesture_wasm.go": true, "native_gesture_stub.go": true,
		// the low-level bindings themselves DEFINE these funcs:
		"filepicker_wasm.go": true, "filepicker_stub.go": true,
		"softkeyboard_wasm.go": true, "softkeyboard_stub.go": true,
		"mobile_input_wasm.go": true, "mobile_input_stub.go": true,
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		base := filepath.Base(f)
		if seam[base] || strings.HasSuffix(base, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := banned.FindIndex(b); loc != nil {
			t.Errorf("%s calls a low-level native-gesture registrar directly (offset %d) — "+
				"route it through a producer in native_gesture.go instead", base, loc[0])
		}
	}
}
```

- [ ] **Step 2: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNativeGesture_NoDirectRegistrarCallsOutsideSeam`
Expected: PASS (Tasks 4-6 removed every out-of-seam call). If it FAILS, it names the file still calling a registrar directly — migrate that call into a producer, then re-run.

- [ ] **Step 3: Write the occlusion invariant test**

Add to `native_gesture_test.go`:

```go
func TestNativeGesture_OccludedWidgetNotArmed(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)
	dv := g.drum

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	// Open the overflow menu (a blocking scrim portal). Any non-menu native rect
	// (e.g. a BPM native input behind the menu) must be suppressed because the
	// menu owns input — TopmostOwnerAt returns "overflow-menu", not the widget.
	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()

	for _, r := range dv.lastNativeRects {
		if r.Intent.ID == "bpm" {
			t.Fatal("BPM native rect armed while a blocking overlay is open — occlusion gate failed")
		}
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNativeGesture_OccludedWidgetNotArmed`
Expected: PASS (the BPM box sits behind the scrim menu; `TopmostOwnerAt(bpmCenter) == "overflow-menu"` ≠ the BPM owner → suppressed).

- [ ] **Step 5: Full regression sweep**

Run the input-isolation, overflow, template, and mobile suites together:
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'NativeGesture|Overflow|Template|FilePicker|SoftKeyboard|MobileNative|MobileText|ZoneInvisible|TabIsolation|Rename|ColorButton'
```
Expected: all PASS except the branch's documented pre-existing failures.

- [ ] **Step 6: Checkpoint** — leave dirty; confirm green.

---

## Self-Review Notes

- **Spec coverage:** Types + seam (Task 1) ↔ spec Components 1,5. `TopmostOwnerAt` (Task 2) ↔ Components 2. Sync phase + single call site (Task 3) ↔ Components 3,4 + Data flow. File-picker producer + bug fix (Task 4) ↔ Components 6 + Problem. Soft-keyboard (Task 5) + native-input incl. trigger split (Task 6) ↔ Components 6 + trigger/input note. Discipline + occlusion tests (Task 7) ↔ Testing 2,3 + Isolation invariants.
- **Placeholders:** `"transport"`, `"inst-menu"`, `"naming"`, `"context-menu"` are owner IDs the implementer must confirm by grep before writing (each task states exactly how). These are deliberately unresolved because the exact zone/overlay ID constant must be read from the codebase at implementation time, not guessed — the plan states the resolution procedure precisely, which is the anti-guess requirement.
- **Type consistency:** `NativeRect{Rect,Intent}`, `NativeRectCandidate{Owner,Rect,Intent}`, `NativeIntent{Channel,ID,Accept,InputMode,Text,MaxLen,InputRect}`, `platformSyncNativeRects([]NativeRect)`, `TopmostOwnerAt(int,int) string`, `topOwnerAt(int,int) string`, `collectNativeRectCandidates()/syncNativeGestures()`, producers `filePickerCandidates/softKeyboardCandidates/nativeInputCandidates` — names are identical across all tasks.
```
