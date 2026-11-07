package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Dispatch only: every pixel comes from a tree Zone or Layer, and a discipline test forbids
// draw primitives in this file.
func (dv *DrumView) Draw(dst *ebiten.Image, highlightsByRow [][]highlightEntry, frame int64, beatInfos []model.BeatInfo, elapsedBeats float64) {
	dv.frame++
	uiAnimFrame = dv.frame
	dv.rowsLayerBytes = 0
	dv.rowsRepaints = 0
	dv.rowCacheShift = 0
	dv.rowCachePatch = 0
	dv.rowCacheFull = 0
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

	// Draw can run without Update (tests) or after a late scroll; EnsureLayouts also republishes the
	// HitIndex, so input always matches what this frame renders.
	if dv.rootTree != nil {
		dv.rootTree.EnsureLayouts()
	}

	dv.logger.Tracef("[drumview] Draw called. beatInfos: %v, highlightsByRow: %v", beatInfos, highlightsByRow)

	// Animations — state-only, not drawing.
	dv.decayAnims()

	// Before any zone reads them, whichever draws first.
	dv.updateRowRects()

	dv.lastElapsedBeats = elapsedBeats

	if dv.timelineZone != nil {
		dv.timelineZone.SetDrawParams(elapsedBeats, highlightsByRow)
	}

	if dv.rootTree != nil {
		dv.rootTree.SetBounds(dv.Bounds)
		dv.rootTree.Draw(dst)
	}

	// Post-draw test/JS-export sync.
	if dv.eqPanelZone != nil {
		dv.eqLastBands = append(dv.eqLastBands[:0], dv.eqPanelZone.eqBandVals...)
	}
	if dv.rowRackZone != nil {
		dv.rowControlsCacheRect = dv.rowRackZone.controlsCacheRect
	}

	if dv.logger != nil && dv.rowsLayerFrame == dv.frame {
		kb := float64(dv.rowsLayerBytes) / 1024.0
		dv.logger.Debugf("[drumview perf] frame=%d repaints=%d layerKB=%.2f", dv.frame, dv.rowsRepaints, kb)
	}
}

// Bypasses the rowCache → rowsLayer compositing chain, which fails on mobile WebGL.
func (dv *DrumView) drawRowsDirect(dst *ebiten.Image) {
	vis := dv.visibleRows()
	rowBase := dv.Bounds.Min.Y + dv.headerH
	startX := dv.cellGridRect.Min.X
	totalW := dv.cellGridRect.Dx()
	rh := dv.rowHeight()
	if totalW <= 0 || rh <= 0 {
		return
	}
	cells := 0
	stripeEven := genColorDrumStripeEven
	stripeOdd := genColorDrumStripeOdd
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		r := dv.Rows[i]
		y := rowBase + (i-dv.rowOffset)*rh
		stripCol := stripeEven
		if i%2 != 0 {
			stripCol = stripeOdd
		}
		drawRect(dst, image.Rect(startX, y, startX+totalW, y+rh), stripCol, true)
		if intensity := dv.RowFireIntensity(i); intensity > 0 {
			alpha := uint8(float64(genAlphaFaint) * intensity)
			if alpha > 0 {
				drawRect(dst, image.Rect(startX, y, startX+totalW, y+rh), WithAlpha(TokenAccent(), alpha), true)
			}
		}
		n := len(r.Steps)
		if n < 1 {
			continue
		}
		if n <= totalW {
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
				drawDrumCellTyped(dst, rect, on, fillCol, cellType)
				cells++
			}
		} else {
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

// Each zone is clipped to its own WidgetBoard cell so it cannot bleed into a neighbour.
func (dv *DrumView) zoneClipRect(z Zone) image.Rectangle {
	if z == nil {
		return dv.Bounds
	}
	var kind WidgetKind
	switch z.ID() {
	case "transport":
		kind = WidgetTransport
	case "row-rack":
		kind = WidgetRack
	case "timeline":
		kind = WidgetTimeline
	case "eq-panel":
		kind = WidgetWave
	default:
		return dv.Bounds
	}
	if r, ok := dv.widgetRects[kind]; ok && !r.Empty() {
		return r
	}
	return dv.Bounds
}

func (dv *DrumView) drawRowComposite(dst *ebiten.Image) {
	dv.ensureRowCache()
	if Profile().DirectDrawRows {
		dv.lastRowsRenderPath = "direct"
		dv.drawRowsDirect(dst)
		vis := dv.visibleRows()
		for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
			if i >= 0 && i < len(dv.rowsDrawnMask) {
				dv.rowsDrawnMask[i] = true
			}
		}
		return
	}
	if dv.drawRowCompositeWindowed(dst) {
		dv.lastRowsRenderPath = "windowed"
		return
	}
	dv.lastRowsRenderPath = "legacy"
	dv.rowsLayerMaybeRebuild()
	if dv.rowsLayer != nil {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
		dst.DrawImage(dv.rowsLayer, &op)
		vis := dv.visibleRows()
		for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
			if i >= 0 && i < len(dv.rowsDrawnMask) {
				dv.rowsDrawnMask[i] = true
			}
		}
	}
}
