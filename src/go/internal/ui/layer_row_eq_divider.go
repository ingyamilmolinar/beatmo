package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// rowEQDividerLayer draws the production draggable divider between the
// drum-rows panel and the audio panel (EQ / analysis tabs) — the full-width
// row divider at the EQ boundary. It is DRAW-ONLY chrome: hit-testing, hover,
// and the actual resize are owned by LayoutResizeHandler / layoutResizeZone,
// which are already active on desktop (Profile().EnableLayoutResize).
//
// Visually it mirrors the main grid↔drum splitter (see (*Game).drawDivider):
// an azure "horizon" line (shadow + accent + soft bloom) plus a centered
// SplitterHandle pill that brightens on hover. Unlike LayoutPillsLayer — the
// debug-only chrome gated by Profile().ShowLayoutGuides that paints EVERY inner
// divider — this layer is visible in production but renders ONLY the EQ
// boundary. The debug pills layer defers this one divider to us (see
// LayoutPillsLayer.Draw) so the two never double-draw.
//
// Z = ZRowEQDivider (186), just above the debug pills, so the line composites
// over both the row rack and the audio panel.
type rowEQDividerLayer struct {
	dv     *DrumView
	handle SplitterHandle // shared animated line+pill (glow + grow on hover)
}

func newRowEQDividerLayer(dv *DrumView) *rowEQDividerLayer { return &rowEQDividerLayer{dv: dv} }

// rowEQDividerLayer returns the DrumView's EQ-boundary divider layer (the one
// registered in the tree), so its animated handle can be inspected/driven.
func (dv *DrumView) rowEQDividerLayer() *rowEQDividerLayer { return dv.rowEQDivider }

func (l *rowEQDividerLayer) ID() string  { return "row-eq-divider" }
func (l *rowEQDividerLayer) ZIndex() int { return ZRowEQDivider }

func (l *rowEQDividerLayer) Visible() bool { return l.dv.showRowEQDivider() }

func (l *rowEQDividerLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		return
	}
	hr := dv.layoutHandler.rowHandleRect(idx)
	if hr.Empty() {
		return
	}
	y := (hr.Min.Y + hr.Max.Y) / 2

	// Hover lights the pill. Read it from the resize handler's state (set by
	// layoutResizeZone.Update's cursor probe) OR — crucially — compute it
	// DIRECTLY from the cursor here, the same way (*Game).drawDivider does for
	// the main splitter. The probe is disabled on WASM (RuntimeProf().
	// SkipCursorHover), so relying on it alone left the EQ pill permanently
	// un-glowing in the browser. The direct check makes both pills behave
	// identically on every platform. Drag also lights it up.
	hover := (dv.layoutHoverAxis == "row" && dv.layoutHoverIdx == idx) ||
		(dv.layoutDragAxis == "row" && dv.layoutDragIdx == idx)
	if !hover && dv.layoutHandler != nil && !dv.layoutHandler.dragging {
		mx, my := cursorPosition()
		if image.Pt(mx, my).In(dv.layoutHandler.rowDividerGrabRect(idx)) &&
			!dv.layoutHandler.pointInsideRowControl(mx, my) {
			hover = true
		}
	}

	// Shared with the main grid↔drum splitter: identical azure line + animated
	// pill, drawn full-width across the drum view at the audio-panel top edge.
	l.handle.Advance(hover)
	l.handle.DrawHorizontalDivider(dst, dv.Bounds.Min.X, dv.Bounds.Max.X, y)
}

// eqDividerRowIdx returns the row-divider index for the EQ boundary — the
// full-width row divider between the drum-rows panel and the audio panel — or
// -1 when no such divider exists (e.g. a multi-row widget spans the boundary).
// This is the single divider the production rowEQDividerLayer renders.
func (dv *DrumView) eqDividerRowIdx() int {
	if dv.widgets == nil || dv.layoutHandler == nil {
		return -1
	}
	// Row dividers run between rowPos[i] and rowPos[i+1] for i in
	// [0, len(rowPos)-2). The EQ boundary is the one with a full-width widget
	// directly below it whose pill handle is non-empty.
	for i := 0; i < len(dv.widgets.rowPos)-2; i++ {
		if dv.layoutHandler.fullWidthWidgetBelow(i) && !dv.layoutHandler.rowHandleRect(i).Empty() {
			return i
		}
	}
	return -1
}

// showRowEQDivider reports whether the production EQ↔rows divider should be
// drawn. It gates the rowEQDividerLayer's Draw AND tells LayoutPillsLayer to
// skip the same divider so the two never double-draw. Desktop only: on mobile,
// layout resize is disabled, so there is no draggable divider to show.
func (dv *DrumView) showRowEQDivider() bool {
	if dv.perfDrawLite || dv.widgets == nil || dv.layoutHandler == nil {
		return false
	}
	if Profile().IsMobile() || !Profile().EnableLayoutResize {
		return false
	}
	return dv.eqDividerRowIdx() >= 0
}
