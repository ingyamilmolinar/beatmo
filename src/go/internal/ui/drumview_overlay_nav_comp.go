package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// A portal component, so press/drag/release, capture, hover, Esc and click-outside all come from
// the tree. A PARTIAL keyboard claimant: the wheel's arrows and Enter; Space still plays.
type NavigationMenuComponent struct {
	overlayBase
	panel *NavigationPanel

	open         bool
	capturing    bool
	screenBounds image.Rectangle // clamp region for AnchorPopupRect
	anchor       image.Rectangle // trigger rect (beat counter) the card drops from
	card         image.Rectangle // positioned popup surface
	closeBtn     *Button         // desktop-only header × (nil on mobile; see newPopupCloseButton)
	frame        *MenuCard       // shared frame: panel / sheet, title band (IconTarget + readout), close ×
	onClose      func()
}

func NewNavigationMenuComponent(panel *NavigationPanel) *NavigationMenuComponent {
	return &NavigationMenuComponent{
		overlayBase: newOverlayBase(),
		panel:       panel,
	}
}

// Panel returns the wrapped NavigationPanel (control rects, callbacks).
func (c *NavigationMenuComponent) Panel() *NavigationPanel { return c.panel }

func (c *NavigationMenuComponent) CloseBtn() *Button { return c.closeBtn }

// SetOnClose registers a callback fired when the menu closes (portal cleanup).
func (c *NavigationMenuComponent) SetOnClose(f func()) { c.onClose = f }

// SetScreenBounds feeds the clamp region for AnchorPopupRect / the bottom sheet.
func (c *NavigationMenuComponent) SetScreenBounds(b image.Rectangle) {
	if b == c.screenBounds {
		return
	}
	c.screenBounds = b
	if c.open {
		c.rebuild()
	}
}

// Menus drop from their trigger.
func (c *NavigationMenuComponent) SetAnchor(r image.Rectangle) {
	if r == c.anchor {
		return
	}
	c.anchor = r
	if c.open {
		c.rebuild()
	}
}

func (c *NavigationMenuComponent) Open() {
	c.open = true
	if c.panel != nil {
		c.panel.ResetGoto()
	}
	c.rebuild()
}

// Close closes the menu (and cancels an in-flight editor).
func (c *NavigationMenuComponent) Close() {
	if !c.open {
		return
	}
	c.open = false
	c.capturing = false
	if c.panel != nil {
		c.panel.CancelGoto() // closing (×, Esc, tap-away) never seeks
	}
	if c.onClose != nil {
		c.onClose()
	}
}

// IsOpen reports whether the menu is open.
func (c *NavigationMenuComponent) IsOpen() bool { return c.open }

// Never narrower than the position readout needs.
func navMenuCardWidth() int {
	w := menuWEditor()
	if need := SpaceMD + menuRowIconSize() + SpaceSM +
		StyledTextWidth("Beat 000 · 00:00", RolePanelTitle) + SpaceMD; w < need {
		w = need
	}
	return w
}

func (c *NavigationMenuComponent) rebuild() {
	if !c.open || c.panel == nil {
		c.card = image.Rectangle{}
		c.SetBounds(image.Rectangle{})
		return
	}
	// On desktop one row, so the card fits under the ribbon and the go-to ghost stays visible.
	c.panel.SharedJogRow = !Profile().UseBottomSheet
	h := c.panel.PanelHeight()
	if Profile().UseBottomSheet {
		b := c.screenBounds
		if b.Empty() {
			b = c.anchor
		}
		ch := h
		if ch > b.Dy() {
			ch = b.Dy()
		}
		c.card = image.Rect(b.Min.X, b.Max.Y-ch, b.Max.X, b.Max.Y)
	} else {
		w := navMenuCardWidth()
		b := c.screenBounds
		if b.Empty() {
			b = image.Rect(c.anchor.Min.X, c.anchor.Min.Y, c.anchor.Min.X+w, c.anchor.Max.Y+h+touchMinTargetPx)
		}
		c.card = AnchorPopupRect(b, c.anchor, w, h, PopupBelow)
	}
	c.panel.Layout(c.card)
	// Desktop only (nil on mobile, where an outside tap closes); the panel reserves this corner.
	c.closeBtn = newPopupCloseButton(c.Close)
	if c.frame == nil {
		c.frame = NewMenuCard(nil)
	}
	c.frame.SetCloseButton(c.closeBtn)
	c.frame.Layout(c.card, c.frameSpec())
	// The title band IS the readout row; the panel keeps it for geometry only.
	c.panel.HideReadout = true
	c.SetBounds(c.card)
}

// frameSpec is the nav menu's title band: target icon + "Beat N · M:SS".
func (c *NavigationMenuComponent) frameSpec() MenuCardSpec {
	return MenuCardSpec{Title: c.panel.ReadoutText(), IconID: IconTarget, BottomSheet: Profile().UseBottomSheet}
}

// JUST the card, never unioned with the anchor: on mobile that box would span the middle of the
// screen, capture every tap there and block click-outside.
func (c *NavigationMenuComponent) InputBounds() image.Rectangle {
	if !c.open {
		return image.Rectangle{}
	}
	return c.card
}

// A press on a control returns InputCaptured, so the tree delivers drag and release natively
// (jog buttons pop back, the latch resets, a wheel drag keeps flowing).
func (c *NavigationMenuComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !c.open {
		return InputIgnored
	}
	// Ahead of the panel's controls, so it cannot be read as a jog tap.
	if pressed && c.closeBtn != nil && image.Pt(x, y).In(c.closeBtn.Rect()) {
		c.Close()
		c.capturing = false
		return InputConsumed
	}
	c.panel.HandleInput(x, y, pressed)
	if pressed {
		c.capturing = true
		return InputCaptured
	}
	c.capturing = false
	return InputConsumed
}

// Swallows every other wheel event in the card so none leaks to the grid.
func (c *NavigationMenuComponent) HandleWheel(x, y, steps int) InputResult {
	if !c.open {
		return InputIgnored
	}
	if c.panel != nil {
		c.panel.HandleWheel(x, y, steps)
	}
	return InputConsumed
}

func (c *NavigationMenuComponent) Capturing() bool { return c.capturing }

func (c *NavigationMenuComponent) Update() {}

// HoverAt forwards desktop hover to the panel's buttons (portalHoverer).
func (c *NavigationMenuComponent) HoverAt(mx, my int) {
	if c.panel == nil {
		return
	}
	for _, b := range c.panel.Buttons() {
		if b != nil {
			b.UpdateHover(mx, my)
		}
	}
	c.panel.GotoWheel().HoverAt(mx, my)
	if c.closeBtn != nil {
		c.closeBtn.UpdateHover(mx, my)
	}
}

// Drops the pending go-to preview; the universal handler then closes the menu.
func (c *NavigationMenuComponent) HandleEscape() bool {
	if c.panel != nil {
		c.panel.CancelGoto()
	}
	return false
}

// HandleEnter (portalEnterHandler): Enter is Go — seek to the pending position.
func (c *NavigationMenuComponent) HandleEnter() bool {
	if !c.open || c.panel == nil {
		return false
	}
	c.panel.Go()
	return true
}

// Nothing when closed.
func (c *NavigationMenuComponent) ClaimedKeys() []ebiten.Key {
	if !c.open || c.panel == nil {
		return nil
	}
	return c.panel.GotoWheel().ClaimedKeys()
}

// HandleClaimedKeys (KeyboardKeyClaimant) steps the go-to wheel.
func (c *NavigationMenuComponent) HandleClaimedKeys() bool {
	if !c.open || c.panel == nil {
		return false
	}
	return c.panel.GotoWheel().HandleClaimedKeys()
}

// Draw renders the card surface, the panel content, and the editor on top.
func (c *NavigationMenuComponent) Draw(dst *ebiten.Image) {
	if !c.open || c.card.Empty() {
		return
	}
	// Re-laid each frame so the live readout stays current.
	if c.frame != nil {
		c.frame.Layout(c.card, c.frameSpec())
		c.frame.Draw(dst)
	}
	// Re-layout each frame so the live ×N / Beats·Subdiv labels stay current.
	c.panel.Layout(c.card)
	c.panel.Draw(dst)
}
