package ui

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// rowControlsCacheValid checks if the cached row controls image is still valid.
func (dv *DrumView) rowControlsCacheValid() bool {
	if dv.rowControlsCache == nil || dv.rowControlsCacheDirty {
		return false
	}
	vis := dv.visibleRows()
	if dv.rowControlsCacheRowOff != dv.rowOffset || dv.rowControlsCacheVis != vis {
		return false
	}
	// Check if bounds changed (resize)
	if dv.rowControlsCacheRect.Empty() {
		return false
	}
	// Check mute/solo state changes
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		if i < len(dv.rowMuteBtns) && dv.rowMuteBtns[i].pressed != dv.Rows[i].Muted {
			return false
		}
		if i < len(dv.rowSoloBtns) && dv.rowSoloBtns[i].pressed != dv.Rows[i].Solo {
			return false
		}
	}
	return true
}

// markRowControlsDirty invalidates the row controls cache.
func (dv *DrumView) markRowControlsDirty() {
	dv.rowControlsCacheDirty = true
}

// computeRowControlsBounds returns the bounding rectangle for all row controls.
func (dv *DrumView) computeRowControlsBounds() image.Rectangle {
	vis := dv.visibleRows()
	if vis == 0 || len(dv.Rows) == 0 {
		return image.Rectangle{}
	}
	// Compute bounds from the first visible row's buttons
	startRow := dv.rowOffset
	endRow := dv.rowOffset + vis
	if endRow > len(dv.Rows) {
		endRow = len(dv.Rows)
	}
	if startRow >= endRow {
		return image.Rectangle{}
	}
	// Use label and delete button rects to determine bounds
	var minX, minY, maxX, maxY int
	first := true
	for i := startRow; i < endRow; i++ {
		if i < len(dv.rowLabels) {
			r := dv.rowLabels[i].Rect()
			if first {
				minX, minY, maxX, maxY = r.Min.X, r.Min.Y, r.Max.X, r.Max.Y
				first = false
			} else {
				if r.Min.X < minX {
					minX = r.Min.X
				}
				if r.Min.Y < minY {
					minY = r.Min.Y
				}
			}
		}
		if i < len(dv.rowDeleteBtns) {
			r := dv.rowDeleteBtns[i].Rect()
			if r.Max.X > maxX {
				maxX = r.Max.X
			}
			if r.Max.Y > maxY {
				maxY = r.Max.Y
			}
		}
	}
	if first {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

func (dv *DrumView) drawRowControls(dst *ebiten.Image) {
	vis := dv.visibleRows()

	// Check if we can use cached controls
	if dv.rowControlsCacheValid() {
		// Blit cached controls
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(dv.rowControlsCacheRect.Min.X), float64(dv.rowControlsCacheRect.Min.Y))
		dst.DrawImage(dv.rowControlsCache, &op)
		return
	}

	// Compute bounds for the cache
	bounds := dv.computeRowControlsBounds()
	if bounds.Empty() {
		// Fallback: draw directly without caching
		dv.drawRowControlsDirect(dst)
		return
	}

	// Create or resize cache image
	w, h := bounds.Dx(), bounds.Dy()
	if dv.rowControlsCache == nil || dv.rowControlsCache.Bounds().Dx() != w || dv.rowControlsCache.Bounds().Dy() != h {
		dv.rowControlsCache = ebiten.NewImage(w, h)
	} else {
		dv.rowControlsCache.Clear()
	}

	// Draw to cache with offset translation
	dv.drawRowControlsToCache(dv.rowControlsCache, bounds.Min.X, bounds.Min.Y)

	// Update cache metadata
	dv.rowControlsCacheDirty = false
	dv.rowControlsCacheRowOff = dv.rowOffset
	dv.rowControlsCacheVis = vis
	dv.rowControlsCacheRect = bounds

	// Blit to destination
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(bounds.Min.X), float64(bounds.Min.Y))
	dst.DrawImage(dv.rowControlsCache, &op)
}

// drawRowControlsToCache draws row controls to the cache image with an offset.
func (dv *DrumView) drawRowControlsToCache(cache *ebiten.Image, offsetX, offsetY int) {
	vis := dv.visibleRows()
	for i := range dv.Rows {
		if i < dv.rowOffset || i >= dv.rowOffset+vis {
			continue
		}
		if i < len(dv.rowLabels) {
			if dv.renameRow != i {
				dv.drawButtonOffset(cache, dv.rowLabels[i], offsetX, offsetY)
			}
		}
		if i < len(dv.rowEditBtns) {
			dv.drawButtonOffset(cache, dv.rowEditBtns[i], offsetX, offsetY)
		}
		if i < len(dv.rowSaveBtns) {
			dv.drawButtonOffset(cache, dv.rowSaveBtns[i], offsetX, offsetY)
		}
		if i < len(dv.rowColorBtns) {
			dv.drawButtonOffset(cache, dv.rowColorBtns[i], offsetX, offsetY)
		}
		if i < len(dv.rowVolSliders) {
			dv.drawSliderOffset(cache, dv.rowVolSliders[i], offsetX, offsetY)
		}
		if i < len(dv.rowMuteBtns) {
			dv.rowMuteBtns[i].pressed = dv.Rows[i].Muted
			dv.drawButtonOffset(cache, dv.rowMuteBtns[i], offsetX, offsetY)
		}
		if i < len(dv.rowSoloBtns) {
			dv.rowSoloBtns[i].pressed = dv.Rows[i].Solo
			dv.drawButtonOffset(cache, dv.rowSoloBtns[i], offsetX, offsetY)
		}
		if i < len(dv.rowOriginBtns) {
			dv.drawButtonOffset(cache, dv.rowOriginBtns[i], offsetX, offsetY)
		}
		if i < len(dv.rowDeleteBtns) {
			dv.drawButtonOffset(cache, dv.rowDeleteBtns[i], offsetX, offsetY)
		}
	}
}

// drawRowControlsDirect draws row controls directly without caching (fallback).
func (dv *DrumView) drawRowControlsDirect(dst *ebiten.Image) {
	vis := dv.visibleRows()
	for i := range dv.Rows {
		if i < dv.rowOffset || i >= dv.rowOffset+vis {
			continue
		}
		if i < len(dv.rowLabels) {
			if dv.renameRow != i {
				dv.rowLabels[i].Draw(dst)
			}
		}
		if i < len(dv.rowEditBtns) {
			dv.rowEditBtns[i].Draw(dst)
		}
		if i < len(dv.rowSaveBtns) {
			dv.rowSaveBtns[i].Draw(dst)
		}
		if i < len(dv.rowColorBtns) {
			dv.rowColorBtns[i].Draw(dst)
		}
		if i < len(dv.rowVolSliders) {
			dv.rowVolSliders[i].Draw(dst)
		}
		if i < len(dv.rowMuteBtns) {
			dv.rowMuteBtns[i].pressed = dv.Rows[i].Muted
			dv.rowMuteBtns[i].Draw(dst)
		}
		if i < len(dv.rowSoloBtns) {
			dv.rowSoloBtns[i].pressed = dv.Rows[i].Solo
			dv.rowSoloBtns[i].Draw(dst)
		}
		if i < len(dv.rowOriginBtns) {
			dv.rowOriginBtns[i].Draw(dst)
		}
		if i < len(dv.rowDeleteBtns) {
			dv.rowDeleteBtns[i].Draw(dst)
		}
	}
}

// drawButtonOffset draws a button to a cache image with coordinate offset.
func (dv *DrumView) drawButtonOffset(cache *ebiten.Image, btn *Button, offsetX, offsetY int) {
	// Temporarily shift button rect
	origRect := btn.Rect()
	btn.SetRect(origRect.Sub(image.Pt(offsetX, offsetY)))
	btn.Draw(cache)
	btn.SetRect(origRect)
}

// drawSliderOffset draws a slider to a cache image with coordinate offset.
func (dv *DrumView) drawSliderOffset(cache *ebiten.Image, slider *Slider, offsetX, offsetY int) {
	// Temporarily shift slider rect
	origRect := slider.Rect()
	slider.SetRect(origRect.Sub(image.Pt(offsetX, offsetY)))
	slider.Draw(cache)
	slider.SetRect(origRect)
}

func (dv *DrumView) drawEQ(dst *ebiten.Image) {
	if dv.eqRect.Dy() < 8 || dv.eqRect.Dx() < 8 {
		return
	}
	// Background and border.
	drawRect(dst, dv.eqRect, colEQBg, true)

	// Fetch latest analyzer snapshot.
	snap := dv.analyzerSnapshot()
	// Channel selector button
	if dv.eqChannelBtn != nil {
		dv.eqChannelBtn.Draw(dst)
	}
	// Toggle button UI
	if dv.eqToggleBtn != nil {
		dv.eqToggleBtn.Draw(dst)
	}
	if dv.eqWaveformMode {
		dv.drawWaveform(dst, snap)
		drawRect(dst, dv.eqRect, colButtonBorder, false)
		return
	}

	spec := snap.Spectrum
	if len(spec) == 0 {
		drawRect(dst, dv.eqRect, colButtonBorder, false)
		return
	}

	if len(dv.eqBandVals) != len(eqBandDefs) {
		dv.eqBandVals = make([]float64, len(eqBandDefs))
	}
	bandVals := dv.eqBandVals
	blend := 0.5
	specLen := len(spec)
	for i, band := range eqBandDefs {
		nq := 24000.0 // assume 48k sample rate; normalized mapping.
		start := int(math.Floor((band.loHz / nq) * float64(specLen)))
		end := int(math.Ceil((band.hiHz / nq) * float64(specLen)))
		if end <= start {
			end = start + 1
		}
		if start < 0 {
			start = 0
		}
		if end > specLen {
			end = specLen
		}
		maxV := 0.0
		for j := start; j < end; j++ {
			if spec[j] > maxV {
				maxV = spec[j]
			}
		}
		if maxV > 1 {
			maxV = 1
		}
		// Apply gentle expansion so small signals are still visible.
		display := math.Sqrt(maxV)
		bandVals[i] = bandVals[i]*blend + display*(1-blend)
	}
	dv.eqLastBands = make([]float64, len(bandVals))
	copy(dv.eqLastBands, bandVals)

	bandW := dv.eqRect.Dx() / len(eqBandDefs)
	if bandW < 1 {
		bandW = 1
	}
	maxHeight := dv.eqRect.Dy() - 8
	sliderH := 14
	muteBtnH := 14
	muteBtnW := 16
	sliderY := dv.eqRect.Max.Y - sliderH - 2
	muteBtnY := sliderY - muteBtnH - 2
	for i, v := range bandVals {
		if v < 0 {
			continue
		}
		h := int(v * float64(maxHeight))
		if h < 2 && v > 0 {
			h = 2
		}
		x0 := dv.eqRect.Min.X + i*bandW
		x1 := x0 + bandW
		if i == len(eqBandDefs)-1 {
			x1 = dv.eqRect.Max.X
		}
		y0 := dv.eqRect.Max.Y - h
		// Alternating backgrounds to delineate bands.
		bg := fadeColor(colEQBg, 0.2)
		if i%2 == 1 {
			bg = fadeColor(colGridLine, 0.3)
		}
		// Check if this band is muted
		isMuted := i < len(dv.eqBandMuted) && dv.eqBandMuted[i]
		if dv.activeEQChannel() != "main" {
			// Per-instrument EQ: check row's mute state
			for _, r := range dv.Rows {
				if r.Instrument == dv.activeEQChannel() {
					if i < len(r.EQBandMuted) {
						isMuted = r.EQBandMuted[i]
					}
					break
				}
			}
		}
		drawRect(dst, image.Rect(x0, dv.eqRect.Min.Y, x1, dv.eqRect.Max.Y), bg, true)
		// If muted, dim the band visualization
		if isMuted {
			rect := image.Rect(x0, dv.eqRect.Min.Y, x1, dv.eqRect.Max.Y)
			drawRect(dst, rect, fadeColor(colEQBg, 0.5), true)
		} else {
			rect := image.Rect(x0, y0, x1, dv.eqRect.Max.Y)
			col := colEQBar
			if v > 0.8 {
				col = colEQBarPeak
			}
			drawRect(dst, rect, col, true)
			// Thin cap for quick reading at the band peak.
			drawRect(dst, image.Rect(x0, y0-2, x1, y0-1), fadeColor(col, 0.6), true)
		}
		// Separator line between bands.
		if i > 0 {
			drawRect(dst, image.Rect(x0, dv.eqRect.Min.Y, x0+1, dv.eqRect.Max.Y), colGridLine, true)
		}
		// Label near the bottom (Hz range).
		lbl := fmt.Sprintf("%.0f-%.0f", eqBandDefs[i].loHz, eqBandDefs[i].hiHz)
		if spr := TextSprite(lbl); spr != nil {
			lw, lh := spr.Bounds().Dx(), spr.Bounds().Dy()
			cx := x0 + (x1-x0-lw)/2
			if cx < dv.eqRect.Min.X {
				cx = dv.eqRect.Min.X
			}
			ly := dv.eqRect.Max.Y - lh - sliderH - muteBtnH - 8
			var op ebiten.DrawImageOptions
			op.GeoM.Translate(float64(cx), float64(ly))
			dst.DrawImage(spr, &op)
		}
		// Mute button above the slider
		if i < len(dv.eqMuteBtns) && dv.eqMuteBtns[i] != nil {
			btn := dv.eqMuteBtns[i]
			muteBtnX := x0 + (x1-x0-muteBtnW)/2
			btn.SetRect(image.Rect(muteBtnX, muteBtnY, muteBtnX+muteBtnW, muteBtnY+muteBtnH))
			// Update button style based on mute state
			if isMuted {
				btn.Style = EQMuteButtonActiveStyle
			} else {
				btn.Style = EQMuteButtonStyle
			}
			btn.Draw(dst)
		}
		// Slider overlay at the bottom of each band.
		if i < len(dv.eqSliders) && dv.eqSliders[i] != nil {
			s := dv.eqSliders[i]
			s.SetRect(image.Rect(x0+4, sliderY, x1-4, sliderY+sliderH))
			s.Draw(dst)
		}
	}

	// RMS overlay line.
	level := snap.RMS
	if level > 1 {
		level = 1
	}
	if level > 0 {
		y := dv.eqRect.Max.Y - int(level*float64(dv.eqRect.Dy()))
		drawRect(dst, image.Rect(dv.eqRect.Min.X, y, dv.eqRect.Max.X, y+1), colEQBarPeak, true)
	}

	drawRect(dst, dv.eqRect, colButtonBorder, false)
}
