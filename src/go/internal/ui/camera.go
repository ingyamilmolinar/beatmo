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

// Reset restores the default zoom and recenters the view (the keyboard "0"
// shortcut). Default scale matches NewCamera (2.0).
func (c *Camera) Reset() {
	c.Scale = 2.0
	c.OffsetX = 0
	c.OffsetY = 0
	c.Snap()
}

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
	debugGeom := (os.Getenv("DEBUG_GEOM") == "1")
	if allowPan {
		if _, wheelY := wheel(); wheelY != 0 {
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
				// Press edge: anchor here, but pan nothing yet. Movement under
				// the tap slop (tapMaxMovePx) is treated as a stationary tap so
				// a click/tap that jitters a few pixels neither pans the grid
				// nor suppresses node creation (handleEditor gates node-add on
				// !camDragged; the touch path gates it on movedBeyondTap). Pan
				// commits only once the pointer crosses the same slop that
				// classifies a touch tap-vs-drag — one threshold for the whole
				// gesture model.
				camDrag = dragTracker{active: true, startX: x, startY: y, lastX: x, lastY: y}
			case camDrag.committed:
				if x != camDrag.lastX || y != camDrag.lastY {
					c.OffsetX += float64(x - camDrag.lastX)
					c.OffsetY += float64(y - camDrag.lastY)
					dragging = true
				}
				camDrag.lastX, camDrag.lastY = x, y
			case abs(x-camDrag.startX) >= tapMaxMovePx || abs(y-camDrag.startY) >= tapMaxMovePx:
				// Crossed the slop deadzone. Commit and pan by the full travel
				// from the press anchor (not just this frame's delta) so no
				// motion is lost when an entire drag lands in one frame — a fast
				// flick, or a single-step programmatic move in a test.
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

// dragTracker is the per-gesture pan state for a single held pointer/touch.
// It replaces the old raw prev-position delta so the camera can apply a slop
// deadzone: a gesture pans only after the pointer travels at least tapMaxMovePx
// from where the button went down. Package-level (one active pointer at a time);
// reset when the button releases so a stale anchor never leaks into the next
// gesture.
type dragTracker struct {
	active    bool // button is currently held
	committed bool // travelled past the slop → panning
	startX    int  // press anchor
	startY    int
	lastX     int // last position while committed (frame-delta base)
	lastY     int
}

var camDrag dragTracker
