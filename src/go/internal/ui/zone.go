package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Zone is a self-contained UI region within the DrumView component tree.
// Each zone owns its geometry, input handling, and rendering for a logical
// section of the drum view (transport, row rack, timeline, EQ panel).
//
// Lifecycle is strict 4-phase per frame: Layout → Update → Input → Draw.
// Layout is lazy — only runs when NeedsLayout() returns true or the zone's
// rect changes. Draw must never compute geometry.
type Zone interface {
	ID() string
	Layout(rect image.Rectangle)
	Update()
	HitAreas() []HitArea
	Draw(screen *ebiten.Image)
	NeedsLayout() bool
	Invalidate()
	HandleKey(key ebiten.Key) InputResult
	HandleChars(chars []rune) InputResult
}

// HitArea declares an interactive region within a zone. The HitIndex
// collects these from all zones and overlays to route input by z-order.
type HitArea struct {
	Rect     image.Rectangle
	ZIndex   int
	Handler  HitHandler
	Tag      string          // debug label
	Touch    bool            // if true, HitIndex expands rect by TouchMinTarget()
	ClipRect image.Rectangle // optional: constrains touch expansion (zero = no clip)
}

// HitHandler processes pointer events for a HitArea. The tree owns capture
// state — one capturedHandler per tree, not per zone.
type HitHandler interface {
	OnPress(x, y int) InputResult
	OnDrag(x, y int)
	OnRelease(x, y int)
	OnWheel(x, y, steps int) InputResult
}

// wheel2DHandler is an optional HitHandler extension for handlers that need the
// raw two-axis wheel delta (a two-finger trackpad drag). Knobs use it so a
// LEFT/RIGHT scroll changes the value while an UP/DOWN scroll is reserved for
// moving between overflow rows. The tree prefers OnWheel2D over OnWheel when a
// handler implements it; dx>0 = scroll right, dy>0 = scroll down.
type wheel2DHandler interface {
	OnWheel2D(x, y, dx, dy int) InputResult
}

// DrumViewState provides read-only access to shared mutable state.
// Function fields (not direct pointers) so zones always read current values.
type DrumViewState struct {
	IsPlaying     func() bool
	CurrentBeat   func() int
	BPM           func() int
	Rows          func() []*DrumRow
	IsSmallScreen func() bool
	ScreenSize    func() (int, int)
}
