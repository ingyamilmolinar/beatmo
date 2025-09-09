package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

const (
	asciiPrintableMin = 32
	asciiPrintableMax = 126
	// timelineHeight reserves vertical space for the transport controls and
	// timeline bar above the drum rows. Increasing this ensures row labels
	// never overlap with the top control panel.
	timelineHeight    = 110
	timelineBarHeight = 10
	buttonPad         = 2
)

/* ───────────────────────────────────────────────────────────── */

type DrumRow struct {
	Name       string
	Instrument string
	Steps      []bool
	Color      color.Color
	Origin     model.NodeID
	Node       *uiNode
	Volume     float64
	Muted      bool
	Solo       bool
}

func instColor(id string) color.Color {
	if c, ok := instColors[id]; ok {
		return c
	}
	if c, ok := customColors[id]; ok {
		return c
	}
	c := customPalette[nextCustomColor%len(customPalette)]
	nextCustomColor++
	customColors[id] = c
	return c
}

/* ───────────────────────────────────────────────────────────── */

type uploadResult struct {
	path string
	err  error
}

type importResult struct {
	data []byte
	err  error
}

type DrumView struct {
	Rows   []*DrumRow
	Bounds image.Rectangle
	Graph  *model.Graph
	logger *game_log.Logger

	cell      int // px per step
	labelW    int
	controlsW int // width reserved for control buttons

	// control-panel components
	playBtn   *Button
	stopBtn   *Button
	bpmDecBtn *Button // decrease BPM
	bpmBox    *TextInput
	bpmIncBtn *Button // increase BPM
	lenDecBtn *Button // decrease length
	lenIncBtn *Button // increase length
	trackBtn  *Button // toggle follow playback
	uploadBtn *Button
	importBtn *Button
	exportBtn *Button
	saveBtn   *Button

	// per-row components
	addRowBtn     *Button
	rowLabels     []*Button
	rowEditBtns   []*Button
	rowDeleteBtns []*Button
	rowVolSliders []*Slider
	rowOriginBtns []*Button
	rowMuteBtns   []*Button
	rowSoloBtns   []*Button
	selRow        int
	activeSlider  int // index of slider capturing mouse events, -1 if none

	// instrument selection dropdown
	instMenuOpen bool
	instMenuRow  int
	instMenuBtns []*Button
	instHold     bool

	deleted    []deletedRow
	added      []int
	originReq  []int
	renameRow  int
	renameBox  *TextInput
	renameHold bool

	// import handler
	onImport func([]byte) error

	bgDirty bool
	bgCache []*ebiten.Image

	instOptions []string

	uploading  bool
	uploadCh   chan uploadResult
	pendingWAV string
	naming     bool
	nameInput  string

	// JSON import
	importing bool
	importCh  chan importResult

	// sample loading status (WASM): show a transient message while embedded
	// samples are being registered and another once finished.
	samplesTotal  int
	samplesLoaded int
	showLoading   bool
	doneMsgTimer  int // frames to show "Finished loading samples"

	timelineRect  image.Rectangle // progress bar for fast seek
	timelineBeats int             // total beats represented by timeline
	// Units per beat used by the timeline header. When running under Game,
	// this is set to the grid's MaxDiv so offsets/length (which are tracked
	// in subdivision steps) are mapped to beats consistently in the header.
	timelineUnitsPerBeat int

	// internal ui state
	bpm           int
	secPerBeat    float64
	playPressed   bool
	stopPressed   bool
	Length        int  // Length of the drum view, independent of graph
	lenIncPressed bool // State for length increase button
	lenDecPressed bool // State for length decrease button
	follow        bool // auto-scroll with playback
	// smooth wheel zoom accumulator (in beats). Each notch contributes a
	// fraction; when |accum| >= 1, we apply whole-beat length changes.
	zoomAccum float64

	bpmPrev  int // previous BPM before editing
	bpmDelta int // accumulated BPM adjustments from +/- buttons

	// button animations
	playAnim     float64
	stopAnim     float64
	bpmDecAnim   float64
	bpmIncAnim   float64
	lenDecAnim   float64
	lenIncAnim   float64
	uploadAnim   float64
	bpmErrorAnim float64
	saveAnim     float64
	namePhase    float64
	savePressed  bool

	// window scrolling
	Offset        int // index of first visible beat
	dragging      bool
	dragStartX    int
	startOffset   int
	offsetChanged bool

	rowOffset      int
	scrollDrag     bool
	scrollStartY   int
	scrollStartOff int

	scrubbing bool
}

// Capturing reports whether the drum view is actively handling a mouse drag
// (e.g. scrollbar or slider) and should therefore block camera panning.
func (dv *DrumView) Capturing() bool {
	return dv.scrollDrag || dv.activeSlider >= 0
}

// BlocksAt reports whether a point (x,y) lies over a temporary overlay such as
// the instrument dropdown or rename dialog. When true, clicks at that position
// should not reach underlying UI elements.
func (dv *DrumView) BlocksAt(x, y int) bool {
	if dv.instMenuOpen || dv.instHold || dv.renameBox != nil || dv.naming {
		return true
	}
	return false
}

type deletedRow struct {
	index  int
	origin model.NodeID
}

/* ─── geometry helpers ─────────────────────────────────────── */

// rowHeight returns the fixed pixel height for each drum row and for the
// trailing "+" button row. Keeping this constant avoids oversized buttons when
// only a few rows are present, yielding a minimal and consistent layout.
func (dv *DrumView) rowHeight() int { return 24 }

func (dv *DrumView) visibleRows() int {
	return (dv.Bounds.Dy() - timelineHeight) / dv.rowHeight()
}

func (dv *DrumView) scrollBarRect() image.Rectangle {
	w := 6
	return image.Rect(dv.Bounds.Max.X-w, dv.Bounds.Min.Y+timelineHeight, dv.Bounds.Max.X, dv.Bounds.Max.Y)
}

func (dv *DrumView) scrollThumbRect() image.Rectangle {
	total := len(dv.Rows) + 1
	vis := dv.visibleRows()
	bar := dv.scrollBarRect()
	if total <= vis {
		return image.Rect(0, 0, 0, 0)
	}
	h := bar.Dy() * vis / total
	if h < 10 {
		h = 10
	}
	track := bar.Dy() - h
	y := bar.Min.Y
	if total-vis > 0 {
		y += track * dv.rowOffset / (total - vis)
	}
	return image.Rect(bar.Min.X, y, bar.Max.X, y+h)
}

// SetBeatLength sets the beat length in the underlying graph.
func (dv *DrumView) SetBeatLength(length int) {
	if dv.Graph != nil {
		dv.Graph.SetBeatLength(length)
		dv.logger.Debugf("[DRUMVIEW] Graph beat length set to: %d", length)
	}
	if length > dv.timelineBeats {
		dv.timelineBeats = length
	}
}

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
		bgDirty:              true,
		Graph:                g,
		logger:               logger,
		Length:               8, // Default length
		Offset:               0,
		instOptions:          opts,
		uploadCh:             make(chan uploadResult, 1),
		importCh:             make(chan importResult, 1),
		timelineUnitsPerBeat: 1,
		timelineBeats:        8,
		selRow:               0,
		activeSlider:         -1,
		renameRow:            -1,
		follow:               true,
	}
	dv.playBtn = NewButton("▶", PlayButtonStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Play button pressed")
		dv.playPressed = true
		dv.playAnim = 1
	})
	dv.stopBtn = NewButton("■", StopButtonStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Stop button pressed")
		dv.stopPressed = true
		dv.stopAnim = 1
	})
	dv.bpmDecBtn = NewButton("-", BPMDecStyle, func() {
		dv.logger.Infof("[DRUMVIEW] BPM - button pressed")
		dv.bpmDelta--
		dv.bpmDecAnim = 1
	})
	dv.bpmDecBtn.Repeat = true
	dv.bpmBox = NewTextInput(image.Rect(0, 0, 0, 0), BPMBoxStyle)
	dv.bpmBox.SetText("120")
	dv.bpmIncBtn = NewButton("+", BPMIncStyle, func() {
		dv.logger.Infof("[DRUMVIEW] BPM + button pressed")
		dv.bpmDelta++
		dv.bpmIncAnim = 1
	})
	dv.bpmIncBtn.Repeat = true
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
		dv.follow = !dv.follow
		if dv.follow {
			dv.trackBtn.Text = "Track"
			dv.logger.Infof("[DRUMVIEW] Track/Free toggled: follow=Track")
		} else {
			dv.trackBtn.Text = "Free"
			dv.logger.Infof("[DRUMVIEW] Track/Free toggled: follow=Free")
		}
	})
	dv.uploadBtn = NewButton("Upload", UploadBtnStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Upload button pressed")
		dv.logger.Debugf("[DRUMVIEW] Upload button clicked. uploading=%v naming=%v menuOpen=%v", dv.uploading, dv.naming, dv.instMenuOpen)
		dv.instMenuOpen = false
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
		dv.importing = true
		selectJSONAsync(func(data []byte, err error) {
			jsLog("Import callback invoked; bytes=%d err=%v", len(data), err)
			dv.importCh <- importResult{data: data, err: err}
		})
	})
	dv.exportBtn = NewButton("Export", UploadBtnStyle, func() {
		dv.logger.Infof("[DRUMVIEW] Export button pressed")
		dv.logger.Debugf("[DRUMVIEW] Export button clicked")
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

	dv.Rows = []*DrumRow{{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), Color: instColor(inst), Origin: model.InvalidNodeID, Volume: 1}}
	dv.SetBeatLength(dv.Length) // Initialize graph's beat length
	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}
	return dv
}

// SetBounds is called from Game whenever the splitter moves or the window
// resizes; it invalidates the cached background so dimensions update next draw.
func (dv *DrumView) SetBounds(b image.Rectangle) {
	if dv.Bounds != b {
		dv.Bounds = b
		dv.bgDirty = true
		dv.recalcButtons()
		if dv.bgDirty {
			dv.calcLayout()
			dv.bgDirty = false
		}
	}
}

// AddRow appends a new drum row with default settings.
func (dv *DrumView) AddRow() {
	inst := "snare"
	name := "Snare"
	if len(dv.instOptions) > 0 {
		inst = dv.instOptions[0]
		name = strings.ToUpper(inst[:1]) + inst[1:]
	}
	idx := len(dv.Rows)
	dv.Rows = append(dv.Rows, &DrumRow{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), Color: instColor(inst), Origin: model.InvalidNodeID, Node: nil, Volume: 1})
	dv.logger.Infof("[DRUMVIEW] Row added index=%d instrument=%s name=%s", idx, inst, name)
	dv.added = append(dv.added, idx)
	dv.bgDirty = true
	dv.activeSlider = -1
	dv.calcLayout()
	maxOff := len(dv.Rows) + 1 - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset > maxOff {
		dv.rowOffset = maxOff
	}
}

// DeleteRow removes the drum row at the given index.
func (dv *DrumView) DeleteRow(i int) {
	if i < 0 || i >= len(dv.Rows) || len(dv.Rows) <= 1 {
		return
	}
	dv.logger.Infof("[DRUMVIEW] Row deleted index=%d name=%s instrument=%s", i, dv.Rows[i].Name, dv.Rows[i].Instrument)
	origin := dv.Rows[i].Origin
	dv.Rows = append(dv.Rows[:i], dv.Rows[i+1:]...)
	dv.deleted = append(dv.deleted, deletedRow{index: i, origin: origin})
	dv.bgDirty = true
	dv.activeSlider = -1
	if dv.selRow >= len(dv.Rows) {
		dv.selRow = len(dv.Rows) - 1
	}
	dv.calcLayout()
	maxOff := len(dv.Rows) + 1 - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset > maxOff {
		dv.rowOffset = maxOff
	}
}

func (dv *DrumView) toggleMute(idx int) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	r := dv.Rows[idx]
	r.Muted = !r.Muted
	if r.Muted {
		r.Solo = false
	}
}

func (dv *DrumView) toggleSolo(idx int) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	r := dv.Rows[idx]
	r.Solo = !r.Solo
	if r.Solo {
		r.Muted = false
		for i, o := range dv.Rows {
			if i == idx {
				continue
			}
			o.Solo = false
			o.Muted = true
		}
	} else {
		anySolo := false
		for _, o := range dv.Rows {
			if o.Solo {
				anySolo = true
				break
			}
		}
		if !anySolo {
			for _, o := range dv.Rows {
				o.Muted = false
			}
		}
	}
}

// ConsumeDeletedRows returns and clears the recently deleted rows info.
func (dv *DrumView) ConsumeDeletedRows() []deletedRow {
	rows := dv.deleted
	dv.deleted = nil
	return rows
}

// ConsumeAddedRows returns and clears indexes of newly added rows.
func (dv *DrumView) ConsumeAddedRows() []int {
	rows := dv.added
	dv.added = nil
	return rows
}

// ConsumeOriginRequests returns and clears indexes of rows requesting origin reassignment.
func (dv *DrumView) ConsumeOriginRequests() []int {
	rows := dv.originReq
	dv.originReq = nil
	return rows
}

/* ─── public update ────────────────────────────────────────── */

func (dv *DrumView) recalcButtons() {
	dv.controlsW = dv.Bounds.Dx() / 4
	if dv.controlsW < 140 {
		dv.controlsW = 140
	}
	if dv.controlsW > 320 {
		dv.controlsW = 320
	}

	topBounds := image.Rect(dv.Bounds.Min.X+dv.labelW, dv.Bounds.Min.Y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+dv.rowHeight())
	topGrid := NewGridLayout(topBounds, []float64{1, 1, 1, 2, 1, 1, 1, 1}, []float64{1})
	dv.playBtn.SetRect(insetRect(topGrid.Cell(0, 0), buttonPad))
	dv.stopBtn.SetRect(insetRect(topGrid.Cell(1, 0), buttonPad))
	dv.bpmDecBtn.SetRect(insetRect(topGrid.Cell(2, 0), buttonPad))
	dv.bpmBox.Rect = insetRect(topGrid.Cell(3, 0), buttonPad)
	dv.bpmIncBtn.SetRect(insetRect(topGrid.Cell(4, 0), buttonPad))
	dv.lenDecBtn.SetRect(insetRect(topGrid.Cell(5, 0), buttonPad))
	dv.lenIncBtn.SetRect(insetRect(topGrid.Cell(6, 0), buttonPad))
	dv.trackBtn.SetRect(insetRect(topGrid.Cell(7, 0), buttonPad))

	botBounds := image.Rect(dv.Bounds.Min.X+dv.labelW, dv.Bounds.Min.Y+dv.rowHeight(), dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+2*dv.rowHeight())
	// Split bottom row into three equal columns: Upload, Import, Export.
	botGrid := NewGridLayout(botBounds, []float64{1, 1, 1}, []float64{1})
	dv.uploadBtn.SetRect(insetRect(botGrid.Cell(0, 0), buttonPad))
	dv.importBtn.SetRect(insetRect(botGrid.Cell(1, 0), buttonPad))
	dv.exportBtn.SetRect(insetRect(botGrid.Cell(2, 0), buttonPad))

	top := dv.Bounds.Min.Y + timelineHeight - timelineBarHeight - 5
	dv.timelineRect = image.Rect(
		dv.Bounds.Min.X+dv.labelW+dv.controlsW,
		top,
		dv.Bounds.Max.X-10,
		top+timelineBarHeight,
	)
}

// clampLength enforces global min/max zoom limits for the drum view.
// Minimum: one full beat (timelineUnitsPerBeat). Maximum: pixels available
// in the timeline so each subdivision remains at least ~1px wide.
func (dv *DrumView) clampLength(n int) int {
	inc := max1(dv.timelineUnitsPerBeat)
	minLen := inc
	maxLen := dv.timelineRect.Dx()
	if maxLen < minLen {
		maxLen = minLen
	}
	if n < minLen {
		n = minLen
	}
	if n > maxLen {
		n = maxLen
	}
	return n
}

func (dv *DrumView) calcLayout() {
	if len(dv.Rows) > 0 {
		dv.cell = (dv.Bounds.Dx() - dv.labelW - dv.controlsW) / len(dv.Rows[0].Steps)
	}
	dv.rowLabels = dv.rowLabels[:0]
	dv.rowEditBtns = dv.rowEditBtns[:0]
	dv.rowDeleteBtns = dv.rowDeleteBtns[:0]
	dv.rowVolSliders = dv.rowVolSliders[:0]
	dv.rowOriginBtns = dv.rowOriginBtns[:0]
	dv.rowMuteBtns = dv.rowMuteBtns[:0]
	dv.rowSoloBtns = dv.rowSoloBtns[:0]
	vis := dv.visibleRows()
	for i := range dv.Rows {
		y := dv.Bounds.Min.Y + timelineHeight + (i-dv.rowOffset)*dv.rowHeight()
		rowRect := image.Rect(dv.Bounds.Min.X, y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, y+dv.rowHeight())
		if i < dv.rowOffset || i >= dv.rowOffset+vis {
			rowRect = image.Rect(0, 0, 0, 0)
		}
		g := NewGridLayout(rowRect, []float64{6, 2, 5, 2, 2, 2, 2}, []float64{1})
		lbl := NewButton(dv.Rows[i].Name, InstButtonStyle, nil)
		lbl.SetRect(insetRect(g.Cell(0, 0), buttonPad))
		idx := i
		lbl.OnClick = func() {
			dv.selRow = idx
			if dv.instMenuOpen && dv.instMenuRow == idx {
				dv.logger.Debugf("[DRUMVIEW] Closing instrument menu for row %d", idx)
				dv.instMenuOpen = false
			} else {
				dv.instMenuRow = idx
				dv.instMenuOpen = true
				dv.buildInstMenu()
				dv.logger.Debugf("[DRUMVIEW] Opening instrument menu for row %d", idx)
			}
		}
		edit := NewButton("✎", InstButtonStyle, nil)
		edit.SetRect(insetRect(g.Cell(1, 0), buttonPad))
		editIdx := i
		edit.OnClick = func() {
			dv.renameRow = editIdx
			r := dv.rowLabels[editIdx].Rect()
			dv.renameBox = NewTextInput(r, BPMBoxStyle)
			dv.renameBox.SetText(dv.Rows[editIdx].Name)
			dv.renameBox.focused = true
			dv.renameBox.anim = 1
			dv.renameHold = true
			dv.instMenuOpen = false
		}
		slider := NewSlider(dv.Rows[i].Volume)
		slider.SetRect(insetRect(g.Cell(2, 0), buttonPad))
		mute := NewButton("M", InstButtonStyle, nil)
		mute.SetRect(insetRect(g.Cell(3, 0), buttonPad))
		solo := NewButton("S", InstButtonStyle, nil)
		solo.SetRect(insetRect(g.Cell(4, 0), buttonPad))
		origin := NewButton("O", InstButtonStyle, nil)
		origin.SetRect(insetRect(g.Cell(5, 0), buttonPad))
		del := NewButton("X", InstButtonStyle, nil)
		del.SetRect(insetRect(g.Cell(6, 0), buttonPad))
		delIdx := i
		if len(dv.Rows) > 1 {
			del.OnClick = func() { dv.DeleteRow(delIdx) }
		} else {
			del.Style = DisabledButtonStyle
		}
		originIdx := i
		origin.OnClick = func() { dv.originReq = append(dv.originReq, originIdx) }
		muteIdx := i
		mute.OnClick = func() { dv.toggleMute(muteIdx) }
		soloIdx := i
		solo.OnClick = func() { dv.toggleSolo(soloIdx) }
		dv.rowLabels = append(dv.rowLabels, lbl)
		dv.rowEditBtns = append(dv.rowEditBtns, edit)
		dv.rowVolSliders = append(dv.rowVolSliders, slider)
		dv.rowMuteBtns = append(dv.rowMuteBtns, mute)
		dv.rowSoloBtns = append(dv.rowSoloBtns, solo)
		dv.rowOriginBtns = append(dv.rowOriginBtns, origin)
		dv.rowDeleteBtns = append(dv.rowDeleteBtns, del)
	}
	y := dv.Bounds.Min.Y + timelineHeight + (len(dv.Rows)-dv.rowOffset)*dv.rowHeight()
	dv.addRowBtn.SetRect(insetRect(image.Rect(dv.Bounds.Min.X, y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, y+dv.rowHeight()), buttonPad))
}

// buildInstMenu rebuilds the instrument dropdown buttons for the selected row.
func (dv *DrumView) buildInstMenu() {
	dv.instMenuBtns = dv.instMenuBtns[:0]
	if dv.instMenuRow < 0 || dv.instMenuRow >= len(dv.rowLabels) {
		return
	}
	base := dv.rowLabels[dv.instMenuRow].Rect()
	menuH := dv.rowHeight() * len(dv.instOptions)
	openUp := base.Max.Y+menuH > dv.Bounds.Max.Y
	for i, id := range dv.instOptions {
		var r image.Rectangle
		if openUp {
			r = image.Rect(base.Min.X, base.Min.Y-(i+1)*dv.rowHeight(), base.Max.X, base.Min.Y-i*dv.rowHeight())
		} else {
			r = image.Rect(base.Min.X, base.Max.Y+i*dv.rowHeight(), base.Max.X, base.Max.Y+(i+1)*dv.rowHeight())
		}
		optID := id
		btn := NewButton(strings.ToUpper(id[:1])+id[1:], DropdownStyle, func() {
			dv.SetInstrument(optID)
		})
		btn.SetRect(insetRect(r, buttonPad))
		dv.instMenuBtns = append(dv.instMenuBtns, btn)
	}
}

func (dv *DrumView) refreshInstruments() {
	opts := audio.Instruments()
	if !slices.Equal(opts, dv.instOptions) {
		dv.instOptions = opts
		if dv.instMenuOpen {
			dv.buildInstMenu()
		}
	}
}

func (dv *DrumView) PlayPressed() bool {
	if dv.playPressed {
		dv.playPressed = false
		return true
	}
	return false
}

// SetPlaying updates the play button label to reflect playback state.
func (dv *DrumView) SetPlaying(p bool) {
	if p {
		dv.playBtn.Text = "⏸"
	} else {
		dv.playBtn.Text = "▶"
	}
}

func (dv *DrumView) StopPressed() bool {
	if dv.stopPressed {
		dv.stopPressed = false
		return true
	}
	return false
}

func (dv *DrumView) BPM() int {
	return dv.bpm
}

func (dv *DrumView) SetBPM(b int) {
	if b < 1 {
		dv.bpm = 1
		dv.bpmErrorAnim = 1
		return
	}
	if b > maxBPM {
		dv.bpm = maxBPM
		dv.bpmErrorAnim = 1
		return
	}
	prev := dv.bpm
	dv.logger.Infof("[DRUMVIEW] BPM set: %d -> %d", prev, b)
	dv.bpm = b
	dv.secPerBeat = 60.0 / float64(dv.bpm)
	if dv.bpmBox != nil && !dv.bpmBox.Focused() {
		dv.bpmBox.SetText(strconv.Itoa(dv.bpm))
	}
}

func (dv *DrumView) OffsetChanged() bool {
	if dv.offsetChanged {
		dv.offsetChanged = false
		return true
	}
	return false
}

// FollowPlayback reports whether the drum view auto-scrolls with playback.
func (dv *DrumView) FollowPlayback() bool { return dv.follow }

// TrackBeat adjusts the drum view offset to keep the given beat visible when
// auto-tracking is enabled.
func (dv *DrumView) TrackBeat(cur int) {
	if !dv.follow {
		return
	}
	half := dv.Length / 2
	desired := cur - half
	if desired < 0 {
		desired = 0
	}
	if dv.Offset != desired {
		dv.Offset = desired
		dv.offsetChanged = true
		dv.logger.Debugf("[DRUMVIEW] TrackBeat cur=%d offset->%d", cur, dv.Offset)
	}
}

func (dv *DrumView) SetLength(length int) {
	if length < 1 {
		length = 1
	}
	if length != dv.Length {
		dv.logger.Infof("[DRUMVIEW] Length set: %d -> %d", dv.Length, length)
	}
	dv.Length = length
	for _, r := range dv.Rows {
		r.Steps = make([]bool, dv.Length)
	}
	dv.SetBeatLength(dv.Length)
	dv.bgDirty = true
}

func (dv *DrumView) SetInstrument(id string) {
	if len(dv.Rows) == 0 {
		return
	}
	dv.logger.Infof("[DRUMVIEW] Instrument set row=%d id=%s", dv.selRow, id)
	dv.Rows[dv.selRow].Instrument = id
	if id != "" {
		dv.Rows[dv.selRow].Name = strings.ToUpper(id[:1]) + id[1:]
	}
	dv.Rows[dv.selRow].Color = instColor(id)
	if dv.selRow < len(dv.rowLabels) {
		dv.rowLabels[dv.selRow].Text = dv.Rows[dv.selRow].Name
	}
}

func (dv *DrumView) AddInstrument(id string) {
	dv.instOptions = audio.Instruments()
	dv.SetInstrument(id)
}

func (dv *DrumView) CycleInstrument() {
	if len(dv.instOptions) == 0 || len(dv.Rows) == 0 {
		return
	}
	cur := dv.Rows[dv.selRow].Instrument
	for i, id := range dv.instOptions {
		if id == cur {
			next := dv.instOptions[(i+1)%len(dv.instOptions)]
			dv.SetInstrument(next)
			return
		}
	}
}

func (dv *DrumView) registerInstrument(id string) {
	if err := audio.RegisterWAV(id, dv.pendingWAV); err == nil {
		dv.instOptions = audio.Instruments()
		dv.SetInstrument(id)
		if dv.instMenuOpen {
			dv.buildInstMenu()
		}
		dv.logger.Infof("[DRUMVIEW] Loaded user WAV %s", id)
	} else {
		dv.logger.Infof("[DRUMVIEW] Failed to load WAV: %v", err)
	}
	dv.naming = false
	dv.pendingWAV = ""
	dv.nameInput = ""
	dv.savePressed = false
}

func (dv *DrumView) decayAnims() {
	decay := func(v *float64) {
		*v *= 0.85
		if *v < 0.01 {
			*v = 0
		}
	}
	decay(&dv.playAnim)
	decay(&dv.stopAnim)
	decay(&dv.bpmDecAnim)
	decay(&dv.bpmIncAnim)
	decay(&dv.lenDecAnim)
	decay(&dv.lenIncAnim)
	decay(&dv.uploadAnim)
	decay(&dv.bpmErrorAnim)
	decay(&dv.saveAnim)
}

func (dv *DrumView) Update() {
	if len(dv.Rows) == 0 {
		return
	}

	dv.refreshInstruments()

	// Handle pending JSON import
	if dv.importing {
		select {
		case res := <-dv.importCh:
			dv.importing = false
			if res.err != nil {
				dv.logger.Infof("[DRUMVIEW] Import failed: %v", res.err)
			} else if dv.onImport != nil {
				if err := dv.onImport(res.data); err != nil {
					dv.logger.Infof("[DRUMVIEW] Import error: %v", err)
				}
			}
		default:
		}
	}

	// Update sample loading status (WASM returns non-zero).
	if loaded, total := audio.SampleLoadProgress(); total > 0 {
		// When total becomes available, consider ourselves loading until done.
		dv.samplesTotal = total
		dv.samplesLoaded = loaded
		if loaded < total {
			dv.showLoading = true
			dv.doneMsgTimer = 0
		} else if dv.showLoading && loaded >= total {
			// Just finished.
			dv.showLoading = false
			dv.doneMsgTimer = 180 // ~3 seconds at 60fps
		}
	}

	if dv.uploading {
		select {
		case res := <-dv.uploadCh:
			dv.uploading = false
			dv.logger.Debugf("[DRUMVIEW] Upload result path=%s err=%v", res.path, res.err)
			if res.err != nil {
				dv.logger.Infof("[DRUMVIEW] Failed to load WAV: %v", res.err)
			} else {
				dv.pendingWAV = res.path
				dv.naming = true
				dv.nameInput = ""
			}
		default:
		}
	}

	if dv.naming {
		box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
		dv.saveBtn.SetRect(image.Rect(box.Max.X+10, box.Min.Y, box.Max.X+50, box.Max.Y))
		dv.saveBtn.OnClick = func() {
			id := strings.TrimSpace(dv.nameInput)
			dv.logger.Infof("[DRUMVIEW] Save instrument pressed id=%q", id)
			if id != "" {
				dv.registerInstrument(id)
			}
		}
		for _, r := range inputChars() {
			if r >= asciiPrintableMin && r <= asciiPrintableMax {
				dv.nameInput += string(r)
			}
		}
		if isKeyPressed(ebiten.KeyBackspace) && len(dv.nameInput) > 0 {
			dv.nameInput = dv.nameInput[:len(dv.nameInput)-1]
		}
		if isKeyPressed(ebiten.KeyEnter) {
			id := strings.TrimSpace(dv.nameInput)
			if id != "" {
				dv.registerInstrument(id)
			}
		}
		if isKeyPressed(ebiten.KeyEscape) {
			dv.naming = false
			dv.pendingWAV = ""
			dv.nameInput = ""
		}
		mx, my := cursorPosition()
		left := isMouseButtonPressed(ebiten.MouseButtonLeft)
		if dv.saveBtn.Handle(mx, my, left) {
			dv.saveAnim = 1
		}
		dv.namePhase += 0.1
		return
	}

	if dv.renameBox != nil {
		if dv.renameHold {
			if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
				dv.renameHold = false
			}
		} else {
			dv.renameBox.Update()
		}
		if !dv.renameHold && isKeyPressed(ebiten.KeyEnter) {
			name := strings.TrimSpace(dv.renameBox.Value())
			if name != "" && dv.renameRow >= 0 && dv.renameRow < len(dv.Rows) {
				oldID := dv.Rows[dv.renameRow].Instrument
				newID := strings.ToLower(name)
				dv.logger.Infof("[DRUMVIEW] Rename instrument row=%d %q -> %q", dv.renameRow, oldID, newID)
				audio.RenameInstrument(oldID, newID)
				dv.Rows[dv.renameRow].Instrument = newID
				dv.Rows[dv.renameRow].Name = name
				dv.rowLabels[dv.renameRow].Text = name
				customColors[newID] = dv.Rows[dv.renameRow].Color
				dv.refreshInstruments()
			}
			dv.renameBox = nil
			dv.renameRow = -1
		}
		if !dv.renameHold && isKeyPressed(ebiten.KeyEscape) {
			dv.logger.Infof("[DRUMVIEW] Rename instrument canceled (row=%d)", dv.renameRow)
			dv.renameBox = nil
			dv.renameRow = -1
		}
		return
	}

	if dv.instHold {
		if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
			dv.instHold = false
		}
		return
	}

	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}

	prevFocus := dv.bpmBox.Focused()

	// Early BPM text input handling so focus/blur on the BPM box is
	// registered even if other controls short-circuit later in Update.
	prevVal := dv.bpmBox.Value()
	dv.bpmBox.Update()
	if !prevFocus && dv.bpmBox.Focused() {
		dv.logger.Debugf("[DRUMVIEW] BPM box focused")
	}
	if !prevFocus && dv.bpmBox.Focused() {
		dv.bpmPrev = dv.bpm
		dv.bpmBox.SetText("")
	}
	if prevFocus && !dv.bpmBox.Focused() {
		dv.logger.Debugf("[DRUMVIEW] BPM box blurred value=%q", prevVal)
	}

	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	totalRows := len(dv.Rows) + 1
	visRows := dv.visibleRows()
	if totalRows > visRows {
		if _, whY := wheel(); whY != 0 {
			dv.rowOffset -= int(whY)
			if dv.rowOffset < 0 {
				dv.rowOffset = 0
			}
			if dv.rowOffset > totalRows-visRows {
				dv.rowOffset = totalRows - visRows
			}
			dv.calcLayout()
		}
		bar := dv.scrollBarRect()
		thumb := dv.scrollThumbRect()
		if dv.scrollDrag {
			if left {
				track := bar.Dy() - thumb.Dy()
				if track > 0 {
					delta := my - dv.scrollStartY
					dv.rowOffset = dv.scrollStartOff + delta*totalRows/track
					if dv.rowOffset < 0 {
						dv.rowOffset = 0
					}
					if dv.rowOffset > totalRows-visRows {
						dv.rowOffset = totalRows - visRows
					}
					dv.calcLayout()
				}
			} else {
				dv.scrollDrag = false
			}
		} else if left && image.Pt(mx, my).In(thumb) {
			dv.scrollDrag = true
			dv.scrollStartY = my
			dv.scrollStartOff = dv.rowOffset
		}
	}

	if dv.instMenuOpen {
		for _, btn := range dv.instMenuBtns {
			if btn.Handle(mx, my, left) {
				dv.instMenuOpen = false
				dv.instHold = true
				return
			}
		}
		if left {
			lbl := dv.rowLabels[dv.instMenuRow].Rect()
			menu := image.Rect(lbl.Min.X, lbl.Max.Y, lbl.Max.X, lbl.Max.Y+len(dv.instMenuBtns)*dv.rowHeight())
			if !pt(mx, my, lbl) && !pt(mx, my, menu) {
				dv.instMenuOpen = false
				dv.instHold = true
			}
			return
		}
		return
	}

	stepsRect := image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+timelineHeight, dv.Bounds.Max.X, dv.Bounds.Max.Y)

	// wheel zoom for length adjustment. Apply smoothing so each wheel notch
	// accumulates a fraction of a beat and only commits whole-beat changes
	// when enough deltas have been collected. This avoids abrupt jumps.
	if _, whY := wheel(); whY != 0 {
		if pt(mx, my, stepsRect) {
			// Each notch contributes 0.25 beat; adjust if needed.
			sensitivity := 0.25
			dv.zoomAccum += whY * sensitivity
			incBeats := 0
			for dv.zoomAccum >= 1.0 {
				incBeats++
				dv.zoomAccum -= 1.0
			}
			for dv.zoomAccum <= -1.0 {
				incBeats--
				dv.zoomAccum += 1.0
			}
			if incBeats != 0 {
				inc := max1(dv.timelineUnitsPerBeat)
				delta := inc * incBeats
				newLen := dv.clampLength(dv.Length + delta)
				if newLen != dv.Length {
					dv.Length = newLen
					for _, r := range dv.Rows {
						r.Steps = make([]bool, dv.Length)
					}
					dv.SetBeatLength(dv.Length)
					dv.bgDirty = true
					if incBeats > 0 {
						dv.logger.Infof("[DRUMVIEW] Length increased to: %d via wheel", dv.Length)
					} else {
						dv.logger.Infof("[DRUMVIEW] Length decreased to: %d via wheel", dv.Length)
					}
				}
			}
		}
	}

	/* ——— widget clicks & dragging ——— */
	if dv.activeSlider >= 0 {
		s := dv.rowVolSliders[dv.activeSlider]
		if s.Handle(mx, my, left) {
			dv.Rows[dv.activeSlider].Volume = s.Value
			dv.logger.Infof("[DRUMVIEW] Row %d volume changed via slider: %.3f", dv.activeSlider, s.Value)
		}
		if !left {
			dv.activeSlider = -1
		}
		return
	}
	for i, s := range dv.rowVolSliders {
		if s.Handle(mx, my, left) {
			dv.Rows[i].Volume = s.Value
			dv.logger.Infof("[DRUMVIEW] Row %d volume changed via slider: %.3f", i, s.Value)
			dv.activeSlider = i
			if !left {
				dv.activeSlider = -1
			}
			return
		}
	}

	handled := false
	if !dv.dragging {
		for _, btn := range dv.rowOriginBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowDeleteBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowMuteBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowSoloBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, btn := range dv.rowEditBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		for _, lbl := range dv.rowLabels {
			if lbl.Handle(mx, my, left) {
				handled = true
			}
		}
		if handled && left {
			return
		}
		buttons := []*Button{dv.playBtn, dv.stopBtn, dv.bpmDecBtn, dv.bpmIncBtn, dv.lenDecBtn, dv.lenIncBtn, dv.trackBtn, dv.addRowBtn, dv.uploadBtn, dv.importBtn, dv.exportBtn}
		for _, btn := range buttons {
			if handled {
				break
			}
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
	}

	// Handle BPM text box early so clicks on it are processed even when
	// other controls are being interacted with in the same frame. This keeps
	// the manual BPM editor responsive.
	{
		// Reuse the prevFocus computed at the beginning of Update.
		dv.bpmBox.Update()
		if !prevFocus && dv.bpmBox.Focused() {
			dv.bpmPrev = dv.bpm
			dv.bpmBox.SetText("")
		}
		if dv.bpmBox.Focused() {
			if txt := dv.bpmBox.Value(); txt != "" {
				if _, ok := parseBPM(txt); !ok {
					dv.bpmErrorAnim = 1
				}
			}
		} else if prevFocus {
			txt := dv.bpmBox.Value()
			if txt == "" {
				dv.SetBPM(dv.bpmPrev)
			} else if v, ok := parseBPM(txt); ok {
				dv.SetBPM(v)
			} else {
				dv.bpmErrorAnim = 1
				dv.SetBPM(dv.bpmPrev)
			}
			dv.bpmBox.SetText(strconv.Itoa(dv.bpm))
		}

		if dv.bpmDelta != 0 {
			dv.SetBPM(dv.bpm + dv.bpmDelta)
			dv.bpmDelta = 0
		}
	}

	if left {
		if !dv.dragging {
			if pt(mx, my, stepsRect) {
				dv.dragging = true
				dv.dragStartX = mx
				dv.startOffset = dv.Offset
			}
		}
	} else {
		dv.dragging = false
	}

	if dv.dragging {
		delta := (dv.dragStartX - mx) / dv.cell
		newOffset := dv.startOffset + delta
		if newOffset < 0 {
			newOffset = 0
		}
		if newOffset != dv.Offset {
			dv.Offset = newOffset
			dv.offsetChanged = true
			dv.logger.Debugf("[DRUMVIEW] Dragging: offset=%d", dv.Offset)
		}
	}

	// timeline scrubbing (map click proportionally to [0..maxOffset])
	if left && pt(mx, my, dv.timelineRect) {
		dv.scrubbing = true
	}
	if dv.scrubbing {
		pos := mx
		if pos < dv.timelineRect.Min.X {
			pos = dv.timelineRect.Min.X
		}
		if pos > dv.timelineRect.Max.X {
			pos = dv.timelineRect.Max.X
		}
		frac := float64(pos-dv.timelineRect.Min.X) / float64(dv.timelineRect.Dx())
		unitsPerBeat := max1(dv.timelineUnitsPerBeat)
		// Map desired offset in beats, then convert to subdivision steps.
		lengthBeats := float64(dv.Length) / float64(unitsPerBeat)
		maxOffBeats := float64(dv.timelineBeats) - lengthBeats
		if maxOffBeats < 0 {
			maxOffBeats = 0
		}
		desiredBeats := frac * maxOffBeats
		desiredSteps := int(math.Round(desiredBeats * float64(unitsPerBeat)))
		maxOffSteps := int(math.Round(maxOffBeats * float64(unitsPerBeat)))
		if desiredSteps < 0 {
			desiredSteps = 0
		}
		if desiredSteps > maxOffSteps {
			desiredSteps = maxOffSteps
		}
		if desiredSteps != dv.Offset {
			dv.Offset = desiredSteps
			dv.offsetChanged = true
			dv.logger.Debugf("[DRUMVIEW] Timeline scrub: offset=%d len=%d total=%d", dv.Offset, dv.Length, dv.timelineBeats)
		}
		if !left {
			dv.scrubbing = false
		}
	}

	// (moved BPM text input handling earlier)

	/* ——— Length editing ——— */
	if dv.lenIncPressed {
		inc := max1(dv.timelineUnitsPerBeat)
		newLen := dv.clampLength(dv.Length + inc)
		if newLen != dv.Length {
			dv.Length = newLen
			dv.logger.Infof("[DRUMVIEW] Length increased to: %d", dv.Length)
			for _, r := range dv.Rows {
				r.Steps = make([]bool, dv.Length)
			}
			dv.SetBeatLength(dv.Length) // Update graph's beat length
			dv.bgDirty = true
		}
		dv.lenIncPressed = false
	}
	if dv.lenDecPressed {
		inc := max1(dv.timelineUnitsPerBeat)
		newLen := dv.clampLength(dv.Length - inc)
		if newLen != dv.Length {
			dv.Length = newLen
			dv.logger.Infof("[DRUMVIEW] Length decreased to: %d", dv.Length)
			for _, r := range dv.Rows {
				r.Steps = make([]bool, dv.Length)
			}
			dv.SetBeatLength(dv.Length) // Update graph's beat length
			dv.bgDirty = true
		}
		dv.lenDecPressed = false
	}
	if handled && left {
		return
	}
}

func (dv *DrumView) Draw(dst *ebiten.Image, highlightedBeats map[int]int64, frame int64, beatInfos []model.BeatInfo, elapsedBeats float64) {
	dv.logger.Debugf("[DRUMVIEW] Draw called. beatInfos: %v, highlightedBeats: %v", beatInfos, highlightedBeats)
	// draw background
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
	dst.DrawImage(dv.bg(dv.Bounds.Dx(), dv.Bounds.Dy()), op)

	dv.decayAnims()
	// Update only the rectangles/positions for existing per-row controls.
	// Avoid recreating buttons and sliders every frame to keep rendering
	// lightweight even with many rows or long timelines.
	dv.updateRowRects()

	dv.playBtn.Draw(dst)
	dv.stopBtn.Draw(dst)
	dv.bpmDecBtn.Draw(dst)
	dv.bpmBox.Draw(dst)
	if dv.bpmErrorAnim > 0 {
		drawRect(dst, dv.bpmBox.Rect, fadeColor(colError, dv.bpmErrorAnim), false)
	}
	dv.bpmIncBtn.Draw(dst)
	dv.lenDecBtn.Draw(dst)
	dv.lenIncBtn.Draw(dst)
	dv.trackBtn.Draw(dst)
	dv.uploadBtn.Draw(dst)
	dv.importBtn.Draw(dst)
	dv.exportBtn.Draw(dst)

	// Display sample loading messages near the upload button area.
	if dv.showLoading && dv.samplesTotal > 0 {
		msg := fmt.Sprintf("loading samples... (%d/%d)", dv.samplesLoaded, dv.samplesTotal)
		ebitenutil.DebugPrintAt(dst, msg, dv.uploadBtn.Rect().Min.X, dv.uploadBtn.Rect().Max.Y+20)
	} else if dv.doneMsgTimer > 0 {
		dv.doneMsgTimer--
		msg := "Finished loading samples"
		ebitenutil.DebugPrintAt(dst, msg, dv.uploadBtn.Rect().Min.X, dv.uploadBtn.Rect().Max.Y+20)
	}
	// timeline and progress
	// Keep the visible timeline long enough to contain the graph traversal,
	// the current playhead and the visible window. All values expressed in beats.
	if dv.Graph != nil {
		units := dv.Graph.BeatLength() // may be in subdivision steps under Game
		u := dv.timelineUnitsPerBeat
		if u <= 0 {
			u = 1
		}
		// Round up to cover any partial beat at the end of the step sequence.
		beats := int(math.Ceil(float64(units) / float64(u)))
		if dv.timelineBeats < beats {
			dv.timelineBeats = beats
		}
	}
	lengthBeats := float64(dv.Length) / float64(max1(dv.timelineUnitsPerBeat))
	if int(math.Ceil(elapsedBeats)+math.Ceil(lengthBeats)) > dv.timelineBeats {
		dv.timelineBeats = int(math.Ceil(elapsedBeats) + math.Ceil(lengthBeats))
	}
	offsetBeats := float64(dv.Offset) / float64(max1(dv.timelineUnitsPerBeat))
	if int(math.Ceil(offsetBeats+lengthBeats)) > dv.timelineBeats {
		dv.timelineBeats = int(math.Ceil(offsetBeats + lengthBeats))
	}
	totalBeats := dv.timelineBeats
	info := dv.timelineInfo(elapsedBeats)
	ebitenutil.DebugPrintAt(dst, info, dv.timelineRect.Min.X, dv.Bounds.Min.Y+5)
	drawRect(dst, dv.timelineRect, colTimelineTotal, true)

	// current view rectangle
	viewStart := dv.timelineRect.Min.X + int((offsetBeats/float64(totalBeats))*float64(dv.timelineRect.Dx()))
	viewWidth := int((lengthBeats / float64(totalBeats)) * float64(dv.timelineRect.Dx()))
	if viewWidth < 1 {
		viewWidth = 1
	}
	if viewWidth < 1 {
		viewWidth = 1
	}
	viewRect := image.Rect(viewStart, dv.timelineRect.Min.Y, viewStart+viewWidth, dv.timelineRect.Max.Y)
	drawRect(dst, viewRect, colTimelineView, true)
	drawRect(dst, viewRect, colTimelineViewHi, false)

	// beat markers, decimated to at most one per pixel column
	step := 1
	width := dv.timelineRect.Dx()
	if totalBeats > width {
		step = int(math.Ceil(float64(totalBeats) / float64(width)))
	}
	prevX := -1
	for i := 0; i <= totalBeats; i += step {
		x := dv.timelineRect.Min.X + int(float64(i)/float64(totalBeats)*float64(dv.timelineRect.Dx()))
		if x != prevX {
			drawRect(dst, image.Rect(x, dv.timelineRect.Min.Y, x+1, dv.timelineRect.Max.Y), colTimelineBeat, true)
			prevX = x
		}
	}

	// current playback cursor (beats)
	cursorX := dv.timelineRect.Min.X + int((elapsedBeats/float64(totalBeats))*float64(dv.timelineRect.Dx()))
	cursorRect := image.Rect(cursorX-1, dv.timelineRect.Min.Y, cursorX+1, dv.timelineRect.Max.Y)
	drawRect(dst, cursorRect, colTimelineCursor, true)

	drawRect(dst, dv.timelineRect, colButtonBorder, false)

	// draw steps
	vis := dv.visibleRows()
	for i, r := range dv.Rows {
		if len(r.Steps) != dv.Length {
			r.Steps = make([]bool, dv.Length)
		}
		if i < dv.rowOffset || i >= dv.rowOffset+vis {
			continue
		}
		y := dv.Bounds.Min.Y + timelineHeight + (i-dv.rowOffset)*dv.rowHeight()
		if dv.renameBox == nil || dv.renameRow != i {
			dv.rowLabels[i].Draw(dst)
		}
		dv.rowEditBtns[i].Draw(dst)
		dv.rowVolSliders[i].Draw(dst)
		dv.rowMuteBtns[i].pressed = dv.Rows[i].Muted
		dv.rowSoloBtns[i].pressed = dv.Rows[i].Solo
		dv.rowMuteBtns[i].Draw(dst)
		dv.rowSoloBtns[i].Draw(dst)
		dv.rowOriginBtns[i].Draw(dst)
		dv.rowDeleteBtns[i].Draw(dst)
		// Align steps exactly with the timeline width for a perfect fit.
		startX := dv.timelineRect.Min.X
		totalW := dv.timelineRect.Dx()
		n := len(r.Steps)
		if n < 1 {
			continue
		}
		// Always-visible baseline across the steps area so the time line is
		// perceivable even when individual subdivisions collapse to sub-pixel
		// widths at extreme zoom levels.
		baseY := y + dv.rowHeight()/2
		drawRect(dst, image.Rect(dv.timelineRect.Min.X, baseY, dv.timelineRect.Max.X, baseY+1), colTimelineBeat, true)
		// When subdivisions exceed pixel columns, render decimated marker lines
		// so the time lane remains visible even at extreme zoom-out.
		if n > totalW {
			step := int(math.Ceil(float64(n) / float64(totalW)))
			if step < 1 {
				step = 1
			}
			prevX := -1
			for j := 0; j <= n; j += step {
				x := startX + (j*totalW)/n
				if x != prevX {
					drawRect(dst, image.Rect(x, y, x+1, y+dv.rowHeight()), colTimelineBeat, true)
					prevX = x
				}
			}
			continue
		}
		for j, step := range r.Steps {
			// Proportional integer boundaries: distribute rounding across cells.
			x0 := startX + (j*totalW)/n
			x1 := startX + ((j+1)*totalW)/n
			if x1 <= x0 {
				x1 = x0 + 1
			}
			rect := image.Rect(x0, y, x1, y+dv.rowHeight())

			// Highlighting logic
			key := makeBeatKey(i, j+dv.Offset)
			highlighted := false
			isRegularNode := step

			if _, ok := highlightedBeats[key]; ok {
				highlighted = true
				if isRegularNode {
					dv.logger.Debugf("[DRUMVIEW] Draw: Highlighting regular node at row %d index %d", i, j)
				} else {
					dv.logger.Debugf("[DRUMVIEW] Draw: Highlighting empty beat at row %d index %d", i, j)
				}
			}

			DrumCellUI.Draw(dst, rect, step, highlighted, r.Color)
		}
	}

	if dv.renameBox != nil {
		dv.renameBox.Draw(dst)
	}

	// trailing "+" row
	dv.addRowBtn.Draw(dst)
	if len(dv.Rows)+1 > vis {
		bar := dv.scrollBarRect()
		drawRect(dst, bar, color.RGBA{80, 80, 80, 255}, true)
		thumb := dv.scrollThumbRect()
		drawRect(dst, thumb, color.RGBA{200, 200, 200, 255}, true)
	}

	if dv.instMenuOpen {
		for _, btn := range dv.instMenuBtns {
			btn.Draw(dst)
		}
	}

	if dv.uploading {
		ebitenutil.DebugPrintAt(dst, "Loading...", dv.uploadBtn.Rect().Min.X, dv.uploadBtn.Rect().Max.Y+20)
	}
	if dv.naming {
		box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
		BPMBoxStyle.Draw(dst, box, true, false)
		ebitenutil.DebugPrintAt(dst, "Name: "+dv.nameInput, box.Min.X+5, box.Min.Y+18)
		dv.saveBtn.Draw(dst)
	}
}

func (dv *DrumView) timelineInfo(elapsedBeats float64) string {
	totalBeats := math.Max(float64(dv.timelineBeats), elapsedBeats)

	// Convert to seconds and milliseconds with rounding, carrying overflows.
	curMS := int(math.Round(elapsedBeats * dv.secPerBeat * 1000.0))
	totMS := int(math.Round(totalBeats * dv.secPerBeat * 1000.0))
	curS, curMs := curMS/1000, curMS%1000
	totS, totMs := totMS/1000, totMS%1000

	return fmt.Sprintf("Beat %.3f/%.3f Time %ds %dms/%ds %dms", elapsedBeats, totalBeats, curS, curMs, totS, totMs)
}

// max1 returns at least 1 to avoid division by zero for unit conversions.
func max1(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func (dv *DrumView) bg(w, h int) *ebiten.Image {
	if dv.bgDirty || len(dv.bgCache) == 0 || !dv.bgCache[0].Bounds().Eq(image.Rect(0, 0, w, h)) {
		dv.bgCache = make([]*ebiten.Image, 1)
		img := ebiten.NewImage(w, h)
		img.Fill(colBGBottom)
		dv.bgCache[0] = img
		dv.bgDirty = false
	}
	return dv.bgCache[0]
}

// updateRowRects updates positions of existing per-row widgets without
// reallocating them. It computes the geometry based on the current bounds,
// visible window, and scroll offset. This keeps per-frame work minimal.
func (dv *DrumView) updateRowRects() {
	if len(dv.Rows) == 0 {
		return
	}
	// Ensure we have widgets for each row. This is a safety net for callers
	// that may have changed the number of rows without invoking calcLayout yet.
	if len(dv.rowLabels) != len(dv.Rows) ||
		len(dv.rowEditBtns) != len(dv.Rows) ||
		len(dv.rowVolSliders) != len(dv.Rows) ||
		len(dv.rowMuteBtns) != len(dv.Rows) ||
		len(dv.rowSoloBtns) != len(dv.Rows) ||
		len(dv.rowOriginBtns) != len(dv.Rows) ||
		len(dv.rowDeleteBtns) != len(dv.Rows) {
		// Full rebuild on mismatch to guarantee integrity.
		dv.calcLayout()
		return
	}
	if len(dv.Rows[0].Steps) == 0 {
		return
	}
	dv.cell = (dv.Bounds.Dx() - dv.labelW - dv.controlsW) / len(dv.Rows[0].Steps)
	vis := dv.visibleRows()
	for i := range dv.Rows {
		y := dv.Bounds.Min.Y + timelineHeight + (i-dv.rowOffset)*dv.rowHeight()
		rowRect := image.Rect(dv.Bounds.Min.X, y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, y+dv.rowHeight())
		if i < dv.rowOffset || i >= dv.rowOffset+vis {
			// Move rects off-screen to avoid accidental interactions if any draw slips through.
			rowRect = image.Rect(-1, -1, -1, -1)
		}
		g := NewGridLayout(rowRect, []float64{6, 2, 5, 2, 2, 2, 2}, []float64{1})
		if i < len(dv.rowLabels) {
			dv.rowLabels[i].SetRect(insetRect(g.Cell(0, 0), buttonPad))
		}
		if i < len(dv.rowEditBtns) {
			dv.rowEditBtns[i].SetRect(insetRect(g.Cell(1, 0), buttonPad))
		}
		if i < len(dv.rowVolSliders) {
			dv.rowVolSliders[i].SetRect(insetRect(g.Cell(2, 0), buttonPad))
		}
		if i < len(dv.rowMuteBtns) {
			dv.rowMuteBtns[i].SetRect(insetRect(g.Cell(3, 0), buttonPad))
		}
		if i < len(dv.rowSoloBtns) {
			dv.rowSoloBtns[i].SetRect(insetRect(g.Cell(4, 0), buttonPad))
		}
		if i < len(dv.rowOriginBtns) {
			dv.rowOriginBtns[i].SetRect(insetRect(g.Cell(5, 0), buttonPad))
		}
		if i < len(dv.rowDeleteBtns) {
			dv.rowDeleteBtns[i].SetRect(insetRect(g.Cell(6, 0), buttonPad))
		}
	}
	// Update trailing "+" button position
	y := dv.Bounds.Min.Y + timelineHeight + (len(dv.Rows)-dv.rowOffset)*dv.rowHeight()
	dv.addRowBtn.SetRect(insetRect(image.Rect(dv.Bounds.Min.X, y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, y+dv.rowHeight()), buttonPad))
}
