package ui

import (
	"fmt"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

// Camera owns zoom & pan parameters and exposes a GeoM matrix.
type Camera struct {
	Scale   float64
	OffsetX float64
	OffsetY float64
}

func NewCamera() *Camera { return &Camera{Scale: 2.0} }

// The keyboard "0" shortcut; scale 2.0 as in NewCamera.
func (c *Camera) Reset() {
	c.Scale = 2.0
	c.OffsetX = 0
	c.OffsetY = 0
	c.Snap()
}

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

// Integer translation keeps grid lines and nodes aligned under fractional offsets.
func (c *Camera) GeoMRounded() ebiten.GeoM {
	var m ebiten.GeoM
	m.Scale(c.Scale, c.Scale)
	m.Translate(math.Round(c.OffsetX), math.Round(c.OffsetY))
	return m
}

// Bounded offsets stop far pans from accumulating floating-point error.
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

// allowPan false (cursor over the drum view) ignores wheel and drag so drum input never moves the grid.
func (c *Camera) HandleMouse(allowPan bool) bool {
	dragging := false
	debugGeom := (os.Getenv("DEBUG_GEOM") == "1")
	if allowPan {
		// Detents, not raw units: raw browser wheel values are ~100× native ones.
		if _, wheelY := wheelNotches(); wheelY != 0 {
			mx, my := cursorPosition()
			wx := (float64(mx) - c.OffsetX) / c.Scale
			// Account for the transport bar offset in screen space
			wy := (float64(my) - float64(gridTopOffset()) - c.OffsetY) / c.Scale
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
			if debugGeom {
				fmt.Printf("[CAM-ZOOM] before: scale=%.4f off=(%.2f,%.2f) cursor=(%d,%d) anchorWorld=(%.4f,%.4f)\n", c.Scale, c.OffsetX, c.OffsetY, mx, my, wx, wy)
			}
			c.OffsetX = float64(mx) - wx*newScale
			c.OffsetY = float64(my) - float64(gridTopOffset()) - wy*newScale
			c.Scale = newScale
			if debugGeom {
				fmt.Printf("[CAM-ZOOM] after:  scale=%.4f off=(%.2f,%.2f)\n", c.Scale, c.OffsetX, c.OffsetY)
			}
		}
		if isMouseButtonPressed(ebiten.MouseButtonLeft) {
			x, y := cursorPosition()
			switch {
			case !camDrag.active:
				// Pans nothing until the pointer crosses the tap slop, so a jittery click neither pans nor suppresses node
				// creation: one threshold for the whole gesture model.
				camDrag = dragTracker{active: true, startX: x, startY: y, lastX: x, lastY: y}
			case camDrag.committed:
				if x != camDrag.lastX || y != camDrag.lastY {
					c.OffsetX += float64(x - camDrag.lastX)
					c.OffsetY += float64(y - camDrag.lastY)
					dragging = true
				}
				camDrag.lastX, camDrag.lastY = x, y
			case abs(x-camDrag.startX) >= tapMaxMovePx || abs(y-camDrag.startY) >= tapMaxMovePx:
				// Pan by the full travel from the press anchor, so a drag that lands in one frame loses nothing.
				c.OffsetX += float64(x - camDrag.startX)
				c.OffsetY += float64(y - camDrag.startY)
				dragging = true
				camDrag.committed = true
				camDrag.lastX, camDrag.lastY = x, y
			}
		} else {
			camDrag = dragTracker{}
		}
	} else {
		camDrag = dragTracker{}
	}
	c.Snap()
	return dragging
}

/* ─── internal helpers ─── */

// Package-level (one active pointer), reset on release so a stale anchor never leaks into the next gesture.
type dragTracker struct {
	active    bool // button is currently held
	committed bool // travelled past the slop → panning
	startX    int  // press anchor
	startY    int
	lastX     int // last position while committed (frame-delta base)
	lastY     int
}

var camDrag dragTracker
