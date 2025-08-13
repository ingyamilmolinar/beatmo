package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// pixel helper is defined in widgets.go and reused here.

// fadeColor returns c with its alpha scaled by t (0..1).
func fadeColor(c color.Color, t float64) color.Color {
	r, g, b, a := c.RGBA()
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(float64(a>>8) * t)}
}

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
	// Outer glow
	var op ebiten.DrawImageOptions
	glowSize := size * 1.5
	op.GeoM.Scale(glowSize, glowSize)
	op.GeoM.Translate(x-glowSize/2, y-glowSize/2)
	op.GeoM.Concat(*cam)
	dst.DrawImage(pixel(fadeColor(s.Color, 0.3)), &op)

	// Inner core
	var op2 ebiten.DrawImageOptions
	op2.GeoM.Scale(size, size)
	op2.GeoM.Translate(x-size/2, y-size/2)
	op2.GeoM.Concat(*cam)
	dst.DrawImage(pixel(s.Color), &op2)
}

// EdgeStyle draws directional edges between nodes.
type EdgeStyle struct {
	Color     color.Color
	Thickness float64
	ArrowSize float64
}

// Draw renders a directed edge from (x1,y1) to (x2,y2) using cam.
func (s EdgeStyle) Draw(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM) {
	s.DrawProgress(dst, x1, y1, x2, y2, cam, 1)
}

// DrawProgress renders a portion of the edge according to progress t
// (0..1). When t reaches 1 the arrow head is drawn.
func (s EdgeStyle) DrawProgress(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM, t float64) {
	if t <= 0 {
		return
	}
	if t > 1 {
		t = 1
	}
	col := fadeColor(s.Color, t)
	ex := x1 + (x2-x1)*t
	ey := y1 + (y2-y1)*t
        DrawLineCam(dst, x1, y1, ex, ey, cam, col, s.Thickness)
        if t < 1 {
                return
        }
        angle := math.Atan2(y2-y1, x2-x1)

        // Arrowhead at the end of the edge
        leftX := x2 - s.ArrowSize*math.Cos(angle-math.Pi/6)
        leftY := y2 - s.ArrowSize*math.Sin(angle-math.Pi/6)
        rightX := x2 - s.ArrowSize*math.Cos(angle+math.Pi/6)
        rightY := y2 - s.ArrowSize*math.Sin(angle+math.Pi/6)
        DrawLineCam(dst, x2, y2, leftX, leftY, cam, col, s.Thickness)
        DrawLineCam(dst, x2, y2, rightX, rightY, cam, col, s.Thickness)

        // Permanent direction marker at midpoint
        mx := (x1 + x2) / 2
        my := (y1 + y2) / 2
        midLeftX := mx - s.ArrowSize*math.Cos(angle-math.Pi/6)
        midLeftY := my - s.ArrowSize*math.Sin(angle-math.Pi/6)
        midRightX := mx - s.ArrowSize*math.Cos(angle+math.Pi/6)
        midRightY := my - s.ArrowSize*math.Sin(angle+math.Pi/6)
        DrawLineCam(dst, mx, my, midLeftX, midLeftY, cam, col, s.Thickness)
        DrawLineCam(dst, mx, my, midRightX, midRightY, cam, col, s.Thickness)
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

// DrawAnimated draws the button with a shrink animation controlled by anim
// (0..1). anim is typically set to 1 on click and decays toward 0.
func (s ButtonStyle) DrawAnimated(dst *ebiten.Image, r image.Rectangle, pressed bool, anim float64) {
	if anim < 0 {
		anim = 0
	}
	inset := int(anim * float64(r.Dx()) * 0.1)
	animRect := image.Rect(r.Min.X+inset, r.Min.Y+inset, r.Max.X-inset, r.Max.Y-inset)
	drawButton(dst, animRect, s.Fill, s.Border, pressed)
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

// DrawAnimated draws the text box with a subtle focus animation.
func (s TextInputStyle) DrawAnimated(dst *ebiten.Image, r image.Rectangle, focused bool, anim float64) {
	if anim < 0 {
		anim = 0
	}
	inset := int(anim * float64(r.Dx()) * 0.1)
	animRect := image.Rect(r.Min.X+inset, r.Min.Y+inset, r.Max.X-inset, r.Max.Y-inset)
	drawButton(dst, animRect, s.Fill, s.Border, focused)
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
