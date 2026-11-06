# File-Menu / Nav-Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the navigation menu from the file-options menu — file options open from a new folder icon beside the grid settings gear; navigation opens from a restyled, pressable beat/time counter; remove the transport bullseye + kebab buttons (freeing two mobile slots). Desktop and mobile behave the same.

**Architecture:** Reuse the single global `OverlayPortal` (`dv.portal()`) with a new `overflowMode` (Nav | File) that selects content and anchor. The file menu is pixel-anchored under the folder button and screen-clamped (it can paint into the grid pane because the drum pane draws after the grid pane, and the scrim portal owns all input while open). Add a `gridFileBtn` mirroring the existing `gridHelpBtn` gear infrastructure.

**Tech Stack:** Go 1.26, Ebiten 2.9, project `internal/ui` package, `-tags test` fast test path.

## Global Constraints

- Build/test with the bundled Go only: `.tools/go/bin/go`.
- Fast test command: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`.
- **Never stage or commit** (project convention `feedback_never_stage_or_commit`). Each task ends by running its tests; do NOT `git add`/`git commit`. The "Verify" step replaces the usual commit step.
- **TDD**: write the failing test first, watch it fail, implement minimally, watch it pass.
- **Design-token discipline**: no new inline `color.RGBA{}`/`color.NRGBA{}` literals in `internal/ui/`; use `Token*()` accessors / existing `col*` vars. Forbidden chrome glyphs — use `IconID` + `DrawIcon`.
- **Both platforms**: every feature works under desktop (`Profile().IsMobile()==false`) and mobile. Tests use `SetRuntimeProfileForTest` / mobile profile helpers already present in the suite.
- Execute **inline** (not via dispatched subagents): the working tree carries extensive uncommitted WIP, and `feedback_subagents_unsafe_in_concurrent_tree` warns dispatched agents can delete untracked files.

---

### Task 1: Add the `IconFolder` glyph

**Files:**
- Modify: `src/go/internal/ui/icons.go` (enum + dispatch case)
- Modify: `src/go/internal/ui/drawing_icons.go` (new `drawFolderIcon`)
- Test: `src/go/internal/ui/icons_extras_test.go` (add a case)

**Interfaces:**
- Produces: `IconFolder IconID = "folder"`; `func drawFolderIcon(dst *ebiten.Image, r image.Rectangle, col color.Color)`.

- [ ] **Step 1: Write the failing test** — add to `icons_extras_test.go`:

```go
func TestIconFolderRenders(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	if ok := DrawIcon(dst, IconFolder, image.Rect(0, 0, 24, 24), color.White); !ok {
		t.Fatal("DrawIcon(IconFolder) returned false — icon not dispatched")
	}
	// The glyph must paint at least one non-transparent pixel.
	painted := false
	for y := 0; y < 24 && !painted; y++ {
		for x := 0; x < 24; x++ {
			if _, _, _, a := dst.At(x, y).RGBA(); a > 0 {
				painted = true
				break
			}
		}
	}
	if !painted {
		t.Fatal("IconFolder painted nothing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestIconFolderRenders`
Expected: FAIL (`undefined: IconFolder`).

- [ ] **Step 3: Implement** — in `icons.go`, add the const after `IconTemplate`/`IconSearch` group:

```go
	IconFolder IconID = "folder"
```

Add the dispatch case in `drawIconByID` (near `IconTemplate`):

```go
	case IconFolder:
		drawFolderIcon(dst, r, col)
```

In `drawing_icons.go`, add a Lucide-style folder (tab + body) mirroring the stroke conventions of the neighbouring icons (use `newIconCanvas`, `iconStrokeOpenRoundedRect`, `IconStrokeWeight`, `IconCornerRadius`):

```go
// drawFolderIcon renders a folder: a body rounded-rect with a raised left tab.
func drawFolderIcon(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	c := newIconCanvas(r)
	// Tab across the top-left third.
	iconStrokeLine(c, 3, 6, 9, 6, col)
	iconStrokeLine(c, 9, 6, 11, 8, col)
	// Body.
	iconStrokeOpenRoundedRect(dst, c, 3, 6, 18, 13, float32(IconCornerRadius), "", col)
}
```

(If `iconStrokeLine` is not the local helper name, mirror the exact primitive used by an existing multi-segment icon such as `drawTemplateIcon` — read it first and match its helper calls.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestIconFolderRenders`
Expected: PASS.

- [ ] **Step 5: Verify (no commit)**

Run the icons test group: `... -run 'TestIcon|TestFolder' ./internal/ui/`. Confirm PASS.

---

### Task 2: Split the overflow portal into Nav / File modes

**Files:**
- Modify: `src/go/internal/ui/drumview.go` (add `overflowMode` field + enum)
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`overflowItems`, `overflowNavPanelH`)
- Modify: `src/go/internal/ui/drumview_open_dialogs.go` (`OpenNavigation`, new `OpenFileMenu`)
- Test: `src/go/internal/ui/file_nav_menu_split_test.go` (new)

**Interfaces:**
- Produces: `overflowMenuMode` type with `overflowModeNav`, `overflowModeFile`; `dv.overflowMode overflowMenuMode`; `func (dv *DrumView) OpenFileMenu()`. `OpenNavigation()` sets nav mode.
- Consumes: existing `overflowItems`, `overflowNavPanelH`, `OpenOverflowMenu`.

- [ ] **Step 1: Write the failing test** — create `file_nav_menu_split_test.go`:

```go
package ui

import "testing"

func TestNavMenuHasNoFileRows(t *testing.T) {
	dv := newTestDrumView(t) // existing helper used across drumview tests
	dv.OpenNavigation()
	if dv.overflowNavPanelH() <= 0 {
		t.Fatal("nav menu must reserve the NAVIGATION panel band")
	}
	for _, it := range dv.overflowItems() {
		if it.header || it.search {
			continue
		}
		t.Fatalf("nav menu must have no file rows; found row %q", it.label)
	}
}

func TestFileMenuHasNoNavBand(t *testing.T) {
	dv := newTestDrumView(t)
	dv.OpenFileMenu()
	if dv.overflowNavPanelH() != 0 {
		t.Fatal("file menu must NOT reserve the NAVIGATION panel band")
	}
	var labels []string
	for _, it := range dv.overflowItems() {
		if it.header || it.search {
			continue
		}
		labels = append(labels, it.label)
	}
	if len(labels) == 0 {
		t.Fatal("file menu must list the file actions")
	}
}
```

(Confirm the drum-view construction helper name by grepping the test files, e.g. `grep -rn "func newTestDrumView\|func newDrumViewForTest\|makeTestDrumView" internal/ui/*_test.go`, and use whichever exists.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestNavMenuHasNoFileRows|TestFileMenuHasNoNavBand'`
Expected: FAIL (`OpenFileMenu` undefined; nav menu still returns file rows).

- [ ] **Step 3: Implement.**

In `drumview.go`, near the `overflowPage` field, add:

```go
	overflowMode overflowMenuMode // Nav vs File content for the shared overflow portal
```

and the type (top-level, near other small enums in the file):

```go
type overflowMenuMode int

const (
	overflowModeNav overflowMenuMode = iota // navigation panel only (from the beat counter)
	overflowModeFile                        // file actions (from the grid folder button)
)
```

In `drumview_context_menu.go`, gate the file list on file mode. At the top of `overflowItems()` page-0 branch (the `items := []overflowItem{ ... File group ... }` block at ~1023), return early for nav mode:

```go
	if dv.overflowMode == overflowModeNav {
		// Navigation menu: the embedded NavigationPanel band IS the whole body;
		// no file rows. (overflowNavPanelH returns the band height in nav mode.)
		return nil
	}
```

Change `overflowNavPanelH()` (~791) to key on mode, not page:

```go
func (dv *DrumView) overflowNavPanelH() int {
	if dv.overflowMode != overflowModeNav || dv.navPanel == nil {
		return 0
	}
	return dv.navPanel.PanelHeight()
}
```

(The templates page-1 branch stays as-is; it is only reachable in file mode, so no extra guard is needed — but add `if dv.overflowMode != overflowModeFile { ... }` protection only if a test shows page 1 leaking into nav mode. Nav mode never sets page 1.)

In `drumview_open_dialogs.go`, set the mode in `OpenNavigation` and add `OpenFileMenu`:

```go
func (dv *DrumView) OpenNavigation() {
	dv.overflowMode = overflowModeNav
	dv.overflowPage = 0
	dv.OpenOverflowMenu()
}

// OpenFileMenu opens the file-actions menu (Upload / Import / Export / Load
// Template) pixel-anchored under the grid-pane folder button. Reuses the shared
// overflow portal in File mode (no NAVIGATION band).
func (dv *DrumView) OpenFileMenu() {
	dv.overflowMode = overflowModeFile
	dv.overflowPage = 0
	dv.OpenOverflowMenu()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestNavMenuHasNoFileRows|TestFileMenuHasNoNavBand'`
Expected: PASS.

- [ ] **Step 5: Verify (no commit)** — run the existing overflow tests to catch regressions from the page→mode change:

Run: `... -run 'TestOverflow|TestNav' ./internal/ui/`. Fix any that assumed `overflowPage`-keyed nav band (repoint them to set `overflowMode`).

---

### Task 3: Mode-aware anchor + screen clamp (`overflowPopupRect`)

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`overflowPopupRect`, new `overflowAnchorRect`)
- Test: `src/go/internal/ui/file_nav_menu_split_test.go` (extend)

**Interfaces:**
- Produces: `func (dv *DrumView) overflowAnchorRect() image.Rectangle`.
- Consumes: `dv.game.gridFileButtonRect()` (Task 4 provides it; for this task, add a temporary `image.Rectangle{}` fallback so it compiles, then Task 4 fills it).

> Ordering note: this task references `gridFileButtonRect`. To avoid a forward
> dependency, add `overflowAnchorRect` now using `dv.beatCounterRect` for nav
> mode and a best-effort file anchor that falls back to the drum-pane top-right
> until Task 4 wires the real folder rect. Task 4's test locks the real anchor.

- [ ] **Step 1: Write the failing test** — extend the split test:

```go
func TestNavMenuAnchoredUnderBeatCounter(t *testing.T) {
	dv := newTestDrumView(t)
	dv.OpenNavigation()
	pop := dv.overflowPopupRect()
	bc := dv.beatCounterRect
	if bc.Empty() {
		t.Skip("beat counter rect not laid out in this harness")
	}
	// Popup drops below the counter (or flips above): its horizontal span must
	// overlap the counter's column, and it must sit adjacent (top within a row
	// height of the counter's bottom, or bottom adjacent to its top).
	if pop.Min.X > bc.Max.X || pop.Max.X < bc.Min.X {
		t.Fatalf("nav popup %v not horizontally adjacent to beat counter %v", pop, bc)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `... -run TestNavMenuAnchoredUnderBeatCounter`
Expected: FAIL (popup still anchored to the removed kebab / wrong column).

- [ ] **Step 3: Implement.** Add `overflowAnchorRect` and rewrite the anchor line in `overflowPopupRect`:

```go
// overflowAnchorRect returns the trigger rect the overflow popup drops from:
// the grid-pane folder button in File mode, the beat counter in Nav mode.
func (dv *DrumView) overflowAnchorRect() image.Rectangle {
	if dv.overflowMode == overflowModeFile {
		if dv.game != nil {
			if r := dv.game.gridFileButtonRect(); !r.Empty() {
				return r
			}
		}
		// Fallback until the folder button exists: drum-pane top-right.
		return image.Rect(dv.Bounds.Max.X-1, dv.Bounds.Min.Y, dv.Bounds.Max.X, dv.Bounds.Min.Y+1)
	}
	return dv.beatCounterRect
}
```

In `overflowPopupRect`, replace the nil-guard and `anchor := dv.overflowBtn().Rect()`:

```go
	anchor := dv.overflowAnchorRect()
	if anchor.Empty() {
		return image.Rectangle{}
	}
```

and, for File mode, clamp to the **screen** so the popup may paint into the grid pane. After computing `w`/`h`, choose the clamp box by mode:

```go
	clampBox := dv.Bounds
	if dv.overflowMode == overflowModeFile && dv.game != nil {
		clampBox = image.Rect(0, 0, dv.game.winW, dv.game.winH)
	}
```

Then use `clampBox` in place of `dv.Bounds` for the desktop-branch min/max X clamps, the flip-above test, the top clamp, and the height cap. (Keep the mobile `UseBottomSheet` branch anchored to `dv.Bounds.Max.Y` — a bottom sheet is still valid there; adjacency is covered by Task 6's DESIGN rule text. If a mobile file-menu-under-button variant is desired later, extend then.)

Delete/replace the old `if dv.overflowBtn() == nil { return image.Rectangle{} }` guard at the function top.

- [ ] **Step 4: Run test to verify it passes**

Run: `... -run TestNavMenuAnchoredUnderBeatCounter`
Expected: PASS.

- [ ] **Step 5: Verify (no commit)** — `... -run 'TestOverflow|TestNav|TestFileMenu' ./internal/ui/`.

---

### Task 4: Folder button beside the settings gear

**Files:**
- Modify: `src/go/internal/ui/game_struct.go` (add `gridFileBtn`)
- Modify: `src/go/internal/ui/game_new.go` (construct it)
- Modify: `src/go/internal/ui/game_input_shortcuts.go` (add `gridFileButtonRect`, `openFileMenu`)
- Create: `src/go/internal/ui/grid_file_input_handler.go` (mirror `grid_help_input_handler.go`)
- Modify: `src/go/internal/ui/grid_pane_draw.go` (`drawGridHelpButton` → also draw file btn, or add `drawGridFileButton`)
- Modify: `src/go/internal/ui/game_layout.go` (layout sync)
- Modify: `src/go/internal/ui/grid_reserved_zone.go` (`gridHelpReservedRect` → union)
- Modify: wherever `gridHelpInputHandler` is registered in the grid dispatcher (grep `gridHelpInputHandler{`), register `gridFileInputHandler` too.
- Test: `src/go/internal/ui/grid_file_button_test.go` (new)

**Interfaces:**
- Produces: `g.gridFileBtn *Button`; `func (g *Game) gridFileButtonRect() image.Rectangle`; `func (g *Game) openFileMenu()`; `type gridFileInputHandler struct{ g *Game }`.

- [ ] **Step 1: Write the failing test** — `grid_file_button_test.go`:

```go
package ui

import (
	"testing"
)

func TestGridFileButtonPresentBothPlatforms(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		g := newTestGame(t, mobile) // use the suite's Game constructor/helper
		if g.gridFileBtn == nil {
			t.Fatalf("mobile=%v: gridFileBtn is nil", mobile)
		}
		if g.gridFileBtn.Icon != string(IconFolder) {
			t.Fatalf("mobile=%v: folder icon=%q want %q", mobile, g.gridFileBtn.Icon, IconFolder)
		}
		gear := g.gridHelpButtonRect()
		file := g.gridFileButtonRect()
		if file.Empty() || gear.Empty() {
			t.Fatalf("mobile=%v: rects empty gear=%v file=%v", mobile, gear, file)
		}
		if file.Overlaps(gear) {
			t.Fatalf("mobile=%v: file %v overlaps gear %v", mobile, file, gear)
		}
		if file.Max.X > gear.Min.X {
			t.Fatalf("mobile=%v: file button must sit LEFT of the gear (file=%v gear=%v)", mobile, file, gear)
		}
		if file.Dx() != gear.Dx() || file.Dy() != gear.Dy() {
			t.Fatalf("mobile=%v: file/gear size mismatch file=%v gear=%v", mobile, file, gear)
		}
	}
}

func TestGridFileButtonOpensFileMenu(t *testing.T) {
	g := newTestGame(t, false)
	g.openFileMenu()
	if g.drum.overflowMode != overflowModeFile {
		t.Fatal("openFileMenu did not set File mode")
	}
	if !g.drum.IsOverflowMenuOpen() {
		t.Fatal("openFileMenu did not open the overflow portal")
	}
}
```

(Match the suite's Game construction helper — grep `func newTestGame\|func newGameForTest\|setupTestGame` in `internal/ui/*_test.go` and adapt. If the helper doesn't take a `mobile` bool, wrap with `SetRuntimeProfileForTest`/mobile profile setter used elsewhere.)

- [ ] **Step 2: Run test to verify it fails**

Run: `... -run 'TestGridFileButton'`
Expected: FAIL (`gridFileBtn` undefined).

- [ ] **Step 3: Implement.**

`game_struct.go` — add beside `gridHelpBtn`:

```go
	gridFileBtn *Button // folder button left of the settings gear (opens the file menu; both platforms)
```

`game_new.go` — after the `gridHelpBtn` construction (~149):

```go
	g.gridFileBtn = NewSpecButton("", ComponentButtonSecondary, g.openFileMenu)
	g.gridFileBtn.Icon = string(IconFolder)
```

`game_input_shortcuts.go` — add rect + opener (mirror `gridHelpButtonRect`):

```go
// gridFileButtonRect is the folder button, one SpaceSM gap to the LEFT of the
// settings gear in the grid pane's top-right corner. Same size as the gear.
func (g *Game) gridFileButtonRect() image.Rectangle {
	gear := g.gridHelpButtonRect()
	if gear.Empty() {
		return image.Rectangle{}
	}
	w := gear.Dx()
	x1 := gear.Min.X - SpaceSM
	return image.Rect(x1-w, gear.Min.Y, x1, gear.Max.Y)
}

// openFileMenu opens the file-actions menu from the grid folder button.
func (g *Game) openFileMenu() {
	if g.drum == nil {
		return
	}
	g.drum.OpenFileMenu()
}
```

`grid_file_input_handler.go` (new) — copy `grid_help_input_handler.go` verbatim, rename to `gridFileInputHandler`, point `InputBounds` at `gridFileButtonRect()`, forward to `g.gridFileBtn`, use a z one below the gear:

```go
package ui

import "image"

const gridFileInputZ = gridHelpInputZ - 1 // just below the gear; rects are disjoint

type gridFileInputHandler struct{ g *Game }

func (h gridFileInputHandler) InputBounds() image.Rectangle { return h.g.gridFileButtonRect() }
func (h gridFileInputHandler) ZIndex() int                  { return gridFileInputZ }

func (h gridFileInputHandler) HandleInput(x, y int, pressed bool) InputResult {
	g := h.g
	if g.gridFileBtn == nil {
		return InputIgnored
	}
	g.gridFileBtn.SetRect(g.gridFileButtonRect())
	res := g.gridFileBtn.HandleInputResult(x, y, pressed)
	if res == InputConsumed {
		g.gridHelpCapturing = true // reuse the same defer-tree-close flag as the gear
	}
	return res
}

func (h gridFileInputHandler) HandleWheel(x, y, steps int) InputResult { return InputIgnored }
func (h gridFileInputHandler) Capturing() bool                         { return false }
```

Register it where `gridHelpInputHandler{g}` is registered (grep for it — likely `game_new.go` or an `inputDispatcher.Register` site). Add `dispatcher.Register(gridFileInputHandler{g})` alongside.

`grid_pane_draw.go` — in `drawGridHelpButton`, also draw the file button:

```go
	if g.gridFileBtn != nil {
		g.gridFileBtn.SetRect(g.gridFileButtonRect())
		g.gridFileBtn.Draw(dst)
	}
```

`game_layout.go` — after the `gridHelpBtn.SetRect(...)` sync (~162):

```go
	if g.gridFileBtn != nil {
		g.gridFileBtn.SetRect(g.gridFileButtonRect())
	}
```

`grid_reserved_zone.go` — extend the reserved rect to the union:

```go
func (g *Game) gridHelpReservedRect() image.Rectangle {
	gear := g.gridHelpButtonRect()
	if gear.Empty() {
		return image.Rectangle{}
	}
	zone := gear
	if file := g.gridFileButtonRect(); !file.Empty() {
		zone = zone.Union(file)
	}
	return zone.Inset(-gridHelpReservedMargin)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `... -run 'TestGridFileButton'`
Expected: PASS.

- [ ] **Step 5: Verify (no commit)** — run gear/settings + reserved-zone tests: `... -run 'TestSettings|TestGridHelp|TestSettingsGear|TestGroupMenu' ./internal/ui/`. Confirm the union didn't break existing avoidance tests; adjust any that hard-coded the single-gear zone width.

---

### Task 5: Remove the transport bullseye + kebab buttons

**Files:**
- Modify: `src/go/internal/ui/transport_zone.go` (fields, `initButtons`, styles, `layoutMobile`, `layoutDesktop`, `rebuildHitAreas`, draw, dirty hash, expand rects)
- Modify: `src/go/internal/ui/drumview_ctor.go` (remove `OnNavigate`/`OnOverflowOpen` wiring)
- Test: `src/go/internal/ui/transport_slot_removal_test.go` (new); update `transport_nav_button_test.go`, `transport_icon_uniqueness_test.go`

**Interfaces:**
- Removes: `z.navBtn`, `z.overflowBtn`, callbacks `OnNavigate`, `OnOverflowOpen`, hit tags `transport-nav`, `transport-overflow`.

- [ ] **Step 1: Write the failing test** — `transport_slot_removal_test.go`:

```go
package ui

import "testing"

func TestTransportHasNoNavOrOverflowHitAreas(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		z := newTestTransportZone(t, mobile) // suite helper; lay it out
		for _, ha := range z.HitAreas() {
			if ha.Tag == "transport-nav" || ha.Tag == "transport-overflow" {
				t.Fatalf("mobile=%v: %q hit area must be gone", mobile, ha.Tag)
			}
		}
	}
}
```

(Grep the transport tests for the exact zone construction + layout helper and hit-area accessor names; e.g. `grep -rn "transport-nav\|newTestTransport\|\.HitAreas()" internal/ui/*_test.go`. Reuse them.)

- [ ] **Step 2: Run test to verify it fails**

Run: `... -run TestTransportHasNoNavOrOverflowHitAreas`
Expected: FAIL (both hit areas still present).

- [ ] **Step 3: Implement.** In `transport_zone.go`:

1. Delete the `navBtn` (line ~59) and `overflowBtn` (~73) struct fields.
2. In `initButtons`, delete the `z.navBtn = NewButton(...)` block (~211-217) and the `z.overflowBtn = NewButton(...)` block (~321-327).
3. Delete the style assignments (`z.navBtn.Style`/`IconColor` ~356-358; `z.overflowBtn.Style` ~384-392).
4. `layoutMobile`: delete the `z.navBtn`/`z.overflowBtn` rect sets in the two-row branch (~741-746) and single-row branch (~823-828). Change the two-row row-1 grid to a single Vol cell:

```go
		row1Grid := NewGridLayout(row1Bounds, []float64{1.0}, []float64{1})
		z.mainVolIconRect = safeInsetTransport(row1Grid.Cell(0, 0), pad)
```

Change the single-row weights (~762-765):

```go
	weights := []float64{1.0, 1.0, 1.0, 3.0, 1.0}
	if !useTwoRow {
		weights = []float64{1.0, 1.0, 1.0, 3.0, 1.0, 1.0} // + Vol only (Nav/Overflow removed)
	}
```

and the single-row append (~817-828) becomes just Vol at cell 5:

```go
	if !useTwoRow {
		z.mainVolIconRect = safeInsetTransport(row0Grid.Cell(5, 0), pad)
		if z.mainVolSlider != nil {
			z.mainVolSlider.SetRect(image.Rectangle{})
			z.mainVolRect = image.Rectangle{}
		}
	}
```

5. `layoutDesktop`: delete `square(z.navBtn)` (~914) and `square(z.overflowBtn)` (~926). Update the run constants (~885-886):

```go
	const nGaps = 8 // 9 controls in the run → 8 inter-control gaps (Nav + Overflow removed)
	totalUnits := 7*1.0 + transportBPMBoxUnits + transportBPMStepUnits
```

Update the running-order comment to drop Nav and Overflow.

6. `rebuildHitAreas`: delete the `{z.navBtn, "transport-nav", true}` and `{z.overflowBtn, "transport-overflow", true}` entries from `simpleButtons` (~972, ~979).
7. Delete the dirty-hash mixing blocks for `navBtn` (~1323-1325) and `overflowBtn` (~1352-1357), and the expand-rect blocks (~1427-1428, ~1442-1443) and any `drawBtnOff` references (~1524-1525) — grep `navBtn`/`overflowBtn` in the file and remove every remaining reference.

In `drumview_ctor.go`: delete `OnNavigate` (~831) and `OnOverflowOpen` (~838) from the transport callbacks struct literal. Delete the callback fields from the `TransportCallbacks`/`TopBarCallbacks` struct definition (grep `OnNavigate`/`OnOverflowOpen`).

- [ ] **Step 4: Run test to verify it passes**

Run: `... -run TestTransportHasNoNavOrOverflowHitAreas`
Expected: PASS.

- [ ] **Step 5: Verify (no commit)** — build + transport tests:

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDesktopTransport|TestMobileTransport|TestTransport'`
Fix fallout: `transport_nav_button_test.go` (delete or repoint to the beat-counter path — the Navigate button no longer exists), `transport_icon_uniqueness_test.go` (drop `IconTarget`/`IconOverflow` from the transport icon set), any `TestDesktopTransportNoSpacerGap`/`AreEqualSquares` count assumptions (now 9 controls), and mobile touch-min ratchet tests referencing 7/8 cells.

---

### Task 6: Beat/time counter → pressable keycap

**Files:**
- Modify: `src/go/internal/ui/timeline_zone.go` (`drawBeatCounter`, beat-counter hit adapter hover/press state)
- Modify: `src/go/internal/ui/drumview_layout.go` (widen `beatCounterRect`/`beatCounterSlotWidth` for the leading icon)
- Test: `src/go/internal/ui/beat_counter_keycap_test.go` (new); update `beat_counter_chip_test.go`, `square_chips_test.go`

**Interfaces:**
- Consumes: existing `beatCounterHitAdapter`, `BeatCounterRect` callback, `timelineInfoCached`.

- [ ] **Step 1: Write the failing test** — `beat_counter_keycap_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestBeatCounterDrawsLeadingIcon(t *testing.T) {
	z := newTestTimelineZone(t) // suite helper
	dst := ebiten.NewImage(400, 60)
	// Count DrawIcon calls inside the counter rect via the test interceptor
	// (mirror the pattern already used by beat_counter_chip_test.go).
	icons := iconsDrawnInside(t, z.callbacks.BeatCounterRect(), func() {
		z.drawBeatCounter(dst, 0)
	})
	found := false
	for _, id := range icons {
		if id == IconTarget {
			found = true
		}
	}
	if !found {
		t.Fatal("beat counter must draw a leading IconTarget affordance")
	}
	_ = image.Rect // keep import if unused
}
```

(Use whatever icon-capture harness the suite already has; `beat_counter_chip_test.go` already intercepts `drawRect` via `collectFilledRects` — add or reuse an equivalent `DrawIcon` interceptor. If none exists, assert on the leading-icon sub-rect being non-empty instead.)

- [ ] **Step 2: Run test to verify it fails**

Run: `... -run TestBeatCounterDrawsLeadingIcon`
Expected: FAIL (no icon drawn today).

- [ ] **Step 3: Implement.** In `timeline_zone.go`:

Add hover/press state to the beat-counter hit adapter (it already receives press). Store `beatCounterHovered`/`beatCounterPressed` on the zone; set them in the adapter's `OnPress`/`OnRelease`/hover and clear on leave.

Rewrite `drawBeatCounter` to render a keycap: raised fill + border, hover-glow when `beatCounterHovered`, pressed inset when `beatCounterPressed`, and a leading `IconTarget` before the text. Reuse the existing keycap/button visual helpers rather than raw literals (mirror how `subdivBtn`/`DropdownStyle` keycaps draw — grep `DropdownStyle`/`drawKeycap`/`ButtonVisual`). Reserve `menuRowIconSize()`-ish leading space for the icon, then draw `info` text after it. Keep the pill/rect clamps that guard against timeline-bar bleed.

In `drumview_layout.go`, widen `beatCounterRect` and the `beatCounterSlotWidth` bound strings by the icon width + gap so the readout never truncates (desktop `"Beat 888 · 88:88"`, mobile `"888 · 8:88"`).

- [ ] **Step 4: Run test to verify it passes**

Run: `... -run TestBeatCounterDrawsLeadingIcon`
Expected: PASS.

- [ ] **Step 5: Verify (no commit)** — `... -run 'TestBeatCounter|TestSquareChips|TestNotif' ./internal/ui/`. Update `beat_counter_chip_test.go`/`square_chips_test.go` expectations for the new keycap fill/border and the leading-icon offset.

---

### Task 7: DESIGN.md — menu-adjacency rule

**Files:**
- Modify: `DESIGN.md` (add the rule near the Context-menu / Bottom-sheet section, ~line 2378)
- Test: `src/go/internal/ui/design_md_drift_test.go` is token-only (won't fail on prose); the lint snapshot `design_md_lint_test.go` may need refresh.

- [ ] **Step 1:** Add the rule after the "Bottom sheet (mobile)" / "Context menu" block:

```markdown
**Menu placement — adjacent to trigger.** A popup menu opens adjacent to the
control that triggered it, dropping below the trigger (flipping above when there
is no room below), so the menu's visual origin is unambiguous. Applies to
floating dropdown / context menus on both platforms. The mobile full-width
bottom sheet remains the form for large modal menus, but even then it is
invoked from — and reads as originating from — its trigger control.
```

- [ ] **Step 2: Run the lint snapshot** (may need refresh):

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDesignMDLintSnapshot|TestDesignMDDrift'`
Expected: PASS. If the lint warning set changed, update `testdata/design_md_lint.expected.json` to match (only if the delta is our added prose, not a regression).

- [ ] **Step 3: Verify (no commit).**

---

### Task 8: Integration sweep + visual verification

**Files:** whole `internal/ui/` package; scene catalog (`scene_catalog.go` scenes `mobile_overflow_open`, `overflow_menu_open`).

- [ ] **Step 1:** Update the two overflow scenes to the new openers: `overflow_menu_open` → `dv.OpenFileMenu()`; add/rename a nav scene driven by `dv.OpenNavigation()`. Update any scene that clicked the removed kebab/bullseye.

- [ ] **Step 2: Run the full fast suite:**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS (fix any remaining references to `navBtn`/`overflowBtn`/`overflowPage`-keyed nav band).

- [ ] **Step 3: Build for WASM** (catch `//go:build js` breakage):

Run: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -tags test ./internal/ui/`
Expected: success.

- [ ] **Step 4: Visual capture** — desktop + mobile:

Run: `make screenshots-all SCENES=overflow_menu_open,mobile_overflow_open` (plus a grid-pane scene showing the folder+gear, and a counter scene). Also `make screenshot`.
Inspect the PNGs in `screenshots/`: folder icon sits left of the gear; file menu drops under the folder icon; nav menu drops under the counter; counter reads as a pressable keycap with the target icon; mobile transport shows two freed slots.

- [ ] **Step 5: Report** — summarise pass/fail with the screenshot paths for the user to review. No commit.

---

## Self-Review

**Spec coverage:**
- Split nav/file → Task 2. ✓
- Remove kebab, folder icon opens file menu → Tasks 4, 5. ✓
- Remove bullseye, counter opens nav → Tasks 5, 6. ✓
- Free two mobile slots → Task 5 (weights `[1,1,1,3,1,1]`). ✓
- Desktop parity → Tasks 4–6 (both-platform tests). ✓
- Pixel-anchored file menu → Task 3 (screen clamp) + Task 4 (folder rect). ✓
- Keycap counter → Task 6. ✓
- DESIGN.md rule → Task 7. ✓
- TDD + visual → every task tests-first; Task 8 screenshots. ✓

**Placeholder scan:** Code shown for every implementation step; the few "grep the suite helper name" notes are because the test-harness constructor names must be read from the existing test files (they vary), not invented — each names the exact grep to run.

**Type consistency:** `overflowMenuMode`/`overflowModeNav`/`overflowModeFile`, `overflowMode`, `overflowAnchorRect`, `OpenFileMenu`, `gridFileBtn`, `gridFileButtonRect`, `openFileMenu`, `gridFileInputHandler`, `gridFileInputZ`, `IconFolder`/`drawFolderIcon` are used consistently across tasks.
