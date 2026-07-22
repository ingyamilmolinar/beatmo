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
	peekH   int // bottom peek-band height (0 when the grid fits or has no room)
	maxCols int // 0 = unlimited; caps the adaptive column count (overrides the floor)
}

// controlGridPeekFrac is the fraction of one row height reserved as a bottom
// "peek band" when the grid overflows — a clipped sliver of the next row that
// signals more controls lie below. Draw-only; it publishes no hit area.
const controlGridPeekFrac = 0.4

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

	// Adaptive columns (independent of the visible-row count): at least 2 (when
	// there's more than one control), more when the rect is wide enough to fit
	// them at the ideal cell width. Computed before visRows because the row count
	// depends on it.
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

	// Visible rows. When the grid overflows AND there is room, reserve a bottom
	// peek band (~0.4 row) so the first off-window row renders as a clipped
	// sliver — a "more below" affordance. The window height is derived from the
	// rect + peek only (never the scroll position), so it stays constant while
	// scrolling and never reflows.
	fullRows := rect.Dy() / g.rowH
	if fullRows < 1 {
		fullRows = 1
	}
	visRows := fullRows
	g.peekH = 0
	if totalRows > fullRows {
		ph := int(float64(g.rowH) * controlGridPeekFrac)
		if ph > 0 && (rect.Dy()-ph)/g.rowH >= 1 {
			g.peekH = ph
			visRows = (rect.Dy() - ph) / g.rowH
			if visRows < 1 {
				visRows = 1
			}
		}
	}
	g.visRows = visRows

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

// PeekH reports the reserved bottom peek-band height (0 when the grid fits or has
// no room for a band). Draw sites use it as the fade-edge height.
func (g *ControlGrid) PeekH() int { return g.peekH }

// PeekRect returns the clipped top-sliver rect for control i when it lies in the
// first row just below the visible window (the peek band), and whether it does.
// It is DRAW-ONLY — the peek row publishes no hit area (CellRect returns false for
// it). Returns false when there is no peek band, when i is not in that first
// off-window row, or when scrolled to the bottom (no next row exists).
func (g *ControlGrid) PeekRect(i int) (image.Rectangle, bool) {
	if g.peekH <= 0 || g.cols <= 0 || i < 0 || i >= g.count {
		return image.Rectangle{}, false
	}
	if i/g.cols != g.scroll.VS.First+g.visRows {
		return image.Rectangle{}, false
	}
	col := i % g.cols
	x0 := g.rect.Min.X + col*(g.cellW+g.hGap)
	yTop := g.rect.Min.Y + g.visRows*g.rowH
	yBot := yTop + g.peekH
	if yBot > g.rect.Max.Y {
		yBot = g.rect.Max.Y
	}
	return image.Rect(x0, yTop, x0+g.cellW, yBot), true
}

// ContentRect returns the content rect the grid was last laid out into.
func (g *ControlGrid) ContentRect() image.Rectangle { return g.rect }

// ShowTopFade reports whether the top fade edge should paint (content is scrolled
// down from the first row).
func (g *ControlGrid) ShowTopFade() bool { return g.scroll.VS.First > 0 }

// ShowBottomFade reports whether the bottom fade edge should paint (more rows
// exist below the visible window). Hides at the bottom of travel.
func (g *ControlGrid) ShowBottomFade() bool {
	return g.scroll.VS.First+g.visRows < g.scroll.VS.Total
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
// wheel/step scrolls. Both are FIXED — independent of the (often tiny) scrollbar
// track — so every gesture lands one clean row at a time instead of flying
// through the list. Shared with the row rack and every list menu so all
// scrollbars feel identical (see menu_scroll.go). The cooldown stays clicky (one
// item per notch, one discrete step at a time) but is short enough that a held /
// repeatedly-flicked wheel ticks briskly rather than crawling. 2 frames is the
// floor that still reads as discrete clicks — 1 frame moves every frame and blurs
// into a continuous glide. (At 60 fps, 2 frames ≈ 30 rows per second while the
// wheel is spun; a single notch still moves immediately.) Guarded by
// TestAllMenuScrolls_ClickyCadenceIsFast.
const (
	controlGridDragStepPx           = 44
	controlGridScrollCooldownFrames = 2
)

// BeginDrag / BeginContentDrag / DragTo / EndDrag / WheelStep / Tick are thin
// wrappers over the embedded ScrollBehavior's step-by-step API (shared with the
// row rack). BeginDrag is the scrollbar THUMB grab (direct — the thumb follows
// the finger). BeginContentDrag is the card-BODY grab (natural — the content
// follows the finger, so dragging down reveals earlier rows). Both share DragTo/
// EndDrag; the direction is chosen at Begin time.
func (g *ControlGrid) BeginDrag(y int) bool        { return g.scroll.BeginStepDrag(y) }
func (g *ControlGrid) BeginContentDrag(y int) bool { return g.scroll.BeginStepDragNatural(y) }
func (g *ControlGrid) DragTo(y int) bool           { return g.scroll.StepDragTo(y, controlGridDragStepPx) }
func (g *ControlGrid) EndDrag()                    { g.scroll.EndStepDrag() }
func (g *ControlGrid) WheelStep(steps int) bool {
	return g.scroll.WheelStep(steps, controlGridScrollCooldownFrames)
}
func (g *ControlGrid) Tick() { g.scroll.TickStep() }

// Draw paints the scrollbar (no-op when the grid fits without scrolling).
func (g *ControlGrid) Draw(dst *ebiten.Image) { g.scroll.Draw(dst) }
