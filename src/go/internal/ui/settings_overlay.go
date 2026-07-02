package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// settingsOverlayW is the panel width in px.
const settingsOverlayW = 480

// settingsPanelHandler is the full-panel catch-all: it consumes any press that
// lands on the panel background (not on a pill or the close button) so a
// panel-background click never falls through. The pills and close button carry
// their own per-control hit areas (shared buttonHitAdapter) registered BEFORE
// this catch-all, so they win the equal-z stable-sort tiebreak on overlap. The
// overlay is opened modal, so presses outside the panel are filtered out by the
// HitIndex before they ever reach a handler.
type settingsPanelHandler struct{}

func (settingsPanelHandler) OnPress(x, y int) InputResult { return InputConsumed }
func (settingsPanelHandler) OnDrag(x, y int)              {}
func (settingsPanelHandler) OnRelease(x, y int)           {}
func (settingsPanelHandler) OnWheel(x, y, steps int) InputResult { return InputConsumed }

// SettingsOverlay is the portal panel for language selection plus a localized
// keyboard-shortcuts list. onPick is invoked when the user taps a language pill.
type SettingsOverlay struct {
	onPick func(i18n.Locale)
	closed bool
	rect   image.Rectangle
	enRect image.Rectangle
	esRect image.Rectangle

	// Real Buttons so the pills + close affordance get the shared keycap chrome
	// and press/hover animation as every other control (DESIGN.md). The active
	// language pill is rendered toggled (latched amber). Created once in the
	// constructor; SetRect'd each Layout.
	enBtn    *Button
	esBtn    *Button
	closeBtn *Button
}

// NewSettingsOverlay creates a settings portal overlay; onPick fires on a pill tap.
func NewSettingsOverlay(onPick func(i18n.Locale)) *SettingsOverlay {
	o := &SettingsOverlay{onPick: onPick}
	o.enBtn = NewButtonKey(i18n.KeyLangEnglish, ComponentButtonSecondary, func() { o.pick(i18n.LocaleEN) })
	o.esBtn = NewButtonKey(i18n.KeyLangSpanish, ComponentButtonSecondary, func() { o.pick(i18n.LocaleES) })
	o.closeBtn = NewSpecButton("", ComponentButtonSecondary, func() { o.Close() })
	o.closeBtn.Icon = "close"
	o.closeBtn.IconColor = closeIconColor()
	return o
}

// Close marks the overlay for removal on the next portal sweep.
func (o *SettingsOverlay) Close() { o.closed = true }

// ShouldClose reports whether Close() has been invoked.
func (o *SettingsOverlay) ShouldClose() bool { return o.closed }

// Layout centers the panel on the screen and positions the two language pills
// and the close button.
func (o *SettingsOverlay) Layout(anchor, screenBounds image.Rectangle) {
	b := screenBounds
	w := settingsOverlayW
	// Clamp to the available width with a side margin so the panel (and its
	// content, inset 24px) stays fully on-screen on narrow mobile panes — the
	// fixed 480px width otherwise centers off both edges of a ~390px phone.
	if maxW := b.Dx() - 2*24; maxW > 0 && w > maxW {
		w = maxW
	}
	// Height: header + language row, plus the shortcuts section on desktop only
	// (mobile drops it entirely, so the panel is correspondingly shorter).
	h := 40 + 56 + 24
	if o.shortcutsVisible() {
		rows := len(localizedShortcutRows())
		h += 26 * (rows + 1)
	}
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	o.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	// Two language pills laid out under the language label.
	pillW, pillH := 150, 32
	px := o.rect.Min.X + 24
	py := o.rect.Min.Y + 40 + 24
	o.enRect = image.Rect(px, py, px+pillW, py+pillH)
	o.esRect = image.Rect(px+pillW+12, py, px+pillW+12+pillW, py+pillH)
	o.enBtn.SetRect(o.enRect)
	o.esBtn.SetRect(o.esRect)
	// Close button in the panel's top-right corner, sized via the shared helper
	// so it matches every other pop-up.
	o.closeBtn.SetRect(closeButtonRect(o.rect, SpaceXS))
}

// HitAreas publishes one hit area per interactive control (the two language
// pills and the close button) plus a full-panel catch-all registered LAST so
// the pill/close areas win the portal's equal-z stable-sort on overlap. Each
// control routes through the shared buttonHitAdapter so it shares the canonical
// press lifecycle (edge fire, press/hover animation, release settle).
func (o *SettingsOverlay) HitAreas() []HitArea {
	if o.rect.Empty() {
		return nil
	}
	return []HitArea{
		{Rect: o.enBtn.Rect(), ZIndex: ZOverlayMin, Tag: "settings-lang-en",
			Handler: &buttonHitAdapter{btn: o.enBtn}},
		{Rect: o.esBtn.Rect(), ZIndex: ZOverlayMin, Tag: "settings-lang-es",
			Handler: &buttonHitAdapter{btn: o.esBtn}},
		{Rect: o.closeBtn.Rect(), ZIndex: ZOverlayMin, Tag: "settings-close",
			Handler: &buttonHitAdapter{btn: o.closeBtn}},
		{Rect: o.rect, ZIndex: ZOverlayMin, Tag: "settings-panel",
			Handler: settingsPanelHandler{}},
	}
}

// shortcutsVisible reports whether the keyboard-shortcuts section is shown.
// Desktop shows the localized shortcut list; mobile hides the section entirely
// (no header, no rows, no desktop-only note) since touch devices have no
// physical keyboard.
func (o *SettingsOverlay) shortcutsVisible() bool { return !Profile().IsMobile() }

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

// CloseButtonRect returns the close button's bounds. Valid after Layout.
func (o *SettingsOverlay) CloseButtonRect() image.Rectangle { return o.closeBtn.Rect() }

// Draw paints the modal scrim, the panel chrome, the language pills, the close
// button, and the localized keyboard-shortcuts list (desktop only).
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
	// Language pills as real Buttons: the active locale renders toggled (latched)
	// so it matches every other active control. Buttons handle their own keycap
	// chrome + press/hover animation.
	o.enBtn.SetToggled(i18n.ActiveLocale() == i18n.LocaleEN)
	o.esBtn.SetToggled(i18n.ActiveLocale() == i18n.LocaleES)
	o.enBtn.Draw(screen)
	o.esBtn.Draw(screen)
	// Close button (top-right).
	o.closeBtn.Draw(screen)
	// Shortcuts section — desktop only. Mobile hides it entirely (no physical
	// keyboard), so there is no header and no desktop-only note.
	if !o.shortcutsVisible() {
		return
	}
	y = o.esRect.Max.Y + 18
	DrawTextStyled(screen, i18n.T(i18n.KeySettingsShortcuts), x, y, RoleSectionHeader, colTextPrimary)
	y += 28
	for _, row := range localizedShortcutRows() {
		DrawTextStyled(screen, row.keys, x, y, RoleCaption, colTextPrimary)
		DrawTextStyled(screen, i18n.T(row.actionKey), x+120, y, RoleCaption, colTextSecondary)
		y += 26
	}
}
