package ui

import "image"

// overlayBase provides minimal shared state for overlay components:
// a layout bounds rectangle. It replaces the heavier BaseComponent
// that was removed during the component-system cleanup.
type overlayBase struct {
	bounds image.Rectangle
}

func newOverlayBase() overlayBase {
	return overlayBase{}
}

func (o *overlayBase) Bounds() image.Rectangle     { return o.bounds }
func (o *overlayBase) SetBounds(r image.Rectangle) { o.bounds = r }
