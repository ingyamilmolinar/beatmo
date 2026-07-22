package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// confirmConfig declares a confirmation dialog. onConfirm/onCancel are the
// pluggable domain effects (the dialog itself never touches the graph or the
// mode manager); the caller owns whatever mode state sits behind the dialog.
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

// ConfirmDialog is the single shared modal confirmation, rendered as a
// PortalOverlay in the settings-overlay shape: real *Buttons with keycap chrome,
// per-button hit areas + a panel catch-all (NO full-screen backdrop, so a press
// outside the panel misses every area and the tree's CloseTop route fires
// OnClose → onCancel — the shared click-outside contract). Implements
// portalEnterHandler (Enter → confirm) and portalEscapeHandler (Esc → cancel).
type ConfirmDialog struct {
	cfg        confirmConfig
	confirmBtn *Button
	cancelBtn  *Button
	closeBtn   *Button
	rect       image.Rectangle
	closed     bool
}

// newConfirmDialog constructs a dialog from cfg. Buttons are created once here
// and SetRect'd on every Layout call.
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
	// Close × is desktop-only (shared desktopCloseVisible gate). On mobile the
	// dialog dismisses via the Cancel button or a tap outside the panel
	// (CloseTop → OnClose → cancel), so no × is drawn. Kept as the dialog-native
	// spec button to match the Cancel/Confirm chrome; only the platform gate is
	// shared. Every closeBtn use site below guards `d.closeBtn != nil`.
	if desktopCloseVisible() {
		d.closeBtn = NewSpecButton("", ComponentButtonSecondary, d.cancel)
		d.closeBtn.Icon = string(IconClose)
		d.closeBtn.IconColor = closeIconColor()
	}
	return d
}

// confirm/cancel fire the pluggable callback exactly once, then flag closed so
// the portal sweep (ShouldClose) removes the entry. Idempotent.
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

// Layout centers the panel over anchor when non-empty (the "future anchored
// variant" portalEntry's doc comment alludes to — a caller wants the dialog
// centered over a specific pane, e.g. the grid content pane, rather than the
// portal's full screenBounds); otherwise it falls back to centering over
// screenBounds, matching the original screen-centered behavior. Token-driven
// sizing: title width + both buttons + padding.
func (d *ConfirmDialog) Layout(anchor, screenBounds image.Rectangle) {
	title := i18n.T(d.cfg.titleKey)
	titleW := StyledTextWidth(title, RoleSectionHeader)
	btnH := BtnHeightMD
	btnW := StyledTextWidth(i18n.T(d.cfg.confirmKey), RoleBody) + 2*SpaceMD
	if cw := StyledTextWidth(i18n.T(i18n.KeyCancel), RoleBody) + 2*SpaceMD; cw > btnW {
		btnW = cw
	}
	// On mobile there is no close × (see newConfirmDialog), so the title band
	// reclaims that reserved space and can run to the panel's right padding.
	closeSz := 0
	if d.closeBtn != nil {
		closeSz = Profile().CloseButtonSize
	}
	// One extra SpaceMD of breathing room beyond the exact-fit term below (see
	// titleAvailWidth) so the natural title measures strictly less than the
	// available header width and never trips clipTextToWidthMeasure's
	// ellipsis — an exact fit (the pre-fix formula) leaves zero slack and a
	// 1px integer-division rounding loss (cx-w/2 floors on odd w) was enough
	// to truncate "Break the circuit?" to "Break the circu...".
	titleTerm := titleW + 3*SpaceMD + closeSz + SpaceXS
	if d.cfg.swatch != nil {
		titleTerm += menuTitleSwatchSz + SpaceSM // mirror titleAvailWidth's swatch deduction
	}
	w := titleTerm
	if minW := 2*btnW + SpaceMD + 2*SpaceMD; w < minW {
		w = minW
	}
	h := SpaceMD + StyledTextHeight(RoleSectionHeader) + SpaceLG + btnH + SpaceMD
	b := screenBounds
	if !anchor.Empty() {
		b = anchor
	}
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	d.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	// Buttons: Cancel left, Confirm right, bottom row.
	by := d.rect.Max.Y - SpaceMD - btnH
	d.cancelBtn.SetRect(image.Rect(d.rect.Min.X+SpaceMD, by, d.rect.Min.X+SpaceMD+btnW, by+btnH))
	d.confirmBtn.SetRect(image.Rect(d.rect.Max.X-SpaceMD-btnW, by, d.rect.Max.X-SpaceMD, by+btnH))
	if d.closeBtn != nil {
		d.closeBtn.SetRect(closeButtonRect(d.rect, SpaceXS))
	}
}

// HitAreas publishes one hit area per interactive control (confirm, cancel,
// close) plus a full-panel catch-all registered LAST so the button areas win
// the portal's equal-z stable-sort on overlap. Deliberately NO full-screen
// backdrop: a press outside the panel rect misses every area here, so the
// portal's outside-click handling (CloseTop → OnClose) drives the cancel path.
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
	// Panel catch-all LAST: swallows in-panel taps that miss a button so
	// they never fall through, but covers ONLY the panel rect — outside
	// taps still yield len(hits)==0 → tree CloseTop → OnClose → cancel.
	areas = append(areas, HitArea{Rect: d.rect, ZIndex: ZOverlayMin, Tag: d.cfg.id + "-panel",
		Handler: confirmPanelHandler{}})
	return areas
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
	if screen == nil || d.rect.Empty() {
		return
	}
	drawScrim(screen)
	drawPanel(screen, d.rect)
	headerMaxX := d.rect.Max.X - SpaceMD
	if d.closeBtn != nil { // reserve the close-× lane only when it exists (desktop)
		if closeMinX := closeButtonRect(d.rect, SpaceXS).Min.X - SpaceMD; closeMinX < headerMaxX {
			headerMaxX = closeMinX
		}
	}
	headerRect := image.Rect(d.rect.Min.X+SpaceMD, d.rect.Min.Y+SpaceXS, headerMaxX, d.rect.Min.Y+SpaceXS+StyledTextHeight(RoleSectionHeader))
	// drawMenuTitle does not clip its label itself (see drawFXPanelHeader's
	// header-band precedent), so pre-clip here to guarantee the title never
	// runs under the close-X even at extreme widths/short panels.
	maxTitleW := headerRect.Dx()
	if d.cfg.swatch != nil {
		maxTitleW -= menuTitleSwatchSz + SpaceSM // the leading swatch's share of the band
	}
	title := clipTextToWidthMeasure(i18n.T(d.cfg.titleKey), maxTitleW,
		func(s string) int { return StyledTextWidth(s, RoleSectionHeader) })
	drawMenuTitle(screen, headerRect, title, d.cfg.swatch)
	d.cancelBtn.Draw(screen)
	d.confirmBtn.Draw(screen)
	if d.closeBtn != nil {
		d.closeBtn.Draw(screen)
	}
}

// ShouldClose reports whether confirm() or cancel() has fired.
func (d *ConfirmDialog) ShouldClose() bool { return d.closed }

// HandleEnter (portalEnterHandler): Enter accepts.
func (d *ConfirmDialog) HandleEnter() bool { d.confirm(); return true }

// HandleEscape (portalEscapeHandler): Esc cancels; return false so the
// universal handler's CloseTop removes the (still-topmost) entry — the
// proven mid-stack contract (see grid_move_break_confirm.go history).
func (d *ConfirmDialog) HandleEscape() bool { d.cancel(); return false }

// portalEntry builds the PortalEntry for this dialog. centerIn returns the
// pane rect the caller wants the dialog centered over (e.g. the grid pane vs
// the full screen) — Anchor is set from it and Layout centers on it directly
// when non-empty (see Layout), so the dialog centers correctly even before
// the portal's own screenBounds have been populated by a frame's Update()/
// Draw() pass. Pass a nil centerIn (or one returning an empty rect) to fall
// back to the portal's screenBounds (full-screen centering).
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

// confirmPanelHandler swallows taps on the panel background (missing both
// buttons) so they don't fall through to a lower surface.
type confirmPanelHandler struct{}

func (confirmPanelHandler) OnPress(x, y int) InputResult        { return InputConsumed }
func (confirmPanelHandler) OnDrag(x, y int)                     {}
func (confirmPanelHandler) OnRelease(x, y int)                  {}
func (confirmPanelHandler) OnWheel(x, y, steps int) InputResult { return InputConsumed }
