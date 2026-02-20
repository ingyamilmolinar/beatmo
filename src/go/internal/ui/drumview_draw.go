package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (dv *DrumView) Draw(dst *ebiten.Image, highlightsByRow [][]highlightEntry, frame int64, beatInfos []model.BeatInfo, elapsedBeats float64) {
	dv.frame++
	dv.rowsLayerBytes = 0
	dv.rowsRepaints = 0
	dv.rowCacheShift = 0
	dv.rowCachePatch = 0
	dv.rowCacheFull = 0
	// reset per-frame row-drawn mask
	if len(dv.rowsDrawnMask) != len(dv.Rows) {
		dv.rowsDrawnMask = make([]bool, len(dv.Rows))
	} else {
		for i := range dv.rowsDrawnMask {
			dv.rowsDrawnMask[i] = false
		}
	}
	if len(dv.rowRepaint) != len(dv.Rows) {
		rr := make([]int, len(dv.Rows))
		copy(rr, dv.rowRepaint)
		dv.rowRepaint = rr
	}
	if len(dv.rowFrame) != len(dv.Rows) {
		rf := make([]int64, len(dv.Rows))
		copy(rf, dv.rowFrame)
		dv.rowFrame = rf
	}

	lite := dv.perfDrawLite
	dv.logger.Tracef("[DRUMVIEW] Draw called. beatInfos: %v, highlightsByRow: %v", beatInfos, highlightsByRow)
	// draw background
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
	dst.DrawImage(dv.bg(dv.Bounds.Dx(), dv.Bounds.Dy()), op)
	// Fill widget backgrounds to avoid gaps after resizes.
	for _, kind := range []WidgetKind{WidgetTransport, WidgetRack, WidgetTimeline} {
		if r := dv.widgetRects[kind]; !r.Empty() {
			fill := colBGBottom
			if isSmallScreen() && kind == WidgetTransport {
				fill = colTransportSurface
			}
			drawRect(dst, r, fill, true)
			if isSmallScreen() && kind == WidgetTransport {
				drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y),
					color.NRGBA{255, 255, 255, 10}, true)
			}
		}
	}
	if r := dv.widgetRects[WidgetWave]; !r.Empty() {
		drawRect(dst, r, colEQBg, true)
	}
	// Draw widget guides (divider lines + pills) AFTER widget background fills
	// so pills render on top of opaque backgrounds instead of being covered.
	if !lite {
		dv.drawLayoutGuides(dst)
	}

	dv.decayAnims()
	dv.renderComponents(RenderPhaseToolbar, dst)
	// Pulsing glow around play button during playback
	if dv.isPlaying && dv.playBtn != nil {
		pr := dv.playBtn.Rect()
		if !pr.Empty() {
			pulse := 0.4 + 0.6*math.Abs(math.Sin(float64(dv.frame)*0.05))
			alpha := uint8(pulse * 120)
			glowCol := color.NRGBA{40, 200, 100, alpha}
			drawRect(dst, pr.Inset(-2), glowCol, false)
		}
	}
	// timeline and progress
	// Keep the visible timeline long enough to contain the graph traversal,
	// the current playhead and the visible window. All values expressed in beats.
	if !dv.isPlaying {
		if dv.Graph != nil {
			units := dv.Graph.BeatLength() // may be in subdivision steps under Game
			u := dv.timelineUnitsPerBeat
			if u <= 0 {
				u = 1
			}
			// Round up to cover any partial beat at the end of the step sequence.
			beats := int(math.Ceil(float64(units) / float64(u)))
			if dv.timelineBeats < beats {
				dv.timelineBeats = beats
			}
		}
	}
	units := float64(max1(dv.timelineUnitsPerBeat))
	lengthBeats := float64(dv.Length) / units
	// Ensure timeline is long enough to contain current playhead + visible window
	needBeats1 := int(math.Ceil(float64(elapsedBeats)/units + lengthBeats))
	offsetBeats := float64(dv.Offset) / units
	needBeats2 := int(math.Ceil(offsetBeats + lengthBeats))
	if needBeats1 > dv.timelineBeats {
		dv.timelineBeats = needBeats1
	}
	if needBeats2 > dv.timelineBeats {
		dv.timelineBeats = needBeats2
	}
	totalBeats := dv.timelineBeats
	info := dv.timelineInfoCached(elapsedBeats)
	// Build or reuse timeline base cache (background + beat markers)
	step := 1
	width := dv.timelineRect.Dx()
	if totalBeats > width {
		step = int(math.Ceil(float64(totalBeats) / float64(width)))
	}
	if dv.tlCache == nil || dv.tlCacheW != dv.timelineRect.Dx() || dv.tlCacheH != dv.timelineRect.Dy() || dv.tlCacheBeats != totalBeats || dv.tlCacheStep != step {
		dv.tlCache = ebiten.NewImage(dv.timelineRect.Dx(), dv.timelineRect.Dy())
		dv.tlCacheW, dv.tlCacheH = dv.timelineRect.Dx(), dv.timelineRect.Dy()
		dv.tlCacheBeats, dv.tlCacheStep = totalBeats, step
		// Fill background
		drawRect(dv.tlCache, image.Rect(0, 0, dv.tlCacheW, dv.tlCacheH), colTimelineTotal, true)
		// Beat markers (decimated)
		prevX := -1
		for i := 0; i <= totalBeats; i += step {
			x := int(float64(i) / float64(totalBeats) * float64(dv.tlCacheW))
			if x != prevX {
				drawRect(dv.tlCache, image.Rect(x, 0, x+1, dv.tlCacheH), colTimelineBeat, true)
				prevX = x
			}
		}
	}
	if dv.tlCache != nil {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(dv.timelineRect.Min.X), float64(dv.timelineRect.Min.Y))
		dst.DrawImage(dv.tlCache, &op)
	} else {
		drawRect(dst, dv.timelineRect, colTimelineTotal, true)
	}

	// current view rectangle
	viewStart := dv.timelineRect.Min.X + int((offsetBeats/float64(totalBeats))*float64(dv.timelineRect.Dx()))
	viewWidth := int((lengthBeats / float64(totalBeats)) * float64(dv.timelineRect.Dx()))
	if viewWidth < 1 {
		viewWidth = 1
	}
	if viewWidth < 1 {
		viewWidth = 1
	}
	viewRect := image.Rect(viewStart, dv.timelineRect.Min.Y, viewStart+viewWidth, dv.timelineRect.Max.Y)
	drawRect(dst, viewRect, colTimelineView, true)
	drawRect(dst, viewRect, colTimelineViewHi, false)

	// (beat markers are baked into tlCache)

	// current playback cursor (beats)
	cursorX := dv.timelineRect.Min.X + int((elapsedBeats/float64(totalBeats))*float64(dv.timelineRect.Dx()))
	cursorRect := image.Rect(cursorX-1, dv.timelineRect.Min.Y, cursorX+1, dv.timelineRect.Max.Y)
	drawRect(dst, cursorRect, colTimelineCursor, true)

	drawRect(dst, dv.timelineRect, colButtonBorder, false)

	// Beat/time counter — drawn AFTER the timeline bar so it's not covered.
	// Positioned above the timeline bar on both platforms.
	if !dv.simpleDraw && !lite && !dv.beatCounterRect.Empty() {
		infoY := dv.beatCounterRect.Min.Y + (dv.beatCounterRect.Dy()-debugCharH)/2
		DrawTextAt(dst, info, dv.beatCounterRect.Min.X, infoY)
	}

	// Draw len +/- buttons (floating at top-right of timeline area).
	dv.lenIncBtn.Draw(dst)
	dv.lenDecBtn.Draw(dst)

	// Draw Track button at timeline position on mobile.
	if isSmallScreen() && dv.trackBtn != nil && !dv.trackBtn.Rect().Empty() {
		dv.trackBtn.Draw(dst)
	}

	mobileEQActive := isSmallScreen() && dv.mobileEQMode

	// Build and draw rows composite layer (static rows without highlights)
	if !mobileEQActive {
		dv.ensureRowCache()
		if dv.rowsStripingEnabled {
			// Stripe-based path: rebuild stripes. Fallback to single-layer if not drawable.
			if dv.rowsStripesMaybeRebuild() && len(dv.rowsStripes) > 0 {
				for i := 0; i < len(dv.rowsStripes); i++ {
					img := dv.rowsStripes[i]
					if img == nil {
						continue
					}
					x := dv.timelineRect.Min.X + dv.rowsStripeStarts[i]
					var op ebiten.DrawImageOptions
					op.GeoM.Translate(float64(x), float64(dv.Bounds.Min.Y))
					dst.DrawImage(img, &op)
				}
				// mark all visible rows as drawn when stripes are in use
				vis := dv.visibleRows()
				for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
					if i >= 0 && i < len(dv.rowsDrawnMask) {
						dv.rowsDrawnMask[i] = true
					}
				}
			} else {
				// Stripes bailed (narrow timeline or non-WASM fallback).
				// On mobile, bypass intermediate textures and draw cells directly
				// to dst — the multi-level compositing chain (rowCache → rowsLayer
				// → dst) fails on mobile WebGL, while direct draws work.
				if isSmallScreen() {
					dv.drawRowsDirect(dst)
				} else {
					// Always mark dirty so the layer path rebuilds if needed.
					dv.rowsLayerDirty = true
					dv.rowsLayerMaybeRebuild()
					if dv.rowsLayer != nil {
						var op ebiten.DrawImageOptions
						op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
						dst.DrawImage(dv.rowsLayer, &op)
						// mark all visible rows as drawn for rowsLayer path
						vis := dv.visibleRows()
						for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
							if i >= 0 && i < len(dv.rowsDrawnMask) {
								dv.rowsDrawnMask[i] = true
							}
						}
					}
				}
			}
		} else {
			// Single-layer path (default)
			dv.rowsLayerMaybeRebuild()
			if dv.rowsLayer != nil {
				var op ebiten.DrawImageOptions
				op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
				dst.DrawImage(dv.rowsLayer, &op)
			}
		}

		// Fill the gap below visible rows so it's not black.
		{
			vis := dv.visibleRows()
			rowsTop := dv.Bounds.Min.Y + dv.headerH
			nBelow := len(dv.Rows) - dv.rowOffset
			if nBelow > vis {
				nBelow = vis
			}
			bottomOfRows := rowsTop + nBelow*dv.rowHeight()
			rackBottom := dv.widgetRects[WidgetRack].Max.Y
			if rackBottom == 0 {
				rackBottom = dv.Bounds.Max.Y - dv.eqH
			}
			tlLeft := dv.timelineRect.Min.X
			tlRight := dv.timelineRect.Max.X
			if bottomOfRows < rackBottom && tlRight > tlLeft {
				gapRect := image.Rect(tlLeft, bottomOfRows, tlRight, rackBottom)
				drawRect(dst, gapRect, colStepOff, true)
			}
		}

		// draw steps (rows) – dynamic overlays and controls
		vis := dv.visibleRows()
		rowsTop := dv.Bounds.Min.Y + dv.headerH
		for i, r := range dv.Rows {
			if len(r.Steps) != dv.Length {
				r.Steps = make([]bool, dv.Length)
				r.CellTypes = make([]model.NodeType, dv.Length)
			}
			if i < dv.rowOffset || i >= dv.rowOffset+vis {
				continue
			}
			y := rowsTop + (i-dv.rowOffset)*dv.rowHeight()
			// Row highlights are drawn dynamically below. Overlay highlights for this row only.
			n := len(r.Steps)
			if n > 0 && i < len(highlightsByRow) && len(highlightsByRow[i]) > 0 {
				startX := dv.timelineRect.Min.X
				totalW := dv.timelineRect.Dx()
				for _, hl := range highlightsByRow[i] {
					val := hl.val
					j := hl.idx - dv.Offset
					if j < 0 || j >= n {
						continue
					}
					x0 := startX + (j*totalW)/n
					x1 := startX + ((j+1)*totalW)/n
					if x1 <= x0 {
						x1 = x0 + 1
					}
					if dv.simpleDraw {
						dv.ensureHighlightSprites()
						spr := dv.hlSpriteReg
						if isMuteHighlight(val) {
							spr = dv.hlSpriteMute
						}
						if spr != nil {
							var hop ebiten.DrawImageOptions
							sx := float64(x0)
							sy := float64(y)
							w := float64(x1 - x0)
							hop.GeoM.Scale(w/float64(spr.Bounds().Dx()), 1)
							hop.GeoM.Translate(sx, sy)
							dst.DrawImage(spr, &hop)
						}
					} else {
						rect := image.Rect(x0, y, x1, y+dv.rowHeight())
						if isMuteHighlight(val) {
							drawRect(dst, rect, colMuteHighlight, true)
							drawRect(dst, rect, DrumCellUI.Border, false)
						} else {
							DrumCellUI.Draw(dst, rect, r.Steps[j], true, r.Color)
						}
					}
				}
			}
		}
	}

	// Mute/Solo visual dimming: draw semi-transparent overlay on muted or non-soloed rows
	if !mobileEQActive {
		anySolo := false
		for _, r := range dv.Rows {
			if r.Solo {
				anySolo = true
				break
			}
		}
		if anySolo || func() bool {
			for _, r := range dv.Rows {
				if r.Muted {
					return true
				}
			}
			return false
		}() {
			vis := dv.visibleRows()
			rowsTop := dv.Bounds.Min.Y + dv.headerH
			for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
				shouldDim := dv.Rows[i].Muted || (anySolo && !dv.Rows[i].Solo)
				if shouldDim {
					y := rowsTop + (i-dv.rowOffset)*dv.rowHeight()
					dimRect := image.Rect(dv.timelineRect.Min.X, y, dv.timelineRect.Max.X, y+dv.rowHeight())
					drawRect(dst, dimRect, color.NRGBA{0, 0, 0, 100}, true)
				}
			}
		}
	}

	// Mask instrument panel column using rack widget bounds so controls stay visually grouped.
	if !mobileEQActive {
		panelRect := dv.widgetRects[WidgetRack]
		if panelRect.Empty() {
			panelLeft := dv.Bounds.Min.X
			panelRight := dv.timelineRect.Min.X
			if panelRight > panelLeft {
				top := dv.Bounds.Min.Y + dv.headerH
				bot := dv.Bounds.Max.Y
				panelRect = image.Rect(panelLeft, top, panelRight, bot)
			}
		}
		if runningUnderGoTest() {
			panelRect.Max.Y = dv.Bounds.Max.Y
		}
		// Ensure the panel mask begins immediately below the transport widget header,
		// which may be taller than the legacy timelineHeight constant when the bottom
		// row (Upload/Import/Export buttons) is visible.
		expectedTop := dv.Bounds.Min.Y + dv.headerH
		if panelRect.Min.Y != expectedTop {
			panelRect.Min.Y = expectedTop
			if panelRect.Max.Y < panelRect.Min.Y {
				panelRect.Max.Y = panelRect.Min.Y
			}
			if panelRect.Max.Y > dv.Bounds.Max.Y {
				panelRect.Max.Y = dv.Bounds.Max.Y
			}
		}
		// Tighten mask right edge to actual controls width so the mask
		// doesn't overdraw into the timeline area.
		maskRight := dv.Bounds.Min.X + dv.labelW + dv.controlsW
		if panelRect.Max.X > maskRight {
			panelRect.Max.X = maskRight
		}
		if !panelRect.Empty() && !isSmallScreen() {
			drawRect(dst, panelRect, colRackSurface, true)
		}
		dv.panelMaskRect = panelRect
	} else {
		dv.panelMaskRect = image.Rectangle{}
	}
	if !lite {
		dv.drawEQ(dst)
	}
	dv.renderComponents(RenderPhaseRowControls, dst)
	// Draw pill handles on top of row content and rack mask so they stay visible.
	if !lite {
		dv.drawLayoutPills(dst)
	}
	// Mobile popups: overflow menu and context menu drawn after controls.
	dv.drawOverflowMenu(dst)
	dv.drawContextMenu(dst)
	dv.drawFXPanel(dst)
	dv.drawVolumePopup(dst)
	dv.drawMasterVolumePopup(dst)
	dv.drawEQPopup(dst)
	if dv.logger != nil && dv.rowsLayerFrame == dv.frame {
		kb := float64(dv.rowsLayerBytes) / 1024.0
		dv.logger.Debugf("[DRUMVIEW PERF] frame=%d repaints=%d layerKB=%.2f", dv.frame, dv.rowsRepaints, kb)
	}
}

// drawRowsDirect draws visible row cells directly to dst without intermediate
// textures. This bypasses the multi-level compositing chain (rowCache →
// rowsLayer → dst) that fails on mobile WebGL. Uses the same cell layout math
// as buildRowSprite and draws via DrumCellUI.Draw → drawRect → direct pixel
// writes, which is the same proven path used by row controls and EQ.
func (dv *DrumView) drawRowsDirect(dst *ebiten.Image) {
	vis := dv.visibleRows()
	rowBase := dv.Bounds.Min.Y + dv.headerH
	startX := dv.timelineRect.Min.X
	totalW := dv.timelineRect.Dx()
	rh := dv.rowHeight()
	if totalW <= 0 || rh <= 0 {
		return
	}
	cells := 0
	// Alternating row stripe colors for subtle visual grouping.
	stripeEven := color.RGBA{24, 24, 30, 255}
	stripeOdd := color.RGBA{18, 18, 22, 255}
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		r := dv.Rows[i]
		y := rowBase + (i-dv.rowOffset)*rh
		// Draw alternating row stripe background.
		stripCol := stripeEven
		if i%2 != 0 {
			stripCol = stripeOdd
		}
		drawRect(dst, image.Rect(startX, y, startX+totalW, y+rh), stripCol, true)
		n := len(r.Steps)
		if n < 1 {
			continue
		}
		if n <= totalW {
			// Full-resolution cells.
			for j := 0; j < n; j++ {
				x0 := startX + (j*totalW)/n
				x1 := startX + ((j+1)*totalW)/n
				if x1 <= x0 {
					x1 = x0 + 1
				}
				rect := image.Rect(x0, y, x1, y+rh)
				on := r.Steps[j]
				cellType := model.NodeTypeRegular
				if j < len(r.CellTypes) {
					cellType = r.CellTypes[j]
				}
				fillCol := r.Color
				if cellType == model.NodeTypeMute {
					fillCol = colMuteCell
				}
				DrumCellUI.Draw(dst, rect, on, false, fillCol)
				cells++
			}
		} else {
			// Zoomed out: draw decimated marker ticks only (same as buildRowSprite).
			step := int(math.Ceil(float64(n) / float64(totalW)))
			if step < 1 {
				step = 1
			}
			prevX := -1
			for j := 0; j <= n; j += step {
				x := startX + (j*totalW)/n
				if x != prevX {
					drawRect(dst, image.Rect(x, y, x+1, y+rh), colTimelineBeat, true)
					prevX = x
					cells++
				}
			}
		}
		if i >= 0 && i < len(dv.rowsDrawnMask) {
			dv.rowsDrawnMask[i] = true
		}
	}
	dv.directDrawCount++
	dv.directDrawCells = cells
}
