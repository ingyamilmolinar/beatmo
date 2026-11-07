package ui

import "image"

// The slider a press starts on captures all input until release.
type SliderGroup struct {
	sliders         []*Slider
	onChange        func(idx int, val float64)
	active          int             // index of the slider currently capturing, -1 if none
	containerBounds image.Rectangle // if non-empty, reject new presses outside this rect
}

func NewSliderGroup(sliders []*Slider, onChange func(idx int, val float64)) *SliderGroup {
	return &SliderGroup{
		sliders:  sliders,
		onChange: onChange,
		active:   -1,
	}
}

// The capture survives only if the same slider object is still present (a reposition).
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

// New presses outside are rejected; an active drag may leave the container.
func (g *SliderGroup) SetContainerBounds(r image.Rectangle) { g.containerBounds = r }

// Active returns the index of the currently captured slider, or -1.
func (g *SliderGroup) Active() int { return g.active }

// Capturing returns true if any slider in the group is actively being dragged.
func (g *SliderGroup) Capturing() bool { return g.active >= 0 }

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
