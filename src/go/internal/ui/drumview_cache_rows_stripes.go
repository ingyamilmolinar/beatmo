package ui

import (
	"image"
	"image/color"
	"math"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
)

// rowsStripesMaybeRebuild composes the rows layer as a set of vertical stripes.
// Returns true if stripes were built and are drawable this frame; otherwise false.
func (dv *DrumView) rowsStripesMaybeRebuild() bool {
	if !dv.rowsStripingEnabled {
		return false
	}
	if runtime.GOARCH != "wasm" {
		return false
	}
	w := dv.Bounds.Dx()
	h := dv.Bounds.Dy()
	if w <= 0 || h <= 0 {
		return false
	}

	// Frame-level skip: if nothing changed since last frame, skip entire rebuild.
	n := dv.rowsStripeCount
	if !dv.rowsLayerDirty && dv.rowsStripeLastFrame == dv.frame-1 &&
		dv.rowsStripeOffset == dv.Offset && dv.rowsStripeRowOff == dv.rowOffset &&
		n > 0 && len(dv.rowsStripes) == n && len(dv.rowsStripeCachedW) == n {
		// Verify cached dimensions are valid.
		ready := true
		for si := 0; si < n; si++ {
			if dv.rowsStripes[si] == nil || dv.rowsStripeCachedW[si] != dv.rowsStripeWidths[si] || dv.rowsStripeCachedH[si] != h {
				ready = false
				break
			}
		}
		if ready {
			dv.rowsStripeLastFrame = dv.frame
			return true
		}
	}

	rowBase := dv.headerH
	baseX := dv.timelineRect.Min.X - dv.Bounds.Min.X
	rowWidth := dv.timelineRect.Dx()
	if rowWidth <= 0 || dv.Length <= 0 {
		return false
	}
	// On narrow timelines, stripes add overhead with no benefit.
	// Fall through to the single-layer path which is simpler.
	if rowWidth < wasmStripeTargetPx {
		return false
	}

	targetCount := dv.rowsStripeCount
	if dv.rowsStripeAuto || targetCount < 2 {
		targetCount = (rowWidth + wasmStripeTargetPx - 1) / wasmStripeTargetPx
		if targetCount < 2 {
			targetCount = 2
		}
		if targetCount > wasmStripeMaxCount {
			targetCount = wasmStripeMaxCount
		}
	}
	if targetCount > wasmStripeMaxCount {
		targetCount = wasmStripeMaxCount
	}
	if targetCount < 2 {
		return false
	}
	if targetCount != dv.rowsStripeCount {
		dv.rowsStripeCount = targetCount
		dv.rowsStripes = nil
		dv.rowsStripeStarts = nil
		dv.rowsStripeWidths = nil
		dv.rowsStripeCachedW = nil
		dv.rowsStripeCachedH = nil
		dv.rowsStripeGen = 0
	}

	dv.ensureRowCache()
	vis := dv.visibleRows()
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		if i < 0 {
			continue
		}
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

	n = dv.rowsStripeCount
	if len(dv.rowsStripes) != n {
		dv.rowsStripes = make([]*ebiten.Image, n)
		dv.rowsStripeStarts = make([]int, n)
		dv.rowsStripeWidths = make([]int, n)
		dv.rowsStripeCachedW = make([]int, n)
		dv.rowsStripeCachedH = make([]int, n)
	}
	if len(dv.rowsStripeScratch) != n {
		dv.rowsStripeScratch = make([]*ebiten.Image, n)
	}

	base := rowWidth / n
	rem := rowWidth % n
	start := 0
	for i := 0; i < n; i++ {
		width := base
		if i < rem {
			width++
		}
		dv.rowsStripeStarts[i] = start
		dv.rowsStripeWidths[i] = width
		start += width
	}

	// Global shift in pixels for current offset.
	offsetDelta := dv.Offset - dv.rowsStripeOffset
	dxPx := 0
	if rowWidth > 0 {
		dxPx = int(math.Round(float64(offsetDelta) * float64(rowWidth) / float64(max1(dv.Length))))
	}

	// Determine if we can reuse stripes (same row offset); size/local starts are recomputed.
	canReuse := dv.rowsStripeRowOff == dv.rowOffset
	padPx := max1(dv.rowsLayerPadPx)
	if len(dv.rowsStripeWidths) > 0 {
		widest := dv.rowsStripeWidths[0]
		for _, w := range dv.rowsStripeWidths {
			if w > widest {
				widest = w
			}
		}
		if widest > padPx {
			padPx = widest
		}
	}

	// Fast path: nothing moved and stripes are already sized.
	// Use cached dimensions to avoid expensive img.Size() GPU queries.
	if !dv.rowsLayerDirty && canReuse && dv.rowsStripeOffset == dv.Offset && dxPx == 0 {
		ready := len(dv.rowsStripeCachedW) == n && len(dv.rowsStripeCachedH) == n
		for si := 0; si < n && ready; si++ {
			if dv.rowsStripes[si] == nil ||
				dv.rowsStripeCachedW[si] != dv.rowsStripeWidths[si] ||
				dv.rowsStripeCachedH[si] != h {
				ready = false
			}
		}
		if ready {
			dv.rowsStripeAutoLarge = 0
			dv.rowsStripeAutoCalm = 0
			dv.rowsStripeLastFrame = dv.frame
			return true
		}
	}

	builtAny := false
	hasContent := false
	// ensureImage checks cached dimensions first to avoid GPU queries, then creates
	// a new image if needed. The stripeIdx parameter is used to update the cache.
	ensureImage := func(img *ebiten.Image, stripeIdx, w, h int) *ebiten.Image {
		if img != nil {
			// Check cached dimensions first to avoid img.Size() GPU query
			if stripeIdx >= 0 && stripeIdx < len(dv.rowsStripeCachedW) &&
				dv.rowsStripeCachedW[stripeIdx] == w && dv.rowsStripeCachedH[stripeIdx] == h {
				return img
			}
			// Cache miss or invalid: fall back to actual size check (rare)
			bounds := img.Bounds()
			if bounds.Dx() != w || bounds.Dy() != h {
				img = nil
			}
		}
		if img == nil {
			img = ebiten.NewImage(w, h)
		}
		// Update cached dimensions
		if stripeIdx >= 0 && stripeIdx < len(dv.rowsStripeCachedW) {
			dv.rowsStripeCachedW[stripeIdx] = w
			dv.rowsStripeCachedH[stripeIdx] = h
		}
		return img
	}
	for si := 0; si < n; si++ {
		sw := dv.rowsStripeWidths[si]
		if sw <= 0 {
			continue
		}
		prev := dv.rowsStripes[si]
		scratch := dv.rowsStripeScratch[si]
		needFull := dv.rowsLayerDirty || dv.rowsStripeRowOff != dv.rowOffset || dv.rowsStripeOffset != dv.Offset || prev == nil || dv.rowsStripeStarts[si] < 0
		smallShift := !dv.rowsLayerDirty && canReuse && dxPx != 0 && abs(dxPx) <= padPx
		if needFull {
			smallShift = false
		}

		var out *ebiten.Image

		if smallShift {
			out = ensureImage(scratch, -1, sw, h) // -1 for scratch buffer (no caching)
			out.Fill(color.RGBA{})
			if prev != nil {
				var op ebiten.DrawImageOptions
				op.GeoM.Translate(float64(-dxPx), 0)
				out.DrawImage(prev, &op)
				hasContent = true
			}
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
				stripeX0 := dv.rowsStripeStarts[si]
				localStart := fillStart - (baseX + stripeX0)
				localEnd := fillEnd - (baseX + stripeX0)
				if localStart < 0 {
					localStart = 0
				}
				if localEnd > sw {
					localEnd = sw
				}
				if localEnd > localStart {
					for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.rowCache); i++ {
						if i < 0 || dv.rowCache[i] == nil {
							continue
						}
						y := rowBase + (i-dv.rowOffset)*dv.rowHeight()
						globalStart := stripeX0 + localStart
						globalEnd := stripeX0 + localEnd
						sub := dv.rowCache[i].SubImage(image.Rect(globalStart, 0, globalEnd, dv.rowHeight())).(*ebiten.Image)
						var subOp ebiten.DrawImageOptions
						subOp.GeoM.Translate(float64(localStart), float64(y))
						out.DrawImage(sub, &subOp)
						hasContent = true
						steps := len(dv.Rows[i].Steps)
						if steps > rowWidth && steps > 0 {
							// Use coarser granularity when very zoomed out to reduce draw calls
							var step int
							if steps > rowWidth*2 {
								step = int(math.Ceil(float64(steps) / float64(rowWidth/4)))
							} else {
								step = int(math.Ceil(float64(steps) / float64(rowWidth)))
							}
							if step < 1 {
								step = 1
							}
							prevX := -1
							rowH := dv.rowHeight()
							for j := 0; j <= steps; j += step {
								x := (j*rowWidth)/steps - stripeX0
								if x < localStart || x >= localEnd {
									continue
								}
								if x != prevX {
									drawRect(out, image.Rect(x, y, x+1, y+rowH), colTimelineBeat, true)
									prevX = x
									hasContent = true
								}
							}
						}
					}
				}
			}
			dv.rowsStripeScratch[si] = prev
		} else {
			out = ensureImage(prev, si, sw, h)
			out.Fill(color.RGBA{})
			stripeX0 := dv.rowsStripeStarts[si]
			for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.rowCache); i++ {
				if i < 0 || dv.rowCache[i] == nil {
					continue
				}
				y := rowBase + (i-dv.rowOffset)*dv.rowHeight()
				sub := dv.rowCache[i].SubImage(image.Rect(stripeX0, 0, stripeX0+sw, dv.rowHeight())).(*ebiten.Image)
				var op ebiten.DrawImageOptions
				op.GeoM.Translate(0, float64(y))
				out.DrawImage(sub, &op)
				hasContent = true
				steps := len(dv.Rows[i].Steps)
				if steps > rowWidth && steps > 0 {
					// Use coarser granularity when very zoomed out to reduce draw calls
					var step int
					if steps > rowWidth*2 {
						step = int(math.Ceil(float64(steps) / float64(rowWidth/4)))
					} else {
						step = int(math.Ceil(float64(steps) / float64(rowWidth)))
					}
					if step < 1 {
						step = 1
					}
					prevX := -1
					rowH := dv.rowHeight()
					for j := 0; j <= steps; j += step {
						x := (j*rowWidth)/steps - stripeX0
						if x < 0 || x >= sw {
							continue
						}
						if x != prevX {
							drawRect(out, image.Rect(x, y, x+1, y+rowH), colTimelineBeat, true)
							prevX = x
							hasContent = true
						}
					}
				}
			}
			dv.rowsStripeScratch[si] = scratch
		}

		dv.rowsStripes[si] = out
		builtAny = true
	}

	dv.rowsStripeOffset = dv.Offset
	dv.rowsStripeRowOff = dv.rowOffset
	dv.rowsStripeGen++
	dv.rowsLayerDirty = false
	dv.rowsStripeAutoLarge = 0
	dv.rowsStripeAutoCalm = 0
	dv.rowsStripeLastFrame = dv.frame

	// Only report drawable when we actually wrote content.
	return builtAny && hasContent
}
