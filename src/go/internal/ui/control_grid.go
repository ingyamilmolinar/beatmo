package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// ControlGrid lays a flat list of fixed-size controls out in an adaptive grid
// and scrolls vertically when the rows overflow the content rect. It owns only
// GEOMETRY + scroll state — never the controls themselves. The caller asks for
// each control's slot via CellRect and fits its widget inside.
//
// Column count is adaptive: at least 2 columns whenever there is more than one
// control, and more columns when the content rect is wide enough to fit them at
// the ideal cell width. Rows beyond the visible window are reported as hidden
// (CellRect returns ok == false) so the caller can skip drawing + hit-testing
// them. The embedded *ScrollBehavior provides the wheel / thumb-drag / touch +
// momentum handling and scrollbar rendering already used by the row rack and
// dropdowns, so this component adds no new input or drawing primitives.
//
// Scroll state lives on the embedded ScrollBehavior, which is preserved across
// the per-frame Layout calls (the caller keeps the *ControlGrid alive), so the
// scroll position survives re-layout exactly like the row rack's scroller.
type ControlGrid struct {
	scroll *ScrollBehavior

	rect    image.Rectangle // content area (inside the card, below its title)
	count   int
	cols    int
	cellW   int // per-cell width (scrollbar width reserved when HasScroll)
	cellH   int // per-cell height (control + caption band)
	rowH    int // cellH + vGap; the scroll item height
	hGap    int
	visRows int
	maxCols int // 0 = unlimited; caps the adaptive column count (overrides the floor)
}

// SetMaxCols caps how many columns Layout will use, overriding both the
// width-adaptive count AND the 2-column floor. 0 (default) leaves the grid
// fully adaptive. The Synth tab sets this to make each knob cell wide enough to
// host a plain-English purpose line beside its concept mini-visual.
func (g *ControlGrid) SetMaxCols(n int) { g.maxCols = n }

// NewControlGrid creates an empty grid using the given scrollbar style.
func NewControlGrid(style ScrollbarStyle) *ControlGrid {
	return &ControlGrid{scroll: NewScrollBehavior(style, 1)}
}

// Layout recomputes the grid geometry for the given content rect and control
// count. cellIdealW is the width a cell wants in order to host its control at
// full size (used only to pick the column count); cellH is the fixed cell
// height. hGap / vGap are the inter-cell gaps. The scroll window (First) is
// preserved and re-clamped — Layout never resets the scroll position.
func (g *ControlGrid) Layout(rect image.Rectangle, count, cellIdealW, cellH, hGap, vGap int) {
	g.rect = rect
	g.count = count
	g.hGap = hGap
	g.cellH = cellH
	if rect.Empty() || count <= 0 || cellH <= 0 {
		g.cols = 0
		g.cellW = 0
		g.rowH = 0
		g.visRows = 0
		g.scroll.VS.Total = 0
		g.scroll.VS.Visible = 1
		g.scroll.VS.First = 0
		g.scroll.VS.View = rect
		return
	}

	g.rowH = cellH + vGap
	visRows := rect.Dy() / g.rowH
	if visRows < 1 {
		visRows = 1
	}
	g.visRows = visRows

	// Adaptive columns: at least 2 (when there's more than one control), more
	// when the rect is wide enough to fit them at the ideal cell width.
	cell := cellIdealW + hGap
	cols := 1
	if cell > 0 {
		cols = (rect.Dx() + hGap) / cell
	}
	if cols < 2 {
		cols = 2
	}
	if g.maxCols > 0 && cols > g.maxCols {
		cols = g.maxCols // explicit cap overrides the adaptive count AND the floor
	}
	if cols > count {
		cols = count
	}
	if cols < 1 {
		cols = 1
	}
	g.cols = cols

	totalRows := (count + cols - 1) / cols
	g.scroll.ItemHeight = g.rowH
	g.scroll.VS.Total = totalRows
	g.scroll.VS.Visible = visRows
	g.scroll.VS.View = rect
	g.scroll.VS.Clamp()

	availW := rect.Dx()
	if g.HasScroll() {
		availW -= g.scroll.Style.Width
	}
	if availW < 0 {
		availW = 0
	}
	cellW := (availW - (cols-1)*hGap) / cols
	if cellW < 0 {
		cellW = 0
	}
	g.cellW = cellW
}

// CellRect returns the slot rectangle for control i and whether it lies inside
// the visible scroll window. Off-window cells return (image.Rectangle{}, false).
func (g *ControlGrid) CellRect(i int) (image.Rectangle, bool) {
	if g.cols <= 0 || i < 0 || i >= g.count {
		return image.Rectangle{}, false
	}
	row := i / g.cols
	col := i % g.cols
	first := g.scroll.VS.First
	if row < first || row >= first+g.visRows {
		return image.Rectangle{}, false
	}
	x0 := g.rect.Min.X + col*(g.cellW+g.hGap)
	y0 := g.rect.Min.Y + (row-first)*g.rowH
	return image.Rect(x0, y0, x0+g.cellW, y0+g.cellH), true
}

// Cols reports the current column count.
func (g *ControlGrid) Cols() int { return g.cols }

// ScrollToIndex scrolls the minimal amount so control i's row lies inside the
// visible window. No-op when i is already visible. Used to bring a freshly
// focused control (e.g. a selected synth knob on a scrolled-off row) into view.
func (g *ControlGrid) ScrollToIndex(i int) {
	if g.cols <= 0 || i < 0 {
		return
	}
	row := i / g.cols
	vs := &g.scroll.VS
	if row < vs.First {
		vs.First = row
	} else if vs.Visible > 0 && row >= vs.First+vs.Visible {
		vs.First = row - vs.Visible + 1
	}
	vs.Clamp()
}

// HasScroll reports whether the grid overflows its content rect vertically.
func (g *ControlGrid) HasScroll() bool { return g.scroll.HasScroll() }

// VisibleCount reports how many controls currently fall inside the scroll window.
func (g *ControlGrid) VisibleCount() int {
	n := 0
	for i := 0; i < g.count; i++ {
		if _, vis := g.CellRect(i); vis {
			n++
		}
	}
	return n
}

// Scroll exposes the embedded scroll behaviour for input adapters and tests.
func (g *ControlGrid) Scroll() *ScrollBehavior { return g.scroll }

// controlGridDragStepPx is the pointer travel (px) required to advance one row
// during a drag, and controlGridScrollCooldownFrames is the frames between two
// wheel/step scrolls. Both are deliberately large and FIXED — independent of
// the (often tiny) scrollbar track — so every gesture lands one clean,
// human-paced row at a time instead of flying through the list. Shared with the
// row rack so both scrollbars feel identical. (At 60 fps, 14 frames ≈ one row
// per quarter-second while the wheel is held; a single notch moves immediately.)
const (
	controlGridDragStepPx          = 44
	controlGridScrollCooldownFrames = 14
)

// BeginDrag / DragTo / EndDrag / WheelStep / Tick are thin wrappers over the
// embedded ScrollBehavior's step-by-step API (shared with the row rack). Used
// for BOTH the scrollbar thumb and the card-body grab so the feel is identical
// wherever the user grabs.
func (g *ControlGrid) BeginDrag(y int) bool { return g.scroll.BeginStepDrag(y) }
func (g *ControlGrid) DragTo(y int) bool    { return g.scroll.StepDragTo(y, controlGridDragStepPx) }
func (g *ControlGrid) EndDrag()             { g.scroll.EndStepDrag() }
func (g *ControlGrid) WheelStep(steps int) bool {
	return g.scroll.WheelStep(steps, controlGridScrollCooldownFrames)
}
func (g *ControlGrid) Tick() { g.scroll.TickStep() }

// Draw paints the scrollbar (no-op when the grid fits without scrolling).
func (g *ControlGrid) Draw(dst *ebiten.Image) { g.scroll.Draw(dst) }
