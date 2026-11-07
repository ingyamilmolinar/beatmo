package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Owns only GEOMETRY and scroll state, never the controls: at least 2 columns, more when wide enough; off-window
// cells report !ok. Scroll lives on the embedded ScrollBehavior, so it survives the per-frame Layout.
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
	peekH   int  // bottom peek-band height (0 when the grid fits or has no room)
	maxCols int  // 0 = unlimited; caps the adaptive column count (overrides the floor)
	noPeek  bool // SetPeekBand(false): never reserve the bottom peek band

	// Off by default so the sampler and macro grids keep the stepped feel; the scrollbar THUMB always stays stepped.
	smooth bool
}

func (g *ControlGrid) SetSmoothScroll(v bool) { g.smooth = v }

// A clipped sliver of the next row signals more below. Draw-only.
const controlGridPeekFrac = 0.4

// A grid of tall rows opts out: the band can cost a whole row, and the scrollbar still says "more".
func (g *ControlGrid) SetPeekBand(v bool) { g.noPeek = !v }

// Overrides the width-adaptive count AND the 2-column floor; 0 = adaptive.
func (g *ControlGrid) SetMaxCols(n int) { g.maxCols = n }

// NewControlGrid creates an empty grid using the given scrollbar style.
func NewControlGrid(style ScrollbarStyle) *ControlGrid {
	return &ControlGrid{scroll: NewScrollBehavior(style, 1)}
}

// cellIdealW only picks the column count. The scroll window is preserved and re-clamped, never reset.
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

	// Before visRows, which depends on it.
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

	// The window height derives from the rect and peek only, never the scroll position, so it never reflows
	// while scrolling.
	fullRows := rect.Dy() / g.rowH
	if fullRows < 1 {
		fullRows = 1
	}
	visRows := fullRows
	g.peekH = 0
	if totalRows > fullRows && !g.noPeek {
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

// 0 when the grid fits; draw sites use it as the fade height.
func (g *ControlGrid) PeekH() int { return g.peekH }

// Draw-only: the peek row publishes no hit area.
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

func (g *ControlGrid) ShowTopFade() bool { return g.scroll.VS.First > 0 }

func (g *ControlGrid) ShowBottomFade() bool {
	return g.scroll.VS.First+g.visRows < g.scroll.VS.Total
}

// Cols reports the current column count.
func (g *ControlGrid) Cols() int { return g.cols }

// Exposed so a reflow can address the same lattice.
func (g *ControlGrid) CellW() int { return g.cellW }
func (g *ControlGrid) CellH() int { return g.cellH }
func (g *ControlGrid) RowH() int  { return g.rowH }
func (g *ControlGrid) HGap() int  { return g.hGap }

// Not bounded by count: a reflow addresses lattice cells past the uniform count.
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

// CellAt's off-window result is a zero rect at the SCREEN ORIGIN, a sentinel, not geometry: animating toward it
// flies the control to (0,0). Callers needing a real destination use this and clip with ClipBottom.
func (g *ControlGrid) CellAtUnclipped(row, col int) (image.Rectangle, bool) {
	if g.cols <= 0 || col < 0 || col >= g.cols {
		return image.Rectangle{}, false
	}
	x0 := g.rect.Min.X + col*(g.cellW+g.hGap)
	y0 := g.rect.Min.Y + (row-g.scroll.VS.First)*g.rowH
	return image.Rect(x0, y0, x0+g.cellW, y0+g.cellH), true
}

// When the rect is too short for one row the resting row's bottom wins, so clipping never moves a resting cell.
func (g *ControlGrid) ClipBottom() int {
	bottom := g.rect.Max.Y
	if g.visRows > 0 && g.rowH > 0 {
		if last := g.rect.Min.Y + (g.visRows-1)*g.rowH + g.cellH; last > bottom {
			bottom = last
		}
	}
	return bottom
}

// For a reflow taller than count/cols, so the grid scrolls instead of clipping.
func (g *ControlGrid) SetContentRows(total int) {
	if total <= 0 {
		return
	}
	g.scroll.VS.Total = total
	g.scroll.VS.Clamp()
}

// The minimal scroll that brings control i into view.
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

// FIXED, never scaled to the scrollbar track, so every gesture lands one clean row; shared by every list so
// all scrollbars feel alike. 2 frames is the floor that still reads as discrete clicks.
const (
	controlGridDragStepPx           = 44
	controlGridScrollCooldownFrames = 2
)

// BeginDrag is the thumb grab (direct, always stepped); BeginContentDrag is the body grab (natural), smooth
// when enabled. DragTo/EndDrag pick the path by TS.Active(), true only on the smooth body path.
func (g *ControlGrid) BeginDrag(y int) bool { return g.scroll.BeginStepDrag(y) }

func (g *ControlGrid) BeginContentDrag(y int) bool {
	if g.smooth {
		if !g.scroll.HasScroll() {
			return false
		}
		g.scroll.HandleTouchBegin(0, y)
		return true
	}
	return g.scroll.BeginStepDragNatural(y)
}

func (g *ControlGrid) DragTo(y int) bool {
	if g.smooth && g.scroll.TS.Active() {
		return g.scroll.HandleTouchMove(0, y)
	}
	return g.scroll.StepDragTo(y, controlGridDragStepPx)
}

func (g *ControlGrid) EndDrag() {
	if g.smooth && g.scroll.TS.Active() {
		g.scroll.HandleTouchEnd()
		return
	}
	g.scroll.EndStepDrag()
}

// Stepped mode moves one row per notch with a cooldown; smooth mode honours the notch magnitude.
func (g *ControlGrid) WheelStep(steps int) bool {
	if g.smooth {
		return g.scroll.HandleWheel(steps)
	}
	return g.scroll.WheelStep(steps, controlGridScrollCooldownFrames)
}

func (g *ControlGrid) Tick() { g.scroll.TickStep() }

// Always false for a stepped grid.
func (g *ControlGrid) UpdateMomentum() bool { return g.scroll.UpdateMomentum() }

func (g *ControlGrid) HasMomentum() bool { return g.scroll.HasMomentum() }

// Draw paints the scrollbar (no-op when the grid fits without scrolling).
func (g *ControlGrid) Draw(dst *ebiten.Image) { g.scroll.Draw(dst) }
