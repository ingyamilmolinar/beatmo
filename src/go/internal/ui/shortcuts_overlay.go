package ui

import (
	"image"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Layout and Draw must agree, so both read these.
const (
	shortcutsOverlayW    = 460 // panel width (px)
	shortcutsOverlayLine = 26  // per-row vertical stride (px)
	shortcutsOverlayPadX = 24  // left/right inner padding (px)
	shortcutsOverlayPadY = 20  // top/bottom inner padding (px)
	shortcutsOverlayActX = 110 // x-offset of the action column from the panel's left padding
)

type shortcutRow struct {
	keys      string
	actionKey i18n.Key
	text      string // a composed description (wins over actionKey when set)
}

// action is the row's description in the active locale.
func (r shortcutRow) action() string {
	if r.text != "" {
		return r.text
	}
	return i18n.T(r.actionKey)
}

// From the tabs the keys actually reach: production has no Chain tab.
func tabShortcutRow() shortcutRow {
	tabs := AllPanelTabs()
	names := make([]string, len(tabs))
	for i, t := range tabs {
		names[i] = PanelTabLabel(t)
	}
	return shortcutRow{keys: "1 - " + strconv.Itoa(len(tabs)), actionKey: i18n.KeyShortcutTabs, text: strings.Join(names, " / ")}
}

// The key glyphs stay literal.
func localizedShortcutRows() []shortcutRow {
	return []shortcutRow{
		{keys: "Space", actionKey: i18n.KeyShortcutPlayPause},
		{keys: "Esc", actionKey: i18n.KeyShortcutCancel},
		{keys: "Arrows", actionKey: i18n.KeyShortcutPan},
		{keys: "[  ]", actionKey: i18n.KeyShortcutZoom},
		{keys: "0", actionKey: i18n.KeyShortcutResetZoom},
		{keys: "+  -", actionKey: i18n.KeyShortcutBPM},
		tabShortcutRow(),
		{keys: "?", actionKey: i18n.KeyShortcutHelp},
	}
}

// Presentational: no hit areas; a click anywhere closes it via click-outside.
type ShortcutsOverlay struct {
	rect   image.Rectangle
	closed bool
}

// NewShortcutsOverlay creates a new keyboard shortcuts help overlay.
func NewShortcutsOverlay() *ShortcutsOverlay { return &ShortcutsOverlay{} }

// Close marks the overlay for removal on the next portal sweep.
func (o *ShortcutsOverlay) Close() { o.closed = true }

// A no-op: the overlay centres on the whole framebuffer at Draw, since the portal's bounds are only the drum pane.
func (o *ShortcutsOverlay) Layout(_, _ image.Rectangle) {}

// A click anywhere routes to the tree's click-outside → CloseTop.
func (o *ShortcutsOverlay) HitAreas() []HitArea { return nil }

// Draw gets the full screen, so centring on its bounds is window-global whichever pane opened it.
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
	DrawTextStyled(screen, i18n.T(i18n.KeySettingsShortcuts), keyCol, y, RoleSectionHeader, colTextPrimary)
	y += StyledTextHeight(RoleSectionHeader) + SpaceSM
	for _, row := range rows {
		DrawTextStyled(screen, row.keys, keyCol, y, RoleCaption, colTextPrimary)
		DrawTextStyled(screen, row.action(), actCol, y, RoleCaption, colTextSecondary)
		y += shortcutsOverlayLine
	}
}

func (o *ShortcutsOverlay) ShouldClose() bool { return o.closed }
