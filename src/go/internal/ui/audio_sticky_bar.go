package ui

import (
	"image"
	"image/color"
	"strconv"

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
	// trailingBtns are per-tab action buttons the parent hands the bar each
	// frame (mobile only) — e.g. the Synth/Sampler [Preview][Save][⋯] cluster,
	// which lives on the always-present chrome row instead of a separate
	// in-panel action row. Owned/wired by the panels; the bar only lays them
	// out (right cluster) and registers their hit areas. Empty on desktop,
	// where the right cluster is the tab strip.
	trailingBtns []*Button
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
				// No room left at all — collapse the remaining (leftmost) tabs
				// rather than draw them over the channel pill.
				b.tabBtns[i].SetRect(image.Rectangle{})
				continue
			}
			b.tabBtns[i].SetRect(image.Rect(rightEdge-tw, y, rightEdge, y+btnH))
			rightEdge -= tw + tabGap
		}
	}

	// Trailing per-tab action buttons (mobile only — the desktop right cluster
	// is the tab strip). Cleared first so a stale rect never survives a
	// desktop frame or a collapse, then packed right-to-left from the current
	// rightEdge, hard-clamped left of the channel pill.
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
				// No room without crossing the channel pill — collapse this
				// (and, being leftmost-first, the remaining) button.
				btn.SetRect(image.Rectangle{})
				continue
			}
			btn.SetRect(image.Rect(rightEdge-w, y, rightEdge, y+btnH))
			rightEdge -= w + d.AudioPillGap
		}
	}
	b.rebuildHitAreas()
}

// SetTrailingActions supplies the per-tab action buttons the bar lays into its
// right cluster on mobile (Synth/Sampler [Preview][Save][⋯]). The parent calls
// this before Layout, mirroring SetActiveTab; pass nil to clear. The buttons
// keep their own OnClick behavior — the bar only positions them and publishes
// their hit areas.
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
		addBtn(tb, indexedTag(eqTabTags[:], "eq-tab-", i))
	}
	addBtn(b.legendBtn, "eq-legend-btn")
	// Trailing per-tab action buttons: register with a touch-min expansion on
	// mobile (same discipline as the channel pill) so the ~32px pills meet the
	// 44px finger target without physically enlarging their visible rect.
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
		// No accent outline: flat-tonal draws no border at rest. The caret
		// below is what signals "this opens a list".
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
	// Trailing per-tab action buttons (mobile only — empty rects on desktop are
	// skipped by their own Draw). Rendered via the buttons' own spec Draw so
	// they match the app's action-button styling, not the tab-pill treatment.
	for _, tb := range b.trailingBtns {
		if tb == nil || tb.Rect().Empty() {
			continue
		}
		tb.Draw(dst)
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
// Derived in rebuildThemeAliases (theme.go) — pre-boxed color.Color once per
// theme apply so drawPillTabAt stays allocation-free. Active = lit lamp-amber
// face (sunset-gold-300), pressed-IN, dark text; disabled = recessed/dim.
var (
	pillCapFillInactive color.Color
	pillTextInactive    color.Color
	pillCapFillActive   color.Color
	pillTextActive      color.Color
	pillCapFillDisabled color.Color
	pillTextDisabled    color.Color
)

// drawPillTabAt is the file-scope free-function version of the legacy
// EQPanelZone.drawPillTab method. Copied here so AudioStickyBar can render
// pills without holding a receiver reference to the zone.
// It applies the keycap look: dark socket/wall + a travelling cap that raises
// when active. A one-shot engage flash fires on the first off→on transition.
// drawPillTabAt is a func VAR so tests can take an exact census of the pills
// a tab renders (the drawMenuRow / drawSpectrumBarGradient precedent). The
// audio panel's control count is a thing worth guarding, not discovering.
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

	// Active = lit amber face (recessed); inactive = neutral surface cap. All
	// palette values are the pre-boxed package-level color.Color interfaces
	// (see the var block above) so selecting a state copies an interface header
	// rather than boxing a fresh color.RGBA on every pill, every frame.
	capFill, textCol := pillCapFillInactive, pillTextInactive
	switch {
	case btn.Disabled:
		capFill, textCol = pillCapFillDisabled, pillTextDisabled
	case active:
		// Bright legend on the accent-TINTED body (see below). The flat-tonal
		// restyle replaced the solid amber flood, so the old dark legend would
		// now be unreadable.
		capFill, textCol = pillCapFillInactive, colTextPrimary
	}
	// Flat-tonal body: a single filled rounded rect, no shell, no bevel, no
	// border. Selection is a tint plus a 2px accent bar — never a flood.
	drawFlatButtonBody(dst, capR, capFill, pillRadius)
	if active && !btn.Disabled {
		drawRoundedRect(dst, capR, WithAlpha(colAccent, AlphaSubtle), pillRadius, true)
		drawFlatSelectionBar(dst, capR, colAccent, selectionEdgeBottom, 1)
	}

	// Icon-only pills (e.g. expander) draw the glyph centered; otherwise draw
	// the button's text label, color-coded by active/inactive state.
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

// Hit-area tags by index, built once: the bar's hit areas are rebuilt on every
// layout pass (every frame) and formatting the tags there was a Sprintf per
// pill per frame.
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
