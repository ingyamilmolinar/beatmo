package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The dialog never touches the graph or the mode manager; the caller owns the state behind it.
type confirmConfig struct {
	id          string
	titleKey    i18n.Key
	confirmKey  i18n.Key
	cancelKey   i18n.Key // custom cancel/second-choice label; "" → KeyCancel
	destructive bool
	swatch      color.Color // optional instrument accent under the title; nil = none
	onConfirm   func()
	onCancel    func()
}

// NO full-screen backdrop: a press outside the panel misses every area, so the tree's CloseTop fires OnClose →
// onCancel, the shared click-outside contract.
type ConfirmDialog struct {
	cfg        confirmConfig
	confirmBtn *Button
	cancelBtn  *Button
	closeBtn   *Button
	card       *MenuCard // shared frame: panel, title band, footer keycaps, close ×
	rect       image.Rectangle
	closed     bool
}

func newConfirmDialog(cfg confirmConfig) *ConfirmDialog {
	d := &ConfirmDialog{cfg: cfg}
	confirmSpec := ComponentButtonPrimary
	if cfg.destructive {
		confirmSpec = ComponentButtonDestructive
	}
	cancelKey := cfg.cancelKey
	if cancelKey == "" {
		cancelKey = i18n.KeyCancel
	}
	d.confirmBtn = NewButtonKey(cfg.confirmKey, confirmSpec, d.confirm)
	d.cancelBtn = NewButtonKey(cancelKey, ComponentButtonSecondary, d.cancel)
	d.confirmBtn.SetTextRole(RoleBody)
	d.cancelBtn.SetTextRole(RoleBody)
	// Desktop only: on mobile Cancel or a tap outside dismisses it. Every closeBtn use guards nil.
	if desktopCloseVisible() {
		d.closeBtn = NewSpecButton("", ComponentButtonSecondary, d.cancel)
		d.closeBtn.Icon = string(IconClose)
		d.closeBtn.IconColor = closeIconColor()
	}
	return d
}

// Fires exactly once, then flags closed so the portal sweep removes it.
func (d *ConfirmDialog) confirm() {
	if d.closed {
		return
	}
	d.closed = true
	if d.cfg.onConfirm != nil {
		d.cfg.onConfirm()
	}
}

func (d *ConfirmDialog) cancel() {
	if d.closed {
		return
	}
	d.closed = true
	if d.cfg.onCancel != nil {
		d.cfg.onCancel()
	}
}

// Centred over anchor when given (a caller wanting a specific pane), else over screenBounds.
func (d *ConfirmDialog) Layout(anchor, screenBounds image.Rectangle) {
	title := i18n.T(d.cfg.titleKey)
	titleW := StyledTextWidth(title, RoleSectionHeader)
	btnH := BtnHeightMD
	btnW := StyledTextWidth(i18n.T(d.cfg.confirmKey), RoleBody) + 2*SpaceMD
	if cw := StyledTextWidth(i18n.T(i18n.KeyCancel), RoleBody) + 2*SpaceMD; cw > btnW {
		btnW = cw
	}
	// No × on mobile, so the title band reclaims its space.
	closeSz := 0
	if d.closeBtn != nil {
		closeSz = Profile().CloseButtonSize
	}
	// One extra SpaceMD so the title fits with slack: an exact fit lost 1 px to integer rounding and truncated
	// "Break the circuit?".
	titleTerm := titleW + 3*SpaceMD + closeSz + SpaceXS
	if d.cfg.swatch != nil {
		titleTerm += menuTitleSwatchSz + SpaceSM // mirror titleAvailWidth's swatch deduction
	}
	w := titleTerm
	if minW := 2*btnW + SpaceMD + 2*SpaceMD; w < minW {
		w = minW
	}
	h := menuCardTitleH + menuCardPad + SpaceSM + menuCardFooterH + menuCardPad
	b := screenBounds
	if !anchor.Empty() {
		b = anchor
	}
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	d.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	_ = btnH
	if d.card == nil {
		d.card = NewMenuCard(nil)
	}
	d.card.SetCloseButton(d.closeBtn)
	d.card.Layout(d.rect, MenuCardSpec{Title: title, Swatch: d.cfg.swatch, Footer: []*Button{d.cancelBtn, d.confirmBtn}})
}

// The catch-all goes LAST so buttons win the equal-z stable sort; no backdrop, so an outside press reaches
// the portal's CloseTop → cancel.
func (d *ConfirmDialog) HitAreas() []HitArea {
	if d.rect.Empty() {
		return nil
	}
	areas := []HitArea{
		{Rect: d.confirmBtn.Rect(), ZIndex: ZOverlayMin, Tag: d.cfg.id + "-confirm",
			Handler: &buttonHitAdapter{btn: d.confirmBtn}},
		{Rect: d.cancelBtn.Rect(), ZIndex: ZOverlayMin, Tag: d.cfg.id + "-cancel",
			Handler: &buttonHitAdapter{btn: d.cancelBtn}},
	}
	if d.closeBtn != nil { // desktop only
		areas = append(areas, HitArea{Rect: d.closeBtn.Rect(), ZIndex: ZOverlayMin, Tag: d.cfg.id + "-close",
			Handler: &buttonHitAdapter{btn: d.closeBtn}})
	}
	// Covers ONLY the panel, so outside taps still reach CloseTop.
	return append(areas, d.card.CatchAll(ZOverlayMin, d.cfg.id, confirmPanelHandler{}))
}

// HoverAt refreshes desktop hover for the three buttons (portalHoverer).
func (d *ConfirmDialog) HoverAt(mx, my int) {
	d.confirmBtn.UpdateHover(mx, my)
	d.cancelBtn.UpdateHover(mx, my)
	if d.closeBtn != nil {
		d.closeBtn.UpdateHover(mx, my)
	}
}

func (d *ConfirmDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.rect.Empty() || d.card == nil {
		return
	}
	drawScrim(screen)
	d.card.Draw(screen) // panel, clipped title (+ swatch), footer keycaps, close ×
}

// ShouldClose reports whether confirm() or cancel() has fired.
func (d *ConfirmDialog) ShouldClose() bool { return d.closed }

// HandleEnter (portalEnterHandler): Enter accepts.
func (d *ConfirmDialog) HandleEnter() bool { d.confirm(); return true }

// Returns false so the universal handler's CloseTop removes the still-topmost entry.
func (d *ConfirmDialog) HandleEscape() bool { d.cancel(); return false }

// centerIn lets Layout centre on the pane before the portal's screenBounds exist; nil or empty falls back to
// full-screen centring.
func (d *ConfirmDialog) portalEntry(centerIn func() image.Rectangle) PortalEntry {
	anchor := image.Rectangle{}
	if centerIn != nil {
		anchor = centerIn()
	}
	return PortalEntry{
		ID:      d.cfg.id,
		Overlay: d,
		Modal:   true,
		Scrim:   false, // dialog paints its own scrim in Draw (modal feel)
		Anchor:  anchor,
		OnClose: d.cancel,
	}
}

// So a tap on the panel background never falls through.
type confirmPanelHandler struct{}

func (confirmPanelHandler) OnPress(x, y int) InputResult        { return InputConsumed }
func (confirmPanelHandler) OnDrag(x, y int)                     {}
func (confirmPanelHandler) OnRelease(x, y int)                  {}
func (confirmPanelHandler) OnWheel(x, y, steps int) InputResult { return InputConsumed }
