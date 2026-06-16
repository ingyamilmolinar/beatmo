package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// settingsOverlayW is the panel width in px.
const settingsOverlayW = 480

// settingsPanelHandler routes a press anywhere in the settings panel to the
// language pill it lands on. The overlay exposes ONE full-panel hit area with
// this handler rather than separate per-pill hit areas, because the portal
// flattens every overlay hit area's ZIndex to the same value (300+stackPos):
// multiple z-layered areas don't arbitrate, and a full-panel catch-all
// registered first would swallow the pill clicks. Mirrors comp_portal_overlay.go.
type settingsPanelHandler struct{ o *SettingsOverlay }

func (h settingsPanelHandler) OnPress(x, y int) InputResult {
	p := image.Pt(x, y)
	switch {
	case p.In(h.o.enRect):
		h.o.pick(i18n.LocaleEN)
	case p.In(h.o.esRect):
		h.o.pick(i18n.LocaleES)
	}
	// Consume regardless: a panel-background click must not fall through to
	// click-outside-close. The overlay closes via the gear toggle or Esc.
	return InputConsumed
}
func (settingsPanelHandler) OnDrag(x, y int)                     {}
func (settingsPanelHandler) OnRelease(x, y int)                  {}
func (settingsPanelHandler) OnWheel(x, y, steps int) InputResult { return InputConsumed }

// SettingsOverlay is the portal panel for language selection plus a localized
// keyboard-shortcuts list. onPick is invoked when the user taps a language pill.
type SettingsOverlay struct {
	onPick func(i18n.Locale)
	closed bool
	rect   image.Rectangle
	enRect image.Rectangle
	esRect image.Rectangle
}

// NewSettingsOverlay creates a settings portal overlay; onPick fires on a pill tap.
func NewSettingsOverlay(onPick func(i18n.Locale)) *SettingsOverlay {
	return &SettingsOverlay{onPick: onPick}
}

// Close marks the overlay for removal on the next portal sweep.
func (o *SettingsOverlay) Close() { o.closed = true }

// ShouldClose reports whether Close() has been invoked.
func (o *SettingsOverlay) ShouldClose() bool { return o.closed }

// Layout centers the panel on the screen and positions the two language pills.
func (o *SettingsOverlay) Layout(anchor, screenBounds image.Rectangle) {
	b := screenBounds
	w := settingsOverlayW
	// Height: header + language row + shortcuts rows (or desktop-only note on mobile).
	rows := len(localizedShortcutRows())
	if Profile().IsMobile() {
		rows = 1
	}
	h := 40 + 56 + 26*(rows+1) + 24
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	o.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	// Two language pills laid out under the language label.
	pillW, pillH := 150, 32
	px := o.rect.Min.X + 24
	py := o.rect.Min.Y + 40 + 24
	o.enRect = image.Rect(px, py, px+pillW, py+pillH)
	o.esRect = image.Rect(px+pillW+12, py, px+pillW+12+pillW, py+pillH)
}

// HitAreas exposes ONE full-panel hit area whose handler internally routes the
// press to whichever language pill it lands on (and consumes panel-background
// clicks). A single area is required because the portal flattens overlay hit
// areas to one ZIndex — see settingsPanelHandler.
func (o *SettingsOverlay) HitAreas() []HitArea {
	if o.rect.Empty() {
		return nil
	}
	return []HitArea{
		{Rect: o.rect, ZIndex: ZOverlayMin, Tag: "settings-panel",
			Handler: settingsPanelHandler{o: o}},
	}
}

func (o *SettingsOverlay) pick(l i18n.Locale) {
	if o.onPick != nil {
		o.onPick(l)
	}
}

// LanguagePillRects returns the English and Spanish language-pill rects. Valid
// after Layout. Used by the single-hit-area router and by tests.
func (o *SettingsOverlay) LanguagePillRects() (en, es image.Rectangle) {
	return o.enRect, o.esRect
}

// Draw paints the modal scrim, the panel chrome, the language pills, and the
// localized keyboard-shortcuts list (or a desktop-only note on mobile).
func (o *SettingsOverlay) Draw(screen *ebiten.Image) {
	if screen == nil || o.rect.Empty() {
		return
	}
	b := screen.Bounds()
	drawRect(screen, b, colScrim, true) // own scrim (modal feel)
	drawRoundedRect(screen, o.rect, colSurface2, RadiusSM, true)
	drawRoundedRect(screen, o.rect, colButtonBorder, RadiusSM, false)
	x := o.rect.Min.X + 24
	y := o.rect.Min.Y + 12
	DrawTextStyled(screen, i18n.T(i18n.KeySettingsTitle), x, y, RolePanelTitle, colTextPrimary)
	y += 36
	DrawTextStyled(screen, i18n.T(i18n.KeySettingsLanguage), x, y, RoleCaption, colTextSecondary)
	// Pills (active = toggled look).
	o.drawPill(screen, o.enRect, i18n.T(i18n.KeyLangEnglish), i18n.ActiveLocale() == i18n.LocaleEN)
	o.drawPill(screen, o.esRect, i18n.T(i18n.KeyLangSpanish), i18n.ActiveLocale() == i18n.LocaleES)
	// Shortcuts section.
	y = o.esRect.Max.Y + 18
	DrawTextStyled(screen, i18n.T(i18n.KeySettingsShortcuts), x, y, RoleSectionHeader, colTextPrimary)
	y += 28
	if Profile().IsMobile() {
		DrawTextStyled(screen, i18n.T(i18n.KeySettingsDesktopOnly), x, y, RoleCaption, colTextSecondary)
		return
	}
	for _, row := range localizedShortcutRows() {
		DrawTextStyled(screen, row.keys, x, y, RoleCaption, colTextPrimary)
		DrawTextStyled(screen, i18n.T(row.actionKey), x+120, y, RoleCaption, colTextSecondary)
		y += 26
	}
}

func (o *SettingsOverlay) drawPill(screen *ebiten.Image, r image.Rectangle, label string, active bool) {
	var fill color.Color = colSurface2
	if active {
		fill = colButtonBorder
	}
	drawRoundedRect(screen, r, fill, RadiusSM, true)
	drawRoundedRect(screen, r, colButtonBorder, RadiusSM, false)
	tx := r.Min.X + 12
	ty := r.Min.Y + (r.Dy()-StyledTextHeight(RoleBody))/2
	DrawTextStyled(screen, label, tx, ty, RoleBody, colTextPrimary)
}
