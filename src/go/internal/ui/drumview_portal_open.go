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
	// Feed the clamp region so the menu card positions via AnchorPopupRect and
	// never runs off-screen (SubdivMenuProps carries no bounds field).
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

// openNavMenuPortal opens the navigation menu as a first-class portal component
// (compOverlayable), anchored under the beat counter per the DESIGN.md
// menu-placement rule. The tree's compHitHandler delivers press/drag/RELEASE
// natively (so momentary jog buttons pop back and the Beats/Subdiv latch
// resets), the component owns its go-to editor (keyboard-claimed while active),
// and click-outside/Esc/hover are all handled by the tree — identical to the
// subdiv / instrument / colour menus.
func (dv *DrumView) openNavMenuPortal() {
	if dv.tree == nil || dv.navMenuComp == nil {
		return
	}
	dv.navMenuComp.SetScreenBounds(dv.Bounds)
	dv.navMenuComp.SetAnchor(dv.beatCounterRect)
	dv.navMenuComp.Open()
	dv.portal().Open(PortalEntry{
		ID: navMenuPortalID,
		Overlay: &compPortalOverlay{
			comp:     dv.navMenuComp,
			tag:      navMenuPortalID,
			updateFn: dv.navMenuComp.Update, // tick the owned go-to editor
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

// openJumpMenuPortal opens the FF/RW jump-size (×N) menu component through
// the portal. Mirrors openSubdivMenuPortal — jumpMenuComp is a second
// SubdivMenuComponent instance (generic int-options dropdown) reused for
// beat-count options instead of subdivisions. It stacks over the navigation
// menu (its ×N pill is the only opener — see OpenJumpMenu / NavigationPanel).
func (dv *DrumView) openJumpMenuPortal() {
	if dv.tree == nil || dv.jumpMenuComp == nil {
		return
	}
	// Feed the clamp region so the menu card positions via AnchorPopupRect and
	// never runs off-screen (SubdivMenuProps carries no bounds field).
	dv.jumpMenuComp.SetScreenBounds(dv.Bounds)
	anchor := dv.jumpMenuComp.Props().AnchorRect
	if anchor.Empty() && dv.navMenuComp != nil {
		anchor = dv.navMenuComp.Bounds()
	}
	dv.portal().Open(PortalEntry{
		ID:      "jump-menu",
		Overlay: &compPortalOverlay{comp: dv.jumpMenuComp, tag: "jump-menu"},
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
			// Upload button special case: if click-outside was on the
			// upload button, trigger it now.
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
		ID:      "color-wheel",
		Overlay: &compPortalOverlay{comp: dv.colorWheelComp, tag: "color-wheel"},
		Modal:   false,
		// Scrim left off: the color picker (drumview_overlay_color_comp.go) draws
		// its own scrim today; migrating it to the portal scrim is a later phase.
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
	// Ensure the shared scroll component exists/refreshed for the current page so
	// callers (and tests) see a configured MenuScroll the moment the menu opens.
	dv.configureOverflowScroll()
	dv.portal().Open(PortalEntry{
		ID: "overflow-menu",
		Overlay: &dvOverlayPortal{
			id:       "overflow-menu",
			isOpenFn: func() bool { return dv.IsOverflowMenuOpen() },
			rectFn:   func() image.Rectangle { return dv.overflowPopupRect() },
			hoverFn: func(mx, my int) {
				for _, b := range dv.overflowBtns {
					b.UpdateHover(mx, my)
				}
				// NAVIGATION panel buttons (page 0 only) — mirrors overflowBtns
				// so the embedded panel's prev/amount/next/unit/go-to buttons
				// get the same hover-glow feedback as every other menu row.
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
					// Capture while the scroll component holds a deferred tap
					// (tap-vs-scroll disambiguation); otherwise one-shot consume.
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
				// Touch-fling momentum (the inertial half of the shared
				// instrument-menu feel). rebuild is nil: overflowPopupBtns already
				// refreshes every button's rect from the live offset each draw
				// frame, so the advancing offset is picked up without rebuilding
				// the button objects. Runs on every page (the search box, handled
				// by updateTemplateSearch, only exists on page 1).
				if dv.overflowMenuScroll != nil {
					dv.overflowMenuScroll.Tick(nil)
				}
				dv.updateTemplateSearch()
			},
			escapeFn: func() bool { return dv.handleTemplateSearchEscape() },
			// While the menu is open its subtree (overlayTree) owns the
			// keyboard, so the sibling GridTree's arrow-pan/zoom shortcuts
			// yield — the template search caret gets the arrows, not the
			// camera. Mirrors InstrumentMenuComponent.ClaimsKeyboard.
			claimsKeyboardFn: func() bool { return dv.IsOverflowMenuOpen() },
		},
		Modal: false,
		Scrim: true, // overflow menu — backdrop dims so modality is visible
		OnClose: func() {
			// Do non-portal cleanup only (portal removal is handled by
			// the portal system itself — avoid calling closeOverflowMenu
			// which would recursively call portal.Close).
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
				// Smooth: continuous, magnitude-respecting wheel with no cooldown
				// lock — the shared instrument-menu cadence every list menu uses.
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
			},
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
				// Smooth pixel-scroll through the shared engine (~20px/notch,
				// magnitude-respecting, no cooldown), matching every list menu.
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
				// Scrim intentionally off: the FX panel is a docked tool panel
				// the user interacts around (not a modal menu/picker).
				// Touch-fling momentum via the shared driver — same as every menu.
				if dv.fxScroll != nil {
					dv.fxScroll.Tick(func() { dv.buildFXPanel(); dv.refreshFXPortalHitAreas() })
				}
				dv.updateFXAddSearch()
			},
			escapeFn: func() bool { return dv.fxPanelHandleEscape() },
			// While the add-menu search box is focused, the FX panel's subtree
			// owns the keyboard so arrow keys/typing go to the search caret
			// instead of the sibling GridTree's pan/zoom shortcuts. Mirrors
			// the overflow menu's claimsKeyboardFn.
			claimsKeyboardFn: func() bool {
				return dv.fxAddMenuOpen && dv.fxAddSearch != nil && dv.fxAddSearch.Focused()
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

// openSynthWheelPortal opens the mobile synth-control wheel popup through the portal.
func (p *synthPanel) openSynthWheelPortal() {
	dv := p.dv

	if dv.tree == nil || dv.synthWheelPopup == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID:      "synth-wheel-popup",
		Overlay: &mobileWheelPopupPortalOverlay{popup: dv.synthWheelPopup, tag: "synth-wheel-popup"},
		Modal:   true,
		Scrim:   true, // synth wheel popup — backdrop dims so modality is visible
		OnClose: func() {
			// Persist-on-dismiss chokepoint: every dismissal funnels through
			// OnClose (tap-outside via tree CloseTop, CloseAllPopups drain,
			// programmatic close). Accept commits a pending edit; it no-ops when
			// the popup is already closed — Esc's Cancel runs first, so revert
			// is preserved, and Enter/anchor/center-box Accept first too.
			dv.synthWheelPopup.Accept()
		},
	})
}

// closeSynthWheelPortal closes the synth wheel popup portal entry.
func (p *synthPanel) closeSynthWheelPortal() {
	dv := p.dv

	if dv.tree != nil {
		dv.portal().Close("synth-wheel-popup")
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
			// Persist-on-dismiss chokepoint (see synth wheel OnClose). Accept
			// commits a pending edit; no-ops when already closed.
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
			// Persist-on-dismiss chokepoint (see synth wheel OnClose). Accept
			// commits a pending edit; no-ops when already closed.
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
					dv.saveBtn = NewButton(i18n.T(i18n.KeySave), UploadBtnStyle, nil)
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
		Scrim: true, // WAV-naming dialog — backdrop dims so modality is visible
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
		dv.portal().Close("naming")
	}
}

// ── *DrumView forwarding shims (Rung A relocation onto *samplerPanel) ──────

func (dv *DrumView) openSamplerWheelPortal() { dv.sp().openSamplerWheelPortal() }

func (dv *DrumView) closeSamplerWheelPortal() { dv.sp().closeSamplerWheelPortal() }

// ── *DrumView forwarding shims (Batch B relocation onto *synthPanel) ───────

func (dv *DrumView) openSynthWheelPortal() { dv.synp().openSynthWheelPortal() }

func (dv *DrumView) closeSynthWheelPortal() { dv.synp().closeSynthWheelPortal() }
