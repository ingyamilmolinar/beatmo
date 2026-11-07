package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

type actionMenuItem struct {
	Label       string
	Destructive bool
	OnSelect    func()
}

// actionMenuProps is the external state for actionMenuComponent.
type actionMenuProps struct {
	AnchorRect image.Rectangle
	Items      []actionMenuItem
	OnClose    func()
	RowHeight  int
}

// The string sibling of SubdivMenuComponent, built from the same overlay machinery so it behaves like every
// other list menu. No scroll: the item count is tiny and fixed.
type actionMenuComponent struct {
	overlayBase
	props        actionMenuProps
	open         bool
	buttons      []*Button
	cardRect     image.Rectangle
	screenBounds image.Rectangle
	nav          *MenuKeyNav
}

func newActionMenuComponent() *actionMenuComponent {
	return &actionMenuComponent{overlayBase: newOverlayBase()}
}

func (m *actionMenuComponent) SetScreenBounds(b image.Rectangle) {
	if b == m.screenBounds {
		return
	}
	m.screenBounds = b
	if m.open {
		m.rebuild()
	}
}

// SetProps replaces the external props and rebuilds if open.
func (m *actionMenuComponent) SetProps(p actionMenuProps) {
	m.props = p
	if m.open {
		m.rebuild()
	}
}

func (m *actionMenuComponent) Open()              { m.open = true; m.rebuild() }
func (m *actionMenuComponent) IsOpen() bool       { return m.open }
func (m *actionMenuComponent) Buttons() []*Button { return m.buttons }
func (m *actionMenuComponent) CardRect() image.Rectangle {
	return m.cardRect
}

func (m *actionMenuComponent) Close() {
	if !m.open {
		return
	}
	m.open = false
	m.buttons = nil
	if m.nav != nil {
		// The next open starts with nothing highlighted, not on a row of a different item set.
		m.nav.Reset()
	}
	if m.props.OnClose != nil {
		m.props.OnClose()
	}
}

func (m *actionMenuComponent) rebuild() {
	m.buttons = m.buttons[:0]
	m.cardRect = image.Rectangle{}
	if len(m.props.Items) == 0 || m.props.AnchorRect.Empty() {
		m.SetBounds(image.Rectangle{})
		return
	}
	base := m.props.AnchorRect
	rowH := m.props.RowHeight
	if rowH <= 0 {
		rowH = 24
	}
	pad := SpaceSM
	n := len(m.props.Items)

	if Profile().UseBottomSheet {
		bounds := m.screenBounds
		if bounds.Empty() {
			bounds = base
		}
		cardH := n*rowH + pad*2
		if cardH > bounds.Dy() {
			cardH = bounds.Dy()
		}
		m.cardRect = image.Rect(bounds.Min.X, bounds.Max.Y-cardH, bounds.Max.X, bounds.Max.Y)
	} else {
		labelW := 0
		for _, it := range m.props.Items {
			if w := StyledTextWidth(it.Label, RoleBody); w > labelW {
				labelW = w
			}
		}
		cardW := labelW + SpaceMD*2 + pad*2
		if minW := base.Dx(); cardW < minW {
			cardW = minW
		}
		cardH := n*rowH + pad*2
		bounds := m.screenBounds
		if bounds.Empty() {
			bounds = image.Rect(base.Min.X, base.Min.Y, base.Min.X+cardW, base.Max.Y+cardH+rowH)
		}
		m.cardRect = AnchorPopupRect(bounds, base, cardW, cardH, PopupBelow)
	}

	itemX0 := m.cardRect.Min.X + pad
	itemX1 := m.cardRect.Max.X - pad
	for i, it := range m.props.Items {
		y0 := m.cardRect.Min.Y + pad + i*rowH
		r := image.Rect(itemX0, y0, itemX1, y0+rowH)
		sel := it.OnSelect
		btn := NewButton(it.Label, DropdownStyle, func() {
			if sel != nil {
				sel()
			}
			m.Close()
		})
		btn.ConsumeOnPress = true
		if it.Destructive {
			btn.TextColor = colError
		}
		btn.SetRect(insetRect(r, SpaceXS))
		m.buttons = append(m.buttons, btn)
	}
	m.SetBounds(m.cardRect)
}

// Draw renders the card + rows through the shared menu-row primitive.
func (m *actionMenuComponent) Draw(dst *ebiten.Image) {
	if !m.open || m.cardRect.Empty() {
		return
	}
	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, m.cardRect)
		handleW, handleH := 36, 4
		hx := m.cardRect.Min.X + m.cardRect.Dx()/2 - handleW/2
		hy := m.cardRect.Min.Y + 8
		drawCapsule(dst, image.Rect(hx, hy, hx+handleW, hy+handleH),
			WithAlpha(genColorBorder, genAlphaScrollbarThumb), true) // a drag handle is a capsule
	} else {
		drawPanel(dst, m.cardRect)
	}
	for i, btn := range m.buttons {
		m.drawRow(dst, btn, i)
	}
}

func (m *actionMenuComponent) drawRow(dst *ebiten.Image, btn *Button, i int) {
	if btn.Rect().Empty() {
		return
	}
	state := menuItemRest
	var labelCol color.Color // nil → colTextPrimary in drawMenuRow
	if i < len(m.props.Items) && m.props.Items[i].Destructive {
		labelCol = colError
	}
	if btn.hovered || btn.pressed || i == m.KeyNav().Selected() {
		state = menuItemHover
	}
	drawMenuRow(dst, btn, MenuRowSpec{
		State:       state,
		Label:       btn.Text,
		LabelColor:  labelCol,
		CenterLabel: true,
	})
}

// --- compOverlayable / portalHoverer implementation ---

func (m *actionMenuComponent) HoverAt(mx, my int) {
	for _, b := range m.buttons {
		b.UpdateHover(mx, my)
	}
}

func (m *actionMenuComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !m.open {
		return InputIgnored
	}
	pt := image.Pt(x, y)
	for _, btn := range m.buttons {
		if btn.HandleInputResult(x, y, pressed) != InputIgnored {
			return InputConsumed
		}
	}
	// Include the anchor so a tap on the ⋯ button while open doesn't close it.
	fullBounds := m.bounds.Union(m.props.AnchorRect)
	if pressed && !pt.In(fullBounds) {
		m.Close()
		return InputConsumed
	}
	if pt.In(fullBounds) {
		return InputConsumed
	}
	return InputIgnored
}

func (m *actionMenuComponent) Capturing() bool { return false }

func (m *actionMenuComponent) InputBounds() image.Rectangle {
	if !m.open {
		return image.Rectangle{}
	}
	return m.bounds.Union(m.props.AnchorRect)
}

func (m *actionMenuComponent) HandleWheel(x, y, steps int) InputResult {
	if !m.open {
		return InputIgnored
	}
	return InputConsumed
}

// Without this the arrows fall through to the grid's arrow-pan under an open menu.
func (m *actionMenuComponent) ClaimsKeyboard() bool { return m != nil && m.open }

// KeyNav is the menu's keyboard cursor, created on first use.
func (m *actionMenuComponent) KeyNav() *MenuKeyNav {
	if m.nav == nil {
		m.nav = NewMenuKeyNav()
	}
	return m.nav
}

// Driven from the portal's per-frame update so the arrows work wherever the pointer is.
func (m *actionMenuComponent) UpdateKeyNav() {
	if !m.open {
		if m.nav != nil {
			m.nav.Reset()
		}
		return
	}
	m.KeyNav().Update(m)
}

// --- MenuKeyNavHost ---

func (m *actionMenuComponent) MenuItemCount() int { return len(m.buttons) }

// Every row is an action, so all rows are selectable.
func (m *actionMenuComponent) MenuItemSelectable(i int) bool {
	return i >= 0 && i < len(m.buttons) && m.buttons[i] != nil && m.buttons[i].OnClick != nil
}

// Identical to clicking it, including the Close the row closure performs.
func (m *actionMenuComponent) MenuActivate(i int) {
	if !m.MenuItemSelectable(i) {
		return
	}
	m.buttons[i].OnClick()
}

// The card never scrolls, so there is nothing to reveal.
func (m *actionMenuComponent) MenuScrollBehavior() *ScrollBehavior { return nil }
