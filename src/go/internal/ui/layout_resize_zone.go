package ui

import (
	"encoding/binary"
	"hash/fnv"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

type layoutResizeZone struct {
	handler *LayoutResizeHandler
	rect    image.Rectangle
	dirty   bool

	// Cached by geometry: recomputing every divider each frame was the largest layout cost left on WASM.
	areas    []HitArea
	areasKey uint64
	haveKey  bool
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

// Not on WASM, where cursorPosition()'s interop cost outweighs the hover.
func (z *layoutResizeZone) Update() {
	if z.handler == nil || z.handler.dragging || RuntimeProf().SkipCursorHover {
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
			if image.Pt(mx, my).In(z.handler.rowDividerGrabRect(idx)) {
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
	key := z.geometryKey()
	if z.haveKey && key == z.areasKey {
		return z.areas
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

	// The visible pill plus SpaceSM, so the footprint never exceeds what the user sees.
	numRows := len(z.handler.dv.widgets.rowPos)
	for i := 0; i < numRows-2; i++ {
		grab := z.handler.rowDividerGrabRect(i)
		if grab.Empty() {
			continue
		}
		areas = append(areas, HitArea{
			Rect:    grab,
			ZIndex:  ZResize,
			Handler: &layoutResizeHitHandler{handler: z.handler, axis: "row", idx: i},
			Tag:     "layout-resize-row",
		})
	}

	z.areas, z.areasKey, z.haveKey = areas, key, true
	return areas
}

// Every input the handle rects derive from, including the panel top the EQ boundary follows.
func (z *layoutResizeZone) geometryKey() uint64 {
	dv := z.handler.dv
	wb := dv.widgets
	h := fnv.New64a()
	var buf [8]byte
	put := func(vs ...int) {
		for _, v := range vs {
			binary.LittleEndian.PutUint64(buf[:], uint64(int64(v)))
			_, _ = h.Write(buf[:])
		}
	}
	put(dv.Bounds.Min.X, dv.Bounds.Min.Y, dv.Bounds.Max.X, dv.Bounds.Max.Y)
	put(dv.eqRect.Min.X, dv.eqRect.Min.Y, dv.eqRect.Max.X, dv.eqRect.Max.Y)
	put(wb.offset.X, wb.offset.Y, len(wb.cols), len(wb.rows), SplitterHandleLen(), SplitterHandleThick())
	put(len(wb.colPos))
	put(wb.colPos...)
	put(len(wb.rowPos))
	put(wb.rowPos...)
	// Placements are a map: combine per-entry hashes order-independently.
	var placements uint64
	for id, p := range wb.placements {
		e := fnv.New64a()
		_, _ = e.Write([]byte(id))
		var b [8]byte
		for _, v := range [...]int{p.Col, p.Row, p.ColSpan, p.RowSpan, boolInt(p.Visible)} {
			binary.LittleEndian.PutUint64(b[:], uint64(int64(v)))
			_, _ = e.Write(b[:])
		}
		placements += e.Sum64()
	}
	put(len(wb.placements), int(placements))
	return h.Sum64()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (z *layoutResizeZone) Draw(screen *ebiten.Image) {}
func (z *layoutResizeZone) HandleKey(key ebiten.Key) InputResult {
	return InputIgnored
}
func (z *layoutResizeZone) HandleChars(chars []rune) InputResult {
	return InputIgnored
}

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
