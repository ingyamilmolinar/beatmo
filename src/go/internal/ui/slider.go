package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Slider is a horizontal slider component with a 0..1 value.
type Slider struct {
	r        image.Rectangle
	Value    float64
	dragging bool
}

func NewSlider(v float64) *Slider { return &Slider{Value: v} }

func (s *Slider) SetRect(r image.Rectangle) { s.r = r }

func (s *Slider) Rect() image.Rectangle { return s.r }

// Handle processes mouse interaction.
func (s *Slider) Handle(mx, my int, pressed bool) bool {
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
	w := s.r.Dx() - 1
	if w <= 0 {
		s.Value = 0
		return
	}
	pos := float64(mx - s.r.Min.X)
	if pos < 0 {
		pos = 0
	}
	if pos > float64(w) {
		pos = float64(w)
	}
	s.Value = pos / float64(w)
}

// Draw renders the slider and its percentage label.
func (s *Slider) Draw(dst *ebiten.Image) {
	// track
	trackY := s.r.Min.Y + s.r.Dy()/2 - 2
	trackRect := image.Rect(s.r.Min.X, trackY, s.r.Max.X, trackY+4)
	drawRect(dst, trackRect, color.RGBA{80, 80, 80, 255}, true)

	// knob
	knobX := s.r.Min.X + int(s.Value*float64(s.r.Dx()-1))
	knobRect := image.Rect(knobX-2, s.r.Min.Y, knobX+2, s.r.Max.Y)
	drawRect(dst, knobRect, color.RGBA{200, 200, 200, 255}, true)

	// label
	txt := fmt.Sprintf("%d%%", int(s.Value*100))
	ebitenutil.DebugPrintAt(dst, txt, s.r.Min.X, s.r.Min.Y-15)
}
