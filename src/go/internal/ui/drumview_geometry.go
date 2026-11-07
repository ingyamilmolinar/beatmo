package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

/* ─── geometry helpers ─────────────────────────────────────── */

// Fixed, never zoomed: rows are the user's stable touch targets, and the splitter math assumes
// rh = TouchRowHeight(). The zoom chips resize the whole pane instead.
func (dv *DrumView) rowHeight() int {
	return TouchRowHeight()
}

func mobileTransportMinH() int { return Profile().HeaderMinH }

func (dv *DrumView) HeaderRect() image.Rectangle {
	return image.Rect(
		dv.Bounds.Min.X,
		dv.Bounds.Min.Y,
		dv.Bounds.Max.X,
		dv.Bounds.Min.Y+dv.headerH,
	)
}

// The mobile toast's fallback host, so the toast lands on the panel instead of covering the
// transport it describes.
func (dv *DrumView) BelowHeaderRect() image.Rectangle {
	r := dv.Bounds
	r.Min.Y = dv.HeaderRect().Max.Y
	if r.Min.Y > r.Max.Y {
		r.Min.Y = r.Max.Y
	}
	return r
}

// Header, sections and knob chrome need this even when the profile multiplier resolves smaller.
const minSynthPanelH = 240

// The ONE floor shared by eqRect and eqH (which drives rowsBottom): deriving both from it is what
// keeps the row rack from rendering under the panel. 0 without a panel.
func (dv *DrumView) audioPanelFloorHeight() int {
	if dv.eqPanelZone == nil || dv.eqPanelZone.tabState == nil {
		return 0
	}
	h := dv.eqPanelZone.tabState.PanelHeightAt(dv.Bounds.Dy())
	if dv.eqPanelZone.ActiveTab() == TabSynth && h < minSynthPanelH {
		h = minSynthPanelH
	}
	return h
}

// A tab switch to Synth does not re-run refreshWidgetLayout, so eqH can lag the panel's floored
// height and the rack would render under it. Only ever raises eqH; skipped for a user drag and mobile.
func (dv *DrumView) reconcileEQHeightToPanelFloor() {
	if dv.userEqH != 0 || Profile().IsMobile() {
		return
	}
	if runningUnderGoTest() && eqPanelHeight == 0 {
		return // test-mode collapse pins eqH = 0; don't re-inflate it.
	}
	floorH := dv.audioPanelFloorHeight()
	if floorH <= 0 {
		return
	}
	if floorH > dv.Bounds.Dy() {
		floorH = dv.Bounds.Dy()
	}
	if dv.eqH < floorH {
		dv.eqH = floorH
	}
}

// RowsRect is the public alias for the rows scroll area; see rowsRect.
func (dv *DrumView) RowsRect() image.Rectangle { return dv.rowsRect() }

// Through rowsBottom(), so the mobile bottom bar is excluded from the touch dead zone.
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

	// Mobile with the EQ collapsed: stretch the rack and timeline over the row the board still
	// allocates, or a grey gap shows below the last row.
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

	// The collapse extension runs below, after eqH is known.

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
	maxH := spec.Height
	if dv.headerH > maxH {
		dv.headerH = maxH
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
	// The board's row weights leave the analysis tabs too short, so lift the floor; a user drag may
	// enlarge the panel but never shrink it below the tab minimum.
	if dv.userEqH > 0 && !p.IsMobile() {
		// A user divider drag overrides the floor in both directions, or in a short pane it has no
		// visible effect.
		dv.eqH = clampUserEqH(dv, dv.userEqH)
	} else if dv.eqPanelZone != nil && dv.eqPanelZone.tabState != nil {
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

	// Last, after every clamp above.
	dv.reconcileEQHeightToPanelFloor()

	// With eqH forced to 0 the rack spans the whole rows area, but only down to the wave widget's
	// top: the two must never overlap.
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
	// The real fixed cluster, not "what the column leaves after the label": that circular definition
	// let the label bar grow into empty space.
	dv.controlsW = dv.rackControlsWidth()
	// On narrow screens, cap controls so label+controls fits in the column.
	if leftW > 0 && dv.labelW+dv.controlsW > leftW {
		dv.controlsW = leftW - dv.labelW
		if dv.controlsW < 0 {
			dv.controlsW = 0
		}
	}

	// The rack bed paints exactly labelW + controlsW, so a wider column shows a background lane.
	needW := dv.labelW + dv.rackControlsWidth()
	if dv.widgets != nil && len(dv.widgets.cols) >= 2 && dv.Bounds.Dx() > 0 {
		col0W := dv.widgets.ColWidth(0)
		totalWeight := dv.widgets.cols[0] + dv.widgets.cols[1]
		maxShare := 0.5
		if pw := p.ColWeights; len(pw) >= 2 && pw[0]+pw[1] > 0 {
			maxShare = pw[0] / (pw[0] + pw[1])
		}
		target := needW
		if lim := int(maxShare * float64(dv.Bounds.Dx())); target > lim {
			target = lim
		}
		if target != col0W && target > 0 && !dv.widgets.userColSized {
			newCol0 := float64(target) / float64(dv.Bounds.Dx()) * totalWeight
			dv.widgets.cols[0] = newCol0
			dv.widgets.cols[1] = totalWeight - newCol0
			dv.widgets.recalc()
			// Take only the new X edges: re-reading whole rects undoes the Y adjustments made above.
			for _, kind := range []WidgetKind{WidgetTransport, WidgetRack, WidgetTimeline, WidgetWave} {
				r, ok := dv.widgetRects[kind]
				if !ok || r.Empty() {
					continue
				}
				nr := dv.widgets.Rect(kind)
				r.Min.X, r.Max.X = nr.Min.X, nr.Max.X
				dv.widgetRects[kind] = r
			}
		}
		// The board can hold the column above target, and anything the bed leaves unpainted shows as a lane.
		if c := dv.widgets.ColWidth(0); c > 0 {
			dv.controlsW = c - dv.labelW
			if dv.controlsW < 0 {
				dv.controlsW = 0
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

// Never shorter than two sticky-bar rows, never tall enough to hide the transport and every row;
// re-applied every layout so it tracks window resizes.
func clampUserEqH(dv *DrumView, h int) int {
	minH := 2 * stickyBarHeight()
	maxH := dv.Bounds.Dy() - dv.headerH - dv.rowHeight()
	if maxH < minH {
		maxH = minH
	}
	if h < minH {
		h = minH
	}
	if h > maxH {
		h = maxH
	}
	return h
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
	if n == 0 && dv.rowsAreaHeight() >= rh {
		n = 1
	}
	return n
}

// The rack zone's row count, clamped to rowsBottom(): the board's rect reaches the pane bottom,
// and over-counted rows would render under the anchored add-row band.
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

// Call before reading BarRect/ThumbRect or any ScrollBehavior input handler.
func (dv *DrumView) syncRowScroll() {
	if dv.rowScroll() == nil {
		return
	}
	dv.rowScroll().VS.Total = len(dv.Rows)
	dv.rowScroll().VS.Visible = dv.visibleRows()
	dv.rowScroll().VS.First = dv.rowOffset
	dv.rowScroll().ItemHeight = dv.rowHeight()
	// The track spans only the visible rows area.
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

// 180 px until the rack zone exists.
func (dv *DrumView) rackControlsWidth() int {
	if dv.rowRackZone == nil {
		return 180
	}
	return dv.rowRackZone.FixedControlsWidth()
}
