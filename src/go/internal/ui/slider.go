package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const sliderLabelPad = 8

// Slider is a horizontal slider component with a 0..1 value.
type Slider struct {
	r        image.Rectangle
	Value    float64
	dragging bool
	// cache for label to avoid per-frame fmt.Sprintf churn
	lastPct   int
	lastLabel string

	// Optional overrides (0/nil = use Profile() defaults).
	TrackH     int   // track height override; 0 = Profile().SliderTrackH
	LabelAbove *bool // label position override; nil = Profile().SliderLabelAbove
	// SuppressLabel, when true, skips rendering the percent label and
	// reserves no horizontal space for it. Use when the surrounding
	// component already shows the value (e.g. FX-panel parameter rows
	// render `Drive: 8.19` next to the slider, making "80%" duplicate).
	SuppressLabel bool
}

func NewSlider(v float64) *Slider { return &Slider{Value: v} }

func (s *Slider) SetRect(r image.Rectangle) { s.r = r }

func (s *Slider) Rect() image.Rectangle { return s.r }

// HandleInputResult processes mouse interaction and returns an InputResult.
// Returns InputCaptured during an active drag, InputConsumed on release after
// a drag, and InputIgnored when the slider is not involved.
func (s *Slider) HandleInputResult(mx, my int, pressed bool) InputResult {
	if suppressClicksUntilRelease {
		if !pressed {
			suppressClicksUntilRelease = false
		}
		s.dragging = false
		return InputIgnored
	}
	if pressed {
		if s.dragging || image.Pt(mx, my).In(s.r) {
			s.dragging = true
			s.setFromX(mx)
			return InputCaptured
		}
	} else if s.dragging {
		s.dragging = false
		return InputConsumed
	}
	return InputIgnored
}

// Capturing returns whether the slider is in an active drag.
func (s *Slider) Capturing() bool { return s.dragging }

func (s *Slider) setFromX(mx int) {
	track, _ := s.trackRect()
	w := track.Dx() - 1
	if w <= 0 {
		s.Value = 0
		return
	}
	pos := mx - track.Min.X
	if pos < 0 {
		pos = 0
	}
	if pos > w {
		pos = w
	}
	s.Value = float64(pos) / float64(w)
}

// Draw renders the slider and its percentage label.
func (s *Slider) Draw(dst *ebiten.Image) {
	track, spr := s.trackRect()

	// Track height: thicker on mobile for easier interaction.
	// Use per-slider override if set, otherwise fall back to Profile.
	p := Profile()
	trackH := p.SliderTrackH
	if s.TrackH > 0 {
		trackH = s.TrackH
	}
	thumbH := p.SliderThumbH
	thumbW := p.SliderThumbW
	midY := track.Min.Y + track.Dy()/2
	trackY := midY - trackH/2
	trackDraw := image.Rect(track.Min.X, trackY, track.Max.X, trackY+trackH)

	// Rounded track background.
	trackRadius := trackH / 2
	drawRoundedRect(dst, trackDraw, genColorSliderTrackFill, trackRadius, true)

	// Filled portion (accent color from left to current value).
	knobX := track.Min.X + int(s.Value*float64(track.Dx()-1))
	// Ensure thumb is always visible — at least thumbW/2+1 from track start.
	minKnobX := track.Min.X + thumbW/2 + 1
	if knobX < minKnobX {
		knobX = minKnobX
	}
	fillRect := image.Rect(track.Min.X, trackY, knobX, trackY+trackH)
	if fillRect.Dx() > 0 {
		drawRoundedRect(dst, fillRect, WithAlpha(genColorSliderFill, genAlphaOverlay), trackRadius, true)
	}

	// Thumb/handle — a visible rounded rectangle centered on the value position.
	thumbRect := image.Rect(knobX-thumbW/2, midY-thumbH/2, knobX+thumbW/2, midY+thumbH/2)
	thumbRadius := thumbW / 2
	var thumbCol color.Color = genColorSliderThumbFill
	if s.dragging {
		thumbCol = genColorBorder // pure white when dragging — matches the existing brighter look
	}
	drawRoundedRect(dst, thumbRect, thumbCol, thumbRadius, true)
	// Subtle border on thumb for definition.
	drawRoundedRect(dst, thumbRect, WithAlpha(genColorSliderThumbShadow, genAlphaSubtle), thumbRadius, false)

	// Label position: use per-slider override if set, otherwise Profile default.
	labelAbove := p.SliderLabelAbove
	if s.LabelAbove != nil {
		labelAbove = *s.LabelAbove
	}

	if s.SuppressLabel {
		return
	}

	// Label: above the track when labelAbove is set, to the left otherwise.
	pct := int(math.Round(s.Value * 100))
	if pct != s.lastPct {
		s.lastPct = pct
		s.lastLabel = fmt.Sprintf("%d%%", pct)
		spr = TextSprite(s.lastLabel)
	}
	if spr == nil {
		spr = TextSprite(s.lastLabel)
	}
	if labelAbove {
		// Position label above the track so it doesn't overlap.
		labelX := float64(track.Min.X+track.Dx()/2) - float64(spr.Bounds().Dx())/2
		labelY := float64(trackY) - float64(spr.Bounds().Dy()) - 2
		if labelY < float64(s.r.Min.Y) {
			labelY = float64(s.r.Min.Y)
		}
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(labelX, labelY)
		dst.DrawImage(spr, &op)
	} else {
		labelX := float64(track.Min.X - spr.Bounds().Dx() - sliderLabelPad)
		labelY := float64(midY - spr.Bounds().Dy()/2)
		if labelX < float64(s.r.Min.X) {
			labelX = float64(s.r.Min.X)
		}
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(labelX, labelY)
		dst.DrawImage(spr, &op)
	}
}

func (s *Slider) trackRect() (image.Rectangle, *ebiten.Image) {
	if s.SuppressLabel {
		return s.r, nil
	}
	label := fmt.Sprintf("%d%%", int(math.Round(s.Value*100)))
	spr := TextSprite(label)
	labelW := spr.Bounds().Dx() + sliderLabelPad
	// Ensure the track gets at least minTrackW pixels so setFromX
	// can resolve a meaningful value even when the label is wide
	// (e.g. TrueType fonts in real Ebiten builds).
	const minTrackW = 20
	maxLabelW := s.r.Dx() - minTrackW
	if maxLabelW < 0 {
		maxLabelW = 0
	}
	if labelW > maxLabelW {
		labelW = maxLabelW
	}
	trackMinX := s.r.Min.X + labelW
	track := image.Rect(trackMinX, s.r.Min.Y, s.r.Max.X, s.r.Max.Y)
	return track, spr
}

// TrackRect exposes the current slider track rectangle for tests/layout logic.
func (s *Slider) TrackRect() image.Rectangle {
	rect, _ := s.trackRect()
	return rect
}
