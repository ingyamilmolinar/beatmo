package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// stickyBarH is the chrome strip height shared by the Wave Analyzer panel
// and the ChainPanelZone. 4px top pad + 18px button + 4px bottom pad.
const stickyBarH = 26

// AudioStickyBar owns the chrome strip at the top of EQPanelZone: channel
// dropdown trigger, five tab pills, freeze toggle, close button. The bar is
// fully self-contained; its constructor receives callbacks for each action,
// so the bar never reaches back into its parent zone.
type AudioStickyBar struct {
	rect         image.Rectangle
	channelBtn   *Button
	tabBtns      [5]*Button
	freezeBtn    *Button
	closeBtn     *Button
	freqScaleBtn *Button // log/lin chip (Phase D); nil until then
	hitAreas     []HitArea
	parentZIndex int
}

// NewAudioStickyBar builds the chrome bar. parentZIndex is the owning zone's
// z-index; the bar's own hit areas register at parentZIndex+1 so they sit
// above the panel body. The onTab callback receives the tab the user picked;
// the bar itself does not own the active-tab state — the parent passes the
// active tab to Draw so the bar can render the correct active pill.
func NewAudioStickyBar(parentZIndex int, onChannel, onFreeze, onClose func(), onTab func(PanelTab)) *AudioStickyBar {
	b := &AudioStickyBar{parentZIndex: parentZIndex}
	b.channelBtn = NewButton("Master", InstButtonStyle, onChannel)
	for i, tab := range AllPanelTabs() {
		t := tab // capture
		b.tabBtns[i] = NewButton(PanelTabLabelForProfile(t), InstButtonStyle, func() {
			if onTab != nil {
				onTab(t)
			}
		})
	}
	// Freeze indicator uses single-character text per DESIGN.md §5d
	// (permitted text-glyph exception): "||" when capturing, ">" when frozen.
	b.freezeBtn = NewButton("||", InstButtonStyle, onFreeze)
	b.freezeBtn.TextColor = colTextSecondary
	// Close button: drawn as a glyph via Button.Icon; the empty Text keeps
	// the pill compact while still routing through the standard hit-area
	// adapter for click delivery.
	b.closeBtn = NewButton("", InstButtonStyle, onClose)
	b.closeBtn.Icon = string(IconClose)
	b.closeBtn.IconColor = colTextSecondary
	return b
}

// Layout positions every button inside rect. Channel goes on the left;
// close, freeze, and the tab strip pack right-to-left. On mobile the tab
// strip is suppressed — the bottom-bar segmented switcher owns it.
func (b *AudioStickyBar) Layout(rect image.Rectangle) {
	b.rect = rect
	btnH := 18
	y := rect.Min.Y + 4

	// Channel pill (left).
	chW := TextWidth(b.channelBtn.Text) + 12
	if chW < 72 {
		chW = 72
	}
	if chW > rect.Dx()/3 && rect.Dx()/3 > 0 {
		chW = rect.Dx() / 3
	}
	b.channelBtn.SetRect(image.Rect(rect.Min.X+6, y, rect.Min.X+6+chW, y+btnH))

	// Right-aligned cluster: close, freeze, tabs (when desktop).
	rightEdge := rect.Max.X - 6
	closeW := 24
	b.closeBtn.SetRect(image.Rect(rightEdge-closeW, y, rightEdge, y+btnH))
	rightEdge -= closeW + 3

	freezeW := 24
	b.freezeBtn.SetRect(image.Rect(rightEdge-freezeW, y, rightEdge, y+btnH))
	rightEdge -= freezeW + 3

	if Profile().IsMobile() {
		// Hide tab pills on mobile (Theme 1: bottom-bar switcher owns tabs).
		for i := range b.tabBtns {
			if b.tabBtns[i] != nil {
				b.tabBtns[i].SetRect(image.Rectangle{})
			}
		}
	} else {
		const tabGap = 2
		// Refresh tab labels in case the screen-class crossed mobile↔desktop.
		for i, tab := range AllPanelTabs() {
			if i < len(b.tabBtns) && b.tabBtns[i] != nil {
				b.tabBtns[i].Text = PanelTabLabelForProfile(tab)
			}
		}
		for i := len(b.tabBtns) - 1; i >= 0; i-- {
			tw := TextWidth(b.tabBtns[i].Text) + 12
			if tw < 28 {
				tw = 28
			}
			b.tabBtns[i].SetRect(image.Rect(rightEdge-tw, y, rightEdge, y+btnH))
			rightEdge -= tw + tabGap
		}
	}
	b.rebuildHitAreas()
}

func (b *AudioStickyBar) rebuildHitAreas() {
	b.hitAreas = b.hitAreas[:0]
	z := b.parentZIndex + 1
	addBtn := func(btn *Button, tag string) {
		if btn == nil {
			return
		}
		r := btn.Rect()
		if r.Empty() {
			return
		}
		b.hitAreas = append(b.hitAreas, HitArea{
			Rect:    r,
			ZIndex:  z,
			Handler: &buttonHitAdapter{btn: btn},
			Tag:     tag,
		})
	}
	addBtn(b.channelBtn, "eq-channel-btn")
	for i, tb := range b.tabBtns {
		addBtn(tb, fmt.Sprintf("eq-tab-%d", i))
	}
	addBtn(b.freezeBtn, "eq-freeze-btn")
	addBtn(b.closeBtn, "eq-close-btn")
}

// Draw renders the entire chrome strip. activeTab tells the bar which tab
// pill should be highlighted as active; the channel pill is always rendered
// in its active state (it's a persistent selector, not a tab).
func (b *AudioStickyBar) Draw(dst *ebiten.Image, activeTab PanelTab) {
	drawPillTabAt(dst, b.channelBtn, true)
	if !Profile().IsMobile() {
		tabs := AllPanelTabs()
		for i, tb := range b.tabBtns {
			if tb == nil {
				continue
			}
			active := i < len(tabs) && tabs[i] == activeTab
			drawPillTabAt(dst, tb, active)
		}
	}
	// Freeze pill: parent owns the text/color update (so it reflects the
	// analyzer state); the bar just draws whatever the pill currently shows.
	frozen := b.freezeBtn != nil && b.freezeBtn.Text == ">"
	drawPillTabAt(dst, b.freezeBtn, frozen)
	drawPillTabAt(dst, b.closeBtn, false)
}

// HitAreas returns the cached chrome hit areas. Call Layout first.
func (b *AudioStickyBar) HitAreas() []HitArea { return b.hitAreas }

// ChannelBtn returns the channel-selector pill button (persistent left chip).
func (b *AudioStickyBar) ChannelBtn() *Button { return b.channelBtn }

// TabBtn returns the tab pill at index i, or nil for out-of-range i.
func (b *AudioStickyBar) TabBtn(i int) *Button {
	if i < 0 || i >= len(b.tabBtns) {
		return nil
	}
	return b.tabBtns[i]
}

// FreezeBtn returns the freeze/resume toggle pill.
func (b *AudioStickyBar) FreezeBtn() *Button { return b.freezeBtn }

// CloseBtn returns the close-the-panel chip.
func (b *AudioStickyBar) CloseBtn() *Button { return b.closeBtn }

// drawPillTabAt is the file-scope free-function version of the legacy
// EQPanelZone.drawPillTab method. Copied here so AudioStickyBar can render
// pills without holding a receiver reference to the zone.
func drawPillTabAt(dst *ebiten.Image, btn *Button, active bool) {
	if btn == nil {
		return
	}
	r := btn.Rect()
	if r.Empty() {
		return
	}
	pillRadius := RadiusMD / 2 // 4px corners
	if active {
		drawRoundedRect(dst, r, colSurface2, pillRadius, true)
		drawRoundedRect(dst, r, colAccent, pillRadius, false)
	} else {
		drawRoundedRect(dst, r, colButtonBorder, pillRadius, false)
	}
	// Icon-only pills (e.g. close) draw the glyph centered; otherwise draw
	// the button's text label, color-coded by active/inactive state.
	if btn.Icon != "" {
		iconCol := btn.IconColor
		if iconCol == nil {
			iconCol = colTextSecondary
		}
		DrawIcon(dst, IconID(btn.Icon), r, iconCol)
		return
	}
	var textCol color.Color = colTextSecondary
	if active {
		textCol = colTextAccent
	}
	if btn.TextColor != nil {
		textCol = btn.TextColor
	}
	tw := TextWidth(btn.Text)
	th := TextHeight()
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	DrawTextColorAt(dst, btn.Text, tx, ty, textCol)
}
