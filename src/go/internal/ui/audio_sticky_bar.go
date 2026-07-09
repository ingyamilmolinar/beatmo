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
	// Phase 5 audio-panel redesign: legend chip.
	// legendBtn ("?") opens a 220-px kid-friendly explanation
	// popover for the active tab.
	legendBtn *Button
	// activeTab is the tab the parent says is currently selected. Parent must
	// call SetActiveTab before Layout for the active pill to render correctly;
	// Draw also re-syncs it.
	activeTab    PanelTab
	hitAreas     []HitArea
	parentZIndex int
	// synthTabIdx caches the index of the Synth pill within tabBtns (computed
	// once at construction) so SetSynthDisabled — called every frame — never
	// re-allocates via AllPanelTabs(). -1 when there is no Synth tab.
	synthTabIdx int
	// samplerTabIdx mirrors synthTabIdx for the Sampler pill, so
	// SetSamplerDisabled is likewise allocation-free. -1 when absent.
	samplerTabIdx int
}

// NewAudioStickyBar builds the tab-switcher row. parentZIndex is the owning
// zone's z-index; the bar's own hit areas register at parentZIndex+1 so they
// sit above the panel body. The onTab callback receives the tab the user
// picked; the bar itself does not own the active-tab state — the parent passes
// the active tab to Draw so the bar can render the correct active pill.
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

	// Phase 5: kid-friendly legend chip. Always visible (every tab
	// benefits) and sits at the left end of the right-aligned cluster.
	// Its OnClick handler is bound by the parent zone (toggles the
	// popover state).
	b.legendBtn = NewButton("?", InstButtonStyle, nil)
	b.legendBtn.TextColor = colTextSecondary
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

	// Right-aligned cluster: legend, then the tab strip (desktop).
	rightEdge := rect.Max.X - SpaceSM

	// Phase 5: legend chip. Hidden on mobile so the chrome row stays
	// narrow (the bottom-nav strip on mobile takes the discoverability
	// role the legend chip plays on desktop).
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
}

// Draw renders the entire tab-switcher row. activeTab tells the bar which tab
// pill should be highlighted as active; the channel pill is always rendered
// in its active state (it's a persistent selector, not a tab).
func (b *AudioStickyBar) Draw(dst *ebiten.Image, activeTab PanelTab) {
	// Sync activeTab so the next Layout pass renders the right pills.
	// (Most call sites already call SetActiveTab before Layout; this
	// catches direct-Draw callers.)
	b.activeTab = activeTab
	// Channel SELECTOR pill: neutral cap + accent-tinted border + accent
	// chevron — the button-dropdown affordance ("this opens a list"), NOT
	// the latched-amber tab treatment. The pill is identity, not state:
	// rendering it lit-amber made every screen read as having two active
	// tabs and diluted the "this is on" signal (D2, 2026-07-04 design
	// pass; mobile Chain showed five gold actives at once).
	drawPillTabAt(dst, b.channelBtn, false)
	if r := b.channelBtn.Rect(); !r.Empty() {
		capR := keycapCapRect(r, b.channelBtn.pressTravelPx(), false)
		drawRoundedRect(dst, capR, WithAlpha(colAccent, genAlphaAccentTint), RadiusMD/2, false)
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
}

// LegendBtn returns the "?" legend-chip pill (visible on every tab).
// Parent zone binds OnClick to toggle the legend popover state.
func (b *AudioStickyBar) LegendBtn() *Button { return b.legendBtn }

// SetActiveTab tells the bar which tab is currently active so the next
// Layout/Draw call renders the correct active pill. Parent zone calls this
// before Layout; Draw also syncs it so direct-Draw callers (tests) work.
func (b *AudioStickyBar) SetActiveTab(tab PanelTab) {
	b.activeTab = tab
}

// SetSynthDisabled greys/disables the Synth tab pill (or re-enables it). The
// parent zone calls this every frame (before Layout and Draw) with
// !activeInstrumentHasSynth() so the pill tracks the active instrument. The
// disabled flag lives on the pill Button, so the shared buttonHitAdapter
// already swallows clicks on it — no separate hit-area bookkeeping needed.
func (b *AudioStickyBar) SetSynthDisabled(disabled bool) {
	if pill := b.synthTabPill(); pill != nil {
		pill.Disabled = disabled
	}
}

// SetSamplerDisabled greys/disables the Sampler tab pill (or re-enables it).
// Mirrors SetSynthDisabled: the parent zone calls it every frame so the pill
// tracks the active channel — the Sampler tab is meaningless on the master
// bus (there is no single instrument sample to chop). The Disabled flag lives
// on the pill Button, so the shared buttonHitAdapter swallows clicks on it.
func (b *AudioStickyBar) SetSamplerDisabled(disabled bool) {
	if pill := b.samplerTabPill(); pill != nil {
		pill.Disabled = disabled
	}
}

// synthTabPill returns the tab pill that corresponds to TabSynth, or nil.
// Uses the cached index so it never allocates on the per-frame path.
func (b *AudioStickyBar) synthTabPill() *Button {
	return b.TabBtn(b.synthTabIdx)
}

// samplerTabPill returns the tab pill that corresponds to TabSampler, or nil.
func (b *AudioStickyBar) samplerTabPill() *Button {
	return b.TabBtn(b.samplerTabIdx)
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

// Pre-boxed pill palette. The theme colors (colSurface2/colAccent/…) are
// concrete color.RGBA set once at package init; assigning them to a
// color.Color interface local boxes a fresh copy on every call. drawPillTabAt
// runs ~9× per audio-panel frame, so boxing those five palette values per pill
// was the dominant per-frame allocation on the Levels tab (it, not the
// DrawImage cost, is what TestEQPanelDrawAllocDiscipline measures under the
// ebitenstub backend). Boxing once at init keeps the hot path allocation-free.
// The shell colors are the wall shade drawKeycapShell derives via
// adjustColor(fill, -45) — pre-boxed here so the pill path never re-derives it.
var (
	pillCapFillInactive color.Color = colSurface2
	pillCapBorderInact  color.Color = colButtonBorder
	pillTextInactive    color.Color = colTextSecondary
	// Active pill = lit lamp-amber face (#FFB30A), pressed-IN (recessed), with
	// dark text — matte retro-analogue restyle (2026-06-17). Replaces the
	// lit-cyan raised face + cyan engage flash.
	pillCapFillActive   color.Color = genColorSunsetGold300
	pillCapBorderActive color.Color = genColorSunsetGold300
	pillTextActive      color.Color = genColorBackground
	pillShellInactive   color.Color = adjustColor(colSurface2, -45)
	pillShellActive     color.Color = adjustColor(genColorSunsetGold300, -45)
	// Disabled pill: recessed/dim cap + greyed label so it reads as inert
	// (mirrors the subdiv button's DisabledButtonStyle treatment). Pre-boxed
	// once like the rest of the palette to keep drawPillTabAt allocation-free.
	pillCapFillDisabled color.Color = adjustColor(colSurface2, -20)
	pillCapBorderDisab  color.Color = adjustColor(colButtonBorder, -20)
	pillTextDisabled    color.Color = colTextDisabled
	pillShellDisabled   color.Color = adjustColor(colSurface2, -55)
)

// drawPillTabAt is the file-scope free-function version of the legacy
// EQPanelZone.drawPillTab method. Copied here so AudioStickyBar can render
// pills without holding a receiver reference to the zone.
// It applies the keycap look: dark socket/wall + a travelling cap that raises
// when active. A one-shot engage flash fires on the first off→on transition.
func drawPillTabAt(dst *ebiten.Image, btn *Button, active bool) {
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

	pillRadius := RadiusMD / 2 // 4px corners
	// Active pill sits recessed (pressed-IN), never raised — raised=false.
	capR := keycapCapRect(r, btn.pressTravelPx(), false)

	// Active = lit amber face (recessed); inactive = neutral surface cap. All
	// palette values are the pre-boxed package-level color.Color interfaces
	// (see the var block above) so selecting a state copies an interface header
	// rather than boxing a fresh color.RGBA on every pill, every frame.
	capFill, borderCol, textCol, shellCol := pillCapFillInactive, pillCapBorderInact, pillTextInactive, pillShellInactive
	switch {
	case btn.Disabled:
		capFill, borderCol, textCol, shellCol = pillCapFillDisabled, pillCapBorderDisab, pillTextDisabled, pillShellDisabled
	case active:
		// Dark legend on the lit-cyan face: matches button-primary textColor
		// (DESIGN.md: textColor: "{colors.background}") — genColorBackground.
		capFill, borderCol, textCol, shellCol = pillCapFillActive, pillCapBorderActive, pillTextActive, pillShellActive
	}
	// Shell (dark socket/side-wall) as a cached fill-only composite — the rect
	// is static so the (w,h,fill,border,radius) sprite stays warm. Mirrors
	// drawKeycapShell but with the pre-derived wall shade (no per-call
	// adjustColor box). Guards match drawKeycapShell.
	if !r.Empty() && genGeomKeycapWallDepth != 0 {
		drawRoundedButton(dst, r, shellCol, shellCol, pillRadius, false)
	}
	if btn.pressDepth <= 0 {
		drawKeycapContactShadow(dst, capR, pillRadius)
	}
	// Cap face fill+border as a single cached composite sprite (keyed by
	// w,h,fill,border,radius). capR's size is travel-invariant (only Y moves),
	// so the sprite stays warm across press travel — steady-state cost is one
	// allocation-free blit instead of two uncached drawRoundedRect primitives.
	drawRoundedButton(dst, capR, capFill, borderCol, pillRadius, false)
	if !btn.Disabled {
		if active {
			// Latched amber pill reads pressed-IN (inverted bevel).
			drawKeycapActiveInset(dst, capR, pillRadius)
		} else {
			drawKeycapBevel(dst, capR, pillRadius)
		}
	}
	// No-op since the matte restyle; engageAnim is still ticked above so the
	// pill press-spring contract (TestPillKeycapEngageAndTravel) is unchanged.
	drawKeycapEngageFlash(dst, capR, pillRadius, btn.engageAnim)

	// Icon-only pills (e.g. expander) draw the glyph centered; otherwise draw
	// the button's text label, color-coded by active/inactive state.
	if btn.Icon != "" {
		iconCol := btn.IconColor
		if iconCol == nil {
			iconCol = textCol
		}
		DrawIcon(dst, IconID(btn.Icon), capR, iconCol)
		return
	}
	if btn.TextColor != nil {
		textCol = btn.TextColor
	}
	tw := TextWidth(btn.Text)
	th := TextHeight()
	tx := capR.Min.X + (capR.Dx()-tw)/2
	ty := capR.Min.Y + (capR.Dy()-th)/2
	DrawTextColorAt(dst, btn.Text, tx, ty, textCol)
}
