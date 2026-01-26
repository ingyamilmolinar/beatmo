package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/utils"
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
		h.syncHoverState(axis, idx)
		if pressed {
			h.startDrag(axis, idx, x)
			return InputConsumed
		}
		return InputIgnored
	}

	// Check row dividers
	if axis, idx := h.detectRowDivider(x, y); idx >= 0 {
		h.syncHoverState(axis, idx)
		if pressed {
			h.startDrag(axis, idx, y)
			suppressClicksUntilRelease = true
			return InputConsumed
		}
		return InputIgnored
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

// detectRowDivider checks if (x,y) is over a row divider.
// Returns ("row", idx) if detected, ("", -1) otherwise.
func (h *LayoutResizeHandler) detectRowDivider(x, y int) (string, int) {
	if h.dv.widgets == nil {
		return "", -1
	}
	off := h.dv.Bounds.Min
	for i := 1; i < len(h.dv.widgets.rowPos)-1; i++ {
		rowY := h.dv.widgets.rowPos[i] + off.Y
		if utils.Abs(y-rowY) <= layoutGrab && x >= h.dv.Bounds.Min.X && x <= h.dv.Bounds.Max.X {
			return "row", i - 1
		}
	}
	return "", -1
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
