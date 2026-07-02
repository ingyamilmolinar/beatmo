package ui

import (
	"fmt"
	"image"
	"image/color"

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
	// cardRect is the contained popup surface (drawn as a single panel). It is
	// positioned via AnchorPopupRect on rebuild so a bottom-row anchor never
	// runs the menu off-screen, and so the items render as one card rather than
	// floating chips.
	cardRect image.Rectangle
	// screenBounds is the clamp region for AnchorPopupRect, fed by the portal
	// opener (SetScreenBounds) since SubdivMenuProps carries no bounds field.
	screenBounds image.Rectangle
	// menuScroll shares the app-wide scroll component so a long options list
	// (rare, but possible) scrolls like every other list menu.
	menuScroll *MenuScroll
}

// SetScreenBounds sets the clamp region used by AnchorPopupRect. The portal
// opener calls this before Open so the menu positions inside the drum view.
func (s *SubdivMenuComponent) SetScreenBounds(b image.Rectangle) {
	if b == s.screenBounds {
		return
	}
	s.screenBounds = b
	if s.state.open {
		s.rebuildButtons()
	}
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

// Buttons returns the option buttons (in option order). Used by the legacy
// hit-rect mirror (buildSubdivMenu) so JS exports and tests report the same
// rects the card actually renders.
func (s *SubdivMenuComponent) Buttons() []*Button { return s.buttons }

// CardRect returns the contained popup surface rect (empty when closed).
func (s *SubdivMenuComponent) CardRect() image.Rectangle { return s.cardRect }

// rebuildButtons recreates the menu buttons based on current props. The menu
// renders as one contained card positioned via the shared AnchorPopupRect
// primitive (flip + clamp). Desktop drops the card below the ÷n anchor; mobile
// renders a full-width bottom sheet (mirrors the context menu).
func (s *SubdivMenuComponent) rebuildButtons() {
	s.buttons = s.buttons[:0]
	s.cardRect = image.Rectangle{}
	if len(s.props.Options) == 0 || s.props.AnchorRect.Empty() {
		s.SetBounds(image.Rectangle{})
		return
	}

	base := s.props.AnchorRect
	rowH := s.props.RowHeight
	if rowH <= 0 {
		rowH = 24 // fallback
	}

	// Inner padding of the card surface.
	pad := SpaceSM
	nOpts := len(s.props.Options)

	if Profile().UseBottomSheet {
		// Mobile: full-width bottom sheet anchored to the bottom of the bounds.
		bounds := s.screenBounds
		if bounds.Empty() {
			bounds = base
		}
		cardH := nOpts*rowH + pad*2
		if cardH > bounds.Dy() {
			cardH = bounds.Dy()
		}
		s.cardRect = image.Rect(bounds.Min.X, bounds.Max.Y-cardH, bounds.Max.X, bounds.Max.Y)
	} else {
		// Desktop: size the card width from the widest RoleBody-styled label so
		// "16"/"32" never truncate to "···". Width = widest label + icon-free
		// padding; height = items + card padding.
		labelW := 0
		for _, v := range s.props.Options {
			if w := StyledTextWidth(fmt.Sprintf("%d", v), RoleBody); w > labelW {
				labelW = w
			}
		}
		cardW := labelW + SpaceMD*2 + pad*2
		if minW := base.Dx(); cardW < minW {
			cardW = minW
		}
		cardH := nOpts*rowH + pad*2
		bounds := s.screenBounds
		if bounds.Empty() {
			// No clamp region: fall back to a direct below-anchor placement.
			bounds = image.Rect(base.Min.X, base.Min.Y, base.Min.X+cardW, base.Max.Y+cardH+rowH)
		}
		s.cardRect = AnchorPopupRect(bounds, base, cardW, cardH, PopupBelow)
	}

	// Configure shared scroll over the card's interior.
	if s.menuScroll == nil {
		s.menuScroll = NewMenuScroll(dropdownScrollbarStyle(), rowH)
	}
	view := s.itemViewport()
	visible := view.Dy() / rowH
	if visible < 1 {
		visible = 1
	}
	if visible > nOpts {
		visible = nOpts
	}
	s.menuScroll.Configure(view, nOpts, visible)
	offset := s.menuScroll.OffsetPx()

	// Stacked items, uniform pitch. Active highlight is painted in Draw via
	// drawMenuRow (shared menu-row renderer), so all buttons use the same base
	// DropdownStyle.
	itemX0 := s.cardRect.Min.X + pad
	itemX1 := s.cardRect.Max.X - pad
	for i, v := range s.props.Options {
		y0 := s.cardRect.Min.Y + pad + i*rowH - offset
		r := image.Rect(itemX0, y0, itemX1, y0+rowH)
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

	s.SetBounds(s.cardRect)
}

// HandleInput processes mouse input for the subdiv menu.
func (s *SubdivMenuComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !s.state.open {
		return InputIgnored
	}

	pt := image.Pt(x, y)

	// Scrollbar thumb drag is consumed first so a drag on the bar never falls
	// through to the option buttons. Normal taps still reach the buttons below.
	if s.menuScroll != nil {
		if s.menuScroll.HandleScrollbarDrag(pt, pressed, s.rebuildButtons) {
			return InputConsumed
		}
	}

	// Handle button clicks
	for _, btn := range s.buttons {
		if btn.HandleInputResult(x, y, pressed) != InputIgnored {
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

// Draw renders the subdiv menu as one contained card surface with stacked
// option buttons. The backdrop scrim is painted by the OverlayPortal
// (PortalEntry.Scrim).
func (s *SubdivMenuComponent) Draw(dst *ebiten.Image) {
	if !s.state.open || s.cardRect.Empty() {
		return
	}
	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, s.cardRect)
		// Pill-shaped drag handle at top-center (mirrors context menu).
		handleW, handleH := 36, 4
		hx := s.cardRect.Min.X + s.cardRect.Dx()/2 - handleW/2
		hy := s.cardRect.Min.Y + 8
		drawRoundedRect(dst, image.Rect(hx, hy, hx+handleW, hy+handleH),
			WithAlpha(genColorBorder, genAlphaScrollbarThumb), handleH/2, true)
	} else {
		drawPanel(dst, s.cardRect)
	}
	for i, btn := range s.buttons {
		v := 0
		if i < len(s.props.Options) {
			v = s.props.Options[i]
		}
		s.drawSubdivRow(dst, btn, v)
	}
	if s.menuScroll != nil {
		s.menuScroll.Draw(dst)
	}
}

// drawSubdivRow renders one subdivision option row through the shared
// drawMenuRow primitive (keycap chrome + press/hover animation). Current
// subdivision → active with colTextAccent label; hover/press → hover; rest
// = plain. Label is centered.
func (s *SubdivMenuComponent) drawSubdivRow(dst *ebiten.Image, btn *Button, v int) {
	if btn.Rect().Empty() {
		return
	}
	// Current subdivision → active; hover/press → hover.
	state := menuItemRest
	var labelCol color.Color // nil → colTextPrimary in drawMenuRow
	if v == s.props.Current {
		state = menuItemActive
		labelCol = colTextAccent
	} else if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, btn, MenuRowSpec{
		State:       state,
		Label:       btn.Text,
		LabelColor:  labelCol,
		CenterLabel: true,
	})
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

// HandleWheel scrolls the options list when it overflows, and always consumes
// the event to prevent pass-through to the grid beneath.
func (s *SubdivMenuComponent) HandleWheel(x, y, steps int) InputResult {
	if !s.state.open {
		return InputIgnored
	}
	if s.menuScroll != nil && s.menuScroll.HasScroll() {
		s.menuScroll.HandleWheel(steps)
		s.rebuildButtons()
	}
	return InputConsumed
}

// MenuScrollForTest exposes the scroll component for tests.
func (s *SubdivMenuComponent) MenuScrollForTest() *MenuScroll { return s.menuScroll }

// itemViewport is the scrollable region inside the card (below any pad).
func (s *SubdivMenuComponent) itemViewport() image.Rectangle {
	pad := SpaceSM
	r := s.cardRect
	if r.Empty() {
		return r
	}
	return image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
}
