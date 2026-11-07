package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Overlays render only inside their bounds and never bleed into other regions.
type ClipScope struct {
	sub  *ebiten.Image
	clip image.Rectangle
}

// nil when the clip is empty or misses the parent.
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

// Coordinates are relative to the clip's Min.
func (c *ClipScope) Target() *ebiten.Image {
	if c == nil {
		return nil
	}
	return c.sub
}

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

func (c *ClipScope) TranslateOpts(opts *ebiten.DrawImageOptions, x, y float64) {
	if c == nil || opts == nil {
		return
	}
	opts.GeoM.Translate(x-float64(c.clip.Min.X), y-float64(c.clip.Min.Y))
}
