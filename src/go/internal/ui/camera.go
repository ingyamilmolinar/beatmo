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

// Snap clamps the camera offsets to integer pixels and limits their magnitude
// so panning across huge distances doesn't accumulate floating-point error.
// Snapping keeps grid lines aligned with world coordinates and avoids
// precision loss when the camera moves very far from the origin.
func (c *Camera) Snap() {
	c.OffsetX = math.Round(c.OffsetX)
	c.OffsetY = math.Round(c.OffsetY)
	const limit = 1e6 // keep offsets in a sane range for numeric stability
	if c.OffsetX > limit {
		c.OffsetX = limit
	} else if c.OffsetX < -limit {
		c.OffsetX = -limit
	}
	if c.OffsetY > limit {
		c.OffsetY = limit
	} else if c.OffsetY < -limit {
		c.OffsetY = -limit
	}
}

// HandleMouse mutates Scale / Offset by reading Ebiten’s mouse state.
// When allowPan is false (e.g. cursor over drum view) the camera ignores
// both wheel zoom and dragging so drum interactions don’t affect the grid.
func (c *Camera) HandleMouse(allowPan bool) bool {
	dragging := false
	if allowPan {
		if _, wheelY := wheel(); wheelY != 0 {
			mx, my := cursorPosition()
			wx := (float64(mx) - c.OffsetX) / c.Scale
			wy := (float64(my) - c.OffsetY) / c.Scale
			const (
				zoomFactor      = 1.05
				zoomSensitivity = 0.1
			)
			newScale := c.Scale * math.Pow(zoomFactor, wheelY*zoomSensitivity)
			const minScale, maxScale = 0.1, 10.0
			if newScale < minScale {
				newScale = minScale
			} else if newScale > maxScale {
				newScale = maxScale
			}
			c.OffsetX = float64(mx) - wx*newScale
			c.OffsetY = float64(my) - wy*newScale
			c.Scale = newScale
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
	c.Snap()
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
