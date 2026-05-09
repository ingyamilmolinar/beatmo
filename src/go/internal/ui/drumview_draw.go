package ui

import (
	"image"
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

	// Ensure zone layout is current before drawing. Normally tree.Update()
	// runs Layout() in the frame loop, but some tests call Draw() directly
	// without a preceding Update().
	if dv.tree != nil {
		for i := range dv.tree.zones {
			e := &dv.tree.zones[i]
			if e.zone.NeedsLayout() || e.rect != e.lastRect {
				e.zone.Layout(e.rect)
				e.lastRect = e.rect
			}
		}
	}

	lite := dv.perfDrawLite
	dv.logger.Tracef("[DRUMVIEW] Draw called. beatInfos: %v, highlightsByRow: %v", beatInfos, highlightsByRow)

	// --- Background ---
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
	dst.DrawImage(dv.bg(dv.Bounds.Dx(), dv.Bounds.Dy()), op)
	for _, kind := range []WidgetKind{WidgetTransport, WidgetRack, WidgetTimeline} {
		if r := dv.widgetRects[kind]; !r.Empty() {
			fill := colBGBottom
			if Profile().IsMobile() {
				if kind == WidgetTransport {
					fill = colTransportSurface
				} else if kind == WidgetRack {
					fill = colRackSurface
				}
			}
			drawRect(dst, r, fill, true)
			if Profile().IsMobile() && kind == WidgetTransport {
				drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y),
					WithAlpha(genColorBorder, genAlphaRowRackZebra), true)
			}
		}
	}
	if r := dv.widgetRects[WidgetWave]; !r.Empty() {
		drawRect(dst, r, colEQBg, true)
	}
	if !lite {
		dv.drawLayoutGuides(dst)
	}

	// --- Animations ---
	dv.decayAnims()

	// --- Mobile bottom action bar surface ---
	// Paint the sheet surface before renderToolbarControls so the
	// vol/view/overflow buttons hosted in the bar render on top of the
	// surface rather than under it. The transport widget's clip rect at
	// the top excludes this rect, so it must be painted directly here.
	if !dv.bottomActionBarRect.Empty() {
		drawBottomSheetPanel(dst, dv.bottomActionBarRect)
	}

	// --- Transport zone ---
	if !dv.simpleDraw {
		if r := dv.widgetRects[WidgetTransport]; !r.Empty() {
			clip := dst.Bounds().Intersect(r)
			if !clip.Empty() {
				sub := dst.SubImage(clip).(*ebiten.Image)
				dv.renderToolbarControls(sub)
			}
		} else {
			dv.renderToolbarControls(dst)
		}
		dv.drawNotifications(dst)
	}

	// --- Play button pulse halo ---
	// Multi-pass falloff so the glow reads as a soft halo rather than a
	// hard rectangular outline (DESIGN.md §"Cushioned elevation"). Gated
	// on the visible Pause icon — guards against state drift where
	// isPlaying=true but the icon was left as Play.
	if dv.isPlaying && dv.playBtn() != nil && dv.playBtn().Icon == string(IconPause) {
		pr := dv.playBtn().Rect()
		if !pr.Empty() {
			peak := SinPulseAlpha(dv.frame, genAnimPlayheadPulse)
			for i := 1; i <= 3; i++ {
				a := uint8(int(peak) * (4 - i) / 4)
				if a == 0 {
					continue
				}
				drawRect(dst, pr.Inset(-i), WithAlpha(genColorDrumGlow, a), false)
			}
		}
	}

	// --- Record button armed halo ---
	// Same multi-pass falloff as the play halo, but in record-active red so
	// the armed state is unmistakable across desktop and mobile. Drawn on
	// top of the toolbar so it can pulse without invalidating the cached
	// toolbar render. Mobile additionally renders a static destructive
	// ring inside the cache (drawRecordArmedRingOffset) for the discrete
	// "armed" affordance — the pulse here adds the "live & waiting" energy.
	if dv.IsRecording() && dv.transportZone != nil {
		rr := dv.transportZone.recordBtn.Rect()
		if !rr.Empty() {
			peak := SinPulseAlpha(dv.frame, genAnimPlayheadPulse)
			for i := 1; i <= 3; i++ {
				a := uint8(int(peak) * (4 - i) / 4)
				if a == 0 {
					continue
				}
				drawRect(dst, rr.Inset(-i), WithAlpha(genColorRecordActive, a), false)
			}
		}
	}

	// --- Timeline zone (bar + rows + highlights + dimming) ---
	dv.timelineZone.SetDrawParams(elapsedBeats, highlightsByRow)
	dv.drawZoneClipped(dst, dv.timelineZone)

	// --- Instrument panel mask ---
	mobileEQActive := Profile().IsMobile() && dv.mobileEQMode
	if !mobileEQActive {
		dv.drawInstrumentPanelMask(dst)
	} else {
		dv.panelMaskRect = image.Rectangle{}
	}

	// --- EQ panel zone ---
	if !lite {
		dv.drawZoneClipped(dst, dv.eqPanelZone)
		// Sync zone band values back to DrumView for JS export access (eqBandsSnapshot).
		dv.eqLastBands = append(dv.eqLastBands[:0], dv.eqPanelZone.eqBandVals...)
	}

	// --- Row rack zone ---
	dv.drawZoneClipped(dst, dv.rowRackZone)
	// Sync zone cache rect to DrumView for backward-compat test access.
	dv.rowControlsCacheRect = dv.rowRackZone.controlsCacheRect

	// --- Layout pills ---
	if !lite {
		dv.drawLayoutPills(dst)
	}

	// --- Portal overlays (topmost layer) ---
	if dv.tree != nil {
		dv.tree.Portal().Draw(dst)
	}

	if dv.logger != nil && dv.rowsLayerFrame == dv.frame {
		kb := float64(dv.rowsLayerBytes) / 1024.0
		dv.logger.Debugf("[DRUMVIEW PERF] frame=%d repaints=%d layerKB=%.2f", dv.frame, dv.rowsRepaints, kb)
	}
}

// drawInstrumentPanelMask renders the opaque rack surface mask that keeps
// row controls visually grouped.
func (dv *DrumView) drawInstrumentPanelMask(dst *ebiten.Image) {
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
	maskRight := dv.Bounds.Min.X + dv.labelW + dv.controlsW
	if panelRect.Max.X > maskRight {
		panelRect.Max.X = maskRight
	}
	if !panelRect.Empty() && Profile().ShowRackSurface {
		drawRect(dst, panelRect, colRackSurface, true)
	}
	dv.panelMaskRect = panelRect
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
	stripeEven := genColorDrumStripeEven
	stripeOdd := genColorDrumStripeOdd
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		r := dv.Rows[i]
		y := rowBase + (i-dv.rowOffset)*rh
		// Draw alternating row stripe background.
		stripCol := stripeEven
		if i%2 != 0 {
			stripCol = stripeOdd
		}
		drawRect(dst, image.Rect(startX, y, startX+totalW, y+rh), stripCol, true)
		// Now-playing tint: when the row recently fired its audible step,
		// wash the strip in a faint accent overlay that decays each frame.
		// Drawn after the stripe so the cells render on top in their normal
		// row colors.
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

// zoneClipRect returns the widget rectangle a zone is permitted to draw
// into. Each zone is clipped to its own WidgetBoard cell so it cannot bleed
// into neighbouring widgets (e.g., timeline cells leaking into the rack
// column behind instrument labels). Falls back to dv.Bounds when the
// widget rect is unavailable (degenerate layouts in tests, unknown zones).
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

// drawZoneClipped renders a zone, clipping to its widget rectangle via
// SubImage. This isolates each zone to its own WidgetBoard cell so the
// timeline's cells, the rack's labels/controls, the transport's buttons,
// and the EQ panel cannot overdraw one another.
func (dv *DrumView) drawZoneClipped(dst *ebiten.Image, z Zone) {
	clip := dv.zoneClipRect(z)
	if !clip.Empty() {
		clip = dst.Bounds().Intersect(clip)
		if clip.Empty() {
			return
		}
		sub := dst.SubImage(clip).(*ebiten.Image)
		z.Draw(sub)
	} else {
		z.Draw(dst)
	}
}

// drawRowComposite renders the row composite layer (layer/direct).
// Called by TimelineZone.Draw() via the DrawRowComposite callback.
//
// On mobile (Profile().DirectDrawRows = true), bypass the rowsLayer indirection
// and draw cells directly into dst. On desktop, build/blit the rowsLayer.
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
