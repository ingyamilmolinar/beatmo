package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// openSubdivMenuPortal opens the subdiv menu component through the portal.
func (dv *DrumView) openSubdivMenuPortal() {
	if dv.tree == nil || dv.subdivMenuComp == nil {
		return
	}
	// SubdivMenuProps has no bounds field.
	dv.subdivMenuComp.SetScreenBounds(dv.Bounds)
	dv.portal().Open(PortalEntry{
		ID:      "subdiv-menu",
		Overlay: &compPortalOverlay{comp: dv.subdivMenuComp, tag: "subdiv-menu", updateFn: dv.subdivMenuComp.Update},
		Modal:   false,
		Scrim:   true, // menu/picker — backdrop dims so modality is visible
		Anchor:  dv.subdivBtn().Rect(),
		OnClose: func() {
			dv.subdivMenuComp.Close()
		},
	})
}

// closeSubdivMenuPortal closes the subdiv menu portal entry.
func (dv *DrumView) closeSubdivMenuPortal() {
	if dv.tree != nil {
		dv.portal().Close("subdiv-menu")
	}
}

// navMenuPortalID identifies the navigation-menu portal entry.
const navMenuPortalID = "nav-menu"

// A portal component: the tree delivers press/drag/release natively, so jog buttons pop back and
// the latch resets.
func (dv *DrumView) openNavMenuPortal() {
	if dv.tree == nil || dv.navMenuComp == nil {
		return
	}
	dv.navMenuComp.SetScreenBounds(dv.navMenuBounds())
	dv.navMenuComp.SetAnchor(dv.navMenuAnchor())
	dv.navMenuComp.Open()
	dv.portal().Open(PortalEntry{
		ID: navMenuPortalID,
		Overlay: &compPortalOverlay{
			comp:     dv.navMenuComp,
			tag:      navMenuPortalID,
			updateFn: dv.navMenuComp.Update,
		},
		Modal:   false,
		Scrim:   true, // menu — backdrop dims so modality is visible
		Anchor:  dv.beatCounterRect,
		OnClose: func() { dv.navMenuComp.Close() },
	})
}

// closeNavMenuPortal closes the navigation-menu portal entry.
func (dv *DrumView) closeNavMenuPortal() {
	if dv.tree != nil {
		dv.portal().Close(navMenuPortalID)
	}
}

// IsNavMenuOpen reports whether the navigation menu portal is open.
func (dv *DrumView) IsNavMenuOpen() bool {
	return dv.navMenuComp != nil && dv.navMenuComp.IsOpen()
}

// jumpMenuComp is a second SubdivMenuComponent with beat-count options; it stacks over the nav menu.
func (dv *DrumView) openJumpMenuPortal() {
	if dv.tree == nil || dv.jumpMenuComp == nil {
		return
	}
	// SubdivMenuProps has no bounds field.
	dv.jumpMenuComp.SetScreenBounds(dv.Bounds)
	anchor := dv.jumpMenuComp.Props().AnchorRect
	if anchor.Empty() && dv.navMenuComp != nil {
		anchor = dv.navMenuComp.Bounds()
	}
	dv.portal().Open(PortalEntry{
		ID: "jump-menu",
		// updateFn is required, or the menu gets neither fling momentum nor keyboard navigation.
		Overlay: &compPortalOverlay{comp: dv.jumpMenuComp, tag: "jump-menu", updateFn: dv.jumpMenuComp.Update},
		Modal:   false,
		Scrim:   true, // menu/picker — backdrop dims so modality is visible
		Anchor:  anchor,
		OnClose: func() {
			dv.jumpMenuComp.Close()
		},
	})
}

// closeJumpMenuPortal closes the jump-size menu portal entry.
func (dv *DrumView) closeJumpMenuPortal() {
	if dv.tree != nil {
		dv.portal().Close("jump-menu")
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
	dv.portal().Open(PortalEntry{
		ID: "inst-menu",
		Overlay: &compPortalOverlay{
			comp: dv.instMenuComp,
			tag:  "inst-menu",
			updateFn: func() {
				dv.instMenuComp.Update()
				dv.syncInstMenuScrollFromComp()
			},
		},
		Modal:  false,
		Scrim:  true, // instrument picker — backdrop dims so modality is visible
		Anchor: anchor,
		OnClose: func() {
			if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
				dv.instMenuComp.Close()
			}
			// A click-outside that landed on the upload button triggers it.
			mx, my := cursorPosition()
			if isMouseButtonPressed(ebiten.MouseButtonLeft) && dv.uploadBtn() != nil && image.Pt(mx, my).In(dv.uploadBtn().Rect()) {
				dv.uploadBtn().HandleInputResult(mx, my, true)
			}
		},
	})
}

// closeInstMenuPortal closes the instrument menu portal entry.
func (dv *DrumView) closeInstMenuPortal() {
	if dv.tree != nil {
		dv.portal().Close("inst-menu")
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
	dv.portal().Open(PortalEntry{
		ID: "color-wheel",
		// updateFn pumps the keyboard cursor; without it the picker claims the arrows and does nothing.
		Overlay: &compPortalOverlay{
			comp:     dv.colorWheelComp,
			tag:      "color-wheel",
			updateFn: dv.colorWheelComp.UpdateKeyNav,
		},
		Modal: false,
		// No portal scrim: the color picker draws its own.
		Anchor: anchor,
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
		dv.portal().Close("color-wheel")
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
	dv.portal().Open(PortalEntry{
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
		Scrim:  true, // rename dialog — backdrop dims so modality is visible
		Anchor: anchor,
	})
}

// closeRenamePortal closes the rename portal entry.
func (dv *DrumView) closeRenamePortal() {
	if dv.tree != nil {
		dv.portal().Close("rename")
	}
}

// openOverflowMenuPortal opens the overflow menu through the portal.
func (dv *DrumView) openOverflowMenuPortal() {
	if dv.tree == nil {
		return
	}
	// So a configured MenuScroll exists the moment the menu opens.
	dv.configureOverflowScroll()
	dv.portal().Open(PortalEntry{
		ID: "overflow-menu",
		Overlay: &dvOverlayPortal{
			id:       "overflow-menu",
			isOpenFn: func() bool { return dv.IsOverflowMenuOpen() },
			rectFn:   func() image.Rectangle { return dv.overflowPopupRect() },
			hoverFn: func(mx, my int) {
				if dv.overflowCard != nil {
					dv.overflowCard.HoverAt(mx, my)
				}
				for _, b := range dv.overflowBtns {
					b.UpdateHover(mx, my)
				}
				// Same hover glow as every other menu row.
				if dv.overflowPage == 0 && dv.navPanel != nil {
					for _, b := range dv.navPanel.Buttons() {
						if b != nil {
							b.UpdateHover(mx, my)
						}
					}
				}
			},
			inputFn: func(x, y int, pressed bool) InputResult {
				if dv.handleOverflowMenuInput(x, y, pressed) {
					// Capture while a deferred tap is pending (tap vs scroll); otherwise one-shot.
					if dv.overflowMenuScroll != nil && dv.overflowMenuScroll.TapActive() {
						return InputCaptured
					}
					return InputConsumed
				}
				return InputIgnored
			},
			wheelFn: func(x, y, steps int) InputResult {
				if dv.overflowMenuScroll != nil && dv.overflowMenuScroll.HasScroll() {
					dv.overflowMenuScroll.HandleWheel(steps)
				}
				return InputConsumed
			},
			drawFn: func(dst *ebiten.Image) { dv.drawOverflowMenu(dst) },
			updateFn: func() {
				// rebuild is nil: overflowPopupBtns refreshes every rect from the live offset each frame.
				if dv.overflowMenuScroll != nil {
					dv.overflowMenuScroll.Tick(nil)
				}
				dv.updateTemplateSearch()
				// After updateTemplateSearch, so the cursor indexes this frame's results.
				dv.updateOverflowKeyNav()
			},
			escapeFn: func() bool { return dv.handleTemplateSearchEscape() },
			// While open the overlay owns the keyboard, so the grid's arrow shortcuts yield to the search caret.
			claimsKeyboardFn: func() bool { return dv.IsOverflowMenuOpen() },
		},
		Modal: false,
		Scrim: true, // overflow menu — backdrop dims so modality is visible
		OnClose: func() {
			// Not closeOverflowMenu: it would call portal.Close recursively.
			if dv.overflowMenuScroll != nil {
				dv.overflowMenuScroll.DeferredTap().Cancel()
			}
			dv.overflowPage = 0
			dv.setTemplateSearchQuery("")
			if dv.templateSearchBox != nil {
				dv.templateSearchBox.SetFocus(false)
			}
		},
	})
}

// closeOverflowMenuPortal closes the overflow menu portal entry.
func (dv *DrumView) closeOverflowMenuPortal() {
	if dv.tree != nil {
		dv.portal().Close("overflow-menu")
	}
}

// openContextMenuPortal opens the context menu through the portal.
func (dv *DrumView) openContextMenuPortal() {
	if dv.tree == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID: "context-menu",
		Overlay: &dvOverlayPortal{
			id:       "context-menu",
			isOpenFn: func() bool { return dv.IsContextMenuOpen() },
			rectFn:   func() image.Rectangle { return dv.contextMenuRect },
			hoverFn: func(mx, my int) {
				for _, b := range dv.contextMenuBtns {
					b.UpdateHover(mx, my)
				}
				dv.contextMenuCloseButton.UpdateHover(mx, my)
			},
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
					if dv.contextMenuScroll.HandleWheel(steps) {
						dv.rebuildContextMenuButtons()
					}
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
				dv.updateContextKeyNav()
			},
			// Arrows drive the list, so the grid's arrow-pan must yield.
			claimsKeyboardFn: func() bool { return dv.IsContextMenuOpen() },
		},
		Modal:   false,
		Scrim:   true, // row context menu — backdrop dims so modality is visible
		OnClose: func() {},
	})
}

// closeContextMenuPortal closes the context menu portal entry.
func (dv *DrumView) closeContextMenuPortal() {
	if dv.tree != nil {
		dv.portal().Close("context-menu")
	}
}

// openFXPanelPortal opens the FX panel through the portal.
func (dv *DrumView) openFXPanelPortal() {
	if dv.tree == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID: "fx-panel",
		Overlay: &dvOverlayPortal{
			id:       "fx-panel",
			isOpenFn: func() bool { return dv.IsFXPanelOpen() },
			rectFn: func() image.Rectangle {
				return dv.fxPanelRect
			},
			hoverFn: func(mx, my int) {
				for _, b := range dv.fxPanelBtns {
					b.UpdateHover(mx, my)
				}
			},
			inputFn: func(x, y int, pressed bool) InputResult {
				if dv.handleFXPanelInput(x, y, pressed) {
					if dv.fxPanelDeferredTap.Active() || (dv.fxScroll != nil && dv.fxScroll.ScrollBehavior().TouchActive()) || dv.fxSliderDragging {
						return InputCaptured
					}
					return InputConsumed
				}
				return InputIgnored
			},
			wheelFn: func(x, y, steps int) InputResult {
				if dv.fxScroll != nil && dv.fxScroll.HasScroll() {
					if dv.fxScroll.HandleWheel(steps * fxWheelStepPx) {
						dv.buildFXPanel()
						dv.refreshFXPortalHitAreas()
					}
				}
				return InputConsumed
			},
			drawFn: func(dst *ebiten.Image) { dv.drawFXPanel(dst) },
			updateFn: func() {
				// No scrim: a docked tool panel the user works around, not a modal.
				if dv.fxScroll != nil {
					dv.fxScroll.Tick(func() { dv.buildFXPanel(); dv.refreshFXPortalHitAreas() })
				}
				dv.updateFXAddSearch()
			},
			escapeFn: func() bool { return dv.fxPanelHandleEscape() },
			// While the add-menu search is focused the panel owns the keyboard, so typing never pans the grid.
			claimsKeyboardFn: func() bool {
				return dv.fxAddMenuOpen && dv.fxAddSearch != nil && dv.fxAddSearch.Focused()
			},
		},
		Modal: false,
		OnClose: func() {
			// Not closeFXPanel: it would call portal.Close recursively.
			dv.fxAddMenuOpen = false
			dv.fxPanelBtns = nil
			dv.fxPanelSliders = nil
			dv.fxSliderDragging = false
			dv.fxPanelDeferredTap.Cancel()
			dv.fxScroll = nil
			dv.fxPanelRect = image.Rectangle{}
		},
	})
}

// closeFXPanelPortal closes the FX panel portal entry.
func (dv *DrumView) closeFXPanelPortal() {
	if dv.tree != nil {
		dv.portal().Close("fx-panel")
	}
}

// openVolPopupPortal opens the row volume popup through the portal.
func (dv *DrumView) openVolPopupPortal() {
	if dv.tree == nil || dv.volPopup == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID:      "volume-popup",
		Overlay: &sliderPopupPortalOverlay{popup: dv.volPopup, tag: "volume-popup"},
		Modal:   true,
		Scrim:   true, // row volume popup — backdrop dims so modality is visible
		OnClose: func() {
			dv.volPopup.Close()
		},
	})
}

// closeVolPopupPortal closes the row volume popup portal entry.
func (dv *DrumView) closeVolPopupPortal() {
	if dv.tree != nil {
		dv.portal().Close("volume-popup")
	}
}

// openSamplerWheelPortal opens the mobile sampler-knob wheel popup through the portal.
func (p *samplerPanel) openSamplerWheelPortal() {
	dv := p.dv
	if dv.tree == nil || dv.samplerWheelPopup == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID:      "sampler-wheel-popup",
		Overlay: &mobileWheelPopupPortalOverlay{popup: dv.samplerWheelPopup, tag: "sampler-wheel-popup"},
		Modal:   true,
		Scrim:   true, // sampler wheel popup — backdrop dims so modality is visible
		OnClose: func() {
			// Persist on dismiss: Accept commits a pending edit and no-ops when closed.
			dv.samplerWheelPopup.Accept()
		},
	})
}

// closeSamplerWheelPortal closes the sampler wheel popup portal entry.
func (p *samplerPanel) closeSamplerWheelPortal() {
	dv := p.dv
	if dv.tree != nil {
		dv.portal().Close("sampler-wheel-popup")
	}
}

// openEQWheelPortal opens the EQ precision wheel popup through the portal.
func (dv *DrumView) openEQWheelPortal() {
	if dv.tree == nil || dv.eqWheelPopup == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID:      "eq-wheel-popup",
		Overlay: &mobileWheelPopupPortalOverlay{popup: dv.eqWheelPopup, tag: "eq-wheel-popup"},
		Modal:   true,
		Scrim:   true, // EQ wheel popup — backdrop dims so modality is visible
		OnClose: func() {
			// Persist on dismiss: Accept commits a pending edit and no-ops when closed.
			dv.eqWheelPopup.Accept()
		},
	})
}

// closeEQWheelPortal closes the EQ wheel popup portal entry.
func (dv *DrumView) closeEQWheelPortal() {
	if dv.tree != nil {
		dv.portal().Close("eq-wheel-popup")
	}
}

// openMasterVolPopupPortal opens the master volume popup through the portal.
func (dv *DrumView) openMasterVolPopupPortal() {
	if dv.tree == nil || dv.masterVolPopup == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID:      "master-volume-popup",
		Overlay: &sliderPopupPortalOverlay{popup: dv.masterVolPopup, tag: "master-vol-popup"},
		Modal:   true,
		Scrim:   true, // master volume popup — backdrop dims so modality is visible
		OnClose: func() {
			dv.masterVolPopup.Close()
		},
	})
}

// closeMasterVolPopupPortal closes the master volume popup portal entry.
func (dv *DrumView) closeMasterVolPopupPortal() {
	if dv.tree != nil {
		dv.portal().Close("master-volume-popup")
	}
}

// openNamingPortal opens the naming overlay through the portal.
func (dv *DrumView) openNamingPortal() {
	if dv.tree == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID: "naming",
		Overlay: &dvOverlayPortal{
			id:       "naming",
			isOpenFn: func() bool { return dv.IsNamingOpen() },
			rectFn:   func() image.Rectangle { return dv.Bounds },
			hoverFn: func(mx, my int) {
				dv.saveBtn.UpdateHover(mx, my)
			},
			inputFn: func(x, y int, pressed bool) InputResult {
				if dv.saveBtn != nil && dv.saveBtn.HandleInputResult(x, y, pressed) != InputIgnored {
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
					dv.nameBox = dv.newWAVNameBox(box)
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
				if Profile().IsMobile() && mobileInputActive(wavNameMobileInputID) {
					if val, committed, ok := mobileInputPollResult(wavNameMobileInputID); ok {
						if committed {
							if name := strings.TrimSpace(val); name != "" {
								dv.registerInstrument(name)
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
					dv.nameBox = dv.newWAVNameBox(box)
					dv.nameBox.focused = true
				}
				dv.nameBox.Rect = box
				if dv.saveBtn == nil {
					dv.saveBtn = NewButton(i18n.T(i18n.KeySave), UploadBtnStyle, nil)
				}
				dv.saveBtn.SetRect(image.Rect(box.Max.X+10, box.Min.Y, box.Max.X+60, box.Max.Y))
				dv.saveBtn.OnClick = func() {
					name := strings.TrimSpace(dv.nameBox.Value())
					dv.logger.Debugf("[drumview] save instrument pressed name=%q", name)
					if name != "" {
						dv.registerInstrument(name)
					}
				}
				dv.nameBox.Update()
				if isKeyPressed(ebiten.KeyEnter) {
					if name := strings.TrimSpace(dv.nameBox.Value()); name != "" {
						dv.registerInstrument(name)
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
		Scrim: true, // WAV-naming dialog — backdrop dims so modality is visible
		OnClose: func() {
			// Not closeNaming: it would call portal.Close recursively.
			dv.pendingWAV = ""
			dv.nameInput = ""
			dv.nameBox = nil
		},
	})
}

// closeNamingPortal closes the naming portal entry.
func (dv *DrumView) closeNamingPortal() {
	if dv.tree != nil {
		dv.portal().Close("naming")
	}
}

// ── *DrumView forwarding shims (Rung A relocation onto *samplerPanel) ──────

func (dv *DrumView) openSamplerWheelPortal() { dv.sp().openSamplerWheelPortal() }

func (dv *DrumView) closeSamplerWheelPortal() { dv.sp().closeSamplerWheelPortal() }

// wavNameMobileInputID is the native-input id of the WAV-upload naming box.
const wavNameMobileInputID = "wav-name"

func (dv *DrumView) newWAVNameBox(r image.Rectangle) *TextInput {
	f := NewTextField(dv.textFields, TextFieldSpec{ID: wavNameMobileInputID, Owner: "naming", ProxyOnMobile: true}, 32)
	f.Rect = r
	return f
}

// The whole window: the card can be taller than a short drum pane, so it is window-bounded like
// the theme editor and repainted above the divider.
func (dv *DrumView) navMenuBounds() image.Rectangle {
	if dv.game != nil {
		if w := dv.game.windowRect(); !w.Empty() {
			return w
		}
	}
	return dv.Bounds
}

// Extended to the ribbon's bottom, so the card never covers the go-to ghost on the ribbon.
func (dv *DrumView) navMenuAnchor() image.Rectangle {
	a := dv.beatCounterRect
	if dv.timelineZone != nil {
		if bar := dv.timelineZone.timelineBarRect; !bar.Empty() && bar.Max.Y > a.Max.Y {
			a.Max.Y = bar.Max.Y
		}
	}
	return a
}
