package ui

import "github.com/hajimehoshi/ebiten/v2"

// Embedded in DrumView, so its fields resolve by promotion.
type rowsWinCache struct {
	rowsWinBuf          *ebiten.Image // wide cache: width = rowWidth + padPx
	rowsWinBufW         int
	rowsWinBufH         int
	rowsWinBakeOffset   int  // dv.Offset the buffer's cell 0 corresponds to
	rowsWinRenderedTo   int  // exclusive buffer-cell index rendered so far
	rowsWinRowWidth     int  // visible window width (px) the buffer pitch uses
	rowsWinLength       int  // dv.Length the buffer was baked with
	rowsWinBaseX        int  // baseX the buffer was baked with
	rowsWinRowOff       int  // dv.rowOffset (vertical scroll) the buffer was baked with
	rowsWinContentDirty bool // a real content/edit change → force re-bake
	rowsWinValid        bool
}
