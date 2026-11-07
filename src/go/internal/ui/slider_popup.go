package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	// Fits title, readout and a margined rail at Spacious density.
	volPopupHorizW = 240
	volPopupHorizH = 96
	// volPopupVertW/H: desktop vertical popup (compact footprint).
	volPopupVertW = 52
	volPopupVertH = 160
)

// SliderPopupConfig configures a reusable vertical slider popup.
type SliderPopupConfig struct {
	ID       string
	ZIndex   int
	GetValue func() float64
	SetValue func(float64)
	Label    func() string // optional label above percentage (e.g. frequency); nil = no label
	// Names the subject ("Kick-1", "Master").
	Title   func() string
	OnClose func() // optional callback on close
	// The commit point (emit + undo) once per drag; SetValue stays live per frame.
	OnRelease func()
	// The row's instrument colour, the same source the grid nodes draw from, so it tracks live colour edits; nil falls
	// back to genColorPrimary.
	Accent func() color.Color
}

func (sp *SliderPopup) railColor() color.RGBA {
	if sp.config.Accent != nil {
		if c := sp.config.Accent(); c != nil {
			if rgba, ok := c.(color.RGBA); ok {
				return rgba
			}
			return color.RGBAModel.Convert(c).(color.RGBA)
		}
	}
	return genColorPrimary
}

// SliderPopup is a reusable vertical slider popup panel.
type SliderPopup struct {
	config     SliderPopupConfig
	open       bool
	rect       image.Rectangle
	anchor     image.Rectangle // anchor icon rect; used for tap-to-close
	dragging   bool
	horizontal bool // chosen at Open() time: mobile = horizontal
}

// IsHorizontal reports the popup's drag axis (true on mobile).
func (sp *SliderPopup) IsHorizontal() bool { return sp.horizontal }

// NewSliderPopup creates a new slider popup with the given config.
func NewSliderPopup(cfg SliderPopupConfig) *SliderPopup {
	return &SliderPopup{config: cfg}
}

// The construction site does not know the subject, so the opener sets it just before Open().
func (sp *SliderPopup) SetTitle(fn func() string) { sp.config.Title = fn }

// AnchorPopupRect flips and clamps, so a bottom-edge anchor never leaves the LOW range unreachable. headerH is kept
// for API compatibility.
func (sp *SliderPopup) Open(anchor, bounds image.Rectangle, headerH int) {
	sp.horizontal = Profile().IsMobile()

	var popupW, popupH int
	if sp.horizontal {
		popupW, popupH = volPopupHorizW, volPopupHorizH
		if popupW > bounds.Dx()-2*SpaceMD {
			popupW = bounds.Dx() - 2*SpaceMD // clamp width to viewport
		}
	} else {
		popupW, popupH = volPopupVertW, volPopupVertH
	}
	if sp.config.Title != nil {
		if w := StyledTextWidth(sp.config.Title(), RolePanelTitle) + SpaceMD*2; w > popupW {
			popupW = w
		}
	}
	// After the Title expansion, so a long title cannot push past the margin.
	if sp.horizontal && popupW > bounds.Dx()-2*SpaceMD {
		popupW = bounds.Dx() - 2*SpaceMD
	}

	// The header inset keeps a flipped popup off the transport bar.
	clamp := bounds
	if headerH > 0 && clamp.Min.Y+headerH < clamp.Max.Y {
		clamp.Min.Y += headerH
	}

	sp.rect = AnchorPopupRect(clamp, anchor, popupW, popupH, PopupBelow)
	sp.anchor = anchor
	sp.dragging = false
	sp.open = true
}

// Lets a second press on the icon close the popup, bypassing the touch-expanded popup area that would swallow it.
func (sp *SliderPopup) Anchor() image.Rectangle { return sp.anchor }

// Close closes the popup.
func (sp *SliderPopup) Close() {
	sp.open = false
	sp.dragging = false
	if sp.config.OnClose != nil {
		sp.config.OnClose()
	}
}

// IsOpen returns whether the popup is currently open.
func (sp *SliderPopup) IsOpen() bool { return sp.open }

// Rect returns the popup panel rect.
func (sp *SliderPopup) Rect() image.Rectangle { return sp.rect }

// IsDragging returns whether the user is currently dragging the thumb.
func (sp *SliderPopup) IsDragging() bool { return sp.dragging }

// From StyledTextHeight so the track never overlaps the text on any font backend.
func (sp *SliderPopup) trackTop() int {
	top := sp.rect.Min.Y + SpaceXS
	if sp.config.Title != nil {
		top += StyledTextHeight(RolePanelTitle) + SpaceXS
	}
	top += StyledTextHeight(RoleBody) + SpaceXS
	if sp.config.Label != nil {
		top += StyledTextHeight(RoleCaption) + SpaceXS
	}
	return top
}

// trackBot returns the Y coordinate where the vertical track ends.
func (sp *SliderPopup) trackBot() int {
	return sp.rect.Max.Y - 8
}

func (sp *SliderPopup) trackStart() int {
	if sp.horizontal {
		return sp.rect.Min.X + SpaceMD
	}
	return sp.trackTop()
}
func (sp *SliderPopup) trackEnd() int {
	if sp.horizontal {
		return sp.rect.Max.X - SpaceMD
	}
	return sp.trackBot()
}

// Draw renders the popup panel with optional label, percentage, track, and thumb.
func (sp *SliderPopup) Draw(dst *ebiten.Image) {
	if !sp.open || sp.rect.Empty() {
		return
	}
	r := sp.rect
	drawPanel(dst, r)

	val := sp.config.GetValue()

	yOff := SpaceXS
	if sp.config.Title != nil {
		titleH := StyledTextHeight(RolePanelTitle)
		title := clipTextToWidth(sp.config.Title(), r.Dx()-SpaceSM*2)
		tx := r.Min.X + (r.Dx()-StyledTextWidth(title, RolePanelTitle))/2
		ty := r.Min.Y + yOff
		DrawTextStyled(dst, title, tx, ty, RolePanelTitle, colTextPrimary)
		yOff += titleH + SpaceXS
	}
	// Optional label (e.g. frequency).
	if sp.config.Label != nil {
		label := sp.config.Label()
		lx := r.Min.X + (r.Dx()-StyledTextWidth(label, RoleCaption))/2
		ly := r.Min.Y + yOff
		DrawTextStyled(dst, label, lx, ly, RoleCaption, colTextSecondary)
		yOff += StyledTextHeight(RoleCaption) + SpaceXS
	}

	// Percentage readout rendered via RoleBody for a legible, role-consistent size.
	pct := int(math.Round(val * 100))
	pctLabel := fmt.Sprintf("%d%%", pct)
	pctW := StyledTextWidth(pctLabel, RoleBody)
	px := r.Min.X + (r.Dx()-pctW)/2
	py := r.Min.Y + yOff
	DrawTextStyled(dst, pctLabel, px, py, RoleBody, colTextPrimary)

	if sp.horizontal {
		railH := Profile().DensityValues().SliderTrackH
		if railH < densityValuesFor(DensitySpacious).SliderTrackH {
			railH = densityValuesFor(DensitySpacious).SliderTrackH
		}
		dia := Profile().DensityValues().SliderThumbH
		// The fixed popup height leaves the header text room above the rail.
		midY := sp.rect.Max.Y - SpaceMD - dia/2
		x0 := sp.trackStart()
		x1 := sp.trackEnd()
		if x1-x0 <= 0 {
			return
		}
		rail := image.Rect(x0, midY-railH/2, x1, midY+railH/2)
		drawSliderRail(dst, rail, val, true /*horizontal*/, sp.railColor())
		// clamp thumb center inward by dia/2 so it never overdraws the rail ends
		thumbX := x0 + int(float64(x1-x0)*val)
		if thumbX < x0+dia/2 {
			thumbX = x0 + dia/2
		}
		if thumbX > x1-dia/2 {
			thumbX = x1 - dia/2
		}
		drawSliderThumb(dst, image.Pt(thumbX, midY), dia, sp.dragging, false)
		return
	}

	railW := Profile().DensityValues().SliderTrackH
	// At least the Spacious width: Compact's 3 px track is too thin to grip here.
	if minW := densityValuesFor(DensitySpacious).SliderTrackH; railW < minW {
		railW = minW
	}
	trackX := r.Min.X + r.Dx()/2 - railW/2
	trackTop := sp.trackTop()
	trackBot := sp.trackBot()
	trackH := trackBot - trackTop
	if trackH <= 0 {
		return
	}
	rail := image.Rect(trackX, trackTop, trackX+railW, trackBot)
	drawSliderRail(dst, rail, val, false /*vertical*/, sp.railColor())

	// thumbY is the centre, clamped by the radius so the circle never overdraws a rail end.
	dia := Profile().DensityValues().SliderThumbH
	thumbY := trackTop + int(float64(trackH)*(1-val))
	if lo := trackTop + dia/2; thumbY < lo {
		thumbY = lo
	}
	if hi := trackBot - dia/2; thumbY > hi {
		thumbY = hi
	}
	drawSliderThumb(dst, image.Pt(r.Min.X+r.Dx()/2, thumbY), dia, sp.dragging, false)
}

func (sp *SliderPopup) HandleInput(mx, my int, pressed bool) bool {
	if !sp.open {
		return false
	}

	if pressed && (sp.dragging || image.Pt(mx, my).In(sp.rect)) {
		sp.dragging = true
		var val float64
		if sp.horizontal {
			x0, x1 := sp.trackStart(), sp.trackEnd()
			if x1-x0 <= 0 {
				return true
			}
			c := mx
			if c < x0 {
				c = x0
			}
			if c > x1 {
				c = x1
			}
			val = float64(c-x0) / float64(x1-x0)
		} else {
			tTop, tBot := sp.trackTop(), sp.trackBot()
			if tBot-tTop <= 0 {
				return true
			}
			c := my
			if c < tTop {
				c = tTop
			}
			if c > tBot {
				c = tBot
			}
			val = 1.0 - float64(c-tTop)/float64(tBot-tTop)
		}
		val = math.Round(val*100) / 100
		if val < 0 {
			val = 0
		}
		if val > 1 {
			val = 1
		}
		sp.config.SetValue(val)
		return true
	}
	if !pressed {
		if sp.dragging && sp.config.OnRelease != nil {
			sp.config.OnRelease()
		}
		sp.dragging = false
	}
	return false
}

// SliderPopupOverlay wraps a SliderPopup as a generic Overlay.
type SliderPopupOverlay struct {
	Popup *SliderPopup
}

func (o *SliderPopupOverlay) ID() string                   { return o.Popup.config.ID }
func (o *SliderPopupOverlay) IsOpen() bool                 { return o.Popup.IsOpen() }
func (o *SliderPopupOverlay) ZIndex() int                  { return o.Popup.config.ZIndex }
func (o *SliderPopupOverlay) InputBounds() image.Rectangle { return o.Popup.Rect() }
func (o *SliderPopupOverlay) Capturing() bool              { return o.Popup.IsDragging() }
func (o *SliderPopupOverlay) Close() {
	o.Popup.Close()
}
func (o *SliderPopupOverlay) HandleInput(x, y int, pressed bool) InputResult {
	if o.Popup.HandleInput(x, y, pressed) {
		if o.Capturing() {
			return InputCaptured
		}
		return InputConsumed
	}
	return InputIgnored
}
func (o *SliderPopupOverlay) HandleWheel(x, y, steps int) InputResult {
	return InputConsumed // prevent scroll-through
}

// ── keyboard editing (KeyboardKeyClaimant, see keyboard_key_claim.go) ────────

// ≈2% per press, ≈0.5% with Shift. Left/Right are NOT claimed: one axis, so they stay with the grid's pan.
const (
	sliderPopupKeyStep     = 0.02
	sliderPopupKeyStepFine = 0.005
)

// ClaimedKeys implements KeyboardKeyClaimant: Up/Down while open.
func (sp *SliderPopup) ClaimedKeys() []ebiten.Key {
	if sp == nil || !sp.open {
		return nil
	}
	return valueStepKeys()
}

// One key per frame, so the arrow spellings real Ebiten aliases cannot double-fire.
func (sp *SliderPopup) HandleClaimedKeys() bool {
	if sp == nil || !sp.open {
		return false
	}
	switch {
	case anyKeyJustPressed(keysStepUp...):
		return sp.StepValue(+1)
	case anyKeyJustPressed(keysStepDown...):
		return sp.StepValue(-1)
	}
	return false
}

// Each press is a whole gesture: it commits through OnRelease, one undo step, never a silent live-only edit.
func (sp *SliderPopup) StepValue(dir int) bool {
	if sp == nil || dir == 0 || sp.config.GetValue == nil || sp.config.SetValue == nil {
		return false
	}
	step := sliderPopupKeyStep
	if isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight) {
		step = sliderPopupKeyStepFine
	}
	cur := sp.config.GetValue()
	next := clampF64(cur+float64(sign(dir))*step, 0, 1)
	if next == cur {
		return false // already at an end stop
	}
	sp.config.SetValue(next)
	if sp.config.OnRelease != nil {
		sp.config.OnRelease()
	}
	return true
}
