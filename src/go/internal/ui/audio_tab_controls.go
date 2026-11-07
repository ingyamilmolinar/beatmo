package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Each tab owns its controls: no tab shares a widget or visibility state with another.
type tabControls interface {
	// HeaderH returns the control-header height in pixels (0 = no header).
	HeaderH() int
	Layout(header image.Rectangle)
	// Draw renders the buttons.
	Draw(dst *ebiten.Image)
	// HitAreas returns the buttons' hit areas (z-indexed above the panel body).
	HitAreas() []HitArea
	// No-op without a freeze button.
	SyncFreeze(frozen bool)
}

// Matches stickyBarH, so the two rows read as a pair.
const audioControlHeaderH = 32

// Taller on mobile, so the freeze pill meets the touch minimum.
func controlHeaderHeight() int {
	if Profile().IsMobile() {
		return 40
	}
	return audioControlHeaderH
}

func audioPillHeight() int { return Profile().DensityValues().AudioPillH }

// Clamped to the square minimum so a 1-char label stays tappable.
func audioPillWidth(btn *Button) int {
	h := audioPillHeight()
	if btn == nil {
		return h
	}
	if btn.Icon != "" || btn.Text == "" {
		return h
	}
	w := TextWidth(btn.Text) + 2*Profile().DensityValues().AudioPillPadX
	if w < h {
		w = h
	}
	return w
}

// On mobile expands to 44 px via Touch + ClipRect; the visible pill stays.
func audioPillHit(btn *Button, z int, tag string) HitArea {
	ha := HitArea{Rect: btn.Rect(), ZIndex: z, Handler: &buttonHitAdapter{btn: btn}, Tag: tag}
	if Profile().IsMobile() {
		ha.Touch = true
		ha.ClipRect = expandToTouchMin(btn.Rect())
	}
	return ha
}

// Each tab owns its own instance. Icon-only: the icon names the click action (pause while live,
// play while frozen).
func newFreezePill(onFreeze func() bool) *Button {
	b := NewButton("", InstButtonStyle, nil)
	b.OnClick = func() {
		if onFreeze == nil {
			return
		}
		applyFreezeVisual(b, onFreeze())
	}
	applyFreezeVisual(b, false)
	return b
}

// Text is cleared so drawPillTabAt draws the glyph.
func applyFreezeVisual(b *Button, frozen bool) {
	if b == nil {
		return
	}
	b.Text = ""
	if frozen {
		b.Icon = string(IconPlay)
		b.IconColor = colAccent
		b.TextColor = colAccent
	} else {
		b.Icon = string(IconPause)
		b.IconColor = colTextSecondary
		b.TextColor = colTextSecondary
	}
}

// Text, not the icon pill: under the transport bar a pause glyph reads as "stop the music", and it
// was the only icon in a row of text codes.
func newFreezeCodePill(onFreeze func() bool) *Button {
	b := NewButton(i18n.T(i18n.KeyChainFreeze), InstButtonStyle, nil)
	b.OnClick = func() {
		if onFreeze == nil {
			return
		}
		applyFreezeCodeVisual(b, onFreeze())
	}
	applyFreezeCodeVisual(b, false)
	return b
}

// The label names the click's target: FRZ while live, LIVE while frozen.
func applyFreezeCodeVisual(b *Button, frozen bool) {
	if b == nil {
		return
	}
	b.Icon = ""
	if frozen {
		b.Text = i18n.T(i18n.KeyChainLive)
		b.TextColor = colAccent
	} else {
		b.Text = i18n.T(i18n.KeyChainFreeze)
		b.TextColor = colTextSecondary
	}
}

// Shared, so header geometry is identical across tabs.
func layoutRightPill(header image.Rectangle, btn *Button, w int) (btnH, y, rightEdge int) {
	btnH = audioPillHeight()
	y = header.Min.Y + (header.Dy()-btnH)/2
	rightEdge = header.Max.X - SpaceSM
	btn.SetRect(image.Rect(rightEdge-w, y, rightEdge, y+btnH))
	rightEdge -= w + Profile().DensityValues().AudioPillGap
	return btnH, y, rightEdge
}

// The shared base every tab header embeds; frozen is the single source of the pill's highlight.
type freezeControls struct {
	freezeBtn *Button
	frozen    bool
	z         int // hit-area z-index (panel zIdx + 1)
	hitAreas  []HitArea
}

func (c *freezeControls) HeaderH() int { return controlHeaderHeight() }

func (c *freezeControls) HitAreas() []HitArea { return c.hitAreas }

func (c *freezeControls) SyncFreeze(frozen bool) {
	c.frozen = frozen
	applyFreezeVisual(c.freezeBtn, frozen)
}

// Skips nil buttons and empty rects.
func (c *freezeControls) addHit(b *Button, tag string) {
	if b == nil {
		return
	}
	if r := b.Rect(); !r.Empty() {
		c.hitAreas = append(c.hitAreas, audioPillHit(b, c.z, tag))
	}
}

func (c *freezeControls) layoutFreezePill(header image.Rectangle) (btnH, y, rightEdge int) {
	return layoutRightPill(header, c.freezeBtn, audioPillWidth(c.freezeBtn))
}

// Implements tabControls directly (no freezeControls embed) so it can carry two pills.
type waveControls struct {
	autoBtn   *Button
	freezeBtn *Button
	autoOn    bool
	frozen    bool
	z         int
	hitAreas  []HitArea
}

func newWaveControls(z int, onAuto func() bool, onFreeze func() bool) *waveControls {
	c := &waveControls{z: z, autoOn: true}
	c.autoBtn = NewButton(i18n.T(i18n.KeyAuto), InstButtonStyle, nil)
	c.autoBtn.TextColor = colTextSecondary
	c.autoBtn.OnClick = func() {
		if onAuto == nil {
			return
		}
		c.autoOn = onAuto()
	}
	c.freezeBtn = newFreezePill(onFreeze)
	return c
}

func (c *waveControls) HeaderH() int        { return controlHeaderHeight() }
func (c *waveControls) HitAreas() []HitArea { return c.hitAreas }

// SyncFreeze drives the freeze pill icon from this tab's frozen state.
func (c *waveControls) SyncFreeze(frozen bool) {
	c.frozen = frozen
	applyFreezeVisual(c.freezeBtn, frozen)
}

// So the highlight follows a wheel zoom (which disables auto) too.
func (c *waveControls) SyncAuto(on bool) { c.autoOn = on }

func (c *waveControls) Layout(header image.Rectangle) {
	_, _, rightEdge := layoutRightPill(header, c.freezeBtn, audioPillWidth(c.freezeBtn))
	btnH := audioPillHeight()
	y := header.Min.Y + (header.Dy()-btnH)/2
	aw := audioPillWidth(c.autoBtn)
	c.autoBtn.SetRect(image.Rect(rightEdge-aw, y, rightEdge, y+btnH))

	c.hitAreas = c.hitAreas[:0]
	if r := c.freezeBtn.Rect(); !r.Empty() {
		c.hitAreas = append(c.hitAreas, audioPillHit(c.freezeBtn, c.z, "wave-freeze-btn"))
	}
	if r := c.autoBtn.Rect(); !r.Empty() {
		c.hitAreas = append(c.hitAreas, audioPillHit(c.autoBtn, c.z, "wave-auto-btn"))
	}
}

func (c *waveControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.autoBtn, c.autoOn)
	drawPillTabAt(dst, c.freezeBtn, c.frozen)
}

// CLR and K-20 are desktop-only; freeze shows on both.
type levelsControls struct {
	freezeControls
	clearClipsBtn *Button
	k20Btn        *Button
	k20View       bool
}

func newLevelsControls(z int, onFreeze func() bool, onClearClips func()) *levelsControls {
	c := &levelsControls{}
	c.freezeBtn = newFreezePill(onFreeze)
	c.z = z
	c.clearClipsBtn = NewButton("CLR", InstButtonStyle, onClearClips)
	c.clearClipsBtn.SetTextKey(i18n.KeyLevelsClear)
	c.clearClipsBtn.TextColor = colTextSecondary
	c.k20Btn = NewButton("K20", InstButtonStyle, nil)
	c.k20Btn.TextColor = colTextSecondary
	c.k20Btn.OnClick = func() { c.k20View = !c.k20View }
	return c
}

// K20View reports whether the Bob Katz -20 dBFS reference scale is active.
func (c *levelsControls) K20View() bool { return c.k20View }

func (c *levelsControls) Layout(header image.Rectangle) {
	btnH, y, rightEdge := c.layoutFreezePill(header)
	if !Profile().IsMobile() {
		d := Profile().DensityValues()
		cw := audioPillWidth(c.clearClipsBtn)
		c.clearClipsBtn.SetRect(image.Rect(rightEdge-cw, y, rightEdge, y+btnH))
		rightEdge -= cw + d.AudioPillGap
		kw := audioPillWidth(c.k20Btn)
		c.k20Btn.SetRect(image.Rect(rightEdge-kw, y, rightEdge, y+btnH))
	} else {
		c.clearClipsBtn.SetRect(image.Rectangle{})
		c.k20Btn.SetRect(image.Rectangle{})
	}
	c.rebuildHitAreas()
}

func (c *levelsControls) rebuildHitAreas() {
	c.hitAreas = c.hitAreas[:0]
	c.addHit(c.freezeBtn, "levels-freeze-btn")
	c.addHit(c.clearClipsBtn, "levels-clear-clips-btn")
	c.addHit(c.k20Btn, "levels-k20-btn")
}

func (c *levelsControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.freezeBtn, c.frozen)
	drawPillTabAt(dst, c.clearClipsBtn, false)
	drawPillTabAt(dst, c.k20Btn, c.k20View)
}

// Only freeze remains: the other pills needed analyser vocabulary to mean anything, and the
// frequency axis is always logarithmic. The axis follows the signal, so there is no detail to hide.
type spectrumControls struct {
	freezeControls
}

func newSpectrumControls(z int, onFreeze func() bool) *spectrumControls {
	c := &spectrumControls{}
	c.z = z
	c.freezeBtn = newFreezePill(onFreeze)
	return c
}

// A method so the renderer keeps one name for "show the marks".
func (c *spectrumControls) Detail() bool { return true }

func (c *spectrumControls) Layout(header image.Rectangle) {
	c.layoutFreezePill(header)
	c.rebuildHitAreas()
}

func (c *spectrumControls) rebuildHitAreas() {
	c.hitAreas = c.hitAreas[:0]
	c.addHit(c.freezeBtn, "spectrum-freeze-btn")
}

func (c *spectrumControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.freezeBtn, c.frozen)
}
