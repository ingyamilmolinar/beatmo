# Armed Control — Reflow + Grow/Shrink Animation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement task-by-task. Steps use checkbox (`- [ ]`). Spec: `docs/superpowers/specs/2026-07-23-armed-control-reflow-animation-design.md`. Central-UI feature — every render task ends with a **screenshot verification** (green Go tests are NOT sufficient per CLAUDE.md).

**Goal:** When a synth control is armed, it grows to a **2× width × 2× height** footprint with a brief eased animation; surrounding controls **reflow (never hide)** — sliding to the side / wrapping below; locking reverses it with a shrink animation. Shared `growArmedCard`, so the stage editor and the macro drill-down behave identically.

**Architecture:** A per-frame eased progress `armGrowT` (advanced in `synthTabUpdate`, same loop as the barrel kinetics) drives a masonry reflow inside the rewritten `growArmedCard`: it computes the resting uniform rects AND a reflowed-target layout (armed control = 2×2 block, others packed into earliest free cells), then sets each control's rect to the `lerp` of the two by `smoothstep(armGrowT)`. Layout sets interpolated rects, so hit-testing follows the visual.

**Tech Stack:** Go 1.26, Ebitengine 2.9, `internal/ui` (`ControlGrid`, `synthPanel`, `DrumView`).

## Global Constraints

- **Bundled Go:** `.tools/go/bin/go`. Fast tests from `src/go`: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <RE> -count=1`.
- **Screenshots mandatory:** `CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd` then `xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>`; mobile via `make screenshots-all SCENES=<name> MOBILE=1` (writes `screenshots/all/mobile/<name>.browser.png`). Open every PNG.
- **Never stage or commit** — leave work in the tree.
- **2×2 target:** `spanCols = min(2, cols)`, `spanRows = 2`; the block rect is at least `max(2·cellW, SynthArmedCardW) × max(2·cellH, SynthArmedCardH)` so the two barrel lanes always fit, clamped to the pane.
- **Shared behavior:** the rewrite lives in `growArmedCard`; both the stage path (`synth_panel_zone.go ~:2205`) and `layoutSynthMacroLayer` call it.
- **Existing tokens:** `Profile().DensityValues().SynthArmedCardW / SynthArmedCardH` (ints). `synthMaxGridCols = 12`.

---

## Task 1: Easing helpers + grow-animation state (pure logic, TDD)

**Files:**
- Create: `src/go/internal/ui/synth_armed_grow_anim.go`
- Modify: `src/go/internal/ui/drumview.go` (4 fields), `src/go/internal/ui/synth_armed_control.go` (`armSynthControl`/`lockSynthControl` set the target)
- Test: `src/go/internal/ui/synth_armed_grow_anim_test.go`

**Interfaces produced:**
- On `*DrumView`: fields `armGrowIdx int`, `armGrowInst string`, `armGrowT float64`, `armGrowTarget float64`.
- `func (dv *DrumView) armGrowControl(instID string) (idx int, active bool)` — `active == armGrowIdx >= 0 && armGrowInst == instID`.
- `func (dv *DrumView) armGrowProgress() float64` — `smoothstep(clamp01(armGrowT))`.
- `func (dv *DrumView) advanceArmGrow() bool` — steps `armGrowT` toward `armGrowTarget`; returns `true` while animating; clears `armGrowIdx=-1` when target 0 reached.
- `func (dv *DrumView) setArmGrowSubject(instID string, idx int, target float64)`.
- Helpers: `func smoothstep(t float64) float64`, `func lerpInt(a, b int, t float64) int`, `func lerpRect(a, b image.Rectangle, t float64) image.Rectangle`.

- [ ] **Step 1: Failing tests** in `synth_armed_grow_anim_test.go`:

```go
package ui

import (
	"image"
	"testing"
)

func TestSmoothstepAndLerp(t *testing.T) {
	if smoothstep(0) != 0 || smoothstep(1) != 1 {
		t.Fatal("smoothstep endpoints must be 0 and 1")
	}
	if v := smoothstep(0.5); v <= 0.4 || v >= 0.6 {
		t.Fatalf("smoothstep(0.5) ~0.5, got %v", v)
	}
	if lerpInt(0, 10, 0.5) != 5 {
		t.Fatal("lerpInt midpoint")
	}
	a := image.Rect(0, 0, 10, 10)
	b := image.Rect(10, 10, 30, 30)
	if got := lerpRect(a, b, 0); got != a {
		t.Fatalf("lerpRect t=0 must equal a, got %v", got)
	}
	if got := lerpRect(a, b, 1); got != b {
		t.Fatalf("lerpRect t=1 must equal b, got %v", got)
	}
}

func TestArmGrow_EasesInThenOut(t *testing.T) {
	dv := &DrumView{}
	dv.setArmGrowSubject("inst", 3, 1) // arm
	if idx, active := dv.armGrowControl("inst"); !active || idx != 3 {
		t.Fatalf("armGrowControl want (3,true) got (%d,%v)", idx, active)
	}
	// Ease in: T climbs 0->1 over a bounded number of ticks.
	ticks := 0
	for dv.advanceArmGrow() {
		ticks++
		if ticks > 100 {
			t.Fatal("grow never settled")
		}
	}
	if dv.armGrowT < 0.999 {
		t.Fatalf("after settle armGrowT ~1, got %v", dv.armGrowT)
	}
	// Lock: target 0, subject retained until settled, then cleared.
	dv.setArmGrowSubject("inst", 3, 0)
	if _, active := dv.armGrowControl("inst"); !active {
		t.Fatal("subject must be retained through the shrink")
	}
	for dv.advanceArmGrow() {
	}
	if dv.armGrowT > 0.001 {
		t.Fatalf("after shrink armGrowT ~0, got %v", dv.armGrowT)
	}
	if _, active := dv.armGrowControl("inst"); active {
		t.Fatal("subject must clear once shrink settles")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`smoothstep` etc. undefined).
  `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSmoothstepAndLerp|TestArmGrow_EasesInThenOut' -count=1`

- [ ] **Step 3: Implement** `synth_armed_grow_anim.go`:

```go
package ui

import "image"

// armGrowStep is the per-frame progress delta (~6–7 frames ≈ 100–120 ms at 60 fps).
// Deterministic so tests can tick a bounded number of frames.
const armGrowStep = 0.16

func clamp01(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

func smoothstep(t float64) float64 {
	t = clamp01(t)
	return t * t * (3 - 2*t)
}

func lerpInt(a, b int, t float64) int {
	return a + int(float64(b-a)*t+0.5)
}

func lerpRect(a, b image.Rectangle, t float64) image.Rectangle {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return image.Rect(
		lerpInt(a.Min.X, b.Min.X, t), lerpInt(a.Min.Y, b.Min.Y, t),
		lerpInt(a.Max.X, b.Max.X, t), lerpInt(a.Max.Y, b.Max.Y, t),
	)
}

func (dv *DrumView) setArmGrowSubject(instID string, idx int, target float64) {
	if dv.armGrowIdx != idx || dv.armGrowInst != instID {
		// New subject: start from rest so it eases in from scratch.
		dv.armGrowT = 0
	}
	dv.armGrowIdx = idx
	dv.armGrowInst = instID
	dv.armGrowTarget = target
}

func (dv *DrumView) armGrowControl(instID string) (int, bool) {
	return dv.armGrowIdx, dv.armGrowIdx >= 0 && dv.armGrowInst == instID
}

func (dv *DrumView) armGrowProgress() float64 { return smoothstep(dv.armGrowT) }

func (dv *DrumView) advanceArmGrow() bool {
	if dv.armGrowIdx < 0 {
		return false
	}
	if dv.armGrowT == dv.armGrowTarget {
		if dv.armGrowTarget == 0 {
			dv.armGrowIdx = -1
			dv.armGrowInst = ""
		}
		return false
	}
	if dv.armGrowT < dv.armGrowTarget {
		dv.armGrowT = clamp01(dv.armGrowT + armGrowStep)
	} else {
		dv.armGrowT = clamp01(dv.armGrowT - armGrowStep)
	}
	if dv.armGrowT == dv.armGrowTarget && dv.armGrowTarget == 0 {
		dv.armGrowIdx = -1
		dv.armGrowInst = ""
	}
	return true
}
```

  Add the four fields to `DrumView` in `drumview.go` (near `instEditorArmedControl`), initializing `armGrowIdx` to `-1` in `NewDrumView` (so a zero-value `&DrumView{}` in a test still reads `0`; the test above sets it explicitly). **Also** set `armGrowIdx = -1` default: add `armGrowIdx: -1` to the `NewDrumView` composite literal.

- [ ] **Step 4: Wire arm/lock** in `synth_armed_control.go`: at the end of `armSynthControl` add `dv.setArmGrowSubject(instID, idx, 1)`; at the start of `lockSynthControl` (before deleting the map entry) add `dv.setArmGrowSubject(instID, dv.armGrowIdxFor(instID, idx), 0)` — simplest: `if _, ok := dv.instEditorArmedControl[instID]; ok { dv.setArmGrowSubject(instID, dv.instEditorArmedControl[instID], 0) }`. In `toggleSynthControlArm`, the off-branch calls `lockSynthControl` (already covered).

- [ ] **Step 5: Run — expect PASS** (Step 2 command). Then `-run 'Armed' -count=1` to confirm no build break.

## Task 2: ControlGrid geometry accessors + scroll-rows override (TDD)

**Files:**
- Modify: `src/go/internal/ui/control_grid.go`
- Test: `src/go/internal/ui/control_grid_geom_test.go`

**Interfaces produced:**
- `func (g *ControlGrid) CellW() int`, `CellH() int`, `RowH() int`, `HGap() int`.
- `func (g *ControlGrid) CellAt(row, col int) (image.Rectangle, bool)` — like `CellRect` but by explicit (row,col), honoring the scroll `First` offset; `ok=false` when off-window or out of column range.
- `func (g *ControlGrid) SetContentRows(total int)` — override `scroll.VS.Total` (for a reflow that occupies more rows than `count/cols`), then `Clamp()`. No-op if `total <= 0`.

- [ ] **Step 1: Failing tests** `control_grid_geom_test.go`:

```go
package ui

import (
	"image"
	"testing"
)

func TestControlGridGeom(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.SetMaxCols(3)
	g.Layout(image.Rect(0, 0, 320, 200), 6, 90, 40, 8, 8)
	if g.Cols() != 3 {
		t.Fatalf("cols want 3 got %d", g.Cols())
	}
	if g.CellH() != 40 {
		t.Fatalf("cellH want 40 got %d", g.CellH())
	}
	if g.RowH() != 48 {
		t.Fatalf("rowH want 48 got %d", g.RowH())
	}
	c00, ok := g.CellAt(0, 0)
	if !ok {
		t.Fatal("cell (0,0) should be visible")
	}
	c01, _ := g.CellAt(0, 1)
	if c01.Min.X != c00.Max.X+g.HGap() {
		t.Fatalf("cell (0,1) x=%d want %d", c01.Min.X, c00.Max.X+g.HGap())
	}
	r0, _ := g.CellRect(0)
	if c00 != r0 {
		t.Fatalf("CellAt(0,0) %v must match CellRect(0) %v", c00, r0)
	}
}

func TestControlGridSetContentRows(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.SetMaxCols(2)
	g.Layout(image.Rect(0, 0, 200, 60), 2, 90, 40, 8, 8) // 1 row, fits
	if g.HasScroll() {
		t.Fatal("2 controls in 2 cols fit one row; no scroll expected")
	}
	g.SetContentRows(5) // reflow now needs 5 rows
	if !g.HasScroll() {
		t.Fatal("after SetContentRows(5) the grid must report scrollable")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`CellH`/`CellAt`/`SetContentRows` undefined).
- [ ] **Step 3: Implement** the accessors in `control_grid.go`:

```go
func (g *ControlGrid) CellW() int { return g.cellW }
func (g *ControlGrid) CellH() int { return g.cellH }
func (g *ControlGrid) RowH() int  { return g.rowH }
func (g *ControlGrid) HGap() int  { return g.hGap }

// CellAt returns the slot for an explicit (row,col), honoring the scroll window.
// Unlike CellRect it is not bounded by `count` — a reflow addresses cells beyond
// the uniform count. ok=false when col is out of range or the row is off-window.
func (g *ControlGrid) CellAt(row, col int) (image.Rectangle, bool) {
	if g.cols <= 0 || col < 0 || col >= g.cols {
		return image.Rectangle{}, false
	}
	first := g.scroll.VS.First
	if row < first || row >= first+g.visRows {
		return image.Rectangle{}, false
	}
	x0 := g.rect.Min.X + col*(g.cellW+g.hGap)
	y0 := g.rect.Min.Y + (row-first)*g.rowH
	return image.Rect(x0, y0, x0+g.cellW, y0+g.cellH), true
}

// SetContentRows overrides the scrollable total-row count (used when a reflow
// occupies more rows than count/cols). Re-clamps the scroll window.
func (g *ControlGrid) SetContentRows(total int) {
	if total <= 0 {
		return
	}
	g.scroll.VS.Total = total
	g.scroll.VS.Clamp()
}
```

- [ ] **Step 4: Run — expect PASS.**

## Task 3: Masonry reflow function (pure geometry, TDD)

**Files:**
- Create: `src/go/internal/ui/synth_armed_reflow.go`
- Test: `src/go/internal/ui/synth_armed_reflow_test.go`

**Interface produced:**
- `func reflowArmed(order []int, armedPos, cols, spanCols, spanRows int, cellAt func(row, col int) image.Rectangle) (rects []image.Rectangle, rowMax int)` — returns a target rect per `order` entry (index-aligned), with `order[armedPos]` a `spanCols×spanRows` block and every other control packed into the earliest free cell in reading order; `rowMax` = highest row index used (caller feeds it to `SetContentRows`). Nothing is empty; no two rects overlap.

- [ ] **Step 1: Failing tests** `synth_armed_reflow_test.go`:

```go
package ui

import (
	"image"
	"testing"
)

// cellAt stub: uniform 10x10 cells, no gaps, origin 0.
func stubCellAt(row, col int) image.Rectangle {
	return image.Rect(col*10, row*10, col*10+10, row*10+10)
}

func TestReflowArmed_BlockIs2x2_NothingHidden_NoOverlap(t *testing.T) {
	order := []int{10, 11, 12, 13, 14, 15} // 6 controls
	cols, spanCols, spanRows := 3, 2, 2
	armedPos := 0 // arm order[0]
	rects, rowMax := reflowArmed(order, armedPos, cols, spanCols, spanRows, stubCellAt)

	// Block is 2x2 = 20x20.
	if got := rects[armedPos]; got.Dx() != 20 || got.Dy() != 20 {
		t.Fatalf("armed block want 20x20 got %v", got)
	}
	// Nothing hidden.
	for i, r := range rects {
		if r.Empty() {
			t.Fatalf("control at order[%d] has empty rect (hidden)", i)
		}
	}
	// No overlaps.
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].Overlaps(rects[j]) {
				t.Fatalf("rects %d and %d overlap: %v %v", i, j, rects[i], rects[j])
			}
		}
	}
	if rowMax < 1 {
		t.Fatalf("a 2x2 block spans >=2 rows; rowMax=%d", rowMax)
	}
}

func TestReflowArmed_ClampsSpanToCols(t *testing.T) {
	// 1-column grid: spanCols must clamp to 1 (block = 1x2), still no overlap.
	order := []int{0, 1, 2}
	rects, _ := reflowArmed(order, 0, 1, 2, 2, stubCellAt)
	if rects[0].Dx() != 10 {
		t.Fatalf("1-col block width must clamp to one cell (10), got %d", rects[0].Dx())
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].Overlaps(rects[j]) {
				t.Fatalf("overlap %d %d", i, j)
			}
		}
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`reflowArmed` undefined).
- [ ] **Step 3: Implement** `synth_armed_reflow.go`:

```go
package ui

import "image"

// reflowArmed packs `order` into a grid where order[armedPos] occupies a
// spanCols×spanRows block and every other control takes the earliest free cell in
// reading order. Nothing is hidden; no cells overlap. cellAt(row,col) yields the
// pixel rect for a lattice cell (honoring scroll). Returns index-aligned rects and
// the highest row used (for scroll extension).
func reflowArmed(order []int, armedPos, cols, spanCols, spanRows int, cellAt func(row, col int) image.Rectangle) ([]image.Rectangle, int) {
	if cols < 1 {
		cols = 1
	}
	if spanCols > cols {
		spanCols = cols
	}
	if spanCols < 1 {
		spanCols = 1
	}
	if spanRows < 1 {
		spanRows = 1
	}
	rects := make([]image.Rectangle, len(order))
	occ := map[[2]int]bool{}
	rowMax := 0
	mark := func(r, c int) {
		occ[[2]int{r, c}] = true
		if r > rowMax {
			rowMax = r
		}
	}
	fits := func(r, c int) bool {
		if c+spanCols > cols {
			return false
		}
		for dr := 0; dr < spanRows; dr++ {
			for dc := 0; dc < spanCols; dc++ {
				if occ[[2]int{r + dr, c + dc}] {
					return false
				}
			}
		}
		return true
	}
	earliestFree := func(block bool) (int, int) {
		for lin := 0; ; lin++ {
			r, c := lin/cols, lin%cols
			if occ[[2]int{r, c}] {
				continue
			}
			if !block || fits(r, c) {
				return r, c
			}
		}
	}
	for i := range order {
		if i == armedPos {
			r, c := earliestFree(true)
			for dr := 0; dr < spanRows; dr++ {
				for dc := 0; dc < spanCols; dc++ {
					mark(r+dr, c+dc)
				}
			}
			tl := cellAt(r, c)
			br := cellAt(r+spanRows-1, c+spanCols-1)
			rects[i] = image.Rect(tl.Min.X, tl.Min.Y, br.Max.X, br.Max.Y)
			continue
		}
		r, c := earliestFree(false)
		mark(r, c)
		rects[i] = cellAt(r, c)
	}
	return rects, rowMax
}
```

  Note: `cellAt` for an off-window row returns the zero rect, so `tl`/`br` union can be empty when the block is scrolled off — acceptable (the block is what the user interacts with and is kept on-window). The reflow uses the geometry directly, so the union stays correct while visible.

- [ ] **Step 4: Run — expect PASS.**

## Task 4: Rewrite `growArmedCard` (reflow + interpolation) + wire tick + migrate tests

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` — rewrite `growArmedCard`; `synthTabUpdate` (advance); the stage call site (`~:2205`).
- Modify: `src/go/internal/ui/synth_macro_render.go` — `layoutSynthMacroLayer` call site.
- Modify (migrate): `synth_armed_layout_test.go`, `synth_advanced_divider_test.go`, `synth_armed_pixels_test.go` — settle the animation before asserting.
- Test: extend `synth_armed_reflow_test.go` with a functional grow test.

**Interfaces consumed:** `armGrowControl`, `armGrowProgress`, `advanceArmGrow`, `lerpRect` (Task 1); `CellAt`, `RowH`, `SetContentRows` (Task 2); `reflowArmed` (Task 3).

- [ ] **Step 1: Rewrite `growArmedCard`** (`synth_panel_zone.go`). Replace the whole body (the width-span + hide + shift logic) with reflow + interpolation:

```go
// growArmedCard reflows the section/macro grid so the grow-animation subject
// (armedKIdx) occupies a 2×2 block and every other control slides to a free cell
// (never hidden), interpolated by the eased grow progress. `order` is the visible
// control-index slice; the grid is already uniform-laid-out by the caller (that is
// the RESTING layout). No-op once fully settled at rest.
func (dv *DrumView) growArmedCard(order []int, grid *ControlGrid, innerR image.Rectangle, armedKIdx int) {
	prog := dv.armGrowProgress()
	if prog <= 0 || grid == nil {
		return
	}
	cols := grid.Cols()
	if cols < 1 {
		return
	}
	armedPos := -1
	for i, k := range order {
		if k == armedKIdx {
			armedPos = i
			break
		}
	}
	if armedPos < 0 {
		return
	}
	// Resting rects = the uniform layout the caller already applied.
	resting := make([]image.Rectangle, len(order))
	for i, k := range order {
		if k >= 0 && k < len(dv.instEditorControls) {
			resting[i] = dv.instEditorControls[k].Rect()
		}
	}
	// 2×2 span, widened to guarantee the barrel lanes fit.
	spanCols := 2
	if spanCols > cols {
		spanCols = cols
	}
	spanRows := 2
	dvv := Profile().DensityValues()
	for spanCols < cols && spanCols*grid.CellW()+(spanCols-1)*grid.HGap() < dvv.SynthArmedCardW {
		spanCols++
	}
	cellAt := func(r, c int) image.Rectangle {
		rect, _ := grid.CellAt(r, c)
		return rect
	}
	target, rowMax := reflowArmed(order, armedPos, cols, spanCols, spanRows, cellAt)
	grid.SetContentRows(rowMax + 1) // extend scroll to fit the reflow
	for i, k := range order {
		if k < 0 || k >= len(dv.instEditorControls) {
			continue
		}
		r := lerpRect(resting[i], target[i], prog)
		dv.instEditorControls[k].SetRect(r)
		if k < len(dv.instEditorSliders) {
			dv.instEditorSliders[k].SetRect(r)
		}
	}
}
```

  (The old `growArmedCard` doc comment block above the function should be replaced with this function's new doc. Delete the now-unused width-span/hide/shift code entirely.)

- [ ] **Step 2: Switch the stage call site** (`synth_panel_zone.go ~:2205`) from `synthControlArmed` to `armGrowControl`:

```go
	if idx, active := p.dv.armGrowControl(instID); active {
		p.dv.growArmedCard(visible, grid, innerR, idx)
	}
```

- [ ] **Step 3: Switch the macro call site** in `layoutSynthMacroLayer` (`synth_macro_render.go`) the same way:

```go
	if idx, active := dv.armGrowControl(resolved); active {
		dv.growArmedCard(idxs, grid, inner, idx)
	}
```

- [ ] **Step 4: Advance the animation** in `synthTabUpdate` (`synth_panel_zone.go ~:1994`, next to the barrel `Tick()`): add, inside the same `instID != ""` block or just after it,

```go
	if dv.advanceArmGrow() {
		changed = true
	}
```

  `changed = true` drives the redraw + relayout each frame, so `growArmedCard` re-interpolates. (The armed editor `Tick()` already sets `changed`; this is the sibling for the grow/shrink.)

- [ ] **Step 5: Functional grow test** — append to `synth_armed_reflow_test.go` (uses the macro helpers from `synth_macro_render_test.go`):

```go
func TestArmedGrow_ReflowNothingHiddenAfterSettle(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	_, kIdx := drillMacro(t, g, "brightness")
	order := dv.macroConceptKnobIdxs("brightness")
	// Snapshot resting rects (pre-arm), all non-empty.
	rest := map[int]bool{}
	for _, k := range order {
		if !dv.instEditorControls[k].Rect().Empty() {
			rest[k] = true
		}
	}
	dv.armSynthControl(dv.synthTabActiveInstrument(), kIdx)
	// Settle the grow animation.
	for i := 0; i < 50 && dv.advanceArmGrow(); i++ {
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	}
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	// Nothing that was visible is now hidden.
	for k := range rest {
		if dv.instEditorControls[k].Rect().Empty() {
			t.Errorf("control %d hidden after arming (should reflow, not hide)", k)
		}
	}
	// Armed control grew to >= 2x a resting sibling's area.
	armedR := dv.instEditorControls[kIdx].Rect()
	if armedR.Dx()*armedR.Dy() < 2*grid0Area(dv, order, kIdx) {
		t.Errorf("armed control did not grow to a 2x2-ish block: %v", armedR)
	}
}

func grid0Area(dv *DrumView, order []int, except int) int {
	for _, k := range order {
		if k != except {
			r := dv.instEditorControls[k].Rect()
			if !r.Empty() {
				return r.Dx() * r.Dy()
			}
		}
	}
	return 1
}
```

- [ ] **Step 6: Migrate existing armed tests** so they settle the animation before asserting the grown geometry. In `synth_armed_layout_test.go` (`TestArmedCardGrowsInPlace`, `TestArmedCardShiftsAdvancedDivider`) and `synth_advanced_divider_test.go` and `synth_armed_pixels_test.go`, after `dv.armSynthControl(...)` + `layoutSynthTab(t, g)`, add a settle loop:

```go
	for i := 0; i < 50 && dv.advanceArmGrow(); i++ {
		layoutSynthTab(t, g)
	}
	layoutSynthTab(t, g)
```

  Re-read each test's assertion: `TestArmedCardGrowsInPlace` (armed Dy > resting) still holds once settled; `TestArmedCardShiftsAdvancedDivider` (divider shifts down, no overlap) still holds (rows below the block shift down in the reflow). If any test asserted a *specific* hidden sibling, change it to assert the sibling moved (non-empty, rect changed).

- [ ] **Step 7: Build + run** the armed + reflow + macro suites:
  `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Armed|Reflow|Macro|Barrel|ArmGrow|ControlGrid' -count=1`
  Expect PASS.

## Task 5: Screenshots (mid + settled, desktop + mobile) + full verification + docs

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go` — add `crop_synth_macro_arming` (a MID-animation frame: arm, then advance the anim a couple ticks so `armGrowT ≈ 0.4` and relayout).
- Modify: memory `project_synth_macro_drilldown_ui.md` / `project_synth_inline_armed_controls.md`.

- [ ] **Step 1: Add a mid-animation scene.** In `scene_catalog.go`, add a helper `macroArmingMidSceneSetup` that arms then advances `advanceArmGrow` twice (so the card is partway grown) with a relayout after each, then a scene `crop_synth_macro_arming` using it (Subject `SubjectSynthPanel`, desktop + mobile).

```go
func macroArmingMidSceneSetup(instrument, concept string) func(*Game) {
	return func(g *Game) {
		macroArmedSceneSetup(instrument, concept)(g) // arms + settles at target 1... so instead:
		// Reset to a mid frame:
		g.drum.armGrowT = 0
		g.drum.advanceArmGrow()
		g.drum.advanceArmGrow() // T ≈ 0.32
		if g.drum.audioTree != nil {
			g.drum.audioTree.LayoutZoneNow("eq-panel")
		}
	}
}
```

  (Adjust: `macroArmedSceneSetup` already arms; do NOT settle it there. Set `armGrowT` to a mid value like `0.35` directly for a deterministic capture, then relayout.)

- [ ] **Step 2: Build + capture** desktop:
  ```bash
  cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
  cd /home/ymolinar/Repos/beatmo
  for s in crop_synth_macro_armed crop_synth_macro_arming crop_synth_control_armed; do \
    xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene $s -screenshot /tmp/$s.png; done
  ```
- [ ] **Step 3: OPEN all three PNGs.** Verify: settled frame = 2×2 block, every neighbour still visible (none hidden), reflowed to the side/below; mid frame = partially-grown card with neighbours partway to their new spots; stage `crop_synth_control_armed` = same reflow behaviour (no hidden sibling). Fix + re-capture.
- [ ] **Step 4: Mobile pass:** `make screenshots-all SCENES=crop_synth_macro_armed,crop_synth_macro_arming MOBILE=1`; **open** `screenshots/all/mobile/*.browser.png`. Verify 2×2 block + reflow, nothing hidden, on the narrow viewport.
- [ ] **Step 5: Full verification:**
  ```bash
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ -count=1
  ```
  Expect only the two known pre-existing failures (`TestSamplerMobileOverflowMenuHasSaveAsReset`, `TestSynthTab_ReleaseFollowingDragPropagatesFinalValue`) — both proven unrelated; **zero** new failures. If a new failure appears, prove/fix it before claiming done.
- [ ] **Step 6: Update memory** with the reflow+animation design + the `growArmedCard` rewrite (push-not-hide, 2×2, eased via `armGrowT`, shared).

---

## Self-review notes
- Spec coverage: 2×2 footprint (Task 4 span logic), reflow-not-hide (Task 3 + tests), grow/shrink animation (Task 1 + Task 4 tick + interpolation), shared (both call sites Task 4 Steps 2-3), scroll extension (Task 2 `SetContentRows` + Task 4), tests + screenshots (Tasks 3–5). ✔
- Shrink correctness: call sites guard on `armGrowControl` (active through shrink), so the reflow keeps running as `armGrowT`→0; the locked control draws as the resting ParamCard at its shrinking rect (editor nil'd on lock). ✔
- No placeholders: reflow + anim + geometry code given in full; migration is a concrete settle-loop snippet.
- Type consistency: `armGrowIdx/armGrowInst/armGrowT/armGrowTarget`, `armGrowControl/armGrowProgress/advanceArmGrow/setArmGrowSubject`, `reflowArmed`, `CellAt/CellW/CellH/RowH/HGap/SetContentRows`, `lerpRect/smoothstep` used consistently.
- Risk: `growArmedCard` is shared; the ~3 migrated tests are the blast radius. The reflow changes hit-rect positions during animation (intended — hit follows visual).
