package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// openSubdivMenuPortal opens the subdiv menu component through the portal.
func (dv *DrumView) openSubdivMenuPortal() {
	if dv.tree == nil || dv.subdivMenuComp == nil {
		return
	}
	dv.tree.Portal().Open(PortalEntry{
		ID:      "subdiv-menu",
		Overlay: &compPortalOverlay{comp: dv.subdivMenuComp, tag: "subdiv-menu"},
		Modal:   false,
		Anchor:  dv.subdivBtn().Rect(),
		OnClose: func() {
			dv.subdivMenuComp.Close()
		},
	})
}

// closeSubdivMenuPortal closes the subdiv menu portal entry.
func (dv *DrumView) closeSubdivMenuPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("subdiv-menu")
	}
}

// openInstMenuPortal opens the instrument menu component through the portal.
func (dv *DrumView) openInstMenuPortal() {
	if dv.tree == nil || dv.instMenuComp == nil {
		return
	}
	anchor := image.Rectangle{}
	if dv.instMenuRow >= 0 && dv.instMenuRow < len(dv.rowLabels()) {
		anchor = dv.rowLabels()[dv.instMenuRow].Rect()
	}
	dv.tree.Portal().Open(PortalEntry{
		ID: "inst-menu",
		Overlay: &compPortalOverlay{
			comp:     dv.instMenuComp,
			tag:      "inst-menu",
			updateFn: func() {
				dv.instMenuComp.Update()
				dv.syncInstMenuScrollFromComp()
			},
		},
		Modal:  false,
		Anchor: anchor,
		OnClose: func() {
			if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
				dv.instMenuComp.Close()
			}
			// Upload button special case: if click-outside was on the
			// upload button, trigger it now.
			mx, my := cursorPosition()
			if isMouseButtonPressed(ebiten.MouseButtonLeft) && dv.uploadBtn() != nil && image.Pt(mx, my).In(dv.uploadBtn().Rect()) {
				_ = dv.uploadBtn().Handle(mx, my, true)
			}
		},
	})
}

// closeInstMenuPortal closes the instrument menu portal entry.
func (dv *DrumView) closeInstMenuPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("inst-menu")
	}
}

// openColorWheelPortal opens the color wheel component through the portal.
func (dv *DrumView) openColorWheelPortal() {
	if dv.tree == nil || dv.colorWheelComp == nil {
		return
	}
	anchor := image.Rectangle{}
	if dv.colorMenuRow >= 0 && dv.colorMenuRow < len(dv.rowColorBtns()) {
		anchor = dv.rowColorBtns()[dv.colorMenuRow].Rect()
	}
	// Desktop: color button is hidden; fall back to label rect.
	if anchor.Empty() && dv.colorMenuRow >= 0 && dv.colorMenuRow < len(dv.rowLabels()) {
		anchor = dv.rowLabels()[dv.colorMenuRow].Rect()
	}
	dv.tree.Portal().Open(PortalEntry{
		ID:      "color-wheel",
		Overlay: &compPortalOverlay{comp: dv.colorWheelComp, tag: "color-wheel"},
		Modal:   false,
		Anchor:  anchor,
		OnClose: func() {
			if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
				dv.colorWheelComp.Close()
			}
		},
	})
}

// closeColorWheelPortal closes the color wheel portal entry.
func (dv *DrumView) closeColorWheelPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("color-wheel")
	}
}

// openRenamePortal opens the rename component through the portal.
func (dv *DrumView) openRenamePortal() {
	if dv.tree == nil || dv.renameComp == nil {
		return
	}
	anchor := image.Rectangle{}
	if dv.renameRow >= 0 && dv.renameRow < len(dv.rowLabels()) {
		anchor = dv.rowLabels()[dv.renameRow].Rect()
	}
	dv.tree.Portal().Open(PortalEntry{
		ID: "rename",
		Overlay: &compPortalOverlay{
			comp: dv.renameComp,
			tag:  "rename",
			updateFn: func() {
				// Poll keyboard (Enter/Escape) + TextInput updates through portal.
				dv.renameComp.PollKeyboard()
			},
		},
		Modal:  false,
		Anchor: anchor,
	})
}

// closeRenamePortal closes the rename portal entry.
func (dv *DrumView) closeRenamePortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("rename")
	}
}

// openOverflowMenuPortal opens the overflow menu through the portal.
func (dv *DrumView) openOverflowMenuPortal() {
	if dv.tree == nil {
		return
	}
	dv.tree.Portal().Open(PortalEntry{
		ID: "overflow-menu",
		Overlay: &dvOverlayPortal{
			id:       "overflow-menu",
			isOpenFn: func() bool { return dv.IsOverflowMenuOpen() },
			rectFn:   func() image.Rectangle { return dv.overflowPopupRect() },
			inputFn: func(x, y int, pressed bool) InputResult {
				if dv.handleOverflowMenuInput(x, y, pressed) {
					if dv.overflowDeferredTap.Active() {
						return InputCaptured
					}
					return InputConsumed
				}
				return InputIgnored
			},
			wheelFn: func(x, y, steps int) InputResult {
				if dv.overflowScroll != nil && dv.overflowScroll.HasScroll() {
					dv.overflowScroll.HandleWheel(steps)
				}
				return InputConsumed
			},
			drawFn: func(dst *ebiten.Image) { dv.drawOverflowMenu(dst) },
		},
		Modal: false,
		OnClose: func() {
			// Do non-portal cleanup only (portal removal is handled by
			// the portal system itself — avoid calling closeOverflowMenu
			// which would recursively call portal.Close).
			dv.overflowDeferredTap.Cancel()
			filePickerClearRects()
		},
	})
}

// closeOverflowMenuPortal closes the overflow menu portal entry.
func (dv *DrumView) closeOverflowMenuPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("overflow-menu")
	}
}

// openContextMenuPortal opens the context menu through the portal.
func (dv *DrumView) openContextMenuPortal() {
	if dv.tree == nil {
		return
	}
	dv.tree.Portal().Open(PortalEntry{
		ID: "context-menu",
		Overlay: &dvOverlayPortal{
			id:       "context-menu",
			isOpenFn: func() bool { return dv.IsContextMenuOpen() },
			rectFn:   func() image.Rectangle { return dv.contextMenuRect },
			inputFn: func(x, y int, pressed bool) InputResult {
				if dv.handleContextMenuInput(x, y, pressed) {
					if dv.contextMenuDeferredTap.Active() {
						return InputCaptured
					}
					if s := dv.contextMenuScroll; s != nil {
						if s.Dragging() || s.ScrollingCommitted() || s.TouchActive() {
							return InputCaptured
						}
					}
					return InputConsumed
				}
				return InputIgnored
			},
			wheelFn: func(x, y, steps int) InputResult {
				if dv.contextMenuScroll != nil && dv.contextMenuScroll.HasScroll() {
					dv.contextMenuScroll.HandleWheel(steps)
					dv.rebuildContextMenuButtons()
				}
				return InputConsumed
			},
			drawFn: func(dst *ebiten.Image) { dv.drawContextMenu(dst) },
			updateFn: func() {
				if dv.contextMenuScroll != nil && dv.contextMenuScroll.HasMomentum() {
					if dv.contextMenuScroll.UpdateMomentum() {
						dv.rebuildContextMenuButtons()
					}
				}
			},
		},
		Modal:   false,
		OnClose: func() {},
	})
}

// closeContextMenuPortal closes the context menu portal entry.
func (dv *DrumView) closeContextMenuPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("context-menu")
	}
}

// openFXPanelPortal opens the FX panel through the portal.
func (dv *DrumView) openFXPanelPortal() {
	if dv.tree == nil {
		return
	}
	dv.tree.Portal().Open(PortalEntry{
		ID: "fx-panel",
		Overlay: &dvOverlayPortal{
			id:       "fx-panel",
			isOpenFn: func() bool { return dv.IsFXPanelOpen() },
			rectFn: func() image.Rectangle {
				return dv.fxPanelRect
			},
			inputFn: func(x, y int, pressed bool) InputResult {
				if dv.handleFXPanelInput(x, y, pressed) {
					if dv.fxPanelDeferredTap.Active() || dv.fxScrollTS.Active() || dv.fxSliderDragging {
						return InputCaptured
					}
					return InputConsumed
				}
				return InputIgnored
			},
			wheelFn: func(x, y, steps int) InputResult {
				if dv.fxScrollMaxPx > 0 {
					dv.fxScrollOffsetPx -= steps * 20
					if dv.fxScrollOffsetPx < 0 {
						dv.fxScrollOffsetPx = 0
					}
					if dv.fxScrollOffsetPx > dv.fxScrollMaxPx {
						dv.fxScrollOffsetPx = dv.fxScrollMaxPx
					}
					dv.buildFXPanel()
					dv.refreshFXPortalHitAreas()
				}
				return InputConsumed
			},
			drawFn: func(dst *ebiten.Image) { dv.drawFXPanel(dst) },
			updateFn: func() {
				if dv.fxScrollTS.HasMomentum() {
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
						dv.refreshFXPortalHitAreas()
					}
				}
			},
		},
		Modal: false,
		OnClose: func() {
			// Do non-portal cleanup only (portal removal is handled by
			// the portal system — avoid calling closeFXPanel which would
			// recursively call portal.Close).
			dv.fxAddMenuOpen = false
			dv.fxPanelBtns = nil
			dv.fxPanelSliders = nil
			dv.fxSliderDragging = false
			dv.fxPanelDeferredTap.Cancel()
			dv.fxScrollOffsetPx = 0
			dv.fxScrollTS.Reset()
			dv.fxScrollMaxPx = 0
			dv.fxPanelRect = image.Rectangle{}
		},
	})
}

// closeFXPanelPortal closes the FX panel portal entry.
func (dv *DrumView) closeFXPanelPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("fx-panel")
	}
}

// openVolPopupPortal opens the row volume popup through the portal.
func (dv *DrumView) openVolPopupPortal() {
	if dv.tree == nil || dv.volPopup == nil {
		return
	}
	dv.tree.Portal().Open(PortalEntry{
		ID:      "volume-popup",
		Overlay: &sliderPopupPortalOverlay{popup: dv.volPopup, tag: "volume-popup"},
		Modal:   true,
		OnClose: func() {
			dv.volPopup.Close()
		},
	})
}

// closeVolPopupPortal closes the row volume popup portal entry.
func (dv *DrumView) closeVolPopupPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("volume-popup")
	}
}

// openMasterVolPopupPortal opens the master volume popup through the portal.
func (dv *DrumView) openMasterVolPopupPortal() {
	if dv.tree == nil || dv.masterVolPopup == nil {
		return
	}
	dv.tree.Portal().Open(PortalEntry{
		ID:      "master-volume-popup",
		Overlay: &sliderPopupPortalOverlay{popup: dv.masterVolPopup, tag: "master-vol-popup"},
		Modal:   true,
		OnClose: func() {
			dv.masterVolPopup.Close()
		},
	})
}

// closeMasterVolPopupPortal closes the master volume popup portal entry.
func (dv *DrumView) closeMasterVolPopupPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("master-volume-popup")
	}
}

// openNamingPortal opens the naming overlay through the portal.
func (dv *DrumView) openNamingPortal() {
	if dv.tree == nil {
		return
	}
	dv.tree.Portal().Open(PortalEntry{
		ID: "naming",
		Overlay: &dvOverlayPortal{
			id:       "naming",
			isOpenFn: func() bool { return dv.IsNamingOpen() },
			rectFn:   func() image.Rectangle { return dv.Bounds },
			inputFn: func(x, y int, pressed bool) InputResult {
				if dv.saveBtn != nil && dv.saveBtn.Handle(x, y, pressed) {
					dv.saveAnim = 1
					return InputConsumed
				}
				if pressed && dv.nameBox != nil && !pt(x, y, dv.nameBox.Rect) {
					// Click outside cancels naming (same as Esc)
					dv.pendingWAV = ""
					dv.nameInput = ""
					dv.nameBox = nil
					dv.closeNamingPortal()
					return InputConsumed
				}
				return InputConsumed
			},
			wheelFn: func(x, y, steps int) InputResult { return InputConsumed },
			drawFn: func(dst *ebiten.Image) {
				box := dv.nameBoxRect()
				if dv.nameBox == nil {
					dv.nameBox = NewTextInput(box, BPMBoxStyle)
					dv.nameBox.MaxLen = 32
				}
				dv.nameBox.Rect = box
				dv.nameBox.Draw(dst)
				if dv.saveBtn != nil {
					dv.saveBtn.Draw(dst)
				}
			},
			updateFn: func() {
				if !dv.IsNamingOpen() {
					return
				}
				// Poll mobile native input for WAV name
				if Profile().IsMobile() && mobileInputActive("wav-name") {
					if val, committed, ok := mobileInputPollResult("wav-name"); ok {
						if committed {
							id := strings.TrimSpace(val)
							if id != "" {
								dv.registerInstrument(id)
							} else {
								dv.pendingWAV = ""
								dv.nameInput = ""
								dv.nameBox = nil
								dv.closeNamingPortal()
							}
						} else {
							dv.pendingWAV = ""
							dv.nameInput = ""
							dv.nameBox = nil
							dv.closeNamingPortal()
						}
					}
					dv.namePhase += 0.1
					return
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
					dv.logger.Debugf("[drumview] save instrument pressed id=%q", id)
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
					dv.pendingWAV = ""
					dv.nameInput = ""
					dv.nameBox = nil
					dv.closeNamingPortal()
				}
				dv.namePhase += 0.1
			},
		},
		Modal: true,
		OnClose: func() {
			// Do non-portal cleanup only (avoid closeNaming which
			// calls closeNamingPortal → recursive portal.Close).
			dv.pendingWAV = ""
			dv.nameInput = ""
			dv.nameBox = nil
		},
	})
}

// closeNamingPortal closes the naming portal entry.
func (dv *DrumView) closeNamingPortal() {
	if dv.tree != nil {
		dv.tree.Portal().Close("naming")
	}
}
