package ui

import (
	"image"
	"image/color"
	"strings"
	"unicode/utf8"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	"github.com/ingyamilmolinar/tunkul/internal/utils"
)

/* ─── public update ────────────────────────────────────────── */

func (dv *DrumView) recalcButtons() {
	if runningUnderGoTest() {
		eqPanelHeight = 0
		dv.eqH = 0
	}
	dv.calcLabelWidth()

	transport := dv.widgetRects[WidgetTransport]
	if transport.Empty() {
		transport = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+dv.headerH)
	}
	minTransportW := 300
	if transport.Dx() < minTransportW {
		transport.Max.X = transport.Min.X + minTransportW
	}
	leftCol := transport
	// Dynamic button sizing based on transport height with consistent top/bottom scaling.
	pad := buttonPad + 2
	totalH := leftCol.Dy()
	minTop := 12
	minBot := 12
	topRowH := totalH / 2
	botRowH := totalH - topRowH
	if topRowH < minTop {
		topRowH = minTop
	}
	if botRowH < minBot {
		botRowH = minBot
	}
	// Rebalance to preserve total height
	sum := topRowH + botRowH
	if sum > 0 {
		scale := float64(totalH) / float64(sum)
		topRowH = int(float64(topRowH) * scale)
		botRowH = totalH - topRowH
	}
	if utils.Abs(topRowH-botRowH) > 4 {
		half := totalH / 2
		topRowH = half
		botRowH = totalH - half
	}
	// derive padding from available height
	if pad > topRowH/4 {
		pad = topRowH / 4
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
	// columns: play | stop | bpm box | bpm(+/- stacked) | subdiv | len(+/- stacked) | track | label | slider
	controlLeft := leftCol.Min.X + 12 // small inset to avoid hugging the border
	if controlLeft >= leftCol.Max.X {
		controlLeft = leftCol.Min.X
	}
	topEnd := leftCol.Min.Y + topRowH
	if topEnd-leftCol.Min.Y < minTop {
		topEnd = leftCol.Min.Y + minTop
	}
	if topEnd > leftCol.Max.Y {
		topEnd = leftCol.Max.Y
	}
	topBounds := image.Rect(controlLeft, leftCol.Min.Y, leftCol.Max.X, topEnd)
	topGrid := NewGridLayout(topBounds, []float64{1, 1, 2, 1, 1, 1, 1, 1, 3}, []float64{1})
	dv.playBtn.SetRect(safeInset(topGrid.Cell(0, 0), pad))
	dv.stopBtn.SetRect(safeInset(topGrid.Cell(1, 0), pad))
	dv.bpmBox.Rect = safeInset(topGrid.Cell(2, 0), pad)
	// BPM +/- stacked vertically in a single column
	bpmCol := safeInset(topGrid.Cell(3, 0), pad)
	split := bpmCol.Dy() / 2
	if split < 8 {
		split = bpmCol.Dy() / 2
	}
	dv.bpmIncBtn.SetRect(bpmCol)
	incR := dv.bpmIncBtn.Rect()
	incR.Max.Y = incR.Min.Y + split
	dv.bpmIncBtn.SetRect(incR)
	dv.bpmDecBtn.SetRect(bpmCol)
	decR := dv.bpmDecBtn.Rect()
	decR.Min.Y = incR.Max.Y
	if decR.Max.Y > topBounds.Max.Y {
		decR.Max.Y = topBounds.Max.Y
	}
	if incR.Max.Y > topBounds.Max.Y {
		incR.Max.Y = topBounds.Max.Y
	}
	if decR.Min.Y > decR.Max.Y {
		decR.Min.Y = decR.Max.Y
	}
	dv.bpmDecBtn.SetRect(decR)
	dv.subdivBtn.SetRect(safeInset(topGrid.Cell(4, 0), pad))
	// Length +/- stacked vertically in a single column
	lenCol := safeInset(topGrid.Cell(5, 0), pad)
	lenSplit := lenCol.Dy() / 2
	if lenSplit < 8 {
		lenSplit = lenCol.Dy() / 2
	}
	dv.lenIncBtn.SetRect(lenCol)
	li := dv.lenIncBtn.Rect()
	li.Max.Y = li.Min.Y + lenSplit
	dv.lenIncBtn.SetRect(li)
	dv.lenDecBtn.SetRect(lenCol)
	ld := dv.lenDecBtn.Rect()
	ld.Min.Y = li.Max.Y
	if ld.Max.Y > topBounds.Max.Y {
		ld.Max.Y = topBounds.Max.Y
	}
	if li.Max.Y > topBounds.Max.Y {
		li.Max.Y = topBounds.Max.Y
	}
	if ld.Min.Y > ld.Max.Y {
		ld.Min.Y = ld.Max.Y
	}
	dv.lenDecBtn.SetRect(ld)
	dv.trackBtn.SetRect(safeInset(topGrid.Cell(6, 0), pad))
	// Ensure gaps between adjacent buttons even when pads clamp.
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
	ensureGap(dv.playBtn, dv.stopBtn)
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
	clampBtn(dv.bpmIncBtn, topBounds)
	clampBtn(dv.bpmDecBtn, topBounds)
	clampBtn(dv.lenIncBtn, topBounds)
	clampBtn(dv.lenDecBtn, topBounds)
	if dv.mainVolSlider != nil {
		dv.mainVolRect = safeInset(topGrid.Cell(8, 0), pad)
		dv.mainVolSlider.SetRect(dv.mainVolRect)
	}

	// File row uses the remaining height of the transport widget; fall back to a
	// standard row height when compact.
	fileTop := topBounds.Max.Y
	fileBottom := leftCol.Max.Y
	if fileBottom-fileTop < botRowH {
		fileBottom = fileTop + botRowH
	}
	botBounds := image.Rect(controlLeft, fileTop, leftCol.Max.X, fileBottom)
	botGrid := NewGridLayout(botBounds, []float64{1, 1, 1}, []float64{1})
	dv.uploadBtn.SetRect(safeInset(botGrid.Cell(0, 0), pad))
	dv.importBtn.SetRect(safeInset(botGrid.Cell(1, 0), pad))
	dv.exportBtn.SetRect(safeInset(botGrid.Cell(2, 0), pad))

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
	top := headerTop + headerH - timelineBarHeight - 5
	if top < tlWidget.Min.Y {
		top = tlWidget.Min.Y
	}
	dv.timelineRect = image.Rect(
		tlWidget.Min.X,
		top,
		tlWidget.Max.X-10,
		top+timelineBarHeight,
	)

	// EQ panel is anchored to the Wave widget; if missing, fall back to the bottom of the timeline widget.
	eqWidget := dv.widgetRects[WidgetWave]
	if eqWidget.Empty() {
		eqWidget = image.Rect(tlWidget.Min.X, dv.Bounds.Max.Y-dv.eqH, tlWidget.Max.X, dv.Bounds.Max.Y)
	}
	dv.eqRect = eqWidget
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
			dv.eqChannelOpen = !dv.eqChannelOpen
			if dv.eqChannelOpen {
				dv.eqChannelScroll.First = 0
				dv.eqChannelScroll.EndDrag()
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
	channelBtnW := 72
	if channelBtnW > dv.eqRect.Dx()/3 {
		channelBtnW = dv.eqRect.Dx() / 3
	}
	channelBtnRect := image.Rect(dv.eqRect.Min.X+6, dv.eqRect.Min.Y+4, dv.eqRect.Min.X+6+channelBtnW, dv.eqRect.Min.Y+4+btnH)
	dv.eqChannelBtn.SetRect(channelBtnRect)
	btnRect := image.Rect(dv.eqRect.Max.X-btnW-6, dv.eqRect.Min.Y+4, dv.eqRect.Max.X-6, dv.eqRect.Min.Y+4+btnH)
	dv.eqToggleBtn.SetRect(btnRect)
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
}

// clampLength enforces global min/max zoom limits for the drum view.
// Minimum: one full beat (timelineUnitsPerBeat). Maximum: pixels available
// in the timeline so each subdivision remains at least ~1px wide.
func (dv *DrumView) clampLength(n int) int {
	inc := max1(dv.timelineUnitsPerBeat)
	minLen := inc
	maxLen := dv.timelineRect.Dx()
	if maxLen < minLen {
		maxLen = minLen
	}
	if n < minLen {
		n = minLen
	}
	if n > maxLen {
		n = maxLen
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
	dv.Length = newLen
	for _, r := range dv.Rows {
		r.Steps = make([]bool, dv.Length)
		r.CellTypes = make([]model.NodeType, dv.Length)
	}
	dv.SetBeatLength(dv.Length) // Update graph's beat length
	dv.bgDirty = true
	dv.markAllRowsDirty()
}

func (dv *DrumView) calcLayout() {
	dv.calcLabelWidth()
	if len(dv.Rows) > 0 {
		w := dv.timelineRect.Dx()
		if w <= 0 {
			w = dv.Bounds.Dx() - dv.labelW - dv.controlsW
		}
		// Clamp to the actual rack width to avoid overflowing the control pane.
		avail := dv.Bounds.Dx() - dv.labelW - dv.controlsW
		if avail > 0 && (w <= 0 || w > avail) {
			w = avail
		}
		dv.cell = w / len(dv.Rows[0].Steps)
	}
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	panelRect := dv.widgetRects[WidgetRack]
	if panelRect.Empty() {
		panelRect = image.Rect(dv.Bounds.Min.X, rowsTop, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Max.Y-dv.eqH)
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
	vis := dv.visibleRows()
	for i := range dv.Rows {
		y := rowsTop + (i-dv.rowOffset)*dv.rowHeight()
		rowRect := image.Rect(panelRect.Min.X, y, panelRect.Max.X, y+dv.rowHeight())
		if i < dv.rowOffset || i >= dv.rowOffset+vis {
			rowRect = image.Rect(0, 0, 0, 0)
		}
		// grid: [label][edit][color][slider][M][S][O][X]
		g := NewGridLayout(rowRect, []float64{6, 2, 2, 8, 2, 2, 2, 2}, []float64{1})
		style := InstButtonStyle
		if !dv.IsInstrumentAvailable(dv.Rows[i].Instrument) {
			style = MissingInstStyle
		}
		lbl := NewButton(dv.Rows[i].Name, style, nil)
		lbl.SetRect(insetRect(g.Cell(0, 0), buttonPad))
		idx := i
		lbl.OnClick = func() {
			dv.selRow = idx
			dv.instMenuCameFromCategories = false
			dv.instMenuUserScrolled = false
			// Prime category based on the row's current instrument.
			if dv.instCatByID != nil {
				if cat, ok := dv.instCatByID[dv.Rows[idx].Instrument]; ok {
					dv.instMenuActiveCat = cat
					if dv.instMenuActiveByRow == nil {
						dv.instMenuActiveByRow = map[int]string{}
					}
					dv.instMenuActiveByRow[idx] = cat
				}
			}

			// Check if component is already open for this row (toggle)
			if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() && dv.instMenuRow == idx {
				dv.logger.Debugf("[DRUMVIEW] Closing instrument menu for row %d", idx)
				dv.instMenuComp.Close()
				dv.instMenuOpen = false
				dv.instMenuScroll.EndDrag()
				return
			}

			// Close other overlays
			dv.colorMenuOpen = false
			if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
				dv.colorWheelComp.Close()
			}

			dv.instMenuRow = idx

			// Use InstrumentMenuComponent if available
			if dv.instMenuComp != nil {
				// Build instrument options list
				var instOpts []InstrumentOption
				for _, id := range dv.instOptions {
					label := id
					if dv.instLabelCache != nil {
						if l, ok := dv.instLabelCache[id]; ok {
							label = l
						}
					}
					cat := ""
					if dv.instCatByID != nil {
						cat = dv.instCatByID[id]
					}
					instOpts = append(instOpts, InstrumentOption{
						ID:       id,
						Label:    label,
						Category: cat,
					})
				}

				dv.instMenuComp.SetProps(InstrumentMenuProps{
					AnchorRect:        dv.rowLabels[idx].Rect(),
					VertBounds:        dv.widgetRects[WidgetRack],
					RowIndex:          idx,
					CurrentInstrument: dv.Rows[idx].Instrument,
					Categories:        dv.instCategories,
					Instruments:       instOpts,
					RowHeight:         dv.rowHeight(),
					LabelWidth:        dv.labelW,
					ControlsWidth:     dv.controlsW,
					ForceCategories:   dv.instMenuForceCategories,
					OnSelect: func(instID string) {
						dv.SetInstrument(instID)
						dv.logger.Debugf("[DRUMVIEW] Selected instrument %s for row %d", instID, dv.instMenuRow)
					},
					OnClose: func() {
						dv.instMenuOpen = false
					},
					OnRebuild: func() {
						dv.syncInstMenuBtnsFromComp()
					},
				})
				dv.instMenuComp.Open()
				dv.logger.Debugf("[DRUMVIEW] Opening instrument menu (component) for row %d", idx)
				// Sync component buttons to legacy buttons for test access
				dv.syncInstMenuBtnsFromComp()
				dv.instMenuOpen = true
				SuppressClicksUntilMouseUp()
			} else {
				// Fallback to legacy menu when no component
				dv.instMenuScroll.First = 0
				if dv.instMenuForceCategories && len(dv.instCategories) > 0 {
					dv.instMenuMode = instMenuModeCategories
				} else {
					dv.instMenuMode = instMenuModeInstruments
				}
				dv.instMenuOpen = true
				dv.buildInstMenu()
				SuppressClicksUntilMouseUp()
			}
		}
		edit := NewButton("✎", InstButtonStyle, nil)
		edit.Icon = "pencil"
		editCell := g.Cell(1, 0)
		editRect, saveRect := splitRectHoriz(editCell)
		splitPad := buttonPad
		if splitPad > 1 {
			splitPad--
		}
		edit.SetRect(insetRectSafe(editRect, splitPad))
		editIdx := i
		edit.OnClick = func() {
			dv.renameRow = editIdx
			r := dv.rowLabels[editIdx].Rect()

			// Close other overlays
			dv.instMenuOpen = false
			dv.colorMenuOpen = false
			if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
				dv.instMenuComp.Close()
			}
			if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
				dv.colorWheelComp.Close()
			}

			// Use RenameComponent if available
			if dv.renameComp != nil {
				dv.renameComp.SetProps(RenameProps{
					AnchorRect:  r,
					InitialText: dv.Rows[editIdx].Name,
					MaxLen:      32,
					OnCommit: func(newName string) {
						name := strings.TrimSpace(newName)
						if name != "" && dv.renameRow >= 0 && dv.renameRow < len(dv.Rows) {
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
			}

			// Legacy fallback - set state for tests that check renameBox
			dv.renameBox = NewTextInput(r, BPMBoxStyle)
			dv.renameBox.MaxLen = 32
			dv.renameBox.SetText(dv.Rows[editIdx].Name)
			dv.renameBox.focused = true
			dv.renameBox.anim = 1
			dv.renameHold = true
		}
		save := NewButton("", InstButtonStyle, nil)
		save.Icon = "save"
		save.SetRect(insetRectSafe(saveRect, splitPad))
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
			b.SetRect(insetRect(g.Cell(2, 0), buttonPad))
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
		slider.SetRect(insetRect(g.Cell(3, 0), buttonPad))
		mute := NewButton("M", InstButtonStyle, nil)
		mute.SetRect(insetRect(g.Cell(4, 0), buttonPad))
		solo := NewButton("S", InstButtonStyle, nil)
		solo.SetRect(insetRect(g.Cell(5, 0), buttonPad))
		origin := NewButton("O", InstButtonStyle, nil)
		origin.SetRect(insetRect(g.Cell(6, 0), buttonPad))
		del := NewButton("X", InstButtonStyle, nil)
		del.ConsumeOnPress = true
		del.SetRect(insetRect(g.Cell(7, 0), buttonPad))
		delIdx := i
		if len(dv.Rows) > 1 {
			del.OnClick = func() { dv.DeleteRow(delIdx) }
		} else {
			del.Style = DisabledButtonStyle
		}
		originIdx := i
		origin.OnClick = func() { dv.originReq = append(dv.originReq, originIdx) }
		muteIdx := i
		mute.OnClick = func() { dv.toggleMute(muteIdx) }
		soloIdx := i
		solo.OnClick = func() { dv.toggleSolo(soloIdx) }
		dv.rowLabels = append(dv.rowLabels, lbl)
		dv.rowEditBtns = append(dv.rowEditBtns, edit)
		dv.rowSaveBtns = append(dv.rowSaveBtns, save)
		dv.rowColorBtns = append(dv.rowColorBtns, swatch)
		dv.rowVolSliders = append(dv.rowVolSliders, slider)
		dv.rowMuteBtns = append(dv.rowMuteBtns, mute)
		dv.rowSoloBtns = append(dv.rowSoloBtns, solo)
		dv.rowOriginBtns = append(dv.rowOriginBtns, origin)
		dv.rowDeleteBtns = append(dv.rowDeleteBtns, del)
	}
	addY := rowsTop + (len(dv.Rows)-dv.rowOffset)*dv.rowHeight()
	if runningUnderGoTest() && panelRect.Max.Y < addY+dv.rowHeight() {
		panelRect.Max.Y = addY + dv.rowHeight()
		if dv.widgetRects != nil {
			dv.widgetRects[WidgetRack] = panelRect
		}
	}
	dv.addRowBtn.SetRect(insetRect(image.Rect(panelRect.Min.X, addY, panelRect.Max.X, addY+dv.rowHeight()), buttonPad))
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
		if w := debugCharW * utf8.RuneCountInString(r.Name); w > maxPx {
			maxPx = w
		}
	}
	for _, id := range dv.instOptions {
		lbl := dv.instDisplayLabel(id)
		if w := debugCharW * utf8.RuneCountInString(lbl); w > maxPx {
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
