package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The Settings panel is an editor card (DESIGN.md "Menu sizing & layout"):
// its desktop width is the editor class, menuWEditor().

// settingsPanelHandler is the full-panel catch-all: it consumes any press that
// lands on the panel background (not on a pill or the close button) so a
// panel-background click never falls through. The pills and close button carry
// their own per-control hit areas (shared buttonHitAdapter) registered BEFORE
// this catch-all, so they win the equal-z stable-sort tiebreak on overlap. The
// overlay is opened modal, so presses outside the panel are filtered out by the
// HitIndex before they ever reach a handler.
//
// It also owns the shortcuts-list scroll input: wheel + scrollbar-thumb drag go
// through the shared MenuScroll (o.scroll), so the settings shortcuts scroll with
// the same smooth wheel feel as every other list menu.
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

// SettingsOverlay is the portal panel for language selection plus a localized
// keyboard-shortcuts list. onPick is invoked when the user taps a language pill.
type SettingsOverlay struct {
	onPick func(i18n.Locale)
	closed bool
	card   *MenuCard // shared frame: panel, title band (IconSettings), close ×
	rect   image.Rectangle
	enRect image.Rectangle
	esRect image.Rectangle

	// Real Buttons so the pills get the shared keycap chrome and press/hover
	// animation as every other control (DESIGN.md). The active language pill is
	// rendered toggled (latched amber). Created once; SetRect'd each Layout.
	// There is NO close (×) button: the gear button toggles this overlay closed.
	enBtn *Button
	esBtn *Button

	// place, when set, positions the panel (owner-injected so the overlay can
	// anchor to a grid-pane button via the shared helper without importing Game).
	// nil → the panel centers on screenBounds (mobile / no owner).
	place func(w, h int) image.Rectangle

	// scroll drives the desktop shortcuts list when the panel is height-clamped
	// (short pane). The same shared MenuScroll engine every list menu uses, so
	// the shortcuts scroll with the identical smooth wheel + thumb-drag feel. It
	// no-ops (HasScroll()==false) whenever the whole list fits, which is the
	// common case. shortcutsView is the row viewport (excludes the fixed header).
	scroll        *MenuScroll
	shortcutsView image.Rectangle

	// langLabelY is the baseline of the "Language" section label, computed in
	// Layout from the shared card padding + role heights so Draw places the
	// label (and everything below it) in lock-step.
	langLabelY int
}

// settingsShortcutRowH is the per-row stride of the shortcuts list (Draw and the
// scroll viewport must agree).
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

// SetPositioner injects an owner-supplied positioner: place(w,h) returns the
// panel rect for a panel of the given size. When set, Layout uses it instead of
// centering — this is how the desktop overlay anchors under the grid gear.
func (o *SettingsOverlay) SetPositioner(place func(w, h int) image.Rectangle) { o.place = place }

// Close marks the overlay for removal on the next portal sweep.
func (o *SettingsOverlay) Close() { o.closed = true }

// ShouldClose reports whether Close() has been invoked.
func (o *SettingsOverlay) ShouldClose() bool { return o.closed }

// Layout centers the panel on the screen and positions the two language pills
// and the close button.
func (o *SettingsOverlay) Layout(anchor, screenBounds image.Rectangle) {
	b := screenBounds
	w := menuWEditor()
	// Clamp to the available width with a side margin so the panel (and its
	// content, inset 24px) stays fully on-screen on narrow mobile panes — the
	// fixed 480px width otherwise centers off both edges of a ~390px phone.
	if maxW := b.Dx() - 2*24; maxW > 0 && w > maxW {
		w = maxW
	}
	// Height: header + language row, plus the shortcuts section on desktop only
	// (mobile drops it entirely, so the panel is correspondingly shorter).
	rows := 0
	h := menuCardTitleH + menuCardPad + 76
	if o.shortcutsVisible() {
		rows = len(localizedShortcutRows())
		h += settingsShortcutRowH * (rows + 1)
	}
	// Clamp to the available vertical space (side/top margin) so a short pane
	// never hides shortcut rows past the bottom edge with no way to reach them —
	// the list scrolls instead of truncating.
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
	// Title band → "Language" section label → the two pills. Every offset
	// derives from the shared card padding + text-role heights, so Draw stays
	// in lock-step (it recomputes langLabelY the same way). One Title >
	// Section > control ladder shared by all three cards.
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

	// Configure the shortcuts scroll viewport: the region below the fixed
	// "Shortcuts" header down to the panel's inner bottom. Draw offsets the rows
	// by o.scroll.OffsetPx() and clips them here. No-ops when the list fits.
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

// HitAreas publishes one hit area per interactive control (the two language
// pills and the close button) plus a full-panel catch-all registered LAST so
// the pill/close areas win the portal's equal-z stable-sort on overlap. Each
// control routes through the shared buttonHitAdapter so it shares the canonical
// press lifecycle (edge fire, press/hover animation, release settle).
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

// Draw paints the modal scrim, the panel chrome, the language pills, the close
// button, and the localized keyboard-shortcuts list (desktop only).
func (o *SettingsOverlay) Draw(screen *ebiten.Image) {
	if screen == nil || o.rect.Empty() {
		return
	}
	// Shared MenuCard frame (panel, title band with the gear icon, close ×) on
	// the UNCLIPPED screen so the drop shadow survives; the portal paints the
	// scrim (menuCardEntry: Scrim).
	o.card.Draw(screen)
	// Clip the panel CONTENT to o.rect (grid-pane-clamped) so a tall shortcuts
	// list can never spill past the split into the drum-view panel below.
	dst := screen.SubImage(o.rect).(*ebiten.Image)
	x := o.card.Body().Min.X
	// "Language" section label — same role/size/colour as "Keyboard shortcuts"
	// below, so the two peer sections read at one hierarchy level (subtitle).
	DrawTextStyled(dst, i18n.T(i18n.KeySettingsLanguage), x, o.langLabelY, RoleSectionHeader, colTextPrimary)
	// Language pills as real Buttons: the active locale renders toggled (latched)
	// so it matches every other active control. Buttons handle their own keycap
	// chrome + press/hover animation.
	o.enBtn.SetToggled(i18n.ActiveLocale() == i18n.LocaleEN)
	o.esBtn.SetToggled(i18n.ActiveLocale() == i18n.LocaleES)
	o.enBtn.Draw(dst)
	o.esBtn.Draw(dst)
	// Shortcuts section — desktop only. Mobile hides it entirely (no physical
	// keyboard), so there is no header and no desktop-only note.
	if !o.shortcutsVisible() {
		return
	}
	// Fixed "Shortcuts" header (never scrolls).
	DrawTextStyled(dst, i18n.T(i18n.KeySettingsShortcuts), x, o.esRect.Max.Y+18, RoleSectionHeader, colTextPrimary)
	// Rows scroll inside shortcutsView by o.scroll.OffsetPx(); the SubImage clip
	// keeps a scrolled row from painting over the header or past the panel bottom,
	// and o.scroll.Draw paints the scrollbar (a no-op when the list fits).
	if !o.shortcutsView.Empty() {
		rowsDst := dst.SubImage(o.shortcutsView).(*ebiten.Image)
		ry := o.shortcutsView.Min.Y - o.scroll.OffsetPx()
		// term/definition: the key is the primary body term (white), the action
		// its caption gloss (muted) — vertically centred to the taller key row.
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
