package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

/* ─── geometry helpers ─────────────────────────────────────── */

// rowHeight returns the per-row pixel height — the fixed profile value
// on both desktop and mobile. The row size is *not* scaled by any
// per-instance zoom factor: per-row dimensions are the user's contract
// for stable touch targets, and the historical row-zoom chip / pinch
// gesture have been repurposed to resize the entire drum-view pane
// (see `Game.adjustDrumViewHeight`). Per-row scaling would also break
// the "tight layout" invariant — the splitter Y is computed assuming
// `rh = TouchRowHeight()`.
func (dv *DrumView) rowHeight() int {
	return TouchRowHeight()
}

// mobileTransportMinH returns the minimum transport header height.
// On mobile it targets the compact bar; on desktop a two-row transport.
func mobileTransportMinH() int { return Profile().HeaderMinH }

// HeaderRect returns the rectangle covering the top header strip of the
// drum pane (from `dv.Bounds.Min.Y` to `dv.Bounds.Min.Y+dv.headerH`),
// spanning the full pane width. This is the "toolbar" the user sees:
// transport cluster + BPM + length controls + master volume + overflow
// + lock — all the chrome above the row labels.
func (dv *DrumView) HeaderRect() image.Rectangle {
	return image.Rect(
		dv.Bounds.Min.X,
		dv.Bounds.Min.Y,
		dv.Bounds.Max.X,
		dv.Bounds.Min.Y+dv.headerH,
	)
}

// RowsRect is the public alias for the rows scroll area; see rowsRect.
func (dv *DrumView) RowsRect() image.Rectangle { return dv.rowsRect() }

// rowsRect returns the rectangle covering the row scroll area (between
// the header and the EQ panel / mobile bottom action bar). Used to scope
// the mobile touch dead zone so that only touches in this area are
// blocked during scroll disambiguation. Routes through dv.rowsBottom()
// — the single source of truth for the rows-area floor — so the bar's
// vertical band is correctly excluded on mobile.
func (dv *DrumView) rowsRect() image.Rectangle {
	return image.Rect(
		dv.Bounds.Min.X,
		dv.Bounds.Min.Y+dv.headerH,
		dv.Bounds.Max.X,
		dv.rowsBottom(),
	)
}

func (dv *DrumView) rowsAreaHeight() int {
	h := dv.rowsBottom() - (dv.Bounds.Min.Y + dv.headerH)
	if h < 0 {
		return 0
	}
	return h
}

// refreshWidgetLayout synchronises cached widget rectangles and derived
// measurements (header height, EQ height, control widths) from the board.
func (dv *DrumView) refreshWidgetLayout() {
	if dv.widgets == nil {
		dv.headerH = timelineHeight
		dv.eqH = eqPanelHeight
		return
	}
	if dv.widgetRects == nil {
		dv.widgetRects = map[WidgetKind]image.Rectangle{}
	}
	dv.widgetRects[WidgetTransport] = dv.widgets.Rect(WidgetTransport)
	dv.widgetRects[WidgetRack] = dv.widgets.Rect(WidgetRack)
	dv.widgetRects[WidgetTimeline] = dv.widgets.Rect(WidgetTimeline)
	dv.widgetRects[WidgetWave] = dv.widgets.Rect(WidgetWave)

	// When wave widget is hidden (mobile EQ collapsed), extend rack and timeline
	// to fill the space the widget board still allocates to row 2. This prevents
	// a grey gap below the last row, fixes FAB positioning, and corrects the
	// scrollbar track height.
	if Profile().IsMobile() && dv.mobileEQCollapsed && dv.widgetRects[WidgetWave].Empty() {
		if r := dv.widgetRects[WidgetRack]; !r.Empty() {
			r.Max.Y = dv.Bounds.Max.Y
			dv.widgetRects[WidgetRack] = r
		}
		if r := dv.widgetRects[WidgetTimeline]; !r.Empty() {
			r.Max.Y = dv.Bounds.Max.Y
			dv.widgetRects[WidgetTimeline] = r
		}
	}

	// Note: rack/timeline rect-extension when EQ collapses runs below,
	// AFTER dv.eqH is computed (so we can detect "EQ collapsed" robustly).

	// Row/column derived sizes
	if h := dv.widgets.RowHeight(0); h > 0 {
		dv.headerH = h
	} else {
		dv.headerH = timelineHeight
	}
	p := Profile()
	spec := ActiveTopBarSpec()
	minH := timelineHeight
	if spec.Height > minH {
		minH = spec.Height
	}
	if dv.headerH < minH {
		dv.headerH = minH
	}
	// Cap header height: locked-in via TopBarSpec.Height (sourced from
	// DESIGN.md profileOverrides.headerMaxH; equal to MinH so the bar is
	// fixed-height on both desktop and mobile).
	maxH := spec.Height
	if dv.headerH > maxH {
		dv.headerH = maxH
		// Adjust widget rects so the rack/timeline start at the capped headerH,
		// not the widget board's uncapped row 0 height.
		capY := dv.Bounds.Min.Y + dv.headerH
		if r := dv.widgetRects[WidgetRack]; r.Min.Y > capY {
			dv.widgetRects[WidgetRack] = image.Rect(r.Min.X, capY, r.Max.X, r.Max.Y)
		}
		if r := dv.widgetRects[WidgetTimeline]; r.Min.Y > capY {
			dv.widgetRects[WidgetTimeline] = image.Rect(r.Min.X, capY, r.Max.X, r.Max.Y)
		}
	}
	if h := dv.widgets.RowHeight(2); h > 0 {
		dv.eqH = h
	} else {
		dv.eqH = eqPanelHeight * 2
	}
	// Phase 0a of audio-panel redesign: enforce a tall default for every
	// analysis tab. RuntimeProfile.AudioPanelHeightMultiplier (default 3×
	// eqPanelHeight ≈ 570 px) clamped by AudioPanelHeightScreenFrac of
	// the framebuffer so the panel never crowds the grid. The widget-
	// board row weights compute too short by default for analysis-tab
	// chrome (Spectrum dB grid, Levels segmented strips, Chain stage
	// cards, Synth knob cards), so we lift the floor here.
	//
	// A user drag-resize is still respected when it asks for MORE — the
	// floor never shrinks an explicitly-enlarged panel — but it cannot
	// shrink the panel below the analysis-tab minimum. (Earlier
	// behaviour where the widget-board could collapse the panel to
	// ~130 px is what produced the "panel mostly empty during playback"
	// screenshots that drove this redesign.)
	if dv.eqPanelZone != nil && dv.eqPanelZone.tabState != nil {
		desired := dv.eqPanelZone.tabState.PanelHeightAt(dv.Bounds.Dy())
		if dv.eqH < desired {
			dv.eqH = desired
		}
	}
	if runningUnderGoTest() && eqPanelHeight == 0 {
		dv.eqH = 0
	}
	// Mobile EQ collapse: when the user has toggled EQ off, force zero height.
	if p.IsMobile() && dv.mobileEQCollapsed {
		dv.eqH = 0
	}
	// On small screens, shrink EQ panel to guarantee at least 2 visible rows.
	if p.IsMobile() && !dv.mobileEQCollapsed {
		maxEQ := dv.Bounds.Dy() - dv.headerH - dv.rowHeight()*2
		if maxEQ < 0 {
			maxEQ = 0
		}
		if dv.eqH > maxEQ {
			dv.eqH = maxEQ
		}
	}
	// Safety guard: if we have rows but can't fit any, collapse EQ to make room.
	if dv.rowsAreaHeight() < dv.rowHeight() && len(dv.Rows) > 0 && dv.eqH > 0 {
		needed := dv.rowHeight() - dv.rowsAreaHeight()
		dv.eqH -= needed
		if dv.eqH < 0 {
			dv.eqH = 0
		}
	}

	// Invariant: when dv.eqH is forced to 0 (test-mode eqPanelHeight=0,
	// mobile EQ collapsed, safety guard, or any other collapse path),
	// the row rack should span the full available rows-area so per-row
	// widgets don't end up positioned at Y bands past the rack's
	// painted area (the screenshot bug's "phantom row + misplaced
	// add-row button"). We extend only as far as the wave widget's top
	// edge — when wave is non-empty it owns those pixels, even if
	// dv.eqH is forced to 0 in tests; rack and wave must NOT overlap.
	if dv.eqH == 0 {
		desiredBottom := dv.Bounds.Max.Y
		if waveR := dv.widgetRects[WidgetWave]; !waveR.Empty() {
			desiredBottom = waveR.Min.Y
		}
		if r := dv.widgetRects[WidgetRack]; !r.Empty() && r.Max.Y < desiredBottom {
			r.Max.Y = desiredBottom
			dv.widgetRects[WidgetRack] = r
		}
		if r := dv.widgetRects[WidgetTimeline]; !r.Empty() && r.Max.Y < desiredBottom {
			r.Max.Y = desiredBottom
			dv.widgetRects[WidgetTimeline] = r
		}
	}
	leftW := dv.widgets.ColWidth(0)
	if leftW <= 0 {
		leftW = dv.labelW + dv.controlsW
	}
	dv.controlsW = leftW - dv.labelW
	if dv.controlsW < 180 {
		dv.controlsW = 180
	}
	if dv.controlsW > 520 {
		dv.controlsW = 520
	}
	// On narrow screens, cap controls so label+controls fits in the column.
	if leftW > 0 && dv.labelW+dv.controlsW > leftW {
		dv.controlsW = leftW - dv.labelW
		if dv.controlsW < 0 {
			dv.controlsW = 0
		}
	}

	// Tighten rack column to actual content width (prevents black gap
	// between controls and timeline on wide windows). controlsW is
	// already capped at 520, so when the column is wider than
	// labelW + controlsW the excess is wasted black space.
	needW := dv.labelW + dv.controlsW
	if dv.widgets != nil && len(dv.widgets.cols) >= 2 {
		col0W := dv.widgets.ColWidth(0)
		if col0W > needW+SpaceMD {
			totalWeight := dv.widgets.cols[0] + dv.widgets.cols[1]
			newCol0 := float64(needW+SpaceMD) / float64(dv.Bounds.Dx()) * totalWeight
			if newCol0 < 0.3*totalWeight {
				newCol0 = 0.3 * totalWeight // floor
			}
			dv.widgets.cols[0] = newCol0
			dv.widgets.cols[1] = totalWeight - newCol0
			dv.widgets.recalc()
			// Re-read rects after recalc.
			dv.widgetRects[WidgetTransport] = dv.widgets.Rect(WidgetTransport)
			dv.widgetRects[WidgetRack] = dv.widgets.Rect(WidgetRack)
			dv.widgetRects[WidgetTimeline] = dv.widgets.Rect(WidgetTimeline)
			dv.widgetRects[WidgetWave] = dv.widgets.Rect(WidgetWave)
			// Re-apply wave-hidden extension after recalc.
			if p.IsMobile() && dv.mobileEQCollapsed && dv.widgetRects[WidgetWave].Empty() {
				if r := dv.widgetRects[WidgetRack]; !r.Empty() {
					r.Max.Y = dv.Bounds.Max.Y
					dv.widgetRects[WidgetRack] = r
				}
				if r := dv.widgetRects[WidgetTimeline]; !r.Empty() {
					r.Max.Y = dv.Bounds.Max.Y
					dv.widgetRects[WidgetTimeline] = r
				}
			}
		}
	}

	// Clamp scroll so end of list stays reachable after resizes.
	maxOff := len(dv.Rows) - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset > maxOff {
		dv.rowOffset = maxOff
	}
}

// handleLayoutEdit enables lightweight widget resizing when layout edit mode
// is active. Toggle with the "L" key. Drags near column/row boundaries adjust
// weights and trigger a layout rebuild.
func (dv *DrumView) handleLayoutResize() {
	if dv.widgets == nil {
		return
	}
	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	// Do not treat scrollbar drags as layout resizes.
	if image.Pt(mx, my).In(dv.scrollBarRect()) {
		dv.layoutHoverAxis = ""
		dv.layoutHoverIdx = -1
		if !left {
			dv.layoutDragIdx = -1
		}
		return
	}
	if dv.layoutDragIdx >= 0 {
		cur := mx
		if dv.layoutDragAxis == "row" {
			cur = my
		}
		delta := cur - dv.layoutDragPrev
		if delta != 0 {
			dv.widgets.ResizeAxis(dv.layoutDragAxis, dv.layoutDragIdx, delta)
			dv.refreshWidgetLayout()
			dv.recalcButtons()
			dv.calcLayout()
			dv.invalidateRowCaches()
			dv.rowsLayerDirty = true
		}
		dv.layoutDragPrev = cur
		if !left {
			dv.layoutDragIdx = -1
		}
		return
	}
	dv.layoutHoverAxis = ""
	dv.layoutHoverIdx = -1
	const grab = 6
	off := dv.Bounds.Min
	for i := 1; i < len(dv.widgets.colPos)-1; i++ {
		x := dv.widgets.colPos[i] + off.X
		if utils.Abs(mx-x) <= grab && my >= dv.Bounds.Min.Y && my <= dv.Bounds.Max.Y {
			dv.layoutHoverAxis = "col"
			dv.layoutHoverIdx = i - 1
			if left {
				dv.layoutDragAxis = "col"
				dv.layoutDragIdx = i - 1
				dv.layoutDragPrev = mx
			}
			return
		}
	}
	for i := 1; i < len(dv.widgets.rowPos)-1; i++ {
		if dv.layoutHandler != nil && dv.layoutHandler.anyWidgetSpansRow(i-1) {
			continue
		}
		y := dv.widgets.rowPos[i] + off.Y
		if utils.Abs(my-y) <= grab && mx >= dv.Bounds.Min.X && mx <= dv.Bounds.Max.X {
			dv.layoutHoverAxis = "row"
			dv.layoutHoverIdx = i - 1
			if left {
				dv.layoutDragAxis = "row"
				dv.layoutDragIdx = i - 1
				dv.layoutDragPrev = my
			}
			return
		}
	}
}

// WidgetRectsSnapshot captures the current widget layout and key control rects.
type WidgetRectsSnapshot struct {
	Layout    LayoutSnapshot
	AddButton image.Rectangle
	Timeline  image.Rectangle
	Rack      image.Rectangle
	Wave      image.Rectangle
	Transport image.Rectangle
}

func (dv *DrumView) widgetRectsSnapshot() WidgetRectsSnapshot {
	snap := WidgetRectsSnapshot{
		AddButton: dv.addRowBtn().Rect(),
		Timeline:  dv.timelineRect,
	}
	if dv.widgets != nil {
		snap.Layout = dv.widgets.Snapshot()
	}
	if r, ok := dv.widgetRects[WidgetRack]; ok {
		snap.Rack = r
	}
	if r, ok := dv.widgetRects[WidgetWave]; ok {
		snap.Wave = r
	}
	if r, ok := dv.widgetRects[WidgetTransport]; ok {
		snap.Transport = r
	}
	return snap
}

func (dv *DrumView) visibleRows() int {
	rh := dv.rowHeight()
	if rh <= 0 {
		return 0
	}
	h := dv.rowsAreaHeight()
	if Profile().ReserveAddRowSpace {
		h -= rh // reserve one rowHeight for the "+" footer (desktop only)
	}
	if h < 0 {
		h = 0
	}
	n := h / rh
	// Guarantee at least 1 visible row when the area fits a full row,
	// even if there isn't extra space for the "+" footer.
	if n == 0 && dv.rowsAreaHeight() >= rh {
		n = 1
	}
	return n
}

// rackVisibleRows returns the number of rows that fit inside the rack
// widget rect (the actual region the row rack zone draws into), reserving
// space for the "+" add-row button on desktop. This is the authoritative
// row-count source for the row rack zone — it never over-estimates beyond
// the panel's true height. Falls back to dv.visibleRows() when the rack
// rect is empty (early init / tests without a realised widget board).
//
// Clamps the rack height to rowsBottom() so the count does NOT include
// rows that would draw on top of the mobile bottom action bar / EQ peek
// strip — the WidgetBoard's rect extends to dv.Bounds.Max.Y by default
// but the addRowBtn is anchored to rowsBottom() (which excludes those
// surfaces). Without this clamp, with many rows the over-counted
// "visible" rows would render UNDER the anchored addRowBtn band
// (regression caught by TestMobileAddRowButton_AlwaysVisible_ManyRows
// after the rack-bottom anchor fix).
func (dv *DrumView) rackVisibleRows() int {
	rh := dv.rowHeight()
	if rh <= 0 {
		return 0
	}
	r := dv.widgetRects[WidgetRack]
	if r.Empty() {
		return dv.visibleRows()
	}
	maxY := r.Max.Y
	if rb := dv.rowsBottom(); rb < maxY {
		maxY = rb
	}
	rackH := maxY - r.Min.Y
	if rackH < 0 {
		rackH = 0
	}
	h := rackH
	if Profile().ReserveAddRowSpace {
		h -= rh
	}
	if h < 0 {
		h = 0
	}
	n := h / rh
	if n == 0 && rackH >= rh {
		n = 1
	}
	return n
}

func (dv *DrumView) scrollBarRect() image.Rectangle {
	dv.syncRowScroll()
	return dv.rowScroll().BarRect()
}

func (dv *DrumView) scrollThumbRect() image.Rectangle {
	dv.syncRowScroll()
	return dv.rowScroll().ThumbRect()
}

// syncRowScroll copies the current row state into rowScroll so its geometry
// and clamping are up to date. Call before reading BarRect/ThumbRect or
// invoking any ScrollBehavior input handler.
func (dv *DrumView) syncRowScroll() {
	if dv.rowScroll() == nil {
		return
	}
	dv.rowScroll().VS.Total = len(dv.Rows)
	dv.rowScroll().VS.Visible = dv.visibleRows()
	dv.rowScroll().VS.First = dv.rowOffset
	dv.rowScroll().ItemHeight = dv.rowHeight()
	// Set View for BarRect/ThumbRect calculations.
	// Constrain the scrollbar track to the actual visible rows area so it
	// does not extend into leftover space (e.g., the FAB overlay region).
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	vis := dv.visibleRows()
	y1 := rowsTop + vis*dv.rowHeight()
	if maxY := rowsTop + dv.rowsAreaHeight(); y1 > maxY {
		y1 = maxY
	}
	x1 := dv.widgetRects[WidgetTimeline].Max.X
	if x1 == 0 {
		x1 = dv.Bounds.Max.X
	}
	dv.rowScroll().VS.View = image.Rect(x1-dv.rowScroll().Style.Width, rowsTop, x1, y1)
}

// flushRowScroll writes rowScroll.VS.First back to rowOffset and recalculates
// layout if the value changed.
func (dv *DrumView) flushRowScroll() {
	if dv.rowScroll() == nil {
		return
	}
	if dv.rowScroll().VS.First != dv.rowOffset {
		dv.rowOffset = dv.rowScroll().VS.First
		if dv.rowRackZone != nil {
			dv.rowRackZone.SetRowOffset(dv.rowOffset)
		}
		dv.calcLayout()
	}
}
