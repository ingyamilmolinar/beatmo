package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// tabControls is the control header owned by a single audio-panel tab. Each
// tab's controls are an independent component — no tab shares a button widget
// or visibility state with another. The dispatcher (EQPanelZone) asks the
// active tab's component for its header height, lays it into the strip just
// below the shared tab-switcher row, draws it, and routes its hit areas. This
// is the per-tab ownership model that ChainPanelZone and the Synth header
// already follow.
type tabControls interface {
	// HeaderH returns the control-header height in pixels (0 = no header).
	HeaderH() int
	// Layout positions the buttons inside header (the strip reserved at the
	// top of the tab's content region, directly below the tab-switcher row).
	Layout(header image.Rectangle)
	// Draw renders the buttons.
	Draw(dst *ebiten.Image)
	// HitAreas returns the buttons' hit areas (z-indexed above the panel body).
	HitAreas() []HitArea
	// SyncFreeze updates the freeze button's glyph/color from the analyzer's
	// global frozen state. No-op for components without a freeze button.
	SyncFreeze(frozen bool)
}

// audioControlHeaderH is the desktop height of a per-tab control header — the
// row of tab-specific pills directly below the shared tab-switcher row. Mirrors
// stickyBarH so the two rows read as a stacked pair.
const audioControlHeaderH = 32

// controlHeaderHeight returns the per-tab control-header height (taller on
// mobile so the freeze pill meets the touch-min, mirroring stickyBarHeight).
func controlHeaderHeight() int {
	if Profile().IsMobile() {
		return 40
	}
	return audioControlHeaderH
}

// audioPillHeight is the single height for every audio-panel pill, Density-
// driven (two effective sizes: Comfortable desktop, Spacious mobile).
func audioPillHeight() int { return Profile().DensityValues().AudioPillH }

// audioPillWidth is content-fit: icon/empty-text pills are square (= height);
// text pills are TextWidth(label) + 2*AudioPillPadX, clamped to the square
// minimum so a 1-char label stays comfortably tappable.
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

// audioPillHit builds a pill's hit area; on mobile it expands to the 44px
// touch-min via Touch+ClipRect (bounded — the visible pill is not enlarged).
func audioPillHit(btn *Button, z int, tag string) HitArea {
	ha := HitArea{Rect: btn.Rect(), ZIndex: z, Handler: &buttonHitAdapter{btn: btn}, Tag: tag}
	if Profile().IsMobile() {
		ha.Touch = true
		ha.ClipRect = expandToTouchMin(btn.Rect())
	}
	return ha
}

// newFreezePill builds a freeze/resume toggle pill wired to onFreeze (which
// returns the new frozen state). Each tab owns its own instance — there is no
// shared freeze widget. The pill is icon-only: a pause icon while live (the
// click action is "pause") and a play icon while frozen (action "resume"), set
// by applyFreezeVisual.
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

// applyFreezeVisual sets the freeze pill's icon + color from a frozen bool.
// Live → pause icon (the action is "pause"); frozen → play icon (action
// "resume"). Icon-only: Text is cleared so drawPillTabAt renders the glyph.
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

// layoutRightPill places btn flush against header's right edge and returns the
// clamped button height, the pill top Y, and the running right-edge cursor for
// any further pills laid out leftward. Shared by the freeze-pill base and the
// Wave tab's AUTO pill so the header geometry stays identical across tabs.
func layoutRightPill(header image.Rectangle, btn *Button, w int) (btnH, y, rightEdge int) {
	btnH = audioPillHeight()
	y = header.Min.Y + (header.Dy()-btnH)/2
	rightEdge = header.Max.X - SpaceSM
	btn.SetRect(image.Rect(rightEdge-w, y, rightEdge, y+btnH))
	rightEdge -= w + Profile().DensityValues().AudioPillGap
	return btnH, y, rightEdge
}

// freezeControls is the shared base every per-tab control header embeds. It
// owns the freeze/resume pill, the frozen flag (the single source of truth for
// the pill's active-highlight — no re-parsing the glyph string), the hit-area
// z-index, and the rebuilt hit-area slice. Concrete tabs embed it and supply
// only Layout + Draw + rebuildHitAreas; HeaderH/HitAreas/SyncFreeze and the
// addHit / layoutFreezePill helpers are inherited so adding a fourth tab does
// not re-copy them.
type freezeControls struct {
	freezeBtn *Button
	frozen    bool
	z         int // hit-area z-index (panel zIdx + 1)
	hitAreas  []HitArea
}

func (c *freezeControls) HeaderH() int { return controlHeaderHeight() }

func (c *freezeControls) HitAreas() []HitArea { return c.hitAreas }

// SyncFreeze records the analyzer's global frozen state (read by each tab's
// Draw to decide the active highlight) and updates the pill glyph/color.
func (c *freezeControls) SyncFreeze(frozen bool) {
	c.frozen = frozen
	applyFreezeVisual(c.freezeBtn, frozen)
}

// addHit appends a button's hit area at the component's z-index, skipping nil
// buttons and empty (laid-out-away) rects.
func (c *freezeControls) addHit(b *Button, tag string) {
	if b == nil {
		return
	}
	if r := b.Rect(); !r.Empty() {
		c.hitAreas = append(c.hitAreas, audioPillHit(b, c.z, tag))
	}
}

// layoutFreezePill clamps the header button height, places the freeze pill
// flush against the header's right edge, and returns the button height, the
// pill top Y, and the running right-edge cursor for any further pills the tab
// lays out leftward.
func (c *freezeControls) layoutFreezePill(header image.Rectangle) (btnH, y, rightEdge int) {
	return layoutRightPill(header, c.freezeBtn, audioPillWidth(c.freezeBtn))
}

// waveControls owns the Wave tab's control header: an AUTO pill that toggles
// adaptive Y-auto-gain plus a freeze (pause/play) pill that pauses this tab's
// trace. waveControls does not embed freezeControls; it implements the
// tabControls interface directly so it can carry both pills.
type waveControls struct {
	autoBtn   *Button
	freezeBtn *Button
	autoOn    bool
	frozen    bool
	z         int
	hitAreas  []HitArea
}

// newWaveControls builds the Wave header: AUTO pill + freeze (pause/play) pill.
// onAuto toggles auto-gain; onFreeze toggles this tab's freeze. Both return the
// new state.
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

// SyncAuto records the zone's auto-gain state so the pill's active-highlight
// follows wheel-zoom (which disables auto) as well as the pill click.
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

// levelsControls owns the Levels tab's control header: freeze, Clear-Clips
// (CLR), and the K-20 reference-scale toggle. Clear-Clips and K-20 are
// desktop-only (mobile hides them, mirroring the legacy bar); freeze shows on
// both platforms.
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

// spectrumControls owns the Spectrum tab's control header: freeze, freq-scale
// (log/lin), slope tilt (0/3/4.5 dB/oct), Pre overlay toggle, and Reset-Hold.
// Freeze shows on both platforms; the other four are desktop-only (mirroring
// the legacy bar gating).
type spectrumControls struct {
	freezeControls
	freqScaleBtn *Button
	freqScaleLog bool
	slopeBtn     *Button
	slopeOptions []float64
	slopeIdx     int
	preBtn       *Button
	preOverlay   bool
	resetHoldBtn *Button
}

func newSpectrumControls(z int, onFreeze func() bool, onResetHold func()) *spectrumControls {
	c := &spectrumControls{freqScaleLog: true, slopeOptions: []float64{0, 3, 4.5}}
	c.z = z
	c.freezeBtn = newFreezePill(onFreeze)

	c.freqScaleBtn = NewButton("log", InstButtonStyle, nil)
	c.freqScaleBtn.TextColor = colTextSecondary
	c.freqScaleBtn.OnClick = func() {
		c.freqScaleLog = !c.freqScaleLog
		if c.freqScaleLog {
			c.freqScaleBtn.Text = "log"
		} else {
			c.freqScaleBtn.Text = "lin"
		}
	}

	c.slopeBtn = NewButton(formatSlopeLabel(0), InstButtonStyle, nil)
	c.slopeBtn.TextColor = colTextSecondary
	c.slopeBtn.OnClick = func() {
		c.slopeIdx = (c.slopeIdx + 1) % len(c.slopeOptions)
		v := c.slopeOptions[c.slopeIdx]
		c.slopeBtn.Text = formatSlopeLabel(v)
		SetSpectrumSlope(v)
	}

	c.preBtn = NewButton(i18n.T(i18n.KeyPre), InstButtonStyle, nil)
	c.preBtn.TextColor = colTextSecondary
	c.preBtn.OnClick = func() { c.preOverlay = !c.preOverlay }

	c.resetHoldBtn = NewButton("R", InstButtonStyle, onResetHold)
	c.resetHoldBtn.TextColor = colTextSecondary
	return c
}

func (c *spectrumControls) SlopeDBPerOct() float64 { return c.slopeOptions[c.slopeIdx] }
func (c *spectrumControls) PreOverlay() bool       { return c.preOverlay }
func (c *spectrumControls) FreqScaleLog() bool     { return c.freqScaleLog }

func (c *spectrumControls) Layout(header image.Rectangle) {
	btnH, y, rightEdge := c.layoutFreezePill(header)
	if !Profile().IsMobile() {
		d := Profile().DensityValues()
		qw := audioPillWidth(c.freqScaleBtn)
		c.freqScaleBtn.SetRect(image.Rect(rightEdge-qw, y, rightEdge, y+btnH))
		rightEdge -= qw + d.AudioPillGap
		rw := audioPillWidth(c.resetHoldBtn)
		c.resetHoldBtn.SetRect(image.Rect(rightEdge-rw, y, rightEdge, y+btnH))
		rightEdge -= rw + d.AudioPillGap
		pw := audioPillWidth(c.preBtn)
		c.preBtn.SetRect(image.Rect(rightEdge-pw, y, rightEdge, y+btnH))
		rightEdge -= pw + d.AudioPillGap
		c.slopeBtn.Text = formatSlopeLabel(c.slopeOptions[c.slopeIdx])
		sw := audioPillWidth(c.slopeBtn)
		c.slopeBtn.SetRect(image.Rect(rightEdge-sw, y, rightEdge, y+btnH))
	} else {
		c.freqScaleBtn.SetRect(image.Rectangle{})
		c.resetHoldBtn.SetRect(image.Rectangle{})
		c.preBtn.SetRect(image.Rectangle{})
		c.slopeBtn.SetRect(image.Rectangle{})
	}
	c.rebuildHitAreas()
}

func (c *spectrumControls) rebuildHitAreas() {
	c.hitAreas = c.hitAreas[:0]
	c.addHit(c.freezeBtn, "spectrum-freeze-btn")
	c.addHit(c.freqScaleBtn, "spectrum-freqscale-btn")
	c.addHit(c.slopeBtn, "spectrum-slope-btn")
	c.addHit(c.preBtn, "spectrum-pre-btn")
	c.addHit(c.resetHoldBtn, "spectrum-reset-hold-btn")
}

func (c *spectrumControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.freezeBtn, c.frozen)
	drawPillTabAt(dst, c.freqScaleBtn, !c.freqScaleLog)
	drawPillTabAt(dst, c.slopeBtn, c.slopeIdx != 0)
	drawPillTabAt(dst, c.preBtn, c.preOverlay)
	drawPillTabAt(dst, c.resetHoldBtn, false)
}
