package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// The ONE frame every floating menu shares, composed only of existing primitives. The swatch picker
// is the reference: its bands are the card's. The card knows only rects and never reads Game state, so
// owners keep their own body and input.
type MenuCardSpec struct {
	Title        string      // already-localized title (instrument name, i18n.T(key), …)
	Swatch       color.Color // leading rounded chip for identity menus (instrument / group); nil → none
	IconID       IconID      // leading icon for global menus; "" → none (Swatch wins when both set)
	Accent       color.Color // hairline + owner accent; nil → colAccent
	Caption      string      // optional left caption in the hint band (RoleMicro, muted); "" → no band
	CaptionRight string      // optional right caption in the same band
	Footer       []*Button   // optional keycap actions, laid out right-to-left (last = rightmost)
	Actions      []*Button   // optional header actions in the title band, right-to-left before the close × (Logs "Clear history", group "Save")
	BottomSheet  bool        // mobile bottom sheet: sheet chrome + drag handle instead of the floating panel
}

// Card band constants — shared with the swatch picker by construction.
const (
	menuCardTitleH   = colorSwatchHeader // title band height (title + close ×)
	menuCardPad      = colorSwatchPad    // panel inner padding around the body
	menuCardCaptionH = colorShadeHintH   // caption band height (top of the body)
	menuCardFooterH  = 32                // footer keycap height
)

// MenuCard holds the solved frame rects for one panel size.
type MenuCard struct {
	spec     MenuCardSpec
	panel    image.Rectangle
	title    image.Rectangle // full-width band at the top
	caption  image.Rectangle // empty when spec.Caption == ""
	body     image.Rectangle // content area (below the caption, above the footer)
	footer   image.Rectangle // empty when no footer buttons
	closeBtn *Button         // nil on mobile (desktopCloseVisible)
}

// NewMenuCard builds a card whose close × invokes onClose (desktop only).
func NewMenuCard(onClose func()) *MenuCard {
	return &MenuCard{closeBtn: newPopupCloseButton(onClose)}
}

// For menus whose input already tracks their × by identity; nil hides it.
func (c *MenuCard) SetCloseButton(b *Button) { c.closeBtn = b }

// Layout solves the bands for panel. Call every Layout pass (cheap; rects only).
func (c *MenuCard) Layout(panel image.Rectangle, spec MenuCardSpec) {
	c.spec = spec
	c.panel = panel
	if panel.Empty() {
		c.title, c.caption, c.body, c.footer = image.Rectangle{}, image.Rectangle{}, image.Rectangle{}, image.Rectangle{}
		return
	}
	c.title = image.Rect(panel.Min.X, panel.Min.Y, panel.Max.X, panel.Min.Y+menuCardTitleH)
	if c.closeBtn != nil {
		c.closeBtn.SetRect(closeButtonRect(c.title, SpaceXS))
	}
	// Header actions: right-to-left from the close × (or the band's edge).
	ax := c.title.Max.X - menuCardPad
	if c.closeBtn != nil {
		ax = c.closeBtn.Rect().Min.X - SpaceSM
	}
	ah := menuCardTitleH - 2*SpaceXS
	ay := c.title.Min.Y + (menuCardTitleH-ah)/2
	for i := len(spec.Actions) - 1; i >= 0; i-- {
		b := spec.Actions[i]
		w := StyledTextWidth(b.displayText(), RoleBody) + 2*SpaceMD
		if b.Icon != "" && b.displayText() == "" {
			w = ah
		}
		b.SetRect(image.Rect(ax-w, ay, ax, ay+ah))
		ax -= w + SpaceSM
	}
	body := image.Rect(panel.Min.X+menuCardPad, c.title.Max.Y+menuCardPad, panel.Max.X-menuCardPad, panel.Max.Y-menuCardPad)
	if spec.Caption != "" || spec.CaptionRight != "" {
		c.caption = image.Rect(body.Min.X, body.Min.Y, body.Max.X, body.Min.Y+menuCardCaptionH)
		body.Min.Y = c.caption.Max.Y
	} else {
		c.caption = image.Rectangle{}
	}
	if len(spec.Footer) > 0 {
		c.footer = image.Rect(body.Min.X, body.Max.Y-menuCardFooterH, body.Max.X, body.Max.Y)
		body.Max.Y = c.footer.Min.Y - SpaceSM
		x := c.footer.Max.X
		for i := len(spec.Footer) - 1; i >= 0; i-- {
			b := spec.Footer[i]
			w := StyledTextWidth(b.displayText(), RoleBody) + 2*SpaceMD
			if b.Icon != "" && b.displayText() == "" {
				w = menuCardFooterH
			}
			b.SetRect(image.Rect(x-w, c.footer.Min.Y, x, c.footer.Max.Y))
			x -= w + SpaceSM
		}
	} else {
		c.footer = image.Rectangle{}
	}
	if body.Max.Y < body.Min.Y {
		body.Max.Y = body.Min.Y
	}
	c.body = body
}

// Owners draw their body afterwards.
func (c *MenuCard) Draw(dst *ebiten.Image) {
	if dst == nil || c.panel.Empty() {
		return
	}
	if c.spec.BottomSheet {
		drawBottomSheetPanel(dst, c.panel)
		drawSheetDragHandle(dst, c.panel)
	} else {
		drawPanel(dst, c.panel)
	}
	c.DrawBand(dst)
}

// For docked panels that own their surface.
func (c *MenuCard) DrawBand(dst *ebiten.Image) {
	if dst == nil || c.panel.Empty() {
		return
	}
	accent := c.spec.Accent
	if accent == nil {
		accent = colAccent
	}
	drawMenuHeaderBandAccent(dst, c.title, accent)
	titleRect := image.Rect(c.title.Min.X+menuCardPad, c.title.Min.Y, c.titleTextRight(), c.title.Max.Y)
	// drawMenuTitle does not clip, and a long name must never run under the actions.
	avail := titleRect.Dx()
	if c.spec.Swatch != nil || c.spec.IconID != "" {
		avail -= menuTitleSwatchSz + SpaceSM
	}
	title := clipTextToWidthMeasure(c.spec.Title, avail, func(s string) int { return StyledTextWidth(s, menuTitleRole) })
	switch {
	case c.spec.Swatch != nil:
		drawMenuTitle(dst, titleRect, title, c.spec.Swatch)
	case c.spec.IconID != "":
		ir := menuTitleSwatchRect(titleRect)
		DrawIcon(dst, c.spec.IconID, ir, colTextSecondary)
		drawMenuTitle(dst, image.Rect(ir.Max.X+SpaceSM, titleRect.Min.Y, titleRect.Max.X, titleRect.Max.Y), title, nil)
	default:
		drawMenuTitle(dst, titleRect, title, nil)
	}
	if !c.caption.Empty() {
		hh := StyledTextHeight(RoleMicro)
		hy := c.caption.Min.Y + (c.caption.Dy()-hh)/2
		if c.spec.Caption != "" {
			DrawTextStyled(dst, c.spec.Caption, c.caption.Min.X, hy, RoleMicro, colTextSecondary)
		}
		if c.spec.CaptionRight != "" {
			rw := StyledTextWidth(c.spec.CaptionRight, RoleMicro)
			DrawTextStyled(dst, c.spec.CaptionRight, c.caption.Max.X-rw, hy, RoleMicro, colTextSecondary)
		}
	}
	for _, b := range c.spec.Footer {
		b.Draw(dst)
	}
	for _, b := range c.spec.Actions {
		b.Draw(dst)
	}
	if c.closeBtn != nil {
		c.closeBtn.Draw(dst)
	}
}

func (c *MenuCard) titleTextRight() int {
	if len(c.spec.Actions) > 0 {
		return c.spec.Actions[0].Rect().Min.X - SpaceSM
	}
	if c.closeBtn != nil {
		return c.closeBtn.Rect().Min.X - SpaceSM
	}
	return c.title.Max.X - menuCardPad
}

// Frame controls FIRST (they win the equal-z stable sort), the catch-all LAST; owners put body
// areas between via ControlAreas/CatchAll.
func (c *MenuCard) HitAreas(z int, tag string, catchAll HitHandler) []HitArea {
	areas := c.ControlAreas(z, tag)
	return append(areas, c.CatchAll(z, tag, catchAll))
}

// ControlAreas is the buttons-only part of HitAreas.
func (c *MenuCard) ControlAreas(z int, tag string) []HitArea {
	var areas []HitArea
	for i, b := range c.spec.Footer {
		areas = append(areas, HitArea{Rect: b.Rect(), ZIndex: z, Tag: tag + "-footer-" + itoaMenuNav(i), Handler: &buttonHitAdapter{btn: b}})
	}
	for i, b := range c.spec.Actions {
		areas = append(areas, HitArea{Rect: b.Rect(), ZIndex: z, Tag: tag + "-action-" + itoaMenuNav(i), Handler: &buttonHitAdapter{btn: b}})
	}
	if c.closeBtn != nil {
		areas = append(areas, HitArea{Rect: c.closeBtn.Rect(), ZIndex: z, Tag: tag + "-close", Handler: &buttonHitAdapter{btn: c.closeBtn}})
	}
	return areas
}

// CatchAll is the full-panel area that swallows presses on panel whitespace.
func (c *MenuCard) CatchAll(z int, tag string, h HitHandler) HitArea {
	return HitArea{Rect: c.panel, ZIndex: z, Tag: tag + "-panel", Handler: h}
}

// InputIgnored when no frame control took the press.
func (c *MenuCard) HandleInput(x, y int, pressed bool) InputResult {
	if c.closeBtn != nil {
		if r := c.closeBtn.HandleInputResult(x, y, pressed); r != InputIgnored {
			return r
		}
	}
	for _, b := range c.spec.Footer {
		if r := b.HandleInputResult(x, y, pressed); r != InputIgnored {
			return r
		}
	}
	for _, b := range c.spec.Actions {
		if r := b.HandleInputResult(x, y, pressed); r != InputIgnored {
			return r
		}
	}
	return InputIgnored
}

// HoverAt refreshes hover on the frame's buttons (desktop hover glow).
func (c *MenuCard) HoverAt(mx, my int) {
	if c.closeBtn != nil {
		c.closeBtn.UpdateHover(mx, my)
	}
	for _, b := range c.spec.Footer {
		b.UpdateHover(mx, my)
	}
	for _, b := range c.spec.Actions {
		b.UpdateHover(mx, my)
	}
}

func (c *MenuCard) Panel() image.Rectangle      { return c.panel }
func (c *MenuCard) TitleRect() image.Rectangle  { return c.title }
func (c *MenuCard) Caption() image.Rectangle    { return c.caption }
func (c *MenuCard) Body() image.Rectangle       { return c.body }
func (c *MenuCard) FooterRect() image.Rectangle { return c.footer }
func (c *MenuCard) CloseBtn() *Button           { return c.closeBtn }

// One entry builder so open/close semantics cannot drift. mustAnswer (modal) for dialogs, value
// popups, and a picker stacked on an open menu, where only the modal filter stops a tap reaching the menu beneath.
func menuCardEntry(id string, ov PortalOverlay, anchor image.Rectangle, mustAnswer bool, onClose func()) PortalEntry {
	return PortalEntry{ID: id, Overlay: ov, Modal: mustAnswer, Scrim: true, Anchor: anchor, OnClose: onClose}
}
