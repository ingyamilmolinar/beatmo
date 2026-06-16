package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Windowed rows-layer scroll cache.
//
// During steady follow-scroll playback the drum-row CONTENT does not change —
// the visible window merely translates as the playhead advances. The legacy
// composite path (drumview_cache_rows_layer.go) recomposited the whole layer (a
// full-layer shift-copy into a scratch buffer + double-buffer swap, which churns
// Ebiten's image-dependency graph via makeStaleIfDependingOn/mapiternext) on
// essentially every recenter (~every 2 cells). On the single cooperatively-
// scheduled WASM thread that per-scroll work steals time from the sequencer
// goroutine and shows up as audio jitter (bench-results/choppy_audio_revisit_
// 2026-06-13.md).
//
// This path renders cells into a buffer WIDER than the visible window
// (Length + rowsWinPadCells cells), fills the leading edge incrementally as the
// playhead scrolls, and blits a moving sub-rectangle each frame. It recomposites
// (rebakes the wide buffer) only when the playhead scrolls past the pad or when
// content/size changes — turning ~every-scroll full-layer copies into one cheap
// sub-rect blit per frame. Regression guard:
// rows_layer_scroll_recompose_test.go.
//
// Scope/safety: engaged only during scrolled follow playback (Offset>0 &&
// FollowPlayback), desktop (non DirectDrawRows), and the full-resolution case
// (Length <= rowWidth). Every other situation falls back to the legacy path, so
// the static/edit rowsLayer tests are unaffected. Gated by the
// rowsLayerWindowingEnabled flag (SetRowsLayerWindowingForTest).

// rowsLayerWindowingEnabled toggles the windowed scroll cache (default on).
var rowsLayerWindowingEnabled = true

// SetRowsLayerWindowingForTest flips the windowed-scroll-cache flag and returns
// a restore func. Test/bench seam only.
func SetRowsLayerWindowingForTest(v bool) func() {
	prev := rowsLayerWindowingEnabled
	rowsLayerWindowingEnabled = v
	return func() { rowsLayerWindowingEnabled = prev }
}

// rowsWinPadCells returns the lookahead pad (in cells) the wide buffer carries
// on the scroll-ahead side. Larger pad → fewer rebakes but a bigger buffer.
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

// drawRowCompositeWindowed renders the row composite via the windowed scroll
// cache. Returns true if it handled the draw; false if the caller should fall
// back to the legacy rowsLayerMaybeRebuild path.
func (dv *DrumView) drawRowCompositeWindowed(dst *ebiten.Image) bool {
	if !rowsLayerWindowingEnabled || Profile().DirectDrawRows {
		return false
	}
	if !dv.FollowPlayback() || dv.Offset <= 0 {
		// Static / non-scrolling: legacy path keeps the many rowsLayer tests
		// and edit flows unchanged.
		return false
	}
	w, h := dv.Bounds.Dx(), dv.Bounds.Dy()
	rowWidth := dv.timelineRect.Dx()
	n := dv.Length
	rh := dv.rowHeight()
	if w <= 0 || h <= 0 || rowWidth <= 0 || n <= 0 || rh <= 0 || n > rowWidth {
		return false // high-res (n>rowWidth) and degenerate cases use legacy.
	}
	baseX := dv.timelineRect.Min.X - dv.Bounds.Min.X
	padCells := rowsWinPadCells(n)
	bufCells := n + padCells
	bufW := (bufCells*rowWidth)/n + 2
	bufH := h

	rebake := !dv.rowsWinValid || dv.rowsWinBuf == nil ||
		dv.rowsWinBufW != bufW || dv.rowsWinBufH != bufH ||
		dv.rowsWinRowWidth != rowWidth || dv.rowsWinLength != n ||
		dv.rowsWinBaseX != baseX || dv.rowsWinContentDirty ||
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
		// Keep the legacy per-row sprite cache coherent on a rebake so any
		// sprite-based consumer (and the live-edit-during-shift guards) still
		// sees rebuilt sprites — the windowed buffer renders from Steps, but the
		// sprite cache must not silently go stale across a content edit.
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

	// Beat-group separators are drawn per-frame (window-relative, matching
	// drawBeatGroupSeparators in the sprite path) so they stay locked to the
	// visible grid rather than scrolling with the cached content.
	dv.drawWinBeatSeparators(dst, baseX, rowWidth, n, rh)

	vis := dv.visibleRows()
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		if i >= 0 && i < len(dv.rowsDrawnMask) {
			dv.rowsDrawnMask[i] = true
		}
	}
	return true
}

// renderRowsWinRange renders buffer cells [kStart,kEnd) for every visible row
// into the wide buffer, matching the sprite path's cell rendering (stripe
// background + drawDrumCell). Buffer cell k holds absolute cell
// (rowsWinBakeOffset+k); its content comes from the current row window at index
// k-drift.
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
			isMute := cellType == model.NodeTypeMute
			if isMute {
				fillCol = colMuteCell
			}
			drawDrumCell(buf, image.Rect(x0, y, x1, y+rh), on, fillCol, isMute)
		}
	}
}

// drawWinBeatSeparators draws the faint per-beat group separators directly to
// the screen, window-relative, matching drawBeatGroupSeparators in the sprite
// path. They are NOT baked into the wide buffer so they stay aligned to the
// visible grid as the cached content scrolls underneath.
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
