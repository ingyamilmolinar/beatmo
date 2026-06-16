package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Shared geometry for the shortcuts overlay. Layout (panel height) and Draw
// (row stride) must agree, so they read the same constants — never redeclare
// these locally in one method without the other.
const (
	shortcutsOverlayW    = 460 // panel width (px)
	shortcutsOverlayLine = 26  // per-row vertical stride (px)
	shortcutsOverlayPadX = 24  // left/right inner padding (px)
	shortcutsOverlayPadY = 20  // top/bottom inner padding (px)
	shortcutsOverlayActX = 110 // x-offset of the action column from the panel's left padding
)

// shortcutRow is one line in the help overlay: a key glyph and a localized
// action description.
type shortcutRow struct {
	keys      string
	actionKey i18n.Key
}

// localizedShortcutRows returns the keyboard-shortcut rows with localized action
// descriptions (the physical key glyphs stay literal).
func localizedShortcutRows() []shortcutRow {
	return []shortcutRow{
		{"Space", i18n.KeyShortcutPlayPause},
		{"Esc", i18n.KeyShortcutCancel},
		{"Arrows", i18n.KeyShortcutPan},
		{"[  ]", i18n.KeyShortcutZoom},
		{"0", i18n.KeyShortcutResetZoom},
		{"+  -", i18n.KeyShortcutBPM},
		{"1 - 7", i18n.KeyShortcutTabs},
		{"?", i18n.KeyShortcutHelp},
	}
}

// ShortcutsOverlay is a modal portal panel listing the keyboard shortcuts.
// Presentational only (no hit areas); dismissed via the ? key, Esc, or toggling.
type ShortcutsOverlay struct {
	rect   image.Rectangle
	closed bool
}

// NewShortcutsOverlay creates a new keyboard shortcuts help overlay.
func NewShortcutsOverlay() *ShortcutsOverlay { return &ShortcutsOverlay{} }

// Close marks the overlay for removal on the next portal sweep.
func (o *ShortcutsOverlay) Close() { o.closed = true }

// Layout is a no-op: this overlay is window-global, so it centers on the full
// framebuffer at Draw time (the portal's screenBounds is only the drum pane).
// Kept to satisfy the PortalOverlay interface.
func (o *ShortcutsOverlay) Layout(_, _ image.Rectangle) {}

// HitAreas always returns nil — the shortcuts overlay does not consume input.
// A click anywhere routes through the tree's click-outside → CloseTop.
func (o *ShortcutsOverlay) HitAreas() []HitArea { return nil }

// Draw paints a full-window scrim and the shortcut panel centered on the whole
// framebuffer. The portal hands Draw the full screen image, so deriving
// geometry from screen.Bounds() centers the panel window-globally regardless of
// which pane opened it.
func (o *ShortcutsOverlay) Draw(screen *ebiten.Image) {
	b := screen.Bounds()
	if b.Empty() {
		return
	}
	// Full-window scrim so the panel reads as modal wherever it was triggered.
	drawRect(screen, b, colScrim, true)

	rows := localizedShortcutRows()
	w := shortcutsOverlayW
	h := shortcutsOverlayPadY*2 + shortcutsOverlayLine*(len(rows)+1)
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	r := image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	o.rect = r

	drawRoundedRect(screen, r, colSurface2, RadiusSM, true)
	drawRoundedRect(screen, r, colButtonBorder, RadiusSM, false)

	keyCol := r.Min.X + shortcutsOverlayPadX
	actCol := keyCol + shortcutsOverlayActX
	y := r.Min.Y + shortcutsOverlayPadY
	DrawTextStyled(screen, i18n.T(i18n.KeySettingsShortcuts), keyCol, y, RoleBody, colTextPrimary)
	y += shortcutsOverlayLine
	for _, row := range rows {
		DrawTextStyled(screen, row.keys, keyCol, y, RoleCaption, colTextPrimary)
		DrawTextStyled(screen, i18n.T(row.actionKey), actCol, y, RoleCaption, colTextSecondary)
		y += shortcutsOverlayLine
	}
}

// ShouldClose reports whether Close() has been invoked. Polled by
// OverlayPortal.CleanupClosed each frame.
func (o *ShortcutsOverlay) ShouldClose() bool { return o.closed }
