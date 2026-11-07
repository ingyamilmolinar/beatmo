package ui

import (
	"image"
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Matches audioControlHeaderH, so the two rows read as a stacked pair.
const stickyBarH = 32

// Taller on mobile so every control in it is finger-sized.
func stickyBarHeight() int {
	if Profile().IsMobile() {
		return 40
	}
	return stickyBarH
}

// Per-tab controls live in each tab's own component; the bar takes callbacks, so it never reaches
// back into its parent.
type AudioStickyBar struct {
	rect       image.Rectangle
	channelBtn *Button
	tabBtns    []*Button
	legendBtn  *Button
	// The parent sets it before Layout; Draw re-syncs it too.
	activeTab    PanelTab
	hitAreas     []HitArea
	parentZIndex int
	// Cached so the per-frame SetSynthDisabled never allocates; -1 when absent.
	synthTabIdx int
	// -1 when absent.
	samplerTabIdx int
	// Mobile only: action buttons the panels own, laid out on the always-present chrome row instead
	// of a separate in-panel row. Empty on desktop.
	trailingBtns []*Button
}

// Hit areas at parentZIndex+1, above the panel body. The bar does not own the active tab.
func NewAudioStickyBar(parentZIndex int, onChannel func(), onTab func(PanelTab)) *AudioStickyBar {
	b := &AudioStickyBar{parentZIndex: parentZIndex, synthTabIdx: -1, samplerTabIdx: -1}
	b.channelBtn = NewButton(i18n.T(i18n.KeyMaster), InstButtonStyle, onChannel)
	tabs := AllPanelTabs()
	b.tabBtns = make([]*Button, len(tabs))
	for i, tab := range tabs {
		t := tab // capture
		if t == TabSynth {
			b.synthTabIdx = i
		}
		if t == TabSampler {
			b.samplerTabIdx = i
		}
		b.tabBtns[i] = NewButton(PanelTabLabelForProfile(t), InstButtonStyle, func() {
			if onTab != nil {
				onTab(t)
			}
		})
	}

	// The parent binds OnClick.
	b.legendBtn = NewButton("?", InstButtonStyle, nil)
	b.legendBtn.TextColor = colTextSecondary
	return b
}

// On mobile the tab strip is suppressed: the bottom-bar switcher owns it.
func (b *AudioStickyBar) Layout(rect image.Rectangle) {
	b.rect = rect
	d := Profile().DensityValues()
	btnH := rect.Dy() - 8
	if btnH < d.AudioPillH {
		btnH = d.AudioPillH
	}
	y := rect.Min.Y + 4

	// Channel pill (left).
	chW := StyledTextWidth(b.channelBtn.Text, RoleCaption) + d.AudioPillPadX
	if chW < d.AudioChannelMinW {
		chW = d.AudioChannelMinW
	}
	if chW > rect.Dx()/3 && rect.Dx()/3 > 0 {
		chW = rect.Dx() / 3
	}
	b.channelBtn.SetRect(image.Rect(rect.Min.X+SpaceSM, y, rect.Min.X+6+chW, y+btnH))

	// Right-aligned cluster: legend, then the tab strip (desktop).
	rightEdge := rect.Max.X - SpaceSM

	// Hidden on mobile to keep the chrome row narrow; the bottom nav plays its role there.
	mobileChromeSquish := Profile().IsMobile()
	if b.legendBtn != nil {
		if mobileChromeSquish {
			b.legendBtn.SetRect(image.Rectangle{})
		} else {
			lW := d.AudioPillNarrowW
			b.legendBtn.SetRect(image.Rect(rightEdge-lW, y, rightEdge, y+btnH))
			rightEdge -= lW + d.AudioPillGap
		}
	}

	if Profile().IsMobile() {
		// Hide tab pills on mobile (Theme 1: bottom-bar switcher owns tabs).
		for i := range b.tabBtns {
			if b.tabBtns[i] != nil {
				b.tabBtns[i].SetRect(image.Rectangle{})
			}
		}
	} else {
		tabGap := d.AudioPillGap
		// Refresh tab labels in case the screen-class crossed mobile↔desktop.
		for i, tab := range AllPanelTabs() {
			if i < len(b.tabBtns) && b.tabBtns[i] != nil {
				b.tabBtns[i].Text = PanelTabLabelForProfile(tab)
			}
		}
		// Right to left, shrinking uniformly to fit, and clamped so no tab can overlap the channel pill.
		n := len(b.tabBtns)
		leftBound := b.channelBtn.Rect().Max.X + tabGap
		avail := rightEdge - leftBound
		naturalW := make([]int, n)
		naturalTotal := 0
		for i := 0; i < n; i++ {
			tw := StyledTextWidth(b.tabBtns[i].Text, RoleCaption) + d.AudioPillPadX
			if tw < d.AudioTabMinW {
				tw = d.AudioTabMinW
			}
			naturalW[i] = tw
			naturalTotal += tw
		}
		if n > 0 {
			naturalTotal += (n - 1) * tabGap
		}
		uniform := 0
		if n > 0 && avail > 0 && naturalTotal > avail {
			uniform = (avail - (n-1)*tabGap) / n
			if uniform < 1 {
				uniform = 1
			}
		}
		for i := n - 1; i >= 0; i-- {
			tw := naturalW[i]
			if uniform > 0 && uniform < tw {
				tw = uniform
			}
			// Hard clamp: never start left of the channel pill.
			if rightEdge-tw < leftBound {
				tw = rightEdge - leftBound
			}
			if tw <= 0 {
				// Collapse rather than draw over the channel pill.
				b.tabBtns[i].SetRect(image.Rectangle{})
				continue
			}
			b.tabBtns[i].SetRect(image.Rect(rightEdge-tw, y, rightEdge, y+btnH))
			rightEdge -= tw + tabGap
		}
	}

	// Cleared first so a stale rect never survives a desktop frame or a collapse.
	for i := range b.trailingBtns {
		if b.trailingBtns[i] != nil {
			b.trailingBtns[i].SetRect(image.Rectangle{})
		}
	}
	if Profile().IsMobile() && len(b.trailingBtns) > 0 {
		leftBound := b.channelBtn.Rect().Max.X + d.AudioPillGap
		for i := len(b.trailingBtns) - 1; i >= 0; i-- {
			btn := b.trailingBtns[i]
			if btn == nil {
				continue
			}
			var w int
			if btn.Text == "" { // icon-only (e.g. the ⋯ overflow) — square cell
				w = btnH
			} else {
				w = StyledTextWidth(btn.Text, RoleCaption) + d.AudioPillPadX
				if w < d.AudioPillNarrowW {
					w = d.AudioPillNarrowW
				}
			}
			if rightEdge-w < leftBound {
				// Collapse rather than cross the channel pill.
				btn.SetRect(image.Rectangle{})
				continue
			}
			btn.SetRect(image.Rect(rightEdge-w, y, rightEdge, y+btnH))
			rightEdge -= w + d.AudioPillGap
		}
	}
	b.rebuildHitAreas()
}

// Call before Layout; nil clears. The buttons keep their own OnClick.
func (b *AudioStickyBar) SetTrailingActions(btns []*Button) {
	b.trailingBtns = btns
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
	// Touch + ClipRect for a touch-min target without enlarging the visible pill.
	addChannelBtn := func() {
		btn := b.channelBtn
		if btn == nil || btn.Rect().Empty() {
			return
		}
		ha := HitArea{Rect: btn.Rect(), ZIndex: z, Handler: &buttonHitAdapter{btn: btn}, Tag: "eq-channel-btn"}
		if Profile().IsMobile() {
			ha.Touch = true
			ha.ClipRect = expandToTouchMin(btn.Rect())
		}
		b.hitAreas = append(b.hitAreas, ha)
	}
	addChannelBtn()
	for i, tb := range b.tabBtns {
		addBtn(tb, indexedTag(eqTabTags[:], "eq-tab-", i))
	}
	addBtn(b.legendBtn, "eq-legend-btn")
	// Same touch expansion as the channel pill.
	for i, tb := range b.trailingBtns {
		if tb == nil || tb.Rect().Empty() {
			continue
		}
		ha := HitArea{Rect: tb.Rect(), ZIndex: z, Handler: &buttonHitAdapter{btn: tb}, Tag: indexedTag(eqTrailingTags[:], "eq-trailing-", i)}
		if Profile().IsMobile() {
			ha.Touch = true
			ha.ClipRect = expandToTouchMin(tb.Rect())
		}
		b.hitAreas = append(b.hitAreas, ha)
	}
}

func (b *AudioStickyBar) Draw(dst *ebiten.Image, activeTab PanelTab) {
	// For direct-Draw callers.
	b.activeTab = activeTab
	// A selector (identity), not a tab: lit like a tab it made every screen read as having two active tabs.
	drawPillTabAt(dst, b.channelBtn, false)
	if r := b.channelBtn.Rect(); !r.Empty() {
		// No outline: flat-tonal has no border at rest; the caret says "this opens a list".
		const caretW = 8
		caretR := image.Rect(r.Max.X-caretW-3, r.Min.Y+(r.Dy()-caretW)/2, r.Max.X-3, r.Min.Y+(r.Dy()-caretW)/2+caretW)
		DrawIcon(dst, IconChevronDown, caretR, colTextAccent)
	}

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
	if b.legendBtn != nil {
		drawPillTabAt(dst, b.legendBtn, false)
	}
	// Their own spec Draw, matching the app's action buttons.
	for _, tb := range b.trailingBtns {
		if tb == nil || tb.Rect().Empty() {
			continue
		}
		tb.Draw(dst)
	}
}

// The parent binds OnClick.
func (b *AudioStickyBar) LegendBtn() *Button { return b.legendBtn }

// Call before Layout; Draw also syncs it for direct-Draw callers.
func (b *AudioStickyBar) SetActiveTab(tab PanelTab) {
	b.activeTab = tab
}

// Called every frame. The Disabled flag lives on the Button, so the shared adapter already
// swallows clicks.
func (b *AudioStickyBar) SetSynthDisabled(disabled bool) {
	if pill := b.synthTabPill(); pill != nil {
		pill.Disabled = disabled
	}
}

// Meaningless on the master bus, which has no single sample.
func (b *AudioStickyBar) SetSamplerDisabled(disabled bool) {
	if pill := b.samplerTabPill(); pill != nil {
		pill.Disabled = disabled
	}
}

// Cached index, so the per-frame path never allocates.
func (b *AudioStickyBar) synthTabPill() *Button {
	return b.TabBtn(b.synthTabIdx)
}

// samplerTabPill returns the tab pill that corresponds to TabSampler, or nil.
func (b *AudioStickyBar) samplerTabPill() *Button {
	return b.TabBtn(b.samplerTabIdx)
}

// HitAreas returns the cached chrome hit areas. Call Layout first.
func (b *AudioStickyBar) HitAreas() []HitArea { return b.hitAreas }

func (b *AudioStickyBar) Rect() image.Rectangle { return b.rect }

// ChannelBtn returns the channel-selector pill button (persistent left chip).
func (b *AudioStickyBar) ChannelBtn() *Button { return b.channelBtn }

// TabBtn returns the tab pill at index i, or nil for out-of-range i.
func (b *AudioStickyBar) TabBtn(i int) *Button {
	if i < 0 || i >= len(b.tabBtns) {
		return nil
	}
	return b.tabBtns[i]
}

// Pre-boxed once per theme apply (in rebuildThemeAliases): boxing these per pill per frame was the
// dominant allocation on the Levels tab.
var (
	pillCapFillInactive color.Color
	pillCapBorderInact  color.Color
	pillTextInactive    color.Color
	pillCapFillActive   color.Color
	pillCapBorderActive color.Color
	pillTextActive      color.Color
	pillShellInactive   color.Color
	pillShellActive     color.Color
	pillCapFillDisabled color.Color
	pillCapBorderDisab  color.Color
	pillTextDisabled    color.Color
	pillShellDisabled   color.Color
)

// A func var so tests can count the pills a tab renders: the panel's control count is worth guarding.
var drawPillTabAt = func(dst *ebiten.Image, btn *Button, active bool) {
	if btn == nil {
		return
	}
	r := btn.Rect()
	if r.Empty() {
		return
	}
	// A disabled pill is inert: never raised/active, no engage flash, greyed.
	if btn.Disabled {
		active = false
	}
	// Pills bypass Button.Draw, so tick the press/engage springs here.
	btn.AdvancePressAnim()
	if active && !btn.wasActive {
		btn.engageAnim = 1 // one-shot flash on selection
	}
	btn.wasActive = active

	pillRadius := RadiusControl // every control shares one radius (DESIGN.md "Shapes")
	// Flat-tonal pill: the body IS the full rect. No cap/socket split.
	capR := r

	// Pre-boxed interfaces, so picking a state copies a header instead of boxing a color.
	capFill, textCol := pillCapFillInactive, pillTextInactive
	switch {
	case btn.Disabled:
		capFill, textCol = pillCapFillDisabled, pillTextDisabled
	case active:
		// A bright label on the accent-tinted body.
		capFill, textCol = pillCapFillInactive, colTextPrimary
	}
	// Selection is a tint plus a 2px bar, never a flood.
	drawFlatButtonBody(dst, capR, capFill, pillRadius)
	if active && !btn.Disabled {
		drawRoundedRect(dst, capR, WithAlpha(colAccent, AlphaSubtle), pillRadius, true)
		drawFlatSelectionBar(dst, capR, colAccent, selectionEdgeBottom, 1)
	}

	if btn.Icon != "" {
		iconCol := btn.effectiveIconColor()
		if iconCol == nil {
			iconCol = textCol
		}
		DrawIcon(dst, IconID(btn.Icon), capR, iconCol)
		return
	}
	if tc := btn.effectiveTextColor(); tc != nil {
		textCol = tc
	}
	tw := StyledTextWidth(btn.Text, RoleCaption)
	th := StyledTextHeight(RoleCaption)
	tx := capR.Min.X + (capR.Dx()-tw)/2
	ty := capR.Min.Y + (capR.Dy()-th)/2
	DrawTextStyled(dst, btn.Text, tx, ty, RoleCaption, textCol)
}

// Built once: these hit areas are rebuilt every frame.
var eqTabTags, eqTrailingTags = indexedTags("eq-tab-"), indexedTags("eq-trailing-")

func indexedTags(prefix string) (tags [16]string) {
	for i := range tags {
		tags[i] = prefix + strconv.Itoa(i)
	}
	return
}

// indexedTag returns prefix+i from the prebuilt table (formatted past its end).
func indexedTag(tags []string, prefix string, i int) string {
	if i >= 0 && i < len(tags) {
		return tags[i]
	}
	return prefix + strconv.Itoa(i)
}
