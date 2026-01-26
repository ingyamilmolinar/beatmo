package ui

import (
	"image"
	"math"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
)

// rowsLayerMaybeRebuild composes all visible row sprites into a cached layer
// image. It excludes dynamic overlays (highlights) and UI controls.
func (dv *DrumView) rowsLayerMaybeRebuild() {
	w := dv.Bounds.Dx()
	h := dv.Bounds.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	rowBase := dv.headerH
	baseX := dv.timelineRect.Min.X - dv.Bounds.Min.X
	rowWidth := dv.timelineRect.Dx()
	padPx := max1(dv.rowsLayerPadPx)
	canReuse := dv.rowsLayer != nil && dv.rowsLayerW == w && dv.rowsLayerH == h && dv.rowsLayerRowOff == dv.rowOffset && dv.rowsLayerBaseX == baseX
	offsetDelta := dv.Offset - dv.rowsLayerOffset
	dxPx := 0
	if rowWidth > 0 && dv.Length > 0 {
		dxPx = int(math.Round(float64(offsetDelta) * float64(rowWidth) / float64(max1(dv.Length))))
	}
	// Lightweight adaptive pad: when under heavy horizontal pan (dx > pad),
	// transiently widen the pad on WASM to turn full rebuilds into partial
	// shifts + narrow fills. Decay back toward default when movement calms.
	if runtime.GOARCH == "wasm" {
		if canReuse && rowWidth > 0 && dv.Length > 0 {
			if abs(dxPx) > padPx {
				dv.rowsPadFullRebuilds++
				if dv.rowsPadFullRebuilds >= 2 {
					// Double up to a modest ceiling.
					const maxPad = 256
					np := dv.rowsLayerPadPx * 2
					if np > maxPad {
						np = maxPad
					}
					if np != dv.rowsLayerPadPx {
						dv.rowsLayerPadPx = np
						padPx = np
					}
					dv.rowsPadFullRebuilds = 0
					dv.rowsPadLastDecay = dv.frame
				}
			} else {
				// Decay by small steps when relatively still.
				const calmFrames = 45
				if dv.rowsLayerPadPx > defaultRowsLayerPadPx && dv.frame-dv.rowsPadLastDecay > calmFrames {
					dv.rowsLayerPadPx -= 16
					if dv.rowsLayerPadPx < defaultRowsLayerPadPx {
						dv.rowsLayerPadPx = defaultRowsLayerPadPx
					}
					padPx = dv.rowsLayerPadPx
					dv.rowsPadLastDecay = dv.frame
				}
			}
		}
	}

	// Invalidate on size changes, scroll changes, base alignment, or explicit dirty flag.
	needFull := dv.rowsLayer == nil || dv.rowsLayerW != w || dv.rowsLayerH != h || dv.rowsLayerRowOff != dv.rowOffset || dv.rowsLayerDirty || dv.rowsLayerBaseX != baseX
	smallShift := canReuse && dxPx != 0 && abs(dxPx) <= padPx && rowWidth > 0 && dv.Length > 0
	// If any row needs rebuild, ensure we rebuild row sprites first and mark layer dirty.
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
			needFull = true
		}
	}
	if !needFull && !smallShift {
		if offsetDelta != 0 {
			dv.rowsLayerOffset = dv.Offset
			dv.rowsLayerFrame = dv.frame
		}
		return
	}
	if smallShift {
		img := ebiten.NewImage(w, h)
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(-dxPx), 0)
		img.DrawImage(dv.rowsLayer, &op)
		fillStart := baseX
		fillEnd := baseX + rowWidth
		if dxPx > 0 {
			fillStart = fillEnd - dxPx
		} else {
			fillEnd = fillStart - dxPx
		}
		if fillStart < baseX {
			fillStart = baseX
		}
		if fillEnd > baseX+rowWidth {
			fillEnd = baseX + rowWidth
		}
		if fillEnd > fillStart {
			subX0 := fillStart - baseX
			subX1 := fillEnd - baseX
			for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
				if i < 0 || i >= len(dv.rowCache) || dv.rowCache[i] == nil {
					continue
				}
				y := rowBase + (i-dv.rowOffset)*dv.rowHeight()
				width := dv.rowCacheW
				if width == 0 {
					width = rowWidth
				}
				localStart := subX0
				localEnd := subX1
				if localStart < 0 {
					localStart = 0
				}
				if localEnd > width {
					localEnd = width
				}
				if localEnd <= localStart {
					continue
				}
				sub := dv.rowCache[i].SubImage(image.Rect(localStart, 0, localEnd, dv.rowHeight())).(*ebiten.Image)
				var subOp ebiten.DrawImageOptions
				subOp.GeoM.Translate(float64(baseX+localStart), float64(y))
				img.DrawImage(sub, &subOp)
				dv.rowsLayerBytes += int64((localEnd - localStart) * dv.rowHeight() * 4)
				dv.rowsRepaints++
				steps := len(dv.Rows[i].Steps)
				if steps > rowWidth && steps > 0 {
					step := int(math.Ceil(float64(steps) / float64(rowWidth)))
					if step < 1 {
						step = 1
					}
					prevX := -1
					rowH := dv.rowHeight()
					for j := 0; j <= steps; j += step {
						x := baseX + (j*rowWidth)/steps
						if x < fillStart || x >= fillEnd {
							continue
						}
						if x != prevX {
							drawRect(img, image.Rect(x, y, x+1, y+rowH), colTimelineBeat, true)
							prevX = x
						}
					}
				}
			}
		}
		dv.rowsLayer = img
		dv.rowsLayerW, dv.rowsLayerH = w, h
		dv.rowsLayerOffset = dv.Offset
		dv.rowsLayerRowOff = dv.rowOffset
		dv.rowsLayerBaseX = baseX
		dv.rowsLayerGen++
		dv.rowsLayerDirty = false
		dv.rowsLayerFrame = dv.frame
		return
	}
	img := ebiten.NewImage(w, h)
	// Draw each visible row sprite at its position inside dv.Bounds.
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		if i < 0 || i >= len(dv.rowCache) || dv.rowCache[i] == nil {
			continue
		}
		y := rowBase + (i-dv.rowOffset)*dv.rowHeight()
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(baseX), float64(y))
		img.DrawImage(dv.rowCache[i], &op)
		steps := len(dv.Rows[i].Steps)
		dv.rowsRepaints++
		dv.rowsLayerBytes += int64(rowWidth * dv.rowHeight() * 4)
		if steps > rowWidth && steps > 0 {
			step := int(math.Ceil(float64(steps) / float64(rowWidth)))
			if step < 1 {
				step = 1
			}
			prevX := -1
			rowH := dv.rowHeight()
			for j := 0; j <= steps; j += step {
				x := baseX + (j*rowWidth)/steps
				if x != prevX {
					drawRect(img, image.Rect(x, y, x+1, y+rowH), colTimelineBeat, true)
					prevX = x
				}
			}
		}
	}
	dv.rowsLayer = img
	dv.rowsLayerW, dv.rowsLayerH = w, h
	dv.rowsLayerOffset = dv.Offset
	dv.rowsLayerRowOff = dv.rowOffset
	dv.rowsLayerBaseX = baseX
	dv.rowsLayerGen++
	dv.rowsLayerDirty = false
	dv.rowsLayerFrame = dv.frame
}
