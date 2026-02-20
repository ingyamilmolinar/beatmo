package ui

import (
	"image"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
)

// layoutResizeZone wraps LayoutResizeHandler as a Zone so it participates
// in the DrumViewTree's z-ordered input dispatch. Hit areas are generated
// from the pill handle rects at ZResize (200).
type layoutResizeZone struct {
	handler *LayoutResizeHandler
	rect    image.Rectangle
	dirty   bool
}

func newLayoutResizeZone(h *LayoutResizeHandler) *layoutResizeZone {
	return &layoutResizeZone{handler: h, dirty: true}
}

func (z *layoutResizeZone) ID() string { return "layout-resize" }

func (z *layoutResizeZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.dirty = false
}

func (z *layoutResizeZone) NeedsLayout() bool { return z.dirty }
func (z *layoutResizeZone) Invalidate()       { z.dirty = true }

// Update runs hover detection every frame so the cursor changes when
// hovering over divider pills. Skipped on WASM where the JS interop
// cost of cursorPosition() outweighs the visual benefit.
func (z *layoutResizeZone) Update() {
	if z.handler == nil || z.handler.dragging || runtime.GOARCH == "wasm" {
		return
	}
	mx, my := cursorPosition()
	if !image.Pt(mx, my).In(z.handler.dv.Bounds) {
		z.handler.syncHoverState("", -1)
		return
	}
	// Hover detection only (no press): check pill proximity.
	if axis, idx := z.handler.detectColumnDivider(mx, my); idx >= 0 {
		handleR := z.handler.columnHandleRect(idx)
		if image.Pt(mx, my).In(handleR.Inset(-SpaceSM)) {
			z.handler.syncHoverState(axis, idx)
		} else {
			z.handler.syncHoverState("", -1)
		}
		return
	}
	if axis, idx := z.handler.detectRowDivider(mx, my); idx >= 0 {
		if !z.handler.pointInsideRowControl(mx, my) {
			handleR := z.handler.rowHandleRect(idx)
			if image.Pt(mx, my).In(handleR.Inset(-SpaceSM)) {
				z.handler.syncHoverState(axis, idx)
			} else {
				z.handler.syncHoverState("", -1)
			}
			return
		}
	}
	z.handler.syncHoverState("", -1)
}

// HitAreas returns pill-handle rects for all visible dividers.
func (z *layoutResizeZone) HitAreas() []HitArea {
	if z.handler == nil || z.handler.dv.widgets == nil || !Profile().EnableLayoutResize {
		return nil
	}
	var areas []HitArea

	// Column divider pills.
	numCols := len(z.handler.dv.widgets.colPos)
	for i := 0; i < numCols-2; i++ {
		r := z.handler.columnHandleRect(i)
		if r.Empty() {
			continue
		}
		// Expand hit area by SpaceSM around the pill.
		expanded := r.Inset(-SpaceSM)
		areas = append(areas, HitArea{
			Rect:    expanded,
			ZIndex:  ZResize,
			Handler: &layoutResizeHitHandler{handler: z.handler, axis: "col", idx: i},
			Tag:     "layout-resize-col",
		})
	}

	// Row divider pills.
	numRows := len(z.handler.dv.widgets.rowPos)
	for i := 0; i < numRows-2; i++ {
		r := z.handler.rowHandleRect(i)
		if r.Empty() {
			continue
		}
		expanded := r.Inset(-SpaceSM)
		areas = append(areas, HitArea{
			Rect:    expanded,
			ZIndex:  ZResize,
			Handler: &layoutResizeHitHandler{handler: z.handler, axis: "row", idx: i},
			Tag:     "layout-resize-row",
		})
	}

	return areas
}

func (z *layoutResizeZone) Draw(screen *ebiten.Image) {}
func (z *layoutResizeZone) HandleKey(key ebiten.Key) InputResult {
	return InputIgnored
}
func (z *layoutResizeZone) HandleChars(chars []rune) InputResult {
	return InputIgnored
}

// layoutResizeHitHandler adapts a single divider pill to the HitHandler
// interface for tree-based input dispatch.
type layoutResizeHitHandler struct {
	handler *LayoutResizeHandler
	axis    string
	idx     int
}

func (h *layoutResizeHitHandler) OnPress(x, y int) InputResult {
	pos := x
	if h.axis == "row" {
		pos = y
	}
	h.handler.startDrag(h.axis, h.idx, pos)
	return InputCaptured
}

func (h *layoutResizeHitHandler) OnDrag(x, y int) {
	h.handler.handleDrag(x, y, true)
}

func (h *layoutResizeHitHandler) OnRelease(x, y int) {
	h.handler.endDrag()
}

func (h *layoutResizeHitHandler) OnWheel(x, y, steps int) InputResult {
	return InputIgnored
}
