package ui

import (
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
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
				dv.CloseAllPopups() // close rename/menus before entering naming mode
				dv.pendingWAV = res.path
				dv.naming = true
				dv.nameInput = ""
				// Initialize naming input box for consistent UX
				dv.nameBox = NewTextInput(dv.nameBoxRect(), BPMBoxStyle)
				dv.nameBox.MaxLen = 32
				dv.nameBox.InputMode = "text"
				dv.nameBox.OnFocusGained = func() { softKeyboardShow("text") }
				dv.nameBox.OnFocusLost = func() { softKeyboardHide() }
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
		// Safety: if another overlay opened on top of naming, close naming gracefully.
		if dv.overlays != nil && dv.overlays.HasOpenExcept("naming") {
			dv.closeNaming()
		} else {
			// Poll mobile native input for WAV name
			if isSmallScreen() && mobileInputActive("wav-name") {
				if val, committed, ok := mobileInputPollResult("wav-name"); ok {
					if committed {
						id := strings.TrimSpace(val)
						if id != "" {
							dv.registerInstrument(id)
						} else {
							dv.naming = false
							dv.pendingWAV = ""
							dv.nameInput = ""
							dv.nameBox = nil
						}
					} else {
						dv.naming = false
						dv.pendingWAV = ""
						dv.nameInput = ""
						dv.nameBox = nil
					}
				}
				dv.namePhase += 0.1
				return // Skip normal naming input while mobile input active
			}

			// Keep rect in sync with layout and update input
			box := dv.nameBoxRect()
			if dv.nameBox == nil {
				dv.nameBox = NewTextInput(box, BPMBoxStyle)
				dv.nameBox.MaxLen = 32
				dv.nameBox.InputMode = "text"
				dv.nameBox.OnFocusGained = func() { softKeyboardShow("text") }
				dv.nameBox.OnFocusLost = func() { softKeyboardHide() }
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
	}

	// Handle rename via component if available
	if dv.renameComp != nil && dv.renameComp.IsOpen() {
		// Only process if rename is the topmost active overlay.
		if dv.overlays == nil || !dv.overlays.HasOpenExcept("rename") {
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
		}
	} else if dv.renameBox != nil {
		if dv.overlays != nil && dv.overlays.HasOpenExcept("rename") {
			// Another overlay is on top — skip rename keyboard processing.
		} else {
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
					dv.markRowControlsDirty()
					dv.bgDirty = true
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
	}

	if dv.instHold {
		if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
			dv.instHold = false
		}
		return
	}

	if !dv.anyDropdownOpen() && !dv.rowScroll.TouchActive() {
		if dv.layoutHandler != nil && dv.layoutHandler.Update() {
			return
		}
	}

	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}

	// Mobile native input for BPM box — poll result before normal handling
	mobileBPMActive := isSmallScreen() && mobileInputActive("bpm")
	if mobileBPMActive {
		if val, committed, ok := mobileInputPollResult("bpm"); ok {
			if committed && val != "" {
				if v, ok := parseBPM(val); ok {
					dv.SetBPM(v)
				} else {
					dv.bpmErrorAnim = 1
					dv.notifyError("Invalid BPM")
				}
			}
			dv.bpmBox.SetText(strconv.Itoa(dv.bpm))
			dv.bpmBox.focused = false
		}
	}

	if !mobileBPMActive && dv.anyDropdownOpen() {
		// A popup is open — force-blur the BPM box to prevent stale focus
		// and skip its Update entirely so it can't read mouse state.
		if dv.bpmBox.Focused() {
			dv.bpmBox.focused = false
		}
	} else if !mobileBPMActive {
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
	} // end of non-mobile BPM block

	// Per-frame updates for overlay components (momentum, touch continuation)
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
		dv.instMenuComp.Update()
	}
	if dv.eqChannelOpen && dv.eqChannelScroll != nil && dv.eqChannelScroll.HasMomentum() {
		if dv.eqChannelScroll.UpdateMomentum() {
			dv.buildEQChannelMenu()
		}
	}
	if dv.fxPanelOpen && dv.fxScrollTS.HasMomentum() {
		delta := dv.fxScrollTS.UpdateMomentum()
		if delta != 0 {
			dv.fxScrollOffsetPx -= int(delta)
			if dv.fxScrollOffsetPx < 0 {
				dv.fxScrollOffsetPx = 0
			}
			if dv.fxScrollOffsetPx > dv.fxScrollMaxPx {
				dv.fxScrollOffsetPx = dv.fxScrollMaxPx
			}
			dv.buildFXPanel()
		}
	}
	if dv.contextMenuOpen && dv.contextMenuScroll != nil && dv.contextMenuScroll.HasMomentum() {
		if dv.contextMenuScroll.UpdateMomentum() {
			dv.rebuildContextMenuButtons()
		}
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

	// Instrument menu
	if dv.isInstMenuOpen() || (dv.instMenuComp != nil && dv.instMenuComp.Capturing()) {
		if dv.instMenuComp != nil {
			// Allow the Upload button to be clicked even when the menu is open
			if left && dv.uploadBtn != nil && image.Pt(mx, my).In(dv.uploadBtn.Rect()) {
				dv.instMenuComp.Close()
				dv.instMenuOpen = false
				dv.syncInstMenuBtnsFromComp()
				_ = dv.uploadBtn.Handle(mx, my, left)
				return
			}
			result := dv.instMenuComp.HandleInput(mx, my, left)
			dv.instMenuOpen = dv.instMenuComp.IsOpen()
			dv.syncInstMenuBtnsFromComp()
			if result != InputIgnored {
				return
			}
		} else if isKeyPressed(ebiten.KeyEscape) {
			dv.instMenuOpen = false
			return
		}
	}

	// EQ channel menu
	if dv.eqChannelOpen {
		if isKeyPressed(ebiten.KeyEscape) {
			dv.eqChannelOpen = false
			dv.eqChDeferredTap.Cancel()
			return
		}
		if handled := dv.handleEQChannelMenuInput(mx, my, left); handled {
			return
		}
	}

	// Color menu — primary dispatch is via OverlayStack in HandleInput().
	// This fallback handles Escape key and direct Update()-only test paths.
	if dv.isColorMenuOpen() {
		if isKeyPressed(ebiten.KeyEscape) {
			dv.logger.Debugf("[COLOR] close by Esc")
			if dv.colorWheelComp != nil {
				dv.colorWheelComp.Close()
			}
			dv.colorMenuOpen = false
			return
		}
		if dv.colorWheelComp != nil {
			result := dv.colorWheelComp.HandleInput(mx, my, left)
			dv.colorMenuOpen = dv.colorWheelComp.IsOpen()
			dv.colorHold = dv.colorWheelComp.Capturing()
			if result != InputIgnored {
				return
			}
		}
	}

	// FX panel — primary dispatch is via OverlayStack in HandleInput() for
	// proper capture/DeferredTap routing on mobile. This fallback handles
	// Escape key and direct Update()-only test paths (no Game/InputDispatcher).
	if dv.fxPanelOpen || dv.fxPanelDeferredTap.Active() {
		if isKeyPressed(ebiten.KeyEscape) {
			dv.closeFXPanel()
			return
		}
		if dv.fxPanelOverlay.HandleInput(mx, my, left) != InputIgnored {
			return
		}
	}

	// Subdivision menu
	if dv.isSubdivMenuOpen() {
		if dv.subdivMenuComp != nil {
			if isKeyPressed(ebiten.KeyEscape) {
				dv.subdivMenuComp.Close()
				dv.subdivMenuOpen = false
				return
			}
			result := dv.subdivMenuComp.HandleInput(mx, my, left)
			dv.subdivMenuOpen = dv.subdivMenuComp.IsOpen()
			if result != InputIgnored {
				return
			}
		} else if isKeyPressed(ebiten.KeyEscape) {
			dv.subdivMenuOpen = false
			return
		}
	}

	// Mobile overlay menus (context, overflow) are dispatched via the
	// OverlayStack in HandleInput(). Only handle Escape here (keyboard
	// is position-independent and not routed through the overlay stack).
	if dv.overflowMenuOpen && isKeyPressed(ebiten.KeyEscape) {
		dv.closeOverflowMenu()
		return
	}
	if dv.contextMenuOpen && isKeyPressed(ebiten.KeyEscape) {
		dv.contextMenuOpen = false
		return
	}

	// Block all remaining Update() handlers (row scroll, widgets, buttons,
	// sliders, scrubbing) when any overlay/popup is open — or when an overlay
	// was just closed by the OverlayStack this frame (suppressClicksUntilRelease
	// is set by OverlayStack.Close and clears on mouse release). This prevents
	// click-through to elements underneath popups.
	if dv.contextMenuOpen || dv.overflowMenuOpen || dv.volPopup.IsOpen() || dv.eqPopup.IsOpen() ||
		dv.fxPanelOpen || dv.masterVolPopup.IsOpen() || dv.isSubdivMenuOpen() {
		return
	}
	if left && suppressClicksUntilRelease {
		return
	}

	mobileEQActive := isSmallScreen() && dv.mobileEQMode

	// ─── ROW SCROLLING (after popups) ───
	if !mobileEQActive {
		dv.syncRowScroll()
		totalRows := len(dv.Rows)
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
				if dv.rowScroll.HandleWheel(wheelSteps) {
					dv.flushRowScroll()
				}
			}

			// Scrollbar drag
			if dv.rowScroll.Dragging() {
				if left {
					if dv.rowScroll.HandleDragTo(my) {
						dv.flushRowScroll()
					}
				} else {
					dv.rowScroll.HandleDragEnd()
				}
			} else if left && !dv.inputCapturedExternally && image.Pt(mx, my).In(dv.scrollThumbRect()) {
				dv.rowScroll.HandleDragStart(my)
			}

			// ─── TOUCH SCROLL ───
			if !dv.rowScroll.Dragging() {
				rowsRect := image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Max.X, dv.Bounds.Max.Y-dv.eqH)
				if left && touchOverrideActive && !dv.inputCapturedExternally && !dv.rowScroll.TouchActive() && !isTouchTapInjecting() && image.Pt(mx, my).In(rowsRect) && !image.Pt(mx, my).In(dv.scrollBarRect()) {
					dv.rowScroll.HandleTouchBegin(mx, my)
				} else if dv.rowScroll.TouchActive() && left && !isTouchTapInjecting() {
					if dv.rowScroll.HandleTouchMove(mx, my) {
						dv.flushRowScroll()
					}
				} else if dv.rowScroll.TouchActive() && !left {
					dv.rowScroll.HandleTouchEnd()
				}
			}
		}

		// ─── TOUCH SCROLL MOMENTUM (runs every frame) ───
		if dv.rowScroll.HasMomentum() {
			if len(dv.Rows) > dv.visibleRows() {
				dv.syncRowScroll()
				if dv.rowScroll.UpdateMomentum() {
					dv.flushRowScroll()
				}
			} else {
				dv.rowScroll.ResetTouch()
			}
		}
	}

	tlRect := dv.widgetRects[WidgetTimeline]
	if tlRect.Empty() {
		tlRect = image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Max.X, dv.Bounds.Max.Y-dv.eqH)
	}
	stepsRect := image.Rect(tlRect.Min.X, dv.Bounds.Min.Y+dv.headerH, tlRect.Max.X, dv.Bounds.Max.Y-dv.eqH)

	// On mobile, suppress row buttons while the touch scroller is in its
	// dead zone (TouchActive but not yet ScrollingCommitted). A touch that
	// enters the rows area from the header would otherwise fire a button
	// on the first frame inside the button's expanded hit area, before the
	// scroll dead zone has a chance to commit. Legitimate taps still work
	// because the gesture detector emits GestureTap on touch-end, and
	// injectTouchTap re-synthesises a 2-frame press for the drum view.
	// The scroll begin condition above skips injected taps via
	// !isTouchTapInjecting(), so the re-synthesised press is never
	// recaptured as a new scroll gesture. The dead zone and touch-move
	// handler are also exempted during injection: the gesture detector has
	// already confirmed the touch is a tap (not a scroll), so the dead
	// zone's purpose is moot and the scroller should not consume the
	// injected press as a move event.
	touchDeadZone := isSmallScreen() && dv.rowScroll.TouchActive() && !dv.rowScroll.ScrollingCommitted() && !isTouchTapInjecting()

	/* ——— widget clicks & dragging ——— */

	// ─── SLIDER ISOLATION ───
	// When a slider group is actively being dragged, route input exclusively
	// to that group and block everything else until mouse release.
	if dv.activeSliderKind != sliderKindNone {
		if !left {
			// Mouse released — end the drag.
			switch dv.activeSliderKind {
			case sliderKindRowVol:
				if dv.activeSlider >= 0 && dv.activeSlider < len(dv.rowVolSliders) {
					dv.rowVolSliders[dv.activeSlider].Handle(mx, my, false)
				}
				dv.activeSlider = -1
			case sliderKindMainVol:
				if dv.mainVolSlider != nil {
					dv.mainVolSlider.Handle(mx, my, false)
				}
			case sliderKindEQ:
				// Let each slider see the release so it clears its internal dragging flag.
				for _, s := range dv.eqSliders {
					if s != nil {
						s.Handle(mx, my, false)
					}
				}
			case sliderKindEQCurve:
				dv.handleEQCurveDrag(mx, my, false)
			}
			dv.activeSliderKind = sliderKindNone
			return
		}
		// Still dragging — route to the active group only.
		switch dv.activeSliderKind {
		case sliderKindRowVol:
			if dv.activeSlider >= 0 && dv.activeSlider < len(dv.rowVolSliders) {
				s := dv.rowVolSliders[dv.activeSlider]
				if s.Handle(mx, my, true) {
					dv.Rows[dv.activeSlider].Volume = math.Round(s.Value*100) / 100
					dv.markRowControlsDirty()
				}
			}
		case sliderKindMainVol:
			if dv.mainVolSlider != nil && dv.mainVolSlider.Handle(mx, my, true) {
				audio.SetMainVolume(dv.mainVolSlider.Value)
			}
		case sliderKindEQ:
			for i, s := range dv.eqSliders {
				if s != nil && s.Handle(mx, my, true) {
					gain := sliderToGainDB(s.Value)
					ch := dv.activeEQChannel()
					if ch == "main" {
						dv.eqBandGainsDB[i] = gain
						dv.applyMasterEQ()
					} else {
						for j, r := range dv.Rows {
							if r.Instrument == ch {
								dv.ensureRowEQ(j)
								r.EQGainsDB[i] = gain
								dv.applyRowEQ(j)
								break
							}
						}
					}
					break
				}
			}
		case sliderKindEQCurve:
			dv.handleEQCurveDrag(mx, my, true)
		}
		return
	}

	if !mobileEQActive {
		if dv.activeSlider >= 0 {
			s := dv.rowVolSliders[dv.activeSlider]
			if s.Handle(mx, my, left) {
				dv.Rows[dv.activeSlider].Volume = math.Round(s.Value*100) / 100
				dv.logger.Infof("[DRUMVIEW] Row %d volume changed via slider: %.3f", dv.activeSlider, s.Value)
				dv.markRowControlsDirty()
			}
			if !left {
				dv.activeSlider = -1
				dv.activeSliderKind = sliderKindNone
			}
			return
		}
		if !dv.anyDragActive() && !touchDeadZone {
			for _, btn := range dv.rowColorBtns {
				if btn.Handle(mx, my, left) {
					return
				}
			}
		}
		for i, s := range dv.rowVolSliders {
			if s.Handle(mx, my, left) {
				if isSmallScreen() {
					// Mobile: tap on volume icon opens the popup instead of dragging.
					dv.openVolumePopup(i)
				} else {
					dv.Rows[i].Volume = math.Round(s.Value*100) / 100
					dv.logger.Infof("[DRUMVIEW] Row %d volume changed via slider: %.3f", i, s.Value)
					dv.markRowControlsDirty()
					dv.activeSlider = i
					dv.activeSliderKind = sliderKindRowVol
					if !left {
						dv.activeSlider = -1
						dv.activeSliderKind = sliderKindNone
					}
				}
				return
			}
		}
	}

	// Mobile: master volume icon click opens/closes popup.
	// Desktop uses inline slider instead (handled below).
	if isSmallScreen() && !dv.mainVolIconRect.Empty() && left && image.Pt(mx, my).In(dv.mainVolIconRect) {
		if dv.masterVolPopup.IsOpen() {
			dv.closeMasterVolumePopup()
		} else {
			dv.openMasterVolumePopup()
		}
		SuppressClicksUntilMouseUp()
		return
	}

	handled := false
	if dv.mainVolSlider != nil {
		if dv.mainVolSlider.Handle(mx, my, left) {
			audio.SetMainVolume(dv.mainVolSlider.Value)
			if left {
				dv.activeSliderKind = sliderKindMainVol
			}
			return
		}
	}
	// EQ curve drag has priority over sliders.
	if dv.handleEQCurveDrag(mx, my, left) {
		if left {
			dv.activeSliderKind = sliderKindEQCurve
		}
		return
	}
	// Mobile: use eqBandBtns (Button.Handle() with touch expansion) instead
	// of sliders for opening the popup. Desktop: use sliders for direct drag.
	if isSmallScreen() {
		for _, btn := range dv.eqBandBtns {
			if btn == nil {
				continue
			}
			if btn.Handle(mx, my, left) {
				return
			}
		}
	} else {
		for i, s := range dv.eqSliders {
			if s == nil {
				continue
			}
			if s.Handle(mx, my, left) {
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
				if left {
					dv.activeSliderKind = sliderKindEQ
				}
				return
			}
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
	if dv.hpfBtn != nil && dv.hpfBtn.Handle(mx, my, left) {
		handled = true
	}
	if dv.lpfBtn != nil && dv.lpfBtn.Handle(mx, my, left) {
		handled = true
	}
	if !dv.anyDragActive() {
		// Block row/toolbar buttons when any overlay is open — the OverlayStack
		// is the single authority for input while a modal is active.
		// Exempt "inst-menu": its HandleInput returns InputIgnored to let
		// Update() drive component input (instrument selection, scrolling, etc.).
		overlayBlocking := dv.overlays != nil && dv.overlays.HasOpenExcept("inst-menu")
		if !mobileEQActive && !touchDeadZone && !overlayBlocking {
			for i := range dv.rowGroups {
				if dv.rowGroups[i].HandleInput(mx, my, left) {
					handled = true
					break
				}
			}
		}
		// Cancel touch scroller if a button consumed the tap (before dead zone).
		if handled && dv.rowScroll.TouchActive() && !dv.rowScroll.ScrollingCommitted() {
			dv.rowScroll.ResetTouch()
		}
		if handled && left {
			return
		}
		if !overlayBlocking {
			buttons := []*Button{dv.playBtn, dv.stopBtn, dv.bpmDecBtn, dv.bpmIncBtn, dv.subdivBtn, dv.lenDecBtn, dv.lenIncBtn, dv.trackBtn, dv.addRowBtn, dv.uploadBtn, dv.importBtn, dv.exportBtn, dv.eqToggleMobile, dv.viewSwitchBtn, dv.overflowBtn}
			// Pass 1: exact rect match (no touch expansion) — prevents
			// expanded hit areas of adjacent buttons from stealing taps.
			for _, btn := range buttons {
				if btn == nil || btn.Rect().Empty() {
					continue
				}
				if image.Pt(mx, my).In(btn.Rect()) {
					if btn.Handle(mx, my, left) {
						handled = true
					}
					break
				}
			}
			// Pass 2: expanded hit areas (existing behavior) — only if
			// pass 1 didn't match any button's exact rect.
			if !handled {
				for _, btn := range buttons {
					if handled {
						break
					}
					if btn.Handle(mx, my, left) {
						handled = true
					}
				}
			}
		}
		// Cancel touch scroller if a button consumed the tap.
		if handled && dv.rowScroll.TouchActive() && !dv.rowScroll.ScrollingCommitted() {
			dv.rowScroll.ResetTouch()
		}
	}

	// Single-pass BPM handling already performed above. Apply any +/- delta.
	if dv.bpmDelta != 0 {
		dv.SetBPM(dv.bpm + dv.bpmDelta)
		dv.bpmDelta = 0
	}

	if left {
		if !handled && !dv.dragging && !dv.inputCapturedExternally {
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
	if !handled && left && !dv.inputCapturedExternally && pt(mx, my, dv.timelineRect) {
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
}
