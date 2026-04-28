package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// SubdivMenuProps contains the external state passed to the subdiv menu component.
type SubdivMenuProps struct {
	// AnchorRect is the button that opens the menu (used for positioning).
	AnchorRect image.Rectangle
	// Current is the current subdivision value.
	Current int
	// Options are the available subdivision values.
	Options []int
	// OnSelect is called when a subdivision is selected.
	OnSelect func(value int)
	// OnClose is called when the menu should close.
	OnClose func()
	// RowHeight is used for calculating button heights.
	RowHeight int
}

// SubdivMenuState contains the internal state for the subdiv menu component.
type SubdivMenuState struct {
	open bool
}

// SubdivMenuComponent is a self-contained dropdown menu for selecting subdivisions.
type SubdivMenuComponent struct {
	overlayBase
	props   SubdivMenuProps
	state   SubdivMenuState
	buttons []*Button
}

// NewSubdivMenuComponent creates a new subdivision menu component.
func NewSubdivMenuComponent() *SubdivMenuComponent {
	return &SubdivMenuComponent{
		overlayBase: newOverlayBase(),
	}
}

// SetProps updates the external props.
func (s *SubdivMenuComponent) SetProps(p SubdivMenuProps) {
	needsRebuild := p.AnchorRect != s.props.AnchorRect ||
		p.RowHeight != s.props.RowHeight ||
		len(p.Options) != len(s.props.Options)
	if !needsRebuild {
		for i, opt := range p.Options {
			if opt != s.props.Options[i] {
				needsRebuild = true
				break
			}
		}
	}
	s.props = p
	if needsRebuild && s.state.open {
		s.rebuildButtons()
	}
}

// Props returns the current props.
func (s *SubdivMenuComponent) Props() SubdivMenuProps { return s.props }

// Open opens the menu.
func (s *SubdivMenuComponent) Open() {
	s.state.open = true
	s.rebuildButtons()
}

// Close closes the menu.
func (s *SubdivMenuComponent) Close() {
	s.state.open = false
	s.buttons = nil
	if s.props.OnClose != nil {
		s.props.OnClose()
	}
}

// IsOpen returns whether the menu is currently open.
func (s *SubdivMenuComponent) IsOpen() bool {
	return s.state.open
}

// rebuildButtons recreates the menu buttons based on current props.
func (s *SubdivMenuComponent) rebuildButtons() {
	s.buttons = s.buttons[:0]
	if len(s.props.Options) == 0 || s.props.AnchorRect.Empty() {
		return
	}

	base := s.props.AnchorRect
	rowH := s.props.RowHeight
	if rowH <= 0 {
		rowH = 24 // fallback
	}

	for i, v := range s.props.Options {
		r := image.Rect(
			base.Min.X,
			base.Max.Y+i*rowH,
			base.Max.X,
			base.Max.Y+(i+1)*rowH,
		)
		text := fmt.Sprintf("%d", v)
		vv := v // capture for closure
		btn := NewButton(text, DropdownStyle, func() {
			if s.props.OnSelect != nil {
				s.props.OnSelect(vv)
			}
			s.Close()
		})
		btn.ConsumeOnPress = true
		btn.SetRect(insetRect(r, SpaceXS))
		s.buttons = append(s.buttons, btn)
	}

	// Update component bounds to cover all buttons
	s.updateBounds()
}

// updateBounds recalculates the component bounds based on buttons.
func (s *SubdivMenuComponent) updateBounds() {
	if len(s.buttons) == 0 {
		s.SetBounds(image.Rectangle{})
		return
	}
	first := s.buttons[0].Rect()
	last := s.buttons[len(s.buttons)-1].Rect()
	s.SetBounds(image.Rect(first.Min.X, first.Min.Y, last.Max.X, last.Max.Y))
}

// HandleInput processes mouse input for the subdiv menu.
func (s *SubdivMenuComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !s.state.open {
		return InputIgnored
	}

	pt := image.Pt(x, y)

	// Handle button clicks
	for _, btn := range s.buttons {
		if btn.Handle(x, y, pressed) {
			return InputConsumed
		}
	}

	// Calculate full bounds including anchor button to prevent close on anchor click
	fullBounds := s.bounds.Union(s.props.AnchorRect)

	// If pressed outside full bounds (menu + anchor), close it
	if pressed && !pt.In(fullBounds) {
		s.Close()
		return InputConsumed
	}

	// If within full bounds but not on a menu button, still consume to prevent click-through
	if pt.In(fullBounds) {
		return InputConsumed
	}

	return InputIgnored
}

// Draw renders the subdiv menu.
func (s *SubdivMenuComponent) Draw(dst *ebiten.Image) {
	if !s.state.open {
		return
	}
	for _, btn := range s.buttons {
		btn.Draw(dst)
	}
}

// Capturing returns whether the component is capturing input.
func (s *SubdivMenuComponent) Capturing() bool {
	return false // Subdiv menu doesn't have drag/capture state
}

// InputBounds returns the menu bounds for overlay compatibility.
func (s *SubdivMenuComponent) InputBounds() image.Rectangle {
	if !s.state.open {
		return image.Rectangle{}
	}
	// Include anchor button in bounds to prevent close on anchor click
	return s.bounds.Union(s.props.AnchorRect)
}

// HandleWheel consumes wheel events to prevent pass-through.
func (s *SubdivMenuComponent) HandleWheel(x, y, steps int) InputResult {
	if !s.state.open {
		return InputIgnored
	}
	return InputConsumed
}
