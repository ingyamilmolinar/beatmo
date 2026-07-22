package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// NavigationMenuComponent is the framework-compliant host for the navigation
// menu (opened by tapping the beat counter). It is a first-class portal
// component — implementing compOverlayable — so it flows through
// compPortalOverlay + compHitHandler and the DrumView tree's NATIVE input
// dispatch: press/drag/release, capture, hover, keyboard-claim, escape, and
// click-outside are all handled by the tree, exactly like SubdivMenuComponent /
// InstrumentMenuComponent / ColorWheelComponent.
//
// It reuses the existing NavigationPanel for layout/draw/control logic, and
// OWNS the numeric go-to editor (a ParamValueEditor) so the text input is a
// child of this component (keyboard claimed while active, its rect inside
// InputBounds), rather than a floating view-owned editor drawn outside the tree.
type NavigationMenuComponent struct {
	overlayBase
	panel  *NavigationPanel
	editor *ParamValueEditor

	open         bool
	capturing    bool
	screenBounds image.Rectangle // clamp region for AnchorPopupRect
	anchor       image.Rectangle // trigger rect (beat counter) the card drops from
	card         image.Rectangle // positioned popup surface
	closeBtn     *Button         // desktop-only header × (nil on mobile; see newPopupCloseButton)
	onClose      func()
}

// NewNavigationMenuComponent wraps an existing NavigationPanel as a portal
// component and constructs the owned go-to editor.
func NewNavigationMenuComponent(panel *NavigationPanel) *NavigationMenuComponent {
	return &NavigationMenuComponent{
		overlayBase: newOverlayBase(),
		panel:       panel,
		editor:      NewParamValueEditor(),
	}
}

// Panel returns the wrapped NavigationPanel (control rects, callbacks).
func (c *NavigationMenuComponent) Panel() *NavigationPanel { return c.panel }

// CloseBtn returns the desktop-only header × (nil on mobile / before layout).
// Exposed for tests, mirroring InstrumentMenuComponent / ColorWheelComponent.
func (c *NavigationMenuComponent) CloseBtn() *Button { return c.closeBtn }

// Editor returns the owned go-to numeric editor. DrumView opens it (over the
// panel's Go-to-beat / Go-to-time rows) via Editor().OpenValue(...).
func (c *NavigationMenuComponent) Editor() *ParamValueEditor { return c.editor }

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

// SetAnchor sets the trigger rect (beat counter) the card drops from, per the
// DESIGN.md menu-placement rule.
func (c *NavigationMenuComponent) SetAnchor(r image.Rectangle) {
	if r == c.anchor {
		return
	}
	c.anchor = r
	if c.open {
		c.rebuild()
	}
}

// Open opens the menu and positions its card.
func (c *NavigationMenuComponent) Open() {
	c.open = true
	c.rebuild()
}

// Close closes the menu (and cancels an in-flight editor).
func (c *NavigationMenuComponent) Close() {
	if !c.open {
		return
	}
	c.open = false
	c.capturing = false
	if c.editor != nil && c.editor.Active() {
		c.editor.cancel()
	}
	if c.onClose != nil {
		c.onClose()
	}
}

// IsOpen reports whether the menu is open.
func (c *NavigationMenuComponent) IsOpen() bool { return c.open }

// navMenuCardWidth sizes the desktop card to fit the position readout + leading
// icon + padding, with a comfortable minimum for the jog cluster and the
// Beats/Subdiv segments on narrow windows.
func navMenuCardWidth() int {
	w := SpaceMD + menuRowIconSize() + SpaceSM +
		StyledTextWidth("Beat 000 · 00:00", RolePanelTitle) + SpaceMD
	if min := 12 * touchMinTargetPx / 2; w < min { // ~6 touch units
		w = min
	}
	return w
}

// rebuild positions the card via the shared AnchorPopupRect primitive (desktop:
// dropped below the beat counter, flipped/clamped on-screen) or as a full-width
// bottom sheet on mobile, then lays out the panel into it.
func (c *NavigationMenuComponent) rebuild() {
	if !c.open || c.panel == nil {
		c.card = image.Rectangle{}
		c.SetBounds(image.Rectangle{})
		return
	}
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
	// Desktop-only close × in the card's reserved top-right (nil on mobile, where
	// the bottom sheet closes by tapping away) — the same shared component every
	// other pop-up uses. NavigationPanel reserves this corner so it never overlaps
	// the position readout / jog cluster.
	c.closeBtn = newPopupCloseButton(c.Close)
	if c.closeBtn != nil {
		c.closeBtn.SetRect(closeButtonRect(c.card, SpaceXS))
	}
	c.SetBounds(c.card)
}

// InputBounds is the single hit area published to the tree: JUST the card (plus
// the active editor rect so a click in the editor is not an outside-click).
//
// It deliberately does NOT union the anchor (the beat counter). On mobile the
// card is a full-width bottom sheet while the anchor sits at the top of the
// pane, so a card∪anchor bounding box would span the whole middle — making taps
// there land "inside" the menu and get captured, which BLOCKS the shared portal
// click-outside (the menu would never close). A tap on the beat counter while
// the menu is open simply closes it via click-outside (the press is consumed,
// so it does not re-open) — the expected "click outside = close" behavior.
func (c *NavigationMenuComponent) InputBounds() image.Rectangle {
	if !c.open {
		return image.Rectangle{}
	}
	b := c.card
	if c.editor != nil && c.editor.Active() {
		if er := c.editor.Rect(); !er.Empty() {
			b = b.Union(er)
		}
	}
	return b
}

// HandleInput receives tree-routed pointer events via compHitHandler. A press on
// a control returns InputCaptured so the tree delivers OnDrag+OnRelease natively
// — the framework-native way to make momentary jog buttons pop back up and the
// Beats/Subdiv latch reset on the release edge (NO bespoke capture flag). While
// the go-to editor is active it owns the gesture (self-polls caret/commit).
func (c *NavigationMenuComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !c.open {
		return InputIgnored
	}
	if c.editor != nil && c.editor.Active() {
		// The editor overlays the panel and self-polls the global mouse for
		// caret placement, focus, and commit-on-outside. Capture so the tree
		// keeps the gesture here (no click-outside dismiss mid-edit).
		if pressed {
			c.capturing = true
			return InputCaptured
		}
		c.capturing = false
		return InputConsumed
	}
	// Desktop close × (nil on mobile): a press on it closes the menu outright,
	// ahead of the panel's control handling, so it can't be read as a jog tap.
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

// HandleWheel swallows wheel events (the nav menu doesn't scroll) so they never
// leak to the grid beneath.
func (c *NavigationMenuComponent) HandleWheel(x, y, steps int) InputResult {
	if !c.open {
		return InputIgnored
	}
	return InputConsumed
}

// Capturing reports whether a control gesture is in flight (drives the tree's
// capture routing).
func (c *NavigationMenuComponent) Capturing() bool { return c.capturing }

// Update is the per-frame hook (wired through compPortalOverlay.updateFn): tick
// the owned editor while it is active so its caret/keyboard poll runs.
func (c *NavigationMenuComponent) Update() {
	if c.editor != nil && c.editor.Active() {
		c.editor.Update()
	}
}

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
	if c.closeBtn != nil {
		c.closeBtn.UpdateHover(mx, my)
	}
}

// HandleEscape (portalEscapeHandler): Esc cancels an in-flight editor first,
// keeping the menu open; otherwise it lets the universal handler close the menu.
func (c *NavigationMenuComponent) HandleEscape() bool {
	if c.editor != nil && c.editor.Active() {
		c.editor.cancel()
		return true
	}
	return false
}

// ClaimsKeyboard (KeyboardClaimant): the menu owns the keyboard only while its
// go-to editor is active, so arrows/backspace reach the caret, not the camera.
func (c *NavigationMenuComponent) ClaimsKeyboard() bool {
	return c.editor != nil && c.editor.Active()
}

// Draw renders the card surface, the panel content, and the editor on top.
func (c *NavigationMenuComponent) Draw(dst *ebiten.Image) {
	if !c.open || c.card.Empty() {
		return
	}
	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, c.card)
	} else {
		drawPanel(dst, c.card)
	}
	// Re-layout each frame so the live ×N / Beats·Subdiv labels stay current.
	c.panel.Layout(c.card)
	c.panel.Draw(dst)
	if c.closeBtn != nil {
		c.closeBtn.Draw(dst)
	}
	if c.editor != nil && c.editor.Active() {
		c.editor.Draw(dst)
	}
}
