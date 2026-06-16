package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Draw renders the drum pane via the DrumViewTree. Every pixel emitted
// inside dv.Bounds originates from a Zone or Layer registered with the
// tree (see drumview_tree.go and layer.go); this function itself only
// performs per-frame bookkeeping (counter resets, mask resync, lazy
// layout, animation decay, sync of test-observable state) and then
// hands dst to dv.tree.Draw().
//
// The render-pipeline discipline test (render_pipeline_discipline_test.go)
// forbids draw primitives in this file — adding any drawRect, DrawImage,
// or similar call here will fail CI.
func (dv *DrumView) Draw(dst *ebiten.Image, highlightsByRow [][]highlightEntry, frame int64, beatInfos []model.BeatInfo, elapsedBeats float64) {
	dv.frame++
	// Publish the frame counter for button cushion animations (toggle-pulse
	// glow) so widgets don't need a frame argument threaded through Draw.
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

	// Lazy layout. Normally tree.Update() runs Layout() in the frame
	// loop, but some tests call Draw() directly without a preceding
	// Update(), and a scroll flushed after the Update-phase pass (wheel
	// adapter, momentum, legacy touch/step-drag) leaves needLayout set
	// here. EnsureLayouts re-runs the tree's layout+publish pass so the
	// HitIndex always follows what this frame renders — a bare Layout
	// loop here used to consume needLayout WITHOUT republishing, leaving
	// input permanently dispatched to pre-scroll row positions.
	if dv.rootTree != nil {
		dv.rootTree.EnsureLayouts()
	}

	dv.logger.Tracef("[DRUMVIEW] Draw called. beatInfos: %v, highlightsByRow: %v", beatInfos, highlightsByRow)

	// Animations — state-only, not drawing.
	dv.decayAnims()

	// Refresh per-row widget rects (vol slider, mute/solo/fx button rects)
	// before zones consume them. Previously called from
	// renderToolbarControls; pulled up here so the per-frame work happens
	// regardless of which zone draws first.
	dv.updateRowRects()

	// Remember the live readout position so recalcButtons can size the
	// beat-counter slot to the rendered text width (notif hugs the counter).
	dv.lastElapsedBeats = elapsedBeats

	// Hand timeline draw parameters to the zone before dispatching the
	// tree's draw walk; TimelineZone reads these in its Draw method.
	if dv.timelineZone != nil {
		dv.timelineZone.SetDrawParams(elapsedBeats, highlightsByRow)
	}

	// Single dispatch — every pixel in dv.Bounds originates here.
	// Mobile audio mode hides the rack mask layer at the tree level
	// (see layer_rack_mask.go Visible()), so the mask never paints into
	// dv.panelMaskRect — the rect naturally stays at whatever the layer
	// last wrote (or empty) without a post-Draw fixup here.
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
		dv.logger.Debugf("[DRUMVIEW PERF] frame=%d repaints=%d layerKB=%.2f", dv.frame, dv.rowsRepaints, kb)
	}
}

// drawRowsDirect draws visible row cells directly to dst without intermediate
// textures. Bypasses the multi-level compositing chain (rowCache → rowsLayer
// → dst) that fails on mobile WebGL. Called by drawRowComposite via the
// TimelineZone DrawRowComposite callback — not from DrumView.Draw.
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
				DrumCellUI.Draw(dst, rect, on, false, fillCol)
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

// zoneClipRect returns the widget rectangle a zone is permitted to draw
// into. Each zone is clipped to its own WidgetBoard cell so it cannot bleed
// into neighbouring widgets. Falls back to dv.Bounds when the widget rect
// is unavailable. Used by drumview_zone_clip_test.go.
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

// drawRowComposite renders the row composite layer (layer/direct).
// Called by TimelineZone.Draw() via the DrawRowComposite callback.
//
// On mobile (Profile().DirectDrawRows = true), bypass the rowsLayer
// indirection and draw cells directly into dst. On desktop, build/blit
// the rowsLayer.
func (dv *DrumView) drawRowComposite(dst *ebiten.Image) {
	dv.ensureRowCache()
	if Profile().DirectDrawRows {
		dv.drawRowsDirect(dst)
		vis := dv.visibleRows()
		for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
			if i >= 0 && i < len(dv.rowsDrawnMask) {
				dv.rowsDrawnMask[i] = true
			}
		}
		return
	}
	// Windowed scroll cache: during scrolled follow playback this serves the
	// composite from a wider buffer via a moving sub-rect blit, avoiding the
	// per-scroll full-layer recomposite (drumview_cache_rows_window.go).
	if dv.drawRowCompositeWindowed(dst) {
		return
	}
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
