package ui

import (
	"fmt"
	"image"
	"runtime"
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
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
		activeSlider:         -1,
		deleteConfirmRow:     -1,
		renameRow:            -1,
		follow:               true,
		samplePath:           make(map[string]string),
	}
	dv.overlays = NewOverlayStack()
	// Register overlay handlers for modal UI elements
	dv.overlays.Push(&InstrumentMenuOverlay{dv: dv})
	dv.overlays.Push(&ColorWheelOverlay{dv: dv})
	dv.overlays.Push(&SubdivMenuOverlay{dv: dv})
	dv.overlays.Push(&RenameOverlay{dv: dv})
	dv.overlays.Push(&EQChannelMenuOverlay{dv: dv})
	dv.fxPanelOverlay = &FXPanelOverlay{dv: dv}
	dv.contextMenuOverlay = &ContextMenuOverlay{dv: dv}
	dv.overflowMenuOverlay = &OverflowMenuOverlay{dv: dv}
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
				if row < len(dv.rowVolSliders) {
					dv.rowVolSliders[row].Value = v
				}
				dv.markRowControlsDirty()
			}
		},
	})
	dv.volPopupOverlay = &SliderPopupOverlay{Popup: dv.volPopup}
	dv.masterVolPopup = NewSliderPopup(SliderPopupConfig{
		ID:     "master-volume-popup",
		ZIndex: 226,
		GetValue: func() float64 {
			if dv.mainVolSlider != nil {
				return dv.mainVolSlider.Value
			}
			return 0
		},
		SetValue: func(v float64) {
			if dv.mainVolSlider != nil {
				dv.mainVolSlider.Value = v
			}
			audio.SetMainVolume(v)
		},
	})
	dv.masterVolPopupOverlay = &SliderPopupOverlay{Popup: dv.masterVolPopup}
	dv.eqPopup = NewSliderPopup(SliderPopupConfig{
		ID:     "eq-popup",
		ZIndex: 227,
		Label: func() string {
			band := dv.eqPopupBand
			if band >= 0 && band < len(eqCenterLabels) {
				return eqCenterLabels[band]
			}
			return ""
		},
		GetValue: func() float64 {
			band := dv.eqPopupBand
			ch := dv.activeEQChannel()
			if ch == "main" {
				if band < len(dv.eqBandGainsDB) {
					return gainDBToSlider(dv.eqBandGainsDB[band])
				}
			} else {
				for _, row := range dv.Rows {
					if row.Instrument == ch && band < len(row.EQGainsDB) {
						return gainDBToSlider(row.EQGainsDB[band])
					}
				}
			}
			return 0.5
		},
		SetValue: func(val float64) {
			band := dv.eqPopupBand
			gain := sliderToGainDB(val)
			ch := dv.activeEQChannel()
			if ch == "main" {
				if band < len(dv.eqBandGainsDB) {
					dv.eqBandGainsDB[band] = gain
				}
				dv.applyMasterEQ()
			} else {
				for j, row := range dv.Rows {
					if row.Instrument == ch {
						dv.ensureRowEQ(j)
						row.EQGainsDB[band] = gain
						dv.applyRowEQ(j)
						break
					}
				}
			}
			if band < len(dv.eqSliders) && dv.eqSliders[band] != nil {
				dv.eqSliders[band].Value = val
			}
		},
	})
	dv.eqPopupOverlay = &SliderPopupOverlay{Popup: dv.eqPopup}
	dv.overlays.Push(dv.fxPanelOverlay)
	dv.overlays.Push(dv.contextMenuOverlay)
	dv.overlays.Push(dv.overflowMenuOverlay)
	dv.overlays.Push(dv.volPopupOverlay)
	dv.overlays.Push(dv.masterVolPopupOverlay)
	dv.overlays.Push(dv.eqPopupOverlay)
	dv.overlays.Push(&NamingOverlay{dv: dv}) // highest z-index (pushed last)

	// Initialize overlay components (Phase 5)
	dv.subdivMenuComp = NewSubdivMenuComponent("subdiv-menu")
	dv.renameComp = NewRenameComponent("rename")
	dv.colorWheelComp = NewColorWheelComponent("color-wheel")
	dv.instMenuComp = NewInstrumentMenuComponent("inst-menu")

	dv.rowScroll = NewScrollBehavior(ScrollbarStyleForPlatform(), TouchRowHeight())
	dv.eqChannelScroll = NewScrollBehavior(DropdownScrollbarStyle, TouchRowHeight())
	dv.eqActiveChannel = "main"
	dv.eqCurveDragBand = -1
	dv.eqCurveDirty = true
	dv.hpfEnabled = false
	dv.hpfCutoffHz = 20
	dv.lpfEnabled = false
	dv.lpfCutoffHz = 20000
	_ = audio.EnableChannelAnalyzer("main", 512)
	dv.playBtn = NewButton("", PlayButtonStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Play button pressed")
		dv.playPressed = true
		dv.playAnim = 1
	})
	dv.playBtn.Icon = "play"
	if isSmallScreen() {
		dv.playBtn.Style = TransportPlayStyle
		dv.playBtn.IconColor = colPlayIconTint
	}
	dv.stopBtn = NewButton("", StopButtonStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Stop button pressed")
		dv.stopPressed = true
		dv.stopAnim = 1
	})
	dv.stopBtn.Icon = "stop"
	if isSmallScreen() {
		dv.stopBtn.Style = TransportStopStyle
		dv.stopBtn.IconColor = colStopIconTint
	}
	dv.bpmDecBtn = NewButton("", BPMDecStyle, func() {
		dv.logger.Infof("[DRUMVIEW] BPM - button pressed")
		dv.bpmDelta--
		dv.bpmDecAnim = 1
	})
	dv.bpmDecBtn.Repeat = true
	dv.bpmDecBtn.Icon = "minus"
	dv.bpmDecBtn.IconColor = colIncDecIconHi
	if isSmallScreen() {
		dv.bpmDecBtn.Style = TransportDecStyle
		dv.bpmDecBtn.IconColor = colIncDecIcon
	}
	dv.bpmBox = NewTextInput(image.Rect(0, 0, 0, 0), BPMBoxStyle)
	// Cap BPM entry to a small number of runes (max 1000 -> 4 digits). Validation
	// happens on commit; non-digits are allowed in the editor to provide clear
	// error feedback and preserve existing test expectations.
	dv.bpmBox.MaxLen = 4
	dv.bpmBox.SetText("120")
	dv.bpmBox.InputMode = "numeric"
	dv.bpmBox.MobileInputID = "bpm"
	dv.bpmBox.OnFocusGained = func() { softKeyboardShow("numeric") }
	dv.bpmBox.OnFocusLost = func() { softKeyboardHide() }
	dv.bpmIncBtn = NewButton("", BPMIncStyle, func() {
		dv.logger.Infof("[DRUMVIEW] BPM + button pressed")
		dv.bpmDelta++
		dv.bpmIncAnim = 1
	})
	dv.bpmIncBtn.Repeat = true
	dv.bpmIncBtn.Icon = "plus"
	dv.bpmIncBtn.IconColor = colIncDecIconHi
	if isSmallScreen() {
		dv.bpmIncBtn.Style = TransportIncStyle
		dv.bpmIncBtn.IconColor = colIncDecIcon
	}
	subdivStyle := ButtonVisual(InstButtonStyle)
	if isSmallScreen() {
		subdivStyle = TransportMiscStyle
	}
	dv.subdivBtn = NewButton("32", subdivStyle, nil)
	dv.subdivBtn.OnClick = func() {
		// Toggle: close if already open.
		if dv.subdivMenuComp != nil && dv.subdivMenuComp.IsOpen() {
			dv.subdivMenuComp.Close()
			dv.subdivMenuOpen = false
			return
		}
		// Close all other overlays for mutual exclusivity.
		dv.CloseAllPopups()

		// Open subdiv menu using component
		if dv.subdivMenuComp != nil {
			// Set up props and open
			dv.subdivMenuComp.SetProps(SubdivMenuProps{
				AnchorRect: dv.subdivBtn.Rect(),
				Current:    dv.timelineUnitsPerBeat,
				Options:    []int{4, 8, 16, 32},
				RowHeight:  dv.rowHeight(),
				OnSelect: func(value int) {
					if dv.onChangeSubdiv != nil {
						if err := dv.onChangeSubdiv(value); err != nil {
							return
						}
					}
					dv.subdivBtn.Text = fmt.Sprintf("%d", value)
					dv.timelineUnitsPerBeat = value
				},
				OnClose: func() {
					// No additional cleanup needed
				},
			})
			dv.subdivMenuComp.Open()
			SuppressClicksUntilMouseUp()
		}

		// Keep legacy state in sync for now
		dv.subdivMenuOpen = dv.subdivMenuComp != nil && dv.subdivMenuComp.IsOpen()
		if dv.subdivMenuOpen {
			dv.buildSubdivMenu()
		}
	}
	vol := audio.MainVolume()
	if vol < 0 {
		vol = 0
	}
	if vol > 1 {
		vol = 1
	}
	dv.mainVolSlider = NewSlider(vol)
	dv.lenDecBtn = NewButton("", LenDecStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Length - button pressed")
		dv.lenDecPressed = true
		dv.lenDecAnim = 1
	})
	dv.lenDecBtn.Repeat = true
	dv.lenDecBtn.Icon = "minus"
	dv.lenDecBtn.IconColor = colIncDecIconHi
	if isSmallScreen() {
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
	if isSmallScreen() {
		dv.lenIncBtn.Style = TransportIncStyle
		dv.lenIncBtn.IconColor = colIncDecIcon
	}
	trackStyle := ButtonVisual(InstButtonStyle)
	if isSmallScreen() {
		trackStyle = TransportMiscStyle
	}
	dv.trackBtn = NewButton("", trackStyle, func() {
		dv.SetFollow(!dv.follow)
	})
	dv.trackBtn.Icon = "track"
	dv.syncTrackBtnVisual() // set initial icon/style based on follow state
	dv.uploadBtn = NewButton("", UploadBtnStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Upload button pressed")
		dv.logger.Debugf("[DRUMVIEW] Upload button clicked. uploading=%v naming=%v menuOpen=%v", dv.uploading, dv.naming, dv.instMenuOpen)
		if dv.importing {
			return
		}
		dv.instMenuOpen = false
		dv.colorMenuOpen = false
		if !dv.uploading && !dv.naming {
			dv.uploadAnim = 1
			dv.uploading = true
			dv.logger.Debugf("[DRUMVIEW] Opening file chooser")
			go func() {
				path, err := audio.SelectWAV()
				dv.uploadCh <- uploadResult{path: path, err: err}
			}()
		}
	})
	dv.uploadBtn.Icon = "upload"
	dv.importBtn = NewButton("", UploadBtnStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Import button pressed")
		dv.logger.Debugf("[DRUMVIEW] Import button clicked (web=%v)", true)
		// Extra JS console log for web debugging
		log := jsLog
		log("Import button pressed; importing=%v, naming=%v, uploading=%v", dv.importing, dv.naming, dv.uploading)
		if dv.importing || dv.naming || dv.uploading {
			return
		}
		if dv.onImportDialogStart != nil {
			dv.onImportDialogStart()
		}
		dv.importing = true
		dv.importAttemptFrame = int(dv.frame)
		dv.importAttemptUpdate = dv.updateSeq
		dv.colorMenuOpen = false
		dv.instMenuOpen = false
		selectJSONAsyncFn(func(data []byte, err error) {
			jsLog("Import callback invoked; bytes=%d err=%v", len(data), err)
			dv.importCh <- importResult{data: data, err: err}
		})
	})
	dv.importBtn.Icon = "import"
	dv.exportBtn = NewButton("", UploadBtnStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Export button pressed")
		dv.logger.Debugf("[DRUMVIEW] Export button clicked")
		dv.colorMenuOpen = false
		dv.instMenuOpen = false
		if err := dv.Export(); err != nil {
			dv.logger.Infof("[DRUMVIEW] Export failed: %v", err)
		}
	})
	dv.exportBtn.Icon = "export"
	dv.saveBtn = NewButton("Save", InstButtonStyle, nil)
	// Mobile EQ toggle button (legacy, hidden — replaced by viewSwitchBtn)
	dv.eqToggleMobile = NewButton("EQ", InstButtonStyle, func() {
		dv.cycleViewMode()
	})
	// Mobile view switch button (Rows ↔ Audio toggle, icon-only)
	viewStyle := ButtonVisual(TransportMiscStyle)
	if !isSmallScreen() {
		viewStyle = InstButtonStyle
	}
	dv.viewSwitchBtn = NewButton("", viewStyle, func() {
		dv.cycleViewMode()
	})
	dv.viewSwitchBtn.Icon = "audio" // default: in Rows mode, show audio icon
	dv.viewSwitchBtn.IconColor = colIncDecIcon
	// Mobile overflow menu button (visible only on small screens)
	overflowStyle := ButtonVisual(DropdownStyle)
	if isSmallScreen() {
		overflowStyle = TransportMiscStyle
	}
	dv.overflowBtn = NewButton("", overflowStyle, func() {
		if dv.overflowMenuOpen {
			dv.closeOverflowMenu()
		} else {
			dv.CloseAllPopups()
			dv.overflowMenuOpen = true
			dv.initOverflowScroll()
			if isSmallScreen() {
				dv.registerFilePickerRects()
			}
			SuppressClicksUntilMouseUp()
		}
	})
	dv.overflowBtn.Icon = "overflow"
	// Default: collapsed on mobile. For Go tests with forceSmallScreenForTest,
	// isSmallScreen() is already true at construction. On WASM, it becomes true
	// later when Layout() calls SetTouchScreenSize(); refreshWidgetLayout()
	// handles the late-init case.
	if isSmallScreen() {
		dv.mobileEQCollapsed = true
		dv.mobileEQInited = true
	}
	dv.addRowBtn = NewButton("+", InstButtonStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Add row button pressed")
		dv.AddRow()
		dv.selRow = len(dv.Rows) - 1
	})
	dv.addRowBtn.Repeat = true

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
	// (header, rows, EQ). Weights approximate the legacy layout.
	rowWeights := []float64{3, 3, 3} // extra weight for two-row desktop transport
	if isSmallScreen() {
		rowWeights = []float64{3, 5, 2} // two-row mobile transport
	}
	colWeights := []float64{1, 3} // rack gets 25% — tighter fit for controls
	if isSmallScreen() {
		colWeights = []float64{1.5, 1.5} // balanced columns for two-row mobile
	}
	dv.widgets = NewWidgetBoard(b, colWeights, rowWeights)
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetTransport, Title: "Transport", Col: 0, Row: 0, ColSpan: 1, RowSpan: 1, MinW: 180, MinH: mobileTransportMinH()})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetRack, Title: "Instruments", Col: 0, Row: 1, ColSpan: 1, RowSpan: 1, MinW: 200, MinH: dv.rowHeight() * 4, Editable: true})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetTimeline, Title: "Timeline", Col: 1, Row: 0, ColSpan: 1, RowSpan: 2, MinW: 320, MinH: dv.rowHeight() * 4, Editable: true})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetWave, Title: "Wave/EQ", Col: 0, Row: 2, ColSpan: 2, RowSpan: 1, MinW: 240, MinH: eqPanelHeight, Editable: true})
	// On mobile, hide the Wave/EQ widget by default.
	if dv.mobileEQCollapsed {
		dv.widgets.ToggleWidget(WidgetWave, false)
	}
	dv.refreshWidgetLayout()
	dv.refreshInstruments()
	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}
	dv.ensureRowCache()
	dv.markAllRowsDirty()
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
	// Reset global click suppression to ensure clean state for new views/tests.
	suppressClicksUntilRelease = false
	return dv
}
