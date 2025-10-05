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

	// track
	trackY := track.Min.Y + track.Dy()/2 - 2
	trackDraw := image.Rect(track.Min.X, trackY, track.Max.X, trackY+4)
	drawRect(dst, trackDraw, color.RGBA{80, 80, 80, 255}, true)

	// knob
	knobX := track.Min.X + int(s.Value*float64(track.Dx()-1))
	knobRect := image.Rect(knobX-2, track.Min.Y, knobX+2, track.Max.Y)
	drawRect(dst, knobRect, color.RGBA{200, 200, 200, 255}, true)

	// label
	pct := int(math.Round(s.Value * 100))
	if pct != s.lastPct {
		s.lastPct = pct
		s.lastLabel = fmt.Sprintf("%d%%", pct)
		spr = TextSprite(s.lastLabel)
	}
	if spr == nil {
		spr = TextSprite(s.lastLabel)
	}
	labelX := float64(track.Min.X - spr.Bounds().Dx() - sliderLabelPad)
	labelY := float64((track.Min.Y+track.Max.Y)/2 - spr.Bounds().Dy()/2)
	if labelX < float64(s.r.Min.X) {
		labelX = float64(s.r.Min.X)
	}
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(labelX, labelY)
	dst.DrawImage(spr, &op)
}

func (s *Slider) trackRect() (image.Rectangle, *ebiten.Image) {
	label := fmt.Sprintf("%d%%", int(math.Round(s.Value*100)))
	spr := TextSprite(label)
	labelW := spr.Bounds().Dx() + sliderLabelPad
	trackMinX := s.r.Min.X + labelW
	if trackMinX >= s.r.Max.X {
		trackMinX = s.r.Max.X - 1
	}
	track := image.Rect(trackMinX, s.r.Min.Y, s.r.Max.X, s.r.Max.Y)
	if track.Dx() <= 1 {
		track = image.Rect(s.r.Min.X, s.r.Min.Y, s.r.Min.X+1, s.r.Max.Y)
	}
	return track, spr
}

// TrackRect exposes the current slider track rectangle for tests/layout logic.
func (s *Slider) TrackRect() image.Rectangle {
	rect, _ := s.trackRect()
	return rect
}
