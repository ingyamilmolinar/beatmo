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
// It reuses NavigationPanel for layout/draw/control logic, including the GO
// TO wheel. While open it is a PARTIAL keyboard claimant (the wheel's arrows:
// Up/Down value, Left/Right step) and Enter means Go; every other key passes
// through, so Space still plays.
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

// NewNavigationMenuComponent wraps an existing NavigationPanel as a portal
// component.
func NewNavigationMenuComponent(panel *NavigationPanel) *NavigationMenuComponent {
	return &NavigationMenuComponent{
		overlayBase: newOverlayBase(),
		panel:       panel,
	}
}

// Panel returns the wrapped NavigationPanel (control rects, callbacks).
func (c *NavigationMenuComponent) Panel() *NavigationPanel { return c.panel }

// CloseBtn returns the desktop-only header × (nil on mobile / before layout).
// Exposed for tests, mirroring InstrumentMenuComponent / ColorWheelComponent.
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

// Open opens the menu, seats the go-to wheel on the playhead and positions the
// card.
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

// navMenuCardWidth is the desktop card width: the editor-card width class
// (DESIGN.md "Menu sizing & layout" — a card hosting controls), never narrower
// than the position readout needs.
func navMenuCardWidth() int {
	w := menuWEditor()
	if need := SpaceMD + menuRowIconSize() + SpaceSM +
		StyledTextWidth("Beat 000 · 00:00", RolePanelTitle) + SpaceMD; w < need {
		w = need
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
	// Desktop: the jog cluster and the unit toggle share a row so the card fits
	// under the timeline ribbon (keeps the go-to ghost visible). The mobile
	// sheet keeps them on two rows.
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
	// Desktop-only close × in the card's reserved top-right (nil on mobile, where
	// the bottom sheet closes by tapping away) — the same shared component every
	// other pop-up uses. NavigationPanel reserves this corner so it never overlaps
	// the position readout / jog cluster.
	c.closeBtn = newPopupCloseButton(c.Close)
	if c.frame == nil {
		c.frame = NewMenuCard(nil)
	}
	c.frame.SetCloseButton(c.closeBtn)
	c.frame.Layout(c.card, c.frameSpec())
	// The card's title band IS the position readout row; the panel keeps the
	// row for geometry but no longer paints it (HideReadout).
	c.panel.HideReadout = true
	c.SetBounds(c.card)
}

// frameSpec is the nav menu's title band: target icon + "Beat N · M:SS".
func (c *NavigationMenuComponent) frameSpec() MenuCardSpec {
	return MenuCardSpec{Title: c.panel.ReadoutText(), IconID: IconTarget, BottomSheet: Profile().UseBottomSheet}
}

// InputBounds is the single hit area published to the tree: JUST the card.
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
	return c.card
}

// HandleInput receives tree-routed pointer events via compHitHandler. A press on
// a control returns InputCaptured so the tree delivers OnDrag+OnRelease natively
// — the framework-native way to make momentary jog buttons pop back up, the
// Beats/Subdiv latch reset on the release edge, and a wheel drag keep flowing to
// the wheel (NO bespoke capture flag).
func (c *NavigationMenuComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !c.open {
		return InputIgnored
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

// HandleWheel scrolls the go-to wheel when the pointer is over it, and swallows
// every other wheel event inside the card so none leaks to the grid beneath.
func (c *NavigationMenuComponent) HandleWheel(x, y, steps int) InputResult {
	if !c.open {
		return InputIgnored
	}
	if c.panel != nil {
		c.panel.HandleWheel(x, y, steps)
	}
	return InputConsumed
}

// Capturing reports whether a control gesture is in flight (drives the tree's
// capture routing).
func (c *NavigationMenuComponent) Capturing() bool { return c.capturing }

// Update is the per-frame hook (wired through compPortalOverlay.updateFn). The
// go-to wheel has no per-frame animation, so there is nothing to tick.
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

// HandleEscape (portalEscapeHandler): Esc drops the pending go-to preview and
// lets the universal handler close the menu (Close also cancels).
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

// ClaimedKeys (KeyboardKeyClaimant): the go-to wheel's arrows while open —
// Up/Down step the value, Left/Right the step size. Nothing when closed.
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
	// Shared MenuCard frame (panel / bottom sheet, readout title band, close ×);
	// re-laid each frame so the live readout stays current.
	if c.frame != nil {
		c.frame.Layout(c.card, c.frameSpec())
		c.frame.Draw(dst)
	}
	// Re-layout each frame so the live ×N / Beats·Subdiv labels stay current.
	c.panel.Layout(c.card)
	c.panel.Draw(dst)
}
