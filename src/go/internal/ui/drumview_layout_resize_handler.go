package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

// LayoutResizeHandler implements InputHandler for the DrumView layout dividers.
// It provides widget-span-aware hit detection so that column dividers are only
// detected in rows where no widget spans across the column boundary.
type LayoutResizeHandler struct {
	dv       *DrumView
	dragging bool
	dragAxis string // "col" or "row"
	dragIdx  int
	dragPrev int
}

// NewLayoutResizeHandler creates a new layout resize handler for the given DrumView.
func NewLayoutResizeHandler(dv *DrumView) *LayoutResizeHandler {
	return &LayoutResizeHandler{
		dv:       dv,
		dragIdx:  -1,
		dragPrev: 0,
	}
}

// InputBounds returns the DrumView bounds for input dispatch.
func (h *LayoutResizeHandler) InputBounds() image.Rectangle {
	return h.dv.Bounds
}

// ZIndex returns the z-order for input dispatch.
// Higher than DrumView (100) but lower than overlays (200+).
func (h *LayoutResizeHandler) ZIndex() int { return 105 }

// Capturing reports whether a drag is in progress.
func (h *LayoutResizeHandler) Capturing() bool {
	return h.dragging
}

const layoutGrab = 6 // pixels from divider edge to detect

// HandleInput processes mouse input for layout resize.
// Returns InputCaptured while dragging, InputConsumed when starting a drag,
// or InputIgnored when not over a divider.
func (h *LayoutResizeHandler) HandleInput(x, y int, pressed bool) InputResult {
	if h.dv.widgets == nil {
		return InputIgnored
	}

	// Do not handle if over scrollbar
	if image.Pt(x, y).In(h.dv.scrollBarRect()) {
		h.syncHoverState("", -1)
		if !pressed && h.dragging {
			h.endDrag()
		}
		return InputIgnored
	}

	// Continue drag if in progress (even if cursor moves outside bounds)
	if h.dragging {
		h.handleDrag(x, y, pressed)
		if !pressed {
			h.endDrag()
		}
		return InputCaptured
	}

	// Only detect new dividers when cursor is in bounds
	if !image.Pt(x, y).In(h.dv.Bounds) {
		h.syncHoverState("", -1)
		return InputIgnored
	}

	// Check column dividers (only in rows without spanning widgets)
	if axis, idx := h.detectColumnDivider(x, y); idx >= 0 {
		handleR := h.columnHandleRect(idx)
		nearPill := image.Pt(x, y).In(handleR.Inset(-SpaceSM))
		if nearPill {
			h.syncHoverState(axis, idx)
		} else {
			h.syncHoverState("", -1)
		}
		if pressed && nearPill {
			h.startDrag(axis, idx, x)
			return InputConsumed
		}
		return InputIgnored
	}

	// Check row dividers (skip if inside any row control button/slider)
	if axis, idx := h.detectRowDivider(x, y); idx >= 0 {
		if !h.pointInsideRowControl(x, y) {
			handleR := h.rowHandleRect(idx)
			nearPill := image.Pt(x, y).In(handleR.Inset(-SpaceSM))
			if nearPill {
				h.syncHoverState(axis, idx)
			} else {
				h.syncHoverState("", -1)
			}
			if pressed && nearPill {
				h.startDrag(axis, idx, y)
				suppressClicksUntilRelease = true
				return InputConsumed
			}
			return InputIgnored
		}
	}

	h.syncHoverState("", -1)
	return InputIgnored
}

// widgetSpansColumn reports whether any visible widget at rowIdx spans across
// the boundary between colIdx and colIdx+1.
func (h *LayoutResizeHandler) widgetSpansColumn(colIdx, rowIdx int) bool {
	if h.dv.widgets == nil {
		return false
	}
	for _, p := range h.dv.widgets.placements {
		if !p.Visible {
			continue
		}
		// Check if this widget occupies rowIdx
		if p.Row > rowIdx || p.Row+p.RowSpan <= rowIdx {
			continue
		}
		// Check if it spans the column boundary (colIdx | colIdx+1)
		if p.Col <= colIdx && p.Col+p.ColSpan > colIdx+1 {
			return true
		}
	}
	return false
}

// anyWidgetSpansRow reports whether any visible widget spans across the
// boundary between rowIdx and rowIdx+1 (i.e. has RowSpan > 1 crossing that
// boundary). When true, the row divider line and pill are suppressed.
func (h *LayoutResizeHandler) anyWidgetSpansRow(rowIdx int) bool {
	if h.dv.widgets == nil {
		return false
	}
	for _, p := range h.dv.widgets.placements {
		if !p.Visible {
			continue
		}
		if p.Row <= rowIdx && p.Row+p.RowSpan > rowIdx+1 {
			return true
		}
	}
	return false
}

// fullWidthWidgetBelow reports whether any visible widget at rowIdx+1 has
// ColSpan >= len(cols), meaning it occupies the full width. When true, the
// row divider line is suppressed but a pill is still drawn for resizing.
func (h *LayoutResizeHandler) fullWidthWidgetBelow(rowIdx int) bool {
	if h.dv.widgets == nil {
		return false
	}
	numCols := len(h.dv.widgets.cols)
	for _, p := range h.dv.widgets.placements {
		if !p.Visible {
			continue
		}
		if p.Row == rowIdx+1 && p.ColSpan >= numCols {
			return true
		}
	}
	return false
}

// widgetSpansRow reports whether any visible widget at colIdx spans across
// the boundary between rowIdx and rowIdx+1.
func (h *LayoutResizeHandler) widgetSpansRow(rowIdx, colIdx int) bool {
	if h.dv.widgets == nil {
		return false
	}
	for _, p := range h.dv.widgets.placements {
		if !p.Visible {
			continue
		}
		if p.Col > colIdx || p.Col+p.ColSpan <= colIdx {
			continue
		}
		if p.Row <= rowIdx && p.Row+p.RowSpan > rowIdx+1 {
			return true
		}
	}
	return false
}

// columnDividerSegments returns the Y-range segments where a column divider
// at colIdx can be grabbed. Rows where a widget spans across the boundary
// are excluded.
func (h *LayoutResizeHandler) columnDividerSegments(colIdx int) []image.Rectangle {
	if h.dv.widgets == nil {
		return nil
	}
	var segments []image.Rectangle
	x := h.dv.widgets.colPos[colIdx+1] + h.dv.widgets.offset.X
	numRows := len(h.dv.widgets.rows)

	for rowIdx := 0; rowIdx < numRows; rowIdx++ {
		if h.widgetSpansColumn(colIdx, rowIdx) {
			continue
		}
		y0 := h.dv.widgets.rowPos[rowIdx] + h.dv.widgets.offset.Y
		y1 := h.dv.widgets.rowPos[rowIdx+1] + h.dv.widgets.offset.Y
		segments = append(segments, image.Rect(x-layoutGrab, y0, x+layoutGrab, y1))
	}
	return segments
}

// detectColumnDivider checks if (x,y) is within a valid column divider segment.
// Returns ("col", idx) if detected, ("", -1) otherwise.
func (h *LayoutResizeHandler) detectColumnDivider(x, y int) (string, int) {
	if h.dv.widgets == nil {
		return "", -1
	}
	if isSmallScreen() {
		return "", -1
	}
	off := h.dv.Bounds.Min
	for i := 1; i < len(h.dv.widgets.colPos)-1; i++ {
		colX := h.dv.widgets.colPos[i] + off.X
		if utils.Abs(x-colX) > layoutGrab {
			continue
		}
		// Check if Y falls within any valid segment for this column divider
		segments := h.columnDividerSegments(i - 1)
		for _, seg := range segments {
			if y >= seg.Min.Y && y <= seg.Max.Y {
				return "col", i - 1
			}
		}
	}
	return "", -1
}

// rowDividerSegments returns the X-range segments where a row divider
// at rowIdx can be grabbed. Columns where a widget spans across the boundary
// are excluded.
func (h *LayoutResizeHandler) rowDividerSegments(rowIdx int) []image.Rectangle {
	if h.dv.widgets == nil {
		return nil
	}
	if h.fullWidthWidgetBelow(rowIdx) || h.anyWidgetSpansRow(rowIdx) {
		return nil
	}
	var segments []image.Rectangle
	y := h.dv.widgets.rowPos[rowIdx+1] + h.dv.widgets.offset.Y
	numCols := len(h.dv.widgets.cols)

	for colIdx := 0; colIdx < numCols; colIdx++ {
		if h.widgetSpansRow(rowIdx, colIdx) {
			continue
		}
		x0 := h.dv.widgets.colPos[colIdx] + h.dv.widgets.offset.X
		x1 := h.dv.widgets.colPos[colIdx+1] + h.dv.widgets.offset.X
		segments = append(segments, image.Rect(x0, y-layoutGrab, x1, y+layoutGrab))
	}
	return segments
}

// detectRowDivider checks if (x,y) is over a row divider.
// Returns ("row", idx) if detected, ("", -1) otherwise.
// Uses widget-span-aware detection: row dividers are only grabbable in
// columns where no widget spans across the row boundary.
func (h *LayoutResizeHandler) detectRowDivider(x, y int) (string, int) {
	if h.dv.widgets == nil {
		return "", -1
	}
	if isSmallScreen() {
		return "", -1
	}
	off := h.dv.Bounds.Min
	for i := 1; i < len(h.dv.widgets.rowPos)-1; i++ {
		rowY := h.dv.widgets.rowPos[i] + off.Y
		if utils.Abs(y-rowY) > layoutGrab {
			continue
		}
		// Check if X falls within any valid line segment for this row divider
		segments := h.rowDividerSegments(i - 1)
		for _, seg := range segments {
			if x >= seg.Min.X && x <= seg.Max.X {
				return "row", i - 1
			}
		}
		// No line segments but a pill exists (EQ boundary): detect by Y
		// proximity alone; HandleInput will further filter by pill proximity.
		if len(segments) == 0 && h.fullWidthWidgetBelow(i-1) {
			return "row", i - 1
		}
	}
	return "", -1
}

// columnHandleRect returns the pill handle rect for the column divider at idx.
// The handle is centered vertically on the combined segment Y-extent, so it
// sits within the valid rows rather than centered on the full drum view height.
func (h *LayoutResizeHandler) columnHandleRect(idx int) image.Rectangle {
	if h.dv.widgets == nil || idx < 0 || idx+1 >= len(h.dv.widgets.colPos) {
		return image.Rectangle{}
	}
	segments := h.columnDividerSegments(idx)
	if len(segments) == 0 {
		return image.Rectangle{}
	}
	off := h.dv.Bounds.Min
	x := h.dv.widgets.colPos[idx+1] + off.X
	cy := (segments[0].Min.Y + segments[len(segments)-1].Max.Y) / 2
	return SplitterHandleRect(x, cy, false)
}

// rowHandleRect returns the pill handle rect for the row divider at idx.
// The handle is centered horizontally on the combined segment X-extent.
// When a full-width widget sits below (EQ boundary), the pill is centered
// on the full drum view width even though line segments are suppressed.
// Returns an empty rect when fully suppressed (e.g. a multi-row widget spans
// across the boundary).
func (h *LayoutResizeHandler) rowHandleRect(idx int) image.Rectangle {
	if h.dv.widgets == nil || idx < 0 || idx+1 >= len(h.dv.widgets.rowPos) {
		return image.Rectangle{}
	}
	segments := h.rowDividerSegments(idx)
	off := h.dv.Bounds.Min
	y := h.dv.widgets.rowPos[idx+1] + off.Y
	if len(segments) > 0 {
		cx := (segments[0].Min.X + segments[len(segments)-1].Max.X) / 2
		return SplitterHandleRect(cx, y, true)
	}
	// No line segments but a full-width widget below: show a pill-only
	// resize handle centered on the full width (EQ boundary).
	if h.fullWidthWidgetBelow(idx) {
		cx := (h.dv.Bounds.Min.X + h.dv.Bounds.Max.X) / 2
		return SplitterHandleRect(cx, y, true)
	}
	return image.Rectangle{}
}

// startDrag initiates a drag operation.
func (h *LayoutResizeHandler) startDrag(axis string, idx, pos int) {
	h.dragging = true
	h.dragAxis = axis
	h.dragIdx = idx
	h.dragPrev = pos
	// Sync to DrumView for cursor display
	h.dv.layoutDragAxis = axis
	h.dv.layoutDragIdx = idx
	h.dv.layoutDragPrev = pos
}

// endDrag completes a drag operation.
func (h *LayoutResizeHandler) endDrag() {
	h.dragging = false
	h.dragIdx = -1
	h.dv.layoutDragIdx = -1
}

// handleDrag processes drag movement.
func (h *LayoutResizeHandler) handleDrag(x, y int, pressed bool) {
	if !pressed {
		return
	}
	cur := x
	if h.dragAxis == "row" {
		cur = y
	}
	delta := cur - h.dragPrev
	if delta != 0 {
		h.dv.widgets.ResizeAxis(h.dragAxis, h.dragIdx, delta)
		h.dv.refreshWidgetLayout()
		h.dv.recalcButtons()
		h.dv.calcLayout()
		h.dv.invalidateRowCaches()
		h.dv.rowsLayerDirty = true
	}
	h.dragPrev = cur
	h.dv.layoutDragPrev = cur
}

// pointInsideRowControl reports whether (x,y) is inside any non-empty
// per-row button or the timeline bar. This prevents the row divider
// grab zone from stealing clicks meant for row controls.
func (h *LayoutResizeHandler) pointInsideRowControl(x, y int) bool {
	pt := image.Pt(x, y)
	for i := range h.dv.rowGroups {
		for _, btn := range []*Button{
			h.dv.rowGroups[i].Mute, h.dv.rowGroups[i].Solo,
			h.dv.rowGroups[i].FX, h.dv.rowGroups[i].Origin,
			h.dv.rowGroups[i].Delete, h.dv.rowGroups[i].Edit,
			h.dv.rowGroups[i].Save, h.dv.rowGroups[i].Menu,
			h.dv.rowGroups[i].Label,
		} {
			if btn != nil && !btn.Rect().Empty() && pt.In(btn.Rect()) {
				return true
			}
		}
	}
	for _, s := range h.dv.rowVolSliders {
		if s != nil && !s.Rect().Empty() && pt.In(s.Rect()) {
			return true
		}
	}
	return false
}

// syncHoverState updates DrumView hover state for cursor display.
func (h *LayoutResizeHandler) syncHoverState(axis string, idx int) {
	h.dv.layoutHoverAxis = axis
	h.dv.layoutHoverIdx = idx
}

// Update processes layout resize each frame. Called from DrumView.Update()
// when no overlay menus are open. This is a compatibility method that
// delegates to HandleInput using current mouse state.
func (h *LayoutResizeHandler) Update() bool {
	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	result := h.HandleInput(mx, my, left)
	return result == InputCaptured || result == InputConsumed
}

// HandleWheel does not process wheel events for layout resize.
func (h *LayoutResizeHandler) HandleWheel(x, y, steps int) InputResult {
	return InputIgnored
}
