package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Layout → Update → Input → Draw every frame. Layout is lazy (NeedsLayout or a rect change);
// Draw must never compute geometry.
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

type HitArea struct {
	Rect     image.Rectangle
	ZIndex   int
	Handler  HitHandler
	Tag      string          // debug label
	Touch    bool            // if true, HitIndex expands rect by TouchMinTarget()
	ClipRect image.Rectangle // optional: constrains touch expansion (zero = no clip)
}

// The tree owns capture: one captured handler per tree, not per zone.
type HitHandler interface {
	OnPress(x, y int) InputResult
	OnDrag(x, y int)
	OnRelease(x, y int)
	OnWheel(x, y, steps int) InputResult
}

// Knobs take both axes: left/right changes the value, up/down moves between overflow rows. dx > 0 =
// right, dy > 0 = down.
type wheel2DHandler interface {
	OnWheel2D(x, y, dx, dy int) InputResult
}

// Function fields, not pointers, so zones always read current values.
type DrumViewState struct {
	IsPlaying     func() bool
	CurrentBeat   func() int
	BPM           func() int
	Rows          func() []*DrumRow
	IsSmallScreen func() bool
	ScreenSize    func() (int, int)
}
