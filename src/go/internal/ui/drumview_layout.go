package ui

import (
	"image"
	"image/color"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

/* ─── public update ────────────────────────────────────────── */

func (dv *DrumView) refreshLenButtonsStyle() {
	if dv.lenDecBtn == nil || dv.lenIncBtn == nil {
		return
	}
	// Muted glyphs, so the band's keycaps read as one family with the lock and the counter.
	setButtonStyleAndIcon(dv.lenDecBtn, LenDecStyle, colTextSecondary)
	setButtonStyleAndIcon(dv.lenIncBtn, LenIncStyle, colTextSecondary)
}

// Only when changed: storing into the interface fields boxes the value, every layout pass.
func setButtonStyleAndIcon(b *Button, st ButtonStyle, icon color.RGBA) {
	if cur, ok := b.Style.(ButtonStyle); !ok || cur != st {
		b.Style = st
	}
	if cur, ok := b.IconColor.(color.RGBA); !ok || cur != icon {
		b.IconColor = icon
	}
}

func (dv *DrumView) recalcButtons() {
	// First, so everything below sees styles for the current profile.
	dv.refreshLenButtonsStyle()
	// On WASM the mobile profile activates only after Layout calls SetTouchScreenSize.
	p := Profile()
	if p.IsMobile() && !dv.mobileEQInited {
		dv.mobileEQInited = true
		dv.mobileEQCollapsed = true
		dv.bgDirty = true
		if dv.widgets != nil {
			// The board may have been built with desktop weights before the profile was known.
			dv.widgets.SetWeights(p.ColWeights, p.RowWeights)
			dv.widgets.ToggleWidget(WidgetWave, false)
			dv.widgetRects[WidgetWave] = dv.widgets.Rect(WidgetWave)
		}
		dv.refreshWidgetLayout()
		dv.eqH = 0
	}
	if runningUnderGoTest() {
		// Tests collapse the panel; eqPanelHeightForTest lets one exercise the real floored layout.
		if eqPanelHeightForTest > 0 {
			eqPanelHeight = eqPanelHeightForTest
		} else {
			eqPanelHeight = 0
			dv.eqH = 0
		}
	}
	dv.calcLabelWidth()

	// Before the rack is laid out: refreshWidgetLayout only runs on resize, so a tab switch would
	// leave eqH stale and strand the rack under the panel.
	dv.reconcileEQHeightToPanelFloor()

	// Skipped when the pane cannot fit one row above the bar, or the rack would collapse to zero rows;
	// the bar's controls stay reachable from the transport.
	if p.UseBottomSheet {
		barH := TouchMinTarget()
		usable := dv.Bounds.Dy() - dv.headerH - barH
		if usable >= dv.rowHeight() {
			dv.bottomActionBarRect = image.Rect(
				dv.Bounds.Min.X,
				dv.Bounds.Max.Y-barH,
				dv.Bounds.Max.X,
				dv.Bounds.Max.Y,
			)
		} else {
			dv.bottomActionBarRect = image.Rectangle{}
		}
	} else {
		dv.bottomActionBarRect = image.Rectangle{}
	}

	// No EQ peek strip: the bottom nav's EQ tab already expands the EQ.
	dv.eqPeekRect = image.Rectangle{}

	transport := dv.widgetRects[WidgetTransport]
	if transport.Empty() {
		transport = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+dv.headerH)
	}
	widgetColW := 0
	if dv.widgets != nil {
		widgetColW = dv.widgets.ColWidth(0)
	}
	// Cap transport height to headerH so buttons don't overlap row controls.
	if transport.Dy() > dv.headerH {
		transport.Max.Y = transport.Min.Y + dv.headerH
	}
	if p.IsMobile() {
		// Keep transport within its widget column — no overlap into timeline.
	} else if widgetColW > 0 && transport.Dx() < widgetColW {
		transport.Max.X = transport.Min.X + widgetColW
	}
	leftCol := transport

	// Delegate transport layout to TransportZone when available.
	if dv.transportZone != nil && dv.tree != nil {
		controlLeft := leftCol.Min.X + p.ControlLeftInset
		if controlLeft >= leftCol.Max.X {
			controlLeft = leftCol.Min.X
		}
		topBounds := image.Rect(controlLeft, leftCol.Min.Y, leftCol.Max.X, leftCol.Max.Y)
		dv.tree.SetZoneRect("transport", topBounds)
		// When the bar collapses on very short viewports, the zone keeps those buttons in the toolbar.
		dv.transportZone.SetUseBottomBar(p.IsMobile() && !dv.bottomActionBarRect.Empty())
		if dv.transportZone.NeedsLayout() || dv.transportZone.rect != topBounds {
			dv.tree.LayoutZoneNow("transport")
		}
		dv.mainVolIconRect = dv.transportZone.mainVolIconRect
		dv.mainVolRect = dv.transportZone.mainVolRect

		if p.IsMobile() && !dv.bottomActionBarRect.Empty() {
			bar := dv.bottomActionBarRect
			barPad := ActiveTopBarSpec().Padding
			if dv.transportZone.viewSwitchBtn != nil {
				dv.transportZone.viewSwitchBtn.SetRect(image.Rectangle{})
			}
			// Segmented spans the full bar width minus horizontal padding.
			if dv.viewSwitchSegmented != nil {
				segRect := bar
				if barPad > 0 && segRect.Dx() > 2*barPad {
					segRect = image.Rect(segRect.Min.X+barPad, segRect.Min.Y, segRect.Max.X-barPad, segRect.Max.Y)
				}
				dv.viewSwitchSegmented.SetRect(segRect)
				// No instrument context (Master, or a WAV for Synth) greys the segment; HitTest reads this live.
				if idx := bottomNavSynthIndex(); idx >= 0 {
					dv.viewSwitchSegmented.SetSegmentDisabled(idx, dv.synthViewDisabled())
				}
				if idx := segmentIndexForViewMode(viewModeSampler); idx >= 0 {
					dv.viewSwitchSegmented.SetSegmentDisabled(idx, dv.samplerViewDisabled())
				}
			}
			// Again: the transport zone laid out before the control was placed.
			dv.transportZone.rebuildHitAreas()
			dv.transportZone.SetBarRect(bar)
			// ZViewSwitch must sit above all in-panel content, or a tab whose controls reach into the bar
			// (Chain's stage cards) strands the user on it.
			dv.tree.HitIndexRef().Update("transport", dv.transportZone.HitAreas())
			// In the AUDIO subtree: only within the same HitIndex can its z beat the panel's catch-all,
			// which covers the bar too.
			if dv.audioTree != nil {
				if dv.viewSwitchSegmented != nil && !dv.viewSwitchSegmented.Rect().Empty() {
					dv.audioTree.HitIndexRef().Update("view-switch", []HitArea{{
						Rect:     dv.viewSwitchSegmented.Rect(),
						ClipRect: bar,
						ZIndex:   ZViewSwitch,
						Tag:      "transport-view-segmented",
						Handler:  &segmentedHitAdapter{sc: dv.viewSwitchSegmented},
					}})
				} else {
					dv.audioTree.HitIndexRef().Update("view-switch", nil)
				}
			}
		} else if dv.audioTree != nil {
			// Clear it so a mobile→desktop switch cannot strand it.
			dv.audioTree.HitIndexRef().Update("view-switch", nil)
		}
	}

	// Delegate row rack layout to RowRackZone when available.
	if dv.rowRackZone != nil && dv.tree != nil {
		rackRect := dv.widgetRects[WidgetRack]
		if rackRect.Empty() {
			rowsTop := dv.Bounds.Min.Y + dv.headerH
			rackRect = image.Rect(dv.Bounds.Min.X, rowsTop, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.rowsBottom())
		}
		// Clamp rack top to match capped headerH.
		rowsTop := dv.Bounds.Min.Y + dv.headerH
		if rackRect.Min.Y < rowsTop {
			rackRect.Min.Y = rowsTop
		}
		// The board allocates the rack down to the pane bottom; clamped, the add-row button never lands
		// on the bottom bar.
		if rb := dv.rowsBottom(); rb < rackRect.Max.Y {
			rackRect.Max.Y = rb
		}
		// rackVisibleRows, never visibleRows(): the latter can over-count when the board gives space
		// below the rack to the wave widget.
		dv.rowRackZone.SetVisibleRowsOverride(dv.rackVisibleRows())
		dv.tree.SetZoneRect("row-rack", rackRect)
		dv.rowRackZone.SetScreenBounds(dv.Bounds)
		// Adopt a scroll-initiated offset before pushing ours, or it is overwritten.
		if zoneOff := dv.rowRackZone.RowOffset(); zoneOff != dv.rowOffset {
			dv.rowOffset = zoneOff
		}
		dv.rowRackZone.SetRowOffset(dv.rowOffset)
		dv.rowRackZone.SetSelRow(dv.selRow)
		if dv.rowRackZone.NeedsLayout() || dv.rowRackZone.rect != rackRect {
			dv.tree.LayoutZoneNow("row-rack")
		}
		// RowRack fields are now accessed via accessor methods; no alias sync needed.
	}

	// Legacy transport layout — used when TransportZone is nil (fallback).
	if dv.transportZone == nil {
		// Dynamic button sizing — single row transport.
		pad := p.ControlPadding
		// derive padding from available height
		if pad > leftCol.Dy()/4 {
			pad = leftCol.Dy() / 4
		}
		safeInset := func(r image.Rectangle, pad int) image.Rectangle {
			if r.Empty() {
				return r
			}
			minDim := r.Dx()
			if r.Dy() < minDim {
				minDim = r.Dy()
			}
			maxPad := (minDim - 2) / 2
			if maxPad < 0 {
				maxPad = 0
			}
			minW := 48
			minH := debugCharH + 2
			maxPadW := (r.Dx() - minW) / 2
			if maxPadW < 0 {
				maxPadW = 0
			}
			maxPadH := (r.Dy() - minH) / 2
			if maxPadH < 0 {
				maxPadH = 0
			}
			if maxPadW < maxPad {
				maxPad = maxPadW
			}
			if maxPadH < maxPad {
				maxPad = maxPadH
			}
			if pad > maxPad {
				pad = maxPad
			}
			return insetRect(r, pad)
		}
		// Single-row transport: all controls in one row.
		controlLeft := leftCol.Min.X + p.ControlLeftInset
		if controlLeft >= leftCol.Max.X {
			controlLeft = leftCol.Min.X
		}
		topBounds := image.Rect(controlLeft, leftCol.Min.Y, leftCol.Max.X, leftCol.Max.Y)
		// Helper: ensure gap between adjacent buttons.
		ensureGap := func(left, right *Button) {
			if left == nil || right == nil {
				return
			}
			lr, rr := left.Rect(), right.Rect()
			if lr.Max.X >= rr.Min.X {
				dx := lr.Max.X - rr.Min.X + 2
				rr.Min.X += dx
				rr.Max.X += dx
				right.SetRect(rr)
			}
		}
		clampBtn := func(btn *Button, bounds image.Rectangle) {
			r := btn.Rect()
			if r.Min.Y < bounds.Min.Y {
				r.Min.Y = bounds.Min.Y
			}
			if r.Max.Y > bounds.Max.Y {
				r.Max.Y = bounds.Max.Y
			}
			btn.SetRect(r)
		}
		// Helper: stack two buttons vertically in a column rect.
		stackVertical := func(top, bot *Button, col image.Rectangle, bounds image.Rectangle) {
			split := col.Dy() / 2
			if split < 8 {
				split = col.Dy() / 2
			}
			top.SetRect(col)
			tr := top.Rect()
			tr.Max.Y = tr.Min.Y + split
			top.SetRect(tr)
			bot.SetRect(col)
			br := bot.Rect()
			br.Min.Y = tr.Max.Y
			if br.Max.Y > bounds.Max.Y {
				br.Max.Y = bounds.Max.Y
			}
			if tr.Max.Y > bounds.Max.Y {
				tr.Max.Y = bounds.Max.Y
			}
			if br.Min.Y > br.Max.Y {
				br.Min.Y = br.Max.Y
			}
			bot.SetRect(br)
			clampBtn(top, bounds)
			clampBtn(bot, bounds)
		}

		if p.IsMobile() {
			// Mobile two-row transport using nested grids.
			outerGrid := NewGridLayout(topBounds, []float64{1}, []float64{1, 1})
			// Row 0: Play | Stop | BPM box | BPM+/- | Sub
			row0Grid := outerGrid.SubGrid(0, 0,
				[]float64{1.0, 1.0, 2.0, 1.0, 1.0}, []float64{1})
			// Row 1: VolIcon | ViewSwitch | Overflow
			row1Grid := outerGrid.SubGrid(0, 1,
				[]float64{1.0, 1.0, 1.0}, []float64{1})

			row0Bounds := outerGrid.Cell(0, 0)

			dv.playBtn().SetRect(safeInset(row0Grid.Cell(0, 0), pad))
			dv.stopBtn().SetRect(safeInset(row0Grid.Cell(1, 0), pad))
			dv.bpmBox().Rect = safeInset(row0Grid.Cell(2, 0), pad)
			bpmCol := safeInset(row0Grid.Cell(3, 0), pad)
			stackVertical(dv.bpmIncBtn(), dv.bpmDecBtn(), bpmCol, row0Bounds)
			dv.subdivBtn().SetRect(safeInset(row0Grid.Cell(4, 0), pad))
			// Mobile: volume icon opens popup; no inline slider.
			dv.mainVolIconRect = safeInset(row1Grid.Cell(0, 0), pad)
			if dv.mainVolSlider() != nil {
				dv.mainVolSlider().SetRect(image.Rectangle{})
				dv.mainVolRect = image.Rectangle{}
			}
			if dv.viewSwitchBtn() != nil {
				dv.viewSwitchBtn().SetRect(safeInset(row1Grid.Cell(1, 0), pad))
			}
			// Hide desktop-only buttons on mobile.
			dv.trackBtn().SetRect(image.Rectangle{})
			// Upload/Import/Export behind overflow on mobile.
			dv.uploadBtn().SetRect(image.Rectangle{})
			dv.importBtn().SetRect(image.Rectangle{})
			dv.exportBtn().SetRect(image.Rectangle{})
			// Hide legacy EQ toggle on mobile (replaced by viewSwitchBtn).
			if dv.eqToggleMobile() != nil {
				dv.eqToggleMobile().SetRect(image.Rectangle{})
			}
		} else {
			// Desktop single-row transport using persistent LayoutGroup.
			rowWeights := []float64{1.3, 1.3, 2.2, 0.7, 1.0, 0.3, 1.0, 0.8, 0.8, 0.8}
			if dv.transportGroup() == nil {
				dv.transportZone.transportGroup = NewLayoutGroup("transport", topBounds, rowWeights, []float64{1})
			}
			dv.transportGroup().SetBounds(topBounds)

			dv.playBtn().SetRect(safeInset(dv.transportGroup().Cell(0, 0), pad))
			dv.stopBtn().SetRect(safeInset(dv.transportGroup().Cell(1, 0), pad))
			dv.bpmBox().Rect = safeInset(dv.transportGroup().Cell(2, 0), pad)
			bpmCol := safeInset(dv.transportGroup().Cell(3, 0), pad)
			stackVertical(dv.bpmIncBtn(), dv.bpmDecBtn(), bpmCol, topBounds)
			dv.subdivBtn().SetRect(safeInset(dv.transportGroup().Cell(4, 0), pad))
			volCell := safeInset(dv.transportGroup().Cell(6, 0), pad)
			dv.mainVolIconRect = volCell
			if dv.mainVolSlider() != nil {
				dv.mainVolSlider().SetRect(image.Rectangle{})
				dv.mainVolRect = image.Rectangle{}
			}
			dv.uploadBtn().SetRect(safeInset(dv.transportGroup().Cell(7, 0), pad))
			dv.importBtn().SetRect(safeInset(dv.transportGroup().Cell(8, 0), pad))
			dv.exportBtn().SetRect(safeInset(dv.transportGroup().Cell(9, 0), pad))
			// Track button positioned in timeline area (see below), not toolbar.
			dv.trackBtn().SetRect(image.Rectangle{})
			// Hide mobile-only buttons on desktop.
			if dv.eqToggleMobile() != nil {
				dv.eqToggleMobile().SetRect(image.Rectangle{})
			}
			if dv.viewSwitchBtn() != nil {
				dv.viewSwitchBtn().SetRect(image.Rectangle{})
			}
		}
		ensureGap(dv.playBtn(), dv.stopBtn())
	} // end legacy transport layout

	// Timeline/progress lives inside the timeline widget near the bottom of its header row.
	tlWidget := dv.widgetRects[WidgetTimeline]
	if tlWidget.Empty() {
		tlWidget = image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y, dv.Bounds.Max.X-10, dv.Bounds.Min.Y+dv.headerH+dv.rowsAreaHeight())
	}
	headerTop := tlWidget.Min.Y
	headerH := dv.headerH
	if headerH > tlWidget.Dy() {
		headerH = tlWidget.Dy()
	}
	top := headerTop + headerH - tlBarHeight() - 2
	if top < tlWidget.Min.Y {
		top = tlWidget.Min.Y
	}
	const trackBtnGap = 4 // breathing room between track button and counter
	dv.timelineRect = image.Rect(
		tlWidget.Min.X,
		top,
		tlWidget.Max.X,
		top+tlBarHeight(),
	)

	// Tall enough for the keycap chrome drawBeatCounter paints, or it bleeds into the bar.
	const minChromeGap = 2
	pillH := TextHeight() + 2*beatCounterPillPadY + genGeomKeycapWallDepth
	infoH := debugCharH + 4
	if pillH > infoH {
		infoH = pillH
	}
	// Matches the transport buttons so the band reads as keycaps, not slivers.
	if pb := dv.playBtn().Rect(); !pb.Empty() && pb.Dy() > infoH {
		infoH = pb.Dy()
	}
	// Capped even when the room is below pillH (real fonts are taller than the stub's), since
	// drawBeatCounter shrinks to fit; bandTopGap keeps the keycaps clear of the splitter line.
	const bandTopGap = 2
	if avail := dv.timelineRect.Min.Y - tlWidget.Min.Y - minChromeGap - bandTopGap; avail > 0 && infoH > avail {
		infoH = avail
	}
	bcTop := dv.timelineRect.Min.Y - infoH - minChromeGap
	if bcTop < tlWidget.Min.Y+bandTopGap {
		bcTop = tlWidget.Min.Y + bandTopGap
	}
	// One height and one centre for every band keycap, so the band reads as a single row.
	keyH := pillH
	if keyH > infoH {
		keyH = infoH
	}
	keyY := bcTop + (infoH-keyH)/2
	bandLeftReserved := keyH + trackBtnGap
	dv.beatCounterRect = image.Rect(
		tlWidget.Min.X+bandLeftReserved, bcTop,
		dv.timelineRect.Max.X, bcTop+infoH,
	)

	// Desktop only: the pair is too small to discover on a phone, where it lives in the overflow menu.
	if !p.IsMobile() && dv.currentViewMode != viewModeEQ {
		gap := SpaceXS
		incX := dv.beatCounterRect.Max.X - SpaceXS - keyH
		decX := incX - gap - keyH
		dv.lenDecBtn.SetRect(image.Rect(decX, keyY, decX+keyH, keyY+keyH))
		dv.lenIncBtn.SetRect(image.Rect(incX, keyY, incX+keyH, keyY+keyH))
		dv.beatCounterRect.Max.X = decX - SpaceXS
	} else {
		dv.lenIncBtn.SetRect(image.Rectangle{})
		dv.lenDecBtn.SetRect(image.Rectangle{})
	}

	// Before the track button, whose right edge anchors to the counter, so the chips, counter and
	// track button all stay inside the timeline widget.
	if dv.rowRackZone != nil {
		dv.rowRackZone.SetAddRowRightReserve(0)
	}
	if p.IsMobile() && !dv.MobileEQMode() && !dv.bottomActionBarRect.Empty() &&
		!dv.timelineRect.Empty() {
		side := TouchMinTarget()
		rightX := tlWidget.Max.X
		if rightX == 0 {
			rightX = dv.Bounds.Max.X
		}
		chipsRight := rightX
		chipsLeft := chipsRight - side
		// Y span: full timeline header band (beat counter pill + ruler).
		y0 := dv.beatCounterRect.Min.Y
		if dv.beatCounterRect.Empty() || dv.timelineRect.Min.Y < y0 {
			y0 = dv.timelineRect.Min.Y
		}
		y1 := dv.timelineRect.Max.Y
		// At least TouchMinTarget tall, even in a short header.
		if y1-y0 < side {
			y1 = y0 + side
		}
		dv.rowZoomChipRect = image.Rect(chipsLeft, y0, chipsRight, y1)
		incCell, decCell := dv.rowZoomCells()
		chipSide := keyH
		if gapped := incCell.Dy() - SpaceXS; chipSide > gapped {
			chipSide = gapped // keep the two keycaps visibly apart
		}
		if dv.rowZoomIncBtn != nil {
			dv.rowZoomIncBtn.SetRect(centeredSquare(incCell, chipSide))
		}
		if dv.rowZoomDecBtn != nil {
			dv.rowZoomDecBtn.SetRect(centeredSquare(decCell, chipSide))
		}
		rightLimit := chipsLeft - SpaceXS
		if dv.timelineRect.Max.X > rightLimit {
			dv.timelineRect.Max.X = rightLimit
		}
		if dv.beatCounterRect.Max.X > rightLimit {
			dv.beatCounterRect.Max.X = rightLimit
		}
	} else {
		dv.rowZoomChipRect = image.Rectangle{}
		if dv.rowZoomIncBtn != nil {
			dv.rowZoomIncBtn.SetRect(image.Rectangle{})
		}
		if dv.rowZoomDecBtn != nil {
			dv.rowZoomDecBtn.SetRect(image.Rectangle{})
		}
	}

	// Its hit area is touch-expanded, so a phone gets a 44 px target without a big visual block.
	if !dv.beatCounterRect.Empty() {
		dv.trackBtn().SetRect(image.Rect(
			tlWidget.Min.X, keyY,
			tlWidget.Min.X+keyH, keyY+keyH,
		))
	}

	// After the len-button and chip shrinks so it respects their clamps; beatCounterSlotWidth is the ceiling.
	if !dv.beatCounterRect.Empty() {
		// Includes the leading icon and padding, or the keycap crops "0:00".
		readoutW := beatCounterContentWidth(dv.timelineInfo(dv.lastElapsedBeats))
		counterSlotW := readoutW
		if ub := beatCounterSlotWidth(); counterSlotW > ub {
			counterSlotW = ub
		}
		if counterSlotW > dv.beatCounterRect.Dx() {
			counterSlotW = dv.beatCounterRect.Dx()
		}
		dv.beatCounterRect.Max.X = dv.beatCounterRect.Min.X + counterSlotW
	}

	// The chrome reservations narrow the ribbon only, never the grid.
	{
		cellsTop := dv.Bounds.Min.Y + dv.headerH
		cellsBottom := dv.rowsBottom()
		if cellsBottom < cellsTop {
			cellsBottom = cellsTop
		}
		dv.cellGridRect = image.Rect(tlWidget.Min.X, cellsTop, tlWidget.Max.X, cellsBottom)
	}

	// Delegate timeline zone layout when available.
	if dv.timelineZone != nil && dv.tree != nil {
		// The clip must span the chrome above the bar, the bar and the grid, including the len buttons
		// past timelineRect.Max.X and the track button left of it.
		rowsBottom := dv.rowsBottom()
		if p.IsMobile() && dv.MobileEQMode() {
			rowsBottom = dv.Bounds.Max.Y
		}
		tlZoneRect := tlWidget
		if tlZoneRect.Max.Y < rowsBottom {
			tlZoneRect.Max.Y = rowsBottom
		}
		// Defense in depth: chrome pushed outside tlWidget still gets clipped in.
		tlZoneRect = tlZoneRect.Union(dv.beatCounterRect)
		tlZoneRect = tlZoneRect.Union(dv.timelineRect)
		if r := dv.trackBtn().Rect(); !r.Empty() {
			tlZoneRect = tlZoneRect.Union(r)
		}
		if r := dv.lenIncBtn.Rect(); !r.Empty() {
			tlZoneRect = tlZoneRect.Union(r)
		}
		if r := dv.lenDecBtn.Rect(); !r.Empty() {
			tlZoneRect = tlZoneRect.Union(r)
		}
		dv.timelineZone.SetTimelineBarHeight(tlBarHeight())
		dv.timelineZone.SetTimelineBarRect(dv.timelineRect)
		dv.timelineZone.SetCellGridRect(dv.cellGridRect)
		dv.tree.SetZoneRect("timeline", tlZoneRect)
		if dv.timelineLayoutDue(tlZoneRect) {
			dv.tree.LayoutZoneNow("timeline")
		}
	}

	// EQ panel is anchored to the Wave widget; if missing, fall back to the bottom of the timeline widget.
	eqWidget := dv.widgetRects[WidgetWave]
	if eqWidget.Empty() {
		eqWidget = image.Rect(tlWidget.Min.X, dv.Bounds.Max.Y-dv.eqH, tlWidget.Max.X, dv.Bounds.Max.Y)
	}
	// A user drag sets eqH directly and must skip the floor expansion below, or the drag has no effect.
	if dv.userEqH > 0 && !Profile().IsMobile() {
		eqWidget = image.Rect(eqWidget.Min.X, dv.Bounds.Max.Y-dv.eqH, eqWidget.Max.X, dv.Bounds.Max.Y)
	} else if minPanelH := dv.audioPanelFloorHeight(); minPanelH > 0 {
		// eqH is floored to the same value, so rowsBottom() lines up with this panel top.
		if eqWidget.Dy() < minPanelH {
			expanded := image.Rect(
				eqWidget.Min.X,
				dv.Bounds.Max.Y-minPanelH,
				eqWidget.Max.X,
				dv.Bounds.Max.Y,
			)
			if expanded.Min.Y < dv.Bounds.Min.Y {
				expanded.Min.Y = dv.Bounds.Min.Y
			}
			eqWidget = expanded
		}
	}
	dv.eqRect = eqWidget
	// Clamped above the bottom bar so the view switcher stays visible in every mobile mode.
	if p.IsMobile() && dv.MobileEQMode() {
		maxY := dv.Bounds.Max.Y
		if !dv.bottomActionBarRect.Empty() && dv.bottomActionBarRect.Min.Y > 0 {
			maxY = dv.bottomActionBarRect.Min.Y - 1
		}
		dv.eqRect = image.Rect(
			dv.Bounds.Min.X,
			dv.Bounds.Min.Y+dv.headerH,
			dv.Bounds.Max.X,
			maxY,
		)
	}
	if len(dv.eqBandVals) != len(eqBandDefs) {
		dv.eqBandVals = make([]float64, len(eqBandDefs))
	}
	if dv.eqPanelZone != nil && dv.audioTree != nil {
		dv.audioTree.SetZoneRect("eq-panel", dv.eqRect)
		// Immediately, so rects exist before Draw (first-frame clicks); LayoutZoneNow honours the
		// visibility predicate, so a hidden panel's catch-all cannot leak over Pads.
		if dv.eqPanelLayoutDue() {
			dv.audioTree.LayoutZoneNow("eq-panel")
		}
	}

	if dv.layoutResizeZone != nil && dv.audioTree != nil {
		dv.audioTree.HitIndexRef().Update("layout-resize", dv.layoutResizeZone.HitAreas())
	}

	if dv.tree != nil {
		var peekAreas []HitArea
		if !dv.eqPeekRect.Empty() {
			peekAreas = []HitArea{{
				Rect:    dv.eqPeekRect,
				ZIndex:  150, // above bottom action bar buttons
				Handler: &eqPeekHitAdapter{dv: dv},
				Tag:     "eq-peek-expand",
			}}
		}
		dv.tree.HitIndexRef().Update("drumview-eq-peek", peekAreas)

		// Compact keycaps; Touch + ClipRect grow each target to its half of the strip and no further.
		var zoomAreas []HitArea
		incCell, decCell := dv.rowZoomCells()
		if dv.rowZoomIncBtn != nil && !dv.rowZoomIncBtn.Rect().Empty() {
			zoomAreas = append(zoomAreas, HitArea{
				Rect:     dv.rowZoomIncBtn.Rect(),
				ZIndex:   140, // above rack rows, below overlays
				Handler:  &buttonHitAdapter{btn: dv.rowZoomIncBtn},
				Tag:      "drumview-row-zoom-in",
				Touch:    true,
				ClipRect: incCell,
			})
		}
		if dv.rowZoomDecBtn != nil && !dv.rowZoomDecBtn.Rect().Empty() {
			zoomAreas = append(zoomAreas, HitArea{
				Rect:     dv.rowZoomDecBtn.Rect(),
				ZIndex:   140,
				Handler:  &buttonHitAdapter{btn: dv.rowZoomDecBtn},
				Tag:      "drumview-row-zoom-out",
				Touch:    true,
				ClipRect: decCell,
			})
		}
		dv.tree.HitIndexRef().Update("drumview-row-zoom", zoomAreas)
	}

	// The native keyboard producers find the field through dv.instSearchRect, which nothing else
	// assigns; without this a tap on it never opens the phone keyboard.
	if dv.instMenuComp != nil && dv.IsInstMenuOpen() {
		dv.instSearchBox = dv.instMenuComp.SearchBox()
		dv.instSearchRect = dv.instMenuComp.SearchRect()
	} else {
		dv.instSearchRect = image.Rectangle{}
	}
}

func (dv *DrumView) resetOnScreenModeChange(toSmall bool) {
	pp := Profile() // use fresh profile for the new mode
	if !toSmall {
		// Through setViewMode: the only path allowed to write currentViewMode.
		dv.mobileEQInited = false
		dv.SetMobileEQMode(false)
		dv.mobileEQCollapsed = false
		dv.closeVolumePopup()
		if dv.widgets != nil {
			dv.widgets.SetWeights(pp.ColWeights, pp.RowWeights)
			dv.widgets.ToggleWidget(WidgetWave, true)
		}
	} else {
		// Entering mobile: reset so recalcButtons mobile-init runs
		dv.mobileEQInited = false
		if dv.widgets != nil {
			dv.widgets.SetWeights(pp.ColWeights, pp.RowWeights)
		}
	}
	// Common: invalidate all caches (row height 24↔44px change)
	dv.labelWidthDirty = true
	dv.bgDirty = true
	dv.markAllRowsDirty()
	dv.markRowControlsDirty()
	dv.invalidateRowCaches()
	dv.rowsLayerDirty = true
	dv.toolbarCache = nil
	dv.toolbarCacheHash = 0
	// Even if the following SetBounds short-circuits on equal bounds.
	dv.refreshWidgetLayout()
	// Now, so a draw in this same frame sees the new profile's styles.
	dv.refreshLenButtonsStyle()
	dv.CloseAllPopups()
}

// At least one beat; at most what keeps cells readable at the platform's minimum cell width.
func (dv *DrumView) clampLength(n int) int {
	inc := max1(dv.timelineUnitsPerBeat)
	minLen := inc

	// Use the same effective width that calcLayout uses for cell sizing.
	w := dv.cellGridRect.Dx()

	if w > 0 {
		mcw := MinCellWidth()
		if mcw < 1 {
			mcw = 1
		}
		maxLen := w / mcw
		if maxLen < minLen {
			maxLen = minLen
		}
		if n > maxLen {
			n = maxLen
		}
	}

	if n < minLen {
		n = minLen
	}
	return n
}

// newLen is in subdivisions.
func (dv *DrumView) changeLength(newLen int) {
	newLen = dv.clampLength(newLen)
	if newLen == dv.Length {
		return
	}
	dv.lengthChanging = true
	if newLen > dv.Length {
		dv.logger.Debugf("[drumview] length increased to: %d", newLen)
	} else {
		dv.logger.Debugf("[drumview] length decreased to: %d", newLen)
	}
	oldLen := dv.Length
	dv.Length = newLen
	for _, r := range dv.Rows {
		newSteps := make([]bool, dv.Length)
		newTypes := make([]model.NodeType, dv.Length)
		n := oldLen
		if dv.Length < n {
			n = dv.Length
		}
		copy(newSteps[:n], r.Steps[:n])
		copy(newTypes[:n], r.CellTypes[:n])
		r.Steps = newSteps
		r.CellTypes = newTypes
	}
	dv.SetBeatLength(dv.Length) // Update graph's beat length
	dv.bgDirty = true
	dv.markAllRowsDirty()
}

// The one formula for where the rows end; never inline it.
func (dv *DrumView) rowsBottom() int {
	rb := dv.Bounds.Max.Y - dv.eqH
	if !dv.bottomActionBarRect.Empty() {
		rb -= dv.bottomActionBarRect.Dy()
	}
	if !dv.eqPeekRect.Empty() {
		rb -= dv.eqPeekRect.Dy()
	}
	return rb
}

// nameBoxRect returns the fixed rect for the WAV naming input box.
func (dv *DrumView) nameBoxRect() image.Rectangle {
	return image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
}

// For bulk operations such as import, which would otherwise lay out O(rows × instruments).
func (dv *DrumView) SuppressLayout() { dv.layoutSuppressed = true }

// ResumeLayout re-enables layout and performs a single recalculation.
func (dv *DrumView) ResumeLayout() {
	dv.layoutSuppressed = false
	dv.invalidateLabelCaches()
	dv.calcLayout()
}

func (dv *DrumView) calcLayout() {
	if dv.layoutSuppressed {
		return
	}
	dv.calcLabelWidth()
	if len(dv.Rows) > 0 {
		w := dv.cellGridRect.Dx()
		if w <= 0 {
			w = dv.Bounds.Dx() - dv.labelW - dv.controlsW
		}
		dv.cell = w / len(dv.Rows[0].Steps)
		if dv.cell < 1 {
			dv.cell = 1
		}
	}
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	panelRect := dv.widgetRects[WidgetRack]
	if panelRect.Empty() {
		panelRect = image.Rect(dv.Bounds.Min.X, rowsTop, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.rowsBottom())
	}
	// Clamp rack top to match capped headerH (widget board may allocate more).
	if panelRect.Min.Y < rowsTop {
		panelRect.Min.Y = rowsTop
	}
	// Through the tree, so the HitIndex is republished in the same step.
	dv.tree.LayoutZoneNow("row-rack")
	// If timeline dimensions changed, row sprite caches must be rebuilt.
	if dv.rowCacheW != dv.cellGridRect.Dx() || dv.rowCacheH != dv.rowHeight() {
		dv.rowCacheW = dv.cellGridRect.Dx()
		dv.rowCacheH = dv.rowHeight()
		dv.markAllRowsDirty()
	}
	if dv.instMenuFullRect.Dx() == 0 {
		minMenuW := dv.labelW + dv.controlsW/2
		if minMenuW < 260 {
			minMenuW = 260
		}
		hostW := dv.widgetRects[WidgetRack].Dx()
		if hostW == 0 {
			hostW = dv.Bounds.Dx()
		}
		if minMenuW > hostW {
			minMenuW = hostW
		}
		dv.instMenuFullRect = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y, dv.Bounds.Min.X+minMenuW, dv.Bounds.Min.Y+dv.rowHeight()*3)
	}
	// Invalidate row controls cache when button positions change.
	dv.markRowControlsDirty()
}

func (dv *DrumView) invalidateLabelCaches() {
	dv.labelWidthDirty = true
	dv.instLabelCache = nil
}

// Short names ("Kick", "Hat") must not collapse the rack into a sliver.
const rackLabelMinW = 140

// Never more than half the viewport.
func (dv *DrumView) calcLabelWidth() {
	// Fast path: return cached value if not dirty and data hasn't changed.
	maxAllowed := dv.Bounds.Dx() / 2
	// Check if underlying data has changed (catches direct modifications).
	dataChanged := len(dv.Rows) != dv.labelCacheRowCount || len(dv.instOptions) != dv.labelCacheInstLen
	if !dataChanged && len(dv.Rows) == len(dv.labelCacheRowNames) {
		// Quick check if any row name differs from cached.
		for i, r := range dv.Rows {
			if r.Name != dv.labelCacheRowNames[i] {
				dataChanged = true
				break
			}
		}
	}
	if !dv.labelWidthDirty && !dataChanged && dv.cachedLabelW > 0 {
		// Bounds can change without invalidating the cache (window resize).
		target := dv.cachedLabelW
		if target > maxAllowed {
			target = maxAllowed
		}
		dv.labelW = target
		return
	}

	maxPx := 0
	for _, r := range dv.Rows {
		if w := TextWidth(r.Name); w > maxPx {
			maxPx = w
		}
	}
	// Measured over the rows on screen only: the whole catalog sized the column to the roster's
	// longest name.
	pad := SpaceSM*3 + SwatchSizeMD
	target := maxPx + pad
	if target < rackLabelMinW {
		target = rackLabelMinW
	}
	// Cache the pre-clamped value so bounds changes don't require full recompute.
	dv.cachedLabelW = target
	dv.labelCacheRowCount = len(dv.Rows)
	dv.labelCacheInstLen = len(dv.instOptions)
	// Cache row names to detect direct modifications.
	if cap(dv.labelCacheRowNames) < len(dv.Rows) {
		dv.labelCacheRowNames = make([]string, len(dv.Rows))
	} else {
		dv.labelCacheRowNames = dv.labelCacheRowNames[:len(dv.Rows)]
	}
	for i, r := range dv.Rows {
		dv.labelCacheRowNames[i] = r.Name
	}
	if target > maxAllowed {
		target = maxAllowed
	}
	dv.labelW = target
	dv.labelWidthDirty = false
}

type eqPeekHitAdapter struct {
	dv *DrumView
}

func (h *eqPeekHitAdapter) OnPress(x, y int) InputResult {
	if globalTouchState != nil && globalTouchState.RecentMultiTouch() {
		return InputIgnored
	}
	if h.dv == nil {
		return InputIgnored
	}
	h.dv.mobileEQCollapsed = false
	h.dv.setViewMode(viewModeEQ)
	h.dv.bgDirty = true
	return InputCaptured
}

func (h *eqPeekHitAdapter) OnDrag(x, y int)                     {}
func (h *eqPeekHitAdapter) OnRelease(x, y int)                  {}
func (h *eqPeekHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

type segmentedHitAdapter struct{ sc *SegmentedControl }

func (h *segmentedHitAdapter) OnPress(x, y int) InputResult {
	if h.sc.HitTest(x, y) {
		return InputCaptured
	}
	return InputIgnored
}
func (h *segmentedHitAdapter) OnDrag(x, y int)                     {}
func (h *segmentedHitAdapter) OnRelease(x, y int)                  {}
func (h *segmentedHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

func (dv *DrumView) rowZoomCells() (inc, dec image.Rectangle) {
	r := dv.rowZoomChipRect
	if r.Empty() {
		return image.Rectangle{}, image.Rectangle{}
	}
	mid := (r.Min.Y + r.Max.Y) / 2
	return image.Rect(r.Min.X, r.Min.Y, r.Max.X, mid), image.Rect(r.Min.X, mid, r.Max.X, r.Max.Y)
}

// centeredSquare is the side×side square centred in cell, shrunk to fit it.
func centeredSquare(cell image.Rectangle, side int) image.Rectangle {
	if side > cell.Dx() {
		side = cell.Dx()
	}
	if side > cell.Dy() {
		side = cell.Dy()
	}
	x := cell.Min.X + (cell.Dx()-side)/2
	y := cell.Min.Y + (cell.Dy()-side)/2
	return image.Rect(x, y, x+side, y+side)
}
