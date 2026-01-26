package ui

import (
	"fmt"
	"image"
	"runtime"
	"strings"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
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

	// Initialize overlay components (Phase 5)
	dv.subdivMenuComp = NewSubdivMenuComponent("subdiv-menu")
	dv.renameComp = NewRenameComponent("rename")
	dv.colorWheelComp = NewColorWheelComponent("color-wheel")
	dv.instMenuComp = NewInstrumentMenuComponent("inst-menu")

	dv.eqActiveChannel = "main"
	_ = audio.EnableChannelAnalyzer("main", 512)
	dv.playBtn = NewButton("▶", PlayButtonStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Play button pressed")
		dv.playPressed = true
		dv.playAnim = 1
	})
	dv.playBtn.Icon = "play"
	dv.stopBtn = NewButton("■", StopButtonStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Stop button pressed")
		dv.stopPressed = true
		dv.stopAnim = 1
	})
	dv.stopBtn.Icon = "stop"
	dv.bpmDecBtn = NewButton("-", BPMDecStyle, func() {
		dv.logger.Infof("[DRUMVIEW] BPM - button pressed")
		dv.bpmDelta--
		dv.bpmDecAnim = 1
	})
	dv.bpmDecBtn.Repeat = true
	dv.bpmBox = NewTextInput(image.Rect(0, 0, 0, 0), BPMBoxStyle)
	// Cap BPM entry to a small number of runes (max 1000 -> 4 digits). Validation
	// happens on commit; non-digits are allowed in the editor to provide clear
	// error feedback and preserve existing test expectations.
	dv.bpmBox.MaxLen = 4
	dv.bpmBox.SetText("120")
	dv.bpmIncBtn = NewButton("+", BPMIncStyle, func() {
		dv.logger.Infof("[DRUMVIEW] BPM + button pressed")
		dv.bpmDelta++
		dv.bpmIncAnim = 1
	})
	dv.bpmIncBtn.Repeat = true
	dv.subdivBtn = NewButton("32", InstButtonStyle, nil)
	dv.subdivBtn.OnClick = func() {
		// Close other overlays
		dv.colorMenuOpen = false
		dv.instMenuOpen = false
		if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
			dv.colorWheelComp.Close()
		}
		if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
			dv.instMenuComp.Close()
		}

		// Toggle subdiv menu using component
		if dv.subdivMenuComp != nil {
			if dv.subdivMenuComp.IsOpen() {
				dv.subdivMenuComp.Close()
			} else {
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
	dv.lenDecBtn = NewButton("-", LenDecStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Length - button pressed")
		dv.lenDecPressed = true
		dv.lenDecAnim = 1
	})
	dv.lenDecBtn.Repeat = true
	dv.lenIncBtn = NewButton("+", LenIncStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Length + button pressed")
		dv.lenIncPressed = true
		dv.lenIncAnim = 1
	})
	dv.lenIncBtn.Repeat = true
	dv.trackBtn = NewButton("Track", InstButtonStyle, func() {
		dv.SetFollow(!dv.follow)
	})
	dv.uploadBtn = NewButton("Upload", UploadBtnStyle, func() {
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
	dv.importBtn = NewButton("Import", UploadBtnStyle, func() {
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
	dv.exportBtn = NewButton("Export", UploadBtnStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Export button pressed")
		dv.logger.Debugf("[DRUMVIEW] Export button clicked")
		dv.colorMenuOpen = false
		dv.instMenuOpen = false
		if err := dv.Export(); err != nil {
			dv.logger.Infof("[DRUMVIEW] Export failed: %v", err)
		}
	})
	dv.saveBtn = NewButton("Save", InstButtonStyle, nil)
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
	dv.widgets = NewWidgetBoard(b, []float64{1, 2}, []float64{2, 3, 3})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetTransport, Title: "Transport", Col: 0, Row: 0, ColSpan: 1, RowSpan: 1, MinW: 180, MinH: dv.rowHeight() * 2})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetRack, Title: "Instruments", Col: 0, Row: 1, ColSpan: 1, RowSpan: 1, MinW: 200, MinH: dv.rowHeight() * 4, Editable: true})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetTimeline, Title: "Timeline", Col: 1, Row: 0, ColSpan: 1, RowSpan: 2, MinW: 320, MinH: dv.rowHeight() * 4, Editable: true})
	dv.widgets.AddWidget(WidgetPlacement{ID: WidgetWave, Title: "Wave/EQ", Col: 0, Row: 2, ColSpan: 2, RowSpan: 1, MinW: 240, MinH: eqPanelHeight, Editable: true})
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
