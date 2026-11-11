package ui

import "image"

// SliderGroup wraps a set of sliders with automatic capture semantics,
// implementing InputHandler. It replaces the manual activeSlider/activeSliderKind
// tracking by managing which slider is capturing input internally.
//
// When a press starts inside any slider, that slider captures all subsequent
// input until release. The onChange callback fires whenever the captured slider's
// value changes.
type SliderGroup struct {
	sliders         []*Slider
	onChange        func(idx int, val float64)
	active          int             // index of the slider currently capturing, -1 if none
	containerBounds image.Rectangle // if non-empty, reject new presses outside this rect
}

// NewSliderGroup creates a new SliderGroup. The onChange callback receives the
// slider index and its current value whenever the value changes during a drag.
func NewSliderGroup(sliders []*Slider, onChange func(idx int, val float64)) *SliderGroup {
	return &SliderGroup{
		sliders:  sliders,
		onChange: onChange,
		active:   -1,
	}
}

// SetSliders replaces the slider set. Preserves the active capture index
// only when the active slider pointer is the same object (reposition case).
// Releases capture when the set changes or the active index is out of range.
func (g *SliderGroup) SetSliders(sliders []*Slider) {
	if g.active >= 0 {
		// Preserve capture only if the same slider object is at the same index.
		if g.active < len(sliders) && g.active < len(g.sliders) && sliders[g.active] == g.sliders[g.active] {
			g.sliders = sliders
			return
		}
		g.active = -1
	}
	g.sliders = sliders
}

// Sliders returns the current slider set.
func (g *SliderGroup) Sliders() []*Slider { return g.sliders }

// SetContainerBounds sets an optional spatial constraint. When non-empty,
// new presses outside this rectangle are rejected before testing individual
// sliders. Active drags are unaffected (the cursor can leave the container
// while dragging without releasing).
func (g *SliderGroup) SetContainerBounds(r image.Rectangle) { g.containerBounds = r }

// Active returns the index of the currently captured slider, or -1.
func (g *SliderGroup) Active() int { return g.active }

// Capturing returns true if any slider in the group is actively being dragged.
func (g *SliderGroup) Capturing() bool { return g.active >= 0 }

// HandleInput dispatches input to the slider group.
// If a slider is already captured, it receives all input until release.
// Otherwise, each slider is tested in order for a new press.
// Returns InputCaptured during drag, InputConsumed on release, InputIgnored otherwise.
func (g *SliderGroup) HandleInput(mx, my int, pressed bool) InputResult {
	if g.active >= 0 {
		// Already captured — route to the active slider.
		s := g.sliders[g.active]
		result := s.HandleInputResult(mx, my, pressed)
		if result == InputCaptured && g.onChange != nil {
			g.onChange(g.active, s.Value)
		}
		if !s.Capturing() {
			g.active = -1
		}
		return result
	}

	// No active capture — test each slider for a new press.
	if !pressed {
		return InputIgnored
	}
	// Reject presses outside the container bounds (if set).
	if !g.containerBounds.Empty() && !image.Pt(mx, my).In(g.containerBounds) {
		return InputIgnored
	}
	for i, s := range g.sliders {
		if s == nil {
			continue
		}
		result := s.HandleInputResult(mx, my, pressed)
		if result != InputIgnored {
			g.active = i
			if g.onChange != nil {
				g.onChange(i, s.Value)
			}
			return result
		}
	}
	return InputIgnored
}

// InputBounds returns the union of all slider bounds.
func (g *SliderGroup) InputBounds() image.Rectangle {
	var r image.Rectangle
	for _, s := range g.sliders {
		if s == nil {
			continue
		}
		if r.Empty() {
			r = s.Rect()
		} else {
			r = r.Union(s.Rect())
		}
	}
	return r
}

// ZIndex returns 0. Override if needed.
func (g *SliderGroup) ZIndex() int { return 0 }

// HandleWheel returns InputIgnored — sliders do not process wheel events.
func (g *SliderGroup) HandleWheel(_, _, _ int) InputResult { return InputIgnored }

// Release forces capture release without firing onChange.
func (g *SliderGroup) Release() {
	if g.active >= 0 && g.active < len(g.sliders) {
		s := g.sliders[g.active]
		if s.dragging {
			s.dragging = false
		}
	}
	g.active = -1
}
