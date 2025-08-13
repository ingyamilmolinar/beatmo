package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
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
	timelineHeight    = 25
)

/* ───────────────────────────────────────────────────────────── */

type DrumRow struct {
	Name       string
	Instrument string
	Steps      []bool
	Color      color.Color
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

	bgDirty bool
	bgCache []*ebiten.Image

	instBtn     image.Rectangle
	uploadBtn   image.Rectangle
	instOptions []string

	uploading  bool
	uploadCh   chan uploadResult
	pendingWAV string
	naming     bool
	nameInput  string
	saveBtn    image.Rectangle

	timelineRect image.Rectangle // progress bar for fast seek

	// ui widgets (re-computed every frame)
	playBtn   image.Rectangle
	stopBtn   image.Rectangle
	bpmDecBtn image.Rectangle // Decrease BPM
	bpmBox    image.Rectangle
	bpmIncBtn image.Rectangle // Increase BPM
	lenDecBtn image.Rectangle // Decrease length
	lenIncBtn image.Rectangle // Increase length

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

	// button animations
	playAnim     float64
	stopAnim     float64
	bpmDecAnim   float64
	bpmIncAnim   float64
	lenDecAnim   float64
	lenIncAnim   float64
	instAnim     float64
	uploadAnim   float64
	bpmFocusAnim float64
	saveAnim     float64
	namePhase    float64
	savePressed  bool

	// window scrolling
	Offset        int // index of first visible beat
	dragging      bool
	dragStartX    int
	startOffset   int
	offsetChanged bool

	scrubbing     bool
	seekBeat      int
	seekRequested bool
}

/* ─── geometry helpers ─────────────────────────────────────── */

func (dv *DrumView) rowHeight() int {
	rows := len(dv.Rows)
	if rows == 0 {
		return 0
	}
	return (dv.Bounds.Dy() - timelineHeight) / rows
}

// SetBeatLength sets the beat length in the underlying graph.
func (dv *DrumView) SetBeatLength(length int) {
	if dv.Graph != nil {
		dv.Graph.SetBeatLength(length)
		dv.logger.Debugf("[DRUMVIEW] Graph beat length set to: %d", length)
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
		Bounds:      b,
		labelW:      80,
		bpm:         120,
		bgDirty:     true,
		Graph:       g,
		logger:      logger,
		Length:      8, // Default length
		Offset:      0,
		instOptions: opts,
		uploadCh:    make(chan uploadResult, 1),
	}
	dv.Rows = []*DrumRow{{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), Color: colStep}}
	dv.SetBeatLength(dv.Length) // Initialize graph's beat length
	return dv
}

// SetBounds is called from Game whenever the splitter moves or the window
// resizes; it invalidates the cached background so dimensions update next draw.
func (dv *DrumView) SetBounds(b image.Rectangle) {
	if dv.Bounds != b {
		dv.Bounds = b
		dv.bgDirty = true
	}
}

/* ─── public update ────────────────────────────────────────── */

func (dv *DrumView) recalcButtons() {
	dv.playBtn = image.Rect(10, dv.Bounds.Min.Y+10, 90, dv.Bounds.Min.Y+50)
	dv.stopBtn = image.Rect(100, dv.Bounds.Min.Y+10, 180, dv.Bounds.Min.Y+50)
	dv.bpmDecBtn = image.Rect(190, dv.Bounds.Min.Y+10, 230, dv.Bounds.Min.Y+50)
	dv.bpmBox = image.Rect(235, dv.Bounds.Min.Y+10, 275, dv.Bounds.Min.Y+50)
	dv.bpmIncBtn = image.Rect(280, dv.Bounds.Min.Y+10, 320, dv.Bounds.Min.Y+50)
	dv.lenDecBtn = image.Rect(325, dv.Bounds.Min.Y+10, 365, dv.Bounds.Min.Y+50)
	dv.lenIncBtn = image.Rect(370, dv.Bounds.Min.Y+10, 410, dv.Bounds.Min.Y+50)
	dv.instBtn = image.Rect(10, dv.Bounds.Min.Y+60, 150, dv.Bounds.Min.Y+100)
	dv.uploadBtn = image.Rect(160, dv.Bounds.Min.Y+60, 300, dv.Bounds.Min.Y+100)
	dv.controlsW = dv.lenIncBtn.Max.X
	dv.timelineRect = image.Rect(
		dv.Bounds.Min.X+dv.labelW+dv.controlsW,
		dv.Bounds.Min.Y+5,
		dv.Bounds.Max.X-10,
		dv.Bounds.Min.Y+15,
	)
}

func (dv *DrumView) calcLayout() {
	if len(dv.Rows) > 0 {
		dv.cell = (dv.Bounds.Dx() - dv.labelW - dv.controlsW) / len(dv.Rows[0].Steps) // Leave space for buttons
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
	dv.bpm = b
}

func (dv *DrumView) OffsetChanged() bool {
	if dv.offsetChanged {
		dv.offsetChanged = false
		return true
	}
	return false
}

func (dv *DrumView) SeekRequested() bool { return dv.seekRequested }

func (dv *DrumView) SeekBeat() int { return dv.seekBeat }

func (dv *DrumView) ClearSeek() { dv.seekRequested = false }

func (dv *DrumView) SetLength(length int) {
	if length < 1 {
		length = 1
	}
	dv.Length = length
	dv.Rows[0].Steps = make([]bool, dv.Length)
	dv.SetBeatLength(dv.Length)
	dv.bgDirty = true
}

func (dv *DrumView) SetInstrument(id string) {
	dv.Rows[0].Instrument = id
	if id != "" {
		dv.Rows[0].Name = strings.ToUpper(id[:1]) + id[1:]
	}
}

func (dv *DrumView) AddInstrument(id string) {
	dv.instOptions = audio.Instruments()
	dv.SetInstrument(id)
}

func (dv *DrumView) CycleInstrument() {
	if len(dv.instOptions) == 0 {
		return
	}
	cur := dv.Rows[0].Instrument
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
	decay(&dv.instAnim)
	decay(&dv.uploadAnim)
	decay(&dv.bpmFocusAnim)
	decay(&dv.saveAnim)
}

func (dv *DrumView) Update() {
	if len(dv.Rows) == 0 {
		return
	}

	if dv.uploading {
		select {
		case res := <-dv.uploadCh:
			dv.uploading = false
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
		dv.saveBtn = image.Rect(box.Max.X+10, box.Min.Y, box.Max.X+50, box.Max.Y)
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
		if left && !dv.savePressed && pt(mx, my, dv.saveBtn) {
			dv.savePressed = true
			dv.saveAnim = 1
		}
		if !left && dv.savePressed {
			dv.savePressed = false
			if pt(mx, my, dv.saveBtn) {
				id := strings.TrimSpace(dv.nameInput)
				if id != "" {
					dv.registerInstrument(id)
				}
			}
		}
		dv.namePhase += 0.1
		return
	}

	dv.recalcButtons()
	dv.calcLayout()

	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	stepsRect := image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+timelineHeight, dv.Bounds.Max.X, dv.Bounds.Max.Y)

	// wheel zoom for length adjustment
	if _, whY := wheel(); whY != 0 {
		if pt(mx, my, stepsRect) {
			if whY > 0 && dv.Length < 64 {
				dv.Length++
				dv.Rows[0].Steps = make([]bool, dv.Length)
				dv.SetBeatLength(dv.Length)
				dv.bgDirty = true
				dv.logger.Infof("[DRUMVIEW] Length increased to: %d via wheel", dv.Length)
			}
			if whY < 0 && dv.Length > 1 {
				dv.Length--
				dv.Rows[0].Steps = make([]bool, dv.Length)
				dv.SetBeatLength(dv.Length)
				dv.bgDirty = true
				dv.logger.Infof("[DRUMVIEW] Length decreased to: %d via wheel", dv.Length)
			}
		}
	}

	/* ——— widget clicks & dragging ——— */
	if left {
		switch {
		case pt(mx, my, dv.playBtn):
			dv.playPressed = true
			dv.playAnim = 1
			dv.logger.Infof("[DRUMVIEW] Play button pressed.")
		case pt(mx, my, dv.stopBtn):
			dv.stopPressed = true
			dv.stopAnim = 1
			dv.logger.Infof("[DRUMVIEW] Stop button pressed.")
		case pt(mx, my, dv.bpmDecBtn):
			dv.bpmDecPressed = true
			dv.bpmDecAnim = 1
			dv.logger.Infof("[DRUMVIEW] BPM decrease button pressed.")
		case pt(mx, my, dv.bpmIncBtn):
			dv.bpmIncPressed = true
			dv.bpmIncAnim = 1
			dv.logger.Infof("[DRUMVIEW] BPM increase button pressed.")
		case pt(mx, my, dv.bpmBox):
			if !dv.focusBPM {
				dv.focusBPM = true
				dv.bpmFocusAnim = 1
				dv.logger.Debugf("[DRUMVIEW] BPM box clicked. focusingBPM: %t", dv.focusBPM)
			}
		case pt(mx, my, dv.lenIncBtn):
			dv.lenIncPressed = true
			dv.lenIncAnim = 1
			dv.logger.Infof("[DRUMVIEW] Length increase button pressed.")
		case pt(mx, my, dv.lenDecBtn):
			dv.lenDecPressed = true
			dv.lenDecAnim = 1
			dv.logger.Infof("[DRUMVIEW] Length decrease button pressed.")
		case pt(mx, my, dv.instBtn):
			if len(dv.instOptions) > 0 {
				cur := dv.Rows[0].Instrument
				for i, id := range dv.instOptions {
					if id == cur {
						next := dv.instOptions[(i+1)%len(dv.instOptions)]
						dv.Rows[0].Instrument = next
						dv.Rows[0].Name = strings.ToUpper(next[:1]) + next[1:]
						break
					}
				}
				dv.instAnim = 1
			}
		case pt(mx, my, dv.uploadBtn):
			if !dv.uploading && !dv.naming {
				dv.uploadAnim = 1
				dv.uploading = true
				go func() {
					path, err := audio.SelectWAV()
					dv.uploadCh <- uploadResult{path: path, err: err}
				}()
			}
		default:
			if pt(mx, my, stepsRect) {
				if !dv.dragging {
					dv.dragging = true
					dv.dragStartX = mx
					dv.startOffset = dv.Offset
				}
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
		totalBeats := dv.Length
		beat := int(float64(pos-dv.timelineRect.Min.X) / float64(dv.timelineRect.Dx()) * float64(totalBeats))
		if beat < 0 {
			beat = 0
		}
		if beat != dv.Offset {
			dv.Offset = beat
			dv.offsetChanged = true
		}
		dv.seekBeat = beat
		if !left {
			dv.scrubbing = false
			dv.seekRequested = true
		}
	}

	/* ——— BPM editing ——— */
	if dv.focusBPM {
		for _, r := range inputChars() {
			if r >= '0' && r <= '9' {
				val, _ := strconv.Atoi(string(r))
				newBPM := dv.bpm*10 + val
				if newBPM != dv.bpm {
					dv.bpm = newBPM
					dv.logger.Debugf("[DRUMVIEW] BPM changed to: %d", dv.bpm)
				}
			}
		}
		if isKeyPressed(ebiten.KeyBackspace) {
			newBPM := dv.bpm / 10
			if newBPM == 0 {
				newBPM = 1
			}
			if newBPM != dv.bpm {
				dv.bpm = newBPM
				dv.logger.Debugf("[DRUMVIEW] BPM changed to: %d", dv.bpm)
			}
		}
	}

	/* ——— BPM editing via buttons ——— */
	if dv.bpmIncPressed {
		dv.bpm++
		dv.logger.Infof("[DRUMVIEW] BPM increased to: %d", dv.bpm)
		dv.bpmIncPressed = false
	}
	if dv.bpmDecPressed {
		if dv.bpm > 1 {
			dv.bpm--
			dv.logger.Infof("[DRUMVIEW] BPM decreased to: %d", dv.bpm)
		}
		dv.bpmDecPressed = false
	}

	/* ——— Length editing ——— */
	if dv.lenIncPressed {
		if dv.Length < 64 { // Set a reasonable max length
			dv.Length++
			dv.logger.Infof("[DRUMVIEW] Length increased to: %d", dv.Length)
			dv.Rows[0].Steps = make([]bool, dv.Length) // Update the steps slice
			dv.SetBeatLength(dv.Length)                // Update graph's beat length
			dv.bgDirty = true
		}
		dv.lenIncPressed = false
	}
	if dv.lenDecPressed {
		if dv.Length > 1 { // Prevent length from going below 1
			dv.Length--
			dv.logger.Infof("[DRUMVIEW] Length decreased to: %d", dv.Length)
			dv.Rows[0].Steps = make([]bool, dv.Length) // Update the steps slice
			dv.SetBeatLength(dv.Length)                // Update graph's beat length
			dv.bgDirty = true
		}
		dv.lenDecPressed = false
	}
}

func (dv *DrumView) Draw(dst *ebiten.Image, highlightedBeats map[int]int64, frame int64, beatInfos []model.BeatInfo, elapsedBeats int) {
	dv.logger.Debugf("[DRUMVIEW] Draw called. beatInfos: %v, highlightedBeats: %v", beatInfos, highlightedBeats)
	// draw background
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
	dst.DrawImage(dv.bg(dv.Bounds.Dx(), dv.Bounds.Dy()), op)

	dv.decayAnims()

	PlayButtonStyle.DrawAnimated(dst, dv.playBtn, dv.playPressed, dv.playAnim)
	StopButtonStyle.DrawAnimated(dst, dv.stopBtn, dv.stopPressed, dv.stopAnim)
	BPMDecStyle.DrawAnimated(dst, dv.bpmDecBtn, dv.bpmDecPressed, dv.bpmDecAnim)
	BPMBoxStyle.DrawAnimated(dst, dv.bpmBox, dv.focusBPM, dv.bpmFocusAnim)
	BPMIncStyle.DrawAnimated(dst, dv.bpmIncBtn, dv.bpmIncPressed, dv.bpmIncAnim)
	LenDecStyle.DrawAnimated(dst, dv.lenDecBtn, dv.lenDecPressed, dv.lenDecAnim)
	LenIncStyle.DrawAnimated(dst, dv.lenIncBtn, dv.lenIncPressed, dv.lenIncAnim)
	InstButtonStyle.DrawAnimated(dst, dv.instBtn, false, dv.instAnim)
	UploadBtnStyle.DrawAnimated(dst, dv.uploadBtn, false, dv.uploadAnim)
	ebitenutil.DebugPrintAt(dst, "▶", dv.playBtn.Min.X+30, dv.playBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "■", dv.stopBtn.Min.X+30, dv.stopBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "-", dv.bpmDecBtn.Min.X+15, dv.bpmDecBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, strconv.Itoa(dv.bpm), dv.bpmBox.Min.X+8, dv.bpmBox.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "+", dv.bpmIncBtn.Min.X+15, dv.bpmIncBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "-", dv.lenDecBtn.Min.X+15, dv.lenDecBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "+", dv.lenIncBtn.Min.X+15, dv.lenIncBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, dv.Rows[0].Name+" ▼", dv.instBtn.Min.X+5, dv.instBtn.Min.Y+18)
	ebitenutil.DebugPrintAt(dst, "Upload", dv.uploadBtn.Min.X+5, dv.uploadBtn.Min.Y+18)
	// time/progress
	beatsToDuration := func(beats int) time.Duration {
		return time.Duration(float64(beats) * 60 / float64(dv.bpm) * float64(time.Second))
	}
	totalBeats := dv.Length
	totalDur := beatsToDuration(totalBeats)
	curDur := beatsToDuration(elapsedBeats)
	curMin := int(curDur / time.Minute)
	curSec := int((curDur % time.Minute) / time.Second)
	curMilli := int((curDur % time.Second) / time.Millisecond)
	totMin := int(totalDur / time.Minute)
	totSec := int((totalDur % time.Minute) / time.Second)
	totMilli := int((totalDur % time.Second) / time.Millisecond)
	startBeat := dv.Offset + 1
	endBeat := dv.Offset + dv.Length
	info := fmt.Sprintf("%02d:%02d.%03d/%02d:%02d.%03d | Beats %d-%d/%d",
		curMin, curSec, curMilli, totMin, totSec, totMilli, startBeat, endBeat, totalBeats)
	ebitenutil.DebugPrintAt(dst, info, dv.timelineRect.Min.X, dv.timelineRect.Max.Y+5)
	drawRect(dst, dv.timelineRect, color.RGBA{60, 60, 60, 255}, true)
	prog := float64(elapsedBeats) / float64(totalBeats)
	if prog > 1 {
		prog = 1
	}
	fill := image.Rect(
		dv.timelineRect.Min.X,
		dv.timelineRect.Min.Y,
		dv.timelineRect.Min.X+int(float64(dv.timelineRect.Dx())*prog),
		dv.timelineRect.Max.Y,
	)
	drawRect(dst, fill, color.RGBA{200, 200, 200, 255}, true)
	drawRect(dst, dv.timelineRect, colButtonBorder, false)

	// draw steps
	for i, r := range dv.Rows {
		y := dv.Bounds.Min.Y + timelineHeight + i*dv.rowHeight()
		for j, step := range r.Steps {
			x := dv.Bounds.Min.X + dv.labelW + dv.controlsW + j*dv.cell // Adjusted for buttons
			rect := image.Rect(x, y, x+dv.cell, y+dv.rowHeight())

			// Highlighting logic
			highlighted := false
			isRegularNode := len(beatInfos) > j && beatInfos[j].NodeType == model.NodeTypeRegular

			if _, ok := highlightedBeats[j]; ok {
				highlighted = true
				if isRegularNode {
					dv.logger.Debugf("[DRUMVIEW] Draw: Highlighting regular node at index %d, NodeID: %v", j, beatInfos[j].NodeID)
				} else {
					dv.logger.Debugf("[DRUMVIEW] Draw: Highlighting empty beat at index %d", j)
				}
			}

			DrumCellUI.Draw(dst, rect, step && isRegularNode, highlighted, r.Color)
		}
	}

	if dv.uploading {
		ebitenutil.DebugPrintAt(dst, "Loading...", dv.uploadBtn.Min.X, dv.uploadBtn.Max.Y+20)
	}
	if dv.naming {
		box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
		anim := (math.Sin(dv.namePhase) + 1) * 0.1
		BPMBoxStyle.DrawAnimated(dst, box, true, anim)
		ebitenutil.DebugPrintAt(dst, "Name: "+dv.nameInput, box.Min.X+5, box.Min.Y+18)
		InstButtonStyle.DrawAnimated(dst, dv.saveBtn, dv.savePressed, dv.saveAnim)
		ebitenutil.DebugPrintAt(dst, "Save", dv.saveBtn.Min.X+5, dv.saveBtn.Min.Y+18)
	}
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
