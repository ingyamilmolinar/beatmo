package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// EQCallbacks is the panel's only channel to DrumView and the audio engine; zones never reference Game or each other.
type EQCallbacks struct {
	OnGainChange func(band int, db float64)
	// OnEQBandCommit fires once per settled band-gain gesture (emit + one undo step); the live write is OnGainChange.
	OnEQBandCommit   func()
	OnMuteToggle     func(band int)
	OnChannelChange  func(channelID channel.ID)
	OnToggleHPF      func()
	OnToggleLPF      func()
	OnApplyEQ        func()
	OnOpenValueWheel func(band int)
	AnalyzerSnapshot func(ch channel.ID) audio.AnalyzerSnapshot
	ActiveRows       func() []*DrumRow

	// HPF/LPF state accessors and cutoff change callbacks.
	HPFEnabled        func() bool
	HPFCutoffHz       func() float64
	LPFEnabled        func() bool
	LPFCutoffHz       func() float64
	OnHPFCutoffChange func(hz float64)
	OnLPFCutoffChange func(hz float64)

	OnChannelDropdownClose func()

	DrawWaveform func(dst *ebiten.Image)

	AnalyzerState func() *analyzer.State

	SpectrumRowFacts func(*analyzer.State) []rowFacts

	// AnalyzerMetricsOnly is a scalar-only state for the Levels tab: the full one allocates slices per frame,
	// which drove a long-session OOM on WASM. May be nil.
	AnalyzerMetricsOnly func() *analyzer.State

	OnFreezeToggle func() bool

	OnTabChange func(tab PanelTab)

	// OnLeaveSynthTab flushes the debounced wheel-nudge commit: the debounce ticks only while the Synth tab
	// updates, so leaving within its pause would drop the pending undo step.
	OnLeaveSynthTab func()

	// OnLeaveSamplerTab: same contract as OnLeaveSynthTab for the Sampler wheel nudge.
	OnLeaveSamplerTab func()

	// BeatGridFrac returns beat-marker X positions in [0,1) for the Wave tab; nil disables them.
	BeatGridFrac func() []float64

	// The panel owns no synth/sampler state; these callbacks delegate the tab to DrumView.
	OnSynthTabLayout func(contentR image.Rectangle)

	DrawSynthTab func(dst *ebiten.Image, contentR image.Rectangle)

	// ReleaseSynthSprites frees the Synth tab's GPU caches on tab-hide instead of waiting for a GC finalizer.
	ReleaseSynthSprites func()

	SynthTabHitAreas func() []HitArea

	// TrailingActionsForTab returns the mobile sticky-bar actions ([Preview][Save][⋯]) for a tab, or nil.
	TrailingActionsForTab func(tab PanelTab) []*Button

	// SynthTabUpdate reports whether a scroll position changed, so the zone re-lays-out next frame.
	SynthTabUpdate func() bool

	// SynthTabDisabled greys the Synth pill for a WAV-sample instrument (nothing to shape). Nil = never.
	SynthTabDisabled func() bool

	OnSamplerTabLayout func(contentR image.Rectangle)
	DrawSamplerTab     func(dst *ebiten.Image, contentR image.Rectangle)
	SamplerTabHitAreas func() []HitArea

	// SamplerTabUpdate reports whether the editor just closed, so the zone re-lays-out next frame.
	SamplerTabUpdate func() bool

	// IsHiddenForInput makes HitAreas() return nil while the panel is hidden: a second gate behind the tree's
	// visibility predicate, so dropping that one cannot re-open the Pads-tab input leak.
	IsHiddenForInput func() bool
}

type EQPanelZone struct {
	rect       image.Rectangle
	needLayout bool
	// layouts counts Layout calls (recalcButtons' layout gate is judged by it).
	layouts   int
	callbacks EQCallbacks
	portal    *OverlayPortal // set by tree wiring

	// UI elements
	eqMuteBtns       []*Button       // per-band mute
	stickyBar        *AudioStickyBar // chrome strip: channel, tabs, freeze, close
	waveControls     *waveControls
	levelsControls   *levelsControls
	spectrumControls *spectrumControls
	hpfBtn           *Button // EQ-tab content item (sub-strip above curve)
	lpfBtn           *Button // EQ-tab content item (sub-strip above curve)

	// Visual state
	eqBandVals []float64
	tabState   *PanelTabState

	// EQ curve state
	curveDragBand   int
	curveDragFilter hooks.EQFilter
	// curveDragFilterHz is the cutoff the filter drag last applied (NaN at press); OnDrag runs every frame and
	// only a moved cutoff may reach the audio graph.
	curveDragFilterHz float64

	// Desktop-only hover state so the handle under the pointer blooms; curveHoverBand is -1 for none.
	curveHoverBand   int
	curveHoverFilter hooks.EQFilter
	curveDirty       bool
	curveCache       []audio.FreqResponsePoint
	// curveSprite caches the static curve content (one blit per frame); handles and drag labels stay live.
	curveSprite TraceSprite
	curveRev    int
	// The band chrome and the labels are cached separately because the labels paint OVER the live bars.
	eqBackdropSprite    TraceSprite
	eqLabelsSprite      TraceSprite
	spectrumCurveSprite TraceSprite
	// Static Spectrum chrome (dB grid behind the trace, labels above the bars): re-rendered only on resize,
	// theme or locale change.
	spectrumGridSprite   TraceSprite
	spectrumLabelsSprite TraceSprite

	// spectrumBannerHold keeps the headline steady: the top finding alternates with the beat and an
	// undwelled sentence strobes too fast to read.
	spectrumBannerHold bannerHold

	// Real-time dB label during curve handle drag
	curveDragDBText string // e.g., "+6.0 dB" or "" when not dragging
	curveDragLabelX int
	curveDragLabelY int

	// Spectrum tab peak-hold state
	spectrumPeaks SpectrumPeakState

	// levelsLatch serves the zero-instrument fallback (Master only); levelsLatches holds one latch per channel.
	levelsLatch   LevelsLatch
	levelsLatches *MultiLevelsLatch
	// levelsLastTickFrame detects a hidden Levels tab (a gap of more than one frame), so re-entry re-seeds
	// the latches from live audio instead of decaying from a stale held value.
	levelsLastTickFrame int64
	// lastClipCountsByID turns per-channel clip totals into deltas for the audio package's rolling 10 s window.
	lastClipCountsByID map[channel.ID]int

	// Scope tab: delegated to its own zone for layout/draw/hit areas.
	chainZone *ChainPanelZone

	// Master channel EQ state (per-row stays in DrumRow)
	bandGainsDB []float64
	bandMuted   []bool

	// Channel dropdown state
	channelOpen   bool
	activeChannel channel.ID
	channelScroll *ScrollBehavior

	// Per-band dB readout row
	dbRowH         int // height of the per-band dB readout row (set in layout)
	eqSelectedBand int // band most recently grabbed (drives the drag highlight)

	dbReadoutRects [10]image.Rectangle // per-band dB readout rects (drive the editor anchor + hit areas)

	// WASM only: desktop's analyzer.Service owns freeze and sets state.Capture.Frozen.
	frozenAnalyzer *analyzer.State

	// frozenByTab pauses each signal tab independently on a held immutable snapshot; the analyzer itself
	// is never frozen.
	frozenByTab map[PanelTab]*analyzer.State

	// Hit areas cache (rebuilt on Layout)
	hitAreas []HitArea

	drawCalls int

	// frameAnalyzerState memoises the analyzer callback for one Draw; frameAnalyzerTab records which builder
	// (metrics-only for Levels) produced it.
	frameAnalyzerState *analyzer.State
	frameAnalyzerTab   PanelTab
	frameAnalyzerValid bool

	// Wave/Spectrum cursor, in zone-local pixels. On mobile cursorPinned keeps it after the finger lifts.
	cursorX      int
	cursorActive bool
	cursorPinned bool

	// The waveform renderer blits a column per pixel but its data refreshes at ~30 Hz, so the trace is cached
	// on the analyzer-state identity; the cursor crosshair is drawn live over it.
	waveCache        *ebiten.Image
	waveKeyW         int
	waveKeyH         int
	waveKeyState     *analyzer.State // held to keep ch/cap pointers valid + as freshness signal
	waveKeyCh        *analyzer.ChannelMetrics
	waveKeyCap       *analyzer.CaptureBuffer
	waveKeyFrozen    bool
	waveKeyTabFrozen bool // per-tab freeze flag folded into the cache key
	waveKeyOK        bool
	// waveSmoothedPeak advances once per analyzer-state refresh (not per frame) so auto-gain eases.
	waveAutoGain     bool
	waveYGain        float64
	waveSmoothedPeak float64
	waveSmoothState  *analyzer.State

	// specMaxDB only rises (raiseDBMax): the axis the bars, curve and grid map through must hold still.
	specMaxDB       float64
	specSmoothState *analyzer.State

	// pinnedWindow is the buffer-derived scale of one instrument's channel, computed once per channel pick.
	pinnedWindow    spectrumDBWindow
	pinnedWindowFor channel.ID
	hasPinnedWindow bool
	// Until settleUntil the buffer estimate is provisional; then the axis re-pins on settleMax, the live peak
	// measured meanwhile. Zero = settled.
	settleUntil time.Time
	settleMaxDB float64
	nowFn       func() time.Time

	// eqMaxDB cannot share the Spectrum anchor: the EQ backdrop reads a fresh audio.AnalyzerSnapshot of
	// linear magnitudes, not the analyzer.State.
	eqMaxDB     float64
	waveKeyGain float64 // quantized effective gain folded into the cache key
	legendOpen  bool

	// Levels icon-row rects, snapshotted at draw so a long-press can show the full value without re-reading
	// analyzer state. Empty in the full and chevron readout modes.
	levelsHeadroomRect image.Rectangle
	levelsClipsRect    image.Rectangle
	levelsLoudestRect  image.Rectangle
	levelsAggSnapshot  LevelsAggregate
}

func (z *EQPanelZone) DrawCallsForTest() int { return z.drawCalls }

// NewEQPanelZone creates a new EQPanelZone with the provided callbacks.
func NewEQPanelZone(cb EQCallbacks) *EQPanelZone {
	z := &EQPanelZone{
		needLayout:     true,
		callbacks:      cb,
		tabState:       NewPanelTabState(),
		curveDragBand:  -1,
		curveHoverBand: -1,
		activeChannel:  channel.Master,
		bandGainsDB:    make([]float64, len(eqBandDefs)),
		bandMuted:      make([]bool, len(eqBandDefs)),
		eqBandVals:     make([]float64, len(eqBandDefs)),
	}
	z.tabState.SetOnLeave(func(prev PanelTab) {
		if prev == TabSynth && z.callbacks.OnLeaveSynthTab != nil {
			z.callbacks.OnLeaveSynthTab()
		}
		if prev == TabSampler && z.callbacks.OnLeaveSamplerTab != nil {
			z.callbacks.OnLeaveSamplerTab()
		}
	})
	z.initButtons()
	z.channelScroll = newScrollBehavior()
	// The first Draw needs its tab's analyzer taps enabled already, or it renders flat.
	EnsureAnalyzersForTab(z.tabState.ActiveTab(), z.activeChannel, z.analyzerInstrumentIDs()...)
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
	onTab := func(tab PanelTab) {
		// The Synth/Sampler pills are greyed under Master, but the raw callback path must refuse too.
		if instrumentTabRequiresInstrument(tab) && z.masterChannelSelected() {
			return
		}
		z.tabState.SetActiveTab(tab)
		EnsureAnalyzersForTab(tab, z.activeChannel, z.analyzerInstrumentIDs()...)
		z.releaseHiddenTabSprites(tab)
		emitAudioPanelStateChanged("tab:" + PanelTabSlug(tab))
		if z.callbacks.OnTabChange != nil {
			z.callbacks.OnTabChange(tab)
		}
	}
	z.stickyBar = NewAudioStickyBar(130, onChannel, onTab)
	if lg := z.stickyBar.LegendBtn(); lg != nil {
		lg.OnClick = func() {
			z.legendOpen = !z.legendOpen
		}
	}

	// ctrlZ sits one above the panel catch-all (130) so header pills win taps over the body.
	const ctrlZ = 131
	z.waveAutoGain = true
	z.waveYGain = 1.0
	z.waveControls = newWaveControls(ctrlZ, func() bool {
		z.waveAutoGain = !z.waveAutoGain
		return z.waveAutoGain
	}, func() bool { return z.toggleTabFreeze(TabWave) })
	z.levelsControls = newLevelsControls(ctrlZ, func() bool { return z.toggleTabFreeze(TabMeters) }, func() {
		if z.levelsLatches != nil {
			z.levelsLatches.Clear()
		}
		audio.ResetClipsWindow()
	})
	z.spectrumControls = newSpectrumControls(ctrlZ, func() bool { return z.toggleTabFreeze(TabSpectrum) })

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

}

// --- Zone interface ---

func (z *EQPanelZone) ID() string { return "eq-panel" }

func (z *EQPanelZone) NeedsLayout() bool { return z.needLayout }

func (z *EQPanelZone) Invalidate() { z.needLayout = true }

func (z *EQPanelZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.needLayout = false
	z.layouts++
	z.layoutButtons()
	if c := z.activeTabControls(); c != nil {
		c.Layout(z.headerRect())
	}
	z.layoutMuteAndDBInputs()
	// Delegate layout to scope zone when Scope tab is active.
	if z.chainZone != nil && z.tabState.ActiveTab() == TabScope {
		z.chainZone.Layout(z.contentRect())
	}
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

func (z *EQPanelZone) ContentRect() image.Rectangle { return z.contentRect() }

// ActiveTab returns the tab currently selected on the panel.
func (z *EQPanelZone) ActiveTab() PanelTab {
	if z.tabState == nil {
		return TabEQ
	}
	return z.tabState.ActiveTab()
}

// SetActiveTab is the programmatic switch (tests, scenes, JS export); user clicks go through the sticky
// bar's onTab closure, and both run the same dispatcher.
func (z *EQPanelZone) SetActiveTab(tab PanelTab) {
	if z.tabState == nil || !panelTabAvailable(tab) {
		return
	}
	if z.tabState.ActiveTab() == tab {
		return
	}
	z.tabState.SetActiveTab(tab)
	z.needLayout = true
	EnsureAnalyzersForTab(tab, z.activeChannel, z.analyzerInstrumentIDs()...)
	z.releaseHiddenTabSprites(tab)
	if z.callbacks.OnTabChange != nil {
		z.callbacks.OnTabChange(tab)
	}
}

// releaseHiddenTabSprites frees other tabs' GPU textures at once rather than at the next GC (which can be
// 20+ s away; on WASM atlas residency is the OOM driver). Both tab-switch paths call it.
func (z *EQPanelZone) releaseHiddenTabSprites(active PanelTab) {
	if active != TabEQ {
		z.curveSprite.Release()
		z.eqBackdropSprite.Release()
		z.eqLabelsSprite.Release()
	}
	if active != TabSpectrum {
		z.spectrumCurveSprite.Release()
		z.spectrumGridSprite.Release()
		z.spectrumLabelsSprite.Release()
	}
	if active != TabSynth && z.callbacks.ReleaseSynthSprites != nil {
		z.callbacks.ReleaseSynthSprites()
	}
}

func (z *EQPanelZone) Update() {
	z.updateAudioPanelCursor()

	z.updateEQHandleHover()

	if z.tabState.ActiveTab() == TabSynth && z.callbacks.SynthTabUpdate != nil {
		if z.callbacks.SynthTabUpdate() {
			z.needLayout = true
		}
	}

	// Sampler-tab numeric editor (blur-to-commit). Mirrors the synth branch.
	if z.tabState.ActiveTab() == TabSampler && z.callbacks.SamplerTabUpdate != nil {
		if z.callbacks.SamplerTabUpdate() {
			z.needLayout = true
		}
	}
}

// Filter handles take priority over band handles, as in OnPress, with the same hit geometry, so the
// highlighted handle is exactly the grabbable one.
func (z *EQPanelZone) updateEQHandleHover() {
	if z.tabState.ActiveTab() != TabEQ || Profile().IsMobile() || z.rect.Empty() {
		z.curveHoverBand = -1
		z.curveHoverFilter = hooks.FilterNone
		return
	}
	mx, my := cursorPosition()
	if f := z.filterHandleAtPoint(mx, my); f != hooks.FilterNone {
		z.curveHoverFilter = f
		z.curveHoverBand = -1
		return
	}
	z.curveHoverFilter = hooks.FilterNone
	z.curveHoverBand = z.bandHandleAtPoint(mx, my)
}

func (z *EQPanelZone) HitAreas() []HitArea {
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
			z.drawWaveformCached(screen, cr, state, ch, cap, beatGrid)
			if z.cursorActive || z.cursorPinned {
				drawWaveformCursor(screen, cr, ch, cap, z.cursorX)
			}
		} else if z.callbacks.DrawWaveform != nil {
			z.callbacks.DrawWaveform(screen)
		} else {
			// With no analyzer state, still draw the idle axis chrome so the panel never reads as dead.
			drawAnalyzerWaveform(screen, cr, nil, nil, nil, 1.0, false, false)
		}
	case TabSpectrum:
		// Every spectrum layer re-derives its geometry from the reduced rect, so they stay aligned without
		// threading the banner height through.
		bannerRect, cr := spectrumBannerSplit(z.bodyRect())
		// Log only: a spectrum is read logarithmically, so LIN was a strictly worse view.
		scale := freqScaleLog
		if state := z.getAnalyzerStateForTab(activeTab); state != nil {
			ch, _ := z.resolveChannel(state)
			var rows []rowFacts
			if z.callbacks.SpectrumRowFacts != nil {
				rows = z.callbacks.SpectrumRowFacts(state)
			}
			banner := z.spectrumBannerHold.update(
				resolveSpectrumBanner(spectrumFindings(ch, rows)), time.Now())
			// The emphasis lane sits BEHIND the data, so it draws first.
			if barRect, _, _ := spectrumPanelRows(cr); banner.Band >= 0 {
				drawSpectrumBandEmphasis(screen, barRect, banner.Band)
			}
			drawAnalyzerSpectrumImpl(screen, cr, ch, &z.spectrumPeaks, scale, spectrumChromeOpts{
				curve:  &z.spectrumCurveSprite,
				grid:   &z.spectrumGridSprite,
				labels: &z.spectrumLabelsSprite,
				detail: z.spectrumControls != nil && z.spectrumControls.Detail(),
				dbWin:  z.resolveSpectrumWindow(state, ch),
			})
			drawSpectrumBanner(screen, bannerRect, banner)
			if z.cursorActive || z.cursorPinned {
				drawSpectrumCursor(screen, cr, ch, z.cursorX)
			}
		} else {
			drawAnalyzerSpectrumWithScale(screen, cr, nil, &z.spectrumPeaks, scale)
		}
	case TabMeters:
		if state := z.getAnalyzerStateForTab(activeTab); state != nil {
			if z.levelsLatches == nil {
				z.levelsLatches = NewMultiLevelsLatch()
			}
			if z.lastClipCountsByID == nil {
				z.lastClipCountsByID = map[channel.ID]int{}
			}
			// A gap since the last tick means the tab was hidden and the held values are stale: re-seed from live audio.
			if uiAnimFrame-z.levelsLastTickFrame != 1 {
				z.levelsLatches.Reset()
				z.levelsLatch.Reset()
			}
			z.levelsLastTickFrame = uiAnimFrame
			if z.tabFrozen(TabMeters) {
				// Frozen: hold the snapshot's reading at once instead of decaying; no clips count while paused.
				z.applyLevelsFreezeHold(state)
			} else {
				for i := range state.Instruments {
					inst := &state.Instruments[i]
					z.levelsLatches.Get(inst.ID).Update(inst.ClipCount, inst.PeakDB, inst.RMSDB)
					if d := inst.ClipCount - z.lastClipCountsByID[inst.ID]; d > 0 {
						audio.PushClipDelta(d)
					}
					z.lastClipCountsByID[inst.ID] = inst.ClipCount
				}
				z.levelsLatches.Get(channel.Master).Update(state.Master.ClipCount, state.Master.PeakDB, state.Master.RMSDB)
				if d := state.Master.ClipCount - z.lastClipCountsByID[channel.Master]; d > 0 {
					audio.PushClipDelta(d)
				}
				z.lastClipCountsByID[channel.Master] = state.Master.ClipCount
				z.levelsLatch.Update(state.Master.ClipCount, state.Master.PeakDB, state.Master.RMSDB)
			}
			var visibleIDs map[channel.ID]bool
			if z.callbacks.ActiveRows != nil {
				visibleIDs = audibleInstrumentIDs(z.callbacks.ActiveRows())
			}
			drawLevelsMultiChannel(screen, z.bodyRect(), state, z.levelsLatches, visibleIDs)
			z.snapshotLevelsIconRow(state, visibleIDs)
		} else {
			// Before the first analyzer tick (desktop before Play), draw the strip chrome at -inf from the row set,
			// as the browser does, instead of an empty rect.
			drawLevelsMultiChannel(screen, z.bodyRect(), z.idleLevelsState(), z.levelsLatches, nil)
		}
	case TabScope:
		if z.chainZone != nil {
			z.chainZone.Draw(screen)
		}
	case TabSynth:
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

	if c := z.activeTabControls(); c != nil {
		c.SyncFreeze(z.tabFrozen(activeTab))
		if wc, ok := c.(*waveControls); ok {
			wc.SyncAuto(z.waveAutoGain)
		}
		c.Draw(screen)
	}

	if z.tabState.ActiveTab() == TabEQ {
		if z.hpfBtn != nil {
			hpfActive := z.callbacks.HPFEnabled != nil && z.callbacks.HPFEnabled()
			drawPillTabAt(screen, z.hpfBtn, hpfActive)
		}
		if z.lpfBtn != nil {
			lpfActive := z.callbacks.LPFEnabled != nil && z.callbacks.LPFEnabled()
			drawPillTabAt(screen, z.lpfBtn, lpfActive)
		}
	}

	// Sticky bar drawn last so chrome is never occluded by band overlays.
	if z.stickyBar != nil {
		// Refreshed every frame so the pills follow the channel even when nothing re-laid out.
		z.stickyBar.SetSynthDisabled(z.synthTabDisabled())
		z.stickyBar.SetSamplerDisabled(z.samplerTabDisabled())
		z.stickyBar.Draw(screen, activeTab)
	}
	if z.legendOpen && z.stickyBar != nil {
		if lg := z.stickyBar.LegendBtn(); lg != nil && !lg.Rect().Empty() {
			drawAudioPanelLegend(screen, lg.Rect(), z.rect.Max.X, activeTab)
		}
	}

	drawRect(screen, z.rect, colButtonBorder, false)
}

func (z *EQPanelZone) HandleKey(k ebiten.Key) InputResult { return InputIgnored }

func (z *EQPanelZone) HandleChars(_ []rune) InputResult { return InputIgnored }

// toggleButtonLabel returns the display text for the EQ/Wave toggle button.
func (z *EQPanelZone) toggleButtonLabel() string {
	return PanelTabLabel(z.tabState.ActiveTab())
}

func (z *EQPanelZone) contentRect() image.Rectangle {
	return image.Rect(z.rect.Min.X, z.rect.Min.Y+stickyBarHeight(), z.rect.Max.X, z.rect.Max.Y)
}

func (z *EQPanelZone) activeTabControls() tabControls {
	switch z.tabState.ActiveTab() {
	case TabWave:
		if z.waveControls != nil {
			return z.waveControls
		}
	case TabMeters:
		if z.levelsControls != nil {
			return z.levelsControls
		}
	case TabSpectrum:
		if z.spectrumControls != nil {
			return z.spectrumControls
		}
	}
	return nil
}

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

func (z *EQPanelZone) bodyRect() image.Rectangle {
	cr := z.contentRect()
	return image.Rect(cr.Min.X, cr.Min.Y+z.controlHeaderH(), cr.Max.X, cr.Max.Y)
}

const hpfLPFSubStripH = 22

func (z *EQPanelZone) hpfStripH() int {
	if Profile().IsMobile() {
		return 40
	}
	return hpfLPFSubStripH
}

func (z *EQPanelZone) eqCurveRect() image.Rectangle {
	cr := z.contentRect()
	if z.tabState != nil && z.tabState.ActiveTab() == TabEQ {
		return image.Rect(cr.Min.X, cr.Min.Y+z.hpfStripH(), cr.Max.X, cr.Max.Y)
	}
	return cr
}

// The gain plot stops above this strip so handles never cover the labels.
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

// Mapping gain to Y inside this rect, not the whole panel, keeps handles out from under the sticky bar at
// high gain and off the labels at low gain.
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

// Exactly the gain-plot area, so the bars can never enter the label rows.
func (z *EQPanelZone) eqLiveBarsRect() image.Rectangle {
	return z.eqPlotRect()
}

func (z *EQPanelZone) eqLabelStripRect() image.Rectangle {
	return image.Rect(z.rect.Min.X, z.eqLabelStripTop(), z.rect.Max.X, z.rect.Max.Y)
}

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

func (z *EQPanelZone) curveSelectBand(i int) {
	if i >= 0 && i < len(eqBandDefs) {
		z.eqSelectedBand = i
	}
}

// The one source of a handle's centre, shared by draw and hit-test so they cannot disagree.
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
	// dB readouts derive live from bandGainsDB; no input-text sync needed.
}

// ActiveChannel returns the current EQ channel ID.
func (z *EQPanelZone) ActiveChannel() channel.ID { return z.activeChannel }

// Scenes and tests set the channel directly; users pick through the dropdown. The zero ID is ignored.
func (z *EQPanelZone) SetActiveChannel(id channel.ID) {
	if id.IsZero() {
		return
	}
	z.setActiveChannelID(id)
	if z.stickyBar != nil && z.stickyBar.ChannelBtn() != nil {
		if id == channel.Master {
			z.stickyBar.ChannelBtn().Text = i18n.T(i18n.KeyMaster)
		} else {
			z.stickyBar.ChannelBtn().Text = id.String()
		}
	}
}

// Peaks/RMS are -Inf (what the analyzer computes for silence); the zero value would draw full-scale meters.
func (z *EQPanelZone) idleLevelsState() *analyzer.State {
	neg := math.Inf(-1)
	st := &analyzer.State{
		Master: analyzer.ChannelMetrics{ID: channel.Master, PeakDB: neg, RMSDB: neg, TruePeakDB: neg, HeadroomDB: math.Inf(1)},
	}
	if z.callbacks.ActiveRows != nil {
		for _, r := range z.callbacks.ActiveRows() {
			if r == nil || r.Instrument == "" {
				continue
			}
			st.Instruments = append(st.Instruments, analyzer.InstrumentMetrics{
				ID:     channel.Of(r.Instrument),
				Name:   r.Name,
				PeakDB: neg,
				RMSDB:  neg,
			})
		}
	}
	return st
}

// Derived every frame so the pill cannot disagree with the content: Synth/Sampler under Master show the
// fallback row, and instrument channels show the row name, never the raw id.
func (z *EQPanelZone) refreshChannelPillLabel() {
	if z.stickyBar == nil || z.stickyBar.ChannelBtn() == nil {
		return
	}
	z.stickyBar.ChannelBtn().Text = z.channelPillLabel()
}

func (z *EQPanelZone) channelPillLabel() string {
	id := z.activeChannel
	if z.tabState != nil {
		if t := z.tabState.ActiveTab(); (t == TabSynth || t == TabSampler) && (id.IsMaster()) {
			// Same fallback as synthTabActiveInstrument: the first row with an instrument.
			if z.callbacks.ActiveRows != nil {
				for _, r := range z.callbacks.ActiveRows() {
					if r != nil && r.Instrument != "" {
						id = channel.Of(r.Instrument)
						break
					}
				}
			}
		}
	}
	if id.IsMaster() {
		return i18n.T(i18n.KeyMaster)
	}
	label := id.String()
	if z.callbacks.ActiveRows != nil {
		for _, r := range z.callbacks.ActiveRows() {
			if r != nil && channel.Of(r.Instrument) == id && r.Name != "" {
				label = r.Name
				break
			}
		}
	}
	return label
}

// ChannelDropdownOpen returns whether the channel dropdown is open.
func (z *EQPanelZone) ChannelDropdownOpen() bool {
	return z.channelOpen
}

func (z *EQPanelZone) OpenChannelDropdown() {
	if z.channelOpen {
		return
	}
	z.channelOpen = true
	z.buildChannelDropdown()
}

// --- Layout helpers ---

func instrumentTabRequiresInstrument(tab PanelTab) bool {
	return tab == TabSynth || tab == TabSampler
}

func (z *EQPanelZone) masterChannelSelected() bool {
	return z.ActiveChannel() == channel.Master
}

// Never true while the Synth tab is active: the gate stops ENTERING under Master, but a programmatic
// open resolves the first-row fallback and must not draw its own pill as blocked.
func (z *EQPanelZone) synthTabDisabled() bool {
	if z.tabState != nil && z.tabState.ActiveTab() == TabSynth {
		return false
	}
	if z.masterChannelSelected() {
		return true
	}
	return z.callbacks.SynthTabDisabled != nil && z.callbacks.SynthTabDisabled()
}

func (z *EQPanelZone) samplerTabDisabled() bool {
	if z.tabState != nil && z.tabState.ActiveTab() == TabSampler {
		return false
	}
	return z.masterChannelSelected()
}

func (z *EQPanelZone) layoutButtons() {
	r := z.rect

	if z.stickyBar != nil {
		// The bar needs the active tab before Layout to decide whether to claim space for tab-only pills.
		z.stickyBar.SetActiveTab(z.tabState.ActiveTab())
		z.stickyBar.SetSynthDisabled(z.synthTabDisabled())
		z.stickyBar.SetSamplerDisabled(z.samplerTabDisabled())
		// Mobile: the bar hosts the Synth/Sampler actions. nil on desktop, where the right cluster is the tab strip.
		var trailing []*Button
		if Profile().IsMobile() && z.callbacks.TrailingActionsForTab != nil {
			trailing = z.callbacks.TrailingActionsForTab(z.tabState.ActiveTab())
		}
		z.stickyBar.SetTrailingActions(trailing)
		z.refreshChannelPillLabel()
		z.stickyBar.Layout(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+stickyBarHeight()))
	}

	// Laid out on every tab so the rects stay stable; drawn and hit-tested only on TabEQ.
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

// The frequency label sits above the level, both confined to the band's column so neither neighbours
// nor the two labels overlap. Shared by layout and draw.
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

	// ExpandHitArea alone floors at MinTarget, which is 0 on desktop densities, so the mute row also
	// takes the per-density floor.
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

	muteBtnY := (r.Max.Y - dbInputH) - muteBtnH - 1
	// In a short panel the bottom-anchored mute row must not climb into the sticky bar, or a mute hit
	// area steals the channel pill's tap.
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

		_, level := z.eqBandLabelRects(i)
		z.dbReadoutRects[i] = level
	}
}

// dbReadoutRect returns the laid-out readout rect for band (zero rect if unset).
func (z *EQPanelZone) dbReadoutRect(band int) image.Rectangle {
	if band < 0 || band >= len(z.dbReadoutRects) {
		return image.Rectangle{}
	}
	return z.dbReadoutRects[band]
}

// applyBandGainLive writes the gain and fires only the LIVE callbacks; the commit (emit + undo) is
// release-only, as on the curve drag.
func (z *EQPanelZone) applyBandGainLive(band int, v float64) {
	if band < 0 || band >= len(z.bandGainsDB) {
		return
	}
	v = clampF64(v, eqDBMin, eqDBMax)
	z.bandGainsDB[band] = v
	z.curveDirty = true
	if z.callbacks.OnGainChange != nil {
		z.callbacks.OnGainChange(band, v)
	}
	if z.callbacks.OnApplyEQ != nil {
		z.callbacks.OnApplyEQ()
	}
}

type eqDBOpenAdapter struct {
	z    *EQPanelZone
	band int
}

func (a *eqDBOpenAdapter) OnPress(x, y int) InputResult {
	if a.z.callbacks.OnOpenValueWheel != nil {
		a.z.callbacks.OnOpenValueWheel(a.band)
	}
	return InputConsumed
}
func (a *eqDBOpenAdapter) OnDrag(x, y int)                     {}
func (a *eqDBOpenAdapter) OnRelease(x, y int)                  {}
func (a *eqDBOpenAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

func (a *eqDBOpenAdapter) glowRect(_, _ int, _ image.Rectangle) image.Rectangle {
	return a.z.dbReadoutRect(a.band)
}

func (z *EQPanelZone) analyzerInstrumentIDs() []channel.ID {
	if z.callbacks.ActiveRows == nil {
		return nil
	}
	rows := z.callbacks.ActiveRows()
	ids := make([]channel.ID, 0, len(rows))
	for _, r := range rows {
		if r != nil && r.Instrument != "" {
			ids = append(ids, channel.Of(r.Instrument))
		}
	}
	return ids
}

// reapplyAnalyzerTaps re-runs the analyzer rule after an import changes the rows under an open tab:
// SetActiveTab returns early on an unchanged tab, so WASM would never wire the new taps.
func (z *EQPanelZone) reapplyAnalyzerTaps() {
	if z == nil || z.tabState == nil {
		return
	}
	EnsureAnalyzersForTab(z.tabState.ActiveTab(), z.activeChannel, z.analyzerInstrumentIDs()...)
}

func (z *EQPanelZone) calcChannelBtnWidth() int {
	maxPx := TextWidth(i18n.T(i18n.KeyMaster))
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

// Built once: rebuildHitAreas runs every frame and formatting here was an allocation per band per frame.
var eqMuteTags, eqDBTags = func() (mute, db [len(eqCenterLabels)]string) {
	for i, l := range eqCenterLabels {
		mute[i] = "eq-mute-" + l
		db[i] = "eq-db-" + l
	}
	return
}()

func (z *EQPanelZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]

	const zIdx = 130 // EQPanelZone z-index
	activeTab := z.tabState.ActiveTab()
	// EQ band controls are live only on TabEQ, or their hit areas steal taps from other tabs' buttons.
	eqTab := activeTab == TabEQ

	// The panel is opaque, so it must be opaque to input too: a full-bounds catch-all keeps a tap in chrome
	// whitespace from falling through to the timeline or the row rack.
	if !z.rect.Empty() {
		// Consume-wheel: no scroll-owning zone lies beneath, so an unhandled wheel must stop here instead of
		// scrolling the drum rows.
		z.hitAreas = append(z.hitAreas, NewInputCaptureHitAreaConsumeWheel(z.rect, zIdx, "eq-panel-capture"))
	}

	// Must sit above the catch-all: at the same z and rect the stable sort keeps the catch-all first and
	// it swallows every handle press. Point controls sit above this overlay (HPF/LPF +2, dB +3, mute +4).
	if eqTab && !z.rect.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    z.rect,
			ZIndex:  zIdx + 1,
			Handler: &curveHandleHitAdapter{zone: z},
			Tag:     "eq-curve-area",
		})
	}

	// No touch expansion: the mute buttons are already wide on mobile, and expanding them would steal
	// taps from the bands below.
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
				Rect: r,
				// Above the dB readout: in a short panel they overlap and the consuming dB adapter would win.
				ZIndex:  zIdx + 4,
				Handler: &buttonHitAdapter{btn: btn},
				Tag:     eqMuteTags[i],
				Touch:   false,
			})
		}
	}

	if z.stickyBar != nil {
		z.hitAreas = append(z.hitAreas, z.stickyBar.HitAreas()...)
	}

	if c := z.activeTabControls(); c != nil {
		z.hitAreas = append(z.hitAreas, c.HitAreas()...)
	}

	// Mobile only: desktop reads hover directly and needs no hit area.
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

	// One above the consume-wheel catch-all, or it swallows the wheel first. The handler ignores presses,
	// so taps still reach the catch-all and the AUTO pill.
	if activeTab == TabWave {
		if cr := z.bodyRect(); !cr.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:    cr,
				ZIndex:  zIdx + 1,
				Handler: &waveZoomHandler{zone: z},
				Tag:     "wave-zoom",
			})
		}
	}

	// On mobile the hit area grows to touch-min through Touch + ClipRect, not by growing the visual.
	if eqTab {
		mobile := Profile().IsMobile()
		filterHit := func(btn *Button, tag string) {
			r := btn.Rect()
			if r.Empty() {
				return
			}
			ha := HitArea{Rect: r, ZIndex: zIdx + 2, Handler: &buttonHitAdapter{btn: btn}, Tag: tag}
			if mobile {
				ha.Touch = true
				ha.ClipRect = expandToTouchMin(r)
			}
			z.hitAreas = append(z.hitAreas, ha)
		}
		filterHit(z.hpfBtn, "eq-hpf-btn")
		filterHit(z.lpfBtn, "eq-lpf-btn")
	}

	// On mobile these rects are cleared (the stepper replaces them), so nothing registers there.
	if eqTab {
		for i := range z.dbReadoutRects {
			r := z.dbReadoutRects[i]
			if r.Empty() {
				continue
			}
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:    r,
				ZIndex:  zIdx + 3, // above HPF/LPF (zIdx+2) and the curve overlay (zIdx+1); the mute hit area (zIdx+4) intentionally sits above this so a mute press wins in the short-panel overlap region.
				Handler: &eqDBOpenAdapter{z: z, band: i},
				Tag:     eqDBTags[i],
			})
		}
	}

}

// --- Hit handler adapters ---

// buttonHitAdapter is the one adapter from *Button to a tree HitHandler. Returning InputCaptured makes
// the tree deliver OnRelease, so the pressed state never latches.
type buttonHitAdapter struct {
	btn *Button
}

func (h *buttonHitAdapter) OnPress(x, y int) InputResult {
	if h.btn == nil {
		return InputIgnored
	}
	// A disabled button swallows the press (no fall-through to a lower sibling) and does nothing.
	if h.btn.Disabled {
		return InputConsumed
	}
	// A prior press whose release was never delivered must not swallow this press edge.
	h.btn.ResetPress()
	h.btn.PressFromTree(true)
	return InputCaptured
}

func (h *buttonHitAdapter) OnDrag(x, y int) {}

func (h *buttonHitAdapter) OnRelease(x, y int) {
	// Deliver the release so the button settles out of its pressed state.
	if h.btn != nil {
		h.btn.PressFromTree(false)
	}
}

func (h *buttonHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// One axis, so plain scroll zooms Y; any scroll turns auto-gain off so the manual scale sticks.
type waveZoomHandler struct {
	zone *EQPanelZone
}

func (h *waveZoomHandler) OnPress(x, y int) InputResult { return InputIgnored }
func (h *waveZoomHandler) OnDrag(x, y int)              {}
func (h *waveZoomHandler) OnRelease(x, y int)           {}
func (h *waveZoomHandler) OnWheel(x, y, steps int) InputResult {
	if steps == 0 {
		return InputIgnored
	}
	z := h.zone
	z.waveAutoGain = false
	if z.waveYGain <= 0 {
		z.waveYGain = 1.0
	}
	z.waveYGain *= 1 + float64(steps)*0.15
	if z.waveYGain < 0.25 {
		z.waveYGain = 0.25
	}
	if z.waveYGain > 16.0 {
		z.waveYGain = 16.0
	}
	return InputConsumed
}

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

type curveHandleHitAdapter struct {
	zone *EQPanelZone
}

// Filter handles take priority over band handles; hover and grab share this geometry.
func (z *EQPanelZone) filterHandleAtPoint(x, y int) hooks.EQFilter {
	r := z.rect
	if r.Empty() {
		return hooks.FilterNone
	}
	hitRadius := Profile().EQHandleRadius
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
			return hooks.FilterHPF
		}
	}
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
			return hooks.FilterLPF
		}
	}
	return hooks.FilterNone
}

// Desktop only: on mobile the nearest-column grab in OnPress owns band selection.
func (z *EQPanelZone) bandHandleAtPoint(x, y int) int {
	if Profile().IsMobile() {
		return -1
	}
	hitRadius := Profile().EQHandleRadius
	for i := range eqBandDefs {
		hx, hy := z.eqBandHandlePos(i)
		dx := x - hx
		dy := y - hy
		if dx*dx+dy*dy <= hitRadius*hitRadius {
			return i
		}
	}
	return -1
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

	// Hit-test filter handles first (HPF/LPF take priority over band handles).
	if f := z.filterHandleAtPoint(x, y); f != hooks.FilterNone {
		z.curveDragFilter = f
		z.curveDragFilterHz = math.NaN()
		return InputCaptured
	}

	// Desktop only: on mobile the 26 px hit radius overlaps the ~39 px band pitch, so a precise grab picked
	// the lower band even when the press was nearer its neighbour.
	if i := z.bandHandleAtPoint(x, y); i >= 0 {
		hx, hy := z.eqBandHandlePos(i)
		gain := 0.0
		if i < len(z.bandGainsDB) {
			gain = z.bandGainsDB[i]
		}
		z.curveDragBand = i
		z.curveSelectBand(i)
		z.setDragLabel(fmt.Sprintf("%+.1f dB", gain), hx, hy)
		return InputCaptured
	}

	// Mobile: a press anywhere in the plot grabs the nearest band by column, since a finger cannot land on a
	// 26 px handle among 10 packed bands. The gain changes only on drag, so a still tap edits nothing.
	if Profile().IsMobile() {
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

// Used as a ClipRect, so compact chrome gets a finger-sized target without growing the visual.
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
	if z.curveDragFilter != hooks.FilterNone {
		freq := xToFreq(x, z.rect)
		if z.curveDragFilter == hooks.FilterHPF {
			freq = clampF64(freq, 20, 2000)
		} else {
			freq = clampF64(freq, 1000, 20000)
		}
		// OnDrag fires every frame; an unmoved cutoff writes nothing (each write retunes the live filters).
		if freq == z.curveDragFilterHz {
			return
		}
		z.curveDragFilterHz = freq
		if z.curveDragFilter == hooks.FilterHPF {
			if z.callbacks.OnHPFCutoffChange != nil {
				z.callbacks.OnHPFCutoffChange(freq)
			}
			z.setDragLabel(fmt.Sprintf("%.0f Hz", freq), x, y)
		} else {
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
	// OnDrag fires every frame; a handle held still writes nothing (each write retunes the live filters).
	if band < len(z.bandGainsDB) && z.bandGainsDB[band] == db {
		return
	}
	if band < len(z.bandGainsDB) {
		z.bandGainsDB[band] = db
	}
	// dB readout derives live from bandGainsDB — no text sync needed.
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
	if z.curveDragFilter != hooks.FilterNone {
		z.curveDragFilter = hooks.FilterNone
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

func (z *EQPanelZone) setDragLabel(text string, hx, hy int) {
	z.curveDragDBText = text
	z.curveDragLabelX = hx
	labelY := hy - 16
	// Handles are clamped to the plot, not the panel, so the flip threshold is the plot top.
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

// Geometry is laid out separately in layoutFor, from Layout, so the portal can resize the menu with the
// viewport.
func (o *eqChannelDropdownOverlay) buildCallbacks() {
	o.buttons = nil
	o.allOnClicks = nil
	o.allLabels = nil
	z := o.zone

	// Master is no target on the Synth/Sampler tabs: there is no instrument to edit on the master bus.
	hideMaster := false
	if z.tabState != nil {
		t := z.tabState.ActiveTab()
		hideMaster = t == TabSynth || t == TabSampler
	}
	if !hideMaster {
		o.allLabels = append(o.allLabels, i18n.T(i18n.KeyMaster))
		o.allOnClicks = append(o.allOnClicks, func() {
			if z.callbacks.OnChannelChange != nil {
				z.callbacks.OnChannelChange(channel.Master)
			}
			z.channelOpen = false
			z.setActiveChannelID(channel.Master)
			z.stickyBar.ChannelBtn().Text = i18n.T(i18n.KeyMaster)
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
					z.callbacks.OnChannelChange(channel.Of(instID))
				}
				z.channelOpen = false
				z.setActiveChannelID(channel.Of(instID))
				z.stickyBar.ChannelBtn().Text = name
				if z.portal != nil {
					z.portal.Close("eq-channel-dropdown")
				}
			})
		}
	}
}

// visible = min(max visible rows, rows that fit below the anchor, total); an empty screenBounds skips
// the pixel cap.
func (o *eqChannelDropdownOverlay) layoutFor(anchor, screenBounds image.Rectangle) {
	z := o.zone
	btnH := 24

	total := len(o.allOnClicks)
	scroll := z.channelScroll
	scroll.ItemHeight = btnH
	scroll.Style = dropdownScrollbarStyle()
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
		buttonMaxX -= dropdownScrollbarWidth()
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

// Test entry point outside the portal flow; production uses buildCallbacks + the portal's Layout.
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
		buttonMaxX -= dropdownScrollbarWidth()
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

	if len(z.eqBandVals) != len(eqBandDefs) {
		z.eqBandVals = make([]float64, len(eqBandDefs))
	}
	// Only the bar heights depend on analyzer data; the stripes, labels and mute rects must draw on the
	// first frame, before any spectrum exists (startup, a suspended AudioContext). With no data the bars decay to zero.
	z.eqBandVals = eqBandLevels(spec, z.eqBandVals, z.resolveSpectrumWindowForEQ())

	bandW := r.Dx() / len(eqBandDefs)
	if bandW < 1 {
		bandW = 1
	}
	// Confined to the gain plot so the bars never paint over the HPF strip or the dB readouts.
	barR := z.eqLiveBarsRect()
	if barR.Empty() {
		return
	}
	maxHeight := barR.Dy() - 8
	if maxHeight < 0 {
		maxHeight = 0
	}
	muteBtnH := Profile().EQSliderH
	muteBtnW := 16
	if Profile().IsMobile() {
		muteBtnW = bandW - 4
	}
	dbInputH := z.dbRowH
	if dbInputH <= 0 {
		dbInputH = Profile().EQDBInputH
	}
	dbInputY := r.Max.Y - dbInputH - 1
	muteBtnY := dbInputY - muteBtnH - 1

	mutedH, gainsH := z.eqBandStateHashes()
	density := Profile().Density()

	// Cached backdrop: re-rendered only on size, density or mute changes.
	z.eqBackdropSprite.tag = "eqBackdropSprite"
	z.eqBackdropSprite.Draw(dst, r,
		eqBackdropKey{w: r.Dx(), h: r.Dy(), density: density, muted: mutedH},
		z.renderEQBackdropContent)

	// The bars animate per frame, so they stay on the direct path; the mute-rect upkeep here is hit-test
	// layout, not pixels.
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
		y0 := barR.Max.Y - h
		if y0 < barR.Min.Y {
			y0 = barR.Min.Y
		}
		isMuted := i < len(z.bandMuted) && z.bandMuted[i]
		if !isMuted {
			base := colEQBar
			if v > 0.8 {
				base = colEQBarPeak
			}
			// Translucent so the curve and handles behind a tall bar stay legible: the bars are a backdrop here.
			drawRect(dst, image.Rect(x0, y0, x1, barR.Max.Y), WithAlpha(base, AlphaSubtle), true)
			if y0-2 >= barR.Min.Y {
				drawRect(dst, image.Rect(x0, y0-2, x1, y0-1), WithAlpha(base, AlphaMedium), true)
			}
		}
		// The mute button is hit-tested but not drawn; the column tint shows its state.
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
	}

	// Labels paint over the live bars, so this layer comes after them.
	z.eqLabelsSprite.tag = "eqLabelsSprite"
	z.eqLabelsSprite.Draw(dst, r,
		eqLabelsKey{w: r.Dx(), h: r.Dy(), density: density, muted: mutedH,
			gains: gainsH, dbRowH: dbInputH, mobile: Profile().IsMobile()},
		z.renderEQLabelsContent)
}

type eqBackdropKey struct {
	w, h    int
	density Density
	muted   uint64
}

type eqLabelsKey struct {
	w, h    int
	density Density
	muted   uint64
	gains   uint64
	dbRowH  int
	mobile  bool
}

func (z *EQPanelZone) eqBandStateHashes() (muted, gains uint64) {
	const prime = 1099511628211
	muted, gains = 14695981039346656037, 14695981039346656037
	for i := range eqBandDefs {
		var m uint64
		if i < len(z.bandMuted) && z.bandMuted[i] {
			m = 1
		}
		muted = (muted ^ m) * prime
		var g float64
		if i < len(z.bandGainsDB) {
			g = z.bandGainsDB[i]
		}
		gains = (gains ^ math.Float64bits(g)) * prime
	}
	return
}

// The layout rects are absolute, so everything is translated by rr - z.rect: zero under the RENDER_SAFE
// bypass, -z.rect.Min when cached.
func (z *EQPanelZone) renderEQBackdropContent(img *ebiten.Image, rr image.Rectangle) {
	delta := rr.Min.Sub(z.rect.Min)
	r := z.rect
	barR := z.eqLiveBarsRect()
	if barR.Empty() {
		return
	}
	bandW := r.Dx() / len(eqBandDefs)
	if bandW < 1 {
		bandW = 1
	}
	for i := range eqBandDefs {
		x0 := r.Min.X + i*bandW
		x1 := x0 + bandW
		if i == len(eqBandDefs)-1 {
			x1 = r.Max.X
		}
		bg := fadeColor(colEQBg, 0.2)
		if i%2 == 1 {
			bg = fadeColor(colGridLine, 0.3)
		}
		drawRect(img, image.Rect(x0, barR.Min.Y, x1, barR.Max.Y).Add(delta), bg, true)
		if i < len(z.bandMuted) && z.bandMuted[i] {
			drawRect(img, image.Rect(x0, barR.Min.Y, x1, barR.Max.Y).Add(delta), fadeColor(colEQBg, 0.5), true)
		}
		if i > 0 {
			drawRect(img, image.Rect(x0, barR.Min.Y, x0+1, barR.Max.Y).Add(delta), colGridLine, true)
		}
	}
}

func (z *EQPanelZone) renderEQLabelsContent(img *ebiten.Image, rr image.Rectangle) {
	delta := rr.Min.Sub(z.rect.Min)
	for i := range eqBandDefs {
		freqR, _ := z.eqBandLabelRects(i)
		z.drawEQCellText(img, eqCenterLabels[i], freqR.Add(delta), colTextSecondary)

		isMuted := i < len(z.bandMuted) && z.bandMuted[i]
		if !z.dbReadoutRects[i].Empty() {
			var labelCol color.Color
			if isMuted {
				labelCol = colEQDBLabelMuted
			} else {
				gain := 0.0
				if i < len(z.bandGainsDB) {
					gain = z.bandGainsDB[i]
				}
				if gain > 0 {
					// The base accent reads dimmer than the cream negatives; the bright variant matches their luminance.
					labelCol = colAccentBright
				} else {
					labelCol = colTextSecondary // gray for cut or zero
				}
			}
			z.drawEQCellText(img, formatDB(z.bandGainsDB[i]), z.dbReadoutRects[i].Add(delta), labelCol)
		}
	}
}

// Drops to RoleMicro when the label is wider than its cell, so it never spills into the next column.
func (z *EQPanelZone) drawEQCellText(dst *ebiten.Image, txt string, cell image.Rectangle, col color.Color) {
	if cell.Empty() || txt == "" {
		return
	}
	avail := cell.Dx() - 2
	role := RoleCaption
	if StyledTextWidth(txt, role) > avail {
		role = RoleMicro
		txt = clipTextToWidthMeasure(txt, avail,
			func(s string) int { return StyledTextWidth(s, RoleMicro) })
	}
	dw := StyledTextWidth(txt, role)
	dh := StyledTextHeight(role)
	tx := cell.Min.X + (cell.Dx()-dw)/2
	ty := cell.Min.Y + (cell.Dy()-dh)/2
	DrawTextStyled(dst, txt, tx, ty, role, col)
}

// Bypasses the per-Draw memo; draw code uses getAnalyzerStateForTab.
func (z *EQPanelZone) getAnalyzerState() *analyzer.State {
	if z.callbacks.AnalyzerState != nil {
		return z.callbacks.AnalyzerState()
	}
	return nil
}

// Bypasses the freeze so toggleTabFreeze can snapshot live state; Levels reads the metrics-only callback.
func (z *EQPanelZone) liveAnalyzerStateForTab(tab PanelTab) *analyzer.State {
	if tab == TabMeters && z.callbacks.AnalyzerMetricsOnly != nil {
		return z.callbacks.AnalyzerMetricsOnly()
	}
	if z.callbacks.AnalyzerState != nil {
		return z.callbacks.AnalyzerState()
	}
	return nil
}

// Freezing holds the current published analyzer.State pointer: it is immutable, so the tab pauses while
// the service keeps producing for the live tabs.
func (z *EQPanelZone) toggleTabFreeze(tab PanelTab) bool {
	if z.frozenByTab == nil {
		z.frozenByTab = map[PanelTab]*analyzer.State{}
	}
	if _, ok := z.frozenByTab[tab]; ok {
		delete(z.frozenByTab, tab)
		return false
	}
	live := z.liveAnalyzerStateForTab(tab)
	if live == nil {
		return false
	}
	z.frozenByTab[tab] = live
	return true
}

// tabFrozen reports whether the given signal tab is currently paused.
func (z *EQPanelZone) tabFrozen(tab PanelTab) bool {
	return z.frozenByTab[tab] != nil
}

// Pins every latch to the snapshot so the meters hold the paused reading at once instead of settling.
func (z *EQPanelZone) applyLevelsFreezeHold(state *analyzer.State) {
	if state == nil || z.levelsLatches == nil {
		return
	}
	hold := func(l *LevelsLatch, peakDB, rmsDB float64) {
		if l == nil {
			return
		}
		l.PeakHoldDB = peakDB
		l.RMSHoldDB = rmsDB
		l.peakHoldSeeded = true
		l.rmsHoldSeeded = true
	}
	for i := range state.Instruments {
		inst := &state.Instruments[i]
		hold(z.levelsLatches.Get(inst.ID), inst.PeakDB, inst.RMSDB)
	}
	hold(z.levelsLatches.Get(channel.Master), state.Master.PeakDB, state.Master.RMSDB)
}

func (z *EQPanelZone) getAnalyzerStateForTab(tab PanelTab) *analyzer.State {
	if z.frameAnalyzerValid && z.frameAnalyzerTab == tab {
		return z.frameAnalyzerState
	}
	var state *analyzer.State
	if fs := z.frozenByTab[tab]; fs != nil {
		state = fs
	} else if tab == TabMeters && z.callbacks.AnalyzerMetricsOnly != nil {
		state = z.callbacks.AnalyzerMetricsOnly()
	} else if z.callbacks.AnalyzerState != nil {
		state = z.callbacks.AnalyzerState()
	}
	z.frameAnalyzerState = state
	z.frameAnalyzerTab = tab
	z.frameAnalyzerValid = true
	return state
}

func (z *EQPanelZone) resolveChannel(state *analyzer.State) (*analyzer.ChannelMetrics, *analyzer.CaptureBuffer) {
	if !z.activeChannel.IsMaster() && state.Detail != nil {
		return state.Detail, state.Capture
	}
	return &state.Master, state.Capture
}

// Auto-gain advances once per analyzer-state refresh, not per frame, so the scale eases instead of jittering.
func (z *EQPanelZone) resolveWaveGain(state *analyzer.State, ch *analyzer.ChannelMetrics, cap *analyzer.CaptureBuffer) (float64, bool) {
	if state != z.waveSmoothState {
		z.waveSmoothState = state
		z.waveSmoothedPeak = advanceWavePeak(z.waveSmoothedPeak, waveWindowPeak(ch, cap))
	}
	if z.waveAutoGain {
		return autoGainForPeak(z.waveSmoothedPeak), true
	}
	if z.waveYGain <= 0 {
		return 1.0, false
	}
	return z.waveYGain, false
}

// Advanced once per analyzer-state refresh, so the release rate follows the ~30 Hz republish, not the
// frame rate.
func (z *EQPanelZone) resolveSpectrumWindow(state *analyzer.State, ch *analyzer.ChannelMetrics) spectrumDBWindow {
	// One instrument's channel is scaled from its pre-rendered one-shot: it is silent between hits, so the live
	// analyser cannot say how loud it gets. Pinned once per channel, which also keeps it still.
	now := time.Now
	if z.nowFn != nil {
		now = z.nowFn
	}
	if z.pinnedWindowFor != z.activeChannel {
		z.pinnedWindowFor = z.activeChannel
		z.pinnedWindow, z.hasPinnedWindow = prerenderedChannelWindow(z.activeChannel)
		if z.hasPinnedWindow {
			// The buffer peak is the raw voice, before row volume, headroom and EQ, so it runs high: show it now,
			// then re-pin once on the measured level.
			z.settleUntil = now().Add(spectrumChannelSettle)
			z.settleMaxDB = spectrumMinDB
		}
	}
	if z.hasPinnedWindow {
		if !z.settleUntil.IsZero() {
			z.settleMaxDB = raiseDBMax(z.settleMaxDB, spectrumPeakDB(ch))
			switch {
			case z.settleMaxDB <= audioDBSilenceDB:
				// Nothing measured yet (muted row, next hit a bar away): freezing now would lock in the high estimate
				// for the rest of the session, so keep waiting.
				z.settleUntil = now().Add(spectrumChannelSettle)
			case now().After(z.settleUntil):
				z.settleUntil = time.Time{}
				floor, ceil := audioDBWindow(z.settleMaxDB)
				z.pinnedWindow = spectrumDBWindow{Floor: floor, Ceil: ceil}
			}
		}
		return z.pinnedWindow
	}

	// Master has no single buffer behind it, so it stays on the live max-anchored axis.
	if state != z.specSmoothState {
		z.specSmoothState = state
		if z.specMaxDB == 0 {
			z.specMaxDB = spectrumMinDB // start silent, not at 0 dBFS
		}
		z.specMaxDB = raiseDBMax(z.specMaxDB, spectrumPeakDB(ch))
	}
	floor, ceil := audioDBWindow(z.specMaxDB)
	return spectrumDBWindow{Floor: floor, Ceil: ceil}
}

// The only writer of activeChannel: every pick re-baselines the dB axis, whose anchor only rises and
// would squash a quieter instrument under the previous channel's peak.
func (z *EQPanelZone) setActiveChannelID(id channel.ID) {
	if z.activeChannel == id {
		return
	}
	z.activeChannel = id
	z.InvalidateSpectrumReadout()
}

// Call on a discontinuity (mute, solo, channel change): it changes WHAT is measured, so the banner dwell,
// the per-bar hold and the axis anchor are all wrong across it. Never per frame - that defeats the smoothing.
func (z *EQPanelZone) InvalidateSpectrumReadout() {
	// The sentence and, with it, the emphasised band.
	z.spectrumBannerHold.reset()
	// The MAX watermark never decays on its own.
	z.spectrumPeaks = SpectrumPeakState{}
	// Drop the pinned window too, so the next frame derives it from the channel selected now.
	z.specMaxDB = spectrumMinDB
	z.eqMaxDB = spectrumMinDB
	z.specSmoothState = nil
	z.pinnedWindowFor = channel.ID{}
	z.hasPinnedWindow = false
	z.settleUntil = time.Time{}
	z.settleMaxDB = spectrumMinDB
	// The EQ backdrop's smoothed band heights.
	for i := range z.eqBandVals {
		z.eqBandVals[i] = 0
	}
}

// Shares the window model with the Spectrum tab but keeps its own peak-hold (see eqSmoothedPeakDB).
func (z *EQPanelZone) resolveSpectrumWindowForEQ() spectrumDBWindow {
	if z.eqMaxDB == 0 {
		z.eqMaxDB = spectrumMinDB // start silent, not at 0 dBFS
	}
	peak := spectrumMinDB
	for _, m := range z.analyzerSnapshot().Spectrum {
		if db := linearToDB(m); db > peak {
			peak = db
		}
	}
	z.eqMaxDB = raiseDBMax(z.eqMaxDB, peak)
	floor, ceil := audioDBWindow(z.eqMaxDB)
	return spectrumDBWindow{Floor: floor, Ceil: ceil}
}

// Not gated on ch.Active, like the renderer: on WASM that flag is false between hits, and the axis would
// refuse readings the bars already draw. A quiet frame sits at the floor and raiseDBMax only rises.
func spectrumPeakDB(ch *analyzer.ChannelMetrics) float64 {
	if ch == nil || len(ch.FFTBins) == 0 {
		return spectrumMinDB
	}
	peak := spectrumMinDB
	for _, db := range ch.FFTBins {
		if db > peak {
			peak = db
		}
	}
	return peak
}

func (z *EQPanelZone) analyzerSnapshot() audio.AnalyzerSnapshot {
	if z.callbacks.AnalyzerSnapshot != nil {
		return z.callbacks.AnalyzerSnapshot(z.ActiveChannel())
	}
	return audio.AnalyzerSnapshot{}
}

// Inset by SpaceXS so the stroke ends with a margin instead of running into the panel border.
func (z *EQPanelZone) eqCurveSpanX(r image.Rectangle) (int, int) {
	minX := r.Min.X + SpaceXS
	maxX := r.Max.X - SpaceXS
	if maxX <= minX {
		minX, maxX = r.Min.X, r.Max.X
	}
	return minX, maxX
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

	// Static curve content is cached on curveRev, size and density; handles, filter markers and the drag
	// label track the cursor live on top.
	key := eqCurveKey{rev: z.curveRev, w: r.Dx(), h: r.Dy(), density: Profile().Density()}
	z.curveSprite.minInterval = traceSpriteDragInterval
	z.curveSprite.tag = "eqCurveSprite"
	z.curveSprite.Draw(dst, r, key, z.renderEQCurveContent)

	mobileSel := Profile().IsMobile()
	for i := range eqBandDefs {
		hx, hy := z.eqBandHandlePos(i)
		hr := eqHandleRadius
		col := colEQHandle
		isDragging := z.curveDragBand == i
		if z.curveHoverBand == i {
			drawRectHoverGlow(dst, image.Rect(hx-eqHandleRadius, hy-eqHandleRadius, hx+eqHandleRadius, hy+eqHandleRadius), colEQHandle, 1.0)
		}
		// On mobile emphasise the selected band, so the user sees which band the stepper edits.
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
		drawCapsule(dst, glowRect, glowCol, true)
		// Handle fill.
		handleRect := image.Rect(hx-hr, hy-hr, hx+hr, hy+hr)
		drawCapsule(dst, handleRect, col, true)
		// 2px border ring.
		drawCapsule(dst, handleRect, colEQHandleBorder, false)
		borderInner := image.Rect(hx-hr+1, hy-hr+1, hx+hr-1, hy+hr-1)
		drawCapsule(dst, borderInner, colEQHandleBorder, false)
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
		if z.curveDragFilter == hooks.FilterHPF {
			col = colEQFilterHandleActive
			hr = eqHandleRadius + 3
		}
		if z.curveHoverFilter == hooks.FilterHPF {
			drawRectHoverGlow(dst, image.Rect(hx-hr, hy-hr, hx+hr, hy+hr), colEQFilterHandle, 1.0)
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
		if z.curveDragFilter == hooks.FilterLPF {
			col = colEQFilterHandleActive
			lr = eqHandleRadius + 3
		}
		if z.curveHoverFilter == hooks.FilterLPF {
			drawRectHoverGlow(dst, image.Rect(lx-lr, ly-lr, lx+lr, ly+lr), colEQFilterHandle, 1.0)
		}
		drawRect(dst, image.Rect(lx, ly, lx+1, r.Max.Y), colEQFilterLine, true)
		drawRect(dst, image.Rect(lx-lr, ly-lr, lx+lr, ly+lr), col, true)
		drawRect(dst, image.Rect(lx-lr, ly-lr, lx+lr, ly+lr), colEQFilterHandle, false)
	}

	// Draw real-time dB/Hz label during curve handle drag.
	if z.curveDragDBText != "" {
		lbl := z.curveDragDBText
		tw := StyledTextWidth(lbl, RoleCaption)
		th := StyledTextHeight(RoleCaption)
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
		DrawTextStyled(dst, lbl, lx, ly, RoleCaption, genColorBorder)
	}
}

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

// The guide labels are not localized, so language is not part of the key.
type eqCurveKey struct {
	rev     int
	w, h    int
	density Density
}

// Coordinates are r-relative: TraceSprite passes an origin rect when cached, the real plot rect under
// RENDER_SAFE.
func (z *EQPanelZone) renderEQCurveContent(img *ebiten.Image, r image.Rectangle) {
	curveMinX, curveMaxX := z.eqCurveSpanX(r)

	// 0 dB reference line (dashed).
	zeroY := gainDBToY(0, r)
	for dx := curveMinX; dx < curveMaxX; dx += 6 {
		end := dx + 3
		if end > curveMaxX {
			end = curveMaxX
		}
		drawRect(img, image.Rect(dx, zeroY, end, zeroY+1), colEQZeroLine, true)
	}

	guideCol := WithAlpha(genColorEqZeroLine, AlphaSubtle)
	guideLabelCol := WithAlpha(colTextSecondary, AlphaStrong)
	for _, gdb := range [...]float64{eqGuideDB, -eqGuideDB} {
		gy := gainDBToY(gdb, r)
		if gy <= r.Min.Y || gy >= r.Max.Y-1 {
			continue
		}
		drawRect(img, image.Rect(curveMinX, gy, curveMaxX, gy+1), guideCol, true)
		lbl := "+6"
		if gdb < 0 {
			lbl = "-6"
		}
		ly := gy - StyledTextHeight(RoleMicro) - 1
		if ly < r.Min.Y {
			ly = gy + 2
		}
		DrawTextStyled(img, lbl, curveMinX+2, ly, RoleMicro, guideLabelCol)
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
		if prevX >= 0 && px > prevX && px >= curveMinX && px <= curveMaxX {
			fillY0, fillY1 := py, zeroY
			if fillY0 > fillY1 {
				fillY0, fillY1 = fillY1, fillY0
			}
			if fillY1-fillY0 > 1 {
				drawRect(img, image.Rect(px, fillY0, px+1, fillY1), colEQCurveFill, true)
			}
			lineY0, lineY1 := py, py+2
			if lineY1 > r.Max.Y {
				lineY1 = r.Max.Y
			}
			drawRect(img, image.Rect(px, lineY0, px+1, lineY1), colEQCurve, true)
		}
		prevX = px
	}
}

func (z *EQPanelZone) rebuildCurveCache() {
	bands := z.buildCurrentBands()
	z.curveCache = audio.ComputeFreqResponse(audio.SampleRate(), bands, eqCurvePoints, 20, 20000)
	z.curveDirty = false
	z.curveRev++
}

func (z *EQPanelZone) SetBandGainDB(band int, db float64) {
	if band < 0 || band >= len(z.bandGainsDB) {
		return
	}
	z.bandGainsDB[band] = db
	z.curveDirty = true
	z.curveCache = nil
}

// The slice is rebuilt on every gain change; do not retain it across frames.
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

// newScrollBehavior creates a fresh ScrollBehavior for use in the zone.
func newScrollBehavior() *ScrollBehavior {
	return NewScrollBehavior(dropdownScrollbarStyle(), 24)
}

// A per-density floor on top of ExpandHitArea (whose MinTarget is 0 on desktop), so the target grows with
// density without forcing big buttons on Compact.
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

// Same floor as the mute toggle above it, so neighbouring taps do not miss each other.
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

// Rects are empty in the other readout modes; levelsAggregateSlotForPoint checks that.
func (z *EQPanelZone) snapshotLevelsIconRow(state *analyzer.State, visibleIDs map[channel.ID]bool) {
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
	// Must mirror the cascade thresholds in drawLevelsMultiChannel.
	if rect.Dx() >= 2*readoutWFull || rect.Dx() < readoutWFull+readoutWIcons {
		return
	}
	colRect := image.Rect(rect.Max.X-readoutWIcons+2, rect.Min.Y+4, rect.Max.X-2, rect.Max.Y-4)
	z.levelsHeadroomRect, z.levelsClipsRect, z.levelsLoudestRect = levelsAggregateIconRects(colRect)
	z.levelsAggSnapshot = levelsAggregatesValues(state, z.levelsLatches, visibleIDs)
}

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
		Passive: true, // display-only hint; never owns input / closes on outside press
		Anchor:  anchor,
	})
	return true
}

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
