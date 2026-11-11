# Native-Gesture Rect Tree Isolation — Design

**Date:** 2026-07-09
**Status:** Approved (design), pending implementation plan
**Area:** `internal/ui` — input dispatch / DrumViewTree / mobile-WASM gesture channels

## Problem

On WASM mobile, tapping a **template row** in the overflow menu can open the
native **file picker**. The tap is on a legitimate template row, yet the OS
file-open dialog appears — a component "behind" the visible menu row leaks its
input handling.

### Root cause (evidence)

The mobile file picker is a **JS-side `touchend` gesture channel**
(`_fpRegisterRect` in JS, bound via `filepicker_wasm.go`). File pickers must be
opened synchronously inside a trusted user-gesture handler, so Go registers
canvas rects with JS; on `touchend` inside a registered rect, JS creates a file
input and clicks it. **This channel runs entirely outside the Go
`DrumViewTree` / `HitIndex` z-ordered dispatch.**

`registerFilePickerRects()` (`drumview_context_menu.go`) registers the
**Upload** (`.wav`) and **Import** (`.json`) rects at their **File-page**
positions when the overflow menu opens. When the user taps "Load template," the
menu flips to the **Templates page** (`dv.overflowPage = 1`) — but the
file-picker rects are **only cleared on menu _close_** (`filePickerClearRects()`
at `drumview_context_menu.go` closeOverflowMenu and `drumview_portal_open.go`
OnClose), never on the page switch. So the stale Upload/Import rects still sit
under the template rows, and a `touchend` there fires the picker. The Go tree
handled the tap correctly (the menu's buttons win at z≥300); the JS channel
fired independently.

### This is a bug class, not a one-off

There are **three** imperatively-registered, out-of-tree JS gesture channels,
none of them z-aware:

| Channel | Register fn | Binding | Cleared |
|---|---|---|---|
| File picker | `filePickerRegisterRect` | `filepicker_wasm.go` | on menu **close** only → **stale across page switch (the bug)** |
| Soft keyboard | `softKeyboardRegisterRect` | `softkeyboard_wasm.go` | rebuilt each `calcLayout` (`drumview_layout.go`) |
| Mobile native input | `mobileInputRegister*` | `mobile_input_wasm.go` | rebuilt each `calcLayout` |

The soft-keyboard and native-input channels rebuild each layout so they rarely
go stale, but they register **even when their widget is occluded by an open
overlay** — the same z-blindness that produced the file-picker leak. All three
share the defect: a rect's liveness is not tied to whether its owning component
is the topmost thing at that location.

### What already works (scope reduction)

The Go `DrumViewTree` **already** guarantees z-order + no-leak for in-tree
components. When any portal/overlay is open, a tap "behind" it takes the
click-outside path (`drumview_tree.go` ~646-651): the tree closes the overlay,
sets `suppress`, marks the press handled, and **returns without dispatching to
the component beneath**. Portal overlays register hit areas at `ZOverlayMin`
(300) + stack index and absorb input within their bounds; modal overlays set a
`HitIndex` modal filter that hides all non-modal areas. **No Go-side change is
needed.** The only leak is the three JS channels.

## Goal

Make the leak class **structurally impossible**: a native-gesture rect must be
armed **only** where its owning `HitArea` is the current **topmost** hit
(respecting portal z-order, occlusion, and modality). Then page switches,
occlusion, and modality all clear stale rects automatically, and no component
can reintroduce an out-of-tree rect.

## Design

### Core idea

Once per frame, after the `RootTree` has ticked every subtree (so all
per-subtree hit indexes are final), a single **native-gesture sync** phase:

1. **Pulls** the current native-rect *candidates* from a collector — a pure
   function of current UI state. Each candidate is `{Owner, Rect, Intent}` where
   `Owner` is the ownerID (zone id or overlay id) whose hit area sits at that
   rect, and `Intent` is the channel + params.
2. **Gates** each candidate by the composed dispatch precedence: a candidate
   survives iff `rootTree.TopmostOwnerAt(center) == candidate.Owner` — i.e. a
   fresh press at that point would actually be delivered to the owning
   component. `TopmostOwnerAt` reuses the exact routing order the dispatcher
   uses (capture-holder → blocking-portal owner → top-down by z, then that
   subtree's `HitIndex.At(...)[0].ownerID`).
3. **Diffs** the survivors against the previous frame's synced set; if unchanged,
   does nothing (no JS churn — arming/clearing every frame could interrupt an
   in-flight gesture).
4. **Syncs** the delta to JS via the platform seam.

The collector replaces the three imperative registration sites: instead of
calling `filePickerRegisterRect` / `softKeyboardRegisterRect` /
`mobileInputRegister*` directly, each site becomes a small pure function
returning candidates (`filePickerCandidates()`, `softKeyboardCandidates()`,
`nativeInputCandidates()`), so a native rect is never armed as a side effect of
layout.

Why `TopmostOwnerAt` rather than a `HitArea.Native` field: producers do not all
own a per-control hit area. The overflow menu registers a *single* full-rect
portal hit area (owner `"overflow-menu"`) and routes its buttons internally, and
the soft-keyboard/native-input rects are computed in `calcLayout` from widget
rects with platform gating. A pull collector + owner-match gate handles all
three shapes uniformly; a per-area field would only fit the zone-widget case.

Because native rects are now **derived** from live, topmost, modality-respecting
dispatch each frame:

- **Page switch** (File→Templates): the Upload/Import hit areas cease to exist →
  their picker rects vanish. **Reported bug fixed.**
- **Occlusion**: a hit area covered by a higher-z overlay is not topmost → not
  projected.
- **Modality**: the `HitIndex` modal filter hides non-modal areas → their native
  rects cannot project.
- **Staleness**: the set is rebuilt from the index every frame → nothing
  survives its owner's disappearance.

The manual close-button crop stays inside `filePickerCandidates()`: the overflow
menu is a single hit area (owner `"overflow-menu"`), so the crop is a
menu-internal detail, not derivable from z-order. Occlusion gating still applies
at whole-candidate granularity (a higher subtree's blocking overlay suppresses
the menu's candidates entirely).

### Components

1. **`NativeIntent` / `NativeRectCandidate` / `NativeRect`** (new
   `native_gesture.go`):

   ```go
   type NativeChannel int
   const (
       NativeFilePicker   NativeChannel = iota // opens OS file dialog on touchend
       NativeSoftKeyboard                       // focuses a soft-keyboard input rect (desktop browser)
       NativeTextInput                          // opens the mobile native text-input overlay
   )

   type NativeIntent struct {
       Channel   NativeChannel
       ID        string          // stable per-widget id: "import","upload","bpm","rename-3",...
       Accept    string          // file picker: ".wav" / "application/json,.json"
       InputMode string          // keyboard/text: "numeric" | "text"
       Text      string          // native text input: initial value
       MaxLen    int             // native text input: max length
       InputRect image.Rectangle // native text input trigger/input split: where the
                                  // <input> appears when the (gated) trigger rect is
                                  // tapped. Zero → same as the candidate Rect (direct
                                  // rect). Non-zero → NativeTextInput uses
                                  // mobileInputRegisterTrigger (e.g. rename: trigger =
                                  // Rename menu item, input = row-label rect).
   }

   // Producer output, pre-gating. Owner is the ownerID that TopmostOwnerAt must
   // return at Rect's center for this candidate to be armed. Rect is always the
   // TRIGGER rect (what the user taps) — the thing gated by z-order.
   type NativeRectCandidate struct {
       Owner  string
       Rect   image.Rectangle
       Intent NativeIntent
   }

   // Gated, screen-space output unit synced to JS.
   type NativeRect struct {
       Rect   image.Rectangle
       Intent NativeIntent
   }
   ```

2. **`RootTree.TopmostOwnerAt(x, y int) string`** (`root_tree.go`) — returns the
   ownerID a fresh press at `(x, y)` would resolve to, using the SAME precedence
   as `dispatchInput`: capture-holder → blocking-portal owner → top-down by z;
   within the chosen subtree, `hitIndex.At(x, y)[0].ownerID`. Returns `""` if the
   point resolves to nothing. Needs a small `(*DrumViewTree).topOwnerAt(x,y)`
   helper reading `t.hitIndex.At`.

3. **`(*DrumView).collectNativeRectCandidates() []NativeRectCandidate`**
   (`native_gesture.go`) — pure pull collector: concatenates
   `filePickerCandidates()`, `softKeyboardCandidates()`, `nativeInputCandidates()`.
   Each sub-collector is the current registration logic rewritten to *return*
   candidates instead of calling JS, preserving the existing platform gates
   (soft-keyboard `!IsMobile()`, native-input `IsMobile()`) and open-state guards.

4. **`(*DrumView).syncNativeGestures()`** (`native_gesture.go`) — the single
   per-frame phase: `collect → gate via rootTree.TopmostOwnerAt → diff vs
   dv.lastNativeRects → platformSyncNativeRects(delta)`. Stores the synced set on
   `dv.lastNativeRects` for the next diff. Called from exactly one site: the end
   of `RootTree.Update()` via a callback the DrumView installs (so the RootTree
   stays UI-agnostic), or directly from `(*DrumView).Update` right after
   `rootTree.Update()`. Chosen site: **`(*DrumView).Update`, immediately after
   `dv.rootTree.Update()`** (`drumview_update.go`) — one call, after all subtrees
   ticked and input dispatched.

5. **Platform seam `platformSyncNativeRects(rects []NativeRect)`**
   (`native_gesture_wasm.go` / `native_gesture_stub.go`) —
   - `_wasm.go`: groups by channel; for each channel calls its existing clear +
     per-rect register JS binding (`filePickerClearRects`/`filePickerRegisterRect`,
     `softKeyboardClearRects`/`softKeyboardRegisterRect`,
     `mobileInputClear`/`mobileInputRegister`). Low-level bindings and JS
     unchanged.
   - `_stub.go`: records the last synced `[]NativeRect` in a test-inspectable var.

6. **Call-site migration** — replace imperative registration with candidate
   production, deleting the direct arm/clear calls:
   - `registerFilePickerRects` (`drumview_context_menu.go`) → `filePickerCandidates()`
     (returns Upload/Import candidates owner `"overflow-menu"`, **only when the
     overflow menu is open on the File page**, i.e. `overflowPage == 0`). The
     `OnOverflowOpen` mobile `registerFilePickerRects()` call and every
     `filePickerClearRects()` call site are removed.
   - Soft-keyboard block in `drumview_layout.go` → `softKeyboardCandidates()`
     (BPM/inst-search/wav-name/rename, `!IsMobile()`). The `softKeyboardClearRects()`
     + `softKeyboardRegisterRect()` calls are removed.
   - `mobileInputRegister` (`param_value_editor.go`) + BPM block in
     `drumview_layout.go` → `nativeInputCandidates()` (`IsMobile()`). The
     `mobileInputClear()` call is removed.

The manual close-button crop stays inside `filePickerCandidates()` (menu-internal
detail): the menu is one hit area, so the crop is not derivable from z-order.

### Data flow

```
(*DrumView).Update:
  dv.rootTree.Update()                     // tick all subtrees, dispatch input — indexes final
  dv.syncNativeGestures():
     cands := dv.collectNativeRectCandidates()          // pure: file-picker + soft-kbd + native-input
     kept  := filter cands where rootTree.TopmostOwnerAt(center)==cand.Owner
     if kept != dv.lastNativeRects:                      // diff — skip on no change
        platformSyncNativeRects(kept)                    // wasm: per-channel clear+register | stub: record
        dv.lastNativeRects = kept
```

### Isolation invariants (the guarantee)

- **Topmost-only**: a native rect is armed only where its `HitArea` is the
  current topmost hit. (Occlusion + page-switch + modal all fall out of this.)
- **Single owner**: only the tree's `syncNativeGestures` → `platformSyncNativeRects`
  path may arm/clear native rects. No other production code calls the low-level
  `*RegisterRect` / `*Register*` / `*ClearRects` functions.
- **Derived, not stored**: the armed set is a pure function of the current hit
  index each frame; there is no independently mutable rect state to go stale.

## Testing

1. **Reported-bug functional regression** (the deliverable; RED before, GREEN
   after). Drive real `Game.Update` under mobile + browser runtime profile
   (`SetRuntimeProfileForTest(browserRuntimeProfile())` + `SetForceMobileProfile`).
   Open the overflow menu (File page) → assert a `NativeFilePicker` rect is armed
   over the Upload and Import rows. Switch to the Templates page → assert **no**
   file-picker rect is armed over any template row. Switch back → File-page rects
   return.

2. **Occlusion / modality invariant**. With a modal (or scrim) overlay open over
   a widget that carries a `Native` intent, assert the projection excludes that
   widget's rect (topmost-only holds under the modal filter).

3. **Discipline test** (makes the class un-reintroducible; mirrors
   `zone_input_isolation_discipline_test.go`). Assert no production file in
   `internal/ui` outside the platform seam (`native_gesture_*.go`) references the
   low-level `filePickerRegisterRect`, `softKeyboardRegisterRect`,
   `mobileInputRegister`, `mobileInputRegisterTrigger`, or their `*ClearRects`
   siblings.

4. **Channel-parity**. Existing browser tests that exercise the file picker,
   soft keyboard, and mobile native input (e.g. `mobile_file_picker.*`,
   `mobile_native_input.*`, `mobile_text_input.*`) still pass — the JS bindings
   and their behavior are unchanged; only the caller moved.

## Scope guard (YAGNI)

- **No** change to the Go dispatcher, portal modality, click-outside, `suppress`,
  or render path — all already correct.
- **No** new visual behavior; purely relocating native-rect ownership to the tree.
- **No** change to the JS-side gesture handlers or the low-level Go↔JS bindings.
- Partial-occlusion is handled at rect granularity via the topmost-at-point test;
  sub-rect clipping beyond what a higher-z sibling area already provides is out of
  scope (the close-button crop is covered because it is itself a hit area).

## Constraints / gotchas

- The `syncNativeGestures` phase must have exactly **one** call site in the tree
  (like the single Draw dispatch site) so it always runs against the freshly
  built index — a second caller would sync a stale set. Pin with a discipline
  test.
- Native rects are **screen-space**; the projection reads `HitArea.Rect` which is
  already screen-space (the same coordinates the imperative calls used).
- Diff-before-sync is required: the file-picker JS `clear+register` on every
  frame would be wasteful and could interrupt an in-flight gesture. Only sync on
  set change.
- Per project convention, this document is **not committed** (working tree left
  dirty; see the repo's no-stage/no-commit rule).
```
