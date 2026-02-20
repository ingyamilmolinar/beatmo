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
}

func NewSlider(v float64) *Slider { return &Slider{Value: v} }

func (s *Slider) SetRect(r image.Rectangle) { s.r = r }

func (s *Slider) Rect() image.Rectangle { return s.r }

// Handle processes mouse interaction.
func (s *Slider) Handle(mx, my int, pressed bool) bool {
	if suppressClicksUntilRelease {
		if !pressed {
			suppressClicksUntilRelease = false
		}
		s.dragging = false
		return false
	}
	if pressed {
		if s.dragging || image.Pt(mx, my).In(s.r) {
			s.dragging = true
			s.setFromX(mx)
			return true
		}
	} else if s.dragging {
		s.dragging = false
		return true
	}
	return false
}

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
	trackH := 4
	thumbH := 12
	thumbW := 8
	if isSmallScreen() {
		trackH = 6
		thumbH = 18
		thumbW = 10
	}
	midY := track.Min.Y + track.Dy()/2
	trackY := midY - trackH/2
	trackDraw := image.Rect(track.Min.X, trackY, track.Max.X, trackY+trackH)

	// Rounded track background.
	trackRadius := trackH / 2
	drawRoundedRect(dst, trackDraw, color.RGBA{45, 45, 55, 255}, trackRadius, true)

	// Filled portion (accent color from left to current value).
	knobX := track.Min.X + int(s.Value*float64(track.Dx()-1))
	fillRect := image.Rect(track.Min.X, trackY, knobX, trackY+trackH)
	if fillRect.Dx() > 0 {
		drawRoundedRect(dst, fillRect, color.RGBA{0, 150, 200, 220}, trackRadius, true)
	}

	// Thumb/handle — a visible rounded rectangle centered on the value position.
	thumbRect := image.Rect(knobX-thumbW/2, midY-thumbH/2, knobX+thumbW/2, midY+thumbH/2)
	thumbRadius := thumbW / 2
	thumbCol := color.RGBA{230, 230, 235, 255}
	if s.dragging {
		thumbCol = color.RGBA{255, 255, 255, 255}
	}
	drawRoundedRect(dst, thumbRect, thumbCol, thumbRadius, true)
	// Subtle border on thumb for definition.
	drawRoundedRect(dst, thumbRect, color.NRGBA{0, 0, 0, 60}, thumbRadius, false)

	// Label: above the track on mobile, to the left on desktop.
	pct := int(math.Round(s.Value * 100))
	if pct != s.lastPct {
		s.lastPct = pct
		s.lastLabel = fmt.Sprintf("%d%%", pct)
		spr = TextSprite(s.lastLabel)
	}
	if spr == nil {
		spr = TextSprite(s.lastLabel)
	}
	if isSmallScreen() {
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
