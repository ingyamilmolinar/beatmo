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
	peekH   int  // bottom peek-band height (0 when the grid fits or has no room)
	maxCols int  // 0 = unlimited; caps the adaptive column count (overrides the floor)
	noPeek  bool // SetPeekBand(false): never reserve the bottom peek band

	// smooth opts this grid into the continuous pixel + momentum scroll path
	// (BeginContentDrag/DragTo/EndDrag/WheelStep) instead of the default
	// row-by-row stepped path. Off by default so shared callers (sampler knob
	// grid, synth macro grid) keep the original stepped feel byte-for-byte;
	// the synth section grids opt in via SetSmoothScroll(true). The scrollbar
	// THUMB grab (BeginDrag) is unaffected by this flag — it always stays
	// stepped/direct.
	smooth bool
}

// SetSmoothScroll opts this grid into continuous pixel + momentum content
// scrolling (see the smooth field doc). Callers that don't call this keep the
// original stepped behavior unchanged.
func (g *ControlGrid) SetSmoothScroll(v bool) { g.smooth = v }

// controlGridPeekFrac is the fraction of one row height reserved as a bottom
// "peek band" when the grid overflows — a clipped sliver of the next row that
// signals more controls lie below. Draw-only; it publishes no hit area.
const controlGridPeekFrac = 0.4

// SetPeekBand opts this grid in (default) or out of the bottom peek band. A
// grid whose rows are tall relative to its window opts out: reserving the band
// can cost a whole row (the Synth root's dial cards on a phone showed one card
// above ~110 px of blank where two fit), and the scrollbar still says "more".
func (g *ControlGrid) SetPeekBand(v bool) { g.noPeek = !v }

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

// CellW / CellH / RowH / HGap expose the laid-out cell geometry so a caller can
// build a masonry reflow (armed-control 2×2 block) over the same lattice.
func (g *ControlGrid) CellW() int { return g.cellW }
func (g *ControlGrid) CellH() int { return g.cellH }
func (g *ControlGrid) RowH() int  { return g.rowH }
func (g *ControlGrid) HGap() int  { return g.hGap }

// CellAt returns the slot for an explicit (row,col), honoring the scroll window.
// Unlike CellRect it is NOT bounded by `count` — a reflow addresses lattice cells
// beyond the uniform count. ok=false when col is out of range or the row is
// off-window.
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

// CellAtUnclipped returns the lattice slot for (row,col) IGNORING the scroll
// window: a row below the window yields a rect below the visible rows. ok=false
// only when the column is out of range.
//
// CellAt's off-window return is `image.Rectangle{}` — a "not visible" SENTINEL
// that happens to be a zero-size rect anchored at the SCREEN ORIGIN. Anything
// that animates a control toward a destination must not treat that sentinel as
// geometry: interpolating toward it flies the control up-and-left to (0,0)
// instead of down to its real slot. Callers that need a real destination for an
// off-window cell use this and clip the result with ClipBottom.
func (g *ControlGrid) CellAtUnclipped(row, col int) (image.Rectangle, bool) {
	if g.cols <= 0 || col < 0 || col >= g.cols {
		return image.Rectangle{}, false
	}
	x0 := g.rect.Min.X + col*(g.cellW+g.hGap)
	y0 := g.rect.Min.Y + (row-g.scroll.VS.First)*g.rowH
	return image.Rect(x0, y0, x0+g.cellW, y0+g.cellH), true
}

// ClipBottom is the lowest Y a control may paint to. It is the content rect's
// bottom, except when the rect is too short for even one row (Layout floors
// visRows at 1, so resting cells legitimately overhang) — there the last resting
// row's bottom wins, so clipping against this never moves a cell the plain
// resting layout already produced.
func (g *ControlGrid) ClipBottom() int {
	bottom := g.rect.Max.Y
	if g.visRows > 0 && g.rowH > 0 {
		if last := g.rect.Min.Y + (g.visRows-1)*g.rowH + g.cellH; last > bottom {
			bottom = last
		}
	}
	return bottom
}

// SetContentRows overrides the scrollable total-row count — used when a reflow
// occupies more rows than count/cols so the grid scrolls to reveal them instead
// of clipping. Re-clamps the scroll window. No-op for total <= 0.
func (g *ControlGrid) SetContentRows(total int) {
	if total <= 0 {
		return
	}
	g.scroll.VS.Total = total
	g.scroll.VS.Clamp()
}

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
// the finger) and is ALWAYS stepped, in both modes. BeginContentDrag is the
// card-BODY grab (natural — the content follows the finger, so dragging down
// reveals earlier rows); when SetSmoothScroll(true) was called it switches to
// the continuous pixel + momentum touch path instead of the stepped one. DragTo/
// EndDrag branch on which path is live by checking ScrollBehavior.TS.Active() —
// that's only true when BeginContentDrag took the smooth path, so a thumb drag
// on a smooth grid still routes to the stepped path untouched.
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

// WheelStep advances the scroll window. In the default (stepped) mode it moves
// exactly one row per notch with a cooldown (see ScrollBehavior.WheelStep). In
// smooth mode it scrolls continuously via ScrollBehavior.HandleWheel, honoring
// the notch magnitude instead of capping at one row.
func (g *ControlGrid) WheelStep(steps int) bool {
	if g.smooth {
		return g.scroll.HandleWheel(steps)
	}
	return g.scroll.WheelStep(steps, controlGridScrollCooldownFrames)
}

func (g *ControlGrid) Tick() { g.scroll.TickStep() }

// UpdateMomentum advances one frame of fling decay for a smooth grid; call once
// per frame while HasMomentum() is true. No-op (always returns false) for a
// default (non-smooth) grid, since it never accrues touch velocity.
func (g *ControlGrid) UpdateMomentum() bool { return g.scroll.UpdateMomentum() }

// HasMomentum reports whether a smooth grid is still coasting from a flick.
// Always false for a default (non-smooth) grid.
func (g *ControlGrid) HasMomentum() bool { return g.scroll.HasMomentum() }

// Draw paints the scrollbar (no-op when the grid fits without scrolling).
func (g *ControlGrid) Draw(dst *ebiten.Image) { g.scroll.Draw(dst) }
