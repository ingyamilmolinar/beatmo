package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// toggleFXPanel opens or closes the FX panel for the given row.
func (dv *DrumView) toggleFXPanel(row int) {
	if dv.fxPanelOpen && dv.fxPanelRow == row {
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
	dv.fxPanelOpen = true
	dv.fxPanelOpenSeq = dv.updateSeq
	dv.fxScrollOffsetPx = 0
	dv.fxExpandedSlots = map[int]bool{}
	dv.fxScrollTS.Reset()
	dv.fxScrollMaxPx = 0
	SuppressClicksUntilMouseUp()
	dv.buildFXPanel()
}

func (dv *DrumView) closeFXPanel() {
	dv.fxPanelOpen = false
	dv.fxAddMenuOpen = false
	dv.fxPanelBtns = nil
	dv.fxPanelSliders = nil
	dv.fxSliderDragging = false
	dv.fxPanelDeferredTap.Cancel()
	dv.fxScrollOffsetPx = 0
	dv.fxScrollTS.Reset()
	dv.fxScrollMaxPx = 0
	// Clear the opening-click suppression. Callers that close the panel in
	// response to a click-outside (handleFXPanelInput Phase 2, OverlayStack)
	// must call SuppressClicksUntilMouseUp() AFTER this to prevent
	// click-through to elements underneath.
	suppressClicksUntilRelease = false
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

// Effect type display names
var effectTypeNames = map[audio.EffectType]string{
	audio.EffectDistortion: "Distortion",
	audio.EffectDelay:      "Delay",
	audio.EffectReverb:     "Reverb",
	audio.EffectChorus:     "Chorus",
	audio.EffectBitcrusher: "Bitcrusher",
	audio.EffectFilter:     "Filter",
}

var effectTypeOrder = []audio.EffectType{
	audio.EffectDistortion,
	audio.EffectDelay,
	audio.EffectReverb,
	audio.EffectChorus,
	audio.EffectBitcrusher,
	audio.EffectFilter,
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
	if !isSmallScreen() {
		return true
	}
	return dv.fxExpandedSlots[slotIdx]
}

// buildFXPanel computes the layout and creates buttons/sliders for the FX panel.
func (dv *DrumView) buildFXPanel() {
	row := dv.fxPanelRow
	if row < 0 || row >= len(dv.Rows) {
		dv.fxPanelOpen = false
		return
	}

	// Anchor fallback: prefer FX button, fall back to row label (on mobile
	// the FX column has zero width).
	anchor := image.Rectangle{}
	if row < len(dv.rowFXBtns) {
		anchor = dv.rowFXBtns[row].Rect()
	}
	if anchor.Empty() && row < len(dv.rowLabels) {
		anchor = dv.rowLabels[row].Rect()
	}

	r := dv.Rows[row]
	instID := r.Instrument
	effects := audio.GetInsertEffects(instID)

	// Panel dimensions — scaled up on mobile for touch targets.
	var panelW, lineH, headerH, paramH, footerH int
	mobile := isSmallScreen()
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
		panelW = 320
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
		// Slider draws a "XX%" label (~28px) inside its rect, so rect needs 148px min.
		minPanelW := dv.fxSliderLeft + 148 + 8
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
		contentH += len(effectTypeOrder)*lineH + lineH // 6 type buttons + cancel
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
		btnW := 24
		btnGap := 3
		if mobile {
			btnW = 36
			btnGap = 4
		}

		// Skip creating buttons for rows entirely outside viewport.
		rowTop := y
		rowBot := y + lineH
		inView := rowBot > dv.fxViewportRect.Min.Y && rowTop < dv.fxViewportRect.Max.Y

		if inView {
			// Toggle button
			toggleText := "✓"
			toggleStyle := InstButtonStyle
			if !enabled {
				toggleText = " "
				toggleStyle = DisabledButtonStyle
			}
			toggle := NewButton(toggleText, toggleStyle, nil)
			toggle.SetRect(image.Rect(x+4, y+2, x+4+btnW, y+lineH-2))
			toggle.OnClick = func() {
				audio.ToggleInsertEffect(instID, si, !enabled)
				dv.syncFXToRow(row)
				dv.buildFXPanel()
				SuppressClicksUntilMouseUp()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, toggle)

			// Expand/collapse button on mobile (enabled effects only)
			if mobile && enabled {
				chevron := "▶"
				if dv.fxExpandedSlots[si] {
					chevron = "▼"
				}
				expBtn := NewButton(chevron, InstButtonStyle, nil)
				expX := x + 4 + btnW + btnGap
				expBtn.SetRect(image.Rect(expX, y+2, expX+btnW, y+lineH-2))
				expBtn.OnClick = func() {
					if dv.fxExpandedSlots == nil {
						dv.fxExpandedSlots = map[int]bool{}
					}
					dv.fxExpandedSlots[si] = !dv.fxExpandedSlots[si]
					dv.buildFXPanel()
					SuppressClicksUntilMouseUp()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, expBtn)
			}

			// Move up button
			if slotIdx > 0 {
				up := NewButton("▲", InstButtonStyle, nil)
				upX := x + w - 3*(btnW+btnGap) - 4 + btnGap
				up.SetRect(image.Rect(upX, y+2, upX+btnW, y+lineH-2))
				up.OnClick = func() {
					audio.MoveInsertEffect(instID, si, si-1)
					dv.syncFXToRow(row)
					dv.buildFXPanel()
					SuppressClicksUntilMouseUp()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, up)
			}

			// Move down button
			if slotIdx < len(effects)-1 {
				down := NewButton("▼", InstButtonStyle, nil)
				downX := x + w - 2*(btnW+btnGap) - 4 + btnGap
				down.SetRect(image.Rect(downX, y+2, downX+btnW, y+lineH-2))
				down.OnClick = func() {
					audio.MoveInsertEffect(instID, si, si+1)
					dv.syncFXToRow(row)
					dv.buildFXPanel()
					SuppressClicksUntilMouseUp()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, down)
			}

			// Remove button
			remove := NewButton("✕", InstButtonStyle, nil)
			removeX := x + w - btnW - 4
			remove.SetRect(image.Rect(removeX, y+2, removeX+btnW, y+lineH-2))
			remove.OnClick = func() {
				audio.RemoveInsertEffect(instID, si)
				dv.syncFXToRow(row)
				dv.buildFXPanel()
				SuppressClicksUntilMouseUp()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, remove)
		}

		y += lineH

		// Parameter sliders (only if should be shown)
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
						sliderLeft := x + dv.fxSliderLeft
						if mobile {
							sliderLeft = x + 100
						}
						sl.SetRect(image.Rect(sliderLeft, y+2, x+w-4, y+paramH-2))
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
		for _, et := range effectTypeOrder {
			etCopy := et
			name := effectTypeNames[et]
			fTop := y
			fBot := y + lineH
			fInView := fBot > dv.fxViewportRect.Min.Y && fTop < dv.fxViewportRect.Max.Y
			if fInView {
				btn := NewButton(name, InstButtonStyle, nil)
				btn.SetRect(image.Rect(x+4, y+2, x+w-4, y+lineH-2))
				btn.OnClick = func() {
					audio.AddInsertEffect(instID, etCopy, nil)
					dv.syncFXToRow(row)
					dv.fxAddMenuOpen = false
					dv.buildFXPanel()
					SuppressClicksUntilMouseUp()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, btn)
			}
			y += lineH
		}
		cTop := y
		cBot := y + lineH
		cInView := cBot > dv.fxViewportRect.Min.Y && cTop < dv.fxViewportRect.Max.Y
		if cInView {
			cancelBtn := NewButton("Cancel", DisabledButtonStyle, nil)
			cancelBtn.SetRect(image.Rect(x+4, y+2, x+w-4, y+lineH-2))
			cancelBtn.OnClick = func() {
				dv.fxAddMenuOpen = false
				dv.buildFXPanel()
				SuppressClicksUntilMouseUp()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, cancelBtn)
		}
	} else {
		aTop := y
		aBot := y + footerH
		aInView := aBot > dv.fxViewportRect.Min.Y && aTop < dv.fxViewportRect.Max.Y
		if aInView {
			addBtn := NewButton("+ Add Effect", InstButtonStyle, nil)
			addBtn.SetRect(image.Rect(x+4, y+4, x+w-4, y+footerH-4))
			addBtn.OnClick = func() {
				dv.fxAddMenuOpen = true
				dv.fxScrollOffsetPx = 0 // reset scroll when opening add menu
				dv.buildFXPanel()
				SuppressClicksUntilMouseUp()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, addBtn)
		}
	}

	// Close button at top-right (fixed, not scrolled)
	closeR := closeButtonRect(dv.fxPanelRect, buttonPad)
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeFXPanel() })
	closeB.Icon = "close"
	closeB.IconColor = colButtonBorder
	closeB.SetRect(closeR)
	closeB.ConsumeOnPress = true
	dv.fxPanelBtns = append(dv.fxPanelBtns, closeB)
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
	if !dv.fxPanelOpen {
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
	mobile := isSmallScreen()
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
	drawRoundedRect(dst, headerR, color.RGBA{45, 45, 55, 255}, popupCornerRadius(), true)
	title := fmt.Sprintf("FX: %s", dv.Rows[row].Name)
	if len(title) > 30 {
		title = title[:30]
	}
	textY := headerR.Min.Y + (headerH-12)/2
	DrawTextAt(dst, title, headerR.Min.X+8, textY)

	// Draw effect names (scrollable content)
	effects := audio.GetInsertEffects(dv.Rows[row].Instrument)
	y := r.Min.Y + headerH - dv.fxScrollOffsetPx

	for slotIdx, e := range effects {
		rowTop := y
		rowBot := y + lineH
		inView := rowBot > dv.fxViewportRect.Min.Y && rowTop < dv.fxViewportRect.Max.Y

		if inView {
			name := effectTypeNames[e.Type]
			if name == "" {
				name = string(e.Type)
			}
			if !e.Enabled {
				name = "(" + name + ")"
			}
			nameX := r.Min.X + 28
			if mobile {
				nameX = r.Min.X + 44 // wider toggle button
				if e.Enabled {
					nameX = r.Min.X + 84 // room for expand button
				}
			}
			DrawTextAt(dst, name, nameX, y+(lineH-12)/2)
		}
		y += lineH

		if dv.fxShowParams(slotIdx, e.Enabled) {
			cat := audio.InsertEffectCatalog()
			if defs, ok := cat[e.Type]; ok {
				for _, def := range defs {
					slTop := y
					slBot := y + paramH
					slInView := slBot > dv.fxViewportRect.Min.Y && slTop < dv.fxViewportRect.Max.Y

					if slInView {
						val := e.Params[def.Name]
						label := fxParamLabel(def.Name, val, def.Unit)
						maxLabelW := dv.fxSliderLeft - 16
						if mobile {
							maxLabelW = 92 // mobile uses fixed slider offset
						}
						label = clipTextToWidth(label, maxLabelW)
						DrawTextAt(dst, label, r.Min.X+8, y+(paramH-12)/2)
					}
					y += paramH
				}
			}
		}
	}

	// Draw buttons (skip those outside viewport, except close button which is last)
	for i, btn := range dv.fxPanelBtns {
		if btn == nil {
			continue
		}
		// Close button is always the last one and is fixed (not scrolled)
		if i == len(dv.fxPanelBtns)-1 {
			btn.Draw(dst)
			continue
		}
		br := btn.Rect()
		if br.Max.Y > dv.fxViewportRect.Min.Y && br.Min.Y < dv.fxViewportRect.Max.Y {
			btn.Draw(dst)
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
	drawRoundedRect(dst, headerR, color.RGBA{45, 45, 55, 255}, popupCornerRadius(), true)
	DrawTextAt(dst, title, headerR.Min.X+8, textY)
	// Redraw close button on top of header
	if len(dv.fxPanelBtns) > 0 {
		closeBtn := dv.fxPanelBtns[len(dv.fxPanelBtns)-1]
		if closeBtn != nil {
			closeBtn.Draw(dst)
		}
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
	if !dv.fxPanelOpen {
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
			sl.Handle(mx, my, left)
			dv.propagateFXSliderValue(idx)
		}
		if !left {
			dv.fxSliderDragging = false
		}
		return true
	}

	// Phase 1: Mobile — touch scroll + sliders + deferred tap coordination.
	if isSmallScreen() {
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
					sl.Handle(mx, my, left)
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

	// Phase 2: Click outside panel closes it — but NOT if clicking the FX
	// button itself (the row button group will handle the toggle), and skip
	// for 2 frames after opening to avoid the same click that opened the
	// panel from closing it. Also skip while suppressClicksUntilRelease is
	// active to prevent closing on geometry changes.
	if left && !suppressClicksUntilRelease && !p.In(dv.fxPanelRect) {
		if dv.updateSeq-dv.fxPanelOpenSeq < 2 {
			return true // absorb click during debounce window
		}
		fxBtnClick := false
		for _, fb := range dv.rowFXBtns {
			if fb != nil && p.In(fb.Rect()) {
				fxBtnClick = true
				break
			}
		}
		if !fxBtnClick {
			dv.closeFXPanel()
			SuppressClicksUntilMouseUp()
			return true
		}
	}

	// Phase 3: Consume any press while click suppression is active to prevent
	// fall-through during geometry transitions.
	if left && suppressClicksUntilRelease {
		return true
	}

	// Phase 4: Desktop button handling.
	for _, btn := range dv.fxPanelBtns {
		if btn != nil && btn.Handle(mx, my, left) {
			return true
		}
	}

	// Phase 5: Desktop slider handling (enhanced with drag tracking).
	for i, sl := range dv.fxPanelSliders {
		if sl != nil && sl.Handle(mx, my, left) {
			if sl.dragging {
				dv.fxSliderDragging = true
				dv.fxSliderDragIdx = i
			}
			dv.propagateFXSliderValue(i)
			return true
		}
	}

	// Phase 6: Consume all input within panel rect.
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
	return fmt.Sprintf("%s: %s%s", capitalize(name), valStr, unit)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
