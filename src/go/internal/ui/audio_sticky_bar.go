package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// stickyBarH is the chrome strip height shared by the Wave Analyzer panel
// and the ChainPanelZone. Sized to contain the unified audio pill
// (audioPillHeight, Comfortable 24) with ~4px top/bottom pad, matching the
// per-tab control header (audioControlHeaderH) so the two rows read as a
// stacked pair.
const stickyBarH = 32

// stickyBarHeight is the chrome strip height: taller on mobile so the Master
// channel pill, tab pills, legend, and expander all render at a finger-friendly
// size (matches controlHeaderHeight's mobile value).
func stickyBarHeight() int {
	if Profile().IsMobile() {
		return 40
	}
	return stickyBarH
}

// AudioStickyBar owns the tab-switcher row at the top of EQPanelZone: the
// channel dropdown trigger, the tab pills, the "?" legend chip, and the panel
// expander chevron. Every per-tab control (freeze / freq-scale / slope / Pre /
// reset-hold / clear-clips / K-20) now lives in its own per-tab component
// (audio_tab_controls.go); the close button was removed entirely. The bar is
// fully self-contained — its constructor receives callbacks for channel + tab
// selection, so the bar never reaches back into its parent zone.
type AudioStickyBar struct {
	rect       image.Rectangle
	channelBtn *Button
	// tabBtns is a slice rather than a fixed array so the strip can grow
	// (Phase 4 added a 6th Synth tab; future plugin recipes may add more).
	tabBtns []*Button
	// Phase 5 audio-panel redesign: legend chip + tab expander.
	// legendBtn ("?") opens a 220-px kid-friendly explanation
	// popover for the active tab; expanderBtn (chevron-down)
	// toggles PanelTabState.Expanded so the panel can grow/shrink
	// in place without leaving the active tab.
	legendBtn   *Button
	expanderBtn *Button
	// activeTab is the tab the parent says is currently selected. Parent must
	// call SetActiveTab before Layout for the active pill to render correctly;
	// Draw also re-syncs it.
	activeTab    PanelTab
	hitAreas     []HitArea
	parentZIndex int
}

// NewAudioStickyBar builds the tab-switcher row. parentZIndex is the owning
// zone's z-index; the bar's own hit areas register at parentZIndex+1 so they
// sit above the panel body. The onTab callback receives the tab the user
// picked; the bar itself does not own the active-tab state — the parent passes
// the active tab to Draw so the bar can render the correct active pill.
func NewAudioStickyBar(parentZIndex int, onChannel func(), onTab func(PanelTab)) *AudioStickyBar {
	b := &AudioStickyBar{parentZIndex: parentZIndex}
	b.channelBtn = NewButton(i18n.T(i18n.KeyMaster), InstButtonStyle, onChannel)
	tabs := AllPanelTabs()
	b.tabBtns = make([]*Button, len(tabs))
	for i, tab := range tabs {
		t := tab // capture
		b.tabBtns[i] = NewButton(PanelTabLabelForProfile(t), InstButtonStyle, func() {
			if onTab != nil {
				onTab(t)
			}
		})
	}

	// Phase 5: kid-friendly legend chip + tab expander chevron. Both
	// are always visible (every tab benefits) and sit at the left end
	// of the right-aligned cluster. Their OnClick handlers are bound
	// by the parent zone (legend toggles popover state; expander calls
	// PanelTabState.ToggleExpanded).
	b.legendBtn = NewButton("?", InstButtonStyle, nil)
	b.legendBtn.TextColor = colTextSecondary
	b.expanderBtn = NewButton("", InstButtonStyle, nil)
	b.expanderBtn.Icon = string(IconChevronDown)
	b.expanderBtn.IconColor = colTextSecondary
	return b
}

// formatSlopeLabel renders a dB/oct slope value as a pill label
// (e.g. "0 dB/oct", "3 dB/oct", "4.5 dB/oct"). The unit is spelled out in
// full — the previous "/o" abbreviation read as a truncated label. The pill
// auto-sizes from TextWidth(label)+padding in Layout, so the full unit always
// fits. Used by spectrumControls (audio_tab_controls.go).
func formatSlopeLabel(v float64) string {
	if v == 0 {
		return "0 dB/oct"
	}
	if v == float64(int(v)) {
		return fmt.Sprintf("%d dB/oct", int(v))
	}
	return fmt.Sprintf("%.1f dB/oct", v)
}

// Layout positions every button inside rect. Channel goes on the left; the
// legend/expander cluster and the tab strip pack right-to-left. On mobile the
// tab strip is suppressed — the bottom-bar segmented switcher owns it.
func (b *AudioStickyBar) Layout(rect image.Rectangle) {
	b.rect = rect
	// Pill sizing comes from the audio-chrome density tokens so the whole tab
	// strip is re-styleable from DESIGN.md (densities.audioPill*).
	d := Profile().DensityValues()
	// Buttons fill the strip with a small margin (bar height minus centering
	// inset), clamped to the density pill height as a floor.
	btnH := rect.Dy() - 8
	if btnH < d.AudioPillH {
		btnH = d.AudioPillH
	}
	y := rect.Min.Y + 4

	// Channel pill (left).
	chW := TextWidth(b.channelBtn.Text) + d.AudioPillPadX
	if chW < d.AudioChannelMinW {
		chW = d.AudioChannelMinW
	}
	if chW > rect.Dx()/3 && rect.Dx()/3 > 0 {
		chW = rect.Dx() / 3
	}
	b.channelBtn.SetRect(image.Rect(rect.Min.X+SpaceSM, y, rect.Min.X+6+chW, y+btnH))

	// Right-aligned cluster: legend + expander, then the tab strip (desktop).
	rightEdge := rect.Max.X - SpaceSM

	// Phase 5: legend chip + tab expander. Hidden on mobile so the
	// chrome row stays narrow (the bottom-nav strip on mobile takes
	// the discoverability role the legend chip plays on desktop; the
	// expander chevron is desktop-only because mobile uses the full-
	// height bottom-sheet pattern, not a collapsible panel).
	mobileChromeSquish := Profile().IsMobile()
	if b.expanderBtn != nil {
		if mobileChromeSquish {
			b.expanderBtn.SetRect(image.Rectangle{})
		} else {
			exW := d.AudioPillNarrowW
			b.expanderBtn.SetRect(image.Rect(rightEdge-exW, y, rightEdge, y+btnH))
			rightEdge -= exW + d.AudioPillGap
		}
	}
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
		// Tabs fill the span between the channel pill (left) and the already-
		// placed right cluster (legend/expander). They are laid out
		// right-to-left; when the natural widths would march past the channel
		// pill's right edge they shrink uniformly to fit, and every tab is
		// clamped so it can never overlap the channel pill (the garbled
		// "WaMaSspectrum" overlap at narrow desktop widths — screenshot review).
		n := len(b.tabBtns)
		leftBound := b.channelBtn.Rect().Max.X + tabGap
		avail := rightEdge - leftBound
		naturalW := make([]int, n)
		naturalTotal := 0
		for i := 0; i < n; i++ {
			tw := TextWidth(b.tabBtns[i].Text) + d.AudioPillPadX
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
				// No room left at all — collapse the remaining (leftmost) tabs
				// rather than draw them over the channel pill.
				b.tabBtns[i].SetRect(image.Rectangle{})
				continue
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
	// The Master/channel pill is the primary chrome control; on mobile give it
	// a touch-min hit target via Touch+ClipRect (the visible pill stays inside
	// the bar; the larger tap area doesn't disturb sibling layout).
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
		addBtn(tb, fmt.Sprintf("eq-tab-%d", i))
	}
	addBtn(b.legendBtn, "eq-legend-btn")
	addBtn(b.expanderBtn, "eq-expander-btn")
}

// Draw renders the entire tab-switcher row. activeTab tells the bar which tab
// pill should be highlighted as active; the channel pill is always rendered
// in its active state (it's a persistent selector, not a tab).
func (b *AudioStickyBar) Draw(dst *ebiten.Image, activeTab PanelTab) {
	// Sync activeTab so the next Layout pass renders the right pills.
	// (Most call sites already call SetActiveTab before Layout; this
	// catches direct-Draw callers.)
	b.activeTab = activeTab
	drawPillTabAt(dst, b.channelBtn, true)
	// Chevron-down glyph at the right edge of the channel pill so it
	// reads as a dropdown selector instead of a passive label. Phase 1.
	if r := b.channelBtn.Rect(); !r.Empty() {
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
	if b.expanderBtn != nil {
		drawPillTabAt(dst, b.expanderBtn, false)
	}
}

// LegendBtn returns the "?" legend-chip pill (visible on every tab).
// Parent zone binds OnClick to toggle the legend popover state.
func (b *AudioStickyBar) LegendBtn() *Button { return b.legendBtn }

// ExpanderBtn returns the chevron-down panel-expander pill (visible
// on every tab). Parent zone binds OnClick to call
// PanelTabState.ToggleExpanded so the panel grows/shrinks in place.
func (b *AudioStickyBar) ExpanderBtn() *Button { return b.expanderBtn }

// SetActiveTab tells the bar which tab is currently active so the next
// Layout/Draw call renders the correct active pill. Parent zone calls this
// before Layout; Draw also syncs it so direct-Draw callers (tests) work.
func (b *AudioStickyBar) SetActiveTab(tab PanelTab) {
	b.activeTab = tab
}

// HitAreas returns the cached chrome hit areas. Call Layout first.
func (b *AudioStickyBar) HitAreas() []HitArea { return b.hitAreas }

// Rect returns the chrome strip's screen-space rectangle as set by the last
// Layout call. Exposed so the screenshot harness (SubjectAudioStickyBar)
// can crop to the bar without reaching into its private fields.
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
	// Icon-only pills (e.g. expander) draw the glyph centered; otherwise draw
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
