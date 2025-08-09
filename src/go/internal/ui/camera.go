package ui

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Camera owns zoom & pan parameters and exposes a GeoM matrix.
type Camera struct {
	Scale   float64
	OffsetX float64
	OffsetY float64
}

func NewCamera() *Camera { return &Camera{Scale: 2.0} }

// ScreenPos converts world coordinates to screen-space using the current
// camera transform.
func (c *Camera) ScreenPos(x, y float64) (sx, sy float64) {
	sx = x*c.Scale + c.OffsetX
	sy = y*c.Scale + c.OffsetY
	return
}

// GeoM returns the affine transform applied to all world-space drawings.
func (c *Camera) GeoM() ebiten.GeoM {
	var m ebiten.GeoM
	m.Scale(c.Scale, c.Scale)
	m.Translate(c.OffsetX, c.OffsetY)
	return m
}

// GeoMRounded returns a matrix like GeoM but rounds the translation
// to integer pixels. This keeps grid lines and nodes aligned when
// the camera moves with fractional offsets.
func (c *Camera) GeoMRounded() ebiten.GeoM {
	var m ebiten.GeoM
	m.Scale(c.Scale, c.Scale)
	m.Translate(math.Round(c.OffsetX), math.Round(c.OffsetY))
	return m
}

// HandleMouse mutates Scale / Offset by reading Ebiten’s mouse state.
// When allowPan is false (e.g. cursor over drum view) the camera ignores
// both wheel zoom and dragging so drum interactions don’t affect the grid.
func (c *Camera) HandleMouse(allowPan bool) bool {
	dragging := false
	if allowPan {
		if _, wheelY := wheel(); wheelY != 0 {
			if wheelY > 0 {
				c.Scale *= 1.1
			} else {
				c.Scale *= 0.9
			}
		}
		if isMouseButtonPressed(ebiten.MouseButtonLeft) {
			x, y := cursorPosition()
			if last, ok := prevMousePos(); ok {
				if x != last.x || y != last.y {
					c.OffsetX += float64(x - last.x)
					c.OffsetY += float64(y - last.y)
					dragging = true
				}
			}
			markMousePos(x, y)
		} else {
			clearMousePos()
		}
	} else {
		clearMousePos()
	}
	return dragging
}

/* ─── internal helpers ─── */

type mousePos struct{ x, y int }

var lastMouse *mousePos

func prevMousePos() (mousePos, bool) {
	if lastMouse == nil {
		return mousePos{}, false
	}
	return *lastMouse, true
}
func markMousePos(x, y int) { p := mousePos{x, y}; lastMouse = &p }
func clearMousePos()        { lastMouse = nil }
