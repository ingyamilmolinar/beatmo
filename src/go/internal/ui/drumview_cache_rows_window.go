package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// During follow-scroll the row content only translates, so render a buffer wider than the
// window and blit a moving sub-rect; recompositing the layer every scroll steals the single
// WASM thread from the sequencer. Only for scrolled desktop follow playback at full resolution.

// rowsLayerWindowingEnabled toggles the windowed scroll cache (default on).
var rowsLayerWindowingEnabled = true

func SetRowsLayerWindowingForTest(v bool) func() {
	prev := rowsLayerWindowingEnabled
	rowsLayerWindowingEnabled = v
	return func() { rowsLayerWindowingEnabled = prev }
}

// A larger pad means fewer rebakes but a bigger buffer.
func rowsWinPadCells(length int) int {
	p := length / 2
	if p < 8 {
		p = 8
	}
	if p > 48 {
		p = 48
	}
	return p
}

// false: the caller falls back to rowsLayerMaybeRebuild.
func (dv *DrumView) drawRowCompositeWindowed(dst *ebiten.Image) bool {
	if !rowsLayerWindowingEnabled || Profile().DirectDrawRows {
		return false
	}
	if !dv.FollowPlayback() || dv.Offset <= 0 {
		return false
	}
	w, h := dv.Bounds.Dx(), dv.Bounds.Dy()
	rowWidth := dv.cellGridRect.Dx()
	n := dv.Length
	rh := dv.rowHeight()
	if w <= 0 || h <= 0 || rowWidth <= 0 || n <= 0 || rh <= 0 || n > rowWidth {
		return false // high-res (n>rowWidth) and degenerate cases use legacy.
	}
	baseX := dv.cellGridRect.Min.X - dv.Bounds.Min.X
	padCells := rowsWinPadCells(n)
	bufCells := n + padCells
	bufW := (bufCells*rowWidth)/n + 2
	bufH := h

	rebake := !dv.rowsWinValid || dv.rowsWinBuf == nil ||
		dv.rowsWinBufW != bufW || dv.rowsWinBufH != bufH ||
		dv.rowsWinRowWidth != rowWidth || dv.rowsWinLength != n ||
		dv.rowsWinBaseX != baseX || dv.rowsWinRowOff != dv.rowOffset ||
		dv.rowsWinContentDirty ||
		dv.Offset < dv.rowsWinBakeOffset ||
		(dv.Offset-dv.rowsWinBakeOffset) > padCells
	if rebake {
		if dv.rowsWinBuf == nil || dv.rowsWinBufW != bufW || dv.rowsWinBufH != bufH {
			releaseImage(dv.rowsWinBuf)
			dv.rowsWinBuf = newTrackedImage("rowsWinBuf", bufW, bufH)
		} else {
			dv.rowsWinBuf.Clear()
		}
		dv.rowsWinBakeOffset = dv.Offset
		dv.rowsWinRenderedTo = 0
		dv.rowsWinBufW, dv.rowsWinBufH = bufW, bufH
		dv.rowsWinRowWidth = rowWidth
		dv.rowsWinLength = n
		dv.rowsWinBaseX = baseX
		dv.rowsWinRowOff = dv.rowOffset
		// Keep the per-row sprite cache coherent on a rebake: sprite consumers must not go stale.
		vis := dv.visibleRows()
		for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
			if dv.needsRowRebuild(i) {
				dv.buildRowSprite(i)
				if i < len(dv.rowDirty) {
					dv.rowDirty[i] = false
				}
				if i < len(dv.rowFullDirty) {
					dv.rowFullDirty[i] = false
				}
			}
		}
		dv.renderRowsWinRange(0, n)
		dv.rowsWinRenderedTo = n
		dv.rowsWinContentDirty = false
		dv.rowsWinValid = true
		dv.rowsLayerGen++ // this is a recomposite — guarded by the perf test.
	}

	drift := dv.Offset - dv.rowsWinBakeOffset
	if needTo := drift + n; needTo > dv.rowsWinRenderedTo {
		if needTo > bufCells {
			needTo = bufCells
		}
		dv.renderRowsWinRange(dv.rowsWinRenderedTo, needTo)
		dv.rowsWinRenderedTo = needTo
	}

	srcX0 := (drift * rowWidth) / n
	srcX1 := srcX0 + rowWidth
	if srcX1 > bufW {
		srcX1 = bufW
	}
	sub := dv.rowsWinBuf.SubImage(image.Rect(srcX0, 0, srcX1, bufH)).(*ebiten.Image)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(dv.Bounds.Min.X+baseX), float64(dv.Bounds.Min.Y))
	dst.DrawImage(sub, &op)
	dv.rowsLayerBytes += int64((srcX1 - srcX0) * bufH * 4)

	// Per frame and window-relative, so they stay on the visible grid instead of scrolling.
	dv.drawWinBeatSeparators(dst, baseX, rowWidth, n, rh)

	vis := dv.visibleRows()
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		if i >= 0 && i < len(dv.rowsDrawnMask) {
			dv.rowsDrawnMask[i] = true
		}
	}
	return true
}

// Buffer cell k is absolute cell rowsWinBakeOffset+k, read from the row window at k-drift.
func (dv *DrumView) renderRowsWinRange(kStart, kEnd int) {
	if kEnd <= kStart || dv.rowsWinBuf == nil {
		return
	}
	buf := dv.rowsWinBuf
	rowWidth := dv.rowsWinRowWidth
	n := dv.rowsWinLength
	if rowWidth <= 0 || n <= 0 {
		return
	}
	drift := dv.Offset - dv.rowsWinBakeOffset
	rowBase := dv.headerH
	rh := dv.rowHeight()
	vis := dv.visibleRows()
	xStart := (kStart * rowWidth) / n
	xEnd := (kEnd * rowWidth) / n
	if xEnd <= xStart {
		xEnd = xStart + 1
	}
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		y := rowBase + (i-dv.rowOffset)*rh
		stripe := genColorDrumStripeEven
		if i%2 != 0 {
			stripe = genColorDrumStripeOdd
		}
		drawRect(buf, image.Rect(xStart, y, xEnd, y+rh), stripe, true)
		r := dv.Rows[i]
		onCol := r.Color
		for k := kStart; k < kEnd; k++ {
			j := k - drift
			if j < 0 || j >= len(r.Steps) {
				continue
			}
			on := r.Steps[j]
			x0 := (k * rowWidth) / n
			x1 := ((k + 1) * rowWidth) / n
			if x1 <= x0 {
				x1 = x0 + 1
			}
			cellType := model.NodeTypeRegular
			if j < len(r.CellTypes) {
				cellType = r.CellTypes[j]
			}
			fillCol := onCol
			if cellType == model.NodeTypeMute {
				fillCol = colMuteCell
			}
			drawDrumCellTyped(buf, image.Rect(x0, y, x1, y+rh), on, fillCol, cellType)
		}
	}
}

func (dv *DrumView) drawWinBeatSeparators(dst *ebiten.Image, baseX, rowWidth, n, rh int) {
	g := dv.timelineUnitsPerBeat
	if g <= 1 || n <= g {
		return
	}
	sepCol := WithAlpha(genColorOnSurface, AlphaFaint)
	rowBase := dv.headerH
	vis := dv.visibleRows()
	for j := g; j < n; j += g {
		x := (j * rowWidth) / n
		if x <= 0 || x >= rowWidth {
			continue
		}
		sx := dv.Bounds.Min.X + baseX + x
		for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
			y := dv.Bounds.Min.Y + rowBase + (i-dv.rowOffset)*rh
			drawRect(dst, image.Rect(sx, y, sx+1, y+rh), sepCol, true)
		}
	}
}
