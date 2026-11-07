package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// An editor card: its desktop width is the editor class.

// Consumes background presses so none fall through; the per-control areas register BEFORE it and win the equal-z
// tie. Also owns the shortcuts list's scroll, through the shared MenuScroll.
type settingsPanelHandler struct{ o *SettingsOverlay }

func (h settingsPanelHandler) OnPress(x, y int) InputResult {
	if h.o != nil && h.o.scroll != nil && h.o.scroll.HandleScrollbarDrag(image.Pt(x, y), true, nil) {
		return InputCaptured
	}
	return InputConsumed
}

func (h settingsPanelHandler) OnDrag(x, y int) {
	if h.o != nil && h.o.scroll != nil {
		h.o.scroll.HandleScrollbarDrag(image.Pt(x, y), true, nil)
	}
}

func (h settingsPanelHandler) OnRelease(x, y int) {
	if h.o != nil && h.o.scroll != nil {
		h.o.scroll.HandleScrollbarDrag(image.Pt(x, y), false, nil)
	}
}

func (h settingsPanelHandler) OnWheel(x, y, steps int) InputResult {
	if h.o != nil && h.o.scroll != nil {
		h.o.scroll.HandleWheel(steps)
	}
	return InputConsumed
}

type SettingsOverlay struct {
	onPick func(i18n.Locale)
	closed bool
	card   *MenuCard // shared frame: panel, title band (IconSettings), close ×
	rect   image.Rectangle
	enRect image.Rectangle
	esRect image.Rectangle

	// Real Buttons, so the pills share keycap chrome; the active language renders latched. No close ×: the gear button
	// toggles the overlay.
	enBtn *Button
	esBtn *Button

	// Owner-injected, so the panel anchors to a grid-pane button without importing Game; nil centres it.
	place func(w, h int) image.Rectangle

	// The shared MenuScroll, used when a short pane clamps the panel; a no-op when the list fits.
	scroll        *MenuScroll
	shortcutsView image.Rectangle

	// Computed in Layout from the card padding and role heights so Draw stays in lock-step.
	langLabelY int
}

// Draw and the scroll viewport must agree.
const settingsShortcutRowH = 26

// NewSettingsOverlay creates a settings portal overlay; onPick fires on a pill tap.
func NewSettingsOverlay(onPick func(i18n.Locale)) *SettingsOverlay {
	o := &SettingsOverlay{onPick: onPick}
	o.card = NewMenuCard(o.Close)
	o.enBtn = NewButtonKey(i18n.KeyLangEnglish, ComponentButtonSecondary, func() { o.pick(i18n.LocaleEN) })
	o.esBtn = NewButtonKey(i18n.KeyLangSpanish, ComponentButtonSecondary, func() { o.pick(i18n.LocaleES) })
	o.scroll = NewMenuScroll(DropdownScrollbarStyle, settingsShortcutRowH)
	return o
}

// How the desktop overlay anchors under the grid gear.
func (o *SettingsOverlay) SetPositioner(place func(w, h int) image.Rectangle) { o.place = place }

// Close marks the overlay for removal on the next portal sweep.
func (o *SettingsOverlay) Close() { o.closed = true }

// ShouldClose reports whether Close() has been invoked.
func (o *SettingsOverlay) ShouldClose() bool { return o.closed }

func (o *SettingsOverlay) Layout(anchor, screenBounds image.Rectangle) {
	b := screenBounds
	w := menuWEditor()
	// A side margin keeps the fixed 480 px panel on a ~390 px phone.
	if maxW := b.Dx() - 2*24; maxW > 0 && w > maxW {
		w = maxW
	}
	// Mobile drops the shortcuts section, so its panel is shorter.
	rows := 0
	h := menuCardTitleH + menuCardPad + 76
	if o.shortcutsVisible() {
		rows = len(localizedShortcutRows())
		h += settingsShortcutRowH * (rows + 1)
	}
	// Clamped so a short pane scrolls the list instead of hiding rows past the edge.
	if maxH := b.Dy() - 2*12; maxH > 0 && h > maxH {
		h = maxH
	}
	if o.place != nil {
		o.rect = o.place(w, h)
	} else {
		cx := (b.Min.X + b.Max.X) / 2
		cy := (b.Min.Y + b.Max.Y) / 2
		o.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	}
	// Every offset derives from the card padding and role heights, so Draw recomputes the same positions.
	o.card.Layout(o.rect, MenuCardSpec{Title: i18n.T(i18n.KeySettingsTitle), IconID: IconSettings})
	labelH := StyledTextHeight(RoleSectionHeader)
	o.langLabelY = o.card.Body().Min.Y
	pillW, pillH := 138, 32
	px := o.card.Body().Min.X
	py := o.langLabelY + labelH + SpaceXS
	o.enRect = image.Rect(px, py, px+pillW, py+pillH)
	o.esRect = image.Rect(px+pillW+SpaceMD, py, px+pillW+SpaceMD+pillW, py+pillH)
	o.enBtn.SetRect(o.enRect)
	o.esBtn.SetRect(o.esRect)

	// Below the fixed header; rows offset by OffsetPx and clip here.
	if o.shortcutsVisible() && o.scroll != nil {
		listTop := o.esRect.Max.Y + 18 + 28 // header at +18, first row at +28 (see Draw)
		listBot := o.rect.Max.Y - 8
		o.shortcutsView = image.Rect(o.card.Body().Min.X, listTop, o.rect.Max.X-SpaceMD, listBot)
		visRows := 0
		if listBot > listTop {
			visRows = (listBot - listTop) / settingsShortcutRowH
		}
		if visRows < 1 {
			visRows = 1
		}
		o.scroll.Configure(o.shortcutsView, rows, visRows)
	} else {
		o.shortcutsView = image.Rectangle{}
	}
}

// The catch-all goes LAST so the controls win the equal-z stable sort.
func (o *SettingsOverlay) HitAreas() []HitArea {
	if o.rect.Empty() {
		return nil
	}
	areas := o.card.ControlAreas(ZOverlayMin, "settings")
	areas = append(areas,
		HitArea{Rect: o.enBtn.Rect(), ZIndex: ZOverlayMin, Tag: "settings-lang-en",
			Handler: &buttonHitAdapter{btn: o.enBtn}},
		HitArea{Rect: o.esBtn.Rect(), ZIndex: ZOverlayMin, Tag: "settings-lang-es",
			Handler: &buttonHitAdapter{btn: o.esBtn}},
	)
	return append(areas, o.card.CatchAll(ZOverlayMin, "settings", settingsPanelHandler{o: o}))
}

// Mobile has no physical keyboard, so the whole section goes.
func (o *SettingsOverlay) shortcutsVisible() bool { return !Profile().IsMobile() }

func (o *SettingsOverlay) pick(l i18n.Locale) {
	if o.onPick != nil {
		o.onPick(l)
	}
}

// Valid after Layout.
func (o *SettingsOverlay) LanguagePillRects() (en, es image.Rectangle) {
	return o.enRect, o.esRect
}

func (o *SettingsOverlay) Draw(screen *ebiten.Image) {
	if screen == nil || o.rect.Empty() {
		return
	}
	// On the UNCLIPPED screen so the drop shadow survives; the portal paints the scrim.
	o.card.Draw(screen)
	// So a tall list never spills past the split into the drum view.
	dst := screen.SubImage(o.rect).(*ebiten.Image)
	x := o.card.Body().Min.X
	// The same role as "Keyboard shortcuts", so the two sections read at one level.
	DrawTextStyled(dst, i18n.T(i18n.KeySettingsLanguage), x, o.langLabelY, RoleSectionHeader, colTextPrimary)
	o.enBtn.SetToggled(i18n.ActiveLocale() == i18n.LocaleEN)
	o.esBtn.SetToggled(i18n.ActiveLocale() == i18n.LocaleES)
	o.enBtn.Draw(dst)
	o.esBtn.Draw(dst)
	if !o.shortcutsVisible() {
		return
	}
	// Fixed "Shortcuts" header (never scrolls).
	DrawTextStyled(dst, i18n.T(i18n.KeySettingsShortcuts), x, o.esRect.Max.Y+18, RoleSectionHeader, colTextPrimary)
	// The clip keeps a scrolled row off the header and the panel bottom.
	if !o.shortcutsView.Empty() {
		rowsDst := dst.SubImage(o.shortcutsView).(*ebiten.Image)
		ry := o.shortcutsView.Min.Y - o.scroll.OffsetPx()
		// Term/definition: the key primary, the action a muted gloss.
		keyH := StyledTextHeight(RoleBody)
		descDY := (keyH - StyledTextHeight(RoleCaption)) / 2
		for _, row := range localizedShortcutRows() {
			DrawTextStyled(rowsDst, row.keys, x, ry, RoleBody, colTextPrimary)
			DrawTextStyled(rowsDst, row.action(), x+96, ry+descDY, RoleCaption, colTextSecondary)
			ry += settingsShortcutRowH
		}
		o.scroll.Draw(dst)
	}
}
