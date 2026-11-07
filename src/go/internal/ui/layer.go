package ui

import "github.com/hajimehoshi/ebiten/v2"

// Every pixel in the drum pane comes from a Layer or a Zone registered with the tree (enforced by the
// render-pipeline discipline test). A Layer is a Zone without input.
type Layer interface {
	ID() string
	ZIndex() int
	Visible() bool
	Draw(dst *ebiten.Image)
}

// A nil visible means always drawn.
type zoneAsLayer struct {
	zone    Zone
	zIndex  int
	visible func() bool
}

func (a zoneAsLayer) ID() string  { return a.zone.ID() }
func (a zoneAsLayer) ZIndex() int { return a.zIndex }
func (a zoneAsLayer) Visible() bool {
	if a.visible == nil {
		return true
	}
	return a.visible()
}
func (a zoneAsLayer) Draw(dst *ebiten.Image) { a.zone.Draw(dst) }
func (a zoneAsLayer) underlyingZone() Zone   { return a.zone }
