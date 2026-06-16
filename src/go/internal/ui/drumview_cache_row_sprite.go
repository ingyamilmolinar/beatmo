package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

const rowCachePatchMax = 12

// drumCellGradientBands is the band count for the backlit-hardware micro
// gradient baked into ON drum cells. The gradient draws into the cached row
// sprite (once per dirty, not per frame), so band fills are served from
// pixelCache after the first paint — alloc-free in steady state.
const drumCellGradientBands = 4

// drawDrumCell renders a single drum cell into the row sprite. ON regular
// cells get a subtle vertical micro-gradient (top slightly lighter → bottom
// slightly darker) in the instrument color for a backlit-hardware ("808")
// look; everything else (OFF cells, mute cells, narrow cells, highlights)
// falls through to the shared DrumCellUI.Draw path unchanged. The gradient is
// derived from the instrument color via adjustColor so it threads the same hue
// the graph cluster uses — chrome stays restrained; color is the signal.
func drawDrumCell(dst *ebiten.Image, rect image.Rectangle, on bool, fillCol color.Color, isMute bool) {
	// Backlit gradient only applies to ON, non-mute, non-narrow cells where
	// the gradient bands are actually visible. Otherwise defer to the shared
	// flat-cell style so muted/off/narrow rendering is byte-identical.
	if on && !isMute && fillCol != nil && rect.Dx() > narrowCellThreshold && rect.Dy() > drumCellGradientBands {
		top := adjustColor(fillCol, 22)  // lighter top edge — "lit from above"
		bot := adjustColor(fillCol, -26) // darker bottom — recessed body
		fillVerticalGradient(dst, rect, top, bot, drumCellGradientBands)
		// 1px border keeps cell separation crisp over the gradient body.
		drawRect(dst, rect, DrumCellUI.Border, false)
		return
	}
	DrumCellUI.Draw(dst, rect, on, false, fillCol)
}

// drawBeatGroupSeparators bakes faint vertical lines at every per-beat group
// boundary into the row sprite so the rack reads as an 808 step row rather
// than a flat CSV heatmap. group is the subdivisions-per-beat; <=1 disables.
func drawBeatGroupSeparators(dst *ebiten.Image, w, h, n, group int) {
	if group <= 1 || n <= group || w <= 0 || h <= 0 {
		return
	}
	sepCol := WithAlpha(genColorOnSurface, AlphaFaint)
	for j := group; j < n; j += group {
		x := (j * w) / n
		if x <= 0 || x >= w {
			continue
		}
		drawRect(dst, image.Rect(x, 0, x+1, h), sepCol, true)
	}
}

// buildRowSprite rebuild kinds returned to callers.
const (
	rowRebuildFull  = 0 // full rebuild or shift (layer must recomposite)
	rowRebuildPatch = 1 // in-place cell patch (layer can overdraw)
)

func (dv *DrumView) buildRowSprite(i int) int {
	if i < 0 || i >= len(dv.Rows) {
		return rowRebuildFull
	}
	w := dv.timelineRect.Dx()
	h := dv.rowHeight()
	if w <= 0 || h <= 0 {
		return rowRebuildFull
	}
	n := len(dv.Rows[i].Steps)
	if n < 1 {
		releaseImage(dv.rowCache[i])
		dv.rowCache[i] = newTrackedImage("rowCache.empty", w, h)
		dv.rowCacheLen = dv.Length
		dv.rowCacheW, dv.rowCacheH = w, h
		dv.rowCacheOff[i] = dv.Offset
		if i < len(dv.rowCacheSig) {
			dv.rowCacheSig[i] = rowRenderSignature(dv.Rows[i].Steps, dv.Rows[i].CellTypes)
		}
		dv.cacheRowSteps(i)
		return rowRebuildFull
	}
	fullRebuild := false
	if i >= 0 && i < len(dv.rowFullDirty) {
		fullRebuild = dv.rowFullDirty[i]
	}
	if dv.offsetChanged {
		fullRebuild = true
	}
	// Attempt incremental reuse on small offset shifts.
	if !fullRebuild && dv.rowCache[i] != nil && dv.rowCacheW == w && dv.rowCacheH == h && dv.rowCacheLen == n {
		// Pixel shift for the new offset relative to the cached one.
		dxPx := int(math.Round(float64(dv.Offset-dv.rowCacheOff[i]) * float64(w) / float64(n)))
		if dxPx != 0 && abs(dxPx) <= max1(dv.rowCachePadPx) {
			// Shift existing content into scratch buffer, then swap.
			if i >= len(dv.rowCacheScratch) {
				scratch := make([]*ebiten.Image, len(dv.Rows))
				copy(scratch, dv.rowCacheScratch)
				dv.rowCacheScratch = scratch
			}
			if dv.rowCacheScratch[i] == nil || dv.rowCacheScratch[i].Bounds().Dx() != w || dv.rowCacheScratch[i].Bounds().Dy() != h {
				releaseImage(dv.rowCacheScratch[i])
				dv.rowCacheScratch[i] = newTrackedImage("rowCacheScratch", w, h)
			} else {
				dv.rowCacheScratch[i].Clear()
			}
			newImg := dv.rowCacheScratch[i]
			var op ebiten.DrawImageOptions
			op.GeoM.Translate(float64(-dxPx), 0)
			newImg.DrawImage(dv.rowCache[i], &op)
			// Redraw newly revealed region at one side.
			start := 0
			end := 0
			if dxPx > 0 {
				start, end = w-dxPx, w
			} else {
				start, end = 0, -dxPx
			}
			// Compute cell range that intersects the newly revealed strip.
			jStart := (start * n) / w
			jEnd := (end*n + w - 1) / w
			if jStart < 0 {
				jStart = 0
			}
			if jEnd > n {
				jEnd = n
			}
			if jEnd < jStart {
				jEnd = jStart
			}
			cellActive := func(idx int) bool { return idx < len(dv.Rows[i].Steps) && dv.Rows[i].Steps[idx] }
			onCol := dv.Rows[i].Color
			for j := jStart; j < jEnd; j++ {
				on := cellActive(j)
				x0 := (j * w) / n
				x1 := ((j + 1) * w) / n
				if x1 <= x0 {
					x1 = x0 + 1
				}
				if x1 <= start || x0 >= end {
					continue
				}
				rect := image.Rect(x0, 0, x1, h)
				cellType := model.NodeTypeRegular
				if j < len(dv.Rows[i].CellTypes) {
					cellType = dv.Rows[i].CellTypes[j]
				}
				fillCol := onCol
				isMute := cellType == model.NodeTypeMute
				if isMute {
					fillCol = colMuteCell
				}
				drawDrumCell(newImg, rect, on, fillCol, isMute)
			}
			dv.rowCache[i], dv.rowCacheScratch[i] = newImg, dv.rowCache[i]
			dv.rowCacheLen = dv.Length
			dv.rowCacheW, dv.rowCacheH = w, h
			dv.rowCacheOff[i] = dv.Offset
			if i < len(dv.rowCacheSig) {
				dv.rowCacheSig[i] = rowRenderSignature(dv.Rows[i].Steps, dv.Rows[i].CellTypes)
			}
			dv.cacheRowSteps(i)
			dv.rowCacheShift++
			// Generation unchanged on incremental update.
			return rowRebuildFull
		}
	}
	// Patch a handful of changed cells when the cache is otherwise valid.
	if !fullRebuild && dv.rowCache[i] != nil && dv.rowCacheW == w && dv.rowCacheH == h && dv.rowCacheLen == n && !dv.offsetChanged {
		if n <= w && i < len(dv.rowCacheSteps) && i < len(dv.rowCacheTypes) {
			prevSteps := dv.rowCacheSteps[i]
			prevTypes := dv.rowCacheTypes[i]
			if len(prevSteps) == n && len(prevTypes) == n {
				diff := 0
				for j := 0; j < n; j++ {
					step := dv.Rows[i].Steps[j]
					typ := model.NodeTypeRegular
					if j < len(dv.Rows[i].CellTypes) {
						typ = dv.Rows[i].CellTypes[j]
					}
					if step != prevSteps[j] || typ != prevTypes[j] {
						diff++
					}
				}
				if diff > 0 {
					onCol := dv.Rows[i].Color
					cellActive := func(idx int) bool { return idx < len(dv.Rows[i].Steps) && dv.Rows[i].Steps[idx] }
					for j := 0; j < n; j++ {
						step := cellActive(j)
						typ := model.NodeTypeRegular
						if j < len(dv.Rows[i].CellTypes) {
							typ = dv.Rows[i].CellTypes[j]
						}
						if step == prevSteps[j] && typ == prevTypes[j] {
							continue
						}
						x0 := (j * w) / n
						x1 := ((j + 1) * w) / n
						if x1 <= x0 {
							x1 = x0 + 1
						}
						rect := image.Rect(x0, 0, x1, h)
						fillCol := onCol
						isMute := typ == model.NodeTypeMute
						if isMute {
							fillCol = colMuteCell
						}
						drawDrumCell(dv.rowCache[i], rect, step, fillCol, isMute)
						// A patched cell starts exactly on its group-boundary
						// separator column; re-draw the line so the 808 grouping
						// survives in-place cell edits.
						if g := dv.timelineUnitsPerBeat; g > 1 && j%g == 0 && j > 0 && x0 > 0 && x0 < w {
							drawRect(dv.rowCache[i], image.Rect(x0, 0, x0+1, h), WithAlpha(genColorOnSurface, AlphaFaint), true)
						}
					}
					dv.rowCacheLen = dv.Length
					dv.rowCacheW, dv.rowCacheH = w, h
					dv.rowCacheOff[i] = dv.Offset
					if i < len(dv.rowCacheSig) {
						dv.rowCacheSig[i] = rowRenderSignature(dv.Rows[i].Steps, dv.Rows[i].CellTypes)
					}
					dv.cacheRowSteps(i)
					dv.rowCachePatch++
					return rowRebuildPatch
				}
			}
		}
	}
	var img *ebiten.Image
	if dv.rowCache[i] != nil && dv.rowCacheW == w && dv.rowCacheH == h {
		img = dv.rowCache[i]
		img.Clear()
	} else {
		releaseImage(dv.rowCache[i])
		img = newTrackedImage("rowSprite", w, h)
	}
	// Alternating row stripe background for subtle visual grouping.
	if i%2 == 0 {
		drawRect(img, image.Rect(0, 0, w, h), genColorDrumStripeEven, true)
	} else {
		drawRect(img, image.Rect(0, 0, w, h), genColorDrumStripeOdd, true)
	}
	if n <= w {
		// Full-resolution cells.
		onCol := dv.Rows[i].Color
		cellActive := func(idx int) bool { return idx < len(dv.Rows[i].Steps) && dv.Rows[i].Steps[idx] }
		for j := 0; j < n; j++ {
			on := cellActive(j)
			x0 := (j * w) / n
			x1 := ((j + 1) * w) / n
			if x1 <= x0 {
				x1 = x0 + 1
			}
			rect := image.Rect(x0, 0, x1, h)
			cellType := model.NodeTypeRegular
			if j < len(dv.Rows[i].CellTypes) {
				cellType = dv.Rows[i].CellTypes[j]
			}
			fillCol := onCol
			isMute := cellType == model.NodeTypeMute
			if isMute {
				fillCol = colMuteCell
			}
			drawDrumCell(img, rect, on, fillCol, isMute)
		}
		// Bake faint per-beat group separators so the rack reads as an 808
		// step row. Drawn after cells so ON-cell gradients don't overpaint
		// the boundary lines (the patch path re-draws them per-cell below).
		drawBeatGroupSeparators(img, w, h, n, dv.timelineUnitsPerBeat)
	}

	// When zoomed out (more steps than pixels), bake decimated marker ticks
	// into the row sprite to avoid per-frame draws.
	if n > w && n > 0 {
		step := int(math.Ceil(float64(n) / float64(w)))
		if step < 1 {
			step = 1
		}
		prevX := -1
		for j := 0; j <= n; j += step {
			x := (j * w) / n
			if x != prevX {
				drawRect(img, image.Rect(x, 0, x+1, h), colTimelineBeat, true)
				prevX = x
			}
		}
	}
	dv.rowCache[i] = img
	dv.rowCacheLen = dv.Length
	dv.rowCacheW, dv.rowCacheH = w, h
	dv.rowCacheOff[i] = dv.Offset
	if i < len(dv.rowCacheGen) {
		dv.rowCacheGen[i]++
	}
	if i < len(dv.rowCacheSig) {
		dv.rowCacheSig[i] = rowRenderSignature(dv.Rows[i].Steps, dv.Rows[i].CellTypes)
	}
	dv.cacheRowSteps(i)
	if i < len(dv.rowFrame) {
		dv.rowFrame[i] = dv.frame
	}
	if i < len(dv.rowRepaint) {
		dv.rowRepaint[i]++
	}
	dv.rowCacheFull++
	return rowRebuildFull
}

// rowHasContent samples the composed rows image (stripes or single layer)
// at several x positions on the centerline of the given visible row.
// Returns true if any sampled pixel has non-zero alpha.
func (dv *DrumView) rowHasContent(row int) bool {
	if row < dv.rowOffset || row >= dv.rowOffset+dv.visibleRows() {
		return false
	}
	if dv.timelineRect.Dx() <= 0 || dv.rowHeight() <= 0 {
		return false
	}
	// Sample along the row center.
	y := dv.Bounds.Min.Y + dv.headerH + (row-dv.rowOffset)*dv.rowHeight() + dv.rowHeight()/2
	if y < dv.Bounds.Min.Y || y >= dv.Bounds.Max.Y {
		return false
	}
	// Choose up to 16 sample points across timeline rect.
	samples := 16
	if dv.timelineRect.Dx() < samples {
		samples = dv.timelineRect.Dx()
	}
	if samples < 1 {
		samples = 1
	}
	// Helper to read alpha at absolute screen (x,y) from the rows layer.
	readAlpha := func(x int, y int) uint32 {
		if dv.rowsLayer != nil {
			lx := x - dv.Bounds.Min.X
			ly := y - dv.Bounds.Min.Y
			if lx >= 0 && lx < dv.rowsLayerW && ly >= 0 && ly < dv.rowsLayerH {
				_, _, _, a := dv.rowsLayer.At(lx, ly).RGBA()
				return a
			}
		}
		return 0
	}
	for i := 0; i < samples; i++ {
		// Distribute samples evenly across [MinX, MaxX)
		x := dv.timelineRect.Min.X + (i*dv.timelineRect.Dx())/samples
		if readAlpha(x, y) > 0 {
			return true
		}
	}
	return false
}
