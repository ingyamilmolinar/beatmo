package ui

import "image"

type overlayBase struct {
	bounds image.Rectangle
}

func newOverlayBase() overlayBase {
	return overlayBase{}
}

func (o *overlayBase) Bounds() image.Rectangle     { return o.bounds }
func (o *overlayBase) SetBounds(r image.Rectangle) { o.bounds = r }
