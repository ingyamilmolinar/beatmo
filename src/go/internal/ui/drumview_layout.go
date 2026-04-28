package ui

import (
	"fmt"
	"image"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

/* ─── public update ────────────────────────────────────────── */

func (dv *DrumView) recalcButtons() {
	// Late mobile init: when mobile profile first activates (after Layout()
	// calls SetTouchScreenSize on WASM), collapse EQ and mark layout dirty.
	p := Profile()
	if p.IsMobile() && !dv.mobileEQInited {
		dv.mobileEQInited = true
		dv.mobileEQCollapsed = true
		dv.bgDirty = true
		if dv.widgets != nil {
			// Update widget board for mobile column proportions.
			// At construction time the profile may be desktop on WASM
			// (SetTouchScreenSize hasn't been called yet), so the board
			// was created with desktop weights [1, 3]. Fix that now.
			dv.widgets.SetWeights(p.ColWeights, p.RowWeights)
			dv.widgets.ToggleWidget(WidgetWave, false)
			dv.widgetRects[WidgetWave] = dv.widgets.Rect(WidgetWave)
		}
		dv.refreshWidgetLayout()
		dv.eqH = 0
	}
	if runningUnderGoTest() {
		eqPanelHeight = 0
		dv.eqH = 0
	}
	dv.calcLabelWidth()

	transport := dv.widgetRects[WidgetTransport]
	if transport.Empty() {
		transport = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+dv.headerH)
	}
	// On desktop, cap at the widget column width to prevent buttons
	// from extending into the timeline widget area.
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
		if dv.transportZone.NeedsLayout() || dv.transportZone.rect != topBounds {
			dv.transportZone.Layout(topBounds)
			dv.tree.HitIndexRef().Update("transport", dv.transportZone.HitAreas())
		}
		dv.mainVolIconRect = dv.transportZone.mainVolIconRect
		dv.mainVolRect = dv.transportZone.mainVolRect
	}

	// Delegate row rack layout to RowRackZone when available.
	if dv.rowRackZone != nil && dv.tree != nil {
		rackRect := dv.widgetRects[WidgetRack]
		if rackRect.Empty() {
			rowsTop := dv.Bounds.Min.Y + dv.headerH
			rackRect = image.Rect(dv.Bounds.Min.X, rowsTop, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Max.Y-dv.eqH)
		}
		// Clamp rack top to match capped headerH.
		rowsTop := dv.Bounds.Min.Y + dv.headerH
		if rackRect.Min.Y < rowsTop {
			rackRect.Min.Y = rowsTop
		}
		// Override the zone's visible rows to the row count that actually
		// fits inside the rack widget rect (rackVisibleRows) — never the
		// bounds-derived dv.visibleRows() which can over-estimate when
		// the widget board allocates space below the rack to the wave
		// widget. Over-estimating placed per-row widgets at Y bands
		// outside the rack rect, producing the screenshot's "phantom
		// row + addRowBtn-in-the-middle" layout glitch. The complementary
		// invariant (rack + timeline span the full rows-area when EQ is
		// collapsed) is enforced in refreshWidgetLayout, so for the
		// EQ-visible case rackVisibleRows == dv.visibleRows() in practice.
		dv.rowRackZone.SetVisibleRowsOverride(dv.rackVisibleRows())
		dv.tree.SetZoneRect("row-rack", rackRect)
		dv.rowRackZone.SetScreenBounds(dv.Bounds)
		// Bidirectional sync: if RowRackZone's scroll updated its own
		// rowOffset (via scroll handler), adopt that value before we
		// push ours. This prevents overwriting scroll-initiated changes.
		if zoneOff := dv.rowRackZone.RowOffset(); zoneOff != dv.rowOffset {
			dv.rowOffset = zoneOff
		}
		dv.rowRackZone.SetRowOffset(dv.rowOffset)
		dv.rowRackZone.SetSelRow(dv.selRow)
		if dv.rowRackZone.NeedsLayout() || dv.rowRackZone.rect != rackRect {
			dv.rowRackZone.Layout(rackRect)
			dv.tree.HitIndexRef().Update("row-rack", dv.rowRackZone.HitAreas())
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
			if dv.overflowBtn() != nil {
				dv.overflowBtn().SetRect(safeInset(row1Grid.Cell(2, 0), pad))
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
			// col 5 is flexible spacer
			// Desktop: icon-only volume (popup on click).
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
			if dv.overflowBtn() != nil {
				dv.overflowBtn().SetRect(image.Rectangle{})
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
	// Compute track button width — positioned in timeline area on desktop only.
	trackBtnW := 0
	const trackBtnGap = 4 // breathing room between track button and timeline
	if !p.IsMobile() {
		trackBtnW = dv.playBtn().Rect().Dx()
		if trackBtnW <= 0 {
			trackBtnW = 44
		}
	}
	// timelineRect starts AFTER the track button on desktop so the button
	// never paints over the leftmost portion of the timeline progress bar
	// (initial seconds were invisible when the two shared the same X range).
	// On mobile trackBtnW=0, so the timeline keeps the full widget width.
	tlLeftReserved := 0
	if trackBtnW > 0 {
		tlLeftReserved = trackBtnW + trackBtnGap
	}
	dv.timelineRect = image.Rect(
		tlWidget.Min.X+tlLeftReserved,
		top,
		tlWidget.Max.X,
		top+tlBarHeight(),
	)

	// Beat counter rect: above the timeline bar. Aligned to timelineRect's
	// left edge — that edge already accounts for the track button reservation.
	infoH := debugCharH + 4
	bcTop := dv.timelineRect.Min.Y - infoH - 2
	if bcTop < tlWidget.Min.Y {
		bcTop = tlWidget.Min.Y
	}
	dv.beatCounterRect = image.Rect(
		dv.timelineRect.Min.X, bcTop,
		dv.timelineRect.Max.X, bcTop+infoH,
	)

	// Position len +/- buttons at top-right of timeline widget area.
	{
		btnW := 36
		btnH := dv.beatCounterRect.Dy()
		if btnH < 20 {
			btnH = 20
		}
		// Stack vertically: Inc on top, Dec below.
		x := dv.timelineRect.Max.X - btnW
		y := dv.beatCounterRect.Min.Y
		dv.lenIncBtn.SetRect(image.Rect(x, y, x+btnW, y+btnH))
		dv.lenDecBtn.SetRect(image.Rect(x, y+btnH, x+btnW, y+2*btnH))
		// Shrink beat counter and timeline to avoid overlapping the buttons.
		dv.beatCounterRect.Max.X = x - 4
		dv.timelineRect.Max.X = x - 4
	}

	// Hide len +/- buttons when EQ/Wave panel is active (mobile view toggle).
	if dv.currentViewMode == viewModeAudio {
		dv.lenIncBtn.SetRect(image.Rectangle{})
		dv.lenDecBtn.SetRect(image.Rectangle{})
	}

	// Position track button in timeline area on desktop; hidden on mobile.
	if !p.IsMobile() {
		trackBtnBottom := dv.timelineRect.Max.Y
		if pb := dv.playBtn().Rect(); !pb.Empty() && pb.Max.Y > trackBtnBottom {
			trackBtnBottom = pb.Max.Y
		}
		dv.trackBtn().SetRect(image.Rect(
			tlWidget.Min.X, dv.beatCounterRect.Min.Y,
			tlWidget.Min.X+trackBtnW, trackBtnBottom,
		))
	}

	// Delegate timeline zone layout when available.
	if dv.timelineZone != nil && dv.tree != nil {
		// The timeline zone covers the timeline bar + the steps grid below it.
		// timelineRect is positioned at the bottom of the header; the grid
		// extends from there down to the bottom of the rows area.
		rowsBottom := dv.Bounds.Max.Y - dv.eqH
		if p.IsMobile() && dv.mobileEQMode {
			rowsBottom = dv.Bounds.Max.Y
		}
		tlZoneRect := image.Rect(
			dv.timelineRect.Min.X,
			dv.timelineRect.Min.Y,
			dv.timelineRect.Max.X,
			rowsBottom,
		)
		if tlZoneRect.Max.Y < dv.timelineRect.Max.Y {
			tlZoneRect.Max.Y = dv.timelineRect.Max.Y
		}
		dv.timelineZone.SetTimelineBarHeight(tlBarHeight())
		dv.tree.SetZoneRect("timeline", tlZoneRect)
		dv.timelineZone.Layout(tlZoneRect)
		dv.tree.HitIndexRef().Update("timeline", dv.timelineZone.HitAreas())
	}

	// EQ panel is anchored to the Wave widget; if missing, fall back to the bottom of the timeline widget.
	eqWidget := dv.widgetRects[WidgetWave]
	if eqWidget.Empty() {
		eqWidget = image.Rect(tlWidget.Min.X, dv.Bounds.Max.Y-dv.eqH, tlWidget.Max.X, dv.Bounds.Max.Y)
	}
	dv.eqRect = eqWidget
	// Mobile EQ mode: use full drum pane area below the transport header.
	if p.IsMobile() && dv.mobileEQMode {
		dv.eqRect = image.Rect(
			dv.Bounds.Min.X,
			dv.Bounds.Min.Y+dv.headerH,
			dv.Bounds.Max.X,
			dv.Bounds.Max.Y,
		)
	}
	// EQ buttons, sliders, and rect layout are owned by EQPanelZone (Phase 2).
	// Delegate layout to the zone, which sets all button/slider rects.
	if len(dv.eqBandVals) != len(eqBandDefs) {
		dv.eqBandVals = make([]float64, len(eqBandDefs))
	}
	if dv.eqPanelZone != nil && dv.tree != nil {
		dv.tree.SetZoneRect("eq-panel", dv.eqRect)
		// Force immediate layout so slider/button rects are available
		// before Draw or the next Update cycle (fixes first-frame clicks).
		dv.eqPanelZone.Layout(dv.eqRect)
		dv.tree.HitIndexRef().Update("eq-panel", dv.eqPanelZone.HitAreas())
	}

	// Layout resize zone — refresh hit areas so column/row divider pills
	// stay clickable after bounds changes and widget board resizes.
	if dv.layoutResizeZone != nil && dv.tree != nil {
		dv.tree.HitIndexRef().Update("layout-resize", dv.layoutResizeZone.HitAreas())
	}

	// Register focusable rects for mobile soft keyboard gesture-based focus.
	// On small screens, the mobile native input system handles text inputs
	// directly (creating real HTML <input> overlays), so we skip focus-rect
	// registration to avoid the proxy also capturing the touchend gesture.
	softKeyboardClearRects()
	if !p.IsMobile() {
		if dv.bpmBox() != nil && !dv.bpmBox().Rect.Empty() {
			r := dv.bpmBox().Rect
			softKeyboardRegisterRect("bpm", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "numeric")
		}
		if dv.IsInstMenuOpen() && dv.instSearchBox != nil && !dv.instSearchRect.Empty() {
			r := dv.instSearchRect
			softKeyboardRegisterRect("inst-search", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
		if dv.IsNamingOpen() && dv.nameBox != nil && !dv.nameBox.Rect.Empty() {
			r := dv.nameBox.Rect
			softKeyboardRegisterRect("wav-name", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
		if dv.renameBox != nil && !dv.renameBox.Rect.Empty() {
			r := dv.renameBox.Rect
			softKeyboardRegisterRect("rename", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
	}

	// Register mobile native input rects (replaces focus-rects for text inputs on mobile)
	if p.IsMobile() {
		mobileInputClear()

		// BPM box — direct rect
		if dv.bpmBox() != nil && !dv.bpmBox().Rect.Empty() {
			r := dv.bpmBox().Rect
			mobileInputRegister("bpm", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.bpmBox().Text, 4, "numeric")
		}

		// Rename — register trigger only when context menu is open.
		// The "Rename" button in the context menu is the trigger (index 1).
		// We must NOT register the kebab button itself as a trigger, otherwise
		// the JS touchend handler intercepts the tap and creates a native
		// rename input instead of letting the context menu open.
		if dv.IsContextMenuOpen() && dv.contextMenuRow >= 0 &&
			dv.contextMenuRow < len(dv.Rows) && dv.contextMenuRow < len(dv.rowLabels()) &&
			len(dv.contextMenuBtns) > 1 {
			renameBtn := dv.contextMenuBtns[1] // "Rename" is index 1
			trigR := renameBtn.Rect()
			labelR := dv.rowLabels()[dv.contextMenuRow].Rect()
			if !trigR.Empty() && !labelR.Empty() {
				mobileInputRegisterTrigger(
					fmt.Sprintf("rename-%d", dv.contextMenuRow),
					trigR.Min.X, trigR.Min.Y, trigR.Dx(), trigR.Dy(),
					labelR.Min.X, labelR.Min.Y, labelR.Dx(), labelR.Dy(),
					dv.Rows[dv.contextMenuRow].Name, 32, "text",
				)
			}
		}

		// Instrument search — direct rect (when inst menu is open)
		if dv.IsInstMenuOpen() && dv.instSearchBox != nil && !dv.instSearchRect.Empty() {
			r := dv.instSearchRect
			mobileInputRegister("inst-search", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.instSearchBox.Text, 40, "text")
		}

		// WAV name — direct rect (when naming)
		if dv.IsNamingOpen() && dv.nameBox != nil && !dv.nameBox.Rect.Empty() {
			r := dv.nameBox.Rect
			mobileInputRegister("wav-name", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.nameBox.Text, 32, "text")
		}
	}
}

// resetOnScreenModeChange resets DrumView state when crossing the mobile ↔ desktop
// threshold. Called from Game.Layout() when the profile's Class changes.
func (dv *DrumView) resetOnScreenModeChange(toSmall bool) {
	pp := Profile() // use fresh profile for the new mode
	if !toSmall {
		// Leaving mobile → desktop: reset mobile flags for next entry
		dv.mobileEQInited = false
		dv.mobileEQMode = false
		dv.mobileEQCollapsed = false
		dv.currentViewMode = viewModeRows
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
	dv.CloseAllPopups()
}

// clampLength enforces global min/max zoom limits for the drum view.
// Minimum: one full beat (timelineUnitsPerBeat). Maximum: derived from
// the effective cell-drawing width and a platform-aware minimum cell width
// so cells stay visually readable on small screens.
func (dv *DrumView) clampLength(n int) int {
	inc := max1(dv.timelineUnitsPerBeat)
	minLen := inc

	// Use the same effective width that calcLayout uses for cell sizing.
	w := dv.timelineRect.Dx()

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

// changeLength applies a new drum length with clamping and refreshes row
// buffers/caches. Callers should supply the desired length in subdivisions.
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

// rowControlWeights returns the grid column weights for per-row controls.
// Desktop: Label, VolBar, Mute, Solo, FX, Overflow(⋯)
// Mobile:  Label, (hidden), (hidden), VolumeIcon, (hidden), (hidden)
func rowControlWeights() []float64 {
	if Profile().IsMobile() {
		// Mobile: wider label + compact volume icon; other controls in context menu.
		// Kept at 9 elements to match mobile positionRowWidgets indexing.
		return []float64{7, 0, 0, 1.5, 0, 0, 0, 0, 0}
	}
	return []float64{
		6,   // Label
		2.5, // Volume mini-bar
		2,   // Mute
		2,   // Solo
		2.5, // FX (wider: "FX" needs more space than single chars)
		1,   // Overflow (⋯)
	}
}

// rowRectForIndex, positionRowWidgets, and positionAddRowBtn are now
// handled exclusively by RowRackZone.

// nameBoxRect returns the fixed rect for the WAV naming input box.
func (dv *DrumView) nameBoxRect() image.Rectangle {
	return image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
}

// SuppressLayout prevents calcLayout from running until ResumeLayout is called.
// Use during bulk operations (e.g., import) to avoid O(rows × instruments) work.
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
		w := dv.timelineRect.Dx()
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
		panelRect = image.Rect(dv.Bounds.Min.X, rowsTop, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Max.Y-dv.eqH)
	}
	// Clamp rack top to match capped headerH (widget board may allocate more).
	if panelRect.Min.Y < rowsTop {
		panelRect.Min.Y = rowsTop
	}
	// RowRackZone owns per-row buttons/sliders and the add-row button.
	// Ensure the zone's entries match the current row count by triggering
	// a re-layout. Accessor methods on DrumView delegate to the zone.
	dv.rowRackZone.Layout(dv.rowRackZone.rect)
	// If timeline dimensions changed, row sprite caches must be rebuilt.
	if dv.rowCacheW != dv.timelineRect.Dx() || dv.rowCacheH != dv.rowHeight() {
		dv.rowCacheW = dv.timelineRect.Dx()
		dv.rowCacheH = dv.rowHeight()
		dv.markAllRowsDirty()
	}
	// Seed a sane default popup rect so layout tests have dimensions even
	// before the menu opens.
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

// invalidateLabelCaches marks label-related caches dirty so they are
// recomputed on the next frame. Call this when instruments or rows change.
func (dv *DrumView) invalidateLabelCaches() {
	dv.labelWidthDirty = true
	dv.instLabelCache = nil
}

// calcLabelWidth adjusts labelW to fit the longest row or instrument label
// while keeping it within half of the viewport width.
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
		// Re-apply maxAllowed constraint in case bounds changed without
		// invalidating the cache (e.g., window resize).
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
	for _, id := range dv.instOptions {
		lbl := dv.instDisplayLabel(id)
		if w := TextWidth(lbl); w > maxPx {
			maxPx = w
		}
	}
	pad := SpaceXS*2 + 12
	target := maxPx + pad
	if target < 80 {
		target = 80
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
