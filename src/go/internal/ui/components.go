package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// NodeStyle defines visual appearance for graph nodes.
type NodeStyle struct {
	Radius float32
	Fill   color.Color
	Border color.Color
}

// Draw renders a node at world coordinates using the provided camera matrix.
func (s NodeStyle) Draw(dst *ebiten.Image, x, y float64, cam *ebiten.GeoM) {
	size := float64(s.Radius) * 2
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(size, size)
	op.GeoM.Translate(x-float64(s.Radius), y-float64(s.Radius))
	op.GeoM.Concat(*cam)
	dst.DrawImage(pixel(s.Fill), &op)
	DrawLineCam(dst, x-float64(s.Radius), y-float64(s.Radius), x+float64(s.Radius), y-float64(s.Radius), cam, s.Border, 1)
	DrawLineCam(dst, x+float64(s.Radius), y-float64(s.Radius), x+float64(s.Radius), y+float64(s.Radius), cam, s.Border, 1)
	DrawLineCam(dst, x+float64(s.Radius), y+float64(s.Radius), x-float64(s.Radius), y+float64(s.Radius), cam, s.Border, 1)
	DrawLineCam(dst, x-float64(s.Radius), y+float64(s.Radius), x-float64(s.Radius), y-float64(s.Radius), cam, s.Border, 1)
}

// SignalStyle defines the appearance of travelling pulses between nodes.
type SignalStyle struct {
	Radius float32
	Color  color.Color
}

// Draw renders the signal at world coordinates with the given camera transform.
func (s SignalStyle) Draw(dst *ebiten.Image, x, y float64, cam *ebiten.GeoM) {
	size := float64(s.Radius) * 2
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(size, size)
	op.GeoM.Translate(x-float64(s.Radius), y-float64(s.Radius))
	op.GeoM.Concat(*cam)
	dst.DrawImage(pixel(s.Color), &op)
}

// EdgeStyle draws directional edges between nodes.
type EdgeStyle struct {
	Color     color.Color
	Thickness float64
	ArrowSize float64
}

// Draw renders a directed edge from (x1,y1) to (x2,y2) using cam.
func (s EdgeStyle) Draw(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM) {
	DrawLineCam(dst, x1, y1, x2, y2, cam, s.Color, s.Thickness)
	angle := math.Atan2(y2-y1, x2-x1)
	leftX := x2 - s.ArrowSize*math.Cos(angle-math.Pi/6)
	leftY := y2 - s.ArrowSize*math.Sin(angle-math.Pi/6)
	rightX := x2 - s.ArrowSize*math.Cos(angle+math.Pi/6)
	rightY := y2 - s.ArrowSize*math.Sin(angle+math.Pi/6)
	DrawLineCam(dst, x2, y2, leftX, leftY, cam, s.Color, s.Thickness)
	DrawLineCam(dst, x2, y2, rightX, rightY, cam, s.Color, s.Thickness)
}

// ButtonStyle describes rectangular button visuals.
type ButtonStyle struct {
	Fill   color.Color
	Border color.Color
}

// Draw renders the button rectangle using the global drawButton primitive.
func (s ButtonStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed bool) {
	drawButton(dst, r, s.Fill, s.Border, pressed)
}

// TextInputStyle styles a text input box.
type TextInputStyle struct {
	Fill   color.Color
	Border color.Color
}

// Draw renders the text box using drawButton for consistency.
func (s TextInputStyle) Draw(dst *ebiten.Image, r image.Rectangle, focused bool) {
	drawButton(dst, r, s.Fill, s.Border, focused)
}

// DrumCellStyle styles individual drum machine cells.
type DrumCellStyle struct {
	On        color.Color
	Off       color.Color
	Highlight color.Color
	Border    color.Color
}

// Draw renders a drum cell considering its state. onCol overrides the default On color.
func (s DrumCellStyle) Draw(dst *ebiten.Image, r image.Rectangle, on, highlighted bool, onCol color.Color) {
	fill := s.Off
	if on {
		if onCol != nil {
			fill = onCol
		} else {
			fill = s.On
		}
	}
	if highlighted {
		fill = s.Highlight
	}
	drawRect(dst, r, fill, true)
	drawRect(dst, r, s.Border, false)
}

// DrumRowStyle is reserved for future customisation of entire rows.
type DrumRowStyle struct{}
