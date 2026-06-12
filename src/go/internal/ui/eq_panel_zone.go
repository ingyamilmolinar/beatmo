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
	// OnEQBandCommit fires once when an EQ band-gain edit settles (curve-handle
	// drag release or dB text-input commit). DrumView uses it to emit the
	// EQ-band event + record one undo step (the live audio update stays
	// per-frame via OnGainChange).
	OnEQBandCommit   func()
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

	// Phase 4 — Synth tab routing. Each callback bridges the EQ panel's
	// active-tab dispatch to DrumView's per-row SynthRecipe state. Wiring
	// lives in drumview_ctor.go; the EQ panel itself does not own any
	// synth-tab state, mirroring how TabScope delegates to chainZone.
	//
	// OnSynthTabLayout is called every Layout() while TabSynth is active.
	// The content rect spans the area below the sticky bar and above the
	// reset footer; DrumView lays out one slider per ParamDef inside it.
	OnSynthTabLayout func(contentR image.Rectangle)

	// DrawSynthTab is called from Draw() when activeTab == TabSynth and
	// is expected to render whatever DrumView's buildSynthTab produced.
	DrawSynthTab func(dst *ebiten.Image, contentR image.Rectangle)

	// SynthTabHitAreas returns the slider + button hit areas DrumView
	// built during OnSynthTabLayout. EQPanelZone.HitAreas() appends them
	// when TabSynth is active.
	SynthTabHitAreas func() []HitArea

	// SynthTabUpdate advances per-frame synth-tab state (section scroll
	// momentum). Called from Update() when TabSynth is active; returns true
	// when a scroll position changed so the zone re-lays-out next frame.
	SynthTabUpdate func() bool

	// Sampler tab routing — mirrors the Synth-tab callbacks above. The EQ
	// panel owns no sampler state; DrumView builds/draws the sampler tab and
	// the wiring lives in drumview_ctor.go.
	OnSamplerTabLayout func(contentR image.Rectangle)
	DrawSamplerTab     func(dst *ebiten.Image, contentR image.Rectangle)
	SamplerTabHitAreas func() []HitArea

	// IsHiddenForInput is the zone's self-defense input gate. When set
	// and returning true, `HitAreas()` returns nil — guaranteeing no
	// audio-panel hit areas reach the tree's HitIndex while the panel
	// is logically hidden (mobile + viewMode=Rows). The DrumView ctor
	// wires this to the same predicate the tree uses for
	// `RegisterZoneVisible`, so a future tree refactor that drops the
	// visibility gate can't silently re-open the Pads-tab leak.
	// Belt-and-suspenders for the tree-level fix in drumview_tree.go.
	IsHiddenForInput func() bool
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
	// `levelsLatch` is the legacy single-channel latch (Master only) used
	// when no instruments are present; `levelsLatches` holds per-channel
	// latches for the Phase 2 multi-channel strip layout.
	levelsLatch   LevelsLatch
	levelsLatches *MultiLevelsLatch
	// lastClipCountsByID memoises the previous tick's per-channel
	// ClipCount so we can compute per-channel deltas and push them
	// into the audio package's rolling 10 s clip window. Phase 2
	// audio-panel redesign.
	lastClipCountsByID map[string]int

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
	dbRowH         int     // height of the per-band dB readout row (set in layout)
	eqSelectedBand int     // band most recently grabbed (drives the drag highlight)

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
	// Phase 5: legend popover state. Toggled by the ? chip OnClick.
	// When true, drawAudioPanelLegend paints a 220-px kid-readable
	// sheet anchored under the chip during Draw().
	legendOpen bool

	// Phase 4 audio-panel redesign: Levels icon-row tooltip state.
	// Snapshotted during the Levels-tab Draw so a subsequent long-press
	// (GestureLongPress routed via the game) can surface the
	// unabbreviated aggregate value without re-reading analyzer state.
	// All three rects are empty when the current readout mode is full or
	// chevron; only the icon-row mode populates them.
	levelsHeadroomRect image.Rectangle
	levelsClipsRect    image.Rectangle
	levelsLoudestRect  image.Rectangle
	levelsAggSnapshot  LevelsAggregate
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
	// Phase 0 audio-panel dispatcher: ensure the initial tab's analyzer
	// taps are enabled so the very first Draw has signal to render. The
	// EQ tab is the default; switching elsewhere routes through the
	// onTab closure which re-applies the rule.
	EnsureAnalyzersForTab(z.tabState.ActiveTab(), z.activeChannel)
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
		// Phase 0 audio-panel dispatcher: every transition into a tab
		// that reads the master analyzer (Wave / Spectrum / Levels /
		// EQ / Chain) must enable that analyzer before the next Draw
		// or the panel renders flat output. Idempotent — calling this
		// every transition is cheap. See audio_panel_dispatcher.go for
		// the full rule.
		EnsureAnalyzersForTab(tab, z.activeChannel)
		if z.callbacks.OnTabChange != nil {
			z.callbacks.OnTabChange(tab)
		}
	}
	z.stickyBar = NewAudioStickyBar(130, onChannel, onFreeze, onClose, onTab)
	// Wire the Reset Hold pill (Phase 1 spectrum redesign) to clear the
	// all-time MaxPeak watermark on the spectrum bars. The pill itself
	// lives in the sticky bar; the SpectrumPeakState lives here.
	if rst := z.stickyBar.ResetHoldBtn(); rst != nil {
		rst.OnClick = func() {
			z.spectrumPeaks.ResetMax()
		}
	}
	// Wire the Clear Clips pill (Phase 2 levels redesign) to clear the
	// per-channel latches + the audio package's rolling 10 s clip
	// window. Without this the persistent "!" markers couldn't be
	// acknowledged.
	if clr := z.stickyBar.ClearClipsBtn(); clr != nil {
		clr.OnClick = func() {
			if z.levelsLatches != nil {
				z.levelsLatches.Clear()
			}
			audio.ResetClipsWindow()
		}
	}
	// Phase 5 audio-panel redesign: legend chip + tab expander.
	if lg := z.stickyBar.LegendBtn(); lg != nil {
		lg.OnClick = func() {
			z.legendOpen = !z.legendOpen
		}
	}
	if ex := z.stickyBar.ExpanderBtn(); ex != nil {
		ex.OnClick = func() {
			if z.tabState != nil {
				z.tabState.ToggleExpanded()
			}
		}
	}

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
	// Phase 4: delegate layout to DrumView's synth-tab builder so per-row
	// slider rects are recomputed alongside the rest of the panel chrome.
	if z.tabState.ActiveTab() == TabSynth && z.callbacks.OnSynthTabLayout != nil {
		z.callbacks.OnSynthTabLayout(z.contentRect())
	}
	if z.tabState.ActiveTab() == TabSampler && z.callbacks.OnSamplerTabLayout != nil {
		z.callbacks.OnSamplerTabLayout(z.contentRect())
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

// SetActiveTab updates the active tab and flags layout for the next
// frame. Exposed for tests + scene catalog programmatic tab switches.
// Production user-driven tab switches still go through the sticky bar
// onTab closure in NewEQPanelZone.
func (z *EQPanelZone) SetActiveTab(tab PanelTab) {
	if z.tabState == nil {
		return
	}
	if z.tabState.ActiveTab() == tab {
		return
	}
	z.tabState.SetActiveTab(tab)
	z.needLayout = true
	// Mirror the onTab-closure analyzer-enable so programmatic tab
	// switches (scene catalog, tests, SetActiveEQTab JS export) get the
	// same dispatcher behaviour as a user click on a sticky-bar pill.
	EnsureAnalyzersForTab(tab, z.activeChannel)
	if z.callbacks.OnTabChange != nil {
		z.callbacks.OnTabChange(tab)
	}
}

func (z *EQPanelZone) Update() {
	// Refresh cursor readout state first — desktop reads hover, mobile is
	// driven by the scrub hit-area but still wants the tab-changed gate.
	z.updateAudioPanelCursor()

	// Synth-tab section scroll momentum (touch fling decay). Invalidate so the
	// next Layout re-derives the knob rects for the scrolled window.
	if z.tabState.ActiveTab() == TabSynth && z.callbacks.SynthTabUpdate != nil {
		if z.callbacks.SynthTabUpdate() {
			z.needLayout = true
		}
	}

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
	// Self-defense gate: when the host says we're hidden for input,
	// publish zero hit areas. Prevents the panel's catch-all (and
	// every per-control area) from reaching the tree's HitIndex while
	// the panel is logically hidden — e.g. mobile + viewMode=Rows
	// (Pads). The tree-level visibility gate is the primary
	// enforcement; this is the belt-and-suspenders layer so a future
	// tree refactor that drops the gate can't silently re-open the
	// Pads-tab input leak.
	if z.callbacks.IsHiddenForInput != nil && z.callbacks.IsHiddenForInput() {
		return nil
	}
	if z.chainZone != nil && z.tabState.ActiveTab() == TabScope {
		return append(z.hitAreas, z.chainZone.HitAreas()...)
	}
	if z.tabState.ActiveTab() == TabSynth && z.callbacks.SynthTabHitAreas != nil {
		return append(z.hitAreas, z.callbacks.SynthTabHitAreas()...)
	}
	if z.tabState.ActiveTab() == TabSampler && z.callbacks.SamplerTabHitAreas != nil {
		return append(z.hitAreas, z.callbacks.SamplerTabHitAreas()...)
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
		cr := z.bodyRect()
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
		cr := z.bodyRect()
		scale := freqScaleLog
		if z.stickyBar != nil && !z.stickyBar.FreqScaleLog() {
			scale = freqScaleLinear
		}
		if state := z.getAnalyzerStateForTab(activeTab); state != nil {
			ch, _ := z.resolveChannel(state)
			drawAnalyzerSpectrumWithScale(screen, cr, ch, &z.spectrumPeaks, scale)
			// Phase 1 Pre|Post overlay: when the sticky-bar pill is
			// active, fetch the pre-EQ snapshot for the active channel
			// and paint a thin "pre" trace beneath the post-EQ bars.
			// The two traces let kids see exactly what the EQ is
			// shaping (the post-EQ curve is the bars + curve underlay,
			// the pre-EQ trace is the orange dotted line below).
			if z.stickyBar != nil && z.stickyBar.PreOverlay() {
				snap := audio.PreEQAnalyzerSnapshot(z.activeChannel)
				if len(snap.Spectrum) > 0 {
					drawPreEQOverlayFromSnapshot(screen, cr, snap.Spectrum, float64(audio.SampleRate()), scale)
				}
			}
			if z.cursorActive || z.cursorPinned {
				drawSpectrumCursor(screen, cr, ch, z.cursorX)
			}
		} else {
			drawAnalyzerSpectrumWithScale(screen, cr, nil, &z.spectrumPeaks, scale)
		}
	case TabMeters:
		// Levels tab: per-channel strips (Phase 2). One vertical Peak /
		// RMS strip per instrument plus a wider Master strip on the
		// right; aggregate readouts (Headroom / Clips / Loudest) live
		// in a side panel. Each strip drives an independent peak-hold
		// latch so per-instrument transients persist between hits.
		if state := z.getAnalyzerStateForTab(activeTab); state != nil {
			if z.levelsLatches == nil {
				z.levelsLatches = NewMultiLevelsLatch()
			}
			if z.lastClipCountsByID == nil {
				z.lastClipCountsByID = map[string]int{}
			}
			// Tick every per-instrument latch + the master. Push the
			// clip delta into the audio package's rolling 10 s window
			// so the LevelsAggregates side panel can display
			// "Clips (10s)" instead of a monotonic session total.
			for i := range state.Instruments {
				inst := &state.Instruments[i]
				z.levelsLatches.Get(inst.ID).Update(inst.ClipCount, inst.PeakDB, inst.RMSDB)
				if d := inst.ClipCount - z.lastClipCountsByID[inst.ID]; d > 0 {
					audio.PushClipDelta(d)
				}
				z.lastClipCountsByID[inst.ID] = inst.ClipCount
			}
			z.levelsLatches.Get("main").Update(state.Master.ClipCount, state.Master.PeakDB, state.Master.RMSDB)
			if d := state.Master.ClipCount - z.lastClipCountsByID["main"]; d > 0 {
				audio.PushClipDelta(d)
			}
			z.lastClipCountsByID["main"] = state.Master.ClipCount
			// Keep the legacy single-channel latch in sync so the
			// fallback path (zero instruments) still works.
			z.levelsLatch.Update(state.Master.ClipCount, state.Master.PeakDB, state.Master.RMSDB)
			drawLevelsMultiChannel(screen, z.bodyRect(), state, z.levelsLatches)
			z.snapshotLevelsIconRow(state)
		} else {
			drawLevelsDetail(screen, z.bodyRect(), nil, nil)
		}
	case TabScope:
		if z.chainZone != nil {
			z.chainZone.Draw(screen)
		}
	case TabSynth:
		// Phase 4: delegate to DrumView's synth-tab renderer.
		if z.callbacks.DrawSynthTab != nil {
			z.callbacks.DrawSynthTab(screen, z.contentRect())
		}
	case TabSampler:
		// Delegate to DrumView's sampler-tab renderer.
		if z.callbacks.DrawSamplerTab != nil {
			z.callbacks.DrawSamplerTab(screen, z.contentRect())
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
	// Phase 5: legend popover sits above everything else when the
	// user has clicked the ? chip. Drawn last (after the panel
	// border) so it's never occluded.
	if z.legendOpen && z.stickyBar != nil {
		if lg := z.stickyBar.LegendBtn(); lg != nil && !lg.Rect().Empty() {
			drawAudioPanelLegend(screen, lg.Rect(), z.rect.Max.X, activeTab)
		}
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

// toggleButtonLabel returns the display text for the EQ/Wave toggle button.
func (z *EQPanelZone) toggleButtonLabel() string {
	return PanelTabLabel(z.tabState.ActiveTab())
}

// contentRect returns the drawable area below the sticky-bar chrome strip.
// The strip occupies stickyBarH (=26) pixels at the top of z.rect.
func (z *EQPanelZone) contentRect() image.Rectangle {
	return image.Rect(z.rect.Min.X, z.rect.Min.Y+stickyBarHeight(), z.rect.Max.X, z.rect.Max.Y)
}

// activeTabControls returns the control component owning the active tab's
// chrome, or nil for tabs that own their controls elsewhere (EQ inline, Chain
// via ChainPanelZone, Synth/Sampler via DrumView header). Phase 0 stub — no
// components are built yet, so this always returns nil. Later phases replace
// the body with a per-tab switch.
func (z *EQPanelZone) activeTabControls() tabControls { return nil }

// controlHeaderH is the height the active tab's control header needs (0 none).
func (z *EQPanelZone) controlHeaderH() int {
	if c := z.activeTabControls(); c != nil {
		return c.HeaderH()
	}
	return 0
}

// headerRect is the control-header strip at the top of contentRect().
func (z *EQPanelZone) headerRect() image.Rectangle {
	cr := z.contentRect()
	return image.Rect(cr.Min.X, cr.Min.Y, cr.Max.X, cr.Min.Y+z.controlHeaderH())
}

// bodyRect is the drawable area below the control header — where the active
// tab's content (waveform / spectrum / meters) is rendered. Equals
// contentRect() when the active tab has no control header.
func (z *EQPanelZone) bodyRect() image.Rectangle {
	cr := z.contentRect()
	return image.Rect(cr.Min.X, cr.Min.Y+z.controlHeaderH(), cr.Max.X, cr.Max.Y)
}

// hpfLPFSubStripH is the desktop height of the HPF/LPF button row drawn at the
// top of the EQ-tab content rect (above the EQ curve / spectrum bars).
const hpfLPFSubStripH = 22

// hpfStripH is the HPF/LPF sub-strip height — taller on mobile so the HP/LP
// toggles can render at a finger-friendly size.
func (z *EQPanelZone) hpfStripH() int {
	if Profile().IsMobile() {
		return 40
	}
	return hpfLPFSubStripH
}

// eqCurveRect returns the rectangle inside contentRect() that holds the EQ
// curve / spectrum visualization on TabEQ — i.e. content rect minus the
// HPF/LPF sub-strip at the top.
func (z *EQPanelZone) eqCurveRect() image.Rectangle {
	cr := z.contentRect()
	if z.tabState != nil && z.tabState.ActiveTab() == TabEQ {
		return image.Rect(cr.Min.X, cr.Min.Y+z.hpfStripH(), cr.Max.X, cr.Max.Y)
	}
	return cr
}

// eqLabelStripH is the height reserved at the bottom of the panel for the
// per-band frequency label row + the dB value / stepper row. The gain plot
// stops above this strip so handles never cover the labels.
func (z *EQPanelZone) eqLabelStripH() int {
	h := z.dbRowH
	if h <= 0 {
		h = ExpandHitArea(Profile().EQDBInputH)
	}
	return TextHeight() + h + 6
}

// eqLabelStripTop is the Y where the bottom label strip begins.
func (z *EQPanelZone) eqLabelStripTop() int {
	return z.rect.Max.Y - z.eqLabelStripH()
}

// eqPlotRect is the region the EQ curve + band handles are mapped into:
// full panel width, vertically between the curve-area top (below the sticky
// bar + HPF strip) and the bottom label strip. Keeping gain↔Y mapping inside
// this rect (rather than the whole panel) is what stops handles from sliding
// under the sticky bar at high gain or over the freq/dB labels at low gain.
func (z *EQPanelZone) eqPlotRect() image.Rectangle {
	cr := z.eqCurveRect()
	bottom := z.eqLabelStripTop()
	if bottom < cr.Min.Y {
		bottom = cr.Min.Y
	}
	if bottom > cr.Max.Y {
		bottom = cr.Max.Y
	}
	return image.Rect(cr.Min.X, cr.Min.Y, cr.Max.X, bottom)
}

// eqHandleY returns the clamped Y a band handle is drawn / hit-tested at for a
// given gain, kept within the plot rect by the handle radius.
func (z *EQPanelZone) eqHandleY(db float64) int {
	pr := z.eqPlotRect()
	y := gainDBToY(db, pr)
	if y < pr.Min.Y+eqHandleRadius {
		y = pr.Min.Y + eqHandleRadius
	}
	if y > pr.Max.Y-eqHandleRadius {
		y = pr.Max.Y - eqHandleRadius
	}
	return y
}

// curveSelectBand records which band the mobile stepper edits. Called whenever
// a band is grabbed (precise handle or column) so the stepper follows the
// user's focus.
func (z *EQPanelZone) curveSelectBand(i int) {
	if i >= 0 && i < len(eqBandDefs) {
		z.eqSelectedBand = i
	}
}

// eqBandHandlePos is the single source of truth for a band handle's centre —
// used by both the renderer and the hit-test so they can never disagree. X is
// the band's log-frequency position; Y is the clamped gain position within the
// plot rect (so handles never cover the bottom freq/dB labels).
func (z *EQPanelZone) eqBandHandlePos(i int) (int, int) {
	def := eqBandDefs[i]
	hx := freqToX(math.Sqrt(def.loHz*def.hiHz), z.eqPlotRect())
	gain := 0.0
	if i < len(z.bandGainsDB) {
		gain = z.bandGainsDB[i]
	}
	return hx, z.eqHandleY(gain)
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

// SetActiveChannel updates the EQ panel's active channel id. Intended for
// scene catalog setups and tests that need deterministic channel
// resolution; production code routes through the channel dropdown's
// click handlers in (*OverlayPortal). Empty / "main" resets to Master,
// which fans out to the first non-empty row via synthTabActiveInstrument
// when the Synth tab is open.
func (z *EQPanelZone) SetActiveChannel(id string) {
	if id == "" {
		id = "main"
	}
	z.activeChannel = id
	if z.stickyBar != nil && z.stickyBar.ChannelBtn() != nil {
		if id == "main" {
			z.stickyBar.ChannelBtn().Text = "Master"
		} else {
			z.stickyBar.ChannelBtn().Text = id
		}
	}
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
		// Phase 1 audio-panel redesign: the bar needs to know the active
		// tab before Layout so it can decide whether to claim space for
		// the Spectrum-only pills (slope / Pre / Reset Hold).
		z.stickyBar.SetActiveTab(z.tabState.ActiveTab())
		z.stickyBar.Layout(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+stickyBarHeight()))
	}

	// HPF/LPF live inside the content rect on TabEQ — at the top, above the
	// EQ curve. They participate in layout regardless of active tab (so the
	// rects are stable) but are only drawn / hit-tested when TabEQ is active.
	cr := z.contentRect()
	btnH := 18
	filterBtnW := 28
	pad := 2
	if Profile().IsMobile() {
		// Bigger, finger-friendly HP/LP toggles inside the taller mobile strip.
		btnH = z.hpfStripH() - 8
		filterBtnW = 52
		pad = SpaceSM
	}
	subY := cr.Min.Y + pad
	hpfX := cr.Min.X + 6
	z.hpfBtn.SetRect(image.Rect(hpfX, subY, hpfX+filterBtnW, subY+btnH))
	lpfX := hpfX + filterBtnW + SpaceSM
	z.lpfBtn.SetRect(image.Rect(lpfX, subY, lpfX+filterBtnW, subY+btnH))
}

// stepSelectedBand nudges the selected band's gain by delta dB (clamped to the
// curve range) and propagates the change — the mobile stepper's edit path.
// eqBandLabelRects returns the per-band frequency-label rect and level (dB)
// rect, stacked vertically within the band's column: frequency directly above
// the level, with a small gap so the two never overlap, and each confined to
// the band's own column so neighbouring bands never overlap either. Single
// source of truth for both layout (dB-input hit rect) and draw.
func (z *EQPanelZone) eqBandLabelRects(i int) (freq, level image.Rectangle) {
	r := z.rect
	if r.Empty() || i < 0 || i >= len(eqBandDefs) {
		return image.Rectangle{}, image.Rectangle{}
	}
	bandW := r.Dx() / len(eqBandDefs)
	if bandW < 1 {
		bandW = 1
	}
	x0 := r.Min.X + i*bandW
	x1 := x0 + bandW
	if i == len(eqBandDefs)-1 {
		x1 = r.Max.X
	}
	dbH := z.dbRowH
	if dbH <= 0 {
		dbH = ExpandHitArea(Profile().EQDBInputH)
		if floor := eqDBInputTouchFloor(Profile().Density()); dbH < floor {
			dbH = floor
		}
	}
	lh := TextHeight()
	levelTop := r.Max.Y - dbH
	level = image.Rect(x0+1, levelTop, x1-1, r.Max.Y)
	freqBottom := levelTop - 2 // gap so freq sits stacked above the level
	freq = image.Rect(x0+1, freqBottom-lh, x1-1, freqBottom)
	return freq, level
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

	// Phase 5 audio-panel redesign: mute button + dB input touch-min
	// compliance. The visible chrome floors at a per-density touch
	// target so Comfortable users get a 36-px tap area and Spacious
	// users 44 px — even when the underlying slider strip is shorter
	// (EQSliderH=14 on Comfortable / 28 on Spacious). ExpandHitArea
	// alone is not enough because MinTarget is 0 on desktop densities;
	// the explicit per-density floor below mirrors the plan's intent
	// ("Comfortable ≥ BtnHeightMD², Spacious ≥ BtnHeightLG²").
	muteBtnH := ExpandHitArea(Profile().EQSliderH)
	if floor := eqMuteTouchFloor(Profile().Density()); muteBtnH < floor {
		muteBtnH = floor
	}
	muteBtnW := 16
	if Profile().IsMobile() {
		muteBtnW = bandW - 4
	}
	dbInputH := ExpandHitArea(Profile().EQDBInputH)
	if floor := eqDBInputTouchFloor(Profile().Density()); dbInputH < floor {
		dbInputH = floor
	}
	z.dbRowH = dbInputH

	// dB inputs at the bottom (stacked under the freq label), mute buttons
	// shifted up above them. The dB-input rect comes from eqBandLabelRects so
	// the tap target matches the drawn level number exactly.
	muteBtnY := (r.Max.Y - dbInputH) - muteBtnH - 1
	// Clamp the (bottom-anchored) mute row so it never creeps above the
	// content area into the sticky bar in a short panel — otherwise a mute
	// hit area would overlap the channel pill and steal its tap.
	if top := z.contentRect().Min.Y; muteBtnY < top {
		muteBtnY = top
	}

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
			_, level := z.eqBandLabelRects(i)
			z.eqDBInputs[i].Rect = level
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
	activeTab := z.tabState.ActiveTab()
	// EQ-band controls (curve, mute, dB inputs) are only live on TabEQ.
	// Without this gate, switching to TabSynth (or any other tab) leaves
	// the EQ band hit areas active over the audio panel rect, stealing
	// taps from buttons drawn by the other tabs — the root cause of the
	// "Save/Save As/Reset are on top of other buttons that trigger if
	// clicked" report.
	eqTab := activeTab == TabEQ

	// Audio-panel input isolation (see plan: hey-please-review-and-
	// sharded-harbor.md). The EQ panel is visually opaque; for the
	// DrumView tree's z-priority dispatch to be load-bearing the
	// panel must also be opaque to input. Register a full-bounds
	// catch-all at ZEQPanel — per-control hit areas inside the same
	// zone sit at ZEQPanel+1 / +2 / +3 and still win on overlap
	// (HitIndex.At sorts exact-rect hits then z descending). Without
	// this, a tap in chrome whitespace falls through to lower-z
	// zones (timeline at 110-111, row rack at 120) and the user
	// hits the canonical "drag a synth knob also pans the timeline"
	// leak.
	if !z.rect.Empty() {
		z.hitAreas = append(z.hitAreas, NewInputCaptureHitArea(z.rect, zIdx, "eq-panel-capture"))
	}

	// EQ curve handle drag area (both desktop and mobile).
	if eqTab && !z.rect.Empty() {
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
	if eqTab {
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
			if cr := z.bodyRect(); !cr.Empty() {
				z.hitAreas = append(z.hitAreas, HitArea{
					Rect:    cr,
					ZIndex:  zIdx, // below chrome (zIdx+1) so the chrome wins
					Handler: &cursorScrubHandler{zone: z},
					Tag:     "eq-audio-cursor-scrub",
				})
			}
		}
	}

	// HPF/LPF are content-rect items on TabEQ only. The visible chrome is a
	// compact 28×18 pill; on mobile the hit area is expanded to the touch-min
	// via Touch + ClipRect (not by growing the visual, which would crowd the
	// curve) so a finger can reliably toggle the filters.
	if eqTab {
		mobile := Profile().IsMobile()
		filterHit := func(btn *Button, tag string) {
			r := btn.Rect()
			if r.Empty() {
				return
			}
			ha := HitArea{Rect: r, ZIndex: zIdx + 1, Handler: &buttonHitAdapter{btn: btn}, Tag: tag}
			if mobile {
				ha.Touch = true
				ha.ClipRect = expandToTouchMin(r)
			}
			z.hitAreas = append(z.hitAreas, ha)
		}
		filterHit(z.hpfBtn, "eq-hpf-btn")
		filterHit(z.lpfBtn, "eq-lpf-btn")
	}

	// Per-band dB text inputs — only live when the EQ tab is active. On mobile
	// their rects are cleared (replaced by the stepper) so this loop registers
	// nothing there.
	if eqTab {
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

	// Hit-test band handles (position from the shared eqBandHandlePos so the
	// hit target matches exactly where the handle is drawn).
	for i := range eqBandDefs {
		hx, hy := z.eqBandHandlePos(i)
		gain := 0.0
		if i < len(z.bandGainsDB) {
			gain = z.bandGainsDB[i]
		}
		dx := x - hx
		dy := y - hy
		if dx*dx+dy*dy <= hitRadius*hitRadius {
			z.curveDragBand = i
			z.curveSelectBand(i)
			z.setDragLabel(fmt.Sprintf("%+.1f dB", gain), hx, hy)
			return InputCaptured
		}
	}

	// Mobile: forgiving column grab. A finger can't reliably land on a
	// 26-px handle among 10 packed bands, so a press anywhere in the curve
	// plot (above the mute/dB chrome) selects the nearest band by frequency
	// column and begins a drag. Desktop keeps precise-handle grab — a mouse
	// is precise and column grab would be ambiguous where band handles
	// overlap. The gain is NOT changed on press (only on drag), so a tap
	// that doesn't move leaves the value untouched.
	if Profile().IsMobile() {
		// The plot rect already stops above the bottom label/stepper strip,
		// so a press inside it is safely clear of the dB/mute chrome.
		if pt.In(z.eqPlotRect()) {
			band := z.nearestBandByX(x)
			z.curveDragBand = band
			z.curveSelectBand(band)
			gain := 0.0
			if band < len(z.bandGainsDB) {
				gain = z.bandGainsDB[band]
			}
			hx, hy := z.eqBandHandlePos(band)
			z.setDragLabel(fmt.Sprintf("%+.1f dB", gain), hx, hy)
			return InputCaptured
		}
	}
	return InputIgnored
}

// expandToTouchMin grows a rect symmetrically so each axis is at least the
// current profile's MinTarget (a no-op on desktop densities where MinTarget
// is 0). Used as a HitArea.ClipRect to give compact chrome a finger-friendly
// touch target without enlarging the visual.
func expandToTouchMin(r image.Rectangle) image.Rectangle {
	m := Profile().MinTarget
	if m <= 0 {
		return r
	}
	if w := r.Dx(); w < m {
		g := (m - w + 1) / 2
		r.Min.X -= g
		r.Max.X += g
	}
	if h := r.Dy(); h < m {
		g := (m - h + 1) / 2
		r.Min.Y -= g
		r.Max.Y += g
	}
	return r
}

// nearestBandByX returns the band index whose handle is closest to x.
func (z *EQPanelZone) nearestBandByX(x int) int {
	best, bestD := 0, math.MaxInt32
	for i, def := range eqBandDefs {
		cx := freqToX(math.Sqrt(def.loHz*def.hiHz), z.rect)
		d := x - cx
		if d < 0 {
			d = -d
		}
		if d < bestD {
			bestD, best = d, i
		}
	}
	return best
}

func (h *curveHandleHitAdapter) OnDrag(x, y int) {
	z := h.zone

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
	db := yToGainDB(y, z.eqPlotRect())
	if band < len(z.bandGainsDB) {
		z.bandGainsDB[band] = db
	}
	z.syncDBInputText(band)
	hx, hy := z.eqBandHandlePos(band)
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
	if z.callbacks.OnEQBandCommit != nil {
		z.callbacks.OnEQBandCommit()
	}
}

// setDragLabel positions the dB/Hz label 16px above the handle, flipping
// below if near the top of the rect.
func (z *EQPanelZone) setDragLabel(text string, hx, hy int) {
	z.curveDragDBText = text
	z.curveDragLabelX = hx
	labelY := hy - 16
	// Flip the label below the handle when it would otherwise be clipped at
	// the top of the plot region (handles are clamped to the plot, not the
	// full panel, so the threshold is the plot top — not z.rect.Min.Y).
	if labelY < z.eqPlotRect().Min.Y+4 {
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
	allLabels   []string // display labels aligned 1:1 with allOnClicks
	deferredTap DeferredTap
}

// buildCallbacks populates allOnClicks for the Master + per-row entries.
// Geometry (visible-row count, view rect, button rects, hit rect) is set up
// in layoutFor which runs from Layout(anchor, screenBounds) so the portal
// can resize the menu when the viewport changes.
func (o *eqChannelDropdownOverlay) buildCallbacks() {
	o.buttons = nil
	o.allOnClicks = nil
	o.allLabels = nil
	z := o.zone

	// Master is not a valid target on the Synth / Sampler tabs (you edit an
	// instrument's recipe or chop a sample — neither applies to the master
	// bus), so hide it there. Every other tab keeps it as the first entry.
	hideMaster := false
	if z.tabState != nil {
		t := z.tabState.ActiveTab()
		hideMaster = t == TabSynth || t == TabSampler
	}
	if !hideMaster {
		o.allLabels = append(o.allLabels, "Master")
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
	}

	if z.callbacks.ActiveRows != nil {
		for _, row := range z.callbacks.ActiveRows() {
			instID := row.Instrument
			name := row.Name
			o.allLabels = append(o.allLabels, name)
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
		if idx < len(o.allLabels) {
			label = o.allLabels[idx]
			if len(label) > 12 {
				label = label[:12] + "…"
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
		if idx < len(o.allLabels) {
			label = o.allLabels[idx]
			if len(label) > 12 {
				label = label[:12] + "…"
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
	// Use the actual laid-out dB/stepper row height so the freq labels sit
	// directly above the real bottom row on every density / platform.
	dbInputH := z.dbRowH
	if dbInputH <= 0 {
		dbInputH = Profile().EQDBInputH
	}
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

		// Row 1: Frequency band label — stacked above the level, centred and
		// scaled to fit its own column so it never overlaps the level number
		// below it or the neighbouring band's label.
		freqR, _ := z.eqBandLabelRects(i)
		z.drawEQCellText(dst, eqCenterLabels[i], freqR, colTextSecondary)

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

		// Row 2: level (dB) value — stacked under the frequency label, colour-
		// coded (cyan boost / gray cut), centred + scaled to fit the column.
		if z.eqDBInputs[i] != nil && !z.eqDBInputs[i].Rect.Empty() {
			ti := z.eqDBInputs[i]
			if ti.Focused() {
				ti.Draw(dst)
			} else {
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
				z.drawEQCellText(dst, ti.Value(), ti.Rect, labelCol)
			}
		}
	}
}

// drawEQCellText draws txt centred within cell, scaling the text down (to a
// legible floor) when it is wider than the cell so a per-band label never
// spills into the neighbouring column.
func (z *EQPanelZone) drawEQCellText(dst *ebiten.Image, txt string, cell image.Rectangle, col color.Color) {
	if cell.Empty() || txt == "" {
		return
	}
	tw := TextWidth(txt)
	th := TextHeight()
	scale := 1.0
	if avail := cell.Dx() - 2; tw > avail && tw > 0 {
		scale = float64(avail) / float64(tw)
		if scale < 0.6 {
			scale = 0.6
		}
	}
	dw := int(float64(tw) * scale)
	dh := int(float64(th) * scale)
	tx := cell.Min.X + (cell.Dx()-dw)/2
	ty := cell.Min.Y + (cell.Dy()-dh)/2
	if scale >= 1.0 {
		DrawTextColorAt(dst, txt, tx, ty, col)
	} else {
		DrawTextColorAtScale(dst, txt, tx, ty, col, scale)
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
	r := z.eqPlotRect()
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

	// Band handles with glow effect and border ring. Position comes from the
	// shared eqBandHandlePos so the handle is drawn exactly where it's hit-
	// tested, clamped within the plot (never over the bottom labels).
	mobileSel := Profile().IsMobile()
	for i := range eqBandDefs {
		hx, hy := z.eqBandHandlePos(i)
		hr := eqHandleRadius
		col := colEQHandle
		isDragging := z.curveDragBand == i
		// On mobile the stepper edits the selected band — emphasise its handle
		// so the user can see which band the −/+ buttons control.
		if isDragging || (mobileSel && i == z.eqSelectedBand) {
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
	r := z.eqPlotRect()
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
	if z.callbacks.OnEQBandCommit != nil {
		z.callbacks.OnEQBandCommit()
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

// eqMuteTouchFloor returns the minimum mute-button hit-rect height for
// the active density. Phase 5 audio-panel redesign: the plan called for
// Compact 0 (no floor) / Comfortable BtnHeightMD / Spacious BtnHeightLG
// so the visible chrome scales to discover-ability without forcing
// desktop power users into chunky buttons. ExpandHitArea only floors at
// Profile().MinTarget (which is 0 on Compact / Comfortable) — this
// helper layers the per-density floor on top.
func eqMuteTouchFloor(d Density) int {
	switch d {
	case DensityCompact:
		return 0
	case DensityComfortable:
		return BtnHeightMD
	case DensitySpacious:
		return BtnHeightLG
	}
	return 0
}

// eqDBInputTouchFloor mirrors eqMuteTouchFloor for the per-band dB text
// input. The numeric entry needs the same tap area as the mute toggle
// above it so finger-driven tweaks don't miss-tap each other.
func eqDBInputTouchFloor(d Density) int {
	switch d {
	case DensityCompact:
		return 0
	case DensityComfortable:
		return BtnHeightMD
	case DensitySpacious:
		return BtnHeightLG
	}
	return 0
}

// snapshotLevelsIconRow caches the icon-row rects + formatted aggregate
// values so a subsequent long-press can resolve the slot without
// re-reading analyzer state. Called only when the Levels-tab draw path
// picks the icon-row cascade mode. Empty rects when another mode is
// active — checked by levelsAggregateSlotForPoint.
func (z *EQPanelZone) snapshotLevelsIconRow(state *analyzer.State) {
	z.levelsHeadroomRect = image.Rectangle{}
	z.levelsClipsRect = image.Rectangle{}
	z.levelsLoudestRect = image.Rectangle{}
	z.levelsAggSnapshot = LevelsAggregate{}
	if state == nil {
		return
	}
	rect := z.contentRect()
	if rect.Empty() {
		return
	}
	ldv := Profile().DensityValues()
	readoutWFull := ldv.LevelsReadoutWFull
	readoutWIcons := ldv.LevelsReadoutWIcons
	// Only the icon-row mode populates rects. Mirrors the cascade in
	// drawLevelsMultiChannel — keep the threshold expressions in sync.
	if rect.Dx() >= 2*readoutWFull || rect.Dx() < readoutWFull+readoutWIcons {
		return
	}
	colRect := image.Rect(rect.Max.X-readoutWIcons+2, rect.Min.Y+4, rect.Max.X-2, rect.Max.Y-4)
	z.levelsHeadroomRect, z.levelsClipsRect, z.levelsLoudestRect = levelsAggregateIconRects(colRect)
	z.levelsAggSnapshot = levelsAggregatesValues(state, z.levelsLatches)
}

// LevelsAggregateSlotForPoint reports which Levels icon-row slot
// contains (x, y), or "" if none.
func (z *EQPanelZone) LevelsAggregateSlotForPoint(x, y int) string {
	pt := image.Pt(x, y)
	if pt.In(z.levelsHeadroomRect) {
		return "headroom"
	}
	if pt.In(z.levelsClipsRect) {
		return "clips"
	}
	if pt.In(z.levelsLoudestRect) {
		return "loudest"
	}
	return ""
}

// ShowLevelsAggregateTooltip opens a tooltip overlay describing the
// given aggregate slot. Returns true when the slot was resolved and the
// tooltip was opened. Called from the game's GestureLongPress dispatch
// (mobile) and from updateLevelsAggregateHover (desktop hover-dwell).
func (z *EQPanelZone) ShowLevelsAggregateTooltip(slot string) bool {
	if z.portal == nil {
		return false
	}
	var anchor image.Rectangle
	switch slot {
	case "headroom":
		anchor = z.levelsHeadroomRect
	case "clips":
		anchor = z.levelsClipsRect
	case "loudest":
		anchor = z.levelsLoudestRect
	default:
		return false
	}
	if anchor.Empty() {
		return false
	}
	text := levelsAggregateTooltipText(slot, z.levelsAggSnapshot)
	if text == "" {
		return false
	}
	z.portal.Open(PortalEntry{
		ID:      "levels-agg-tt",
		Owner:   z,
		Overlay: NewTooltipOverlay(text),
		Modal:   false,
		Anchor:  anchor,
	})
	return true
}

// HandleLevelsAggregateLongPress is the entry point from the game's
// GestureLongPress dispatch: resolves (x, y) to a Levels icon-row slot
// (if any) and surfaces the tooltip. Returns true when the press was
// consumed.
func (z *EQPanelZone) HandleLevelsAggregateLongPress(x, y int) bool {
	if z.tabState.ActiveTab() != TabMeters {
		return false
	}
	slot := z.LevelsAggregateSlotForPoint(x, y)
	if slot == "" {
		return false
	}
	return z.ShowLevelsAggregateTooltip(slot)
}
