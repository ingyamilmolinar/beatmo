package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

/* ─── ctor ─────────────────────────────────────────────────── */

func NewDrumView(b image.Rectangle, g *model.Graph, logger *game_log.Logger) *DrumView {
	opts := audio.Instruments()
	// Named explicitly: the roster order changes, and a positional pick silently changed the default row.
	inst := roster.Snare
	if !slices.Contains(opts, inst) {
		if len(opts) == 0 {
			inst = ""
		} else {
			inst = opts[0]
		}
	}
	name := audio.PrettyName(string(inst))
	dv := &DrumView{
		textFields:           newTextFieldRegistry(),
		Bounds:               b,
		labelW:               100,
		bpm:                  120,
		secPerBeat:           0.5,
		headerH:              timelineHeight,
		eqH:                  eqPanelHeight,
		bgDirty:              true,
		Graph:                g,
		logger:               logger,
		notifStore:           newNotificationStore(notifHistoryCap),
		armGrowIdx:           -1,
		Length:               8, // Default length
		Offset:               0,
		instOptions:          opts,
		instRefreshDirty:     true,
		labelWidthDirty:      true, // Force initial label width calculation
		uploadCh:             make(chan uploadResult, 1),
		importCh:             make(chan importResult, 1),
		instImportCh:         make(chan importResult, 1),
		timelineUnitsPerBeat: 1,
		timelineBeats:        8,
		jumpBeats:            4, // default FF/RW jump size in whole beats
		selRow:               0,
		deleteConfirmRow:     -1,
		renameRow:            -1,
		samplePath:           make(map[instrument.ID]string),
		// Tests that index Categories[0] expecting their own first category turn this off.
		instMenuShowFavoritesCategory: true,
	}
	// A statement, because a composite literal cannot reference dv. A DrumView built without this
	// ctor must set it before calling a sampler method.
	dv.samplerPanel = &samplerPanel{dv: dv}
	dv.synthPanel = &synthPanel{dv: dv}
	dv.initNotifPersistence()
	dv.initVolumeAndWheelPopups()
	dv.initOverlayComponents()

	dv.initEQChannelState()
	dv.initLengthAndSaveButtons()

	// By row index: the first row takes the first series color.
	firstColor := seriesColorAt(0)
	dv.Rows = []*DrumRow{{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), CellTypes: make([]model.NodeType, dv.Length), Color: firstColor, Origin: model.InvalidNodeID, Volume: 0.5}}
	dv.SetBeatLength(dv.Length) // Initialize graph's beat length
	// Before the first Update, so early highlight/audio paths see valid instruments and play.
	dv.initWidgetBoard(b)
	dv.refreshInstruments()
	// recalcButtons waits for TransportZone below: the transport buttons are nil until then.
	dv.rowCachePadPx = defaultRowCachePadPx
	dv.rowsLayerPadPx = defaultRowsLayerPadPx
	dv.layoutDragIdx = -1
	dv.layoutHoverIdx = -1
	dv.layoutHandler = NewLayoutResizeHandler(dv)
	dv.initInputTrees(b)
	dv.initEQPanelZone()

	dv.initChainZone()

	// Layout waits until both the EQ and transport zones exist.

	dv.initTransportZone()

	dv.initRowRackZone()

	dv.initTimelineZone()

	dv.initLayoutResizeAndDrawLayers()

	dv.initMobileViewControls()

	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}
	dv.ensureRowCache()
	dv.markAllRowsDirty()
	// Cache aliases are handled by ensureRowCache via timelineZone delegation.

	// Reset global click suppression to ensure clean state for new views/tests.
	suppressClicksUntilRelease = false
	return dv
}

func (dv *DrumView) initVolumeAndWheelPopups() {
	dv.volPopup = NewSliderPopup(SliderPopupConfig{
		ID:     "volume-popup",
		ZIndex: 225,
		GetValue: func() float64 {
			row := dv.volPopupRow
			if row >= 0 && row < len(dv.Rows) {
				return dv.Rows[row].Volume
			}
			return 0
		},
		SetValue: func(v float64) {
			row := dv.volPopupRow
			if row >= 0 && row < len(dv.Rows) {
				dv.Rows[row].Volume = v
				if row < len(dv.rowVolSliders()) {
					dv.rowVolSliders()[row].Value = v
				}
				dv.markRowControlsDirty()
			}
		},
		// Emit + undo record once at drag release (live SetValue stays per-frame).
		OnRelease: func() { dv.commitRowVolume(dv.volPopupRow) },
		// Read live off volPopupRow, so the rail follows a color edit.
		Accent: func() color.Color {
			row := dv.volPopupRow
			if row >= 0 && row < len(dv.Rows) {
				return dv.Rows[row].Color
			}
			return nil
		},
	})
	dv.masterVolPopup = NewSliderPopup(SliderPopupConfig{
		ID:     "master-volume-popup",
		ZIndex: 226,
		GetValue: func() float64 {
			if dv.mainVolSlider() != nil {
				return dv.mainVolSlider().Value
			}
			return 0
		},
		SetValue: func(v float64) {
			if dv.mainVolSlider() != nil {
				dv.mainVolSlider().Value = v
			}
			audio.SetMainVolume(v)
			// Live audio stays per-frame; stash for the release commit below.
			dv.mainVolPending, dv.mainVolPendingDirty = v, true
		},
		// Emit + undo record once at drag release.
		OnRelease: func() { dv.commitMainVolume() },
	})
	dv.samplerWheelPopup = NewMobileWheelPopup()
	dv.eqWheelPopup = NewMobileWheelPopup()
	dv.valueWheelPopup = NewMobileWheelPopup()
	// Built once so the EQ wheel's StepMul is always defined; its saved rung is restored on first
	// open because the step sink is installed after this ctor.
	dv.eqWheelStepBadge = NewKnobStepBadge(audio.ParamDef{
		Name: "eq_band_gain", Min: eqDBMin, Max: eqDBMax, Unit: "dB",
	})
}

func (dv *DrumView) initOverlayComponents() {
	dv.subdivMenuComp = NewSubdivMenuComponent()
	dv.jumpMenuComp = NewSubdivMenuComponent()
	dv.renameComp = NewRenameComponent()
	dv.colorWheelComp = NewColorWheelComponent()
	dv.instMenuComp = NewInstrumentMenuComponent()

	dv.jumpUnitBeats = true
	dv.navPanel = &NavigationPanel{}
	dv.navPanel.SetCallbacks(NavPanelCallbacks{
		Position: func() PlayheadPos {
			if dv.game != nil {
				return dv.game.PlayheadPosition()
			}
			return PlayheadPos{}
		},
		PlayheadStep: func() int {
			if dv.game != nil {
				return dv.game.playheadAbsSubdiv()
			}
			return 0
		},
		BPM:           func() int { return dv.BPM() },
		Div:           func() int { return dv.timelineUnitsPerBeat },
		TimelineBeats: func() int { return dv.timelineBeats },
		JumpAmount:    func() int { return dv.jumpBeats },
		JumpUnitBeats: func() bool { return dv.jumpUnitBeats },
		OnPrev: func() {
			if dv.jumpUnitBeats {
				dv.RequestSeekDeltaBeats(-dv.jumpBeats, hooks.SeekCounter)
			} else {
				dv.RequestSeekDeltaSteps(-dv.jumpBeats, hooks.SeekCounter)
			}
		},
		OnNext: func() {
			if dv.jumpUnitBeats {
				dv.RequestSeekDeltaBeats(dv.jumpBeats, hooks.SeekCounter)
			} else {
				dv.RequestSeekDeltaSteps(dv.jumpBeats, hooks.SeekCounter)
			}
		},
		OnPickAmount: func() { dv.OpenJumpMenu() },
		OnToggleUnit: func() { dv.jumpUnitBeats = !dv.jumpUnitBeats },
		OnGotoPreview: func(steps int, on bool) {
			if dv.timelineZone != nil {
				dv.timelineZone.SetGotoGhost(steps, on)
			}
		},
		OnGoto: func(steps int) { dv.RequestSeekSteps(steps, hooks.SeekCounter) },
	})
	dv.navMenuComp = NewNavigationMenuComponent(dv.navPanel)
}

func (dv *DrumView) initEQChannelState() {
	dv.eqChannelScroll = NewScrollBehavior(dropdownScrollbarStyle(), TouchRowHeight())
	dv.eqActiveChannel = channel.Master
	dv.eqCurveDragBand = -1
	dv.eqCurveDirty = true
	dv.hpfEnabled = false
	dv.hpfCutoffHz = 20
	dv.lpfEnabled = false
	dv.lpfCutoffHz = 20000
	_ = audio.EnableChannelAnalyzer(channel.Master, 512)
	// Feeds the Chain tab's Synth/AntiPop stages in the browser; desktop has the scope service.
	_ = audio.EnableSynthAnalyzer(channel.Master, 512)
}

func (dv *DrumView) initLengthAndSaveButtons() {
	dv.lenDecBtn = NewButton("", LenDecStyle, func() {
		dv.logger.Debugf("[drumview] length - button pressed")
		dv.lenDecPressed = true
		dv.lenDecAnim = 1
	})
	dv.lenDecBtn.Repeat = true
	dv.lenDecBtn.Icon = "minus"
	dv.lenIncBtn = NewButton("", LenIncStyle, func() {
		dv.logger.Debugf("[drumview] length + button pressed")
		dv.lenIncPressed = true
		dv.lenIncAnim = 1
	})
	dv.lenIncBtn.Repeat = true
	dv.lenIncBtn.Icon = "plus"
	// Re-derived every layout; seeded here so the buttons are valid before the first one.
	dv.refreshLenButtonsStyle()
	dv.saveBtn = NewButton(i18n.T(i18n.KeySave), InstButtonStyle, nil)
	// On WASM IsMobile() only becomes true later, in Layout; refreshWidgetLayout handles that.
	if Profile().IsMobile() {
		dv.mobileEQCollapsed = true
		dv.mobileEQInited = true
	}
}

func (dv *DrumView) initWidgetBoard(b image.Rectangle) {
	dv.widgetRects = map[WidgetKind]image.Rectangle{}
	pp := Profile()
	dv.widgets = NewWidgetBoard(b, pp.ColWeights, pp.RowWeights)
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetTransport, Title: "Transport", Col: 0, Row: 0, ColSpan: 1, RowSpan: 1, MinW: 180, MinH: mobileTransportMinH()})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetRack, Title: "Instruments", Col: 0, Row: 1, ColSpan: 1, RowSpan: 1, MinW: 200, MinH: dv.rowHeight() * 4, Editable: true})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetTimeline, Title: "Timeline", Col: 1, Row: 0, ColSpan: 1, RowSpan: 2, MinW: 320, MinH: dv.rowHeight() * 4, Editable: true})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetWave, Title: "Wave/EQ", Col: 0, Row: 2, ColSpan: 2, RowSpan: 1, MinW: 240, MinH: eqPanelHeight, Editable: true})
	// On mobile, hide the Wave/EQ widget by default.
	if dv.mobileEQCollapsed {
		dv.widgets.ToggleWidget(WidgetWave, false)
	}
	dv.refreshWidgetLayout()
}

func (dv *DrumView) initInputTrees(b image.Rectangle) {
	dv.tree = NewDrumViewTree()
	dv.tree.SetBounds(b)
	dv.tree.SetDragActive(func() bool { return dv.anyDragActive() })
	// Defer to the InputDispatcher while it owns the press (the splitter), so the two input systems
	// never both act on one press.
	dv.tree.SetExternalCapture(func() bool { return dv.inputCapturedExternally })

	// Its own tree, so its HitIndex is isolated: a press or wheel in the panel never reaches the row rack.
	dv.audioTree = NewDrumViewTree()
	dv.audioTree.SetDragActive(func() bool { return dv.anyDragActive() })
	dv.audioTree.SetExternalCapture(func() bool { return dv.inputCapturedExternally })

	// The highest-z tree, so it is offered every press first; it owns the one global OverlayPortal,
	// and without it dv.portal() is nil and every popup silently does nothing.
	dv.overlayTree = NewDrumViewTree()
	dv.overlayTree.SetDragActive(func() bool { return dv.anyDragActive() })
	dv.overlayTree.SetExternalCapture(func() bool { return dv.inputCapturedExternally })

	dv.rootTree = NewRootTree()
	dv.rootTree.AddChild("drumview", dv.tree, 0)
	dv.rootTree.AddChild("audio", dv.audioTree, 1)
	dv.rootTree.AddChild("overlay", dv.overlayTree, 2)
}

func (dv *DrumView) initEQPanelZone() {
	dv.eqPanelZone = NewEQPanelZone(EQCallbacks{
		OnGainChange: func(band int, db float64) {
			ch := dv.eqPanelZone.ActiveChannel()
			if ch == channel.Master {
				dv.applyMasterEQ()
			} else {
				for j, r := range dv.Rows {
					if channel.Of(r.Instrument) == ch {
						if eq := rowEQ(r); band < len(eq.GainsDB) {
							eq.GainsDB[band] = db
						}
						dv.applyRowEQ(j)
						break
					}
				}
			}
			// Audio stays live per change; the emit and undo record fire once on release (commitEQBand).
			dv.eqPendingChannel, dv.eqPendingBand, dv.eqPendingGainDB, dv.eqPendingDirty = ch, band, db, true
		},
		OnEQBandCommit:   func() { dv.commitEQBand() },
		OnOpenValueWheel: func(band int) { dv.openEQKnobWheelPopup(band) },
		OnMuteToggle: func(band int) {
			dv.toggleEQBandMute(band)
		},
		OnChannelChange: func(id channel.ID) {
			dv.selectAudioChannel(id)
		},
		OnToggleHPF: func() {
			dv.toggleHPF()
		},
		OnToggleLPF: func() {
			dv.toggleLPF()
		},
		OnApplyEQ: func() {
			dv.applyEQ()
		},
		AnalyzerSnapshot: func(ch channel.ID) audio.AnalyzerSnapshot {
			return dv.analyzerSnapshot()
		},
		ActiveRows: func() []*DrumRow {
			return dv.Rows
		},
		HPFEnabled:  func() bool { return dv.activeHPFEnabled() },
		HPFCutoffHz: func() float64 { return dv.activeHPFCutoffHz() },
		LPFEnabled:  func() bool { return dv.activeLPFEnabled() },
		LPFCutoffHz: func() float64 { return dv.activeLPFCutoffHz() },
		OnHPFCutoffChange: func(hz float64) {
			dv.setActiveHPF(true, hz)
			dv.eqCurveDirty = true
		},
		OnLPFCutoffChange: func(hz float64) {
			dv.setActiveLPF(true, hz)
			dv.eqCurveDirty = true
		},
		OnChannelDropdownClose: func() {
			dv.eqChDeferredTap.Cancel()
		},
		DrawWaveform: func(dst *ebiten.Image) {
			snap := dv.analyzerSnapshot()
			dv.drawWaveform(dst, snap)
		},
		SpectrumRowFacts: func(st *analyzer.State) []rowFacts {
			return dv.spectrumRowFacts(st)
		},
		AnalyzerState: func() *analyzer.State {
			if svc := audio.AnalyzerService(); svc != nil {
				return svc.State()
			}
			if cached := dv.eqPanelZone.frozenAnalyzer; cached != nil {
				return cached
			}
			return BuildAnalyzerStateFromSnapshots(dv.activeEQChannel(), dv.Rows, audio.SampleRate())
		},
		AnalyzerMetricsOnly: func() *analyzer.State {
			// Reuse the service's whole state so freeze and capture stay coherent.
			if svc := audio.AnalyzerService(); svc != nil {
				return svc.State()
			}
			if cached := dv.eqPanelZone.frozenAnalyzer; cached != nil {
				return cached
			}
			return BuildAnalyzerMetricsOnly(dv.Rows)
		},
		OnFreezeToggle: func() bool {
			if svc := audio.AnalyzerService(); svc != nil {
				state := svc.State()
				if state != nil && state.Capture != nil && state.Capture.Frozen {
					svc.Unfreeze()
					return false
				}
				svc.Freeze()
				return true
			}
			// WASM: toggle the zone-local cache.
			if dv.eqPanelZone.frozenAnalyzer != nil {
				dv.eqPanelZone.frozenAnalyzer = nil
				return false
			}
			dv.eqPanelZone.frozenAnalyzer = BuildAnalyzerStateFromSnapshots(dv.activeEQChannel(), dv.Rows, audio.SampleRate())
			return dv.eqPanelZone.frozenAnalyzer != nil
		},
		OnLeaveSynthTab: func() {
			// A wheel spin that has not self-committed yet must not be dropped by a tab switch.
			dv.synthWheelNudge.flush()
		},
		OnLeaveSamplerTab: func() {
			// A wheel spin that has not self-committed yet must not be dropped by a tab switch.
			dv.samplerWheelNudge.flush()
		},
		OnTabChange: func(tab PanelTab) {
			// Scope tab auto-expands the panel; trigger layout recalc.
			dv.bgDirty = true
			// While the audio view owns the screen the nav follows programmatic tab switches; a background
			// switch while Pads is visible never pulls the user into the audio view.
			if Profile().IsMobile() && dv.MobileEQMode() {
				if vm, ok := viewModeForPanelTab(tab); ok {
					dv.setViewMode(vm)
				}
			}
		},
		BeatGridFrac: func() []float64 {
			if dv == nil || dv.game == nil {
				return nil
			}
			// Master or an unmatched id falls back to the first audible row, so the beat grid has an anchor.
			chID := dv.eqPanelZone.ActiveChannel()
			rowIdx := -1
			for i, r := range dv.Rows {
				if r != nil && channel.Of(r.Instrument) == chID {
					rowIdx = i
					break
				}
			}
			if rowIdx < 0 {
				for i := range dv.Rows {
					if dv.game.rowIsAudible(i) {
						rowIdx = i
						break
					}
				}
			}
			if rowIdx < 0 {
				return nil
			}
			// Matches the trace width drawAnalyzerWaveform renders (≈ 46 ms at 48 kHz).
			const windowSamples = 2200
			return dv.game.beatGridFractions(rowIdx, windowSamples)
		},
		// Delegated so the synth state stays anchored to the row that owns it.
		OnSynthTabLayout: func(r image.Rectangle) {
			dv.buildSynthTab(r, dv.synthTabActiveInstrument())
		},
		DrawSynthTab: func(dst *ebiten.Image, r image.Rectangle) {
			dv.drawSynthTab(dst, r, dv.synthTabActiveInstrument())
		},
		ReleaseSynthSprites: func() { dv.synp().sprites.releaseAll(); dv.synp().glow.release() },
		SynthTabHitAreas: func() []HitArea {
			return dv.synthTabHitAreas()
		},
		TrailingActionsForTab: func(tab PanelTab) []*Button {
			switch tab {
			case TabSynth:
				return dv.synthPanelMobileActions()
			case TabSampler:
				return dv.samplerPanelMobileActions()
			}
			return nil
		},
		SynthTabUpdate: func() bool {
			return dv.synthTabUpdate()
		},
		SynthTabDisabled: func() bool {
			return !dv.activeInstrumentHasSynth()
		},
		OnSamplerTabLayout: func(r image.Rectangle) {
			dv.buildSamplerTab(r, dv.samplerActiveInstrument())
		},
		DrawSamplerTab: func(dst *ebiten.Image, r image.Rectangle) {
			dv.drawSamplerTab(dst, r)
		},
		SamplerTabHitAreas: func() []HitArea {
			return dv.samplerTabHitAreas()
		},
		SamplerTabUpdate: func() bool {
			return dv.samplerTabUpdate()
		},
		IsHiddenForInput: func() bool {
			// Must match the tree's visibility predicate: a hidden panel publishes no hit areas, so the
			// row rack below it gets the input.
			if dv.perfDrawLite {
				return true
			}
			if Profile().IsMobile() {
				return !dv.MobileEQMode()
			}
			return false
		},
	})
	dv.eqPanelZone.SetPortal(dv.overlayTree.Portal())
	// The one place that decides whether the EQ panel paints.
	dv.audioTree.RegisterZoneVisible(dv.eqPanelZone, ZEQPanel, func() bool {
		if dv.perfDrawLite {
			return false
		}
		// On mobile the panel and the rack share a band, so only one may be visible.
		if Profile().IsMobile() {
			return dv.MobileEQMode()
		}
		return true
	})
}

func (dv *DrumView) initChainZone() {
	chainZ := NewChainPanelZone(ChainCallbacks{
		ScopeState: func() *scope.State {
			if svc := audio.ScopeService(); svc != nil {
				return svc.State()
			}
			z := dv.eqPanelZone.chainZone
			if z == nil {
				return nil
			}
			if z.frozenState != nil {
				return z.frozenState
			}
			return BuildScopeStateFromSnapshots(z.instrumentID, z.tapA, z.tapB)
		},
		ActiveRows: func() []*DrumRow {
			return dv.Rows
		},
		OnTapAChange: func(stage scope.Stage) {
			if svc := audio.ScopeService(); svc != nil {
				svc.SetTapA(stage)
				return
			}
			// The zone already updated its own tapA.
		},
		OnTapBChange: func(stage scope.Stage) {
			if svc := audio.ScopeService(); svc != nil {
				svc.SetTapB(stage)
				return
			}
		},
		OnClearTapA: func() {
			if svc := audio.ScopeService(); svc != nil {
				svc.ClearTapA()
				return
			}
		},
		OnClearTapB: func() {
			if svc := audio.ScopeService(); svc != nil {
				svc.ClearTapB()
				return
			}
		},
		OnFreezeToggle: func() bool {
			if svc := audio.ScopeService(); svc != nil {
				if svc.IsFrozen() {
					svc.Unfreeze()
					return false
				}
				svc.Freeze()
				return true
			}
			// WASM: toggle zone-local freeze cache.
			z := dv.eqPanelZone.chainZone
			if z == nil {
				return false
			}
			if z.frozenState != nil {
				z.frozenState = nil
				return false
			}
			z.frozenState = BuildScopeStateFromSnapshots(z.instrumentID, z.tapA, z.tapB)
			return z.frozenState != nil
		},
		// Desktop reads every stage's lock-free stats; the browser has no such feed, so -Inf keeps it dark.
		StagePeak: func(stage scope.Stage) (float64, float64) {
			if svc := audio.ScopeService(); svc != nil {
				return svc.LatestPeak(stage)
			}
			return math.Inf(-1), math.Inf(-1)
		},
		IsHiddenForInput: func() bool {
			// Hidden whenever the host EQ panel is: the same predicate the tree consults.
			if dv.perfDrawLite {
				return true
			}
			if Profile().IsMobile() {
				return !dv.MobileEQMode()
			}
			return false
		},
	})
	dv.eqPanelZone.SetChainZone(chainZ)
}

func (dv *DrumView) initTransportZone() {
	dv.transportZone = NewTransportZone(TransportCallbacks{
		OnPlayToggle: func() {
			dv.logger.Debugf("[drumview] play button pressed")
		},
		PlaybackReady: func() bool { return dv.game == nil || dv.game.playbackReady() },
		OnStop: func() {
			dv.logger.Debugf("[drumview] stop button pressed")
		},
		OnOpenBPMWheel: func(anchor image.Rectangle) { dv.openBPMWheel(anchor) },
		OnBPMLive:      func(bpm int) { dv.applyBPMState(bpm) },
		OnBPMChange: func(bpm int) {
			// The only chokepoint the interactive BPM controls reach; DrumView.SetBPM is not on that path.
			dv.applyBPMState(bpm)
			emitBPMChange(bpm)
		},
		OnNotifyErrorKey: func(key i18n.Key, args ...string) {
			dv.notifyErrorKey(key, args...)
		},
		OnFollowChange: func(follow bool) {
			if follow {
				dv.logger.Debugf("[drumview] track/free toggled: follow=Track")
			} else {
				dv.logger.Debugf("[drumview] track/free toggled: follow=Free")
			}
		},
		OnUploadClick: func() {
			dv.logger.Debugf("[drumview] upload button pressed")
			dv.logger.Debugf("[drumview] Upload button clicked. uploading=%v naming=%v menuOpen=%v", dv.uploading, dv.IsNamingOpen(), dv.IsInstMenuOpen())
			if dv.importing {
				return
			}
			if !dv.uploading && !dv.IsNamingOpen() {
				dv.uploading = true
				dv.logger.Debugf("[drumview] Opening file chooser")
				if err := async.Go("ui.dialog", func(_ context.Context) {
					path, err := audio.SelectWAV()
					dv.uploadCh <- uploadResult{path: path, err: err}
				}); err != nil {
					// Pool saturated/closed — bail out gracefully.
					dv.uploadCh <- uploadResult{path: "", err: err}
				}
			}
		},
		OnImportClick: func() {
			dv.logger.Debugf("[drumview] import button pressed")
			log := jsLog
			log("Import button pressed; importing=%v, naming=%v, uploading=%v", dv.importing, dv.IsNamingOpen(), dv.uploading)
			if dv.importing || dv.IsNamingOpen() || dv.uploading {
				return
			}
			if dv.onImportDialogStart != nil {
				dv.onImportDialogStart()
			}
			dv.importing = true
			dv.importAttemptFrame = int(dv.frame)
			dv.importAttemptUpdate = dv.updateSeq
			selectJSONAsyncFn(func(data []byte, name string, err error) {
				jsLog("Import callback invoked; bytes=%d name=%s err=%v", len(data), name, err)
				dv.importCh <- importResult{data: data, name: name, err: err}
			})
		},
		OnExportClick: func() {
			dv.logger.Debugf("[drumview] export button pressed")
			if err := dv.Export(); err != nil {
				dv.logger.Errorf("[drumview] export failed: %v", err)
			} else {
				hooks.PublishKind(hooks.EventExport, nil)
			}
		},
		OnViewCycle: func() {
			if dv.currentViewMode == viewModeRows {
				dv.setViewMode(viewModeEQ)
			} else {
				dv.setViewMode(viewModeRows)
			}
		},
		IsPlaying: func() bool {
			return dv.isPlaying
		},
		GetMainVolume: audio.MainVolume,
		SetMainVolume: func(v float64) {
			audio.SetMainVolume(v)
			// Audio stays live per change; the emit and undo record fire once on release (commitMainVolume).
			dv.mainVolPending, dv.mainVolPendingDirty = v, true
		},
		OnMainVolCommit: func() { dv.commitMainVolume() },
		OnSubdivClick: func() {
			// Toggle: close if already open.
			if dv.subdivMenuComp != nil && dv.subdivMenuComp.IsOpen() {
				dv.subdivMenuComp.Close()
				dv.closeSubdivMenuPortal()
				return
			}
			dv.CloseAllPopups()
			if dv.subdivMenuComp != nil {
				dv.subdivMenuComp.SetProps(SubdivMenuProps{
					AnchorRect: dv.subdivBtn().Rect(),
					Current:    dv.timelineUnitsPerBeat,
					Options:    []int{4, 8, 16, 32},
					RowHeight:  dv.rowHeight(),
					OnSelect: func(value int) {
						if dv.onChangeSubdiv != nil {
							if err := dv.onChangeSubdiv(value); err != nil {
								return
							}
						}
						dv.subdivBtn().Text = fmt.Sprintf("\u00f7%d", value)
						dv.timelineUnitsPerBeat = value
					},
					OnClose: func() {},
				})
				dv.subdivMenuComp.Open()
				dv.openSubdivMenuPortal()
			}
			if dv.IsSubdivMenuOpen() {
				dv.buildSubdivMenu()
			}
		},
		OnMasterVolClick: func() {
			dv.masterVolPopup.Open(dv.mainVolIconRect, dv.Bounds, dv.headerH)
			dv.openMasterVolPopupPortal()
		},
		MasterVolPopup: dv.masterVolPopup,
		// Deferred through QueueAction: these buttons run under seqMu, and Undo re-imports through
		// updateBeatInfos, which takes seqMu again (a self-deadlock).
		OnUndo: func() {
			if dv.game != nil && dv.game.undoManager != nil {
				dv.game.QueueAction(func(g *Game) { g.performUndo() })
			}
		},
		OnRedo: func() {
			if dv.game != nil && dv.game.undoManager != nil {
				dv.game.QueueAction(func(g *Game) { g.performRedo() })
			}
		},
		CanUndo: func() bool {
			return dv.game != nil && dv.game.undoManager != nil && dv.game.undoManager.CanUndo()
		},
		CanRedo: func() bool {
			return dv.game != nil && dv.game.undoManager != nil && dv.game.undoManager.CanRedo()
		},
	})
	dv.transportZone.SetPortal(dv.overlayTree.Portal())
	dv.tree.RegisterZoneVisible(dv.transportZone, ZTransport, func() bool { return !dv.simpleDraw })

	// Follow state lives only on TransportZone.
	dv.transportZone.SetBPM(dv.bpm)

	// Not tree.Suppress(): that flag persists into the next frame and would blur the box after a
	// normal click on it.
	dv.transportZone.SetInputBlocked(func() bool {
		treeBlocking := dv.tree != nil && dv.tree.Portal().IsOpen()
		return dv.anyDropdownOpen() || treeBlocking
	})
}

func (dv *DrumView) initRowRackZone() {
	dv.rowRackZone = NewRowRackZone(RowRackCallbacks{
		OnMuteToggle: func(row int) { dv.toggleMute(row) },
		OnSoloToggle: func(row int) { dv.toggleSolo(row) },
		OnDeleteRow: func(row int) {
			if dv.deleteConfirmRow == row && (dv.frame-dv.deleteConfirmFrame) < 120 {
				dv.DeleteRow(row)
				dv.deleteConfirmRow = -1
			} else {
				dv.deleteConfirmRow = row
				dv.deleteConfirmFrame = dv.frame
			}
			dv.markRowControlsDirty()
		},
		OnOriginReq: func(row int) { dv.originReq = append(dv.originReq, row) },
		OnAddRow: func() {
			dv.logger.Debugf("[drumview] add row button pressed")
			dv.AddRow()
			dv.selRow = len(dv.Rows) - 1
		},
		OnRowSelect: func(row int) { dv.selRow = row },
		OnVolumeChange: func(row int, vol float64) {
			if row >= 0 && row < len(dv.Rows) {
				dv.Rows[row].Volume = vol
				dv.markRowControlsDirty()
			}
		},
		OnContextMenuOpen: func(row int) {
			dv.openContextMenu(row)
		},
		OnInstMenuOpen: func(row int) {
			dv.openInstMenuForRow(row)
		},
		OnColorWheelOpen: func(row int) {
			dv.selRow = row
			if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() && dv.colorMenuRow == row {
				dv.colorWheelComp.Close()
				dv.closeColorWheelPortal()
				return
			}
			if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
				dv.instMenuComp.Close()
				dv.closeInstMenuPortal()
			}
			dv.colorMenuRow = row
			if dv.colorWheelComp != nil {
				rackBounds := dv.widgetRects[WidgetRack]
				if rackBounds.Empty() {
					rackBounds = dv.Bounds
				}
				anchor := dv.rowColorBtns()[row].Rect()
				if anchor.Empty() && row < len(dv.rowLabels()) {
					// Desktop: color button is hidden; use the label as anchor.
					anchor = dv.rowLabels()[row].Rect()
				}
				dv.colorWheelComp.SetProps(ColorWheelProps{
					AnchorRect:   anchor,
					Bounds:       rackBounds,
					RowHeight:    dv.rowHeight(),
					CurrentColor: dv.rowColorAt(row),
					OnColorPick: func(c color.Color) {
						dv.SetRowColorManual(dv.colorMenuRow, c)
					},
					OnClose: func() {},
				})
				dv.colorWheelComp.Open()
				dv.colorWheelComp.ClearHold()
			}
			dv.buildColorMenu()
			dv.openColorWheelPortal()
		},
		OnRenameOpen: func(row int) {
			dv.CloseAllPopups()
			dv.renameRow = row
			r := dv.rowLabels()[row].Rect()
			if dv.renameComp != nil {
				mobileID := fmt.Sprintf("rename-%d", row)
				dv.renameComp.SetProps(RenameProps{
					AnchorRect:    r,
					InitialText:   dv.Rows[row].Name,
					MaxLen:        32,
					MobileInputID: mobileID,
					TextFields:    dv.textFields,
					OnCommit: func(newName string) {
						name := strings.TrimSpace(newName)
						if name != "" && dv.renameRow >= 0 && dv.renameRow < len(dv.Rows) {
							if strings.ContainsAny(name, "/\\<>\x00") {
								dv.notifyErrorKey(i18n.KeyNotifInvalidName)
								dv.renameBox = nil
								dv.renameRow = -1
								return
							}
							dv.logger.Debugf("[drumview] rename instrument row=%d %q -> %q", dv.renameRow, dv.Rows[dv.renameRow].Instrument, name)
							dv.renameInstrumentTo(dv.renameRow, name)
							dv.notifyInfoKey(i18n.KeyNotifRenamedInstrument, name)
						}
						dv.renameBox = nil
						dv.renameRow = -1
					},
					OnCancel: func() {
						dv.renameBox = nil
						dv.renameRow = -1
					},
				})
				dv.renameComp.Open()
				dv.renameComp.ClearHold()
				dv.openRenamePortal()
				if tb := dv.renameComp.TextBox(); tb != nil {
					dv.renameBox = tb
				}
			}
			if dv.renameBox == nil {
				dv.renameBox = NewTextField(dv.textFields, renameTextFieldSpec, 32)
				dv.renameBox.Rect = r
				dv.renameBox.Accept = AcceptTextRune
				dv.renameBox.SetText(dv.Rows[row].Name)
				dv.renameBox.focused = true
				dv.renameBox.anim = 1
			}
		},
		OnFXPanelToggle: func(row int) { dv.toggleFXPanel(row) },
		OnVolPopupOpen:  func(row int) { dv.openVolumePopup(row) },
		VolumePopup:     dv.volPopup,
		OnScrollChanged: func() {
			dv.rowScrollFromZone = true
		},
		Rows:              func() []*DrumRow { return dv.Rows },
		IsInstrumentAvail: func(id instrument.ID) bool { return dv.IsInstrumentAvailable(id) },
		RowHeight:         func() int { return dv.rowHeight() },
		DeleteConfirm:     func() (int, int64) { return dv.deleteConfirmRow, dv.deleteConfirmFrame },
		RenameRow:         func() int { return dv.renameRow },
		IsMobileEQMode:    func() bool { return dv.MobileEQMode() },
		Frame:             func() int64 { return dv.frame },
	})
	dv.rowRackZone.SetPortal(dv.overlayTree.Portal())
	dv.tree.RegisterZone(dv.rowRackZone, ZRowRack)
}

func (dv *DrumView) initTimelineZone() {
	dv.timelineZone = NewTimelineZone(TimelineCallbacks{
		Rows:                 func() []*DrumRow { return dv.Rows },
		IsPlaying:            func() bool { return dv.isPlaying },
		Follow:               func() bool { return dv.FollowPlayback() },
		BPM:                  func() int { return dv.bpm },
		SecPerBeat:           func() float64 { return dv.secPerBeat },
		TimelineUnitsPerBeat: func() int { return dv.timelineUnitsPerBeat },
		Length:               func() int { return dv.Length },
		Offset:               func() int { return dv.Offset },
		RowOffset:            func() int { return dv.rowOffset },
		VisibleRows:          func() int { return dv.visibleRows() },
		RowHeight:            func() int { return dv.rowHeight() },
		Cell:                 func() int { return dv.cell },
		TimelineBeats:        func() int { return dv.timelineBeats },
		Frame:                func() int64 { return dv.frame },
		OnOffsetChange: func(newOffset int) {
			if newOffset != dv.Offset {
				dv.Offset = newOffset
				dv.offsetChanged = true
				dv.logger.Tracef("[drumview/drag] offset=%d", dv.Offset)
			}
		},
		OnSeekToStep: func(step int, origin hooks.SeekOrigin) {
			dv.RequestSeekSteps(step, origin)
		},
		PlayheadStep: func() int {
			if dv.game != nil {
				return dv.game.playheadAbsSubdiv()
			}
			return 0
		},
		OnRowsLayerDirty: func() {
			dv.rowsLayerDirty = true
		},
		BeatLength: func() int {
			if dv.Graph != nil {
				return dv.Graph.BeatLength()
			}
			return 0
		},
		SimpleDraw:      func() bool { return dv.simpleDraw },
		PerfDrawLite:    func() bool { return dv.perfDrawLite },
		MobileEQActive:  func() bool { return Profile().IsMobile() && dv.MobileEQMode() },
		BeatCounterRect: func() image.Rectangle { return dv.beatCounterRect },
		RowsTopY:        func() int { return dv.Bounds.Min.Y + dv.headerH },
		SetTimelineBeats: func(beats int) {
			dv.timelineBeats = beats
		},
		OnOpenNavigation: func() { dv.OpenNavigation() },
		OnRowScrollWheel: func(steps int) bool {
			// The rack's own wheel scroll, so the grid scrolls exactly like the labels.
			return dv.rowRackZone.ScrollByWheel(steps)
		},
		OnRowScrollDrag: func(targetRowOffset int) {
			dv.rowRackZone.syncScroll()
			vs := &dv.rowRackZone.RowScroll().VS
			if targetRowOffset < 0 {
				targetRowOffset = 0
			}
			max := vs.Total - vs.Visible
			if max < 0 {
				max = 0
			}
			if targetRowOffset > max {
				targetRowOffset = max
			}
			if vs.First != targetRowOffset {
				vs.First = targetRowOffset
				dv.rowRackZone.flushScroll()
				dv.rowScrollFromZone = true
			}
		},
		DrawRowComposite: func(dst *ebiten.Image) {
			dv.drawRowComposite(dst)
		},
	})
	dv.timelineZone.SetPortal(dv.overlayTree.Portal())
	dv.timelineZone.SetButtons(dv.transportZone.trackBtn, dv.lenDecBtn, dv.lenIncBtn)
	dv.tree.RegisterZone(dv.timelineZone, ZTimeline)
}

func (dv *DrumView) initLayoutResizeAndDrawLayers() {
	// In the AUDIO subtree: the divider pill straddles the panel's top edge, and only in the same
	// HitIndex can its z beat the panel's catch-all.
	dv.layoutResizeZone = newLayoutResizeZone(dv.layoutHandler)
	dv.audioTree.RegisterZone(dv.layoutResizeZone, ZResize)

	// Every pixel in the drum pane comes from these layers or the zones; registered last so zones
	// and layers walk in one ordered pass.
	dv.tree.RegisterLayer(newBackgroundLayer(dv))
	dv.tree.RegisterLayer(newEQPeekLayer(dv))
	dv.tree.RegisterLayer(newRackMaskLayer(dv))
	dv.tree.RegisterLayer(newTransportPulseLayer(dv))
	dv.audioTree.RegisterLayer(newViewSwitchLayer(dv))
	dv.tree.RegisterLayer(newRowZoomChipsLayer(dv))
	dv.tree.RegisterLayer(newLayoutPillsLayer(dv))
	dv.tree.RegisterLayer(newLayoutGuidesLayer(dv))
	// Row↔EQ divider (ZRowEQDivider=186) marks the panel boundary — audioTree.
	dv.rowEQDivider = newRowEQDividerLayer(dv)
	dv.audioTree.RegisterLayer(dv.rowEQDivider)
	// A Layer, not a Zone: it registers no hit area, so it can never swallow a tap.
	dv.audioTree.RegisterLayer(newDrumToastLayer(dv))
}

func (dv *DrumView) initMobileViewControls() {
	// Constructed on every platform so the field is valid; only mobile gives it a rect.
	dv.viewSwitchSegmented = NewSegmentedControl(
		dv.bottomNavLabels(),
		0, // Pads active by default
		func(i int) {
			modes := bottomNavModes()
			if i >= 0 && i < len(modes) {
				dv.setViewMode(modes[i])
			}
		},
	)

	// They change the visible beat count, not row size or the pane; desktop has lenIncBtn/lenDecBtn.
	dv.rowZoomIncBtn = IconOnlyButton(IconPlus, LenIncStyle)
	dv.rowZoomIncBtn.OnClick = func() {
		dv.lenIncPressed = true
	}
	dv.rowZoomDecBtn = IconOnlyButton(IconMinus, LenDecStyle)
	dv.rowZoomDecBtn.OnClick = func() {
		dv.lenDecPressed = true
	}
}

// Package-level so the per-frame disabled-state refresh never allocates; read-only.
var bottomNavModeList = []viewMode{
	viewModeRows,
	viewModeEQ,
	viewModeWave,
	viewModeSpectrum,
	viewModeMeters,
	viewModeChain,
	viewModeSynth,
	viewModeSampler,
}

var bottomNavModeListProduction = []viewMode{
	viewModeRows,
	viewModeEQ,
	viewModeWave,
	viewModeSpectrum,
	viewModeMeters,
	viewModeSynth,
	viewModeSampler,
}

// Shared by the click handler and bottomNavSynthIndex so they cannot drift.
func bottomNavModes() []viewMode {
	if devToolsOn() {
		return bottomNavModeList
	}
	return bottomNavModeListProduction
}

func viewModeAvailable(m viewMode) bool { return m != viewModeChain || devToolsOn() }

// The one inverse of bottomNavModeList, so the selected segment and the click mapping cannot drift.
func segmentIndexForViewMode(m viewMode) int {
	for i, mode := range bottomNavModes() {
		if mode == m {
			return i
		}
	}
	return -1
}

func bottomNavSynthIndex() int { return segmentIndexForViewMode(viewModeSynth) }

// Called again on a locale switch: the SegmentedControl caches its labels.
func (dv *DrumView) bottomNavLabels() []string {
	modes := bottomNavModes()
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = bottomNavLabel(m)
	}
	return out
}

// bottomNavLabel is a bottom-nav segment's label in the active locale.
func bottomNavLabel(m viewMode) string {
	switch m {
	case viewModeRows:
		return i18n.T(i18n.KeyNavPads)
	case viewModeEQ:
		return i18n.T(i18n.KeyTabEQShort)
	case viewModeWave:
		return i18n.T(i18n.KeyTabWaveShort)
	case viewModeSpectrum:
		return i18n.T(i18n.KeyTabSpectrumShort)
	case viewModeMeters:
		return i18n.T(i18n.KeyTabLevelsShort)
	case viewModeChain:
		return i18n.T(i18n.KeyTabChainShort)
	case viewModeSynth:
		return i18n.T(i18n.KeyTabSynthShort)
	case viewModeSampler:
		return i18n.T(i18n.KeyTabSamplerShort)
	}
	return ""
}
