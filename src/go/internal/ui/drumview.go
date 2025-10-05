package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

const (
	asciiPrintableMin = 32
	asciiPrintableMax = 126
	// timelineHeight reserves vertical space for the transport controls and
	// the thin timeline bar above the instrument rows. Keep this as small as
	// possible so the bottom panel wastes no vertical space. Two rows of
	// controls (2×rowHeight) + a 10px bar + a small margin is sufficient.
	timelineHeight    = 64
	timelineBarHeight = 10
	buttonPad         = 2
)

// Number of auto-generated color swatches offered in the color menu.
// Reduced to keep selection concise.
const generatedColorCount = 6

/* ───────────────────────────────────────────────────────────── */

type DrumRow struct {
	Name       string
	Instrument string
	Steps      []bool
	CellTypes  []model.NodeType
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
	if len(customPalette) == 0 {
		// Graceful fallback when palette is empty (e.g., in tests).
		return color.RGBA{200, 200, 200, 255}
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
	playBtn       *Button
	stopBtn       *Button
	bpmDecBtn     *Button // decrease BPM
	bpmBox        *TextInput
	bpmIncBtn     *Button // increase BPM
	subdivBtn     *Button // subdivisions-per-beat dropdown
	lenDecBtn     *Button // decrease length
	lenIncBtn     *Button // increase length
	trackBtn      *Button // toggle follow playback
	uploadBtn     *Button
	importBtn     *Button
	exportBtn     *Button
	saveBtn       *Button
	mainVolSlider *Slider
	mainVolRect   image.Rectangle

	// per-row components
	addRowBtn     *Button
	rowLabels     []*Button
	rowEditBtns   []*Button
	rowColorBtns  []*Button
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

	// color picker: wheel popup
	colorMenuOpen  bool
	colorMenuRow   int
	colorMenuBtns  []*Button  // deprecated (kept for compatibility; unused)
	colorHexBox    *TextInput // deprecated; no longer used
	colorWheelRect image.Rectangle
	colorWheelImg  *ebiten.Image
	wheelCacheW    int
	wheelCacheH    int
	colorHold      bool

	// subdiv dropdown
	subdivMenuOpen bool
	subdivMenuBtns []*Button

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
	instAvail   map[string]bool
	missingInst map[string]bool
	instMu      sync.RWMutex

	uploading  bool
	uploadCh   chan uploadResult
	pendingWAV string
	naming     bool
	nameInput  string     // legacy helper, not used for typing anymore
	nameBox    *TextInput // active while naming

	// JSON import
	importing bool
	importCh  chan importResult

	// callback to change subdivisions per beat
	onChangeSubdiv func(int) error

	// sample loading status (WASM): show a transient message while embedded
	// samples are being registered and another once finished.
	samplesTotal  int
	samplesLoaded int
	showLoading   bool
	doneMsgTimer  int // frames to show "Finished loading samples"

	// timeline base cache (background + beat markers)
	tlCache      *ebiten.Image
	tlCacheW     int
	tlCacheH     int
	tlCacheBeats int
	tlCacheStep  int
	// cached timeline info label to reduce fmt/string allocs
	lastInfoCurMS int
	lastInfoTotMS int
	lastInfoText  string

	// per-row cached sprites for the steps area (no highlights). Each sprite
	// covers the full timeline width and one row height. Rebuilt when length,
	// row color, or timeline width/height change.
	rowCache      []*ebiten.Image
	rowDirty      []bool
	rowFullDirty  []bool
	rowCacheW     int
	rowCacheH     int
	rowCacheLen   int
	rowCacheOff   []int
	rowCacheGen   []int
	rowCachePadPx int

	// rowsLayer caches the composition of all visible row sprites (rowCache)
	// for the current offset and scroll. Highlights are drawn on top separately.
	rowsLayer       *ebiten.Image
	rowsLayerW      int
	rowsLayerH      int
	rowsLayerOffset int
	rowsLayerRowOff int
	rowsLayerGen    int
	rowsLayerDirty  bool
	rowsLayerBytes  int64
	rowsLayerFrame  int64
	rowsRepaints    int
	rowFrame        []int64
	rowRepaint      []int

	frame int64

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
	// Local cache of custom instrument WAV paths by instrument ID. Used for
	// export so imported projects can auto-register custom samples.
	samplePath map[string]string

	// Playing flag (copied from Game) so DrumView can freeze header scale
	// and avoid cursor jumps during active playback.
	isPlaying bool

	// simple draw hint propagated from Game; enables low-overhead highlight path
	simpleDraw bool

	// cached sprites for simple highlight rendering
	hlSpriteReg  *ebiten.Image
	hlSpriteMute *ebiten.Image
	hlSpriteH    int
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
	if dv.instMenuOpen || dv.colorMenuOpen || dv.subdivMenuOpen || dv.instHold || dv.renameBox != nil || dv.naming {
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
	// Do not change the visual timeline scale while playing to avoid jumps
	// in the cursor position. The header can be updated after Stop.
	if !dv.isPlaying {
		if length > dv.timelineBeats {
			dv.timelineBeats = length
		}
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
		samplePath:           make(map[string]string),
	}
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
	dv.bpmBox.SetText("120")
	dv.bpmIncBtn = NewButton("+", BPMIncStyle, func() {
		dv.logger.Infof("[DRUMVIEW] BPM + button pressed")
		dv.bpmDelta++
		dv.bpmIncAnim = 1
	})
	dv.bpmIncBtn.Repeat = true
	dv.subdivBtn = NewButton("32", InstButtonStyle, nil)
	dv.subdivBtn.OnClick = func() {
		dv.colorMenuOpen = false
		dv.instMenuOpen = false
		dv.subdivMenuOpen = !dv.subdivMenuOpen
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
		dv.importing = true
		dv.colorMenuOpen = false
		dv.instMenuOpen = false
		selectJSONAsync(func(data []byte, err error) {
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
	dv.Rows = []*DrumRow{{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), CellTypes: make([]model.NodeType, dv.Length), Color: uniq, Origin: model.InvalidNodeID, Volume: 1}}
	dv.SetBeatLength(dv.Length) // Initialize graph's beat length
	// Initialize instrument availability/options immediately so early
	// highlight/audio paths (e.g., tests calling spawnPulseFrom before the
	// first Update) see valid instruments and do not suppress playback.
	dv.refreshInstruments()
	dv.recalcButtons()
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}
	dv.ensureRowCache()
	dv.markAllRowsDirty()
	dv.rowCachePadPx = 32
	// Reset global click suppression to ensure clean state for new views/tests.
	suppressClicksUntilRelease = false
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
		// Invalidate row caches when bounds change (timeline width/height).
		dv.invalidateRowCaches()
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
	baseCol := instColor(inst)
	uniq := dv.ensureUniqueColor(baseCol, idx)
	dv.Rows = append(dv.Rows, &DrumRow{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), CellTypes: make([]model.NodeType, dv.Length), Color: uniq, Origin: model.InvalidNodeID, Node: nil, Volume: 1})
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
	// Ensure cache slices account for the new row and mark it dirty.
	dv.ensureRowCache()
	if len(dv.rowDirty) > 0 {
		idx := len(dv.rowDirty) - 1
		dv.rowDirty[idx] = true
		if idx < len(dv.rowFullDirty) {
			dv.rowFullDirty[idx] = true
		}
	}
	if len(dv.rowFrame) != len(dv.Rows) {
		rf := make([]int64, len(dv.Rows))
		copy(rf, dv.rowFrame)
		dv.rowFrame = rf
	}
	if len(dv.rowRepaint) != len(dv.Rows) {
		rr := make([]int, len(dv.Rows))
		copy(rr, dv.rowRepaint)
		dv.rowRepaint = rr
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
	// Reset caches so indexes realign.
	dv.rowCache = nil
	dv.rowDirty = nil
	dv.rowFullDirty = nil
	dv.rowFrame = nil
	dv.rowRepaint = nil
	dv.rowsLayer = nil
	dv.rowsLayerW, dv.rowsLayerH = 0, 0
	dv.rowsLayerOffset = 0
	dv.rowsLayerRowOff = 0
	dv.rowsLayerGen = 0
	dv.rowsLayerDirty = true
	dv.rowsLayerBytes = 0
	dv.rowsLayerFrame = 0
	dv.rowsRepaints = 0
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
	dv.controlsW = dv.Bounds.Dx() / 3
	if dv.controlsW < 180 {
		dv.controlsW = 180
	}
	if dv.controlsW > 480 {
		dv.controlsW = 480
	}

	topBounds := image.Rect(dv.Bounds.Min.X+dv.labelW, dv.Bounds.Min.Y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+dv.rowHeight())
	// columns: play | stop | bpm box | bpm(+/- stacked) | subdiv | len(+/- stacked) | track | label | slider
	topGrid := NewGridLayout(topBounds, []float64{1, 1, 2, 1, 1, 1, 1, 1, 3}, []float64{1})
	dv.playBtn.SetRect(insetRect(topGrid.Cell(0, 0), buttonPad))
	dv.stopBtn.SetRect(insetRect(topGrid.Cell(1, 0), buttonPad))
	dv.bpmBox.Rect = insetRect(topGrid.Cell(2, 0), buttonPad)
	// BPM +/- stacked vertically in a single column
	bpmCol := insetRect(topGrid.Cell(3, 0), buttonPad)
	bpmGrid := NewGridLayout(bpmCol, []float64{1}, []float64{1, 1})
	dv.bpmIncBtn.SetRect(insetRect(bpmGrid.Cell(0, 0), 1))
	dv.bpmDecBtn.SetRect(insetRect(bpmGrid.Cell(0, 1), 1))
	dv.subdivBtn.SetRect(insetRect(topGrid.Cell(4, 0), buttonPad))
	// Length +/- stacked vertically in a single column
	lenCol := insetRect(topGrid.Cell(5, 0), buttonPad)
	lenGrid := NewGridLayout(lenCol, []float64{1}, []float64{1, 1})
	dv.lenIncBtn.SetRect(insetRect(lenGrid.Cell(0, 0), 1))
	dv.lenDecBtn.SetRect(insetRect(lenGrid.Cell(0, 1), 1))
	dv.trackBtn.SetRect(insetRect(topGrid.Cell(6, 0), buttonPad))
	if dv.mainVolSlider != nil {
		dv.mainVolRect = insetRect(topGrid.Cell(8, 0), buttonPad)
		dv.mainVolSlider.SetRect(dv.mainVolRect)
	}

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
	dv.rowColorBtns = dv.rowColorBtns[:0]
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
		// grid: [label][edit][color][slider][M][S][O][X]
		g := NewGridLayout(rowRect, []float64{6, 2, 2, 5, 2, 2, 2, 2}, []float64{1})
		style := InstButtonStyle
		if !dv.IsInstrumentAvailable(dv.Rows[i].Instrument) {
			style = MissingInstStyle
		}
		lbl := NewButton(dv.Rows[i].Name, style, nil)
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
				dv.colorMenuOpen = false
				dv.buildInstMenu()
				dv.logger.Debugf("[DRUMVIEW] Opening instrument menu for row %d", idx)
				SuppressClicksUntilMouseUp()
			}
		}
		edit := NewButton("✎", InstButtonStyle, nil)
		edit.Icon = "pencil"
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
			dv.colorMenuOpen = false
		}
		// Color swatch button. Use an immediate function to bind the index.
		swatch := func(idx int) *Button {
			colorFn := func() color.Color {
				if idx >= 0 && idx < len(dv.Rows) {
					return dv.Rows[idx].Color
				}
				return color.RGBA{200, 200, 200, 255}
			}
			b := NewButton("", ColorSwatchStyle{Color: colorFn, Border: colButtonBorder}, nil)
			b.SetRect(insetRect(g.Cell(2, 0), buttonPad))
			b.OnClick = func() {
				dv.selRow = idx
				if dv.colorMenuOpen && dv.colorMenuRow == idx {
					dv.colorMenuOpen = false
					dv.logger.Debugf("[COLOR] row=%d: toggle close", idx)
				} else {
					dv.colorMenuRow = idx
					dv.colorMenuOpen = true
					dv.instMenuOpen = false
					dv.buildColorMenu()
					dv.colorHold = true
					dv.logger.Debugf("[COLOR] row=%d: open requested (hold=true) base=%v wheel=%v", idx, dv.rowColorBtns[idx].Rect(), dv.colorWheelRect)
					SuppressClicksUntilMouseUp()
				}
			}
			return b
		}(i)
		slider := NewSlider(dv.Rows[i].Volume)
		slider.SetRect(insetRect(g.Cell(3, 0), buttonPad))
		mute := NewButton("M", InstButtonStyle, nil)
		mute.SetRect(insetRect(g.Cell(4, 0), buttonPad))
		solo := NewButton("S", InstButtonStyle, nil)
		solo.SetRect(insetRect(g.Cell(5, 0), buttonPad))
		origin := NewButton("O", InstButtonStyle, nil)
		origin.SetRect(insetRect(g.Cell(6, 0), buttonPad))
		del := NewButton("X", InstButtonStyle, nil)
		del.ConsumeOnPress = true
		del.SetRect(insetRect(g.Cell(7, 0), buttonPad))
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
		dv.rowColorBtns = append(dv.rowColorBtns, swatch)
		dv.rowVolSliders = append(dv.rowVolSliders, slider)
		dv.rowMuteBtns = append(dv.rowMuteBtns, mute)
		dv.rowSoloBtns = append(dv.rowSoloBtns, solo)
		dv.rowOriginBtns = append(dv.rowOriginBtns, origin)
		dv.rowDeleteBtns = append(dv.rowDeleteBtns, del)
	}
	y := dv.Bounds.Min.Y + timelineHeight + (len(dv.Rows)-dv.rowOffset)*dv.rowHeight()
	dv.addRowBtn.SetRect(insetRect(image.Rect(dv.Bounds.Min.X, y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, y+dv.rowHeight()), buttonPad))
	// If timeline dimensions changed, row sprite caches must be rebuilt.
	if dv.rowCacheW != dv.timelineRect.Dx() || dv.rowCacheH != dv.rowHeight() {
		dv.rowCacheW = dv.timelineRect.Dx()
		dv.rowCacheH = dv.rowHeight()
		dv.markAllRowsDirty()
	}
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
	// Determine starting Y while clamping to DrumView bounds to avoid
	// negative coordinates when the menu opens upward in short panels.
	startY := 0
	if openUp {
		startY = base.Min.Y - menuH
		if startY < dv.Bounds.Min.Y {
			startY = dv.Bounds.Min.Y
		}
	} else {
		startY = base.Max.Y
		if startY+menuH > dv.Bounds.Max.Y {
			startY = dv.Bounds.Max.Y - menuH
		}
	}
	for i, id := range dv.instOptions {
		var r image.Rectangle
		r = image.Rect(base.Min.X, startY+i*dv.rowHeight(), base.Max.X, startY+(i+1)*dv.rowHeight())
		optID := id
		btn := NewButton(strings.ToUpper(id[:1])+id[1:], DropdownStyle, func() {
			dv.SetInstrument(optID)
		})
		btn.SetRect(insetRect(r, buttonPad))
		dv.instMenuBtns = append(dv.instMenuBtns, btn)
	}
}

// buildColorMenu rebuilds the color picker dropdown for the selected row.
func (dv *DrumView) buildColorMenu() {
	dv.colorMenuBtns = dv.colorMenuBtns[:0]
	dv.colorHexBox = nil
	dv.colorWheelRect = image.Rect(0, 0, 0, 0)
	if dv.colorMenuRow < 0 || dv.colorMenuRow >= len(dv.rowColorBtns) {
		return
	}
	base := dv.rowColorBtns[dv.colorMenuRow].Rect()
	// Determine wheel size and placement; always keep fully within dv.Bounds.
	// Start from a target diameter based on row height, clamp to bounds.
	target := dv.rowHeight() * 6
	if target < 60 {
		target = 60
	}
	if target > 200 {
		target = 200
	}
	maxSize := dv.Bounds.Dx()
	if dv.Bounds.Dy() < maxSize {
		maxSize = dv.Bounds.Dy()
	}
	if maxSize < 1 {
		maxSize = 1
	}
	wheel := target
	if wheel > maxSize {
		wheel = maxSize
	}
	// If panel is extremely small, still render something.
	if wheel < 20 {
		wheel = maxSize
	}

	// Prefer opening upwards if there's room, else below; clamp both axes.
	wantY := base.Min.Y - wheel
	if wantY < dv.Bounds.Min.Y {
		// place below
		wantY = base.Max.Y
	}
	// Clamp to bounds
	if wantY < dv.Bounds.Min.Y {
		wantY = dv.Bounds.Min.Y
	}
	if wantY > dv.Bounds.Max.Y-wheel {
		wantY = dv.Bounds.Max.Y - wheel
	}
	if wantY < dv.Bounds.Min.Y {
		wantY = dv.Bounds.Min.Y
	}

	wantX := base.Min.X
	if wantX < dv.Bounds.Min.X {
		wantX = dv.Bounds.Min.X
	}
	if wantX > dv.Bounds.Max.X-wheel {
		wantX = dv.Bounds.Max.X - wheel
	}
	if wantX < dv.Bounds.Min.X {
		wantX = dv.Bounds.Min.X
	}

	dv.colorWheelRect = image.Rect(wantX, wantY, wantX+wheel, wantY+wheel)
	dv.logger.Debugf("[COLOR] build wheel: bounds=%v base=%v target=%d maxSize=%d final=%dx%d at=(%d,%d)", dv.Bounds, base, target, maxSize, dv.colorWheelRect.Dx(), dv.colorWheelRect.Dy(), dv.colorWheelRect.Min.X, dv.colorWheelRect.Min.Y)
	dv.rebuildColorWheelImage()
}

func (dv *DrumView) buildSubdivMenu() {
	dv.subdivMenuBtns = dv.subdivMenuBtns[:0]
	vals := []int{4, 8, 16, 32}
	base := dv.subdivBtn.Rect()
	for i, v := range vals {
		r := image.Rect(base.Min.X, base.Max.Y+i*dv.rowHeight(), base.Max.X, base.Max.Y+(i+1)*dv.rowHeight())
		text := fmt.Sprintf("%d", v)
		vv := v
		btn := NewButton(text, DropdownStyle, func() {
			if dv.onChangeSubdiv != nil {
				if err := dv.onChangeSubdiv(vv); err != nil {
					return
				}
			}
			dv.subdivBtn.Text = text
			dv.timelineUnitsPerBeat = vv
			dv.subdivMenuOpen = false
		})
		btn.ConsumeOnPress = true
		btn.SetRect(insetRect(r, buttonPad))
		dv.subdivMenuBtns = append(dv.subdivMenuBtns, btn)
	}
}

func (dv *DrumView) refreshInstruments() {
	// Query available instruments
	opts := audio.Instruments()
	dv.instMu.Lock()
	dv.instAvail = map[string]bool{}
	for _, id := range opts {
		dv.instAvail[id] = true
	}
	// Initialize missing map if needed
	if dv.missingInst == nil {
		dv.missingInst = map[string]bool{}
	}
	// Remove from missing any IDs that just became available
	for id := range dv.missingInst {
		if dv.instAvail[id] {
			delete(dv.missingInst, id)
		}
	}
	// Build options: available first, then missing IDs
	combined := make([]string, 0, len(opts)+len(dv.missingInst))
	combined = append(combined, opts...)
	for id := range dv.missingInst {
		if !dv.instAvail[id] {
			combined = append(combined, id)
		}
	}
	changed := !slices.Equal(combined, dv.instOptions)
	dv.instOptions = combined
	dv.instMu.Unlock()
	// Update row label styles to reflect new availability.
	for i := range dv.Rows {
		if i < len(dv.rowLabels) {
			id := dv.Rows[i].Instrument
			if dv.IsInstrumentAvailable(id) {
				dv.rowLabels[i].Style = InstButtonStyle
			} else {
				dv.rowLabels[i].Style = MissingInstStyle
			}
		}
	}
	if changed && dv.instMenuOpen {
		dv.buildInstMenu()
	}
}

// IsInstrumentAvailable reports whether an instrument id is currently available.
func (dv *DrumView) IsInstrumentAvailable(id string) bool {
	if id == "" {
		return false
	}
	dv.instMu.RLock()
	ok := dv.instAvail != nil && dv.instAvail[id]
	dv.instMu.RUnlock()
	return ok
}

// EnsureInstrumentKnown records an instrument id so it appears in menus even
// when not currently available (e.g., imported from another machine). Once the
// instrument becomes available, it will be removed from the missing list.
func (dv *DrumView) EnsureInstrumentKnown(id string) {
	if id == "" {
		return
	}
	// If already available, nothing to do.
	if dv.IsInstrumentAvailable(id) {
		return
	}
	dv.instMu.Lock()
	if dv.missingInst == nil {
		dv.missingInst = map[string]bool{}
	}
	dv.missingInst[id] = true
	dv.instMu.Unlock()
	dv.refreshInstruments()
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
	dv.isPlaying = p
	if p {
		dv.playBtn.Text = "⏸"
		dv.playBtn.Icon = "pause"
	} else {
		dv.playBtn.Text = "▶"
		dv.playBtn.Icon = "play"
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
		r.CellTypes = make([]model.NodeType, dv.Length)
	}
	dv.SetBeatLength(dv.Length)
	dv.bgDirty = true
}

func (dv *DrumView) SetInstrument(id string) {
	if len(dv.Rows) == 0 {
		return
	}
	// Clamp selected row to valid range to avoid out-of-bounds when rows were
	// recently added/removed or after import/naming flows.
	if dv.selRow < 0 || dv.selRow >= len(dv.Rows) {
		if len(dv.Rows) == 0 {
			return
		}
		dv.selRow = len(dv.Rows) - 1
	}
	dv.logger.Infof("[DRUMVIEW] Instrument set row=%d id=%s", dv.selRow, id)
	dv.Rows[dv.selRow].Instrument = id
	if id != "" {
		dv.Rows[dv.selRow].Name = strings.ToUpper(id[:1]) + id[1:]
	}
	dv.Rows[dv.selRow].Color = dv.ensureUniqueColor(instColor(id), dv.selRow)
	// Update label text and style immediately; also mark layout dirty so full rebuild
	if dv.selRow < len(dv.rowLabels) {
		dv.rowLabels[dv.selRow].Text = dv.Rows[dv.selRow].Name
		if dv.IsInstrumentAvailable(id) {
			dv.rowLabels[dv.selRow].Style = InstButtonStyle
		} else {
			dv.rowLabels[dv.selRow].Style = MissingInstStyle
		}
	}
	dv.bgDirty = true
}

func (dv *DrumView) AddInstrument(id string) {
	dv.instOptions = audio.Instruments()
	dv.SetInstrument(id)
	// If this is a known custom sample id, try to re-register from cache.
	if dv.samplePath != nil {
		if p, ok := dv.samplePath[id]; ok && p != "" && !dv.IsInstrumentAvailable(id) {
			_ = audio.RegisterWAV(id, p)
			dv.refreshInstruments()
		}
	}
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
		if dv.samplePath == nil {
			dv.samplePath = map[string]string{}
		}
		dv.samplePath[id] = dv.pendingWAV
		dv.refreshInstruments()
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
	dv.nameBox = nil
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

	// Global click-suppression guard resets on mouse release each frame
	if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
		suppressClicksUntilRelease = false
	}

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
				// Initialize naming input box for consistent UX
				box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
				dv.nameBox = NewTextInput(box, BPMBoxStyle)
				dv.nameBox.SetText("")
				dv.nameBox.focused = true
			}
		default:
		}
	}

	if dv.naming {
		// Keep rect in sync with layout and update input
		box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
		if dv.nameBox == nil {
			dv.nameBox = NewTextInput(box, BPMBoxStyle)
			dv.nameBox.focused = true
		}
		dv.nameBox.Rect = box
		if dv.saveBtn == nil {
			dv.saveBtn = NewButton("Save", UploadBtnStyle, nil)
		}
		dv.saveBtn.SetRect(image.Rect(box.Max.X+10, box.Min.Y, box.Max.X+60, box.Max.Y))
		dv.saveBtn.OnClick = func() {
			id := strings.TrimSpace(dv.nameBox.Value())
			dv.logger.Infof("[DRUMVIEW] Save instrument pressed id=%q", id)
			if id != "" {
				dv.registerInstrument(id)
			}
		}
		dv.nameBox.Update()
		if isKeyPressed(ebiten.KeyEnter) {
			id := strings.TrimSpace(dv.nameBox.Value())
			if id != "" {
				dv.registerInstrument(id)
			}
		}
		if isKeyPressed(ebiten.KeyEscape) {
			dv.naming = false
			dv.pendingWAV = ""
			dv.nameInput = ""
			dv.nameBox = nil
		}
		mx, my := cursorPosition()
		left := isMouseButtonPressed(ebiten.MouseButtonLeft)
		if dv.saveBtn.Handle(mx, my, left) {
			dv.saveAnim = 1
		} else if left && !pt(mx, my, dv.nameBox.Rect) {
			// Click outside cancels naming (same as Esc)
			dv.naming = false
			dv.pendingWAV = ""
			dv.nameInput = ""
			dv.nameBox = nil
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
		// Click outside cancels (same as Esc) once hold is released.
		mx, my := cursorPosition()
		left := isMouseButtonPressed(ebiten.MouseButtonLeft)
		if !dv.renameHold && left && !pt(mx, my, dv.renameBox.Rect) {
			dv.logger.Debugf("[DRUMVIEW] Rename canceled by outside click (row=%d)", dv.renameRow)
			dv.renameBox = nil
			dv.renameRow = -1
			return
		}
		if !dv.renameHold && isKeyPressed(ebiten.KeyEnter) {
			name := strings.TrimSpace(dv.renameBox.Value())
			if name != "" && dv.renameRow >= 0 && dv.renameRow < len(dv.Rows) {
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
	// Enter key should commit immediately like other editors.
	if dv.bpmBox.Focused() && isKeyPressed(ebiten.KeyEnter) {
		txt := dv.bpmBox.Value()
		if txt == "" {
			prev := dv.bpmPrev
			if prev < 1 {
				prev = dv.bpm
			}
			dv.SetBPM(prev)
		} else if v, ok := parseBPM(txt); ok {
			dv.SetBPM(v)
		} else {
			dv.bpmErrorAnim = 1
			prev := dv.bpmPrev
			if prev < 1 {
				prev = dv.bpm
			}
			dv.SetBPM(prev)
		}
		dv.bpmBox.SetText(strconv.Itoa(dv.bpm))
		dv.bpmBox.focused = false
	}
	if !prevFocus && dv.bpmBox.Focused() {
		dv.logger.Debugf("[DRUMVIEW] BPM box focused")
		dv.bpmPrev = dv.bpm
		dv.bpmBox.SetText("")
	}
	if prevFocus && !dv.bpmBox.Focused() {
		dv.logger.Debugf("[DRUMVIEW] BPM box blurred value=%q", prevVal)
		// Commit immediately on blur (e.g., Enter pressed) so BPM updates without waiting
		// for later handlers in this frame.
		txt := dv.bpmBox.Value()
		if txt == "" {
			prev := dv.bpmPrev
			if prev < 1 {
				prev = dv.bpm
			}
			dv.SetBPM(prev)
		} else if v, ok := parseBPM(txt); ok {
			dv.SetBPM(v)
		} else {
			dv.bpmErrorAnim = 1
			prev := dv.bpmPrev
			if prev < 1 {
				prev = dv.bpm
			}
			dv.SetBPM(prev)
		}
		dv.bpmBox.SetText(strconv.Itoa(dv.bpm))
	}

	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	totalRows := len(dv.Rows) + 1
	visRows := dv.visibleRows()
	if totalRows > visRows {
		// Only consume wheel for row scrolling when the cursor is over
		// the drum pane (including the scrollbar). This prevents wheel
		// events from being eaten while the cursor is over the grid pane.
		overBar := image.Pt(mx, my).In(dv.scrollBarRect())
		overDrum := image.Pt(mx, my).In(dv.Bounds)
		if overBar || overDrum {
			if steps := wheelScrollSteps(); steps != 0 {
				dv.logger.Infof("[DRUMVIEW] row wheel steps=%d at (%d,%d) rowOffset=%d", steps, mx, my, dv.rowOffset)
				dv.rowOffset -= steps
				if dv.rowOffset < 0 {
					dv.rowOffset = 0
				}
				if dv.rowOffset > totalRows-visRows {
					dv.rowOffset = totalRows - visRows
				}
				dv.calcLayout()
			}
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

	// Subdivision dropdown capture
	if dv.subdivMenuOpen {
		for _, btn := range dv.subdivMenuBtns {
			if btn.Handle(mx, my, left) {
				dv.subdivMenuOpen = false
				return
			}
		}
		if left {
			base := dv.subdivBtn.Rect()
			menu := image.Rect(base.Min.X, base.Max.Y, base.Max.X, base.Max.Y+len(dv.subdivMenuBtns)*dv.rowHeight())
			if !pt(mx, my, base) && !pt(mx, my, menu) {
				dv.subdivMenuOpen = false
			}
			return
		}
		return
	}

	// Color menu should capture input before any other controls when open.
	if dv.colorMenuOpen {
		if !dv.colorHold && isKeyPressed(ebiten.KeyEscape) {
			dv.logger.Debugf("[COLOR] close by Esc")
			dv.colorMenuOpen = false
			return
		}
		// Release colorHold once the mouse is released after opening to avoid
		// immediate close on the same press that opened the menu.
		if dv.colorHold {
			if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
				dv.colorHold = false
				dv.logger.Debugf("[COLOR] hold released; wheel=%v", dv.colorWheelRect)
			}
		}
		// Handle clicks inside the color wheel: pick color at position.
		// Do not pick while colorHold is true (same press that opened the menu).
		if left && !dv.colorHold && image.Pt(mx, my).In(dv.colorWheelRect) {
			col := dv.pickColorFromWheel(mx, my)
			dv.SetRowColor(dv.colorMenuRow, col)
			dv.colorMenuOpen = false
			// Avoid click-through to underlying controls until release.
			SuppressClicksUntilMouseUp()
			dv.logger.Debugf("[COLOR] pick row=%d at=(%d,%d) wheel=%v sel=%s", dv.colorMenuRow, mx, my, dv.colorWheelRect, dv.colorKey(col))
			return
		}
		if left && !dv.colorHold {
			// Click outside the wheel closes (ignore base button; it may be offscreen)
			if !image.Pt(mx, my).In(dv.colorWheelRect) {
				dv.logger.Debugf("[COLOR] close on outside click at=(%d,%d) wheel=%v", mx, my, dv.colorWheelRect)
				dv.colorMenuOpen = false
			}
		}
		return
	}

	stepsRect := image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+timelineHeight, dv.Bounds.Max.X, dv.Bounds.Max.Y)

	// wheel zoom for length adjustment. Apply smoothing so each wheel notch
	// accumulates a fraction of a beat and only commits whole-beat changes
	// when enough deltas have been collected. This avoids abrupt jumps.
	// IMPORTANT: Only read the wheel delta when the cursor is over the steps
	// rectangle. Ebiten's Wheel reports the delta since the previous call, so
	// calling it when the cursor is elsewhere would consume the event and
	// prevent the grid pane from receiving it for camera zoom.
	if pt(mx, my, stepsRect) {
		if dz := wheelZoomDelta(); dz != 0 {
			// Each notch contributes 0.25 beat; adjust if needed.
			sensitivity := 0.25
			dv.zoomAccum += dz * sensitivity
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
						r.CellTypes = make([]model.NodeType, dv.Length)
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
			dv.Rows[dv.activeSlider].Volume = math.Round(s.Value*100) / 100
			dv.logger.Infof("[DRUMVIEW] Row %d volume changed via slider: %.3f", dv.activeSlider, s.Value)
		}
		if !left {
			dv.activeSlider = -1
		}
		return
	}
	for i, s := range dv.rowVolSliders {
		if s.Handle(mx, my, left) {
			dv.Rows[i].Volume = math.Round(s.Value*100) / 100
			dv.logger.Infof("[DRUMVIEW] Row %d volume changed via slider: %.3f", i, s.Value)
			dv.activeSlider = i
			if !left {
				dv.activeSlider = -1
			}
			return
		}
	}

	handled := false
	if dv.mainVolSlider != nil {
		if dv.mainVolSlider.Handle(mx, my, left) {
			audio.SetMainVolume(dv.mainVolSlider.Value)
			handled = true
		}
	}
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
		for _, btn := range dv.rowColorBtns {
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
		if handled && left {
			return
		}
		buttons := []*Button{dv.playBtn, dv.stopBtn, dv.bpmDecBtn, dv.bpmIncBtn, dv.subdivBtn, dv.lenDecBtn, dv.lenIncBtn, dv.trackBtn, dv.addRowBtn, dv.uploadBtn, dv.importBtn, dv.exportBtn}
		for _, btn := range buttons {
			if handled {
				break
			}
			if btn.Handle(mx, my, left) {
				handled = true
			}
		}
	}

	// Single-pass BPM handling already performed above. Apply any +/- delta.
	if dv.bpmDelta != 0 {
		dv.SetBPM(dv.bpm + dv.bpmDelta)
		dv.bpmDelta = 0
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
				r.CellTypes = make([]model.NodeType, dv.Length)
			}
			dv.SetBeatLength(dv.Length) // Update graph's beat length
			dv.bgDirty = true
			dv.markAllRowsDirty()
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
				r.CellTypes = make([]model.NodeType, dv.Length)
			}
			dv.SetBeatLength(dv.Length) // Update graph's beat length
			dv.bgDirty = true
			dv.markAllRowsDirty()
		}
		dv.lenDecPressed = false
	}
	if handled && left {
		return
	}

	// Color menu handled above; nothing here
}

func (dv *DrumView) Draw(dst *ebiten.Image, highlightedBeats map[int]int64, frame int64, beatInfos []model.BeatInfo, elapsedBeats float64) {
	dv.frame++
	dv.rowsLayerBytes = 0
	dv.rowsRepaints = 0
	if len(dv.rowRepaint) != len(dv.Rows) {
		rr := make([]int, len(dv.Rows))
		copy(rr, dv.rowRepaint)
		dv.rowRepaint = rr
	}
	if len(dv.rowFrame) != len(dv.Rows) {
		rf := make([]int64, len(dv.Rows))
		copy(rf, dv.rowFrame)
		dv.rowFrame = rf
	}

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
	dv.subdivBtn.Draw(dst)
	dv.lenDecBtn.Draw(dst)
	dv.lenIncBtn.Draw(dst)
	dv.trackBtn.Draw(dst)
	dv.uploadBtn.Draw(dst)
	dv.importBtn.Draw(dst)
	dv.exportBtn.Draw(dst)
	if dv.mainVolSlider != nil {
		dv.mainVolSlider.Draw(dst)
	}

	// Display sample loading messages near the upload button area.
	if dv.showLoading && dv.samplesTotal > 0 {
		msg := fmt.Sprintf("loading samples... (%d/%d)", dv.samplesLoaded, dv.samplesTotal)
		DrawTextAt(dst, msg, dv.uploadBtn.Rect().Min.X, dv.uploadBtn.Rect().Max.Y+20)
	} else if dv.doneMsgTimer > 0 {
		dv.doneMsgTimer--
		msg := "Finished loading samples"
		DrawTextAt(dst, msg, dv.uploadBtn.Rect().Min.X, dv.uploadBtn.Rect().Max.Y+20)
	}
	// timeline and progress
	// Keep the visible timeline long enough to contain the graph traversal,
	// the current playhead and the visible window. All values expressed in beats.
	if !dv.isPlaying {
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
	}
	lengthBeats := float64(dv.Length) / float64(max1(dv.timelineUnitsPerBeat))
	if !dv.isPlaying {
		if int(math.Ceil(elapsedBeats)+math.Ceil(lengthBeats)) > dv.timelineBeats {
			dv.timelineBeats = int(math.Ceil(elapsedBeats) + math.Ceil(lengthBeats))
		}
	}
	offsetBeats := float64(dv.Offset) / float64(max1(dv.timelineUnitsPerBeat))
	if !dv.isPlaying {
		if int(math.Ceil(offsetBeats+lengthBeats)) > dv.timelineBeats {
			dv.timelineBeats = int(math.Ceil(offsetBeats + lengthBeats))
		}
	}
	totalBeats := dv.timelineBeats
	info := dv.timelineInfoCached(elapsedBeats)
	DrawTextAt(dst, info, dv.timelineRect.Min.X, dv.Bounds.Min.Y+5)
	// Build or reuse timeline base cache (background + beat markers)
	step := 1
	width := dv.timelineRect.Dx()
	if totalBeats > width {
		step = int(math.Ceil(float64(totalBeats) / float64(width)))
	}
	if dv.tlCache == nil || dv.tlCacheW != dv.timelineRect.Dx() || dv.tlCacheH != dv.timelineRect.Dy() || dv.tlCacheBeats != totalBeats || dv.tlCacheStep != step {
		dv.tlCache = ebiten.NewImage(dv.timelineRect.Dx(), dv.timelineRect.Dy())
		dv.tlCacheW, dv.tlCacheH = dv.timelineRect.Dx(), dv.timelineRect.Dy()
		dv.tlCacheBeats, dv.tlCacheStep = totalBeats, step
		// Fill background
		drawRect(dv.tlCache, image.Rect(0, 0, dv.tlCacheW, dv.tlCacheH), colTimelineTotal, true)
		// Beat markers (decimated)
		prevX := -1
		for i := 0; i <= totalBeats; i += step {
			x := int(float64(i) / float64(totalBeats) * float64(dv.tlCacheW))
			if x != prevX {
				drawRect(dv.tlCache, image.Rect(x, 0, x+1, dv.tlCacheH), colTimelineBeat, true)
				prevX = x
			}
		}
	}
	if dv.tlCache != nil {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(dv.timelineRect.Min.X), float64(dv.timelineRect.Min.Y))
		dst.DrawImage(dv.tlCache, &op)
	} else {
		drawRect(dst, dv.timelineRect, colTimelineTotal, true)
	}

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

	// (beat markers are baked into tlCache)

	// current playback cursor (beats)
	cursorX := dv.timelineRect.Min.X + int((elapsedBeats/float64(totalBeats))*float64(dv.timelineRect.Dx()))
	cursorRect := image.Rect(cursorX-1, dv.timelineRect.Min.Y, cursorX+1, dv.timelineRect.Max.Y)
	drawRect(dst, cursorRect, colTimelineCursor, true)

	drawRect(dst, dv.timelineRect, colButtonBorder, false)

	// Build and draw rows composite layer (static rows without highlights)
	dv.ensureRowCache()
	dv.rowsLayerMaybeRebuild()
	if dv.rowsLayer != nil {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
		dst.DrawImage(dv.rowsLayer, &op)
	}

	// draw steps (rows) – dynamic overlays and controls
	vis := dv.visibleRows()
	for i, r := range dv.Rows {
		if len(r.Steps) != dv.Length {
			r.Steps = make([]bool, dv.Length)
			r.CellTypes = make([]model.NodeType, dv.Length)
		}
		if i < dv.rowOffset || i >= dv.rowOffset+vis {
			continue
		}
		y := dv.Bounds.Min.Y + timelineHeight + (i-dv.rowOffset)*dv.rowHeight()
		// Always draw label/edit/color buttons even during rename; the rename
		// TextInput overlays the label area without removing other controls.
		if i < len(dv.rowLabels) {
			dv.rowLabels[i].Draw(dst)
		}
		if i < len(dv.rowEditBtns) {
			dv.rowEditBtns[i].Draw(dst)
		}
		if i < len(dv.rowColorBtns) {
			dv.rowColorBtns[i].Draw(dst)
		}
		dv.rowVolSliders[i].Draw(dst)
		dv.rowMuteBtns[i].pressed = dv.Rows[i].Muted
		dv.rowSoloBtns[i].pressed = dv.Rows[i].Solo
		dv.rowMuteBtns[i].Draw(dst)
		dv.rowSoloBtns[i].Draw(dst)
		dv.rowOriginBtns[i].Draw(dst)
		dv.rowDeleteBtns[i].Draw(dst)
		// Row highlights are drawn dynamically below.
		// Overlay highlights for this row only
		n := len(r.Steps)
		if n > 0 {
			startX := dv.timelineRect.Min.X
			totalW := dv.timelineRect.Dx()
			for key, val := range highlightedBeats {
				row, idx := splitBeatKey(key)
				if row != i {
					continue
				}
				j := idx - dv.Offset
				if j < 0 || j >= n {
					continue
				}
				x0 := startX + (j*totalW)/n
				x1 := startX + ((j+1)*totalW)/n
				if x1 <= x0 {
					x1 = x0 + 1
				}
				if dv.simpleDraw {
					dv.ensureHighlightSprites()
					spr := dv.hlSpriteReg
					if isMuteHighlight(val) {
						spr = dv.hlSpriteMute
					}
					if spr != nil {
						var hop ebiten.DrawImageOptions
						sx := float64(x0)
						sy := float64(y)
						w := float64(x1 - x0)
						hop.GeoM.Scale(w/float64(spr.Bounds().Dx()), 1)
						hop.GeoM.Translate(sx, sy)
						dst.DrawImage(spr, &hop)
					}
				} else {
					rect := image.Rect(x0, y, x1, y+dv.rowHeight())
					if isMuteHighlight(val) {
						drawRect(dst, rect, colMuteHighlight, true)
						drawRect(dst, rect, DrumCellUI.Border, false)
					} else {
						DrumCellUI.Draw(dst, rect, r.Steps[j], true, r.Color)
					}
				}
			}
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
	if dv.subdivMenuOpen {
		for _, btn := range dv.subdivMenuBtns {
			btn.Draw(dst)
		}
	}
	if dv.colorMenuOpen {
		// Draw cached color wheel image (built once per open/resize)
		r := dv.colorWheelRect
		if !r.Empty() {
			if dv.colorWheelImg == nil || dv.wheelCacheW != r.Dx() || dv.wheelCacheH != r.Dy() {
				dv.rebuildColorWheelImage()
			}
			if dv.colorWheelImg != nil {
				var op ebiten.DrawImageOptions
				op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
				dst.DrawImage(dv.colorWheelImg, &op)
				drawRect(dst, r, colButtonBorder, false)
			}
		}
	}

	if dv.uploading {
		DrawTextAt(dst, "Loading...", dv.uploadBtn.Rect().Min.X, dv.uploadBtn.Rect().Max.Y+20)
	}
	if dv.naming {
		box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
		if dv.nameBox == nil {
			dv.nameBox = NewTextInput(box, BPMBoxStyle)
		}
		dv.nameBox.Rect = box
		dv.nameBox.Draw(dst)
		dv.saveBtn.Draw(dst)
	}
	if dv.logger != nil && dv.rowsLayerFrame == dv.frame {
		kb := float64(dv.rowsLayerBytes) / 1024.0
		dv.logger.Debugf("[DRUMVIEW PERF] frame=%d repaints=%d layerKB=%.2f", dv.frame, dv.rowsRepaints, kb)
	}
}

func (dv *DrumView) ensureHighlightSprites() {
	h := dv.rowHeight()
	if h <= 0 {
		h = 1
	}
	if dv.hlSpriteReg != nil && dv.hlSpriteH == h {
		return
	}
	// Base regular highlight: semi-transparent overlay
	reg := ebiten.NewImage(1, h)
	drawRect(reg, image.Rect(0, 0, 1, h), fadeColor(colHighlight, 0.5), true)
	// Mute highlight: use existing colMuteHighlight
	mute := ebiten.NewImage(1, h)
	drawRect(mute, image.Rect(0, 0, 1, h), colMuteHighlight, true)
	dv.hlSpriteReg = reg
	dv.hlSpriteMute = mute
	dv.hlSpriteH = h
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

// timelineInfoCached caches the last formatted timeline info string and only
// re-renders when milliseconds change. This avoids per-frame allocations.
func (dv *DrumView) timelineInfoCached(elapsedBeats float64) string {
	totalBeats := math.Max(float64(dv.timelineBeats), elapsedBeats)
	curMS := int(math.Round(elapsedBeats * dv.secPerBeat * 1000.0))
	totMS := int(math.Round(totalBeats * dv.secPerBeat * 1000.0))
	// Optional throttling on web builds to reduce per-frame text churn.
	if timelineInfoThrottleMS > 0 {
		thr := timelineInfoThrottleMS
		if (curMS/thr) == (dv.lastInfoCurMS/thr) && (totMS/thr) == (dv.lastInfoTotMS/thr) && dv.lastInfoText != "" {
			return dv.lastInfoText
		}
	} else {
		if curMS == dv.lastInfoCurMS && totMS == dv.lastInfoTotMS && dv.lastInfoText != "" {
			return dv.lastInfoText
		}
	}
	curS, curMs := curMS/1000, curMS%1000
	totS, totMs := totMS/1000, totMS%1000
	dv.lastInfoCurMS = curMS
	dv.lastInfoTotMS = totMS
	dv.lastInfoText = fmt.Sprintf("Beat %.3f/%.3f Time %ds %dms/%ds %dms", elapsedBeats, totalBeats, curS, curMs, totS, totMs)
	return dv.lastInfoText
}

// max1 returns at least 1 to avoid division by zero for unit conversions.
func max1(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// pickColorFromWheel maps a screen coordinate to a color in the wheel rectangle.
// Hue is angle, saturation is radius; value is fixed at 1. Alpha is 255.
func (dv *DrumView) pickColorFromWheel(x, y int) color.Color {
	r := dv.colorWheelRect
	if r.Empty() {
		return color.RGBA{200, 200, 200, 255}
	}
	// Map point to [-1,1] range centered in rect
	cx := float64(r.Min.X + r.Dx()/2)
	cy := float64(r.Min.Y + r.Dy()/2)
	rx := float64(x) - cx
	ry := float64(y) - cy
	radius := float64(imin(r.Dx(), r.Dy())) / 2
	if radius <= 0 {
		return color.RGBA{200, 200, 200, 255}
	}
	// Normalize radius to [0,1]
	rnorm := math.Hypot(rx, ry) / radius
	if rnorm > 1 {
		rnorm = 1
	}
	// Hue from angle in [0,1)
	h := math.Atan2(ry, rx) // [-pi, pi]
	if h < 0 {
		h += 2 * math.Pi
	}
	h /= 2 * math.Pi
	// Two-zone mapping for broader gamut:
	// Inner half: saturated darks (s=1, v in [0..1])
	// Outer half: bright pastels to saturated (v=1, s in [0..1]) with white at the seam
	var s, v float64
	if rnorm < 0.5 {
		s = 1
		v = rnorm / 0.5 // 0..1
	} else {
		s = (rnorm - 0.5) / 0.5 // 0..1
		v = 1
	}
	return hsvToRGBA(h, s, v)
}

func hsvToRGBA(h, s, v float64) color.Color {
	if s <= 0 {
		c := uint8(clamp(int(v*255), 0, 255))
		return color.RGBA{c, c, c, 255}
	}
	h6 := h * 6
	i := int(math.Floor(h6))
	f := h6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(clamp(int(r*255), 0, 255)), uint8(clamp(int(g*255), 0, 255)), uint8(clamp(int(b*255), 0, 255)), 255}
}

// rebuildColorWheelImage regenerates the cached wheel image for the current rect.
func (dv *DrumView) rebuildColorWheelImage() {
	r := dv.colorWheelRect
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		dv.colorWheelImg = nil
		dv.wheelCacheW, dv.wheelCacheH = 0, 0
		return
	}
	dv.logger.Debugf("[COLOR] rebuild wheel image %dx%d at=(%d,%d)", w, h, r.Min.X, r.Min.Y)
	// Build an RGBA buffer for speed, then upload to ebiten
	buf := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := dv.pickColorFromWheel(r.Min.X+x, r.Min.Y+y)
			rr, gg, bb, aa := color.RGBAModel.Convert(c).(color.RGBA).RGBA()
			buf.SetRGBA(x, y, color.RGBA{uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8), uint8(aa >> 8)})
		}
	}
	dv.colorWheelImg = ebiten.NewImageFromImage(buf)
	dv.wheelCacheW, dv.wheelCacheH = w, h
}

// colorKey returns a canonical string key for a color.
func (dv *DrumView) colorKey(c color.Color) string {
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("%02X%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}

// isColorUsed reports whether the color is used by any row except excludeIdx.
func (dv *DrumView) isColorUsed(c color.Color, excludeIdx int) bool {
	key := dv.colorKey(c)
	for i, r := range dv.Rows {
		if i == excludeIdx {
			continue
		}
		if dv.colorKey(r.Color) == key {
			return true
		}
	}
	return false
}

// generatedColor derives a pseudo-random but deterministic vivid color from a seed.
func (dv *DrumView) generatedColor(seed int) color.Color {
	h := uint32(seed) * 2654435761
	r := uint8((h >> 16) & 0xFF)
	g := uint8((h >> 8) & 0xFF)
	b := uint8(h & 0xFF)
	// Ensure minimum brightness.
	if int(r)+int(g)+int(b) < 200 {
		r = r/2 + 60
		g = g/2 + 60
		b = b/2 + 60
	}
	return color.RGBA{r, g, b, 255}
}

// ensureUniqueColor adjusts base to avoid conflicts with other rows.
func (dv *DrumView) ensureUniqueColor(base color.Color, idx int) color.Color {
	if !dv.isColorUsed(base, idx) {
		return base
	}
	// Try light/dark adjustments
	for d := 20; d <= 120; d += 20 {
		for _, s := range []int{+1, -1} {
			c := adjustColor(base, s*d)
			if !dv.isColorUsed(c, idx) {
				return c
			}
		}
	}
	// Try palette fallbacks
	for _, c := range instColors {
		if !dv.isColorUsed(c, idx) {
			return c
		}
	}
	for _, c := range customPalette {
		if !dv.isColorUsed(c, idx) {
			return c
		}
	}
	// Generate until unique
	for i := 0; i < 256; i++ {
		c := dv.generatedColor(len(dv.Rows) + 1 + i*7)
		if !dv.isColorUsed(c, idx) {
			return c
		}
	}
	// Fallback to white (unlikely)
	return color.RGBA{255, 255, 255, 255}
}

// parseHexRGB parses #RRGGBB or #RGB and returns the color if valid.
func (dv *DrumView) parseHexRGB(s string) (color.Color, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) == 6 {
		var r, g, b uint8
		if _, err := fmt.Sscanf(s, "%02X%02X%02X", &r, &g, &b); err == nil {
			return color.RGBA{r, g, b, 255}, true
		}
	}
	if len(s) == 3 {
		var r, g, b uint8
		if _, err := fmt.Sscanf(s, "%1X%1X%1X", &r, &g, &b); err == nil {
			// expand 4-bit to 8-bit by duplication (e.g., A -> AA)
			r = r * 17
			g = g * 17
			b = b * 17
			return color.RGBA{r, g, b, 255}, true
		}
	}
	return color.RGBA{255, 255, 255, 255}, false
}

// SetRowColor sets the color for a row ensuring uniqueness across rows.
func (dv *DrumView) SetRowColor(idx int, c color.Color) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	dv.Rows[idx].Color = dv.ensureUniqueColor(c, idx)
	// Mark the row dirty so cache picks up new color.
	if idx >= 0 && idx < len(dv.rowDirty) {
		dv.rowDirty[idx] = true
		if idx < len(dv.rowFullDirty) {
			dv.rowFullDirty[idx] = true
		}
	}
}

// EnsureUniqueRowColors scans all rows and adjusts any duplicates to unique variants.
func (dv *DrumView) EnsureUniqueRowColors() {
	for i := range dv.Rows {
		dv.Rows[i].Color = dv.ensureUniqueColor(dv.Rows[i].Color, i)
	}
	dv.markAllRowsDirty()
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
	if dv.mainVolSlider != nil {
		dv.mainVolSlider.SetRect(dv.mainVolRect)
	}
	// Ensure we have widgets for each row. This is a safety net for callers
	// that may have changed the number of rows without invoking calcLayout yet.
	if len(dv.rowLabels) != len(dv.Rows) ||
		len(dv.rowEditBtns) != len(dv.Rows) ||
		len(dv.rowColorBtns) != len(dv.Rows) ||
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
		g := NewGridLayout(rowRect, []float64{6, 2, 2, 5, 2, 2, 2, 2}, []float64{1})
		if i < len(dv.rowLabels) {
			dv.rowLabels[i].SetRect(insetRect(g.Cell(0, 0), buttonPad))
		}
		if i < len(dv.rowEditBtns) {
			dv.rowEditBtns[i].SetRect(insetRect(g.Cell(1, 0), buttonPad))
		}
		if i < len(dv.rowColorBtns) {
			dv.rowColorBtns[i].SetRect(insetRect(g.Cell(2, 0), buttonPad))
		}
		if i < len(dv.rowVolSliders) {
			dv.rowVolSliders[i].SetRect(insetRect(g.Cell(3, 0), buttonPad))
		}
		if i < len(dv.rowMuteBtns) {
			dv.rowMuteBtns[i].SetRect(insetRect(g.Cell(4, 0), buttonPad))
		}
		if i < len(dv.rowSoloBtns) {
			dv.rowSoloBtns[i].SetRect(insetRect(g.Cell(5, 0), buttonPad))
		}
		if i < len(dv.rowOriginBtns) {
			dv.rowOriginBtns[i].SetRect(insetRect(g.Cell(6, 0), buttonPad))
		}
		if i < len(dv.rowDeleteBtns) {
			dv.rowDeleteBtns[i].SetRect(insetRect(g.Cell(7, 0), buttonPad))
		}
	}
	// Update trailing "+" button position
	y := dv.Bounds.Min.Y + timelineHeight + (len(dv.Rows)-dv.rowOffset)*dv.rowHeight()
	dv.addRowBtn.SetRect(insetRect(image.Rect(dv.Bounds.Min.X, y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, y+dv.rowHeight()), buttonPad))
}

// --- Row cache helpers ---

func (dv *DrumView) ensureRowCache() {
	if len(dv.rowCache) != len(dv.Rows) {
		dv.rowCache = make([]*ebiten.Image, len(dv.Rows))
		dv.rowDirty = make([]bool, len(dv.Rows))
		dv.rowFullDirty = make([]bool, len(dv.Rows))
		for i := range dv.rowDirty {
			dv.rowDirty[i] = true
		}
		for i := range dv.rowFullDirty {
			dv.rowFullDirty[i] = true
		}
		dv.rowCacheOff = make([]int, len(dv.Rows))
		dv.rowCacheGen = make([]int, len(dv.Rows))
		for i := range dv.rowCacheOff {
			dv.rowCacheOff[i] = dv.Offset
		}
	}
	if len(dv.rowFrame) != len(dv.Rows) {
		rf := make([]int64, len(dv.Rows))
		copy(rf, dv.rowFrame)
		dv.rowFrame = rf
	}
	if len(dv.rowRepaint) != len(dv.Rows) {
		rr := make([]int, len(dv.Rows))
		copy(rr, dv.rowRepaint)
		dv.rowRepaint = rr
	}
}

func (dv *DrumView) markAllRowsDirty() {
	dv.ensureRowCache()
	for i := range dv.rowDirty {
		dv.rowDirty[i] = true
	}
	for i := range dv.rowFullDirty {
		dv.rowFullDirty[i] = true
	}
	dv.rowsLayerDirty = true
}

// markRowsShiftDirty invalidates cached sprites for offset shifts while
// allowing incremental reuse (no forced full rebuild).
func (dv *DrumView) markRowsShiftDirty() {
	dv.ensureRowCache()
	for i := range dv.rowDirty {
		dv.rowDirty[i] = true
	}
	dv.rowsLayerDirty = true
}

// markRowDirty invalidates the cached sprite for a single row. Safe for
// concurrent callers on the UI thread (Game.Update/Draw) which is the only
// place DrumView is mutated.
func (dv *DrumView) markRowDirty(i int) {
	dv.ensureRowCache()
	if i >= 0 && i < len(dv.rowDirty) {
		dv.rowDirty[i] = true
		if i < len(dv.rowFullDirty) {
			dv.rowFullDirty[i] = true
		}
	}
	dv.rowsLayerDirty = true
}

func (dv *DrumView) invalidateRowCaches() {
	dv.rowCacheW = 0
	dv.rowCacheH = 0
	dv.markAllRowsDirty()
	dv.rowsLayerDirty = true
}

func (dv *DrumView) needsRowRebuild(i int) bool {
	if i < 0 || i >= len(dv.Rows) {
		return false
	}
	if len(dv.rowCache) != len(dv.Rows) || len(dv.rowDirty) != len(dv.Rows) {
		return true
	}
	if len(dv.rowFullDirty) != len(dv.Rows) {
		return true
	}
	if dv.rowDirty[i] {
		return true
	}
	if i < len(dv.rowFullDirty) && dv.rowFullDirty[i] {
		return true
	}
	if dv.rowCacheW != dv.timelineRect.Dx() || dv.rowCacheH != dv.rowHeight() {
		return true
	}
	if dv.rowCacheLen != dv.Length {
		return true
	}
	if dv.rowCache[i] == nil {
		return true
	}
	return false
}

func (dv *DrumView) buildRowSprite(i int) {
	if i < 0 || i >= len(dv.Rows) {
		return
	}
	w := dv.timelineRect.Dx()
	h := dv.rowHeight()
	if w <= 0 || h <= 0 {
		return
	}
	n := len(dv.Rows[i].Steps)
	if n < 1 {
		dv.rowCache[i] = ebiten.NewImage(w, h)
		dv.rowCacheLen = dv.Length
		dv.rowCacheW, dv.rowCacheH = w, h
		dv.rowCacheOff[i] = dv.Offset
		return
	}
	fullRebuild := false
	if i >= 0 && i < len(dv.rowFullDirty) {
		fullRebuild = dv.rowFullDirty[i]
	}
	// Attempt incremental reuse on small offset shifts.
	if !fullRebuild && dv.rowCache[i] != nil && dv.rowCacheW == w && dv.rowCacheH == h && dv.rowCacheLen == n {
		// Pixel shift for the new offset relative to the cached one.
		dxPx := int(math.Round(float64(dv.Offset-dv.rowCacheOff[i]) * float64(w) / float64(n)))
		if dxPx != 0 && abs(dxPx) <= max1(dv.rowCachePadPx) {
			// Shift existing content and redraw only the uncovered strip.
			newImg := ebiten.NewImage(w, h)
			var op ebiten.DrawImageOptions
			op.GeoM.Translate(float64(-dxPx), 0)
			newImg.DrawImage(dv.rowCache[i], &op)
			// Redraw newly revealed region at one side.
			start := 0
			end := 0
			if dxPx > 0 {
				start, end = w-dxPx, w
			} else {
				start, end = 0, -dxPx
			}
			onCol := dv.Rows[i].Color
			for j, on := range dv.Rows[i].Steps {
				x0 := (j * w) / n
				x1 := ((j + 1) * w) / n
				if x1 <= x0 {
					x1 = x0 + 1
				}
				if x1 <= start || x0 >= end {
					continue
				}
				rect := image.Rect(x0, 0, x1, h)
				cellType := model.NodeTypeRegular
				if j < len(dv.Rows[i].CellTypes) {
					cellType = dv.Rows[i].CellTypes[j]
				}
				fillCol := onCol
				if cellType == model.NodeTypeMute {
					fillCol = colMuteCell
				}
				DrumCellUI.Draw(newImg, rect, on, false, fillCol)
			}
			dv.rowCache[i] = newImg
			dv.rowCacheLen = dv.Length
			dv.rowCacheW, dv.rowCacheH = w, h
			dv.rowCacheOff[i] = dv.Offset
			// Generation unchanged on incremental update.
			return
		}
	}
	img := ebiten.NewImage(w, h)
	if n <= w {
		// Full-resolution cells.
		onCol := dv.Rows[i].Color
		for j, on := range dv.Rows[i].Steps {
			x0 := (j * w) / n
			x1 := ((j + 1) * w) / n
			if x1 <= x0 {
				x1 = x0 + 1
			}
			rect := image.Rect(x0, 0, x1, h)
			cellType := model.NodeTypeRegular
			if j < len(dv.Rows[i].CellTypes) {
				cellType = dv.Rows[i].CellTypes[j]
			}
			fillCol := onCol
			if cellType == model.NodeTypeMute {
				fillCol = colMuteCell
			}
			DrumCellUI.Draw(img, rect, on, false, fillCol)
		}
	}

	// When zoomed out (more steps than pixels), bake decimated marker ticks
	// into the row sprite to avoid per-frame draws.
	if n > w && n > 0 {
		step := int(math.Ceil(float64(n) / float64(w)))
		if step < 1 {
			step = 1
		}
		prevX := -1
		for j := 0; j <= n; j += step {
			x := (j * w) / n
			if x != prevX {
				drawRect(img, image.Rect(x, 0, x+1, h), colTimelineBeat, true)
				prevX = x
			}
		}
	}
	dv.rowCache[i] = img
	dv.rowCacheLen = dv.Length
	dv.rowCacheW, dv.rowCacheH = w, h
	dv.rowCacheOff[i] = dv.Offset
	if i < len(dv.rowCacheGen) {
		dv.rowCacheGen[i]++
	}
	if i < len(dv.rowFrame) {
		dv.rowFrame[i] = dv.frame
	}
	if i < len(dv.rowRepaint) {
		dv.rowRepaint[i]++
	}
}

// rowsLayerMaybeRebuild composes all visible row sprites into a cached layer
// image. It excludes dynamic overlays (highlights) and UI controls.
func (dv *DrumView) rowsLayerMaybeRebuild() {
	w := dv.Bounds.Dx()
	h := dv.Bounds.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	// Invalidate on size changes, offset/scroll changes, or explicit dirty flag.
	need := dv.rowsLayer == nil || dv.rowsLayerW != w || dv.rowsLayerH != h || dv.rowsLayerOffset != dv.Offset || dv.rowsLayerRowOff != dv.rowOffset || dv.rowsLayerDirty
	// If any row needs rebuild, ensure we rebuild row sprites first and mark layer dirty.
	vis := dv.visibleRows()
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		if dv.needsRowRebuild(i) {
			dv.buildRowSprite(i)
			if i < len(dv.rowDirty) {
				dv.rowDirty[i] = false
			}
			if i < len(dv.rowFullDirty) {
				dv.rowFullDirty[i] = false
			}
			need = true
		}
	}
	if !need {
		return
	}
	img := ebiten.NewImage(w, h)
	// Draw each visible row sprite at its position inside dv.Bounds.
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		if i < 0 || i >= len(dv.rowCache) || dv.rowCache[i] == nil {
			continue
		}
		y := timelineHeight + (i-dv.rowOffset)*dv.rowHeight()
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(dv.timelineRect.Min.X-dv.Bounds.Min.X), float64(y))
		img.DrawImage(dv.rowCache[i], &op)
		steps := len(dv.Rows[i].Steps)
		rowWidth := dv.timelineRect.Dx()
		dv.rowsRepaints++
		dv.rowsLayerBytes += int64(rowWidth * dv.rowHeight() * 4)
		if steps > rowWidth && steps > 0 {
			step := int(math.Ceil(float64(steps) / float64(rowWidth)))
			if step < 1 {
				step = 1
			}
			prevX := -1
			baseX := dv.timelineRect.Min.X - dv.Bounds.Min.X
			rowH := dv.rowHeight()
			for j := 0; j <= steps; j += step {
				x := baseX + (j*rowWidth)/steps
				if x != prevX {
					drawRect(img, image.Rect(x, y, x+1, y+rowH), colTimelineBeat, true)
					prevX = x
				}
			}
		}
	}
	dv.rowsLayer = img
	dv.rowsLayerW, dv.rowsLayerH = w, h
	dv.rowsLayerOffset = dv.Offset
	dv.rowsLayerRowOff = dv.rowOffset
	dv.rowsLayerGen++
	dv.rowsLayerDirty = false
	dv.rowsLayerFrame = dv.frame
}
