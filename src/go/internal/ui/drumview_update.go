package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (dv *DrumView) Update() {
	if len(dv.Rows) == 0 {
		return
	}

	dv.updateSeq++
	dv.refreshInstruments()

	// Sync DrumView → TransportZone state before tree runs.
	// Tests may set dv.bpmPrev/dv.bpmDelta directly; push those to the zone.
	if dv.transportZone != nil {
		if dv.bpmPrev != dv.transportZone.BPMPrev() {
			dv.transportZone.SetBPMPrev(dv.bpmPrev)
		}
		if dv.bpmDelta != 0 {
			dv.transportZone.SetBPMDelta(dv.transportZone.BPMDelta() + dv.bpmDelta)
			dv.bpmDelta = 0
		}
	}

	// Ensure zone rects are current BEFORE the tree runs its Layout phase.
	// calcLayout sets zone rects via tree.SetZoneRect; the tree's Phase 1
	// detects rect changes and rebuilds hit areas. Without this, the first
	// tree.Update after construction would run with zero rects, producing
	// hit areas without proper ClipRects (touch expansion leaks).
	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}

	// Run the zone-based component tree (runs in parallel with existing
	// code during incremental migration; zones are added one at a time).
	if dv.tree != nil {
		dv.tree.SetBounds(dv.Bounds)

		dv.tree.Update()

		// Sync zone → DrumView state that the tree may have changed:
		// EQ toggle: sync zone state → DrumView.
		if dv.eqPanelZone != nil {
			dv.eqWaveformMode = dv.eqPanelZone.WaveformMode()
		}
		// BPM state: TransportZone is the single authority.
		if dv.transportZone != nil {
			dv.bpm = dv.transportZone.BPM()
			dv.bpmPrev = dv.transportZone.BPMPrev()
			dv.bpmDelta = 0 // zone consumed it
			dv.bpmErrorAnim = dv.transportZone.BPMErrorAnim()
			dv.secPerBeat = 60.0 / float64(dv.bpm)
		}
		// Row scroll sync: when a zone callback changed the scroll,
		// zone's rowOffset is authoritative → copy to dv.
		// Otherwise, legacy code may have changed dv.rowOffset → push to zone.
		if dv.rowRackZone != nil {
			if dv.rowScrollFromZone {
				dv.rowOffset = dv.rowRackZone.RowOffset()
				dv.rowScrollFromZone = false
			} else {
				dv.rowRackZone.SetRowOffset(dv.rowOffset)
			}
		}
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
				dv.openNamingPortal()
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

	// Naming overlay is now fully handled by the naming portal (updateFn/inputFn/drawFn).

	// Rename keyboard/input is now handled by the rename portal's updateFn.

	// Layout resize hover/drag is now handled by layoutResizeZone in the tree.

	// BPM text input (focus/blur/commit, mobile polling) is now handled by
	// TransportZone.Update() in the tree. No legacy BPM handling needed here.

	// Process one-frame flags set by tree-dispatched button callbacks.
	// These must run before the popup/suppress guard because the tree's
	// capture (e.g., len button hold-to-repeat) sets suppress=true.
	if dv.lenIncPressed {
		inc := max1(dv.timelineUnitsPerBeat)
		dv.changeLength(dv.Length + inc)
		dv.lenIncPressed = false
		dv.userAdjustedLength = true
	}
	if dv.lenDecPressed {
		inc := max1(dv.timelineUnitsPerBeat)
		dv.changeLength(dv.Length - inc)
		dv.lenDecPressed = false
		dv.userAdjustedLength = true
	}

	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)

	// Read wheel delta once at start of frame. The tree dispatches wheel
	// events to portal overlays; this value is used by the row scroll fallback.
	wheelSteps := wheelScrollSteps()

	// ─── POPUP INPUT GUARD ───
	// All popup/overlay input is now dispatched through the portal/tree system.
	// Block remaining handlers when any portal overlay is open.
	if dv.tree != nil && dv.tree.Portal().IsOpen() {
		// Ensure touch scroll is cleaned up even when the portal blocks
		// further processing. Without this, TouchActive() stays true if a
		// portal opened while a touch scroll was active, causing
		// Capturing() to return true permanently and blocking grid panning.
		if dv.rowScroll().TouchActive() && !left {
			dv.rowScroll().HandleTouchEnd()
		}
		return
	}
	if dv.tree != nil && dv.tree.Suppress() {
		return
	}

	mobileEQActive := Profile().IsMobile() && dv.mobileEQMode

	// ─── ROW SCROLLING (after popups) ───
	// When RowRackZone exists, sync rowOffset/selRow from zone state.
	if dv.rowRackZone != nil {
		dv.rowOffset = dv.rowRackZone.RowOffset()
		dv.selRow = dv.rowRackZone.SelRow()
	}
	if !mobileEQActive {
		dv.syncRowScroll()
		totalRows := len(dv.Rows)
		visRows := dv.visibleRows()
		if totalRows > visRows {
			// Only consume wheel for row scrolling when the cursor is over
			// the drum pane (including the scrollbar). This prevents wheel
			// events from being eaten while the cursor is over the grid pane.
			// Skip if wheel was already consumed by the tree (zone hit handler).
			treeHandled := dv.tree != nil && dv.tree.WheelHandled()
			overBar := image.Pt(mx, my).In(dv.scrollBarRect())
			overDrum := image.Pt(mx, my).In(dv.Bounds)
			if !treeHandled && (overBar || overDrum) && wheelSteps != 0 {
				dv.logger.Infof("[DRUMVIEW] row wheel steps=%d at (%d,%d) rowOffset=%d", wheelSteps, mx, my, dv.rowOffset)
				if dv.rowScroll().HandleWheel(wheelSteps) {
					dv.flushRowScroll()
				}
			}

			// Scrollbar drag
			if dv.rowScroll().Dragging() {
				if left {
					if dv.rowScroll().HandleDragTo(my) {
						dv.flushRowScroll()
					}
				} else {
					dv.rowScroll().HandleDragEnd()
				}
			} else if left && !dv.inputCapturedExternally && image.Pt(mx, my).In(dv.scrollThumbRect()) {
				dv.rowScroll().HandleDragStart(my)
			}

			// ─── TOUCH SCROLL ───
			if !dv.rowScroll().Dragging() {
				rowsRect := dv.rowsRect()
				if left && touchOverrideActive && !dv.inputCapturedExternally && !dv.rowScroll().TouchActive() && !isTouchTapInjecting() && image.Pt(mx, my).In(rowsRect) && !image.Pt(mx, my).In(dv.scrollBarRect()) {
					dv.rowScroll().HandleTouchBegin(mx, my)
				} else if dv.rowScroll().TouchActive() && left && !isTouchTapInjecting() {
					if dv.rowScroll().HandleTouchMove(mx, my) {
						dv.flushRowScroll()
					}
				} else if dv.rowScroll().TouchActive() && !left {
					dv.rowScroll().HandleTouchEnd()
				}
			}
		}

		// ─── TOUCH SCROLL MOMENTUM (runs every frame) ───
		if dv.rowScroll().HasMomentum() {
			if len(dv.Rows) > dv.visibleRows() {
				dv.syncRowScroll()
				if dv.rowScroll().UpdateMomentum() {
					dv.flushRowScroll()
				}
			} else {
				dv.rowScroll().ResetTouch()
			}
		}
	}

	// All button/slider input is now routed through the zone tree's HitAreas:
	// - EQ sliders/buttons → EQPanelZone
	// - Transport buttons → TransportZone
	// - Row buttons/sliders → RowRackZone
	// - Track/Len+/- buttons → TimelineZone
	// - EQ curve drag → EQPanelZone
}
