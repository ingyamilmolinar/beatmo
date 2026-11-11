package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// ClipScope provides render isolation via SubImage.
// This ensures overlays and controls only render within their designated bounds,
// preventing visual bleeding into other UI regions.
type ClipScope struct {
	sub  *ebiten.Image
	clip image.Rectangle
}

// NewClipScope creates a clipped rendering target from a parent image.
// Returns nil if the clip rectangle is empty or doesn't intersect the parent.
func NewClipScope(parent *ebiten.Image, clip image.Rectangle) *ClipScope {
	if parent == nil {
		return nil
	}
	clip = clip.Intersect(parent.Bounds())
	if clip.Empty() {
		return nil
	}
	return &ClipScope{
		sub:  parent.SubImage(clip).(*ebiten.Image),
		clip: clip,
	}
}

// Target returns the clipped image to render into.
// Coordinates should be relative to the clip rectangle's Min point.
func (c *ClipScope) Target() *ebiten.Image {
	if c == nil {
		return nil
	}
	return c.sub
}

// Offset returns the clip rectangle's origin, which should be subtracted
// from world coordinates to get coordinates relative to Target().
func (c *ClipScope) Offset() image.Point {
	if c == nil {
		return image.Point{}
	}
	return c.clip.Min
}

// Bounds returns the clip rectangle in parent coordinates.
func (c *ClipScope) Bounds() image.Rectangle {
	if c == nil {
		return image.Rectangle{}
	}
	return c.clip
}

// DrawAt draws an image at the given position (in parent coordinates),
// automatically adjusting for the clip offset.
func (c *ClipScope) DrawAt(img *ebiten.Image, x, y float64, opts *ebiten.DrawImageOptions) {
	if c == nil || c.sub == nil || img == nil {
		return
	}
	if opts == nil {
		opts = &ebiten.DrawImageOptions{}
	}
	// Adjust for clip offset
	opts.GeoM.Translate(x-float64(c.clip.Min.X), y-float64(c.clip.Min.Y))
	c.sub.DrawImage(img, opts)
}

// TranslateOpts adjusts draw options for the clip offset.
// This is useful when you need more control over the draw operation.
func (c *ClipScope) TranslateOpts(opts *ebiten.DrawImageOptions, x, y float64) {
	if c == nil || opts == nil {
		return
	}
	opts.GeoM.Translate(x-float64(c.clip.Min.X), y-float64(c.clip.Min.Y))
}
