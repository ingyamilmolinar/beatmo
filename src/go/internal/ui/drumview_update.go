package ui

import (
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func (dv *DrumView) Update() {
	if len(dv.Rows) == 0 {
		return
	}

	dv.updateSeq++
	dv.refreshInstruments()

	// Global click-suppression guard resets on mouse release each frame
	if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
		suppressClicksUntilRelease = false
	}

	// Handle pending JSON import
	if dv.importing {
		select {
		case res := <-dv.importCh:
			dv.importing = false
			if dv.onImportDialogEnd != nil {
				dv.onImportDialogEnd()
			}
			if res.err != nil {
				dv.logger.Infof("[DRUMVIEW] Import failed: %v", res.err)
				dv.notifyError("Error loading JSON: " + res.err.Error())
			} else if len(res.data) == 0 {
				dv.logger.Infof("[DRUMVIEW] Import canceled (no data)")
			} else if dv.onImport != nil {
				// When wired to Game, onImport queues data for deferred processing
				// and returns nil; notifications are handled by game.Update().
				// For standalone use (tests with mock handlers), onImport may
				// return an error directly, which we handle here.
				if err := dv.onImport(res.data); err != nil {
					dv.logger.Infof("[DRUMVIEW] Import error: %v", err)
					dv.notifyError("Error loading JSON: " + err.Error())
				}
				// Note: success notifications are handled by Game.Update() after
				// the deferred import completes, not here.
			}
		default:
		}
		// Safety: if the browser cancel does not trigger a change event,
		// ensure we eventually release the guard.
		if int(dv.frame)-dv.importAttemptFrame > 600 || dv.updateSeq-dv.importAttemptUpdate > 600 { // ~10s at 60fps
			dv.importing = false
			if dv.onImportDialogEnd != nil {
				dv.onImportDialogEnd()
			}
			dv.notifyError("Import canceled")
		}
	}

	// Update sample loading status (WASM returns non-zero).
	if loaded, total := audio.SampleLoadProgress(); total > 0 {
		// When total becomes available, consider ourselves loading until done.
		dv.samplesTotal = total
		dv.samplesLoaded = loaded
		if loaded < total {
			dv.showLoading = true
			dv.doneMsgTimer = 0
		} else if dv.showLoading && loaded >= total {
			// Just finished.
			dv.showLoading = false
			dv.doneMsgTimer = 180 // ~3 seconds at 60fps
		}
	}

	if dv.uploading {
		select {
		case res := <-dv.uploadCh:
			dv.uploading = false
			dv.logger.Debugf("[DRUMVIEW] Upload result path=%s err=%v", res.path, res.err)
			if res.err != nil {
				dv.logger.Infof("[DRUMVIEW] Failed to load WAV: %v", res.err)
				dv.notifyError("Error loading WAV: " + res.err.Error())
			} else {
				dv.pendingWAV = res.path
				dv.naming = true
				dv.nameInput = ""
				// Initialize naming input box for consistent UX
				box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
				dv.nameBox = NewTextInput(box, BPMBoxStyle)
				dv.nameBox.MaxLen = 32
				dv.nameBox.SetText("")
				dv.nameBox.focused = true
				dv.notifyInfo("Selected WAV: " + res.path)
			}
		drainUpload:
			for {
				select {
				case extra := <-dv.uploadCh:
					dv.logger.Debugf("[DRUMVIEW] Dropping stale upload result path=%s err=%v", extra.path, extra.err)
				default:
					break drainUpload
				}
			}
		default:
		}
	}

	if dv.naming {
		// Keep rect in sync with layout and update input
		box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
		if dv.nameBox == nil {
			dv.nameBox = NewTextInput(box, BPMBoxStyle)
			dv.nameBox.MaxLen = 32
			dv.nameBox.focused = true
		}
		dv.nameBox.Rect = box
		if dv.saveBtn == nil {
			dv.saveBtn = NewButton("Save", UploadBtnStyle, nil)
		}
		dv.saveBtn.SetRect(image.Rect(box.Max.X+10, box.Min.Y, box.Max.X+60, box.Max.Y))
		dv.saveBtn.OnClick = func() {
			id := strings.TrimSpace(dv.nameBox.Value())
			dv.logger.Infof("[DRUMVIEW] Save instrument pressed id=%q", id)
			if id != "" {
				dv.registerInstrument(id)
			}
		}
		dv.nameBox.Update()
		if isKeyPressed(ebiten.KeyEnter) {
			id := strings.TrimSpace(dv.nameBox.Value())
			if id != "" {
				dv.registerInstrument(id)
			}
		}
		if isKeyPressed(ebiten.KeyEscape) {
			dv.naming = false
			dv.pendingWAV = ""
			dv.nameInput = ""
			dv.nameBox = nil
		}
		mx, my := cursorPosition()
		left := isMouseButtonPressed(ebiten.MouseButtonLeft)
		if dv.saveBtn.Handle(mx, my, left) {
			dv.saveAnim = 1
		} else if left && !pt(mx, my, dv.nameBox.Rect) {
			// Click outside cancels naming (same as Esc)
			dv.naming = false
			dv.pendingWAV = ""
			dv.nameInput = ""
			dv.nameBox = nil
		}
		dv.namePhase += 0.1
		return
	}

	// Handle rename via component if available
	if dv.renameComp != nil && dv.renameComp.IsOpen() {
		mx, my := cursorPosition()
		left := isMouseButtonPressed(ebiten.MouseButtonLeft)
		result := dv.renameComp.HandleInput(mx, my, left)
		// Sync legacy state
		if !dv.renameComp.IsOpen() {
			dv.renameBox = nil
		}
		if result != InputIgnored {
			return
		}
	} else if dv.renameBox != nil {
		// Legacy fallback path
		if dv.renameHold {
			if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
				dv.renameHold = false
			}
		} else {
			dv.renameBox.Update()
		}
		// Click outside cancels (same as Esc) once hold is released.
		mx, my := cursorPosition()
		left := isMouseButtonPressed(ebiten.MouseButtonLeft)
		if !dv.renameHold && left && !pt(mx, my, dv.renameBox.Rect) {
			dv.logger.Debugf("[DRUMVIEW/RENAME] canceled by outside click (row=%d)", dv.renameRow)
			dv.renameBox = nil
			dv.renameRow = -1
			return
		}
		if !dv.renameHold && isKeyPressed(ebiten.KeyEnter) {
			name := strings.TrimSpace(dv.renameBox.Value())
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
				// Invalidate label caches since row/instrument name changed.
				dv.invalidateLabelCaches()
				dv.refreshInstruments()
				dv.notifyInfo("Renamed instrument to: " + name)
			}
			dv.renameBox = nil
			dv.renameRow = -1
		}
		if !dv.renameHold && isKeyPressed(ebiten.KeyEscape) {
			dv.logger.Infof("[DRUMVIEW] Rename instrument canceled (row=%d)", dv.renameRow)
			dv.renameBox = nil
			dv.renameRow = -1
		}
		return
	}

	if dv.instHold {
		if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
			dv.instHold = false
		}
		return
	}

	if !dv.anyDropdownOpen() {
		if dv.layoutHandler != nil && dv.layoutHandler.Update() {
			return
		}
	}

	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}

	prevFocus := dv.bpmBox.Focused()

	// Early BPM text input handling so focus/blur on the BPM box is
	// registered even if other controls short-circuit later in Update.
	prevVal := dv.bpmBox.Value()
	dv.bpmBox.Update()
	// Enter key should commit immediately like other editors.
	if dv.bpmBox.Focused() && isKeyPressed(ebiten.KeyEnter) {
		txt := dv.bpmBox.Value()
		if txt == "" {
			prev := dv.bpmPrev
			if prev < 1 {
				prev = dv.bpm
			}
			dv.SetBPM(prev)
		} else if v, ok := parseBPM(txt); ok {
			dv.SetBPM(v)
		} else {
			dv.bpmErrorAnim = 1
			dv.notifyError("Invalid BPM")
			prev := dv.bpmPrev
			if prev < 1 {
				prev = dv.bpm
			}
			dv.SetBPM(prev)
		}
		dv.bpmBox.SetText(strconv.Itoa(dv.bpm))
		dv.bpmBox.focused = false
	}
	if !prevFocus && dv.bpmBox.Focused() {
		dv.logger.Debugf("[DRUMVIEW] BPM box focused")
		dv.bpmPrev = dv.bpm
		dv.bpmBox.SetText("")
	}
	if prevFocus && !dv.bpmBox.Focused() {
		dv.logger.Debugf("[DRUMVIEW] BPM box blurred value=%q", prevVal)
		// Commit immediately on blur (e.g., Enter pressed) so BPM updates without waiting
		// for later handlers in this frame.
		txt := dv.bpmBox.Value()
		if txt == "" {
			prev := dv.bpmPrev
			if prev < 1 {
				prev = dv.bpm
			}
			dv.SetBPM(prev)
		} else if v, ok := parseBPM(txt); ok {
			dv.SetBPM(v)
		} else {
			dv.bpmErrorAnim = 1
			dv.notifyError("Invalid BPM")
			prev := dv.bpmPrev
			if prev < 1 {
				prev = dv.bpm
			}
			dv.SetBPM(prev)
		}
		dv.bpmBox.SetText(strconv.Itoa(dv.bpm))
	}

	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)

	// Read wheel delta once at start of frame to ensure consistent handling.
	// This value will be used by overlay dispatch and fallback handlers.
	wheelSteps := wheelScrollSteps()
	wheelConsumed := false

	// ─── WHEEL EVENT DISPATCH (overlays first) ───
	// Dispatch wheel to overlays before popup input handlers. This ensures
	// dropdown menus receive wheel events for scrolling instead of the
	// underlying drum rows.
	if wheelSteps != 0 && dv.overlays != nil {
		if dv.overlays.HandleWheel(mx, my, wheelSteps) != InputIgnored {
			wheelConsumed = true
		}
	}

	// ─── POPUP INPUT PRECEDENCE ───
	// Handle all open popup menus FIRST - they have input priority over underlying
	// elements. If a popup consumes the input, return immediately to prevent
	// underlying handlers from processing the same events.

	// Instrument menu - delegate to component if available
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
		// Allow the Upload button to be clicked even when the menu is open
		if left && dv.uploadBtn != nil && image.Pt(mx, my).In(dv.uploadBtn.Rect()) {
			dv.instMenuComp.Close()
			dv.instMenuOpen = false
			dv.syncInstMenuBtnsFromComp()
			_ = dv.uploadBtn.Handle(mx, my, left)
			return
		}
		result := dv.instMenuComp.HandleInput(mx, my, left)
		// Keep legacy state in sync
		dv.instMenuOpen = dv.instMenuComp.IsOpen()
		dv.syncInstMenuBtnsFromComp()
		if result != InputIgnored {
			return
		}
	} else if dv.instMenuOpen {
		if handled := dv.handleInstMenuInput(mx, my, left); handled {
			return
		}
	}

	// EQ channel menu
	if dv.eqChannelOpen {
		if handled := dv.handleEQChannelMenuInput(mx, my, left); handled {
			return
		}
	}

	// Color menu - delegate to component if available
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
		// Handle Escape to close
		if isKeyPressed(ebiten.KeyEscape) {
			dv.logger.Debugf("[COLOR] close by Esc")
			dv.colorWheelComp.Close()
			dv.colorMenuOpen = false
			return
		}
		result := dv.colorWheelComp.HandleInput(mx, my, left)
		// Sync legacy state
		dv.colorMenuOpen = dv.colorWheelComp.IsOpen()
		if result != InputIgnored {
			return
		}
	} else if dv.colorMenuOpen {
		if handled := dv.handleColorMenuInput(mx, my, left); handled {
			return
		}
	}

	// Subdivision menu - delegate to component if available
	if dv.subdivMenuComp != nil && dv.subdivMenuComp.IsOpen() {
		result := dv.subdivMenuComp.HandleInput(mx, my, left)
		// Keep legacy state in sync
		dv.subdivMenuOpen = dv.subdivMenuComp.IsOpen()
		if result != InputIgnored {
			return
		}
	} else if dv.subdivMenuOpen {
		if handled := dv.handleSubdivMenuInput(mx, my, left); handled {
			return
		}
	}

	// ─── ROW SCROLLING (after popups) ───
	totalRows := len(dv.Rows) + 1
	visRows := dv.visibleRows()
	if totalRows > visRows {
		// Only consume wheel for row scrolling when the cursor is over
		// the drum pane (including the scrollbar). This prevents wheel
		// events from being eaten while the cursor is over the grid pane.
		// Skip if wheel was already consumed by an overlay.
		overBar := image.Pt(mx, my).In(dv.scrollBarRect())
		overDrum := image.Pt(mx, my).In(dv.Bounds)
		if !wheelConsumed && (overBar || overDrum) && wheelSteps != 0 {
			dv.logger.Infof("[DRUMVIEW] row wheel steps=%d at (%d,%d) rowOffset=%d", wheelSteps, mx, my, dv.rowOffset)
			dv.rowOffset -= wheelSteps
			if dv.rowOffset < 0 {
				dv.rowOffset = 0
			}
			if dv.rowOffset > totalRows-visRows {
				dv.rowOffset = totalRows - visRows
			}
			dv.calcLayout()
			wheelConsumed = true
		}
		bar := dv.scrollBarRect()
		thumb := dv.scrollThumbRect()
		if dv.scrollDrag {
			if left {
				track := bar.Dy() - thumb.Dy()
				if track > 0 {
					delta := my - dv.scrollStartY
					dv.rowOffset = dv.scrollStartOff + delta*totalRows/track
				} else {
					// Degenerate case: bar height <= thumb height (tiny viewports).
					// Map cursor position directly across the available offset range.
					if bar.Dy() > 0 && totalRows > visRows {
						rel := my - bar.Min.Y
						if rel < 0 {
							rel = 0
						}
						if rel > bar.Dy() {
							rel = bar.Dy()
						}
						dv.rowOffset = rel * (totalRows - visRows) / bar.Dy()
					} else if my > dv.scrollStartY {
						dv.rowOffset = totalRows - visRows
					} else if my < dv.scrollStartY {
						dv.rowOffset = 0
					} else {
						dv.rowOffset = dv.scrollStartOff
					}
				}
				if dv.rowOffset < 0 {
					dv.rowOffset = 0
				}
				if dv.rowOffset > totalRows-visRows {
					dv.rowOffset = totalRows - visRows
				}
				dv.calcLayout()
			} else {
				dv.scrollDrag = false
			}
		} else if left && image.Pt(mx, my).In(thumb) {
			dv.scrollDrag = true
			dv.scrollStartY = my
			dv.scrollStartOff = dv.rowOffset
		}
	}

	tlRect := dv.widgetRects[WidgetTimeline]
	if tlRect.Empty() {
		tlRect = image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Max.X, dv.Bounds.Max.Y-dv.eqH)
	}
	stepsRect := image.Rect(tlRect.Min.X, dv.Bounds.Min.Y+dv.headerH, tlRect.Max.X, dv.Bounds.Max.Y-dv.eqH)
	panelRect := dv.widgetRects[WidgetRack]
	if panelRect.Empty() {
		panelRect = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Max.Y-dv.eqH)
	}

	// wheel zoom for length adjustment. Apply smoothing so each wheel notch
	// accumulates a fraction of a beat and only commits whole-beat changes
	// when enough deltas have been collected. This avoids abrupt jumps.
	// IMPORTANT: Only read the wheel delta when the cursor is over the steps
	// Wheel scrolling: only scroll rows; zooming is handled by +/- buttons.
	// Skip if wheel was already consumed by an overlay or row scroll.
	if !wheelConsumed && (pt(mx, my, stepsRect) || pt(mx, my, panelRect)) {
		totalRows := len(dv.Rows) + 1
		visRows := dv.visibleRows()
		// When vertical scrolling is possible, row scroll is preferred above.
		// When no row scroll is needed, use wheel for zoom.
		if totalRows <= visRows && wheelSteps != 0 {
			// Zoom in gently (¼ beat per notch) but zoom out aggressively (1 beat
			// per notch) so users can quickly reach the minimum view. The
			// accumulator smooths fine-grained scroll wheels.
			notchFrac := 0.25
			if wheelSteps < 0 {
				notchFrac = 1.0
			}
			dv.zoomAccum += float64(wheelSteps) * notchFrac
			beatsDelta := 0
			for dv.zoomAccum >= 1 {
				beatsDelta++
				dv.zoomAccum -= 1
			}
			for dv.zoomAccum <= -1 {
				beatsDelta--
				dv.zoomAccum += 1
			}
			if beatsDelta != 0 {
				inc := max1(dv.timelineUnitsPerBeat)
				dv.changeLength(dv.Length + beatsDelta*inc)
			}
		}
	}

	/* ——— widget clicks & dragging ——— */
	if dv.activeSlider >= 0 {
		s := dv.rowVolSliders[dv.activeSlider]
		if s.Handle(mx, my, left) {
			dv.Rows[dv.activeSlider].Volume = math.Round(s.Value*100) / 100
			dv.logger.Infof("[DRUMVIEW] Row %d volume changed via slider: %.3f", dv.activeSlider, s.Value)
			dv.markRowControlsDirty()
		}
		if !left {
			dv.activeSlider = -1
		}
		return
	}
	for i, s := range dv.rowVolSliders {
		if s.Handle(mx, my, left) {
			dv.Rows[i].Volume = math.Round(s.Value*100) / 100
			dv.logger.Infof("[DRUMVIEW] Row %d volume changed via slider: %.3f", i, s.Value)
			dv.markRowControlsDirty()
			dv.activeSlider = i
			if !left {
				dv.activeSlider = -1
			}
			return
		}
	}

	handled := false
	if dv.mainVolSlider != nil {
		if dv.mainVolSlider.Handle(mx, my, left) {
			audio.SetMainVolume(dv.mainVolSlider.Value)
			handled = true
		}
	}
	for i, s := range dv.eqSliders {
		if s == nil {
			continue
		}
		if s.Handle(mx, my, left) {
			handled = true
			// Map slider (0..1) -> gain dB:
			// Standard linear: 0% = -12 dB, 50% = 0 dB, 100% = +12 dB
			gain := sliderToGainDB(s.Value)

			// Apply to the active channel
			ch := dv.activeEQChannel()
			if ch == "main" {
				dv.eqBandGainsDB[i] = gain
				dv.applyMasterEQ()
			} else {
				// Find the row with this instrument and apply per-row EQ
				for j, r := range dv.Rows {
					if r.Instrument == ch {
						dv.ensureRowEQ(j)
						r.EQGainsDB[i] = gain
						dv.applyRowEQ(j)
						break
					}
				}
			}
			if !left {
				// allow other widgets after release
			}
			return
		}
	}
	// Handle EQ band mute button clicks.
	// Toggle logic is handled via OnClick callback (set in drumview_layout.go).
	// We still call Handle() to update button visual state (pressed/hover).
	for _, btn := range dv.eqMuteBtns {
		if btn == nil {
			continue
		}
		if btn.Handle(mx, my, left) {
			handled = true
		}
	}
	if dv.eqChannelBtn != nil && dv.eqChannelBtn.Handle(mx, my, left) {
		handled = true
	}
	if dv.eqToggleBtn != nil && dv.eqToggleBtn.Handle(mx, my, left) {
		handled = true
	}
	if !dv.dragging {
		for _, btn := range dv.rowOriginBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowDeleteBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowMuteBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowSoloBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowEditBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowSaveBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, lbl := range dv.rowLabels {
			if lbl.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowColorBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		if handled && left {
			return
		}
		buttons := []*Button{dv.playBtn, dv.stopBtn, dv.bpmDecBtn, dv.bpmIncBtn, dv.subdivBtn, dv.lenDecBtn, dv.lenIncBtn, dv.trackBtn, dv.addRowBtn, dv.uploadBtn, dv.importBtn, dv.exportBtn}
		for _, btn := range buttons {
			if handled {
				break
			}
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
	}

	// Single-pass BPM handling already performed above. Apply any +/- delta.
	if dv.bpmDelta != 0 {
		dv.SetBPM(dv.bpm + dv.bpmDelta)
		dv.bpmDelta = 0
	}

	if left {
		if !dv.dragging {
			if pt(mx, my, stepsRect) {
				dv.dragging = true
				dv.dragStartX = mx
				dv.startOffset = dv.Offset
			}
		}
	} else {
		dv.dragging = false
	}

	if dv.dragging {
		delta := (dv.dragStartX - mx) / dv.cell
		newOffset := dv.startOffset + delta
		if newOffset < 0 {
			newOffset = 0
		}
		if newOffset != dv.Offset {
			dv.Offset = newOffset
			dv.offsetChanged = true
			dv.logger.Tracef("[DRUMVIEW/DRAG] offset=%d", dv.Offset)
		}
	}

	// timeline scrubbing (map click proportionally to [0..maxOffset])
	if left && pt(mx, my, dv.timelineRect) {
		dv.scrubbing = true
	}
	if dv.scrubbing {
		pos := mx
		if pos < dv.timelineRect.Min.X {
			pos = dv.timelineRect.Min.X
		}
		if pos > dv.timelineRect.Max.X {
			pos = dv.timelineRect.Max.X
		}
		frac := float64(pos-dv.timelineRect.Min.X) / float64(dv.timelineRect.Dx())
		unitsPerBeat := max1(dv.timelineUnitsPerBeat)
		// Map desired offset in beats, then convert to subdivision steps.
		lengthBeats := float64(dv.Length) / float64(unitsPerBeat)
		maxOffBeats := float64(dv.timelineBeats) - lengthBeats
		if maxOffBeats < 0 {
			maxOffBeats = 0
		}
		desiredBeats := frac * maxOffBeats
		desiredSteps := int(math.Round(desiredBeats * float64(unitsPerBeat)))
		maxOffSteps := int(math.Round(maxOffBeats * float64(unitsPerBeat)))
		if desiredSteps < 0 {
			desiredSteps = 0
		}
		if desiredSteps > maxOffSteps {
			desiredSteps = maxOffSteps
		}
		if desiredSteps != dv.Offset {
			dv.Offset = desiredSteps
			dv.offsetChanged = true
			dv.logger.Tracef("[DRUMVIEW/SCRUB] offset=%d len=%d total=%d", dv.Offset, dv.Length, dv.timelineBeats)
		}
		if !left {
			dv.scrubbing = false
		}
	}

	// (moved BPM text input handling earlier)

	/* ——— Length editing ——— */
	if dv.lenIncPressed {
		inc := max1(dv.timelineUnitsPerBeat)
		dv.changeLength(dv.Length + inc)
		dv.lenIncPressed = false
	}
	if dv.lenDecPressed {
		inc := max1(dv.timelineUnitsPerBeat)
		dv.changeLength(dv.Length - inc)
		dv.lenDecPressed = false
	}
	if handled && left {
		return
	}

	// Color menu handled above; nothing here
}
