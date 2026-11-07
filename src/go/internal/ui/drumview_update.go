package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func (dv *DrumView) Update() {
	// Before the early-out, so the debounce window keeps ticking with no rows.
	dv.tapGate.advance()
	// A dial drag whose capture was dropped without a release is cancelled even off the Synth tab.
	dv.healSynthDialGesture()

	if len(dv.Rows) == 0 {
		return
	}

	// Ahead of the tree, so the TextInput claims Enter/Escape before the portal-close shortcut.
	dv.updateSaveAsDialog()

	dv.updateSeq++
	dv.refreshInstruments()

	// Tests may set dv.bpmDelta directly.
	if dv.transportZone != nil {
		if dv.bpmDelta != 0 {
			dv.transportZone.SetBPMDelta(dv.transportZone.BPMDelta() + dv.bpmDelta)
			dv.bpmDelta = 0
		}
	}

	// Before the tree's layout: the first Update after construction would otherwise publish hit areas
	// from zero rects, without ClipRects.
	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}

	if dv.rootTree != nil {
		dv.rootTree.SetBounds(dv.Bounds)

		dv.rootTree.Update()

		if dv.eqPanelZone != nil {
			dv.eqWaveformMode = dv.eqPanelZone.ActiveTab() == TabWave
		}
		// BPM state: TransportZone is the single authority.
		if dv.transportZone != nil {
			dv.bpm = dv.transportZone.BPM()
			dv.bpmDelta = 0 // zone consumed it
			dv.bpmErrorAnim = dv.transportZone.BPMErrorAnim()
			dv.secPerBeat = 60.0 / float64(dv.bpm)
		}
		// A zone callback's scroll wins; otherwise push dv.rowOffset to the zone.
		if dv.rowRackZone != nil {
			if dv.rowScrollFromZone {
				dv.rowOffset = dv.rowRackZone.RowOffset()
				dv.rowScrollFromZone = false
			} else {
				dv.rowRackZone.SetRowOffset(dv.rowOffset)
			}
		}

		dv.syncNativeGestures() // single native-gesture projection site
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
				dv.logger.Errorf("[drumview] import failed: %v", res.err)
				dv.notifyErrorKey(i18n.KeyNotifErrLoadJSON, res.err.Error())
			} else if len(res.data) == 0 {
				dv.logger.Debugf("[drumview] import canceled (no data)")
			} else if dv.onImport != nil {
				// Wired to Game, onImport defers and returns nil; standalone mocks may return an error here.
				if err := dv.onImport(res.data, importSourceName(res.name)); err != nil {
					dv.logger.Errorf("[drumview] import error: %v", err)
					dv.notifyErrorKey(i18n.KeyNotifErrLoadJSON, err.Error())
				}
				// Game reports success once the deferred import completes.
			}
		default:
		}
		// A cancelled browser picker may send no change event, so release the guard eventually.
		if int(dv.frame)-dv.importAttemptFrame > 600 || dv.updateSeq-dv.importAttemptUpdate > 600 { // ~10s at 60fps
			dv.importing = false
			if dv.onImportDialogEnd != nil {
				dv.onImportDialogEnd()
			}
			dv.notifyErrorKey(i18n.KeyNotifImportCanceled)
		}
	}

	dv.drainInstrumentImport()

	if dv.uploading {
		select {
		case res := <-dv.uploadCh:
			dv.uploading = false
			dv.logger.Debugf("[drumview] Upload result path=%s err=%v", res.path, res.err)
			if res.err != nil {
				dv.logger.Errorf("[drumview] failed to load WAV: %v", res.err)
				dv.notifyErrorKey(i18n.KeyNotifErrLoadWAV, res.err.Error())
			} else {
				dv.CloseAllPopups() // close rename/menus before entering naming mode
				dv.pendingWAV = res.path
				dv.openNamingPortal()
				dv.nameInput = ""
				// Initialize naming input box for consistent UX
				dv.nameBox = dv.newWAVNameBox(dv.nameBoxRect())
				dv.nameBox.SetText("")
				dv.nameBox.focused = true
				dv.notifyInfoKey(i18n.KeyNotifSelectedWAV, importSourceName(res.path))
			}
		drainUpload:
			for {
				select {
				case extra := <-dv.uploadCh:
					dv.logger.Debugf("[drumview] Dropping stale upload result path=%s err=%v", extra.path, extra.err)
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

	// Before the popup guard: a hold-to-repeat capture sets suppress.
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

	// Read once per frame; the tree sends wheel to portals, this is the row-scroll fallback.
	wheelSteps := wheelScrollSteps()

	// Block the remaining handlers while any portal is open.
	if dv.portal().IsOpen() {
		// Clean up the touch scroll even here, or a portal opened mid-scroll leaves Capturing() true and
		// blocks grid panning forever.
		if dv.rowScroll().TouchActive() && !left {
			dv.rowScroll().HandleTouchEnd()
		}
		return
	}
	if (dv.tree != nil && dv.tree.Suppressing()) ||
		(dv.audioTree != nil && dv.audioTree.Suppressing()) {
		return
	}

	mobileEQActive := dv.MobileEQMode()

	if dv.rowRackZone != nil {
		dv.rowOffset = dv.rowRackZone.RowOffset()
		dv.selRow = dv.rowRackZone.SelRow()
	}
	if !mobileEQActive {
		dv.syncRowScroll()
		totalRows := len(dv.Rows)
		visRows := dv.visibleRows()
		if totalRows > visRows {
			// Only over the drum pane, so the grid pane's wheel is never eaten; skip when the tree consumed it.
			treeHandled := (dv.tree != nil && dv.tree.WheelHandled()) ||
				(dv.audioTree != nil && dv.audioTree.WheelHandled())
			overBar := image.Pt(mx, my).In(dv.scrollBarRect())
			overDrum := image.Pt(mx, my).In(dv.Bounds)
			if !treeHandled && (overBar || overDrum) && wheelSteps != 0 {
				dv.logger.Debugf("[drumview] row wheel steps=%d at (%d,%d) rowOffset=%d", wheelSteps, mx, my, dv.rowOffset)
				if dv.rowScroll().WheelStep(wheelSteps, controlGridScrollCooldownFrames) {
					dv.flushRowScroll()
				}
			}

			// Stepped and anchored at the grab point, so it never flies through the list.
			if dv.rowScroll().StepDragging() {
				if left {
					if dv.rowScroll().StepDragTo(my, controlGridDragStepPx) {
						dv.flushRowScroll()
					}
				} else {
					dv.rowScroll().EndStepDrag()
				}
			} else if left && !dv.inputCapturedExternally && image.Pt(mx, my).In(dv.scrollThumbRect()) {
				dv.rowScroll().BeginStepDrag(my)
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

}
