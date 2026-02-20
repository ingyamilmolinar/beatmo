package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

/* ─── public update ────────────────────────────────────────── */

func (dv *DrumView) recalcButtons() {
	// Late mobile init: when isSmallScreen() first becomes true (after Layout()
	// calls SetTouchScreenSize on WASM), collapse EQ and mark layout dirty.
	if isSmallScreen() && !dv.mobileEQInited {
		dv.mobileEQInited = true
		dv.mobileEQCollapsed = true
		dv.bgDirty = true
		if dv.widgets != nil {
			dv.widgets.ToggleWidget(WidgetWave, false)
			dv.widgetRects[WidgetWave] = dv.widgets.Rect(WidgetWave)
		}
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
	if isSmallScreen() {
		// Keep transport within its widget column — no overlap into timeline.
	} else if widgetColW > 0 && transport.Dx() < widgetColW {
		transport.Max.X = transport.Min.X + widgetColW
	}
	leftCol := transport
	// Dynamic button sizing — single row transport.
	pad := buttonPad + 2
	if isSmallScreen() {
		pad = buttonPad + 1 // tighter spacing on mobile
	}
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
	controlLeft := leftCol.Min.X + 12 // small inset to avoid hugging the border
	if isSmallScreen() {
		controlLeft = leftCol.Min.X + 4 // tighter on mobile
	}
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

	if isSmallScreen() {
		// Mobile two-row transport using nested grids.
		outerGrid := NewGridLayout(topBounds, []float64{1}, []float64{1, 1})
		// Row 0: Play | Stop | BPM box | BPM+/- | Sub
		row0Grid := outerGrid.SubGrid(0, 0,
			[]float64{1.0, 1.0, 2.0, 1.0, 1.0}, []float64{1})
		// Row 1: VolIcon | ViewSwitch | Overflow
		row1Grid := outerGrid.SubGrid(0, 1,
			[]float64{1.0, 1.0, 1.0}, []float64{1})

		row0Bounds := outerGrid.Cell(0, 0)

		dv.playBtn.SetRect(safeInset(row0Grid.Cell(0, 0), pad))
		dv.stopBtn.SetRect(safeInset(row0Grid.Cell(1, 0), pad))
		dv.bpmBox.Rect = safeInset(row0Grid.Cell(2, 0), pad)
		bpmCol := safeInset(row0Grid.Cell(3, 0), pad)
		stackVertical(dv.bpmIncBtn, dv.bpmDecBtn, bpmCol, row0Bounds)
		dv.subdivBtn.SetRect(safeInset(row0Grid.Cell(4, 0), pad))
		// Mobile: volume icon opens popup; no inline slider.
		dv.mainVolIconRect = safeInset(row1Grid.Cell(0, 0), pad)
		if dv.mainVolSlider != nil {
			dv.mainVolSlider.SetRect(image.Rectangle{})
			dv.mainVolRect = image.Rectangle{}
		}
		if dv.viewSwitchBtn != nil {
			dv.viewSwitchBtn.SetRect(safeInset(row1Grid.Cell(1, 0), pad))
		}
		if dv.overflowBtn != nil {
			dv.overflowBtn.SetRect(safeInset(row1Grid.Cell(2, 0), pad))
		}
		// Hide desktop-only buttons on mobile.
		dv.trackBtn.SetRect(image.Rectangle{})
		// Upload/Import/Export behind overflow on mobile.
		dv.uploadBtn.SetRect(image.Rectangle{})
		dv.importBtn.SetRect(image.Rectangle{})
		dv.exportBtn.SetRect(image.Rectangle{})
		// Hide legacy EQ toggle on mobile (replaced by viewSwitchBtn).
		if dv.eqToggleMobile != nil {
			dv.eqToggleMobile.SetRect(image.Rectangle{})
		}
	} else {
		// Desktop two-row transport using persistent LayoutGroup.
		row0Weights := []float64{1.3, 1.3, 2.5, 0.8, 1.2}
		row1Weights := []float64{1.0, 1.0, 1.0, 3.0}
		if dv.transportGroup == nil {
			dv.transportGroup = NewLayoutGroup("transport", topBounds, []float64{1}, []float64{1, 1})
			dv.transportGroup.AddChild("row0", 0, 0, row0Weights, []float64{1})
			dv.transportGroup.AddChild("row1", 0, 1, row1Weights, []float64{1})
		}
		dv.transportGroup.SetBounds(topBounds)
		row0 := dv.transportGroup.Find("row0")
		row1 := dv.transportGroup.Find("row1")
		row0Bounds := dv.transportGroup.Cell(0, 0)

		dv.playBtn.SetRect(safeInset(row0.Cell(0, 0), pad))
		dv.stopBtn.SetRect(safeInset(row0.Cell(1, 0), pad))
		dv.bpmBox.Rect = safeInset(row0.Cell(2, 0), pad)
		bpmCol := safeInset(row0.Cell(3, 0), pad)
		stackVertical(dv.bpmIncBtn, dv.bpmDecBtn, bpmCol, row0Bounds)
		dv.subdivBtn.SetRect(safeInset(row0.Cell(4, 0), pad))

		dv.uploadBtn.SetRect(safeInset(row1.Cell(0, 0), pad))
		dv.importBtn.SetRect(safeInset(row1.Cell(1, 0), pad))
		dv.exportBtn.SetRect(safeInset(row1.Cell(2, 0), pad))
		// Desktop: inline volume slider with speaker icon (like instrument rows).
		volCell := safeInset(row1.Cell(3, 0), pad)
		dv.mainVolIconRect = volCell
		if dv.mainVolSlider != nil {
			dv.mainVolRect = volCell
			dv.mainVolSlider.SetRect(volCell)
		}
		// Track button positioned in timeline area (see below), not toolbar.
		dv.trackBtn.SetRect(image.Rectangle{})
		// Hide mobile-only buttons on desktop.
		if dv.eqToggleMobile != nil {
			dv.eqToggleMobile.SetRect(image.Rectangle{})
		}
		if dv.viewSwitchBtn != nil {
			dv.viewSwitchBtn.SetRect(image.Rectangle{})
		}
		if dv.overflowBtn != nil {
			dv.overflowBtn.SetRect(image.Rectangle{})
		}
	}
	ensureGap(dv.playBtn, dv.stopBtn)

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
	if !isSmallScreen() {
		trackBtnW = dv.playBtn.Rect().Dx()
		if trackBtnW <= 0 {
			trackBtnW = 44
		}
	}
	// timelineRect spans the full timeline widget width so drum cells
	// fill the available space without a gap from the track button offset.
	dv.timelineRect = image.Rect(
		tlWidget.Min.X,
		top,
		tlWidget.Max.X,
		top+tlBarHeight(),
	)

	// Beat counter rect: above the timeline bar, offset by trackBtnW
	// so text doesn't overlap the track button on desktop.
	infoH := debugCharH + 4
	bcTop := dv.timelineRect.Min.Y - infoH - 2
	if bcTop < tlWidget.Min.Y {
		bcTop = tlWidget.Min.Y
	}
	dv.beatCounterRect = image.Rect(
		dv.timelineRect.Min.X+trackBtnW, bcTop,
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

	// Position track button in timeline area on desktop; hidden on mobile.
	if !isSmallScreen() {
		trackBtnBottom := dv.timelineRect.Max.Y
		if pb := dv.playBtn.Rect(); !pb.Empty() && pb.Max.Y > trackBtnBottom {
			trackBtnBottom = pb.Max.Y
		}
		dv.trackBtn.SetRect(image.Rect(
			tlWidget.Min.X, dv.beatCounterRect.Min.Y,
			tlWidget.Min.X+trackBtnW, trackBtnBottom,
		))
	}

	// EQ panel is anchored to the Wave widget; if missing, fall back to the bottom of the timeline widget.
	eqWidget := dv.widgetRects[WidgetWave]
	if eqWidget.Empty() {
		eqWidget = image.Rect(tlWidget.Min.X, dv.Bounds.Max.Y-dv.eqH, tlWidget.Max.X, dv.Bounds.Max.Y)
	}
	dv.eqRect = eqWidget
	// Mobile EQ mode: use full drum pane area below the transport header.
	if isSmallScreen() && dv.mobileEQMode {
		dv.eqRect = image.Rect(
			dv.Bounds.Min.X,
			dv.Bounds.Min.Y+dv.headerH,
			dv.Bounds.Max.X,
			dv.Bounds.Max.Y,
		)
	}
	// Toggle button to switch between waveform and EQ views.
	if dv.eqToggleBtn == nil {
		dv.eqToggleBtn = NewButton("EQ", InstButtonStyle, func() {
			dv.eqWaveformMode = !dv.eqWaveformMode
			if dv.eqWaveformMode {
				dv.eqToggleBtn.Text = "EQ"
			} else {
				dv.eqToggleBtn.Text = "Wave"
			}
		})
		dv.eqWaveformMode = false
	}
	// Channel selector button for per-instrument EQ
	if dv.eqChannelBtn == nil {
		dv.eqChannelBtn = NewButton("Master", InstButtonStyle, func() {
			if dv.eqChannelOpen {
				dv.eqChannelOpen = false
			} else {
				dv.CloseAllPopups()
				dv.eqChannelOpen = true
				dv.eqChannelScroll.VS.First = 0
				dv.eqChannelScroll.HandleDragEnd()
				dv.buildEQChannelMenu()
				SuppressClicksUntilMouseUp()
			}
		})
	}
	btnW := 56
	btnH := 18
	if btnW > dv.eqRect.Dx()/2 {
		btnW = dv.eqRect.Dx() / 2
	}
	// Position channel button to the left of the toggle button
	channelBtnW := dv.calcEQChannelBtnWidth()
	if channelBtnW > dv.eqRect.Dx()/3 {
		channelBtnW = dv.eqRect.Dx() / 3
	}
	channelBtnRect := image.Rect(dv.eqRect.Min.X+6, dv.eqRect.Min.Y+4, dv.eqRect.Min.X+6+channelBtnW, dv.eqRect.Min.Y+4+btnH)
	dv.eqChannelBtn.SetRect(channelBtnRect)
	btnRect := image.Rect(dv.eqRect.Max.X-btnW-6, dv.eqRect.Min.Y+4, dv.eqRect.Max.X-6, dv.eqRect.Min.Y+4+btnH)
	dv.eqToggleBtn.SetRect(btnRect)
	// HPF/LPF toggle buttons — positioned between channel and toggle buttons.
	filterBtnW := 28
	filterBtnH := btnH
	if dv.hpfBtn == nil {
		dv.hpfBtn = NewButton("HP", InstButtonStyle, func() {
			dv.toggleHPF()
		})
	}
	if dv.lpfBtn == nil {
		dv.lpfBtn = NewButton("LP", InstButtonStyle, func() {
			dv.toggleLPF()
		})
	}
	hpfX := channelBtnRect.Max.X + 4
	dv.hpfBtn.SetRect(image.Rect(hpfX, dv.eqRect.Min.Y+4, hpfX+filterBtnW, dv.eqRect.Min.Y+4+filterBtnH))
	lpfX := hpfX + filterBtnW + 2
	dv.lpfBtn.SetRect(image.Rect(lpfX, dv.eqRect.Min.Y+4, lpfX+filterBtnW, dv.eqRect.Min.Y+4+filterBtnH))
	if len(dv.eqBandVals) != len(eqBandDefs) {
		dv.eqBandVals = make([]float64, len(eqBandDefs))
	}
	if len(dv.eqBandGainsDB) != len(eqBandDefs) {
		dv.eqBandGainsDB = make([]float64, len(eqBandDefs))
		for i := range dv.eqBandGainsDB {
			dv.eqBandGainsDB[i] = 0
		}
	}
	if len(dv.eqBandMuted) != len(eqBandDefs) {
		dv.eqBandMuted = make([]bool, len(eqBandDefs))
	}
	if len(dv.eqSliders) != len(eqBandDefs) {
		dv.eqSliders = make([]*Slider, len(eqBandDefs))
		for i := range dv.eqSliders {
			s := NewSlider(0.5) // center = 0 dB
			dv.eqSliders[i] = s
		}
	}
	if len(dv.eqMuteBtns) != len(eqBandDefs) {
		dv.eqMuteBtns = make([]*Button, len(eqBandDefs))
		for i := range dv.eqMuteBtns {
			bandIdx := i // capture loop variable for closure
			btn := NewButton("M", EQMuteButtonStyle, func() {
				dv.toggleEQBandMute(bandIdx)
			})
			dv.eqMuteBtns[i] = btn
		}
	}
	// Mobile EQ band buttons: tappable buttons that open the EQ popup.
	// These replace Slider.Handle() on mobile because Button.Handle()
	// expands the touch target to TouchMinTarget (44px), making the 14px-tall
	// band area reliably tappable. Desktop continues to use sliders directly.
	if isSmallScreen() && len(dv.eqBandBtns) != len(eqBandDefs) {
		dv.eqBandBtns = make([]*Button, len(eqBandDefs))
		for i := range dv.eqBandBtns {
			bandIdx := i
			btn := NewButton("", nil, func() {
				dv.openEQPopup(bandIdx)
			})
			dv.eqBandBtns[i] = btn
		}
	}

	// Register focusable rects for mobile soft keyboard gesture-based focus.
	// On small screens, the mobile native input system handles text inputs
	// directly (creating real HTML <input> overlays), so we skip focus-rect
	// registration to avoid the proxy also capturing the touchend gesture.
	softKeyboardClearRects()
	if !isSmallScreen() {
		if dv.bpmBox != nil && !dv.bpmBox.Rect.Empty() {
			r := dv.bpmBox.Rect
			softKeyboardRegisterRect("bpm", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "numeric")
		}
		if dv.instMenuOpen && dv.instSearchBox != nil && !dv.instSearchRect.Empty() {
			r := dv.instSearchRect
			softKeyboardRegisterRect("inst-search", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
		if dv.naming && dv.nameBox != nil && !dv.nameBox.Rect.Empty() {
			r := dv.nameBox.Rect
			softKeyboardRegisterRect("wav-name", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
		if dv.renameBox != nil && !dv.renameBox.Rect.Empty() {
			r := dv.renameBox.Rect
			softKeyboardRegisterRect("rename", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
	}

	// Register mobile native input rects (replaces focus-rects for text inputs on mobile)
	if isSmallScreen() {
		mobileInputClear()

		// BPM box — direct rect
		if dv.bpmBox != nil && !dv.bpmBox.Rect.Empty() {
			r := dv.bpmBox.Rect
			mobileInputRegister("bpm", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.bpmBox.Text, 4, "numeric")
		}

		// Rename — register trigger only when context menu is open.
		// The "Rename" button in the context menu is the trigger (index 1).
		// We must NOT register the kebab button itself as a trigger, otherwise
		// the JS touchend handler intercepts the tap and creates a native
		// rename input instead of letting the context menu open.
		if dv.contextMenuOpen && dv.contextMenuRow >= 0 &&
			dv.contextMenuRow < len(dv.Rows) && dv.contextMenuRow < len(dv.rowLabels) &&
			len(dv.contextMenuBtns) > 1 {
			renameBtn := dv.contextMenuBtns[1] // "Rename" is index 1
			trigR := renameBtn.Rect()
			labelR := dv.rowLabels[dv.contextMenuRow].Rect()
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
		if dv.instMenuOpen && dv.instSearchBox != nil && !dv.instSearchRect.Empty() {
			r := dv.instSearchRect
			mobileInputRegister("inst-search", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.instSearchBox.Text, 40, "text")
		}

		// WAV name — direct rect (when naming)
		if dv.naming && dv.nameBox != nil && !dv.nameBox.Rect.Empty() {
			r := dv.nameBox.Rect
			mobileInputRegister("wav-name", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.nameBox.Text, 32, "text")
		}
	}
}

// resetOnScreenModeChange resets DrumView state when crossing the mobile ↔ desktop
// threshold. Called from Game.Layout() when isSmallScreen() changes value.
func (dv *DrumView) resetOnScreenModeChange(toSmall bool) {
	if !toSmall {
		// Leaving mobile → desktop: reset mobile flags for next entry
		dv.mobileEQInited = false
		dv.mobileEQMode = false
		dv.mobileEQCollapsed = false
		dv.currentViewMode = viewModeRows
		dv.contextMenuOpen = false
		dv.overflowMenuOpen = false
		dv.volPopup.Close()
		if dv.widgets != nil {
			dv.widgets.ToggleWidget(WidgetWave, true)
		}
	} else {
		// Entering mobile: reset so recalcButtons mobile-init runs
		dv.mobileEQInited = false
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
		dv.logger.Infof("[DRUMVIEW] Length increased to: %d", newLen)
	} else {
		dv.logger.Infof("[DRUMVIEW] Length decreased to: %d", newLen)
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
// Desktop: Label, Edit/Save, Color, Volume, Mute, Solo, FX, Origin, Delete
// Mobile:  Label, (hidden), (hidden), VolumeIcon, (hidden), (hidden), (hidden), (hidden), (hidden)
func rowControlWeights() []float64 {
	if isSmallScreen() {
		// Mobile: wider label + compact volume icon; other controls in context menu
		return []float64{7, 0, 0, 1.5, 0, 0, 0, 0, 0}
	}
	// Button columns get weights proportional to the pixel width needed
	// for their text plus padding (insetRect + clipTextToWidth = 4*buttonPad).
	// Single-char buttons are the baseline (weight 3); wider text gets more.
	btnColWeight := func(text string) float64 {
		ref := TextWidth("M")
		if ref <= 0 {
			return 3
		}
		padOverhead := 4 * buttonPad // insetRect (2×) + clipTextToWidth (2×)
		w := 3.0 * float64(TextWidth(text)+padOverhead) / float64(ref+padOverhead)
		if w < 3 {
			w = 3
		}
		return w
	}
	return []float64{
		6,                  // Label
		2,                  // Edit/Save (icon)
		2,                  // Color (swatch)
		7,                  // Volume (slider)
		btnColWeight("M"),  // Mute
		btnColWeight("S"),  // Solo
		btnColWeight("FX"), // FX
		btnColWeight("O"),  // Origin
		btnColWeight("X"),  // Delete
	}
}

// rowRectForIndex computes the bounding rect for row i given layout params.
// Returns a zero rect for rows outside the visible window.
func (dv *DrumView) rowRectForIndex(i, rowsTop, vis int, panelRect image.Rectangle) image.Rectangle {
	if i < dv.rowOffset || i >= dv.rowOffset+vis {
		return image.Rectangle{}
	}
	y := rowsTop + (i-dv.rowOffset)*dv.rowHeight()
	return image.Rect(panelRect.Min.X, y, panelRect.Max.X, y+dv.rowHeight())
}

// positionRowWidgets sets rects for all per-row controls at index i.
// rowRect should be the bounding rect for the row (or zero for invisible rows).
func (dv *DrumView) positionRowWidgets(i int, rowRect image.Rectangle) {
	g := NewGridLayout(rowRect, rowControlWeights(), []float64{1})
	if i < len(dv.rowLabels) {
		dv.rowLabels[i].SetRect(insetRect(g.Cell(0, 0), buttonPad))
	}
	if isSmallScreen() {
		// Mobile: hide kebab, edit/save, mute, solo (all in context menu)
		if i < len(dv.rowMenuBtns) {
			dv.rowMenuBtns[i].SetRect(image.Rectangle{})
		}
		if i < len(dv.rowEditBtns) {
			dv.rowEditBtns[i].SetRect(image.Rectangle{})
		}
		if i < len(dv.rowSaveBtns) {
			dv.rowSaveBtns[i].SetRect(image.Rectangle{})
		}
		if i < len(dv.rowMuteBtns) {
			dv.rowMuteBtns[i].SetRect(image.Rectangle{})
		}
		if i < len(dv.rowSoloBtns) {
			dv.rowSoloBtns[i].SetRect(image.Rectangle{})
		}
	} else {
		// Desktop: show edit/save in column 1, hide kebab
		if i < len(dv.rowMenuBtns) {
			dv.rowMenuBtns[i].SetRect(image.Rectangle{})
		}
		if i < len(dv.rowEditBtns) {
			editCell := g.Cell(1, 0)
			editRect, saveRect := splitRectHoriz(editCell)
			splitPad := buttonPad
			if splitPad > 1 {
				splitPad--
			}
			dv.rowEditBtns[i].SetRect(insetRectSafe(editRect, splitPad))
			if i < len(dv.rowSaveBtns) {
				dv.rowSaveBtns[i].SetRect(insetRectSafe(saveRect, splitPad))
			}
		}
		if i < len(dv.rowSaveBtns) && i >= len(dv.rowEditBtns) {
			editCell := g.Cell(1, 0)
			_, saveRect := splitRectHoriz(editCell)
			splitPad := buttonPad
			if splitPad > 1 {
				splitPad--
			}
			dv.rowSaveBtns[i].SetRect(insetRectSafe(saveRect, splitPad))
		}
	}
	if i < len(dv.rowColorBtns) {
		dv.rowColorBtns[i].SetRect(insetRect(g.Cell(2, 0), buttonPad))
	}
	if i < len(dv.rowVolSliders) {
		dv.rowVolSliders[i].SetRect(insetRect(g.Cell(3, 0), buttonPad))
	}
	if i < len(dv.rowMuteBtns) {
		dv.rowMuteBtns[i].SetRect(insetRect(g.Cell(4, 0), buttonPad))
	}
	if i < len(dv.rowSoloBtns) {
		dv.rowSoloBtns[i].SetRect(insetRect(g.Cell(5, 0), buttonPad))
	}
	if i < len(dv.rowFXBtns) {
		dv.rowFXBtns[i].SetRect(insetRect(g.Cell(6, 0), buttonPad))
	}
	if i < len(dv.rowOriginBtns) {
		dv.rowOriginBtns[i].SetRect(insetRect(g.Cell(7, 0), buttonPad))
	}
	if i < len(dv.rowDeleteBtns) {
		dv.rowDeleteBtns[i].SetRect(insetRect(g.Cell(8, 0), buttonPad))
	}
}

// positionAddRowBtn sets the "+" button rect based on current row count,
// scroll offset, visible rows, and platform. This is the single source of
// truth for the add-row button position — called from both calcLayout()
// and updateRowRects().
func (dv *DrumView) positionAddRowBtn(rowsTop int, panelRect image.Rectangle, vis int) {
	nBelow := len(dv.Rows) - dv.rowOffset
	if nBelow > vis {
		nBelow = vis
	}
	addY := rowsTop + nBelow*dv.rowHeight()
	// Safety clamp so the button never extends below the rack area.
	rackBottom := panelRect.Max.Y
	if addY+dv.rowHeight() > rackBottom {
		addY = rackBottom - dv.rowHeight()
	}
	if addY < rowsTop {
		addY = rowsTop
	}
	if isSmallScreen() {
		if dv.mobileEQMode {
			dv.addRowBtn.SetRect(image.Rectangle{})
		} else {
			// Mobile: simple "+" text button in bottom-right, no fancy styling
			dv.addRowBtn.Text = "+"
			dv.addRowBtn.Icon = ""
			btnW := 28
			btnH := 24
			fabX := dv.Bounds.Max.X - btnW - 20
			fabY := dv.Bounds.Min.Y + dv.headerH + dv.rowsAreaHeight() - btnH - 8
			if fabY < rowsTop {
				fabY = rowsTop
			}
			dv.addRowBtn.SetRect(image.Rect(fabX, fabY, fabX+btnW, fabY+btnH))
		}
	} else {
		dv.addRowBtn.SetRect(insetRect(image.Rect(panelRect.Min.X, addY, panelRect.Max.X, addY+dv.rowHeight()), buttonPad))
	}
}

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
	dv.rowLabels = dv.rowLabels[:0]
	dv.rowEditBtns = dv.rowEditBtns[:0]
	dv.rowSaveBtns = dv.rowSaveBtns[:0]
	dv.rowColorBtns = dv.rowColorBtns[:0]
	dv.rowDeleteBtns = dv.rowDeleteBtns[:0]
	dv.rowVolSliders = dv.rowVolSliders[:0]
	dv.rowOriginBtns = dv.rowOriginBtns[:0]
	dv.rowMuteBtns = dv.rowMuteBtns[:0]
	dv.rowSoloBtns = dv.rowSoloBtns[:0]
	dv.rowMenuBtns = dv.rowMenuBtns[:0]
	dv.rowFXBtns = dv.rowFXBtns[:0]
	dv.rowGroups = dv.rowGroups[:0]
	vis := dv.visibleRows()
	if isSmallScreen() && dv.mobileEQMode {
		vis = 0
	}
	for i := range dv.Rows {
		rowRect := dv.rowRectForIndex(i, rowsTop, vis, panelRect)
		style := ButtonVisual(InstButtonStyle)
		if isSmallScreen() {
			style = MobileRowLabelStyle
		}
		if !dv.IsInstrumentAvailable(dv.Rows[i].Instrument) {
			style = MissingInstStyle
		}
		lbl := NewButton(dv.Rows[i].Name, style, nil)
		idx := i
		lbl.OnClick = func() {
			if isSmallScreen() {
				dv.openContextMenu(idx)
			} else {
				dv.openInstMenuForRow(idx)
			}
		}
		edit := NewButton("", InstButtonStyle, nil)
		edit.Icon = "pencil"
		editIdx := i
		edit.OnClick = func() {
			// Close ALL popups first (context menu, inst menu, color, FX, etc.)
			dv.CloseAllPopups()

			dv.renameRow = editIdx
			r := dv.rowLabels[editIdx].Rect()

			// Use RenameComponent if available
			if dv.renameComp != nil {
				mobileID := fmt.Sprintf("rename-%d", editIdx)
				dv.renameComp.SetProps(RenameProps{
					AnchorRect:    r,
					InitialText:   dv.Rows[editIdx].Name,
					MaxLen:        32,
					MobileInputID: mobileID,
					OnCommit: func(newName string) {
						name := strings.TrimSpace(newName)
						if name != "" && dv.renameRow >= 0 && dv.renameRow < len(dv.Rows) {
							if strings.ContainsAny(name, "/\\<>\x00") {
								dv.notifyError("Invalid characters in name")
								dv.renameBox = nil
								dv.renameRow = -1
								dv.renameHold = false
								return
							}
							oldID := dv.Rows[dv.renameRow].Instrument
							newID := strings.ToLower(name)
							dv.logger.Infof("[DRUMVIEW] Rename instrument row=%d %q -> %q", dv.renameRow, oldID, newID)
							audio.RenameInstrument(oldID, newID)
							if dv.samplePath != nil {
								if p, ok := dv.samplePath[oldID]; ok {
									dv.samplePath[newID] = p
									delete(dv.samplePath, oldID)
								}
							}
							dv.Rows[dv.renameRow].Instrument = newID
							dv.Rows[dv.renameRow].Name = name
							dv.rowLabels[dv.renameRow].Text = name
							customColors[newID] = dv.Rows[dv.renameRow].Color
							dv.invalidateLabelCaches()
							dv.refreshInstruments()
							dv.markRowControlsDirty()
							dv.bgDirty = true
							dv.notifyInfo("Renamed instrument to: " + name)
						}
						// Clear legacy state
						dv.renameBox = nil
						dv.renameRow = -1
						dv.renameHold = false
					},
					OnCancel: func() {
						// Clear legacy state
						dv.renameBox = nil
						dv.renameRow = -1
						dv.renameHold = false
					},
				})
				dv.renameComp.Open()
				// Share the same TextInput so legacy renameBox access also
				// sees the same text (important for tests and legacy update path).
				if tb := dv.renameComp.TextBox(); tb != nil {
					dv.renameBox = tb
				}
			}

			if dv.renameBox == nil {
				// Legacy fallback when renameComp is unavailable
				dv.renameBox = NewTextInput(r, BPMBoxStyle)
				dv.renameBox.MaxLen = 32
				dv.renameBox.SetText(dv.Rows[editIdx].Name)
				dv.renameBox.focused = true
				dv.renameBox.anim = 1
			}
			dv.renameHold = true
			SuppressClicksUntilMouseUp()
		}
		save := NewButton("", InstButtonStyle, nil)
		save.Icon = "save"
		saveIdx := i
		save.OnClick = func() {
			dv.saveInstrument(saveIdx)
		}
		// Color swatch button. Use an immediate function to bind the index.
		swatch := func(idx int) *Button {
			colorFn := func() color.Color {
				if idx >= 0 && idx < len(dv.Rows) {
					return dv.Rows[idx].Color
				}
				return color.RGBA{200, 200, 200, 255}
			}
			b := NewButton("", ColorSwatchStyle{Color: colorFn, Border: colButtonBorder}, nil)
			b.OnClick = func() {
				dv.selRow = idx

				// Check if component is already open for this row (toggle)
				if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() && dv.colorMenuRow == idx {
					dv.colorWheelComp.Close()
					dv.colorMenuOpen = false
					dv.logger.Debugf("[COLOR] row=%d: toggle close", idx)
					return
				}

				// Close other overlays
				dv.instMenuOpen = false
				if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
					dv.instMenuComp.Close()
				}

				dv.colorMenuRow = idx

				// Use ColorWheelComponent if available
				if dv.colorWheelComp != nil {
					dv.colorWheelComp.SetProps(ColorWheelProps{
						AnchorRect: dv.rowColorBtns[idx].Rect(),
						Bounds:     dv.Bounds,
						RowHeight:  dv.rowHeight(),
						OnColorPick: func(c color.Color) {
							dv.SetRowColor(dv.colorMenuRow, c)
							dv.logger.Debugf("[COLOR] pick row=%d sel=%s", dv.colorMenuRow, dv.colorKey(c))
						},
						OnClose: func() {
							// Sync legacy state
							dv.colorMenuOpen = false
							dv.colorHold = false
						},
					})
					dv.colorWheelComp.Open()
					dv.logger.Debugf("[COLOR] row=%d: open requested via component", idx)
				}

				// Keep legacy state in sync
				dv.colorMenuOpen = true
				dv.buildColorMenu()
				dv.colorHold = true
				SuppressClicksUntilMouseUp()
			}
			return b
		}(i)
		slider := NewSlider(dv.Rows[i].Volume)
		mute := NewButton("M", InstButtonStyle, nil)
		solo := NewButton("S", InstButtonStyle, nil)
		origin := NewButton("O", InstButtonStyle, nil)
		del := NewButton("X", InstButtonStyle, nil)
		del.ConsumeOnPress = true
		delIdx := i
		if len(dv.Rows) > 1 {
			del.OnClick = func() {
				if dv.deleteConfirmRow == delIdx && (dv.frame-dv.deleteConfirmFrame) < 120 {
					dv.DeleteRow(delIdx)
					dv.deleteConfirmRow = -1
				} else {
					dv.deleteConfirmRow = delIdx
					dv.deleteConfirmFrame = dv.frame
				}
				dv.markRowControlsDirty()
			}
		} else {
			del.Style = DisabledButtonStyle
		}
		originIdx := i
		origin.OnClick = func() { dv.originReq = append(dv.originReq, originIdx) }
		muteIdx := i
		mute.OnClick = func() { dv.toggleMute(muteIdx) }
		soloIdx := i
		solo.OnClick = func() { dv.toggleSolo(soloIdx) }
		menu := NewButton("", InstButtonStyle, nil)
		menu.Icon = "overflow"
		menuIdx := i
		menu.OnClick = func() { dv.openContextMenu(menuIdx) }
		fx := NewButton("FX", InstButtonStyle, nil)
		fxIdx := i
		fx.OnClick = func() { dv.toggleFXPanel(fxIdx) }
		dv.rowLabels = append(dv.rowLabels, lbl)
		dv.rowEditBtns = append(dv.rowEditBtns, edit)
		dv.rowSaveBtns = append(dv.rowSaveBtns, save)
		dv.rowColorBtns = append(dv.rowColorBtns, swatch)
		dv.rowVolSliders = append(dv.rowVolSliders, slider)
		dv.rowMuteBtns = append(dv.rowMuteBtns, mute)
		dv.rowSoloBtns = append(dv.rowSoloBtns, solo)
		dv.rowOriginBtns = append(dv.rowOriginBtns, origin)
		dv.rowDeleteBtns = append(dv.rowDeleteBtns, del)
		dv.rowMenuBtns = append(dv.rowMenuBtns, menu)
		dv.rowFXBtns = append(dv.rowFXBtns, fx)
		dv.rowGroups = append(dv.rowGroups, RowButtonGroup{
			Mute: mute, Solo: solo, FX: fx, Origin: origin, Delete: del,
			Edit: edit, Save: save, Menu: menu, Label: lbl,
		})
		// Set all widget rects via the shared positioning method.
		dv.positionRowWidgets(i, rowRect)
	}
	// Position the "+" button after the last visible row (not after all rows).
	// visibleRows() already reserves one rowHeight for this footer.
	dv.positionAddRowBtn(rowsTop, panelRect, vis)
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
	pad := buttonPad*2 + 12
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

// calcEQChannelBtnWidth returns the width for the EQ channel selector button,
// sized to fit the longest channel name ("Master" or any row name) without truncation.
func (dv *DrumView) calcEQChannelBtnWidth() int {
	maxPx := TextWidth("Master")
	for _, r := range dv.Rows {
		if w := TextWidth(r.Name); w > maxPx {
			maxPx = w
		}
	}
	w := maxPx + buttonPad*2 + 4
	if w < 72 {
		w = 72
	}
	return w
}
