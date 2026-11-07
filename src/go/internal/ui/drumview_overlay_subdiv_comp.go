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
	// Positioned via AnchorPopupRect so a bottom-row anchor never runs the menu off-screen.
	cardRect image.Rectangle
	// Fed by the portal opener: SubdivMenuProps has no bounds field.
	screenBounds image.Rectangle
	menuScroll   *MenuScroll

	nav *MenuKeyNav
}

// Call before Open.
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

// The same rects the card renders, so JS exports and tests agree with the screen.
func (s *SubdivMenuComponent) Buttons() []*Button { return s.buttons }

// CardRect returns the contained popup surface rect (empty when closed).
func (s *SubdivMenuComponent) CardRect() image.Rectangle { return s.cardRect }

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
		// Sized from the widest label so "16"/"32" never truncate.
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

// Portal input handlers only run on press edges, so hover needs its own per-frame pass.
func (s *SubdivMenuComponent) HoverAt(mx, my int) {
	for i, b := range s.buttons {
		b.UpdateHover(mx, my)
		// One highlight shared with the keyboard, so mouse and arrows never light two rows.
		if b.hovered {
			s.keyNav().BindHost(s)
			s.keyNav().SetSelected(i)
		}
	}
}

func (s *SubdivMenuComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !s.state.open {
		return InputIgnored
	}

	pt := image.Pt(x, y)

	// First, so a drag on the bar never falls through to the options.
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
		drawCapsule(dst, image.Rect(hx, hy, hx+handleW, hy+handleH),
			WithAlpha(genColorBorder, genAlphaScrollbarThumb), true)
	} else {
		drawPanel(dst, s.cardRect)
	}
	for i, btn := range s.buttons {
		v := 0
		if i < len(s.props.Options) {
			v = s.props.Options[i]
		}
		s.drawSubdivRow(dst, btn, i, v)
	}
	if s.menuScroll != nil {
		s.menuScroll.Draw(dst)
	}
}

func (s *SubdivMenuComponent) drawSubdivRow(dst *ebiten.Image, btn *Button, idx, v int) {
	if btn.Rect().Empty() {
		return
	}
	// The keyboard cursor and the mouse share one highlight, so read both.
	state := menuItemRest
	var labelCol color.Color // nil → colTextPrimary in drawMenuRow
	if v == s.props.Current {
		state = menuItemActive
		labelCol = colTextAccent
	} else if btn.hovered || btn.pressed || idx == s.keyNav().Selected() {
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

// Always consumed, so nothing passes through to the grid.
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

func (s *SubdivMenuComponent) Update() {
	if !s.state.open {
		return
	}
	// Momentum needs the scroller, keyboard navigation does not: a short list has no MenuScroll.
	if s.menuScroll != nil {
		s.menuScroll.Tick(s.rebuildButtons)
	}
	s.keyNav().Update(s)
}

// keyNav lazily creates this menu's keyboard cursor.
func (s *SubdivMenuComponent) keyNav() *MenuKeyNav {
	if s.nav == nil {
		s.nav = NewMenuKeyNav()
	}
	return s.nav
}

// Every row is a plain choice: no unselectable chrome to skip.

// MenuItemCount implements MenuKeyNavHost.
func (s *SubdivMenuComponent) MenuItemCount() int { return len(s.props.Options) }

// MenuItemSelectable implements MenuKeyNavHost.
func (s *SubdivMenuComponent) MenuItemSelectable(i int) bool {
	return i >= 0 && i < len(s.props.Options)
}

// MenuActivate implements MenuKeyNavHost: identical to clicking the row.
func (s *SubdivMenuComponent) MenuActivate(i int) {
	if i < 0 || i >= len(s.props.Options) {
		return
	}
	v := s.props.Options[i]
	if s.props.OnSelect != nil {
		s.props.OnSelect(v)
	}
	s.Close()
}

// MenuScrollBehavior implements MenuKeyNavHost.
func (s *SubdivMenuComponent) MenuScrollBehavior() *ScrollBehavior {
	if s.menuScroll == nil {
		return nil
	}
	return s.menuScroll.ScrollBehavior()
}

// MenuKeyNavForTest exposes the keyboard cursor for tests.
func (s *SubdivMenuComponent) MenuKeyNavForTest() *MenuKeyNav { return s.keyNav() }

func (s *SubdivMenuComponent) ClaimsKeyboard() bool { return s.state.open }

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
