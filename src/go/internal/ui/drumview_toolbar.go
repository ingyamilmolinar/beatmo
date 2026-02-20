package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

func (dv *DrumView) decayAnims() {
	decay := func(v *float64) {
		*v *= 0.85
		if *v < 0.01 {
			*v = 0
		}
	}
	decay(&dv.playAnim)
	decay(&dv.stopAnim)
	decay(&dv.bpmDecAnim)
	decay(&dv.bpmIncAnim)
	decay(&dv.lenDecAnim)
	decay(&dv.lenIncAnim)
	decay(&dv.uploadAnim)
	decay(&dv.bpmErrorAnim)
	decay(&dv.saveAnim)
	// Reset delete confirmation after ~2s timeout
	if dv.deleteConfirmRow >= 0 && (dv.frame-dv.deleteConfirmFrame) >= 120 {
		dv.deleteConfirmRow = -1
		dv.markRowControlsDirty()
	}
}

// toolbarStateHash computes a hash of all toolbar-visible state. When this
// changes the toolbar cache must be rebuilt. The hash is cheap to compute
// each frame (~30ns) and avoids the need for manual dirty-marking.
func (dv *DrumView) toolbarStateHash() uint64 {
	h := uint64(17)
	mix := func(v uint64) { h = h*31 + v }
	boolBit := func(b bool) uint64 {
		if b {
			return 1
		}
		return 0
	}
	// Layout bounds
	mix(uint64(dv.Bounds.Min.X))
	mix(uint64(dv.Bounds.Min.Y))
	mix(uint64(dv.Bounds.Dx()))
	mix(uint64(dv.Bounds.Dy()))
	// Transport state
	mix(boolBit(dv.isPlaying))
	mix(uint64(dv.bpm))
	mix(uint64(dv.Length))
	mix(boolBit(dv.follow))
	// BPM text (editable field)
	for _, r := range dv.bpmBox.Text {
		mix(uint64(r))
	}
	mix(boolBit(dv.bpmBox.Focused()))
	// Subdiv text
	for _, r := range dv.subdivBtn.Text {
		mix(uint64(r))
	}
	// Volume slider position
	if dv.mainVolSlider != nil {
		mix(math.Float64bits(dv.mainVolSlider.Value))
	}
	// Button hover/press state (changes visual appearance)
	mix(boolBit(dv.playBtn.hovered))
	mix(boolBit(dv.playBtn.pressed))
	mix(boolBit(dv.stopBtn.hovered))
	mix(boolBit(dv.stopBtn.pressed))
	mix(boolBit(dv.bpmDecBtn.hovered))
	mix(boolBit(dv.bpmDecBtn.pressed))
	mix(boolBit(dv.bpmIncBtn.hovered))
	mix(boolBit(dv.bpmIncBtn.pressed))
	mix(boolBit(dv.subdivBtn.hovered))
	mix(boolBit(dv.subdivBtn.pressed))
	mix(boolBit(dv.trackBtn.hovered))
	mix(boolBit(dv.trackBtn.pressed))
	mix(boolBit(dv.uploadBtn.hovered))
	mix(boolBit(dv.uploadBtn.pressed))
	mix(boolBit(dv.importBtn.hovered))
	mix(boolBit(dv.importBtn.pressed))
	mix(boolBit(dv.exportBtn.hovered))
	mix(boolBit(dv.exportBtn.pressed))
	// Track button text changes with follow state
	for _, r := range dv.trackBtn.Text {
		mix(uint64(r))
	}
	// Play button icon changes with play state
	if dv.playBtn.Icon != "" {
		for _, r := range dv.playBtn.Icon {
			mix(uint64(r))
		}
	}
	if dv.eqToggleMobile != nil {
		for _, r := range dv.eqToggleMobile.Text {
			mix(uint64(r))
		}
		mix(boolBit(dv.eqToggleMobile.hovered))
		mix(boolBit(dv.eqToggleMobile.pressed))
	}
	if dv.overflowBtn != nil {
		for _, r := range dv.overflowBtn.Text {
			mix(uint64(r))
		}
		mix(boolBit(dv.overflowBtn.hovered))
		mix(boolBit(dv.overflowBtn.pressed))
	}
	if dv.viewSwitchBtn != nil {
		for _, r := range dv.viewSwitchBtn.Icon {
			mix(uint64(r))
		}
		mix(boolBit(dv.viewSwitchBtn.hovered))
		mix(boolBit(dv.viewSwitchBtn.pressed))
	}
	mix(boolBit(dv.mobileEQMode))
	mix(boolBit(dv.overflowMenuOpen))
	mix(uint64(dv.currentViewMode))
	mix(boolBit(dv.masterVolPopup.IsOpen()))
	// Animation state (non-zero means visual change)
	mix(math.Float64bits(dv.bpmErrorAnim))
	return h
}

// toolbarBounds computes the bounding rectangle of all toolbar controls.
func (dv *DrumView) toolbarBounds() image.Rectangle {
	first := true
	var minX, minY, maxX, maxY int
	expand := func(r image.Rectangle) {
		if r.Empty() {
			return
		}
		if first {
			minX, minY, maxX, maxY = r.Min.X, r.Min.Y, r.Max.X, r.Max.Y
			first = false
		} else {
			if r.Min.X < minX {
				minX = r.Min.X
			}
			if r.Min.Y < minY {
				minY = r.Min.Y
			}
			if r.Max.X > maxX {
				maxX = r.Max.X
			}
			if r.Max.Y > maxY {
				maxY = r.Max.Y
			}
		}
	}
	expand(dv.playBtn.Rect())
	expand(dv.stopBtn.Rect())
	expand(dv.bpmDecBtn.Rect())
	expand(dv.bpmBox.Rect)
	expand(dv.bpmIncBtn.Rect())
	expand(dv.subdivBtn.Rect())
	// Len +/- buttons are positioned in the timeline area,
	// drawn separately (not as part of the toolbar cache).
	// Track button is positioned in the timeline area on both platforms,
	// drawn separately (not as part of the toolbar cache).
	expand(dv.uploadBtn.Rect())
	expand(dv.importBtn.Rect())
	expand(dv.exportBtn.Rect())
	if dv.eqToggleMobile != nil {
		expand(dv.eqToggleMobile.Rect())
	}
	if dv.overflowBtn != nil {
		expand(dv.overflowBtn.Rect())
	}
	if dv.viewSwitchBtn != nil {
		expand(dv.viewSwitchBtn.Rect())
	}
	if dv.mainVolSlider != nil {
		expand(dv.mainVolSlider.Rect())
	}
	expand(dv.mainVolIconRect)
	if first {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

func (dv *DrumView) renderToolbarControls(dst *ebiten.Image) {
	// Update only the rectangles/positions for existing per-row controls.
	// Avoid recreating buttons and sliders every frame to keep rendering lightweight.
	dv.updateRowRects()

	hash := dv.toolbarStateHash()
	rect := dv.toolbarBounds()
	if rect.Empty() {
		// Fallback: draw directly (shouldn't happen in practice).
		dv.renderToolbarDirect(dst)
		return
	}

	// Cache hit: blit the cached toolbar image.
	if dv.toolbarCache != nil && dv.toolbarCacheHash == hash && dv.toolbarCacheRect == rect {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
		dst.DrawImage(dv.toolbarCache, &op)
		return
	}

	// Cache miss: rebuild.
	w, h := rect.Dx(), rect.Dy()
	if dv.toolbarCache == nil || dv.toolbarCache.Bounds().Dx() != w || dv.toolbarCache.Bounds().Dy() != h {
		dv.toolbarCache = ebiten.NewImage(w, h)
	} else {
		dv.toolbarCache.Clear()
	}

	dv.renderToolbarToCache(dv.toolbarCache, rect.Min.X, rect.Min.Y)

	dv.toolbarCacheHash = hash
	dv.toolbarCacheRect = rect

	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
	dst.DrawImage(dv.toolbarCache, &op)
}

// renderToolbarToCache draws all toolbar controls to the cache image with
// coordinate offsets so controls are positioned relative to (0,0).
func (dv *DrumView) renderToolbarToCache(cache *ebiten.Image, offsetX, offsetY int) {
	dv.drawButtonOffset(cache, dv.playBtn, offsetX, offsetY)
	dv.drawButtonOffset(cache, dv.stopBtn, offsetX, offsetY)
	dv.drawButtonOffset(cache, dv.bpmDecBtn, offsetX, offsetY)
	dv.drawTextInputOffset(cache, dv.bpmBox, offsetX, offsetY)
	if dv.bpmErrorAnim > 0 {
		r := dv.bpmBox.Rect.Sub(image.Pt(offsetX, offsetY))
		drawRect(cache, r, fadeColor(colError, dv.bpmErrorAnim), false)
	}
	dv.drawButtonOffset(cache, dv.bpmIncBtn, offsetX, offsetY)
	dv.drawButtonOffset(cache, dv.subdivBtn, offsetX, offsetY)
	dv.drawButtonOffset(cache, dv.trackBtn, offsetX, offsetY)
	dv.drawButtonOffset(cache, dv.uploadBtn, offsetX, offsetY)
	dv.drawButtonOffset(cache, dv.importBtn, offsetX, offsetY)
	dv.drawButtonOffset(cache, dv.exportBtn, offsetX, offsetY)
	if dv.eqToggleMobile != nil {
		dv.drawButtonOffset(cache, dv.eqToggleMobile, offsetX, offsetY)
	}
	if dv.overflowBtn != nil {
		dv.drawButtonOffset(cache, dv.overflowBtn, offsetX, offsetY)
	}
	if dv.viewSwitchBtn != nil {
		dv.drawButtonOffset(cache, dv.viewSwitchBtn, offsetX, offsetY)
	}
	if dv.mainVolSlider != nil {
		if isSmallScreen() {
			dv.drawMasterVolIconOffset(cache, offsetX, offsetY)
		}
		dv.drawSliderOffset(cache, dv.mainVolSlider, offsetX, offsetY)
	}
	// Draw subtle group separators between logical button groups.
	dv.drawToolbarSeparators(cache, offsetX, offsetY)
}

// drawToolbarSeparators draws 1px vertical separator lines between toolbar groups.
func (dv *DrumView) drawToolbarSeparators(cache *ebiten.Image, offsetX, offsetY int) {
	if !isSmallScreen() {
		return // desktop does not draw toolbar separators
	}
	sepCol := color.NRGBA{255, 255, 255, 12}
	var pairs [][2]image.Rectangle
	// Mobile separators: [Play|Stop] | [BPM box|BPM+/-] | [Subdiv] | [ViewSwitch|Overflow]
	pairs = [][2]image.Rectangle{
		{dv.stopBtn.Rect(), dv.bpmBox.Rect},
		{dv.bpmIncBtn.Rect(), dv.subdivBtn.Rect()},
	}
	if dv.viewSwitchBtn != nil {
		pairs = append(pairs, [2]image.Rectangle{dv.subdivBtn.Rect(), dv.viewSwitchBtn.Rect()})
	}
	_ = dv.overflowBtn // overflow is the last element; no trailing separator needed
	for _, p := range pairs {
		left, right := p[0], p[1]
		if left.Empty() || right.Empty() {
			continue
		}
		sepX := (left.Max.X + right.Min.X) / 2
		top := left.Min.Y + left.Dy()/4
		bot := left.Max.Y - left.Dy()/4
		if bot <= top {
			continue
		}
		drawRect(cache, image.Rect(sepX-offsetX, top-offsetY, sepX-offsetX+1, bot-offsetY), sepCol, true)
	}
}

// renderToolbarDirect draws toolbar controls directly without caching (fallback).
func (dv *DrumView) renderToolbarDirect(dst *ebiten.Image) {
	dv.playBtn.Draw(dst)
	dv.stopBtn.Draw(dst)
	dv.bpmDecBtn.Draw(dst)
	dv.bpmBox.Draw(dst)
	if dv.bpmErrorAnim > 0 {
		drawRect(dst, dv.bpmBox.Rect, fadeColor(colError, dv.bpmErrorAnim), false)
	}
	dv.bpmIncBtn.Draw(dst)
	dv.subdivBtn.Draw(dst)
	dv.trackBtn.Draw(dst)
	dv.uploadBtn.Draw(dst)
	dv.importBtn.Draw(dst)
	dv.exportBtn.Draw(dst)
	if dv.eqToggleMobile != nil {
		dv.eqToggleMobile.Draw(dst)
	}
	if dv.overflowBtn != nil {
		dv.overflowBtn.Draw(dst)
	}
	if dv.viewSwitchBtn != nil {
		dv.viewSwitchBtn.Draw(dst)
	}
	if dv.mainVolSlider != nil {
		if isSmallScreen() {
			dv.drawMasterVolIconOffset(dst, 0, 0)
		}
		dv.mainVolSlider.Draw(dst)
	}
}

// drawTextInputOffset draws a TextInput to a cache image with coordinate offset.
func (dv *DrumView) drawTextInputOffset(cache *ebiten.Image, ti *TextInput, offsetX, offsetY int) {
	origRect := ti.Rect
	ti.Rect = origRect.Sub(image.Pt(offsetX, offsetY))
	ti.Draw(cache)
	ti.Rect = origRect
}

// drawNotifications renders up to 2 active notifications at the top-right of
// the drum view panel. Messages fade out as ttl decays.
func (dv *DrumView) drawNotifications(dst *ebiten.Image) {
	// Decay and prune
	out := dv.notifs[:0]
	for i := range dv.notifs {
		n := dv.notifs[i]
		if n.ttl <= 0 {
			continue
		}
		n.ttl--
		out = append(out, n)
	}
	dv.notifs = out
	// Draw last two (most recent at top)
	maxShow := 2
	pad := 6
	y := dv.Bounds.Min.Y + 6
	for i := len(dv.notifs) - 1; i >= 0 && maxShow > 0; i-- {
		n := dv.notifs[i]
		txt := n.msg
		w := TextWidth(txt)
		h := TextHeight() + pad
		boxW := w + pad*2
		x := dv.Bounds.Max.X - boxW - 10
		r := image.Rect(x, y, x+boxW, y+h)
		fill := colBPMBox
		if n.isErr {
			fill = colError
		}
		// Slight transparency when close to expiry
		drawButton(dst, r, fill, colButtonBorder, false)
		// Right align text within the box with small inner pad
		DrawTextAt(dst, txt, r.Min.X+pad, r.Min.Y+(h-TextHeight())/2)
		y += h + 4
		maxShow--
	}
}
