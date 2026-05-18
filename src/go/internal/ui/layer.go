package ui

import "github.com/hajimehoshi/ebiten/v2"

// Layer is a draw-only participant in the DrumView component tree. Every
// pixel emitted inside the drum pane MUST originate from a Layer or a Zone
// registered with the DrumViewTree. The render-pipeline discipline test
// (render_pipeline_discipline_test.go) enforces this at compile time by
// forbidding draw primitives in drumview_draw.go and drumview_toolbar.go.
//
// A Layer is a Zone-without-input — no Layout/Update/HitAreas/HandleKey.
// Layers are used for decorative chrome (background fills, halos,
// notifications, debug overlays) where input handling lives elsewhere
// (or doesn't exist).
type Layer interface {
	ID() string
	ZIndex() int
	Visible() bool
	Draw(dst *ebiten.Image)
}

// zoneAsLayer adapts a Zone to the Layer interface so the tree can store
// both kinds in one ordered slice. The zone's own Draw method is invoked.
// An optional visibility callback gates the entire zone draw — useful for
// flags that historically gated calls in DrumView.Draw (simpleDraw,
// perfDrawLite). When visible is nil the zone is always drawn.
type zoneAsLayer struct {
	zone    Zone
	zIndex  int
	visible func() bool
}

func (a zoneAsLayer) ID() string             { return a.zone.ID() }
func (a zoneAsLayer) ZIndex() int            { return a.zIndex }
func (a zoneAsLayer) Visible() bool {
	if a.visible == nil {
		return true
	}
	return a.visible()
}
func (a zoneAsLayer) Draw(dst *ebiten.Image) { a.zone.Draw(dst) }
func (a zoneAsLayer) underlyingZone() Zone   { return a.zone }
