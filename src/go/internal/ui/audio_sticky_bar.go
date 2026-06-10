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

// stickyBarHeight is the chrome strip height: taller on mobile so the Master
// channel pill, tab pills, freeze, and close all render at a finger-friendly
// size (the 26-px desktop strip is below the touch-min).
func stickyBarHeight() int {
	if Profile().IsMobile() {
		return 36
	}
	return stickyBarH
}

// AudioStickyBar owns the chrome strip at the top of EQPanelZone: channel
// dropdown trigger, five tab pills, freeze toggle, freq-scale chip (log/lin),
// close button. The bar is fully self-contained; its constructor receives
// callbacks for each action, so the bar never reaches back into its parent
// zone.
type AudioStickyBar struct {
	rect       image.Rectangle
	channelBtn *Button
	// tabBtns is a slice rather than a fixed array so the strip can grow
	// (Phase 4 added a 6th Synth tab; future plugin recipes may add more).
	tabBtns      []*Button
	freezeBtn    *Button
	closeBtn     *Button
	freqScaleBtn *Button // log/lin chip — toggles the Spectrum tab's frequency axis
	freqScaleLog bool    // true = log axis (default), false = linear axis
	// Spectrum-tab-only controls: slope tilt (0/3/4.5 dB/oct cycle),
	// Pre|Post overlay toggle (pre-EQ + post-EQ stacked), Reset Hold
	// (clears the all-time MaxPeak watermark on the spectrum bars).
	// All three are laid out only when activeTab == TabSpectrum so they
	// don't compete for click space on other tabs.
	slopeBtn     *Button
	preBtn       *Button
	resetHoldBtn *Button
	slopeOptions []float64 // cyclic list of dB/oct values (e.g. 0, 3, 4.5)
	slopeIdx     int       // current index into slopeOptions
	preOverlay   bool      // true when Pre|Post overlay is enabled
	// Levels-tab-only controls: Clear Clips (resets the rolling 10s
	// clip window + persistent "!" markers on per-channel strips) and
	// K-20 toggle (display shifts so 0 dB sits at -20 dBFS, the Bob
	// Katz reference scale). Layout/draw gates on activeTab==TabMeters.
	clearClipsBtn *Button
	k20Btn        *Button
	k20View       bool
	// Phase 5 audio-panel redesign: legend chip + tab expander.
	// legendBtn ("?") opens a 220-px kid-friendly explanation
	// popover for the active tab; expanderBtn (chevron-down)
	// toggles PanelTabState.Expanded so the panel can grow/shrink
	// in place without leaving the active tab.
	legendBtn   *Button
	expanderBtn *Button
	// activeTab is the tab the parent says is currently selected — used by
	// Layout to decide whether to claim space for the Spectrum-only pills
	// (slope / Pre / Reset Hold). Parent must call SetActiveTab before
	// Layout for the layout to be correct; Draw also re-syncs it.
	activeTab    PanelTab
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
	// Freeze indicator uses single-character text per DESIGN.md §5d
	// (permitted text-glyph exception): "||" when capturing, ">" when frozen.
	b.freezeBtn = NewButton("||", InstButtonStyle, onFreeze)
	b.freezeBtn.TextColor = colTextSecondary
	// Frequency-scale chip: toggles between log (default, 10 ISO bands) and
	// linear (10 equal-Hz bands across [20, 22000]) on the Spectrum tab.
	// Per-session sticky — no persistence across launches.
	b.freqScaleLog = true
	b.freqScaleBtn = NewButton("log", InstButtonStyle, nil)
	b.freqScaleBtn.OnClick = func() {
		b.freqScaleLog = !b.freqScaleLog
		if b.freqScaleLog {
			b.freqScaleBtn.Text = "log"
		} else {
			b.freqScaleBtn.Text = "lin"
		}
	}
	b.freqScaleBtn.TextColor = colTextSecondary

	// Spectrum-tab pills: slope cycle, Pre|Post overlay toggle, Reset
	// Hold (clear all-time peak watermark). Initial state: slope=0
	// dB/oct (raw magnitude), preOverlay=false. The Reset Hold pill's
	// onClick handler is bound by the parent zone (EQPanelZone owns
	// SpectrumPeakState); the bar exposes SlopeDBPerOct() and
	// PreOverlay() so the renderer can read current state.
	b.slopeOptions = []float64{0, 3, 4.5}
	b.slopeIdx = 0
	b.slopeBtn = NewButton("0dB/o", InstButtonStyle, nil)
	b.slopeBtn.OnClick = func() {
		b.slopeIdx = (b.slopeIdx + 1) % len(b.slopeOptions)
		v := b.slopeOptions[b.slopeIdx]
		b.slopeBtn.Text = formatSlopeLabel(v)
		SetSpectrumSlope(v)
	}
	b.slopeBtn.TextColor = colTextSecondary
	b.preBtn = NewButton("Pre", InstButtonStyle, nil)
	b.preBtn.OnClick = func() {
		b.preOverlay = !b.preOverlay
	}
	b.preBtn.TextColor = colTextSecondary
	// resetHoldBtn calls back to the parent through OnClick (bound by
	// EQPanelZone after construction). The label is a single character
	// per DESIGN.md §5d permitted-text-glyph exception list.
	b.resetHoldBtn = NewButton("R", InstButtonStyle, nil)
	b.resetHoldBtn.TextColor = colTextSecondary

	// Levels-tab pills: Clear Clips ("CLR") + K-20 toggle ("K20").
	// Like the spectrum-only pills above, their OnClick handlers may
	// be re-bound by the parent zone after construction; the bar
	// reads only the K-20 toggle state via K20View().
	b.clearClipsBtn = NewButton("CLR", InstButtonStyle, nil)
	b.clearClipsBtn.TextColor = colTextSecondary
	b.k20Btn = NewButton("K20", InstButtonStyle, nil)
	b.k20Btn.OnClick = func() {
		b.k20View = !b.k20View
	}
	b.k20Btn.TextColor = colTextSecondary

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

	// Close button: drawn as a glyph via Button.Icon; the empty Text keeps
	// the pill compact while still routing through the standard hit-area
	// adapter for click delivery.
	b.closeBtn = NewButton("", InstButtonStyle, onClose)
	b.closeBtn.Icon = string(IconClose)
	b.closeBtn.IconColor = colTextSecondary
	return b
}

// formatSlopeLabel renders a dB/oct slope value as a compact pill label
// (e.g. "0dB/o", "3dB/o", "4.5dB/o"). The trailing "/o" makes the unit
// explicit so kids understand the chip describes octaves, not flat dB.
func formatSlopeLabel(v float64) string {
	if v == 0 {
		return "0dB/o"
	}
	if v == float64(int(v)) {
		return fmt.Sprintf("%ddB/o", int(v))
	}
	return fmt.Sprintf("%.1fdB/o", v)
}

// Layout positions every button inside rect. Channel goes on the left;
// close, freeze, and the tab strip pack right-to-left. On mobile the tab
// strip is suppressed — the bottom-bar segmented switcher owns it.
func (b *AudioStickyBar) Layout(rect image.Rectangle) {
	b.rect = rect
	// Buttons fill the strip with a small margin: 18 px on the 26-px desktop
	// bar, ~36 px on the 44-px mobile bar (finger-friendly).
	btnH := rect.Dy() - 8
	if btnH < 18 {
		btnH = 18
	}
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

	// Freeze button: only meaningful on tabs that capture analyzer
	// state (Wave / Spectrum / Levels / Chain). Hidden on the EQ tab
	// (no live capture) and the Synth tab (per-row recipe editor —
	// nothing to freeze). Mobile-usability pass: removing it on
	// Synth/EQ buys ~27 px back for the active-control chrome.
	freezeWantsButton := b.activeTab == TabWave || b.activeTab == TabSpectrum ||
		b.activeTab == TabMeters || b.activeTab == TabScope
	if freezeWantsButton {
		freezeW := 24
		b.freezeBtn.SetRect(image.Rect(rightEdge-freezeW, y, rightEdge, y+btnH))
		rightEdge -= freezeW + 3
	} else {
		b.freezeBtn.SetRect(image.Rectangle{})
	}

	// Frequency-scale chip (28 px, "log" / "lin"). Visible only on the
	// Spectrum tab where it has a function (toggles log vs linear
	// frequency axis). Mobile-usability pass: showing it on every tab
	// wasted ~30 px of horizontal chrome on a 360-px portrait phone
	// where the Synth tab needs every pixel for the knob grid.
	if b.freqScaleBtn != nil {
		if b.activeTab == TabSpectrum {
			freqW := 28
			b.freqScaleBtn.SetRect(image.Rect(rightEdge-freqW, y, rightEdge, y+btnH))
			rightEdge -= freqW + 3
		} else {
			b.freqScaleBtn.SetRect(image.Rectangle{})
		}
	}

	// Spectrum-only pills: slope cycle, Pre|Post toggle, Reset Hold.
	// Laid out (and hit-tested) only when the parent is on TabSpectrum
	// so they don't crowd other tabs. They sit immediately to the left
	// of the freq-scale chip so all spectrum chrome clusters together.
	if b.activeTab == TabSpectrum && !Profile().IsMobile() {
		if b.resetHoldBtn != nil {
			rstW := 20
			b.resetHoldBtn.SetRect(image.Rect(rightEdge-rstW, y, rightEdge, y+btnH))
			rightEdge -= rstW + 3
		}
		if b.preBtn != nil {
			preW := 28
			b.preBtn.SetRect(image.Rect(rightEdge-preW, y, rightEdge, y+btnH))
			rightEdge -= preW + 3
		}
		if b.slopeBtn != nil {
			b.slopeBtn.Text = formatSlopeLabel(b.slopeOptions[b.slopeIdx])
			slpW := TextWidth(b.slopeBtn.Text) + 10
			if slpW < 44 {
				slpW = 44
			}
			b.slopeBtn.SetRect(image.Rect(rightEdge-slpW, y, rightEdge, y+btnH))
			rightEdge -= slpW + 3
		}
	} else {
		// Off-tab: collapse rects so they're never hit-testable.
		if b.slopeBtn != nil {
			b.slopeBtn.SetRect(image.Rectangle{})
		}
		if b.preBtn != nil {
			b.preBtn.SetRect(image.Rectangle{})
		}
		if b.resetHoldBtn != nil {
			b.resetHoldBtn.SetRect(image.Rectangle{})
		}
	}

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
			exW := 18
			b.expanderBtn.SetRect(image.Rect(rightEdge-exW, y, rightEdge, y+btnH))
			rightEdge -= exW + 3
		}
	}
	if b.legendBtn != nil {
		if mobileChromeSquish {
			b.legendBtn.SetRect(image.Rectangle{})
		} else {
			lW := 18
			b.legendBtn.SetRect(image.Rect(rightEdge-lW, y, rightEdge, y+btnH))
			rightEdge -= lW + 3
		}
	}

	// Levels-tab pills: Clear Clips + K-20 toggle.
	if b.activeTab == TabMeters && !Profile().IsMobile() {
		if b.clearClipsBtn != nil {
			clrW := 28
			b.clearClipsBtn.SetRect(image.Rect(rightEdge-clrW, y, rightEdge, y+btnH))
			rightEdge -= clrW + 3
		}
		if b.k20Btn != nil {
			k20W := 28
			b.k20Btn.SetRect(image.Rect(rightEdge-k20W, y, rightEdge, y+btnH))
			rightEdge -= k20W + 3
		}
	} else {
		if b.clearClipsBtn != nil {
			b.clearClipsBtn.SetRect(image.Rectangle{})
		}
		if b.k20Btn != nil {
			b.k20Btn.SetRect(image.Rectangle{})
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
	addBtn(b.freezeBtn, "eq-freeze-btn")
	addBtn(b.freqScaleBtn, "eq-freqscale-btn")
	addBtn(b.slopeBtn, "eq-slope-btn")
	addBtn(b.preBtn, "eq-pre-btn")
	addBtn(b.resetHoldBtn, "eq-reset-hold-btn")
	addBtn(b.clearClipsBtn, "eq-clear-clips-btn")
	addBtn(b.k20Btn, "eq-k20-btn")
	addBtn(b.legendBtn, "eq-legend-btn")
	addBtn(b.expanderBtn, "eq-expander-btn")
	addBtn(b.closeBtn, "eq-close-btn")
}

// Draw renders the entire chrome strip. activeTab tells the bar which tab
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
	// Freeze pill: parent owns the text/color update (so it reflects the
	// analyzer state); the bar just draws whatever the pill currently shows.
	frozen := b.freezeBtn != nil && b.freezeBtn.Text == ">"
	drawPillTabAt(dst, b.freezeBtn, frozen)
	// Freq-scale chip: drawn "active" when in linear mode so the chip stands
	// out from the default log layout. Click toggles between log <-> lin.
	if b.freqScaleBtn != nil {
		drawPillTabAt(dst, b.freqScaleBtn, !b.freqScaleLog)
	}
	// Spectrum-only pills: drawn only when active (Layout already
	// collapsed their rects off-tab so drawPillTabAt would no-op).
	if b.activeTab == TabSpectrum {
		if b.slopeBtn != nil {
			drawPillTabAt(dst, b.slopeBtn, b.slopeIdx != 0)
		}
		if b.preBtn != nil {
			drawPillTabAt(dst, b.preBtn, b.preOverlay)
		}
		if b.resetHoldBtn != nil {
			drawPillTabAt(dst, b.resetHoldBtn, false)
		}
	}
	if b.activeTab == TabMeters {
		if b.clearClipsBtn != nil {
			drawPillTabAt(dst, b.clearClipsBtn, false)
		}
		if b.k20Btn != nil {
			drawPillTabAt(dst, b.k20Btn, b.k20View)
		}
	}
	if b.legendBtn != nil {
		drawPillTabAt(dst, b.legendBtn, false)
	}
	if b.expanderBtn != nil {
		drawPillTabAt(dst, b.expanderBtn, false)
	}
	drawPillTabAt(dst, b.closeBtn, false)
}

// LegendBtn returns the "?" legend-chip pill (visible on every tab).
// Parent zone binds OnClick to toggle the legend popover state.
func (b *AudioStickyBar) LegendBtn() *Button { return b.legendBtn }

// ExpanderBtn returns the chevron-down panel-expander pill (visible
// on every tab). Parent zone binds OnClick to call
// PanelTabState.ToggleExpanded so the panel grows/shrinks in place.
func (b *AudioStickyBar) ExpanderBtn() *Button { return b.expanderBtn }

// SetActiveTab tells the bar which tab is currently active so the next
// Layout call can decide whether to claim space for the Spectrum-only
// pills (slope / Pre / Reset Hold). Parent zone calls this before
// Layout; Draw also syncs it so direct-Draw callers (tests) work.
func (b *AudioStickyBar) SetActiveTab(tab PanelTab) {
	b.activeTab = tab
}

// SlopeDBPerOct returns the currently-selected spectrum slope tilt in
// dB/octave. 0 = raw magnitude, 3 = broadcast (pink-noise flat), 4.5 =
// FabFilter-style treble bias. The renderer reads this on every Draw.
func (b *AudioStickyBar) SlopeDBPerOct() float64 {
	if b == nil || len(b.slopeOptions) == 0 {
		return 0
	}
	return b.slopeOptions[b.slopeIdx]
}

// PreOverlay reports whether the Pre|Post overlay is enabled — the
// renderer paints the pre-EQ FFT trace under the post-EQ bars at
// reduced alpha so the user can see what the EQ is shaping.
func (b *AudioStickyBar) PreOverlay() bool {
	if b == nil {
		return false
	}
	return b.preOverlay
}

// SlopeBtn returns the slope-cycle pill (Spectrum tab only).
func (b *AudioStickyBar) SlopeBtn() *Button { return b.slopeBtn }

// PreBtn returns the Pre|Post overlay toggle pill (Spectrum tab only).
func (b *AudioStickyBar) PreBtn() *Button { return b.preBtn }

// ResetHoldBtn returns the Reset Hold pill (Spectrum tab only). The
// parent zone is expected to set its OnClick to clear the spectrum's
// MaxPeak watermark — see (*EQPanelZone).initButtons.
func (b *AudioStickyBar) ResetHoldBtn() *Button { return b.resetHoldBtn }

// ClearClipsBtn returns the Clear Clips pill (Levels tab only). The
// parent zone's initButtons binds its OnClick to clear the rolling
// 10-second clip window + per-channel latch state.
func (b *AudioStickyBar) ClearClipsBtn() *Button { return b.clearClipsBtn }

// K20Btn returns the K-20 view toggle pill (Levels tab only).
func (b *AudioStickyBar) K20Btn() *Button { return b.k20Btn }

// K20View reports whether the K-20 (Bob Katz) reference scale is
// active — when true the Levels renderer offsets the meter scale so
// 0 dB sits at -20 dBFS.
func (b *AudioStickyBar) K20View() bool { return b.k20View }

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

// FreezeBtn returns the freeze/resume toggle pill.
func (b *AudioStickyBar) FreezeBtn() *Button { return b.freezeBtn }

// CloseBtn returns the close-the-panel chip.
func (b *AudioStickyBar) CloseBtn() *Button { return b.closeBtn }

// FreqScaleBtn returns the log/lin frequency-scale chip. The button toggles
// the chip's internal log/lin state on click; consumers read FreqScaleLog()
// for the current value when rendering the spectrum.
func (b *AudioStickyBar) FreqScaleBtn() *Button { return b.freqScaleBtn }

// FreqScaleLog reports whether the spectrum panel should use the default
// log axis (true, ISO 1/3-octave bands) or the linear axis (false, 10
// equal-Hz bands across [20, 22000]).
func (b *AudioStickyBar) FreqScaleLog() bool { return b.freqScaleLog }

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
