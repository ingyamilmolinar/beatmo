package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// fxToggleTag is the prefix used to identify toggle buttons in the FX panel.
// The button Text is set to fxToggleTag+"on" or fxToggleTag+"off" so that
// drawFXPanel can render a pill-style toggle switch instead of a checkbox.
const fxToggleTag = "\x00fxtoggle:"

// addEffectButtonStyle draws the "+ Add Effect" button with a dashed border,
// colSurface1 fill, and RadiusMD corners.
type addEffectButtonStyle struct{}

func (addEffectButtonStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	if r.Empty() {
		return
	}
	var fill color.Color = colSurface1
	if pressed {
		fill = adjustColor(fill, -20)
	} else if hovered {
		fill = adjustColor(fill, 12)
	}
	drawRoundedRect(dst, r, fill, RadiusMD, true)
	drawDashedRoundedBorder(dst, r, colBorderMedium, RadiusMD)
}

// drawFXTogglePill draws a pill-style toggle switch in the given rect.
// enabled controls the ON/OFF color scheme; sizing follows the active
// LayoutProfile (FXToggle*() accessors).
//
// This shared helper is also used by the Synth stage-enable pills
// (synth_panel_zone.go), so its default ON tint is left unchanged. The per-row
// insert-FX panel calls drawFXTogglePillCol with colAccent so the FX enable
// toggle obeys the single-accent rule (azure, not iOS-green).
func drawFXTogglePill(dst *ebiten.Image, r image.Rectangle, enabled bool) {
	drawFXTogglePillCol(dst, r, enabled, colPlayGreen)
}

// drawFXTogglePillCol is drawFXTogglePill with an explicit ON-track color so
// individual call sites can pick a role-correct tint without recoloring the
// shared widget for every caller. onCol is used for the track fill when
// enabled; the OFF track stays colSurface3.
func drawFXTogglePillCol(dst *ebiten.Image, r image.Rectangle, enabled bool, onCol color.Color) {
	trackW, trackH := FXToggleTrackW(), FXToggleTrackH()
	thumbD := FXToggleThumbD()

	// Center the track vertically and left-align within the rect.
	cx := r.Min.X + (r.Dx()-trackW)/2
	cy := r.Min.Y + (r.Dy()-trackH)/2
	trackR := image.Rect(cx, cy, cx+trackW, cy+trackH)
	trackRadius := trackH / 2

	// Track color: ON = onCol, OFF = colSurface3.
	var trackCol color.Color
	if enabled {
		trackCol = onCol
	} else {
		trackCol = colSurface3
	}
	drawRoundedRect(dst, trackR, trackCol, trackRadius, true)

	// Thumb: circle centered vertically in the track.
	thumbY := cy + (trackH-thumbD)/2
	var thumbX int
	if enabled {
		thumbX = cx + trackW - thumbD - (trackH-thumbD)/2
	} else {
		thumbX = cx + (trackH-thumbD)/2
	}
	thumbR := image.Rect(thumbX, thumbY, thumbX+thumbD, thumbY+thumbD)
	thumbRadius := thumbD / 2

	// Thumb color: ON = white, OFF = colTextSecondary.
	var thumbCol color.Color
	if enabled {
		thumbCol = genColorBorder
	} else {
		thumbCol = colTextSecondary
	}
	drawRoundedRect(dst, thumbR, thumbCol, thumbRadius, true)
}

// isFXToggleBtn returns whether a button is an FX panel toggle, and its state.
func isFXToggleBtn(btn *Button) (isToggle bool, enabled bool) {
	if btn == nil {
		return false, false
	}
	if strings.HasPrefix(btn.Text, fxToggleTag) {
		return true, btn.Text == fxToggleTag+"on"
	}
	// Legacy fallback for tests that search by "✓" or " ".
	return false, false
}

// toggleFXPanel opens or closes the FX panel for the given row.
func (dv *DrumView) toggleFXPanel(row int) {
	if dv.IsFXPanelOpen() && dv.fxPanelRow == row {
		dv.closeFXPanel()
		return
	}
	// Close all other overlays for mutual exclusivity.
	dv.CloseAllPopups()
	dv.openFXPanel(row)
}

func (dv *DrumView) openFXPanel(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	dv.fxPanelRow = row
	dv.fxScrollOffsetPx = 0
	dv.fxExpandedSlots = map[int]bool{}
	dv.fxScrollTS.Reset()
	dv.fxScrollMaxPx = 0
	dv.buildFXPanel()      // compute correct fxPanelRect FIRST
	dv.openFXPanelPortal() // register portal with correct rect
}

func (dv *DrumView) closeFXPanel() {
	dv.closeFXPanelPortal()
	dv.fxAddMenuOpen = false
	dv.fxPanelBtns = nil
	dv.fxPanelSliders = nil
	dv.fxSliderDragging = false
	dv.fxPanelDeferredTap.Cancel()
	dv.fxScrollOffsetPx = 0
	dv.fxScrollTS.Reset()
	dv.fxScrollMaxPx = 0
	dv.fxPanelRect = image.Rectangle{}
}

// OpenFXPanel opens the per-row insert-effects panel programmatically.
// Used by the screenshot harness, scene catalog, and tests.
func (dv *DrumView) OpenFXPanel(row int) {
	if dv.IsFXPanelOpen() && dv.fxPanelRow == row {
		return
	}
	dv.CloseAllPopups()
	dv.openFXPanel(row)
}

// fxPanelAccent returns the FX panel's owning-row instrument color — the accent
// the enable-toggle pill and other FX highlights tint to, so the effects panel
// reads as owned by the instrument it edits. Falls back to the azure chrome
// accent when the row has no color.
func (dv *DrumView) fxPanelAccent() color.Color {
	if dv.fxPanelRow >= 0 && dv.fxPanelRow < len(dv.Rows) {
		if c := dv.Rows[dv.fxPanelRow].Color; c != nil {
			return c
		}
	}
	return colAccent
}

// fxPanelCloseBtn returns the FX panel's close (×) button by identity, or nil
// when the panel is closed. Tracked as a field so retrieval never depends on
// the button being the last entry in fxPanelBtns.
func (dv *DrumView) fxPanelCloseBtn() *Button { return dv.fxPanelCloseButton }

// CloseFXPanel closes the FX panel.
func (dv *DrumView) CloseFXPanel() { dv.closeFXPanel() }

// FXPanelRow returns the row index of the currently open FX panel, or -1.
func (dv *DrumView) FXPanelRow() int {
	if !dv.IsFXPanelOpen() {
		return -1
	}
	return dv.fxPanelRow
}

// FXPanelRect returns the screen-space rectangle of the currently open FX
// panel and ok=true. Returns the zero rectangle and ok=false when no FX
// panel is open.
func (dv *DrumView) FXPanelRect() (image.Rectangle, bool) {
	if !dv.IsFXPanelOpen() {
		return image.Rectangle{}, false
	}
	if dv.fxPanelRect.Empty() {
		return image.Rectangle{}, false
	}
	return dv.fxPanelRect, true
}

// propagateFXSliderValue pushes the current slider value to the audio engine.
func (dv *DrumView) propagateFXSliderValue(idx int) {
	if idx >= len(dv.fxPanelSliders) || idx >= len(dv.fxSliderBindings) {
		return
	}
	sl := dv.fxPanelSliders[idx]
	b := dv.fxSliderBindings[idx]
	actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	instID := dv.Rows[dv.fxPanelRow].Instrument
	audio.SetInsertEffectParam(instID, b.slotIndex, b.paramName, actual)
}

// commitFXSlider emits + records an insert-FX param change once, at release.
func (dv *DrumView) commitFXSlider(idx int) {
	if idx < 0 || idx >= len(dv.fxSliderBindings) {
		return
	}
	b := dv.fxSliderBindings[idx]
	sl := dv.fxPanelSliders[idx]
	actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	instID := dv.Rows[dv.fxPanelRow].Instrument
	emitInsertEffectParam(instID, b.slotIndex, b.paramName, actual)
	dv.recordUndoStep(hooks.EventInsertEffectParam)
}

// effectTypeName returns the display name for an effect type from the registry.
func effectTypeName(t audio.EffectType) string {
	regs := audio.EffectRegistrations()
	if r, ok := regs[t]; ok {
		return r.DisplayName
	}
	return string(t)
}

// effectTypeOrder returns all effect types in registration order.
func effectTypeOrder() []audio.EffectType {
	return audio.EffectTypeOrder()
}

// fxSliderBinding tracks which effect param a slider controls.
type fxSliderBinding struct {
	slotIndex int
	paramName string
	def       audio.EffectParamDef
}

// fxShowParams returns whether parameter sliders should be shown for a given
// effect slot. On desktop, enabled effects always show params. On mobile,
// effects must also be expanded.
func (dv *DrumView) fxShowParams(slotIdx int, enabled bool) bool {
	if !enabled {
		return false
	}
	if !Profile().UseBottomSheet {
		return true
	}
	return dv.fxExpandedSlots[slotIdx]
}

// buildFXPanel computes the layout and creates buttons/sliders for the FX panel.
func (dv *DrumView) buildFXPanel() {
	row := dv.fxPanelRow
	if row < 0 || row >= len(dv.Rows) {
		dv.closeFXPanelPortal()
		return
	}

	// Anchor fallback: prefer FX button, fall back to row label (on mobile
	// the FX column has zero width).
	anchor := image.Rectangle{}
	if row < len(dv.rowFXBtns()) {
		anchor = dv.rowFXBtns()[row].Rect()
	}
	if anchor.Empty() && row < len(dv.rowLabels()) {
		anchor = dv.rowLabels()[row].Rect()
	}

	r := dv.Rows[row]
	instID := r.Instrument
	effects := audio.GetInsertEffects(instID)

	// Panel dimensions — scaled up on mobile for touch targets.
	var panelW, lineH, headerH, paramH, footerH int
	mobile := Profile().IsMobile()
	if mobile {
		panelW = dv.Bounds.Dx() - 32 // near full-width with 16px margin each side
		if panelW < 250 {
			panelW = 250
		}
		lineH = touchMinTargetPx      // 44px
		headerH = touchMinTargetPx    // 44px
		paramH = touchMinTargetPx - 4 // 40px
		footerH = touchMinTargetPx    // 44px
	} else {
		panelW = 340
		lineH = 28
		headerH = 32
		paramH = 26
		footerH = 32
	}

	// Compute dynamic slider left offset based on longest visible param label.
	if !mobile {
		maxLabelW := 0
		for _, e := range effects {
			if e.Enabled {
				cat := audio.InsertEffectCatalog()
				if defs, ok := cat[e.Type]; ok {
					for _, def := range defs {
						val := e.Params[def.Name]
						label := fxParamLabel(def.Name, val, def.Unit)
						if w := TextWidth(label); w > maxLabelW {
							maxLabelW = w
						}
					}
				}
			}
		}
		dv.fxSliderLeft = maxLabelW + 16 // 8px left pad + 8px gap
		if dv.fxSliderLeft < 90 {
			dv.fxSliderLeft = 90
		}
		// Ensure panel is wide enough for slider to have at least 120px usable track.
		// Sliders are indented by SpaceXL and use label-above, so they need less
		// horizontal space for the "XX%" label — 120px track minimum suffices.
		minPanelW := SpaceXL + dv.fxSliderLeft + 120 + 8
		if panelW < minPanelW {
			panelW = minPanelW
		}
		if panelW > dv.Bounds.Dx()-16 {
			panelW = dv.Bounds.Dx() - 16
		}
	}

	// Compute total content height (including header and footer).
	contentH := headerH
	for slotIdx, e := range effects {
		contentH += lineH
		if dv.fxShowParams(slotIdx, e.Enabled) {
			cat := audio.InsertEffectCatalog()
			if defs, ok := cat[e.Type]; ok {
				contentH += len(defs) * paramH
			}
		}
	}
	if dv.fxAddMenuOpen {
		contentH += len(effectTypeOrder())*lineH + lineH // 6 type buttons + cancel
	} else {
		contentH += footerH
	}
	if contentH < 80 {
		contentH = 80
	}

	// Limit panel height to available space.
	maxH := dv.Bounds.Dy() - 20
	panelH := contentH
	if panelH > maxH {
		panelH = maxH
	}

	// Compute scroll parameters.
	// The scrollable area is everything between the fixed header and the panel bottom.
	scrollContentH := contentH - headerH
	viewportH := panelH - headerH
	dv.fxScrollMaxPx = scrollContentH - viewportH
	if dv.fxScrollMaxPx < 0 {
		dv.fxScrollMaxPx = 0
	}
	if dv.fxScrollOffsetPx > dv.fxScrollMaxPx {
		dv.fxScrollOffsetPx = dv.fxScrollMaxPx
	}
	if dv.fxScrollOffsetPx < 0 {
		dv.fxScrollOffsetPx = 0
	}

	// Position: prefer above the anchor, fall back to below
	wantY := anchor.Min.Y - panelH
	if wantY < dv.Bounds.Min.Y {
		wantY = anchor.Max.Y
	}
	if wantY+panelH > dv.Bounds.Max.Y {
		wantY = dv.Bounds.Max.Y - panelH
	}
	if wantY < dv.Bounds.Min.Y {
		wantY = dv.Bounds.Min.Y
	}

	var wantX int
	if mobile {
		wantX = (dv.Bounds.Min.X + dv.Bounds.Max.X - panelW) / 2
	} else {
		wantX = anchor.Min.X - panelW + anchor.Dx()
	}
	if wantX < dv.Bounds.Min.X {
		wantX = dv.Bounds.Min.X
	}
	if wantX+panelW > dv.Bounds.Max.X {
		wantX = dv.Bounds.Max.X - panelW
	}

	dv.fxPanelRect = image.Rect(wantX, wantY, wantX+panelW, wantY+panelH)
	dv.fxViewportRect = image.Rect(wantX, wantY+headerH, wantX+panelW, wantY+panelH)

	// Build buttons and sliders
	dv.fxPanelBtns = nil
	dv.fxPanelCloseButton = nil
	dv.fxPanelSliders = nil
	dv.fxSliderBindings = nil

	// Scrollable content starts at header bottom, offset by scroll.
	y := dv.fxPanelRect.Min.Y + headerH - dv.fxScrollOffsetPx
	x := dv.fxPanelRect.Min.X
	w := panelW

	for slotIdx, e := range effects {
		si := slotIdx
		enabled := e.Enabled

		// Button sizing: wider on mobile for touch targets.
		// toggleBtnW = pill track width + SpaceSM padding so the hit area
		// extends slightly past the pill on either side.
		toggleBtnW := FXToggleTrackW() + SpaceSM
		btnW := 24
		btnGap := 3
		if mobile {
			btnW = 36
			btnGap = 4
		}

		// Right-cluster inset: when a scrollbar is present it occupies the
		// right edge, so the reorder/remove controls are inset by its width
		// (plus a small gap) to keep the × off the scrollbar. Bug: previously
		// the remove × sat at x+w-btnW-4 and overlapped the scrollbar thumb.
		rightInset := 4
		if dv.fxScrollMaxPx > 0 {
			rightInset += ScrollbarStyleForPlatform().Width + SpaceXS
		}

		// Skip creating buttons for rows entirely outside viewport.
		rowTop := y
		rowBot := y + lineH
		inView := rowBot > dv.fxViewportRect.Min.Y && rowTop < dv.fxViewportRect.Max.Y

		if inView {
			// Toggle button — tagged text so drawFXPanel renders a pill switch.
			toggleText := fxToggleTag + "on"
			if !enabled {
				toggleText = fxToggleTag + "off"
			}
			toggle := NewButton(toggleText, InstButtonStyle, nil)
			toggle.SetRect(image.Rect(x+4, y+2, x+4+toggleBtnW, y+lineH-2))
			toggle.OnClick = func() {
				audio.ToggleInsertEffect(instID, si, !enabled)
				emitInsertEffectToggled(instID, si, !enabled)
				dv.recordUndoStep(hooks.EventInsertEffectToggled)
				dv.syncFXToRow(row)
				dv.buildFXPanel()
				dv.refreshFXPortalHitAreas()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, toggle)

			// Expand/collapse button on mobile (enabled effects only).
			// Uses chevron icons per DESIGN.md §5c (no raw Unicode in chrome).
			if mobile && enabled {
				expBtn := NewButton("", InstButtonStyle, nil)
				if dv.fxExpandedSlots[si] {
					expBtn.Icon = string(IconChevronDown)
				} else {
					expBtn.Icon = string(IconChevronRight)
				}
				expBtn.IconColor = colTextSecondary
				expX := x + 4 + toggleBtnW + btnGap
				expBtn.SetRect(image.Rect(expX, y+2, expX+btnW, y+lineH-2))
				expBtn.OnClick = func() {
					if dv.fxExpandedSlots == nil {
						dv.fxExpandedSlots = map[int]bool{}
					}
					dv.fxExpandedSlots[si] = !dv.fxExpandedSlots[si]
					dv.buildFXPanel()
					dv.refreshFXPortalHitAreas()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, expBtn)
			}

			// Move up button — IconChevronUp per §5c.
			if slotIdx > 0 {
				up := NewButton("", InstButtonStyle, nil)
				up.Icon = string(IconChevronUp)
				up.IconColor = colTextSecondary
				upX := x + w - 3*(btnW+btnGap) - rightInset + btnGap
				up.SetRect(image.Rect(upX, y+2, upX+btnW, y+lineH-2))
				up.OnClick = func() {
					audio.MoveInsertEffect(instID, si, si-1)
					emitInsertEffectMoved(instID, si, si-1)
					dv.recordUndoStep(hooks.EventInsertEffectMoved)
					dv.syncFXToRow(row)
					dv.buildFXPanel()
					dv.refreshFXPortalHitAreas()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, up)
			}

			// Move down button — IconChevronDown per §5c (icon economy: chevrons reused for reorder).
			if slotIdx < len(effects)-1 {
				down := NewButton("", InstButtonStyle, nil)
				down.Icon = string(IconChevronDown)
				down.IconColor = colTextSecondary
				downX := x + w - 2*(btnW+btnGap) - rightInset + btnGap
				down.SetRect(image.Rect(downX, y+2, downX+btnW, y+lineH-2))
				down.OnClick = func() {
					audio.MoveInsertEffect(instID, si, si+1)
					emitInsertEffectMoved(instID, si, si+1)
					dv.recordUndoStep(hooks.EventInsertEffectMoved)
					dv.syncFXToRow(row)
					dv.buildFXPanel()
					dv.refreshFXPortalHitAreas()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, down)
			}

			// Remove button — IconClose per §5c (no raw "✕").
			remove := NewButton("", InstButtonStyle, nil)
			remove.Icon = string(IconClose)
			// Destructive tint (colError) distinguishes the remove control
			// from the neutral collapse/reorder chevrons (colTextSecondary).
			remove.IconColor = colError
			removeX := x + w - btnW - rightInset
			remove.SetRect(image.Rect(removeX, y+2, removeX+btnW, y+lineH-2))
			remove.OnClick = func() {
				audio.RemoveInsertEffect(instID, si)
				emitInsertEffectRemoved(instID, si)
				dv.syncFXToRow(row)
				dv.buildFXPanel()
				dv.refreshFXPortalHitAreas()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, remove)
		}

		y += lineH

		// Parameter sliders (only if should be shown)
		// Indent param rows under their effect header for visual grouping.
		paramIndent := SpaceXL // 16px
		if dv.fxShowParams(si, e.Enabled) {
			cat := audio.InsertEffectCatalog()
			if defs, ok := cat[e.Type]; ok {
				for _, def := range defs {
					slTop := y
					slBot := y + paramH
					slInView := slBot > dv.fxViewportRect.Min.Y && slTop < dv.fxViewportRect.Max.Y

					if slInView {
						val := e.Params[def.Name]
						norm := (val - def.Min) / (def.Max - def.Min)
						if norm < 0 {
							norm = 0
						}
						if norm > 1 {
							norm = 1
						}
						sl := NewSlider(norm)
						sliderLeft := x + paramIndent + dv.fxSliderLeft
						if mobile {
							sliderLeft = x + paramIndent + 100
						}
						sl.SetRect(image.Rect(sliderLeft, y+2, x+w-4, y+paramH-2))
						// FX panel sliders: thicker track. The percent label is
						// suppressed because the param row already shows the
						// raw value via fxParamLabel ("Drive: 8.19"); the
						// previous "80%" duplicate was redundant.
						sl.TrackH = 6
						sl.SuppressLabel = true
						// Tint the rail with the owning row's instrument color
						// so the FX sliders match the instrument, not a fixed
						// light-blue.
						if dv.fxPanelRow >= 0 && dv.fxPanelRow < len(dv.Rows) {
							sl.FillCol = rowShadeToRGBA(rowToggleBaseRGBA(dv.Rows[dv.fxPanelRow].Color))
						}
						dv.fxPanelSliders = append(dv.fxPanelSliders, sl)
						dv.fxSliderBindings = append(dv.fxSliderBindings, fxSliderBinding{
							slotIndex: si,
							paramName: def.Name,
							def:       def,
						})
					}
					y += paramH
				}
			}
		}
	}

	// Footer: effect type picker or "Add Effect" button
	if dv.fxAddMenuOpen {
		for _, et := range effectTypeOrder() {
			etCopy := et
			name := effectTypeName(et)
			fTop := y
			fBot := y + lineH
			fInView := fBot > dv.fxViewportRect.Min.Y && fTop < dv.fxViewportRect.Max.Y
			if fInView {
				btn := NewButton(name, InstButtonStyle, nil)
				btn.SetRect(image.Rect(x+4, y+2, x+w-4, y+lineH-2))
				btn.OnClick = func() {
					slot := audio.AddInsertEffect(instID, etCopy, nil)
					emitInsertEffectAdded(instID, slot, string(etCopy))
					dv.syncFXToRow(row)
					dv.fxAddMenuOpen = false
					dv.buildFXPanel()
					dv.refreshFXPortalHitAreas()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, btn)
			}
			y += lineH
		}
		cTop := y
		cBot := y + lineH
		cInView := cBot > dv.fxViewportRect.Min.Y && cTop < dv.fxViewportRect.Max.Y
		if cInView {
			cancelBtn := NewButton(i18n.T(i18n.KeyCancel), DisabledButtonStyle, nil)
			cancelBtn.SetRect(image.Rect(x+4, y+2, x+w-4, y+lineH-2))
			cancelBtn.OnClick = func() {
				dv.fxAddMenuOpen = false
				dv.buildFXPanel()
				dv.refreshFXPortalHitAreas()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, cancelBtn)
		}
	} else {
		aTop := y
		aBot := y + footerH
		aInView := aBot > dv.fxViewportRect.Min.Y && aTop < dv.fxViewportRect.Max.Y
		if aInView {
			addBtn := NewButton(i18n.T(i18n.KeyAddEffect), addEffectButtonStyle{}, nil)
			addBtn.TextColor = colTextSecondary
			addBtn.SetRect(image.Rect(x+4, y+4, x+w-4, y+footerH-4))
			addBtn.OnClick = func() {
				dv.fxAddMenuOpen = true
				dv.fxScrollOffsetPx = 0 // reset scroll when opening add menu
				dv.buildFXPanel()
				dv.refreshFXPortalHitAreas()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, addBtn)
		}
	}

	// Close button at top-right (fixed, not scrolled)
	closeR := closeButtonRect(dv.fxPanelRect, SpaceXS)
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeFXPanel() })
	closeB.Icon = "close"
	closeB.IconColor = closeIconColor()
	closeB.SetRect(closeR)
	closeB.ConsumeOnPress = true
	dv.fxPanelCloseButton = closeB
	dv.fxPanelBtns = append(dv.fxPanelBtns, closeB)
}

// refreshFXPortalHitAreas immediately updates the portal's hit areas for the
// FX panel after a buildFXPanel() call changes geometry mid-frame.
func (dv *DrumView) refreshFXPortalHitAreas() {
	if dv.tree != nil && dv.tree.Portal().Has("fx-panel") {
		dv.tree.Portal().RefreshEntry("fx-panel")
	}
}

// syncFXToRow copies the current audio insert chain back to the DrumRow model.
func (dv *DrumView) syncFXToRow(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	dv.Rows[row].Effects = audio.GetInsertEffects(dv.Rows[row].Instrument)
}

// drawFXPanel renders the FX panel overlay.
func (dv *DrumView) drawFXPanel(dst *ebiten.Image) {
	if !dv.IsFXPanelOpen() {
		return
	}
	r := dv.fxPanelRect
	if r.Empty() {
		return
	}
	row := dv.fxPanelRow
	if row < 0 || row >= len(dv.Rows) {
		return
	}

	// Background
	drawScrim(dst)
	drawPanel(dst, r)

	// Platform-aware sizes for draw consistency with buildFXPanel.
	var lineH, headerH, paramH int
	mobile := Profile().IsMobile()
	if mobile {
		lineH = touchMinTargetPx
		headerH = touchMinTargetPx
		paramH = touchMinTargetPx - 4
	} else {
		lineH = 28
		headerH = 32
		paramH = 26
	}

	// Header (fixed, not scrolled)
	headerR := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+headerH)
	drawRoundedRect(dst, headerR, genColorSliderTrackFill, popupCornerRadius(), true)
	title := fmt.Sprintf("FX: %s", dv.Rows[row].Name)
	if len(title) > 30 {
		title = title[:30]
	}
	textY := headerR.Min.Y + (headerH-12)/2
	DrawTextAt(dst, title, headerR.Min.X+8, textY)

	// Draw effect names (scrollable content)
	effects := audio.GetInsertEffects(dv.Rows[row].Instrument)
	y := r.Min.Y + headerH - dv.fxScrollOffsetPx

	paramIndent := SpaceXL // 16px indent for param labels

	// Right edge of the bounded effect card: leave room for the right-cluster
	// controls + scrollbar so the card never runs edge-to-edge under the
	// reorder/remove buttons (bug: controls stretched the full panel width).
	cardRight := r.Max.X - 4
	if dv.fxScrollMaxPx > 0 {
		cardRight -= ScrollbarStyleForPlatform().Width + SpaceXS
	}

	for slotIdx, e := range effects {
		rowTop := y
		rowBot := y + lineH
		inView := rowBot > dv.fxViewportRect.Min.Y && rowTop < dv.fxViewportRect.Max.Y

		// Whether this effect's params are shown inline (expanded) or summarized.
		paramsShown := dv.fxShowParams(slotIdx, e.Enabled)

		if inView {
			// Bounded card behind the header row groups the per-effect controls
			// (toggle · name · reorder · remove) so they read as one unit
			// instead of stretching the full panel width.
			cardR := image.Rect(r.Min.X+4, y+1, cardRight, y+lineH-1)
			if cardR.Dx() > 0 {
				drawRoundedRect(dst, cardR, colSurface2, RadiusSM, true)
			}

			name := effectTypeName(e.Type)
			if name == "" {
				name = string(e.Type)
			}
			if !e.Enabled {
				name = "(" + name + ")"
			}
			// Name offset accounts for wider pill toggle (36px desktop, 44px mobile).
			nameX := r.Min.X + 40
			if mobile {
				nameX = r.Min.X + 48
				if e.Enabled {
					nameX = r.Min.X + 88 // room for expand button
				}
			}
			DrawTextAt(dst, name, nameX, y+(lineH-12)/2)

			// Collapsed info scent: when params are not shown inline, render a
			// one-line summary ("Drive 8.2 · Warmth 0.6") in on-surface-muted at
			// caption scale so a collapsed effect still communicates its state.
			if !paramsShown {
				if summary := fxParamSummary(e); summary != "" {
					nameW := TextWidth(name)
					sumX := nameX + nameW + SpaceSM
					maxSumW := cardRight - sumX - 4
					if maxSumW > 24 {
						summary = clipTextToWidth(summary, maxSumW)
						DrawTextColorAtScale(dst, summary, sumX, y+(lineH-9)/2, colTextSecondary, fxSummaryScale)
					}
				}
			}
		}
		y += lineH

		if paramsShown {
			cat := audio.InsertEffectCatalog()
			if defs, ok := cat[e.Type]; ok {
				for _, def := range defs {
					slTop := y
					slBot := y + paramH
					slInView := slBot > dv.fxViewportRect.Min.Y && slTop < dv.fxViewportRect.Max.Y

					if slInView {
						val := e.Params[def.Name]
						label := fxParamLabel(def.Name, val, def.Unit)
						maxLabelW := dv.fxSliderLeft - 8
						if mobile {
							maxLabelW = 92
						}
						label = clipTextToWidth(label, maxLabelW)
						DrawTextAt(dst, label, r.Min.X+8+paramIndent, y+(paramH-12)/2)
					}
					y += paramH
				}
			}
		}
	}

	// Draw buttons (skip those outside viewport, except the close button which
	// is pinned to the header).
	for _, btn := range dv.fxPanelBtns {
		if btn == nil {
			continue
		}
		// Close button is fixed (not scrolled) — matched by identity, not slice tail.
		if btn == dv.fxPanelCloseButton {
			btn.Draw(dst)
			continue
		}
		br := btn.Rect()
		if br.Max.Y > dv.fxViewportRect.Min.Y && br.Min.Y < dv.fxViewportRect.Max.Y {
			// Render pill-style toggle for effect enable/disable buttons.
			if isToggle, enabled := isFXToggleBtn(btn); isToggle {
				// FX enable toggle tints to the owning instrument's color so the
				// effects panel reads as owned by that instrument (NOT the
				// shared green default used by the synth stage pills).
				drawFXTogglePillCol(dst, br, enabled, dv.fxPanelAccent())
			} else {
				btn.Draw(dst)
			}
		}
	}

	// Draw sliders (skip those outside viewport)
	for _, sl := range dv.fxPanelSliders {
		if sl == nil {
			continue
		}
		sr := sl.Rect()
		if sr.Max.Y > dv.fxViewportRect.Min.Y && sr.Min.Y < dv.fxViewportRect.Max.Y {
			sl.Draw(dst)
		}
	}

	// Draw header on top of scrolled content to cover any overflow
	drawRoundedRect(dst, headerR, genColorSliderTrackFill, popupCornerRadius(), true)
	DrawTextAt(dst, title, headerR.Min.X+8, textY)
	// Redraw close button on top of header
	if closeBtn := dv.fxPanelCloseBtn(); closeBtn != nil {
		closeBtn.Draw(dst)
	}

	// Scrollbar
	if dv.fxScrollMaxPx > 0 {
		style := ScrollbarStyleForPlatform()
		vpRect := dv.fxViewportRect
		trackRect := image.Rect(
			vpRect.Max.X-style.Width,
			vpRect.Min.Y,
			vpRect.Max.X,
			vpRect.Max.Y,
		)
		vpH := vpRect.Dy()
		totalH := vpH + dv.fxScrollMaxPx
		thumbH := vpH * vpH / totalH
		if thumbH < style.MinThumbH {
			thumbH = style.MinThumbH
		}
		thumbY := trackRect.Min.Y
		if dv.fxScrollMaxPx > 0 {
			thumbY += dv.fxScrollOffsetPx * (vpH - thumbH) / dv.fxScrollMaxPx
		}
		thumbRect := image.Rect(trackRect.Min.X, thumbY, trackRect.Max.X, thumbY+thumbH)
		style.Draw(dst, trackRect, thumbRect)
	}
}

// handleFXPanelInput processes mouse input for the FX panel.
// Returns true if input was consumed.
func (dv *DrumView) handleFXPanelInput(mx, my int, left bool) bool {
	if !dv.IsFXPanelOpen() {
		return false
	}

	p := image.Pt(mx, my)

	// Phase 0: Continue active slider drag (both platforms).
	// This runs before click-outside or deferred tap so a drag that leaves the
	// panel rect is not interrupted.
	if dv.fxSliderDragging {
		idx := dv.fxSliderDragIdx
		if idx < len(dv.fxPanelSliders) {
			sl := dv.fxPanelSliders[idx]
			sl.HandleInputResult(mx, my, left)
			dv.propagateFXSliderValue(idx)
		}
		if !left {
			dv.fxSliderDragging = false
			dv.commitFXSlider(idx)
		}
		return true
	}

	// Phase 1: Mobile — touch scroll + sliders + deferred tap coordination.
	if Profile().IsMobile() {
		// Continue committed scroll gesture.
		if dv.fxScrollTS.ScrollingCommitted() {
			if left {
				delta := dv.fxScrollTS.Move(mx, my)
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
			} else {
				// Touch ended — capture velocity for momentum.
				dv.fxScrollTS.End()
			}
			return true
		}

		// Touch is being tracked but hasn't committed to scroll yet.
		if dv.fxScrollTS.Active() {
			if left {
				delta := dv.fxScrollTS.Move(mx, my)
				if dv.fxScrollTS.ScrollingCommitted() {
					// Scroll just committed — cancel deferred tap.
					dv.fxPanelDeferredTap.Cancel()
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
					return true
				}
				// Not committed yet — keep tracking.
				return true
			}
			// Touch ended before committing to scroll → fire as tap.
			dv.fxScrollTS.End()
			if dv.fxPanelDeferredTap.Active() {
				dv.fxPanelDeferredTap.End(dv.fireFXPanelTapAt)
			}
			return true
		}

		// Check if touch-begin lands on a slider → start drag directly.
		if left && !dv.fxPanelDeferredTap.Active() {
			for i, sl := range dv.fxPanelSliders {
				if sl != nil && p.In(sl.Rect()) {
					sl.HandleInputResult(mx, my, left)
					dv.fxSliderDragging = true
					dv.fxSliderDragIdx = i
					dv.propagateFXSliderValue(i)
					return true
				}
			}
		}

		// New touch begin in viewport: start both deferred tap and scroll tracking.
		if left && !dv.fxPanelDeferredTap.Active() && p.In(dv.fxPanelRect) {
			if dv.fxScrollMaxPx > 0 && p.In(dv.fxViewportRect) {
				// Start scroll tracking alongside deferred tap.
				dv.fxScrollTS.Begin(mx, my)
			}
			if dv.fxPanelDeferredTap.Begin(mx, my) {
				return true
			}
		}

		// Active deferred tap waiting for release (no scroll committed).
		if dv.fxPanelDeferredTap.Active() {
			if !left {
				dv.fxPanelDeferredTap.End(dv.fireFXPanelTapAt)
				dv.fxScrollTS.Reset()
			}
			return true
		}
	}

	// Phase 2: Desktop button handling.
	for _, btn := range dv.fxPanelBtns {
		if btn != nil && btn.HandleInputResult(mx, my, left) != InputIgnored {
			return true
		}
	}

	// Phase 3: Desktop slider handling (enhanced with drag tracking).
	for i, sl := range dv.fxPanelSliders {
		if sl != nil && sl.HandleInputResult(mx, my, left) != InputIgnored {
			if sl.dragging {
				dv.fxSliderDragging = true
				dv.fxSliderDragIdx = i
			}
			dv.propagateFXSliderValue(i)
			return true
		}
	}

	// Phase 4: Consume all input within panel rect.
	if p.In(dv.fxPanelRect) {
		return true
	}

	return false
}

// fireFXPanelTapAt finds the FX panel button at (x, y) and fires it.
// Iterates in reverse: the close button is appended last and has highest
// z-order, so it must be checked before buttons it spatially overlaps.
func (dv *DrumView) fireFXPanelTapAt(x, y int) {
	pt := image.Pt(x, y)
	for i := len(dv.fxPanelBtns) - 1; i >= 0; i-- {
		btn := dv.fxPanelBtns[i]
		if btn != nil && pt.In(btn.Rect()) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}
	// Check sliders: map position to value update.
	for i, sl := range dv.fxPanelSliders {
		if sl != nil && pt.In(sl.Rect()) {
			slR := sl.Rect()
			norm := float64(x-slR.Min.X) / float64(slR.Dx())
			if norm < 0 {
				norm = 0
			}
			if norm > 1 {
				norm = 1
			}
			sl.Value = norm
			if i < len(dv.fxSliderBindings) {
				b := dv.fxSliderBindings[i]
				actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
				instID := dv.Rows[dv.fxPanelRow].Instrument
				audio.SetInsertEffectParam(instID, b.slotIndex, b.paramName, actual)
				emitInsertEffectParam(instID, b.slotIndex, b.paramName, actual)
				dv.recordUndoStep(hooks.EventInsertEffectParam)
			}
			return
		}
	}
}

// fxParamLabel formats a parameter label with compact number display.
func fxParamLabel(name string, val float64, unit string) string {
	var valStr string
	switch {
	case val >= 1000:
		valStr = fmt.Sprintf("%.0f", val)
	case val >= 10:
		valStr = fmt.Sprintf("%.1f", val)
	case val == float64(int(val)):
		valStr = fmt.Sprintf("%.0f", val)
	default:
		valStr = fmt.Sprintf("%.2f", val)
	}
	if unit != "" && unit[0] != '%' {
		return fmt.Sprintf("%s: %s %s", capitalize(name), valStr, unit)
	}
	return fmt.Sprintf("%s: %s%s", capitalize(name), valStr, unit)
}

func capitalize(s string) string {
	return audio.PrettyName(s)
}

// fxSummaryScale is the text scale for the collapsed-row param summary —
// caption-sized so it reads as secondary info under the effect name.
const fxSummaryScale = 0.8

// fxParamSummary builds a compact one-line summary of an effect's current
// parameter values for display on a collapsed row, e.g. "Drive 8.2 · Warmth
// 0.6". Values are read live from the slot's Params (falling back to the
// catalog default when absent) so the summary always reflects the CURRENT
// chain state. Returns "" when the effect has no catalog params.
func fxParamSummary(e audio.EffectSlot) string {
	defs, ok := audio.InsertEffectCatalog()[e.Type]
	if !ok || len(defs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, def := range defs {
		val, has := e.Params[def.Name]
		if !has {
			val = def.Default
		}
		if i > 0 {
			b.WriteString(" · ") // middle dot separator
		}
		b.WriteString(capitalize(def.Name))
		b.WriteByte(' ')
		b.WriteString(fxFormatVal(val))
	}
	return b.String()
}

// fxFormatVal renders a parameter value compactly (matches fxParamLabel's
// number formatting but without the name/unit).
func fxFormatVal(val float64) string {
	switch {
	case val >= 1000:
		return fmt.Sprintf("%.0f", val)
	case val >= 10:
		return fmt.Sprintf("%.1f", val)
	case val == float64(int(val)):
		return fmt.Sprintf("%.0f", val)
	default:
		return fmt.Sprintf("%.1f", val)
	}
}
