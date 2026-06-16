package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	// volPopupHorizW/H: mobile horizontal popup — fits title + percentage
	// readout + a SpaceMD-margined rail at Spacious density.
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
	// Title, when non-nil, supplies a short heading drawn at the top of the
	// popup that identifies the subject (e.g. "Kick-1" for a row, "Master" for
	// the master bus). The percentage is shown beneath it.
	Title   func() string
	OnClose func() // optional callback on close
	// OnRelease fires once when a drag gesture ends (pointer/touch up after at
	// least one SetValue). The live SetValue stays per-frame; OnRelease is the
	// deterministic commit point for emit + undo-record.
	OnRelease func()
	// Accent optionally supplies the rail fill color so the popup reads as its
	// subject. Per-row volume popups return the row's instrument color
	// (DrumRow.Color) — the SAME source the grid nodes draw from — so the slider
	// matches its instrument and tracks live color edits. Returning nil (or
	// leaving Accent nil, as the master popup does) falls back to genColorPrimary.
	Accent func() color.Color
}

// railColor resolves the rail fill color: the Accent provider when it yields a
// non-nil color, otherwise the cyan genColorPrimary accent.
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

// SetTitle installs the heading-text provider. The popup construction site
// (drumview_ctor.go) doesn't know the per-open subject, so the opener
// (openVolumePopup / openMasterVolumePopup) sets it just before Open().
func (sp *SliderPopup) SetTitle(fn func() string) { sp.config.Title = fn }

// Open positions and opens the popup adjacent to the anchor rect using the
// shared AnchorPopupRect primitive (flip + clamp), so a popup triggered from a
// bottom-row/bottom-edge anchor never runs off-screen with its LOW range
// unreachable. headerH is retained for API compatibility (the clamp already
// keeps the popup inside bounds). The popup widens to fit an optional Title.
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
	// Re-apply the horizontal viewport clamp AFTER the Title expansion so a long
	// title cannot push popupW back above the bounds.Dx()-2*SpaceMD margin.
	if sp.horizontal && popupW > bounds.Dx()-2*SpaceMD {
		popupW = bounds.Dx() - 2*SpaceMD
	}

	// Clamp region honors the header inset so the popup never overlaps the
	// transport bar when it flips above the anchor.
	clamp := bounds
	if headerH > 0 && clamp.Min.Y+headerH < clamp.Max.Y {
		clamp.Min.Y += headerH
	}

	sp.rect = AnchorPopupRect(clamp, anchor, popupW, popupH, PopupBelow)
	sp.anchor = anchor
	sp.dragging = false
	sp.open = true
}

// Anchor returns the anchor icon rect captured at Open() time. Used by the
// portal overlay to register a tap-to-close hit area at the icon, so a
// second press on the icon toggles the popup closed (bypassing the
// touch-expanded popup hit area that would otherwise swallow it on mobile).
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

// trackTop returns the Y coordinate where the vertical track begins. It sits
// below the optional Title heading (RolePanelTitle), the optional Label, and
// the percentage readout (RoleBody). Heights are computed from StyledTextHeight
// so the track never overlaps the text regardless of font-backend.
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

// trackStart / trackEnd are the rail endpoints along the active drag axis.
// Horizontal: X from left inset to right inset. Vertical: the existing Y endpoints.
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
	// Optional title heading (subject name, e.g. "Kick-1" or "Master"). Rendered
	// via RolePanelTitle so it is visually prominent and properly sized.
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
		// midY is pinned to the panel bottom; the fixed volPopupHorizH ensures the
		// title/pct text header block above leaves enough room for the rail and thumb.
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

	// Vertical neon rail. trackTop/trackBot are vertical-orientation helpers;
	// drawSliderRail fills bottom->top for a vertical rail.
	railW := Profile().DensityValues().SliderTrackH
	// Popup floor: the vertical popup rail needs a tactile grip regardless of the
	// active density (Compact's 3px track would be too thin to read here), so it
	// is clamped to at least the Spacious track width.
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

	// Round glowing thumb. thumbY is the thumb CENTER, so clamp it inward by the
	// thumb radius so the circle never overdraws past either rail end.
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

// HandleInput processes mouse/touch interaction with the popup.
// Returns true if the popup consumed the input.
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
