package ui

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

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
	bpmBox    *Button
	bpmIncBtn *Button // increase BPM
	lenDecBtn *Button // decrease length
	lenIncBtn *Button // increase length
	uploadBtn *Button
	saveBtn   *Button

	// per-row components
	addRowBtn     *Button
	rowLabels     []*Button
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

	deleted   []deletedRow
	added     []int
	originReq []int

	bgDirty bool
	bgCache []*ebiten.Image

	instOptions []string

	uploading  bool
	uploadCh   chan uploadResult
	pendingWAV string
	naming     bool
	nameInput  string

	timelineRect  image.Rectangle // progress bar for fast seek
	timelineBeats int             // total beats represented by timeline

	// internal ui state
	bpm           int
	focusBPM      bool
	playPressed   bool
	stopPressed   bool
	Length        int  // Length of the drum view, independent of graph
	bpmIncPressed bool // State for BPM increase button
	bpmDecPressed bool // State for BPM decrease button
	lenIncPressed bool // State for length increase button
	lenDecPressed bool // State for length decrease button

	bpmInput string // typed digits while editing
	bpmPrev  int    // previous BPM before editing

	// button animations
	playAnim     float64
	stopAnim     float64
	bpmDecAnim   float64
	bpmIncAnim   float64
	lenDecAnim   float64
	lenIncAnim   float64
	uploadAnim   float64
	bpmFocusAnim float64
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
		Bounds:        b,
		labelW:        100,
		bpm:           120,
		bgDirty:       true,
		Graph:         g,
		logger:        logger,
		Length:        8, // Default length
		Offset:        0,
		instOptions:   opts,
		uploadCh:      make(chan uploadResult, 1),
		timelineBeats: 8,
		selRow:        0,
		activeSlider:  -1,
	}
	dv.playBtn = NewButton("▶", PlayButtonStyle, func() {
		dv.playPressed = true
		dv.playAnim = 1
	})
	dv.stopBtn = NewButton("■", StopButtonStyle, func() {
		dv.stopPressed = true
		dv.stopAnim = 1
	})
	dv.bpmDecBtn = NewButton("-", BPMDecStyle, func() {
		dv.bpmDecPressed = true
		dv.bpmDecAnim = 1
	})
	dv.bpmDecBtn.Repeat = true
	dv.bpmBox = NewButton("120", BPMBoxStyle, nil)
	dv.bpmIncBtn = NewButton("+", BPMIncStyle, func() {
		dv.bpmIncPressed = true
		dv.bpmIncAnim = 1
	})
	dv.bpmIncBtn.Repeat = true
	dv.lenDecBtn = NewButton("-", LenDecStyle, func() {
		dv.lenDecPressed = true
		dv.lenDecAnim = 1
	})
	dv.lenDecBtn.Repeat = true
	dv.lenIncBtn = NewButton("+", LenIncStyle, func() {
		dv.lenIncPressed = true
		dv.lenIncAnim = 1
	})
	dv.lenIncBtn.Repeat = true
	dv.uploadBtn = NewButton("Upload", UploadBtnStyle, func() {
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
	dv.saveBtn = NewButton("Save", InstButtonStyle, nil)
	dv.addRowBtn = NewButton("+", InstButtonStyle, func() {
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
	if i < 0 || i >= len(dv.Rows) {
		return
	}
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
	topGrid := NewGridLayout(topBounds, []float64{1, 1, 1, 2, 1, 1, 1}, []float64{1})
	dv.playBtn.SetRect(insetRect(topGrid.Cell(0, 0), buttonPad))
	dv.stopBtn.SetRect(insetRect(topGrid.Cell(1, 0), buttonPad))
	dv.bpmDecBtn.SetRect(insetRect(topGrid.Cell(2, 0), buttonPad))
	dv.bpmBox.SetRect(insetRect(topGrid.Cell(3, 0), buttonPad))
	dv.bpmIncBtn.SetRect(insetRect(topGrid.Cell(4, 0), buttonPad))
	dv.lenDecBtn.SetRect(insetRect(topGrid.Cell(5, 0), buttonPad))
	dv.lenIncBtn.SetRect(insetRect(topGrid.Cell(6, 0), buttonPad))

	botBounds := image.Rect(dv.Bounds.Min.X+dv.labelW, dv.Bounds.Min.Y+dv.rowHeight(), dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+2*dv.rowHeight())
	botGrid := NewGridLayout(botBounds, []float64{1}, []float64{1})
	dv.uploadBtn.SetRect(insetRect(botGrid.Cell(0, 0), buttonPad))

	top := dv.Bounds.Min.Y + timelineHeight - timelineBarHeight - 5
	dv.timelineRect = image.Rect(
		dv.Bounds.Min.X+dv.labelW+dv.controlsW,
		top,
		dv.Bounds.Max.X-10,
		top+timelineBarHeight,
	)
}

func (dv *DrumView) calcLayout() {
	if len(dv.Rows) > 0 {
		dv.cell = (dv.Bounds.Dx() - dv.labelW - dv.controlsW) / len(dv.Rows[0].Steps)
	}
	dv.rowLabels = dv.rowLabels[:0]
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
		g := NewGridLayout(rowRect, []float64{6, 5, 2, 2, 2, 2}, []float64{1})
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
		slider := NewSlider(dv.Rows[i].Volume)
		slider.SetRect(insetRect(g.Cell(1, 0), buttonPad))
		mute := NewButton("M", InstButtonStyle, nil)
		mute.SetRect(insetRect(g.Cell(2, 0), buttonPad))
		solo := NewButton("S", InstButtonStyle, nil)
		solo.SetRect(insetRect(g.Cell(3, 0), buttonPad))
		origin := NewButton("O", InstButtonStyle, nil)
		origin.SetRect(insetRect(g.Cell(4, 0), buttonPad))
		del := NewButton("X", InstButtonStyle, nil)
		del.SetRect(insetRect(g.Cell(5, 0), buttonPad))
		delIdx := i
		del.OnClick = func() { dv.DeleteRow(delIdx) }
		originIdx := i
		origin.OnClick = func() { dv.originReq = append(dv.originReq, originIdx) }
		muteIdx := i
		mute.OnClick = func() { dv.toggleMute(muteIdx) }
		soloIdx := i
		solo.OnClick = func() { dv.toggleSolo(soloIdx) }
		dv.rowLabels = append(dv.rowLabels, lbl)
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
			dv.instMenuOpen = false
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
	dv.bpm = b
}

func (dv *DrumView) OffsetChanged() bool {
	if dv.offsetChanged {
		dv.offsetChanged = false
		return true
	}
	return false
}

func (dv *DrumView) SetLength(length int) {
	if length < 1 {
		length = 1
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
	dv.logger.Debugf("[DRUMVIEW] SetInstrument row=%d id=%s", dv.selRow, id)
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
	decay(&dv.bpmFocusAnim)
	decay(&dv.bpmErrorAnim)
	decay(&dv.saveAnim)
}

func (dv *DrumView) Update() {
	if len(dv.Rows) == 0 {
		return
	}

	dv.refreshInstruments()

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

	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}

	prevFocus := dv.focusBPM

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
				return
			}
		}
		if left {
			lbl := dv.rowLabels[dv.instMenuRow].Rect()
			menu := image.Rect(lbl.Min.X, lbl.Max.Y, lbl.Max.X, lbl.Max.Y+len(dv.instMenuBtns)*dv.rowHeight())
			if !pt(mx, my, lbl) && !pt(mx, my, menu) {
				dv.instMenuOpen = false
				// allow the click to reach underlying controls
			} else {
				return
			}
		}
	}

	stepsRect := image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+timelineHeight, dv.Bounds.Max.X, dv.Bounds.Max.Y)

	// wheel zoom for length adjustment
	if _, whY := wheel(); whY != 0 {
		if pt(mx, my, stepsRect) {
			if whY > 0 && dv.Length < 64 {
				dv.Length++
				for _, r := range dv.Rows {
					r.Steps = make([]bool, dv.Length)
				}
				dv.SetBeatLength(dv.Length)
				dv.bgDirty = true
				dv.logger.Infof("[DRUMVIEW] Length increased to: %d via wheel", dv.Length)
			}
			if whY < 0 && dv.Length > 1 {
				dv.Length--
				for _, r := range dv.Rows {
					r.Steps = make([]bool, dv.Length)
				}
				dv.SetBeatLength(dv.Length)
				dv.bgDirty = true
				dv.logger.Infof("[DRUMVIEW] Length decreased to: %d via wheel", dv.Length)
			}
		}
	}

	/* ——— widget clicks & dragging ——— */
	if dv.activeSlider >= 0 {
		s := dv.rowVolSliders[dv.activeSlider]
		if s.Handle(mx, my, left) {
			dv.Rows[dv.activeSlider].Volume = s.Value
		}
		if !left {
			dv.activeSlider = -1
		}
		return
	}
	for i, s := range dv.rowVolSliders {
		if s.Handle(mx, my, left) {
			dv.Rows[i].Volume = s.Value
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
		buttons := []*Button{dv.playBtn, dv.stopBtn, dv.bpmDecBtn, dv.bpmIncBtn, dv.lenDecBtn, dv.lenIncBtn, dv.addRowBtn, dv.uploadBtn}
		for _, btn := range buttons {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		if dv.bpmBox.Handle(mx, my, left) {
			if left && !dv.focusBPM {
				dv.focusBPM = true
				dv.bpmFocusAnim = 1
				dv.bpmPrev = dv.bpm
				dv.bpmInput = ""
				dv.logger.Debugf("[DRUMVIEW] BPM box clicked. focusingBPM: %t", dv.focusBPM)
			}
			if left {
				handled = true
			}
		}
		for _, lbl := range dv.rowLabels {
			if lbl.Handle(mx, my, left) {
				handled = true
			}
		}
	}

	if left {
		if !dv.dragging {
			if pt(mx, my, stepsRect) {
				dv.dragging = true
				dv.dragStartX = mx
				dv.startOffset = dv.Offset
			} else if dv.focusBPM {
				dv.focusBPM = false
				dv.logger.Debugf("[DRUMVIEW] Clicked outside BPM box. focusingBPM: %t", dv.focusBPM)
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

	// timeline scrubbing
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
		totalBeats := dv.timelineBeats
		frac := float64(pos-dv.timelineRect.Min.X) / float64(dv.timelineRect.Dx())
		beat := int(frac * float64(totalBeats))
		if beat < 0 {
			beat = 0
		}
		if beat+dv.Length > dv.timelineBeats {
			dv.timelineBeats = beat + dv.Length
			totalBeats = dv.timelineBeats
			beat = int(frac * float64(totalBeats))
		}
		if beat != dv.Offset {
			dv.Offset = beat
			dv.offsetChanged = true
		}
		if !left {
			dv.scrubbing = false
		}
	}

	/* ——— BPM editing ——— */
	if dv.focusBPM {
		for _, r := range inputChars() {
			if r >= '0' && r <= '9' {
				dv.bpmInput += string(r)
			}
		}
		if isKeyPressed(ebiten.KeyBackspace) {
			if l := len(dv.bpmInput); l > 0 {
				dv.bpmInput = dv.bpmInput[:l-1]
			}
		}
		if isKeyPressed(ebiten.KeyEnter) {
			dv.focusBPM = false
		}
		if dv.bpmInput != "" {
			if v, err := strconv.Atoi(dv.bpmInput); err == nil {
				dv.bpm = v
				dv.logger.Debugf("[DRUMVIEW] BPM changed to: %d", dv.bpm)
			}
		} else {
			dv.bpm = dv.bpmPrev
		}
	}

	if !dv.focusBPM && prevFocus {
		if dv.bpmInput == "" {
			dv.bpm = dv.bpmPrev
		}
		dv.SetBPM(dv.bpm)
		dv.bpmInput = ""
	}

	/* ——— BPM editing via buttons ——— */
	if dv.bpmIncPressed {
		dv.SetBPM(dv.bpm + 1)
		dv.logger.Infof("[DRUMVIEW] BPM increased to: %d", dv.bpm)
		dv.bpmIncPressed = false
	}
	if dv.bpmDecPressed {
		dv.SetBPM(dv.bpm - 1)
		dv.logger.Infof("[DRUMVIEW] BPM decreased to: %d", dv.bpm)
		dv.bpmDecPressed = false
	}

	/* ——— Length editing ——— */
	if dv.lenIncPressed {
		if dv.Length < 64 { // Set a reasonable max length
			dv.Length++
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
		if dv.Length > 1 { // Prevent length from going below 1
			dv.Length--
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

func (dv *DrumView) Draw(dst *ebiten.Image, highlightedBeats map[int]int64, frame int64, beatInfos []model.BeatInfo, elapsedBeats int) {
	dv.logger.Debugf("[DRUMVIEW] Draw called. beatInfos: %v, highlightedBeats: %v", beatInfos, highlightedBeats)
	// draw background
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
	dst.DrawImage(dv.bg(dv.Bounds.Dx(), dv.Bounds.Dy()), op)

	dv.decayAnims()
	dv.calcLayout()

	bpmText := dv.bpmInput
	if !dv.focusBPM {
		bpmText = strconv.Itoa(dv.bpm)
	}
	dv.bpmBox.Text = bpmText
	if len(dv.Rows) > 0 {
	}
	dv.playBtn.Draw(dst)
	dv.stopBtn.Draw(dst)
	dv.bpmDecBtn.Draw(dst)
	dv.bpmBox.Draw(dst)
	dv.bpmIncBtn.Draw(dst)
	dv.lenDecBtn.Draw(dst)
	dv.lenIncBtn.Draw(dst)
	dv.uploadBtn.Draw(dst)
	// timeline and progress
	if dv.timelineBeats < dv.Graph.BeatLength() {
		dv.timelineBeats = dv.Graph.BeatLength()
	}
	if elapsedBeats+dv.Length > dv.timelineBeats {
		dv.timelineBeats = elapsedBeats + dv.Length
	}
	if dv.Offset+dv.Length > dv.timelineBeats {
		dv.timelineBeats = dv.Offset + dv.Length
	}
	totalBeats := dv.timelineBeats
	info := dv.timelineInfo(elapsedBeats)
	ebitenutil.DebugPrintAt(dst, info, dv.timelineRect.Min.X, dv.Bounds.Min.Y+5)
	drawRect(dst, dv.timelineRect, colTimelineTotal, true)

	// current view rectangle
	viewStart := dv.timelineRect.Min.X + int(float64(dv.Offset)/float64(totalBeats)*float64(dv.timelineRect.Dx()))
	viewWidth := int(float64(dv.Length) / float64(totalBeats) * float64(dv.timelineRect.Dx()))
	viewRect := image.Rect(viewStart, dv.timelineRect.Min.Y, viewStart+viewWidth, dv.timelineRect.Max.Y)
	drawRect(dst, viewRect, colTimelineView, true)

	// beat markers
	for i := 0; i <= totalBeats; i++ {
		x := dv.timelineRect.Min.X + int(float64(i)/float64(totalBeats)*float64(dv.timelineRect.Dx()))
		drawRect(dst, image.Rect(x, dv.timelineRect.Min.Y, x+1, dv.timelineRect.Max.Y), color.RGBA{100, 100, 100, 255}, true)
	}

	// current playback cursor
	cursorX := dv.timelineRect.Min.X + int(float64(elapsedBeats)/float64(totalBeats)*float64(dv.timelineRect.Dx()))
	cursorRect := image.Rect(cursorX-1, dv.timelineRect.Min.Y, cursorX+1, dv.timelineRect.Max.Y)
	drawRect(dst, cursorRect, colTimelineCursor, true)

	drawRect(dst, dv.timelineRect, colButtonBorder, false)

	// draw steps
	vis := dv.visibleRows()
	for i, r := range dv.Rows {
		if i < dv.rowOffset || i >= dv.rowOffset+vis {
			continue
		}
		y := dv.Bounds.Min.Y + timelineHeight + (i-dv.rowOffset)*dv.rowHeight()
		dv.rowLabels[i].Draw(dst)
		dv.rowVolSliders[i].Draw(dst)
		dv.rowMuteBtns[i].pressed = dv.Rows[i].Muted
		dv.rowSoloBtns[i].pressed = dv.Rows[i].Solo
		dv.rowMuteBtns[i].Draw(dst)
		dv.rowSoloBtns[i].Draw(dst)
		dv.rowOriginBtns[i].Draw(dst)
		dv.rowDeleteBtns[i].Draw(dst)
		for j, step := range r.Steps {
			x := dv.Bounds.Min.X + dv.labelW + dv.controlsW + j*dv.cell
			rect := image.Rect(x, y, x+dv.cell, y+dv.rowHeight())

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

func (dv *DrumView) timelineInfo(elapsedBeats int) string {
	beatsToDuration := func(beats int) time.Duration {
		return time.Duration(float64(beats) * 60 / float64(dv.bpm) * float64(time.Second))
	}
	totalBeats := dv.timelineBeats
	totalDur := beatsToDuration(totalBeats)
	curDur := beatsToDuration(elapsedBeats)
	curMin := int(curDur / time.Minute)
	curSec := int((curDur % time.Minute) / time.Second)
	curMilli := int((curDur % time.Second) / time.Millisecond)
	totMin := int(totalDur / time.Minute)
	totSec := int((totalDur % time.Minute) / time.Second)
	totMilli := int((totalDur % time.Second) / time.Millisecond)

	viewStartBeat := dv.Offset
	viewEndBeat := dv.Offset + dv.Length
	viewStartDur := beatsToDuration(viewStartBeat)
	viewEndDur := beatsToDuration(viewEndBeat)
	vStartMin := int(viewStartDur / time.Minute)
	vStartSec := int((viewStartDur % time.Minute) / time.Second)
	vStartMilli := int((viewStartDur % time.Second) / time.Millisecond)
	vEndMin := int(viewEndDur / time.Minute)
	vEndSec := int((viewEndDur % time.Minute) / time.Second)
	vEndMilli := int((viewEndDur % time.Second) / time.Millisecond)

	return fmt.Sprintf("%02d:%02d.%03d/%02d:%02d.%03d | View %02d:%02d.%03d-%02d:%02d.%03d | Beats %d-%d/%d",
		curMin, curSec, curMilli,
		totMin, totSec, totMilli,
		vStartMin, vStartSec, vStartMilli,
		vEndMin, vEndSec, vEndMilli,
		viewStartBeat+1, viewEndBeat, totalBeats)
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
