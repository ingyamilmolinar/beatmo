package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// EQCallbacks contains callbacks for the EQPanelZone to communicate with
// the DrumView and audio engine. Zones don't reference Game or each other.
type EQCallbacks struct {
	OnGainChange     func(band int, db float64)
	OnMuteToggle     func(band int)
	OnChannelChange  func(channelID string)
	OnToggleHPF      func()
	OnToggleLPF      func()
	OnApplyEQ        func()
	AnalyzerSnapshot func(ch string) audio.AnalyzerSnapshot
	ActiveRows       func() []*DrumRow

	// HPF/LPF state accessors and cutoff change callbacks.
	HPFEnabled        func() bool
	HPFCutoffHz       func() float64
	LPFEnabled        func() bool
	LPFCutoffHz       func() float64
	OnHPFCutoffChange func(hz float64)
	OnLPFCutoffChange func(hz float64)

	// OnChannelDropdownClose is called when the channel dropdown portal entry
	// is removed (click-outside, ESC, or CleanupClosed). Used to sync legacy
	// DrumView state (eqChannelOpen, eqChDeferredTap).
	OnChannelDropdownClose func()

	// DrawWaveform renders the waveform/spectrum display when waveformMode is active.
	// Kept as a fallback; the new analyzer-based renderers are preferred when
	// AnalyzerState is available.
	DrawWaveform func(dst *ebiten.Image)

	// AnalyzerState returns the latest analyzer snapshot. Used by the new
	// tab renderers (Wave, Spectrum, Meters) that consume analyzer.State.
	AnalyzerState func() *analyzer.State

	// AnalyzerMetricsOnly returns a scalar-only analyzer.State (no
	// Spectrum/Waveform/Detail). Used exclusively by the Meters tab on
	// WASM to avoid the per-frame slice allocation churn that drove the
	// long-session OOM. May be nil; callers fall back to AnalyzerState.
	AnalyzerMetricsOnly func() *analyzer.State

	// OnFreezeToggle toggles the analyzer capture freeze state and returns
	// the new frozen state.
	OnFreezeToggle func() bool

	// OnTabChange is called after the active tab changes. Now telemetry-only —
	// the Scope auto-expand path was retired (every tab defaults to tall).
	OnTabChange func(tab PanelTab)

	// OnClose is invoked when the user clicks the close pill on the sticky
	// bar. May be nil; the close button silently no-ops when unset. Owners
	// (DrumView) typically wire this to "hide the panel" / "switch back to
	// rows view" depending on platform.
	OnClose func()

	// BeatGridFrac returns fractional X positions in [0,1) where vertical
	// beat markers should be drawn on the Wave tab. Nil disables the overlay
	// (the wave still renders cleanly). Wired to DrumView.beatGridFractions
	// in drumview_ctor.go.
	BeatGridFrac func() []float64
}

// EQPanelZone implements the Zone interface for the EQ/Waveform panel.
// It owns the EQ sliders, mute buttons, channel selector, toggle button,
// HPF/LPF buttons, spectrum visualization, and EQ curve display.
type EQPanelZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  EQCallbacks
	portal     *OverlayPortal // set by tree wiring

	// UI elements
	eqMuteBtns []*Button       // per-band mute
	stickyBar  *AudioStickyBar // chrome strip: channel, tabs, freeze, close
	hpfBtn     *Button         // EQ-tab content item (sub-strip above curve)
	lpfBtn     *Button         // EQ-tab content item (sub-strip above curve)

	// Visual state
	eqBandVals []float64
	tabState   *PanelTabState

	// EQ curve state
	curveDragBand   int
	curveDragFilter string
	curveDirty      bool
	curveCache      []audio.FreqResponsePoint

	// Real-time dB label during curve handle drag
	curveDragDBText string // e.g., "+6.0 dB" or "" when not dragging
	curveDragLabelX int
	curveDragLabelY int

	// Spectrum tab peak-hold state
	spectrumPeaks SpectrumPeakState

	// Levels tab clip-latch: holds the clip readout in error color for ~1s
	// after each new clip event (see render_meters.go's LevelsLatch).
	levelsLatch LevelsLatch

	// Scope tab: delegated to its own zone for layout/draw/hit areas.
	chainZone *ChainPanelZone

	// Master channel EQ state (per-row stays in DrumRow)
	bandGainsDB []float64
	bandMuted   []bool

	// Channel dropdown state
	channelOpen   bool
	activeChannel string
	channelScroll *ScrollBehavior

	// Per-band dB text inputs
	eqDBInputs     [10]*TextInput
	dbInputFocused int     // index of focused dB input, -1 if none
	dbInputSyncing bool    // guard flag: true when sync is updating text
	dbInputPrev    float64 // saved dB before editing (for Escape revert)

	// WASM-only cached state held while the analyzer tab is frozen. On desktop
	// stays nil — the analyzer.Service owns freeze and sets state.Capture.Frozen.
	frozenAnalyzer *analyzer.State

	// Hit areas cache (rebuilt on Layout)
	hitAreas []HitArea

	// drawCalls counts every entry into Draw — used by view-mode-ownership
	// tests to verify the tree's visibility gate is short-circuiting the
	// zone in viewModeRows on mobile (no parallel state).
	drawCalls int

	// frameAnalyzerState is a per-Draw memo so the analyzer-state callback
	// runs at most once per frame. Cleared at the top of Draw().
	// frameAnalyzerTab records which builder produced the memo
	// (TabMeters routes through the lightweight metrics path; other tabs
	// use the full state). Eliminates the duplicate per-frame call the
	// freezeBtn block historically triggered.
	frameAnalyzerState *analyzer.State
	frameAnalyzerTab   PanelTab
	frameAnalyzerValid bool

	// Cursor readout state for the Wave / Spectrum tabs.
	//
	// cursorX is the cursor position in zone-local screen pixels. On desktop
	// it tracks the mouse hover position while the cursor is inside
	// contentRect(); on mobile it tracks the active touch and latches in
	// place after release until the next press lands elsewhere.
	//
	// cursorActive gates whether the crosshair / readout is drawn. It is
	// reset whenever the active tab is not Wave/Spectrum. cursorPinned only
	// matters on mobile — it keeps a stale-but-meaningful cursor visible
	// after the user lifts their finger.
	cursorX      int
	cursorActive bool
	cursorPinned bool
}

// DrawCallsForTest returns the running count of Draw invocations.
// Tests reset by reading-then-comparing across phases.
func (z *EQPanelZone) DrawCallsForTest() int { return z.drawCalls }

// NewEQPanelZone creates a new EQPanelZone with the provided callbacks.
func NewEQPanelZone(cb EQCallbacks) *EQPanelZone {
	z := &EQPanelZone{
		needLayout:     true,
		callbacks:      cb,
		tabState:       NewPanelTabState(),
		curveDragBand:  -1,
		dbInputFocused: -1,
		activeChannel:  "main",
		bandGainsDB:    make([]float64, len(eqBandDefs)),
		bandMuted:      make([]bool, len(eqBandDefs)),
		eqBandVals:     make([]float64, len(eqBandDefs)),
	}
	z.initButtons()
	z.channelScroll = newScrollBehavior()
	return z
}

func (z *EQPanelZone) initButtons() {
	onChannel := func() {
		if z.channelOpen {
			z.channelOpen = false
			if z.portal != nil {
				z.portal.Close("eq-channel-dropdown")
			}
		} else {
			z.channelOpen = true
			z.buildChannelDropdown()
		}
	}
	onFreeze := func() {
		if z.callbacks.OnFreezeToggle == nil {
			return
		}
		frozen := z.callbacks.OnFreezeToggle()
		freeze := z.stickyBar.FreezeBtn()
		if freeze == nil {
			return
		}
		if frozen {
			freeze.Text = ">"
			freeze.TextColor = colAccent
		} else {
			freeze.Text = "||"
			freeze.TextColor = colTextSecondary
		}
	}
	onClose := func() {
		if z.callbacks.OnClose != nil {
			z.callbacks.OnClose()
		}
	}
	onTab := func(tab PanelTab) {
		z.tabState.SetActiveTab(tab)
		if z.callbacks.OnTabChange != nil {
			z.callbacks.OnTabChange(tab)
		}
	}
	z.stickyBar = NewAudioStickyBar(130, onChannel, onFreeze, onClose, onTab)

	z.hpfBtn = NewButton("HP", InstButtonStyle, func() {
		if z.callbacks.OnToggleHPF != nil {
			z.callbacks.OnToggleHPF()
		}
	})

	z.lpfBtn = NewButton("LP", InstButtonStyle, func() {
		if z.callbacks.OnToggleLPF != nil {
			z.callbacks.OnToggleLPF()
		}
	})

	// Mute buttons
	z.eqMuteBtns = make([]*Button, len(eqBandDefs))
	for i := range z.eqMuteBtns {
		bandIdx := i
		z.eqMuteBtns[i] = NewButton("M", EQMuteButtonStyle, func() {
			if z.callbacks.OnMuteToggle != nil {
				z.callbacks.OnMuteToggle(bandIdx)
			}
		})
	}

	// Per-band dB text inputs
	for i := 0; i < 10; i++ {
		ti := NewTextInput(image.Rectangle{}, EQDBBoxStyle)
		ti.MaxLen = 5
		ti.InputMode = "numeric"
		ti.MobileInputID = fmt.Sprintf("eq-db-%d", i)
		ti.Accept = func(r rune) bool {
			return (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+'
		}
		ti.SetText(formatDB(0))
		z.eqDBInputs[i] = ti
	}
}

// --- Zone interface ---

func (z *EQPanelZone) ID() string { return "eq-panel" }

func (z *EQPanelZone) NeedsLayout() bool { return z.needLayout }

func (z *EQPanelZone) Invalidate() { z.needLayout = true }

func (z *EQPanelZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.needLayout = false
	z.layoutButtons()
	z.layoutMuteAndDBInputs()
	// Delegate layout to scope zone when Scope tab is active.
	if z.chainZone != nil && z.tabState.ActiveTab() == TabScope {
		z.chainZone.Layout(z.contentRect())
	}
	z.rebuildHitAreas()
}

// SetChainZone sets the scope panel zone that owns the scope tab UI.
func (z *EQPanelZone) SetChainZone(sz *ChainPanelZone) { z.chainZone = sz }

// ChainZone returns the bound scope panel zone (may be nil if not wired).
func (z *EQPanelZone) ChainZone() *ChainPanelZone { return z.chainZone }

// PanelRect returns the full EQ panel rectangle including the tab header.
func (z *EQPanelZone) PanelRect() image.Rectangle { return z.rect }

// ContentRect returns the drawable area below the tab header buttons —
// the region where the active tab's content (waveform, spectrum, scope, …)
// is rendered.
func (z *EQPanelZone) ContentRect() image.Rectangle { return z.contentRect() }

// ActiveTab returns the tab currently selected on the panel.
func (z *EQPanelZone) ActiveTab() PanelTab {
	if z.tabState == nil {
		return TabEQ
	}
	return z.tabState.ActiveTab()
}

func (z *EQPanelZone) Update() {
	// Refresh cursor readout state first — desktop reads hover, mobile is
	// driven by the scrub hit-area but still wants the tab-changed gate.
	z.updateAudioPanelCursor()

	if z.dbInputFocused < 0 {
		// Quick scan: detect if any input gained focus externally (via hit adapter).
		for i, ti := range z.eqDBInputs {
			if ti != nil && ti.Focused() {
				z.dbInputPrev = z.bandGainsDB[i]
				ti.SetText("")
				z.dbInputFocused = i
				break
			}
		}
		if z.dbInputFocused < 0 {
			return // No text input active — skip 10x TextInput.Update() calls
		}
	}
	z.updateDBInputs()
}

func (z *EQPanelZone) HitAreas() []HitArea {
	if z.chainZone != nil && z.tabState.ActiveTab() == TabScope {
		return append(z.hitAreas, z.chainZone.HitAreas()...)
	}
	return z.hitAreas
}

func (z *EQPanelZone) Draw(screen *ebiten.Image) {
	z.drawCalls++
	if z.rect.Dy() < 8 || z.rect.Dx() < 8 {
		return
	}
	// Reset per-Draw memo so subsequent reads re-fetch state. The memo
	// halves the analyzer-state callback rate on every tab (the freeze
	// button block also reads it) and unlocks the metrics-only routing
	// for TabMeters.
	z.frameAnalyzerValid = false
	z.frameAnalyzerState = nil

	drawRect(screen, z.rect, colEQBg, true)

	activeTab := z.tabState.ActiveTab()
	switch activeTab {
	case TabWave:
		cr := z.contentRect()
		if state := z.getAnalyzerStateForTab(activeTab); state != nil {
			ch, cap := z.resolveChannel(state)
			var beatGrid []float64
			if z.callbacks.BeatGridFrac != nil {
				beatGrid = z.callbacks.BeatGridFrac()
			}
			drawAnalyzerWaveform(screen, cr, ch, cap, beatGrid)
			if z.cursorActive || z.cursorPinned {
				drawWaveformCursor(screen, cr, ch, cap, z.cursorX)
			}
		} else if z.callbacks.DrawWaveform != nil {
			z.callbacks.DrawWaveform(screen)
		}
	case TabSpectrum:
		cr := z.contentRect()
		scale := freqScaleLog
		if z.stickyBar != nil && !z.stickyBar.FreqScaleLog() {
			scale = freqScaleLinear
		}
		if state := z.getAnalyzerStateForTab(activeTab); state != nil {
			ch, _ := z.resolveChannel(state)
			drawAnalyzerSpectrumWithScale(screen, cr, ch, &z.spectrumPeaks, scale)
			if z.cursorActive || z.cursorPinned {
				drawSpectrumCursor(screen, cr, ch, z.cursorX)
			}
		} else {
			drawAnalyzerSpectrumWithScale(screen, cr, nil, &z.spectrumPeaks, scale)
		}
	case TabMeters:
		// Levels tab: single-channel detail (Peak + RMS bars + readout).
		// The latch ticks once per draw — passes the current ClipCount and
		// shows the readout in error color for ~1s after each new clip.
		if state := z.getAnalyzerStateForTab(activeTab); state != nil {
			ch, _ := z.resolveChannel(state)
			if ch == nil {
				ch = &state.Master
			}
			z.levelsLatch.Update(ch.ClipCount)
			drawLevelsDetail(screen, z.contentRect(), ch, &z.levelsLatch)
		} else {
			drawLevelsDetail(screen, z.contentRect(), nil, nil)
		}
	case TabScope:
		if z.chainZone != nil {
			z.chainZone.Draw(screen)
		}
	case TabEQ:
		// Draw spectrum bars and EQ curve below buttons.
		if z.rect.Dy() >= 40 {
			z.drawSpectrumBars(screen)
		}
		z.drawEQCurve(screen)
	}

	// Sync the freeze pill's text/color from analyzer state before the
	// sticky bar renders (the bar reads the pill's current Text to decide
	// whether to render the active/inactive variant).
	if z.stickyBar != nil {
		if freeze := z.stickyBar.FreezeBtn(); freeze != nil {
			if state := z.getAnalyzerStateForTab(activeTab); state != nil && state.Capture != nil && state.Capture.Frozen {
				freeze.Text = ">"
				freeze.TextColor = colAccent
			} else {
				freeze.Text = "||"
				freeze.TextColor = colTextSecondary
			}
		}
	}

	// HPF/LPF live inside the content rect on TabEQ only — drawn before the
	// sticky bar (the bar is chrome and should always sit on top).
	if z.tabState.ActiveTab() == TabEQ {
		if z.hpfBtn != nil {
			hpfActive := z.callbacks.HPFEnabled != nil && z.callbacks.HPFEnabled()
			z.drawPillTab(screen, z.hpfBtn, hpfActive, "")
		}
		if z.lpfBtn != nil {
			lpfActive := z.callbacks.LPFEnabled != nil && z.callbacks.LPFEnabled()
			z.drawPillTab(screen, z.lpfBtn, lpfActive, "")
		}
	}

	// Sticky bar drawn last so chrome is never occluded by band overlays.
	if z.stickyBar != nil {
		z.stickyBar.Draw(screen, activeTab)
	}

	drawRect(screen, z.rect, colButtonBorder, false)
}

// drawPillTab draws a tab button with pill styling.
// active: filled with colSurface2 + colAccent border + colTextAccent text.
// inactive: colButtonBorder border + colTextSecondary text.
// prefix is prepended to the button text.
func (z *EQPanelZone) drawPillTab(dst *ebiten.Image, btn *Button, active bool, prefix string) {
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
	text := prefix + btn.Text
	tw := TextWidth(text)
	th := TextHeight()
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	textCol := colTextSecondary
	if active {
		textCol = colTextAccent
	}
	DrawTextColorAt(dst, text, tx, ty, textCol)
}

func (z *EQPanelZone) HandleKey(k ebiten.Key) InputResult {
	if z.dbInputFocused >= 0 {
		switch k {
		case ebiten.KeyEnter:
			z.commitDBText(z.dbInputFocused)
			return InputConsumed
		case ebiten.KeyEscape:
			ti := z.eqDBInputs[z.dbInputFocused]
			if ti != nil {
				ti.SetText(formatDB(z.dbInputPrev))
				ti.focused = false
			}
			z.dbInputFocused = -1
			return InputConsumed
		}
	}
	return InputIgnored
}

func (z *EQPanelZone) HandleChars(_ []rune) InputResult {
	if z.dbInputFocused >= 0 {
		return InputConsumed
	}
	return InputIgnored
}

// WaveformMode returns the current toggle state (compatibility shim).
func (z *EQPanelZone) WaveformMode() bool { return z.tabState.ActiveTab() == TabWave }

// toggleButtonLabel returns the display text for the EQ/Wave toggle button.
func (z *EQPanelZone) toggleButtonLabel() string {
	return PanelTabLabel(z.tabState.ActiveTab())
}

// SetWaveformMode sets the toggle state (compatibility shim).
func (z *EQPanelZone) SetWaveformMode(v bool) {
	if v {
		z.tabState.SetActiveTab(TabWave)
	} else {
		z.tabState.SetActiveTab(TabEQ)
	}
}

// contentRect returns the drawable area below the sticky-bar chrome strip.
// The strip occupies stickyBarH (=26) pixels at the top of z.rect.
func (z *EQPanelZone) contentRect() image.Rectangle {
	return image.Rect(z.rect.Min.X, z.rect.Min.Y+stickyBarH, z.rect.Max.X, z.rect.Max.Y)
}

// hpfLPFSubStripH is the height of the HPF/LPF button row drawn at the top
// of the EQ-tab content rect (above the EQ curve / spectrum bars).
const hpfLPFSubStripH = 22

// eqCurveRect returns the rectangle inside contentRect() that holds the EQ
// curve / spectrum visualization on TabEQ — i.e. content rect minus the
// HPF/LPF sub-strip at the top.
func (z *EQPanelZone) eqCurveRect() image.Rectangle {
	cr := z.contentRect()
	if z.tabState != nil && z.tabState.ActiveTab() == TabEQ {
		return image.Rect(cr.Min.X, cr.Min.Y+hpfLPFSubStripH, cr.Max.X, cr.Max.Y)
	}
	return cr
}

// SetPortal sets the portal reference for opening overlays.
func (z *EQPanelZone) SetPortal(p *OverlayPortal) { z.portal = p }

// SyncBandState copies the given gains and muted slices into the zone's
// working arrays and marks the curve dirty so it redraws with the new data.
// Called by setEQActiveChannel when switching instruments.
func (z *EQPanelZone) SyncBandState(gains []float64, muted []bool) {
	for i := range z.bandGainsDB {
		if i < len(gains) {
			z.bandGainsDB[i] = gains[i]
		} else {
			z.bandGainsDB[i] = 0
		}
	}
	for i := range z.bandMuted {
		if i < len(muted) {
			z.bandMuted[i] = muted[i]
		} else {
			z.bandMuted[i] = false
		}
	}
	z.curveDirty = true
	z.curveCache = nil // force full recompute — curveDirty alone may miss edge cases
	z.syncAllDBInputTexts()
}

// ActiveChannel returns the current EQ channel ID.
func (z *EQPanelZone) ActiveChannel() string {
	if z.activeChannel == "" {
		return "main"
	}
	return z.activeChannel
}

// ChannelDropdownOpen returns whether the channel dropdown is open.
func (z *EQPanelZone) ChannelDropdownOpen() bool {
	return z.channelOpen
}

// --- Layout helpers ---

func (z *EQPanelZone) layoutButtons() {
	r := z.rect

	// Sticky bar owns channel pill + tab pills + freeze + close. Its own
	// Layout() handles right-aligned tab packing and the close/freeze
	// cluster. The channel-pill width is driven by the channel button's
	// current Text (set by setEQActiveChannel when the user picks an
	// instrument), so the cluster expands cleanly without an extra hook.
	if z.stickyBar != nil {
		z.stickyBar.Layout(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+stickyBarH))
	}

	// HPF/LPF live inside the content rect on TabEQ — at the top, above the
	// EQ curve. They participate in layout regardless of active tab (so the
	// rects are stable) but are only drawn / hit-tested when TabEQ is active.
	cr := z.contentRect()
	btnH := 18
	filterBtnW := 28
	subY := cr.Min.Y + 2
	hpfX := cr.Min.X + 6
	z.hpfBtn.SetRect(image.Rect(hpfX, subY, hpfX+filterBtnW, subY+btnH))
	lpfX := hpfX + filterBtnW + 2
	z.lpfBtn.SetRect(image.Rect(lpfX, subY, lpfX+filterBtnW, subY+btnH))
}

func (z *EQPanelZone) layoutMuteAndDBInputs() {
	r := z.rect
	if r.Empty() {
		return
	}

	bandW := r.Dx() / len(eqBandDefs)
	if bandW < 1 {
		bandW = 1
	}

	muteBtnH := Profile().EQSliderH
	muteBtnW := 16
	if Profile().IsMobile() {
		muteBtnW = bandW - 4
	}
	dbInputH := Profile().EQDBInputH

	// dB inputs at the bottom, mute buttons shifted up above them.
	dbInputY := r.Max.Y - dbInputH - 1
	muteBtnY := dbInputY - muteBtnH - 1

	for i := 0; i < len(eqBandDefs); i++ {
		x0 := r.Min.X + i*bandW
		x1 := x0 + bandW
		if i == len(eqBandDefs)-1 {
			x1 = r.Max.X
		}

		if i < len(z.eqMuteBtns) && z.eqMuteBtns[i] != nil {
			muteBtnX := x0 + (x1-x0-muteBtnW)/2
			z.eqMuteBtns[i].SetRect(image.Rect(muteBtnX, muteBtnY, muteBtnX+muteBtnW, muteBtnY+muteBtnH))
		}

		if z.eqDBInputs[i] != nil {
			z.eqDBInputs[i].Rect = image.Rect(x0+1, dbInputY, x1-1, dbInputY+dbInputH)
		}
	}
}

func (z *EQPanelZone) calcChannelBtnWidth() int {
	maxPx := TextWidth("Master")
	if z.callbacks.ActiveRows != nil {
		for _, r := range z.callbacks.ActiveRows() {
			if w := TextWidth(r.Name); w > maxPx {
				maxPx = w
			}
		}
	}
	w := maxPx + SpaceXS*2 + 4
	if w < 72 {
		w = 72
	}
	return w
}

// --- Hit area construction ---

func (z *EQPanelZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]

	const zIdx = 130 // EQPanelZone z-index

	// EQ curve handle drag area (both desktop and mobile).
	if !z.rect.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    z.rect,
			ZIndex:  zIdx,
			Handler: &curveHandleHitAdapter{zone: z},
			Tag:     "eq-curve-area",
		})
	}

	// Mute buttons. Touch expansion is NOT needed: on mobile the mute buttons
	// are already wide (bandW - 4px). Expanding them would overlap with the
	// adjacent band buttons below, stealing taps due to higher z-index.
	for i, btn := range z.eqMuteBtns {
		if btn == nil {
			continue
		}
		r := btn.Rect()
		if r.Empty() {
			continue
		}
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    r,
			ZIndex:  zIdx + 1, // above sliders
			Handler: &buttonHitAdapter{btn: btn},
			Tag:     "eq-mute-" + eqCenterLabels[i],
			Touch:   false,
		})
	}

	// Sticky bar owns chrome hits (channel pill, tab pills, freeze, close).
	// The bar already z-indexes them at parentZIndex+1 to match the legacy
	// in-line build.
	if z.stickyBar != nil {
		z.hitAreas = append(z.hitAreas, z.stickyBar.HitAreas()...)
	}

	// Mobile cursor scrub: when the active tab is Wave or Spectrum, register
	// a low-z hit area covering the content rect so a finger press/drag is
	// dispatched to cursorScrubHandler. Desktop reads hover directly inside
	// updateAudioPanelCursor and needs no hit area.
	if Profile().IsMobile() {
		t := z.tabState.ActiveTab()
		if t == TabWave || t == TabSpectrum {
			if cr := z.contentRect(); !cr.Empty() {
				z.hitAreas = append(z.hitAreas, HitArea{
					Rect:    cr,
					ZIndex:  zIdx, // below chrome (zIdx+1) so the chrome wins
					Handler: &cursorScrubHandler{zone: z},
					Tag:     "eq-audio-cursor-scrub",
				})
			}
		}
	}

	// HPF/LPF are content-rect items on TabEQ only.
	if z.tabState.ActiveTab() == TabEQ {
		if hr := z.hpfBtn.Rect(); !hr.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:    hr,
				ZIndex:  zIdx + 1,
				Handler: &buttonHitAdapter{btn: z.hpfBtn},
				Tag:     "eq-hpf-btn",
			})
		}
		if lr := z.lpfBtn.Rect(); !lr.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:    lr,
				ZIndex:  zIdx + 1,
				Handler: &buttonHitAdapter{btn: z.lpfBtn},
				Tag:     "eq-lpf-btn",
			})
		}
	}

	// Per-band dB text inputs.
	for i, ti := range z.eqDBInputs {
		if ti == nil {
			continue
		}
		r := ti.Rect
		if r.Empty() {
			continue
		}
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    r,
			ZIndex:  zIdx + 2, // above mute (131) and curve (130)
			Handler: &textInputHitAdapter{ti: ti},
			Tag:     fmt.Sprintf("eq-db-%s", eqCenterLabels[i]),
		})
	}
}

// --- Hit handler adapters ---

// buttonHitAdapter wraps a Button as a HitHandler.
type buttonHitAdapter struct {
	btn *Button
}

func (h *buttonHitAdapter) OnPress(x, y int) InputResult {
	if h.btn.OnClick != nil {
		h.btn.OnClick()
	}
	return InputCaptured
}

func (h *buttonHitAdapter) OnDrag(x, y int)                     {}
func (h *buttonHitAdapter) OnRelease(x, y int)                  {}
func (h *buttonHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// sliderGroupHitAdapter wraps a SliderGroup as a HitHandler.
// Used by RowRackZone (row volume) and TransportZone (master volume).
type sliderGroupHitAdapter struct {
	group     *SliderGroup
	onRelease func()
	active    bool
}

func (h *sliderGroupHitAdapter) OnPress(x, y int) InputResult {
	result := h.group.HandleInput(x, y, true)
	if result != InputIgnored {
		h.active = true
		return InputCaptured
	}
	return InputIgnored
}

func (h *sliderGroupHitAdapter) OnDrag(x, y int) {
	if h.active {
		h.group.HandleInput(x, y, true)
	}
}

func (h *sliderGroupHitAdapter) OnRelease(x, y int) {
	if h.active {
		h.group.HandleInput(x, y, false)
		h.active = false
		if h.onRelease != nil {
			h.onRelease()
		}
	}
}

func (h *sliderGroupHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// curveHandleHitAdapter handles press/drag/release on EQ curve band handles.
// It hit-tests the band handle circles on press, then tracks drag to update
// the gain directly.
type curveHandleHitAdapter struct {
	zone *EQPanelZone
}

func (h *curveHandleHitAdapter) OnPress(x, y int) InputResult {
	z := h.zone
	r := z.rect
	if r.Empty() || z.tabState.ActiveTab() == TabWave {
		return InputIgnored
	}

	pt := image.Pt(x, y)
	if !pt.In(r) {
		return InputIgnored
	}

	hitRadius := Profile().EQHandleRadius

	// Hit-test HPF handle first (filter handles take priority).
	if z.callbacks.HPFEnabled != nil && z.callbacks.HPFEnabled() {
		hpfHz := 20.0
		if z.callbacks.HPFCutoffHz != nil {
			hpfHz = z.callbacks.HPFCutoffHz()
		}
		hx := freqToX(hpfHz, r)
		hy := z.curveYAtX(hx)
		dx := x - hx
		dy := y - hy
		if dx*dx+dy*dy <= hitRadius*hitRadius {
			z.curveDragFilter = "hpf"
			return InputCaptured
		}
	}

	// Hit-test LPF handle.
	if z.callbacks.LPFEnabled != nil && z.callbacks.LPFEnabled() {
		lpfHz := 20000.0
		if z.callbacks.LPFCutoffHz != nil {
			lpfHz = z.callbacks.LPFCutoffHz()
		}
		lx := freqToX(lpfHz, r)
		ly := z.curveYAtX(lx)
		dx := x - lx
		dy := y - ly
		if dx*dx+dy*dy <= hitRadius*hitRadius {
			z.curveDragFilter = "lpf"
			return InputCaptured
		}
	}

	// Hit-test band handles.
	for i, def := range eqBandDefs {
		center := math.Sqrt(def.loHz * def.hiHz)
		hx := freqToX(center, r)
		gain := 0.0
		if i < len(z.bandGainsDB) {
			gain = z.bandGainsDB[i]
		}
		hy := gainDBToY(gain, r)
		dx := x - hx
		dy := y - hy
		if dx*dx+dy*dy <= hitRadius*hitRadius {
			z.curveDragBand = i
			z.setDragLabel(fmt.Sprintf("%+.1f dB", gain), hx, hy)
			return InputCaptured
		}
	}
	return InputIgnored
}

func (h *curveHandleHitAdapter) OnDrag(x, y int) {
	z := h.zone
	r := z.rect

	// Handle filter (HPF/LPF) drag.
	if z.curveDragFilter != "" {
		freq := xToFreq(x, z.rect)
		if z.curveDragFilter == "hpf" {
			freq = clampF64(freq, 20, 2000)
			if z.callbacks.OnHPFCutoffChange != nil {
				z.callbacks.OnHPFCutoffChange(freq)
			}
			z.setDragLabel(fmt.Sprintf("%.0f Hz", freq), x, y)
		} else {
			freq = clampF64(freq, 1000, 20000)
			if z.callbacks.OnLPFCutoffChange != nil {
				z.callbacks.OnLPFCutoffChange(freq)
			}
			z.setDragLabel(fmt.Sprintf("%.0f Hz", freq), x, y)
		}
		z.curveDirty = true
		return
	}

	band := z.curveDragBand
	if band < 0 {
		return
	}
	db := yToGainDB(y, z.rect)
	if band < len(z.bandGainsDB) {
		z.bandGainsDB[band] = db
	}
	z.syncDBInputText(band)
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, r)
	hy := gainDBToY(db, r)
	z.setDragLabel(fmt.Sprintf("%+.1f dB", db), hx, hy)
	z.curveDirty = true
	if z.callbacks.OnGainChange != nil {
		z.callbacks.OnGainChange(band, db)
	}
}

func (h *curveHandleHitAdapter) OnRelease(x, y int) {
	z := h.zone
	z.curveDragDBText = ""

	// Handle filter (HPF/LPF) release.
	if z.curveDragFilter != "" {
		z.curveDragFilter = ""
		if z.callbacks.OnApplyEQ != nil {
			z.callbacks.OnApplyEQ()
		}
		return
	}

	if z.curveDragBand < 0 {
		return
	}
	z.curveDragBand = -1
	if z.callbacks.OnApplyEQ != nil {
		z.callbacks.OnApplyEQ()
	}
}

// setDragLabel positions the dB/Hz label 16px above the handle, flipping
// below if near the top of the rect.
func (z *EQPanelZone) setDragLabel(text string, hx, hy int) {
	z.curveDragDBText = text
	z.curveDragLabelX = hx
	labelY := hy - 16
	if labelY < z.rect.Min.Y+4 {
		labelY = hy + 16 // flip below
	}
	z.curveDragLabelY = labelY
}

func (h *curveHandleHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// CloseChannelDropdown closes the channel dropdown portal entry if open.
func (z *EQPanelZone) CloseChannelDropdown() {
	z.channelOpen = false
	if z.portal != nil {
		z.portal.Close("eq-channel-dropdown")
	}
}

func (z *EQPanelZone) buildChannelDropdown() {
	if z.portal == nil {
		return
	}
	overlay := &eqChannelDropdownOverlay{
		zone: z,
	}
	overlay.buildCallbacks()
	z.portal.Open(PortalEntry{
		ID:      "eq-channel-dropdown",
		Overlay: overlay,
		Modal:   false,
		Anchor:  z.stickyBar.ChannelBtn().Rect(),
		OnClose: func() {
			z.channelOpen = false
			if z.callbacks.OnChannelDropdownClose != nil {
				z.callbacks.OnChannelDropdownClose()
			}
		},
	})
}

// --- Channel dropdown overlay (scrollable) ---

type eqChannelDropdownOverlay struct {
	zone        *EQPanelZone
	rect        image.Rectangle
	buttons     []*Button
	allOnClicks []func() // callbacks for ALL items (indexed by absolute position)
	deferredTap DeferredTap
}

// buildCallbacks populates allOnClicks for the Master + per-row entries.
// Geometry (visible-row count, view rect, button rects, hit rect) is set up
// in layoutFor which runs from Layout(anchor, screenBounds) so the portal
// can resize the menu when the viewport changes.
func (o *eqChannelDropdownOverlay) buildCallbacks() {
	o.buttons = nil
	o.allOnClicks = nil
	z := o.zone

	o.allOnClicks = append(o.allOnClicks, func() {
		if z.callbacks.OnChannelChange != nil {
			z.callbacks.OnChannelChange("main")
		}
		z.channelOpen = false
		z.activeChannel = "main"
		z.stickyBar.ChannelBtn().Text = "Master"
		if z.portal != nil {
			z.portal.Close("eq-channel-dropdown")
		}
	})

	if z.callbacks.ActiveRows != nil {
		for _, row := range z.callbacks.ActiveRows() {
			instID := row.Instrument
			name := row.Name
			o.allOnClicks = append(o.allOnClicks, func() {
				if z.callbacks.OnChannelChange != nil {
					z.callbacks.OnChannelChange(instID)
				}
				z.channelOpen = false
				z.activeChannel = instID
				z.stickyBar.ChannelBtn().Text = name
				if z.portal != nil {
					z.portal.Close("eq-channel-dropdown")
				}
			})
		}
	}
}

// layoutFor computes the visible-row count and lays out the visible buttons.
// visible = min(eqChannelMenuMaxVisibleRows, rowsThatFitBelowAnchor, total).
// When screenBounds is empty the pixel cap is skipped (preserves old behaviour
// for callers that haven't routed bounds in yet).
func (o *eqChannelDropdownOverlay) layoutFor(anchor, screenBounds image.Rectangle) {
	z := o.zone
	btnH := 24

	total := len(o.allOnClicks)
	scroll := z.channelScroll
	scroll.ItemHeight = btnH
	scroll.Style = DropdownScrollbarStyle
	scroll.VS.Total = total

	visible := total
	if visible > eqChannelMenuMaxVisibleRows {
		visible = eqChannelMenuMaxVisibleRows
	}
	if !screenBounds.Empty() {
		availPx := screenBounds.Max.Y - anchor.Max.Y
		if availPx < btnH {
			availPx = btnH // never collapse to zero rows
		}
		availRows := availPx / btnH
		if availRows < visible {
			visible = availRows
		}
	}
	if visible < 1 {
		visible = 1
	}
	scroll.VS.Visible = visible
	scroll.VS.Clamp()

	menuH := visible * btnH
	scroll.VS.View = image.Rect(anchor.Min.X, anchor.Max.Y, anchor.Max.X, anchor.Max.Y+menuH)

	buttonMaxX := anchor.Max.X
	if scroll.HasScroll() {
		buttonMaxX -= eqChannelMenuScrollBarWidth
		if buttonMaxX <= anchor.Min.X {
			buttonMaxX = anchor.Min.X + 1
		}
	}

	o.buttons = o.buttons[:0]
	first := scroll.VS.First
	for i := 0; i < visible && first+i < total; i++ {
		idx := first + i
		var label string
		if idx == 0 {
			label = "Master"
		} else {
			rowIdx := idx - 1
			rows := z.callbacks.ActiveRows()
			if rows != nil && rowIdx < len(rows) {
				label = rows[rowIdx].Name
				if len(label) > 12 {
					label = label[:12] + "…"
				}
			}
		}
		onClick := o.allOnClicks[idx]
		r := image.Rect(anchor.Min.X, anchor.Max.Y+i*btnH, buttonMaxX, anchor.Max.Y+(i+1)*btnH)
		btn := NewButton(label, DropdownStyle, onClick)
		btn.ConsumeOnPress = true
		btn.SetRect(insetRect(r, SpaceXS))
		o.buttons = append(o.buttons, btn)
	}

	o.rect = image.Rect(anchor.Min.X, anchor.Max.Y, anchor.Max.X, anchor.Max.Y+menuH)
}

func (o *eqChannelDropdownOverlay) Layout(anchor, screenBounds image.Rectangle) {
	o.layoutFor(anchor, screenBounds)
}

// buildButtons is a convenience entry point used by callers (mostly tests)
// that construct an overlay outside the portal flow. It runs both passes —
// callbacks then geometry — using the zone's eqChannelBtn anchor and the
// portal-known screen bounds. Production code uses buildCallbacks + the
// portal-driven Layout() instead.
func (o *eqChannelDropdownOverlay) buildButtons() {
	o.buildCallbacks()
	var bounds image.Rectangle
	if o.zone.portal != nil {
		bounds = o.zone.portal.screenBounds
	}
	o.layoutFor(o.zone.stickyBar.ChannelBtn().Rect(), bounds)
}

func (o *eqChannelDropdownOverlay) HitAreas() []HitArea {
	if o.rect.Empty() {
		return nil
	}
	return []HitArea{
		{Rect: o.rect, Handler: &scrollableDropdownHandler{overlay: o}, Tag: "eq-channel-dropdown"},
	}
}

func (o *eqChannelDropdownOverlay) Draw(screen *ebiten.Image) {
	for _, btn := range o.buttons {
		btn.Draw(screen)
	}
	o.zone.channelScroll.Draw(screen)
}

func (o *eqChannelDropdownOverlay) ShouldClose() bool { return false }

// Update implements PortalUpdater for momentum scrolling.
func (o *eqChannelDropdownOverlay) Update() {
	scroll := o.zone.channelScroll
	if scroll.HasMomentum() {
		if scroll.UpdateMomentum() {
			o.rebuildVisibleButtons()
			o.updatePortalHitAreas()
		}
	}
}

// rebuildVisibleButtons regenerates the visible button slice based on current
// scroll position without changing the scroll state or allOnClicks.
func (o *eqChannelDropdownOverlay) rebuildVisibleButtons() {
	z := o.zone
	anchor := z.stickyBar.ChannelBtn().Rect()
	scroll := z.channelScroll
	btnH := scroll.ItemHeight
	if btnH < 1 {
		btnH = 24
	}

	buttonMaxX := anchor.Max.X
	if scroll.HasScroll() {
		buttonMaxX -= eqChannelMenuScrollBarWidth
		if buttonMaxX <= anchor.Min.X {
			buttonMaxX = anchor.Min.X + 1
		}
	}

	total := len(o.allOnClicks)
	visible := scroll.VS.Visible
	first := scroll.VS.First

	o.buttons = o.buttons[:0]
	for i := 0; i < visible && first+i < total; i++ {
		idx := first + i
		var label string
		if idx == 0 {
			label = "Master"
		} else {
			rowIdx := idx - 1
			rows := z.callbacks.ActiveRows()
			if rows != nil && rowIdx < len(rows) {
				label = rows[rowIdx].Name
				if len(label) > 12 {
					label = label[:12] + "…"
				}
			}
		}
		onClick := o.allOnClicks[idx]
		r := image.Rect(anchor.Min.X, anchor.Max.Y+i*btnH, buttonMaxX, anchor.Max.Y+(i+1)*btnH)
		btn := NewButton(label, DropdownStyle, onClick)
		btn.ConsumeOnPress = true
		btn.SetRect(insetRect(r, SpaceXS))
		o.buttons = append(o.buttons, btn)
	}
}

// updatePortalHitAreas re-registers hit areas in the portal after scroll changes.
func (o *eqChannelDropdownOverlay) updatePortalHitAreas() {
	if o.zone.portal != nil {
		areas := o.HitAreas()
		for i := range areas {
			areas[i].ZIndex = 300 // portal will correct this
		}
		o.zone.portal.hitIndex.UpdatePortal("eq-channel-dropdown", areas)
	}
}

// --- scrollableDropdownHandler ---

// scrollableDropdownHandler routes all input events for the scrollable
// channel dropdown overlay: wheel scroll, scrollbar drag, touch scroll
// with deferred tap, and button clicks.
type scrollableDropdownHandler struct {
	overlay *scrollableDropdownHandlerOverlay
}

// Use a type alias to avoid import cycles — the handler references the overlay.
type scrollableDropdownHandlerOverlay = eqChannelDropdownOverlay

func (h *scrollableDropdownHandler) OnPress(x, y int) InputResult {
	o := h.overlay
	scroll := o.zone.channelScroll
	pt := image.Pt(x, y)

	// Scrollbar thumb drag start.
	if scroll.HasScroll() {
		thumb := scroll.ThumbRect()
		if pt.In(thumb) {
			scroll.HandleDragStart(y)
			return InputCaptured
		}
	}

	// Begin deferred tap + touch scroll in menu area.
	if pt.In(o.rect) {
		if o.deferredTap.Begin(x, y) {
			scroll.HandleTouchBegin(x, y)
			return InputCaptured
		}
	}

	return InputConsumed
}

func (h *scrollableDropdownHandler) OnDrag(x, y int) {
	o := h.overlay
	scroll := o.zone.channelScroll

	// Scrollbar drag continuation.
	if scroll.Dragging() {
		if scroll.HandleDragTo(y) {
			o.rebuildVisibleButtons()
			o.updatePortalHitAreas()
		}
		return
	}

	// Touch scroll move.
	if scroll.TouchActive() {
		if scroll.HandleTouchMove(x, y) {
			o.rebuildVisibleButtons()
			o.updatePortalHitAreas()
		}
		// Cancel deferred tap if scroll committed.
		if scroll.ScrollingCommitted() {
			o.deferredTap.Cancel()
		}
	}
}

func (h *scrollableDropdownHandler) OnRelease(x, y int) {
	o := h.overlay
	scroll := o.zone.channelScroll

	// Scrollbar drag end.
	if scroll.Dragging() {
		scroll.HandleDragEnd()
		return
	}

	// Touch end.
	wasTap := scroll.TouchActive() && !scroll.ScrollingCommitted()
	scroll.HandleTouchEnd()

	// Fire deferred tap if touch didn't scroll.
	if wasTap {
		o.deferredTap.End(func(tx, ty int) {
			h.fireTapAt(tx, ty)
		})
	} else {
		o.deferredTap.Cancel()
	}
}

func (h *scrollableDropdownHandler) OnWheel(x, y, steps int) InputResult {
	o := h.overlay
	scroll := o.zone.channelScroll
	if scroll.HandleWheel(steps) {
		o.rebuildVisibleButtons()
		o.updatePortalHitAreas()
		return InputConsumed
	}
	return InputConsumed // always consume wheel over dropdown
}

// fireTapAt finds the button at (x, y) and calls its OnClick directly.
func (h *scrollableDropdownHandler) fireTapAt(x, y int) {
	o := h.overlay
	pt := image.Pt(x, y)
	for _, btn := range o.buttons {
		if pt.In(btn.Rect()) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}
}

// --- Drawing helpers ---

func (z *EQPanelZone) drawSpectrumBars(dst *ebiten.Image) {
	r := z.rect
	snap := z.analyzerSnapshot()
	spec := snap.Spectrum
	if len(spec) == 0 {
		return
	}

	if len(z.eqBandVals) != len(eqBandDefs) {
		z.eqBandVals = make([]float64, len(eqBandDefs))
	}
	blend := 0.5
	specLen := len(spec)
	for i, band := range eqBandDefs {
		nq := 24000.0
		start := int(math.Floor((band.loHz / nq) * float64(specLen)))
		end := int(math.Ceil((band.hiHz / nq) * float64(specLen)))
		if end <= start {
			end = start + 1
		}
		if start < 0 {
			start = 0
		}
		if end > specLen {
			end = specLen
		}
		maxV := 0.0
		for j := start; j < end; j++ {
			if spec[j] > maxV {
				maxV = spec[j]
			}
		}
		if maxV > 1 {
			maxV = 1
		}
		display := math.Sqrt(maxV)
		z.eqBandVals[i] = z.eqBandVals[i]*blend + display*(1-blend)
	}

	bandW := r.Dx() / len(eqBandDefs)
	if bandW < 1 {
		bandW = 1
	}
	maxHeight := r.Dy() - 8
	muteBtnH := Profile().EQSliderH
	muteBtnW := 16
	if Profile().IsMobile() {
		muteBtnW = bandW - 4
	}
	dbInputH := Profile().EQDBInputH
	dbInputY := r.Max.Y - dbInputH - 1
	muteBtnY := dbInputY - muteBtnH - 1

	for i, v := range z.eqBandVals {
		if v < 0 {
			continue
		}
		h := int(v * float64(maxHeight))
		if h < 2 && v > 0 {
			h = 2
		}
		x0 := r.Min.X + i*bandW
		x1 := x0 + bandW
		if i == len(eqBandDefs)-1 {
			x1 = r.Max.X
		}
		y0 := r.Max.Y - h

		bg := fadeColor(colEQBg, 0.2)
		if i%2 == 1 {
			bg = fadeColor(colGridLine, 0.3)
		}
		isMuted := i < len(z.bandMuted) && z.bandMuted[i]
		drawRect(dst, image.Rect(x0, r.Min.Y, x1, r.Max.Y), bg, true)

		if isMuted {
			drawRect(dst, image.Rect(x0, r.Min.Y, x1, r.Max.Y), fadeColor(colEQBg, 0.5), true)
		} else {
			col := colEQBar
			if v > 0.8 {
				col = colEQBarPeak
			}
			drawRect(dst, image.Rect(x0, y0, x1, r.Max.Y), col, true)
			drawRect(dst, image.Rect(x0, y0-2, x1, y0-1), fadeColor(col, 0.6), true)
		}

		if i > 0 {
			drawRect(dst, image.Rect(x0, r.Min.Y, x0+1, r.Max.Y), colGridLine, true)
		}

		// Row 1: Frequency band label.
		lbl := eqCenterLabels[i]
		if spr := TextSprite(lbl); spr != nil {
			lw, lh := spr.Bounds().Dx(), spr.Bounds().Dy()
			cx := x0 + (x1-x0-lw)/2
			if cx < r.Min.X {
				cx = r.Min.X
			}
			ly := r.Max.Y - lh - dbInputH - 6
			var op ebiten.DrawImageOptions
			op.GeoM.Translate(float64(cx), float64(ly))
			dst.DrawImage(spr, &op)
		}

		// Mute button: still participates in hit testing but not drawn
		// visually. Its muted state is indicated by the band column tint.
		if i < len(z.eqMuteBtns) && z.eqMuteBtns[i] != nil {
			btn := z.eqMuteBtns[i]
			muteBtnX := x0 + (x1-x0-muteBtnW)/2
			btn.SetRect(image.Rect(muteBtnX, muteBtnY, muteBtnX+muteBtnW, muteBtnY+muteBtnH))
			if isMuted {
				btn.Style = EQMuteButtonActiveStyle
			} else {
				btn.Style = EQMuteButtonStyle
			}
			// Skip btn.Draw — the "M" row is removed per 7C cleanup.
		}

		// Row 2: dB value (color-coded: cyan for boost, gray for cut).
		if z.eqDBInputs[i] != nil {
			ti := z.eqDBInputs[i]
			if ti.Focused() {
				ti.Draw(dst)
			} else {
				// Draw as plain centered text label with color coding.
				txt := ti.Value()
				tw := TextWidth(txt)
				th := TextHeight()
				tx := ti.Rect.Min.X + (ti.Rect.Dx()-tw)/2
				ty := ti.Rect.Min.Y + (ti.Rect.Dy()-th)/2
				var labelCol color.Color
				if isMuted {
					labelCol = colEQDBLabelMuted
				} else {
					gain := 0.0
					if i < len(z.bandGainsDB) {
						gain = z.bandGainsDB[i]
					}
					if gain > 0 {
						labelCol = colTextAccent // cyan for boost
					} else {
						labelCol = colTextSecondary // gray for cut or zero
					}
				}
				DrawTextColorAt(dst, txt, tx, ty, labelCol)
			}
		}
	}
}

// getAnalyzerState returns the latest analyzer.State via the callback, or nil.
//
// DEPRECATED for in-Draw use: prefer getAnalyzerStateForTab(activeTab) so
// the per-frame memo can route TabMeters through the lightweight
// metrics-only path. Direct callers (tests, freezeBtn check before
// memo wiring) bypass the memo and may produce duplicate per-frame
// calls.
func (z *EQPanelZone) getAnalyzerState() *analyzer.State {
	if z.callbacks.AnalyzerState != nil {
		return z.callbacks.AnalyzerState()
	}
	return nil
}

// getAnalyzerStateForTab is the per-Draw memoised resolver. Routes
// TabMeters through AnalyzerMetricsOnly when available — the
// scalar-only builder skips Spectrum/Waveform allocation and (on WASM)
// the per-element js.Value loop that drove the long-session OOM.
// All other tabs use the full AnalyzerState callback.
//
// The memo is keyed by (frameAnalyzerValid, frameAnalyzerTab); a tab
// switch within one Draw call would invalidate it, but Draw never
// changes tab mid-call so a single bool gate is sufficient. Cleared at
// the top of Draw().
func (z *EQPanelZone) getAnalyzerStateForTab(tab PanelTab) *analyzer.State {
	if z.frameAnalyzerValid && z.frameAnalyzerTab == tab {
		return z.frameAnalyzerState
	}
	var state *analyzer.State
	if tab == TabMeters && z.callbacks.AnalyzerMetricsOnly != nil {
		state = z.callbacks.AnalyzerMetricsOnly()
	} else if z.callbacks.AnalyzerState != nil {
		state = z.callbacks.AnalyzerState()
	}
	z.frameAnalyzerState = state
	z.frameAnalyzerTab = tab
	z.frameAnalyzerValid = true
	return state
}

// resolveChannel returns the ChannelMetrics and CaptureBuffer to display based
// on the current channel selection. If a non-master channel is selected and the
// analyzer provides detail data, that is used; otherwise falls back to master.
func (z *EQPanelZone) resolveChannel(state *analyzer.State) (*analyzer.ChannelMetrics, *analyzer.CaptureBuffer) {
	if z.activeChannel != "" && z.activeChannel != "main" && state.Detail != nil {
		return state.Detail, state.Capture
	}
	return &state.Master, state.Capture
}

func (z *EQPanelZone) analyzerSnapshot() audio.AnalyzerSnapshot {
	if z.callbacks.AnalyzerSnapshot != nil {
		return z.callbacks.AnalyzerSnapshot(z.ActiveChannel())
	}
	return audio.AnalyzerSnapshot{}
}

func (z *EQPanelZone) drawEQCurve(dst *ebiten.Image) {
	r := z.rect
	if r.Dx() < 16 || r.Dy() < 16 {
		return
	}

	// Ensure curve cache.
	if z.curveDirty || len(z.curveCache) == 0 {
		z.rebuildCurveCache()
	}

	if len(z.curveCache) == 0 {
		return
	}

	// 0 dB reference line (dashed).
	zeroY := gainDBToY(0, r)
	for dx := r.Min.X; dx < r.Max.X; dx += 6 {
		end := dx + 3
		if end > r.Max.X {
			end = r.Max.X
		}
		drawRect(dst, image.Rect(dx, zeroY, end, zeroY+1), colEQZeroLine, true)
	}

	// Curve.
	prevX := -1
	for _, p := range z.curveCache {
		px := freqToX(p.FreqHz, r)
		py := gainDBToY(p.GainDB, r)
		if py < r.Min.Y {
			py = r.Min.Y
		}
		if py > r.Max.Y-1 {
			py = r.Max.Y - 1
		}
		if prevX >= 0 && px > prevX {
			fillY0, fillY1 := py, zeroY
			if fillY0 > fillY1 {
				fillY0, fillY1 = fillY1, fillY0
			}
			if fillY1-fillY0 > 1 {
				drawRect(dst, image.Rect(px, fillY0, px+1, fillY1), colEQCurveFill, true)
			}
			lineY0, lineY1 := py, py+2
			if lineY1 > r.Max.Y {
				lineY1 = r.Max.Y
			}
			drawRect(dst, image.Rect(px, lineY0, px+1, lineY1), colEQCurve, true)
		}
		prevX = px
	}

	// Band handles with glow effect and border ring.
	for i, def := range eqBandDefs {
		center := math.Sqrt(def.loHz * def.hiHz)
		hx := freqToX(center, r)
		gain := 0.0
		if i < len(z.bandGainsDB) {
			gain = z.bandGainsDB[i]
		}
		hy := gainDBToY(gain, r)
		if hy < r.Min.Y+eqHandleRadius {
			hy = r.Min.Y + eqHandleRadius
		}
		if hy > r.Max.Y-eqHandleRadius {
			hy = r.Max.Y - eqHandleRadius
		}
		hr := eqHandleRadius
		col := colEQHandle
		isDragging := z.curveDragBand == i
		if isDragging {
			col = colEQHandleActive
			hr = eqHandleRadius + 3
		}
		// Subtle glow behind handle: 16px circle at 15% opacity (25% when dragging).
		glowR := 16
		glowAlpha := uint8(38) // ~15% of 255
		if isDragging {
			glowAlpha = 64 // ~25% of 255
		}
		glowCol := WithAlpha(genColorPrimary, glowAlpha)
		glowRect := image.Rect(hx-glowR, hy-glowR, hx+glowR, hy+glowR)
		drawRoundedRect(dst, glowRect, glowCol, glowR, true)
		// Handle fill.
		handleRect := image.Rect(hx-hr, hy-hr, hx+hr, hy+hr)
		drawRoundedRect(dst, handleRect, col, hr, true)
		// 2px border ring.
		drawRoundedRect(dst, handleRect, colEQHandleBorder, hr, false)
		borderInner := image.Rect(hx-hr+1, hy-hr+1, hx+hr-1, hy+hr-1)
		drawRoundedRect(dst, borderInner, colEQHandleBorder, hr-1, false)
	}

	// Draw HPF handle if enabled.
	if z.callbacks.HPFEnabled != nil && z.callbacks.HPFEnabled() {
		hpfHz := 20.0
		if z.callbacks.HPFCutoffHz != nil {
			hpfHz = z.callbacks.HPFCutoffHz()
		}
		hx := freqToX(hpfHz, r)
		hy := z.curveYAtX(hx)
		hr := eqHandleRadius + 1
		col := colEQFilterHandle
		if z.curveDragFilter == "hpf" {
			col = colEQFilterHandleActive
			hr = eqHandleRadius + 3
		}
		drawRect(dst, image.Rect(hx, hy, hx+1, r.Max.Y), colEQFilterLine, true)
		drawRect(dst, image.Rect(hx-hr, hy-hr, hx+hr, hy+hr), col, true)
		drawRect(dst, image.Rect(hx-hr, hy-hr, hx+hr, hy+hr), colEQFilterHandle, false)
	}

	// Draw LPF handle if enabled.
	if z.callbacks.LPFEnabled != nil && z.callbacks.LPFEnabled() {
		lpfHz := 20000.0
		if z.callbacks.LPFCutoffHz != nil {
			lpfHz = z.callbacks.LPFCutoffHz()
		}
		lx := freqToX(lpfHz, r)
		ly := z.curveYAtX(lx)
		lr := eqHandleRadius + 1
		col := colEQFilterHandle
		if z.curveDragFilter == "lpf" {
			col = colEQFilterHandleActive
			lr = eqHandleRadius + 3
		}
		drawRect(dst, image.Rect(lx, ly, lx+1, r.Max.Y), colEQFilterLine, true)
		drawRect(dst, image.Rect(lx-lr, ly-lr, lx+lr, ly+lr), col, true)
		drawRect(dst, image.Rect(lx-lr, ly-lr, lx+lr, ly+lr), colEQFilterHandle, false)
	}

	// Draw real-time dB/Hz label during curve handle drag.
	if z.curveDragDBText != "" {
		lbl := z.curveDragDBText
		tw := TextWidth(lbl)
		th := 12 // approximate text height
		lx := z.curveDragLabelX - tw/2
		ly := z.curveDragLabelY - th/2
		// Clamp horizontally within the rect.
		if lx < r.Min.X+2 {
			lx = r.Min.X + 2
		}
		if lx+tw > r.Max.X-2 {
			lx = r.Max.X - tw - 2
		}
		bgR := image.Rect(lx-3, ly-2, lx+tw+3, ly+th+2)
		drawRect(dst, bgR, WithAlpha(genColorEqReadoutBg, genAlphaSidebarChip), true)
		DrawTextColorAt(dst, lbl, lx, ly, genColorBorder)
	}
}

// curveYAtX returns the Y pixel on the cached frequency response curve closest
// to the given X pixel. Falls back to the 0 dB line.
func (z *EQPanelZone) curveYAtX(px int) int {
	r := z.rect
	// Ensure cache is fresh.
	if z.curveDirty || len(z.curveCache) == 0 {
		z.rebuildCurveCache()
	}
	best := gainDBToY(0, r)
	bestDist := r.Dx() + 1
	for _, p := range z.curveCache {
		cx := freqToX(p.FreqHz, r)
		d := cx - px
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			best = gainDBToY(p.GainDB, r)
		}
	}
	if best < r.Min.Y+eqHandleRadius {
		best = r.Min.Y + eqHandleRadius
	}
	if best > r.Max.Y-eqHandleRadius {
		best = r.Max.Y - eqHandleRadius
	}
	return best
}

// rebuildCurveCache recomputes the EQ frequency response cache from the
// current band gains/mutes and clears the dirty flag. Single source of
// truth for both Draw and SampleCurve.
func (z *EQPanelZone) rebuildCurveCache() {
	bands := z.buildCurrentBands()
	z.curveCache = audio.ComputeFreqResponse(audio.SampleRate(), bands, eqCurvePoints, 20, 20000)
	z.curveDirty = false
}

// SetBandGainDB updates the gain (in dB) for a single EQ band and marks
// the curve cache dirty. Out-of-range band indices are silently ignored.
func (z *EQPanelZone) SetBandGainDB(band int, db float64) {
	if band < 0 || band >= len(z.bandGainsDB) {
		return
	}
	z.bandGainsDB[band] = db
	z.curveDirty = true
	z.curveCache = nil
}

// SampleCurve returns the current EQ frequency response sampled at n
// equally-spaced points (in dB). Used by the mobile peek sparkline.
// Caller must not retain the returned slice across frames; the curve
// cache is rebuilt on every gain change.
func (z *EQPanelZone) SampleCurve(n int) []float64 {
	if n < 2 {
		n = 2
	}
	out := make([]float64, n)
	if z.curveDirty || len(z.curveCache) == 0 {
		z.rebuildCurveCache()
	}
	if len(z.curveCache) == 0 {
		return out
	}
	for i := 0; i < n; i++ {
		idx := i * (len(z.curveCache) - 1) / (n - 1)
		out[i] = z.curveCache[idx].GainDB
	}
	return out
}

func (z *EQPanelZone) buildCurrentBands() []audio.EQBand {
	bands := make([]audio.EQBand, 0, len(eqBandDefs)+2)
	// Prepend HPF when enabled.
	if z.callbacks.HPFEnabled != nil && z.callbacks.HPFEnabled() {
		hz := 20.0
		if z.callbacks.HPFCutoffHz != nil {
			hz = z.callbacks.HPFCutoffHz()
		}
		if hz > 20 {
			bands = append(bands, audio.EQBand{Kind: audio.EQHighpass, Freq: hz, Q: 0.707})
		}
	}
	for i, def := range eqBandDefs {
		kind := audio.EQPeaking
		if i == 0 {
			kind = audio.EQLowShelf
		}
		if i == len(eqBandDefs)-1 {
			kind = audio.EQHighShelf
		}
		center := math.Sqrt(def.loHz * def.hiHz)
		gain := 0.0
		if i < len(z.bandGainsDB) {
			gain = z.bandGainsDB[i]
		}
		isMuted := i < len(z.bandMuted) && z.bandMuted[i]
		bands = append(bands, audio.EQBand{
			Kind:   kind,
			Freq:   center,
			Q:      1.414,
			GainDB: gain,
			Muted:  isMuted,
		})
	}
	// Append LPF when enabled.
	if z.callbacks.LPFEnabled != nil && z.callbacks.LPFEnabled() {
		hz := 20000.0
		if z.callbacks.LPFCutoffHz != nil {
			hz = z.callbacks.LPFCutoffHz()
		}
		if hz < 20000 {
			bands = append(bands, audio.EQBand{Kind: audio.EQLowpass, Freq: hz, Q: 0.707})
		}
	}
	return bands
}

// --- dB text input helpers ---

// formatDB formats a dB gain value for display: "0.0" at zero, "+6.0"/"-3.2" otherwise.
func formatDB(db float64) string {
	if db == 0 {
		return "0.0"
	}
	return fmt.Sprintf("%+.1f", db)
}

// updateDBInputs runs per-frame update for all dB text inputs,
// tracking focus transitions and ensuring only one is focused at a time.
func (z *EQPanelZone) updateDBInputs() {
	newFocused := -1
	for i, ti := range z.eqDBInputs {
		if ti == nil {
			continue
		}
		prevFocus := ti.Focused()
		ti.Update()
		if ti.Focused() && !prevFocus {
			// Focus gained: save prev value, clear text for entry.
			z.dbInputPrev = z.bandGainsDB[i]
			ti.SetText("")
			newFocused = i
		}
		if !ti.Focused() && prevFocus {
			// Focus lost (click-away or Enter in TextInput): commit.
			z.commitDBText(i)
		}
		if ti.Focused() {
			newFocused = i
		}
	}
	// Ensure only one focused at a time.
	if newFocused >= 0 {
		for i, ti := range z.eqDBInputs {
			if ti != nil && i != newFocused && ti.Focused() {
				z.commitDBText(i)
				ti.focused = false
			}
		}
		z.dbInputFocused = newFocused
	} else {
		z.dbInputFocused = -1
	}
}

// commitDBText parses and applies the dB value from the text input at band,
// clamping to [-12, +12] and rounding to 0.1. Invalid/empty text reverts.
func (z *EQPanelZone) commitDBText(band int) {
	if band < 0 || band >= len(z.eqDBInputs) {
		return
	}
	ti := z.eqDBInputs[band]
	if ti == nil {
		return
	}
	val := ti.Value()
	db, err := strconv.ParseFloat(val, 64)
	if err != nil || val == "" {
		// Invalid or empty: revert to previous.
		ti.SetText(formatDB(z.dbInputPrev))
		ti.focused = false
		return
	}
	db = clampF64(db, -12.0, 12.0)
	db = math.Round(db*10) / 10
	z.bandGainsDB[band] = db
	ti.SetText(formatDB(db))
	ti.focused = false
	z.curveDirty = true
	if z.callbacks.OnGainChange != nil {
		z.callbacks.OnGainChange(band, db)
	}
	if z.callbacks.OnApplyEQ != nil {
		z.callbacks.OnApplyEQ()
	}
}

// syncDBInputText updates the dB text input for a single band from bandGainsDB,
// unless that input is currently focused (user is editing) or a sync guard is set.
func (z *EQPanelZone) syncDBInputText(band int) {
	if z.dbInputSyncing {
		return
	}
	if band < 0 || band >= len(z.eqDBInputs) {
		return
	}
	ti := z.eqDBInputs[band]
	if ti == nil || ti.Focused() {
		return
	}
	z.dbInputSyncing = true
	ti.SetText(formatDB(z.bandGainsDB[band]))
	z.dbInputSyncing = false
}

// syncAllDBInputTexts syncs all 10 dB inputs from bandGainsDB.
func (z *EQPanelZone) syncAllDBInputTexts() {
	for i := range z.eqDBInputs {
		z.syncDBInputText(i)
	}
}

// newScrollBehavior creates a fresh ScrollBehavior for use in the zone.
func newScrollBehavior() *ScrollBehavior {
	return NewScrollBehavior(DropdownScrollbarStyle, 24)
}
