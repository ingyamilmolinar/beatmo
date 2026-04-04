package ui

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

/* ─── ctor ─────────────────────────────────────────────────── */

func NewDrumView(b image.Rectangle, g *model.Graph, logger *game_log.Logger) *DrumView {
	opts := audio.Instruments()
	inst := "snare"
	name := "Snare"
	if len(opts) > 0 {
		inst = opts[0]
		name = strings.ToUpper(inst[:1]) + inst[1:]
	}
	dv := &DrumView{
		Bounds:               b,
		labelW:               100,
		bpm:                  120,
		secPerBeat:           0.5,
		headerH:              timelineHeight,
		eqH:                  eqPanelHeight,
		bgDirty:              true,
		Graph:                g,
		logger:               logger,
		Length:               8, // Default length
		Offset:               0,
		instOptions:          opts,
		instRefreshDirty:     true,
		labelWidthDirty:      true, // Force initial label width calculation
		uploadCh:             make(chan uploadResult, 1),
		importCh:             make(chan importResult, 1),
		timelineUnitsPerBeat: 1,
		timelineBeats:        8,
		selRow:               0,
		deleteConfirmRow:     -1,
		renameRow:            -1,
		follow:               true,
		samplePath:           make(map[string]string),
	}
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
		},
	})
	// Initialize overlay components (Phase 5)
	dv.subdivMenuComp = NewSubdivMenuComponent()
	dv.renameComp = NewRenameComponent()
	dv.colorWheelComp = NewColorWheelComponent()
	dv.instMenuComp = NewInstrumentMenuComponent()

	// rowScroll and rowVolGroup are now created by RowRackZone (Phase 4).
	// Fields are aliased after zone creation below tree initialization.
	dv.eqChannelScroll = NewScrollBehavior(DropdownScrollbarStyle, TouchRowHeight())
	dv.eqActiveChannel = "main"
	dv.eqCurveDragBand = -1
	dv.eqCurveDirty = true
	dv.hpfEnabled = false
	dv.hpfCutoffHz = 20
	dv.lpfEnabled = false
	dv.lpfCutoffHz = 20000
	_ = audio.EnableChannelAnalyzer("main", 512)
	// Transport buttons are now created by TransportZone (Phase 3).
	// Fields are aliased after zone creation below tree initialization.
	// Non-transport buttons remain here.
	dv.lenDecBtn = NewButton("", LenDecStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Length - button pressed")
		dv.lenDecPressed = true
		dv.lenDecAnim = 1
	})
	dv.lenDecBtn.Repeat = true
	dv.lenDecBtn.Icon = "minus"
	dv.lenDecBtn.IconColor = colIncDecIconHi
	if Profile().IsMobile() {
		dv.lenDecBtn.Style = TransportDecStyle
		dv.lenDecBtn.IconColor = colIncDecIcon
	}
	dv.lenIncBtn = NewButton("", LenIncStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Length + button pressed")
		dv.lenIncPressed = true
		dv.lenIncAnim = 1
	})
	dv.lenIncBtn.Repeat = true
	dv.lenIncBtn.Icon = "plus"
	dv.lenIncBtn.IconColor = colIncDecIconHi
	if Profile().IsMobile() {
		dv.lenIncBtn.Style = TransportIncStyle
		dv.lenIncBtn.IconColor = colIncDecIcon
	}
	dv.saveBtn = NewButton("Save", InstButtonStyle, nil)
	// Default: collapsed on mobile. For Go tests with forceSmallScreenForTest,
	// Profile().IsMobile() is already true at construction. On WASM, it becomes true
	// later when Layout() calls SetTouchScreenSize(); refreshWidgetLayout()
	// handles the late-init case.
	if Profile().IsMobile() {
		dv.mobileEQCollapsed = true
		dv.mobileEQInited = true
	}
	// addRowBtn is now created by RowRackZone (Phase 4).
	// Field is aliased after zone creation below tree initialization.

	baseCol := instColor(inst)
	// use uniqueness even for first row to keep logic consistent
	uniq := dv.ensureUniqueColor(baseCol, -1)
	dv.Rows = []*DrumRow{{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), CellTypes: make([]model.NodeType, dv.Length), Color: uniq, Origin: model.InvalidNodeID, Volume: 1, EQGainsDB: make([]float64, len(eqBandDefs))}}
	dv.SetBeatLength(dv.Length) // Initialize graph's beat length
	// Initialize instrument availability/options immediately so early
	// highlight/audio paths (e.g., tests spawning pulses before the first
	// Update) see valid instruments and do not suppress playback.
	dv.widgetRects = map[WidgetKind]image.Rectangle{}
	// Default widget grid: 2 columns (instrument vs timeline) × 3 rows
	// (header, rows, EQ). Weights read from the active LayoutProfile.
	pp := Profile()
	dv.widgets = NewWidgetBoard(b, pp.ColWeights, pp.RowWeights)
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetTransport, Title: "Transport", Col: 0, Row: 0, ColSpan: 1, RowSpan: 1, MinW: 180, MinH: mobileTransportMinH()})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetRack, Title: "Instruments", Col: 0, Row: 1, ColSpan: 1, RowSpan: 1, MinW: 200, MinH: dv.rowHeight() * 4, Editable: true})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetTimeline, Title: "Timeline", Col: 1, Row: 0, ColSpan: 1, RowSpan: 2, MinW: 320, MinH: dv.rowHeight() * 4, Editable: true})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetWave, Title: "Wave/EQ", Col: 0, Row: 2, ColSpan: 2, RowSpan: 1, MinW: 240, MinH: eqPanelHeight, Editable: true})
	// Scope panel is not in the widget board grid — it's manually positioned
	// below the EQ panel in calcLayout when scopeVisible is true.
	// On mobile, hide the Wave/EQ widget by default.
	if dv.mobileEQCollapsed {
		dv.widgets.ToggleWidget(WidgetWave, false)
	}
	dv.refreshWidgetLayout()
	dv.refreshInstruments()
	// recalcButtons() is deferred until after TransportZone creation below,
	// because transport buttons are owned by the zone and aliased to DrumView.
	// Calling recalcButtons() here would crash on nil button pointers.
	dv.rowCachePadPx = defaultRowCachePadPx
	dv.rowsLayerPadPx = defaultRowsLayerPadPx
	if runtime.GOARCH == "wasm" {
		dv.rowsStripingEnabled = true
		dv.rowsStripeCount = 0
		dv.rowsStripeAuto = true
	} else {
		dv.rowsStripingEnabled = false
		dv.rowsStripeCount = 0
		dv.rowsStripeAuto = false
	}
	dv.layoutDragIdx = -1
	dv.layoutHoverIdx = -1
	dv.layoutHandler = NewLayoutResizeHandler(dv)
	dv.rowsStripeScratch = nil
	// Initialize zone-based component tree (Phase 1 infrastructure).
	dv.tree = NewDrumViewTree()
	dv.tree.SetBounds(b)
	dv.tree.SetDragActive(func() bool { return dv.anyDragActive() })
	// Phase 2: EQ panel zone — owns EQ sliders, buttons, and state.
	// Callbacks delegate audio operations to DrumView's existing methods.
	dv.eqPanelZone = NewEQPanelZone(EQCallbacks{
		OnGainChange: func(band int, db float64) {
			ch := dv.eqPanelZone.ActiveChannel()
			if ch == "main" {
				dv.applyMasterEQ()
			} else {
				for j, r := range dv.Rows {
					if r.Instrument == ch {
						dv.ensureRowEQ(j)
						if band < len(r.EQGainsDB) {
							r.EQGainsDB[band] = db
						}
						dv.applyRowEQ(j)
						break
					}
				}
			}
		},
		OnMuteToggle: func(band int) {
			dv.toggleEQBandMute(band)
		},
		OnChannelChange: func(id string) {
			dv.setEQActiveChannel(id)
			// Also set the analyzer detail channel so Wave/Spectrum tabs
			// show the selected instrument's data.
			if svc := audio.AnalyzerService(); svc != nil {
				if id == "main" || id == "" {
					svc.SetDetailChannel("")
				} else {
					svc.SetDetailChannel(id)
				}
			}
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
		AnalyzerSnapshot: func(ch string) audio.AnalyzerSnapshot {
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
		AnalyzerState: func() *analyzer.State {
			svc := audio.AnalyzerService()
			if svc == nil {
				return nil
			}
			return svc.State()
		},
		OnFreezeToggle: func() bool {
			svc := audio.AnalyzerService()
			if svc == nil {
				return false
			}
			state := svc.State()
			if state != nil && state.Capture != nil && state.Capture.Frozen {
				svc.Unfreeze()
				return false
			}
			svc.Freeze()
			return true
		},
	})
	dv.eqPanelZone.SetPortal(dv.tree.Portal())
	dv.tree.RegisterZone(dv.eqPanelZone, 130)

	// Scope panel zone — oscilloscope A/B pipeline comparison.
	dv.scopePanelZone = NewScopePanelZone(ScopeCallbacks{
		ScopeState: func() *scope.State {
			svc := audio.ScopeService()
			if svc == nil {
				return nil
			}
			return svc.State()
		},
		ActiveRows: func() []*DrumRow {
			return dv.Rows
		},
		OnTapAChange: func(stage scope.Stage) {
			if svc := audio.ScopeService(); svc != nil {
				svc.SetTapA(stage)
			}
		},
		OnTapBChange: func(stage scope.Stage) {
			if svc := audio.ScopeService(); svc != nil {
				svc.SetTapB(stage)
			}
		},
		OnClearTapA: func() {
			if svc := audio.ScopeService(); svc != nil {
				svc.ClearTapA()
			}
		},
		OnClearTapB: func() {
			if svc := audio.ScopeService(); svc != nil {
				svc.ClearTapB()
			}
		},
		OnInstrChange: func(id string) {
			if svc := audio.ScopeService(); svc != nil {
				svc.SetInstrument(id)
			}
		},
		OnFreezeToggle: func() bool {
			svc := audio.ScopeService()
			if svc == nil {
				return false
			}
			if svc.IsFrozen() {
				svc.Unfreeze()
				return false
			}
			svc.Freeze()
			return true
		},
		OnClose: func() {
			dv.SetScopeVisible(false)
		},
	})
	dv.scopePanelZone.SetPortal(dv.tree.Portal())
	dv.tree.RegisterZone(dv.scopePanelZone, 135)

	// EQ zone now owns all EQ sliders, buttons, and state. DrumView
	// provides accessor methods (eqSliders(), eqBandGainsDB(), etc.)
	// that delegate to eqPanelZone.

	// EQ zone initial layout is deferred to the recalcButtons()+calcLayout()
	// call after both zones (EQ + Transport) are created and aliased.

	// Phase 3: Transport zone — owns transport buttons, BPM state, and
	// animation. Callbacks delegate to DrumView's existing methods.
	dv.transportZone = NewTransportZone(TransportCallbacks{
		OnPlayToggle: func() {
			dv.logger.Infof("[DRUMVIEW] Play button pressed")
		},
		OnStop: func() {
			dv.logger.Infof("[DRUMVIEW] Stop button pressed")
		},
		OnBPMChange: func(bpm int) {
			dv.bpm = bpm
			dv.secPerBeat = 60.0 / float64(bpm)
			dv.logger.Infof("[DRUMVIEW] BPM set: -> %d", bpm)
		},
		OnNotifyError: func(msg string) {
			dv.notifyError(msg)
		},
		OnFollowChange: func(follow bool) {
			if follow {
				dv.logger.Infof("[DRUMVIEW] Track/Free toggled: follow=Track")
			} else {
				dv.logger.Infof("[DRUMVIEW] Track/Free toggled: follow=Free")
			}
		},
		OnUploadClick: func() {
			dv.logger.Infof("[DRUMVIEW] Upload button pressed")
			dv.logger.Debugf("[DRUMVIEW] Upload button clicked. uploading=%v naming=%v menuOpen=%v", dv.uploading, dv.IsNamingOpen(), dv.IsInstMenuOpen())
			if dv.importing {
				return
			}
			if !dv.uploading && !dv.IsNamingOpen() {
				dv.uploading = true
				dv.logger.Debugf("[DRUMVIEW] Opening file chooser")
				go func() {
					path, err := audio.SelectWAV()
					dv.uploadCh <- uploadResult{path: path, err: err}
				}()
			}
		},
		OnImportClick: func() {
			dv.logger.Infof("[DRUMVIEW] Import button pressed")
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
			selectJSONAsyncFn(func(data []byte, err error) {
				jsLog("Import callback invoked; bytes=%d err=%v", len(data), err)
				dv.importCh <- importResult{data: data, err: err}
			})
		},
		OnExportClick: func() {
			dv.logger.Infof("[DRUMVIEW] Export button pressed")
			if err := dv.Export(); err != nil {
				dv.logger.Infof("[DRUMVIEW] Export failed: %v", err)
			}
		},
		OnViewCycle: func() {
			dv.cycleViewMode()
		},
		IsPlaying: func() bool {
			return dv.isPlaying
		},
		GetMainVolume: audio.MainVolume,
		SetMainVolume: func(v float64) {
			audio.SetMainVolume(v)
		},
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
		OnOverflowOpen: func() {
			if dv.IsOverflowMenuOpen() {
				dv.closeOverflowMenu()
			} else {
				dv.CloseAllPopups()
				dv.initOverflowScroll()
				if Profile().IsMobile() {
					dv.registerFilePickerRects()
				}
				dv.openOverflowMenuPortal()
			}
		},
		OnMasterVolClick: func() {
			dv.masterVolPopup.Open(dv.mainVolIconRect, dv.Bounds, dv.headerH)
			dv.openMasterVolPopupPortal()
		},
		MasterVolPopup: dv.masterVolPopup,
	})
	dv.transportZone.SetPortal(dv.tree.Portal())
	dv.tree.RegisterZone(dv.transportZone, 100)

	// Sync initial state from ctor-created values.
	dv.transportZone.SetBPM(dv.bpm)
	dv.transportZone.SetFollow(dv.follow)

	// Wire input blocking: BPM box is force-blurred when popups/overlays are open.
	// Note: we don't check tree.Suppress() here — suppress is a transient flag
	// that persists into the next frame's Phase 2 (zone.Update()), which would
	// incorrectly force-blur the BPM box after a normal click on it.
	dv.transportZone.SetInputBlocked(func() bool {
		treeBlocking := dv.tree != nil && dv.tree.Portal().IsOpen()
		return dv.anyDropdownOpen() || treeBlocking
	})

	// Phase 4: Row rack zone — owns per-row buttons/sliders, add-row button,
	// row scrolling, and row volume slider group.
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
			dv.logger.Infof("[DRUMVIEW] Add row button pressed")
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
					AnchorRect: anchor,
					Bounds:     rackBounds,
					RowHeight:  dv.rowHeight(),
					OnColorPick: func(c color.Color) {
						dv.SetRowColor(dv.colorMenuRow, c)
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
					OnCommit: func(newName string) {
						name := strings.TrimSpace(newName)
						if name != "" && dv.renameRow >= 0 && dv.renameRow < len(dv.Rows) {
							if strings.ContainsAny(name, "/\\<>\x00") {
								dv.notifyError("Invalid characters in name")
								dv.renameBox = nil
								dv.renameRow = -1
								return
							}
							oldID := dv.Rows[dv.renameRow].Instrument
							newID := strings.ToLower(name)
							dv.logger.Infof("[DRUMVIEW] Rename instrument row=%d %q -> %q", dv.renameRow, oldID, newID)
							audio.RenameInstrument(oldID, newID)
							if dv.samplePath != nil {
								if p, ok := dv.samplePath[oldID]; ok {
									dv.samplePath[newID] = p
									delete(dv.samplePath, oldID)
								}
							}
							dv.Rows[dv.renameRow].Instrument = newID
							dv.Rows[dv.renameRow].Name = name
							dv.rowLabels()[dv.renameRow].Text = name
							customColors[newID] = dv.Rows[dv.renameRow].Color
							dv.invalidateLabelCaches()
							dv.refreshInstruments()
							dv.markRowControlsDirty()
							dv.bgDirty = true
							dv.notifyInfo("Renamed instrument to: " + name)
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
				dv.renameBox = NewTextInput(r, BPMBoxStyle)
				dv.renameBox.MaxLen = 32
				dv.renameBox.SetText(dv.Rows[row].Name)
				dv.renameBox.focused = true
				dv.renameBox.anim = 1
			}
		},
		OnFXPanelToggle:   func(row int) { dv.toggleFXPanel(row) },
		OnVolPopupOpen:    func(row int) { dv.openVolumePopup(row) },
		VolumePopup:       dv.volPopup,
		OnSaveInstrument: func(row int) { dv.saveInstrument(row) },
		OnScrollChanged: func() {
			dv.rowScrollFromZone = true
		},
		Rows:              func() []*DrumRow { return dv.Rows },
		IsInstrumentAvail: func(id string) bool { return dv.IsInstrumentAvailable(id) },
		RowHeight:         func() int { return dv.rowHeight() },
		DeleteConfirm:     func() (int, int64) { return dv.deleteConfirmRow, dv.deleteConfirmFrame },
		RenameRow:         func() int { return dv.renameRow },
		IsMobileEQMode:    func() bool { return dv.mobileEQMode },
		Frame:             func() int64 { return dv.frame },
	})
	dv.rowRackZone.SetPortal(dv.tree.Portal())
	dv.tree.RegisterZone(dv.rowRackZone, 120)

	// RowRack zone fields are now accessed via accessor methods on DrumView
	// (addRowBtn(), rowVolGroup(), rowScroll(), etc.).

	// Phase 5: Timeline zone — owns drag/scrub input for the grid and
	// timeline bar areas. Callbacks delegate offset changes to DrumView.
	dv.timelineZone = NewTimelineZone(TimelineCallbacks{
		Rows:                 func() []*DrumRow { return dv.Rows },
		IsPlaying:            func() bool { return dv.isPlaying },
		Follow:               func() bool { return dv.follow },
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
				dv.logger.Tracef("[DRUMVIEW/DRAG] offset=%d", dv.Offset)
			}
		},
		OnScrubPosition: func(newOffset int) {
			if newOffset != dv.Offset {
				dv.Offset = newOffset
				dv.offsetChanged = true
				dv.logger.Tracef("[DRUMVIEW/SCRUB] offset=%d len=%d total=%d", dv.Offset, dv.Length, dv.timelineBeats)
			}
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
		MobileEQActive:  func() bool { return Profile().IsMobile() && dv.mobileEQMode },
		BeatCounterRect: func() image.Rectangle { return dv.beatCounterRect },
		RowsTopY:        func() int { return dv.Bounds.Min.Y + dv.headerH },
		SetTimelineBeats: func(beats int) {
			dv.timelineBeats = beats
		},
		OnRowScrollWheel: func(steps int) bool {
			dv.rowRackZone.syncScroll()
			if dv.rowRackZone.RowScroll().HandleWheel(steps) {
				dv.rowRackZone.flushScroll()
				dv.rowScrollFromZone = true
				return true
			}
			return false
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
	dv.timelineZone.SetPortal(dv.tree.Portal())
	dv.timelineZone.SetButtons(dv.transportZone.trackBtn, dv.lenDecBtn, dv.lenIncBtn)
	dv.tree.RegisterZone(dv.timelineZone, 110)

	// Layout resize zone — wraps LayoutResizeHandler for tree-based input.
	dv.layoutResizeZone = newLayoutResizeZone(dv.layoutHandler)
	dv.tree.RegisterZone(dv.layoutResizeZone, ZResize)

	// Now that all zones (EQ + Transport + RowRack + Timeline) are created
	// and aliased, run the deferred layout initialization that was skipped
	// earlier. This computes button positions, zone rects, and row caches.
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
